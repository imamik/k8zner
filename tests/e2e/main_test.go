//go:build e2e

package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
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
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel() // Run in parallel

			// Setup cleaner for this subtest
			cleaner := &ResourceCleaner{t: t}
			defer cleaner.Cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()

			cleaner := &ResourceCleaner{t: t}
			defer cleaner.Cleanup()

			// Unique image name per test
			imageName := fmt.Sprintf("e2e-test-image-%s-%s", tc.arch, time.Now().Format("20060102-150405"))

			labels := map[string]string{
				"type":       "e2e-test",
				"created_by": "hcloud-k8s-e2e",
				"test_name":  "TestImageBuildLifecycle",
				"arch":       tc.arch,
			}

			t.Logf("Starting build for %s...", tc.arch)
			snapshotID, err := builder.Build(ctx, imageName, "v1.12.0", tc.arch, labels)

			// If snapshot was created, we must clean it up.
			// Even if Build returned error, it might have created a snapshot (unlikely with our fix, but good practice).
			// Since we don't have ID if it fails, we assume builder cleanup or fix in client handles it.
			// If success, we track it.
			if snapshotID != "" {
				cleaner.Add(func() {
					t.Logf("Deleting snapshot %s...", snapshotID)
					if err := client.DeleteImage(context.Background(), snapshotID); err != nil {
						t.Errorf("Failed to delete snapshot %s: %v", snapshotID, err)
					}
				})
			}

			if err != nil {
				t.Fatalf("Build failed for %s: %v", tc.arch, err)
			}
			t.Logf("Build successful for %s, snapshot ID: %s", tc.arch, snapshotID)

			// 3. Verify Server Creation
			verifyServerName := fmt.Sprintf("verify-%s-%s", tc.arch, time.Now().Format("20060102-150405"))

			// Register cleanup for server immediately, so even if CreateServer fails partially or we crash, we try to delete it.
			cleaner.Add(func() {
				t.Logf("Deleting verification server %s...", verifyServerName)
				if err := client.DeleteServer(context.Background(), verifyServerName); err != nil {
					// It's okay if it doesn't exist.
					t.Logf("Failed to delete verification server (might not exist): %v", err)
				}
			})

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

			// Setup SSH Key for verification
			verifyKeyName, _ := setupSSHKey(t, client, cleaner, verifyServerName)

			// We pass the ssh key to prevent password emails
			// Updated CreateServer signature: ..., placementGroupID, networkID, privateIP
			_, err = client.CreateServer(ctx, verifyServerName, snapshotID, serverType, "", []string{verifyKeyName}, verifyLabels, "", nil, 0, "")
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
					// Check connectivity and TLS handshake.
					// Talos uses mTLS, so a normal handshake without client certs will fail during verification or return an error,
					// but establishing the handshake proves the server is listening and speaking TLS.
					conf := &tls.Config{
						InsecureSkipVerify: true, // We don't have the CA, we just want to know if it's speaking TLS.
					}
					conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", ip+":50000", conf)
					if err == nil {
						conn.Close()
						t.Logf("Successfully performed TLS handshake with Talos API on %s:50000", ip)
						success = true
						goto VerifyDone
					} else {
						// Log the error for debugging (it might be connection refused, or timeout)
						t.Logf("Waiting for Talos API... last error: %v", err)
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
