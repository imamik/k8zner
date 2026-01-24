// Package image provides the logic for building Talos disk images on Hetzner Cloud.
// This file handles image building operations.
package image

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/platform/ssh"
	"github.com/imamik/k8zner/internal/util/keygen"
)

// Builder builds a Talos image on Hetzner Cloud.
type Builder struct {
	infra hcloud.InfrastructureManager
}

// NewBuilder creates a new Builder.
func NewBuilder(infra hcloud.InfrastructureManager) *Builder {
	return &Builder{
		infra: infra,
	}
}

// Build creates a temporary server, installs Talos, creates a snapshot, and cleans up.
// serverType and location can be empty to use defaults (architecture-appropriate server type, nbg1 location).
func (b *Builder) Build(ctx context.Context, talosVersion, k8sVersion, architecture, serverType, location string, labels map[string]string) (string, error) {
	// Generate image name from versions and architecture
	imageName := fmt.Sprintf("talos-%s-k8s-%s-%s", talosVersion, k8sVersion, architecture)
	serverName := fmt.Sprintf("build-%s-%s", imageName, time.Now().Format("20060102150405"))

	// Default to nbg1 if no location specified
	if location == "" {
		location = "nbg1"
	}

	// Default to architecture-appropriate server type if not specified
	if serverType == "" {
		serverType = hcloud.GetDefaultServerType(hcloud.Architecture(architecture))
	}

	// 0. Setup SSH Key.
	keyName := fmt.Sprintf("key-%s", serverName)
	log.Printf("Generating SSH key %s...", keyName)
	keyPair, err := keygen.GenerateRSAKeyPair(2048)
	if err != nil {
		return "", fmt.Errorf("failed to generate key pair: %w", err)
	}

	if b.infra == nil {
		return "", fmt.Errorf("InfrastructureManager is required")
	}

	// Merge labels for SSH key (labels already include test-id if present)
	sshKeyLabels := make(map[string]string)
	for k, v := range labels {
		sshKeyLabels[k] = v
	}
	sshKeyLabels["type"] = "build-ssh-key"

	sshKeyID, err := b.infra.CreateSSHKey(ctx, keyName, string(keyPair.PublicKey), sshKeyLabels)
	if err != nil {
		return "", fmt.Errorf("failed to upload ssh key: %w", err)
	}

	defer b.cleanupSSHKey(keyName)

	// 1. Create Server
	log.Printf("Creating server %s in location %s...", serverName, location)

	// We need to pass the ssh key NAME to CreateServer
	sshKeys := []string{keyName}

	defer func() {
		b.cleanupServer(serverName)
	}()

	serverID, err := b.infra.CreateServer(ctx, serverName, "debian-13", serverType, location, sshKeys, labels, "", nil, 0, "")
	if err != nil {
		return "", fmt.Errorf("failed to create server: %w", err)
	}

	ip, err := b.infra.GetServerIP(ctx, serverName)
	if err != nil {
		return "", fmt.Errorf("failed to get server IP: %w", err)
	}
	log.Printf("Server IP: %s", ip)

	// 2. Enable Rescue Mode.
	log.Printf("Enabling Rescue Mode...")
	// We use the same SSH key for rescue mode.
	// EnableRescue expects SSHKey IDs if using API v2 logic?
	// Our wrapper `EnableRescue` takes `[]string` which are expected to be IDs (based on hcloud-go opts).
	// We got `sshKeyID` from `CreateSSHKey`.
	_, err = b.infra.EnableRescue(ctx, serverID, []string{sshKeyID})
	if err != nil {
		return "", fmt.Errorf("failed to enable rescue: %w", err)
	}

	// 3. Reset Server (boot into rescue).
	log.Printf("Resetting server to boot into Rescue Mode...")
	if err := b.infra.ResetServer(ctx, serverID); err != nil {
		return "", fmt.Errorf("failed to reset server: %w", err)
	}

	// 4. Provision Talos (SSH)
	// We need to wait for SSH to come back up. The SSH client handles retries,
	// but after a reboot it might take a moment.
	log.Printf("Waiting for Rescue System...")
	time.Sleep(10 * time.Second) // Give it a head start.

	client, err := ssh.NewClient(&ssh.Config{
		Host:       ip,
		User:       "root",
		PrivateKey: keyPair.PrivateKey,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create SSH client: %w", err)
	}

	// URL generation.
	var talosURL string
	switch architecture {
	case "amd64", "arm64":
		talosURL = fmt.Sprintf("https://github.com/siderolabs/talos/releases/download/%s/metal-%s.raw.zst", talosVersion, architecture)
	default:
		talosURL = fmt.Sprintf("https://github.com/siderolabs/talos/releases/download/%s/talos-%s-%s.raw.zst", talosVersion, architecture, architecture)
	}

	installCmd := fmt.Sprintf("DISK=$(lsblk -d -n -o NAME | grep -E '^sda|^vda' | head -n 1) && if [ -z \"$DISK\" ]; then echo 'No disk found'; exit 1; fi && echo \"Writing to /dev/$DISK\" && apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y zstd wget && wget -qO- %s | zstd -d | dd of=/dev/$DISK bs=4M && sync", talosURL)

	log.Printf("Provisioning Talos (Command: %s)...", installCmd)
	output, err := client.Execute(ctx, installCmd)
	if err != nil {
		return "", fmt.Errorf("failed to provision talos: %w, output: %s", err, output)
	}

	// 5. Poweroff Servep.
	log.Printf("Powering off server for snapshot...")
	if err := b.infra.PoweroffServer(ctx, serverID); err != nil {
		return "", fmt.Errorf("failed to poweroff server: %w", err)
	}

	// 6. Create Snapshot.
	log.Printf("Creating snapshot...")
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["os"] = "talos"
	labels["talos-version"] = talosVersion
	labels["k8s-version"] = k8sVersion
	labels["arch"] = architecture

	snapshotID, err := b.infra.CreateSnapshot(ctx, serverID, imageName, labels)
	if err != nil {
		return "", fmt.Errorf("failed to create snapshot: %w", err)
	}

	return snapshotID, nil
}

func (b *Builder) cleanupServer(serverName string) {
	log.Printf("Cleaning up server %s...", serverName)
	// DeleteServer now has built-in retry logic and timeout (5 minutes default)
	// from Phase 2 improvements, so we can simply call it
	ctx := context.Background()
	err := b.infra.DeleteServer(ctx, serverName)
	if err != nil {
		log.Printf("Failed to delete server %s: %v", serverName, err)
	} else {
		log.Printf("Server %s deleted successfully", serverName)
	}
}

func (b *Builder) cleanupSSHKey(keyName string) {
	log.Printf("Cleaning up SSH key %s...", keyName)
	// DeleteSSHKey now has built-in retry logic and timeout (5 minutes default)
	// from Phase 2 improvements, so we can simply call it
	ctx := context.Background()
	err := b.infra.DeleteSSHKey(ctx, keyName)
	if err != nil {
		log.Printf("Failed to delete SSH key %s: %v", keyName, err)
	} else {
		log.Printf("SSH key %s deleted successfully", keyName)
	}
}
