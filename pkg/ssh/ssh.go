package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"golang.org/x/crypto/ssh"
	"time"
)

type Client struct {
	Config *ssh.ClientConfig
	Host   string
	Port   int
}

func NewClient(user string, privateKey []byte, host string, port int) (*Client, error) {
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // For this task, we ignore host key verification
		Timeout:         10 * time.Second,
	}

	return &Client{
		Config: config,
		Host:   host,
		Port:   port,
	}, nil
}

func (c *Client) Run(command string) (string, error) {
	addr := fmt.Sprintf("%s:%d", c.Host, c.Port)
	conn, err := ssh.Dial("tcp", addr, c.Config)
	if err != nil {
		return "", fmt.Errorf("failed to dial: %w", err)
	}
	defer conn.Close()

	session, err := conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	if err != nil {
		return string(output), fmt.Errorf("failed to run command '%s': %w, output: %s", command, err, string(output))
	}

	return string(output), nil
}

func GenerateSSHKey() (privateKey []byte, publicKey []byte, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	if err := key.Validate(); err != nil {
		return nil, nil, err
	}

	// Private Key
	privDER := x509.MarshalPKCS1PrivateKey(key)
	privBlock := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   privDER,
	}
	privateKey = pem.EncodeToMemory(&privBlock)

	// Public Key
	pub, err := ssh.NewPublicKey(&key.PublicKey)
	if err != nil {
		return nil, nil, err
	}
	publicKey = ssh.MarshalAuthorizedKey(pub)

	return privateKey, publicKey, nil
}
