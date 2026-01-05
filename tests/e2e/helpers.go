//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/keygen"
)

// ResourceCleaner helps track and clean up resources.
// Uses t.Cleanup() to ensure cleanup runs even on test timeout.
type ResourceCleaner struct {
	t *testing.T
}

// Add adds a cleanup function that will run even if the test times out.
// Functions are executed in LIFO order (last added, first executed).
func (rc *ResourceCleaner) Add(f func()) {
	rc.t.Cleanup(f)
}

// setupSSHKey generates a temporary SSH key, uploads it to HCloud, and registers cleanup.
// It returns the key name and private key bytes.
func setupSSHKey(t *testing.T, client *hcloud.RealClient, cleaner *ResourceCleaner, prefix string) (string, []byte) {
	keyName := fmt.Sprintf("%s-key-%d", prefix, time.Now().UnixNano())
	t.Logf("Generating SSH key %s...", keyName)

	keyPair, err := keygen.GenerateRSAKeyPair(2048)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	_, err = client.CreateSSHKey(context.Background(), keyName, string(keyPair.PublicKey))
	if err != nil {
		t.Fatalf("Failed to upload ssh key: %v", err)
	}

	cleaner.Add(func() {
		t.Logf("Deleting SSH key %s...", keyName)
		if err := client.DeleteSSHKey(context.Background(), keyName); err != nil {
			t.Logf("Failed to delete ssh key %s (might not exist): %v", keyName, err)
		}
	})

	return keyName, keyPair.PrivateKey
}

// WaitForPort waits for a TCP port to become accessible.
func WaitForPort(ctx context.Context, ip string, port int, timeout time.Duration) error {
	address := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	deadline := time.Now().Add(timeout)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for %s", address)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", address, 2*time.Second)
			if err == nil {
				_ = conn.Close()
				return nil
			}
			// Continue waiting
		}
	}
}
