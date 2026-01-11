// Package ipsec provides IPSec key generation utilities for Cilium encryption.
package ipsec

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"gopkg.in/yaml.v3"
)

// KeyConfig holds IPSec key configuration.
type KeyConfig struct {
	KeyID     int    // 1-15
	Algorithm string // e.g., "rfc4106(gcm(aes))"
	KeySize   int    // 128, 192, or 256
	KeyHex    string // Hex-encoded key
}

// GenerateKey generates a random IPSec key for the given key size.
// Key length is KeySize/8 + 4 bytes for AES-GCM (key + salt).
func GenerateKey(keySize int) (string, error) {
	if keySize != 128 && keySize != 192 && keySize != 256 {
		return "", fmt.Errorf("invalid key size %d: must be 128, 192, or 256", keySize)
	}

	// Key bytes = AES key size (in bytes) + 4 bytes salt for GCM
	keyBytes := (keySize / 8) + 4

	key := make([]byte, keyBytes)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("failed to generate random key: %w", err)
	}

	return hex.EncodeToString(key), nil
}

// FormatKeyForSecret formats the key for the Cilium IPSec secret.
// Format: "keyID+ algorithm keyHex keySize"
func FormatKeyForSecret(cfg KeyConfig) string {
	return fmt.Sprintf("%d+ %s %s %d", cfg.KeyID, cfg.Algorithm, cfg.KeyHex, cfg.KeySize)
}

// CreateSecretManifest creates the Kubernetes Secret manifest YAML for IPSec keys.
func CreateSecretManifest(cfg KeyConfig) ([]byte, error) {
	keyFormat := FormatKeyForSecret(cfg)

	secret := map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"type":       "Opaque",
		"metadata": map[string]any{
			"name":      "cilium-ipsec-keys",
			"namespace": "kube-system",
			"annotations": map[string]string{
				"cilium.io/key-id":        fmt.Sprintf("%d", cfg.KeyID),
				"cilium.io/key-algorithm": cfg.Algorithm,
				"cilium.io/key-size":      fmt.Sprintf("%d", cfg.KeySize),
			},
		},
		"data": map[string]string{
			"keys": base64.StdEncoding.EncodeToString([]byte(keyFormat)),
		},
	}

	return yaml.Marshal(secret)
}

// DefaultAlgorithm is the default IPSec algorithm.
const DefaultAlgorithm = "rfc4106(gcm(aes))"

// DefaultKeySize is the default key size in bits.
const DefaultKeySize = 256

// DefaultKeyID is the default key ID.
const DefaultKeyID = 1
