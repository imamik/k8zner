package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/util/labels"
	"github.com/imamik/k8zner/internal/util/naming"
)

// reconcileControlPlanes ensures control planes are healthy and at the desired count.
func (r *ClusterReconciler) reconcileControlPlanes(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Check if scaling up is needed (before health checks, since new CPs won't be in status yet)
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
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if currentCount < desiredCount {
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
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Skip health-based replacement if single CP (no HA replacement possible)
	if cluster.Spec.ControlPlanes.Count == 1 {
		return ctrl.Result{}, nil
	}

	// Find unhealthy control planes
	threshold := parseThreshold(cluster.Spec.HealthCheck, "etcd")
	var unhealthyCP *k8znerv1alpha1.NodeStatus

	for i := range cluster.Status.ControlPlanes.Nodes {
		node := &cluster.Status.ControlPlanes.Nodes[i]
		if !node.Healthy && node.UnhealthySince != nil {
			if time.Since(node.UnhealthySince.Time) > threshold {
				unhealthyCP = node
				break // Only handle one at a time
			}
		}
	}

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
		if r.enableMetrics {
			RecordNodeReplacement(cluster.Name, "control-plane", unhealthyCP.UnhealthyReason)
		}
		return ctrl.Result{}, fmt.Errorf("failed to replace control plane: %w", err)
	}

	if r.enableMetrics {
		RecordNodeReplacement(cluster.Name, "control-plane", unhealthyCP.UnhealthyReason)
		RecordNodeReplacementDuration(cluster.Name, "control-plane", time.Since(startTime).Seconds())
	}

	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonNodeReplaced,
		"Successfully replaced control plane %s", unhealthyCP.Name)

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// reconcileWorkers ensures workers are healthy and at the desired count.
func (r *ClusterReconciler) reconcileWorkers(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	threshold := parseThreshold(cluster.Spec.HealthCheck, "node")

	// Find unhealthy workers that need replacement
	var unhealthyWorkers []*k8znerv1alpha1.NodeStatus
	for i := range cluster.Status.Workers.Nodes {
		node := &cluster.Status.Workers.Nodes[i]
		if !node.Healthy && node.UnhealthySince != nil {
			if time.Since(node.UnhealthySince.Time) > threshold {
				unhealthyWorkers = append(unhealthyWorkers, node)
			}
		}
	}

	// Replace unhealthy workers (respect maxConcurrentHeals)
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

		if r.enableMetrics {
			RecordNodeReplacement(cluster.Name, "worker", worker.UnhealthyReason)
			RecordNodeReplacementDuration(cluster.Name, "worker", time.Since(startTime).Seconds())
		}

		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonNodeReplaced,
			"Successfully replaced worker %s", worker.Name)
		replaced++
	}

	// Check if scaling is needed
	currentCount := len(cluster.Status.Workers.Nodes)
	provisioningCount := countWorkersInEarlyProvisioning(cluster.Status.Workers.Nodes)
	desiredCount := cluster.Spec.Workers.Count

	// Skip scaling if workers are already provisioning to prevent duplicate server creation
	// from concurrent reconciles seeing stale status
	if provisioningCount > 0 {
		logger.Info("workers currently provisioning, skipping scaling check",
			"provisioning", provisioningCount,
			"current", currentCount,
			"desired", desiredCount,
		)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if currentCount < desiredCount {
		logger.Info("scaling up workers", "current", currentCount, "desired", desiredCount)
		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonScalingUp,
			"Scaling up workers: %d -> %d", currentCount, desiredCount)

		// Only attempt scaling if HCloud client is configured
		if r.hcloudClient != nil {
			cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseHealing
			toCreate := desiredCount - currentCount
			if err := r.scaleUpWorkers(ctx, cluster, toCreate); err != nil {
				logger.Error(err, "failed to scale up workers")
				// Continue to allow status update, will retry on next reconcile
			}
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	} else if currentCount > desiredCount {
		logger.Info("scaling down workers", "current", currentCount, "desired", desiredCount)
		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonScalingDown,
			"Scaling down workers: %d -> %d", currentCount, desiredCount)

		// Only attempt scaling if HCloud client is configured
		if r.hcloudClient != nil {
			cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseHealing
			toRemove := currentCount - desiredCount
			if err := r.scaleDownWorkers(ctx, cluster, toRemove); err != nil {
				logger.Error(err, "failed to scale down workers")
				// Continue to allow status update, will retry on next reconcile
			}
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if len(unhealthyWorkers) > 0 {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// scaleUpControlPlanes creates new control plane nodes to reach the desired count.
// This is called during initial cluster setup when the operator needs to add CP-2, CP-3, etc.
// after CP-1 was bootstrapped by the CLI. Follows the same pattern as replaceControlPlane
// but without etcd member removal or old server deletion.
func (r *ClusterReconciler) scaleUpControlPlanes(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, count int) error {
	logger := log.FromContext(ctx)

	// Build cluster state for server creation
	clusterState, err := r.buildClusterState(ctx, cluster)
	if err != nil {
		logger.Error(err, "failed to build cluster state")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to build cluster state for CP scaling: %v", err)
		return fmt.Errorf("failed to build cluster state: %w", err)
	}

	// Discover LB info if needed, then load Talos clients.
	// LB discovery must happen BEFORE loadTalosClients because the Talos generator
	// needs the control plane endpoint (LB IP) to generate valid configs.
	if r.talosClient == nil && cluster.Spec.CredentialsRef.Name != "" {
		creds, err := r.phaseAdapter.LoadCredentials(ctx, cluster)
		if err == nil {
			r.discoverLoadBalancerInfo(ctx, cluster, creds.HCloudToken)
		}
	}
	tc := r.loadTalosClients(ctx, cluster)

	// Get Talos snapshot for server creation
	snapshot, err := r.getSnapshot(ctx)
	if err != nil {
		logger.Error(err, "failed to get Talos snapshot")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Talos snapshot not found for CP scaling")
		return err
	}

	// Create ephemeral SSH key
	sshKeyName, cleanupKey, err := r.createEphemeralSSHKey(ctx, cluster, "cp")
	if err != nil {
		return err
	}
	defer cleanupKey()

	// Create control planes one at a time
	created := 0
	for i := 0; i < count; i++ {
		if err := r.provisionAndConfigureCP(ctx, cluster, clusterState, tc, snapshot.ID, sshKeyName); err != nil {
			logger.Error(err, "failed to provision control plane")
			// provisionAndConfigureCP returns a fatal error for etcd-safety reasons
			if created == 0 {
				return err
			}
			return fmt.Errorf("only created %d of %d requested control planes: %w", created, count, err)
		}
		created++
	}

	if created < count {
		return fmt.Errorf("only created %d of %d requested control planes", created, count)
	}

	return nil
}

// provisionAndConfigureCP provisions a single control plane server, applies Talos config, and waits for readiness.
func (r *ClusterReconciler) provisionAndConfigureCP(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, clusterState *ClusterState, tc talosClients, snapshotID int64, sshKeyName string) error {
	logger := log.FromContext(ctx)

	newServerName := naming.ControlPlane(cluster.Name)
	serverLabels := labels.NewLabelBuilder(cluster.Name).
		WithRole("control-plane").
		WithPool("control-plane").
		WithManagedBy(labels.ManagedByOperator).
		Build()
	serverType := normalizeServerSize(cluster.Spec.ControlPlanes.Size)

	logger.Info("creating new control plane server",
		"name", newServerName, "snapshot", snapshotID, "serverType", serverType)

	startTime := time.Now()

	result, err := r.provisionServer(ctx, cluster, serverCreateOpts{
		Name:       newServerName,
		SnapshotID: snapshotID,
		ServerType: serverType,
		Region:     cluster.Spec.Region,
		SSHKeyName: sshKeyName,
		Labels:     serverLabels,
		NetworkID:  clusterState.NetworkID,
		Role:       "control-plane",
	})
	if err != nil {
		logger.Error(err, "failed to provision CP server", "name", newServerName)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to create CP server %s: %v", newServerName, err)
		return err
	}

	// Persist status to prevent duplicate server creation
	if err := r.persistClusterStatus(ctx, cluster); err != nil {
		logger.Error(err, "failed to persist status after server creation", "name", newServerName)
	}

	if err := r.updateNodePhaseAndPersist(ctx, cluster, "control-plane", NodeStatusUpdate{
		Name:      newServerName,
		ServerID:  result.ServerID,
		PublicIP:  result.PublicIP,
		PrivateIP: result.PrivateIP,
		Phase:     k8znerv1alpha1.NodePhaseWaitingForTalosAPI,
		Reason:    fmt.Sprintf("Waiting for Talos API on %s:50000", result.TalosIP),
	}); err != nil {
		logger.Error(err, "failed to persist node status", "name", newServerName)
	}

	// Generate and apply Talos config, then wait for readiness
	if err := r.configureAndWaitForCP(ctx, cluster, clusterState, tc, newServerName, result); err != nil {
		return err
	}

	// Persist final status
	if err := r.persistClusterStatus(ctx, cluster); err != nil {
		logger.Error(err, "failed to persist cluster status", "name", newServerName)
	}

	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonScalingUp,
		"Successfully created control plane %s", newServerName)

	if r.enableMetrics {
		RecordNodeReplacement(cluster.Name, "control-plane", "scale-up")
		RecordNodeReplacementDuration(cluster.Name, "control-plane", time.Since(startTime).Seconds())
	}

	return nil
}

// configureAndWaitForCP generates and applies Talos config to a CP node, then waits for it to become ready.
// Returns a fatal error if the node has joined etcd but is not ready (server must be preserved).
func (r *ClusterReconciler) configureAndWaitForCP(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, clusterState *ClusterState, tc talosClients, serverName string, result *serverProvisionResult) error {
	logger := log.FromContext(ctx)

	if tc.configGen == nil || tc.client == nil {
		logger.Info("skipping Talos config application (no credentials available)", "name", serverName)
		r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
			Name:  serverName,
			Phase: k8znerv1alpha1.NodePhaseWaitingForK8s, Reason: "Waiting for node to join cluster (no Talos credentials)",
		})
		return nil
	}

	r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
		Name: serverName, Phase: k8znerv1alpha1.NodePhaseApplyingTalosConfig,
		Reason: "Generating and applying Talos machine configuration",
	})

	sans := append([]string{}, clusterState.SANs...)
	sans = append(sans, result.PublicIP)

	machineConfig, err := tc.configGen.GenerateControlPlaneConfig(sans, serverName, result.ServerID)
	if err != nil {
		logger.Error(err, "failed to generate CP config", "name", serverName)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
			"Failed to generate config for CP %s: %v", serverName, err)
		r.handleProvisioningFailure(ctx, cluster, "control-plane", serverName,
			fmt.Sprintf("Failed to generate Talos config: %v", err))
		return err
	}

	logger.Info("applying Talos config to CP", "name", serverName, "ip", result.TalosIP)
	if err := tc.client.ApplyConfig(ctx, result.TalosIP, machineConfig); err != nil {
		logger.Error(err, "failed to apply Talos config", "name", serverName)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
			"Failed to apply config to CP %s: %v", serverName, err)
		// Safe to delete: config was never applied, so etcd member wasn't added
		r.handleProvisioningFailure(ctx, cluster, "control-plane", serverName,
			fmt.Sprintf("Failed to apply Talos config: %v", err))
		return err
	}

	// CRITICAL: After Talos config is applied, the node starts joining etcd.
	// We must NOT delete this server on failure, as removing an etcd member
	// that was added but is unreachable can break etcd quorum.

	return r.waitForCPReady(ctx, cluster, tc, serverName, result.TalosIP)
}

// waitForCPReady waits for a control plane node to become ready after Talos config is applied.
// If the node is not ready, the server is preserved (etcd member already added).
func (r *ClusterReconciler) waitForCPReady(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, tc talosClients, serverName, talosIP string) error {
	logger := log.FromContext(ctx)

	r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
		Name: serverName, Phase: k8znerv1alpha1.NodePhaseRebootingWithConfig,
		Reason: "Talos config applied, node is rebooting with new configuration",
	})

	logger.Info("waiting for CP node to become ready", "name", serverName, "ip", talosIP)
	r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
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
		r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
			Name: serverName, Phase: k8znerv1alpha1.NodePhaseWaitingForK8s,
			Reason: fmt.Sprintf("Node not ready yet, will retry: %v", err),
		})
		return fmt.Errorf("CP %s not ready yet after config applied (etcd member added, server preserved): %w", serverName, err)
	}

	r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
		Name: serverName, Phase: k8znerv1alpha1.NodePhaseNodeInitializing,
		Reason: "Kubelet running, waiting for CNI and system pods",
	})
	logger.Info("CP node kubelet is running", "name", serverName)

	return nil
}

// scaleUpWorkers creates new worker nodes to reach the desired count.
// Uses ephemeral SSH keys to avoid Hetzner password emails.
func (r *ClusterReconciler) scaleUpWorkers(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, count int) error {
	logger := log.FromContext(ctx)

	// Build cluster state for server creation
	clusterState, err := r.buildClusterState(ctx, cluster)
	if err != nil {
		logger.Error(err, "failed to build cluster state")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to build cluster state for worker scaling: %v", err)
		return fmt.Errorf("failed to build cluster state: %w", err)
	}

	// Discover LB info if needed, then load Talos clients.
	// LB discovery must happen BEFORE loadTalosClients because the Talos generator
	// needs the control plane endpoint (LB IP) to generate valid configs.
	if r.talosClient == nil && cluster.Spec.CredentialsRef.Name != "" {
		creds, err := r.phaseAdapter.LoadCredentials(ctx, cluster)
		if err == nil {
			r.discoverLoadBalancerInfo(ctx, cluster, creds.HCloudToken)
		}
	}
	tc := r.loadTalosClients(ctx, cluster)

	// Get Talos snapshot for server creation
	snapshot, err := r.getSnapshot(ctx)
	if err != nil {
		logger.Error(err, "failed to get Talos snapshot")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Talos snapshot not found for worker scaling")
		return err
	}

	// Create ephemeral SSH key for this batch of workers
	sshKeyName, cleanupKey, err := r.createEphemeralSSHKey(ctx, cluster, "worker")
	if err != nil {
		return err
	}
	defer cleanupKey()

	// Create workers (respect maxConcurrentHeals)
	created := 0
	for i := 0; i < count && created < r.maxConcurrentHeals; i++ {
		if err := r.provisionAndConfigureWorker(ctx, cluster, clusterState, tc, snapshot.ID, sshKeyName); err != nil {
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

// provisionAndConfigureWorker provisions a single worker server, applies Talos config, and waits for readiness.
func (r *ClusterReconciler) provisionAndConfigureWorker(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, clusterState *ClusterState, tc talosClients, snapshotID int64, sshKeyName string) error {
	logger := log.FromContext(ctx)

	newServerName := naming.Worker(cluster.Name)
	serverLabels := labels.NewLabelBuilder(cluster.Name).
		WithRole("worker").
		WithPool("workers").
		WithManagedBy(labels.ManagedByOperator).
		Build()
	serverType := normalizeServerSize(cluster.Spec.Workers.Size)

	logger.Info("creating new worker server",
		"name", newServerName, "snapshot", snapshotID, "serverType", serverType)

	startTime := time.Now()

	result, err := r.provisionServer(ctx, cluster, serverCreateOpts{
		Name:       newServerName,
		SnapshotID: snapshotID,
		ServerType: serverType,
		Region:     cluster.Spec.Region,
		SSHKeyName: sshKeyName,
		Labels:     serverLabels,
		NetworkID:  clusterState.NetworkID,
		Role:       "worker",
	})
	if err != nil {
		logger.Error(err, "failed to provision worker server", "name", newServerName)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to create worker server %s: %v", newServerName, err)
		return err
	}

	// Persist status to prevent duplicate server creation
	if err := r.persistClusterStatus(ctx, cluster); err != nil {
		logger.Error(err, "failed to persist status after server creation", "name", newServerName)
	}

	if err := r.updateNodePhaseAndPersist(ctx, cluster, "worker", NodeStatusUpdate{
		Name:      newServerName,
		ServerID:  result.ServerID,
		PublicIP:  result.PublicIP,
		PrivateIP: result.PrivateIP,
		Phase:     k8znerv1alpha1.NodePhaseWaitingForTalosAPI,
		Reason:    fmt.Sprintf("Waiting for Talos API on %s:50000", result.TalosIP),
	}); err != nil {
		logger.Error(err, "failed to persist node status", "name", newServerName)
	}

	// Generate and apply Talos config, then wait for readiness
	if err := r.configureAndWaitForWorker(ctx, cluster, tc, newServerName, result); err != nil {
		return err
	}

	// Persist final status for this worker
	if err := r.persistClusterStatus(ctx, cluster); err != nil {
		logger.Error(err, "failed to persist cluster status", "name", newServerName)
	}

	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonScalingUp,
		"Successfully created worker %s", newServerName)

	if r.enableMetrics {
		RecordNodeReplacement(cluster.Name, "worker", "scale-up")
		RecordNodeReplacementDuration(cluster.Name, "worker", time.Since(startTime).Seconds())
	}

	return nil
}

// configureAndWaitForWorker generates and applies Talos config to a worker node, then waits for readiness.
func (r *ClusterReconciler) configureAndWaitForWorker(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, tc talosClients, serverName string, result *serverProvisionResult) error {
	logger := log.FromContext(ctx)

	if tc.configGen == nil || tc.client == nil {
		logger.Info("skipping Talos config application (no credentials available)", "name", serverName)
		r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
			Name: serverName, Phase: k8znerv1alpha1.NodePhaseWaitingForK8s,
			Reason: "Waiting for node to join cluster (no Talos credentials)",
		})
		return nil
	}

	r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
		Name: serverName, Phase: k8znerv1alpha1.NodePhaseApplyingTalosConfig,
		Reason: "Generating and applying Talos machine configuration",
	})

	machineConfig, err := tc.configGen.GenerateWorkerConfig(serverName, result.ServerID)
	if err != nil {
		logger.Error(err, "failed to generate worker config", "name", serverName)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
			"Failed to generate config for worker %s: %v", serverName, err)
		r.handleProvisioningFailure(ctx, cluster, "worker", serverName,
			fmt.Sprintf("Failed to generate Talos config: %v", err))
		return err
	}

	logger.Info("applying Talos config to worker", "name", serverName, "ip", result.TalosIP)
	if err := tc.client.ApplyConfig(ctx, result.TalosIP, machineConfig); err != nil {
		logger.Error(err, "failed to apply Talos config", "name", serverName)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
			"Failed to apply config to worker %s: %v", serverName, err)
		r.handleProvisioningFailure(ctx, cluster, "worker", serverName,
			fmt.Sprintf("Failed to apply Talos config: %v", err))
		return err
	}

	return r.waitForWorkerReady(ctx, cluster, serverName, result.TalosIP)
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

	// After config is applied, the node reboots with NEW TLS certificates.
	// The old Talos API connection won't work anymore, so we check Kubernetes directly
	// for node readiness instead of polling via Talos API.
	if err := r.nodeReadyWaiter(ctx, serverName, nodeReadyTimeout); err != nil {
		logger.Error(err, "worker node not ready in time", "name", serverName)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonNodeReadyTimeout,
			"Worker node %s not ready in time: %v", serverName, err)
		r.handleProvisioningFailure(ctx, cluster, "worker", serverName,
			fmt.Sprintf("Node not ready in time: %v", err))
		return err
	}

	// Node kubelet is running - transition to NodeInitializing
	// The state verifier will promote to Ready once K8s node is fully ready
	r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
		Name: serverName, Phase: k8znerv1alpha1.NodePhaseNodeInitializing,
		Reason: "Kubelet running, waiting for CNI and system pods",
	})

	return nil
}

// scaleDownWorkers removes excess worker nodes to reach the desired count.
// Workers are selected for removal based on: unhealthy first, then newest.
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

	// Update the status to remove the deleted workers
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
		// Continue with removal anyway - pods will be rescheduled
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
			if r.enableMetrics {
				RecordHCloudAPICall("delete_server", "error", time.Since(startTime).Seconds())
			}
			return fmt.Errorf("failed to delete hetzner server %s: %w", worker.Name, err)
		}
		logger.Info("deleted hetzner server", "name", worker.Name, "serverID", worker.ServerID)
		if r.enableMetrics {
			RecordHCloudAPICall("delete_server", "success", time.Since(startTime).Seconds())
		}
	}

	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonScalingDown,
		"Successfully removed worker %s", worker.Name)

	if r.enableMetrics {
		RecordNodeReplacement(cluster.Name, "worker", "scale-down")
		RecordNodeReplacementDuration(cluster.Name, "worker", time.Since(startTime).Seconds())
	}

	return nil
}

// selectWorkersForRemoval selects workers to remove during scale-down.
// Priority: 1. Unhealthy workers, 2. Newest workers (by name, assuming newer names sort last)
func (r *ClusterReconciler) selectWorkersForRemoval(cluster *k8znerv1alpha1.K8znerCluster, count int) []*k8znerv1alpha1.NodeStatus {
	if count <= 0 || len(cluster.Status.Workers.Nodes) == 0 {
		return nil
	}

	// First, collect unhealthy workers
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

	// Select from unhealthy first, then from healthy (newest first)
	// For healthy workers, we pick from the end of the list (newest)
	var selected []*k8znerv1alpha1.NodeStatus

	// Add unhealthy workers first
	for _, node := range unhealthy {
		if len(selected) >= count {
			break
		}
		selected = append(selected, node)
	}

	// If we still need more, select from healthy workers (newest first)
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

	// Create a set of names to remove
	toRemove := make(map[string]bool)
	for _, w := range removed {
		toRemove[w.Name] = true
	}

	// Filter out removed workers
	var remaining []k8znerv1alpha1.NodeStatus
	for _, node := range cluster.Status.Workers.Nodes {
		if !toRemove[node.Name] {
			remaining = append(remaining, node)
		}
	}

	cluster.Status.Workers.Nodes = remaining
}
