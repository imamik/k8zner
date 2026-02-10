package hcloud

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/imamik/k8zner/internal/config"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/hetznercloud/hcloud-go/v2/hcloud/schema"
)

// testServer creates an httptest server that can be used to mock Hetzner Cloud API responses.
type testServer struct {
	server *httptest.Server
	mux    *http.ServeMux
}

// newTestServer creates a new test server for mocking the Hetzner Cloud API.
func newTestServer() *testServer {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	return &testServer{
		server: server,
		mux:    mux,
	}
}

// close shuts down the test server.
func (ts *testServer) close() {
	ts.server.Close()
}

// client returns an hcloud.Client configured to use the test server.
func (ts *testServer) client() *hcloud.Client {
	return hcloud.NewClient(
		hcloud.WithToken("test-token"),
		hcloud.WithEndpoint(ts.server.URL),
	)
}

// realClient returns a RealClient configured to use the test server.
func (ts *testServer) realClient() *RealClient {
	return NewRealClient("test-token",
		WithHCloudClient(ts.client()),
		WithTimeouts(&config.Timeouts{
			ServerCreate:      30 * time.Second,
			ServerIP:          10 * time.Second,
			Delete:            30 * time.Second,
			RetryMaxAttempts:  3,
			RetryInitialDelay: 100 * time.Millisecond,
		}),
	)
}

// handleFunc registers a handler for a specific path.
func (ts *testServer) handleFunc(pattern string, handler http.HandlerFunc) {
	ts.mux.HandleFunc(pattern, handler)
}

// jsonResponse writes a JSON response with the given status code and body.
func jsonResponse(w http.ResponseWriter, statusCode int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(body)
}

func TestRealClient_GetServerIP_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	// Setup mock response for GET /servers?name=test-server
	ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		name := r.URL.Query().Get("name")
		if name == "test-server" {
			jsonResponse(w, http.StatusOK, schema.ServerListResponse{
				Servers: []schema.Server{
					{
						ID:   123,
						Name: "test-server",
						PublicNet: schema.ServerPublicNet{
							IPv4: schema.ServerPublicNetIPv4{
								IP: "203.0.113.42",
							},
						},
					},
				},
			})
			return
		}

		// Server not found
		jsonResponse(w, http.StatusOK, schema.ServerListResponse{
			Servers: []schema.Server{},
		})
	})

	client := ts.realClient()
	ctx := context.Background()

	t.Run("server found", func(t *testing.T) {
		ip, err := client.GetServerIP(ctx, "test-server")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ip != "203.0.113.42" {
			t.Errorf("expected IP '203.0.113.42', got %q", ip)
		}
	})

	t.Run("server not found", func(t *testing.T) {
		_, err := client.GetServerIP(ctx, "nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent server")
		}
	})
}

func TestRealClient_GetServerID_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "existing-server" {
			jsonResponse(w, http.StatusOK, schema.ServerListResponse{
				Servers: []schema.Server{
					{ID: 456, Name: "existing-server"},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.ServerListResponse{
			Servers: []schema.Server{},
		})
	})

	client := ts.realClient()
	ctx := context.Background()

	t.Run("server exists", func(t *testing.T) {
		id, err := client.GetServerID(ctx, "existing-server")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "456" {
			t.Errorf("expected ID '456', got %q", id)
		}
	})

	t.Run("server does not exist", func(t *testing.T) {
		id, err := client.GetServerID(ctx, "nonexistent")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "" {
			t.Errorf("expected empty ID for nonexistent server, got %q", id)
		}
	})
}

func TestRealClient_GetServersByLabel_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		labelSelector := r.URL.Query().Get("label_selector")
		if labelSelector == "cluster=test" {
			jsonResponse(w, http.StatusOK, schema.ServerListResponse{
				Servers: []schema.Server{
					{ID: 1, Name: "server-1", Labels: map[string]string{"cluster": "test"}},
					{ID: 2, Name: "server-2", Labels: map[string]string{"cluster": "test"}},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.ServerListResponse{
			Servers: []schema.Server{},
		})
	})

	client := ts.realClient()
	ctx := context.Background()

	servers, err := client.GetServersByLabel(ctx, map[string]string{"cluster": "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(servers) != 2 {
		t.Errorf("expected 2 servers, got %d", len(servers))
	}
}

func TestRealClient_DeleteServer_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	// Mock GET /servers?name=server-to-delete
	ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "server-to-delete" {
			jsonResponse(w, http.StatusOK, schema.ServerListResponse{
				Servers: []schema.Server{
					{ID: 789, Name: "server-to-delete"},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.ServerListResponse{Servers: []schema.Server{}})
	})

	// Mock DELETE /servers/789
	ts.handleFunc("/servers/789", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			jsonResponse(w, http.StatusOK, schema.ServerDeleteResponse{
				Action: schema.Action{ID: 1, Status: "success"},
			})
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})

	// Mock GET /actions/1 (for waiting)
	ts.handleFunc("/actions/1", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
			Action: schema.Action{ID: 1, Status: "success", Progress: 100},
		})
	})

	client := ts.realClient()
	ctx := context.Background()

	err := client.DeleteServer(ctx, "server-to-delete")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRealClient_GetNetwork_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/networks", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "test-network" {
			jsonResponse(w, http.StatusOK, schema.NetworkListResponse{
				Networks: []schema.Network{
					{ID: 100, Name: "test-network", IPRange: "10.0.0.0/16"},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.NetworkListResponse{Networks: []schema.Network{}})
	})

	client := ts.realClient()
	ctx := context.Background()

	t.Run("network exists", func(t *testing.T) {
		network, err := client.GetNetwork(ctx, "test-network")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if network == nil {
			t.Fatal("expected network, got nil")
		}
		if network.ID != 100 {
			t.Errorf("expected ID 100, got %d", network.ID)
		}
	})

	t.Run("network does not exist", func(t *testing.T) {
		network, err := client.GetNetwork(ctx, "nonexistent")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if network != nil {
			t.Errorf("expected nil for nonexistent network, got %v", network)
		}
	})
}

func TestRealClient_GetFirewall_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/firewalls", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "test-firewall" {
			jsonResponse(w, http.StatusOK, schema.FirewallListResponse{
				Firewalls: []schema.Firewall{
					{ID: 200, Name: "test-firewall"},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.FirewallListResponse{Firewalls: []schema.Firewall{}})
	})

	client := ts.realClient()
	ctx := context.Background()

	firewall, err := client.GetFirewall(ctx, "test-firewall")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if firewall == nil {
		t.Fatal("expected firewall, got nil")
	}
	if firewall.ID != 200 {
		t.Errorf("expected ID 200, got %d", firewall.ID)
	}
}

func TestRealClient_GetLoadBalancer_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/load_balancers", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "test-lb" {
			jsonResponse(w, http.StatusOK, schema.LoadBalancerListResponse{
				LoadBalancers: []schema.LoadBalancer{
					{ID: 300, Name: "test-lb"},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.LoadBalancerListResponse{LoadBalancers: []schema.LoadBalancer{}})
	})

	client := ts.realClient()
	ctx := context.Background()

	lb, err := client.GetLoadBalancer(ctx, "test-lb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lb == nil {
		t.Fatal("expected load balancer, got nil")
	}
	if lb.ID != 300 {
		t.Errorf("expected ID 300, got %d", lb.ID)
	}
}

func TestRealClient_GetPlacementGroup_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/placement_groups", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "test-pg" {
			jsonResponse(w, http.StatusOK, schema.PlacementGroupListResponse{
				PlacementGroups: []schema.PlacementGroup{
					{ID: 400, Name: "test-pg", Type: "spread"},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.PlacementGroupListResponse{PlacementGroups: []schema.PlacementGroup{}})
	})

	client := ts.realClient()
	ctx := context.Background()

	pg, err := client.GetPlacementGroup(ctx, "test-pg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pg == nil {
		t.Fatal("expected placement group, got nil")
	}
	if pg.ID != 400 {
		t.Errorf("expected ID 400, got %d", pg.ID)
	}
}

func TestRealClient_GetCertificate_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/certificates", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "test-cert" {
			jsonResponse(w, http.StatusOK, schema.CertificateListResponse{
				Certificates: []schema.Certificate{
					{ID: 600, Name: "test-cert"},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.CertificateListResponse{Certificates: []schema.Certificate{}})
	})

	client := ts.realClient()
	ctx := context.Background()

	cert, err := client.GetCertificate(ctx, "test-cert")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cert == nil {
		t.Fatal("expected certificate, got nil")
	}
	if cert.ID != 600 {
		t.Errorf("expected ID 600, got %d", cert.ID)
	}
}

func TestRealClient_GetSnapshotByLabels_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/images", func(w http.ResponseWriter, r *http.Request) {
		labelSelector := r.URL.Query().Get("label_selector")
		if labelSelector != "" {
			snapshotName := "test-snapshot"
			jsonResponse(w, http.StatusOK, schema.ImageListResponse{
				Images: []schema.Image{
					{ID: 700, Name: &snapshotName, Type: "snapshot", Labels: map[string]string{"type": "talos"}},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.ImageListResponse{Images: []schema.Image{}})
	})

	client := ts.realClient()
	ctx := context.Background()

	image, err := client.GetSnapshotByLabels(ctx, map[string]string{"type": "talos"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if image == nil {
		t.Fatal("expected image, got nil")
	}
	if image.ID != 700 {
		t.Errorf("expected ID 700, got %d", image.ID)
	}
}

func TestRealClient_CreateServer_ValidationError(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	client := ts.realClient()
	ctx := context.Background()

	// Test validation: networkID provided but privateIP empty
	_, err := client.CreateServer(ctx, ServerCreateOpts{
		Name: "test", ImageType: "image", ServerType: "type", Location: "loc",
		NetworkID: 123, EnablePublicIPv4: true, EnablePublicIPv6: true,
	})
	if err == nil {
		t.Error("expected validation error for mismatched networkID/privateIP")
	}

	// Test validation: privateIP provided but networkID is 0
	_, err = client.CreateServer(ctx, ServerCreateOpts{
		Name: "test", ImageType: "image", ServerType: "type", Location: "loc",
		PrivateIP: "10.0.0.1", EnablePublicIPv4: true, EnablePublicIPv6: true,
	})
	if err == nil {
		t.Error("expected validation error for mismatched networkID/privateIP")
	}
}

func TestRealClient_EnsureNetwork_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	// Track if network was created
	networkCreated := false

	ts.handleFunc("/networks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			networkCreated = true
			jsonResponse(w, http.StatusCreated, schema.NetworkCreateResponse{
				Network: schema.Network{
					ID:      100,
					Name:    "test-network",
					IPRange: "10.0.0.0/16",
				},
			})
			return
		}
		// GET - return empty initially to trigger create
		name := r.URL.Query().Get("name")
		if name == "test-network" && networkCreated {
			jsonResponse(w, http.StatusOK, schema.NetworkListResponse{
				Networks: []schema.Network{
					{ID: 100, Name: "test-network", IPRange: "10.0.0.0/16"},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.NetworkListResponse{Networks: []schema.Network{}})
	})

	client := ts.realClient()
	ctx := context.Background()

	// EnsureNetwork(ctx, name, ipRange, zone, labels)
	network, err := client.EnsureNetwork(ctx, "test-network", "10.0.0.0/16", "eu-central", map[string]string{"test": "true"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if network == nil {
		t.Fatal("expected network, got nil")
	}
	if network.ID != 100 {
		t.Errorf("expected ID 100, got %d", network.ID)
	}
}

func TestRealClient_EnsurePlacementGroup_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/placement_groups", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			jsonResponse(w, http.StatusCreated, schema.PlacementGroupCreateResponse{
				PlacementGroup: schema.PlacementGroup{
					ID:   401,
					Name: "new-pg",
					Type: "spread",
				},
			})
			return
		}
		// GET - return empty to trigger create
		jsonResponse(w, http.StatusOK, schema.PlacementGroupListResponse{PlacementGroups: []schema.PlacementGroup{}})
	})

	client := ts.realClient()
	ctx := context.Background()

	pg, err := client.EnsurePlacementGroup(ctx, "new-pg", "spread", map[string]string{"test": "true"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pg == nil {
		t.Fatal("expected placement group, got nil")
	}
	if pg.ID != 401 {
		t.Errorf("expected ID 401, got %d", pg.ID)
	}
}

func TestRealClient_CreateSSHKey_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/ssh_keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			jsonResponse(w, http.StatusCreated, schema.SSHKeyCreateResponse{
				SSHKey: schema.SSHKey{
					ID:          1001,
					Name:        "test-key",
					Fingerprint: "aa:bb:cc:dd:ee:ff",
					PublicKey:   "ssh-ed25519 AAAA...",
				},
			})
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})

	client := ts.realClient()
	ctx := context.Background()

	// CreateSSHKey returns the key ID as a string
	keyID, err := client.CreateSSHKey(ctx, "test-key", "ssh-ed25519 AAAA...", map[string]string{"test": "true"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if keyID == "" {
		t.Fatal("expected SSH key ID, got empty string")
	}
	if keyID != "1001" {
		t.Errorf("expected ID '1001', got %q", keyID)
	}
}

func TestRealClient_DeleteNetwork_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/networks", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "network-to-delete" {
			jsonResponse(w, http.StatusOK, schema.NetworkListResponse{
				Networks: []schema.Network{
					{ID: 150, Name: "network-to-delete"},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.NetworkListResponse{Networks: []schema.Network{}})
	})

	ts.handleFunc("/networks/150", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})

	client := ts.realClient()
	ctx := context.Background()

	err := client.DeleteNetwork(ctx, "network-to-delete")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRealClient_DeleteFirewall_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/firewalls", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "firewall-to-delete" {
			jsonResponse(w, http.StatusOK, schema.FirewallListResponse{
				Firewalls: []schema.Firewall{
					{ID: 250, Name: "firewall-to-delete"},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.FirewallListResponse{Firewalls: []schema.Firewall{}})
	})

	ts.handleFunc("/firewalls/250", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})

	client := ts.realClient()
	ctx := context.Background()

	err := client.DeleteFirewall(ctx, "firewall-to-delete")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRealClient_DeletePlacementGroup_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/placement_groups", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "pg-to-delete" {
			jsonResponse(w, http.StatusOK, schema.PlacementGroupListResponse{
				PlacementGroups: []schema.PlacementGroup{
					{ID: 450, Name: "pg-to-delete"},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.PlacementGroupListResponse{PlacementGroups: []schema.PlacementGroup{}})
	})

	ts.handleFunc("/placement_groups/450", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})

	client := ts.realClient()
	ctx := context.Background()

	err := client.DeletePlacementGroup(ctx, "pg-to-delete")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRealClient_DeleteSSHKey_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/ssh_keys", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "key-to-delete" {
			jsonResponse(w, http.StatusOK, schema.SSHKeyListResponse{
				SSHKeys: []schema.SSHKey{
					{ID: 1050, Name: "key-to-delete"},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.SSHKeyListResponse{SSHKeys: []schema.SSHKey{}})
	})

	ts.handleFunc("/ssh_keys/1050", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})

	client := ts.realClient()
	ctx := context.Background()

	err := client.DeleteSSHKey(ctx, "key-to-delete")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRealClient_EnsureFirewall_WithHTTPMock(t *testing.T) {
	t.Run("creates firewall when not exists", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		firewallCreated := false

		ts.handleFunc("/firewalls", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				firewallCreated = true
				jsonResponse(w, http.StatusCreated, schema.FirewallCreateResponse{
					Firewall: schema.Firewall{
						ID:   201,
						Name: "test-firewall",
						Rules: []schema.FirewallRule{
							{Direction: "in", Protocol: "tcp", Port: hcloud.Ptr("22")},
						},
					},
					Actions: []schema.Action{{ID: 1, Status: "success"}},
				})
				return
			}
			// GET request
			name := r.URL.Query().Get("name")
			if name == "test-firewall" && firewallCreated {
				jsonResponse(w, http.StatusOK, schema.FirewallListResponse{
					Firewalls: []schema.Firewall{
						{ID: 201, Name: "test-firewall"},
					},
				})
				return
			}
			jsonResponse(w, http.StatusOK, schema.FirewallListResponse{Firewalls: []schema.Firewall{}})
		})

		ts.handleFunc("/actions/1", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
				Action: schema.Action{ID: 1, Status: "success", Progress: 100},
			})
		})

		client := ts.realClient()
		ctx := context.Background()

		rules := []hcloud.FirewallRule{
			{Direction: hcloud.FirewallRuleDirectionIn, Protocol: hcloud.FirewallRuleProtocolTCP, Port: hcloud.Ptr("22")},
		}
		fw, err := client.EnsureFirewall(ctx, "test-firewall", rules, map[string]string{"env": "test"}, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fw == nil {
			t.Fatal("expected firewall, got nil")
		}
		if fw.ID != 201 {
			t.Errorf("expected ID 201, got %d", fw.ID)
		}
	})

	t.Run("creates firewall with label selector", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		ts.handleFunc("/firewalls", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				jsonResponse(w, http.StatusCreated, schema.FirewallCreateResponse{
					Firewall: schema.Firewall{
						ID:        202,
						Name:      "test-firewall-selector",
						AppliedTo: []schema.FirewallResource{},
					},
					Actions: []schema.Action{{ID: 2, Status: "success"}},
				})
				return
			}
			jsonResponse(w, http.StatusOK, schema.FirewallListResponse{Firewalls: []schema.Firewall{}})
		})

		ts.handleFunc("/firewalls/202/actions/apply_to_resources", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusCreated, schema.FirewallActionApplyToResourcesResponse{
				Actions: []schema.Action{{ID: 3, Status: "success"}},
			})
		})

		ts.handleFunc("/actions/2", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
				Action: schema.Action{ID: 2, Status: "success", Progress: 100},
			})
		})

		ts.handleFunc("/actions/3", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
				Action: schema.Action{ID: 3, Status: "success", Progress: 100},
			})
		})

		client := ts.realClient()
		ctx := context.Background()

		rules := []hcloud.FirewallRule{}
		fw, err := client.EnsureFirewall(ctx, "test-firewall-selector", rules, nil, "cluster=test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fw == nil {
			t.Fatal("expected firewall, got nil")
		}
	})

	t.Run("updates existing firewall rules", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		ts.handleFunc("/firewalls", func(w http.ResponseWriter, r *http.Request) {
			name := r.URL.Query().Get("name")
			if name == "existing-firewall" {
				jsonResponse(w, http.StatusOK, schema.FirewallListResponse{
					Firewalls: []schema.Firewall{
						{ID: 203, Name: "existing-firewall", Rules: []schema.FirewallRule{}},
					},
				})
				return
			}
			jsonResponse(w, http.StatusOK, schema.FirewallListResponse{Firewalls: []schema.Firewall{}})
		})

		ts.handleFunc("/firewalls/203/actions/set_rules", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusCreated, schema.FirewallActionSetRulesResponse{
				Actions: []schema.Action{{ID: 4, Status: "success"}},
			})
		})

		ts.handleFunc("/actions/4", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
				Action: schema.Action{ID: 4, Status: "success", Progress: 100},
			})
		})

		client := ts.realClient()
		ctx := context.Background()

		rules := []hcloud.FirewallRule{
			{Direction: hcloud.FirewallRuleDirectionIn, Protocol: hcloud.FirewallRuleProtocolTCP, Port: hcloud.Ptr("443")},
		}
		fw, err := client.EnsureFirewall(ctx, "existing-firewall", rules, nil, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fw == nil {
			t.Fatal("expected firewall, got nil")
		}
		if fw.ID != 203 {
			t.Errorf("expected ID 203, got %d", fw.ID)
		}
	})

	t.Run("skips applying existing label selector", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		ts.handleFunc("/firewalls", func(w http.ResponseWriter, r *http.Request) {
			name := r.URL.Query().Get("name")
			if name == "existing-firewall-selector" {
				jsonResponse(w, http.StatusOK, schema.FirewallListResponse{
					Firewalls: []schema.Firewall{
						{
							ID:   204,
							Name: "existing-firewall-selector",
							AppliedTo: []schema.FirewallResource{
								{
									Type: "label_selector",
									LabelSelector: &schema.FirewallResourceLabelSelector{
										Selector: "cluster=test",
									},
								},
							},
						},
					},
				})
				return
			}
			jsonResponse(w, http.StatusOK, schema.FirewallListResponse{Firewalls: []schema.Firewall{}})
		})

		ts.handleFunc("/firewalls/204/actions/set_rules", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusCreated, schema.FirewallActionSetRulesResponse{
				Actions: []schema.Action{{ID: 5, Status: "success"}},
			})
		})

		ts.handleFunc("/actions/5", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
				Action: schema.Action{ID: 5, Status: "success", Progress: 100},
			})
		})

		client := ts.realClient()
		ctx := context.Background()

		fw, err := client.EnsureFirewall(ctx, "existing-firewall-selector", []hcloud.FirewallRule{}, nil, "cluster=test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fw == nil {
			t.Fatal("expected firewall, got nil")
		}
	})
}

func TestRealClient_EnsureLoadBalancer_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/load_balancers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			jsonResponse(w, http.StatusCreated, schema.LoadBalancerCreateResponse{
				LoadBalancer: schema.LoadBalancer{
					ID:   301,
					Name: "test-lb",
				},
				Action: schema.Action{ID: 10, Status: "success"},
			})
			return
		}
		// GET request
		jsonResponse(w, http.StatusOK, schema.LoadBalancerListResponse{LoadBalancers: []schema.LoadBalancer{}})
	})

	ts.handleFunc("/load_balancer_types", func(w http.ResponseWriter, r *http.Request) {
		// SDK uses GET /load_balancer_types?name=lb11
		name := r.URL.Query().Get("name")
		if name == "lb11" {
			jsonResponse(w, http.StatusOK, schema.LoadBalancerTypeListResponse{
				LoadBalancerTypes: []schema.LoadBalancerType{
					{ID: 1, Name: "lb11"},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.LoadBalancerTypeListResponse{LoadBalancerTypes: []schema.LoadBalancerType{}})
	})

	ts.handleFunc("/locations", func(w http.ResponseWriter, r *http.Request) {
		// SDK uses GET /locations?name=nbg1
		name := r.URL.Query().Get("name")
		if name == "nbg1" {
			jsonResponse(w, http.StatusOK, schema.LocationListResponse{
				Locations: []schema.Location{
					{ID: 1, Name: "nbg1"},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.LocationListResponse{Locations: []schema.Location{}})
	})

	ts.handleFunc("/actions/10", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
			Action: schema.Action{ID: 10, Status: "success", Progress: 100},
		})
	})

	client := ts.realClient()
	ctx := context.Background()

	lb, err := client.EnsureLoadBalancer(ctx, "test-lb", "nbg1", "lb11", hcloud.LoadBalancerAlgorithmTypeRoundRobin, map[string]string{"test": "true"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lb == nil {
		t.Fatal("expected load balancer, got nil")
	}
	if lb.ID != 301 {
		t.Errorf("expected ID 301, got %d", lb.ID)
	}
}

func TestRealClient_ConfigureService_WithHTTPMock(t *testing.T) {
	t.Run("adds new service", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		ts.handleFunc("/load_balancers/301/actions/add_service", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusCreated, schema.LoadBalancerActionAddServiceResponse{
				Action: schema.Action{ID: 11, Status: "success"},
			})
		})

		ts.handleFunc("/actions/11", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
				Action: schema.Action{ID: 11, Status: "success", Progress: 100},
			})
		})

		client := ts.realClient()
		ctx := context.Background()

		lb := &hcloud.LoadBalancer{ID: 301, Services: []hcloud.LoadBalancerService{}}
		service := hcloud.LoadBalancerAddServiceOpts{
			Protocol:        hcloud.LoadBalancerServiceProtocolTCP,
			ListenPort:      hcloud.Ptr(80),
			DestinationPort: hcloud.Ptr(80),
		}

		err := client.ConfigureService(ctx, lb, service)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("skips existing service", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		client := ts.realClient()
		ctx := context.Background()

		lb := &hcloud.LoadBalancer{
			ID: 302,
			Services: []hcloud.LoadBalancerService{
				{ListenPort: 80, DestinationPort: 80, Protocol: hcloud.LoadBalancerServiceProtocolTCP},
			},
		}
		service := hcloud.LoadBalancerAddServiceOpts{
			Protocol:   hcloud.LoadBalancerServiceProtocolTCP,
			ListenPort: hcloud.Ptr(80),
		}

		err := client.ConfigureService(ctx, lb, service)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("errors on nil listen port", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		client := ts.realClient()
		ctx := context.Background()

		lb := &hcloud.LoadBalancer{ID: 303}
		service := hcloud.LoadBalancerAddServiceOpts{
			Protocol: hcloud.LoadBalancerServiceProtocolTCP,
			// ListenPort is nil
		}

		err := client.ConfigureService(ctx, lb, service)
		if err == nil {
			t.Fatal("expected error for nil listen port")
		}
	})
}

func TestRealClient_AddTarget_WithHTTPMock(t *testing.T) {
	t.Run("adds label selector target", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		ts.handleFunc("/load_balancers/301/actions/add_target", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusCreated, schema.LoadBalancerActionAddTargetResponse{
				Action: schema.Action{ID: 12, Status: "success"},
			})
		})

		ts.handleFunc("/actions/12", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
				Action: schema.Action{ID: 12, Status: "success", Progress: 100},
			})
		})

		client := ts.realClient()
		ctx := context.Background()

		lb := &hcloud.LoadBalancer{ID: 301, Targets: []hcloud.LoadBalancerTarget{}}

		err := client.AddTarget(ctx, lb, hcloud.LoadBalancerTargetTypeLabelSelector, "role=worker")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("skips existing target", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		client := ts.realClient()
		ctx := context.Background()

		lb := &hcloud.LoadBalancer{
			ID: 302,
			Targets: []hcloud.LoadBalancerTarget{
				{
					Type:          hcloud.LoadBalancerTargetTypeLabelSelector,
					LabelSelector: &hcloud.LoadBalancerTargetLabelSelector{Selector: "role=worker"},
				},
			},
		}

		err := client.AddTarget(ctx, lb, hcloud.LoadBalancerTargetTypeLabelSelector, "role=worker")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("errors on unsupported target type", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		client := ts.realClient()
		ctx := context.Background()

		lb := &hcloud.LoadBalancer{ID: 303}

		err := client.AddTarget(ctx, lb, hcloud.LoadBalancerTargetTypeIP, "")
		if err == nil {
			t.Fatal("expected error for unsupported target type")
		}
	})
}

func TestRealClient_DeleteLoadBalancer_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/load_balancers", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "lb-to-delete" {
			jsonResponse(w, http.StatusOK, schema.LoadBalancerListResponse{
				LoadBalancers: []schema.LoadBalancer{
					{ID: 350, Name: "lb-to-delete"},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.LoadBalancerListResponse{LoadBalancers: []schema.LoadBalancer{}})
	})

	ts.handleFunc("/load_balancers/350", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})

	client := ts.realClient()
	ctx := context.Background()

	err := client.DeleteLoadBalancer(ctx, "lb-to-delete")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRealClient_EnsureCertificate_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/certificates", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			jsonResponse(w, http.StatusCreated, schema.CertificateCreateResponse{
				Certificate: schema.Certificate{
					ID:   601,
					Name: "test-cert",
					Type: "uploaded",
				},
			})
			return
		}
		// GET request
		jsonResponse(w, http.StatusOK, schema.CertificateListResponse{Certificates: []schema.Certificate{}})
	})

	client := ts.realClient()
	ctx := context.Background()

	cert, err := client.EnsureCertificate(ctx, "test-cert", "-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----", "-----BEGIN PRIVATE KEY-----\nMIIE...\n-----END PRIVATE KEY-----", map[string]string{"test": "true"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cert == nil {
		t.Fatal("expected certificate, got nil")
	}
	if cert.ID != 601 {
		t.Errorf("expected ID 601, got %d", cert.ID)
	}
}

func TestRealClient_DeleteCertificate_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/certificates", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "cert-to-delete" {
			jsonResponse(w, http.StatusOK, schema.CertificateListResponse{
				Certificates: []schema.Certificate{
					{ID: 650, Name: "cert-to-delete"},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.CertificateListResponse{Certificates: []schema.Certificate{}})
	})

	ts.handleFunc("/certificates/650", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})

	client := ts.realClient()
	ctx := context.Background()

	err := client.DeleteCertificate(ctx, "cert-to-delete")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRealClient_CreateSnapshot_WithHTTPMock(t *testing.T) {
	t.Run("creates snapshot successfully", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		ts.handleFunc("/servers/123/actions/create_image", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusCreated, schema.ServerActionCreateImageResponse{
				Image: schema.Image{
					ID:   701,
					Type: "snapshot",
				},
				Action: schema.Action{ID: 20, Status: "success"},
			})
		})

		ts.handleFunc("/actions/20", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
				Action: schema.Action{ID: 20, Status: "success", Progress: 100},
			})
		})

		client := ts.realClient()
		ctx := context.Background()

		imageID, err := client.CreateSnapshot(ctx, "123", "test-snapshot", map[string]string{"type": "backup"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if imageID != "701" {
			t.Errorf("expected image ID '701', got %q", imageID)
		}
	})

	t.Run("errors on invalid server ID", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		client := ts.realClient()
		ctx := context.Background()

		_, err := client.CreateSnapshot(ctx, "invalid", "test-snapshot", nil)
		if err == nil {
			t.Fatal("expected error for invalid server ID")
		}
	})
}

func TestRealClient_DeleteImage_WithHTTPMock(t *testing.T) {
	t.Run("deletes image successfully", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		ts.handleFunc("/images/701", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodDelete {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		})

		client := ts.realClient()
		ctx := context.Background()

		err := client.DeleteImage(ctx, "701")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("errors on invalid image ID", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		client := ts.realClient()
		ctx := context.Background()

		err := client.DeleteImage(ctx, "invalid")
		if err == nil {
			t.Fatal("expected error for invalid image ID")
		}
	})
}

func TestRealClient_SetServerRDNS_InvalidIP(t *testing.T) {
	// Test that invalid IP addresses are rejected before making API calls
	ts := newTestServer()
	defer ts.close()

	client := ts.realClient()
	ctx := context.Background()

	err := client.SetServerRDNS(ctx, 123, "invalid-ip", "server.example.com")
	if err == nil {
		t.Fatal("expected error for invalid IP address")
	}
	if err.Error() != "invalid IP address: invalid-ip" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRealClient_SetLoadBalancerRDNS_InvalidIP(t *testing.T) {
	// Test that invalid IP addresses are rejected before making API calls
	ts := newTestServer()
	defer ts.close()

	client := ts.realClient()
	ctx := context.Background()

	err := client.SetLoadBalancerRDNS(ctx, 301, "not-an-ip", "lb.example.com")
	if err == nil {
		t.Fatal("expected error for invalid IP address")
	}
	if err.Error() != "invalid IP address: not-an-ip" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRealClient_AttachToNetwork_WithHTTPMock(t *testing.T) {
	t.Run("attaches LB to network", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		ts.handleFunc("/load_balancers/301/actions/attach_to_network", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusCreated, schema.LoadBalancerActionAttachToNetworkResponse{
				Action: schema.Action{ID: 40, Status: "success"},
			})
		})

		ts.handleFunc("/actions/40", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
				Action: schema.Action{ID: 40, Status: "success", Progress: 100},
			})
		})

		client := ts.realClient()
		ctx := context.Background()

		lb := &hcloud.LoadBalancer{ID: 301, PrivateNet: []hcloud.LoadBalancerPrivateNet{}}
		network := &hcloud.Network{ID: 100}
		ip := net.ParseIP("10.0.0.50")

		err := client.AttachToNetwork(ctx, lb, network, ip)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("skips if already attached", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		client := ts.realClient()
		ctx := context.Background()

		lb := &hcloud.LoadBalancer{
			ID: 302,
			PrivateNet: []hcloud.LoadBalancerPrivateNet{
				{Network: &hcloud.Network{ID: 100}},
			},
		}
		network := &hcloud.Network{ID: 100}
		ip := net.ParseIP("10.0.0.50")

		err := client.AttachToNetwork(ctx, lb, network, ip)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("errors on nil IP", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		client := ts.realClient()
		ctx := context.Background()

		lb := &hcloud.LoadBalancer{ID: 303}
		network := &hcloud.Network{ID: 100}

		err := client.AttachToNetwork(ctx, lb, network, nil)
		if err == nil {
			t.Fatal("expected error for nil IP")
		}
	})
}

func TestRealClient_EnableRescue_WithHTTPMock(t *testing.T) {
	t.Run("enables rescue mode", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		ts.handleFunc("/servers/123/actions/enable_rescue", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ServerActionEnableRescueResponse{
				RootPassword: "secret123",
				Action:       schema.Action{ID: 50, Status: "success"},
			})
		})

		ts.handleFunc("/actions/50", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
				Action: schema.Action{ID: 50, Status: "success", Progress: 100},
			})
		})

		client := ts.realClient()
		ctx := context.Background()

		password, err := client.EnableRescue(ctx, "123", []string{"456"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if password != "secret123" {
			t.Errorf("expected password 'secret123', got %q", password)
		}
	})

	t.Run("errors on invalid server ID", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		client := ts.realClient()
		ctx := context.Background()

		_, err := client.EnableRescue(ctx, "invalid", nil)
		if err == nil {
			t.Fatal("expected error for invalid server ID")
		}
	})

	t.Run("handles invalid SSH key IDs gracefully", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		ts.handleFunc("/servers/123/actions/enable_rescue", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ServerActionEnableRescueResponse{
				RootPassword: "pass",
				Action:       schema.Action{ID: 51, Status: "success"},
			})
		})

		ts.handleFunc("/actions/51", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
				Action: schema.Action{ID: 51, Status: "success", Progress: 100},
			})
		})

		client := ts.realClient()
		ctx := context.Background()

		// Invalid SSH key IDs should be ignored
		_, err := client.EnableRescue(ctx, "123", []string{"invalid", "456", "abc"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRealClient_ResetServer_WithHTTPMock(t *testing.T) {
	t.Run("resets server successfully", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		ts.handleFunc("/servers/123/actions/reset", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ServerActionResetResponse{
				Action: schema.Action{ID: 60, Status: "success"},
			})
		})

		ts.handleFunc("/actions/60", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
				Action: schema.Action{ID: 60, Status: "success", Progress: 100},
			})
		})

		client := ts.realClient()
		ctx := context.Background()

		err := client.ResetServer(ctx, "123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("errors on invalid server ID", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		client := ts.realClient()
		ctx := context.Background()

		err := client.ResetServer(ctx, "invalid")
		if err == nil {
			t.Fatal("expected error for invalid server ID")
		}
	})
}

func TestRealClient_PoweroffServer_WithHTTPMock(t *testing.T) {
	t.Run("powers off server successfully", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		ts.handleFunc("/servers/123/actions/poweroff", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ServerActionPoweroffResponse{
				Action: schema.Action{ID: 70, Status: "success"},
			})
		})

		ts.handleFunc("/actions/70", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
				Action: schema.Action{ID: 70, Status: "success", Progress: 100},
			})
		})

		client := ts.realClient()
		ctx := context.Background()

		err := client.PoweroffServer(ctx, "123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("errors on invalid server ID", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		client := ts.realClient()
		ctx := context.Background()

		err := client.PoweroffServer(ctx, "invalid")
		if err == nil {
			t.Fatal("expected error for invalid server ID")
		}
	})
}

func TestRealClient_EnsureSubnet_WithHTTPMock(t *testing.T) {
	t.Run("creates subnet when not exists", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		ts.handleFunc("/networks/100/actions/add_subnet", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusCreated, schema.NetworkActionAddSubnetResponse{
				Action: schema.Action{ID: 80, Status: "success"},
			})
		})

		ts.handleFunc("/actions/80", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
				Action: schema.Action{ID: 80, Status: "success", Progress: 100},
			})
		})

		client := ts.realClient()
		ctx := context.Background()

		network := &hcloud.Network{ID: 100, Subnets: []hcloud.NetworkSubnet{}}

		err := client.EnsureSubnet(ctx, network, "10.0.1.0/24", "eu-central", hcloud.NetworkSubnetTypeCloud)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("skips existing subnet", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		client := ts.realClient()
		ctx := context.Background()

		_, ipNet, _ := net.ParseCIDR("10.0.1.0/24")
		network := &hcloud.Network{
			ID: 100,
			Subnets: []hcloud.NetworkSubnet{
				{IPRange: ipNet, Type: hcloud.NetworkSubnetTypeCloud},
			},
		}

		err := client.EnsureSubnet(ctx, network, "10.0.1.0/24", "eu-central", hcloud.NetworkSubnetTypeCloud)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("errors on invalid CIDR", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		client := ts.realClient()
		ctx := context.Background()

		network := &hcloud.Network{ID: 100, Subnets: []hcloud.NetworkSubnet{}}

		err := client.EnsureSubnet(ctx, network, "invalid-cidr", "eu-central", hcloud.NetworkSubnetTypeCloud)
		if err == nil {
			t.Fatal("expected error for invalid CIDR")
		}
	})
}

func TestRealClient_EnsureNetwork_Validation(t *testing.T) {
	t.Run("validates IP range mismatch", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		ts.handleFunc("/networks", func(w http.ResponseWriter, r *http.Request) {
			name := r.URL.Query().Get("name")
			if name == "test-network" {
				jsonResponse(w, http.StatusOK, schema.NetworkListResponse{
					Networks: []schema.Network{
						{ID: 100, Name: "test-network", IPRange: "192.168.0.0/16"},
					},
				})
				return
			}
			jsonResponse(w, http.StatusOK, schema.NetworkListResponse{Networks: []schema.Network{}})
		})

		client := ts.realClient()
		ctx := context.Background()

		// Try to ensure with different IP range
		_, err := client.EnsureNetwork(ctx, "test-network", "10.0.0.0/16", "eu-central", nil)
		if err == nil {
			t.Fatal("expected error for IP range mismatch")
		}
	})
}

func TestRealClient_CreateServer_WithHTTPMock(t *testing.T) {
	t.Run("creates server with all options", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		// Mock server type lookup
		ts.handleFunc("/server_types", func(w http.ResponseWriter, r *http.Request) {
			name := r.URL.Query().Get("name")
			if name == "cx21" {
				jsonResponse(w, http.StatusOK, schema.ServerTypeListResponse{
					ServerTypes: []schema.ServerType{
						{ID: 1, Name: "cx21", Architecture: "x86"},
					},
				})
				return
			}
			jsonResponse(w, http.StatusOK, schema.ServerTypeListResponse{ServerTypes: []schema.ServerType{}})
		})

		// Mock image lookup
		ts.handleFunc("/images", func(w http.ResponseWriter, r *http.Request) {
			name := r.URL.Query().Get("name")
			if name == "ubuntu-22.04" {
				imageName := "ubuntu-22.04"
				jsonResponse(w, http.StatusOK, schema.ImageListResponse{
					Images: []schema.Image{
						{ID: 10, Name: &imageName, Type: "system", Architecture: "x86", Status: "available"},
					},
				})
				return
			}
			jsonResponse(w, http.StatusOK, schema.ImageListResponse{Images: []schema.Image{}})
		})

		// Mock SSH key lookup
		ts.handleFunc("/ssh_keys", func(w http.ResponseWriter, r *http.Request) {
			name := r.URL.Query().Get("name")
			if name == "my-key" {
				jsonResponse(w, http.StatusOK, schema.SSHKeyListResponse{
					SSHKeys: []schema.SSHKey{
						{ID: 100, Name: "my-key"},
					},
				})
				return
			}
			jsonResponse(w, http.StatusOK, schema.SSHKeyListResponse{SSHKeys: []schema.SSHKey{}})
		})

		// Mock location lookup
		ts.handleFunc("/locations", func(w http.ResponseWriter, r *http.Request) {
			name := r.URL.Query().Get("name")
			if name == "nbg1" {
				jsonResponse(w, http.StatusOK, schema.LocationListResponse{
					Locations: []schema.Location{
						{ID: 1, Name: "nbg1"},
					},
				})
				return
			}
			jsonResponse(w, http.StatusOK, schema.LocationListResponse{Locations: []schema.Location{}})
		})

		// Mock server creation
		ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				jsonResponse(w, http.StatusCreated, schema.ServerCreateResponse{
					Server: schema.Server{
						ID:   999,
						Name: "test-server",
					},
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

		// No network attachment (simple case)
		serverID, err := client.CreateServer(ctx, ServerCreateOpts{
			Name: "test-server", ImageType: "ubuntu-22.04", ServerType: "cx21", Location: "nbg1",
			SSHKeys: []string{"my-key"}, Labels: map[string]string{"test": "true"},
			UserData: "#!/bin/bash\necho hello", EnablePublicIPv4: true, EnablePublicIPv6: true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if serverID != "999" {
			t.Errorf("expected server ID '999', got %q", serverID)
		}
	})

	t.Run("resolves SSH keys correctly", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		// Mock SSH key lookup - return empty to trigger "not found" error
		ts.handleFunc("/ssh_keys", func(w http.ResponseWriter, r *http.Request) {
			name := r.URL.Query().Get("name")
			if name == "nonexistent-key" {
				jsonResponse(w, http.StatusOK, schema.SSHKeyListResponse{
					SSHKeys: []schema.SSHKey{},
				})
				return
			}
			jsonResponse(w, http.StatusOK, schema.SSHKeyListResponse{SSHKeys: []schema.SSHKey{}})
		})

		// Mock server type lookup (needed first)
		ts.handleFunc("/server_types", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ServerTypeListResponse{
				ServerTypes: []schema.ServerType{
					{ID: 1, Name: "cx21", Architecture: "x86"},
				},
			})
		})

		// Mock image lookup
		ts.handleFunc("/images", func(w http.ResponseWriter, _ *http.Request) {
			imageName := "ubuntu-22.04"
			jsonResponse(w, http.StatusOK, schema.ImageListResponse{
				Images: []schema.Image{
					{ID: 10, Name: &imageName, Type: "system", Architecture: "x86", Status: "available"},
				},
			})
		})

		client := ts.realClient()
		ctx := context.Background()

		_, err := client.CreateServer(ctx, ServerCreateOpts{
			Name: "test-server", ImageType: "ubuntu-22.04", ServerType: "cx21",
			SSHKeys: []string{"nonexistent-key"}, EnablePublicIPv4: true, EnablePublicIPv6: true,
		})
		if err == nil {
			t.Fatal("expected error for nonexistent SSH key")
		}
	})

	t.Run("resolves location correctly", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		// Mock server type lookup
		ts.handleFunc("/server_types", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ServerTypeListResponse{
				ServerTypes: []schema.ServerType{
					{ID: 1, Name: "cx21", Architecture: "x86"},
				},
			})
		})

		// Mock image lookup
		ts.handleFunc("/images", func(w http.ResponseWriter, _ *http.Request) {
			imageName := "ubuntu-22.04"
			jsonResponse(w, http.StatusOK, schema.ImageListResponse{
				Images: []schema.Image{
					{ID: 10, Name: &imageName, Type: "system", Architecture: "x86", Status: "available"},
				},
			})
		})

		// Mock SSH key lookup - return empty since we pass empty slice
		ts.handleFunc("/ssh_keys", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.SSHKeyListResponse{SSHKeys: []schema.SSHKey{}})
		})

		// Mock location lookup - return empty to trigger "not found" error
		ts.handleFunc("/locations", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.LocationListResponse{
				Locations: []schema.Location{},
			})
		})

		client := ts.realClient()
		ctx := context.Background()

		_, err := client.CreateServer(ctx, ServerCreateOpts{
			Name: "test-server", ImageType: "ubuntu-22.04", ServerType: "cx21", Location: "nonexistent-location",
			SSHKeys: []string{}, EnablePublicIPv4: true, EnablePublicIPv6: true,
		})
		if err == nil {
			t.Fatal("expected error for nonexistent location")
		}
	})

	t.Run("creates server without public IPv4", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		ts.handleFunc("/server_types", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ServerTypeListResponse{
				ServerTypes: []schema.ServerType{{ID: 1, Name: "cx21", Architecture: "x86"}},
			})
		})
		ts.handleFunc("/images", func(w http.ResponseWriter, _ *http.Request) {
			imageName := "ubuntu-22.04"
			jsonResponse(w, http.StatusOK, schema.ImageListResponse{
				Images: []schema.Image{{ID: 10, Name: &imageName, Type: "system", Architecture: "x86", Status: "available"}},
			})
		})
		ts.handleFunc("/locations", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.LocationListResponse{
				Locations: []schema.Location{{ID: 1, Name: "nbg1"}},
			})
		})
		ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				jsonResponse(w, http.StatusCreated, schema.ServerCreateResponse{
					Server: schema.Server{ID: 1001, Name: "ipv6-server"},
					Action: schema.Action{ID: 102, Status: "success"},
				})
				return
			}
			jsonResponse(w, http.StatusOK, schema.ServerListResponse{Servers: []schema.Server{}})
		})
		ts.handleFunc("/actions/102", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
				Action: schema.Action{ID: 102, Status: "success", Progress: 100},
			})
		})

		client := ts.realClient()
		ctx := context.Background()

		// Create with only IPv6 enabled, no IPv4
		serverID, err := client.CreateServer(ctx, ServerCreateOpts{
			Name: "ipv6-server", ImageType: "ubuntu-22.04", ServerType: "cx21", Location: "nbg1",
			SSHKeys: []string{}, EnablePublicIPv4: false, EnablePublicIPv6: true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if serverID != "1001" {
			t.Errorf("expected server ID '1001', got %q", serverID)
		}
	})

	t.Run("server type not found", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		ts.handleFunc("/server_types", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ServerTypeListResponse{
				ServerTypes: []schema.ServerType{},
			})
		})

		client := ts.realClient()
		ctx := context.Background()

		_, err := client.CreateServer(ctx, ServerCreateOpts{
			Name: "test", ImageType: "ubuntu-22.04", ServerType: "nonexistent-type", Location: "nbg1",
			SSHKeys: []string{}, EnablePublicIPv4: true, EnablePublicIPv6: true,
		})
		if err == nil {
			t.Fatal("expected error for nonexistent server type")
		}
	})

	t.Run("resolves talos image by label", func(t *testing.T) {
		ts := newTestServer()
		defer ts.close()

		// Mock server type lookup
		ts.handleFunc("/server_types", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ServerTypeListResponse{
				ServerTypes: []schema.ServerType{
					{ID: 1, Name: "cx21", Architecture: "x86"},
				},
			})
		})

		// Mock image lookup - return image with os=talos label
		ts.handleFunc("/images", func(w http.ResponseWriter, r *http.Request) {
			labelSelector := r.URL.Query().Get("label_selector")
			if labelSelector == "os=talos" {
				imageName := "talos-v1.7.0"
				jsonResponse(w, http.StatusOK, schema.ImageListResponse{
					Images: []schema.Image{
						{ID: 10, Name: &imageName, Type: "snapshot", Architecture: "x86", Status: "available", Labels: map[string]string{"os": "talos"}},
					},
				})
				return
			}
			jsonResponse(w, http.StatusOK, schema.ImageListResponse{Images: []schema.Image{}})
		})

		// Mock location lookup
		ts.handleFunc("/locations", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.LocationListResponse{
				Locations: []schema.Location{
					{ID: 1, Name: "nbg1"},
				},
			})
		})

		// Mock server creation
		ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				jsonResponse(w, http.StatusCreated, schema.ServerCreateResponse{
					Server: schema.Server{ID: 1000, Name: "talos-server"},
					Action: schema.Action{ID: 101, Status: "success"},
				})
				return
			}
			jsonResponse(w, http.StatusOK, schema.ServerListResponse{Servers: []schema.Server{}})
		})

		ts.handleFunc("/actions/101", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
				Action: schema.Action{ID: 101, Status: "success", Progress: 100},
			})
		})

		client := ts.realClient()
		ctx := context.Background()

		serverID, err := client.CreateServer(ctx, ServerCreateOpts{
			Name: "talos-server", ImageType: "talos", ServerType: "cx21", Location: "nbg1",
			SSHKeys: []string{}, EnablePublicIPv4: true, EnablePublicIPv6: true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if serverID != "1000" {
			t.Errorf("expected server ID '1000', got %q", serverID)
		}
	})
}

func TestRealClient_GetServerIP_IPv6Only(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "ipv6-server" {
			jsonResponse(w, http.StatusOK, schema.ServerListResponse{
				Servers: []schema.Server{
					{
						ID:   500,
						Name: "ipv6-server",
						PublicNet: schema.ServerPublicNet{
							IPv4: schema.ServerPublicNetIPv4{
								// empty IPv4 IP
							},
							IPv6: schema.ServerPublicNetIPv6{
								IP: "2001:db8::/64",
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
	// Should return the IPv6 address with ::1 suffix
	if ip == "" {
		t.Fatal("expected non-empty IP for IPv6-only server")
	}
	// Verify it is an IPv6 address containing a colon
	if !containsColon(ip) {
		t.Errorf("expected IPv6 address, got %q", ip)
	}
}

func TestRealClient_GetServerIP_NoPublicIP(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "no-ip-server" {
			jsonResponse(w, http.StatusOK, schema.ServerListResponse{
				Servers: []schema.Server{
					{
						ID:   501,
						Name: "no-ip-server",
						PublicNet: schema.ServerPublicNet{
							IPv4: schema.ServerPublicNetIPv4{},
							IPv6: schema.ServerPublicNetIPv6{},
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

	_, err := client.GetServerIP(ctx, "no-ip-server")
	if err == nil {
		t.Fatal("expected error for server with no public IP")
	}
	if err.Error() != "server has no public IP (neither IPv4 nor IPv6)" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRealClient_GetServerIP_APIError(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/servers", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	})

	client := ts.realClient()
	ctx := context.Background()

	_, err := client.GetServerIP(ctx, "test-server")
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

func TestRealClient_GetServerByName_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "existing-server" {
			jsonResponse(w, http.StatusOK, schema.ServerListResponse{
				Servers: []schema.Server{
					{ID: 777, Name: "existing-server"},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.ServerListResponse{Servers: []schema.Server{}})
	})

	client := ts.realClient()
	ctx := context.Background()

	t.Run("server found", func(t *testing.T) {
		server, err := client.GetServerByName(ctx, "existing-server")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if server == nil {
			t.Fatal("expected server, got nil")
		}
		if server.ID != 777 {
			t.Errorf("expected ID 777, got %d", server.ID)
		}
	})

	t.Run("server not found", func(t *testing.T) {
		server, err := client.GetServerByName(ctx, "nonexistent")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if server != nil {
			t.Errorf("expected nil for nonexistent server, got %v", server)
		}
	})

	t.Run("API error", func(t *testing.T) {
		tsErr := newTestServer()
		defer tsErr.close()
		tsErr.handleFunc("/servers", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		})
		errClient := tsErr.realClient()
		_, err := errClient.GetServerByName(ctx, "test-server")
		if err == nil {
			t.Fatal("expected error for API failure")
		}
	})
}

func TestRealClient_GetServersByLabel_Error(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/servers", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	})

	client := ts.realClient()
	ctx := context.Background()

	_, err := client.GetServersByLabel(ctx, map[string]string{"cluster": "test"})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

func TestRealClient_GetServerID_APIError(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/servers", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	})

	client := ts.realClient()
	ctx := context.Background()

	_, err := client.GetServerID(ctx, "test-server")
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

func TestRealClient_EnableRescue_APIError(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/servers/123/actions/enable_rescue", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	})

	client := ts.realClient()
	ctx := context.Background()

	_, err := client.EnableRescue(ctx, "123", []string{"456"})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

func TestRealClient_ResetServer_APIError(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/servers/123/actions/reset", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	})

	client := ts.realClient()
	ctx := context.Background()

	err := client.ResetServer(ctx, "123")
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

func TestRealClient_PoweroffServer_APIError(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/servers/123/actions/poweroff", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	})

	client := ts.realClient()
	ctx := context.Background()

	err := client.PoweroffServer(ctx, "123")
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

func TestRealClient_GetServersByLabel_EmptyLabels(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/servers", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ServerListResponse{
			Servers: []schema.Server{},
		})
	})

	client := ts.realClient()
	ctx := context.Background()

	servers, err := client.GetServersByLabel(ctx, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(servers) != 0 {
		t.Errorf("expected empty slice, got %d servers", len(servers))
	}
}

// containsColon is a helper for checking IPv6 addresses.
func containsColon(s string) bool {
	for _, c := range s {
		if c == ':' {
			return true
		}
	}
	return false
}

// Tests for AttachServerToNetwork (0% coverage on RealClient)

func TestRealClient_AttachServerToNetwork_ServerNotFound(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/servers", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ServerListResponse{
			Servers: []schema.Server{},
		})
	})

	client := ts.realClient()
	ctx := context.Background()

	err := client.AttachServerToNetwork(ctx, "nonexistent-server", 100, "10.0.0.5")
	if err == nil {
		t.Fatal("expected error for nonexistent server")
	}
	if err.Error() != "server not found: nonexistent-server" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRealClient_AttachServerToNetwork_APIError(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/servers", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	})

	client := ts.realClient()
	ctx := context.Background()

	err := client.AttachServerToNetwork(ctx, "test-server", 100, "10.0.0.5")
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

func TestRealClient_AttachServerToNetwork_AlreadyAttachedAndRunning(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "attached-server" {
			jsonResponse(w, http.StatusOK, schema.ServerListResponse{
				Servers: []schema.Server{
					{
						ID:     123,
						Name:   "attached-server",
						Status: "running",
						PrivateNet: []schema.ServerPrivateNet{
							{
								Network: 100,
								IP:      "10.0.0.5",
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

	// Server already attached and running - should be a no-op
	err := client.AttachServerToNetwork(ctx, "attached-server", 100, "10.0.0.5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRealClient_AttachServerToNetwork_AlreadyAttachedButStopped(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "stopped-server" {
			jsonResponse(w, http.StatusOK, schema.ServerListResponse{
				Servers: []schema.Server{
					{
						ID:     124,
						Name:   "stopped-server",
						Status: "off",
						PrivateNet: []schema.ServerPrivateNet{
							{
								Network: 100,
								IP:      "10.0.0.5",
							},
						},
					},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.ServerListResponse{Servers: []schema.Server{}})
	})

	ts.handleFunc("/servers/124/actions/poweron", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ServerActionPoweronResponse{
			Action: schema.Action{ID: 90, Status: "success"},
		})
	})

	ts.handleFunc("/actions/90", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
			Action: schema.Action{ID: 90, Status: "success", Progress: 100},
		})
	})

	client := ts.realClient()
	ctx := context.Background()

	// Server already attached but stopped - should power on
	err := client.AttachServerToNetwork(ctx, "stopped-server", 100, "10.0.0.5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRealClient_AttachServerToNetwork_NotAttachedYet(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "new-server" {
			jsonResponse(w, http.StatusOK, schema.ServerListResponse{
				Servers: []schema.Server{
					{
						ID:         125,
						Name:       "new-server",
						Status:     "off",
						PrivateNet: []schema.ServerPrivateNet{}, // No networks
					},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.ServerListResponse{Servers: []schema.Server{}})
	})

	ts.handleFunc("/servers/125/actions/attach_to_network", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ServerActionAttachToNetworkResponse{
			Action: schema.Action{ID: 91, Status: "success"},
		})
	})

	ts.handleFunc("/servers/125/actions/poweron", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ServerActionPoweronResponse{
			Action: schema.Action{ID: 92, Status: "success"},
		})
	})

	ts.handleFunc("/actions/91", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
			Action: schema.Action{ID: 91, Status: "success", Progress: 100},
		})
	})

	ts.handleFunc("/actions/92", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
			Action: schema.Action{ID: 92, Status: "success", Progress: 100},
		})
	})

	client := ts.realClient()
	ctx := context.Background()

	err := client.AttachServerToNetwork(ctx, "new-server", 100, "10.0.0.5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRealClient_CreateServer_WithNetworkAttachment(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/server_types", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ServerTypeListResponse{
			ServerTypes: []schema.ServerType{{ID: 1, Name: "cx21", Architecture: "x86"}},
		})
	})

	imageName := "ubuntu-22.04"
	ts.handleFunc("/images", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ImageListResponse{
			Images: []schema.Image{{ID: 10, Name: &imageName, Type: "system", Architecture: "x86", Status: "available"}},
		})
	})

	ts.handleFunc("/locations", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.LocationListResponse{
			Locations: []schema.Location{{ID: 1, Name: "nbg1"}},
		})
	})

	ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			jsonResponse(w, http.StatusCreated, schema.ServerCreateResponse{
				Server: schema.Server{
					ID:     200,
					Name:   "server-with-net",
					Status: "off",
				},
				Action: schema.Action{ID: 110, Status: "success"},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.ServerListResponse{Servers: []schema.Server{}})
	})

	ts.handleFunc("/actions/110", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
			Action: schema.Action{ID: 110, Status: "success", Progress: 100},
		})
	})

	ts.handleFunc("/servers/200/actions/attach_to_network", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ServerActionAttachToNetworkResponse{
			Action: schema.Action{ID: 111, Status: "success"},
		})
	})

	ts.handleFunc("/actions/111", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
			Action: schema.Action{ID: 111, Status: "success", Progress: 100},
		})
	})

	ts.handleFunc("/servers/200/actions/poweron", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ServerActionPoweronResponse{
			Action: schema.Action{ID: 112, Status: "success"},
		})
	})

	ts.handleFunc("/actions/112", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
			Action: schema.Action{ID: 112, Status: "success", Progress: 100},
		})
	})

	client := ts.realClient()
	ctx := context.Background()

	serverID, err := client.CreateServer(ctx, ServerCreateOpts{
		Name:            "server-with-net",
		ImageType:       "ubuntu-22.04",
		ServerType:      "cx21",
		Location:        "nbg1",
		SSHKeys:         []string{},
		NetworkID:       100,
		PrivateIP:       "10.0.0.5",
		EnablePublicIPv4: true,
		EnablePublicIPv6: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if serverID != "200" {
		t.Errorf("expected server ID '200', got %q", serverID)
	}
}

func TestRealClient_CreateServer_RetryOnTransientError(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/server_types", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ServerTypeListResponse{
			ServerTypes: []schema.ServerType{{ID: 1, Name: "cx21", Architecture: "x86"}},
		})
	})

	imageName := "ubuntu-22.04"
	ts.handleFunc("/images", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ImageListResponse{
			Images: []schema.Image{{ID: 10, Name: &imageName, Type: "system", Architecture: "x86", Status: "available"}},
		})
	})

	ts.handleFunc("/locations", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.LocationListResponse{
			Locations: []schema.Location{{ID: 1, Name: "nbg1"}},
		})
	})

	// Server creation fails on first attempt, succeeds on second
	attempt := 0
	ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			attempt++
			if attempt == 1 {
				// Transient error (not an invalid parameter)
				http.Error(w, "service temporarily unavailable", http.StatusServiceUnavailable)
				return
			}
			jsonResponse(w, http.StatusCreated, schema.ServerCreateResponse{
				Server: schema.Server{ID: 300, Name: "retry-server"},
				Action: schema.Action{ID: 120, Status: "success"},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.ServerListResponse{Servers: []schema.Server{}})
	})

	ts.handleFunc("/actions/120", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
			Action: schema.Action{ID: 120, Status: "success", Progress: 100},
		})
	})

	client := ts.realClient()
	ctx := context.Background()

	serverID, err := client.CreateServer(ctx, ServerCreateOpts{
		Name:             "retry-server",
		ImageType:        "ubuntu-22.04",
		ServerType:       "cx21",
		Location:         "nbg1",
		SSHKeys:          []string{},
		EnablePublicIPv4: true,
		EnablePublicIPv6: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if serverID != "300" {
		t.Errorf("expected server ID '300', got %q", serverID)
	}
	if attempt < 2 {
		t.Errorf("expected at least 2 attempts, got %d", attempt)
	}
}

func TestRealClient_CreateServer_FatalErrorNoRetry(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/server_types", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ServerTypeListResponse{
			ServerTypes: []schema.ServerType{{ID: 1, Name: "cx21", Architecture: "x86"}},
		})
	})

	imageName := "ubuntu-22.04"
	ts.handleFunc("/images", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ImageListResponse{
			Images: []schema.Image{{ID: 10, Name: &imageName, Type: "system", Architecture: "x86", Status: "available"}},
		})
	})

	ts.handleFunc("/locations", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.LocationListResponse{
			Locations: []schema.Location{{ID: 1, Name: "nbg1"}},
		})
	})

	attempt := 0
	ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			attempt++
			// Return an "invalid_input" error which should NOT be retried
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"code":    "invalid_input",
					"message": "invalid server name",
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.ServerListResponse{Servers: []schema.Server{}})
	})

	client := ts.realClient()
	ctx := context.Background()

	_, err := client.CreateServer(ctx, ServerCreateOpts{
		Name:             "bad-server",
		ImageType:        "ubuntu-22.04",
		ServerType:       "cx21",
		Location:         "nbg1",
		SSHKeys:          []string{},
		EnablePublicIPv4: true,
		EnablePublicIPv6: true,
	})
	if err == nil {
		t.Fatal("expected error for invalid input")
	}
	// Fatal errors should NOT be retried - only 1 attempt
	if attempt != 1 {
		t.Errorf("expected exactly 1 attempt for fatal error, got %d", attempt)
	}
}

func TestRealClient_AttachServerToNetwork_PowerOnError(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "stopped-server" {
			jsonResponse(w, http.StatusOK, schema.ServerListResponse{
				Servers: []schema.Server{
					{
						ID:     126,
						Name:   "stopped-server",
						Status: "off",
						PrivateNet: []schema.ServerPrivateNet{
							{
								Network: 100,
								IP:      "10.0.0.5",
							},
						},
					},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.ServerListResponse{Servers: []schema.Server{}})
	})

	ts.handleFunc("/servers/126/actions/poweron", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "power on failed", http.StatusInternalServerError)
	})

	client := ts.realClient()
	ctx := context.Background()

	err := client.AttachServerToNetwork(ctx, "stopped-server", 100, "10.0.0.5")
	if err == nil {
		t.Fatal("expected error for power on failure")
	}
}
