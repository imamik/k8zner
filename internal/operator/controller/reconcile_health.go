package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
)

// reconcileHealthCheck checks the health of all nodes and updates status.
func (r *ClusterReconciler) reconcileHealthCheck(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) error {
	logger := log.FromContext(ctx)

	// List all nodes in the cluster
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	logger.V(1).Info("health check: listing nodes", "totalNodes", len(nodeList.Items))

	// Categorize nodes by role
	var cpNodes, workerNodes []corev1.Node
	for _, node := range nodeList.Items {
		isReady := isNodeReady(&node)
		logger.V(1).Info("health check: examining node",
			"name", node.Name,
			"isReady", isReady,
			"providerID", node.Spec.ProviderID,
			"labels", node.Labels,
		)

		if _, isCP := node.Labels["node-role.kubernetes.io/control-plane"]; isCP {
			cpNodes = append(cpNodes, node)
		} else {
			workerNodes = append(workerNodes, node)
		}
	}

	logger.V(1).Info("health check: nodes categorized",
		"cpNodes", len(cpNodes),
		"workerNodes", len(workerNodes),
	)

	// Update control plane status
	cluster.Status.ControlPlanes = r.buildNodeGroupStatus(ctx, cluster, cpNodes, cluster.Spec.ControlPlanes.Count, "control-plane")

	// Update worker status
	cluster.Status.Workers = r.buildNodeGroupStatus(ctx, cluster, workerNodes, cluster.Spec.Workers.Count, "worker")

	// Record metrics
	r.recordNodeCounts(cluster.Name, "control-plane",
		len(cpNodes), cluster.Status.ControlPlanes.Ready, cluster.Spec.ControlPlanes.Count)
	r.recordNodeCounts(cluster.Name, "worker",
		len(workerNodes), cluster.Status.Workers.Ready, cluster.Spec.Workers.Count)

	logger.Info("health check complete",
		"controlPlanes", fmt.Sprintf("%d/%d", cluster.Status.ControlPlanes.Ready, cluster.Status.ControlPlanes.Desired),
		"workers", fmt.Sprintf("%d/%d", cluster.Status.Workers.Ready, cluster.Status.Workers.Desired),
	)

	return nil
}

// buildNodeGroupStatus builds the status for a group of nodes.
func (r *ClusterReconciler) buildNodeGroupStatus(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, nodes []corev1.Node, desired int, role string) k8znerv1alpha1.NodeGroupStatus {
	status := k8znerv1alpha1.NodeGroupStatus{
		Desired: desired,
		Nodes:   make([]k8znerv1alpha1.NodeStatus, 0, len(nodes)),
	}

	now := metav1.Now()

	for _, node := range nodes {
		nodeStatus := k8znerv1alpha1.NodeStatus{
			Name:            node.Name,
			LastHealthCheck: &now,
		}

		// Extract server ID from provider ID (format: hcloud://12345)
		if node.Spec.ProviderID != "" {
			var serverID int64
			if _, err := fmt.Sscanf(node.Spec.ProviderID, "hcloud://%d", &serverID); err == nil {
				nodeStatus.ServerID = serverID
			}
		}

		// Get IPs
		for _, addr := range node.Status.Addresses {
			switch addr.Type {
			case corev1.NodeInternalIP:
				nodeStatus.PrivateIP = addr.Address
			case corev1.NodeExternalIP:
				nodeStatus.PublicIP = addr.Address
			}
		}

		// Check health
		nodeStatus.Healthy = isNodeReady(&node)
		if !nodeStatus.Healthy {
			nodeStatus.UnhealthyReason = getNodeUnhealthyReason(&node)
			// Track when node became unhealthy
			for _, cond := range node.Status.Conditions {
				if cond.Type == corev1.NodeReady && cond.Status != corev1.ConditionTrue {
					nodeStatus.UnhealthySince = &cond.LastTransitionTime
					break
				}
			}
			status.Unhealthy++

			// Record event for newly unhealthy nodes
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonNodeUnhealthy,
				"%s node %s is unhealthy: %s", role, node.Name, nodeStatus.UnhealthyReason)
		} else {
			status.Ready++
		}

		status.Nodes = append(status.Nodes, nodeStatus)
	}

	return status
}

// updateClusterPhase updates the overall cluster phase based on status.
func (r *ClusterReconciler) updateClusterPhase(cluster *k8znerv1alpha1.K8znerCluster) {
	cpReady := cluster.Status.ControlPlanes.Ready == cluster.Status.ControlPlanes.Desired
	workersReady := cluster.Status.Workers.Ready == cluster.Status.Workers.Desired

	switch {
	case cpReady && workersReady:
		cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseRunning
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:    k8znerv1alpha1.ConditionReady,
			Status:  metav1.ConditionTrue,
			Reason:  "AllHealthy",
			Message: "All nodes are healthy",
		})
	case cluster.Status.Phase != k8znerv1alpha1.ClusterPhaseHealing:
		cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseDegraded
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:   k8znerv1alpha1.ConditionReady,
			Status: metav1.ConditionFalse,
			Reason: "NodesUnhealthy",
			Message: fmt.Sprintf("Control planes: %d/%d, Workers: %d/%d",
				cluster.Status.ControlPlanes.Ready, cluster.Status.ControlPlanes.Desired,
				cluster.Status.Workers.Ready, cluster.Status.Workers.Desired),
		})
	}

	// Update individual conditions
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:   k8znerv1alpha1.ConditionControlPlaneReady,
		Status: conditionStatus(cpReady),
		Reason: conditionReason(cpReady, "AllHealthy", "SomeUnhealthy"),
		Message: fmt.Sprintf("%d/%d control planes ready",
			cluster.Status.ControlPlanes.Ready, cluster.Status.ControlPlanes.Desired),
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:   k8znerv1alpha1.ConditionWorkersReady,
		Status: conditionStatus(workersReady),
		Reason: conditionReason(workersReady, "AllHealthy", "SomeUnhealthy"),
		Message: fmt.Sprintf("%d/%d workers ready",
			cluster.Status.Workers.Ready, cluster.Status.Workers.Desired),
	})
}

// Helper functions

func isNodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

func getNodeUnhealthyReason(node *corev1.Node) string {
	for _, cond := range node.Status.Conditions {
		switch {
		case cond.Type == corev1.NodeReady && cond.Status != corev1.ConditionTrue:
			return fmt.Sprintf("NodeNotReady: %s", cond.Message)
		case cond.Type == corev1.NodeMemoryPressure && cond.Status == corev1.ConditionTrue:
			return "MemoryPressure"
		case cond.Type == corev1.NodeDiskPressure && cond.Status == corev1.ConditionTrue:
			return "DiskPressure"
		case cond.Type == corev1.NodePIDPressure && cond.Status == corev1.ConditionTrue:
			return "PIDPressure"
		}
	}
	return "Unknown"
}

func parseThreshold(healthCheck *k8znerv1alpha1.HealthCheckSpec, thresholdType string) time.Duration {
	if healthCheck == nil {
		if thresholdType == "etcd" {
			return defaultEtcdUnhealthyThreshold
		}
		return defaultNodeNotReadyThreshold
	}

	var durationStr string
	switch thresholdType {
	case "etcd":
		durationStr = healthCheck.EtcdUnhealthyThreshold
		if durationStr == "" {
			return defaultEtcdUnhealthyThreshold
		}
	default:
		durationStr = healthCheck.NodeNotReadyThreshold
		if durationStr == "" {
			return defaultNodeNotReadyThreshold
		}
	}

	d, err := time.ParseDuration(durationStr)
	if err != nil {
		if thresholdType == "etcd" {
			return defaultEtcdUnhealthyThreshold
		}
		return defaultNodeNotReadyThreshold
	}
	return d
}

func conditionStatus(ready bool) metav1.ConditionStatus {
	if ready {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

func conditionReason(ready bool, trueReason, falseReason string) string {
	if ready {
		return trueReason
	}
	return falseReason
}
