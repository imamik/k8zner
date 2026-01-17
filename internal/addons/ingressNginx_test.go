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

			// Check admission webhooks - certManager enabled to avoid race conditions
			webhooks, ok := controller["admissionWebhooks"].(helm.Values)
			require.True(t, ok)
			certManager, ok := webhooks["certManager"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, true, certManager["enabled"]) // Use cert-manager for webhook certs

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
			assert.Equal(t, true, proxyConfig["compute-full-forwarded-for"])
			assert.Equal(t, true, proxyConfig["use-proxy-protocol"])

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

func TestSplitIngressNginxManifests(t *testing.T) {
	// Sample manifests with cert-manager resources and other resources
	manifests := `apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: ingress-nginx-self-signed-issuer
  namespace: ingress-nginx
spec:
  selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: ingress-nginx-admission
  namespace: ingress-nginx
spec:
  secretName: ingress-nginx-admission
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ingress-nginx
  namespace: ingress-nginx
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ingress-nginx-controller
  namespace: ingress-nginx
spec:
  replicas: 2`

	certManagerResources, otherResources := splitIngressNginxManifests(manifests)

	// Verify cert-manager resources are split out
	assert.Contains(t, certManagerResources, "kind: Issuer")
	assert.Contains(t, certManagerResources, "kind: Certificate")
	assert.NotContains(t, certManagerResources, "kind: ServiceAccount")
	assert.NotContains(t, certManagerResources, "kind: Deployment")

	// Verify other resources don't have cert-manager resources
	assert.NotContains(t, otherResources, "kind: Issuer")
	assert.NotContains(t, otherResources, "kind: Certificate")
	assert.Contains(t, otherResources, "kind: ServiceAccount")
	assert.Contains(t, otherResources, "kind: Deployment")
}

func TestSplitIngressNginxManifestsNoCertManager(t *testing.T) {
	// Test when there are no cert-manager resources
	manifests := `apiVersion: v1
kind: ServiceAccount
metadata:
  name: ingress-nginx
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ingress-nginx-controller`

	certManagerResources, otherResources := splitIngressNginxManifests(manifests)

	assert.Empty(t, certManagerResources)
	assert.Contains(t, otherResources, "kind: ServiceAccount")
	assert.Contains(t, otherResources, "kind: Deployment")
}

func TestIsCertManagerResource(t *testing.T) {
	tests := []struct {
		name     string
		doc      string
		expected bool
	}{
		{
			name: "cert-manager Issuer",
			doc: `apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: test-issuer`,
			expected: true,
		},
		{
			name: "cert-manager Certificate",
			doc: `apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: test-cert`,
			expected: true,
		},
		{
			name: "Kubernetes Deployment",
			doc: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment`,
			expected: false,
		},
		{
			name: "Kubernetes Service",
			doc: `apiVersion: v1
kind: Service
metadata:
  name: test-service`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCertManagerResource(tt.doc)
			assert.Equal(t, tt.expected, result)
		})
	}
}
