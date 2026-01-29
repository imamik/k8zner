package addons

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/imamik/k8zner/internal/addons/helm"
	"github.com/imamik/k8zner/internal/config"
)

func TestBuildArgoCDValues(t *testing.T) {
	tests := []struct {
		name           string
		argoCDCfg      config.ArgoCDConfig
		expectHA       bool
		expectIngress  bool
		serverReplicas int
	}{
		{
			name: "default configuration",
			argoCDCfg: config.ArgoCDConfig{
				Enabled: true,
			},
			expectHA:       false,
			expectIngress:  false,
			serverReplicas: 1,
		},
		{
			name: "HA mode enabled",
			argoCDCfg: config.ArgoCDConfig{
				Enabled: true,
				HA:      true,
			},
			expectHA:       true,
			expectIngress:  false,
			serverReplicas: 2,
		},
		{
			name: "with ingress enabled",
			argoCDCfg: config.ArgoCDConfig{
				Enabled:        true,
				IngressEnabled: true,
				IngressHost:    "argocd.example.com",
			},
			expectHA:       false,
			expectIngress:  true,
			serverReplicas: 1,
		},
		{
			name: "HA with custom replicas",
			argoCDCfg: config.ArgoCDConfig{
				Enabled:        true,
				HA:             true,
				ServerReplicas: intPtrArgoCD(3),
			},
			expectHA:       true,
			expectIngress:  false,
			serverReplicas: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Addons: config.AddonsConfig{
					ArgoCD: tt.argoCDCfg,
				},
			}

			values := buildArgoCDValues(cfg)

			// Check CRDs are enabled
			crds, ok := values["crds"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, true, crds["install"])

			// Check server configuration
			server, ok := values["server"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, tt.serverReplicas, server["replicas"])

			// Check HA mode
			if tt.expectHA {
				redisHA, ok := values["redis-ha"].(helm.Values)
				require.True(t, ok)
				assert.Equal(t, true, redisHA["enabled"])

				redis, ok := values["redis"].(helm.Values)
				require.True(t, ok)
				assert.Equal(t, false, redis["enabled"])
			} else {
				// Verify redis-ha is explicitly disabled in non-HA mode
				redisHA, ok := values["redis-ha"].(helm.Values)
				require.True(t, ok, "redis-ha should be set in non-HA mode")
				assert.Equal(t, false, redisHA["enabled"], "redis-ha should be disabled in non-HA mode")
			}

			// Check ingress
			if tt.expectIngress {
				_, hasIngress := server["ingress"]
				assert.True(t, hasIngress)
			}
		})
	}
}

func TestBuildArgoCDController(t *testing.T) {
	tests := []struct {
		name             string
		cfg              config.ArgoCDConfig
		expectedReplicas int
	}{
		{
			name:             "default replicas",
			cfg:              config.ArgoCDConfig{},
			expectedReplicas: 1,
		},
		{
			name: "HA mode without custom replicas",
			cfg: config.ArgoCDConfig{
				HA: true,
			},
			expectedReplicas: 1,
		},
		{
			name: "HA mode with custom replicas",
			cfg: config.ArgoCDConfig{
				HA:                 true,
				ControllerReplicas: intPtrArgoCD(2),
			},
			expectedReplicas: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := buildArgoCDController(tt.cfg)

			assert.Equal(t, tt.expectedReplicas, controller["replicas"])

			// Check tolerations exist
			tolerations, ok := controller["tolerations"].([]helm.Values)
			require.True(t, ok)
			require.Len(t, tolerations, 1)
			assert.Equal(t, "node.cloudprovider.kubernetes.io/uninitialized", tolerations[0]["key"])
		})
	}
}

func TestBuildArgoCDServer(t *testing.T) {
	tests := []struct {
		name             string
		cfg              config.ArgoCDConfig
		expectedReplicas int
		expectIngress    bool
	}{
		{
			name:             "default configuration",
			cfg:              config.ArgoCDConfig{},
			expectedReplicas: 1,
			expectIngress:    false,
		},
		{
			name: "HA mode default replicas",
			cfg: config.ArgoCDConfig{
				HA: true,
			},
			expectedReplicas: 2,
			expectIngress:    false,
		},
		{
			name: "with ingress",
			cfg: config.ArgoCDConfig{
				IngressEnabled: true,
				IngressHost:    "argocd.example.com",
			},
			expectedReplicas: 1,
			expectIngress:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := buildArgoCDServer(tt.cfg)

			assert.Equal(t, tt.expectedReplicas, server["replicas"])

			if tt.expectIngress {
				_, hasIngress := server["ingress"]
				assert.True(t, hasIngress)
			}
		})
	}
}

func TestBuildArgoCDIngress(t *testing.T) {
	tests := []struct {
		name              string
		cfg               config.ArgoCDConfig
		expectedHost      string
		expectedClassName string
		expectTLS         bool
	}{
		{
			name: "basic ingress",
			cfg: config.ArgoCDConfig{
				IngressEnabled: true,
				IngressHost:    "argocd.example.com",
			},
			expectedHost:      "argocd.example.com",
			expectedClassName: "",
			expectTLS:         false,
		},
		{
			name: "ingress with class name",
			cfg: config.ArgoCDConfig{
				IngressEnabled:   true,
				IngressHost:      "argocd.mycompany.io",
				IngressClassName: "nginx",
			},
			expectedHost:      "argocd.mycompany.io",
			expectedClassName: "nginx",
			expectTLS:         false,
		},
		{
			name: "ingress with TLS",
			cfg: config.ArgoCDConfig{
				IngressEnabled: true,
				IngressHost:    "argocd.secure.io",
				IngressTLS:     true,
			},
			expectedHost: "argocd.secure.io",
			expectTLS:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ingress := buildArgoCDIngress(tt.cfg)

			assert.Equal(t, true, ingress["enabled"])

			hosts, ok := ingress["hosts"].([]string)
			require.True(t, ok)
			require.Len(t, hosts, 1)
			assert.Equal(t, tt.expectedHost, hosts[0])

			if tt.expectedClassName != "" {
				assert.Equal(t, tt.expectedClassName, ingress["ingressClassName"])
			}

			if tt.expectTLS {
				tls, ok := ingress["tls"].([]helm.Values)
				require.True(t, ok)
				require.Len(t, tls, 1)
				tlsHosts, ok := tls[0]["hosts"].([]string)
				require.True(t, ok)
				assert.Contains(t, tlsHosts, tt.expectedHost)
			}
		})
	}
}

func TestBuildArgoCDRedis(t *testing.T) {
	tests := []struct {
		name          string
		cfg           config.ArgoCDConfig
		expectEnabled bool
	}{
		{
			name:          "standalone redis enabled by default",
			cfg:           config.ArgoCDConfig{},
			expectEnabled: true,
		},
		{
			name: "redis disabled when HA enabled",
			cfg: config.ArgoCDConfig{
				HA: true,
			},
			expectEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redis := buildArgoCDRedis(tt.cfg)

			assert.Equal(t, tt.expectEnabled, redis["enabled"])
		})
	}
}

func TestCreateArgoCDNamespace(t *testing.T) {
	ns := createArgoCDNamespace()

	assert.Contains(t, ns, "apiVersion: v1")
	assert.Contains(t, ns, "kind: Namespace")
	assert.Contains(t, ns, "name: argocd")
}

func TestBuildArgoCDValuesCustomHelmValues(t *testing.T) {
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			ArgoCD: config.ArgoCDConfig{
				Enabled: true,
				Helm: config.HelmChartConfig{
					Values: map[string]any{
						"customKey": "customValue",
					},
				},
			},
		},
	}

	values := buildArgoCDValues(cfg)

	// Custom values should be merged
	assert.Equal(t, "customValue", values["customKey"])

	// Base values should still exist
	_, hasCRDs := values["crds"]
	assert.True(t, hasCRDs)
}

// intPtrArgoCD is a helper to create int pointers for tests
func intPtrArgoCD(i int) *int {
	return &i
}
