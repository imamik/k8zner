package keygen

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestGenerateRSAKeyPair_ValidBits(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		bits int
	}{
		{"2048 bits", 2048},
		{"4096 bits", 4096},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			keyPair, err := GenerateRSAKeyPair(tt.bits)
			if err != nil {
				t.Fatalf("GenerateRSAKeyPair(%d) failed: %v", tt.bits, err)
			}

			if keyPair == nil {
				t.Fatal("expected non-nil KeyPair")
			}

			if len(keyPair.PrivateKey) == 0 { //nolint:staticcheck // t.Fatal above ensures keyPair is not nil
				t.Error("expected non-empty private key")
			}

			if len(keyPair.PublicKey) == 0 {
				t.Error("expected non-empty public key")
			}
		})
	}
}

func TestGenerateRSAKeyPair_InvalidBits(t *testing.T) {
	t.Parallel(
	// RSA key generation fails for very small bit sizes
	// The minimum practical size varies by implementation
	)

	tests := []struct {
		name string
		bits int
	}{
		{"zero bits", 0},
		{"negative bits", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := GenerateRSAKeyPair(tt.bits)
			if err == nil {
				t.Errorf("GenerateRSAKeyPair(%d) should have failed", tt.bits)
			}
		})
	}
}

func TestKeyPair_PrivateKeyPEMFormat(t *testing.T) {
	t.Parallel()
	keyPair, err := GenerateRSAKeyPair(2048)
	if err != nil {
		t.Fatalf("GenerateRSAKeyPair failed: %v", err)
	}

	// Verify PEM format
	block, rest := pem.Decode(keyPair.PrivateKey)
	if block == nil {
		t.Fatal("failed to decode PEM block")
	}

	if len(rest) > 0 && len(bytes.TrimSpace(rest)) > 0 {
		t.Error("unexpected data after PEM block")
	}

	if block.Type != "RSA PRIVATE KEY" { //nolint:staticcheck // t.Fatal above ensures block is not nil
		t.Errorf("expected PEM type 'RSA PRIVATE KEY', got %q", block.Type)
	}

	// Verify it's a valid PKCS1 private key
	_, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		t.Errorf("failed to parse PKCS1 private key: %v", err)
	}
}

func TestKeyPair_PublicKeySSHFormat(t *testing.T) {
	t.Parallel()
	keyPair, err := GenerateRSAKeyPair(2048)
	if err != nil {
		t.Fatalf("GenerateRSAKeyPair failed: %v", err)
	}

	// Verify OpenSSH authorized_keys format
	pubKeyStr := string(keyPair.PublicKey)

	if !strings.HasPrefix(pubKeyStr, "ssh-rsa ") {
		t.Errorf("public key should start with 'ssh-rsa ', got %q", pubKeyStr[:min(20, len(pubKeyStr))])
	}

	// Should end with newline
	if !strings.HasSuffix(pubKeyStr, "\n") {
		t.Error("public key should end with newline")
	}

	// Verify it can be parsed back
	_, _, _, _, err = ssh.ParseAuthorizedKey(keyPair.PublicKey)
	if err != nil {
		t.Errorf("failed to parse public key as authorized key: %v", err)
	}
}

func TestGenerateRSAKeyPair_Uniqueness(t *testing.T) {
	t.Parallel()
	keyPair1, err := GenerateRSAKeyPair(2048)
	if err != nil {
		t.Fatalf("first GenerateRSAKeyPair failed: %v", err)
	}

	keyPair2, err := GenerateRSAKeyPair(2048)
	if err != nil {
		t.Fatalf("second GenerateRSAKeyPair failed: %v", err)
	}

	if bytes.Equal(keyPair1.PrivateKey, keyPair2.PrivateKey) {
		t.Error("two generated key pairs should have different private keys")
	}

	if bytes.Equal(keyPair1.PublicKey, keyPair2.PublicKey) {
		t.Error("two generated key pairs should have different public keys")
	}
}

func TestGenerateRSAKeyPair_KeyPairCorrespondence(t *testing.T) {
	t.Parallel()
	keyPair, err := GenerateRSAKeyPair(2048)
	if err != nil {
		t.Fatalf("GenerateRSAKeyPair failed: %v", err)
	}

	// Parse the private key
	block, _ := pem.Decode(keyPair.PrivateKey)
	if block == nil {
		t.Fatal("failed to decode private key PEM")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes) //nolint:staticcheck // t.Fatal above ensures block is not nil
	if err != nil {
		t.Fatalf("failed to parse private key: %v", err)
	}

	// Parse the public key
	parsedPubKey, _, _, _, err := ssh.ParseAuthorizedKey(keyPair.PublicKey)
	if err != nil {
		t.Fatalf("failed to parse public key: %v", err)
	}

	// Generate SSH public key from private key
	expectedPubKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("failed to create SSH public key from private: %v", err)
	}

	// Compare the public keys
	if !bytes.Equal(parsedPubKey.Marshal(), expectedPubKey.Marshal()) {
		t.Error("public key does not correspond to private key")
	}
}
