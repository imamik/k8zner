package ssh

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHCommunicator implements Communicator using the SSH protocol.
type SSHCommunicator struct {
	host       string
	user       string
	privateKey []byte
}

// NewSSHCommunicator creates a new SSHCommunicator.
func NewSSHCommunicator(host, user string, privateKey []byte) *SSHCommunicator {
	return &SSHCommunicator{
		host:       host,
		user:       user,
		privateKey: privateKey,
	}
}

func (c *SSHCommunicator) Execute(ctx context.Context, command string) (string, error) {
	signer, err := ssh.ParsePrivateKey(c.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	config := &ssh.ClientConfig{
		User: c.user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // For now, ignore host key verification
		Timeout:         10 * time.Second,
	}

	var client *ssh.Client
	// Simple retry logic
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
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	if err != nil {
		return string(output), fmt.Errorf("failed to execute command: %w, output: %s", err, output)
	}

	return string(output), nil
}

func (c *SSHCommunicator) UploadFile(ctx context.Context, localPath, remotePath string) error {
	// Not implemented yet
	return nil
}
