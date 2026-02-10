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
						Nameservers:         []string{"8.8.8.8", "8.8.4.4"},
						TimeServers:         []string{"time.google.com"},
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
				assert.Equal(t, []string{"8.8.8.8", "8.8.4.4"}, opts.Nameservers)
				assert.Equal(t, []string{"time.google.com"}, opts.TimeServers)
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

func TestBuildSysctlsPatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		ipv6Enabled   bool
		customSysctls map[string]string
		expectIPv6Val string
	}{
		{
			name:          "IPv6 enabled",
			ipv6Enabled:   true,
			expectIPv6Val: "0",
		},
		{
			name:          "IPv6 disabled",
			ipv6Enabled:   false,
			expectIPv6Val: "1",
		},
		{
			name:        "custom sysctls merged",
			ipv6Enabled: true,
			customSysctls: map[string]string{
				"net.core.somaxconn": "10000", // Override default
				"custom.sysctl":      "value",
			},
			expectIPv6Val: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := &MachineConfigOptions{
				IPv6Enabled: tt.ipv6Enabled,
				Sysctls:     tt.customSysctls,
			}
			result := buildSysctlsPatch(opts)

			// Check defaults present
			_, hasMaxconn := result["net.core.somaxconn"]
			assert.True(t, hasMaxconn)
			_, hasBacklog := result["net.core.netdev_max_backlog"]
			assert.True(t, hasBacklog)

			// Check IPv6 setting
			assert.Equal(t, tt.expectIPv6Val, result["net.ipv6.conf.all.disable_ipv6"])
			assert.Equal(t, tt.expectIPv6Val, result["net.ipv6.conf.default.disable_ipv6"])

			// Check custom sysctls
			if tt.customSysctls != nil {
				for k, v := range tt.customSysctls {
					assert.Equal(t, v, result[k])
				}
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
				Nameservers:       []string{"8.8.8.8"},
			},
			validateFunc: func(t *testing.T, result map[string]any) {
				assert.Equal(t, "test-node", result["hostname"])
				assert.Equal(t, []string{"8.8.8.8"}, result["nameservers"])

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
		{
			name:     "with extra host entries",
			hostname: "test-node",
			opts: &MachineConfigOptions{
				PublicIPv4Enabled: true,
				ExtraHostEntries: []config.TalosHostEntry{
					{IP: "192.168.1.100", Aliases: []string{"custom-host"}},
				},
			},
			validateFunc: func(t *testing.T, result map[string]any) {
				entries := result["extraHostEntries"].([]map[string]any)
				require.Len(t, entries, 1)
				assert.Equal(t, "192.168.1.100", entries[0]["ip"])
				assert.Equal(t, []string{"custom-host"}, entries[0]["aliases"])
			},
		},
		{
			name:     "with extra routes",
			hostname: "test-node",
			opts: &MachineConfigOptions{
				PublicIPv4Enabled: true,
				ExtraRoutes:       []string{"10.10.0.0/16"},
				NetworkGateway:    "10.0.0.1",
			},
			validateFunc: func(t *testing.T, result map[string]any) {
				interfaces := result["interfaces"].([]map[string]any)
				// Routes should be on the private interface (eth1)
				var privateIface map[string]any
				for _, iface := range interfaces {
					if iface["interface"] == "eth1" {
						privateIface = iface
						break
					}
				}
				require.NotNil(t, privateIface)
				routes := privateIface["routes"].([]map[string]any)
				require.Len(t, routes, 1)
				assert.Equal(t, "10.10.0.0/16", routes[0]["network"])
				assert.Equal(t, "10.0.0.1", routes[0]["gateway"])
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
		{
			name:           "with inline manifests",
			isControlPlane: true,
			opts: &MachineConfigOptions{
				ClusterDomain:           "cluster.local",
				CoreDNSEnabled:          true,
				DiscoveryServiceEnabled: true,
				InlineManifests: []config.TalosInlineManifest{
					{Name: "test-manifest", Contents: "apiVersion: v1\nkind: ConfigMap"},
				},
			},
			validateFunc: func(t *testing.T, result map[string]any) {
				manifests := result["inlineManifests"].([]map[string]any)
				require.Len(t, manifests, 1)
				assert.Equal(t, "test-manifest", manifests[0]["name"])
			},
		},
		{
			name:           "with remote manifests",
			isControlPlane: true,
			opts: &MachineConfigOptions{
				ClusterDomain:           "cluster.local",
				CoreDNSEnabled:          true,
				DiscoveryServiceEnabled: true,
				RemoteManifests:         []string{"https://example.com/manifest.yaml"},
			},
			validateFunc: func(t *testing.T, result map[string]any) {
				ecp := result["externalCloudProvider"].(map[string]any)
				manifests := ecp["manifests"].([]string)
				assert.Contains(t, manifests, "https://example.com/manifest.yaml")
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
		Nameservers:                []string{"8.8.8.8"},
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

func TestBuildMachinePatch_KernelArgs(t *testing.T) {
	t.Parallel()
	opts := &MachineConfigOptions{
		KernelArgs:        []string{"console=tty0", "quiet"},
		PublicIPv4Enabled: true,
	}
	result := buildMachinePatch("node-1", 0, opts, "installer:v1", nil, false)

	install := result["install"].(map[string]any)
	assert.Equal(t, []string{"console=tty0", "quiet"}, install["extraKernelArgs"])
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

func TestBuildMachinePatch_KernelModules(t *testing.T) {
	t.Parallel()
	t.Run("modules without parameters", func(t *testing.T) {
		t.Parallel()
		opts := &MachineConfigOptions{
			PublicIPv4Enabled: true,
			KernelModules: []config.TalosKernelModule{
				{Name: "br_netfilter"},
			},
		}
		result := buildMachinePatch("node-1", 0, opts, "installer:v1", nil, false)

		kernel := result["kernel"].(map[string]any)
		modules := kernel["modules"].([]map[string]any)
		require.Len(t, modules, 1)
		assert.Equal(t, "br_netfilter", modules[0]["name"])
		_, hasParams := modules[0]["parameters"]
		assert.False(t, hasParams, "parameters should not be set when empty")
	})

	t.Run("modules with parameters", func(t *testing.T) {
		t.Parallel()
		opts := &MachineConfigOptions{
			PublicIPv4Enabled: true,
			KernelModules: []config.TalosKernelModule{
				{Name: "bonding", Parameters: []string{"mode=802.3ad", "miimon=100"}},
			},
		}
		result := buildMachinePatch("node-1", 0, opts, "installer:v1", nil, false)

		kernel := result["kernel"].(map[string]any)
		modules := kernel["modules"].([]map[string]any)
		require.Len(t, modules, 1)
		assert.Equal(t, "bonding", modules[0]["name"])
		assert.Equal(t, []string{"mode=802.3ad", "miimon=100"}, modules[0]["parameters"])
	})
}

func TestBuildMachinePatch_Registries(t *testing.T) {
	t.Parallel()
	t.Run("with mirrors", func(t *testing.T) {
		t.Parallel()
		opts := &MachineConfigOptions{
			PublicIPv4Enabled: true,
			Registries: &config.TalosRegistryConfig{
				Mirrors: map[string]config.TalosRegistryMirror{
					"docker.io": {Endpoints: []string{"https://mirror.example.com"}},
				},
			},
		}
		result := buildMachinePatch("node-1", 0, opts, "installer:v1", nil, false)

		registries := result["registries"].(map[string]any)
		mirrors := registries["mirrors"].(map[string]any)
		dockerMirror := mirrors["docker.io"].(map[string]any)
		assert.Equal(t, []string{"https://mirror.example.com"}, dockerMirror["endpoints"])
	})

	t.Run("nil registries", func(t *testing.T) {
		t.Parallel()
		opts := &MachineConfigOptions{
			PublicIPv4Enabled: true,
			Registries:        nil,
		}
		result := buildMachinePatch("node-1", 0, opts, "installer:v1", nil, false)

		_, hasRegistries := result["registries"]
		assert.False(t, hasRegistries, "registries should not be set when nil")
	})

	t.Run("empty mirrors", func(t *testing.T) {
		t.Parallel()
		opts := &MachineConfigOptions{
			PublicIPv4Enabled: true,
			Registries: &config.TalosRegistryConfig{
				Mirrors: map[string]config.TalosRegistryMirror{},
			},
		}
		result := buildMachinePatch("node-1", 0, opts, "installer:v1", nil, false)

		_, hasRegistries := result["registries"]
		assert.False(t, hasRegistries, "registries should not be set when mirrors are empty")
	})
}

func TestBuildMachinePatch_LoggingDestinations(t *testing.T) {
	t.Parallel()
	t.Run("basic endpoint only", func(t *testing.T) {
		t.Parallel()
		opts := &MachineConfigOptions{
			PublicIPv4Enabled: true,
			LoggingDestinations: []config.TalosLoggingDestination{
				{Endpoint: "tcp://logs.example.com:514"},
			},
		}
		result := buildMachinePatch("node-1", 0, opts, "installer:v1", nil, false)

		logging := result["logging"].(map[string]any)
		dests := logging["destinations"].([]map[string]any)
		require.Len(t, dests, 1)
		assert.Equal(t, "tcp://logs.example.com:514", dests[0]["endpoint"])
		_, hasFormat := dests[0]["format"]
		assert.False(t, hasFormat, "format should not be set when empty")
		_, hasTags := dests[0]["extraTags"]
		assert.False(t, hasTags, "extraTags should not be set when empty")
	})

	t.Run("with format and extra tags", func(t *testing.T) {
		t.Parallel()
		opts := &MachineConfigOptions{
			PublicIPv4Enabled: true,
			LoggingDestinations: []config.TalosLoggingDestination{
				{
					Endpoint:  "tcp://logs.example.com:514",
					Format:    "json_lines",
					ExtraTags: map[string]string{"cluster": "prod", "env": "production"},
				},
			},
		}
		result := buildMachinePatch("node-1", 0, opts, "installer:v1", nil, false)

		logging := result["logging"].(map[string]any)
		dests := logging["destinations"].([]map[string]any)
		require.Len(t, dests, 1)
		assert.Equal(t, "json_lines", dests[0]["format"])
		tags := dests[0]["extraTags"].(map[string]string)
		assert.Equal(t, "prod", tags["cluster"])
		assert.Equal(t, "production", tags["env"])
	})
}

func TestBuildMachinePatch_TimeServers(t *testing.T) {
	t.Parallel()
	t.Run("with time servers", func(t *testing.T) {
		t.Parallel()
		opts := &MachineConfigOptions{
			PublicIPv4Enabled: true,
			TimeServers:       []string{"time.google.com", "ntp.ubuntu.com"},
		}
		result := buildMachinePatch("node-1", 0, opts, "installer:v1", nil, false)

		timeSection := result["time"].(map[string]any)
		assert.Equal(t, []string{"time.google.com", "ntp.ubuntu.com"}, timeSection["servers"])
	})

	t.Run("without time servers", func(t *testing.T) {
		t.Parallel()
		opts := &MachineConfigOptions{
			PublicIPv4Enabled: true,
		}
		result := buildMachinePatch("node-1", 0, opts, "installer:v1", nil, false)

		_, hasTime := result["time"]
		assert.False(t, hasTime, "time should not be set when no time servers configured")
	})
}

func TestBuildKubeletPatch_ExtraMounts(t *testing.T) {
	t.Parallel()
	t.Run("with full mount spec", func(t *testing.T) {
		t.Parallel()
		opts := &MachineConfigOptions{
			KubeletExtraMounts: []config.TalosKubeletMount{
				{
					Source:      "/var/local-path-provisioner",
					Destination: "/opt/local-path-provisioner",
					Type:        "tmpfs",
					Options:     []string{"noexec", "nosuid"},
				},
			},
		}
		result := buildKubeletPatch(opts, false, 12345)

		mounts := result["extraMounts"].([]map[string]any)
		require.Len(t, mounts, 1)
		assert.Equal(t, "/var/local-path-provisioner", mounts[0]["source"])
		assert.Equal(t, "/opt/local-path-provisioner", mounts[0]["destination"])
		assert.Equal(t, "tmpfs", mounts[0]["type"])
		assert.Equal(t, []string{"noexec", "nosuid"}, mounts[0]["options"])
	})

	t.Run("defaults destination to source", func(t *testing.T) {
		t.Parallel()
		opts := &MachineConfigOptions{
			KubeletExtraMounts: []config.TalosKubeletMount{
				{Source: "/var/data"},
			},
		}
		result := buildKubeletPatch(opts, false, 12345)

		mounts := result["extraMounts"].([]map[string]any)
		require.Len(t, mounts, 1)
		assert.Equal(t, "/var/data", mounts[0]["destination"])
	})

	t.Run("defaults type to bind", func(t *testing.T) {
		t.Parallel()
		opts := &MachineConfigOptions{
			KubeletExtraMounts: []config.TalosKubeletMount{
				{Source: "/var/data"},
			},
		}
		result := buildKubeletPatch(opts, false, 12345)

		mounts := result["extraMounts"].([]map[string]any)
		assert.Equal(t, "bind", mounts[0]["type"])
	})

	t.Run("defaults options to bind,rshared,rw", func(t *testing.T) {
		t.Parallel()
		opts := &MachineConfigOptions{
			KubeletExtraMounts: []config.TalosKubeletMount{
				{Source: "/var/data"},
			},
		}
		result := buildKubeletPatch(opts, false, 12345)

		mounts := result["extraMounts"].([]map[string]any)
		assert.Equal(t, []string{"bind", "rshared", "rw"}, mounts[0]["options"])
	})
}

func TestBuildKubeletPatch_ExtraArgsMerge(t *testing.T) {
	t.Parallel()
	opts := &MachineConfigOptions{
		KubeletExtraArgs: map[string]string{
			"max-pods":       "200",
			"cloud-provider": "custom", // Override base value
		},
	}
	result := buildKubeletPatch(opts, false, 12345)

	extraArgs := result["extraArgs"].(map[string]any)
	assert.Equal(t, "custom", extraArgs["cloud-provider"], "user args should override base")
	assert.Equal(t, "200", extraArgs["max-pods"], "user args should be merged in")
}

func TestBuildKubeletPatch_ExtraConfigMerge(t *testing.T) {
	t.Parallel()
	opts := &MachineConfigOptions{
		KubeletExtraConfig: map[string]any{
			"maxPods":                         200,
			"shutdownGracePeriod":             "120s", // Override base value
			"shutdownGracePeriodCriticalPods": "30s",  // Override base value
		},
	}
	result := buildKubeletPatch(opts, false, 12345)

	extraConfig := result["extraConfig"].(map[string]any)
	assert.Equal(t, 200, extraConfig["maxPods"], "user config should be merged in")
	assert.Equal(t, "120s", extraConfig["shutdownGracePeriod"], "user config should override base")
	assert.Equal(t, "30s", extraConfig["shutdownGracePeriodCriticalPods"], "user config should override base")
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

func TestBuildClusterPatch_APIServerExtraArgs(t *testing.T) {
	t.Parallel()
	opts := &MachineConfigOptions{
		ClusterDomain:           "cluster.local",
		CoreDNSEnabled:          true,
		DiscoveryServiceEnabled: true,
		APIServerExtraArgs: map[string]string{
			"audit-log-path":    "/var/log/kube-audit.log",
			"audit-log-maxsize": "100",
		},
	}
	result := buildClusterPatch(opts, true)

	apiServer := result["apiServer"].(map[string]any)
	extraArgs := apiServer["extraArgs"].(map[string]any)
	assert.Equal(t, true, extraArgs["enable-aggregator-routing"], "base args preserved")
	assert.Equal(t, "/var/log/kube-audit.log", extraArgs["audit-log-path"], "user args merged")
	assert.Equal(t, "100", extraArgs["audit-log-maxsize"], "user args merged")
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
