package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/util/naming"
)

// replaceControlPlane replaces an unhealthy control plane node.
func (r *ClusterReconciler) replaceControlPlane(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, node *k8znerv1alpha1.NodeStatus) error {
	tc := r.loadTalosClients(ctx, cluster)

	// Remove from etcd cluster, delete K8s node and HCloud server
	r.removeFromEtcd(ctx, cluster, tc, node)
	if err := r.deleteNodeAndServer(ctx, cluster, node, "control-plane"); err != nil {
		return err
	}
	r.removeNodeFromStatus(cluster, "control-plane", node.Name)

	// Provision replacement
	return r.provisionReplacementCP(ctx, cluster, node)
}

// removeFromEtcd removes the unhealthy node's etcd member via the Talos API.
func (r *ClusterReconciler) removeFromEtcd(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, tc talosClients, node *k8znerv1alpha1.NodeStatus) {
	logger := log.FromContext(ctx)

	r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
		Name: node.Name, Phase: k8znerv1alpha1.NodePhaseRemovingFromEtcd,
		Reason: "Removing etcd member before server deletion",
	})

	if tc.client == nil || node.PrivateIP == "" {
		logger.Info("skipping etcd member removal (no credentials or no IP)")
		return
	}

	healthyIP := r.findHealthyControlPlaneIP(cluster)
	if healthyIP == "" {
		return
	}

	members, err := tc.client.GetEtcdMembers(ctx, healthyIP)
	if err != nil {
		logger.Error(err, "failed to get etcd members")
		return
	}

	for _, member := range members {
		if member.Name == node.Name || member.Endpoint == node.PrivateIP {
			if err := tc.client.RemoveEtcdMember(ctx, healthyIP, member.ID); err != nil {
				logger.Error(err, "failed to remove etcd member", "member", member.Name)
			} else {
				logger.Info("removed etcd member", "member", member.Name)
			}
			break
		}
	}
}

// deleteNodeAndServer deletes the Kubernetes node object and the Hetzner server.
func (r *ClusterReconciler) deleteNodeAndServer(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, node *k8znerv1alpha1.NodeStatus, role string) error {
	logger := log.FromContext(ctx)

	r.updateNodePhase(ctx, cluster, role, NodeStatusUpdate{
		Name: node.Name, Phase: k8znerv1alpha1.NodePhaseDeletingServer,
		Reason: "Deleting Kubernetes node and HCloud server",
	})

	k8sNode := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: node.Name}, k8sNode); err == nil {
		if err := r.Delete(ctx, k8sNode); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete k8s node: %w", err)
		}
		logger.Info("deleted kubernetes node", "node", node.Name)
	}

	if node.Name != "" {
		startTime := time.Now()
		if err := r.hcloudClient.DeleteServer(ctx, node.Name); err != nil {
			logger.Error(err, "failed to delete hetzner server", "name", node.Name)
			r.recordHCloudAPICall("delete_server", "error", time.Since(startTime).Seconds())
			// Continue anyway - server might already be gone
		} else {
			logger.Info("deleted hetzner server", "name", node.Name, "serverID", node.ServerID)
			r.recordHCloudAPICall("delete_server", "success", time.Since(startTime).Seconds())
		}
	}

	return nil
}

// provisionReplacementCP creates a new control plane server to replace the deleted one.
func (r *ClusterReconciler) provisionReplacementCP(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, oldNode *k8znerv1alpha1.NodeStatus) error {
	prereqs, cleanup, err := r.prepareForProvisioning(ctx, cluster, "cp")
	if err != nil {
		return err
	}
	defer cleanup()

	return r.provisionAndConfigureNode(ctx, cluster, nodeProvisionParams{
		Name:       naming.ControlPlane(cluster.Name),
		Role:       "control-plane",
		Pool:       "control-plane",
		ServerType: normalizeServerSize(cluster.Spec.ControlPlanes.Size),
		SnapshotID: prereqs.SnapshotID,
		SSHKeyName: prereqs.SSHKeyName,
		NetworkID:  prereqs.ClusterState.NetworkID,
		Configure: func(serverName string, result *serverProvisionResult) error {
			return r.configureReplacementCP(ctx, cluster, prereqs.ClusterState, prereqs.TC, result)
		},
	})
}

// configureReplacementCP applies Talos config and waits for a replacement CP to become ready.
func (r *ClusterReconciler) configureReplacementCP(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, clusterState *ClusterState, tc talosClients, result *serverProvisionResult) error {
	logger := log.FromContext(ctx)

	if tc.configGen == nil || tc.client == nil {
		logger.Info("skipping Talos config application (no credentials available)", "name", result.Name)
		r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
			Name: result.Name, Phase: k8znerv1alpha1.NodePhaseWaitingForK8s,
			Reason: "Waiting for node to join cluster (no Talos credentials)",
		})
		return nil
	}

	r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
		Name: result.Name, Phase: k8znerv1alpha1.NodePhaseApplyingTalosConfig,
		Reason: "Generating and applying Talos machine configuration",
	})

	sans := append([]string{}, clusterState.SANs...)
	sans = append(sans, result.PublicIP)

	machineConfig, err := tc.configGen.GenerateControlPlaneConfig(sans, result.Name, result.ServerID)
	if err != nil {
		logger.Error(err, "failed to generate control plane config", "name", result.Name)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
			"Failed to generate config for control plane %s: %v", result.Name, err)
		r.handleProvisioningFailure(ctx, cluster, "control-plane", result.Name,
			fmt.Sprintf("Failed to generate Talos config: %v", err))
		return fmt.Errorf("failed to generate config: %w", err)
	}

	logger.Info("applying Talos config to control plane", "name", result.Name, "ip", result.TalosIP)
	if err := tc.client.ApplyConfig(ctx, result.TalosIP, machineConfig); err != nil {
		logger.Error(err, "failed to apply Talos config", "name", result.Name)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
			"Failed to apply config to control plane %s: %v", result.Name, err)
		r.handleProvisioningFailure(ctx, cluster, "control-plane", result.Name,
			fmt.Sprintf("Failed to apply Talos config: %v", err))
		return fmt.Errorf("failed to apply config: %w", err)
	}

	return r.waitForReplacementNodeReady(ctx, cluster, "control-plane", tc, result)
}

// replaceWorker replaces an unhealthy worker node.
func (r *ClusterReconciler) replaceWorker(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, node *k8znerv1alpha1.NodeStatus) error {
	// Drain and delete old worker
	if err := r.drainAndDeleteWorker(ctx, cluster, node); err != nil {
		return err
	}
	r.removeNodeFromStatus(cluster, "worker", node.Name)

	// Provision replacement
	return r.provisionReplacementWorker(ctx, cluster, node)
}

// drainAndDeleteWorker cordons, drains, and deletes a worker node and its server.
func (r *ClusterReconciler) drainAndDeleteWorker(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, node *k8znerv1alpha1.NodeStatus) error {
	logger := log.FromContext(ctx)

	r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
		Name: node.Name, Phase: k8znerv1alpha1.NodePhaseDraining,
		Reason: "Cordoning and draining node before replacement",
	})

	k8sNode := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: node.Name}, k8sNode); err == nil {
		if !k8sNode.Spec.Unschedulable {
			k8sNode.Spec.Unschedulable = true
			if err := r.Update(ctx, k8sNode); err != nil {
				logger.Error(err, "failed to cordon node", "node", node.Name)
			} else {
				logger.Info("cordoned node", "node", node.Name)
			}
		}
	}

	if err := r.drainNode(ctx, node.Name); err != nil {
		logger.Error(err, "failed to drain node", "node", node.Name)
		// Continue with replacement anyway
	}

	return r.deleteNodeAndServer(ctx, cluster, node, "worker")
}

// provisionReplacementWorker creates a new worker server to replace the deleted one.
func (r *ClusterReconciler) provisionReplacementWorker(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, oldNode *k8znerv1alpha1.NodeStatus) error {
	prereqs, cleanup, err := r.prepareForProvisioning(ctx, cluster, "worker")
	if err != nil {
		return err
	}
	defer cleanup()

	return r.provisionAndConfigureNode(ctx, cluster, nodeProvisionParams{
		Name:       naming.Worker(cluster.Name),
		Role:       "worker",
		Pool:       "workers",
		ServerType: normalizeServerSize(cluster.Spec.Workers.Size),
		SnapshotID: prereqs.SnapshotID,
		SSHKeyName: prereqs.SSHKeyName,
		NetworkID:  prereqs.ClusterState.NetworkID,
		Configure: func(serverName string, result *serverProvisionResult) error {
			return r.configureReplacementWorker(ctx, cluster, prereqs.TC, result)
		},
	})
}

// configureReplacementWorker applies Talos config and waits for a replacement worker to become ready.
func (r *ClusterReconciler) configureReplacementWorker(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, tc talosClients, result *serverProvisionResult) error {
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
		return fmt.Errorf("failed to generate config: %w", err)
	}

	logger.Info("applying Talos config to worker", "name", result.Name, "ip", result.TalosIP)
	if err := tc.client.ApplyConfig(ctx, result.TalosIP, machineConfig); err != nil {
		logger.Error(err, "failed to apply Talos config", "name", result.Name)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
			"Failed to apply config to worker %s: %v", result.Name, err)
		r.handleProvisioningFailure(ctx, cluster, "worker", result.Name,
			fmt.Sprintf("Failed to apply Talos config: %v", err))
		return fmt.Errorf("failed to apply config: %w", err)
	}

	return r.waitForReplacementNodeReady(ctx, cluster, "worker", tc, result)
}

// waitForReplacementNodeReady waits for a replacement node (CP or worker) to become ready.
func (r *ClusterReconciler) waitForReplacementNodeReady(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, role string, tc talosClients, result *serverProvisionResult) error {
	logger := log.FromContext(ctx)

	r.updateNodePhase(ctx, cluster, role, NodeStatusUpdate{
		Name: result.Name, Phase: k8znerv1alpha1.NodePhaseRebootingWithConfig,
		Reason: "Talos config applied, node is rebooting with new configuration",
	})

	logger.Info("waiting for node to become ready", "role", role, "name", result.Name, "ip", result.TalosIP)
	r.updateNodePhase(ctx, cluster, role, NodeStatusUpdate{
		Name: result.Name, Phase: k8znerv1alpha1.NodePhaseWaitingForK8s,
		Reason: "Waiting for kubelet to register node with Kubernetes",
	})

	if err := tc.client.WaitForNodeReady(ctx, result.TalosIP, int(nodeReadyTimeout.Seconds())); err != nil {
		logger.Error(err, "node failed to become ready", "role", role, "name", result.Name)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonNodeReadyTimeout,
			"%s %s failed to become ready: %v", role, result.Name, err)
		r.handleProvisioningFailure(ctx, cluster, role, result.Name,
			fmt.Sprintf("Node not ready in time: %v", err))
		return fmt.Errorf("node failed to become ready: %w", err)
	}

	r.updateNodePhase(ctx, cluster, role, NodeStatusUpdate{
		Name: result.Name, Phase: k8znerv1alpha1.NodePhaseNodeInitializing,
		Reason: "Kubelet running, waiting for CNI and system pods",
	})
	logger.Info("node kubelet is running", "role", role, "name", result.Name)

	return nil
}
