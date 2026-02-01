// Package controller contains the Kubernetes controllers for the k8zner operator.
package controller

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/platform/hcloud"
)

const (
	// Default reconciliation interval.
	defaultRequeueAfter = 30 * time.Second

	// Default health check thresholds.
	defaultNodeNotReadyThreshold  = 5 * time.Minute
	defaultEtcdUnhealthyThreshold = 2 * time.Minute

	// Server creation timeouts.
	serverIPTimeout    = 2 * time.Minute
	nodeReadyTimeout   = 5 * time.Minute
	serverIPRetryDelay = 5 * time.Second

	// Event reasons.
	EventReasonReconciling         = "Reconciling"
	EventReasonReconcileSucceeded  = "ReconcileSucceeded"
	EventReasonReconcileFailed     = "ReconcileFailed"
	EventReasonNodeUnhealthy       = "NodeUnhealthy"
	EventReasonNodeReplacing       = "NodeReplacing"
	EventReasonNodeReplaced        = "NodeReplaced"
	EventReasonQuorumLost          = "QuorumLost"
	EventReasonScalingUp           = "ScalingUp"
	EventReasonScalingDown         = "ScalingDown"
	EventReasonServerCreationError = "ServerCreationError"
	EventReasonConfigApplyError    = "ConfigApplyError"
	EventReasonNodeReadyTimeout    = "NodeReadyTimeout"
)

// ClusterReconciler reconciles a K8znerCluster object.
type ClusterReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Dependencies (injected via options).
	hcloudClient       HCloudClient
	talosClient        TalosClient
	talosConfigGen     TalosConfigGenerator
	hcloudToken        string
	enableMetrics      bool
	maxConcurrentHeals int
}

// Option configures a ClusterReconciler.
type Option func(*ClusterReconciler)

// WithHCloudClient sets a custom HCloud client.
func WithHCloudClient(c HCloudClient) Option {
	return func(r *ClusterReconciler) {
		r.hcloudClient = c
	}
}

// WithTalosClient sets a custom Talos client.
func WithTalosClient(c TalosClient) Option {
	return func(r *ClusterReconciler) {
		r.talosClient = c
	}
}

// WithTalosConfigGenerator sets a custom Talos config generator.
func WithTalosConfigGenerator(g TalosConfigGenerator) Option {
	return func(r *ClusterReconciler) {
		r.talosConfigGen = g
	}
}

// WithHCloudToken sets the Hetzner Cloud API token for lazy client creation.
func WithHCloudToken(token string) Option {
	return func(r *ClusterReconciler) {
		r.hcloudToken = token
	}
}

// WithMetrics enables Prometheus metrics.
func WithMetrics(enable bool) Option {
	return func(r *ClusterReconciler) {
		r.enableMetrics = enable
	}
}

// WithMaxConcurrentHeals sets the maximum concurrent healing operations.
func WithMaxConcurrentHeals(maxHeals int) Option {
	return func(r *ClusterReconciler) {
		r.maxConcurrentHeals = maxHeals
	}
}

// NewClusterReconciler creates a new ClusterReconciler with the given options.
func NewClusterReconciler(c client.Client, scheme *runtime.Scheme, recorder record.EventRecorder, opts ...Option) *ClusterReconciler {
	r := &ClusterReconciler{
		Client:             c,
		Scheme:             scheme,
		Recorder:           recorder,
		enableMetrics:      true,
		maxConcurrentHeals: 1,
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// ensureHCloudClient ensures the HCloud client is initialized.
func (r *ClusterReconciler) ensureHCloudClient() error {
	if r.hcloudClient != nil {
		return nil
	}
	if r.hcloudToken == "" {
		return fmt.Errorf("HCloud token not configured")
	}
	r.hcloudClient = hcloud.NewRealClient(r.hcloudToken)
	return nil
}

// +kubebuilder:rbac:groups=k8zner.io,resources=k8znerclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=k8zner.io,resources=k8znerclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=k8zner.io,resources=k8znerclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=pods/eviction,verbs=create
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;create;update

// Reconcile handles the reconciliation loop for K8znerCluster resources.
func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("cluster", req.Name)
	ctx = log.IntoContext(ctx, logger)

	startTime := time.Now()
	defer func() {
		if r.enableMetrics {
			duration := time.Since(startTime).Seconds()
			RecordReconcile(req.Name, "completed", duration)
		}
	}()

	// Fetch the K8znerCluster
	cluster := &k8znerv1alpha1.K8znerCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("cluster resource not found, likely deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch K8znerCluster")
		return ctrl.Result{}, err
	}

	// Check if paused
	if cluster.Spec.Paused {
		logger.Info("cluster is paused, skipping reconciliation")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Ensure HCloud client is initialized
	if err := r.ensureHCloudClient(); err != nil {
		logger.Error(err, "failed to initialize HCloud client")
		r.Recorder.Event(cluster, corev1.EventTypeWarning, EventReasonReconcileFailed, err.Error())
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Record reconciliation start
	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonReconciling, "Starting reconciliation")

	// Run the reconciliation phases
	result, err := r.reconcile(ctx, cluster)

	// Update status
	cluster.Status.LastReconcileTime = &metav1.Time{Time: time.Now()}
	cluster.Status.ObservedGeneration = cluster.Generation

	if statusErr := r.Status().Update(ctx, cluster); statusErr != nil {
		logger.Error(statusErr, "failed to update status")
		if err == nil {
			err = statusErr
		}
	}

	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonReconcileFailed,
			"Reconciliation failed: %v", err)
		if r.enableMetrics {
			RecordReconcile(req.Name, "error", time.Since(startTime).Seconds())
		}
	} else {
		r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonReconcileSucceeded,
			"Reconciliation completed successfully")
	}

	return result, err
}

// reconcile runs the main reconciliation logic.
func (r *ClusterReconciler) reconcile(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Always update the cluster phase before returning
	defer r.updateClusterPhase(cluster)

	// Phase 1: Health Check
	logger.V(1).Info("running health check phase")
	if err := r.reconcileHealthCheck(ctx, cluster); err != nil {
		return ctrl.Result{}, fmt.Errorf("health check failed: %w", err)
	}

	// Phase 2: Control Plane Reconciliation
	logger.V(1).Info("running control plane reconciliation")
	if result, err := r.reconcileControlPlanes(ctx, cluster); err != nil || result.Requeue {
		return result, err
	}

	// Phase 3: Worker Reconciliation
	logger.V(1).Info("running worker reconciliation")
	if result, err := r.reconcileWorkers(ctx, cluster); err != nil || result.Requeue {
		return result, err
	}

	// Requeue for continuous monitoring
	return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
}

// reconcileHealthCheck checks the health of all nodes and updates status.
func (r *ClusterReconciler) reconcileHealthCheck(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) error {
	logger := log.FromContext(ctx)

	// List all nodes in the cluster
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	// Categorize nodes by role
	var cpNodes, workerNodes []corev1.Node
	for _, node := range nodeList.Items {
		if _, isCP := node.Labels["node-role.kubernetes.io/control-plane"]; isCP {
			cpNodes = append(cpNodes, node)
		} else {
			workerNodes = append(workerNodes, node)
		}
	}

	// Update control plane status
	cluster.Status.ControlPlanes = r.buildNodeGroupStatus(ctx, cluster, cpNodes, cluster.Spec.ControlPlanes.Count, "control-plane")

	// Update worker status
	cluster.Status.Workers = r.buildNodeGroupStatus(ctx, cluster, workerNodes, cluster.Spec.Workers.Count, "worker")

	// Record metrics
	if r.enableMetrics {
		RecordNodeCounts(cluster.Name, "control-plane",
			len(cpNodes), cluster.Status.ControlPlanes.Ready, cluster.Spec.ControlPlanes.Count)
		RecordNodeCounts(cluster.Name, "worker",
			len(workerNodes), cluster.Status.Workers.Ready, cluster.Spec.Workers.Count)
	}

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

// reconcileControlPlanes ensures control planes are healthy.
func (r *ClusterReconciler) reconcileControlPlanes(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Skip if single CP (no HA replacement possible)
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
	desiredCount := cluster.Spec.Workers.Count

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
		// TODO: Remove excess workers (drain and delete)
	}

	if len(unhealthyWorkers) > 0 {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// scaleUpWorkers creates new worker nodes to reach the desired count.
func (r *ClusterReconciler) scaleUpWorkers(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, count int) error {
	logger := log.FromContext(ctx)

	// Step 1: Build cluster state for server creation
	clusterState, err := r.buildClusterState(ctx, cluster)
	if err != nil {
		logger.Error(err, "failed to build cluster state")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to build cluster state for worker scaling: %v", err)
		return fmt.Errorf("failed to build cluster state: %w", err)
	}

	// Step 2: Get Talos snapshot for server creation
	snapshotLabels := map[string]string{"os": "talos"}
	snapshot, err := r.hcloudClient.GetSnapshotByLabels(ctx, snapshotLabels)
	if err != nil {
		logger.Error(err, "failed to get Talos snapshot")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to get Talos snapshot: %v", err)
		return fmt.Errorf("failed to get Talos snapshot: %w", err)
	}
	if snapshot == nil {
		err := fmt.Errorf("no Talos snapshot found with labels %v", snapshotLabels)
		logger.Error(err, "Talos snapshot not found")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Talos snapshot not found for worker scaling")
		return err
	}

	// Step 3: Find the next available worker index
	nextIndex := r.findNextWorkerIndex(cluster)

	// Step 4: Create workers (respect maxConcurrentHeals)
	created := 0
	for i := 0; i < count && created < r.maxConcurrentHeals; i++ {
		workerIndex := nextIndex + i
		newServerName := fmt.Sprintf("%s-workers-%d", cluster.Name, workerIndex)

		serverLabels := map[string]string{
			"cluster": cluster.Name,
			"role":    "worker",
			"pool":    "workers",
		}

		logger.Info("creating new worker server",
			"name", newServerName,
			"snapshot", snapshot.ID,
			"serverType", cluster.Spec.Workers.Size,
			"index", workerIndex,
		)

		startTime := time.Now()
		_, err = r.hcloudClient.CreateServer(
			ctx,
			newServerName,
			fmt.Sprintf("%d", snapshot.ID),
			cluster.Spec.Workers.Size,
			cluster.Spec.Region,
			clusterState.SSHKeyIDs,
			serverLabels,
			"",  // userData
			nil, // placementGroupID
			clusterState.NetworkID,
			"",   // privateIP - let HCloud assign
			true, // enablePublicIPv4
			true, // enablePublicIPv6
		)
		if err != nil {
			if r.enableMetrics {
				RecordHCloudAPICall("create_server", "error", time.Since(startTime).Seconds())
			}
			logger.Error(err, "failed to create worker server", "name", newServerName)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
				"Failed to create worker server %s: %v", newServerName, err)
			// Continue trying to create remaining workers
			continue
		}
		if r.enableMetrics {
			RecordHCloudAPICall("create_server", "success", time.Since(startTime).Seconds())
		}
		logger.Info("created worker server", "name", newServerName)

		// Step 5: Wait for server IP assignment
		serverIP, err := r.waitForServerIP(ctx, newServerName, serverIPTimeout)
		if err != nil {
			logger.Error(err, "failed to get server IP", "name", newServerName)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
				"Failed to get IP for worker server %s: %v", newServerName, err)
			continue
		}
		logger.Info("server IP assigned", "name", newServerName, "ip", serverIP)

		// Step 6: Get server ID for config generation
		serverIDStr, err := r.hcloudClient.GetServerID(ctx, newServerName)
		if err != nil {
			logger.Error(err, "failed to get server ID", "name", newServerName)
			continue
		}
		var serverID int64
		if _, err := fmt.Sscanf(serverIDStr, "%d", &serverID); err != nil {
			logger.Error(err, "failed to parse server ID", "name", newServerName)
			continue
		}

		// Step 7: Generate and apply Talos config (if talos clients are available)
		if r.talosConfigGen != nil && r.talosClient != nil {
			config, err := r.talosConfigGen.GenerateWorkerConfig(newServerName, serverID)
			if err != nil {
				logger.Error(err, "failed to generate worker config", "name", newServerName)
				r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
					"Failed to generate config for worker %s: %v", newServerName, err)
				continue
			}

			logger.Info("applying Talos config to worker", "name", newServerName, "ip", serverIP)
			if err := r.talosClient.ApplyConfig(ctx, serverIP, config); err != nil {
				logger.Error(err, "failed to apply Talos config", "name", newServerName)
				r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
					"Failed to apply config to worker %s: %v", newServerName, err)
				continue
			}

			// Step 8: Wait for node to be ready
			logger.Info("waiting for worker node to become ready", "name", newServerName)
			if err := r.talosClient.WaitForNodeReady(ctx, serverIP, int(nodeReadyTimeout.Seconds())); err != nil {
				logger.Error(err, "worker node not ready in time", "name", newServerName)
				r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonNodeReadyTimeout,
					"Worker node %s not ready in time: %v", newServerName, err)
				continue
			}
		}

		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonScalingUp,
			"Successfully created worker %s", newServerName)
		created++

		if r.enableMetrics {
			RecordNodeReplacement(cluster.Name, "worker", "scale-up")
			RecordNodeReplacementDuration(cluster.Name, "worker", time.Since(startTime).Seconds())
		}
	}

	if created < count {
		return fmt.Errorf("only created %d of %d requested workers", created, count)
	}

	return nil
}

// findNextWorkerIndex finds the next available worker index for the cluster.
func (r *ClusterReconciler) findNextWorkerIndex(cluster *k8znerv1alpha1.K8znerCluster) int {
	maxIndex := 0
	for _, node := range cluster.Status.Workers.Nodes {
		// Extract index from name like "{cluster}-workers-{index}"
		parts := strings.Split(node.Name, "-")
		if len(parts) >= 1 {
			if idx, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
				if idx >= maxIndex {
					maxIndex = idx + 1
				}
			}
		}
	}
	// If no workers exist, start at 1
	if maxIndex == 0 {
		maxIndex = 1
	}
	return maxIndex
}

// replaceControlPlane replaces an unhealthy control plane node.
func (r *ClusterReconciler) replaceControlPlane(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, node *k8znerv1alpha1.NodeStatus) error {
	logger := log.FromContext(ctx)

	// Step 1: Remove from etcd cluster (via Talos API)
	if r.talosClient != nil && node.PrivateIP != "" {
		// Get etcd members from a healthy control plane
		healthyIP := r.findHealthyControlPlaneIP(cluster)
		if healthyIP != "" {
			members, err := r.talosClient.GetEtcdMembers(ctx, healthyIP)
			if err != nil {
				logger.Error(err, "failed to get etcd members")
			} else {
				for _, member := range members {
					if member.Name == node.Name || member.Endpoint == node.PrivateIP {
						if err := r.talosClient.RemoveEtcdMember(ctx, healthyIP, member.ID); err != nil {
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
		logger.Info("skipping etcd member removal (talos client not configured or no IP)")
	}

	// Step 2: Delete the Kubernetes node
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

	// Step 4: Build cluster state for server creation
	clusterState, err := r.buildClusterState(ctx, cluster)
	if err != nil {
		logger.Error(err, "failed to build cluster state")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to build cluster state for control plane replacement: %v", err)
		return fmt.Errorf("failed to build cluster state: %w", err)
	}

	// Step 5: Get Talos snapshot for server creation
	snapshotLabels := map[string]string{"os": "talos"}
	snapshot, err := r.hcloudClient.GetSnapshotByLabels(ctx, snapshotLabels)
	if err != nil {
		logger.Error(err, "failed to get Talos snapshot")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to get Talos snapshot: %v", err)
		return fmt.Errorf("failed to get Talos snapshot: %w", err)
	}
	if snapshot == nil {
		err := fmt.Errorf("no Talos snapshot found with labels %v", snapshotLabels)
		logger.Error(err, "Talos snapshot not found")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Talos snapshot not found for control plane replacement")
		return err
	}

	// Step 6: Create new server
	newServerName := r.generateReplacementServerName(cluster, "control-plane", node.Name)
	serverLabels := map[string]string{
		"cluster": cluster.Name,
		"role":    "control-plane",
		"pool":    "control-plane",
	}

	logger.Info("creating replacement control plane server",
		"name", newServerName,
		"snapshot", snapshot.ID,
		"serverType", cluster.Spec.ControlPlanes.Size,
	)

	startTime := time.Now()
	_, err = r.hcloudClient.CreateServer(
		ctx,
		newServerName,
		fmt.Sprintf("%d", snapshot.ID), // image ID as string
		cluster.Spec.ControlPlanes.Size,
		cluster.Spec.Region,
		clusterState.SSHKeyIDs,
		serverLabels,
		"",  // userData
		nil, // placementGroupID
		clusterState.NetworkID,
		"",   // privateIP - let HCloud assign
		true, // enablePublicIPv4
		true, // enablePublicIPv6
	)
	if err != nil {
		if r.enableMetrics {
			RecordHCloudAPICall("create_server", "error", time.Since(startTime).Seconds())
		}
		logger.Error(err, "failed to create replacement control plane server", "name", newServerName)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to create replacement control plane server %s: %v", newServerName, err)
		return fmt.Errorf("failed to create server: %w", err)
	}
	if r.enableMetrics {
		RecordHCloudAPICall("create_server", "success", time.Since(startTime).Seconds())
	}
	logger.Info("created replacement control plane server", "name", newServerName)

	// Step 7: Wait for server IP assignment
	serverIP, err := r.waitForServerIP(ctx, newServerName, serverIPTimeout)
	if err != nil {
		logger.Error(err, "failed to get server IP", "name", newServerName)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to get IP for replacement control plane server %s: %v", newServerName, err)
		return fmt.Errorf("failed to get server IP: %w", err)
	}
	logger.Info("server IP assigned", "name", newServerName, "ip", serverIP)

	// Step 8: Get server ID for config generation
	serverIDStr, err := r.hcloudClient.GetServerID(ctx, newServerName)
	if err != nil {
		logger.Error(err, "failed to get server ID", "name", newServerName)
		return fmt.Errorf("failed to get server ID: %w", err)
	}
	var serverID int64
	if _, err := fmt.Sscanf(serverIDStr, "%d", &serverID); err != nil {
		return fmt.Errorf("failed to parse server ID: %w", err)
	}

	// Step 9: Generate and apply Talos config (if talos clients are available)
	if r.talosConfigGen != nil && r.talosClient != nil {
		// Update SANs with new server IP
		sans := append([]string{}, clusterState.SANs...)
		sans = append(sans, serverIP)

		config, err := r.talosConfigGen.GenerateControlPlaneConfig(sans, newServerName, serverID)
		if err != nil {
			logger.Error(err, "failed to generate control plane config", "name", newServerName)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
				"Failed to generate config for control plane %s: %v", newServerName, err)
			return fmt.Errorf("failed to generate config: %w", err)
		}

		logger.Info("applying Talos config to control plane", "name", newServerName, "ip", serverIP)
		if err := r.talosClient.ApplyConfig(ctx, serverIP, config); err != nil {
			logger.Error(err, "failed to apply Talos config", "name", newServerName)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
				"Failed to apply config to control plane %s: %v", newServerName, err)
			return fmt.Errorf("failed to apply config: %w", err)
		}

		// Step 10: Wait for node to become ready
		logger.Info("waiting for control plane node to become ready", "name", newServerName, "ip", serverIP)
		if err := r.talosClient.WaitForNodeReady(ctx, serverIP, int(nodeReadyTimeout.Seconds())); err != nil {
			logger.Error(err, "control plane node failed to become ready", "name", newServerName)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonNodeReadyTimeout,
				"Control plane %s failed to become ready: %v", newServerName, err)
			return fmt.Errorf("node failed to become ready: %w", err)
		}

		logger.Info("control plane node is ready", "name", newServerName)
	} else {
		logger.Info("skipping Talos config application (talos clients not configured)")
	}

	return nil
}

// replaceWorker replaces an unhealthy worker node.
func (r *ClusterReconciler) replaceWorker(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, node *k8znerv1alpha1.NodeStatus) error {
	logger := log.FromContext(ctx)

	// Step 1: Cordon the node
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

	// Step 3: Delete the Kubernetes node
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

	// Step 5: Build cluster state for server creation
	clusterState, err := r.buildClusterState(ctx, cluster)
	if err != nil {
		logger.Error(err, "failed to build cluster state")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to build cluster state for worker replacement: %v", err)
		return fmt.Errorf("failed to build cluster state: %w", err)
	}

	// Step 6: Get Talos snapshot for server creation
	snapshotLabels := map[string]string{"os": "talos"}
	snapshot, err := r.hcloudClient.GetSnapshotByLabels(ctx, snapshotLabels)
	if err != nil {
		logger.Error(err, "failed to get Talos snapshot")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to get Talos snapshot: %v", err)
		return fmt.Errorf("failed to get Talos snapshot: %w", err)
	}
	if snapshot == nil {
		err := fmt.Errorf("no Talos snapshot found with labels %v", snapshotLabels)
		logger.Error(err, "Talos snapshot not found")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Talos snapshot not found for worker replacement")
		return err
	}

	// Step 7: Create new server
	newServerName := r.generateReplacementServerName(cluster, "worker", node.Name)
	serverLabels := map[string]string{
		"cluster": cluster.Name,
		"role":    "worker",
		"pool":    "workers",
	}

	logger.Info("creating replacement worker server",
		"name", newServerName,
		"snapshot", snapshot.ID,
		"serverType", cluster.Spec.Workers.Size,
	)

	startTime := time.Now()
	_, err = r.hcloudClient.CreateServer(
		ctx,
		newServerName,
		fmt.Sprintf("%d", snapshot.ID), // image ID as string
		cluster.Spec.Workers.Size,
		cluster.Spec.Region,
		clusterState.SSHKeyIDs,
		serverLabels,
		"",  // userData
		nil, // placementGroupID
		clusterState.NetworkID,
		"",   // privateIP - let HCloud assign
		true, // enablePublicIPv4
		true, // enablePublicIPv6
	)
	if err != nil {
		if r.enableMetrics {
			RecordHCloudAPICall("create_server", "error", time.Since(startTime).Seconds())
		}
		logger.Error(err, "failed to create replacement worker server", "name", newServerName)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to create replacement worker server %s: %v", newServerName, err)
		return fmt.Errorf("failed to create server: %w", err)
	}
	if r.enableMetrics {
		RecordHCloudAPICall("create_server", "success", time.Since(startTime).Seconds())
	}
	logger.Info("created replacement worker server", "name", newServerName)

	// Step 8: Wait for server IP assignment
	serverIP, err := r.waitForServerIP(ctx, newServerName, serverIPTimeout)
	if err != nil {
		logger.Error(err, "failed to get server IP", "name", newServerName)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to get IP for replacement worker server %s: %v", newServerName, err)
		return fmt.Errorf("failed to get server IP: %w", err)
	}
	logger.Info("server IP assigned", "name", newServerName, "ip", serverIP)

	// Step 9: Get server ID for config generation
	serverIDStr, err := r.hcloudClient.GetServerID(ctx, newServerName)
	if err != nil {
		logger.Error(err, "failed to get server ID", "name", newServerName)
		return fmt.Errorf("failed to get server ID: %w", err)
	}
	var serverID int64
	if _, err := fmt.Sscanf(serverIDStr, "%d", &serverID); err != nil {
		return fmt.Errorf("failed to parse server ID: %w", err)
	}

	// Step 10: Generate and apply Talos config (if talos clients are available)
	if r.talosConfigGen != nil && r.talosClient != nil {
		config, err := r.talosConfigGen.GenerateWorkerConfig(newServerName, serverID)
		if err != nil {
			logger.Error(err, "failed to generate worker config", "name", newServerName)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
				"Failed to generate config for worker %s: %v", newServerName, err)
			return fmt.Errorf("failed to generate config: %w", err)
		}

		logger.Info("applying Talos config to worker", "name", newServerName, "ip", serverIP)
		if err := r.talosClient.ApplyConfig(ctx, serverIP, config); err != nil {
			logger.Error(err, "failed to apply Talos config", "name", newServerName)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
				"Failed to apply config to worker %s: %v", newServerName, err)
			return fmt.Errorf("failed to apply config: %w", err)
		}

		// Step 11: Wait for node to become ready
		logger.Info("waiting for worker node to become ready", "name", newServerName, "ip", serverIP)
		if err := r.talosClient.WaitForNodeReady(ctx, serverIP, int(nodeReadyTimeout.Seconds())); err != nil {
			logger.Error(err, "worker node failed to become ready", "name", newServerName)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonNodeReadyTimeout,
				"Worker %s failed to become ready: %v", newServerName, err)
			return fmt.Errorf("node failed to become ready: %w", err)
		}

		logger.Info("worker node is ready", "name", newServerName)
	} else {
		logger.Info("skipping Talos config application (talos clients not configured)")
	}

	return nil
}

// drainNode evicts all pods from a node.
func (r *ClusterReconciler) drainNode(ctx context.Context, nodeName string) error {
	logger := log.FromContext(ctx)

	// List pods on this node
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.MatchingFields{"spec.nodeName": nodeName}); err != nil {
		return fmt.Errorf("failed to list pods on node: %w", err)
	}

	// Evict each pod (skip DaemonSet pods and mirror pods)
	for _, pod := range podList.Items {
		// Skip mirror pods (static pods)
		if _, isMirror := pod.Annotations[corev1.MirrorPodAnnotationKey]; isMirror {
			continue
		}

		// Skip DaemonSet pods
		isDaemonSet := false
		for _, ref := range pod.OwnerReferences {
			if ref.Kind == "DaemonSet" {
				isDaemonSet = true
				break
			}
		}
		if isDaemonSet {
			continue
		}

		// Create eviction
		eviction := &policyv1.Eviction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
		}
		if err := r.SubResource("eviction").Create(ctx, &pod, eviction); err != nil {
			if !apierrors.IsNotFound(err) && !apierrors.IsTooManyRequests(err) {
				logger.Error(err, "failed to evict pod", "pod", pod.Name, "namespace", pod.Namespace)
			}
		} else {
			logger.V(1).Info("evicted pod", "pod", pod.Name, "namespace", pod.Namespace)
		}
	}

	return nil
}

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

	// Get network by cluster name convention
	networkName := fmt.Sprintf("%s-network", cluster.Name)
	network, err := r.hcloudClient.GetNetwork(ctx, networkName)
	if err != nil {
		return nil, fmt.Errorf("failed to get network %s: %w", networkName, err)
	}
	if network != nil {
		state.NetworkID = network.ID
	}

	// Build SANs from existing healthy control plane IPs
	var sans []string
	for _, node := range cluster.Status.ControlPlanes.Nodes {
		if node.PrivateIP != "" {
			sans = append(sans, node.PrivateIP)
		}
		if node.PublicIP != "" {
			sans = append(sans, node.PublicIP)
		}
	}
	state.SANs = sans

	// Get SSH keys from cluster annotations or use default naming convention
	if cluster.Annotations != nil {
		if sshKeys, ok := cluster.Annotations["k8zner.io/ssh-keys"]; ok {
			state.SSHKeyIDs = strings.Split(sshKeys, ",")
		}
	}
	if len(state.SSHKeyIDs) == 0 {
		// Use default SSH key naming convention
		state.SSHKeyIDs = []string{fmt.Sprintf("%s-key", cluster.Name)}
	}

	// Get control plane endpoint (load balancer IP) from annotations or find healthy CP
	if cluster.Annotations != nil {
		if endpoint, ok := cluster.Annotations["k8zner.io/control-plane-endpoint"]; ok {
			state.ControlPlaneIP = endpoint
		}
	}
	if state.ControlPlaneIP == "" {
		// Fall back to first healthy control plane IP
		state.ControlPlaneIP = r.findHealthyControlPlaneIP(cluster)
	}

	logger.V(1).Info("built cluster state",
		"networkID", state.NetworkID,
		"sans", len(state.SANs),
		"sshKeys", len(state.SSHKeyIDs),
		"controlPlaneIP", state.ControlPlaneIP,
	)

	return state, nil
}

// generateReplacementServerName generates a new server name for a replacement node.
func (r *ClusterReconciler) generateReplacementServerName(cluster *k8znerv1alpha1.K8znerCluster, role string, oldName string) string {
	// Extract the pool and index from the old name if possible
	// Format: {cluster}-{pool}-{index}
	// If the old name follows the expected pattern, reuse it directly
	if strings.HasPrefix(oldName, cluster.Name+"-") {
		parts := strings.Split(oldName, "-")
		if len(parts) >= 3 {
			// Check if last part is a number (index)
			if _, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
				// Reuse the same name for replacement (same pool and index)
				return oldName
			}
		}
	}

	// Fallback: generate new name with timestamp suffix
	timestamp := time.Now().Unix()
	return fmt.Sprintf("%s-%s-%d", cluster.Name, role, timestamp)
}

// waitForServerIP waits for a server to have an IP assigned.
func (r *ClusterReconciler) waitForServerIP(ctx context.Context, serverName string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

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

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Index pods by node name for efficient drain operations
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Pod{}, "spec.nodeName", func(o client.Object) []string {
		pod := o.(*corev1.Pod)
		return []string{pod.Spec.NodeName}
	}); err != nil {
		return fmt.Errorf("failed to create pod node name index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&k8znerv1alpha1.K8znerCluster{}).
		Watches(&corev1.Node{}, &nodeEventHandler{}).
		Complete(r)
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

// nodeEventHandler handles node events and triggers reconciliation.
type nodeEventHandler struct{}

func (h *nodeEventHandler) Create(ctx context.Context, e event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueCluster(ctx, q)
}

func (h *nodeEventHandler) Update(ctx context.Context, e event.UpdateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueCluster(ctx, q)
}

func (h *nodeEventHandler) Delete(ctx context.Context, e event.DeleteEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueCluster(ctx, q)
}

func (h *nodeEventHandler) Generic(ctx context.Context, e event.GenericEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueCluster(ctx, q)
}

func (h *nodeEventHandler) enqueueCluster(ctx context.Context, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// Enqueue all K8znerCluster resources in the k8zner-system namespace
	// In a real implementation, we'd look up which cluster owns this node
	q.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "k8zner-system",
			Name:      "cluster",
		},
	})
}
