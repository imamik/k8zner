package handlers

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/config"
	v2 "github.com/imamik/k8zner/internal/config/v2"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
)

func TestLoadConfig(t *testing.T) {
	t.Parallel()
	t.Run("no config file found", func(t *testing.T) {
		t.Parallel()
		orig := findV2ConfigFile
		defer func() { findV2ConfigFile = orig }()

		findV2ConfigFile = func() (string, error) {
			return "", errors.New("not found")
		}

		_, err := loadConfig("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no config file found")
	})

	t.Run("loads v2 config", func(t *testing.T) {
		t.Parallel()
		origFind := findV2ConfigFile
		origLoad := loadV2ConfigFile
		origExpand := expandV2Config
		defer func() {
			findV2ConfigFile = origFind
			loadV2ConfigFile = origLoad
			expandV2Config = origExpand
		}()

		findV2ConfigFile = func() (string, error) { return "k8zner.yaml", nil }
		loadV2ConfigFile = func(_ string) (*v2.Config, error) {
			return &v2.Config{Name: "test", Region: v2.RegionFalkenstein, Mode: v2.ModeDev, Workers: v2.Worker{Count: 1, Size: v2.SizeCX22}}, nil
		}
		expandV2Config = func(cfg *v2.Config) (*config.Config, error) {
			return &config.Config{ClusterName: cfg.Name}, nil
		}

		cfg, err := loadConfig("")
		require.NoError(t, err)
		assert.Equal(t, "test", cfg.ClusterName)
	})
}

type mockTalosProducer struct {
	clientConfig    []byte
	clientConfigErr error
}

func (m *mockTalosProducer) SetMachineConfigOptions(_ any) {}
func (m *mockTalosProducer) GenerateControlPlaneConfig(_ []string, _ string, _ int64) ([]byte, error) {
	return nil, nil
}
func (m *mockTalosProducer) GenerateWorkerConfig(_ string, _ int64) ([]byte, error) { return nil, nil }
func (m *mockTalosProducer) GenerateAutoscalerConfig(_ string, _ map[string]string, _ []string) ([]byte, error) {
	return nil, nil
}
func (m *mockTalosProducer) GetClientConfig() ([]byte, error) {
	return m.clientConfig, m.clientConfigErr
}
func (m *mockTalosProducer) SetEndpoint(_ string) {}
func (m *mockTalosProducer) GetNodeVersion(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (m *mockTalosProducer) UpgradeNode(_ context.Context, _, _ string, _ provisioning.UpgradeOptions) error {
	return nil
}
func (m *mockTalosProducer) UpgradeKubernetes(_ context.Context, _, _ string) error { return nil }
func (m *mockTalosProducer) WaitForNodeReady(_ context.Context, _ string, _ time.Duration) error {
	return nil
}
func (m *mockTalosProducer) HealthCheck(_ context.Context, _ string) error { return nil }

// --- updateClusterSpecFromConfig tests ---

func TestUpdateClusterSpecFromConfig_FullUpdate(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Addons: &k8znerv1alpha1.AddonSpec{},
		},
	}

	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Count: 3, ServerType: "cx32"},
			},
		},
		Workers: []config.WorkerNodePool{
			{Count: 2, ServerType: "cx22"},
		},
		Kubernetes: config.KubernetesConfig{Version: "1.31.0"},
		Talos: config.TalosConfig{
			Version:     "1.8.3",
			SchematicID: "abc123",
			Extensions:  []string{"siderolabs/iscsi-tools"},
		},
		Network: config.NetworkConfig{
			IPv4CIDR:        "10.0.0.0/16",
			PodIPv4CIDR:     "10.244.0.0/16",
			ServiceIPv4CIDR: "10.96.0.0/16",
		},
		Addons: config.AddonsConfig{
			MetricsServer:       config.MetricsServerConfig{Enabled: true},
			CertManager:         config.CertManagerConfig{Enabled: true},
			Traefik:             config.TraefikConfig{Enabled: true},
			ArgoCD:              config.ArgoCDConfig{Enabled: true},
			KubePrometheusStack: config.KubePrometheusStackConfig{Enabled: true},
		},
	}

	updateClusterSpecFromConfig(cluster, cfg)

	assert.Equal(t, 3, cluster.Spec.ControlPlanes.Count)
	assert.Equal(t, "cx32", cluster.Spec.ControlPlanes.Size)
	assert.Equal(t, 2, cluster.Spec.Workers.Count)
	assert.Equal(t, "cx22", cluster.Spec.Workers.Size)
	assert.Equal(t, "1.31.0", cluster.Spec.Kubernetes.Version)
	assert.Equal(t, "1.8.3", cluster.Spec.Talos.Version)
	assert.Equal(t, "abc123", cluster.Spec.Talos.SchematicID)
	assert.Equal(t, []string{"siderolabs/iscsi-tools"}, cluster.Spec.Talos.Extensions)
	assert.Equal(t, "10.0.0.0/16", cluster.Spec.Network.IPv4CIDR)
	assert.Equal(t, "10.244.0.0/16", cluster.Spec.Network.PodCIDR)
	assert.Equal(t, "10.96.0.0/16", cluster.Spec.Network.ServiceCIDR)
	assert.True(t, cluster.Spec.Addons.MetricsServer)
	assert.True(t, cluster.Spec.Addons.CertManager)
	assert.True(t, cluster.Spec.Addons.Traefik)
	assert.True(t, cluster.Spec.Addons.ArgoCD)
	assert.True(t, cluster.Spec.Addons.Monitoring)
	assert.NotEmpty(t, cluster.Annotations["k8zner.io/last-applied"])
}

func TestUpdateClusterSpecFromConfig_EmptyPools(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 3, Size: "cx32"},
			Workers:       k8znerv1alpha1.WorkerSpec{Count: 2, Size: "cx22"},
		},
	}

	cfg := &config.Config{
		// Empty node pools - should not change existing values
		ControlPlane: config.ControlPlaneConfig{NodePools: nil},
		Workers:      nil,
	}

	updateClusterSpecFromConfig(cluster, cfg)

	// Original values should be unchanged since no pools provided
	assert.Equal(t, 3, cluster.Spec.ControlPlanes.Count)
	assert.Equal(t, "cx32", cluster.Spec.ControlPlanes.Size)
	assert.Equal(t, 2, cluster.Spec.Workers.Count)
	assert.Equal(t, "cx22", cluster.Spec.Workers.Size)
}

func TestUpdateClusterSpecFromConfig_NilAddons(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Addons: nil, // nil addons should be initialized
		},
	}

	cfg := &config.Config{
		Addons: config.AddonsConfig{
			Traefik: config.TraefikConfig{Enabled: true},
		},
	}

	updateClusterSpecFromConfig(cluster, cfg)

	require.NotNil(t, cluster.Spec.Addons)
	assert.True(t, cluster.Spec.Addons.Traefik)
}

func TestUpdateClusterSpecFromConfig_BackupEnabled(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cluster"},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Addons: &k8znerv1alpha1.AddonSpec{},
		},
	}

	cfg := &config.Config{
		Addons: config.AddonsConfig{
			TalosBackup: config.TalosBackupConfig{
				Enabled:     true,
				Schedule:    "0 */6 * * *",
				S3AccessKey: "AKIA...",
			},
		},
	}

	updateClusterSpecFromConfig(cluster, cfg)

	require.NotNil(t, cluster.Spec.Backup)
	assert.True(t, cluster.Spec.Backup.Enabled)
	assert.Equal(t, "0 */6 * * *", cluster.Spec.Backup.Schedule)
	assert.Equal(t, "my-cluster-backup-s3", cluster.Spec.Backup.S3SecretRef.Name)
}

func TestUpdateClusterSpecFromConfig_BackupDisabled(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Addons: &k8znerv1alpha1.AddonSpec{},
		},
	}

	cfg := &config.Config{
		Addons: config.AddonsConfig{
			TalosBackup: config.TalosBackupConfig{
				Enabled: false,
			},
		},
	}

	updateClusterSpecFromConfig(cluster, cfg)
	assert.Nil(t, cluster.Spec.Backup)
}

// Verify factory variables exist and can be saved/restored.
func TestFactoryVariables(t *testing.T) {
	t.Parallel()
	origInfra := newInfraClient
	origSecrets := getOrGenerateSecrets
	origTalos := newTalosGenerator
	origWrite := writeFile
	defer func() {
		newInfraClient = origInfra
		getOrGenerateSecrets = origSecrets
		newTalosGenerator = origTalos
		writeFile = origWrite
	}()

	newInfraClient = func(_ string) hcloud.InfrastructureManager { return &hcloud.MockClient{} }
	getOrGenerateSecrets = func(_, _ string) (*secrets.Bundle, error) { return &secrets.Bundle{}, nil }
	newTalosGenerator = func(_, _, _, _ string, _ *secrets.Bundle) provisioning.TalosConfigProducer {
		return &mockTalosProducer{clientConfig: []byte("talos-config")}
	}
	writeFile = func(_ string, _ []byte, _ os.FileMode) error { return nil }

	// Verify mocks are set (basic sanity check)
	client := newInfraClient("test")
	assert.NotNil(t, client)
}
