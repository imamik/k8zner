// Package image provides the logic for building Talos disk images on Hetzner Cloud.
package image

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/keygen"
	"github.com/sak-d/hcloud-k8s/internal/ssh"
)

// CommunicatorFactory creates a Communicator for a given host.
type CommunicatorFactory func(host string) ssh.Communicator

// Builder builds a Talos image on Hetzner Cloud.
type Builder struct {
	provisioner      hcloud.ServerProvisioner
	snapshotManager  hcloud.SnapshotManager
	communicatorFact CommunicatorFactory
	sshKeyManager    hcloud.SSHKeyManager
}

// NewBuilder creates a new Builder.
func NewBuilder(client interface{}, commFact CommunicatorFactory) *Builder {
	p, _ := client.(hcloud.ServerProvisioner)
	s, _ := client.(hcloud.SnapshotManager)
	k, _ := client.(hcloud.SSHKeyManager)

	return &Builder{
		provisioner:      p,
		snapshotManager:  s,
		communicatorFact: commFact,
		sshKeyManager:    k,
	}
}

// Build creates a temporary server, installs Talos, creates a snapshot, and cleans up.
func (b *Builder) Build(ctx context.Context, imageName, talosVersion, architecture string, labels map[string]string) (string, error) {
	serverName := fmt.Sprintf("build-%s-%s", imageName, time.Now().Format("20060102150405"))

	// 0. Setup SSH Key.
	keyName := fmt.Sprintf("key-%s", serverName)
	log.Printf("Generating SSH key %s...", keyName)
	keyPair, err := keygen.GenerateRSAKeyPair(2048)
	if err != nil {
		return "", fmt.Errorf("failed to generate key pair: %w", err)
	}

	if b.sshKeyManager == nil {
		return "", fmt.Errorf("SSHKeyManager is required")
	}

	sshKeyID, err := b.sshKeyManager.CreateSSHKey(ctx, keyName, string(keyPair.PublicKey))
	if err != nil {
		return "", fmt.Errorf("failed to upload ssh key: %w", err)
	}

	defer func() {
		log.Printf("Deleting SSH key %s...", keyName)
		if err := b.sshKeyManager.DeleteSSHKey(context.Background(), keyName); err != nil {
			log.Printf("Failed to delete ssh key %s: %v", keyName, err)
		}
	}()

	// 1. Create Server.
	log.Printf("Creating server %s...", serverName)

	// We need to pass the ssh key NAME to CreateServer.
	sshKeys := []string{keyName}

	serverType := "cx23"
	if architecture == "arm64" {
		serverType = "cax11"
	}

	defer func() {
		log.Printf("Deleting server %s...", serverName)
		if err := b.provisioner.DeleteServer(context.Background(), serverName); err != nil {
			log.Printf("Failed to delete server %s: %v", serverName, err)
		}
	}()

	serverID, err := b.provisioner.CreateServer(ctx, serverName, "debian-12", serverType, "", sshKeys, labels, "", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create server: %w", err)
	}

	ip, err := b.provisioner.GetServerIP(ctx, serverName)
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
	_, err = b.provisioner.EnableRescue(ctx, serverID, []string{sshKeyID})
	if err != nil {
		return "", fmt.Errorf("failed to enable rescue: %w", err)
	}

	// 3. Reset Server (boot into rescue).
	log.Printf("Resetting server to boot into Rescue Mode...")
	if err := b.provisioner.ResetServer(ctx, serverID); err != nil {
		return "", fmt.Errorf("failed to reset server: %w", err)
	}

	// 4. Provision Talos (SSH)
	// We need to wait for SSH to come back up. The `SSHCommunicator` handles retries,
	// but after a reboot it might take a moment.
	log.Printf("Waiting for Rescue System...")
	time.Sleep(10 * time.Second) // Give it a head start.

	var comm ssh.Communicator
	if b.communicatorFact != nil {
		comm = b.communicatorFact(ip)
	} else {
		comm = ssh.NewClient(ip, "root", keyPair.PrivateKey)
	}

	// URL generation.
	talosURL := fmt.Sprintf("https://github.com/siderolabs/talos/releases/download/%s/talos-%s-%s.raw.zst", talosVersion, architecture, architecture)
	if architecture == "amd64" {
		talosURL = fmt.Sprintf("https://github.com/siderolabs/talos/releases/download/%s/metal-%s.raw.zst", talosVersion, architecture)
	} else if architecture == "arm64" {
		talosURL = fmt.Sprintf("https://github.com/siderolabs/talos/releases/download/%s/metal-%s.raw.zst", talosVersion, architecture)
	}

	installCmd := fmt.Sprintf("DISK=$(lsblk -d -n -o NAME | grep -E '^sda|^vda' | head -n 1) && if [ -z \"$DISK\" ]; then echo 'No disk found'; exit 1; fi && echo \"Writing to /dev/$DISK\" && apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y zstd wget && wget -qO- %s | zstd -d | dd of=/dev/$DISK bs=4M && sync", talosURL)

	log.Printf("Provisioning Talos (Command: %s)...", installCmd)
	output, err := comm.Execute(ctx, installCmd)
	if err != nil {
		return "", fmt.Errorf("failed to provision talos: %w, output: %s", err, output)
	}

	// 5. Poweroff Server.
	log.Printf("Powering off server for snapshot...")
	if err := b.provisioner.PoweroffServer(ctx, serverID); err != nil {
		return "", fmt.Errorf("failed to poweroff server: %w", err)
	}

	// 6. Create Snapshot.
	log.Printf("Creating snapshot...")
	snapshotID, err := b.snapshotManager.CreateSnapshot(ctx, serverID, imageName)
	if err != nil {
		return "", fmt.Errorf("failed to create snapshot: %w", err)
	}

	return snapshotID, nil
}
