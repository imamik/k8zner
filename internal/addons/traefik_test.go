package addons

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/imamik/k8zner/internal/addons/helm"
	"github.com/imamik/k8zner/internal/config"
)

func TestBuildTraefikValues(t *testing.T) {
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
					Traefik: config.TraefikConfig{
						Enabled: true,
					},
				},
			}

			values := buildTraefikValues(cfg)

			// Check deployment exists
			deployment, ok := values["deployment"].(helm.Values)
			require.True(t, ok)

			// Check replica count
			assert.Equal(t, tt.expectedReplicas, deployment["replicas"])

			// Check kind
			assert.Equal(t, "Deployment", deployment["kind"])

			// Check pod disruption budget
			pdb, ok := deployment["podDisruptionBudget"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, true, pdb["enabled"])
			assert.Equal(t, 1, pdb["maxUnavailable"])

			// Check topology spread constraints
			tsc, ok := values["topologySpreadConstraints"].([]helm.Values)
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

			assert.Equal(t, "traefik", hostnameMatchLabels["app.kubernetes.io/instance"])
			assert.Equal(t, "traefik", hostnameMatchLabels["app.kubernetes.io/name"])

			assert.Equal(t, hostnameMatchLabels, zoneMatchLabels)

			// Check service configuration
			service, ok := values["service"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, "NodePort", service["type"])

			spec, ok := service["spec"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, "Local", spec["externalTrafficPolicy"])

			// Check ports configuration
			ports, ok := values["ports"].(helm.Values)
			require.True(t, ok)

			web, ok := ports["web"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, 30000, web["nodePort"])

			websecure, ok := ports["websecure"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, 30001, websecure["nodePort"])

			// Check tolerations for CCM uninitialized taint
			tolerations, ok := values["tolerations"].([]helm.Values)
			require.True(t, ok)
			require.Len(t, tolerations, 1)
			assert.Equal(t, "node.cloudprovider.kubernetes.io/uninitialized", tolerations[0]["key"])
			assert.Equal(t, "Exists", tolerations[0]["operator"])

			// Check ingress class configuration
			ingressClass, ok := values["ingressClass"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, true, ingressClass["enabled"])
			assert.Equal(t, true, ingressClass["isDefaultClass"])
			assert.Equal(t, "traefik", ingressClass["name"])
		})
	}
}

func TestBuildTraefikTopologySpread(t *testing.T) {
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
			constraints := buildTraefikTopologySpread(tt.workerCount)

			assert.Len(t, constraints, 2)
			assert.Equal(t, tt.expectedHostnameUnsatisfiable, constraints[0]["whenUnsatisfiable"])
			assert.Equal(t, "ScheduleAnyway", constraints[1]["whenUnsatisfiable"])
		})
	}
}

func TestCreateTraefikNamespace(t *testing.T) {
	ns := createTraefikNamespace()

	assert.Contains(t, ns, "apiVersion: v1")
	assert.Contains(t, ns, "kind: Namespace")
	assert.Contains(t, ns, "name: traefik")
}

func TestBuildTraefikValuesKind(t *testing.T) {
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
					Traefik: config.TraefikConfig{
						Enabled: true,
						Kind:    tt.kind,
					},
				},
			}

			values := buildTraefikValues(cfg)
			deployment := values["deployment"].(helm.Values)
			assert.Equal(t, tt.expectedKind, deployment["kind"])
		})
	}
}

func TestBuildTraefikValuesReplicas(t *testing.T) {
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
					Traefik: config.TraefikConfig{
						Enabled:  true,
						Replicas: tt.replicas,
					},
				},
			}

			values := buildTraefikValues(cfg)
			deployment := values["deployment"].(helm.Values)
			assert.Equal(t, tt.expectedReplicas, deployment["replicas"])
		})
	}
}

func TestBuildTraefikValuesExternalTrafficPolicy(t *testing.T) {
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
					Traefik: config.TraefikConfig{
						Enabled:               true,
						ExternalTrafficPolicy: tt.policy,
					},
				},
			}

			values := buildTraefikValues(cfg)
			service := values["service"].(helm.Values)
			spec := service["spec"].(helm.Values)
			assert.Equal(t, tt.expectedPolicy, spec["externalTrafficPolicy"])
		})
	}
}

func TestBuildTraefikValuesIngressClass(t *testing.T) {
	tests := []struct {
		name          string
		ingressClass  string
		expectedClass string
	}{
		{
			name:          "default is traefik",
			ingressClass:  "",
			expectedClass: "traefik",
		},
		{
			name:          "custom ingress class",
			ingressClass:  "my-traefik",
			expectedClass: "my-traefik",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Workers: []config.WorkerNodePool{{Count: 2}},
				Addons: config.AddonsConfig{
					Traefik: config.TraefikConfig{
						Enabled:      true,
						IngressClass: tt.ingressClass,
					},
				},
			}

			values := buildTraefikValues(cfg)
			ingressClass := values["ingressClass"].(helm.Values)
			assert.Equal(t, tt.expectedClass, ingressClass["name"])
		})
	}
}

func TestBuildTraefikValuesIngressRoute(t *testing.T) {
	// IngressRoute is always disabled since we don't install Traefik CRDs
	cfg := &config.Config{
		Workers: []config.WorkerNodePool{{Count: 2}},
		Addons: config.AddonsConfig{
			Traefik: config.TraefikConfig{
				Enabled: true,
			},
		},
	}

	values := buildTraefikValues(cfg)
	ingressRoute := values["ingressRoute"].(helm.Values)
	dashboard := ingressRoute["dashboard"].(helm.Values)
	assert.Equal(t, false, dashboard["enabled"])
}
