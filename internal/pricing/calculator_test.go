package pricing

import (
	"testing"

	v2 "github.com/imamik/k8zner/internal/config/v2"
)

func TestCalculator_Calculate(t *testing.T) {
	// Use static prices for testing
	// Include both old and new server type names for backwards compatibility
	prices := &Prices{
		Servers: map[string]float64{
			"cx22": 4.35,
			"cx23": 4.35,
			"cx32": 8.09,
			"cx33": 8.09,
			"cx42": 15.59,
			"cx43": 15.59,
			"cx52": 29.59,
			"cx53": 29.59,
		},
		LoadBalancers: map[string]float64{
			"lb11": 6.41,
		},
		PrimaryIPv4: 0.50,
	}

	calc := NewCalculatorWithPrices(prices)

	tests := []struct {
		name         string
		config       *v2.Config
		wantSubtotal float64
		wantItems    int
	}{
		{
			name: "minimal dev cluster",
			config: &v2.Config{
				Name:   "dev",
				Region: v2.RegionFalkenstein,
				Mode:   v2.ModeDev,
				Workers: v2.Worker{
					Count: 1,
					Size:  v2.SizeCX22,
				},
			},
			// 1x CP (cx23) + 1x worker (cx22) + 1x LB
			// 4.35 + 4.35 + 6.41 = 15.11
			wantSubtotal: 15.11,
			wantItems:    3,
		},
		{
			name: "standard ha cluster",
			config: &v2.Config{
				Name:   "prod",
				Region: v2.RegionFalkenstein,
				Mode:   v2.ModeHA,
				Workers: v2.Worker{
					Count: 3,
					Size:  v2.SizeCX32,
				},
			},
			// 3x CP (cx23) + 3x worker (cx32) + 2x LB
			// (3 * 4.35) + (3 * 8.09) + (2 * 6.41)
			// 13.05 + 24.27 + 12.82 = 50.14
			wantSubtotal: 50.14,
			wantItems:    3,
		},
		{
			name: "high performance cluster",
			config: &v2.Config{
				Name:   "enterprise",
				Region: v2.RegionFalkenstein,
				Mode:   v2.ModeHA,
				Workers: v2.Worker{
					Count: 5,
					Size:  v2.SizeCX52,
				},
			},
			// 3x CP (cx23) + 5x worker (cx52) + 2x LB
			// (3 * 4.35) + (5 * 29.59) + (2 * 6.41)
			// 13.05 + 147.95 + 12.82 = 173.82
			wantSubtotal: 173.82,
			wantItems:    3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			estimate := calc.Calculate(tt.config)

			if len(estimate.Items) != tt.wantItems {
				t.Errorf("Items count = %d, want %d", len(estimate.Items), tt.wantItems)
			}

			// Allow small floating point differences
			if diff := estimate.Subtotal - tt.wantSubtotal; diff > 0.01 || diff < -0.01 {
				t.Errorf("Subtotal = %.2f, want %.2f", estimate.Subtotal, tt.wantSubtotal)
			}
		})
	}
}

func TestCalculator_CalculateWithVAT(t *testing.T) {
	prices := &Prices{
		Servers: map[string]float64{
			"cx22": 4.35,
			"cx23": 4.35,
			"cx32": 8.09,
			"cx33": 8.09,
		},
		LoadBalancers: map[string]float64{
			"lb11": 6.41,
		},
		PrimaryIPv4: 0.50,
	}

	calc := NewCalculatorWithPrices(prices)

	config := &v2.Config{
		Name:   "dev",
		Region: v2.RegionFalkenstein,
		Mode:   v2.ModeDev,
		Workers: v2.Worker{
			Count: 1,
			Size:  v2.SizeCX22,
		},
	}

	estimate := calc.Calculate(config)

	// Subtotal: 15.11
	// VAT 19%: 2.87 (approximately)
	// Total: 17.98
	expectedVAT := estimate.Subtotal * 0.19
	expectedTotal := estimate.Subtotal + expectedVAT

	if diff := estimate.VAT - expectedVAT; diff > 0.01 || diff < -0.01 {
		t.Errorf("VAT = %.2f, want %.2f", estimate.VAT, expectedVAT)
	}

	if diff := estimate.Total - expectedTotal; diff > 0.01 || diff < -0.01 {
		t.Errorf("Total = %.2f, want %.2f", estimate.Total, expectedTotal)
	}
}

func TestCalculator_IPv6Savings(t *testing.T) {
	prices := &Prices{
		Servers: map[string]float64{
			"cx22": 4.35,
			"cx23": 4.35,
			"cx32": 8.09,
			"cx33": 8.09,
		},
		LoadBalancers: map[string]float64{
			"lb11": 6.41,
		},
		PrimaryIPv4: 0.50,
	}

	calc := NewCalculatorWithPrices(prices)

	config := &v2.Config{
		Name:   "test",
		Region: v2.RegionFalkenstein,
		Mode:   v2.ModeHA, // 3 CPs + 3 workers = 6 nodes
		Workers: v2.Worker{
			Count: 3,
			Size:  v2.SizeCX22,
		},
	}

	estimate := calc.Calculate(config)

	// 6 nodes without IPv4 = 6 * 0.50 = 3.00 savings
	expectedSavings := 6 * prices.PrimaryIPv4

	if diff := estimate.IPv6Savings - expectedSavings; diff > 0.01 || diff < -0.01 {
		t.Errorf("IPv6Savings = %.2f, want %.2f", estimate.IPv6Savings, expectedSavings)
	}
}

func TestEstimate_AnnualCost(t *testing.T) {
	estimate := &Estimate{
		Total: 50.00,
	}

	expected := 600.00
	if got := estimate.AnnualCost(); got != expected {
		t.Errorf("AnnualCost() = %.2f, want %.2f", got, expected)
	}
}

func TestLineItem_String(t *testing.T) {
	item := LineItem{
		Description: "Control Planes",
		Quantity:    3,
		UnitType:    "CX22",
		UnitPrice:   4.35,
		Total:       13.05,
	}

	s := item.String()
	if s == "" {
		t.Error("LineItem.String() returned empty string")
	}
}
