package ipsec

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateKey(t *testing.T) {
	tests := []struct {
		name        string
		keySize     int
		wantBytes   int // Expected number of bytes (before hex encoding)
		wantErr     bool
		errContains string
	}{
		{
			name:      "128-bit key",
			keySize:   128,
			wantBytes: 20, // 16 + 4 salt
		},
		{
			name:      "192-bit key",
			keySize:   192,
			wantBytes: 28, // 24 + 4 salt
		},
		{
			name:      "256-bit key",
			keySize:   256,
			wantBytes: 36, // 32 + 4 salt
		},
		{
			name:        "invalid key size",
			keySize:     512,
			wantErr:     true,
			errContains: "invalid key size",
		},
		{
			name:        "zero key size",
			keySize:     0,
			wantErr:     true,
			errContains: "invalid key size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyHex, err := GenerateKey(tt.keySize)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}

			require.NoError(t, err)

			// Verify it's valid hex
			keyBytes, err := hex.DecodeString(keyHex)
			require.NoError(t, err, "key should be valid hex")

			// Verify length
			assert.Len(t, keyBytes, tt.wantBytes, "key should have correct length")

			// Verify randomness (generate another key and ensure they're different)
			keyHex2, err := GenerateKey(tt.keySize)
			require.NoError(t, err)
			assert.NotEqual(t, keyHex, keyHex2, "generated keys should be unique")
		})
	}
}

func TestFormatKeyForSecret(t *testing.T) {
	cfg := KeyConfig{
		KeyID:     3,
		Algorithm: "rfc4106(gcm(aes))",
		KeySize:   256,
		KeyHex:    "abcdef123456",
	}

	result := FormatKeyForSecret(cfg)
	expected := "3+ rfc4106(gcm(aes)) abcdef123456 256"

	assert.Equal(t, expected, result)
}

func TestCreateSecretManifest(t *testing.T) {
	cfg := KeyConfig{
		KeyID:     1,
		Algorithm: DefaultAlgorithm,
		KeySize:   256,
		KeyHex:    "0123456789abcdef",
	}

	manifest, err := CreateSecretManifest(cfg)
	require.NoError(t, err)

	manifestStr := string(manifest)

	// Verify structure
	assert.Contains(t, manifestStr, "apiVersion: v1")
	assert.Contains(t, manifestStr, "kind: Secret")
	assert.Contains(t, manifestStr, "type: Opaque")
	assert.Contains(t, manifestStr, "name: cilium-ipsec-keys")
	assert.Contains(t, manifestStr, "namespace: kube-system")

	// Verify annotations
	assert.Contains(t, manifestStr, "cilium.io/key-id")
	assert.Contains(t, manifestStr, "cilium.io/key-algorithm")
	assert.Contains(t, manifestStr, "cilium.io/key-size")

	// Verify key format in base64
	expectedKeyFormat := FormatKeyForSecret(cfg)
	expectedBase64 := base64.StdEncoding.EncodeToString([]byte(expectedKeyFormat))
	assert.Contains(t, manifestStr, expectedBase64)
}

func TestKeyConfigDefaults(t *testing.T) {
	assert.Equal(t, "rfc4106(gcm(aes))", DefaultAlgorithm)
	assert.Equal(t, 256, DefaultKeySize)
	assert.Equal(t, 1, DefaultKeyID)
}

func TestGenerateKeyUniqueness(t *testing.T) {
	// Generate multiple keys and ensure they're all unique
	keys := make(map[string]bool)
	numKeys := 100

	for i := 0; i < numKeys; i++ {
		key, err := GenerateKey(256)
		require.NoError(t, err)

		if keys[key] {
			t.Errorf("duplicate key generated: %s", key)
		}
		keys[key] = true
	}
}

func TestCreateSecretManifestYAMLValidity(t *testing.T) {
	cfg := KeyConfig{
		KeyID:     1,
		Algorithm: DefaultAlgorithm,
		KeySize:   256,
		KeyHex:    "test-key-hex",
	}

	manifest, err := CreateSecretManifest(cfg)
	require.NoError(t, err)

	// Should be valid YAML (no parsing errors)
	assert.True(t, len(manifest) > 0)
	assert.True(t, strings.HasPrefix(string(manifest), "apiVersion"))
}
