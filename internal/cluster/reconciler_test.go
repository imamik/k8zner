package cluster

import (
	"context"
	"net"
	"testing"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/config"
	hcloud_internal "github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockTalosProducer implementation
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
	mockInfra.EnsureNetworkFunc = func(ctx context.Context, name, ipRange, zone string, labels map[string]string) (*hcloud.Network, error) {
		_, ipNet, _ := net.ParseCIDR(ipRange)
		return &hcloud.Network{ID: 1, IPRange: ipNet}, nil
	}
	mockInfra.EnsureSubnetFunc = func(ctx context.Context, network *hcloud.Network, ipRange, networkZone string, subnetType hcloud.NetworkSubnetType) error {
		return nil
	}
	mockInfra.GetPublicIPFunc = func(ctx context.Context) (string, error) {
		return "1.2.3.4", nil
	}

	// Firewall
	mockInfra.EnsureFirewallFunc = func(ctx context.Context, name string, rules []hcloud.FirewallRule, labels map[string]string) (*hcloud.Firewall, error) {
		return &hcloud.Firewall{ID: 1}, nil
	}

	// LoadBalancer
	mockInfra.EnsureLoadBalancerFunc = func(ctx context.Context, name, location, lbType string, algorithm hcloud.LoadBalancerAlgorithmType, labels map[string]string) (*hcloud.LoadBalancer, error) {
		return &hcloud.LoadBalancer{
			ID: 1,
			PublicNet: hcloud.LoadBalancerPublicNet{
				IPv4: hcloud.LoadBalancerPublicNetIPv4{
					IP: net.ParseIP("5.6.7.8"),
				},
			},
		}, nil
	}
	mockInfra.GetLoadBalancerFunc = func(ctx context.Context, name string) (*hcloud.LoadBalancer, error) {
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
	mockInfra.EnsurePlacementGroupFunc = func(ctx context.Context, name, pgType string, labels map[string]string) (*hcloud.PlacementGroup, error) {
		return &hcloud.PlacementGroup{ID: 1}, nil
	}

	// Floating IP
	mockInfra.EnsureFloatingIPFunc = func(ctx context.Context, name, homeLocation, ipType string, labels map[string]string) (*hcloud.FloatingIP, error) {
		return &hcloud.FloatingIP{ID: 1}, nil
	}

	// Talos
	mockTalos.On("SetEndpoint", "https://5.6.7.8:6443").Return()
	mockTalos.On("GenerateControlPlaneConfig", mock.Anything).Return([]byte("cp-config"), nil)
	mockTalos.On("GenerateWorkerConfig").Return([]byte("worker-config"), nil)
	mockTalos.On("GetClientConfig").Return([]byte("client-config"), nil)

	// Servers
	mockInfra.GetServerIDFunc = func(ctx context.Context, name string) (string, error) {
		return "", nil // Does not exist
	}
	mockInfra.CreateServerFunc = func(ctx context.Context, name, imageType, serverType, location string, sshKeys []string, labels map[string]string, userData string, placementGroupID *int64) (string, error) {
		return "server-id", nil
	}
	mockInfra.GetServerIPFunc = func(ctx context.Context, name string) (string, error) {
		return "10.0.1.2", nil
	}

	// Certificate
	// Used by Bootstrapper to check for state marker
	mockInfra.GetCertificateFunc = func(ctx context.Context, name string) (*hcloud.Certificate, error) {
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
