package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/ui/benchmarks"
)

// styleFunc is a single-string styling function.
type styleFunc func(string) string

// sf wraps a lipgloss.Style into a styleFunc.
func sf(s lipgloss.Style) styleFunc {
	return func(str string) string { return s.Render(str) }
}

func renderView(m Model) string {
	var b strings.Builder

	// Header
	renderHeader(&b, m)

	// Progress bar (apply mode with CRD data)
	if m.Mode == "apply" || (m.Mode == "doctor" && m.ProvPhase != "" && m.ProvPhase != k8znerv1alpha1.PhaseComplete) {
		renderProgressBar(&b, m)
	}

	// Bootstrap phases (apply mode, pre-CRD)
	if m.Mode == "apply" && !m.BootstrapDone {
		renderBootstrapPhases(&b, m)
	}

	// Infrastructure
	renderInfrastructure(&b, m)

	// Nodes
	renderNodes(&b, m)

	// Addons
	renderAddons(&b, m)

	// Phase History
	if len(m.PhaseHistory) > 0 {
		renderPhaseHistory(&b, m)
	}

	// Errors
	if len(m.LastErrors) > 0 {
		renderErrors(&b, m)
	}

	// Footer
	renderFooter(&b, m)

	return b.String()
}

func renderHeader(b *strings.Builder, m Model) {
	title := fmt.Sprintf("k8zner: %s", m.ClusterName)
	if m.Region != "" {
		title += fmt.Sprintf(" (%s)", m.Region)
	}
	b.WriteString(titleStyle.Render(title))

	status := " "
	switch {
	case m.Done:
		status += readyStyle.Render("Running")
	case m.Err != nil:
		status += failedStyle.Render(fmt.Sprintf("Error: %v", m.Err))
	case m.ClusterPhase == k8znerv1alpha1.ClusterPhaseRunning:
		status += readyStyle.Render("Running")
	case m.ClusterPhase == k8znerv1alpha1.ClusterPhaseFailed:
		status += failedStyle.Render("Failed")
	case m.ProvPhase != "":
		status += activeStyle.Render(currentSpinner(m.SpinnerFrame)+" ") + warningStyle.Render(string(m.ProvPhase))
	default:
		status += dimStyle.Render("Bootstrapping...")
	}
	b.WriteString(status)
	b.WriteString("\n")
}

func renderProgressBar(b *strings.Builder, m Model) {
	progress := calculateProgress(m)
	barWidth := 40
	if m.Width > 0 && m.Width < 80 {
		barWidth = m.Width - 30
		if barWidth < 10 {
			barWidth = 10
		}
	}
	filled := int(float64(barWidth) * progress)
	if filled > barWidth {
		filled = barWidth
	}

	bar := progressBarFull.Render(strings.Repeat("█", filled)) +
		progressBarEmpty.Render(strings.Repeat("░", barWidth-filled))

	pct := int(progress * 100)
	eta := ""
	if m.EstimatedRemaining > 0 {
		eta = fmt.Sprintf(" ETA %s", formatDuration(m.EstimatedRemaining))
	}
	if m.PerformanceScale != 0 && m.PerformanceScale != 1.0 {
		eta += fmt.Sprintf("  speed x%.2f", m.PerformanceScale)
	}

	fmt.Fprintf(b, "  %s %d%%%s\n", bar, pct, eta)
}

func renderBootstrapPhases(b *strings.Builder, m Model) {
	b.WriteString(sectionStyle.Render("  Bootstrap"))
	b.WriteString("\n")

	for _, phase := range m.BootstrapPhases {
		var icon string
		var style styleFunc
		switch {
		case phase.Err != nil:
			icon = crossMark
			style = sf(failedStyle)
		case phase.Done:
			icon = checkMark
			style = sf(readyStyle)
		case phase.Active:
			icon = currentSpinner(m.SpinnerFrame)
			style = sf(activeStyle)
		default:
			icon = pending
			style = sf(dimStyle)
		}
		fmt.Fprintf(b, "    %s %s\n", style(icon), style(phase.Name))
	}
}

func renderInfrastructure(b *strings.Builder, m Model) {
	b.WriteString(sectionStyle.Render("  Infrastructure"))
	b.WriteString("\n")

	items := []struct {
		name  string
		ready bool
	}{
		{"Network", m.Infrastructure.NetworkID != 0},
		{"Firewall", m.Infrastructure.FirewallID != 0},
		{"Load Balancer", m.Infrastructure.LoadBalancerID != 0},
		{"Placement Group", m.Infrastructure.PlacementGroupID != 0},
	}

	for _, item := range items {
		icon, style := statusIcon(item.ready)
		fmt.Fprintf(b, "    %s %-20s\n", style(icon), style(item.name))
	}
}

func renderNodes(b *strings.Builder, m Model) {
	b.WriteString(sectionStyle.Render("  Nodes"))
	b.WriteString("\n")

	// Control planes
	cpReady := m.ControlPlanes.Ready == m.ControlPlanes.Desired && m.ControlPlanes.Desired > 0
	cpIcon, cpStyle := statusIcon(cpReady)
	fmt.Fprintf(b, "    %s %-20s %d/%d\n",
		cpStyle(cpIcon), cpStyle("Control Planes"), m.ControlPlanes.Ready, m.ControlPlanes.Desired)

	// Individual control plane nodes
	for _, node := range m.ControlPlanes.Nodes {
		nodeIcon, nodeStyle := nodePhaseIcon(node.Phase)
		dur := ""
		if node.PhaseTransitionTime != nil {
			dur = formatDuration(time.Since(node.PhaseTransitionTime.Time))
		}
		fmt.Fprintf(b, "      %s %-18s %-20s %s\n",
			nodeStyle(nodeIcon), node.Name, nodeStyle(string(node.Phase)), dimStyle.Render(dur))
	}

	// Workers
	wReady := m.Workers.Ready == m.Workers.Desired && m.Workers.Desired > 0
	wIcon, wStyle := statusIcon(wReady)
	fmt.Fprintf(b, "    %s %-20s %d/%d\n",
		wStyle(wIcon), wStyle("Workers"), m.Workers.Ready, m.Workers.Desired)

	for _, node := range m.Workers.Nodes {
		nodeIcon, nodeStyle := nodePhaseIcon(node.Phase)
		dur := ""
		if node.PhaseTransitionTime != nil {
			dur = formatDuration(time.Since(node.PhaseTransitionTime.Time))
		}
		fmt.Fprintf(b, "      %s %-18s %-20s %s\n",
			nodeStyle(nodeIcon), node.Name, nodeStyle(string(node.Phase)), dimStyle.Render(dur))
	}
}

func renderAddons(b *strings.Builder, m Model) {
	if len(m.Addons) == 0 {
		return
	}

	b.WriteString(sectionStyle.Render("  Addons"))
	b.WriteString("\n")

	addonOrder := []string{
		k8znerv1alpha1.AddonNameCilium,
		k8znerv1alpha1.AddonNameCCM,
		k8znerv1alpha1.AddonNameCSI,
		k8znerv1alpha1.AddonNameMetricsServer,
		k8znerv1alpha1.AddonNameCertManager,
		k8znerv1alpha1.AddonNameTraefik,
		k8znerv1alpha1.AddonNameExternalDNS,
		k8znerv1alpha1.AddonNameArgoCD,
		k8znerv1alpha1.AddonNameMonitoring,
		k8znerv1alpha1.AddonNameTalosBackup,
	}

	printed := make(map[string]bool)
	for _, name := range addonOrder {
		if addon, ok := m.Addons[name]; ok {
			renderAddonRow(b, m, name, addon)
			printed[name] = true
		}
	}
	for name, addon := range m.Addons {
		if !printed[name] {
			renderAddonRow(b, m, name, addon)
		}
	}
}

func renderAddonRow(b *strings.Builder, m Model, name string, addon k8znerv1alpha1.AddonStatus) {
	var icon string
	var style styleFunc

	switch addon.Phase {
	case k8znerv1alpha1.AddonPhaseInstalled:
		icon, style = statusIcon(addon.Healthy)
	case k8znerv1alpha1.AddonPhaseInstalling:
		icon = currentSpinner(m.SpinnerFrame)
		style = sf(activeStyle)
	case k8znerv1alpha1.AddonPhaseFailed:
		icon = crossMark
		style = sf(failedStyle)
	default:
		icon = pending
		style = sf(dimStyle)
	}

	extra := ""
	switch {
	case addon.Duration != "":
		extra = sf(dimStyle)(addon.Duration)
	case addon.Phase == k8znerv1alpha1.AddonPhaseInstalling:
		extra = sf(activeStyle)("installing")
		if addon.RetryCount > 0 {
			extra += sf(warningStyle)(fmt.Sprintf(" (retry %d)", addon.RetryCount))
		}
	case addon.Phase == k8znerv1alpha1.AddonPhaseFailed && addon.RetryCount > 0:
		extra = sf(warningStyle)(fmt.Sprintf("retry %d", addon.RetryCount))
	}

	bar := ""
	if (addon.Phase == k8znerv1alpha1.AddonPhaseInstalling || addon.Phase == k8znerv1alpha1.AddonPhaseInstalled) && addon.StartedAt != nil {
		expected := 60 * time.Second
		if exp, ok := benchmarks.AddonExpectedDuration(name); ok {
			expected = time.Duration(float64(exp) * m.PerformanceScale)
		}
		elapsed := time.Since(addon.StartedAt.Time)
		progress := float64(elapsed) / float64(expected)
		if addon.Phase == k8znerv1alpha1.AddonPhaseInstalled || progress > 1 {
			progress = 1
		}
		bar = " " + addonMiniBar(progress)
	}

	fmt.Fprintf(b, "    %s %-20s %s%s\n", style(icon), style(name), extra, bar)
}

func renderPhaseHistory(b *strings.Builder, m Model) {
	b.WriteString(sectionStyle.Render("  Phase History"))
	b.WriteString("\n")

	for _, rec := range m.PhaseHistory {
		icon := checkMark
		style := readyStyle.Render
		dur := ""
		if rec.EndedAt != nil {
			dur = rec.Duration
		} else {
			icon = currentSpinner(m.SpinnerFrame)
			style = activeStyle.Render
			dur = formatDuration(time.Since(rec.StartedAt.Time))
		}
		if rec.Error != "" {
			icon = crossMark
			style = failedStyle.Render
		}
		fmt.Fprintf(b, "    %s %-18s %s\n",
			style(icon), style(string(rec.Phase)), dimStyle.Render(dur))
	}
}

func renderErrors(b *strings.Builder, m Model) {
	b.WriteString(sectionStyle.Render("  Recent Errors"))
	b.WriteString("\n")

	// Show last 3 errors
	start := 0
	if len(m.LastErrors) > 3 {
		start = len(m.LastErrors) - 3
	}
	for _, err := range m.LastErrors[start:] {
		component := err.Component
		if component == "" {
			component = err.Phase
		}
		fmt.Fprintf(b, "    %s [%s] %s\n",
			failedStyle.Render(crossMark), component, dimStyle.Render(err.Message))
	}
}

func renderFooter(b *strings.Builder, m Model) {
	elapsed := formatDuration(time.Since(m.StartTime))
	parts := []string{fmt.Sprintf("elapsed: %s", elapsed)}
	if m.LastReconcile != "" {
		parts = append(parts, fmt.Sprintf("last reconcile: %s", m.LastReconcile))
	}
	pulse := ""
	if !m.Done && m.ClusterPhase != k8znerv1alpha1.ClusterPhaseRunning {
		pulse = "  |  " + currentSpinner(m.SpinnerFrame) + " reconciling"
	}
	b.WriteString(footerStyle.Render(fmt.Sprintf("  %s%s  |  q: quit", strings.Join(parts, "  |  "), pulse)))
	b.WriteString("\n")
}

// Helper functions

func statusIcon(ready bool) (string, styleFunc) {
	if ready {
		return checkMark, sf(readyStyle)
	}
	return crossMark, sf(failedStyle)
}

func nodePhaseIcon(phase k8znerv1alpha1.NodePhase) (string, styleFunc) {
	switch phase {
	case k8znerv1alpha1.NodePhaseReady:
		return checkMark, sf(readyStyle)
	case k8znerv1alpha1.NodePhaseFailed:
		return crossMark, sf(failedStyle)
	case k8znerv1alpha1.NodePhaseUnhealthy:
		return warnMark, sf(warningStyle)
	default:
		return "◌", sf(activeStyle)
	}
}

func currentSpinner(frame int) string {
	if len(spinnerFrames) == 0 {
		return spinner
	}
	if frame < 0 {
		frame = -frame
	}
	return spinnerFrames[frame%len(spinnerFrames)]
}

func addonMiniBar(progress float64) string {
	const width = 10
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	filled := int(progress * width)
	if filled > width {
		filled = width
	}
	return progressBarFull.Render(strings.Repeat("█", filled)) + progressBarEmpty.Render(strings.Repeat("░", width-filled))
}

func calculateProgress(m Model) float64 {
	if m.Done || m.ClusterPhase == k8znerv1alpha1.ClusterPhaseRunning {
		return 1.0
	}

	// Weight: bootstrap phases = 40%, operator phases = 60%
	if !m.BootstrapDone && m.Mode == "apply" {
		done := 0
		for _, p := range m.BootstrapPhases {
			if p.Done {
				done++
			}
		}
		return float64(done) / float64(len(m.BootstrapPhases)) * 0.4
	}

	phaseWeights := map[k8znerv1alpha1.ProvisioningPhase]float64{
		k8znerv1alpha1.PhaseInfrastructure: 0.05,
		k8znerv1alpha1.PhaseImage:          0.15,
		k8znerv1alpha1.PhaseCompute:        0.10,
		k8znerv1alpha1.PhaseBootstrap:      0.20,
		k8znerv1alpha1.PhaseCNI:            0.15,
		k8znerv1alpha1.PhaseAddons:         0.30,
		k8znerv1alpha1.PhaseComplete:       0.05,
	}

	var progress float64
	for _, rec := range m.PhaseHistory {
		if rec.EndedAt != nil {
			if w, ok := phaseWeights[rec.Phase]; ok {
				progress += w
			}
		}
	}

	offset := 0.0
	if m.Mode == "apply" {
		offset = 0.4 // bootstrap phases already done
		// Scale operator phases into remaining 60%
		progress = offset + progress*0.6
	}

	if progress > 1.0 {
		progress = 1.0
	}
	return progress
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
