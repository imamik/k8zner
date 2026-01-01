//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/image"
	"github.com/sak-d/hcloud-k8s/internal/ssh"
)

func TestImageBuildLifecycle(t *testing.T) {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping e2e test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	// 1. Setup
	client := hcloud.NewRealClient(token)

	sshKeyName := os.Getenv("HCLOUD_SSH_KEY_NAME")
	sshKeyPath := os.Getenv("HCLOUD_SSH_KEY_PATH")

	var privateKey []byte
	var commFactory image.CommunicatorFactory

	if sshKeyName != "" && sshKeyPath != "" {
		pk, err := os.ReadFile(sshKeyPath)
		if err != nil {
			t.Fatalf("failed to read ssh key: %v", err)
		}
		privateKey = pk
		commFactory = func(host string) ssh.Communicator {
			return ssh.NewSSHCommunicator(host, "root", privateKey)
		}
	} else {
		// If no keys provided, we rely on the builder generating them.
		// So we pass nil factory.
		commFactory = nil
	}

	builder := image.NewBuilder(client, commFactory)

	// 2. Execute Build
	imageName := "e2e-test-image-" + time.Now().Format("20060102-150405")

	// Using generic args for test
	labels := map[string]string{
		"type":        "e2e-test",
		"created_by":  "hcloud-k8s-e2e",
		"test_name":   "TestImageBuildLifecycle",
	}
	snapshotID, err := builder.Build(ctx, imageName, "v1.12.0", "amd64", labels)

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	} else {
		t.Logf("Build successful, snapshot ID: %s", snapshotID)

		// Cleanup snapshot
		t.Logf("Deleting snapshot %s...", snapshotID)
		if err := client.DeleteImage(ctx, snapshotID); err != nil {
			t.Errorf("Failed to delete snapshot %s: %v", snapshotID, err)
		}
	}
}
