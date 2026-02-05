//go:build e2e

package e2e

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// DiagnosticCollector provides comprehensive diagnostic collection for E2E test failures.
// It collects cluster-wide health, node resources, pod status, events, and logs.
//
// Usage:
//
//	diag := NewDiagnosticCollector(t, kubeconfigPath, "kube-system", "app=hcloud-csi-controller")
//	diag.WithLBEndpoint("167.235.217.34", 6443)  // Query via load balancer
//	diag.WithTalosEndpoint("5.75.228.234")       // Query Talos API
//	diag.Collect()
//	diag.Report()
type DiagnosticCollector struct {
	t              *testing.T
	kubeconfigPath string
	namespace      string
	selector       string
	componentName  string

	// Optional endpoints for external checks
	lbIP         string
	lbPort       int
	talosIP      string
	talosconfig  string

	// Collected diagnostics
	results *DiagnosticResults
}

// DiagnosticResults holds all collected diagnostic information.
type DiagnosticResults struct {
	// Cluster health
	APIServerReachable bool
	APIServerLatency   time.Duration
	CoreDNSHealthy     bool
	NodeCount          int
	ReadyNodeCount     int

	// Node resources
	Nodes []NodeDiagnostic

	// Target component
	ComponentPods      []PodDiagnostic
	ComponentEvents    string
	ComponentLogs      map[string]string // container name -> logs

	// Namespace state
	AllPodsInNamespace string
	NamespaceEvents    string

	// External connectivity (via LB)
	LBReachable     bool
	LBLatency       time.Duration
	LBError         string

	// Talos-level diagnostics
	TalosReachable  bool
	TalosServices   string
	TalosContainers string
	TalosDmesg      string
}

// NodeDiagnostic holds resource info for a single node.
type NodeDiagnostic struct {
	Name           string
	Ready          bool
	CPUCapacity    string
	MemoryCapacity string
	CPUUsage       string
	MemoryUsage    string
	CPUPercent     string
	MemoryPercent  string
	Conditions     string
	Taints         string
}

// PodDiagnostic holds diagnostic info for a single pod.
type PodDiagnostic struct {
	Name            string
	Phase           string
	Ready           string
	Restarts        int
	Node            string
	IP              string
	ContainerStatus string
	Events          string
}

// NewDiagnosticCollector creates a new diagnostic collector for a specific component.
func NewDiagnosticCollector(t *testing.T, kubeconfigPath, namespace, selector string) *DiagnosticCollector {
	return &DiagnosticCollector{
		t:              t,
		kubeconfigPath: kubeconfigPath,
		namespace:      namespace,
		selector:       selector,
		componentName:  selector,
		results:        &DiagnosticResults{
			ComponentLogs: make(map[string]string),
		},
	}
}

// WithComponentName sets a human-readable name for the component.
func (d *DiagnosticCollector) WithComponentName(name string) *DiagnosticCollector {
	d.componentName = name
	return d
}

// WithLBEndpoint configures the load balancer endpoint for external connectivity checks.
func (d *DiagnosticCollector) WithLBEndpoint(ip string, port int) *DiagnosticCollector {
	d.lbIP = ip
	d.lbPort = port
	return d
}

// WithTalosEndpoint configures Talos API endpoint for low-level diagnostics.
func (d *DiagnosticCollector) WithTalosEndpoint(ip string) *DiagnosticCollector {
	d.talosIP = ip
	return d
}

// WithTalosConfig sets the path to talosconfig for Talos API access.
func (d *DiagnosticCollector) WithTalosConfig(path string) *DiagnosticCollector {
	d.talosconfig = path
	return d
}

// Collect gathers all diagnostic information.
func (d *DiagnosticCollector) Collect() {
	d.t.Log("=== COLLECTING DIAGNOSTICS ===")

	// Run collections in order of importance
	d.collectAPIServerHealth()
	d.collectNodeResources()
	d.collectCoreDNSHealth()
	d.collectComponentPods()
	d.collectComponentLogs()
	d.collectNamespaceState()

	if d.lbIP != "" {
		d.collectLBConnectivity()
	}

	if d.talosIP != "" {
		d.collectTalosDiagnostics()
	}
}

// Report outputs all collected diagnostics to the test log.
func (d *DiagnosticCollector) Report() {
	d.t.Log("")
	d.t.Log("╔══════════════════════════════════════════════════════════════════════╗")
	d.t.Logf("║  DIAGNOSTIC REPORT: %s", d.componentName)
	d.t.Log("╠══════════════════════════════════════════════════════════════════════╣")

	// Cluster Health Summary
	d.t.Log("║  📊 CLUSTER HEALTH")
	d.t.Log("╟──────────────────────────────────────────────────────────────────────╢")
	apiStatus := "✅ Reachable"
	if !d.results.APIServerReachable {
		apiStatus = "❌ UNREACHABLE"
	}
	d.t.Logf("║  API Server: %s (latency: %v)", apiStatus, d.results.APIServerLatency)

	dnsStatus := "✅ Healthy"
	if !d.results.CoreDNSHealthy {
		dnsStatus = "⚠️  Unhealthy or Not Ready"
	}
	d.t.Logf("║  CoreDNS: %s", dnsStatus)
	d.t.Logf("║  Nodes: %d/%d Ready", d.results.ReadyNodeCount, d.results.NodeCount)

	// LB Connectivity
	if d.lbIP != "" {
		d.t.Log("╟──────────────────────────────────────────────────────────────────────╢")
		d.t.Log("║  🔗 LOAD BALANCER CONNECTIVITY")
		lbStatus := "✅ Reachable"
		if !d.results.LBReachable {
			lbStatus = fmt.Sprintf("❌ UNREACHABLE: %s", d.results.LBError)
		}
		d.t.Logf("║  LB %s:%d - %s (latency: %v)", d.lbIP, d.lbPort, lbStatus, d.results.LBLatency)
	}

	// Node Resources
	d.t.Log("╟──────────────────────────────────────────────────────────────────────╢")
	d.t.Log("║  💻 NODE RESOURCES")
	for _, node := range d.results.Nodes {
		readyIcon := "✅"
		if !node.Ready {
			readyIcon = "❌"
		}
		d.t.Logf("║  %s %s", readyIcon, node.Name)
		if node.CPUUsage != "" {
			d.t.Logf("║     CPU: %s / %s (%s)", node.CPUUsage, node.CPUCapacity, node.CPUPercent)
			d.t.Logf("║     Memory: %s / %s (%s)", node.MemoryUsage, node.MemoryCapacity, node.MemoryPercent)
		}
		if node.Taints != "" {
			d.t.Logf("║     Taints: %s", node.Taints)
		}
		if !node.Ready && node.Conditions != "" {
			d.t.Logf("║     Conditions: %s", truncate(node.Conditions, 60))
		}
	}

	// Component Pods
	d.t.Log("╟──────────────────────────────────────────────────────────────────────╢")
	d.t.Logf("║  🎯 COMPONENT PODS (%s)", d.componentName)
	if len(d.results.ComponentPods) == 0 {
		d.t.Log("║  ⚠️  NO PODS FOUND!")
	}
	for _, pod := range d.results.ComponentPods {
		icon := "✅"
		if pod.Phase != "Running" {
			icon = "❌"
		} else if !strings.Contains(pod.Ready, "/") || strings.Split(pod.Ready, "/")[0] != strings.Split(pod.Ready, "/")[1] {
			icon = "⚠️"
		}
		d.t.Logf("║  %s %s", icon, pod.Name)
		d.t.Logf("║     Phase: %s, Ready: %s, Restarts: %d", pod.Phase, pod.Ready, pod.Restarts)
		d.t.Logf("║     Node: %s, IP: %s", pod.Node, pod.IP)
		if pod.ContainerStatus != "" {
			// Print container status on multiple lines if needed
			lines := strings.Split(pod.ContainerStatus, "\n")
			for _, line := range lines {
				if line != "" {
					d.t.Logf("║     %s", truncate(line, 65))
				}
			}
		}
	}

	// Component Logs (last 20 lines per container)
	if len(d.results.ComponentLogs) > 0 {
		d.t.Log("╟──────────────────────────────────────────────────────────────────────╢")
		d.t.Log("║  📜 COMPONENT LOGS (last 20 lines per container)")
		for container, logs := range d.results.ComponentLogs {
			d.t.Logf("║  --- %s ---", container)
			for _, line := range strings.Split(logs, "\n") {
				if line != "" {
					d.t.Logf("║  %s", truncate(line, 68))
				}
			}
		}
	}

	// Namespace Events
	if d.results.NamespaceEvents != "" {
		d.t.Log("╟──────────────────────────────────────────────────────────────────────╢")
		d.t.Logf("║  📅 RECENT EVENTS (%s namespace)", d.namespace)
		for _, line := range strings.Split(d.results.NamespaceEvents, "\n") {
			if line != "" {
				d.t.Logf("║  %s", truncate(line, 68))
			}
		}
	}

	// Talos Diagnostics
	if d.talosIP != "" && d.results.TalosReachable {
		d.t.Log("╟──────────────────────────────────────────────────────────────────────╢")
		d.t.Log("║  🔧 TALOS DIAGNOSTICS")
		if d.results.TalosServices != "" {
			d.t.Log("║  Services:")
			for _, line := range strings.Split(d.results.TalosServices, "\n") {
				if line != "" {
					d.t.Logf("║    %s", truncate(line, 66))
				}
			}
		}
	}

	d.t.Log("╚══════════════════════════════════════════════════════════════════════╝")
	d.t.Log("")
}

// collectAPIServerHealth checks API server availability and latency.
func (d *DiagnosticCollector) collectAPIServerHealth() {
	start := time.Now()
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", d.kubeconfigPath,
		"get", "--raw", "/healthz")
	output, err := cmd.CombinedOutput()
	d.results.APIServerLatency = time.Since(start)
	d.results.APIServerReachable = err == nil && strings.TrimSpace(string(output)) == "ok"
}

// collectNodeResources gathers node resource usage via kubectl top and node status.
func (d *DiagnosticCollector) collectNodeResources() {
	ctx := context.Background()

	// Get node list with status
	cmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", d.kubeconfigPath,
		"get", "nodes", "-o", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		d.t.Logf("  Warning: Could not get nodes: %v", err)
		return
	}

	var nodeList struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Spec struct {
				Taints []struct {
					Key    string `json:"key"`
					Effect string `json:"effect"`
				} `json:"taints"`
			} `json:"spec"`
			Status struct {
				Capacity struct {
					CPU    string `json:"cpu"`
					Memory string `json:"memory"`
				} `json:"capacity"`
				Conditions []struct {
					Type   string `json:"type"`
					Status string `json:"status"`
				} `json:"conditions"`
			} `json:"status"`
		} `json:"items"`
	}

	if err := json.Unmarshal(output, &nodeList); err != nil {
		d.t.Logf("  Warning: Could not parse nodes: %v", err)
		return
	}

	d.results.NodeCount = len(nodeList.Items)

	// Get node metrics if available
	metricsCmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", d.kubeconfigPath,
		"top", "nodes", "--no-headers")
	metricsOutput, _ := metricsCmd.CombinedOutput()
	metricsLines := strings.Split(strings.TrimSpace(string(metricsOutput)), "\n")
	metricsMap := make(map[string][]string)
	for _, line := range metricsLines {
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			metricsMap[fields[0]] = fields[1:] // cpu, cpu%, mem, mem%
		}
	}

	for _, node := range nodeList.Items {
		nodeDiag := NodeDiagnostic{
			Name:           node.Metadata.Name,
			CPUCapacity:    node.Status.Capacity.CPU,
			MemoryCapacity: node.Status.Capacity.Memory,
		}

		// Check Ready condition
		for _, cond := range node.Status.Conditions {
			if cond.Type == "Ready" {
				nodeDiag.Ready = cond.Status == "True"
				if nodeDiag.Ready {
					d.results.ReadyNodeCount++
				}
			}
			if cond.Status != "True" && cond.Type != "Ready" {
				nodeDiag.Conditions += fmt.Sprintf("%s=%s ", cond.Type, cond.Status)
			}
		}

		// Get taints
		var taints []string
		for _, taint := range node.Spec.Taints {
			taints = append(taints, fmt.Sprintf("%s:%s", taint.Key, taint.Effect))
		}
		nodeDiag.Taints = strings.Join(taints, ", ")

		// Add metrics if available
		if metrics, ok := metricsMap[node.Metadata.Name]; ok && len(metrics) >= 4 {
			nodeDiag.CPUUsage = metrics[0]
			nodeDiag.CPUPercent = metrics[1]
			nodeDiag.MemoryUsage = metrics[2]
			nodeDiag.MemoryPercent = metrics[3]
		}

		d.results.Nodes = append(d.results.Nodes, nodeDiag)
	}
}

// collectCoreDNSHealth checks if CoreDNS is healthy.
func (d *DiagnosticCollector) collectCoreDNSHealth() {
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", d.kubeconfigPath,
		"get", "pods", "-n", "kube-system", "-l", "k8s-app=kube-dns",
		"-o", "jsonpath={.items[*].status.phase}")
	output, err := cmd.CombinedOutput()
	d.results.CoreDNSHealthy = err == nil && strings.Contains(string(output), "Running")
}

// collectComponentPods gathers detailed information about the target component's pods.
func (d *DiagnosticCollector) collectComponentPods() {
	ctx := context.Background()

	// Get pods in JSON for detailed parsing
	cmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", d.kubeconfigPath,
		"get", "pods", "-n", d.namespace, "-l", d.selector, "-o", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		d.t.Logf("  Could not get component pods: %v", err)
		return
	}

	var podList struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Spec struct {
				NodeName string `json:"nodeName"`
			} `json:"spec"`
			Status struct {
				Phase             string `json:"phase"`
				PodIP             string `json:"podIP"`
				ContainerStatuses []struct {
					Name         string `json:"name"`
					Ready        bool   `json:"ready"`
					RestartCount int    `json:"restartCount"`
					State        struct {
						Waiting *struct {
							Reason  string `json:"reason"`
							Message string `json:"message"`
						} `json:"waiting"`
						Terminated *struct {
							Reason   string `json:"reason"`
							ExitCode int    `json:"exitCode"`
						} `json:"terminated"`
					} `json:"state"`
				} `json:"containerStatuses"`
			} `json:"status"`
		} `json:"items"`
	}

	if err := json.Unmarshal(output, &podList); err != nil {
		return
	}

	for _, pod := range podList.Items {
		podDiag := PodDiagnostic{
			Name:  pod.Metadata.Name,
			Phase: pod.Status.Phase,
			Node:  pod.Spec.NodeName,
			IP:    pod.Status.PodIP,
		}

		// Count ready containers
		readyCount := 0
		totalCount := len(pod.Status.ContainerStatuses)
		var containerInfo []string

		for _, cs := range pod.Status.ContainerStatuses {
			if cs.Ready {
				readyCount++
			}
			podDiag.Restarts += cs.RestartCount

			// Capture container state details
			if cs.State.Waiting != nil {
				containerInfo = append(containerInfo,
					fmt.Sprintf("%s: Waiting (%s) - %s", cs.Name, cs.State.Waiting.Reason, truncate(cs.State.Waiting.Message, 50)))
			} else if cs.State.Terminated != nil {
				containerInfo = append(containerInfo,
					fmt.Sprintf("%s: Terminated (%s, exit=%d)", cs.Name, cs.State.Terminated.Reason, cs.State.Terminated.ExitCode))
			}
		}

		podDiag.Ready = fmt.Sprintf("%d/%d", readyCount, totalCount)
		podDiag.ContainerStatus = strings.Join(containerInfo, "\n")

		d.results.ComponentPods = append(d.results.ComponentPods, podDiag)
	}
}

// collectComponentLogs gathers logs from failing containers.
func (d *DiagnosticCollector) collectComponentLogs() {
	ctx := context.Background()

	// Get pod names first
	cmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", d.kubeconfigPath,
		"get", "pods", "-n", d.namespace, "-l", d.selector,
		"-o", "jsonpath={.items[*].metadata.name}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return
	}

	podNames := strings.Fields(string(output))
	for _, podName := range podNames {
		// Get container names
		containerCmd := exec.CommandContext(ctx, "kubectl",
			"--kubeconfig", d.kubeconfigPath,
			"get", "pod", "-n", d.namespace, podName,
			"-o", "jsonpath={.spec.containers[*].name}")
		containerOutput, _ := containerCmd.CombinedOutput()
		containers := strings.Fields(string(containerOutput))

		for _, container := range containers {
			// Get last 20 lines of logs
			logCmd := exec.CommandContext(ctx, "kubectl",
				"--kubeconfig", d.kubeconfigPath,
				"logs", "-n", d.namespace, podName, "-c", container,
				"--tail=20")
			logOutput, err := logCmd.CombinedOutput()
			if err == nil && len(logOutput) > 0 {
				key := fmt.Sprintf("%s/%s", podName, container)
				d.results.ComponentLogs[key] = strings.TrimSpace(string(logOutput))
			}

			// Also try to get previous logs if container crashed
			prevLogCmd := exec.CommandContext(ctx, "kubectl",
				"--kubeconfig", d.kubeconfigPath,
				"logs", "-n", d.namespace, podName, "-c", container,
				"--previous", "--tail=10")
			prevLogOutput, err := prevLogCmd.CombinedOutput()
			if err == nil && len(prevLogOutput) > 0 {
				key := fmt.Sprintf("%s/%s (previous)", podName, container)
				d.results.ComponentLogs[key] = strings.TrimSpace(string(prevLogOutput))
			}
		}
	}
}

// collectNamespaceState gathers all pods and events in the namespace.
func (d *DiagnosticCollector) collectNamespaceState() {
	ctx := context.Background()

	// Get all pods in namespace
	cmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", d.kubeconfigPath,
		"get", "pods", "-n", d.namespace, "-o", "wide")
	output, _ := cmd.CombinedOutput()
	d.results.AllPodsInNamespace = string(output)

	// Get recent events
	eventsCmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", d.kubeconfigPath,
		"get", "events", "-n", d.namespace,
		"--sort-by=.lastTimestamp",
		"-o", "custom-columns=TIME:.lastTimestamp,TYPE:.type,REASON:.reason,OBJECT:.involvedObject.name,MESSAGE:.message",
		"--tail=15")
	eventsOutput, _ := eventsCmd.CombinedOutput()
	d.results.NamespaceEvents = strings.TrimSpace(string(eventsOutput))
}

// collectLBConnectivity tests connectivity through the load balancer.
func (d *DiagnosticCollector) collectLBConnectivity() {
	if d.lbIP == "" || d.lbPort == 0 {
		return
	}

	url := fmt.Sprintf("https://%s:%d/healthz", d.lbIP, d.lbPort)
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: nil, // Will use default which may reject self-signed
		},
	}

	start := time.Now()
	resp, err := client.Get(url)
	d.results.LBLatency = time.Since(start)

	if err != nil {
		// Try without TLS verification
		client.Transport = &http.Transport{
			TLSClientConfig: &tlsConfigInsecure,
		}
		resp, err = client.Get(url)
		d.results.LBLatency = time.Since(start)
	}

	if err != nil {
		d.results.LBReachable = false
		d.results.LBError = err.Error()
		return
	}
	defer resp.Body.Close()

	d.results.LBReachable = resp.StatusCode == 200
	if !d.results.LBReachable {
		body, _ := io.ReadAll(resp.Body)
		d.results.LBError = fmt.Sprintf("status=%d, body=%s", resp.StatusCode, truncate(string(body), 100))
	}
}

// collectTalosDiagnostics gathers Talos-level diagnostics via talosctl.
func (d *DiagnosticCollector) collectTalosDiagnostics() {
	if d.talosIP == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build talosctl args
	baseArgs := []string{"-n", d.talosIP}
	if d.talosconfig != "" {
		baseArgs = append(baseArgs, "--talosconfig", d.talosconfig)
	}

	// Check if talosctl can reach the node
	versionCmd := exec.CommandContext(ctx, "talosctl", append(baseArgs, "version", "--short")...)
	if _, err := versionCmd.CombinedOutput(); err != nil {
		d.results.TalosReachable = false
		return
	}
	d.results.TalosReachable = true

	// Get services status
	servicesCmd := exec.CommandContext(ctx, "talosctl", append(baseArgs, "services")...)
	if output, err := servicesCmd.CombinedOutput(); err == nil {
		d.results.TalosServices = strings.TrimSpace(string(output))
	}

	// Get container status (kubelet containers)
	containersCmd := exec.CommandContext(ctx, "talosctl", append(baseArgs, "containers", "-k")...)
	if output, err := containersCmd.CombinedOutput(); err == nil {
		d.results.TalosContainers = strings.TrimSpace(string(output))
	}

	// Get last 50 lines of dmesg for kernel issues
	dmesgCmd := exec.CommandContext(ctx, "talosctl", append(baseArgs, "dmesg", "--tail", "30")...)
	if output, err := dmesgCmd.CombinedOutput(); err == nil {
		d.results.TalosDmesg = strings.TrimSpace(string(output))
	}
}

// Helper to truncate long strings
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// tlsConfigInsecure is a TLS config that skips certificate verification.
// Used for testing connectivity to self-signed endpoints.
var tlsConfigInsecure = tls.Config{InsecureSkipVerify: true}

// CollectAndReportOnFailure is a convenience function to collect and report diagnostics
// when a test fails. Call this in timeout/error handlers.
func CollectAndReportOnFailure(t *testing.T, kubeconfigPath, namespace, selector, componentName string) {
	diag := NewDiagnosticCollector(t, kubeconfigPath, namespace, selector)
	diag.WithComponentName(componentName)
	diag.Collect()
	diag.Report()
}
