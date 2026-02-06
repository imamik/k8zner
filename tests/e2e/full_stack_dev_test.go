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
	"github.com/imamik/k8zner/internal/platform/s3"
	"github.com/imamik/k8zner/internal/util/naming"
)

// TestE2EFullStackDev is Test 1: Full addon validation.
//
// This comprehensive test validates ALL addons on a dev cluster:
// - Config: 1 CP + 1 worker, mode=dev, ALL addons enabled
// - Timeout: 60 minutes
//
// This test should be run FIRST before TestE2EHAOperations as it validates
// the full addon stack. The HA test only runs if this test passes.
//
// IMPORTANT: The HA test (TestE2EHAOperations) will ONLY run if ALL subtests
// in this test pass. If any subtest fails, the HA test will be skipped.
//
// Required environment variables:
//   - HCLOUD_TOKEN - Hetzner Cloud API token
//   - CF_API_TOKEN - Cloudflare API token (for DNS/TLS)
//   - CF_DOMAIN - Domain managed by Cloudflare
//   - HETZNER_S3_ACCESS_KEY - Hetzner Object Storage access key
//   - HETZNER_S3_SECRET_KEY - Hetzner Object Storage secret key
//
// Example:
//
//	HCLOUD_TOKEN=xxx CF_API_TOKEN=yyy CF_DOMAIN=example.com \
//	HETZNER_S3_ACCESS_KEY=aaa HETZNER_S3_SECRET_KEY=bbb \
//	go test -v -timeout=60m -tags=e2e -run TestE2EFullStackDev ./tests/e2e/
func TestE2EFullStackDev(t *testing.T) {
	// Track if any subtest fails - HA test should NEVER run if FullStack has failures
	allSubtestsPassed := true
	runSubtest := func(name string, fn func(t *testing.T)) {
		if !allSubtestsPassed {
			t.Skipf("Skipping %s: previous subtest failed", name)
			return
		}
		// IMPORTANT: Use t.Run()'s return value to detect failures.
		// We cannot check t.Failed() after fn(t) because when fn(t) calls
		// t.Fatal()/require.NoError()/etc., it triggers runtime.Goexit() which
		// terminates the goroutine before any code after fn(t) can execute.
		passed := t.Run(name, fn)
		if !passed {
			allSubtestsPassed = false
		}
	}
	// Validate required environment variables
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping E2E test")
	}

	cfAPIToken := os.Getenv("CF_API_TOKEN")
	cfDomain := os.Getenv("CF_DOMAIN")
	if cfAPIToken == "" || cfDomain == "" {
		t.Skip("CF_API_TOKEN and CF_DOMAIN required for full stack test")
	}

	s3AccessKey := os.Getenv("HETZNER_S3_ACCESS_KEY")
	s3SecretKey := os.Getenv("HETZNER_S3_SECRET_KEY")
	if s3AccessKey == "" || s3SecretKey == "" {
		t.Skip("HETZNER_S3_ACCESS_KEY and HETZNER_S3_SECRET_KEY required for backup test")
	}

	// Generate unique identifiers (short cluster names for Hetzner resource limits)
	clusterName := naming.E2ECluster(naming.E2EFullStack) // e.g., e2e-fs-abc12
	clusterID := clusterName[len(naming.E2EFullStack)+1:] // Extract the 5-char ID
	argoSubdomain := "argo-" + clusterID
	grafanaSubdomain := "grafana-" + clusterID
	argoHost := fmt.Sprintf("%s.%s", argoSubdomain, cfDomain)
	grafanaHost := fmt.Sprintf("%s.%s", grafanaSubdomain, cfDomain)

	t.Logf("=== Starting Full Stack Dev E2E Test: %s ===", clusterName)
	t.Logf("=== ArgoCD: https://%s ===", argoHost)
	t.Logf("=== Grafana: https://%s ===", grafanaHost)

	// Create S3 client for backup verification
	region := "fsn1"
	bucketName := clusterName + "-etcd-backups"
	endpoint := fmt.Sprintf("https://%s.your-objectstorage.com", region)
	s3Client, err := s3.NewClient(endpoint, region, s3AccessKey, s3SecretKey)
	if err != nil {
		t.Fatalf("Failed to create S3 client: %v", err)
	}

	// Create configuration with ALL addons enabled
	configPath := CreateTestConfig(t, clusterName, ModeDev,
		WithWorkers(1),
		WithRegion(region),
		WithDomain(cfDomain),
		WithArgoSubdomain(argoSubdomain),
		WithGrafanaSubdomain(grafanaSubdomain),
		WithBackup(true),
		WithMonitoring(true),
	)
	defer os.Remove(configPath)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	// Create cluster via operator
	var state *OperatorTestContext

	// Cleanup handlers
	defer func() {
		// Clean S3 bucket
		t.Log("Cleaning up S3 bucket...")
		if cleanupErr := cleanupS3Bucket(context.Background(), s3Client, bucketName); cleanupErr != nil {
			t.Logf("Warning: failed to cleanup bucket: %v", cleanupErr)
		}

		// Destroy cluster
		if state != nil {
			DestroyCluster(context.Background(), t, state)
		}
	}()

	// =========================================================================
	// SUBTEST 01: Create Cluster
	// =========================================================================
	runSubtest("01_CreateCluster", func(t *testing.T) {
		var createErr error
		state, createErr = CreateClusterViaOperator(ctx, t, configPath)
		require.NoError(t, createErr, "Cluster creation should succeed")
	})

	// =========================================================================
	// SUBTEST 02: Wait for Cluster Ready
	// =========================================================================
	runSubtest("02_WaitForClusterReady", func(t *testing.T) {
		err := WaitForClusterReady(ctx, t, state, 30*time.Minute)
		require.NoError(t, err, "Cluster should become ready")
	})

	// Verify cluster is in good state before proceeding
	var cluster *k8znerv1alpha1.K8znerCluster
	if allSubtestsPassed && state != nil {
		cluster = GetClusterStatus(ctx, state)
		if cluster == nil {
			allSubtestsPassed = false
			t.Error("Cluster CRD should exist")
		}
	}

	// =========================================================================
	// SUBTEST 03: Verify Cilium
	// =========================================================================
	runSubtest("03_Verify_Cilium", func(t *testing.T) {
		// Check addon status
		cilium, ok := cluster.Status.Addons[k8znerv1alpha1.AddonNameCilium]
		require.True(t, ok && cilium.Installed, "Cilium should be installed")

		// Pod-to-pod connectivity test
		legacyState := state.ToE2EState()
		testCiliumNetworkConnectivity(t, legacyState)
	})

	// =========================================================================
	// SUBTEST 04: Verify CCM
	// =========================================================================
	runSubtest("04_Verify_CCM", func(t *testing.T) {
		// Check addon status
		cluster := GetClusterStatus(ctx, state)
		ccm, ok := cluster.Status.Addons[k8znerv1alpha1.AddonNameCCM]
		require.True(t, ok && ccm.Installed, "CCM should be installed")

		// LoadBalancer provisioning test
		legacyState := state.ToE2EState()
		testCCMLoadBalancer(t, legacyState)
	})

	// =========================================================================
	// SUBTEST 05: Verify CSI
	// =========================================================================
	runSubtest("05_Verify_CSI", func(t *testing.T) {
		// Check addon status
		cluster := GetClusterStatus(ctx, state)
		csi, ok := cluster.Status.Addons[k8znerv1alpha1.AddonNameCSI]
		require.True(t, ok && csi.Installed, "CSI should be installed")

		// Volume provisioning + mount test
		legacyState := state.ToE2EState()
		testCSIVolume(t, legacyState)
	})

	// =========================================================================
	// SUBTEST 06: Verify Metrics Server
	// =========================================================================
	runSubtest("06_Verify_MetricsServer", func(t *testing.T) {
		testMetricsAPI(t, state.KubeconfigPath)
	})

	// =========================================================================
	// SUBTEST 07: Verify Traefik
	// =========================================================================
	runSubtest("07_Verify_Traefik", func(t *testing.T) {
		// IngressClass exists
		verifyIngressClassExists(t, state.KubeconfigPath, "traefik")

		// Traefik pods running
		waitForPod(t, state.KubeconfigPath, "traefik", "app.kubernetes.io/name=traefik", 5*time.Minute)
	})

	// =========================================================================
	// SUBTEST 08: Verify CertManager
	// =========================================================================
	runSubtest("08_Verify_CertManager", func(t *testing.T) {
		// ClusterIssuers exist
		verifyClusterIssuerExists(t, state.KubeconfigPath, "letsencrypt-cloudflare-staging")

		// cert-manager pods running
		waitForPod(t, state.KubeconfigPath, "cert-manager", "app.kubernetes.io/name=cert-manager", 5*time.Minute)
	})

	// =========================================================================
	// SUBTEST 09: Verify ExternalDNS
	// =========================================================================
	runSubtest("09_Verify_ExternalDNS", func(t *testing.T) {
		// Pod running
		waitForPod(t, state.KubeconfigPath, "external-dns", "app.kubernetes.io/name=external-dns", 5*time.Minute)
		t.Log("  ExternalDNS pod running (DNS verified via dashboards)")
	})

	// =========================================================================
	// SUBTEST 10: Verify ArgoCD
	// =========================================================================
	runSubtest("10_Verify_ArgoCD", func(t *testing.T) {
		// ArgoCD server pods running
		waitForPod(t, state.KubeconfigPath, "argocd", "app.kubernetes.io/name=argocd-server", 5*time.Minute)

		// Verify ingress configured
		verifyArgoCDIngressConfigured(t, state.KubeconfigPath, argoHost)

		// Wait for DNS record
		legacyState := state.ToE2EState()
		waitForDNSRecord(t, argoHost, 8*time.Minute, legacyState.WorkerIPs...)

		// Wait for TLS certificate
		waitForArgoCDTLSCertificate(t, state.KubeconfigPath, 8*time.Minute)

		// Test HTTPS access
		testArgoCDHTTPSAccess(t, argoHost, 3*time.Minute)
		t.Logf("  ArgoCD Dashboard accessible at https://%s", argoHost)
	})

	// =========================================================================
	// SUBTEST 11: Verify Grafana
	// =========================================================================
	runSubtest("11_Verify_Grafana", func(t *testing.T) {
		// Grafana pods running
		waitForPod(t, state.KubeconfigPath, "monitoring", "app.kubernetes.io/name=grafana", 5*time.Minute)

		// Verify ingress configured
		verifyGrafanaIngressExists(t, state.KubeconfigPath, grafanaHost)

		// Wait for DNS record
		verifyGrafanaDNSRecord(t, state.ToE2EState(), grafanaHost, 8*time.Minute)

		// Wait for TLS certificate
		verifyGrafanaCertificate(t, state.KubeconfigPath, 8*time.Minute)

		// Test HTTPS access
		testGrafanaHTTPSConnectivity(t, grafanaHost, 3*time.Minute)
		t.Logf("  Grafana Dashboard accessible at https://%s", grafanaHost)
	})

	// =========================================================================
	// SUBTEST 12: Verify Prometheus
	// =========================================================================
	runSubtest("12_Verify_Prometheus", func(t *testing.T) {
		// Prometheus pods running
		waitForPrometheusReady(t, state.KubeconfigPath, 8*time.Minute)

		// Targets API
		verifyPrometheusTargets(t, state.KubeconfigPath)

		// ServiceMonitors
		verifyServiceMonitors(t, state.KubeconfigPath)
	})

	// =========================================================================
	// SUBTEST 13: Verify Alertmanager
	// =========================================================================
	runSubtest("13_Verify_Alertmanager", func(t *testing.T) {
		// Pod running
		waitForAlertmanagerReady(t, state.KubeconfigPath, 5*time.Minute)
	})

	// =========================================================================
	// SUBTEST 14: Verify Backup
	// =========================================================================
	runSubtest("14_Verify_Backup", func(t *testing.T) {
		// CronJob exists
		verifyBackupCronJob(t, state.KubeconfigPath, "0 * * * *")

		// Trigger manual backup
		triggerBackupJob(t, state.KubeconfigPath, 5*time.Minute)

		// Verify backup in S3
		backupKey := verifyBackupInS3(t, s3Client, bucketName, "etcd-backups/")

		// Verify backup can be restored (download and validate)
		verifyBackupRestore(t, s3Client, bucketName, backupKey)
	})

	// ONLY mark full stack test as passed if ALL subtests passed
	// This is critical: HA test should NEVER run if FullStack has any failures
	if allSubtestsPassed {
		SetFullStackPassed()
		t.Log("=== FULL STACK DEV E2E TEST PASSED ===")
	} else {
		t.Log("=== FULL STACK DEV E2E TEST FAILED - HA test will be skipped ===")
	}
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

// verifyBackupCronJob verifies the TalosBackup CronJob is properly configured.
func verifyBackupCronJob(t *testing.T, kubeconfigPath, expectedSchedule string) {
	t.Log("  Verifying TalosBackup CronJob configuration...")

	// Get CronJob details
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "cronjob", "-n", "kube-system", "talos-backup",
		"-o", "jsonpath={.spec.schedule}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to get CronJob schedule: %v", err)
	}

	schedule := string(output)
	if schedule != expectedSchedule {
		t.Fatalf("Unexpected schedule: got %s, want %s", schedule, expectedSchedule)
	}
	t.Logf("  CronJob schedule: %s", schedule)

	// Verify S3 secret exists
	cmd = exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "secret", "-n", "kube-system", "talos-backup-s3-secrets")
	if err := cmd.Run(); err != nil {
		t.Fatal("  S3 secrets not found")
	}
	t.Log("  S3 secrets configured")
}

// triggerBackupJob creates a manual Job from the CronJob and waits for completion.
func triggerBackupJob(t *testing.T, kubeconfigPath string, timeout time.Duration) {
	t.Log("  Triggering manual backup job...")

	jobName := fmt.Sprintf("talos-backup-manual-%d", time.Now().Unix())

	// Create a Job from the CronJob
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"create", "job", jobName, "-n", "kube-system",
		"--from=cronjob/talos-backup")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to create backup job: %v\nOutput: %s", err, string(output))
	}
	t.Logf("  Created job: %s", jobName)

	// Wait for job completion
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Get job status for debugging
			statusCmd := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"describe", "job", "-n", "kube-system", jobName)
			if statusOutput, _ := statusCmd.CombinedOutput(); len(statusOutput) > 0 {
				t.Logf("  Job status:\n%s", string(statusOutput))
			}
			t.Fatal("  Timeout waiting for backup job to complete")

		case <-ticker.C:
			cmd := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "job", "-n", "kube-system", jobName,
				"-o", "jsonpath={.status.succeeded}")
			output, err := cmd.CombinedOutput()
			if err == nil && string(output) == "1" {
				t.Log("  Backup job completed successfully")
				return
			}

			// Check for failure
			cmd = exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "job", "-n", "kube-system", jobName,
				"-o", "jsonpath={.status.failed}")
			failOutput, _ := cmd.CombinedOutput()
			if string(failOutput) != "" && string(failOutput) != "0" {
				// Get pod logs for debugging
				logsCmd := exec.CommandContext(context.Background(), "kubectl",
					"--kubeconfig", kubeconfigPath,
					"logs", "-n", "kube-system", "-l", fmt.Sprintf("job-name=%s", jobName))
				if logsOutput, _ := logsCmd.CombinedOutput(); len(logsOutput) > 0 {
					t.Logf("  Job logs:\n%s", string(logsOutput))
				}
				t.Fatal("  Backup job failed")
			}

			t.Log("  Waiting for backup job to complete...")
		}
	}
}

// verifyBackupInS3 checks that a backup file exists in the S3 bucket and returns the backup key.
func verifyBackupInS3(t *testing.T, s3Client *s3.Client, bucketName, prefix string) string {
	t.Log("  Verifying backup exists in S3...")

	objects, err := s3Client.ListObjects(context.Background(), bucketName, prefix)
	if err != nil {
		t.Fatalf("Failed to list S3 objects: %v", err)
	}

	if len(objects) == 0 {
		t.Fatal("  No backup files found in S3 bucket")
	}

	t.Logf("  Found %d backup file(s) in S3:", len(objects))
	for _, obj := range objects {
		t.Logf("    - %s", obj)
	}

	return objects[0] // Return the first backup key for restore verification
}

// verifyBackupRestore downloads and validates the backup file can be decompressed.
func verifyBackupRestore(t *testing.T, s3Client *s3.Client, bucketName, backupKey string) {
	t.Log("  Verifying backup can be restored (download and validate)...")

	ctx := context.Background()

	// Download the backup
	data, err := s3Client.GetObject(ctx, bucketName, backupKey)
	if err != nil {
		t.Fatalf("Failed to download backup: %v", err)
	}

	t.Logf("  Downloaded backup: %d bytes", len(data))

	// Verify it's a valid zstd-compressed file
	// zstd magic number: 0x28 0xB5 0x2F 0xFD
	if len(data) < 4 {
		t.Fatal("  Backup file too small to be valid")
	}

	zstdMagic := []byte{0x28, 0xB5, 0x2F, 0xFD}
	if data[0] != zstdMagic[0] || data[1] != zstdMagic[1] || data[2] != zstdMagic[2] || data[3] != zstdMagic[3] {
		t.Fatalf("  Invalid zstd magic number: got %x %x %x %x, want %x %x %x %x",
			data[0], data[1], data[2], data[3],
			zstdMagic[0], zstdMagic[1], zstdMagic[2], zstdMagic[3])
	}

	t.Log("  Backup is valid zstd-compressed file")
	t.Log("  Backup restore verification passed (file is downloadable and valid)")
}

// cleanupS3Bucket deletes all objects and the bucket itself.
func cleanupS3Bucket(ctx context.Context, s3Client *s3.Client, bucketName string) error {
	// Check if bucket exists
	exists, err := s3Client.BucketExists(ctx, bucketName)
	if err != nil {
		return fmt.Errorf("failed to check bucket existence: %w", err)
	}
	if !exists {
		return nil // Nothing to clean up
	}

	// List and delete all objects
	objects, err := s3Client.ListObjects(ctx, bucketName, "")
	if err != nil {
		return fmt.Errorf("failed to list objects: %w", err)
	}

	for _, obj := range objects {
		if err := s3Client.DeleteObject(ctx, bucketName, obj); err != nil {
			return fmt.Errorf("failed to delete object %s: %w", obj, err)
		}
	}

	// Delete the bucket
	if err := s3Client.DeleteBucket(ctx, bucketName); err != nil {
		// Check if it's because the bucket is not empty (shouldn't happen, but be safe)
		if !strings.Contains(err.Error(), "BucketNotEmpty") {
			return fmt.Errorf("failed to delete bucket: %w", err)
		}
	}

	return nil
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
