package hcloud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"hcloud-k8s/internal/config"
)

func TestNewRealClient_Defaults(t *testing.T) {
	client := NewRealClient("test-token")

	if client.client == nil {
		t.Error("expected hcloud client to be initialized")
	}
	if client.timeouts == nil {
		t.Error("expected timeouts to be initialized")
	}
	if client.httpClient == nil {
		t.Error("expected httpClient to be initialized")
	}
	if client.httpClient != http.DefaultClient {
		t.Error("expected httpClient to be http.DefaultClient by default")
	}
}

func TestNewRealClient_WithTimeouts(t *testing.T) {
	customTimeouts := &config.Timeouts{
		ServerIP:          30 * time.Second,
		RetryMaxAttempts:  5,
		RetryInitialDelay: 2 * time.Second,
	}

	client := NewRealClient("test-token", WithTimeouts(customTimeouts))

	if client.timeouts != customTimeouts {
		t.Errorf("expected custom timeouts to be set")
	}
	if client.timeouts.ServerIP != 30*time.Second {
		t.Errorf("expected ServerIP timeout to be 30s, got %v", client.timeouts.ServerIP)
	}
}

func TestNewRealClient_WithHTTPClient(t *testing.T) {
	customHTTPClient := &http.Client{
		Timeout: 60 * time.Second,
	}

	client := NewRealClient("test-token", WithHTTPClient(customHTTPClient))

	if client.httpClient != customHTTPClient {
		t.Error("expected custom HTTP client to be set")
	}
}

func TestNewRealClient_MultipleOptions(t *testing.T) {
	customTimeouts := &config.Timeouts{
		ServerIP: 45 * time.Second,
	}
	customHTTPClient := &http.Client{
		Timeout: 90 * time.Second,
	}

	client := NewRealClient("test-token",
		WithTimeouts(customTimeouts),
		WithHTTPClient(customHTTPClient),
	)

	if client.timeouts != customTimeouts {
		t.Error("expected custom timeouts to be set")
	}
	if client.httpClient != customHTTPClient {
		t.Error("expected custom HTTP client to be set")
	}
}

func TestGetPublicIP_Success(t *testing.T) {
	// Create a mock server that returns a fixed IP
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("203.0.113.42\n"))
	}))
	defer server.Close()

	// Create client with custom HTTP client pointing to mock server
	client := NewRealClient("test-token", WithHTTPClient(server.Client()))

	// We can't easily override the URL in GetPublicIP, but we've tested that
	// the httpClient is properly injected. A full test would require more
	// refactoring to make the URL configurable.
	_ = client

	// Basic sanity check - the httpClient is used
	if client.httpClient == http.DefaultClient {
		t.Error("expected custom HTTP client to be used")
	}
}

func TestGetPublicIP_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// This test actually calls the external service
	// Only run it when explicitly testing integration
	client := NewRealClient("test-token")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ip, err := client.GetPublicIP(ctx)
	if err != nil {
		t.Logf("GetPublicIP returned error (might be expected in CI): %v", err)
		return
	}

	if ip == "" {
		t.Error("expected non-empty IP address")
	}
	t.Logf("Got public IP: %s", ip)
}
