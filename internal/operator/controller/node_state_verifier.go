// Package controller contains the Kubernetes controllers for the k8zner operator.
package controller

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/siderolabs/talos/pkg/machinery/client"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// NodeStateInfo contains the verified state of a node from external systems.
type NodeStateInfo struct {
	// HCloud server state
	ServerExists bool
	ServerStatus string // "running", "off", "starting", etc.
	ServerIP     string

	// Network reachability
	TalosAPIReachable bool
	SSHReachable      bool

	// Talos state
	TalosInMaintenanceMode bool
	TalosConfigured        bool
	TalosKubeletRunning    bool

	// Kubernetes state
	K8sNodeExists bool
	K8sNodeReady  bool
}

// VerifyNodeState checks the actual state of a node using HCloud, Talos, and K8s APIs.
// This provides ground truth about node state independent of what the operator thinks.
func (r *ClusterReconciler) VerifyNodeState(ctx context.Context, nodeName string, nodeIP string) (*NodeStateInfo, error) {
	logger := log.FromContext(ctx)
	info := &NodeStateInfo{}

	// Step 1: Check HCloud server status
	server, err := r.hcloudClient.GetServerByName(ctx, nodeName)
	if err != nil || server == nil {
		logger.V(1).Info("failed to get server from HCloud", "node", nodeName, "error", err)
		info.ServerExists = false
	} else {
		info.ServerExists = true
		info.ServerStatus = string(server.Status)
		// Get IP from server
		if server.PublicNet.IPv4.IP != nil {
			nodeIP = server.PublicNet.IPv4.IP.String()
		}
	}
	info.ServerIP = nodeIP

	// Step 2: Check network reachability
	if nodeIP != "" {
		info.TalosAPIReachable = r.checkPortReachable(nodeIP, 50000, 5*time.Second)
		info.SSHReachable = r.checkPortReachable(nodeIP, 22, 5*time.Second)
	}

	// Step 3: Check Talos state (if API is reachable)
	if info.TalosAPIReachable {
		talosState, err := r.checkTalosState(ctx, nodeIP)
		if err != nil {
			logger.V(1).Info("failed to check Talos state", "node", nodeName, "error", err)
		} else {
			info.TalosInMaintenanceMode = talosState.inMaintenanceMode
			info.TalosConfigured = talosState.isConfigured
			info.TalosKubeletRunning = talosState.kubeletRunning
		}
	}

	// Step 4: Check Kubernetes node state
	k8sNode := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, k8sNode); err == nil {
		info.K8sNodeExists = true
		info.K8sNodeReady = isNodeReady(k8sNode)
	}

	return info, nil
}

// DetermineNodePhaseFromState determines the appropriate NodePhase based on verified state.
func DetermineNodePhaseFromState(info *NodeStateInfo) (k8znerv1alpha1.NodePhase, string) {
	// If K8s node exists and is ready, it's Ready
	if info.K8sNodeExists && info.K8sNodeReady {
		return k8znerv1alpha1.NodePhaseReady, "Node is registered and ready in Kubernetes"
	}

	// If K8s node exists but not ready, it's initializing (CNI, system pods, etc.)
	if info.K8sNodeExists && !info.K8sNodeReady {
		if info.TalosKubeletRunning {
			return k8znerv1alpha1.NodePhaseNodeInitializing, "Node registered, waiting for system pods (CNI, etc.)"
		}
		return k8znerv1alpha1.NodePhaseWaitingForK8s, "Waiting for kubelet to become ready"
	}

	// No K8s node yet - check Talos state
	if info.TalosConfigured && info.TalosKubeletRunning {
		return k8znerv1alpha1.NodePhaseWaitingForK8s, "Talos configured, waiting for Kubernetes node registration"
	}

	if info.TalosConfigured && !info.TalosKubeletRunning {
		return k8znerv1alpha1.NodePhaseRebootingWithConfig, "Talos configured, kubelet not yet running"
	}

	if info.TalosInMaintenanceMode {
		return k8znerv1alpha1.NodePhaseWaitingForTalosAPI, "Talos in maintenance mode, ready for configuration"
	}

	// Check if Talos API is reachable but we couldn't determine state
	if info.TalosAPIReachable {
		return k8znerv1alpha1.NodePhaseApplyingTalosConfig, "Talos API reachable, checking configuration state"
	}

	// Server exists but Talos not reachable
	if info.ServerExists && info.ServerStatus == "running" {
		return k8znerv1alpha1.NodePhaseWaitingForTalosAPI, fmt.Sprintf("Server running, waiting for Talos API on %s:50000", info.ServerIP)
	}

	if info.ServerExists && info.ServerStatus == "starting" {
		return k8znerv1alpha1.NodePhaseWaitingForIP, "Server starting, waiting for boot"
	}

	if info.ServerExists {
		return k8znerv1alpha1.NodePhaseCreatingServer, fmt.Sprintf("Server exists in state: %s", info.ServerStatus)
	}

	// Server doesn't exist
	return k8znerv1alpha1.NodePhaseFailed, "Server does not exist in HCloud"
}

// checkPortReachable checks if a TCP port is reachable.
func (r *ClusterReconciler) checkPortReachable(ip string, port int, timeout time.Duration) bool {
	address := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// talosStateInfo holds Talos-specific state information.
type talosStateInfo struct {
	inMaintenanceMode bool
	isConfigured      bool
	kubeletRunning    bool
}

// checkTalosState checks the Talos state of a node.
func (r *ClusterReconciler) checkTalosState(ctx context.Context, nodeIP string) (*talosStateInfo, error) {
	info := &talosStateInfo{}

	// Try insecure connection first (maintenance mode check)
	insecureCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	insecureClient, err := client.New(insecureCtx,
		client.WithEndpoints(nodeIP),
		//nolint:gosec // InsecureSkipVerify is required to detect Talos maintenance mode
		client.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create insecure Talos client: %w", err)
	}
	defer func() { _ = insecureClient.Close() }()

	// Try to get version
	_, err = insecureClient.Version(insecureCtx)
	if err != nil {
		errStr := err.Error()
		// Check if error indicates maintenance mode
		if strings.Contains(errStr, "maintenance mode") || strings.Contains(errStr, "not implemented") {
			info.inMaintenanceMode = true
			return info, nil
		}
		// Some other error - might be configured with mTLS
		info.isConfigured = true
	} else {
		// Insecure connection worked - either maintenance mode or no auth required
		// Check services to determine if configured
		resp, err := insecureClient.ServiceList(insecureCtx)
		if err == nil {
			// If we can list services without auth, might be maintenance mode
			// Check if kubelet exists and is running
			for _, msg := range resp.Messages {
				for _, svc := range msg.Services {
					if svc.Id == "kubelet" {
						info.isConfigured = true
						if svc.State == "Running" {
							info.kubeletRunning = true
						}
					}
				}
			}
		} else {
			// ServiceList failed - likely maintenance mode
			info.inMaintenanceMode = true
		}
	}

	return info, nil
}
