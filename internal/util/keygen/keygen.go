// Package keygen provides utilities for generating cryptographic key pairs.
//
// This package generates RSA key pairs suitable for SSH authentication,
// outputting the private key in PEM format and the public key in OpenSSH
// authorized_keys format.
package keygen

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"golang.org/x/crypto/ssh"
)

// KeyPair holds an RSA key pair in ready-to-use formats.
type KeyPair struct {
	// PrivateKey is the RSA private key in PEM-encoded PKCS#1 format.
	PrivateKey []byte
	// PublicKey is the public key in OpenSSH authorized_keys format.
	PublicKey []byte
}

// GenerateRSAKeyPair generates a new RSA key pair with the specified bit size.
// Common bit sizes are 2048 (minimum recommended) and 4096 (high security).
func GenerateRSAKeyPair(bits int) (*KeyPair, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA private key: %w", err)
	}

	err = privateKey.Validate()
	if err != nil {
		return nil, fmt.Errorf("failed to validate RSA private key: %w", err)
	}

	privDER := x509.MarshalPKCS1PrivateKey(privateKey)
	privBlock := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privDER,
	}
	privateKeyPEM := pem.EncodeToMemory(&privBlock)

	publicRsaKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH public key: %w", err)
	}

	pubKeyBytes := ssh.MarshalAuthorizedKey(publicRsaKey)

	return &KeyPair{
		PrivateKey: privateKeyPEM,
		PublicKey:  pubKeyBytes,
	}, nil
}
