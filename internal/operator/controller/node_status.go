// Package controller contains the Kubernetes controllers for the k8zner operator.
package controller

import (
	"context"
	"fmt"
	"time"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Phase timeout configuration - how long a node can be stuck in each phase before being marked as failed.
var phaseTimeouts = map[k8znerv1alpha1.NodePhase]time.Duration{
	k8znerv1alpha1.NodePhaseCreatingServer:      10 * time.Minute,
	k8znerv1alpha1.NodePhaseWaitingForIP:        5 * time.Minute,
	k8znerv1alpha1.NodePhaseWaitingForTalosAPI:  10 * time.Minute,
	k8znerv1alpha1.NodePhaseApplyingTalosConfig: 5 * time.Minute,
	k8znerv1alpha1.NodePhaseRebootingWithConfig: 10 * time.Minute,
	k8znerv1alpha1.NodePhaseWaitingForK8s:       10 * time.Minute,
	k8znerv1alpha1.NodePhaseNodeInitializing:    10 * time.Minute,
	k8znerv1alpha1.NodePhaseDraining:            15 * time.Minute,
	k8znerv1alpha1.NodePhaseRemovingFromEtcd:    5 * time.Minute,
	k8znerv1alpha1.NodePhaseDeletingServer:      5 * time.Minute,
}

// nodeStatusUpdate represents an update to a node's status.
type nodeStatusUpdate struct {
	Name      string
	ServerID  int64
	PublicIP  string
	PrivateIP string
	Phase     k8znerv1alpha1.NodePhase
	Reason    string
}

// updateNodePhase updates or adds a node's phase in the cluster status.
// If the node doesn't exist in the status, it will be added.
// This allows tracking nodes during provisioning before they become k8s nodes.
func (r *ClusterReconciler) updateNodePhase(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, role string, update nodeStatusUpdate) {
	logger := log.FromContext(ctx)
	now := metav1.Now()

	var nodes *[]k8znerv1alpha1.NodeStatus
	if role == "control-plane" {
		nodes = &cluster.Status.ControlPlanes.Nodes
	} else {
		nodes = &cluster.Status.Workers.Nodes
	}

	// Find existing node or create new entry
	found := false
	for i := range *nodes {
		if (*nodes)[i].Name != update.Name {
			continue
		}
		// Only update PhaseTransitionTime if phase actually changed
		if (*nodes)[i].Phase != update.Phase {
			(*nodes)[i].PhaseTransitionTime = &now
		}
		// Update existing node
		(*nodes)[i].Phase = update.Phase
		(*nodes)[i].PhaseReason = update.Reason
		if update.ServerID != 0 {
			(*nodes)[i].ServerID = update.ServerID
		}
		if update.PublicIP != "" {
			(*nodes)[i].PublicIP = update.PublicIP
		}
		if update.PrivateIP != "" {
			(*nodes)[i].PrivateIP = update.PrivateIP
		}
		// Update health based on phase
		(*nodes)[i].Healthy = update.Phase == k8znerv1alpha1.NodePhaseReady
		found = true
		break
	}

	if !found {
		// Add new node entry for tracking during provisioning
		newNode := k8znerv1alpha1.NodeStatus{
			Name:                update.Name,
			ServerID:            update.ServerID,
			PublicIP:            update.PublicIP,
			PrivateIP:           update.PrivateIP,
			Phase:               update.Phase,
			PhaseReason:         update.Reason,
			PhaseTransitionTime: &now,
			Healthy:             update.Phase == k8znerv1alpha1.NodePhaseReady,
		}
		*nodes = append(*nodes, newNode)
	}

	logger.Info("node phase updated",
		"node", update.Name,
		"role", role,
		"phase", update.Phase,
		"reason", update.Reason,
	)
}

// updateNodePhaseAndPersist updates the node phase and persists to the CRD.
// Use this when you need the status change to be immediately visible.
func (r *ClusterReconciler) updateNodePhaseAndPersist(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, role string, update nodeStatusUpdate) error {
	r.updateNodePhase(ctx, cluster, role, update)
	return r.persistClusterStatus(ctx, cluster)
}

// persistClusterStatus saves the cluster status to the Kubernetes API with conflict retry.
// On conflict, it re-fetches the latest version and merges our status changes.
func (r *ClusterReconciler) persistClusterStatus(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) error {
	logger := log.FromContext(ctx)

	for i := 0; i < statusUpdateRetries; i++ {
		err := r.Status().Update(ctx, cluster)
		if err == nil {
			return nil
		}

		if !apierrors.IsConflict(err) {
			logger.Error(err, "failed to persist cluster status")
			return fmt.Errorf("failed to persist cluster status: %w", err)
		}

		// On conflict, re-fetch the latest version and re-apply our status changes
		logger.V(1).Info("persist status conflict, retrying", "attempt", i+1)

		latest := &k8znerv1alpha1.K8znerCluster{}
		if getErr := r.Get(ctx, client.ObjectKeyFromObject(cluster), latest); getErr != nil {
			return fmt.Errorf("failed to get latest cluster for status retry: %w", getErr)
		}

		// Preserve our status changes on the latest version
		savedAddons := latest.Status.Addons
		latest.Status = cluster.Status
		if latest.Status.Addons == nil && savedAddons != nil {
			latest.Status.Addons = savedAddons
		}
		cluster.ObjectMeta = latest.ObjectMeta

		time.Sleep(statusRetryInterval)
	}

	return fmt.Errorf("failed to persist cluster status after %d retries", statusUpdateRetries)
}

// removeNodeFromStatus removes a node from the cluster status.
func (r *ClusterReconciler) removeNodeFromStatus(cluster *k8znerv1alpha1.K8znerCluster, role string, nodeName string) {
	var nodes *[]k8znerv1alpha1.NodeStatus
	if role == "control-plane" {
		nodes = &cluster.Status.ControlPlanes.Nodes
	} else {
		nodes = &cluster.Status.Workers.Nodes
	}

	for i := range *nodes {
		if (*nodes)[i].Name == nodeName {
			// Remove by replacing with last element and truncating
			(*nodes)[i] = (*nodes)[len(*nodes)-1]
			*nodes = (*nodes)[:len(*nodes)-1]
			return
		}
	}
}

// checkStuckNodes checks for nodes that have been stuck in a phase too long.
// Returns a list of stuck nodes that should be cleaned up.
func (r *ClusterReconciler) checkStuckNodes(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) []stuckNode {
	logger := log.FromContext(ctx)
	var stuck []stuckNode

	checkNodes := func(nodes []k8znerv1alpha1.NodeStatus, role string) {
		for _, node := range nodes {
			// Skip nodes that are Ready, Unhealthy (handled separately), or have no transition time
			if node.Phase == k8znerv1alpha1.NodePhaseReady ||
				node.Phase == k8znerv1alpha1.NodePhaseUnhealthy ||
				node.Phase == k8znerv1alpha1.NodePhaseFailed ||
				node.PhaseTransitionTime == nil {
				continue
			}

			timeout, hasTimeout := phaseTimeouts[node.Phase]
			if !hasTimeout {
				continue
			}

			elapsed := time.Since(node.PhaseTransitionTime.Time)
			if elapsed > timeout {
				logger.Info("detected stuck node",
					"node", node.Name,
					"role", role,
					"phase", node.Phase,
					"stuckFor", elapsed.Round(time.Second),
					"timeout", timeout,
				)
				stuck = append(stuck, stuckNode{
					Name:    node.Name,
					Role:    role,
					Phase:   node.Phase,
					Elapsed: elapsed,
					Timeout: timeout,
				})
			}
		}
	}

	checkNodes(cluster.Status.ControlPlanes.Nodes, "control-plane")
	checkNodes(cluster.Status.Workers.Nodes, "worker")

	return stuck
}

// stuckNode represents a node that has been stuck in a phase too long.
type stuckNode struct {
	Name    string
	Role    string
	Phase   k8znerv1alpha1.NodePhase
	Elapsed time.Duration
	Timeout time.Duration
}

// handleStuckNode handles a node that has been stuck too long by marking it as failed
// and cleaning up resources.
func (r *ClusterReconciler) handleStuckNode(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, stuck stuckNode) error {
	logger := log.FromContext(ctx)

	logger.Info("handling stuck node",
		"node", stuck.Name,
		"role", stuck.Role,
		"phase", stuck.Phase,
		"stuckFor", stuck.Elapsed.Round(time.Second),
	)

	// Mark as failed
	r.updateNodePhase(ctx, cluster, stuck.Role, nodeStatusUpdate{
		Name:   stuck.Name,
		Phase:  k8znerv1alpha1.NodePhaseFailed,
		Reason: fmt.Sprintf("Stuck in %s phase for %s (timeout: %s)", stuck.Phase, stuck.Elapsed.Round(time.Second), stuck.Timeout),
	})

	// Clean up the server if it exists
	if err := r.hcloudClient.DeleteServer(ctx, stuck.Name); err != nil {
		logger.Error(err, "failed to delete stuck server", "name", stuck.Name)
		// Don't return error - continue with cleanup
	} else {
		logger.Info("deleted stuck server", "name", stuck.Name)
	}

	// Remove from status
	r.removeNodeFromStatus(cluster, stuck.Role, stuck.Name)

	return nil
}

// verifyAndUpdateNodeStates uses the state verifier to check actual node states
// and update phases accordingly.
func (r *ClusterReconciler) verifyAndUpdateNodeStates(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) error {
	logger := log.FromContext(ctx)

	verifyNodes := func(nodes []k8znerv1alpha1.NodeStatus, role string) error {
		for _, node := range nodes {
			// Skip nodes in terminal states or without IPs
			if node.Phase == k8znerv1alpha1.NodePhaseFailed ||
				node.Phase == k8znerv1alpha1.NodePhaseDeletingServer {
				continue
			}

			// Get the IP to use for verification
			nodeIP := node.PublicIP
			if nodeIP == "" {
				nodeIP = node.PrivateIP
			}

			// Verify actual state
			stateInfo, err := r.verifyNodeState(ctx, node.Name, nodeIP)
			if err != nil {
				logger.V(1).Info("failed to verify node state", "node", node.Name, "error", err)
				continue
			}

			// Determine what phase the node should be in based on actual state
			actualPhase, reason := determineNodePhaseFromState(stateInfo)

			// Only update if phase changed and it's a forward progression or error
			if actualPhase != node.Phase && shouldUpdatePhase(node.Phase, actualPhase) {
				logger.Info("node state verified - updating phase",
					"node", node.Name,
					"role", role,
					"previousPhase", node.Phase,
					"newPhase", actualPhase,
					"reason", reason,
				)
				r.updateNodePhase(ctx, cluster, role, nodeStatusUpdate{
					Name:      node.Name,
					Phase:     actualPhase,
					Reason:    reason,
					PublicIP:  stateInfo.ServerIP,
					PrivateIP: node.PrivateIP, // Preserve existing private IP
				})
			}
		}
		return nil
	}

	if err := verifyNodes(cluster.Status.ControlPlanes.Nodes, "control-plane"); err != nil {
		return err
	}
	return verifyNodes(cluster.Status.Workers.Nodes, "worker")
}

// shouldUpdatePhase determines if we should update from currentPhase to newPhase.
// We allow forward progression and error states, but not backward progression
// (which might indicate temporary API issues).
func shouldUpdatePhase(current, newPhase k8znerv1alpha1.NodePhase) bool {
	// Always allow transition to Failed or Ready
	if newPhase == k8znerv1alpha1.NodePhaseFailed || newPhase == k8znerv1alpha1.NodePhaseReady {
		return true
	}

	// Define phase order for forward progression
	phaseOrder := map[k8znerv1alpha1.NodePhase]int{
		k8znerv1alpha1.NodePhaseCreatingServer:      1,
		k8znerv1alpha1.NodePhaseWaitingForIP:        2,
		k8znerv1alpha1.NodePhaseWaitingForTalosAPI:  3,
		k8znerv1alpha1.NodePhaseApplyingTalosConfig: 4,
		k8znerv1alpha1.NodePhaseRebootingWithConfig: 5,
		k8znerv1alpha1.NodePhaseWaitingForK8s:       6,
		k8znerv1alpha1.NodePhaseNodeInitializing:    7,
		k8znerv1alpha1.NodePhaseReady:               8,
		k8znerv1alpha1.NodePhaseUnhealthy:           9,
		k8znerv1alpha1.NodePhaseDraining:            10,
		k8znerv1alpha1.NodePhaseRemovingFromEtcd:    11,
		k8znerv1alpha1.NodePhaseDeletingServer:      12,
		k8znerv1alpha1.NodePhaseFailed:              13,
	}

	currentOrder, currentOK := phaseOrder[current]
	newOrder, newOK := phaseOrder[newPhase]

	// If either phase is unknown, allow the update
	if !currentOK || !newOK {
		return true
	}

	// Allow forward progression
	return newOrder > currentOrder
}

// isNodeInEarlyProvisioningPhase returns true if the node is in a provisioning phase
// that indicates it's still being created and set up. This is used to prevent
// duplicate server creation from concurrent reconciles.
func isNodeInEarlyProvisioningPhase(phase k8znerv1alpha1.NodePhase) bool {
	switch phase {
	case k8znerv1alpha1.NodePhaseCreatingServer,
		k8znerv1alpha1.NodePhaseWaitingForIP,
		k8znerv1alpha1.NodePhaseWaitingForTalosAPI,
		k8znerv1alpha1.NodePhaseApplyingTalosConfig,
		k8znerv1alpha1.NodePhaseRebootingWithConfig,
		k8znerv1alpha1.NodePhaseWaitingForK8s,
		k8znerv1alpha1.NodePhaseNodeInitializing:
		return true
	default:
		return false
	}
}

// countWorkersInEarlyProvisioning returns the count of workers currently being provisioned.
// This helps prevent duplicate server creation when concurrent reconciles see stale status.
func countWorkersInEarlyProvisioning(nodes []k8znerv1alpha1.NodeStatus) int {
	count := 0
	for _, node := range nodes {
		if isNodeInEarlyProvisioningPhase(node.Phase) {
			count++
		}
	}
	return count
}

// findUnhealthyNodes returns nodes that have been unhealthy past the given threshold.
func findUnhealthyNodes(nodes []k8znerv1alpha1.NodeStatus, threshold time.Duration) []*k8znerv1alpha1.NodeStatus {
	var unhealthy []*k8znerv1alpha1.NodeStatus
	for i := range nodes {
		node := &nodes[i]
		if !node.Healthy && node.UnhealthySince != nil {
			if time.Since(node.UnhealthySince.Time) > threshold {
				unhealthy = append(unhealthy, node)
			}
		}
	}
	return unhealthy
}

// getPrivateIPFromServer retrieves the private IP from HCloud for a server.
func (r *ClusterReconciler) getPrivateIPFromServer(ctx context.Context, serverName string) (string, error) {
	server, err := r.hcloudClient.GetServerByName(ctx, serverName)
	if err != nil {
		return "", err
	}
	if server == nil {
		return "", fmt.Errorf("server %s not found", serverName)
	}

	// Get private IP from private networks
	for _, privNet := range server.PrivateNet {
		if privNet.IP != nil {
			return privNet.IP.String(), nil
		}
	}

	return "", nil
}
