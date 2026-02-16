package compute

import (
	"context"
	"testing"

	"github.com/imamik/k8zner/internal/config"
	hcloud_internal "github.com/imamik/k8zner/internal/platform/hcloud"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureImage_ExistingSnapshot(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)
	cfg.Talos.Version = "v1.8.3"
	cfg.Kubernetes.Version = "v1.31.0"

	var capturedLabels map[string]string
	mockInfra.GetSnapshotByLabelsFunc = func(_ context.Context, labels map[string]string) (*hcloud.Image, error) {
		capturedLabels = labels
		return &hcloud.Image{ID: 456, Description: "talos-v1.8.3-k8s-v1.31.0-amd64"}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)

	imageID, err := ensureImage(ctx, "cx21", "nbg1")

	require.NoError(t, err)
	assert.Equal(t, "456", imageID)
	assert.Equal(t, "talos", capturedLabels["os"])
	assert.Equal(t, "v1.8.3", capturedLabels["talos-version"])
	assert.Equal(t, "v1.31.0", capturedLabels["k8s-version"])
	assert.Equal(t, "amd64", capturedLabels["arch"])
}

func TestEnsureImage_ARMArchitecture(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)
	cfg.Talos.Version = "v1.8.3"
	cfg.Kubernetes.Version = "v1.31.0"

	var capturedLabels map[string]string
	mockInfra.GetSnapshotByLabelsFunc = func(_ context.Context, labels map[string]string) (*hcloud.Image, error) {
		capturedLabels = labels
		return &hcloud.Image{ID: 789, Description: "talos-arm64"}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)

	imageID, err := ensureImage(ctx, "cax11", "nbg1") // cax = ARM

	require.NoError(t, err)
	assert.Equal(t, "789", imageID)
	assert.Equal(t, "arm64", capturedLabels["arch"])
}

func TestEnsureImage_SnapshotNotFound(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)
	cfg.Talos.Version = "v1.8.3"
	cfg.Kubernetes.Version = "v1.31.0"

	mockInfra.GetSnapshotByLabelsFunc = func(_ context.Context, _ map[string]string) (*hcloud.Image, error) {
		return nil, nil // No snapshot found
	}

	ctx := createTestContext(t, mockInfra, cfg)

	_, err := ensureImage(ctx, "cx21", "nbg1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "talos snapshot not found")
	assert.Contains(t, err.Error(), "should have been pre-built")
}

func TestEnsureImage_DefaultVersions(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)
	// Leave versions empty to test defaults
	cfg.Talos.Version = ""
	cfg.Kubernetes.Version = ""

	var capturedLabels map[string]string
	mockInfra.GetSnapshotByLabelsFunc = func(_ context.Context, labels map[string]string) (*hcloud.Image, error) {
		capturedLabels = labels
		return &hcloud.Image{ID: 100}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)

	_, err := ensureImage(ctx, "cx21", "nbg1")

	require.NoError(t, err)
	assert.Equal(t, "v1.8.3", capturedLabels["talos-version"])
	assert.Equal(t, "v1.31.0", capturedLabels["k8s-version"])
}

func TestEnsureImage_GetSnapshotError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)

	mockInfra.GetSnapshotByLabelsFunc = func(_ context.Context, _ map[string]string) (*hcloud.Image, error) {
		return nil, assert.AnError
	}

	ctx := createTestContext(t, mockInfra, cfg)

	_, err := ensureImage(ctx, "cx21", "nbg1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check for existing snapshot")
}

// testConfigWithSubnets creates a Config with subnets pre-calculated.
func testConfigWithSubnets(t *testing.T) *config.Config {
	t.Helper()
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		Talos: config.TalosConfig{
			Version: "v1.8.3",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "v1.31.0",
		},
	}
	require.NoError(t, cfg.CalculateSubnets())
	return cfg
}
