package wizard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/imamik/k8zner/internal/config"
)

func TestWriteConfig_MinimalOutput(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "cluster.yaml")

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "control-plane", ServerType: "cpx21", Count: 1},
			},
		},
		Talos: config.TalosConfig{
			Version: "v1.9.0",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "v1.32.0",
		},
	}

	err := WriteConfig(cfg, outputPath, false)
	require.NoError(t, err)

	// Read the file
	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	// Check header
	assert.Contains(t, string(content), "# k8zner cluster configuration")
	assert.Contains(t, string(content), "Output mode: minimal")

	// Check cluster name
	assert.Contains(t, string(content), "cluster_name: test-cluster")
}

func TestWriteConfig_FullOutput(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "cluster.yaml")

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "control-plane", ServerType: "cpx21", Count: 1},
			},
		},
		Talos: config.TalosConfig{
			Version: "v1.9.0",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "v1.32.0",
		},
	}

	err := WriteConfig(cfg, outputPath, true)
	require.NoError(t, err)

	// Read the file
	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	// Check header
	assert.Contains(t, string(content), "Output mode: full")
	assert.NotContains(t, string(content), "Note: This is a minimal config")
}

func TestWriteConfig_WithWorkers(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "cluster.yaml")

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "control-plane", ServerType: "cpx21", Count: 1},
			},
		},
		Workers: []config.WorkerNodePool{
			{Name: "worker", ServerType: "cpx31", Count: 3},
		},
		Talos: config.TalosConfig{
			Version: "v1.9.0",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "v1.32.0",
		},
	}

	err := WriteConfig(cfg, outputPath, false)
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	assert.Contains(t, string(content), "workers:")
}

func TestWriteConfig_WithAddons(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "cluster.yaml")

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "control-plane", ServerType: "cpx21", Count: 1},
			},
		},
		Talos: config.TalosConfig{
			Version: "v1.9.0",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "v1.32.0",
		},
		Addons: config.AddonsConfig{
			Cilium: config.CiliumConfig{
				Enabled:           true,
				EncryptionEnabled: true,
				EncryptionType:    "wireguard",
			},
			CCM: config.CCMConfig{
				Enabled: true,
			},
		},
	}

	err := WriteConfig(cfg, outputPath, false)
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	assert.Contains(t, string(content), "addons:")
	assert.Contains(t, string(content), "cilium:")
	assert.Contains(t, string(content), "enabled: true")
}

func TestWriteConfig_FilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "cluster.yaml")

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "control-plane", ServerType: "cpx21", Count: 1},
			},
		},
		Talos: config.TalosConfig{
			Version: "v1.9.0",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "v1.32.0",
		},
	}

	err := WriteConfig(cfg, outputPath, false)
	require.NoError(t, err)

	info, err := os.Stat(outputPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestWriteConfig_InvalidPath(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
	}

	err := WriteConfig(cfg, "/nonexistent/dir/cluster.yaml", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write file")
}

func TestBuildMinimalConfig(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		SSHKeys:     []string{"key1", "key2"},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "control-plane", ServerType: "cpx21", Count: 1},
			},
		},
		Workers: []config.WorkerNodePool{
			{Name: "worker", ServerType: "cpx31", Count: 3},
		},
		Talos: config.TalosConfig{
			Version: "v1.9.0",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "v1.32.0",
		},
	}

	minCfg := buildMinimalConfig(cfg)

	assert.Equal(t, "test-cluster", minCfg.ClusterName)
	assert.Equal(t, "nbg1", minCfg.Location)
	assert.Equal(t, []string{"key1", "key2"}, minCfg.SSHKeys)
	assert.Len(t, minCfg.ControlPlane.NodePools, 1)
	assert.Len(t, minCfg.Workers, 1)
	assert.Equal(t, "v1.9.0", minCfg.Talos.Version)
	assert.Equal(t, "v1.32.0", minCfg.Kubernetes.Version)
}

func TestBuildMinimalConfig_WithNetwork(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "control-plane", ServerType: "cpx21", Count: 1},
			},
		},
		Talos: config.TalosConfig{
			Version: "v1.9.0",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "v1.32.0",
		},
		Network: config.NetworkConfig{
			IPv4CIDR:        "10.0.0.0/8",
			PodIPv4CIDR:     "10.244.0.0/16",
			ServiceIPv4CIDR: "10.96.0.0/12",
		},
	}

	minCfg := buildMinimalConfig(cfg)

	require.NotNil(t, minCfg.Network)
	assert.Equal(t, "10.0.0.0/8", minCfg.Network.IPv4CIDR)
	assert.Equal(t, "10.244.0.0/16", minCfg.Network.PodIPv4CIDR)
	assert.Equal(t, "10.96.0.0/12", minCfg.Network.ServiceIPv4CIDR)
}

func TestBuildMinimalConfig_WithEncryption(t *testing.T) {
	stateEncryption := true
	ephemeralEncryption := true

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "control-plane", ServerType: "cpx21", Count: 1},
			},
		},
		Talos: config.TalosConfig{
			Version: "v1.9.0",
			Machine: config.TalosMachineConfig{
				StateEncryption:     &stateEncryption,
				EphemeralEncryption: &ephemeralEncryption,
			},
		},
		Kubernetes: config.KubernetesConfig{
			Version: "v1.32.0",
		},
	}

	minCfg := buildMinimalConfig(cfg)

	require.NotNil(t, minCfg.Talos.MachineEncrypt)
	assert.True(t, *minCfg.Talos.MachineEncrypt.StateEncryption)
	assert.True(t, *minCfg.Talos.MachineEncrypt.EphemeralEncryption)
}

func TestBuildMinimalConfig_PrivateClusterAccess(t *testing.T) {
	cfg := &config.Config{
		ClusterName:   "test-cluster",
		Location:      "nbg1",
		ClusterAccess: "private",
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "control-plane", ServerType: "cpx21", Count: 1},
			},
		},
		Talos: config.TalosConfig{
			Version: "v1.9.0",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "v1.32.0",
		},
	}

	minCfg := buildMinimalConfig(cfg)
	assert.Equal(t, "private", minCfg.ClusterAccess)
}

func TestBuildMinimalConfig_PublicClusterAccessOmitted(t *testing.T) {
	cfg := &config.Config{
		ClusterName:   "test-cluster",
		Location:      "nbg1",
		ClusterAccess: "public", // Default, should be omitted
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "control-plane", ServerType: "cpx21", Count: 1},
			},
		},
		Talos: config.TalosConfig{
			Version: "v1.9.0",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "v1.32.0",
		},
	}

	minCfg := buildMinimalConfig(cfg)
	assert.Empty(t, minCfg.ClusterAccess)
}

func TestGenerateHeader(t *testing.T) {
	header := generateHeader("cluster.yaml", false)

	assert.Contains(t, header, "# k8zner cluster configuration")
	assert.Contains(t, header, "Generated by: k8zner init")
	assert.Contains(t, header, "Output mode: minimal")
	assert.Contains(t, header, "cluster.yaml")
	assert.Contains(t, header, "HCLOUD_TOKEN")
}

func TestGenerateHeader_FullMode(t *testing.T) {
	header := generateHeader("cluster.yaml", true)

	assert.Contains(t, header, "Output mode: full")
	assert.NotContains(t, header, "Note: This is a minimal config")
}

func TestGenerateHeader_ContainsTimestamp(t *testing.T) {
	header := generateHeader("cluster.yaml", false)

	// Should contain a timestamp in RFC3339 format
	assert.True(t, strings.Contains(header, "Generated at:"))
}

func TestBuildMinimalConfig_AllAddons(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "control-plane", ServerType: "cpx21", Count: 1},
			},
		},
		Talos: config.TalosConfig{
			Version: "v1.9.0",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "v1.32.0",
		},
		Addons: config.AddonsConfig{
			Cilium: config.CiliumConfig{
				Enabled:            true,
				EncryptionEnabled:  true,
				EncryptionType:     "wireguard",
				HubbleEnabled:      true,
				HubbleRelayEnabled: true,
				GatewayAPIEnabled:  true,
			},
			CCM:           config.CCMConfig{Enabled: true},
			CSI:           config.CSIConfig{Enabled: true},
			MetricsServer: config.MetricsServerConfig{Enabled: true},
			CertManager:   config.CertManagerConfig{Enabled: true},
			IngressNginx:  config.IngressNginxConfig{Enabled: true},
			Longhorn:      config.LonghornConfig{Enabled: true},
			ArgoCD:        config.ArgoCDConfig{Enabled: true},
		},
	}

	minCfg := buildMinimalConfig(cfg)

	// Check all addons are included
	require.NotNil(t, minCfg.Addons.Cilium)
	assert.True(t, minCfg.Addons.Cilium.Enabled)
	assert.True(t, minCfg.Addons.Cilium.EncryptionEnabled)
	assert.Equal(t, "wireguard", minCfg.Addons.Cilium.EncryptionType)
	assert.True(t, minCfg.Addons.Cilium.HubbleEnabled)
	assert.True(t, minCfg.Addons.Cilium.HubbleRelayEnabled)
	assert.True(t, minCfg.Addons.Cilium.GatewayAPIEnabled)

	require.NotNil(t, minCfg.Addons.CCM)
	assert.True(t, minCfg.Addons.CCM.Enabled)

	require.NotNil(t, minCfg.Addons.CSI)
	assert.True(t, minCfg.Addons.CSI.Enabled)

	require.NotNil(t, minCfg.Addons.MetricsServer)
	assert.True(t, minCfg.Addons.MetricsServer.Enabled)

	require.NotNil(t, minCfg.Addons.CertManager)
	assert.True(t, minCfg.Addons.CertManager.Enabled)

	require.NotNil(t, minCfg.Addons.IngressNginx)
	assert.True(t, minCfg.Addons.IngressNginx.Enabled)

	require.NotNil(t, minCfg.Addons.Longhorn)
	assert.True(t, minCfg.Addons.Longhorn.Enabled)

	require.NotNil(t, minCfg.Addons.ArgoCD)
	assert.True(t, minCfg.Addons.ArgoCD.Enabled)
}

func TestBuildMinimalConfig_NoAddons(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "control-plane", ServerType: "cpx21", Count: 1},
			},
		},
		Talos: config.TalosConfig{
			Version: "v1.9.0",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "v1.32.0",
		},
	}

	minCfg := buildMinimalConfig(cfg)

	// No addons should be included
	assert.Nil(t, minCfg.Addons.Cilium)
	assert.Nil(t, minCfg.Addons.CCM)
	assert.Nil(t, minCfg.Addons.CSI)
	assert.Nil(t, minCfg.Addons.MetricsServer)
	assert.Nil(t, minCfg.Addons.CertManager)
	assert.Nil(t, minCfg.Addons.IngressNginx)
	assert.Nil(t, minCfg.Addons.Longhorn)
	assert.Nil(t, minCfg.Addons.ArgoCD)
}
