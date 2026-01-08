//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"hcloud-k8s/internal/platform/hcloud"
	"hcloud-k8s/internal/util/keygen"
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
// Uses exponential backoff starting at 1s, doubling each time up to 5s max.
func WaitForPort(ctx context.Context, ip string, port int, timeout time.Duration) error {
	address := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	deadline := time.Now().Add(timeout)

	// Start with 1 second, exponential backoff up to 5 seconds
	waitDuration := 1 * time.Second
	maxWait := 5 * time.Second

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for %s", address)
		}

		// Try to connect
		conn, err := net.DialTimeout("tcp", address, 2*time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}

		// Wait before next attempt with exponential backoff
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDuration):
			// Double the wait time for next iteration, cap at maxWait
			waitDuration *= 2
			if waitDuration > maxWait {
				waitDuration = maxWait
			}
		}
	}
}
