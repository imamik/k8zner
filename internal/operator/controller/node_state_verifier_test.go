package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
)

func TestDetermineNodePhaseFromState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		info          *nodeStateInfo
		expectedPhase k8znerv1alpha1.NodePhase
	}{
		{
			name: "K8s node exists and ready",
			info: &nodeStateInfo{
				ServerExists:  true,
				ServerStatus:  "running",
				K8sNodeExists: true,
				K8sNodeReady:  true,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseReady,
		},
		{
			name: "K8s node exists, not ready, kubelet running",
			info: &nodeStateInfo{
				ServerExists:        true,
				ServerStatus:        "running",
				K8sNodeExists:       true,
				K8sNodeReady:        false,
				TalosKubeletRunning: true,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseNodeInitializing,
		},
		{
			name: "K8s node exists, not ready, kubelet not running",
			info: &nodeStateInfo{
				ServerExists:        true,
				ServerStatus:        "running",
				K8sNodeExists:       true,
				K8sNodeReady:        false,
				TalosKubeletRunning: false,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseWaitingForK8s,
		},
		{
			name: "Talos configured, kubelet running, no K8s node",
			info: &nodeStateInfo{
				ServerExists:        true,
				ServerStatus:        "running",
				TalosConfigured:     true,
				TalosKubeletRunning: true,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseWaitingForK8s,
		},
		{
			name: "Talos configured, kubelet not running",
			info: &nodeStateInfo{
				ServerExists:        true,
				ServerStatus:        "running",
				TalosConfigured:     true,
				TalosKubeletRunning: false,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseRebootingWithConfig,
		},
		{
			name: "Talos in maintenance mode",
			info: &nodeStateInfo{
				ServerExists:           true,
				ServerStatus:           "running",
				TalosAPIReachable:      true,
				TalosInMaintenanceMode: true,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseWaitingForTalosAPI,
		},
		{
			name: "Talos API reachable but not in maintenance",
			info: &nodeStateInfo{
				ServerExists:      true,
				ServerStatus:      "running",
				TalosAPIReachable: true,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseApplyingTalosConfig,
		},
		{
			name: "Server running, Talos not reachable",
			info: &nodeStateInfo{
				ServerExists: true,
				ServerStatus: "running",
				ServerIP:     "10.0.0.1",
			},
			expectedPhase: k8znerv1alpha1.NodePhaseWaitingForTalosAPI,
		},
		{
			name: "Server starting",
			info: &nodeStateInfo{
				ServerExists: true,
				ServerStatus: "starting",
			},
			expectedPhase: k8znerv1alpha1.NodePhaseWaitingForIP,
		},
		{
			name: "Server exists in unknown state",
			info: &nodeStateInfo{
				ServerExists: true,
				ServerStatus: "initializing",
			},
			expectedPhase: k8znerv1alpha1.NodePhaseCreatingServer,
		},
		{
			name: "Server does not exist",
			info: &nodeStateInfo{
				ServerExists: false,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			phase, reason := determineNodePhaseFromState(tt.info)
			assert.Equal(t, tt.expectedPhase, phase)
			assert.NotEmpty(t, reason, "reason should always be provided")
		})
	}
}

func TestDetermineNodePhaseFromState_ReasonContent(t *testing.T) {
	t.Parallel()
	t.Run("ready node has descriptive reason", func(t *testing.T) {
		t.Parallel()
		info := &nodeStateInfo{
			K8sNodeExists: true,
			K8sNodeReady:  true,
		}
		_, reason := determineNodePhaseFromState(info)
		assert.Contains(t, reason, "ready")
	})

	t.Run("failed node mentions HCloud", func(t *testing.T) {
		t.Parallel()
		info := &nodeStateInfo{
			ServerExists: false,
		}
		_, reason := determineNodePhaseFromState(info)
		assert.Contains(t, reason, "HCloud")
	})

	t.Run("waiting for talos includes IP in reason", func(t *testing.T) {
		t.Parallel()
		info := &nodeStateInfo{
			ServerExists: true,
			ServerStatus: "running",
			ServerIP:     "10.0.0.42",
		}
		_, reason := determineNodePhaseFromState(info)
		assert.Contains(t, reason, "10.0.0.42")
	})
}
