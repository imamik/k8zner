//go:build e2e

package e2e

import (
	"context"
	"fmt"
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

// Cleanup is a no-op since we use t.Cleanup() now.
// Kept for backwards compatibility.
func (rc *ResourceCleaner) Cleanup() {
	// No-op: t.Cleanup() handles everything automatically
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
