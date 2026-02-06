//go:build kind

package kind

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/imamik/k8zner/internal/addons"
	"github.com/imamik/k8zner/internal/config"
)

// TestKindAddons orchestrates addon tests in dependency order.
func TestKindAddons(t *testing.T) {
	t.Run("01_CRDs", func(t *testing.T) {
		t.Run("GatewayAPI", testGatewayAPICRDs)
		t.Run("PrometheusOperator", testPrometheusOperatorCRDs)
	})

	t.Run("02_Core", func(t *testing.T) {
		t.Run("CertManager", testCertManager)
		t.Run("MetricsServer", testMetricsServer)
	})

	t.Run("03_Ingress", func(t *testing.T) {
		t.Run("Traefik", testTraefik)
	})

	t.Run("04_GitOps", func(t *testing.T) {
		t.Run("ArgoCD", testArgoCD)
	})

	t.Run("05_Monitoring", func(t *testing.T) {
		t.Run("KubePrometheusStack", testKubePrometheusStack)
	})

	t.Run("06_Integration", func(t *testing.T) {
		t.Run("IngressRouting", testIngressRouting)
		t.Run("CertificateIssuance", testCertificateIssuance)
		t.Run("ServiceMonitor", testServiceMonitorCreation)
	})
}

// =============================================================================
// Layer 1: CRDs
// =============================================================================

func testGatewayAPICRDs(t *testing.T) {
	if fw.IsInstalled("gateway-api-crds") {
		t.Log("Already installed, skipping")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	err := addons.Apply(ctx, &config.Config{
		ClusterName: "kind-test",
		Addons: config.AddonsConfig{
			GatewayAPICRDs: config.GatewayAPICRDsConfig{Enabled: true},
		},
	}, fw.Kubeconfig(), 0)
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	crds := []string{
		"gateways.gateway.networking.k8s.io",
		"gatewayclasses.gateway.networking.k8s.io",
		"httproutes.gateway.networking.k8s.io",
		"referencegrants.gateway.networking.k8s.io",
	}
	for _, crd := range crds {
		fw.WaitForCRD(t, crd, 30*time.Second)
	}

	fw.MarkInstalled("gateway-api-crds")
	t.Log("✓ Gateway API CRDs")
}

func testPrometheusOperatorCRDs(t *testing.T) {
	if fw.IsInstalled("prometheus-operator-crds") {
		t.Log("Already installed, skipping")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	err := addons.Apply(ctx, &config.Config{
		ClusterName: "kind-test",
		Addons: config.AddonsConfig{
			PrometheusOperatorCRDs: config.PrometheusOperatorCRDsConfig{Enabled: true},
		},
	}, fw.Kubeconfig(), 0)
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	crds := []string{
		"prometheuses.monitoring.coreos.com",
		"prometheusrules.monitoring.coreos.com",
		"servicemonitors.monitoring.coreos.com",
		"podmonitors.monitoring.coreos.com",
		"alertmanagers.monitoring.coreos.com",
	}
	for _, crd := range crds {
		fw.WaitForCRD(t, crd, 30*time.Second)
	}

	fw.MarkInstalled("prometheus-operator-crds")
	t.Log("✓ Prometheus Operator CRDs")
}

// =============================================================================
// Layer 2: Core
// =============================================================================

func testCertManager(t *testing.T) {
	if fw.IsInstalled("cert-manager") {
		t.Log("Already installed, verifying...")
		fw.AssertDeploymentReady(t, "cert-manager", "cert-manager")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err := addons.Apply(ctx, &config.Config{
		ClusterName: "kind-test",
		Addons: config.AddonsConfig{
			CertManager: config.CertManagerConfig{Enabled: true},
		},
	}, fw.Kubeconfig(), 0)
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	fw.WaitForNamespace(t, "cert-manager", 30*time.Second)
	fw.WaitForDeployment(t, "cert-manager", "cert-manager", 3*time.Minute)
	fw.WaitForDeployment(t, "cert-manager", "cert-manager-webhook", 3*time.Minute)
	fw.WaitForDeployment(t, "cert-manager", "cert-manager-cainjector", 3*time.Minute)

	fw.AssertCRDsExist(t, []string{
		"certificates.cert-manager.io",
		"clusterissuers.cert-manager.io",
	})

	fw.MarkInstalled("cert-manager")
	t.Log("✓ cert-manager")
}

func testMetricsServer(t *testing.T) {
	if fw.IsInstalled("metrics-server") {
		t.Log("Already installed, verifying...")
		fw.AssertDeploymentReady(t, "kube-system", "metrics-server")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err := addons.Apply(ctx, &config.Config{
		ClusterName: "kind-test",
		Addons: config.AddonsConfig{
			MetricsServer: config.MetricsServerConfig{Enabled: true},
		},
	}, fw.Kubeconfig(), 0)
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	fw.WaitForDeployment(t, "kube-system", "metrics-server", 3*time.Minute)
	fw.AssertServiceExists(t, "kube-system", "metrics-server")

	fw.MarkInstalled("metrics-server")
	t.Log("✓ metrics-server")
}

// =============================================================================
// Layer 3: Ingress
// =============================================================================

func testTraefik(t *testing.T) {
	if fw.IsInstalled("traefik") {
		t.Log("Already installed, verifying...")
		fw.AssertDeploymentReady(t, "traefik", "traefik")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err := addons.Apply(ctx, &config.Config{
		ClusterName: "kind-test",
		Addons: config.AddonsConfig{
			Traefik: config.TraefikConfig{Enabled: true},
		},
	}, fw.Kubeconfig(), 0)
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	fw.WaitForNamespace(t, "traefik", 30*time.Second)
	fw.WaitForDeployment(t, "traefik", "traefik", 3*time.Minute)

	if !fw.ResourceExists("ingressclass", "", "traefik") {
		t.Error("IngressClass not created")
	}

	fw.MarkInstalled("traefik")
	t.Log("✓ Traefik")
}

// =============================================================================
// Layer 4: GitOps
// =============================================================================

func testArgoCD(t *testing.T) {
	if fw.IsInstalled("argocd") {
		t.Log("Already installed, verifying...")
		fw.AssertDeploymentReady(t, "argocd", "argocd-server")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	err := addons.Apply(ctx, &config.Config{
		ClusterName: "kind-test",
		Addons: config.AddonsConfig{
			ArgoCD: config.ArgoCDConfig{Enabled: true},
		},
	}, fw.Kubeconfig(), 0)
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	fw.WaitForNamespace(t, "argocd", 30*time.Second)
	fw.WaitForDeployment(t, "argocd", "argocd-server", 5*time.Minute)
	fw.WaitForDeployment(t, "argocd", "argocd-repo-server", 5*time.Minute)
	fw.WaitForDeployment(t, "argocd", "argocd-redis", 3*time.Minute)
	fw.WaitForPod(t, "argocd", "app.kubernetes.io/name=argocd-application-controller", 5*time.Minute)

	fw.AssertCRDsExist(t, []string{
		"applications.argoproj.io",
		"appprojects.argoproj.io",
	})

	fw.MarkInstalled("argocd")
	t.Log("✓ ArgoCD")
}

// =============================================================================
// Layer 5: Monitoring
// =============================================================================

func testKubePrometheusStack(t *testing.T) {
	if !fw.IsInstalled("prometheus-operator-crds") {
		t.Skip("Prometheus CRDs not installed")
	}

	if fw.IsInstalled("kube-prometheus-stack") {
		t.Log("Already installed, verifying...")
		fw.AssertDeploymentReady(t, "monitoring", "kube-prometheus-stack-operator")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	err := addons.Apply(ctx, &config.Config{
		ClusterName: "kind-test",
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{Enabled: true},
		},
	}, fw.Kubeconfig(), 0)
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	fw.WaitForNamespace(t, "monitoring", 30*time.Second)
	fw.WaitForDeployment(t, "monitoring", "kube-prometheus-stack-operator", 5*time.Minute)
	fw.WaitForPod(t, "monitoring", "app.kubernetes.io/name=prometheus", 5*time.Minute)
	fw.WaitForPod(t, "monitoring", "app.kubernetes.io/name=alertmanager", 5*time.Minute)
	fw.WaitForDeployment(t, "monitoring", "kube-prometheus-stack-grafana", 5*time.Minute)

	// Verify ServiceMonitors created
	output, _ := fw.Kubectl("-n", "monitoring", "get", "servicemonitors", "-o", "name")
	count := len(strings.Split(strings.TrimSpace(output), "\n"))
	if count < 5 {
		t.Errorf("expected >=5 ServiceMonitors, got %d", count)
	}
	t.Logf("ServiceMonitors: %d", count)

	fw.MarkInstalled("kube-prometheus-stack")
	t.Log("✓ kube-prometheus-stack")
}

// =============================================================================
// Layer 6: Integration
// =============================================================================

func testIngressRouting(t *testing.T) {
	if !fw.IsInstalled("traefik") {
		t.Skip("Traefik not installed")
	}

	fw.KubectlApply(t, `
apiVersion: v1
kind: Namespace
metadata:
  name: integration-test
`)
	defer fw.KubectlDelete("", "namespace", "integration-test")

	fw.KubectlApply(t, `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: echo
  namespace: integration-test
spec:
  replicas: 1
  selector:
    matchLabels:
      app: echo
  template:
    metadata:
      labels:
        app: echo
    spec:
      containers:
      - name: echo
        image: hashicorp/http-echo:latest
        args: ["-text=hello"]
        ports:
        - containerPort: 5678
---
apiVersion: v1
kind: Service
metadata:
  name: echo
  namespace: integration-test
spec:
  selector:
    app: echo
  ports:
  - port: 80
    targetPort: 5678
`)

	fw.WaitForDeployment(t, "integration-test", "echo", 2*time.Minute)

	fw.KubectlApply(t, `
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: echo
  namespace: integration-test
spec:
  ingressClassName: traefik
  rules:
  - host: echo.local
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: echo
            port:
              number: 80
`)

	time.Sleep(3 * time.Second)
	if !fw.ResourceExists("ingress", "integration-test", "echo") {
		t.Fatal("Ingress not created")
	}

	t.Log("✓ Ingress routing")
}

func testCertificateIssuance(t *testing.T) {
	if !fw.IsInstalled("cert-manager") {
		t.Skip("cert-manager not installed")
	}

	fw.KubectlApply(t, `
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned-test
spec:
  selfSigned: {}
`)

	if !fw.NamespaceExists("integration-test") {
		fw.KubectlApply(t, `
apiVersion: v1
kind: Namespace
metadata:
  name: integration-test
`)
	}

	fw.KubectlApply(t, `
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: test-cert
  namespace: integration-test
spec:
  secretName: test-cert-tls
  duration: 2160h
  issuerRef:
    name: selfsigned-test
    kind: ClusterIssuer
  commonName: test.local
  dnsNames:
  - test.local
`)

	fw.WaitForCertificateReady(t, "integration-test", "test-cert", 60*time.Second)
	fw.AssertSecretExists(t, "integration-test", "test-cert-tls")

	t.Log("✓ Certificate issuance")
}

func testServiceMonitorCreation(t *testing.T) {
	if !fw.IsInstalled("prometheus-operator-crds") {
		t.Skip("Prometheus CRDs not installed")
	}

	if !fw.NamespaceExists("integration-test") {
		fw.KubectlApply(t, `
apiVersion: v1
kind: Namespace
metadata:
  name: integration-test
`)
	}

	fw.KubectlApply(t, `
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: test-monitor
  namespace: integration-test
spec:
  selector:
    matchLabels:
      app: test-app
  endpoints:
  - port: metrics
`)

	time.Sleep(2 * time.Second)
	if !fw.ResourceExists("servicemonitor", "integration-test", "test-monitor") {
		t.Fatal("ServiceMonitor not created")
	}

	t.Log("✓ ServiceMonitor creation")
}
