package cluster

import (
	"context"
	"fmt"
	"net"
	"time"
)

// Default timeouts used when not overridden by config.
// These are kept for backward compatibility with code that doesn't use provisioning.Context.
const (
	// defaultRebootInitialWait is the default wait time before checking if a node has rebooted.
	defaultRebootInitialWait = 10 * time.Second
)

// waitForPort waits for a TCP port to be open with the given timeout.
// It polls at the given interval until the port accepts connections or the timeout expires.
func waitForPort(ctx context.Context, ip string, port int, timeout, pollInterval, dialTimeout time.Duration) error {
	address := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("timeout waiting for %s", address)
			}
			return ctx.Err()
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", address, dialTimeout)
			if err == nil {
				_ = conn.Close()
				return nil
			}
		}
	}
}
