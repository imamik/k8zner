package hcloud

import (
	"context"
	"encoding/json"
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

func TestRealClient_GetFloatingIP_WithHTTPMock(t *testing.T) {
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/floating_ips", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "test-fip" {
			jsonResponse(w, http.StatusOK, schema.FloatingIPListResponse{
				FloatingIPs: []schema.FloatingIP{
					{ID: 500, Name: "test-fip", IP: "1.2.3.4"},
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, schema.FloatingIPListResponse{FloatingIPs: []schema.FloatingIP{}})
	})

	client := ts.realClient()
	ctx := context.Background()

	fip, err := client.GetFloatingIP(ctx, "test-fip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fip == nil {
		t.Fatal("expected floating IP, got nil")
	}
	if fip.ID != 500 {
		t.Errorf("expected ID 500, got %d", fip.ID)
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
	_, err := client.CreateServer(ctx, "test", "image", "type", "loc", nil, nil, "", nil, 123, "")
	if err == nil {
		t.Error("expected validation error for mismatched networkID/privateIP")
	}

	// Test validation: privateIP provided but networkID is 0
	_, err = client.CreateServer(ctx, "test", "image", "type", "loc", nil, nil, "", nil, 0, "10.0.0.1")
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
