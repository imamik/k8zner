package tui

import (
	"strings"
	"testing"
	"time"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{30 * time.Second, "30s"},
		{90 * time.Second, "1m30s"},
		{3600 * time.Second, "1h0m"},
		{3661 * time.Second, "1h1m"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestCalculateProgress_Done(t *testing.T) {
	m := Model{Done: true}
	p := calculateProgress(m)
	if p != 1.0 {
		t.Errorf("expected 1.0, got %v", p)
	}
}

func TestCalculateProgress_Running(t *testing.T) {
	m := Model{ClusterPhase: k8znerv1alpha1.ClusterPhaseRunning}
	p := calculateProgress(m)
	if p != 1.0 {
		t.Errorf("expected 1.0, got %v", p)
	}
}

func TestCalculateProgress_BootstrapPhases(t *testing.T) {
	m := NewApplyModel("test", "fsn1")
	// 2 of 6 phases done
	m.BootstrapPhases[0].Done = true
	m.BootstrapPhases[1].Done = true

	p := calculateProgress(m)
	expected := 2.0 / 6.0 * 0.4
	if p < expected-0.01 || p > expected+0.01 {
		t.Errorf("expected ~%v, got %v", expected, p)
	}
}

func TestModelUpdateBootstrapPhase(t *testing.T) {
	m := NewApplyModel("test", "fsn1")

	// Start image phase
	m.updateBootstrapPhase(BootstrapPhaseMsg{Phase: "image"})
	if !m.BootstrapPhases[0].Active {
		t.Error("expected image phase to be active")
	}

	// Complete image phase
	m.updateBootstrapPhase(BootstrapPhaseMsg{Phase: "image", Done: true})
	if !m.BootstrapPhases[0].Done {
		t.Error("expected image phase to be done")
	}
	if m.BootstrapPhases[0].Active {
		t.Error("expected image phase to not be active after done")
	}

	// Start infrastructure
	m.updateBootstrapPhase(BootstrapPhaseMsg{Phase: "infrastructure"})
	if !m.BootstrapPhases[1].Active {
		t.Error("expected infrastructure to be active")
	}
}

func TestModelUpdateBootstrapPhase_AllDone(t *testing.T) {
	m := NewApplyModel("test", "fsn1")
	phases := []string{"image", "infrastructure", "compute", "bootstrap", "operator", "crd"}
	for _, p := range phases {
		m.updateBootstrapPhase(BootstrapPhaseMsg{Phase: p, Done: true})
	}
	if !m.BootstrapDone {
		t.Error("expected BootstrapDone to be true")
	}
}

func TestModelUpdateCRDStatus(t *testing.T) {
	m := NewDoctorModel("test")
	msg := CRDStatusMsg{
		ClusterPhase: k8znerv1alpha1.ClusterPhaseProvisioning,
		ProvPhase:    k8znerv1alpha1.PhaseCNI,
		Infrastructure: k8znerv1alpha1.InfrastructureStatus{
			NetworkID: 123,
		},
		Addons: map[string]k8znerv1alpha1.AddonStatus{
			"cilium": {Installed: true, Healthy: true, Phase: k8znerv1alpha1.AddonPhaseInstalled},
		},
		LastReconcile: "5s ago",
	}

	m.updateCRDStatus(msg)

	if m.ClusterPhase != k8znerv1alpha1.ClusterPhaseProvisioning {
		t.Errorf("expected Provisioning, got %v", m.ClusterPhase)
	}
	if m.Infrastructure.NetworkID != 123 {
		t.Errorf("expected NetworkID 123, got %d", m.Infrastructure.NetworkID)
	}
	if len(m.Addons) != 1 {
		t.Errorf("expected 1 addon, got %d", len(m.Addons))
	}
}

func TestRenderView_Header(t *testing.T) {
	m := NewDoctorModel("my-cluster")
	m.Region = "fsn1"
	m.StartTime = time.Now()

	output := renderView(m)

	if !strings.Contains(output, "my-cluster") {
		t.Error("expected cluster name in output")
	}
	if !strings.Contains(output, "fsn1") {
		t.Error("expected region in output")
	}
}

func TestRenderView_WithAddons(t *testing.T) {
	m := NewDoctorModel("test")
	m.StartTime = time.Now()
	m.Addons = map[string]k8znerv1alpha1.AddonStatus{
		"cilium": {
			Installed: true,
			Healthy:   true,
			Phase:     k8znerv1alpha1.AddonPhaseInstalled,
			Duration:  "45s",
		},
		"traefik": {
			Phase:      k8znerv1alpha1.AddonPhaseInstalling,
			RetryCount: 2,
		},
	}

	output := renderView(m)

	if !strings.Contains(output, "cilium") {
		t.Error("expected cilium in output")
	}
	if !strings.Contains(output, "traefik") {
		t.Error("expected traefik in output")
	}
	if !strings.Contains(output, "45s") {
		t.Error("expected duration in output")
	}
	if !strings.Contains(output, "retry 2") {
		t.Error("expected retry count in output")
	}
}

func TestRenderView_PhaseHistory(t *testing.T) {
	m := NewDoctorModel("test")
	m.StartTime = time.Now()
	now := metav1.Now()
	m.PhaseHistory = []k8znerv1alpha1.PhaseRecord{
		{Phase: "Infrastructure", StartedAt: now, EndedAt: &now, Duration: "15s"},
		{Phase: "Image", StartedAt: now},
	}

	output := renderView(m)

	if !strings.Contains(output, "Infrastructure") {
		t.Error("expected Infrastructure in phase history")
	}
	if !strings.Contains(output, "15s") {
		t.Error("expected duration in phase history")
	}
}

func TestRenderView_Errors(t *testing.T) {
	m := NewDoctorModel("test")
	m.StartTime = time.Now()
	now := metav1.Now()
	m.LastErrors = []k8znerv1alpha1.ErrorRecord{
		{Time: now, Phase: "Addons", Component: "traefik", Message: "timeout waiting"},
	}

	output := renderView(m)

	if !strings.Contains(output, "Recent Errors") {
		t.Error("expected errors section in output")
	}
	if !strings.Contains(output, "timeout waiting") {
		t.Error("expected error message in output")
	}
}

func TestRenderView_Nodes(t *testing.T) {
	m := NewDoctorModel("test")
	m.StartTime = time.Now()
	now := metav1.Now()
	m.ControlPlanes = k8znerv1alpha1.NodeGroupStatus{
		Desired: 3,
		Ready:   2,
		Nodes: []k8znerv1alpha1.NodeStatus{
			{Name: "cp-abc", Phase: k8znerv1alpha1.NodePhaseReady, PhaseTransitionTime: &now},
			{Name: "cp-def", Phase: k8znerv1alpha1.NodePhaseReady, PhaseTransitionTime: &now},
			{Name: "cp-ghi", Phase: k8znerv1alpha1.NodePhaseWaitingForK8s, PhaseTransitionTime: &now},
		},
	}

	output := renderView(m)

	if !strings.Contains(output, "cp-abc") {
		t.Error("expected node name in output")
	}
	if !strings.Contains(output, "WaitingForK8s") {
		t.Error("expected node phase in output")
	}
	if !strings.Contains(output, "2/3") {
		t.Error("expected ready count in output")
	}
}

func TestRenderView_ProgressBar(t *testing.T) {
	m := NewDoctorModel("test")
	m.StartTime = time.Now()
	m.ProvPhase = k8znerv1alpha1.PhaseCNI

	output := renderView(m)

	// Should have some progress bar characters
	if !strings.Contains(output, "░") && !strings.Contains(output, "█") {
		t.Error("expected progress bar in output")
	}
}

func TestStatusIcon(t *testing.T) {
	icon, _ := statusIcon(true)
	if icon != checkMark {
		t.Errorf("expected checkMark, got %q", icon)
	}
	icon, _ = statusIcon(false)
	if icon != crossMark {
		t.Errorf("expected crossMark, got %q", icon)
	}
}

func TestNodePhaseIcon(t *testing.T) {
	tests := []struct {
		phase k8znerv1alpha1.NodePhase
		icon  string
	}{
		{k8znerv1alpha1.NodePhaseReady, checkMark},
		{k8znerv1alpha1.NodePhaseFailed, crossMark},
		{k8znerv1alpha1.NodePhaseUnhealthy, warnMark},
		{k8znerv1alpha1.NodePhaseCreatingServer, spinner},
	}
	for _, tt := range tests {
		icon, _ := nodePhaseIcon(tt.phase)
		if icon != tt.icon {
			t.Errorf("nodePhaseIcon(%v) = %q, want %q", tt.phase, icon, tt.icon)
		}
	}
}
