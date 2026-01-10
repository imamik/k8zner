package keygen

import (
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestGenerateRSAKeyPair(t *testing.T) {
	// Use 2048 bits for faster tests
	bits := 2048
	kp, err := GenerateRSAKeyPair(bits)
	if err != nil {
		t.Fatalf("GenerateRSAKeyPair failed: %v", err)
	}

	if kp == nil {
		t.Fatal("Returned KeyPair is nil")
	}

	if len(kp.PrivateKey) == 0 {
		t.Error("Private key is empty")
	}

	if len(kp.PublicKey) == 0 {
		t.Error("Public key is empty")
	}

	// 1. Validate Private Key (PEM format)
	block, rest := pem.Decode(kp.PrivateKey)
	if block == nil {
		t.Fatal("Failed to decode PEM block from private key")
	}
	if len(rest) > 0 {
		t.Error("Private key contains extra data")
	}
	if block.Type != "RSA PRIVATE KEY" {
		t.Errorf("Expected PEM type 'RSA PRIVATE KEY', got '%s'", block.Type)
	}

	// Parse the actual RSA key
	privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		t.Errorf("Failed to parse RSA private key: %v", err)
	}
	if privKey.N.BitLen() != bits {
		t.Errorf("Expected %d bits, got %d", bits, privKey.N.BitLen())
	}

	// 2. Validate Public Key (OpenSSH format)
	pubKeyStr := string(kp.PublicKey)
	if !strings.HasPrefix(pubKeyStr, "ssh-rsa ") {
		t.Errorf("Public key should start with 'ssh-rsa', got: %s", pubKeyStr)
	}

	// Parse using ssh package
	parsedKey, _, _, _, err := ssh.ParseAuthorizedKey(kp.PublicKey)
	if err != nil {
		t.Errorf("Failed to parse public key with ssh package: %v", err)
	}
	if parsedKey.Type() != "ssh-rsa" {
		t.Errorf("Expected key type 'ssh-rsa', got '%s'", parsedKey.Type())
	}
}

func TestGenerateRSAKeyPair_InvalidBits(t *testing.T) {
	// Too small key size should typically fail or be rejected by strict implementations,
	// but crypto/rsa might allow it. We mainly want to ensure it doesn't panic.
	// However, let's test a very small valid size just to ensure parameter passing works.
	_, err := GenerateRSAKeyPair(1024)
	if err != nil {
		t.Errorf("GenerateRSAKeyPair(1024) failed: %v", err)
	}
}
