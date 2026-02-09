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
	"github.com/imamik/k8zner/internal/util/labels"
)

// replaceControlPlane replaces an unhealthy control plane node.
func (r *ClusterReconciler) replaceControlPlane(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, node *k8znerv1alpha1.NodeStatus) error {
	logger := log.FromContext(ctx)

	// Use shared helper to load Talos clients (injected mocks or from credentials)
	tc := r.loadTalosClients(ctx, cluster)

	// Step 1: Remove from etcd cluster (via Talos API)
	r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
		Name:   node.Name,
		Phase:  k8znerv1alpha1.NodePhaseRemovingFromEtcd,
		Reason: "Removing etcd member before server deletion",
	})

	if tc.client != nil && node.PrivateIP != "" {
		// Get etcd members from a healthy control plane
		healthyIP := r.findHealthyControlPlaneIP(cluster)
		if healthyIP != "" {
			members, err := tc.client.GetEtcdMembers(ctx, healthyIP)
			if err != nil {
				logger.Error(err, "failed to get etcd members")
			} else {
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
		}
	} else {
		logger.Info("skipping etcd member removal (no credentials or no IP)")
	}

	// Step 2: Delete the Kubernetes node
	r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
		Name:   node.Name,
		Phase:  k8znerv1alpha1.NodePhaseDeletingServer,
		Reason: "Deleting Kubernetes node and HCloud server",
	})

	k8sNode := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: node.Name}, k8sNode); err == nil {
		if err := r.Delete(ctx, k8sNode); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete k8s node: %w", err)
		}
		logger.Info("deleted kubernetes node", "node", node.Name)
	}

	// Step 3: Delete the Hetzner server
	if node.Name != "" {
		startTime := time.Now()
		if err := r.hcloudClient.DeleteServer(ctx, node.Name); err != nil {
			logger.Error(err, "failed to delete hetzner server", "name", node.Name)
			if r.enableMetrics {
				RecordHCloudAPICall("delete_server", "error", time.Since(startTime).Seconds())
			}
			// Continue anyway - server might already be gone
		} else {
			logger.Info("deleted hetzner server", "name", node.Name, "serverID", node.ServerID)
			if r.enableMetrics {
				RecordHCloudAPICall("delete_server", "success", time.Since(startTime).Seconds())
			}
		}
	}

	// Remove old node from status
	r.removeNodeFromStatus(cluster, "control-plane", node.Name)

	// Step 4: Build cluster state for server creation
	clusterState, err := r.buildClusterState(ctx, cluster)
	if err != nil {
		logger.Error(err, "failed to build cluster state")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to build cluster state for control plane replacement: %v", err)
		return fmt.Errorf("failed to build cluster state: %w", err)
	}

	// Step 5: Get Talos snapshot for server creation
	snapshot, err := r.getSnapshot(ctx)
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Snapshot error for CP replacement: %v", err)
		return err
	}

	// Step 5.5: Create ephemeral SSH key to avoid Hetzner password emails
	sshKeyName, cleanupKey, err := r.createEphemeralSSHKey(ctx, cluster, "cp")
	if err != nil {
		return err
	}
	defer cleanupKey()

	// Step 6: Create new server using shared provisioning helper
	newServerName := r.generateReplacementServerName(cluster, "control-plane", node.Name)
	serverType := normalizeServerSize(cluster.Spec.ControlPlanes.Size)
	serverLabels := labels.NewLabelBuilder(cluster.Name).
		WithRole("control-plane").
		WithPool("control-plane").
		WithManagedBy(labels.ManagedByOperator).
		Build()

	result, err := r.provisionServer(ctx, cluster, serverCreateOpts{
		Name:       newServerName,
		SnapshotID: snapshot.ID,
		ServerType: serverType,
		Region:     cluster.Spec.Region,
		SSHKeyName: sshKeyName,
		Labels:     serverLabels,
		NetworkID:  clusterState.NetworkID,
		Role:       "control-plane",
	})
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to provision CP server %s: %v", newServerName, err)
		return err
	}

	// Persist status with server ID and IPs
	if err := r.updateNodePhaseAndPersist(ctx, cluster, "control-plane", NodeStatusUpdate{
		Name:      result.Name,
		ServerID:  result.ServerID,
		PublicIP:  result.PublicIP,
		PrivateIP: result.PrivateIP,
		Phase:     k8znerv1alpha1.NodePhaseWaitingForTalosAPI,
		Reason:    fmt.Sprintf("Waiting for Talos API on %s:50000", result.TalosIP),
	}); err != nil {
		logger.Error(err, "failed to persist node status", "name", result.Name)
	}

	// Step 9: Generate and apply Talos config (if talos clients are available)
	if tc.configGen != nil && tc.client != nil {
		r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
			Name:   result.Name,
			Phase:  k8znerv1alpha1.NodePhaseApplyingTalosConfig,
			Reason: "Generating and applying Talos machine configuration",
		})

		// Update SANs with new server IP
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

		// Step 10: Wait for node to become ready
		r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
			Name:   result.Name,
			Phase:  k8znerv1alpha1.NodePhaseRebootingWithConfig,
			Reason: "Talos config applied, node is rebooting with new configuration",
		})

		logger.Info("waiting for control plane node to become ready", "name", result.Name, "ip", result.TalosIP)
		r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
			Name:   result.Name,
			Phase:  k8znerv1alpha1.NodePhaseWaitingForK8s,
			Reason: "Waiting for kubelet to register node with Kubernetes",
		})

		if err := tc.client.WaitForNodeReady(ctx, result.TalosIP, int(nodeReadyTimeout.Seconds())); err != nil {
			logger.Error(err, "control plane node failed to become ready", "name", result.Name)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonNodeReadyTimeout,
				"Control plane %s failed to become ready: %v", result.Name, err)
			r.handleProvisioningFailure(ctx, cluster, "control-plane", result.Name,
				fmt.Sprintf("Node not ready in time: %v", err))
			return fmt.Errorf("node failed to become ready: %w", err)
		}

		// Node kubelet is running - transition to NodeInitializing
		// The state verifier will promote to Ready once K8s node is fully ready
		r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
			Name:   result.Name,
			Phase:  k8znerv1alpha1.NodePhaseNodeInitializing,
			Reason: "Kubelet running, waiting for CNI and system pods",
		})

		logger.Info("control plane node kubelet is running", "name", result.Name)
	} else {
		logger.Info("skipping Talos config application (no credentials available)", "name", result.Name)
		r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
			Name:   result.Name,
			Phase:  k8znerv1alpha1.NodePhaseWaitingForK8s,
			Reason: "Waiting for node to join cluster (no Talos credentials)",
		})
	}

	// Persist final status
	if err := r.persistClusterStatus(ctx, cluster); err != nil {
		logger.Error(err, "failed to persist cluster status", "name", result.Name)
	}

	return nil
}

// replaceWorker replaces an unhealthy worker node.
func (r *ClusterReconciler) replaceWorker(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, node *k8znerv1alpha1.NodeStatus) error {
	logger := log.FromContext(ctx)

	// Step 1: Cordon and drain the node
	r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
		Name:   node.Name,
		Phase:  k8znerv1alpha1.NodePhaseDraining,
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

	// Step 2: Drain the node (evict pods)
	if err := r.drainNode(ctx, node.Name); err != nil {
		logger.Error(err, "failed to drain node", "node", node.Name)
		// Continue with replacement anyway
	}

	// Step 3: Delete the Kubernetes node and server
	r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
		Name:   node.Name,
		Phase:  k8znerv1alpha1.NodePhaseDeletingServer,
		Reason: "Deleting Kubernetes node and HCloud server",
	})

	if err := r.Get(ctx, types.NamespacedName{Name: node.Name}, k8sNode); err == nil {
		if err := r.Delete(ctx, k8sNode); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete k8s node: %w", err)
		}
		logger.Info("deleted kubernetes node", "node", node.Name)
	}

	// Step 4: Delete the Hetzner server
	if node.Name != "" {
		startTime := time.Now()
		if err := r.hcloudClient.DeleteServer(ctx, node.Name); err != nil {
			logger.Error(err, "failed to delete hetzner server", "name", node.Name)
			if r.enableMetrics {
				RecordHCloudAPICall("delete_server", "error", time.Since(startTime).Seconds())
			}
			// Continue anyway - server might already be gone
		} else {
			logger.Info("deleted hetzner server", "name", node.Name, "serverID", node.ServerID)
			if r.enableMetrics {
				RecordHCloudAPICall("delete_server", "success", time.Since(startTime).Seconds())
			}
		}
	}

	// Remove old node from status
	r.removeNodeFromStatus(cluster, "worker", node.Name)

	// Step 5: Build cluster state for server creation
	clusterState, err := r.buildClusterState(ctx, cluster)
	if err != nil {
		logger.Error(err, "failed to build cluster state")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to build cluster state for worker replacement: %v", err)
		return fmt.Errorf("failed to build cluster state: %w", err)
	}

	// Step 5b: Use shared helper to load Talos clients (injected mocks or from credentials)
	tc := r.loadTalosClients(ctx, cluster)

	// Step 6: Get Talos snapshot for server creation
	snapshot, err := r.getSnapshot(ctx)
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Snapshot error for worker replacement: %v", err)
		return err
	}

	// Step 6.5: Create ephemeral SSH key to avoid Hetzner password emails
	sshKeyName, cleanupKey, err := r.createEphemeralSSHKey(ctx, cluster, "worker")
	if err != nil {
		return err
	}
	defer cleanupKey()

	// Step 7: Create new server using shared provisioning helper
	newServerName := r.generateReplacementServerName(cluster, "worker", node.Name)
	serverType := normalizeServerSize(cluster.Spec.Workers.Size)
	serverLabels := labels.NewLabelBuilder(cluster.Name).
		WithRole("worker").
		WithPool("workers").
		WithManagedBy(labels.ManagedByOperator).
		Build()

	result, err := r.provisionServer(ctx, cluster, serverCreateOpts{
		Name:       newServerName,
		SnapshotID: snapshot.ID,
		ServerType: serverType,
		Region:     cluster.Spec.Region,
		SSHKeyName: sshKeyName,
		Labels:     serverLabels,
		NetworkID:  clusterState.NetworkID,
		Role:       "worker",
	})
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to provision worker server %s: %v", newServerName, err)
		return err
	}

	// Persist status with server ID and IPs
	if err := r.updateNodePhaseAndPersist(ctx, cluster, "worker", NodeStatusUpdate{
		Name:      result.Name,
		ServerID:  result.ServerID,
		PublicIP:  result.PublicIP,
		PrivateIP: result.PrivateIP,
		Phase:     k8znerv1alpha1.NodePhaseWaitingForTalosAPI,
		Reason:    fmt.Sprintf("Waiting for Talos API on %s:50000", result.TalosIP),
	}); err != nil {
		logger.Error(err, "failed to persist node status", "name", result.Name)
	}

	// Step 10: Generate and apply Talos config (if talos clients are available)
	if tc.configGen != nil && tc.client != nil {
		r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
			Name:   result.Name,
			Phase:  k8znerv1alpha1.NodePhaseApplyingTalosConfig,
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

		// Step 11: Wait for node to become ready
		r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
			Name:   result.Name,
			Phase:  k8znerv1alpha1.NodePhaseRebootingWithConfig,
			Reason: "Talos config applied, node is rebooting with new configuration",
		})

		logger.Info("waiting for worker node to become ready", "name", result.Name, "ip", result.TalosIP)
		r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
			Name:   result.Name,
			Phase:  k8znerv1alpha1.NodePhaseWaitingForK8s,
			Reason: "Waiting for kubelet to register node with Kubernetes",
		})

		if err := tc.client.WaitForNodeReady(ctx, result.TalosIP, int(nodeReadyTimeout.Seconds())); err != nil {
			logger.Error(err, "worker node failed to become ready", "name", result.Name)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonNodeReadyTimeout,
				"Worker %s failed to become ready: %v", result.Name, err)
			r.handleProvisioningFailure(ctx, cluster, "worker", result.Name,
				fmt.Sprintf("Node not ready in time: %v", err))
			return fmt.Errorf("node failed to become ready: %w", err)
		}

		// Node kubelet is running - transition to NodeInitializing
		// The state verifier will promote to Ready once K8s node is fully ready
		r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
			Name:   result.Name,
			Phase:  k8znerv1alpha1.NodePhaseNodeInitializing,
			Reason: "Kubelet running, waiting for CNI and system pods",
		})

		logger.Info("worker node kubelet is running", "name", result.Name)
	} else {
		logger.Info("skipping Talos config application (no credentials available)", "name", result.Name)
		r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
			Name:   result.Name,
			Phase:  k8znerv1alpha1.NodePhaseWaitingForK8s,
			Reason: "Waiting for node to join cluster (no Talos credentials)",
		})
	}

	// Persist final status
	if err := r.persistClusterStatus(ctx, cluster); err != nil {
		logger.Error(err, "failed to persist cluster status", "name", result.Name)
	}

	return nil
}
