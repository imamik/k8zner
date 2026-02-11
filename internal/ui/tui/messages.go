// Package tui provides a Bubble Tea-based terminal UI for cluster provisioning.
package tui

import k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"

// BootstrapPhaseMsg reports progress of CLI bootstrap phases.
type BootstrapPhaseMsg struct {
	Phase string
	Done  bool
	Err   error
}

// CRDStatusMsg carries the latest cluster status from the CRD.
type CRDStatusMsg struct {
	ClusterPhase    k8znerv1alpha1.ClusterPhase
	ProvPhase       k8znerv1alpha1.ProvisioningPhase
	Infrastructure  k8znerv1alpha1.InfrastructureStatus
	ControlPlanes   k8znerv1alpha1.NodeGroupStatus
	Workers         k8znerv1alpha1.NodeGroupStatus
	Addons          map[string]k8znerv1alpha1.AddonStatus
	PhaseHistory    []k8znerv1alpha1.PhaseRecord
	LastErrors      []k8znerv1alpha1.ErrorRecord
	LastReconcile   string
	PhaseStartedAt  string
}

// TickMsg is sent periodically to refresh the display.
type TickMsg struct{}

// ErrMsg carries an error.
type ErrMsg struct{ Err error }

// DoneMsg signals that the operation is complete.
type DoneMsg struct{}
