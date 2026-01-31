package pricing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_FetchPrices(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("Missing or invalid Authorization header")
		}

		// Return mock pricing data
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"pricing": {
				"server_types": [
					{
						"name": "cx22",
						"prices": [
							{"location": "fsn1", "price_monthly": {"net": "4.3500"}}
						]
					},
					{
						"name": "cx32",
						"prices": [
							{"location": "fsn1", "price_monthly": {"net": "8.0900"}}
						]
					}
				],
				"load_balancer_types": [
					{
						"name": "lb11",
						"prices": [
							{"location": "fsn1", "price_monthly": {"net": "6.4100"}}
						]
					}
				],
				"primary_ips": [
					{
						"type": "ipv4",
						"prices": [
							{"location": "fsn1", "price_monthly": {"net": "0.5000"}}
						]
					}
				]
			}
		}`))
	}))
	defer server.Close()

	client := NewClientWithEndpoint("test-token", server.URL)
	prices, err := client.FetchPrices(context.Background())
	if err != nil {
		t.Fatalf("FetchPrices() error = %v", err)
	}

	// Verify parsed prices
	if prices.Servers["cx22"] != 4.35 {
		t.Errorf("cx22 price = %.2f, want 4.35", prices.Servers["cx22"])
	}
	if prices.Servers["cx32"] != 8.09 {
		t.Errorf("cx32 price = %.2f, want 8.09", prices.Servers["cx32"])
	}
	if prices.LoadBalancers["lb11"] != 6.41 {
		t.Errorf("lb11 price = %.2f, want 6.41", prices.LoadBalancers["lb11"])
	}
	if prices.PrimaryIPv4 != 0.50 {
		t.Errorf("PrimaryIPv4 = %.2f, want 0.50", prices.PrimaryIPv4)
	}
}

func TestClient_FetchPrices_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClientWithEndpoint("invalid-token", server.URL)
	_, err := client.FetchPrices(context.Background())
	if err == nil {
		t.Error("FetchPrices() expected error for unauthorized request")
	}
}

func TestClient_FetchPrices_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	client := NewClientWithEndpoint("test-token", server.URL)
	_, err := client.FetchPrices(context.Background())
	if err == nil {
		t.Error("FetchPrices() expected error for invalid JSON")
	}
}

func TestParsePriceString(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"4.3500", 4.35},
		{"8.0900", 8.09},
		{"0.5000", 0.50},
		{"10.00", 10.00},
		{"", 0.0},
		{"invalid", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parsePriceString(tt.input)
			if got != tt.want {
				t.Errorf("parsePriceString(%q) = %.2f, want %.2f", tt.input, got, tt.want)
			}
		})
	}
}
