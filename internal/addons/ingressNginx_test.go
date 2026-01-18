package addons

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hcloud-k8s/internal/addons/helm"
	"hcloud-k8s/internal/config"
)

func TestBuildIngressNginxValues(t *testing.T) {
	tests := []struct {
		name             string
		workerCount      int
		expectedReplicas int
		expectedTSCMode  string // "DoNotSchedule" or "ScheduleAnyway"
	}{
		{
			name:             "single worker",
			workerCount:      1,
			expectedReplicas: 2,
			expectedTSCMode:  "ScheduleAnyway",
		},
		{
			name:             "two workers",
			workerCount:      2,
			expectedReplicas: 2,
			expectedTSCMode:  "DoNotSchedule",
		},
		{
			name:             "three workers",
			workerCount:      3,
			expectedReplicas: 3,
			expectedTSCMode:  "DoNotSchedule",
		},
		{
			name:             "five workers",
			workerCount:      5,
			expectedReplicas: 3,
			expectedTSCMode:  "DoNotSchedule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Workers: []config.WorkerNodePool{
					{Count: tt.workerCount},
				},
				Addons: config.AddonsConfig{
					IngressNginx: config.IngressNginxConfig{
						Enabled: true,
					},
				},
			}

			values := buildIngressNginxValues(cfg)

			// Check controller exists
			controller, ok := values["controller"].(helm.Values)
			require.True(t, ok)

			// Check replica count
			assert.Equal(t, tt.expectedReplicas, controller["replicaCount"])

			// Check kind
			assert.Equal(t, "Deployment", controller["kind"])

			// Check admission webhooks are disabled
			// Admission webhooks require either Helm hooks or cert-manager integration,
			// both of which have issues with kubectl apply workflow
			webhooks, ok := controller["admissionWebhooks"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, false, webhooks["enabled"])

			// Check maxUnavailable
			assert.Equal(t, 1, controller["maxUnavailable"])

			// Check watchIngressWithoutClass
			assert.Equal(t, true, controller["watchIngressWithoutClass"])

			// Check topology spread constraints
			tsc, ok := controller["topologySpreadConstraints"].([]helm.Values)
			require.True(t, ok)
			assert.Len(t, tsc, 2)

			// Check hostname constraint
			hostnameConstraint := tsc[0]
			assert.Equal(t, "kubernetes.io/hostname", hostnameConstraint["topologyKey"])
			assert.Equal(t, 1, hostnameConstraint["maxSkew"])
			assert.Equal(t, tt.expectedTSCMode, hostnameConstraint["whenUnsatisfiable"])

			// Check zone constraint (always ScheduleAnyway)
			zoneConstraint := tsc[1]
			assert.Equal(t, "topology.kubernetes.io/zone", zoneConstraint["topologyKey"])
			assert.Equal(t, 1, zoneConstraint["maxSkew"])
			assert.Equal(t, "ScheduleAnyway", zoneConstraint["whenUnsatisfiable"])

			// Verify both constraints have same label selector
			hostnameLabels, ok := hostnameConstraint["labelSelector"].(helm.Values)
			require.True(t, ok)
			zoneLabels, ok := zoneConstraint["labelSelector"].(helm.Values)
			require.True(t, ok)

			hostnameMatchLabels, ok := hostnameLabels["matchLabels"].(helm.Values)
			require.True(t, ok)
			zoneMatchLabels, ok := zoneLabels["matchLabels"].(helm.Values)
			require.True(t, ok)

			assert.Equal(t, "ingress-nginx", hostnameMatchLabels["app.kubernetes.io/instance"])
			assert.Equal(t, "ingress-nginx", hostnameMatchLabels["app.kubernetes.io/name"])
			assert.Equal(t, "controller", hostnameMatchLabels["app.kubernetes.io/component"])

			assert.Equal(t, hostnameMatchLabels, zoneMatchLabels)

			// Check service configuration
			service, ok := controller["service"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, "NodePort", service["type"])
			assert.Equal(t, "Local", service["externalTrafficPolicy"])

			nodePorts, ok := service["nodePorts"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, 30000, nodePorts["http"])
			assert.Equal(t, 30001, nodePorts["https"])

			// Check proxy config
			proxyConfig, ok := controller["config"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, "true", proxyConfig["compute-full-forwarded-for"])
			assert.Equal(t, "true", proxyConfig["use-proxy-protocol"])

			// Check network policy
			networkPolicy, ok := controller["networkPolicy"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, true, networkPolicy["enabled"])

			// Check tolerations for CCM uninitialized taint
			tolerations, ok := controller["tolerations"].([]helm.Values)
			require.True(t, ok)
			require.Len(t, tolerations, 1)
			assert.Equal(t, "node.cloudprovider.kubernetes.io/uninitialized", tolerations[0]["key"])
			assert.Equal(t, "Exists", tolerations[0]["operator"])
		})
	}
}

func TestBuildIngressNginxTopologySpread(t *testing.T) {
	tests := []struct {
		name                          string
		workerCount                   int
		expectedHostnameUnsatisfiable string
	}{
		{
			name:                          "single worker - soft constraint",
			workerCount:                   1,
			expectedHostnameUnsatisfiable: "ScheduleAnyway",
		},
		{
			name:                          "multiple workers - hard constraint",
			workerCount:                   3,
			expectedHostnameUnsatisfiable: "DoNotSchedule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			constraints := buildIngressNginxTopologySpread(tt.workerCount)

			assert.Len(t, constraints, 2)
			assert.Equal(t, tt.expectedHostnameUnsatisfiable, constraints[0]["whenUnsatisfiable"])
			assert.Equal(t, "ScheduleAnyway", constraints[1]["whenUnsatisfiable"])
		})
	}
}

func TestCreateIngressNginxNamespace(t *testing.T) {
	ns := createIngressNginxNamespace()

	assert.Contains(t, ns, "apiVersion: v1")
	assert.Contains(t, ns, "kind: Namespace")
	assert.Contains(t, ns, "name: ingress-nginx")
}

func TestBuildIngressNginxValuesKind(t *testing.T) {
	tests := []struct {
		name         string
		kind         string
		expectedKind string
	}{
		{
			name:         "default is Deployment",
			kind:         "",
			expectedKind: "Deployment",
		},
		{
			name:         "explicit Deployment",
			kind:         "Deployment",
			expectedKind: "Deployment",
		},
		{
			name:         "DaemonSet",
			kind:         "DaemonSet",
			expectedKind: "DaemonSet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Workers: []config.WorkerNodePool{{Count: 2}},
				Addons: config.AddonsConfig{
					IngressNginx: config.IngressNginxConfig{
						Enabled: true,
						Kind:    tt.kind,
					},
				},
			}

			values := buildIngressNginxValues(cfg)
			controller := values["controller"].(helm.Values)
			assert.Equal(t, tt.expectedKind, controller["kind"])
		})
	}
}

func TestBuildIngressNginxValuesReplicas(t *testing.T) {
	tests := []struct {
		name             string
		replicas         *int
		workerCount      int
		expectedReplicas int
	}{
		{
			name:             "auto-calculate for small cluster",
			replicas:         nil,
			workerCount:      2,
			expectedReplicas: 2,
		},
		{
			name:             "auto-calculate for large cluster",
			replicas:         nil,
			workerCount:      5,
			expectedReplicas: 3,
		},
		{
			name:             "explicit replicas",
			replicas:         intPtr(5),
			workerCount:      2,
			expectedReplicas: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Workers: []config.WorkerNodePool{{Count: tt.workerCount}},
				Addons: config.AddonsConfig{
					IngressNginx: config.IngressNginxConfig{
						Enabled:  true,
						Replicas: tt.replicas,
					},
				},
			}

			values := buildIngressNginxValues(cfg)
			controller := values["controller"].(helm.Values)
			assert.Equal(t, tt.expectedReplicas, controller["replicaCount"])
		})
	}
}

func TestBuildIngressNginxValuesExternalTrafficPolicy(t *testing.T) {
	tests := []struct {
		name           string
		policy         string
		expectedPolicy string
	}{
		{
			name:           "default is Local",
			policy:         "",
			expectedPolicy: "Local",
		},
		{
			name:           "explicit Local",
			policy:         "Local",
			expectedPolicy: "Local",
		},
		{
			name:           "Cluster policy",
			policy:         "Cluster",
			expectedPolicy: "Cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Workers: []config.WorkerNodePool{{Count: 2}},
				Addons: config.AddonsConfig{
					IngressNginx: config.IngressNginxConfig{
						Enabled:               true,
						ExternalTrafficPolicy: tt.policy,
					},
				},
			}

			values := buildIngressNginxValues(cfg)
			controller := values["controller"].(helm.Values)
			service := controller["service"].(helm.Values)
			assert.Equal(t, tt.expectedPolicy, service["externalTrafficPolicy"])
		})
	}
}

func TestBuildIngressNginxValuesTopologyAwareRouting(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		expected bool
	}{
		{
			name:     "disabled by default",
			enabled:  false,
			expected: false,
		},
		{
			name:     "enabled",
			enabled:  true,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Workers: []config.WorkerNodePool{{Count: 2}},
				Addons: config.AddonsConfig{
					IngressNginx: config.IngressNginxConfig{
						Enabled:              true,
						TopologyAwareRouting: tt.enabled,
					},
				},
			}

			values := buildIngressNginxValues(cfg)
			controller := values["controller"].(helm.Values)
			assert.Equal(t, tt.expected, controller["enableTopologyAwareRouting"])
		})
	}
}

func TestBuildIngressNginxValuesConfig(t *testing.T) {
	tests := []struct {
		name           string
		customConfig   map[string]string
		expectedConfig map[string]string
	}{
		{
			name:         "default config",
			customConfig: nil,
			expectedConfig: map[string]string{
				"compute-full-forwarded-for": "true",
				"use-proxy-protocol":         "true",
			},
		},
		{
			name: "custom config merges with defaults",
			customConfig: map[string]string{
				"proxy-body-size": "100m",
				"ssl-protocols":   "TLSv1.3",
			},
			expectedConfig: map[string]string{
				"compute-full-forwarded-for": "true",
				"use-proxy-protocol":         "true",
				"proxy-body-size":            "100m",
				"ssl-protocols":              "TLSv1.3",
			},
		},
		{
			name: "custom config can override defaults",
			customConfig: map[string]string{
				"use-proxy-protocol": "false",
			},
			expectedConfig: map[string]string{
				"compute-full-forwarded-for": "true",
				"use-proxy-protocol":         "false",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Workers: []config.WorkerNodePool{{Count: 2}},
				Addons: config.AddonsConfig{
					IngressNginx: config.IngressNginxConfig{
						Enabled: true,
						Config:  tt.customConfig,
					},
				},
			}

			values := buildIngressNginxValues(cfg)
			controller := values["controller"].(helm.Values)
			configMap := controller["config"].(helm.Values)

			for k, v := range tt.expectedConfig {
				assert.Equal(t, v, configMap[k], "key %s should be %s", k, v)
			}
		})
	}
}

func intPtr(i int) *int {
	return &i
}
