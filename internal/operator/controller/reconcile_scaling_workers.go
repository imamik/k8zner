package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/util/naming"
)

// reconcileWorkers ensures workers are healthy and at the desired count.
func (r *ClusterReconciler) reconcileWorkers(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	// Replace unhealthy workers first
	threshold := parseThreshold(cluster.Spec.HealthCheck, "node")
	unhealthyWorkers := findUnhealthyNodes(cluster.Status.Workers.Nodes, threshold)
	replaced := r.replaceUnhealthyWorkers(ctx, cluster, unhealthyWorkers)

	// Then handle scaling
	result, err := r.scaleWorkers(ctx, cluster)
	if err != nil || result.RequeueAfter > 0 {
		return result, err
	}

	if replaced > 0 || len(unhealthyWorkers) > 0 {
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	return ctrl.Result{}, nil
}

// replaceUnhealthyWorkers replaces unhealthy workers up to maxConcurrentHeals.
func (r *ClusterReconciler) replaceUnhealthyWorkers(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, unhealthyWorkers []*k8znerv1alpha1.NodeStatus) int {
	logger := log.FromContext(ctx)

	replaced := 0
	for _, worker := range unhealthyWorkers {
		if replaced >= r.maxConcurrentHeals {
			break
		}

		logger.Info("replacing unhealthy worker",
			"node", worker.Name,
			"serverID", worker.ServerID,
			"unhealthySince", worker.UnhealthySince,
		)

		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonNodeReplacing,
			"Replacing unhealthy worker %s (unhealthy since %v)",
			worker.Name, worker.UnhealthySince)

		cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseHealing

		startTime := time.Now()
		if err := r.replaceWorker(ctx, cluster, worker); err != nil {
			logger.Error(err, "failed to replace worker", "node", worker.Name)
			continue
		}

		r.recordNodeReplacement(cluster.Name, "worker", worker.UnhealthyReason)
		r.recordNodeReplacementDuration(cluster.Name, "worker", time.Since(startTime).Seconds())

		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonNodeReplaced,
			"Successfully replaced worker %s", worker.Name)
		replaced++
	}

	return replaced
}

// scaleWorkers handles worker scaling up and down.
func (r *ClusterReconciler) scaleWorkers(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	currentCount := len(cluster.Status.Workers.Nodes)
	provisioningCount := countWorkersInEarlyProvisioning(cluster.Status.Workers.Nodes)
	desiredCount := cluster.Spec.Workers.Count

	// Skip scaling if workers are already provisioning to prevent duplicate server creation
	if provisioningCount > 0 {
		logger.Info("workers currently provisioning, skipping scaling check",
			"provisioning", provisioningCount,
			"current", currentCount,
			"desired", desiredCount,
		)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	if currentCount < desiredCount {
		logger.Info("scaling up workers", "current", currentCount, "desired", desiredCount)
		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonScalingUp,
			"Scaling up workers: %d -> %d", currentCount, desiredCount)

		if r.hcloudClient != nil {
			cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseHealing
			toCreate := desiredCount - currentCount
			if err := r.scaleUpWorkers(ctx, cluster, toCreate); err != nil {
				logger.Error(err, "failed to scale up workers")
			}
		}
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	} else if currentCount > desiredCount {
		logger.Info("scaling down workers", "current", currentCount, "desired", desiredCount)
		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonScalingDown,
			"Scaling down workers: %d -> %d", currentCount, desiredCount)

		if r.hcloudClient != nil {
			cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseHealing
			toRemove := currentCount - desiredCount
			if err := r.scaleDownWorkers(ctx, cluster, toRemove); err != nil {
				logger.Error(err, "failed to scale down workers")
			}
		}
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	return ctrl.Result{}, nil
}

// scaleUpWorkers creates new worker nodes to reach the desired count.
func (r *ClusterReconciler) scaleUpWorkers(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, count int) error {
	logger := log.FromContext(ctx)

	prereqs, cleanup, err := r.prepareForProvisioning(ctx, cluster, "worker")
	if err != nil {
		return err
	}
	defer cleanup()

	created := 0
	for i := 0; i < count && created < r.maxConcurrentHeals; i++ {
		err := r.provisionAndConfigureNode(ctx, cluster, nodeProvisionParams{
			Name:          naming.Worker(cluster.Name),
			Role:          "worker",
			Pool:          "workers",
			ServerType:    normalizeServerSize(cluster.Spec.Workers.Size),
			SnapshotID:    prereqs.SnapshotID,
			SSHKeyName:    prereqs.SSHKeyName,
			NetworkID:     prereqs.ClusterState.NetworkID,
			MetricsReason: "scale-up",
			Configure: func(serverName string, result *serverProvisionResult) error {
				return r.configureWorkerNode(ctx, cluster, prereqs.TC, result)
			},
		})
		if err != nil {
			logger.Error(err, "failed to provision worker")
			continue
		}
		created++
	}

	if created < count {
		return fmt.Errorf("only created %d of %d requested workers", created, count)
	}

	return nil
}

// configureWorkerNode generates and applies Talos config to a worker node, then waits for readiness.
// Used by both scale-up and healing paths.
func (r *ClusterReconciler) configureWorkerNode(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, tc talosClients, result *serverProvisionResult) error {
	logger := log.FromContext(ctx)

	if tc.configGen == nil || tc.client == nil {
		logger.Info("skipping Talos config application (no credentials available)", "name", result.Name)
		r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
			Name: result.Name, Phase: k8znerv1alpha1.NodePhaseWaitingForK8s,
			Reason: "Waiting for node to join cluster (no Talos credentials)",
		})
		return nil
	}

	r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
		Name: result.Name, Phase: k8znerv1alpha1.NodePhaseApplyingTalosConfig,
		Reason: "Generating and applying Talos machine configuration",
	})

	machineConfig, err := tc.configGen.GenerateWorkerConfig(result.Name, result.ServerID)
	if err != nil {
		logger.Error(err, "failed to generate worker config", "name", result.Name)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
			"Failed to generate config for worker %s: %v", result.Name, err)
		r.handleProvisioningFailure(ctx, cluster, "worker", result.Name,
			fmt.Sprintf("Failed to generate Talos config: %v", err))
		return err
	}

	logger.Info("applying Talos config to worker", "name", result.Name, "ip", result.TalosIP)
	if err := tc.client.ApplyConfig(ctx, result.TalosIP, machineConfig); err != nil {
		logger.Error(err, "failed to apply Talos config", "name", result.Name)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
			"Failed to apply config to worker %s: %v", result.Name, err)
		r.handleProvisioningFailure(ctx, cluster, "worker", result.Name,
			fmt.Sprintf("Failed to apply Talos config: %v", err))
		return err
	}

	return r.waitForWorkerReady(ctx, cluster, result.Name, result.TalosIP)
}

// waitForWorkerReady waits for a worker node to become ready after Talos config is applied.
func (r *ClusterReconciler) waitForWorkerReady(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, serverName, talosIP string) error {
	logger := log.FromContext(ctx)

	r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
		Name: serverName, Phase: k8znerv1alpha1.NodePhaseRebootingWithConfig,
		Reason: "Talos config applied, node is rebooting with new configuration",
	})

	logger.Info("waiting for worker node to become ready", "name", serverName, "ip", talosIP)
	r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
		Name: serverName, Phase: k8znerv1alpha1.NodePhaseWaitingForK8s,
		Reason: "Waiting for kubelet to register node with Kubernetes",
	})

	if err := r.nodeReadyWaiter(ctx, serverName, nodeReadyTimeout); err != nil {
		logger.Error(err, "worker node not ready in time", "name", serverName)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonNodeReadyTimeout,
			"Worker node %s not ready in time: %v", serverName, err)
		r.handleProvisioningFailure(ctx, cluster, "worker", serverName,
			fmt.Sprintf("Node not ready in time: %v", err))
		return err
	}

	r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
		Name: serverName, Phase: k8znerv1alpha1.NodePhaseNodeInitializing,
		Reason: "Kubelet running, waiting for CNI and system pods",
	})

	return nil
}

// scaleDownWorkers removes excess worker nodes to reach the desired count.
func (r *ClusterReconciler) scaleDownWorkers(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, count int) error {
	logger := log.FromContext(ctx)

	workersToRemove := r.selectWorkersForRemoval(cluster, count)
	if len(workersToRemove) == 0 {
		logger.Info("no workers to remove")
		return nil
	}

	logger.Info("removing workers", "count", len(workersToRemove))

	removed := 0
	for _, worker := range workersToRemove {
		if removed >= r.maxConcurrentHeals {
			break
		}

		if err := r.decommissionWorker(ctx, cluster, worker); err != nil {
			logger.Error(err, "failed to decommission worker", "name", worker.Name)
			continue
		}
		removed++
	}

	r.removeWorkersFromStatus(cluster, workersToRemove[:removed])

	if removed < count {
		return fmt.Errorf("only removed %d of %d workers", removed, count)
	}

	return nil
}

// decommissionWorker cordons, drains, and deletes a single worker node and its server.
func (r *ClusterReconciler) decommissionWorker(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, worker *k8znerv1alpha1.NodeStatus) error {
	logger := log.FromContext(ctx)

	logger.Info("removing worker", "name", worker.Name, "serverID", worker.ServerID)
	startTime := time.Now()

	// Cordon the node
	k8sNode := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: worker.Name}, k8sNode); err == nil {
		if !k8sNode.Spec.Unschedulable {
			k8sNode.Spec.Unschedulable = true
			if err := r.Update(ctx, k8sNode); err != nil {
				logger.Error(err, "failed to cordon node", "node", worker.Name)
			} else {
				logger.Info("cordoned node", "node", worker.Name)
			}
		}
	}

	// Drain the node (evict pods)
	if err := r.drainNode(ctx, worker.Name); err != nil {
		logger.Error(err, "failed to drain node", "node", worker.Name)
	}

	// Delete the Kubernetes node object
	if err := r.Get(ctx, types.NamespacedName{Name: worker.Name}, k8sNode); err == nil {
		if err := r.Delete(ctx, k8sNode); err != nil && !apierrors.IsNotFound(err) {
			logger.Error(err, "failed to delete k8s node", "node", worker.Name)
		} else {
			logger.Info("deleted kubernetes node", "node", worker.Name)
		}
	}

	// Delete the Hetzner server
	if worker.Name != "" {
		if err := r.hcloudClient.DeleteServer(ctx, worker.Name); err != nil {
			logger.Error(err, "failed to delete hetzner server", "name", worker.Name)
			r.recordHCloudAPICall("delete_server", "error", time.Since(startTime).Seconds())
			return fmt.Errorf("failed to delete hetzner server %s: %w", worker.Name, err)
		}
		logger.Info("deleted hetzner server", "name", worker.Name, "serverID", worker.ServerID)
		r.recordHCloudAPICall("delete_server", "success", time.Since(startTime).Seconds())
	}

	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonScalingDown,
		"Successfully removed worker %s", worker.Name)

	r.recordNodeReplacement(cluster.Name, "worker", "scale-down")
	r.recordNodeReplacementDuration(cluster.Name, "worker", time.Since(startTime).Seconds())

	return nil
}

// selectWorkersForRemoval selects workers to remove during scale-down.
// Priority: 1. Unhealthy workers, 2. Newest workers (by name, assuming newer names sort last)
func (r *ClusterReconciler) selectWorkersForRemoval(cluster *k8znerv1alpha1.K8znerCluster, count int) []*k8znerv1alpha1.NodeStatus {
	if count <= 0 || len(cluster.Status.Workers.Nodes) == 0 {
		return nil
	}

	var unhealthy []*k8znerv1alpha1.NodeStatus
	var healthy []*k8znerv1alpha1.NodeStatus

	for i := range cluster.Status.Workers.Nodes {
		node := &cluster.Status.Workers.Nodes[i]
		if !node.Healthy {
			unhealthy = append(unhealthy, node)
		} else {
			healthy = append(healthy, node)
		}
	}

	var selected []*k8znerv1alpha1.NodeStatus

	for _, node := range unhealthy {
		if len(selected) >= count {
			break
		}
		selected = append(selected, node)
	}

	for i := len(healthy) - 1; i >= 0 && len(selected) < count; i-- {
		selected = append(selected, healthy[i])
	}

	return selected
}

// removeWorkersFromStatus removes the specified workers from the cluster status.
func (r *ClusterReconciler) removeWorkersFromStatus(cluster *k8znerv1alpha1.K8znerCluster, removed []*k8znerv1alpha1.NodeStatus) {
	if len(removed) == 0 {
		return
	}

	toRemove := make(map[string]bool)
	for _, w := range removed {
		toRemove[w.Name] = true
	}

	var remaining []k8znerv1alpha1.NodeStatus
	for _, node := range cluster.Status.Workers.Nodes {
		if !toRemove[node.Name] {
			remaining = append(remaining, node)
		}
	}

	cluster.Status.Workers.Nodes = remaining
}
