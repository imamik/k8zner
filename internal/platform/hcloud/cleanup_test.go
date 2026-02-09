package hcloud

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/hetznercloud/hcloud-go/v2/hcloud/schema"
)

func TestCleanupError(t *testing.T) {
	t.Parallel()
	t.Run("single error", func(t *testing.T) {
		t.Parallel()
		ce := &CleanupError{}
		ce.Add(errors.New("test error"))

		if !ce.HasErrors() {
			t.Error("expected HasErrors() to return true")
		}

		if ce.Error() != "test error" {
			t.Errorf("expected 'test error', got %q", ce.Error())
		}
	})

	t.Run("multiple errors", func(t *testing.T) {
		t.Parallel()
		ce := &CleanupError{}
		ce.Add(errors.New("error 1"))
		ce.Add(errors.New("error 2"))

		if !ce.HasErrors() {
			t.Error("expected HasErrors() to return true")
		}

		errStr := ce.Error()
		if errStr != "cleanup encountered 2 errors: [error 1 error 2]" {
			t.Errorf("unexpected error message: %q", errStr)
		}
	})

	t.Run("no errors", func(t *testing.T) {
		t.Parallel()
		ce := &CleanupError{}

		if ce.HasErrors() {
			t.Error("expected HasErrors() to return false")
		}
	})

	t.Run("add nil error", func(t *testing.T) {
		t.Parallel()
		ce := &CleanupError{}
		ce.Add(nil)

		if ce.HasErrors() {
			t.Error("adding nil should not create an error")
		}
	})

	t.Run("unwrap single error", func(t *testing.T) {
		t.Parallel()
		original := errors.New("original error")
		ce := &CleanupError{}
		ce.Add(original)

		if !errors.Is(ce.Unwrap(), original) {
			t.Error("Unwrap should return the original error")
		}
	})
}

// TestBuildLabelSelector is already tested in real_client_test.go

func TestGetResourceInfo(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		resource   interface{}
		expectName string
		expectID   int64
	}{
		{
			name:       "server",
			resource:   &hcloud.Server{ID: 1, Name: "server-1"},
			expectName: "server-1",
			expectID:   1,
		},
		{
			name:       "load balancer",
			resource:   &hcloud.LoadBalancer{ID: 2, Name: "lb-1"},
			expectName: "lb-1",
			expectID:   2,
		},
		{
			name:       "firewall",
			resource:   &hcloud.Firewall{ID: 4, Name: "fw-1"},
			expectName: "fw-1",
			expectID:   4,
		},
		{
			name:       "network",
			resource:   &hcloud.Network{ID: 5, Name: "net-1"},
			expectName: "net-1",
			expectID:   5,
		},
		{
			name:       "placement group",
			resource:   &hcloud.PlacementGroup{ID: 6, Name: "pg-1"},
			expectName: "pg-1",
			expectID:   6,
		},
		{
			name:       "SSH key",
			resource:   &hcloud.SSHKey{ID: 7, Name: "key-1"},
			expectName: "key-1",
			expectID:   7,
		},
		{
			name:       "certificate",
			resource:   &hcloud.Certificate{ID: 8, Name: "cert-1"},
			expectName: "cert-1",
			expectID:   8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var info resourceInfo
			switch v := tt.resource.(type) {
			case *hcloud.Server:
				info = getResourceInfo(v)
			case *hcloud.LoadBalancer:
				info = getResourceInfo(v)
			case *hcloud.Firewall:
				info = getResourceInfo(v)
			case *hcloud.Network:
				info = getResourceInfo(v)
			case *hcloud.PlacementGroup:
				info = getResourceInfo(v)
			case *hcloud.SSHKey:
				info = getResourceInfo(v)
			case *hcloud.Certificate:
				info = getResourceInfo(v)
			}

			if info.Name != tt.expectName {
				t.Errorf("expected name %q, got %q", tt.expectName, info.Name)
			}
			if info.ID != tt.expectID {
				t.Errorf("expected ID %d, got %d", tt.expectID, info.ID)
			}
		})
	}
}

func TestRealClient_CleanupByLabel_WithHTTPMock(t *testing.T) {
	t.Parallel()
	ts := newTestServer()
	defer ts.close()

	// Mock all endpoints with empty lists - this tests the happy path with no resources
	ts.handleFunc("/servers", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ServerListResponse{Servers: []schema.Server{}})
	})

	ts.handleFunc("/volumes", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.VolumeListResponse{Volumes: []schema.Volume{}})
	})

	ts.handleFunc("/load_balancers", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.LoadBalancerListResponse{LoadBalancers: []schema.LoadBalancer{}})
	})

	ts.handleFunc("/firewalls", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.FirewallListResponse{Firewalls: []schema.Firewall{}})
	})

	ts.handleFunc("/networks", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.NetworkListResponse{Networks: []schema.Network{}})
	})

	ts.handleFunc("/placement_groups", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.PlacementGroupListResponse{PlacementGroups: []schema.PlacementGroup{}})
	})

	ts.handleFunc("/ssh_keys", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.SSHKeyListResponse{SSHKeys: []schema.SSHKey{}})
	})

	ts.handleFunc("/certificates", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.CertificateListResponse{Certificates: []schema.Certificate{}})
	})

	client := ts.realClient()
	ctx := context.Background()

	err := client.CleanupByLabel(ctx, map[string]string{"cluster": "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRealClient_DeleteServersByLabel_WithHTTPMock(t *testing.T) {
	t.Parallel()
	t.Run("deletes servers successfully", func(t *testing.T) {
		t.Parallel()
		ts := newTestServer()
		defer ts.close()

		callCount := 0
		ts.handleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
			callCount++
			// First call returns servers to delete, subsequent calls return empty
			if callCount == 1 {
				jsonResponse(w, http.StatusOK, schema.ServerListResponse{
					Servers: []schema.Server{
						{ID: 1, Name: "server-1", Labels: map[string]string{"cluster": "test"}},
						{ID: 2, Name: "server-2", Labels: map[string]string{"cluster": "test"}},
					},
				})
				return
			}
			// After first call, return empty (servers deleted)
			jsonResponse(w, http.StatusOK, schema.ServerListResponse{Servers: []schema.Server{}})
		})

		ts.handleFunc("/servers/1", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodDelete {
				jsonResponse(w, http.StatusOK, schema.ServerDeleteResponse{
					Action: schema.Action{ID: 1, Status: "success"},
				})
				return
			}
		})

		ts.handleFunc("/servers/2", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodDelete {
				jsonResponse(w, http.StatusOK, schema.ServerDeleteResponse{
					Action: schema.Action{ID: 2, Status: "success"},
				})
				return
			}
		})

		client := ts.realClient()
		ctx := context.Background()

		err := client.deleteServersByLabel(ctx, "cluster=test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("handles empty label selector", func(t *testing.T) {
		t.Parallel()
		ts := newTestServer()
		defer ts.close()

		ts.handleFunc("/servers", func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, http.StatusOK, schema.ServerListResponse{Servers: []schema.Server{}})
		})

		client := ts.realClient()
		ctx := context.Background()

		err := client.deleteServersByLabel(ctx, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRealClient_DeleteLoadBalancersByLabel_WithHTTPMock(t *testing.T) {
	t.Parallel()
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/load_balancers", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, schema.LoadBalancerListResponse{
			LoadBalancers: []schema.LoadBalancer{
				{ID: 1, Name: "lb-1", Labels: map[string]string{"cluster": "test"}},
			},
		})
	})

	ts.handleFunc("/load_balancers/1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	})

	client := ts.realClient()
	ctx := context.Background()

	err := client.deleteLoadBalancersByLabel(ctx, "cluster=test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRealClient_DeleteFirewallsByLabel_WithHTTPMock(t *testing.T) {
	t.Parallel()
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/firewalls", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, schema.FirewallListResponse{
			Firewalls: []schema.Firewall{
				{ID: 1, Name: "fw-1", Labels: map[string]string{"cluster": "test"}},
			},
		})
	})

	ts.handleFunc("/firewalls/1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	})

	client := ts.realClient()
	ctx := context.Background()

	err := client.deleteFirewallsByLabel(ctx, "cluster=test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRealClient_DeleteNetworksByLabel_WithHTTPMock(t *testing.T) {
	t.Parallel()
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/networks", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, schema.NetworkListResponse{
			Networks: []schema.Network{
				{ID: 1, Name: "net-1", Labels: map[string]string{"cluster": "test"}},
			},
		})
	})

	ts.handleFunc("/networks/1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	})

	client := ts.realClient()
	ctx := context.Background()

	err := client.deleteNetworksByLabel(ctx, "cluster=test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRealClient_DeletePlacementGroupsByLabel_WithHTTPMock(t *testing.T) {
	t.Parallel()
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/placement_groups", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, schema.PlacementGroupListResponse{
			PlacementGroups: []schema.PlacementGroup{
				{ID: 1, Name: "pg-1", Labels: map[string]string{"cluster": "test"}},
			},
		})
	})

	ts.handleFunc("/placement_groups/1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	})

	client := ts.realClient()
	ctx := context.Background()

	err := client.deletePlacementGroupsByLabel(ctx, "cluster=test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRealClient_DeleteSSHKeysByLabel_WithHTTPMock(t *testing.T) {
	t.Parallel()
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/ssh_keys", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, schema.SSHKeyListResponse{
			SSHKeys: []schema.SSHKey{
				{ID: 1, Name: "key-1", Labels: map[string]string{"cluster": "test"}},
			},
		})
	})

	ts.handleFunc("/ssh_keys/1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	})

	client := ts.realClient()
	ctx := context.Background()

	err := client.deleteSSHKeysByLabel(ctx, "cluster=test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRealClient_DeleteCertificatesByLabel_WithHTTPMock(t *testing.T) {
	t.Parallel()
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/certificates", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, schema.CertificateListResponse{
			Certificates: []schema.Certificate{
				{ID: 1, Name: "cert-1", Labels: map[string]string{"cluster": "test"}},
			},
		})
	})

	ts.handleFunc("/certificates/1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	})

	client := ts.realClient()
	ctx := context.Background()

	err := client.deleteCertificatesByLabel(ctx, "cluster=test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRealClient_DeleteVolumesByLabel_WithHTTPMock(t *testing.T) {
	t.Parallel()
	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/volumes", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, schema.VolumeListResponse{
			Volumes: []schema.Volume{
				{ID: 1, Name: "vol-1", Labels: map[string]string{"cluster": "test"}},
			},
		})
	})

	ts.handleFunc("/volumes/1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	})

	client := ts.realClient()
	ctx := context.Background()

	err := client.deleteVolumesByLabel(ctx, "cluster=test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRealClient_CountResourcesByLabel_WithHTTPMock(t *testing.T) {
	t.Parallel()
	ts := newTestServer()
	defer ts.close()

	// Mock all endpoints with varying counts
	ts.handleFunc("/servers", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ServerListResponse{
			Servers: []schema.Server{{ID: 1, Name: "server-1"}},
		})
	})

	ts.handleFunc("/volumes", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.VolumeListResponse{
			Volumes: []schema.Volume{{ID: 1, Name: "vol-1"}, {ID: 2, Name: "vol-2"}},
		})
	})

	ts.handleFunc("/load_balancers", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.LoadBalancerListResponse{LoadBalancers: []schema.LoadBalancer{}})
	})

	ts.handleFunc("/firewalls", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.FirewallListResponse{Firewalls: []schema.Firewall{}})
	})

	ts.handleFunc("/networks", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.NetworkListResponse{Networks: []schema.Network{}})
	})

	ts.handleFunc("/placement_groups", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.PlacementGroupListResponse{PlacementGroups: []schema.PlacementGroup{}})
	})

	ts.handleFunc("/ssh_keys", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.SSHKeyListResponse{SSHKeys: []schema.SSHKey{}})
	})

	ts.handleFunc("/certificates", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.CertificateListResponse{Certificates: []schema.Certificate{}})
	})

	client := ts.realClient()
	ctx := context.Background()

	remaining, err := client.CountResourcesByLabel(ctx, map[string]string{"cluster": "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if remaining.Servers != 1 {
		t.Errorf("expected 1 server, got %d", remaining.Servers)
	}
	if remaining.Volumes != 2 {
		t.Errorf("expected 2 volumes, got %d", remaining.Volumes)
	}
	if remaining.Total() != 3 {
		t.Errorf("expected total 3, got %d", remaining.Total())
	}

	expectedStr := "[1 servers 2 volumes]"
	if remaining.String() != expectedStr {
		t.Errorf("expected %q, got %q", expectedStr, remaining.String())
	}
}

func TestRemainingResources_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		r        RemainingResources
		expected string
	}{
		{
			name:     "no resources",
			r:        RemainingResources{},
			expected: "no resources",
		},
		{
			name:     "single type",
			r:        RemainingResources{Servers: 2},
			expected: "[2 servers]",
		},
		{
			name:     "multiple types",
			r:        RemainingResources{Servers: 1, Volumes: 2, SSHKeys: 3},
			expected: "[1 servers 2 volumes 3 SSH keys]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.r.String(); got != tt.expected {
				t.Errorf("String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDeleteResourcesByLabel(t *testing.T) {
	t.Parallel(
	// Test the generic deleteResourcesByLabel function
	)

	t.Run("handles list error", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		err := deleteResourcesByLabel(ctx, "test",
			func(ctx context.Context) ([]*hcloud.Server, error) {
				return nil, context.DeadlineExceeded
			},
			func(ctx context.Context, s *hcloud.Server) error {
				return nil
			},
		)
		if err == nil {
			t.Fatal("expected error from list function")
		}
	})

	t.Run("returns delete errors", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		// deleteResourcesByLabel now returns errors for failed deletions
		err := deleteResourcesByLabel(ctx, "test",
			func(ctx context.Context) ([]*hcloud.Server, error) {
				return []*hcloud.Server{{ID: 1, Name: "test-server"}}, nil
			},
			func(ctx context.Context, s *hcloud.Server) error {
				return context.DeadlineExceeded
			},
		)
		// Should return error when delete fails
		if err == nil {
			t.Fatal("expected error from delete function")
		}
	})

	t.Run("handles empty list", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		err := deleteResourcesByLabel(ctx, "test",
			func(ctx context.Context) ([]*hcloud.Server, error) {
				return []*hcloud.Server{}, nil
			},
			func(ctx context.Context, s *hcloud.Server) error {
				t.Error("delete should not be called for empty list")
				return nil
			},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
