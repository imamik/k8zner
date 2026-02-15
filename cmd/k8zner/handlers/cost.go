package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"

	"github.com/imamik/k8zner/internal/config"
	"github.com/imamik/k8zner/internal/util/labels"
)

const defaultS3StorageGB = 100.0

// Hetzner Object Storage pricing:
// - €4.99/mo baseline includes 1TB storage + 1TB egress
// - €0.00499/GB/mo for storage beyond 1TB
const (
	hetznerS3BaseMonthlyEUR = 4.99
	hetznerS3PerGBExtraEUR  = 0.00499
	hetznerS3IncludedGB     = 1000.0
)

type costLineItem struct {
	Name         string  `json:"name"`
	Count        int     `json:"count"`
	UnitNet      float64 `json:"unit_net"`
	UnitGross    float64 `json:"unit_gross"`
	MonthlyNet   float64 `json:"monthly_net"`
	MonthlyGross float64 `json:"monthly_gross"`
}

type costSummary struct {
	Currency     string         `json:"currency"`
	Current      []costLineItem `json:"current"`
	Planned      []costLineItem `json:"planned"`
	CurrentTotal costLineItem   `json:"current_total"`
	PlannedTotal costLineItem   `json:"planned_total"`
	DiffTotal    costLineItem   `json:"diff_total"`
}

type desiredResource struct {
	count    int
	typeName string
	location string
	kind     string
	backup   bool
}

// Cost shows detailed current vs planned cluster costs.
func Cost(ctx context.Context, configPath string, jsonOutput bool, s3StorageGB float64) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	summary, err := buildCostSummary(ctx, cfg, s3StorageGB)
	if err != nil {
		return err
	}

	if jsonOutput {
		b, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to encode JSON: %w", err)
		}
		fmt.Println(string(b))
		return nil
	}

	fmt.Print(renderCostSummary(cfg.ClusterName, summary))
	return nil
}

func printOverallCostHint(ctx context.Context, cfg *config.Config, source string) {
	token := strings.TrimSpace(os.Getenv("HCLOUD_TOKEN"))
	if token == "" {
		return
	}
	summary, err := buildCostSummary(ctx, cfg, defaultS3StorageGB)
	if err != nil {
		fmt.Printf("\nEstimated monthly cost (%s): unavailable (%v)\n", source, err)
		return
	}
	fmt.Print(renderCostHint(source, summary))
}

func buildCostSummary(ctx context.Context, cfg *config.Config, s3StorageGB float64) (*costSummary, error) {
	token := strings.TrimSpace(os.Getenv("HCLOUD_TOKEN"))
	if token == "" {
		return nil, fmt.Errorf("HCLOUD_TOKEN environment variable is required")
	}

	hc := hcloud.NewClient(hcloud.WithToken(token))
	pricing, _, err := hc.Pricing.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch hcloud pricing: %w", err)
	}

	current, err := currentCostItems(ctx, hc, cfg.ClusterName, pricing, s3StorageGB)
	if err != nil {
		return nil, err
	}
	planned, err := plannedCostItems(cfg, pricing, s3StorageGB)
	if err != nil {
		return nil, err
	}

	summary := &costSummary{
		Currency: pricing.Currency,
		Current:  current,
		Planned:  planned,
	}
	summary.CurrentTotal = sumCost("current_total", current)
	summary.PlannedTotal = sumCost("planned_total", planned)
	summary.DiffTotal = costLineItem{
		Name:         "diff_total",
		MonthlyNet:   summary.PlannedTotal.MonthlyNet - summary.CurrentTotal.MonthlyNet,
		MonthlyGross: summary.PlannedTotal.MonthlyGross - summary.CurrentTotal.MonthlyGross,
	}
	return summary, nil
}

func currentCostItems(ctx context.Context, hc *hcloud.Client, clusterName string, pricing hcloud.Pricing, s3StorageGB float64) ([]costLineItem, error) {
	servers, err := listClusterServers(ctx, hc, clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to list servers: %w", err)
	}
	lbs, err := listClusterLoadBalancers(ctx, hc, clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to list load balancers: %w", err)
	}

	items := make([]costLineItem, 0)
	backupRate := parseFloat(pricing.ServerBackup.Percentage) / 100

	for _, srv := range servers {
		if srv.ServerType == nil || srv.Location == nil {
			continue
		}
		name := fmt.Sprintf("server:%s", srv.ServerType.Name)
		net, gross := lookupServerPrice(pricing, srv.ServerType.Name, srv.Location.Name)
		addOrMerge(&items, costLineItem{Name: name, Count: 1, UnitNet: net, UnitGross: gross, MonthlyNet: net, MonthlyGross: gross})

		if srv.BackupWindow != "" && backupRate > 0 {
			bNet, bGross := net*backupRate, gross*backupRate
			addOrMerge(&items, costLineItem{Name: "server-backups", Count: 1, UnitNet: bNet, UnitGross: bGross, MonthlyNet: bNet, MonthlyGross: bGross})
		}
	}

	for _, lb := range lbs {
		if lb.LoadBalancerType == nil || lb.Location == nil {
			continue
		}
		name := fmt.Sprintf("load-balancer:%s", lb.LoadBalancerType.Name)
		net, gross := lookupLoadBalancerPrice(pricing, lb.LoadBalancerType.Name, lb.Location.Name)
		addOrMerge(&items, costLineItem{Name: name, Count: 1, UnitNet: net, UnitGross: gross, MonthlyNet: net, MonthlyGross: gross})
	}

	if s3StorageGB > 0 {
		s3Cost := s3MonthlyCost(s3StorageGB)
		addOrMerge(&items, costLineItem{
			Name:         "object-storage",
			Count:        1,
			UnitNet:      s3Cost,
			UnitGross:    s3Cost,
			MonthlyNet:   s3Cost,
			MonthlyGross: s3Cost,
		})
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func plannedCostItems(cfg *config.Config, pricing hcloud.Pricing, s3StorageGB float64) ([]costLineItem, error) {
	items := make([]costLineItem, 0)
	backupRate := parseFloat(pricing.ServerBackup.Percentage) / 100

	resources := desiredResourcesFromConfig(cfg)
	for _, r := range resources {
		n := float64(r.count)
		switch r.kind {
		case "server":
			net, gross := lookupServerPrice(pricing, r.typeName, r.location)
			if net == 0 && gross == 0 {
				return nil, fmt.Errorf("missing pricing for server type %s in %s", r.typeName, r.location)
			}
			addOrMerge(&items, costLineItem{Name: fmt.Sprintf("server:%s", r.typeName), Count: r.count, UnitNet: net, UnitGross: gross, MonthlyNet: net * n, MonthlyGross: gross * n})
			if r.backup && backupRate > 0 {
				bNet, bGross := net*backupRate, gross*backupRate
				addOrMerge(&items, costLineItem{Name: "server-backups", Count: r.count, UnitNet: bNet, UnitGross: bGross, MonthlyNet: bNet * n, MonthlyGross: bGross * n})
			}
		case "lb":
			net, gross := lookupLoadBalancerPrice(pricing, r.typeName, r.location)
			if net == 0 && gross == 0 {
				return nil, fmt.Errorf("missing pricing for load balancer type %s in %s", r.typeName, r.location)
			}
			addOrMerge(&items, costLineItem{Name: fmt.Sprintf("load-balancer:%s", r.typeName), Count: r.count, UnitNet: net, UnitGross: gross, MonthlyNet: net * n, MonthlyGross: gross * n})
		}
	}

	if s3StorageGB > 0 {
		s3Cost := s3MonthlyCost(s3StorageGB)
		addOrMerge(&items, costLineItem{
			Name:         "object-storage",
			Count:        1,
			UnitNet:      s3Cost,
			UnitGross:    s3Cost,
			MonthlyNet:   s3Cost,
			MonthlyGross: s3Cost,
		})
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func desiredResourcesFromConfig(cfg *config.Config) []desiredResource {
	resources := make([]desiredResource, 0)

	// Servers: control plane pools
	for _, cp := range cfg.ControlPlane.NodePools {
		if cp.Count <= 0 {
			continue
		}
		loc := cp.Location
		if loc == "" {
			loc = cfg.Location
		}
		resources = append(resources, desiredResource{kind: "server", typeName: cp.ServerType, count: cp.Count, location: loc, backup: cp.Backups})
	}

	// Servers: worker pools
	for _, w := range cfg.Workers {
		if w.Count <= 0 {
			continue
		}
		loc := w.Location
		if loc == "" {
			loc = cfg.Location
		}
		resources = append(resources, desiredResource{kind: "server", typeName: w.ServerType, count: w.Count, location: loc, backup: w.Backups})
	}

	// Load Balancers: Kube API (always created by CLI during bootstrap)
	resources = append(resources, desiredResource{kind: "lb", typeName: config.LoadBalancerType, count: 1, location: cfg.Location})

	// Load Balancers: Ingress (Traefik creates an LB via CCM when enabled)
	if cfg.Addons.Traefik.Enabled {
		lbType := config.LoadBalancerType // default lb11
		if cfg.Ingress.LoadBalancerType != "" {
			lbType = cfg.Ingress.LoadBalancerType
		}
		resources = append(resources, desiredResource{kind: "lb", typeName: lbType, count: 1, location: cfg.Location})
	}

	// Load Balancers: additional ingress pools (manually configured)
	for _, pool := range cfg.IngressLoadBalancerPools {
		cnt := pool.Count
		if cnt <= 0 {
			cnt = 1
		}
		loc := pool.Location
		if loc == "" {
			loc = cfg.Location
		}
		resources = append(resources, desiredResource{kind: "lb", typeName: pool.Type, count: cnt, location: loc})
	}

	return resources
}

func lookupServerPrice(pricing hcloud.Pricing, serverType, location string) (float64, float64) {
	for _, st := range pricing.ServerTypes {
		if st.ServerType == nil || st.ServerType.Name != serverType {
			continue
		}
		for _, p := range st.Pricings {
			if p.Location != nil && p.Location.Name == location {
				return parseFloat(p.Monthly.Net), parseFloat(p.Monthly.Gross)
			}
		}
	}
	return 0, 0
}

func lookupLoadBalancerPrice(pricing hcloud.Pricing, lbType, location string) (float64, float64) {
	for _, lb := range pricing.LoadBalancerTypes {
		if lb.LoadBalancerType == nil || lb.LoadBalancerType.Name != lbType {
			continue
		}
		for _, p := range lb.Pricings {
			if p.Location != nil && p.Location.Name == location {
				return parseFloat(p.Monthly.Net), parseFloat(p.Monthly.Gross)
			}
		}
	}
	return 0, 0
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func addOrMerge(items *[]costLineItem, next costLineItem) {
	for i := range *items {
		if (*items)[i].Name == next.Name {
			(*items)[i].Count += next.Count
			(*items)[i].MonthlyNet += next.MonthlyNet
			(*items)[i].MonthlyGross += next.MonthlyGross
			return
		}
	}
	*items = append(*items, next)
}

func sumCost(name string, items []costLineItem) costLineItem {
	total := costLineItem{Name: name}
	for _, item := range items {
		total.Count += item.Count
		total.MonthlyNet += item.MonthlyNet
		total.MonthlyGross += item.MonthlyGross
	}
	return total
}

// s3MonthlyCost calculates Hetzner Object Storage monthly cost.
// Baseline is €4.99/mo which includes 1TB storage + 1TB egress.
// Additional storage beyond 1TB costs €0.00499/GB/mo.
func s3MonthlyCost(storageGB float64) float64 {
	extra := storageGB - hetznerS3IncludedGB
	if extra <= 0 {
		return hetznerS3BaseMonthlyEUR
	}
	return hetznerS3BaseMonthlyEUR + extra*hetznerS3PerGBExtraEUR
}

func listClusterServers(ctx context.Context, hc *hcloud.Client, clusterName string) ([]*hcloud.Server, error) {
	selectors := []string{labels.SelectorForCluster(clusterName), "cluster=" + clusterName}
	unique := map[int64]*hcloud.Server{}
	for _, sel := range selectors {
		servers, err := hc.Server.AllWithOpts(ctx, hcloud.ServerListOpts{ListOpts: hcloud.ListOpts{LabelSelector: sel}})
		if err != nil {
			continue
		}
		for _, s := range servers {
			unique[s.ID] = s
		}
	}
	out := make([]*hcloud.Server, 0, len(unique))
	for _, s := range unique {
		out = append(out, s)
	}
	return out, nil
}

func listClusterLoadBalancers(ctx context.Context, hc *hcloud.Client, clusterName string) ([]*hcloud.LoadBalancer, error) {
	selectors := []string{labels.SelectorForCluster(clusterName), "cluster=" + clusterName}
	unique := map[int64]*hcloud.LoadBalancer{}
	for _, sel := range selectors {
		lbs, err := hc.LoadBalancer.AllWithOpts(ctx, hcloud.LoadBalancerListOpts{ListOpts: hcloud.ListOpts{LabelSelector: sel}})
		if err != nil {
			continue
		}
		for _, lb := range lbs {
			unique[lb.ID] = lb
		}
	}
	out := make([]*hcloud.LoadBalancer, 0, len(unique))
	for _, lb := range unique {
		out = append(out, lb)
	}
	return out, nil
}
