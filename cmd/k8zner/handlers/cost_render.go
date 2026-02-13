package handlers

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Colors matching internal/ui/tui/styles.go palette.
var (
	costColorGreen = lipgloss.Color("#22c55e")
	costColorRed   = lipgloss.Color("#ef4444")
	costColorBlue  = lipgloss.Color("#3b82f6")
	costColorDim   = lipgloss.Color("#6b7280")
	costColorWhite = lipgloss.Color("#f9fafb")
)

var (
	costTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(costColorWhite)

	costSectionStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(costColorBlue)

	costDimStyle = lipgloss.NewStyle().
			Foreground(costColorDim)

	costGreenStyle = lipgloss.NewStyle().
			Foreground(costColorGreen)

	costRedStyle = lipgloss.NewStyle().
			Foreground(costColorRed)
)

// renderCostSummary produces a lipgloss-styled cost summary string.
func renderCostSummary(clusterName string, summary *costSummary) string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(costTitleStyle.Render(fmt.Sprintf("  k8zner cost: %s", clusterName)))
	b.WriteString("\n")
	b.WriteString(costDimStyle.Render("  " + strings.Repeat("═", 30)))
	b.WriteString("\n")

	if len(summary.Current) > 0 {
		b.WriteString("\n")
		renderCostSection(&b, "Current Resources", summary.Currency, summary.Current, summary.CurrentTotal)
	}

	if len(summary.Planned) > 0 {
		b.WriteString("\n")
		renderCostSection(&b, "Planned Resources", summary.Currency, summary.Planned, summary.PlannedTotal)
	}

	// Summary block
	b.WriteString("\n")
	b.WriteString(costSectionStyle.Render("  Summary"))
	b.WriteString("\n")
	b.WriteString(costDimStyle.Render("  " + strings.Repeat("─", 35)))
	b.WriteString("\n")

	b.WriteString(fmt.Sprintf("    Current:   %s %7.2f /mo net\n", summary.Currency, summary.CurrentTotal.MonthlyNet))
	b.WriteString(fmt.Sprintf("    Planned:   %s %7.2f /mo net\n", summary.Currency, summary.PlannedTotal.MonthlyNet))

	delta := summary.DiffTotal.MonthlyNet
	deltaStr := formatDelta(delta, summary.Currency)
	b.WriteString("    Delta:     ")
	b.WriteString(deltaStr)
	b.WriteString("\n")

	b.WriteString("\n")
	b.WriteString(costDimStyle.Render("  Note: object storage is an estimate based on configured/default GB."))
	b.WriteString("\n")

	return b.String()
}

// renderCostSection renders a section (current or planned) with table formatting.
func renderCostSection(b *strings.Builder, title, currency string, items []costLineItem, total costLineItem) {
	b.WriteString(costSectionStyle.Render("  " + title))
	b.WriteString("\n")
	b.WriteString(costDimStyle.Render("  " + strings.Repeat("─", 50)))
	b.WriteString("\n")

	// Header
	b.WriteString(costDimStyle.Render(fmt.Sprintf("  %-22s %4s %11s %10s", "Resource", "Qty", "Unit Price", "Total/mo")))
	b.WriteString("\n")

	for _, item := range items {
		unitNet := float64(0)
		if item.Count > 0 {
			unitNet = item.UnitNet
		}
		fmt.Fprintf(b, "  %-22s x%-3d %s %7.2f  %s %7.2f\n",
			item.Name,
			item.Count,
			currency, unitNet,
			currency, item.MonthlyNet,
		)
	}

	b.WriteString(costDimStyle.Render("  " + strings.Repeat("─", 50)))
	b.WriteString("\n")
	fmt.Fprintf(b, "  %-22s %18s %s %7.2f\n", "Total", "", currency, total.MonthlyNet)
}

// formatDelta returns a styled delta string with arrow indicator.
func formatDelta(delta float64, currency string) string {
	switch {
	case delta > 0.005:
		return costRedStyle.Render(fmt.Sprintf("%s %+7.2f  ▲", currency, delta))
	case delta < -0.005:
		return costGreenStyle.Render(fmt.Sprintf("%s %+7.2f  ▼", currency, delta))
	default:
		return costDimStyle.Render(fmt.Sprintf("%s  %5.2f  ─", currency, 0.0))
	}
}

// renderCostHint returns a styled one-line cost hint for init/apply.
func renderCostHint(source string, summary *costSummary) string {
	delta := summary.DiffTotal.MonthlyNet
	deltaStr := ""
	if delta > 0.005 {
		deltaStr = costRedStyle.Render(fmt.Sprintf(", delta +%.2f", delta))
	} else if delta < -0.005 {
		deltaStr = costGreenStyle.Render(fmt.Sprintf(", delta %.2f", delta))
	}

	return fmt.Sprintf("\n%s %s %.2f net (planned %s %.2f%s)\n",
		costDimStyle.Render(fmt.Sprintf("Estimated monthly cost (%s):", source)),
		summary.Currency,
		summary.CurrentTotal.MonthlyNet,
		summary.Currency,
		summary.PlannedTotal.MonthlyNet,
		deltaStr,
	)
}
