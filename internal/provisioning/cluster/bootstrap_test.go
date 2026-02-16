package cluster

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/imamik/k8zner/internal/config"
	hcloud_internal "github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateDummyCert(t *testing.T) {
	t.Parallel()
	cert, key, err := generateDummyCert()
	require.NoError(t, err)

	// Verify certificate is valid PEM
	block, _ := pem.Decode([]byte(cert))
	require.NotNil(t, block, "certificate should be valid PEM")
	assert.Equal(t, "CERTIFICATE", block.Type)

	// Verify it's a valid X.509 certificate
	parsedCert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	assert.Equal(t, "HCloud K8s State Marker", parsedCert.Subject.Organization[0])

	// Verify private key is valid PEM
	keyBlock, _ := pem.Decode([]byte(key))
	require.NotNil(t, keyBlock, "private key should be valid PEM")
	assert.Equal(t, "RSA PRIVATE KEY", keyBlock.Type)

	// Verify it's a valid RSA private key
	_, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	require.NoError(t, err)
}

func TestGetFirstControlPlaneIP(t *testing.T) {
	t.Parallel()

	t.Run("with control plane nodes", func(t *testing.T) {
		t.Parallel()
		ctx := &provisioning.Context{
			State: provisioning.NewState(),
		}
		ctx.State.ControlPlaneIPs = map[string]string{
			"node1": "10.0.0.1",
			"node2": "10.0.0.2",
		}

		ip := getFirstControlPlaneIP(ctx)
		// Should return one of the IPs (map order is not guaranteed)
		assert.Contains(t, []string{"10.0.0.1", "10.0.0.2"}, ip)
	})

	t.Run("with no control plane nodes", func(t *testing.T) {
		t.Parallel()
		ctx := &provisioning.Context{
			State: provisioning.NewState(),
		}
		ctx.State.ControlPlaneIPs = map[string]string{}

		ip := getFirstControlPlaneIP(ctx)
		assert.Equal(t, "", ip)
	})
}

func TestWaitForPort_Success(t *testing.T) {
	t.Parallel()
	// Start a test server

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()

	// Get the port that was assigned
	addr := listener.Addr().(*net.TCPAddr)

	// Wait for port should succeed immediately
	ctx := context.Background()
	timeouts := config.TestTimeouts()
	err = waitForPort(ctx, "127.0.0.1", addr.Port, 5*time.Second, timeouts.PortPoll, timeouts.DialTimeout)
	assert.NoError(t, err)
}

func TestWaitForPort_Timeout(t *testing.T) {
	t.Parallel()
	// Use a port that's definitely not listening

	ctx := context.Background()
	timeouts := config.TestTimeouts()
	err := waitForPort(ctx, "127.0.0.1", 59999, 100*time.Millisecond, timeouts.PortPoll, timeouts.DialTimeout)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestWaitForPort_ContextCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	timeouts := config.TestTimeouts()
	err := waitForPort(ctx, "127.0.0.1", 59999, 5*time.Second, timeouts.PortPoll, timeouts.DialTimeout)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestIsAlreadyBootstrapped(t *testing.T) {
	t.Parallel()

	t.Run("marker exists", func(t *testing.T) {
		t.Parallel()
		mockInfra := &hcloud_internal.MockClient{
			GetCertificateFunc: func(_ context.Context, name string) (*hcloud.Certificate, error) {
				if name == "test-cluster-state" {
					return &hcloud.Certificate{ID: 123}, nil
				}
				return nil, nil
			},
		}

		observer := provisioning.NewConsoleObserver()
		ctx := &provisioning.Context{
			Context:  context.Background(),
			Config:   &config.Config{ClusterName: "test-cluster"},
			State:    provisioning.NewState(),
			Infra:    mockInfra,
			Observer: observer,
		}

		assert.True(t, isAlreadyBootstrapped(ctx))
	})

	t.Run("marker does not exist", func(t *testing.T) {
		t.Parallel()
		mockInfra := &hcloud_internal.MockClient{
			GetCertificateFunc: func(_ context.Context, _ string) (*hcloud.Certificate, error) {
				return nil, nil
			},
		}

		observer := provisioning.NewConsoleObserver()
		ctx := &provisioning.Context{
			Context:  context.Background(),
			Config:   &config.Config{ClusterName: "test-cluster"},
			State:    provisioning.NewState(),
			Infra:    mockInfra,
			Observer: observer,
		}

		assert.False(t, isAlreadyBootstrapped(ctx))
	})
}

func TestEnsureTalosConfigInState(t *testing.T) {
	t.Parallel()

	t.Run("config already in state", func(t *testing.T) {
		t.Parallel()
		ctx := &provisioning.Context{
			State: provisioning.NewState(),
		}
		ctx.State.TalosConfig = []byte("existing-config")

		err := ensureTalosConfigInState(ctx)
		assert.NoError(t, err)
		assert.Equal(t, []byte("existing-config"), ctx.State.TalosConfig)
	})
}

func TestBootstrap_StateMarkerPresent(t *testing.T) {
	t.Parallel()
	mockInfra := new(hcloud_internal.MockClient)

	ctx := context.Background()
	clusterName := "test-cluster"

	// Mock GetCertificate to return a cert (marker exists)
	mockInfra.GetCertificateFunc = func(_ context.Context, name string) (*hcloud.Certificate, error) {
		if name == "test-cluster-state" {
			return &hcloud.Certificate{ID: 123}, nil
		}
		return nil, nil
	}

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  ctx,
		Config:   &config.Config{ClusterName: clusterName},
		State:    provisioning.NewState(),
		Infra:    mockInfra,
		Observer: observer,
		Timeouts: config.TestTimeouts(),
	}
	pCtx.State.ControlPlaneIPs = map[string]string{
		"test-cluster-control-plane-1": "1.2.3.4",
	}
	pCtx.State.TalosConfig = []byte("client-config")

	err := BootstrapCluster(pCtx)
	assert.NoError(t, err)
}

func TestProvision(t *testing.T) {
	t.Parallel()
	// Test that Provision() delegates to BootstrapCluster()

	mockInfra := &hcloud_internal.MockClient{
		GetCertificateFunc: func(_ context.Context, name string) (*hcloud.Certificate, error) {
			if name == "test-cluster-state" {
				return &hcloud.Certificate{ID: 123}, nil // Already bootstrapped
			}
			return nil, nil
		},
	}

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster"},
		State:    provisioning.NewState(),
		Infra:    mockInfra,
		Observer: observer,
		Timeouts: config.TestTimeouts(),
	}
	pCtx.State.TalosConfig = []byte("talos-config")
	pCtx.State.ControlPlaneIPs = map[string]string{"node1": "1.2.3.4"}

	err := BootstrapCluster(pCtx)
	assert.NoError(t, err)
}

func TestApplyWorkerConfigs_NoWorkers(t *testing.T) {
	t.Parallel()

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster"},
		State:    provisioning.NewState(),
		Observer: observer,
	}
	// No workers in state
	pCtx.State.WorkerIPs = map[string]string{}

	err := ApplyWorkerConfigs(pCtx)
	assert.NoError(t, err)
}

func TestCreateStateMarker(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		var capturedLabels map[string]string
		var capturedName string

		mockInfra := &hcloud_internal.MockClient{
			EnsureCertificateFunc: func(_ context.Context, name, _, _ string, labels map[string]string) (*hcloud.Certificate, error) {
				capturedName = name
				capturedLabels = labels
				return &hcloud.Certificate{ID: 456}, nil
			},
		}

		observer := provisioning.NewConsoleObserver()
		pCtx := &provisioning.Context{
			Context:  context.Background(),
			Config:   &config.Config{ClusterName: "my-cluster"},
			State:    provisioning.NewState(),
			Infra:    mockInfra,
			Observer: observer,
		}

		err := createStateMarker(pCtx)
		assert.NoError(t, err)
		assert.Equal(t, "my-cluster-state", capturedName)
		assert.Equal(t, "my-cluster", capturedLabels["cluster"])
		assert.Equal(t, "initialized", capturedLabels["state"])
	})

	t.Run("failure", func(t *testing.T) {
		t.Parallel()
		mockInfra := &hcloud_internal.MockClient{
			EnsureCertificateFunc: func(_ context.Context, _, _, _ string, _ map[string]string) (*hcloud.Certificate, error) {
				return nil, context.DeadlineExceeded
			},
		}

		observer := provisioning.NewConsoleObserver()
		pCtx := &provisioning.Context{
			Context:  context.Background(),
			Config:   &config.Config{ClusterName: "my-cluster"},
			State:    provisioning.NewState(),
			Infra:    mockInfra,
			Observer: observer,
		}

		err := createStateMarker(pCtx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create state marker")
	})
}

func TestTryRetrieveExistingKubeconfig(t *testing.T) {
	t.Parallel()

	// This tests the error path - when kubeconfig cannot be retrieved
	// it should log and return nil (not an error)
	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster"},
		State:    provisioning.NewState(),
		Observer: observer,
		Timeouts: config.TestTimeouts(),
	}
	pCtx.State.ControlPlaneIPs = map[string]string{"node1": "127.0.0.1"}
	pCtx.State.TalosConfig = []byte("invalid-talos-config") // Will fail to parse

	err := tryRetrieveExistingKubeconfig(pCtx)
	// Should NOT return error even when kubeconfig retrieval fails
	assert.NoError(t, err)
	// Kubeconfig should remain empty
	assert.Empty(t, pCtx.State.Kubeconfig)
}

func TestRetrieveAndStoreKubeconfig_Error(t *testing.T) {
	t.Parallel()

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster"},
		State:    provisioning.NewState(),
		Observer: observer,
		Timeouts: config.TestTimeouts(),
	}
	pCtx.State.ControlPlaneIPs = map[string]string{"node1": "127.0.0.1"}
	pCtx.State.TalosConfig = []byte("invalid-config") // Will fail to parse

	err := retrieveAndStoreKubeconfig(pCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to retrieve kubeconfig")
}

// mockTalosConfigProducer is a mock implementation of TalosConfigProducer for testing.
type mockTalosConfigProducer struct {
	getClientConfigFunc          func() ([]byte, error)
	generateControlPlaneConfigFn func(san []string, hostname string, serverID int64) ([]byte, error)
	generateWorkerConfigFn       func(hostname string, serverID int64) ([]byte, error)
}

func (m *mockTalosConfigProducer) SetMachineConfigOptions(_ any) {}
func (m *mockTalosConfigProducer) SetEndpoint(_ string)          {}
func (m *mockTalosConfigProducer) GetNodeVersion(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (m *mockTalosConfigProducer) UpgradeNode(_ context.Context, _, _ string, _ provisioning.UpgradeOptions) error {
	return nil
}
func (m *mockTalosConfigProducer) UpgradeKubernetes(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockTalosConfigProducer) WaitForNodeReady(_ context.Context, _ string, _ time.Duration) error {
	return nil
}
func (m *mockTalosConfigProducer) HealthCheck(_ context.Context, _ string) error { return nil }
func (m *mockTalosConfigProducer) GetClientConfig() ([]byte, error) {
	if m.getClientConfigFunc != nil {
		return m.getClientConfigFunc()
	}
	return nil, fmt.Errorf("not implemented")
}
func (m *mockTalosConfigProducer) GenerateControlPlaneConfig(san []string, hostname string, serverID int64) ([]byte, error) {
	if m.generateControlPlaneConfigFn != nil {
		return m.generateControlPlaneConfigFn(san, hostname, serverID)
	}
	return []byte("mock-config"), nil
}
func (m *mockTalosConfigProducer) GenerateWorkerConfig(hostname string, serverID int64) ([]byte, error) {
	if m.generateWorkerConfigFn != nil {
		return m.generateWorkerConfigFn(hostname, serverID)
	}
	return []byte("mock-config"), nil
}

func TestEnsureTalosConfigInState_ErrorFromGetClientConfig(t *testing.T) {
	t.Parallel()

	mockTalos := &mockTalosConfigProducer{
		getClientConfigFunc: func() ([]byte, error) {
			return nil, fmt.Errorf("talos secrets not found")
		},
	}

	pCtx := &provisioning.Context{
		State: provisioning.NewState(),
		Talos: mockTalos,
	}

	err := ensureTalosConfigInState(pCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get client config")
	assert.Contains(t, err.Error(), "talos secrets not found")
}

func TestEnsureTalosConfigInState_SuccessFromTalos(t *testing.T) {
	t.Parallel()

	mockTalos := &mockTalosConfigProducer{
		getClientConfigFunc: func() ([]byte, error) {
			return []byte("fresh-config-from-talos"), nil
		},
	}

	pCtx := &provisioning.Context{
		State: provisioning.NewState(),
		Talos: mockTalos,
	}

	err := ensureTalosConfigInState(pCtx)
	assert.NoError(t, err)
	assert.Equal(t, []byte("fresh-config-from-talos"), pCtx.State.TalosConfig)
}

func TestBootstrapCluster_FailsWhenTalosConfigMissing(t *testing.T) {
	t.Parallel()

	mockTalos := &mockTalosConfigProducer{
		getClientConfigFunc: func() ([]byte, error) {
			return nil, fmt.Errorf("no secrets bundle")
		},
	}

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster"},
		State:    provisioning.NewState(),
		Talos:    mockTalos,
		Observer: observer,
		Timeouts: config.TestTimeouts(),
	}

	err := BootstrapCluster(pCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get client config")
}

func TestApplyControlPlaneConfigs_GenerateConfigError(t *testing.T) {
	t.Parallel()

	mockTalos := &mockTalosConfigProducer{
		generateControlPlaneConfigFn: func(_ []string, hostname string, _ int64) ([]byte, error) {
			return nil, fmt.Errorf("failed to generate config for %s", hostname)
		},
	}

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster"},
		State:    provisioning.NewState(),
		Talos:    mockTalos,
		Observer: observer,
		Timeouts: config.TestTimeouts(),
	}
	pCtx.State.ControlPlaneIPs = map[string]string{"cp-1": "10.0.0.1"}
	pCtx.State.ControlPlaneServerIDs = map[string]int64{"cp-1": 100}

	err := applyControlPlaneConfigs(pCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to generate machine config")
}

func TestApplyWorkerConfigs_GenerateConfigError(t *testing.T) {
	t.Parallel()

	mockTalos := &mockTalosConfigProducer{
		generateWorkerConfigFn: func(hostname string, _ int64) ([]byte, error) {
			return nil, fmt.Errorf("worker config generation failed for %s", hostname)
		},
	}

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster"},
		State:    provisioning.NewState(),
		Talos:    mockTalos,
		Observer: observer,
		Timeouts: config.TestTimeouts(),
	}
	pCtx.State.WorkerIPs = map[string]string{"worker-1": "10.0.0.10"}
	pCtx.State.WorkerServerIDs = map[string]int64{"worker-1": 200}

	err := ApplyWorkerConfigs(pCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to generate worker config")
}

func TestGetLBEndpoint(t *testing.T) {
	t.Parallel()

	t.Run("returns IP from state LB", func(t *testing.T) {
		t.Parallel()
		observer := provisioning.NewConsoleObserver()
		pCtx := &provisioning.Context{
			Context:  context.Background(),
			Config:   &config.Config{ClusterName: "test-cluster"},
			State:    provisioning.NewState(),
			Observer: observer,
		}
		pCtx.State.LoadBalancer = &hcloud.LoadBalancer{
			PublicNet: hcloud.LoadBalancerPublicNet{
				IPv4: hcloud.LoadBalancerPublicNetIPv4{
					IP: net.ParseIP("5.6.7.8"),
				},
			},
		}

		ip := getLBEndpoint(pCtx)
		assert.Equal(t, "5.6.7.8", ip)
	})

	t.Run("fetches LB from API when not in state", func(t *testing.T) {
		t.Parallel()
		mockInfra := &hcloud_internal.MockClient{
			GetLoadBalancerFunc: func(_ context.Context, name string) (*hcloud.LoadBalancer, error) {
				if name == "test-cluster-kube" {
					return &hcloud.LoadBalancer{
						PublicNet: hcloud.LoadBalancerPublicNet{
							IPv4: hcloud.LoadBalancerPublicNetIPv4{
								IP: net.ParseIP("9.10.11.12"),
							},
						},
					}, nil
				}
				return nil, nil
			},
		}

		observer := provisioning.NewConsoleObserver()
		pCtx := &provisioning.Context{
			Context:  context.Background(),
			Config:   &config.Config{ClusterName: "test-cluster"},
			State:    provisioning.NewState(),
			Infra:    mockInfra,
			Observer: observer,
		}

		ip := getLBEndpoint(pCtx)
		assert.Equal(t, "9.10.11.12", ip)
		// Verify LB was cached in state
		assert.NotNil(t, pCtx.State.LoadBalancer)
	})

	t.Run("returns empty when no LB available", func(t *testing.T) {
		t.Parallel()
		mockInfra := &hcloud_internal.MockClient{
			GetLoadBalancerFunc: func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
				return nil, nil
			},
		}

		observer := provisioning.NewConsoleObserver()
		pCtx := &provisioning.Context{
			Context:  context.Background(),
			Config:   &config.Config{ClusterName: "test-cluster"},
			State:    provisioning.NewState(),
			Infra:    mockInfra,
			Observer: observer,
		}

		ip := getLBEndpoint(pCtx)
		assert.Empty(t, ip)
	})

	t.Run("returns empty when API errors", func(t *testing.T) {
		t.Parallel()
		mockInfra := &hcloud_internal.MockClient{
			GetLoadBalancerFunc: func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
				return nil, fmt.Errorf("API error")
			},
		}

		observer := provisioning.NewConsoleObserver()
		pCtx := &provisioning.Context{
			Context:  context.Background(),
			Config:   &config.Config{ClusterName: "test-cluster"},
			State:    provisioning.NewState(),
			Infra:    mockInfra,
			Observer: observer,
		}

		ip := getLBEndpoint(pCtx)
		assert.Empty(t, ip)
	})
}

func TestIsAlreadyBootstrapped_Error(t *testing.T) {
	t.Parallel()

	mockInfra := &hcloud_internal.MockClient{
		GetCertificateFunc: func(_ context.Context, _ string) (*hcloud.Certificate, error) {
			return nil, fmt.Errorf("API error")
		},
	}

	observer := provisioning.NewConsoleObserver()
	ctx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster"},
		State:    provisioning.NewState(),
		Infra:    mockInfra,
		Observer: observer,
	}

	// API error should be treated as "not bootstrapped" (returns false, not error)
	assert.False(t, isAlreadyBootstrapped(ctx))
}

func TestBootstrapEtcd_InvalidTalosConfig(t *testing.T) {
	t.Parallel()

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster"},
		State:    provisioning.NewState(),
		Observer: observer,
		Timeouts: config.TestTimeouts(),
	}
	pCtx.State.ControlPlaneIPs = map[string]string{"cp-1": "10.0.0.1"}
	pCtx.State.TalosConfig = []byte("not-valid-yaml")

	err := bootstrapEtcd(pCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse talos config")
}

func TestWaitForControlPlaneReady_InvalidTalosConfig(t *testing.T) {
	t.Parallel()

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster", ClusterAccess: "private"},
		State:    provisioning.NewState(),
		Observer: observer,
		Timeouts: config.TestTimeouts(),
	}
	pCtx.State.ControlPlaneIPs = map[string]string{"cp-1": "10.0.0.1"}
	pCtx.State.TalosConfig = []byte("invalid-config")
	pCtx.State.LoadBalancer = &hcloud.LoadBalancer{
		PublicNet: hcloud.LoadBalancerPublicNet{
			IPv4: hcloud.LoadBalancerPublicNetIPv4{
				IP: net.ParseIP("5.5.5.5"),
			},
		},
	}

	err := waitForControlPlaneReadyViaLB(pCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse talos config")
}

func TestRetrieveAndStoreKubeconfig_PrivateFirstNoLB(t *testing.T) {
	t.Parallel()

	mockInfra := &hcloud_internal.MockClient{
		GetLoadBalancerFunc: func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
			return nil, nil
		},
	}

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster", ClusterAccess: "private"},
		State:    provisioning.NewState(),
		Infra:    mockInfra,
		Observer: observer,
		Timeouts: config.TestTimeouts(),
	}

	err := retrieveAndStoreKubeconfig(pCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private-first mode requires Load Balancer")
}

func TestBootstrapEtcd_PrivateFirstNoLB(t *testing.T) {
	t.Parallel()

	mockInfra := &hcloud_internal.MockClient{
		GetLoadBalancerFunc: func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
			return nil, nil
		},
	}

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster", ClusterAccess: "private"},
		State:    provisioning.NewState(),
		Infra:    mockInfra,
		Observer: observer,
		Timeouts: config.TestTimeouts(),
	}

	err := bootstrapEtcd(pCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private-first mode requires Load Balancer")
}

func TestApplyControlPlaneConfigsViaLB_NoLB(t *testing.T) {
	t.Parallel()

	mockInfra := &hcloud_internal.MockClient{
		GetLoadBalancerFunc: func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
			return nil, nil
		},
	}

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster"},
		State:    provisioning.NewState(),
		Infra:    mockInfra,
		Observer: observer,
		Timeouts: config.TestTimeouts(),
	}

	err := applyControlPlaneConfigsViaLB(pCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private-first mode requires Load Balancer")
}
