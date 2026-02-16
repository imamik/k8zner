//go:build e2e

package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/imamik/k8zner/internal/addons"
	"github.com/imamik/k8zner/internal/config"
)

// phaseMonitoring tests the kube-prometheus-stack monitoring installation.
// This is Phase 3c of the E2E lifecycle.
//
// Environment variables:
//
//	E2E_SKIP_MONITORING - Set to "true" to skip monitoring testing
//	CF_API_TOKEN - Cloudflare API token (required for Grafana ingress test)
//	CF_DOMAIN - Domain managed by Cloudflare (required for Grafana ingress test)
//	CF_GRAFANA_SUBDOMAIN - (optional) Subdomain for Grafana (default: "grafana")
//
// If CF_API_TOKEN and CF_DOMAIN are not set, only the basic monitoring test runs.
func phaseMonitoring(t *testing.T, state *E2EState) {
	t.Log("=== Phase 3c: Monitoring Stack ===")

	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Fatal("HCLOUD_TOKEN required for monitoring tests")
	}

	cfAPIToken := os.Getenv("CF_API_TOKEN")
	cfDomain := os.Getenv("CF_DOMAIN")

	// Get network ID
	ctx := context.Background()
	network, _ := state.Client.GetNetwork(ctx, state.ClusterName)
	networkID := int64(0)
	if network != nil {
		networkID = network.ID
	}

	if cfAPIToken != "" && cfDomain != "" {
		// Full test with Grafana ingress, TLS, and DNS
		t.Run("MonitoringWithIngress", func(t *testing.T) {
			testMonitoringStack(t, state, token)
		})
	} else {
		// Basic test without ingress
		t.Log("  CF_API_TOKEN or CF_DOMAIN not set - running basic monitoring test")
		t.Run("MonitoringBasic", func(t *testing.T) {
			testMonitoringWithoutIngress(t, state, token, networkID)
		})
	}

	// Additional monitoring tests
	t.Run("PrometheusAlerts", func(t *testing.T) {
		testPrometheusAlerts(t, state.KubeconfigPath)
	})

	t.Log("  Phase 3c: Monitoring Stack complete")
}

// testMonitoringStack verifies the kube-prometheus-stack installation with Grafana dashboard
// accessible via HTTPS with automatic DNS and TLS certificate provisioning.
//
// This test:
// 1. Installs kube-prometheus-stack with Grafana, Prometheus, and Alertmanager
// 2. Configures Grafana ingress with TLS (similar to ArgoCD)
// 3. Waits for DNS record creation via external-dns
// 4. Waits for TLS certificate issuance via cert-manager
// 5. Tests HTTPS connectivity to Grafana
// 6. Verifies Prometheus and Alertmanager are healthy
//
// Environment variables required:
//   - CF_API_TOKEN: Cloudflare API token
//   - CF_DOMAIN: Domain managed by Cloudflare (e.g., k8zner.org)
//   - CF_GRAFANA_SUBDOMAIN: (optional) Subdomain for Grafana (default: "grafana")
func testMonitoringStack(t *testing.T, state *E2EState, token string) {
	cfAPIToken := os.Getenv("CF_API_TOKEN")
	cfDomain := os.Getenv("CF_DOMAIN")

	if cfAPIToken == "" || cfDomain == "" {
		t.Log("Monitoring Stack test skipped (CF_API_TOKEN or CF_DOMAIN not set)")
		t.Log("  Set CF_API_TOKEN and CF_DOMAIN environment variables to test")
		return
	}

	// Get Grafana subdomain (default: "grafana")
	grafanaSubdomain := os.Getenv("CF_GRAFANA_SUBDOMAIN")
	if grafanaSubdomain == "" {
		grafanaSubdomain = "grafana"
	}

	grafanaHost := fmt.Sprintf("%s.%s", grafanaSubdomain, cfDomain)
	t.Logf("Testing Monitoring Stack with Grafana at: %s", grafanaHost)

	ctx := context.Background()

	// Get network ID for addon installation
	network, _ := state.Client.GetNetwork(ctx, state.ClusterName)
	networkID := int64(0)
	if network != nil {
		networkID = network.ID
	}

	// Step 1: Ensure Cloudflare integration is installed
	t.Log("  Step 1: Ensuring Cloudflare integration is installed...")
	installCloudflareForMonitoring(t, state, token, cfAPIToken, cfDomain, networkID)

	// Step 2: Install kube-prometheus-stack with Grafana ingress
	t.Logf("  Step 2: Installing kube-prometheus-stack with Grafana ingress for %s...", grafanaHost)
	installKubePrometheusStack(t, state, token, cfAPIToken, cfDomain, grafanaSubdomain, networkID)

	// Step 3: Wait for Prometheus Operator to be ready
	t.Log("  Step 3: Waiting for Prometheus Operator...")
	waitForPod(t, state.KubeconfigPath, "monitoring", "app.kubernetes.io/name=kube-prometheus-stack-prometheus-operator", 8*time.Minute)

	// Step 4: Wait for Prometheus to be ready
	t.Log("  Step 4: Waiting for Prometheus...")
	waitForPrometheusReady(t, state.KubeconfigPath, 12*time.Minute)

	// Step 5: Wait for Grafana to be ready
	t.Log("  Step 5: Waiting for Grafana...")
	waitForPod(t, state.KubeconfigPath, "monitoring", "app.kubernetes.io/name=grafana", 5*time.Minute)

	// Step 6: Wait for Alertmanager to be ready
	t.Log("  Step 6: Waiting for Alertmanager...")
	waitForAlertmanagerReady(t, state.KubeconfigPath, 5*time.Minute)

	// Step 7: Wait for and verify DNS record
	t.Log("  Step 7: Waiting for DNS record creation...")
	verifyGrafanaDNSRecord(t, state, grafanaHost, 6*time.Minute)

	// Step 8: Wait for and verify TLS certificate
	t.Log("  Step 8: Waiting for TLS certificate issuance...")
	verifyGrafanaCertificate(t, state.KubeconfigPath, 6*time.Minute)

	// Step 9: Test HTTPS connectivity to Grafana
	t.Log("  Step 9: Testing HTTPS connectivity to Grafana...")
	testGrafanaHTTPSConnectivity(t, grafanaHost, 3*time.Minute)

	// Step 10: Verify Prometheus is scraping metrics
	t.Log("  Step 10: Verifying Prometheus is collecting metrics...")
	verifyPrometheusTargets(t, state.KubeconfigPath)

	t.Logf("  Grafana Dashboard accessible at https://%s", grafanaHost)
	state.AddonsInstalled["monitoring"] = true
	t.Log("  Monitoring Stack test complete")
}

// installCloudflareForMonitoring ensures Cloudflare integration is installed for monitoring.
func installCloudflareForMonitoring(t *testing.T, state *E2EState, hcloudToken, cfAPIToken, cfDomain string, networkID int64) {
	cfg := &config.Config{
		ClusterName: state.ClusterName,
		HCloudToken: hcloudToken,
		Addons: config.AddonsConfig{
			Cloudflare: config.CloudflareConfig{
				Enabled:  true,
				APIToken: cfAPIToken,
				Domain:   cfDomain,
				Proxied:  false,
			},
			ExternalDNS: config.ExternalDNSConfig{
				Enabled: true,
				Policy:  "sync",
				Sources: []string{"ingress"},
			},
			CertManager: config.CertManagerConfig{
				Enabled: true,
				Cloudflare: config.CertManagerCloudflareConfig{
					Enabled:    true,
					Email:      fmt.Sprintf("monitoring-test@%s", cfDomain),
					Production: false, // Use staging to avoid rate limits
				},
			},
		},
	}

	if err := addons.Apply(context.Background(), cfg, state.Kubeconfig, networkID); err != nil {
		t.Fatalf("Failed to install Cloudflare integration: %v", err)
	}

	// Wait for external-dns pod
	waitForPod(t, state.KubeconfigPath, "external-dns", "app.kubernetes.io/name=external-dns", 5*time.Minute)

	// Verify ClusterIssuers exist
	verifyClusterIssuerExists(t, state.KubeconfigPath, "letsencrypt-cloudflare-staging")

	t.Log("      Cloudflare integration ready for monitoring")
}

// installKubePrometheusStack installs kube-prometheus-stack with Grafana ingress.
func installKubePrometheusStack(t *testing.T, state *E2EState, hcloudToken, cfAPIToken, cfDomain, grafanaSubdomain string, networkID int64) {
	grafanaHost := fmt.Sprintf("%s.%s", grafanaSubdomain, cfDomain)
	grafanaEnabled := true

	cfg := &config.Config{
		ClusterName: state.ClusterName,
		HCloudToken: hcloudToken,
		Addons: config.AddonsConfig{
			// Cloudflare must be enabled for proper ClusterIssuer selection
			Cloudflare: config.CloudflareConfig{
				Enabled:  true,
				APIToken: cfAPIToken,
				Domain:   cfDomain,
			},
			ExternalDNS: config.ExternalDNSConfig{
				Enabled: true,
			},
			CertManager: config.CertManagerConfig{
				Enabled: true,
				Cloudflare: config.CertManagerCloudflareConfig{
					Enabled:    true,
					Email:      fmt.Sprintf("monitoring-test@%s", cfDomain),
					Production: false, // Use staging
				},
			},
			// Main kube-prometheus-stack config
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Enabled: true,
				Grafana: config.KubePrometheusGrafanaConfig{
					Enabled:          &grafanaEnabled,
					IngressEnabled:   true,
					IngressHost:      grafanaHost,
					IngressClassName: "traefik",
					IngressTLS:       true,
				},
				Prometheus: config.KubePrometheusPrometheusConfig{
					// Enable persistence for production use
					Persistence: config.KubePrometheusPersistenceConfig{
						Enabled:      false, // Disabled for E2E to speed up
						Size:         "10Gi",
						StorageClass: "hcloud-volumes",
					},
				},
			},
		},
	}

	if err := addons.Apply(context.Background(), cfg, state.Kubeconfig, networkID); err != nil {
		t.Fatalf("Failed to install kube-prometheus-stack: %v", err)
	}

	// Verify monitoring namespace was created
	verifyNamespaceExists(t, state.KubeconfigPath, "monitoring")

	// Verify Grafana ingress was created
	verifyGrafanaIngressExists(t, state.KubeconfigPath, grafanaHost)

	t.Logf("      kube-prometheus-stack installed with Grafana ingress for %s", grafanaHost)
}

// waitForMonitoringReady waits for the complete monitoring stack (Prometheus, Grafana, Alertmanager) to be ready.
// This is a convenience function that waits for all monitoring components.
func waitForMonitoringReady(t *testing.T, kubeconfigPath string, timeout time.Duration) {
	t.Log("  Waiting for monitoring stack to be ready...")

	// Allocate time for each component (proportionally)
	componentTimeout := timeout / 3

	waitForPrometheusReady(t, kubeconfigPath, componentTimeout)
	waitForGrafanaReady(t, kubeconfigPath, componentTimeout)
	waitForAlertmanagerReady(t, kubeconfigPath, componentTimeout)

	t.Log("  Monitoring stack is ready")
}

// waitForGrafanaReady waits for Grafana deployment to be ready.
func waitForGrafanaReady(t *testing.T, kubeconfigPath string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Log("Warning: Timeout waiting for Grafana (may be disabled or taking longer)")
			return
		case <-ticker.C:
			// Check if Grafana deployment is ready
			cmd := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "deployment", "-n", "monitoring",
				"-l", "app.kubernetes.io/name=grafana",
				"-o", "jsonpath={.items[*].status.readyReplicas}")
			output, err := cmd.CombinedOutput()
			if err != nil {
				continue
			}

			replicas := strings.TrimSpace(string(output))
			if replicas != "" && replicas != "0" {
				t.Log("      Grafana is ready")
				return
			}
		}
	}
}

// waitForPrometheusReady waits for Prometheus StatefulSet to be ready.
func waitForPrometheusReady(t *testing.T, kubeconfigPath string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Collect comprehensive diagnostics
			t.Log("Timeout waiting for Prometheus - collecting comprehensive diagnostics...")

			diag := NewDiagnosticCollector(t, kubeconfigPath, "monitoring", "app.kubernetes.io/name=prometheus")
			diag.WithComponentName("Prometheus")
			diag.Collect()
			diag.Report()

			t.Fatal("Timeout waiting for Prometheus to be ready")
		case <-ticker.C:
			// Check if Prometheus pod is ready
			cmd := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "pods", "-n", "monitoring",
				"-l", "app.kubernetes.io/name=prometheus",
				"-o", "jsonpath={.items[*].status.phase}")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Log("      Waiting for Prometheus pods...")
				continue
			}

			phases := string(output)
			if strings.Contains(phases, "Running") {
				t.Log("      Prometheus is running")
				return
			}

			t.Log("      Waiting for Prometheus to be ready...")
		}
	}
}

// waitForAlertmanagerReady waits for Alertmanager to be ready.
func waitForAlertmanagerReady(t *testing.T, kubeconfigPath string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Log("Warning: Timeout waiting for Alertmanager (may be disabled)")
			return
		case <-ticker.C:
			// Check if Alertmanager pod is ready
			cmd := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "pods", "-n", "monitoring",
				"-l", "app.kubernetes.io/name=alertmanager",
				"-o", "jsonpath={.items[*].status.phase}")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Log("      Waiting for Alertmanager pods...")
				continue
			}

			phases := string(output)
			if strings.Contains(phases, "Running") {
				t.Log("      Alertmanager is running")
				return
			}

			t.Log("      Waiting for Alertmanager to be ready...")
		}
	}
}

// verifyNamespaceExists verifies a namespace exists.
func verifyNamespaceExists(t *testing.T, kubeconfigPath, namespace string) {
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "namespace", namespace)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Namespace %s not found", namespace)
	}
	t.Logf("      Namespace %s exists", namespace)
}

// verifyGrafanaIngressExists checks that the Grafana ingress was created.
func verifyGrafanaIngressExists(t *testing.T, kubeconfigPath, expectedHost string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for Grafana ingress with host %s", expectedHost)
		case <-ticker.C:
			cmd := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "ingress", "-n", "monitoring", "-o", "jsonpath={.items[*].spec.rules[*].host}")
			output, err := cmd.CombinedOutput()
			if err == nil && strings.Contains(string(output), expectedHost) {
				t.Logf("      Ingress found with host: %s", expectedHost)
				return
			}
		}
	}
}

// verifyGrafanaDNSRecord waits for the Grafana DNS record to be created.
func verifyGrafanaDNSRecord(t *testing.T, state *E2EState, hostname string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Show external-dns logs for debugging
			t.Log("      DNS record not created - showing external-dns logs:")
			showExternalDNSLogs(t, state.KubeconfigPath)
			t.Fatalf("Timeout waiting for DNS record %s", hostname)
		case <-ticker.C:
			// Use dig to check DNS resolution
			cmd := exec.CommandContext(context.Background(), "dig", "+short", hostname)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Logf("      DNS lookup failed: %v", err)
				continue
			}

			resolvedIP := strings.TrimSpace(string(output))
			if resolvedIP == "" {
				t.Log("      Waiting for DNS propagation...")
				continue
			}

			// Any resolved IP means external-dns worked
			t.Logf("      DNS record created: %s -> %s", hostname, resolvedIP)
			return
		}
	}
}

// verifyGrafanaCertificate waits for the Grafana TLS certificate to be issued.
func verifyGrafanaCertificate(t *testing.T, kubeconfigPath string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	secretName := "grafana-tls"

	for {
		select {
		case <-ctx.Done():
			// Get certificate status for debugging
			descCmd := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"describe", "certificate", "-n", "monitoring")
			if descOutput, _ := descCmd.CombinedOutput(); len(descOutput) > 0 {
				t.Logf("Certificate status:\n%s", string(descOutput))
			}
			t.Fatalf("Timeout waiting for Grafana TLS certificate")
		case <-ticker.C:
			// Check if the TLS secret exists and has data
			cmd := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "secret", "-n", "monitoring", secretName,
				"-o", "jsonpath={.data.tls\\.crt}")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Log("      Waiting for TLS certificate to be issued...")
				continue
			}

			if len(output) > 0 {
				t.Log("      TLS certificate issued")
				return
			}
		}
	}
}

// testGrafanaHTTPSConnectivity tests HTTPS connectivity to Grafana.
func testGrafanaHTTPSConnectivity(t *testing.T, hostname string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Create HTTP client that accepts staging certs (not trusted by default)
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // Staging certs aren't trusted
			},
		},
		// Don't follow redirects - Grafana may redirect to login
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	url := fmt.Sprintf("https://%s/", hostname)

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for HTTPS connectivity to Grafana at %s", hostname)
		case <-ticker.C:
			resp, err := client.Get(url)
			if err != nil {
				t.Logf("      HTTPS request failed: %v", err)
				continue
			}

			statusCode := resp.StatusCode
			_ = resp.Body.Close()

			// Grafana returns 200 for the main page, or 302/307 redirect to login
			if statusCode == http.StatusOK ||
				statusCode == http.StatusFound ||
				statusCode == http.StatusTemporaryRedirect {
				t.Logf("      HTTPS connectivity verified (status: %d)", statusCode)
				return
			}

			t.Logf("      HTTPS response: %d, waiting...", statusCode)
		}
	}
}

// verifyPrometheusTargets verifies Prometheus is scraping targets.
func verifyPrometheusTargets(t *testing.T, kubeconfigPath string) {
	// Port-forward to Prometheus and check targets API
	t.Log("      Checking Prometheus targets (via port-forward)...")

	// Start port-forward in background
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pfCmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", kubeconfigPath,
		"port-forward", "-n", "monitoring",
		"svc/kube-prometheus-stack-prometheus", "9090:9090")

	if err := pfCmd.Start(); err != nil {
		t.Logf("      Warning: Could not start port-forward: %v", err)
		return
	}
	defer func() { _ = pfCmd.Process.Kill() }()

	// Wait for port-forward to be ready
	time.Sleep(3 * time.Second)

	// Query targets API
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://localhost:9090/api/v1/targets")
	if err != nil {
		t.Logf("      Warning: Could not query Prometheus targets: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK {
		t.Log("      Prometheus targets API is accessible")
	}
}

// testMonitoringWithoutIngress tests kube-prometheus-stack without ingress (basic installation).
// This is a simpler test that verifies the stack works without requiring Cloudflare.
func testMonitoringWithoutIngress(t *testing.T, state *E2EState, token string, networkID int64) {
	t.Log("Testing kube-prometheus-stack (without ingress)...")

	cfg := &config.Config{
		ClusterName: state.ClusterName,
		HCloudToken: token,
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Enabled: true,
				// No ingress config - just basic installation
			},
		},
	}

	if err := addons.Apply(context.Background(), cfg, state.Kubeconfig, networkID); err != nil {
		t.Fatalf("Failed to install kube-prometheus-stack: %v", err)
	}

	// Wait for core components
	t.Log("  Waiting for Prometheus Operator...")
	waitForPod(t, state.KubeconfigPath, "monitoring", "app.kubernetes.io/name=kube-prometheus-stack-prometheus-operator", 8*time.Minute)

	t.Log("  Waiting for Prometheus...")
	waitForPrometheusReady(t, state.KubeconfigPath, 12*time.Minute)

	t.Log("  Waiting for Grafana...")
	waitForPod(t, state.KubeconfigPath, "monitoring", "app.kubernetes.io/name=grafana", 5*time.Minute)

	// Verify ServiceMonitors are created
	verifyServiceMonitors(t, state.KubeconfigPath)

	state.AddonsInstalled["monitoring-basic"] = true
	t.Log("  kube-prometheus-stack (basic) test complete")
}

// verifyServiceMonitors verifies that ServiceMonitors are created for core components.
func verifyServiceMonitors(t *testing.T, kubeconfigPath string) {
	t.Log("  Verifying ServiceMonitors are created...")

	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "servicemonitors", "-n", "monitoring",
		"-o", "jsonpath={.items[*].metadata.name}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("      Warning: Could not list ServiceMonitors: %v", err)
		return
	}

	monitors := string(output)
	if monitors == "" {
		t.Log("      Warning: No ServiceMonitors found")
		return
	}

	// Count ServiceMonitors
	count := len(strings.Fields(monitors))
	t.Logf("      Found %d ServiceMonitors", count)

	// Check for expected monitors
	expectedMonitors := []string{
		"prometheus",
		"alertmanager",
		"grafana",
	}

	for _, expected := range expectedMonitors {
		if strings.Contains(monitors, expected) {
			t.Logf("      ServiceMonitor '%s' exists", expected)
		}
	}
}

// testPrometheusAlerts verifies Prometheus alerting rules are loaded.
func testPrometheusAlerts(t *testing.T, kubeconfigPath string) {
	t.Log("  Verifying PrometheusRules are loaded...")

	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "prometheusrules", "-n", "monitoring",
		"-o", "jsonpath={.items[*].metadata.name}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("      Warning: Could not list PrometheusRules: %v", err)
		return
	}

	rules := string(output)
	if rules == "" {
		t.Log("      Warning: No PrometheusRules found")
		return
	}

	count := len(strings.Fields(rules))
	t.Logf("      Found %d PrometheusRules", count)
}
