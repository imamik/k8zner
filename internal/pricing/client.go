package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	// HetznerAPIEndpoint is the default Hetzner Cloud API endpoint.
	HetznerAPIEndpoint = "https://api.hetzner.cloud/v1"

	// PricingEndpoint is the pricing API path.
	PricingEndpoint = "/pricing"
)

// Client fetches pricing data from the Hetzner API.
type Client struct {
	token      string
	endpoint   string
	httpClient *http.Client
}

// NewClient creates a new pricing client with the given API token.
func NewClient(token string) *Client {
	return &Client{
		token:    token,
		endpoint: HetznerAPIEndpoint,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewClientWithEndpoint creates a client with a custom endpoint (for testing).
func NewClientWithEndpoint(token, endpoint string) *Client {
	return &Client{
		token:    token,
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// FetchPrices fetches current pricing from the Hetzner API.
func (c *Client) FetchPrices(ctx context.Context) (*Prices, error) {
	url := c.endpoint + PricingEndpoint

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pricing: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pricing API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return parsePricingResponse(body)
}

// Hetzner API response structures

type pricingResponse struct {
	Pricing pricingData `json:"pricing"`
}

type pricingData struct {
	ServerTypes       []serverTypePricing       `json:"server_types"`
	LoadBalancerTypes []loadBalancerTypePricing `json:"load_balancer_types"`
	PrimaryIPs        []primaryIPPricing        `json:"primary_ips"`
}

type serverTypePricing struct {
	Name   string       `json:"name"`
	Prices []priceByLoc `json:"prices"`
}

type loadBalancerTypePricing struct {
	Name   string       `json:"name"`
	Prices []priceByLoc `json:"prices"`
}

type primaryIPPricing struct {
	Type   string       `json:"type"`
	Prices []priceByLoc `json:"prices"`
}

type priceByLoc struct {
	Location     string       `json:"location"`
	PriceMonthly priceMonthly `json:"price_monthly"`
}

type priceMonthly struct {
	Net string `json:"net"`
}

// parsePricingResponse parses the Hetzner pricing API response.
func parsePricingResponse(data []byte) (*Prices, error) {
	var resp pricingResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse pricing response: %w", err)
	}

	prices := &Prices{
		Servers:       make(map[string]float64),
		LoadBalancers: make(map[string]float64),
	}

	// Parse server prices (use first location's price as baseline)
	for _, st := range resp.Pricing.ServerTypes {
		if len(st.Prices) > 0 {
			prices.Servers[st.Name] = parsePriceString(st.Prices[0].PriceMonthly.Net)
		}
	}

	// Parse load balancer prices
	for _, lb := range resp.Pricing.LoadBalancerTypes {
		if len(lb.Prices) > 0 {
			prices.LoadBalancers[lb.Name] = parsePriceString(lb.Prices[0].PriceMonthly.Net)
		}
	}

	// Parse primary IP prices
	for _, ip := range resp.Pricing.PrimaryIPs {
		if ip.Type == "ipv4" && len(ip.Prices) > 0 {
			prices.PrimaryIPv4 = parsePriceString(ip.Prices[0].PriceMonthly.Net)
		}
	}

	return prices, nil
}

// parsePriceString converts a price string (e.g., "4.3500") to float64.
func parsePriceString(s string) float64 {
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// FetchOrDefault fetches prices from the API, falling back to defaults on error.
func FetchOrDefault(ctx context.Context, token string) *Prices {
	if token == "" {
		return DefaultPrices()
	}

	client := NewClient(token)
	prices, err := client.FetchPrices(ctx)
	if err != nil {
		return DefaultPrices()
	}

	return prices
}
