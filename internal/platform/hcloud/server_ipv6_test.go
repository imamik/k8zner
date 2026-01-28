package hcloud

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"

	"github.com/hetznercloud/hcloud-go/v2/hcloud/schema"
)

func TestCreateServer_IPv6Only(t *testing.T) {
	t.Run("creates server with IPv6-only when enablePublicIPv4 is false", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		var capturedRequest struct {
			PublicNet struct {
				EnableIPv4 bool `json:"enable_ipv4"`
				EnableIPv6 bool `json:"enable_ipv6"`
			} `json:"public_net"`
		}

		// Mock server type lookup
		ts.handleFunc("/server_types", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ServerTypeListResponse{
				ServerTypes: []schema.ServerType{
					{ID: 1, Name: "cx22", Architecture: "x86"},
				},
			})
		})

		// Mock image lookup
		ts.handleFunc("/images", func(w http.ResponseWriter, _ *http.Request) {
			imageName := "debian-13"
			jsonResponse(w, http.StatusOK, schema.ImageListResponse{
				Images: []schema.Image{
					{ID: 10, Name: &imageName, Type: "system", Architecture: "x86", Status: "available"},
				},
			})
		})

		// Mock location lookup
		ts.handleFunc("/locations", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.LocationListResponse{
				Locations: []schema.Location{
					{ID: 1, Name: "fsn1"},
				},
			})
		})

		// Mock server creation - capture the request
		ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				// Capture the request body to verify public_net settings
				if err := json.NewDecoder(r.Body).Decode(&capturedRequest); err != nil {
					t.Errorf("failed to decode request: %v", err)
				}

				jsonResponse(w, http.StatusCreated, schema.ServerCreateResponse{
					Server: schema.Server{ID: 999, Name: "test-ipv6"},
					Action: schema.Action{ID: 100, Status: "success"},
				})
				return
			}
			jsonResponse(w, http.StatusOK, schema.ServerListResponse{Servers: []schema.Server{}})
		})

		// Mock action wait
		ts.handleFunc("/actions/100", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
				Action: schema.Action{ID: 100, Status: "success", Progress: 100},
			})
		})

		client := ts.realClient()
		ctx := context.Background()

		// Create server with IPv6-only (enablePublicIPv4=false, enablePublicIPv6=true)
		serverID, err := client.CreateServer(ctx, "test-ipv6", "debian-13", "cx22", "fsn1",
			[]string{}, nil, "", nil, 0, "", false, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if serverID != "999" {
			t.Errorf("expected server ID '999', got %q", serverID)
		}

		// Verify the public_net configuration was sent correctly
		if capturedRequest.PublicNet.EnableIPv4 {
			t.Error("expected EnableIPv4 to be false for IPv6-only server")
		}
		if !capturedRequest.PublicNet.EnableIPv6 {
			t.Error("expected EnableIPv6 to be true for IPv6-only server")
		}
	})

	t.Run("creates server with both IPv4 and IPv6 by default", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		var capturedRequest struct {
			PublicNet *struct {
				EnableIPv4 bool `json:"enable_ipv4"`
				EnableIPv6 bool `json:"enable_ipv6"`
			} `json:"public_net"`
		}

		// Mock server type lookup
		ts.handleFunc("/server_types", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ServerTypeListResponse{
				ServerTypes: []schema.ServerType{
					{ID: 1, Name: "cx22", Architecture: "x86"},
				},
			})
		})

		// Mock image lookup
		ts.handleFunc("/images", func(w http.ResponseWriter, _ *http.Request) {
			imageName := "debian-13"
			jsonResponse(w, http.StatusOK, schema.ImageListResponse{
				Images: []schema.Image{
					{ID: 10, Name: &imageName, Type: "system", Architecture: "x86", Status: "available"},
				},
			})
		})

		// Mock location lookup
		ts.handleFunc("/locations", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.LocationListResponse{
				Locations: []schema.Location{
					{ID: 1, Name: "fsn1"},
				},
			})
		})

		// Mock server creation
		ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				if err := json.NewDecoder(r.Body).Decode(&capturedRequest); err != nil {
					t.Errorf("failed to decode request: %v", err)
				}
				jsonResponse(w, http.StatusCreated, schema.ServerCreateResponse{
					Server: schema.Server{ID: 999, Name: "test-both"},
					Action: schema.Action{ID: 100, Status: "success"},
				})
				return
			}
			jsonResponse(w, http.StatusOK, schema.ServerListResponse{Servers: []schema.Server{}})
		})

		// Mock action wait
		ts.handleFunc("/actions/100", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
				Action: schema.Action{ID: 100, Status: "success", Progress: 100},
			})
		})

		client := ts.realClient()
		ctx := context.Background()

		// Create server with both IPv4 and IPv6 (enablePublicIPv4=true, enablePublicIPv6=true)
		_, err := client.CreateServer(ctx, "test-both", "debian-13", "cx22", "fsn1",
			[]string{}, nil, "", nil, 0, "", true, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// When both are true, we should either have PublicNet set with both true,
		// or PublicNet should be nil (Hetzner default is both enabled)
		if capturedRequest.PublicNet != nil {
			if !capturedRequest.PublicNet.EnableIPv4 || !capturedRequest.PublicNet.EnableIPv6 {
				t.Error("expected both EnableIPv4 and EnableIPv6 to be true when public_net is set")
			}
		}
		// If PublicNet is nil, that's also fine - Hetzner defaults to both enabled
	})
}

func TestGetServerIP_IPv6Only(t *testing.T) {
	t.Run("returns IPv6 when server has no IPv4", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		// Mock server lookup returning IPv6-only server
		ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
			name := r.URL.Query().Get("name")
			if name == "ipv6-server" {
				jsonResponse(w, http.StatusOK, schema.ServerListResponse{
					Servers: []schema.Server{
						{
							ID:   123,
							Name: "ipv6-server",
							PublicNet: schema.ServerPublicNet{
								IPv4: schema.ServerPublicNetIPv4{
									IP: "", // No IPv4
								},
								IPv6: schema.ServerPublicNetIPv6{
									IP: "2a01:4f8:c010:1234::/64",
								},
							},
						},
					},
				})
				return
			}
			jsonResponse(w, http.StatusOK, schema.ServerListResponse{Servers: []schema.Server{}})
		})

		client := ts.realClient()
		ctx := context.Background()

		ip, err := client.GetServerIP(ctx, "ipv6-server")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should return the IPv6 address
		parsedIP := net.ParseIP(ip)
		if parsedIP == nil {
			t.Fatalf("returned IP %q is not valid", ip)
		}

		// Check it's actually an IPv6 address
		if parsedIP.To4() != nil {
			t.Errorf("expected IPv6 address, got IPv4: %s", ip)
		}
	})

	t.Run("returns IPv4 when server has both", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		// Mock server lookup returning server with both IPv4 and IPv6
		ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
			name := r.URL.Query().Get("name")
			if name == "dual-stack-server" {
				jsonResponse(w, http.StatusOK, schema.ServerListResponse{
					Servers: []schema.Server{
						{
							ID:   456,
							Name: "dual-stack-server",
							PublicNet: schema.ServerPublicNet{
								IPv4: schema.ServerPublicNetIPv4{
									IP: "95.216.100.50",
								},
								IPv6: schema.ServerPublicNetIPv6{
									IP: "2a01:4f8:c010:5678::/64",
								},
							},
						},
					},
				})
				return
			}
			jsonResponse(w, http.StatusOK, schema.ServerListResponse{Servers: []schema.Server{}})
		})

		client := ts.realClient()
		ctx := context.Background()

		ip, err := client.GetServerIP(ctx, "dual-stack-server")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should prefer IPv4 for backwards compatibility
		if ip != "95.216.100.50" {
			t.Errorf("expected IPv4 address 95.216.100.50, got %s", ip)
		}
	})

	t.Run("returns error when server has no public IP", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		// Mock server lookup returning server with no public IPs
		ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
			name := r.URL.Query().Get("name")
			if name == "private-server" {
				jsonResponse(w, http.StatusOK, schema.ServerListResponse{
					Servers: []schema.Server{
						{
							ID:   789,
							Name: "private-server",
							PublicNet: schema.ServerPublicNet{
								IPv4: schema.ServerPublicNetIPv4{
									IP: "",
								},
								IPv6: schema.ServerPublicNetIPv6{
									IP: "",
								},
							},
						},
					},
				})
				return
			}
			jsonResponse(w, http.StatusOK, schema.ServerListResponse{Servers: []schema.Server{}})
		})

		client := ts.realClient()
		ctx := context.Background()

		_, err := client.GetServerIP(ctx, "private-server")
		if err == nil {
			t.Error("expected error when server has no public IP")
		}
	})
}

// TestMockClient_IPv6Parameters verifies the mock client supports IPv6 parameters
func TestMockClient_IPv6Parameters(t *testing.T) {
	m := &MockClient{}

	var capturedIPv4, capturedIPv6 bool
	m.CreateServerFunc = func(ctx context.Context, name, imageType, serverType, location string, sshKeys []string, labels map[string]string, userData string, placementGroupID *int64, networkID int64, privateIP string, enablePublicIPv4, enablePublicIPv6 bool) (string, error) {
		capturedIPv4 = enablePublicIPv4
		capturedIPv6 = enablePublicIPv6
		return "mock-id", nil
	}

	ctx := context.Background()
	_, err := m.CreateServer(ctx, "test", "image", "type", "loc", nil, nil, "", nil, 0, "", false, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedIPv4 {
		t.Error("expected enablePublicIPv4 to be false")
	}
	if !capturedIPv6 {
		t.Error("expected enablePublicIPv6 to be true")
	}
}
