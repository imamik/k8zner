package hcloud

import (
	"context"
	"io"
	"net/http"
	"strings"

	"k8zner/internal/config"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// RealClient implements InfrastructureManager using the Hetzner Cloud API.
type RealClient struct {
	client     *hcloud.Client
	timeouts   *config.Timeouts
	httpClient *http.Client
}

// ClientOption configures a RealClient.
type ClientOption func(*RealClient)

// WithTimeouts sets custom timeouts for the client.
func WithTimeouts(t *config.Timeouts) ClientOption {
	return func(c *RealClient) {
		c.timeouts = t
	}
}

// WithHTTPClient sets a custom HTTP client for external requests.
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *RealClient) {
		c.httpClient = hc
	}
}

// WithHCloudClient sets a custom hcloud client (useful for testing).
func WithHCloudClient(hc *hcloud.Client) ClientOption {
	return func(c *RealClient) {
		c.client = hc
	}
}

// NewRealClient creates a new RealClient with optional configuration.
func NewRealClient(token string, opts ...ClientOption) *RealClient {
	c := &RealClient{
		client:     hcloud.NewClient(hcloud.WithToken(token)),
		timeouts:   config.LoadTimeouts(),
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// HCloudClient returns the underlying hcloud.Client for advanced operations.
// Use this when you need direct access to Hetzner Cloud API features not
// exposed through the RealClient interface.
func (c *RealClient) HCloudClient() *hcloud.Client {
	return c.client
}

// GetPublicIP returns the public IPv4 address of the host.
func (c *RealClient) GetPublicIP(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://ipv4.icanhazip.com", nil)
	if err != nil {
		return "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}
