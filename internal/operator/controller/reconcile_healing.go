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
	"github.com/imamik/k8zner/internal/config"
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

	r.updateNodePhase(ctx, cluster, "control-plane", nodeStatusUpdate{
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

	r.updateNodePhase(ctx, cluster, role, nodeStatusUpdate{
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
		ServerType: string(config.ServerSize(cluster.Spec.ControlPlanes.Size).Normalize()),
		SnapshotID: prereqs.SnapshotID,
		SSHKeyName: prereqs.SSHKeyName,
		NetworkID:  prereqs.ClusterState.NetworkID,
		Configure: func(serverName string, result *serverProvisionResult) error {
			return r.configureCPNode(ctx, cluster, prereqs.ClusterState, prereqs.TC, result)
		},
	})
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

	r.updateNodePhase(ctx, cluster, "worker", nodeStatusUpdate{
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
		ServerType: string(config.ServerSize(cluster.Spec.Workers.Size).Normalize()),
		SnapshotID: prereqs.SnapshotID,
		SSHKeyName: prereqs.SSHKeyName,
		NetworkID:  prereqs.ClusterState.NetworkID,
		Configure: func(serverName string, result *serverProvisionResult) error {
			return r.configureWorkerNode(ctx, cluster, prereqs.TC, result)
		},
	})
}
