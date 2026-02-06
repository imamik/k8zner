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

// testArgoCDDashboard verifies the ArgoCD dashboard is accessible via HTTPS
// with automatic DNS and TLS certificate provisioning.
//
// This test:
// 1. Verifies ArgoCD ingress is configured
// 2. Waits for DNS record creation via external-dns
// 3. Waits for TLS certificate issuance via cert-manager
// 4. Tests HTTPS connectivity to the ArgoCD login page
//
// Environment variables required:
//   - CF_API_TOKEN: Cloudflare API token
//   - CF_DOMAIN: Domain managed by Cloudflare (e.g., k8zner.org)
//   - CF_ARGO_SUBDOMAIN: (optional) Subdomain for ArgoCD (default: "argo")
func testArgoCDDashboard(t *testing.T, state *E2EState, token string) {
	cfAPIToken := os.Getenv("CF_API_TOKEN")
	cfDomain := os.Getenv("CF_DOMAIN")

	if cfAPIToken == "" || cfDomain == "" {
		t.Log("ArgoCD Dashboard test skipped (CF_API_TOKEN or CF_DOMAIN not set)")
		t.Log("  Set CF_API_TOKEN and CF_DOMAIN environment variables to test")
		return
	}

	// Get ArgoCD subdomain (default: "argo")
	argoSubdomain := os.Getenv("CF_ARGO_SUBDOMAIN")
	if argoSubdomain == "" {
		argoSubdomain = "argo"
	}

	argoHost := fmt.Sprintf("%s.%s", argoSubdomain, cfDomain)
	t.Logf("Testing ArgoCD Dashboard at: %s", argoHost)

	ctx := context.Background()

	// Get network ID for addon installation
	network, _ := state.Client.GetNetwork(ctx, state.ClusterName)
	networkID := int64(0)
	if network != nil {
		networkID = network.ID
	}

	// Step 1: Install Cloudflare integration (external-dns + cert-manager DNS01)
	// This may already be installed, but we ensure it's configured
	t.Log("  Step 1: Ensuring Cloudflare integration is installed...")
	installCloudflareForArgoCD(t, state, token, cfAPIToken, cfDomain, networkID)

	// Step 2: Update ArgoCD with ingress configuration
	t.Logf("  Step 2: Configuring ArgoCD ingress for %s...", argoHost)
	configureArgoCDIngress(t, state, token, cfDomain, argoSubdomain, networkID)

	// Step 3: Wait for and verify DNS record
	t.Log("  Step 3: Waiting for DNS record creation...")
	verifyArgoCDDNSRecord(t, state, argoHost, 6*time.Minute)

	// Step 4: Wait for and verify TLS certificate
	t.Log("  Step 4: Waiting for TLS certificate issuance...")
	verifyArgoCDCertificate(t, state.KubeconfigPath, 6*time.Minute)

	// Step 5: Test HTTPS connectivity to ArgoCD
	t.Log("  Step 5: Testing HTTPS connectivity to ArgoCD...")
	testArgoCDHTTPSConnectivity(t, argoHost, 3*time.Minute)

	t.Logf("  ArgoCD Dashboard accessible at https://%s", argoHost)
	state.AddonsInstalled["argocd-dashboard"] = true
	t.Log("  ArgoCD Dashboard test complete")
}

// installCloudflareForArgoCD ensures Cloudflare integration is installed.
func installCloudflareForArgoCD(t *testing.T, state *E2EState, hcloudToken, cfAPIToken, cfDomain string, networkID int64) {
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
					Email:      fmt.Sprintf("argocd-test@%s", cfDomain),
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

	t.Log("      Cloudflare integration ready")
}

// configureArgoCDIngress updates ArgoCD with ingress configuration.
func configureArgoCDIngress(t *testing.T, state *E2EState, hcloudToken, cfDomain, argoSubdomain string, networkID int64) {
	argoHost := fmt.Sprintf("%s.%s", argoSubdomain, cfDomain)

	cfg := &config.Config{
		ClusterName: state.ClusterName,
		HCloudToken: hcloudToken,
		Addons: config.AddonsConfig{
			// Cloudflare must be enabled for proper ClusterIssuer selection
			Cloudflare: config.CloudflareConfig{
				Enabled: true,
				Domain:  cfDomain,
			},
			ExternalDNS: config.ExternalDNSConfig{
				Enabled: true,
			},
			CertManager: config.CertManagerConfig{
				Enabled: true,
				Cloudflare: config.CertManagerCloudflareConfig{
					Enabled:    true,
					Production: false, // Use staging
				},
			},
			ArgoCD: config.ArgoCDConfig{
				Enabled:          true,
				IngressEnabled:   true,
				IngressHost:      argoHost,
				IngressClassName: "traefik",
				IngressTLS:       true,
			},
		},
	}

	if err := addons.Apply(context.Background(), cfg, state.Kubeconfig, networkID); err != nil {
		t.Fatalf("Failed to configure ArgoCD ingress: %v", err)
	}

	// Wait for ArgoCD server to be ready
	waitForPod(t, state.KubeconfigPath, "argocd", "app.kubernetes.io/name=argocd-server", 5*time.Minute)

	// Verify ingress was created
	verifyArgoCDIngressExists(t, state.KubeconfigPath, argoHost)

	t.Logf("      ArgoCD ingress configured for %s", argoHost)
}

// verifyArgoCDIngressExists checks that the ArgoCD ingress was created.
func verifyArgoCDIngressExists(t *testing.T, kubeconfigPath, expectedHost string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for ArgoCD ingress with host %s", expectedHost)
		case <-ticker.C:
			cmd := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "ingress", "-n", "argocd", "-o", "jsonpath={.items[*].spec.rules[*].host}")
			output, err := cmd.CombinedOutput()
			if err == nil && strings.Contains(string(output), expectedHost) {
				t.Logf("      Ingress found with host: %s", expectedHost)
				return
			}
		}
	}
}

// verifyArgoCDDNSRecord waits for the DNS record to be created.
func verifyArgoCDDNSRecord(t *testing.T, state *E2EState, hostname string, timeout time.Duration) {
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

// verifyArgoCDCertificate waits for the TLS certificate to be issued.
func verifyArgoCDCertificate(t *testing.T, kubeconfigPath string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	secretName := "argocd-server-tls"

	for {
		select {
		case <-ctx.Done():
			// Get certificate status for debugging
			descCmd := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"describe", "certificate", "-n", "argocd")
			if descOutput, _ := descCmd.CombinedOutput(); len(descOutput) > 0 {
				t.Logf("Certificate status:\n%s", string(descOutput))
			}
			t.Fatalf("Timeout waiting for ArgoCD TLS certificate")
		case <-ticker.C:
			// Check if the TLS secret exists and has data
			cmd := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "secret", "-n", "argocd", secretName,
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

// testArgoCDHTTPSConnectivity tests HTTPS connectivity to ArgoCD.
func testArgoCDHTTPSConnectivity(t *testing.T, hostname string, timeout time.Duration) {
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
		// Don't follow redirects - ArgoCD may redirect to login
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	url := fmt.Sprintf("https://%s/", hostname)

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for HTTPS connectivity to ArgoCD at %s", hostname)
		case <-ticker.C:
			resp, err := client.Get(url)
			if err != nil {
				t.Logf("      HTTPS request failed: %v", err)
				continue
			}

			statusCode := resp.StatusCode
			_ = resp.Body.Close()

			// ArgoCD returns 200 for the main page, or 302/307 redirect to login
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
