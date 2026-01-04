package cluster

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/config"
	hcloud_internal "github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockTalosProducer implementation.
type MockTalosProducer struct {
	mock.Mock
}

func (m *MockTalosProducer) GenerateControlPlaneConfig(san []string) ([]byte, error) {
	args := m.Called(san)
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockTalosProducer) GenerateWorkerConfig() ([]byte, error) {
	args := m.Called()
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockTalosProducer) GetClientConfig() ([]byte, error) {
	args := m.Called()
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockTalosProducer) SetEndpoint(endpoint string) {
	m.Called(endpoint)
}

func TestReconciler_Reconcile(t *testing.T) {
	// Setup Mocks
	mockInfra := &hcloud_internal.MockClient{}
	mockTalos := &MockTalosProducer{}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{
					Name:       "control-plane",
					ServerType: "cx21",
					Count:      1,
				},
			},
		},
		Workers: []config.WorkerNodePool{
			{
				Name:       "worker",
				ServerType: "cx21",
				Count:      1,
			},
		},
		Ingress: config.IngressConfig{
			Enabled: false,
		},
	}

	// Mock Expectations
	ctx := context.Background()

	// Network
	mockInfra.EnsureNetworkFunc = func(_ context.Context, _, ipRange, _ string, _ map[string]string) (*hcloud.Network, error) {
		_, ipNet, _ := net.ParseCIDR(ipRange)
		return &hcloud.Network{ID: 1, IPRange: ipNet}, nil
	}
	mockInfra.EnsureSubnetFunc = func(_ context.Context, _ *hcloud.Network, _, _ string, _ hcloud.NetworkSubnetType) error {
		return nil
	}
	mockInfra.GetPublicIPFunc = func(_ context.Context) (string, error) {
		return "1.2.3.4", nil
	}

	// Firewall
	mockInfra.EnsureFirewallFunc = func(_ context.Context, _ string, _ []hcloud.FirewallRule, _ map[string]string) (*hcloud.Firewall, error) {
		return &hcloud.Firewall{ID: 1}, nil
	}

	// LoadBalancer
	mockInfra.EnsureLoadBalancerFunc = func(_ context.Context, _, _, _ string, _ hcloud.LoadBalancerAlgorithmType, _ map[string]string) (*hcloud.LoadBalancer, error) {
		return &hcloud.LoadBalancer{
			ID: 1,
			PublicNet: hcloud.LoadBalancerPublicNet{
				IPv4: hcloud.LoadBalancerPublicNetIPv4{
					IP: net.ParseIP("5.6.7.8"),
				},
			},
		}, nil
	}
	mockInfra.GetLoadBalancerFunc = func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
		// Used in reconcileControlPlane to get IP
		return &hcloud.LoadBalancer{
			ID: 1,
			PublicNet: hcloud.LoadBalancerPublicNet{
				IPv4: hcloud.LoadBalancerPublicNetIPv4{
					IP: net.ParseIP("5.6.7.8"),
				},
			},
		}, nil
	}

	// Placement Group
	mockInfra.EnsurePlacementGroupFunc = func(_ context.Context, _, _ string, _ map[string]string) (*hcloud.PlacementGroup, error) {
		return &hcloud.PlacementGroup{ID: 1}, nil
	}

	// Floating IP
	mockInfra.EnsureFloatingIPFunc = func(_ context.Context, _, _, _ string, _ map[string]string) (*hcloud.FloatingIP, error) {
		return &hcloud.FloatingIP{ID: 1}, nil
	}

	// Talos
	mockTalos.On("SetEndpoint", "https://5.6.7.8:6443").Return()
	mockTalos.On("GenerateControlPlaneConfig", mock.Anything).Return([]byte("cp-config"), nil)
	mockTalos.On("GenerateWorkerConfig").Return([]byte("worker-config"), nil)
	mockTalos.On("GetClientConfig").Return([]byte("client-config"), nil)

	// Servers
	mockInfra.GetServerIDFunc = func(_ context.Context, _ string) (string, error) {
		return "", nil // Does not exist
	}
	mockInfra.CreateServerFunc = func(_ context.Context, _, _, _, _ string, _ []string, _ map[string]string, _ string, _ *int64, _ int64, _ string) (string, error) {
		return "server-id", nil
	}
	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "10.0.1.2", nil
	}

	// Snapshots - Return existing snapshot so auto-build is skipped in tests
	mockInfra.GetSnapshotByLabelsFunc = func(_ context.Context, labels map[string]string) (*hcloud.Image, error) {
		// Return a mock Talos snapshot
		return &hcloud.Image{
			ID:          123,
			Description: "talos-v1.8.3-k8s-v1.31.0-amd64",
			Labels:      labels,
		}, nil
	}

	// Certificate
	// Used by Bootstrapper to check for state marker
	mockInfra.GetCertificateFunc = func(_ context.Context, name string) (*hcloud.Certificate, error) {
		if name == "test-cluster-state" {
			// Return a mock certificate to simulate that the cluster is already bootstrapped.
			// This avoids the bootstrapper trying to connect to a real node.
			return &hcloud.Certificate{ID: 99, Name: "test-cluster-state"}, nil
		}
		return nil, nil
	}

	r := NewReconciler(mockInfra, mockTalos, cfg)

	// Run Reconcile
	err := r.Reconcile(ctx)
	assert.NoError(t, err)

	// Verify expectations
	mockTalos.AssertExpectations(t)
}

func TestReconciler_Reconcile_NetworkError(t *testing.T) {
	// Setup Mocks
	mockInfra := &hcloud_internal.MockClient{}
	mockTalos := &MockTalosProducer{}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
	}

	ctx := context.Background()

	// Simulate Network Creation Error
	expectedErr := errors.New("network creation failed")
	mockInfra.EnsureNetworkFunc = func(_ context.Context, _, _, _ string, _ map[string]string) (*hcloud.Network, error) {
		return nil, expectedErr
	}

	r := NewReconciler(mockInfra, mockTalos, cfg)

	// Run Reconcile
	err := r.Reconcile(ctx)
	assert.Error(t, err)
	assert.ErrorIs(t, err, expectedErr)
}

func TestReconciler_Reconcile_ServerCreationError(t *testing.T) {
	// Setup Mocks
	mockInfra := &hcloud_internal.MockClient{}
	mockTalos := &MockTalosProducer{}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{
					Name:       "control-plane",
					ServerType: "cx21",
					Count:      1,
				},
			},
		},
	}

	ctx := context.Background()

	// Basic infra success
	mockInfra.EnsureNetworkFunc = func(_ context.Context, _, _, _ string, _ map[string]string) (*hcloud.Network, error) {
		return &hcloud.Network{}, nil
	}
	mockInfra.EnsureSubnetFunc = func(_ context.Context, _ *hcloud.Network, _, _ string, _ hcloud.NetworkSubnetType) error {
		return nil
	}
	mockInfra.GetPublicIPFunc = func(_ context.Context) (string, error) {
		return "1.2.3.4", nil
	}
	mockInfra.EnsureFirewallFunc = func(_ context.Context, _ string, _ []hcloud.FirewallRule, _ map[string]string) (*hcloud.Firewall, error) {
		return &hcloud.Firewall{}, nil
	}
	mockInfra.GetLoadBalancerFunc = func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
		return nil, nil // No LB for this test to simplify
	}

	// Talos config generation success
	mockTalos.On("GenerateControlPlaneConfig", mock.Anything).Return([]byte("cp-config"), nil)

	// Server Creation Error
	expectedErr := errors.New("server creation failed")
	mockInfra.GetServerIDFunc = func(_ context.Context, _ string) (string, error) {
		return "", nil
	}
	mockInfra.EnsurePlacementGroupFunc = func(_ context.Context, _, _ string, _ map[string]string) (*hcloud.PlacementGroup, error) {
		return &hcloud.PlacementGroup{ID: 1}, nil
	}
	mockInfra.CreateServerFunc = func(_ context.Context, _, _, _, _ string, _ []string, _ map[string]string, _ string, _ *int64, _ int64, _ string) (string, error) {
		return "", expectedErr
	}
	mockInfra.GetSnapshotByLabelsFunc = func(_ context.Context, labels map[string]string) (*hcloud.Image, error) {
		return &hcloud.Image{
			ID:          123,
			Description: "talos-v1.8.3-k8s-v1.31.0-amd64",
			Labels:      labels,
		}, nil
	}

	r := NewReconciler(mockInfra, mockTalos, cfg)

	// Run Reconcile
	err := r.Reconcile(ctx)
	assert.Error(t, err)
	// The error might be wrapped
	assert.Contains(t, err.Error(), expectedErr.Error())
}
