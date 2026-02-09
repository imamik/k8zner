package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/util/naming"
)

// findHealthyControlPlaneIP finds the IP of a healthy control plane for API operations.
func (r *ClusterReconciler) findHealthyControlPlaneIP(cluster *k8znerv1alpha1.K8znerCluster) string {
	for _, node := range cluster.Status.ControlPlanes.Nodes {
		if node.Healthy && node.PrivateIP != "" {
			return node.PrivateIP
		}
	}
	return ""
}

// buildClusterState extracts cluster metadata needed for server creation.
func (r *ClusterReconciler) buildClusterState(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (*ClusterState, error) {
	logger := log.FromContext(ctx)

	state := &ClusterState{
		Name:   cluster.Name,
		Region: cluster.Spec.Region,
		Labels: map[string]string{
			"cluster": cluster.Name,
		},
	}

	networkID, err := r.resolveNetworkIDForState(ctx, cluster)
	if err != nil {
		return nil, err
	}
	state.NetworkID = networkID
	state.SANs = buildClusterSANs(cluster)
	state.SSHKeyIDs = resolveSSHKeyIDs(cluster)
	state.ControlPlaneIP = r.resolveControlPlaneIP(cluster)

	logger.V(1).Info("built cluster state",
		"networkID", state.NetworkID,
		"sans", len(state.SANs),
		"sshKeys", len(state.SSHKeyIDs),
		"controlPlaneIP", state.ControlPlaneIP,
	)

	return state, nil
}

// resolveNetworkIDForState returns the network ID from CRD status or HCloud lookup.
func (r *ClusterReconciler) resolveNetworkIDForState(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (int64, error) {
	if cluster.Status.Infrastructure.NetworkID != 0 {
		return cluster.Status.Infrastructure.NetworkID, nil
	}
	networkName := fmt.Sprintf("%s-network", cluster.Name)
	network, err := r.hcloudClient.GetNetwork(ctx, networkName)
	if err != nil {
		return 0, fmt.Errorf("failed to get network %s: %w", networkName, err)
	}
	if network != nil {
		return network.ID, nil
	}
	return 0, nil
}

// buildClusterSANs builds TLS SANs from control plane endpoint and node IPs.
func buildClusterSANs(cluster *k8znerv1alpha1.K8znerCluster) []string {
	var sans []string

	//nolint:gocritic // ifElseChain is appropriate here as we're checking different sources with fallback logic
	if cluster.Status.ControlPlaneEndpoint != "" {
		sans = append(sans, cluster.Status.ControlPlaneEndpoint)
	} else if cluster.Status.Infrastructure.LoadBalancerIP != "" {
		sans = append(sans, cluster.Status.Infrastructure.LoadBalancerIP)
	} else if cluster.Annotations != nil {
		if endpoint, ok := cluster.Annotations["k8zner.io/control-plane-endpoint"]; ok {
			sans = append(sans, endpoint)
		}
	}

	for _, node := range cluster.Status.ControlPlanes.Nodes {
		if node.PrivateIP != "" {
			sans = append(sans, node.PrivateIP)
		}
		if node.PublicIP != "" {
			sans = append(sans, node.PublicIP)
		}
	}
	return sans
}

// resolveSSHKeyIDs returns SSH key IDs from annotations or default naming convention.
func resolveSSHKeyIDs(cluster *k8znerv1alpha1.K8znerCluster) []string {
	if cluster.Annotations != nil {
		if sshKeys, ok := cluster.Annotations["k8zner.io/ssh-keys"]; ok {
			return strings.Split(sshKeys, ",")
		}
	}
	return []string{fmt.Sprintf("%s-key", cluster.Name)}
}

// resolveControlPlaneIP returns the best available control plane endpoint IP.
// Priority: status endpoint > infrastructure LB IP > annotation > healthy CP IP.
func (r *ClusterReconciler) resolveControlPlaneIP(cluster *k8znerv1alpha1.K8znerCluster) string {
	//nolint:gocritic // ifElseChain is appropriate here as we're checking different sources with fallback logic
	if cluster.Status.ControlPlaneEndpoint != "" {
		return cluster.Status.ControlPlaneEndpoint
	} else if cluster.Status.Infrastructure.LoadBalancerIP != "" {
		return cluster.Status.Infrastructure.LoadBalancerIP
	} else if cluster.Annotations != nil {
		if endpoint, ok := cluster.Annotations["k8zner.io/control-plane-endpoint"]; ok {
			return endpoint
		}
	}
	return r.findHealthyControlPlaneIP(cluster)
}

// generateReplacementServerName generates a new server name for a replacement node.
func (r *ClusterReconciler) generateReplacementServerName(cluster *k8znerv1alpha1.K8znerCluster, role string, oldName string) string {
	switch role {
	case "control-plane":
		return naming.ControlPlane(cluster.Name)
	case "worker":
		return naming.Worker(cluster.Name)
	default:
		return fmt.Sprintf("%s-%s-%s", cluster.Name, role[:2], naming.GenerateID(naming.IDLength))
	}
}

// waitForServerIP waits for a server to have an IP assigned.
func (r *ClusterReconciler) waitForServerIP(ctx context.Context, serverName string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Check immediately first (helps with mocks and already-assigned IPs)
	ip, err := r.hcloudClient.GetServerIP(ctx, serverName)
	if err == nil && ip != "" {
		return ip, nil
	}

	ticker := time.NewTicker(serverIPRetryDelay)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("timeout waiting for server IP: %w", ctx.Err())
		case <-ticker.C:
			ip, err := r.hcloudClient.GetServerIP(ctx, serverName)
			if err != nil {
				continue
			}
			if ip != "" {
				return ip, nil
			}
		}
	}
}

// waitForK8sNodeReady waits for a Kubernetes node to appear and become Ready.
func (r *ClusterReconciler) waitForK8sNodeReady(ctx context.Context, nodeName string, timeout time.Duration) error {
	logger := log.FromContext(ctx)

	time.Sleep(10 * time.Second)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for node %s to become ready", nodeName)
		case <-ticker.C:
			node := &corev1.Node{}
			err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node)
			if err != nil {
				if apierrors.IsNotFound(err) {
					logger.V(1).Info("node not yet registered in Kubernetes", "node", nodeName)
					continue
				}
				logger.V(1).Info("error getting node", "node", nodeName, "error", err)
				continue
			}

			for _, condition := range node.Status.Conditions {
				if condition.Type == corev1.NodeReady {
					if condition.Status == corev1.ConditionTrue {
						logger.Info("node is ready in Kubernetes", "node", nodeName)
						return nil
					}
					logger.V(1).Info("node exists but not ready", "node", nodeName, "status", condition.Status)
					break
				}
			}
		}
	}
}
