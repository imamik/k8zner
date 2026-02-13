package handlers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatDelta(t *testing.T) {
	tests := []struct {
		name     string
		delta    float64
		wantUp   bool // contains ▲
		wantDown bool // contains ▼
		wantDash bool // contains ─
	}{
		{"positive delta", 18.20, true, false, false},
		{"negative delta", -5.50, false, true, false},
		{"zero delta", 0.0, false, false, true},
		{"tiny positive (below threshold)", 0.001, false, false, true},
		{"tiny negative (below threshold)", -0.001, false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDelta(tt.delta, "EUR")
			if tt.wantUp {
				assert.Contains(t, result, "▲")
			}
			if tt.wantDown {
				assert.Contains(t, result, "▼")
			}
			if tt.wantDash {
				assert.Contains(t, result, "─")
			}
		})
	}
}

func TestRenderCostSummary(t *testing.T) {
	t.Run("contains expected sections", func(t *testing.T) {
		summary := &costSummary{
			Currency: "EUR",
			Current: []costLineItem{
				{Name: "server:cx41", Count: 2, UnitNet: 15.17, MonthlyNet: 30.34, MonthlyGross: 36.10},
			},
			Planned: []costLineItem{
				{Name: "server:cx41", Count: 3, UnitNet: 15.17, MonthlyNet: 45.51, MonthlyGross: 54.16},
			},
			CurrentTotal: costLineItem{MonthlyNet: 30.34, MonthlyGross: 36.10},
			PlannedTotal: costLineItem{MonthlyNet: 45.51, MonthlyGross: 54.16},
			DiffTotal:    costLineItem{MonthlyNet: 15.17, MonthlyGross: 18.06},
		}

		output := renderCostSummary("my-cluster", summary)
		assert.Contains(t, output, "k8zner cost: my-cluster")
		assert.Contains(t, output, "Current Resources")
		assert.Contains(t, output, "Planned Resources")
		assert.Contains(t, output, "Summary")
		assert.Contains(t, output, "server:cx41")
		assert.Contains(t, output, "▲") // positive delta
	})

	t.Run("empty current items", func(t *testing.T) {
		summary := &costSummary{
			Currency: "EUR",
			Current:  []costLineItem{},
			Planned: []costLineItem{
				{Name: "server:cx41", Count: 1, UnitNet: 15.17, MonthlyNet: 15.17, MonthlyGross: 18.05},
			},
			CurrentTotal: costLineItem{MonthlyNet: 0},
			PlannedTotal: costLineItem{MonthlyNet: 15.17},
			DiffTotal:    costLineItem{MonthlyNet: 15.17},
		}

		output := renderCostSummary("new-cluster", summary)
		assert.NotContains(t, output, "Current Resources")
		assert.Contains(t, output, "Planned Resources")
	})

	t.Run("negative delta shows green arrow", func(t *testing.T) {
		summary := &costSummary{
			Currency: "EUR",
			Current: []costLineItem{
				{Name: "server:cx41", Count: 3, UnitNet: 15.17, MonthlyNet: 45.51},
			},
			Planned: []costLineItem{
				{Name: "server:cx41", Count: 2, UnitNet: 15.17, MonthlyNet: 30.34},
			},
			CurrentTotal: costLineItem{MonthlyNet: 45.51},
			PlannedTotal: costLineItem{MonthlyNet: 30.34},
			DiffTotal:    costLineItem{MonthlyNet: -15.17},
		}

		output := renderCostSummary("shrink-cluster", summary)
		assert.Contains(t, output, "▼")
	})
}

func TestRenderCostHint(t *testing.T) {
	t.Run("positive delta", func(t *testing.T) {
		summary := &costSummary{
			Currency:     "EUR",
			CurrentTotal: costLineItem{MonthlyNet: 30.0},
			PlannedTotal: costLineItem{MonthlyNet: 50.0},
			DiffTotal:    costLineItem{MonthlyNet: 20.0},
		}
		result := renderCostHint("init", summary)
		assert.True(t, strings.Contains(result, "Estimated monthly cost"))
		assert.True(t, strings.Contains(result, "delta"))
	})

	t.Run("no delta when zero", func(t *testing.T) {
		summary := &costSummary{
			Currency:     "EUR",
			CurrentTotal: costLineItem{MonthlyNet: 30.0},
			PlannedTotal: costLineItem{MonthlyNet: 30.0},
			DiffTotal:    costLineItem{MonthlyNet: 0.0},
		}
		result := renderCostHint("apply", summary)
		assert.True(t, strings.Contains(result, "Estimated monthly cost"))
		// delta text should not appear when zero
		assert.False(t, strings.Contains(result, "delta"))
	})
}
