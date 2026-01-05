package netutil

import (
	"context"
	"fmt"
	"net"
	"time"
)

const (
	// TalosAPIWaitTimeout is the default timeout for waiting for Talos API to become available
	TalosAPIWaitTimeout = 10 * time.Minute
	// KubeAPIWaitTimeout is the default timeout for waiting for Kubernetes API to become available
	KubeAPIWaitTimeout = 10 * time.Minute
)

// WaitForPort waits for a TCP port to be open.
func WaitForPort(ctx context.Context, ip string, port int, timeout time.Duration) error {
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
