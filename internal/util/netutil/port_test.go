package netutil

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestWaitForPort_Success(t *testing.T) {
	// Start a listener on a random port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}
	defer ln.Close()

	// Get the port chosen by the kernel
	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("Failed to split host/port: %v", err)
	}

	var port int
	fmt.Sscanf(portStr, "%d", &port)

	ctx := context.Background()
	// Should connect immediately
	err = WaitForPort(ctx, "127.0.0.1", port, 2*time.Second)
	if err != nil {
		t.Errorf("WaitForPort failed for open port: %v", err)
	}
}

func TestWaitForPort_Timeout(t *testing.T) {
	// Pick a port that is unlikely to be in use (and don't listen on it)
	// Using a closed port on localhost usually results in immediate connection refusal,
	// which WaitForPort should retry until timeout.
	port := 45678

	ctx := context.Background()
	start := time.Now()
	timeout := 200 * time.Millisecond

	err := WaitForPort(ctx, "127.0.0.1", port, timeout)

	elapsed := time.Since(start)

	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	if elapsed < timeout {
		t.Errorf("Returned before timeout: %v < %v", elapsed, timeout)
	}
}

func TestWaitForPort_DelayedStart(t *testing.T) {
	// Let kernel pick a free port first
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to pick free port: %v", err)
	}
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	ln.Close() // Release it immediately

	var port int
	fmt.Sscanf(portStr, "%d", &port)
	address := fmt.Sprintf("127.0.0.1:%d", port)

	// Start listener after a short delay
	go func() {
		time.Sleep(300 * time.Millisecond)
		ln, err := net.Listen("tcp", address)
		if err == nil {
			// keep it open briefly then close
			time.Sleep(1 * time.Second)
			ln.Close()
		}
	}()

	ctx := context.Background()
	// Timeout > delay
	err = WaitForPort(ctx, "127.0.0.1", port, 3*time.Second)
	if err != nil {
		t.Errorf("WaitForPort failed for delayed start on port %d: %v", port, err)
	}
}
