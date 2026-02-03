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

	"github.com/stretchr/testify/require"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
)

// TestE2EDevCluster runs the full E2E test for a dev cluster configuration.
//
// Dev Mode Configuration:
//   - 1 Control Plane node (cpx22)
//   - 1 Worker node (cpx22)
//   - Shared Load Balancer (API + Ingress on same LB)
//   - All standard addons via operator
//
// This test uses the operator-centric pattern for deployment and verification.
//
// Environment variables:
//
//	HCLOUD_TOKEN - Required
//
// Example:
//
//	HCLOUD_TOKEN=xxx go test -v -timeout=45m -tags=e2e -run TestE2EDevCluster ./tests/e2e/
func TestE2EDevCluster(t *testing.T) {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping E2E test")
	}

	clusterName := fmt.Sprintf("e2e-dev-%d", time.Now().Unix())
	t.Logf("=== Starting Dev Cluster E2E Test: %s ===", clusterName)

	// Create configuration
	configPath := CreateTestConfig(t, clusterName, ModeDev, WithWorkers(1))
	defer os.Remove(configPath)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	// Create cluster via operator
	var state *OperatorTestContext
	var err error

	t.Run("Create", func(t *testing.T) {
		state, err = CreateClusterViaOperator(ctx, t, configPath)
		require.NoError(t, err, "Cluster creation should succeed")
	})

	// Ensure cleanup
	defer func() {
		if state != nil {
			DestroyCluster(context.Background(), t, state)
		}
	}()

	// Wait for cluster to be fully ready
	t.Run("WaitForClusterReady", func(t *testing.T) {
		err := WaitForClusterReady(ctx, t, state, 30*time.Minute)
		require.NoError(t, err, "Cluster should become ready")
	})

	t.Log("=== VERIFICATION CHECKLIST ===")

	// Verify cluster health via CRD
	t.Run("VerifyClusterHealth", func(t *testing.T) {
		VerifyClusterHealth(t, state)
	})

	// Verify infrastructure via CRD status
	t.Run("VerifyInfrastructure", func(t *testing.T) {
		cluster := GetClusterStatus(ctx, state)
		require.NotNil(t, cluster)
		require.GreaterOrEqual(t, cluster.Status.ControlPlanes.Ready, 1, "should have 1 CP")
		require.GreaterOrEqual(t, cluster.Status.Workers.Ready, 1, "should have 1 worker")
	})

	// Verify Kubernetes nodes
	t.Run("VerifyKubernetesNodes", func(t *testing.T) {
		nodeCount := CountKubernetesNodesViaKubectl(t, state.KubeconfigPath)
		require.GreaterOrEqual(t, nodeCount, 2, "should have at least 2 nodes (1 CP + 1 worker)")
	})

	// Verify core addons
	t.Run("VerifyCoreAddons", func(t *testing.T) {
		cluster := GetClusterStatus(ctx, state)
		require.NotNil(t, cluster)

		// Check Cilium
		cilium, ok := cluster.Status.Addons[k8znerv1alpha1.AddonNameCilium]
		require.True(t, ok && cilium.Installed, "Cilium should be installed")

		// Check CCM
		ccm, ok := cluster.Status.Addons[k8znerv1alpha1.AddonNameCCM]
		require.True(t, ok && ccm.Installed, "CCM should be installed")

		// Check CSI
		csi, ok := cluster.Status.Addons[k8znerv1alpha1.AddonNameCSI]
		require.True(t, ok && csi.Installed, "CSI should be installed")
	})

	// Functional tests
	t.Run("FunctionalTests", func(t *testing.T) {
		RunFunctionalTests(t, state)
	})

	t.Log("=== DEV CLUSTER E2E TEST PASSED ===")
}

// TestE2EDevClusterWithArgoCD runs a dev cluster E2E test with ArgoCD dashboard verification.
//
// This test specifically validates the ArgoCD dashboard is accessible via HTTPS
// with automatic DNS (via external-dns) and TLS certificate (via cert-manager).
//
// Dev Mode Configuration:
//   - 1 Control Plane node (cpx22)
//   - 1 Worker node (cpx22)
//   - Shared Load Balancer
//   - Domain configured for DNS/TLS
//   - ArgoCD with ingress enabled
//
// Environment variables:
//
//	HCLOUD_TOKEN - Required
//	CF_API_TOKEN - Required (Cloudflare API token)
//	CF_DOMAIN - Required (e.g., k8zner.org)
//
// Example:
//
//	HCLOUD_TOKEN=xxx CF_API_TOKEN=yyy CF_DOMAIN=k8zner.org \
//	go test -v -timeout=45m -tags=e2e -run TestE2EDevClusterWithArgoCD ./tests/e2e/
func TestE2EDevClusterWithArgoCD(t *testing.T) {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping E2E test")
	}

	cfAPIToken := os.Getenv("CF_API_TOKEN")
	cfDomain := os.Getenv("CF_DOMAIN")
	if cfAPIToken == "" || cfDomain == "" {
		t.Skip("CF_API_TOKEN and CF_DOMAIN required for ArgoCD dashboard test")
	}

	// Generate unique cluster name and ArgoCD subdomain for this test run
	timestamp := time.Now().Unix()
	clusterName := fmt.Sprintf("e2e-argo-%d", timestamp)
	argoSubdomain := fmt.Sprintf("argo-%d", timestamp)
	argoHost := fmt.Sprintf("%s.%s", argoSubdomain, cfDomain)
	t.Logf("=== Starting Dev Cluster with ArgoCD Dashboard E2E Test: %s ===", clusterName)
	t.Logf("=== ArgoCD will be accessible at: https://%s ===", argoHost)

	// Create configuration with domain
	configPath := CreateTestConfig(t, clusterName, ModeDev,
		WithWorkers(1),
		WithDomain(cfDomain),
		WithArgoSubdomain(argoSubdomain),
	)
	defer os.Remove(configPath)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	// Create cluster via operator
	var state *OperatorTestContext
	var err error

	t.Run("Create", func(t *testing.T) {
		state, err = CreateClusterViaOperator(ctx, t, configPath)
		require.NoError(t, err, "Cluster creation should succeed")
	})

	// Ensure cleanup
	defer func() {
		if state != nil {
			DestroyCluster(context.Background(), t, state)
		}
	}()

	// Wait for cluster to be fully ready
	t.Run("WaitForClusterReady", func(t *testing.T) {
		err := WaitForClusterReady(ctx, t, state, 30*time.Minute)
		require.NoError(t, err, "Cluster should become ready")
	})

	t.Log("=== VERIFICATION CHECKLIST ===")

	// Verify cluster health via CRD
	t.Run("VerifyClusterHealth", func(t *testing.T) {
		VerifyClusterHealth(t, state)
	})

	// Verify core addons
	t.Run("VerifyCoreAddons", func(t *testing.T) {
		cluster := GetClusterStatus(ctx, state)
		require.NotNil(t, cluster)

		// Check essential addons
		cilium, ok := cluster.Status.Addons[k8znerv1alpha1.AddonNameCilium]
		require.True(t, ok && cilium.Installed, "Cilium should be installed")

		ccm, ok := cluster.Status.Addons[k8znerv1alpha1.AddonNameCCM]
		require.True(t, ok && ccm.Installed, "CCM should be installed")
	})

	// === ARGOCD DASHBOARD E2E TEST ===
	t.Log("=== ARGOCD DASHBOARD E2E TEST ===")

	// Get worker IPs for DNS verification
	legacyState := state.ToE2EState()

	t.Run("ArgoCDDashboard", func(t *testing.T) {
		// Verify ArgoCD ingress is configured
		t.Log("  Step 1: Verifying ArgoCD ingress configuration...")
		verifyArgoCDIngressConfigured(t, state.KubeconfigPath, argoHost)

		// Wait for DNS record creation
		t.Logf("  Step 2: Waiting for DNS record creation (expected IPs: %v)...", legacyState.WorkerIPs)
		waitForDNSRecord(t, argoHost, 8*time.Minute, legacyState.WorkerIPs...)

		// Wait for TLS certificate issuance
		t.Log("  Step 3: Waiting for TLS certificate issuance...")
		waitForArgoCDTLSCertificate(t, state.KubeconfigPath, 8*time.Minute)

		// Test HTTPS connectivity to ArgoCD
		t.Log("  Step 4: Testing HTTPS connectivity to ArgoCD dashboard...")
		testArgoCDHTTPSAccess(t, argoHost, 3*time.Minute)

		t.Logf("  ArgoCD Dashboard accessible at https://%s", argoHost)
	})

	t.Log("=== DEV CLUSTER WITH ARGOCD E2E TEST PASSED ===")
}

// TestE2EHACluster runs the full E2E test for an HA cluster configuration.
//
// HA Mode Configuration:
//   - 3 Control Plane nodes (cpx22)
//   - 2 Worker nodes (cpx22)
//   - Separate Load Balancers for API and Ingress
//   - All standard addons via operator
//
// This test uses the operator-centric pattern for deployment and verification.
//
// Environment variables:
//
//	HCLOUD_TOKEN - Required
//
// Example:
//
//	HCLOUD_TOKEN=xxx go test -v -timeout=60m -tags=e2e -run TestE2EHACluster ./tests/e2e/
func TestE2EHACluster(t *testing.T) {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping E2E test")
	}

	clusterName := fmt.Sprintf("e2e-ha-%d", time.Now().Unix())
	t.Logf("=== Starting HA Cluster E2E Test: %s ===", clusterName)

	// Create HA configuration
	configPath := CreateTestConfig(t, clusterName, ModeHA, WithWorkers(2))
	defer os.Remove(configPath)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	// Create cluster via operator
	var state *OperatorTestContext
	var err error

	t.Run("Create", func(t *testing.T) {
		state, err = CreateClusterViaOperator(ctx, t, configPath)
		require.NoError(t, err, "Cluster creation should succeed")
	})

	// Ensure cleanup
	defer func() {
		if state != nil {
			DestroyCluster(context.Background(), t, state)
		}
	}()

	// Wait for cluster to be fully ready
	t.Run("WaitForClusterReady", func(t *testing.T) {
		err := WaitForClusterReady(ctx, t, state, 35*time.Minute)
		require.NoError(t, err, "Cluster should become ready")
	})

	t.Log("=== VERIFICATION CHECKLIST ===")

	// Verify cluster health via CRD
	t.Run("VerifyClusterHealth", func(t *testing.T) {
		VerifyClusterHealth(t, state)
	})

	// Verify HA infrastructure via CRD status
	t.Run("VerifyHAInfrastructure", func(t *testing.T) {
		cluster := GetClusterStatus(ctx, state)
		require.NotNil(t, cluster)
		require.GreaterOrEqual(t, cluster.Status.ControlPlanes.Ready, 3, "should have 3 CPs for HA")
		require.GreaterOrEqual(t, cluster.Status.Workers.Ready, 2, "should have 2 workers")
	})

	// Verify Kubernetes nodes
	t.Run("VerifyKubernetesNodes", func(t *testing.T) {
		nodeCount := CountKubernetesNodesViaKubectl(t, state.KubeconfigPath)
		require.GreaterOrEqual(t, nodeCount, 5, "should have at least 5 nodes (3 CP + 2 workers)")
	})

	// Verify core addons
	t.Run("VerifyCoreAddons", func(t *testing.T) {
		cluster := GetClusterStatus(ctx, state)
		require.NotNil(t, cluster)

		// Check all core addons
		cilium, ok := cluster.Status.Addons[k8znerv1alpha1.AddonNameCilium]
		require.True(t, ok && cilium.Installed, "Cilium should be installed")

		ccm, ok := cluster.Status.Addons[k8znerv1alpha1.AddonNameCCM]
		require.True(t, ok && ccm.Installed, "CCM should be installed")

		csi, ok := cluster.Status.Addons[k8znerv1alpha1.AddonNameCSI]
		require.True(t, ok && csi.Installed, "CSI should be installed")
	})

	// Functional tests
	t.Run("FunctionalTests", func(t *testing.T) {
		RunFunctionalTests(t, state)
	})

	// HA-specific checks
	t.Run("HASpecificChecks", func(t *testing.T) {
		legacyState := state.ToE2EState()

		// Check all 3 control planes are accessible
		for i, ip := range legacyState.ControlPlaneIPs {
			if err := quickPortCheck(ip, 6443); err != nil {
				t.Errorf("Control plane %d (%s) API not accessible: %v", i+1, ip, err)
			} else {
				t.Logf("Control plane %d (%s) API accessible", i+1, ip)
			}
		}
	})

	t.Log("=== HA CLUSTER E2E TEST PASSED ===")
}

// verifyArgoCDIngressConfigured verifies the ArgoCD ingress is properly configured.
func verifyArgoCDIngressConfigured(t *testing.T, kubeconfigPath, expectedHost string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Show ingress details for debugging
			descCmd := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "ingress", "-n", "argocd", "-o", "yaml")
			if output, _ := descCmd.CombinedOutput(); len(output) > 0 {
				t.Logf("ArgoCD ingress YAML:\n%s", string(output))
			}
			t.Fatalf("Timeout waiting for ArgoCD ingress with host %s", expectedHost)
		case <-ticker.C:
			cmd := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "ingress", "-n", "argocd", "-o", "jsonpath={.items[*].spec.rules[*].host}")
			output, err := cmd.CombinedOutput()
			if err == nil && strings.Contains(string(output), expectedHost) {
				t.Logf("  ArgoCD ingress configured with host: %s", expectedHost)
				return
			}
		}
	}
}

// waitForDNSRecord waits for DNS record to be created and resolvable.
func waitForDNSRecord(t *testing.T, hostname string, timeout time.Duration, expectedIPs ...string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	expectedIPMap := make(map[string]bool)
	for _, ip := range expectedIPs {
		expectedIPMap[ip] = true
	}

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for DNS record %s (expected IPs: %v)", hostname, expectedIPs)
		case <-ticker.C:
			cmd := exec.CommandContext(context.Background(), "dig", "+short", hostname)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Logf("  DNS lookup failed: %v", err)
				continue
			}

			resolvedIP := strings.TrimSpace(string(output))
			if resolvedIP == "" {
				t.Log("  Waiting for DNS propagation...")
				continue
			}

			if len(expectedIPs) > 0 {
				if !expectedIPMap[resolvedIP] {
					t.Logf("  Waiting for DNS update (current: %s, expected: %v)...", resolvedIP, expectedIPs)
					continue
				}
			}

			t.Logf("  DNS record created: %s -> %s", hostname, resolvedIP)
			return
		}
	}
}

// waitForArgoCDTLSCertificate waits for the ArgoCD TLS certificate to be issued.
func waitForArgoCDTLSCertificate(t *testing.T, kubeconfigPath string, timeout time.Duration) {
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
			if output, _ := descCmd.CombinedOutput(); len(output) > 0 {
				t.Logf("Certificate status:\n%s", string(output))
			}
			t.Fatalf("Timeout waiting for ArgoCD TLS certificate")
		case <-ticker.C:
			cmd := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "secret", "-n", "argocd", secretName,
				"-o", "jsonpath={.data.tls\\.crt}")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Log("  Waiting for TLS certificate to be issued...")
				continue
			}

			if len(output) > 0 {
				t.Log("  TLS certificate issued (staging)")
				return
			}
		}
	}
}

// testArgoCDHTTPSAccess tests HTTPS connectivity to ArgoCD dashboard.
func testArgoCDHTTPSAccess(t *testing.T, hostname string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
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
			resp, err := httpClient.Get(url)
			if err != nil {
				t.Logf("  HTTPS request failed: %v", err)
				continue
			}
			resp.Body.Close()

			if resp.StatusCode == http.StatusOK ||
				resp.StatusCode == http.StatusFound ||
				resp.StatusCode == http.StatusTemporaryRedirect {
				t.Logf("  HTTPS connectivity verified (status: %d)", resp.StatusCode)
				return
			}

			t.Logf("  HTTPS response: %d, waiting...", resp.StatusCode)
		}
	}
}
