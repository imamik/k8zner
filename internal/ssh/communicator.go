package ssh

import (
	"context"
)

// Communicator defines the interface for executing commands on a remote server.
type Communicator interface {
	// Execute runs a command on the remote server and returns the output.
	// It should handle retries and connection establishment.
	Execute(ctx context.Context, command string) (string, error)
	// UploadFile uploads a file to the remote server.
	UploadFile(ctx context.Context, localPath, remotePath string) error
}
