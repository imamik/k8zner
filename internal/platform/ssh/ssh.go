// Package ssh provides SSH client utilities for executing commands on remote servers.
// It handles connection establishment with retry logic, key-based authentication,
// and command execution with context support.
//
// The primary use case is provisioning Talos images on Hetzner Cloud servers
// in rescue mode, where SSH becomes available after a boot sequence.
//
// Security: Host key verification is disabled by default for ephemeral infrastructure.
// Configure HostKeyCallback for production environments with persistent servers.
package ssh

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/imamik/k8zner/internal/util/retry"
)

const (
	defaultPort        = 22
	defaultDialTimeout = 10 * time.Second
	defaultMaxRetries  = 60
	defaultRetryDelay  = 5 * time.Second
	defaultMaxDelay    = 10 * time.Second
)

// Config holds SSH client configuration.
type Config struct {
	Host       string
	Port       int
	User       string
	PrivateKey []byte

	// DialTimeout is the timeout for establishing the TCP connection.
	// If zero, defaultDialTimeout is used.
	DialTimeout time.Duration

	// MaxRetries is the maximum number of connection retry attempts.
	// If zero, defaultMaxRetries is used.
	MaxRetries int

	// RetryDelay is the initial delay between retry attempts.
	// If zero, defaultRetryDelay is used.
	RetryDelay time.Duration

	// HostKeyCallback handles host key verification.
	// If nil, ssh.InsecureIgnoreHostKey() is used (suitable for ephemeral infrastructure).
	// For production environments with persistent servers, provide proper host key verification.
	HostKeyCallback ssh.HostKeyCallback
}

// Client executes commands on a remote server via SSH.
// It parses the private key once during construction and
// creates connections on-demand per Execute call.
type Client struct {
	config *Config
	signer ssh.Signer
}

// NewClient creates a new SSH client and validates the private key.
func NewClient(cfg *Config) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Validate required fields
	if cfg.Host == "" {
		return nil, fmt.Errorf("config host cannot be empty")
	}
	if cfg.User == "" {
		return nil, fmt.Errorf("config user cannot be empty")
	}
	if len(cfg.PrivateKey) == 0 {
		return nil, fmt.Errorf("config private key cannot be empty")
	}

	// Copy config to avoid mutating caller's struct
	configCopy := *cfg

	// Apply defaults to copy
	if configCopy.Port == 0 {
		configCopy.Port = defaultPort
	}
	if configCopy.DialTimeout == 0 {
		configCopy.DialTimeout = defaultDialTimeout
	}
	if configCopy.MaxRetries == 0 {
		configCopy.MaxRetries = defaultMaxRetries
	}
	if configCopy.RetryDelay == 0 {
		configCopy.RetryDelay = defaultRetryDelay
	}
	if configCopy.HostKeyCallback == nil {
		configCopy.HostKeyCallback = ssh.InsecureIgnoreHostKey() //nolint:gosec // Default for ephemeral infrastructure
	}

	// Parse private key once during construction
	signer, err := ssh.ParsePrivateKey(configCopy.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return &Client{
		config: &configCopy,
		signer: signer,
	}, nil
}

// Execute runs a command on the remote host with retry logic.
// Returns command output (stdout+stderr) and any execution error.
func (c *Client) Execute(ctx context.Context, command string) (string, error) {
	client, err := c.connect(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = client.Close() }()

	return c.runCommand(client, command)
}

// connect establishes SSH connection with retry logic.
func (c *Client) connect(ctx context.Context) (*ssh.Client, error) {
	config := &ssh.ClientConfig{
		User: c.config.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(c.signer),
		},
		HostKeyCallback: c.config.HostKeyCallback,
		Timeout:         c.config.DialTimeout,
	}

	addr := fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)
	var client *ssh.Client

	// Use retry package for consistent retry behavior across codebase
	// ARM64 servers can be slow to boot into rescue mode
	err := retry.WithExponentialBackoff(ctx, func() error {
		var dialErr error
		client, dialErr = ssh.Dial("tcp", addr, config)
		return dialErr
	},
		retry.WithMaxRetries(c.config.MaxRetries),
		retry.WithInitialDelay(c.config.RetryDelay),
		retry.WithMaxDelay(defaultMaxDelay),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to establish SSH connection to %s after %d retry attempts: %w",
			addr, c.config.MaxRetries, err)
	}

	return client, nil
}

// runCommand executes a command on an established SSH session.
func (c *Client) runCommand(client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create SSH session on %s: %w", c.config.Host, err)
	}
	defer func() { _ = session.Close() }()

	output, err := session.CombinedOutput(command)
	if err != nil {
		return string(output), fmt.Errorf("command failed on %s: %w\nCommand: %s\nOutput: %s",
			c.config.Host, err, command, string(output))
	}

	return string(output), nil
}
