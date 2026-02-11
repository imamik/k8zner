package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RunDoctorTUI runs the doctor command with a Bubble Tea TUI.
func RunDoctorTUI(ctx context.Context, k8sClient client.Client, clusterName string) error {
	m := NewDoctorModel(clusterName)

	p := tea.NewProgram(m, tea.WithAltScreen())

	// Poll CRD status in background
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		// Fetch immediately
		msg, _ := fetchDoctorStatus(ctx, k8sClient, clusterName)
		p.Send(msg)

		for {
			select {
			case <-ctx.Done():
				p.Send(ErrMsg{Err: ctx.Err()})
				return
			case <-ticker.C:
				msg, _ := fetchDoctorStatus(ctx, k8sClient, clusterName)
				p.Send(msg)
			}
		}
	}()

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	fm := finalModel.(Model)
	if fm.Err != nil {
		return fm.Err
	}
	return nil
}

func fetchDoctorStatus(ctx context.Context, k8sClient client.Client, clusterName string) (CRDStatusMsg, bool) {
	cluster := &k8znerv1alpha1.K8znerCluster{}
	key := client.ObjectKey{
		Namespace: "k8zner-system",
		Name:      clusterName,
	}

	if err := k8sClient.Get(ctx, key, cluster); err != nil {
		return CRDStatusMsg{}, false
	}

	lastReconcile := ""
	if cluster.Status.LastReconcileTime != nil {
		lastReconcile = time.Since(cluster.Status.LastReconcileTime.Time).Round(time.Second).String() + " ago"
	}

	msg := CRDStatusMsg{
		ClusterPhase:   cluster.Status.Phase,
		ProvPhase:      cluster.Status.ProvisioningPhase,
		Infrastructure: cluster.Status.Infrastructure,
		ControlPlanes:  cluster.Status.ControlPlanes,
		Workers:        cluster.Status.Workers,
		Addons:         cluster.Status.Addons,
		PhaseHistory:   cluster.Status.PhaseHistory,
		LastErrors:     cluster.Status.LastErrors,
		LastReconcile:  lastReconcile,
	}

	done := cluster.Status.Phase == k8znerv1alpha1.ClusterPhaseRunning

	return msg, done
}

// RenderDoctorOnce renders doctor output once using lipgloss (non-watch mode).
func RenderDoctorOnce(status CRDStatusMsg, clusterName, region string) string {
	m := NewDoctorModel(clusterName)
	m.Region = region
	m.updateCRDStatus(status)
	return renderView(m)
}
