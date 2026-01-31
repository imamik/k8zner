package pricing

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Formatter formats cost estimates for display.
type Formatter struct{}

// NewFormatter creates a new formatter.
func NewFormatter() *Formatter {
	return &Formatter{}
}

// Format returns a detailed, formatted cost estimate for terminal display.
func (f *Formatter) Format(e *Estimate) string {
	var sb strings.Builder

	width := 61

	// Header
	sb.WriteString(boxTop(width))
	sb.WriteString(boxLine("k8zner Cost Estimate", width))
	sb.WriteString(boxLine(fmt.Sprintf("Cluster: %s", e.ClusterName), width))
	sb.WriteString(boxSep(width))

	// Mode info
	sb.WriteString(boxLine(fmt.Sprintf("Mode: %s", e.Mode), width))
	if e.Mode == "ha" {
		sb.WriteString(boxLine("  - 3 control planes (CX22, IPv6-only)", width))
		sb.WriteString(boxLine("  - 2 load balancers (API + ingress)", width))
	} else {
		sb.WriteString(boxLine("  - 1 control plane (CX22, IPv6-only)", width))
		sb.WriteString(boxLine("  - 1 shared load balancer", width))
	}

	// Workers info
	workerItem := findItem(e.Items, "Workers")
	if workerItem != nil {
		sb.WriteString(boxLine(fmt.Sprintf("Workers: %d x %s (IPv6-only)",
			workerItem.Quantity, strings.ToUpper(workerItem.UnitType)), width))
	}

	// Region
	sb.WriteString(boxLine(fmt.Sprintf("Region: %s", e.Region), width))
	sb.WriteString(boxSep(width))

	// Line items
	sb.WriteString(boxEmpty(width))
	for _, item := range e.Items {
		line := fmt.Sprintf("%-18s %d x %-6s %8.2f/mo",
			item.Description, item.Quantity, strings.ToUpper(item.UnitType), item.Total)
		sb.WriteString(boxLine(line, width))
	}

	// Totals
	sb.WriteString(boxDash(width))
	sb.WriteString(boxLine(fmt.Sprintf("%-28s %8.2f/mo", "Subtotal", e.Subtotal), width))
	sb.WriteString(boxLine(fmt.Sprintf("%-28s %8.2f/mo", "VAT (19% DE)", e.VAT), width))
	sb.WriteString(boxDash(width))
	sb.WriteString(boxLine(fmt.Sprintf("%-28s %8.2f/mo", "Total", e.Total), width))
	sb.WriteString(boxEmpty(width))
	sb.WriteString(boxLine(fmt.Sprintf("Annual estimate: %.2f", e.AnnualCost()), width))
	sb.WriteString(boxEmpty(width))

	// IPv6 savings note
	if e.IPv6Savings > 0 {
		sb.WriteString(boxLine(fmt.Sprintf("IPv6-only nodes save ~%.2f/mo vs IPv4", e.IPv6Savings), width))
	}

	sb.WriteString(boxBottom(width))

	// Footer
	sb.WriteString("\n  Prices from Hetzner API (EUR)\n")

	// Tip
	if e.Mode == "ha" {
		sb.WriteString("\n  Tip: Use 'mode: dev' for development (~50%% less)\n")
	}

	return sb.String()
}

// FormatCompact returns a single-line cost summary.
func (f *Formatter) FormatCompact(e *Estimate) string {
	return fmt.Sprintf("%s (%s): %.2f/mo (%.2f/yr incl. VAT)",
		e.ClusterName, e.Mode, e.Total, e.AnnualCost())
}

// FormatJSON returns the estimate as JSON.
func (f *Formatter) FormatJSON(e *Estimate) string {
	type jsonEstimate struct {
		ClusterName string     `json:"cluster_name"`
		Mode        string     `json:"mode"`
		Region      string     `json:"region"`
		Items       []LineItem `json:"items"`
		Subtotal    float64    `json:"subtotal"`
		VAT         float64    `json:"vat"`
		Total       float64    `json:"total"`
		Annual      float64    `json:"annual"`
		IPv6Savings float64    `json:"ipv6_savings"`
	}

	je := jsonEstimate{
		ClusterName: e.ClusterName,
		Mode:        string(e.Mode),
		Region:      string(e.Region),
		Items:       e.Items,
		Subtotal:    e.Subtotal,
		VAT:         e.VAT,
		Total:       e.Total,
		Annual:      e.AnnualCost(),
		IPv6Savings: e.IPv6Savings,
	}

	data, _ := json.MarshalIndent(je, "", "  ")
	return string(data)
}

// Helper functions for box drawing

func boxTop(width int) string {
	return fmt.Sprintf("┌%s┐\n", strings.Repeat("─", width-2))
}

func boxBottom(width int) string {
	return fmt.Sprintf("└%s┘\n", strings.Repeat("─", width-2))
}

func boxSep(width int) string {
	return fmt.Sprintf("├%s┤\n", strings.Repeat("─", width-2))
}

func boxDash(width int) string {
	return fmt.Sprintf("│ %s │\n", strings.Repeat("─", width-4))
}

func boxLine(text string, width int) string {
	padding := width - 4 - len(text)
	if padding < 0 {
		padding = 0
		text = text[:width-4]
	}
	return fmt.Sprintf("│ %s%s │\n", text, strings.Repeat(" ", padding))
}

func boxEmpty(width int) string {
	return fmt.Sprintf("│%s│\n", strings.Repeat(" ", width-2))
}

func findItem(items []LineItem, description string) *LineItem {
	for i := range items {
		if items[i].Description == description {
			return &items[i]
		}
	}
	return nil
}
