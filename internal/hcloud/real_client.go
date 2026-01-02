package hcloud

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// RealClient implements InfrastructureManager using the Hetzner Cloud API.
type RealClient struct {
	client *hcloud.Client
}

// NewRealClient creates a new RealClient.
func NewRealClient(token string) *RealClient {
	return &RealClient{
		client: hcloud.NewClient(hcloud.WithToken(token)),
	}
}

// Helper for public IP detection
func (c *RealClient) GetPublicIP(ctx context.Context) (string, error) {
	// Simple HTTP request to icanhazip.com
	req, err := http.NewRequestWithContext(ctx, "GET", "https://ipv4.icanhazip.com", nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}
