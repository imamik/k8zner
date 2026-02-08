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
				ClusterName: "test-cluster",
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
			assert.Equal(t, "LoadBalancer", service["type"])

			spec, ok := service["spec"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, "Cluster", spec["externalTrafficPolicy"])

			// Check Hetzner LB annotations
			annotations, ok := service["annotations"].(helm.Values)
			require.True(t, ok)
			// New naming: {cluster}-ingress
			assert.Equal(t, "test-cluster-ingress", annotations["load-balancer.hetzner.cloud/name"])
			assert.Equal(t, "true", annotations["load-balancer.hetzner.cloud/use-private-ip"])
			_, hasProxyProtocol := annotations["load-balancer.hetzner.cloud/uses-proxyprotocol"]
			assert.False(t, hasProxyProtocol, "proxy protocol should not be set")

			// Check ports configuration (no longer uses nodePort)
			ports, ok := values["ports"].(helm.Values)
			require.True(t, ok)

			web, ok := ports["web"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, 80, web["exposedPort"])

			websecure, ok := ports["websecure"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, 443, websecure["exposedPort"])

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
				ClusterName: "test-cluster",
				Workers:     []config.WorkerNodePool{{Count: 2}},
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
			replicas:         func(i int) *int { return &i }(5),
			workerCount:      2,
			expectedReplicas: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				ClusterName: "test-cluster",
				Workers:     []config.WorkerNodePool{{Count: tt.workerCount}},
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
			name:           "default is Cluster",
			policy:         "",
			expectedPolicy: "Cluster",
		},
		{
			name:           "explicit Local",
			policy:         "Local",
			expectedPolicy: "Local",
		},
		{
			name:           "explicit Cluster",
			policy:         "Cluster",
			expectedPolicy: "Cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				ClusterName: "test-cluster",
				Workers:     []config.WorkerNodePool{{Count: 2}},
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
				ClusterName: "test-cluster",
				Workers:     []config.WorkerNodePool{{Count: 2}},
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
	// IngressRoute is always disabled - we use standard Kubernetes Ingress
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Workers:     []config.WorkerNodePool{{Count: 2}},
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

func TestBuildTraefikValuesAlwaysLoadBalancer(t *testing.T) {
	// Traefik always uses LoadBalancer service with Deployment, regardless of config
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "fsn1",
		Workers:     []config.WorkerNodePool{{Count: 2}},
		Addons: config.AddonsConfig{
			Traefik: config.TraefikConfig{
				Enabled: true,
			},
		},
	}

	values := buildTraefikValues(cfg)

	// No hostNetwork should be set
	_, hasHostNetwork := values["hostNetwork"]
	assert.False(t, hasHostNetwork, "should not have hostNetwork")

	// Service should be LoadBalancer
	service := values["service"].(helm.Values)
	assert.Equal(t, "LoadBalancer", service["type"])

	// Should have LB annotations
	_, hasAnnotations := service["annotations"]
	assert.True(t, hasAnnotations, "should have LB annotations")

	// Deployment, not DaemonSet
	deployment := values["deployment"].(helm.Values)
	assert.Equal(t, "Deployment", deployment["kind"])

	// No hostPort on ports
	ports := values["ports"].(helm.Values)
	webPort := ports["web"].(helm.Values)
	_, hasHostPort := webPort["hostPort"]
	assert.False(t, hasHostPort, "should not have hostPort")

	// No proxy protocol on ports
	_, hasProxyProtocol := webPort["proxyProtocol"]
	assert.False(t, hasProxyProtocol, "should not have proxyProtocol on ports")

	// TLS enabled on websecure (at ports.websecure.http.tls)
	websecurePort := ports["websecure"].(helm.Values)
	wsHTTP, ok := websecurePort["http"].(helm.Values)
	require.True(t, ok, "websecure should have http config")
	wsTLS, ok := wsHTTP["tls"].(helm.Values)
	require.True(t, ok, "websecure.http should have tls config")
	assert.Equal(t, true, wsTLS["enabled"], "websecure tls should be enabled")

	// No securityContext (no NET_BIND_SERVICE needed)
	_, hasSecCtx := values["securityContext"]
	assert.False(t, hasSecCtx, "should not have securityContext")
}

func TestTraefikChartRenderLoadBalancer(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "fsn1",
		Workers:     []config.WorkerNodePool{{Count: 2}},
		Addons: config.AddonsConfig{
			Traefik: config.TraefikConfig{
				Enabled: true,
			},
		},
	}

	values := buildTraefikValues(cfg)

	// Render the actual chart
	spec := helm.GetChartSpec("traefik", config.HelmChartConfig{})
	manifests, err := helm.RenderFromSpec(t.Context(), spec, "traefik", values)
	require.NoError(t, err, "chart rendering should succeed")

	output := string(manifests)
	t.Logf("Rendered manifests length: %d bytes", len(output))

	// Verify LoadBalancer mode
	require.Contains(t, output, "kind: Deployment", "rendered manifest must have kind: Deployment")
	require.Contains(t, output, "type: LoadBalancer", "rendered manifest must have LoadBalancer service")
	require.NotContains(t, output, "hostNetwork: true", "rendered manifest must NOT have hostNetwork")
	require.NotContains(t, output, "hostPort:", "rendered manifest must NOT have hostPort")
}
