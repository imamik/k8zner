package talos

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/imamik/k8zner/internal/config"
)

func TestNewMachineConfigOptions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		cfg      *config.Config
		validate func(t *testing.T, opts *MachineConfigOptions)
	}{
		{
			name: "defaults applied correctly",
			cfg: &config.Config{
				Talos: config.TalosConfig{
					Machine: config.TalosMachineConfig{},
				},
				Kubernetes: config.KubernetesConfig{},
				Addons: config.AddonsConfig{
					Cilium: config.CiliumConfig{},
				},
			},
			validate: func(t *testing.T, opts *MachineConfigOptions) {
				// Default encryption enabled
				assert.True(t, opts.StateEncryption)
				assert.True(t, opts.EphemeralEncryption)
				// Default IPv6 and public IP enabled
				assert.True(t, opts.IPv6Enabled)
				assert.True(t, opts.PublicIPv4Enabled)
				assert.True(t, opts.PublicIPv6Enabled)
				// Default CoreDNS enabled
				assert.True(t, opts.CoreDNSEnabled)
				// Default discovery settings
				assert.False(t, opts.DiscoveryKubernetesEnabled)
				assert.True(t, opts.DiscoveryServiceEnabled)
			},
		},
		{
			name: "custom values respected",
			cfg: &config.Config{
				Talos: config.TalosConfig{
					SchematicID: "test-schematic",
					Machine: config.TalosMachineConfig{
						StateEncryption:     boolPtr(false),
						EphemeralEncryption: boolPtr(false),
						IPv6Enabled:         boolPtr(false),
						CoreDNSEnabled:      boolPtr(false),
					},
				},
				Kubernetes: config.KubernetesConfig{
					Domain:              "custom.local",
					AllowSchedulingOnCP: boolPtr(true),
				},
				Addons: config.AddonsConfig{
					Cilium: config.CiliumConfig{
						KubeProxyReplacementEnabled: true,
					},
				},
			},
			validate: func(t *testing.T, opts *MachineConfigOptions) {
				assert.Equal(t, "test-schematic", opts.SchematicID)
				assert.False(t, opts.StateEncryption)
				assert.False(t, opts.EphemeralEncryption)
				assert.False(t, opts.IPv6Enabled)
				assert.False(t, opts.CoreDNSEnabled)
				assert.Equal(t, "custom.local", opts.ClusterDomain)
				assert.True(t, opts.AllowSchedulingOnCP)
				assert.True(t, opts.KubeProxyReplacement)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := NewMachineConfigOptions(tt.cfg)
			tt.validate(t, opts)
		})
	}
}

func TestBuildDiskEncryptionPatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name               string
		stateEncryption    bool
		ephemeralEncrypt   bool
		expectStateKey     bool
		expectEphemeralKey bool
	}{
		{
			name:               "both enabled",
			stateEncryption:    true,
			ephemeralEncrypt:   true,
			expectStateKey:     true,
			expectEphemeralKey: true,
		},
		{
			name:               "only state enabled",
			stateEncryption:    true,
			ephemeralEncrypt:   false,
			expectStateKey:     true,
			expectEphemeralKey: false,
		},
		{
			name:               "only ephemeral enabled",
			stateEncryption:    false,
			ephemeralEncrypt:   true,
			expectStateKey:     false,
			expectEphemeralKey: true,
		},
		{
			name:               "both disabled",
			stateEncryption:    false,
			ephemeralEncrypt:   false,
			expectStateKey:     false,
			expectEphemeralKey: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := &MachineConfigOptions{
				StateEncryption:     tt.stateEncryption,
				EphemeralEncryption: tt.ephemeralEncrypt,
			}
			result := buildDiskEncryptionPatch(opts)

			_, hasState := result["state"]
			_, hasEphemeral := result["ephemeral"]
			assert.Equal(t, tt.expectStateKey, hasState)
			assert.Equal(t, tt.expectEphemeralKey, hasEphemeral)

			// Verify structure when enabled
			if tt.expectStateKey {
				state := result["state"].(map[string]any)
				assert.Equal(t, "luks2", state["provider"])
				assert.Contains(t, state["options"], "no_read_workqueue")
			}
		})
	}
}

func TestBuildKubeletPatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		isControlPlane bool
		serverID       int64
		opts           *MachineConfigOptions
		validateFunc   func(t *testing.T, result map[string]any)
	}{
		{
			name:           "control plane with defaults and server ID",
			isControlPlane: true,
			serverID:       12345,
			opts:           &MachineConfigOptions{},
			validateFunc: func(t *testing.T, result map[string]any) {
				// Check extra args
				extraArgs := result["extraArgs"].(map[string]any)
				assert.Equal(t, "external", extraArgs["cloud-provider"])
				// Check provider-id is set with server ID
				assert.Equal(t, "hcloud://12345", extraArgs["provider-id"])
				// Note: rotate-server-certificates is NOT set because it requires a CSR approver

				// Check control plane reserved resources
				extraConfig := result["extraConfig"].(map[string]any)
				systemReserved := extraConfig["systemReserved"].(map[string]any)
				assert.Equal(t, "250m", systemReserved["cpu"])
				assert.Equal(t, "300Mi", systemReserved["memory"])
			},
		},
		{
			name:           "worker with defaults and server ID",
			isControlPlane: false,
			serverID:       67890,
			opts:           &MachineConfigOptions{},
			validateFunc: func(t *testing.T, result map[string]any) {
				// Check provider-id is set with server ID
				extraArgs := result["extraArgs"].(map[string]any)
				assert.Equal(t, "hcloud://67890", extraArgs["provider-id"])
				// Check worker reserved resources (less than control plane)
				extraConfig := result["extraConfig"].(map[string]any)
				systemReserved := extraConfig["systemReserved"].(map[string]any)
				assert.Equal(t, "100m", systemReserved["cpu"])
				assert.Equal(t, "300Mi", systemReserved["memory"])
			},
		},
		{
			name:           "without server ID (provider-id not set)",
			isControlPlane: false,
			serverID:       0,
			opts:           &MachineConfigOptions{},
			validateFunc: func(t *testing.T, result map[string]any) {
				// Check provider-id is NOT set when serverID is 0
				extraArgs := result["extraArgs"].(map[string]any)
				_, hasProviderID := extraArgs["provider-id"]
				assert.False(t, hasProviderID, "provider-id should not be set when serverID is 0")
			},
		},
		{
			name:           "with nodeIP CIDR",
			isControlPlane: false,
			serverID:       12345,
			opts: &MachineConfigOptions{
				NodeIPv4CIDR: "10.0.0.0/16",
			},
			validateFunc: func(t *testing.T, result map[string]any) {
				nodeIP := result["nodeIP"].(map[string]any)
				subnets := nodeIP["validSubnets"].([]string)
				assert.Contains(t, subnets, "10.0.0.0/16")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := buildKubeletPatch(tt.opts, tt.isControlPlane, tt.serverID)
			tt.validateFunc(t, result)
		})
	}
}

func TestBuildNetworkPatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		hostname     string
		opts         *MachineConfigOptions
		validateFunc func(t *testing.T, result map[string]any)
	}{
		{
			name:     "basic configuration",
			hostname: "test-node",
			opts: &MachineConfigOptions{
				PublicIPv4Enabled: true,
				PublicIPv6Enabled: false,
			},
			validateFunc: func(t *testing.T, result map[string]any) {
				assert.Equal(t, "test-node", result["hostname"])

				interfaces := result["interfaces"].([]map[string]any)
				// Should have eth0 (public) and eth1 (private)
				assert.Len(t, interfaces, 2)
			},
		},
		{
			name:     "private only",
			hostname: "private-node",
			opts: &MachineConfigOptions{
				PublicIPv4Enabled: false,
				PublicIPv6Enabled: false,
			},
			validateFunc: func(t *testing.T, result map[string]any) {
				interfaces := result["interfaces"].([]map[string]any)
				// Should have only one interface (eth0 for private)
				assert.Len(t, interfaces, 1)
				assert.Equal(t, "eth0", interfaces[0]["interface"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := buildNetworkPatch(tt.hostname, tt.opts, false)
			tt.validateFunc(t, result)
		})
	}
}

func TestBuildClusterPatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		isControlPlane bool
		opts           *MachineConfigOptions
		validateFunc   func(t *testing.T, result map[string]any)
	}{
		{
			name:           "control plane with defaults",
			isControlPlane: true,
			opts: &MachineConfigOptions{
				ClusterDomain:              "cluster.local",
				AllowSchedulingOnCP:        false,
				KubeProxyReplacement:       true,
				CoreDNSEnabled:             true,
				DiscoveryServiceEnabled:    true,
				DiscoveryKubernetesEnabled: false,
			},
			validateFunc: func(t *testing.T, result map[string]any) {
				network := result["network"].(map[string]any)
				assert.Equal(t, "cluster.local", network["dnsDomain"])
				cni := network["cni"].(map[string]any)
				assert.Equal(t, "none", cni["name"])

				proxy := result["proxy"].(map[string]any)
				assert.Equal(t, true, proxy["disabled"])

				coreDNS := result["coreDNS"].(map[string]any)
				assert.Equal(t, false, coreDNS["disabled"])

				// Control plane specific
				assert.Equal(t, false, result["allowSchedulingOnControlPlanes"])
				assert.NotNil(t, result["apiServer"])
				assert.NotNil(t, result["controllerManager"])
				assert.NotNil(t, result["scheduler"])
				assert.NotNil(t, result["adminKubeconfig"])

				ecp := result["externalCloudProvider"].(map[string]any)
				assert.Equal(t, true, ecp["enabled"])
			},
		},
		{
			name:           "worker has minimal cluster config",
			isControlPlane: false,
			opts: &MachineConfigOptions{
				ClusterDomain:           "cluster.local",
				CoreDNSEnabled:          true,
				DiscoveryServiceEnabled: true,
			},
			validateFunc: func(t *testing.T, result map[string]any) {
				// Worker should not have control plane specific settings
				assert.Nil(t, result["allowSchedulingOnControlPlanes"])
				assert.Nil(t, result["apiServer"])
				assert.Nil(t, result["controllerManager"])
				assert.Nil(t, result["scheduler"])
				assert.Nil(t, result["adminKubeconfig"])
				assert.Nil(t, result["externalCloudProvider"])
			},
		},
		{
			name:           "with pod and service subnets",
			isControlPlane: true,
			opts: &MachineConfigOptions{
				ClusterDomain:           "cluster.local",
				PodIPv4CIDR:             "10.244.0.0/16",
				ServiceIPv4CIDR:         "10.96.0.0/12",
				CoreDNSEnabled:          true,
				DiscoveryServiceEnabled: true,
			},
			validateFunc: func(t *testing.T, result map[string]any) {
				network := result["network"].(map[string]any)
				podSubnets := network["podSubnets"].([]string)
				assert.Contains(t, podSubnets, "10.244.0.0/16")
				serviceSubnets := network["serviceSubnets"].([]string)
				assert.Contains(t, serviceSubnets, "10.96.0.0/12")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := buildClusterPatch(tt.opts, tt.isControlPlane)
			tt.validateFunc(t, result)
		})
	}
}

func TestBuildDiscoveryPatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		kubeEnabled    bool
		serviceEnabled bool
		expectEnabled  bool
	}{
		{
			name:           "both enabled",
			kubeEnabled:    true,
			serviceEnabled: true,
			expectEnabled:  true,
		},
		{
			name:           "only kubernetes enabled",
			kubeEnabled:    true,
			serviceEnabled: false,
			expectEnabled:  true,
		},
		{
			name:           "only service enabled",
			kubeEnabled:    false,
			serviceEnabled: true,
			expectEnabled:  true,
		},
		{
			name:           "both disabled",
			kubeEnabled:    false,
			serviceEnabled: false,
			expectEnabled:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := &MachineConfigOptions{
				DiscoveryKubernetesEnabled: tt.kubeEnabled,
				DiscoveryServiceEnabled:    tt.serviceEnabled,
			}
			result := buildDiscoveryPatch(opts)

			assert.Equal(t, tt.expectEnabled, result["enabled"])

			registries := result["registries"].(map[string]any)
			kubeRegistry := registries["kubernetes"].(map[string]any)
			serviceRegistry := registries["service"].(map[string]any)

			assert.Equal(t, !tt.kubeEnabled, kubeRegistry["disabled"])
			assert.Equal(t, !tt.serviceEnabled, serviceRegistry["disabled"])
		})
	}
}

func TestBuildFeaturesPatch(t *testing.T) {
	t.Parallel()
	t.Run("control plane has Talos API access", func(t *testing.T) {
		t.Parallel()
		result := buildFeaturesPatch(true)

		hostDNS := result["hostDNS"].(map[string]any)
		assert.True(t, hostDNS["enabled"].(bool))

		talosAPI := result["kubernetesTalosAPIAccess"].(map[string]any)
		assert.True(t, talosAPI["enabled"].(bool))
		roles := talosAPI["allowedRoles"].([]string)
		assert.Contains(t, roles, "os:reader")
		assert.Contains(t, roles, "os:etcd:backup")
	})

	t.Run("worker does not have Talos API access", func(t *testing.T) {
		t.Parallel()
		result := buildFeaturesPatch(false)

		hostDNS := result["hostDNS"].(map[string]any)
		assert.True(t, hostDNS["enabled"].(bool))

		// Worker should not have Talos API access
		_, hasTalosAPI := result["kubernetesTalosAPIAccess"]
		assert.False(t, hasTalosAPI)
	})
}

func TestBuildControlPlanePatch(t *testing.T) {
	t.Parallel()
	opts := &MachineConfigOptions{
		StateEncryption:            true,
		EphemeralEncryption:        true,
		IPv6Enabled:                true,
		PublicIPv4Enabled:          true,
		PublicIPv6Enabled:          false,
		ClusterDomain:              "cluster.local",
		AllowSchedulingOnCP:        false,
		KubeProxyReplacement:       true,
		CoreDNSEnabled:             true,
		DiscoveryServiceEnabled:    true,
		DiscoveryKubernetesEnabled: false,
	}

	result := buildControlPlanePatch("cp-1", 12345, opts, "factory.talos.dev/installer/test:v1.7.0", []string{"api.example.com"})

	// Verify top-level structure
	machine, ok := result["machine"].(map[string]any)
	require.True(t, ok, "should have machine section")
	cluster, ok := result["cluster"].(map[string]any)
	require.True(t, ok, "should have cluster section")

	// Verify machine section
	install := machine["install"].(map[string]any)
	assert.Equal(t, "factory.talos.dev/installer/test:v1.7.0", install["image"])

	certSANs := machine["certSANs"].([]string)
	assert.Contains(t, certSANs, "api.example.com")

	network := machine["network"].(map[string]any)
	assert.Equal(t, "cp-1", network["hostname"])

	// Verify cluster section has control plane specific settings
	assert.NotNil(t, cluster["apiServer"])
	assert.NotNil(t, cluster["controllerManager"])
	assert.NotNil(t, cluster["externalCloudProvider"])
}

func TestBuildWorkerPatch(t *testing.T) {
	t.Parallel()
	opts := &MachineConfigOptions{
		StateEncryption:         true,
		EphemeralEncryption:     true,
		IPv6Enabled:             true,
		PublicIPv4Enabled:       true,
		ClusterDomain:           "cluster.local",
		CoreDNSEnabled:          true,
		DiscoveryServiceEnabled: true,
	}

	result := buildWorkerPatch("worker-1", 12345, opts, "ghcr.io/siderolabs/installer:v1.7.0", nil)

	// Verify top-level structure
	machine, ok := result["machine"].(map[string]any)
	require.True(t, ok, "should have machine section")
	cluster, ok := result["cluster"].(map[string]any)
	require.True(t, ok, "should have cluster section")

	// Verify machine section
	install := machine["install"].(map[string]any)
	assert.Equal(t, "ghcr.io/siderolabs/installer:v1.7.0", install["image"])

	network := machine["network"].(map[string]any)
	assert.Equal(t, "worker-1", network["hostname"])

	// Verify cluster section does NOT have control plane specific settings
	assert.Nil(t, cluster["apiServer"])
	assert.Nil(t, cluster["controllerManager"])
	assert.Nil(t, cluster["externalCloudProvider"])
}

func TestBuildMachinePatch_CertSANs(t *testing.T) {
	t.Parallel()
	opts := &MachineConfigOptions{
		PublicIPv4Enabled: true,
	}
	result := buildMachinePatch("node-1", 0, opts, "installer:v1", []string{"api.example.com", "10.0.0.1"}, true)

	certSANs := result["certSANs"].([]string)
	assert.Equal(t, []string{"api.example.com", "10.0.0.1"}, certSANs)
}

func TestBuildMachinePatch_CertSANsEmpty(t *testing.T) {
	t.Parallel()
	opts := &MachineConfigOptions{
		PublicIPv4Enabled: true,
	}
	result := buildMachinePatch("node-1", 0, opts, "installer:v1", nil, false)

	_, hasCertSANs := result["certSANs"]
	assert.False(t, hasCertSANs, "certSANs should not be set when empty")
}

func TestBuildMachinePatch_NodeLabelsWithServerID(t *testing.T) {
	t.Parallel()
	opts := &MachineConfigOptions{
		PublicIPv4Enabled: true,
	}
	result := buildMachinePatch("node-1", 54321, opts, "installer:v1", nil, false)

	nodeLabels := result["nodeLabels"].(map[string]any)
	assert.Equal(t, "54321", nodeLabels["nodeid"])
}

func TestBuildMachinePatch_NodeLabelsWithoutServerID(t *testing.T) {
	t.Parallel()
	opts := &MachineConfigOptions{
		PublicIPv4Enabled: true,
	}
	result := buildMachinePatch("node-1", 0, opts, "installer:v1", nil, false)

	_, hasNodeLabels := result["nodeLabels"]
	assert.False(t, hasNodeLabels, "nodeLabels should not be set when serverID is 0")
}

func TestBuildKubeletPatch_NodeIPEmpty(t *testing.T) {
	t.Parallel()
	opts := &MachineConfigOptions{
		NodeIPv4CIDR: "",
	}
	result := buildKubeletPatch(opts, false, 12345)

	_, hasNodeIP := result["nodeIP"]
	assert.False(t, hasNodeIP, "nodeIP should not be set when CIDR is empty")
}

func TestBuildClusterPatch_EtcdSubnet(t *testing.T) {
	t.Parallel()
	opts := &MachineConfigOptions{
		ClusterDomain:           "cluster.local",
		CoreDNSEnabled:          true,
		DiscoveryServiceEnabled: true,
		EtcdSubnet:              "10.0.0.0/16",
	}
	result := buildClusterPatch(opts, true)

	etcd := result["etcd"].(map[string]any)
	subnets := etcd["advertisedSubnets"].([]string)
	assert.Contains(t, subnets, "10.0.0.0/16")
	extraArgs := etcd["extraArgs"].(map[string]any)
	assert.Equal(t, "http://0.0.0.0:2381", extraArgs["listen-metrics-urls"])
}

func TestBuildClusterPatch_EtcdSubnetEmpty(t *testing.T) {
	t.Parallel()
	opts := &MachineConfigOptions{
		ClusterDomain:           "cluster.local",
		CoreDNSEnabled:          true,
		DiscoveryServiceEnabled: true,
		EtcdSubnet:              "",
	}
	result := buildClusterPatch(opts, true)

	_, hasEtcd := result["etcd"]
	assert.False(t, hasEtcd, "etcd should not be set when subnet is empty")
}

// Helper function
func boolPtr(b bool) *bool {
	return &b
}
