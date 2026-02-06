// Package pricing provides cost calculation for k8zner clusters.
package pricing

import (
	"fmt"

	v2 "github.com/imamik/k8zner/internal/config/v2"
)

// VATRate is the German VAT rate (19%).
const VATRate = 0.19

// Calculator calculates cluster costs based on Hetzner pricing.
type Calculator struct {
	prices *Prices
}

// Prices contains Hetzner pricing data.
type Prices struct {
	// Servers maps server type to monthly price in EUR.
	Servers map[string]float64

	// LoadBalancers maps LB type to monthly price in EUR.
	LoadBalancers map[string]float64

	// PrimaryIPv4 is the monthly cost for a primary IPv4 address.
	PrimaryIPv4 float64
}

// Estimate contains the calculated cost estimate.
type Estimate struct {
	// Items is the list of line items.
	Items []LineItem

	// Subtotal is the sum of all items (before VAT).
	Subtotal float64

	// VAT is the VAT amount (19% for Germany).
	VAT float64

	// Total is the total including VAT.
	Total float64

	// IPv6Savings is the amount saved by using IPv6-only nodes.
	IPv6Savings float64

	// Config metadata
	ClusterName string
	Mode        v2.Mode
	Region      v2.Region
}

// LineItem represents a single cost line item.
type LineItem struct {
	Description string
	Quantity    int
	UnitType    string
	UnitPrice   float64
	Total       float64
}

// String returns a formatted string representation of the line item.
func (l LineItem) String() string {
	return fmt.Sprintf("%s: %d× %s @ €%.2f = €%.2f/mo",
		l.Description, l.Quantity, l.UnitType, l.UnitPrice, l.Total)
}

// AnnualCost returns the estimated annual cost.
func (e *Estimate) AnnualCost() float64 {
	return e.Total * 12
}

// NewCalculator creates a new calculator with default pricing.
// Note: In production, use NewCalculatorWithPrices with live Hetzner pricing.
func NewCalculator() *Calculator {
	return &Calculator{
		prices: DefaultPrices(),
	}
}

// NewCalculatorWithPrices creates a new calculator with specific pricing.
func NewCalculatorWithPrices(prices *Prices) *Calculator {
	return &Calculator{
		prices: prices,
	}
}

// Calculate calculates the cost estimate for a cluster configuration.
func (c *Calculator) Calculate(cfg *v2.Config) *Estimate {
	estimate := &Estimate{
		ClusterName: cfg.Name,
		Mode:        cfg.Mode,
		Region:      cfg.Region,
		Items:       make([]LineItem, 0, 3),
	}

	// Control planes
	cpCount := cfg.ControlPlaneCount()
	cpType := string(cfg.ControlPlaneSize())
	cpPrice := c.prices.Servers[cpType]
	cpTotal := float64(cpCount) * cpPrice

	estimate.Items = append(estimate.Items, LineItem{
		Description: "Control Planes",
		Quantity:    cpCount,
		UnitType:    cpType,
		UnitPrice:   cpPrice,
		Total:       cpTotal,
	})

	// Workers
	workerCount := cfg.Workers.Count
	workerType := string(cfg.Workers.Size.Normalize())
	workerPrice := c.prices.Servers[workerType]
	workerTotal := float64(workerCount) * workerPrice

	estimate.Items = append(estimate.Items, LineItem{
		Description: "Workers",
		Quantity:    workerCount,
		UnitType:    workerType,
		UnitPrice:   workerPrice,
		Total:       workerTotal,
	})

	// Load balancers
	lbCount := cfg.LoadBalancerCount()
	lbType := v2.LoadBalancerType
	lbPrice := c.prices.LoadBalancers[lbType]
	lbTotal := float64(lbCount) * lbPrice

	estimate.Items = append(estimate.Items, LineItem{
		Description: "Load Balancers",
		Quantity:    lbCount,
		UnitType:    lbType,
		UnitPrice:   lbPrice,
		Total:       lbTotal,
	})

	// Calculate totals
	estimate.Subtotal = cpTotal + workerTotal + lbTotal
	estimate.VAT = estimate.Subtotal * VATRate
	estimate.Total = estimate.Subtotal + estimate.VAT

	// Calculate IPv6 savings (what we save by not having IPv4 on nodes)
	totalNodes := cpCount + workerCount
	estimate.IPv6Savings = float64(totalNodes) * c.prices.PrimaryIPv4

	return estimate
}

// DefaultPrices returns default Hetzner pricing (as of 2024).
// These are approximate and should be updated from the Hetzner API.
// DefaultPrices returns hardcoded Hetzner pricing (as of January 2025).
// These are net prices in EUR before VAT.
//
// TODO: Fetch pricing dynamically from Hetzner API to avoid drift.
// The HCloud API provides pricing via GET /pricing endpoint.
// See: https://docs.hetzner.cloud/#pricing-get-all-prices
func DefaultPrices() *Prices {
	return &Prices{
		Servers: map[string]float64{
			// CPX series - Shared vCPU (better availability)
			"cpx22": 4.49,
			"cpx32": 8.49,
			"cpx42": 15.49,
			"cpx52": 29.49,
			// CX series - Old names (Hetzner renamed types in 2024)
			"cx22": 4.35,
			"cx32": 8.09,
			"cx42": 15.59,
			"cx52": 29.59,
			// CX series - New names (dedicated vCPU)
			"cx23": 4.35,
			"cx33": 8.09,
			"cx43": 15.59,
			"cx53": 29.59,
		},
		LoadBalancers: map[string]float64{
			"lb11": 6.41,
			"lb21": 13.81,
			"lb31": 22.21,
		},
		PrimaryIPv4: 0.50,
	}
}
