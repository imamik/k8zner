package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/util/labels"
	"github.com/imamik/k8zner/internal/util/naming"
)

// reconcileControlPlanes ensures control planes are healthy and at the desired count.
func (r *ClusterReconciler) reconcileControlPlanes(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	currentCount := len(cluster.Status.ControlPlanes.Nodes)
	desiredCount := cluster.Spec.ControlPlanes.Count

	// Skip scaling if CPs are already provisioning to prevent duplicate server creation
	provisioningCount := countWorkersInEarlyProvisioning(cluster.Status.ControlPlanes.Nodes)
	if provisioningCount > 0 {
		logger.Info("control planes currently provisioning, skipping scaling check",
			"provisioning", provisioningCount,
			"current", currentCount,
			"desired", desiredCount,
		)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	if currentCount < desiredCount {
		return r.handleCPScaleUp(ctx, cluster, currentCount, desiredCount)
	}

	// Skip health-based replacement if single CP (no HA replacement possible)
	if cluster.Spec.ControlPlanes.Count == 1 {
		return ctrl.Result{}, nil
	}

	return r.replaceUnhealthyCPIfNeeded(ctx, cluster)
}

// handleCPScaleUp triggers control plane scale-up when current < desired.
func (r *ClusterReconciler) handleCPScaleUp(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, currentCount, desiredCount int) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("scaling up control planes", "current", currentCount, "desired", desiredCount)
	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonScalingUp,
		"Scaling up control planes: %d -> %d", currentCount, desiredCount)

	if r.hcloudClient != nil {
		cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseHealing
		toCreate := desiredCount - currentCount
		if err := r.scaleUpControlPlanes(ctx, cluster, toCreate); err != nil {
			logger.Error(err, "failed to scale up control planes")
		}
	}
	return ctrl.Result{RequeueAfter: fastRequeueAfter}, nil
}

// replaceUnhealthyCPIfNeeded finds an unhealthy CP past the threshold and replaces it.
func (r *ClusterReconciler) replaceUnhealthyCPIfNeeded(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	threshold := parseThreshold(cluster.Spec.HealthCheck, "etcd")
	unhealthyCP := findUnhealthyNode(cluster.Status.ControlPlanes.Nodes, threshold)
	if unhealthyCP == nil {
		return ctrl.Result{}, nil
	}

	// Check if we have quorum to replace
	healthyCPs := cluster.Status.ControlPlanes.Ready
	totalCPs := cluster.Spec.ControlPlanes.Count
	quorumNeeded := (totalCPs / 2) + 1

	if healthyCPs < quorumNeeded {
		logger.Error(nil, "cannot replace control plane - quorum would be lost",
			"healthy", healthyCPs,
			"needed", quorumNeeded,
		)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonQuorumLost,
			"Cannot replace control plane %s: only %d/%d healthy, need %d for quorum",
			unhealthyCP.Name, healthyCPs, totalCPs, quorumNeeded)

		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:    k8znerv1alpha1.ConditionControlPlaneReady,
			Status:  metav1.ConditionFalse,
			Reason:  "QuorumLost",
			Message: fmt.Sprintf("Only %d/%d control planes healthy, need %d for quorum", healthyCPs, totalCPs, quorumNeeded),
		})
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Replace the unhealthy control plane
	logger.Info("replacing unhealthy control plane",
		"node", unhealthyCP.Name,
		"serverID", unhealthyCP.ServerID,
		"unhealthySince", unhealthyCP.UnhealthySince,
	)

	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonNodeReplacing,
		"Replacing unhealthy control plane %s (unhealthy since %v)",
		unhealthyCP.Name, unhealthyCP.UnhealthySince)

	cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseHealing

	startTime := time.Now()
	if err := r.replaceControlPlane(ctx, cluster, unhealthyCP); err != nil {
		r.recordNodeReplacement(cluster.Name, "control-plane", unhealthyCP.UnhealthyReason)
		return ctrl.Result{}, fmt.Errorf("failed to replace control plane: %w", err)
	}

	r.recordNodeReplacement(cluster.Name, "control-plane", unhealthyCP.UnhealthyReason)
	r.recordNodeReplacementDuration(cluster.Name, "control-plane", time.Since(startTime).Seconds())

	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonNodeReplaced,
		"Successfully replaced control plane %s", unhealthyCP.Name)

	return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
}

// findUnhealthyNode returns the first node that has been unhealthy past the given threshold.
func findUnhealthyNode(nodes []k8znerv1alpha1.NodeStatus, threshold time.Duration) *k8znerv1alpha1.NodeStatus {
	if unhealthy := findUnhealthyNodes(nodes, threshold); len(unhealthy) > 0 {
		return unhealthy[0]
	}
	return nil
}

// scaleUpControlPlanes creates new control plane nodes to reach the desired count.
// Servers are created in parallel for speed, but Talos configs are applied sequentially
// because etcd membership requires ordered joining.
func (r *ClusterReconciler) scaleUpControlPlanes(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, count int) error {
	logger := log.FromContext(ctx)

	prereqs, cleanup, err := r.prepareForProvisioning(ctx, cluster, "cp")
	if err != nil {
		return err
	}
	defer cleanup()

	serverType := normalizeServerSize(cluster.Spec.ControlPlanes.Size)
	serverLabels := labels.NewLabelBuilder(cluster.Name).
		WithRole("control-plane").
		WithPool("control-plane").
		WithManagedBy(labels.ManagedByOperator).
		Build()

	// Phase 1: Create all servers in parallel
	type serverResult struct {
		name   string
		result *serverProvisionResult
		err    error
	}
	resultCh := make(chan serverResult, count)

	for i := 0; i < count; i++ {
		name := naming.ControlPlane(cluster.Name)
		go func() {
			result, err := r.provisionServer(ctx, cluster, serverCreateOpts{
				Name:       name,
				SnapshotID: prereqs.SnapshotID,
				ServerType: serverType,
				Region:     cluster.Spec.Region,
				SSHKeyName: prereqs.SSHKeyName,
				Labels:     serverLabels,
				NetworkID:  prereqs.ClusterState.NetworkID,
				Role:       "control-plane",
			})
			resultCh <- serverResult{name: name, result: result, err: err}
		}()
	}

	// Collect results
	var servers []serverResult
	for range count {
		res := <-resultCh
		if res.err != nil {
			logger.Error(res.err, "failed to create CP server", "name", res.name)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
				"Failed to create CP server %s: %v", res.name, res.err)
		}
		servers = append(servers, res)
	}

	// Persist status after all servers created
	if err := r.persistClusterStatus(ctx, cluster); err != nil {
		logger.Error(err, "failed to persist status after parallel server creation")
	}

	// Phase 2: Apply Talos configs sequentially (etcd requires ordered joining)
	configured := 0
	for _, srv := range servers {
		if srv.err != nil {
			continue
		}

		if err := r.updateNodePhaseAndPersist(ctx, cluster, "control-plane", nodeStatusUpdate{
			Name:      srv.name,
			ServerID:  srv.result.ServerID,
			PublicIP:  srv.result.PublicIP,
			PrivateIP: srv.result.PrivateIP,
			Phase:     k8znerv1alpha1.NodePhaseWaitingForTalosAPI,
			Reason:    fmt.Sprintf("Waiting for Talos API on %s:50000", srv.result.TalosIP),
		}); err != nil {
			logger.Error(err, "failed to persist node status", "name", srv.name)
		}

		if err := r.configureCPNode(ctx, cluster, prereqs.ClusterState, prereqs.TC, srv.result); err != nil {
			logger.Error(err, "failed to configure CP", "name", srv.name)
			// After Talos config applied, server must be preserved (etcd safety)
			if configured == 0 {
				return err
			}
			return fmt.Errorf("configured %d of %d CPs, failed on %s: %w", configured, count, srv.name, err)
		}

		if err := r.persistClusterStatus(ctx, cluster); err != nil {
			logger.Error(err, "failed to persist cluster status", "name", srv.name)
		}

		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonScalingUp,
			"Successfully created control-plane %s", srv.name)
		r.recordNodeReplacement(cluster.Name, "control-plane", "scale-up")
		configured++
	}

	if configured < count {
		return fmt.Errorf("only created %d of %d requested control planes", configured, count)
	}

	return nil
}

// configureCPNode generates and applies Talos config to a CP node, then waits for it to become ready.
// Used by both scale-up and healing paths. Returns a fatal error if the node has joined etcd but is
// not ready (server must be preserved to avoid breaking etcd quorum).
func (r *ClusterReconciler) configureCPNode(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, clusterState *clusterState, tc talosClients, result *serverProvisionResult) error {
	logger := log.FromContext(ctx)

	if tc.configGen == nil || tc.client == nil {
		logger.Info("skipping Talos config application (no credentials available)", "name", result.Name)
		r.updateNodePhase(ctx, cluster, "control-plane", nodeStatusUpdate{
			Name: result.Name, Phase: k8znerv1alpha1.NodePhaseWaitingForK8s,
			Reason: "Waiting for node to join cluster (no Talos credentials)",
		})
		return nil
	}

	r.updateNodePhase(ctx, cluster, "control-plane", nodeStatusUpdate{
		Name: result.Name, Phase: k8znerv1alpha1.NodePhaseApplyingTalosConfig,
		Reason: "Generating and applying Talos machine configuration",
	})

	sans := append([]string{}, clusterState.SANs...)
	sans = append(sans, result.PublicIP)

	machineConfig, err := tc.configGen.GenerateControlPlaneConfig(sans, result.Name, result.ServerID)
	if err != nil {
		logger.Error(err, "failed to generate CP config", "name", result.Name)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
			"Failed to generate config for CP %s: %v", result.Name, err)
		r.handleProvisioningFailure(ctx, cluster, "control-plane", result.Name,
			fmt.Sprintf("Failed to generate Talos config: %v", err))
		return err
	}

	logger.Info("applying Talos config to CP", "name", result.Name, "ip", result.TalosIP)
	if err := tc.client.ApplyConfig(ctx, result.TalosIP, machineConfig); err != nil {
		logger.Error(err, "failed to apply Talos config", "name", result.Name)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
			"Failed to apply config to CP %s: %v", result.Name, err)
		// Safe to delete: config was never applied, so etcd member wasn't added
		r.handleProvisioningFailure(ctx, cluster, "control-plane", result.Name,
			fmt.Sprintf("Failed to apply Talos config: %v", err))
		return err
	}

	// CRITICAL: After Talos config is applied, the node starts joining etcd.
	// We must NOT delete this server on failure, as removing an etcd member
	// that was added but is unreachable can break etcd quorum.

	return r.waitForCPReady(ctx, cluster, tc, result.Name, result.TalosIP)
}

// waitForCPReady waits for a control plane node to become ready after Talos config is applied.
// If the node is not ready, the server is preserved (etcd member already added).
func (r *ClusterReconciler) waitForCPReady(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, tc talosClients, serverName, talosIP string) error {
	logger := log.FromContext(ctx)

	r.updateNodePhase(ctx, cluster, "control-plane", nodeStatusUpdate{
		Name: serverName, Phase: k8znerv1alpha1.NodePhaseRebootingWithConfig,
		Reason: "Talos config applied, node is rebooting with new configuration",
	})

	logger.Info("waiting for CP node to become ready", "name", serverName, "ip", talosIP)
	r.updateNodePhase(ctx, cluster, "control-plane", nodeStatusUpdate{
		Name: serverName, Phase: k8znerv1alpha1.NodePhaseWaitingForK8s,
		Reason: "Waiting for kubelet to register node with Kubernetes",
	})

	if err := tc.client.WaitForNodeReady(ctx, talosIP, int(nodeReadyTimeout.Seconds())); err != nil {
		// DO NOT delete the server - etcd member is already added.
		// Deleting would break etcd quorum. Leave server running;
		// the next reconcile will detect the node and retry or handle it.
		logger.Error(err, "CP node not ready yet, will retry on next reconcile",
			"name", serverName, "ip", talosIP)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonNodeReadyTimeout,
			"CP %s not ready yet (will retry): %v", serverName, err)
		r.updateNodePhase(ctx, cluster, "control-plane", nodeStatusUpdate{
			Name: serverName, Phase: k8znerv1alpha1.NodePhaseWaitingForK8s,
			Reason: fmt.Sprintf("Node not ready yet, will retry: %v", err),
		})
		return fmt.Errorf("CP %s not ready yet after config applied (etcd member added, server preserved): %w", serverName, err)
	}

	r.updateNodePhase(ctx, cluster, "control-plane", nodeStatusUpdate{
		Name: serverName, Phase: k8znerv1alpha1.NodePhaseNodeInitializing,
		Reason: "Kubelet running, waiting for CNI and system pods",
	})
	logger.Info("CP node kubelet is running", "name", serverName)

	return nil
}
