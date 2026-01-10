package cluster

import (
	"context"
	"fmt"
	"net"
	"time"
)

const (
	// talosAPIWaitTimeout is the timeout for waiting for Talos API to become available.
	talosAPIWaitTimeout = 10 * time.Minute
)

// waitForPort waits for a TCP port to be open with the given timeout.
// It polls every 5 seconds until the port accepts connections or the timeout expires.
func waitForPort(ctx context.Context, ip string, port int, timeout time.Duration) error {
	address := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	ticker := time.NewTicker(5 * time.Second)
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
			conn, err := net.DialTimeout("tcp", address, 2*time.Second)
			if err == nil {
				_ = conn.Close()
				return nil
			}
		}
	}
}
