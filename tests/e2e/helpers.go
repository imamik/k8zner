//go:build e2e

package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"testing"
	"time"

	"github.com/imamik/k8zner/internal/util/keygen"

	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/client/config"
)

// WaitForPort waits for a TCP port to become accessible.
// Uses a fixed polling interval of 5 seconds for predictable behavior.
func WaitForPort(ctx context.Context, ip string, port int, timeout time.Duration) error {
	address := net.JoinHostPort(ip, fmt.Sprintf("%d", port))

	// Use fixed 5-second polling interval for predictable behavior
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Create timeout context that respects parent context cancellation
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Try immediately first
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err == nil {
		_ = conn.Close()
		return nil
	}

	for {
		select {
		case <-timeoutCtx.Done():
			if timeoutCtx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("timeout waiting for %s", address)
			}
			return timeoutCtx.Err()
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", address, 2*time.Second)
			if err == nil {
				_ = conn.Close()
				return nil
			}
		}
	}
}

// ClusterDiagnostics holds diagnostic information about a cluster.
type ClusterDiagnostics struct {
	t              *testing.T
	controlPlaneIP string
	lbIP           string
	talosConfig    []byte
}

// NewClusterDiagnostics creates a new diagnostics helper.
func NewClusterDiagnostics(t *testing.T, cpIP, lbIP string, talosConfig []byte) *ClusterDiagnostics {
	return &ClusterDiagnostics{
		t:              t,
		controlPlaneIP: cpIP,
		lbIP:           lbIP,
		talosConfig:    talosConfig,
	}
}

// RunFullDiagnostics runs comprehensive diagnostics and logs all findings.
func (d *ClusterDiagnostics) RunFullDiagnostics(ctx context.Context) {
	d.t.Log("=== RUNNING FULL CLUSTER DIAGNOSTICS ===")

	// 1. Basic connectivity checks
	d.checkBasicConnectivity(ctx)

	// 2. Load balancer health
	d.checkLoadBalancerHealth(ctx)

	// 3. Talos API status
	d.checkTalosAPIStatus(ctx)

	// 4. Talos services status
	d.checkTalosServices(ctx)

	// 5. etcd status
	d.checkEtcdStatus(ctx)

	d.t.Log("=== END DIAGNOSTICS ===")
}

// checkBasicConnectivity tests TCP connectivity to key ports.
func (d *ClusterDiagnostics) checkBasicConnectivity(ctx context.Context) {
	d.t.Log("--- Basic Connectivity Checks ---")

	// Check Talos API port on control plane
	if err := quickPortCheck(d.controlPlaneIP, 50000); err != nil {
		d.t.Logf("  ✗ Talos API (CP %s:50000): %v", d.controlPlaneIP, err)
	} else {
		d.t.Logf("  ✓ Talos API (CP %s:50000): reachable", d.controlPlaneIP)
	}

	// Check kube-api port directly on control plane
	if err := quickPortCheck(d.controlPlaneIP, 6443); err != nil {
		d.t.Logf("  ✗ Kube API direct (CP %s:6443): %v", d.controlPlaneIP, err)
	} else {
		d.t.Logf("  ✓ Kube API direct (CP %s:6443): reachable", d.controlPlaneIP)
	}

	// Check kube-api port through load balancer
	if d.lbIP != "" {
		if err := quickPortCheck(d.lbIP, 6443); err != nil {
			d.t.Logf("  ✗ Kube API via LB (%s:6443): %v", d.lbIP, err)
		} else {
			d.t.Logf("  ✓ Kube API via LB (%s:6443): reachable", d.lbIP)
		}
	}
}

// checkLoadBalancerHealth checks the LB health endpoint.
func (d *ClusterDiagnostics) checkLoadBalancerHealth(ctx context.Context) {
	d.t.Log("--- Load Balancer Health Check ---")

	if d.lbIP == "" {
		d.t.Log("  ⚠ No LB IP available")
		return
	}

	// Try to hit the /version endpoint (what the LB health check uses)
	url := fmt.Sprintf("https://%s:6443/version", d.lbIP)
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}

	resp, err := client.Get(url)
	if err != nil {
		d.t.Logf("  ✗ LB health check (/version): %v", err)
	} else {
		d.t.Logf("  ✓ LB health check (/version): status=%d", resp.StatusCode)
		_ = resp.Body.Close()
	}
}

// checkTalosAPIStatus verifies Talos API is responding.
func (d *ClusterDiagnostics) checkTalosAPIStatus(ctx context.Context) {
	d.t.Log("--- Talos API Status ---")

	// First try insecure (maintenance mode)
	d.t.Log("  Trying insecure connection (maintenance mode)...")
	insecureClient, err := client.New(ctx,
		client.WithEndpoints(d.controlPlaneIP),
		client.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}), //nolint:gosec
	)
	if err != nil {
		d.t.Logf("  ✗ Failed to create insecure client: %v", err)
	} else {
		defer func() { _ = insecureClient.Close() }()

		ctxTimeout, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		version, err := insecureClient.Version(ctxTimeout)
		if err != nil {
			d.t.Logf("  ✗ Insecure Talos API version check: %v", err)
		} else {
			d.t.Logf("  ✓ Insecure Talos API version: %v", version)
		}
	}

	// Try authenticated connection
	if len(d.talosConfig) > 0 {
		d.t.Log("  Trying authenticated connection...")
		cfg, err := config.FromString(string(d.talosConfig))
		if err != nil {
			d.t.Logf("  ✗ Failed to parse talos config: %v", err)
			return
		}

		authClient, err := client.New(ctx, client.WithConfig(cfg), client.WithEndpoints(d.controlPlaneIP))
		if err != nil {
			d.t.Logf("  ✗ Failed to create authenticated client: %v", err)
			return
		}
		defer func() { _ = authClient.Close() }()

		ctxTimeout, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		version, err := authClient.Version(ctxTimeout)
		if err != nil {
			d.t.Logf("  ✗ Authenticated Talos API version check: %v", err)
		} else {
			d.t.Logf("  ✓ Authenticated Talos API version: %v", version)
		}
	}
}

// checkTalosServices checks the status of Talos services on the control plane.
func (d *ClusterDiagnostics) checkTalosServices(ctx context.Context) {
	d.t.Log("--- Talos Services Status ---")

	if len(d.talosConfig) == 0 {
		d.t.Log("  ⚠ No talos config available, skipping service check")
		return
	}

	cfg, err := config.FromString(string(d.talosConfig))
	if err != nil {
		d.t.Logf("  ✗ Failed to parse talos config: %v", err)
		return
	}

	talosClient, err := client.New(ctx, client.WithConfig(cfg), client.WithEndpoints(d.controlPlaneIP))
	if err != nil {
		d.t.Logf("  ✗ Failed to create talos client: %v", err)
		return
	}
	defer func() { _ = talosClient.Close() }()

	ctxTimeout, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Get service list
	services, err := talosClient.ServiceList(ctxTimeout)
	if err != nil {
		d.t.Logf("  ✗ Failed to get service list: %v", err)
		return
	}

	// Log interesting services
	interestingServices := map[string]bool{
		"etcd":       true,
		"kubelet":    true,
		"apid":       true,
		"trustd":     true,
		"containerd": true,
	}

	for _, msg := range services.Messages {
		for _, svc := range msg.Services {
			if interestingServices[svc.Id] {
				state := svc.State
				health := "unknown"
				if svc.Health != nil {
					if svc.Health.Healthy {
						health = "healthy"
					} else {
						health = "unhealthy"
					}
				}
				d.t.Logf("  %s: state=%s health=%s", svc.Id, state, health)
			}
		}
	}
}

// checkEtcdStatus checks etcd cluster health.
func (d *ClusterDiagnostics) checkEtcdStatus(ctx context.Context) {
	d.t.Log("--- etcd Status ---")

	if len(d.talosConfig) == 0 {
		d.t.Log("  ⚠ No talos config available, skipping etcd check")
		return
	}

	cfg, err := config.FromString(string(d.talosConfig))
	if err != nil {
		d.t.Logf("  ✗ Failed to parse talos config: %v", err)
		return
	}

	talosClient, err := client.New(ctx, client.WithConfig(cfg), client.WithEndpoints(d.controlPlaneIP))
	if err != nil {
		d.t.Logf("  ✗ Failed to create talos client: %v", err)
		return
	}
	defer func() { _ = talosClient.Close() }()

	ctxTimeout, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Try to get etcd members
	members, err := talosClient.EtcdMemberList(ctxTimeout, &machine.EtcdMemberListRequest{})
	if err != nil {
		d.t.Logf("  ✗ Failed to get etcd members: %v", err)
		return
	}

	for _, msg := range members.Messages {
		d.t.Logf("  etcd members: %d", len(msg.Members))
		for _, member := range msg.Members {
			d.t.Logf("    - ID=%d Name=%s", member.Id, member.Hostname)
		}
	}
}

// quickPortCheck performs a quick TCP port check with a short timeout.
func quickPortCheck(ip string, port int) error {
	address := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}

// gatherKubernetesDiagnostics collects comprehensive diagnostic information about
// pods, events, CRDs, and connectivity for debugging test failures.
func gatherKubernetesDiagnostics(t *testing.T, kubeconfigPath, namespace, diagContext string) {
	t.Log("=== GATHERING KUBERNETES DIAGNOSTICS ===")
	t.Logf("Namespace: %s, Context: %s", namespace, diagContext)

	// 1. Get pod status with node placement
	t.Log("--- Pod Status ---")
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "pods", "-n", namespace, "-o", "wide")
	if output, err := cmd.CombinedOutput(); err == nil {
		t.Logf("Pods:\n%s", string(output))
	} else {
		t.Logf("Failed to get pods: %v", err)
	}

	// 2. Describe pods with events and conditions
	t.Log("--- Pod Details ---")
	cmd = exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"describe", "pods", "-n", namespace)
	if output, err := cmd.CombinedOutput(); err == nil {
		// Limit output to avoid overwhelming logs
		outputStr := string(output)
		if len(outputStr) > 10000 {
			outputStr = outputStr[:10000] + "\n... (truncated)"
		}
		t.Logf("Pod descriptions:\n%s", outputStr)
	} else {
		t.Logf("Failed to describe pods: %v", err)
	}

	// 3. Get recent events sorted by timestamp
	t.Log("--- Recent Events ---")
	cmd = exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
	if output, err := cmd.CombinedOutput(); err == nil {
		t.Logf("Events:\n%s", string(output))
	} else {
		t.Logf("Failed to get events: %v", err)
	}

	// 4. Get K8znerCluster CRD status if in k8zner-system namespace
	if namespace == "k8zner-system" {
		t.Log("--- K8znerCluster CRD Status ---")
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"get", "k8znerclusters", "-n", namespace, "-o", "yaml")
		if output, err := cmd.CombinedOutput(); err == nil {
			t.Logf("K8znerClusters:\n%s", string(output))
		} else {
			t.Logf("Failed to get K8znerClusters: %v", err)
		}
	}

	// 5. Get recent logs from pods in namespace
	t.Log("--- Recent Pod Logs ---")
	cmd = exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"logs", "-n", namespace, "--all-containers=true", "--tail=50", "-l", "app")
	if output, err := cmd.CombinedOutput(); err == nil {
		outputStr := string(output)
		if len(outputStr) > 5000 {
			outputStr = outputStr[:5000] + "\n... (truncated)"
		}
		t.Logf("Pod logs:\n%s", outputStr)
	} else {
		t.Logf("Failed to get pod logs: %v (this is normal if no pods match)", err)
	}

	t.Log("=== END KUBERNETES DIAGNOSTICS ===")
}

// gatherConnectivityDiagnostics checks TCP connectivity to control plane IPs.
func gatherConnectivityDiagnostics(t *testing.T, controlPlaneIPs []string) {
	t.Log("=== CONNECTIVITY DIAGNOSTICS ===")

	for _, ip := range controlPlaneIPs {
		// Check Kubernetes API port (6443)
		if err := quickPortCheck(ip, 6443); err != nil {
			t.Logf("  ✗ %s:6443 (kube-api): %v", ip, err)
		} else {
			t.Logf("  ✓ %s:6443 (kube-api): reachable", ip)
		}

		// Check Talos API port (50000)
		if err := quickPortCheck(ip, 50000); err != nil {
			t.Logf("  ✗ %s:50000 (talos-api): %v", ip, err)
		} else {
			t.Logf("  ✓ %s:50000 (talos-api): reachable", ip)
		}
	}

	t.Log("=== END CONNECTIVITY DIAGNOSTICS ===")
}

// runClusterDiagnostics runs comprehensive diagnostics on a cluster for debugging test failures.
func runClusterDiagnostics(ctx context.Context, t *testing.T, state *E2EState) {
	if len(state.ControlPlaneIPs) == 0 {
		t.Log("No control plane IPs available for diagnostics")
		return
	}

	diag := NewClusterDiagnostics(t, state.ControlPlaneIPs[0], state.LoadBalancerIP, state.TalosConfig)
	diag.RunFullDiagnostics(ctx)
}

// setupSSHKeyForCluster creates an SSH key in Hetzner Cloud for the cluster.
func setupSSHKeyForCluster(ctx context.Context, t *testing.T, state *E2EState) error {
	keyName := fmt.Sprintf("%s-key-%d", state.ClusterName, time.Now().UnixNano())

	keyPair, err := keygen.GenerateRSAKeyPair(2048)
	if err != nil {
		return fmt.Errorf("failed to generate key pair: %w", err)
	}

	labels := map[string]string{
		"cluster": state.ClusterName,
		"test-id": state.TestID,
	}

	_, err = state.Client.CreateSSHKey(ctx, keyName, string(keyPair.PublicKey), labels)
	if err != nil {
		return fmt.Errorf("failed to upload SSH key: %w", err)
	}

	state.SSHKeyName = keyName
	state.SSHPrivateKey = keyPair.PrivateKey
	return nil
}
