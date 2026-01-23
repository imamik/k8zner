package cluster

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"net"
	"testing"
	"time"

	"hcloud-k8s/internal/config"
	hcloud_internal "hcloud-k8s/internal/platform/hcloud"
	"hcloud-k8s/internal/provisioning"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProvisioner(t *testing.T) {
	p := NewProvisioner()
	require.NotNil(t, p)
}

func TestProvisioner_Name(t *testing.T) {
	p := NewProvisioner()
	assert.Equal(t, "cluster", p.Name())
}

func TestGenerateDummyCert(t *testing.T) {
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
	p := NewProvisioner()

	t.Run("with control plane nodes", func(t *testing.T) {
		ctx := &provisioning.Context{
			State: provisioning.NewState(),
		}
		ctx.State.ControlPlaneIPs = map[string]string{
			"node1": "10.0.0.1",
			"node2": "10.0.0.2",
		}

		ip := p.getFirstControlPlaneIP(ctx)
		// Should return one of the IPs (map order is not guaranteed)
		assert.Contains(t, []string{"10.0.0.1", "10.0.0.2"}, ip)
	})

	t.Run("with no control plane nodes", func(t *testing.T) {
		ctx := &provisioning.Context{
			State: provisioning.NewState(),
		}
		ctx.State.ControlPlaneIPs = map[string]string{}

		ip := p.getFirstControlPlaneIP(ctx)
		assert.Equal(t, "", ip)
	})
}

func TestWaitForPort_Success(t *testing.T) {
	// Start a test server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()

	// Get the port that was assigned
	addr := listener.Addr().(*net.TCPAddr)

	// Wait for port should succeed immediately
	ctx := context.Background()
	err = waitForPort(ctx, "127.0.0.1", addr.Port, 5*time.Second)
	assert.NoError(t, err)
}

func TestWaitForPort_Timeout(t *testing.T) {
	// Use a port that's definitely not listening
	ctx := context.Background()
	err := waitForPort(ctx, "127.0.0.1", 59999, 100*time.Millisecond)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestWaitForPort_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := waitForPort(ctx, "127.0.0.1", 59999, 5*time.Second)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestIsAlreadyBootstrapped(t *testing.T) {
	p := NewProvisioner()

	t.Run("marker exists", func(t *testing.T) {
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
			Logger:   observer,
		}

		assert.True(t, p.isAlreadyBootstrapped(ctx))
	})

	t.Run("marker does not exist", func(t *testing.T) {
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
			Logger:   observer,
		}

		assert.False(t, p.isAlreadyBootstrapped(ctx))
	})
}

func TestEnsureTalosConfigInState(t *testing.T) {
	p := NewProvisioner()

	t.Run("config already in state", func(t *testing.T) {
		ctx := &provisioning.Context{
			State: provisioning.NewState(),
		}
		ctx.State.TalosConfig = []byte("existing-config")

		err := p.ensureTalosConfigInState(ctx)
		assert.NoError(t, err)
		assert.Equal(t, []byte("existing-config"), ctx.State.TalosConfig)
	})
}

func TestBootstrap_StateMarkerPresent(t *testing.T) {
	mockInfra := new(hcloud_internal.MockClient)
	p := NewProvisioner()

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
		Logger:   observer,
	}
	pCtx.State.ControlPlaneIPs = map[string]string{
		"test-cluster-control-plane-1": "1.2.3.4",
	}
	pCtx.State.TalosConfig = []byte("client-config")

	err := p.BootstrapCluster(pCtx)
	assert.NoError(t, err)
}
