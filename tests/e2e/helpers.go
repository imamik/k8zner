//go:build e2e

package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"hcloud-k8s/internal/platform/hcloud"
	"hcloud-k8s/internal/util/keygen"

	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/client/config"
)

// ResourceCleaner helps track and clean up resources.
// Uses t.Cleanup() to ensure cleanup runs even on test timeout.
type ResourceCleaner struct {
	t *testing.T
}

// Add adds a cleanup function that will run even if the test times out.
// Functions are executed in LIFO order (last added, first executed).
func (rc *ResourceCleaner) Add(f func()) {
	rc.t.Cleanup(f)
}

// setupSSHKey generates a temporary SSH key, uploads it to HCloud, and registers cleanup.
// It returns the key name and private key bytes.
func setupSSHKey(t *testing.T, client *hcloud.RealClient, cleaner *ResourceCleaner, prefix string, labels map[string]string) (string, []byte) {
	keyName := fmt.Sprintf("%s-key-%d", prefix, time.Now().UnixNano())
	t.Logf("Generating SSH key %s...", keyName)

	keyPair, err := keygen.GenerateRSAKeyPair(2048)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	_, err = client.CreateSSHKey(context.Background(), keyName, string(keyPair.PublicKey), labels)
	if err != nil {
		t.Fatalf("Failed to upload ssh key: %v", err)
	}

	cleaner.Add(func() {
		t.Logf("Deleting SSH key %s...", keyName)
		if err := client.DeleteSSHKey(context.Background(), keyName); err != nil {
			t.Logf("Failed to delete ssh key %s (might not exist): %v", keyName, err)
		}
	})

	return keyName, keyPair.PrivateKey
}

// WaitForPort waits for a TCP port to become accessible.
// Uses a fixed polling interval of 5 seconds for predictable behavior.
func WaitForPort(ctx context.Context, ip string, port int, timeout time.Duration) error {
	address := net.JoinHostPort(ip, fmt.Sprintf("%d", port))

	// Use fixed 5-second polling interval for predictable behavior
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Try immediately first
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err == nil {
		_ = conn.Close()
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("timeout waiting for %s", address)
			}
			return ctx.Err()
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", address, 2*time.Second)
			if err == nil {
				_ = conn.Close()
				return nil
			}
		}
	}
}

// httpGet performs an HTTP GET request with a short timeout.
// Returns the response or an error if the request fails or times out.
func httpGet(url string) (*http.Response, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	return client.Get(url) //nolint:noctx // Simple helper for e2e tests
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
		resp.Body.Close()
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
		defer insecureClient.Close()

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
		defer authClient.Close()

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
	defer talosClient.Close()

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
	defer talosClient.Close()

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
	conn.Close()
	return nil
}

// verifyRDNS verifies that RDNS (PTR) records are configured correctly for a resource.
// It performs a reverse DNS lookup with retry logic (for DNS propagation) and checks
// if the result matches expected patterns.
// Returns an error if the lookup fails or the record doesn't match expectations.
func verifyRDNS(t *testing.T, ip, expectedPattern string) error {
	t.Logf("  Verifying RDNS for %s (expected pattern: %s)...", ip, expectedPattern)

	var names []string
	var err error
	maxRetries := 5

	// Retry with exponential backoff for DNS propagation
	// Delays: 1s, 2s, 4s, 8s, 16s (total ~31 seconds max)
	for i := 0; i < maxRetries; i++ {
		names, err = net.LookupAddr(ip)
		if err == nil && len(names) > 0 {
			break
		}

		if i < maxRetries-1 {
			delay := time.Duration(1<<uint(i)) * time.Second
			t.Logf("    DNS lookup attempt %d/%d failed, retrying in %v...", i+1, maxRetries, delay)
			time.Sleep(delay)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to lookup PTR record for %s after %d attempts: %w", ip, maxRetries, err)
	}

	if len(names) == 0 {
		return fmt.Errorf("no PTR record found for %s after %d attempts", ip, maxRetries)
	}

	// Check if any of the returned names match the expected pattern
	ptrRecord := strings.TrimSuffix(names[0], ".")
	t.Logf("  ✓ Found PTR record: %s → %s", ip, ptrRecord)

	// If expectedPattern is not empty, verify it matches
	if expectedPattern != "" && !strings.Contains(ptrRecord, expectedPattern) {
		return fmt.Errorf("PTR record %s does not contain expected pattern %s", ptrRecord, expectedPattern)
	}

	return nil
}

// verifyServerRDNS verifies RDNS configuration for all servers in the cluster.
func verifyServerRDNS(ctx context.Context, t *testing.T, state *E2EState) {
	t.Log("--- Verifying Server RDNS ---")

	// Verify control plane servers
	for i, ip := range state.ControlPlaneIPs {
		expectedPattern := state.ClusterName
		if err := verifyRDNS(t, ip, expectedPattern); err != nil {
			t.Logf("  Warning: RDNS verification failed for control plane node %d: %v", i+1, err)
		}
	}

	// Verify worker servers
	for i, ip := range state.WorkerIPs {
		expectedPattern := state.ClusterName
		if err := verifyRDNS(t, ip, expectedPattern); err != nil {
			t.Logf("  Warning: RDNS verification failed for worker node %d: %v", i+1, err)
		}
	}
}

// verifyLoadBalancerRDNS verifies RDNS configuration for load balancers.
func verifyLoadBalancerRDNS(ctx context.Context, t *testing.T, state *E2EState) {
	t.Log("--- Verifying Load Balancer RDNS ---")

	if state.LoadBalancerIP != "" {
		expectedPattern := state.ClusterName
		if err := verifyRDNS(t, state.LoadBalancerIP, expectedPattern); err != nil {
			t.Logf("  Warning: RDNS verification failed for load balancer: %v", err)
		}
	} else {
		t.Log("  ⚠ No load balancer IP available, skipping RDNS verification")
	}
}
