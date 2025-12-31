package image

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hcloud-k8s/hcloud-k8s/pkg/ssh"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

type Builder struct {
	Client *hcloud.Client
}

func NewBuilder(token string) *Builder {
	return &Builder{
		Client: hcloud.NewClient(hcloud.WithToken(token)),
	}
}

func (b *Builder) EnsureImage(ctx context.Context, clusterName, talosVersion, schematicID, arch, serverType, location, imageURL string) error {
	// Check if snapshot exists
	schematicIDShort := schematicID
	if len(schematicID) > 32 {
		schematicIDShort = schematicID[:32]
	}

	labels := map[string]string{
		"cluster":            clusterName,
		"os":                 "talos",
		"talos_version":      talosVersion,
		"talos_schematic_id": schematicIDShort,
	}

	var labelParts []string
	for k, v := range labels {
		labelParts = append(labelParts, fmt.Sprintf("%s=%s", k, v))
	}
	labelSelector := strings.Join(labelParts, ",")

	opts := hcloud.ImageListOpts{
		ListOpts:     hcloud.ListOpts{LabelSelector: labelSelector},
		Architecture: []hcloud.Architecture{hcloud.Architecture(arch)},
	}

	images, err := b.Client.Image.AllWithOpts(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to list images: %w", err)
	}

	if len(images) > 0 {
		log.Printf("Image for %s already exists with ID: %d", arch, images[0].ID)
		return nil
	}

	log.Printf("Building image for %s...", arch)
	return b.buildImage(ctx, labels, arch, serverType, location, talosVersion, schematicID, imageURL)
}

func (b *Builder) buildImage(ctx context.Context, labels map[string]string, arch, serverType, location, talosVersion, schematicID, imageURL string) error {
	// 1. Generate SSH Key
	privKey, pubKey, err := ssh.GenerateSSHKey()
	if err != nil {
		return fmt.Errorf("failed to generate ssh key: %w", err)
	}

	// 2. Create SSH Key in Hetzner
	sshKeyName := fmt.Sprintf("packer-%s-%s-%d", labels["cluster"], arch, time.Now().Unix())
	sshKey, _, err := b.Client.SSHKey.Create(ctx, hcloud.SSHKeyCreateOpts{
		Name:      sshKeyName,
		PublicKey: string(pubKey),
	})
	if err != nil {
		return fmt.Errorf("failed to upload ssh key: %w", err)
	}
	defer func() {
		if _, err := b.Client.SSHKey.Delete(ctx, sshKey); err != nil {
			log.Printf("failed to delete ssh key: %v", err)
		}
	}()

	// 3. Create Server
	serverName := fmt.Sprintf("packer-%s-%s-%d", labels["cluster"], arch, time.Now().Unix())
	server, _, err := b.Client.Server.Create(ctx, hcloud.ServerCreateOpts{
		Name:             serverName,
		ServerType:       &hcloud.ServerType{Name: serverType},
		Image:            &hcloud.Image{Name: "debian-12"}, // Base image, we will rescue anyway
		Location:         &hcloud.Location{Name: location},
		SSHKeys:          []*hcloud.SSHKey{sshKey},
		StartAfterCreate: hcloud.Ptr(true),
	})
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}
	defer func() {
		// Ensure server is deleted
		if _, err := b.Client.Server.Delete(ctx, server.Server); err != nil {
			log.Printf("failed to delete server: %v", err)
		}
	}()

	log.Printf("Server %s created (ID: %d), waiting for running...", serverName, server.Server.ID)

	// Wait for server to be running (to get IP)
	action := server.Action
	if err := b.waitForAction(ctx, action); err != nil {
		return fmt.Errorf("failed to wait for server creation: %w", err)
	}

	// Refresh server to get IP
	srv, _, err := b.Client.Server.GetByID(ctx, server.Server.ID)
	if err != nil {
		return fmt.Errorf("failed to get server: %w", err)
	}

	ip := srv.PublicNet.IPv4.IP.String()
	log.Printf("Server IP: %s", ip)

	// 4. Enable Rescue Mode
	log.Printf("Enabling rescue mode...")
	rescueResult, _, err := b.Client.Server.EnableRescue(ctx, srv, hcloud.ServerEnableRescueOpts{
		Type:    "linux64",
		SSHKeys: []*hcloud.SSHKey{sshKey},
	})
	if err != nil {
		return fmt.Errorf("failed to enable rescue: %w", err)
	}
	if err := b.waitForAction(ctx, rescueResult.Action); err != nil {
		return fmt.Errorf("failed to wait for rescue enable: %w", err)
	}
	// Note: Rescue password is in rescueResult.Password if needed, but we use SSH key.

	// 5. Reset Server (Reboot into Rescue)
	log.Printf("Resetting server to boot into rescue...")
	resetAction, _, err := b.Client.Server.Reset(ctx, srv)
	if err != nil {
		return fmt.Errorf("failed to reset server: %w", err)
	}
	if err := b.waitForAction(ctx, resetAction); err != nil {
		return fmt.Errorf("failed to wait for reset: %w", err)
	}

	// 6. Wait for SSH availability
	log.Printf("Waiting for SSH...")
	sshClient, err := ssh.NewClient("root", privKey, ip, 22)
	if err != nil {
		return err
	}

	// Retry SSH connection loop
	for i := 0; i < 30; i++ {
		_, err := sshClient.Run("echo hello")
		if err == nil {
			break
		}
		time.Sleep(5 * time.Second)
		if i == 29 {
			return fmt.Errorf("timeout waiting for ssh: %w", err)
		}
	}

	// 7. Run Install Script
	log.Printf("Writing Talos image to disk...")
	script := fmt.Sprintf(`set -euo pipefail
        blkdiscard -v /dev/sda 2>/dev/null || true
        wget --quiet --timeout=20 --waitretry=5 --tries=5 --retry-connrefused --inet4-only --output-document=- '%s' | xz -T0 -dc | dd of=/dev/sda bs=1M iflag=fullblock oflag=direct conv=fsync status=none
    `, imageURL)

	output, err := sshClient.Run(script)
	if err != nil {
		return fmt.Errorf("failed to write image: %w, output: %s", err, output)
	}

	// 8. Create Snapshot
	log.Printf("Creating snapshot...")
	snapshotName := fmt.Sprintf("Talos Linux %s for %s", strings.ToUpper(arch), labels["cluster"])
	snapshotAction, _, err := b.Client.Server.CreateImage(ctx, srv, &hcloud.ServerCreateImageOpts{
		Type:        hcloud.ImageTypeSnapshot,
		Description: hcloud.Ptr(snapshotName),
		Labels:      labels,
	})
	if err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}
	if err := b.waitForAction(ctx, snapshotAction.Action); err != nil {
		return fmt.Errorf("failed to wait for snapshot creation: %w", err)
	}

	log.Printf("Image created successfully: %d", snapshotAction.Image.ID)
	return nil
}

func (b *Builder) waitForAction(ctx context.Context, action *hcloud.Action) error {
	for {
		act, _, err := b.Client.Action.GetByID(ctx, action.ID)
		if err != nil {
			return err
		}
		if act.Status == hcloud.ActionStatusSuccess {
			return nil
		}
		if act.Status == hcloud.ActionStatusError {
			return act.Error()
		}
		time.Sleep(2 * time.Second)
	}
}
