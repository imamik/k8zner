// Package controller contains the Kubernetes controllers for the k8zner operator.
package controller

import (
	"context"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// NodeStatusUpdate represents an update to a node's status.
type NodeStatusUpdate struct {
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
func (r *ClusterReconciler) updateNodePhase(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, role string, update NodeStatusUpdate) {
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
		// Update existing node
		(*nodes)[i].Phase = update.Phase
		(*nodes)[i].PhaseReason = update.Reason
		(*nodes)[i].PhaseTransitionTime = &now
		if update.ServerID != 0 {
			(*nodes)[i].ServerID = update.ServerID
		}
		if update.PublicIP != "" {
			(*nodes)[i].PublicIP = update.PublicIP
		}
		if update.PrivateIP != "" {
			(*nodes)[i].PrivateIP = update.PrivateIP
		}
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
			Healthy:             false, // Not healthy until Ready
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
