// Package ssh provides SSH communication utilities.
package ssh

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"
)

// Client implements Communicator using the SSH protocol.
type Client struct {
	host       string
	user       string
	privateKey []byte
}

// NewClient creates a new SSHCommunicator.
func NewClient(host, user string, privateKey []byte) *Client {
	return &Client{
		host:       host,
		user:       user,
		privateKey: privateKey,
	}
}

// Execute runs a command on the remote host.
func (c *Client) Execute(ctx context.Context, command string) (string, error) {
	signer, err := ssh.ParsePrivateKey(c.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	config := &ssh.ClientConfig{
		User: c.user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // For now, ignore host key verification
		Timeout:         10 * time.Second,
	}

	var client *ssh.Client
	// Simple retry logic.
	for i := 0; i < 10; i++ {
		client, err = ssh.Dial("tcp", c.host+":22", config)
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
			continue
		}
	}
	if client == nil {
		return "", fmt.Errorf("failed to dial ssh: %w", err)
	}
	defer func() { _ = client.Close() }()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer func() { _ = session.Close() }()

	output, err := session.CombinedOutput(command)
	if err != nil {
		return string(output), fmt.Errorf("failed to execute command: %w, output: %s", err, output)
	}

	return string(output), nil
}

// UploadFile uploads a file to the remote host.
func (c *Client) UploadFile(_ context.Context, _, _ string) error {
	// Not implemented yet.
	return nil
}
