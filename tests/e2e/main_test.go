//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/image"
	"github.com/sak-d/hcloud-k8s/internal/keygen"
	"github.com/sak-d/hcloud-k8s/internal/ssh"
)

func TestImageBuildLifecycle(t *testing.T) {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping e2e test")
	}

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
			return ssh.NewClient(host, "root", privateKey)
		}
	} else {
		// If no keys provided, we rely on the builder generating them.
		// So we pass nil factory.
		commFactory = nil
	}

	builder := image.NewBuilder(client, commFactory)

	tests := []struct {
		name string
		arch string
	}{
		{name: "amd64", arch: "amd64"},
		{name: "arm64", arch: "arm64"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()

			// Unique image name per test
			imageName := fmt.Sprintf("e2e-test-image-%s-%s", tc.arch, time.Now().Format("20060102-150405"))

			labels := map[string]string{
				"type":        "e2e-test",
				"created_by":  "hcloud-k8s-e2e",
				"test_name":   "TestImageBuildLifecycle",
				"arch":        tc.arch,
			}

			t.Logf("Starting build for %s...", tc.arch)
			snapshotID, err := builder.Build(ctx, imageName, "v1.12.0", tc.arch, labels)

			if err != nil {
				t.Fatalf("Build failed for %s: %v", tc.arch, err)
			}
			t.Logf("Build successful for %s, snapshot ID: %s", tc.arch, snapshotID)

			// Cleanup Snapshot Defer
			defer func() {
				t.Logf("Deleting snapshot %s...", snapshotID)
				if err := client.DeleteImage(context.Background(), snapshotID); err != nil {
					t.Errorf("Failed to delete snapshot %s: %v", snapshotID, err)
				}
			}()

			// 3. Verify Server Creation
			verifyServerName := fmt.Sprintf("verify-%s-%s", tc.arch, time.Now().Format("20060102-150405"))
			t.Logf("Creating verification server %s...", verifyServerName)

			serverType := "cx23"
			if tc.arch == "arm64" {
				serverType = "cax11"
			}

			verifyLabels := map[string]string{
				"type":       "e2e-verify",
				"created_by": "hcloud-k8s-e2e",
				"test_name":  "TestImageBuildLifecycle",
				"arch":       tc.arch,
			}

			// Create a temporary SSH key for verification to prevent root password emails
			verifyKeyName := fmt.Sprintf("key-%s", verifyServerName)
			verifyKeyData, err := keygen.GenerateRSAKeyPair(2048)
			if err != nil {
				t.Fatalf("Failed to generate key pair: %v", err)
			}

			_, err = client.CreateSSHKey(ctx, verifyKeyName, string(verifyKeyData.PublicKey))
			if err != nil {
				t.Fatalf("Failed to upload ssh key: %v", err)
			}

			defer func() {
				t.Logf("Deleting SSH key %s...", verifyKeyName)
				if err := client.DeleteSSHKey(context.Background(), verifyKeyName); err != nil {
					t.Errorf("Failed to delete ssh key %s: %v", verifyKeyName, err)
				}
			}()

			defer func() {
				t.Logf("Deleting verification server %s...", verifyServerName)
				if err := client.DeleteServer(context.Background(), verifyServerName); err != nil {
					t.Errorf("Failed to delete verification server: %v", err)
				}
			}()

			// We pass the ssh key to prevent password emails
			_, err = client.CreateServer(ctx, verifyServerName, snapshotID, serverType, "", []string{verifyKeyName}, verifyLabels, "", nil)
			if err != nil {
				t.Fatalf("Failed to create verification server: %v", err)
			}

			ip, err := client.GetServerIP(ctx, verifyServerName)
			if err != nil {
				t.Fatalf("Failed to get verification server IP: %v", err)
			}
			t.Logf("Verification server IP: %s", ip)

			// 4. Verify Talos is running (Port 50000)
			t.Logf("Waiting for Talos API (port 50000) on %s...", ip)

			timeout := time.After(5 * time.Minute)
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()

			success := false
			for {
				select {
				case <-timeout:
					t.Fatalf("Timeout waiting for Talos API on %s", ip)
				case <-ticker.C:
					// Simple TCP dial to check if port is open
					// We can use net.Dial
					conn, err := net.Dial("tcp", ip+":50000") // reusing ssh package? No, import net
					if err == nil {
						conn.Close()
						t.Logf("Successfully connected to Talos API on %s:50000", ip)
						success = true
						goto VerifyDone
					}
				}
			}
		VerifyDone:
			if !success {
				t.Fatalf("Failed to verify Talos API")
			}
		})
	}
}
