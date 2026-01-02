// Package hcloud provides a wrapper around the Hetzner Cloud API.
package hcloud

import (
	"context"
)

// ServerProvisioner defines the interface for provisioning servers.
// It abstracts the underlying cloud provider API.
type ServerProvisioner interface {
	// CreateServer creates a new server with the given specifications.
	// It should be idempotent.
	// userData is the cloud-init user data to be passed to the server.
	CreateServer(ctx context.Context, name, imageType, serverType string, sshKeys []string, labels map[string]string, userData string) (string, error)

	// DeleteServer deletes the server with the given name.
	// It should handle the case where the server does not exist.
	DeleteServer(ctx context.Context, name string) error

	// GetServerIP returns the public IP of the server.
	GetServerIP(ctx context.Context, name string) (string, error)
	EnableRescue(ctx context.Context, serverID string, sshKeyIDs []string) (string, error)

	// ResetServer resets (reboots) the server.
	ResetServer(ctx context.Context, serverID string) error

	// PoweroffServer shuts down the server.
	PoweroffServer(ctx context.Context, serverID string) error

	// GetServerID returns the ID of the server by name.
	GetServerID(ctx context.Context, name string) (string, error)
}

// SnapshotManager defines the interface for managing snapshots.
type SnapshotManager interface {
	// CreateSnapshot creates a snapshot of the server.
	CreateSnapshot(ctx context.Context, serverID, snapshotDescription string) (string, error)
	// DeleteImage deletes an image by ID.
	DeleteImage(ctx context.Context, imageID string) error
}

// SSHKeyManager defines the interface for managing SSH keys.
type SSHKeyManager interface {
	// CreateSSHKey creates a new SSH key.
	CreateSSHKey(ctx context.Context, name, publicKey string) (string, error)
	// DeleteSSHKey deletes the SSH key with the given name.
	DeleteSSHKey(ctx context.Context, name string) error
}
