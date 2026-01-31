// Package controller contains the Kubernetes controllers for the k8zner operator.
package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	// Default reconciliation interval
	defaultRequeueAfter = 30 * time.Second

	// Default health check thresholds
	defaultNodeNotReadyThreshold = 5 * time.Minute
	defaultEtcdUnhealthyThreshold = 2 * time.Minute
)

// ClusterReconciler reconciles a K8znerCluster object.
type ClusterReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	HCloudToken string

	// Clients (created lazily)
	hcloudClient *hcloud.RealClient
}

// NewClusterReconciler creates a new ClusterReconciler.
func NewClusterReconciler(client client.Client, scheme *runtime.Scheme, hcloudToken string) *ClusterReconciler {
	return &ClusterReconciler{
		Client:      client,
		Scheme:      scheme,
		HCloudToken: hcloudToken,
	}
}

// +kubebuilder:rbac:groups=k8zner.io,resources=k8znerclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=k8zner.io,resources=k8znerclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=k8zner.io,resources=k8znerclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=pods/eviction,verbs=create
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;create;update

// Reconcile handles the reconciliation loop for K8znerCluster resources.
func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the K8znerCluster
	cluster := &k8znerv1alpha1.K8znerCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			// Object deleted, nothing to do
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

	// Initialize Hetzner client if needed
	if r.hcloudClient == nil {
		r.hcloudClient = hcloud.NewRealClient(r.HCloudToken)
	}

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

	return result, err
}

// reconcile runs the main reconciliation logic.
func (r *ClusterReconciler) reconcile(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

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

	// Phase 4: Addon Reconciliation (future)
	// logger.V(1).Info("running addon reconciliation")
	// if result, err := r.reconcileAddons(ctx, cluster); err != nil || result.Requeue {
	// 	return result, err
	// }

	// Update overall phase
	r.updateClusterPhase(cluster)

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
	cluster.Status.ControlPlanes = r.buildNodeGroupStatus(ctx, cpNodes, cluster.Spec.ControlPlanes.Count)

	// Update worker status
	cluster.Status.Workers = r.buildNodeGroupStatus(ctx, workerNodes, cluster.Spec.Workers.Count)

	logger.Info("health check complete",
		"controlPlanes", fmt.Sprintf("%d/%d", cluster.Status.ControlPlanes.Ready, cluster.Status.ControlPlanes.Desired),
		"workers", fmt.Sprintf("%d/%d", cluster.Status.Workers.Ready, cluster.Status.Workers.Desired),
	)

	return nil
}

// buildNodeGroupStatus builds the status for a group of nodes.
func (r *ClusterReconciler) buildNodeGroupStatus(ctx context.Context, nodes []corev1.Node, desired int) k8znerv1alpha1.NodeGroupStatus {
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
		// All control planes healthy
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
		// Update condition
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

	cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseHealing

	if err := r.replaceControlPlane(ctx, cluster, unhealthyCP); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to replace control plane: %w", err)
	}

	// Requeue immediately to check the new node
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

	// Replace unhealthy workers
	for _, worker := range unhealthyWorkers {
		logger.Info("replacing unhealthy worker",
			"node", worker.Name,
			"serverID", worker.ServerID,
			"unhealthySince", worker.UnhealthySince,
		)

		cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseHealing

		if err := r.replaceWorker(ctx, cluster, worker); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to replace worker %s: %w", worker.Name, err)
		}
	}

	// Check if scaling is needed
	currentCount := len(cluster.Status.Workers.Nodes)
	desiredCount := cluster.Spec.Workers.Count

	if currentCount < desiredCount {
		logger.Info("scaling up workers", "current", currentCount, "desired", desiredCount)
		// TODO: Create new workers
	} else if currentCount > desiredCount {
		logger.Info("scaling down workers", "current", currentCount, "desired", desiredCount)
		// TODO: Remove excess workers (drain and delete)
	}

	if len(unhealthyWorkers) > 0 {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// replaceControlPlane replaces an unhealthy control plane node.
func (r *ClusterReconciler) replaceControlPlane(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, node *k8znerv1alpha1.NodeStatus) error {
	logger := log.FromContext(ctx)

	// Step 1: Remove from etcd cluster (via Talos API)
	// TODO: Implement etcd member removal
	logger.Info("TODO: Remove from etcd cluster", "node", node.Name)

	// Step 2: Delete the Kubernetes node
	k8sNode := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: node.Name}, k8sNode); err == nil {
		if err := r.Delete(ctx, k8sNode); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete k8s node: %w", err)
		}
		logger.Info("deleted kubernetes node", "node", node.Name)
	}

	// Step 3: Delete the Hetzner server (use node name which matches server name)
	if node.Name != "" {
		if err := r.hcloudClient.DeleteServer(ctx, node.Name); err != nil {
			logger.Error(err, "failed to delete hetzner server", "name", node.Name)
			// Continue anyway - server might already be gone
		} else {
			logger.Info("deleted hetzner server", "name", node.Name, "serverID", node.ServerID)
		}
	}

	// Step 4: Create new server
	// TODO: Implement server creation with proper Talos config
	logger.Info("TODO: Create replacement control plane server")

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
	// TODO: Implement proper drain with pod eviction
	logger.Info("TODO: Drain node", "node", node.Name)

	// Step 3: Delete the Kubernetes node
	if err := r.Get(ctx, types.NamespacedName{Name: node.Name}, k8sNode); err == nil {
		if err := r.Delete(ctx, k8sNode); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete k8s node: %w", err)
		}
		logger.Info("deleted kubernetes node", "node", node.Name)
	}

	// Step 4: Delete the Hetzner server (use node name which matches server name)
	if node.Name != "" {
		if err := r.hcloudClient.DeleteServer(ctx, node.Name); err != nil {
			logger.Error(err, "failed to delete hetzner server", "name", node.Name)
			// Continue anyway - server might already be gone
		} else {
			logger.Info("deleted hetzner server", "name", node.Name, "serverID", node.ServerID)
		}
	}

	// Step 5: Create new server
	// TODO: Implement server creation with proper Talos config
	logger.Info("TODO: Create replacement worker server")

	return nil
}

// updateClusterPhase updates the overall cluster phase based on status.
func (r *ClusterReconciler) updateClusterPhase(cluster *k8znerv1alpha1.K8znerCluster) {
	cpReady := cluster.Status.ControlPlanes.Ready == cluster.Status.ControlPlanes.Desired
	workersReady := cluster.Status.Workers.Ready == cluster.Status.Workers.Desired

	if cpReady && workersReady {
		cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseRunning
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:    k8znerv1alpha1.ConditionReady,
			Status:  metav1.ConditionTrue,
			Reason:  "AllHealthy",
			Message: "All nodes are healthy",
		})
	} else if cluster.Status.Phase != k8znerv1alpha1.ClusterPhaseHealing {
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
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8znerv1alpha1.K8znerCluster{}).
		// Watch nodes for changes
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
		if cond.Type == corev1.NodeReady && cond.Status != corev1.ConditionTrue {
			return fmt.Sprintf("NodeNotReady: %s", cond.Message)
		}
		if cond.Type == corev1.NodeMemoryPressure && cond.Status == corev1.ConditionTrue {
			return "MemoryPressure"
		}
		if cond.Type == corev1.NodeDiskPressure && cond.Status == corev1.ConditionTrue {
			return "DiskPressure"
		}
		if cond.Type == corev1.NodePIDPressure && cond.Status == corev1.ConditionTrue {
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
			Name:      "cluster", // Default cluster name
		},
	})
}
