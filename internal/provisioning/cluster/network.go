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

	// nodeReadyTimeout is the timeout for waiting for a node to become ready after reboot.
	nodeReadyTimeout = 10 * time.Minute

	// kubeconfigTimeout is the timeout for waiting for Kubernetes API to be ready.
	kubeconfigTimeout = 15 * time.Minute

	// nodeReadyPollInterval is the interval for polling node readiness status.
	nodeReadyPollInterval = 10 * time.Second

	// rebootInitialWait is the initial wait time before checking if a node has rebooted.
	rebootInitialWait = 10 * time.Second

	// portPollInterval is the interval for polling port connectivity.
	portPollInterval = 5 * time.Second

	// dialTimeout is the timeout for TCP dial attempts.
	dialTimeout = 2 * time.Second
)

// waitForPort waits for a TCP port to be open with the given timeout.
// It polls at portPollInterval until the port accepts connections or the timeout expires.
func waitForPort(ctx context.Context, ip string, port int, timeout time.Duration) error {
	address := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	ticker := time.NewTicker(portPollInterval)
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
