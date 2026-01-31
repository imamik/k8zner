package pricing

import (
	"strings"
	"testing"

	v2 "github.com/imamik/k8zner/internal/config/v2"
)

func TestFormatter_Format(t *testing.T) {
	estimate := &Estimate{
		ClusterName: "my-cluster",
		Mode:        v2.ModeHA,
		Region:      v2.RegionFalkenstein,
		Items: []LineItem{
			{Description: "Control Planes", Quantity: 3, UnitType: "cx22", UnitPrice: 4.35, Total: 13.05},
			{Description: "Workers", Quantity: 3, UnitType: "cx32", UnitPrice: 8.09, Total: 24.27},
			{Description: "Load Balancers", Quantity: 2, UnitType: "lb11", UnitPrice: 6.41, Total: 12.82},
		},
		Subtotal:    50.14,
		VAT:         9.53,
		Total:       59.67,
		IPv6Savings: 3.00,
	}

	formatter := NewFormatter()
	output := formatter.Format(estimate)

	// Check that key elements are present
	checks := []string{
		"my-cluster",
		"ha",
		"fsn1",
		"Control Planes",
		"Workers",
		"Load Balancers",
		"50.14",
		"59.67",
		"IPv6",
	}

	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("Output missing %q", check)
		}
	}
}

func TestFormatter_FormatCompact(t *testing.T) {
	estimate := &Estimate{
		ClusterName: "dev",
		Mode:        v2.ModeDev,
		Region:      v2.RegionFalkenstein,
		Items: []LineItem{
			{Description: "Control Planes", Quantity: 1, UnitType: "cx22", UnitPrice: 4.35, Total: 4.35},
			{Description: "Workers", Quantity: 1, UnitType: "cx22", UnitPrice: 4.35, Total: 4.35},
			{Description: "Load Balancers", Quantity: 1, UnitType: "lb11", UnitPrice: 6.41, Total: 6.41},
		},
		Subtotal:    15.11,
		VAT:         2.87,
		Total:       17.98,
		IPv6Savings: 1.00,
	}

	formatter := NewFormatter()
	output := formatter.FormatCompact(estimate)

	// Compact format should be shorter
	if len(output) > 200 {
		t.Errorf("FormatCompact output too long: %d chars", len(output))
	}

	// Should contain total
	if !strings.Contains(output, "17.98") {
		t.Error("FormatCompact missing total")
	}
}

func TestFormatter_FormatJSON(t *testing.T) {
	estimate := &Estimate{
		ClusterName: "test",
		Mode:        v2.ModeHA,
		Region:      v2.RegionFalkenstein,
		Items: []LineItem{
			{Description: "Control Planes", Quantity: 3, UnitType: "cx22", UnitPrice: 4.35, Total: 13.05},
		},
		Subtotal:    13.05,
		VAT:         2.48,
		Total:       15.53,
		IPv6Savings: 1.50,
	}

	formatter := NewFormatter()
	output := formatter.FormatJSON(estimate)

	// Should be valid JSON structure
	if !strings.HasPrefix(output, "{") || !strings.HasSuffix(strings.TrimSpace(output), "}") {
		t.Error("FormatJSON output is not valid JSON")
	}

	// Should contain key fields
	if !strings.Contains(output, `"cluster_name"`) {
		t.Error("FormatJSON missing cluster_name field")
	}
	if !strings.Contains(output, `"total"`) {
		t.Error("FormatJSON missing total field")
	}
}
