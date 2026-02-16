package talos

import (
	"context"
	"testing"
	"time"

	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests exercise the createClient → GetNodeVersion/GetSchematicID/etc. code paths
// with valid Talos configs but unreachable endpoints, to cover error handling logic.

func newTestGenerator(t *testing.T) *Generator {
	t.Helper()
	sb, err := NewSecrets("v1.7.0")
	require.NoError(t, err)
	return NewGenerator("test-cluster", "v1.30.0", "v1.7.0", "https://127.0.0.1:50001", sb)
}

func TestCreateClient_GeneratesValidConfig(t *testing.T) {
	t.Parallel()

	gen := newTestGenerator(t)

	// GetClientConfig should succeed since we have valid secrets
	cfgBytes, err := gen.GetClientConfig()
	require.NoError(t, err)
	assert.NotEmpty(t, cfgBytes)
}

func TestGetNodeVersion_ConnectionError(t *testing.T) {
	t.Parallel()

	gen := newTestGenerator(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := gen.GetNodeVersion(ctx, "127.0.0.1:50001")
	require.Error(t, err)
	// The error could be from connection refusal or context timeout
	// Either way it should propagate through createClient or Version call
}

func TestUpgradeNode_ConnectionError(t *testing.T) {
	t.Parallel()

	gen := newTestGenerator(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := gen.UpgradeNode(ctx, "127.0.0.1:50001", "factory.talos.dev/installer/test:v1.7.0", provisioning.UpgradeOptions{})
	require.Error(t, err)
}

func TestUpgradeNode_WithStageAndForce(t *testing.T) {
	t.Parallel()

	gen := newTestGenerator(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := gen.UpgradeNode(ctx, "127.0.0.1:50001", "factory.talos.dev/installer/test:v1.7.0", provisioning.UpgradeOptions{
		Stage: true,
		Force: true,
	})
	require.Error(t, err)
}

func TestHealthCheck_ConnectionError(t *testing.T) {
	t.Parallel()

	gen := newTestGenerator(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := gen.HealthCheck(ctx, "127.0.0.1:50001")
	require.Error(t, err)
}

func TestUpgradeKubernetes_NoOp(t *testing.T) {
	t.Parallel()

	gen := newTestGenerator(t)

	// UpgradeKubernetes is a no-op stub - should always return nil regardless of input
	err := gen.UpgradeKubernetes(context.Background(), "127.0.0.1:50001", "v1.32.0")
	require.NoError(t, err)

	// Also works with empty strings
	err = gen.UpgradeKubernetes(context.Background(), "", "")
	require.NoError(t, err)

	// Also works with nil context (since the function ignores all params)
	err = gen.UpgradeKubernetes(nil, "", "")
	require.NoError(t, err)
}

func TestCreateClient_InvalidClientConfig(t *testing.T) {
	t.Parallel()

	// Create generator with invalid talos version to force GetClientConfig to fail
	sb, err := NewSecrets("v1.7.0")
	require.NoError(t, err)

	gen := &Generator{
		clusterName:       "test",
		kubernetesVersion: "1.30.0",
		talosVersion:      "invalid-version",
		endpoint:          "https://127.0.0.1:50001",
		secretsBundle:     sb,
		machineOpts:       &MachineConfigOptions{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// All methods that call createClient should fail with the config generation error
	_, err = gen.GetNodeVersion(ctx, "127.0.0.1:50001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create Talos client")

	err = gen.UpgradeNode(ctx, "127.0.0.1:50001", "image:v1", provisioning.UpgradeOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create Talos client")

	err = gen.HealthCheck(ctx, "127.0.0.1:50001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create Talos client")
}

