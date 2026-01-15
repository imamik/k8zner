package compute

import (
	"context"
	"fmt"
	"sync"

	"hcloud-k8s/internal/config"
	"hcloud-k8s/internal/provisioning"
	"hcloud-k8s/internal/util/async"
	"hcloud-k8s/internal/util/labels"
	"hcloud-k8s/internal/util/naming"
)

// NodePoolSpec defines the configuration for provisioning a pool of servers.
// All fields are self-documenting - no need to remember parameter order.
type NodePoolSpec struct {
	Name             string
	Count            int
	ServerType       string
	Location         string
	Image            string
	Role             string // "control-plane" or "worker"
	ExtraLabels      map[string]string
	UserData         string
	PlacementGroupID *int64
	PoolIndex        int
	RDNSIPv4         string // RDNS template for IPv4
	RDNSIPv6         string // RDNS template for IPv6
}

// reconcileNodePool provisions a pool of servers in parallel.
func (p *Provisioner) reconcileNodePool(ctx *provisioning.Context, spec NodePoolSpec) (map[string]string, error) {
	// Pre-calculate all server configurations
	type serverConfig struct {
		name      string
		privateIP string
		pgID      *int64
	}

	configs := make([]serverConfig, spec.Count)

	for j := 1; j <= spec.Count; j++ {
		// Name: <cluster>-<pool>-<index> (e.g. cluster-control-plane-1)
		srvName := naming.Server(ctx.Config.ClusterName, spec.Name, j)

		// Calculate global index for subnet calculations
		// For CP: 10 * np_index + cp_index + 1
		// For Worker: wkr_index + 1
		var hostNum int
		var subnet string
		var err error

		if spec.Role == "control-plane" {
			// Terraform: ipv4_private = cidrhost(subnet, np_index * 10 + cp_index + 1)
			subnet, err = ctx.Config.GetSubnetForRole("control-plane", 0)
			if err != nil {
				return nil, err
			}
			hostNum = spec.PoolIndex*10 + (j - 1) + 1
		} else {
			// Terraform: ipv4_private = cidrhost(subnet, wkr_index + 1)
			// Note: Terraform iterates worker nodepools and uses separate subnets for each
			// hcloud_network_subnet.worker[np.name]
			// The config.GetSubnetForRole("worker", i) handles the subnet iteration.
			subnet, err = ctx.Config.GetSubnetForRole("worker", spec.PoolIndex)
			if err != nil {
				return nil, err
			}
			hostNum = (j - 1) + 1
		}

		privateIP, err := config.CIDRHost(subnet, hostNum)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate private ip: %w", err)
		}

		// Placement Group Sharding for Workers
		// Terraform: ${cluster}-${pool}-pg-${ceil((index+1)/10)}
		var currentPGID *int64
		if spec.Role == "worker" && spec.PlacementGroupID == nil { // Workers manage their own PGs if enabled
			// Check if enabled in config for this pool
			usePG := false
			// Find the pool config again (slightly inefficient but safe)
			for _, pool := range ctx.Config.Workers {
				if pool.Name == spec.Name {
					usePG = pool.PlacementGroup
					break
				}
			}

			if usePG {
				pgIndex := int((j-1)/10) + 1
				lb := labels.NewLabelBuilder(ctx.Config.ClusterName).
					WithRole("worker").
					WithPool(spec.Name).
					WithNodePool(spec.Name)
				if ctx.Config.TestID != "" {
					lb = lb.WithTestID(ctx.Config.TestID)
				}
				pgLabels := lb.Build()
				pg, err := ctx.Infra.EnsurePlacementGroup(ctx, naming.WorkerPlacementGroupShard(ctx.Config.ClusterName, spec.Name, pgIndex), "spread", pgLabels)
				if err != nil {
					return nil, err
				}
				currentPGID = &pg.ID
			}
		} else {
			currentPGID = spec.PlacementGroupID
		}

		configs[j-1] = serverConfig{
			name:      srvName,
			privateIP: privateIP,
			pgID:      currentPGID,
		}
	}

	// Create all servers in parallel
	ctx.Logger.Printf("[%s] Creating %d servers for pool %s...", phase, spec.Count, spec.Name)

	// Collect IPs in a thread-safe way
	var mu sync.Mutex
	ips := make(map[string]string)

	tasks := make([]async.Task, len(configs))
	for i, cfg := range configs {
		cfg := cfg // capture loop variable
		tasks[i] = async.Task{
			Name: fmt.Sprintf("server-%s", cfg.name),
			Func: func(_ context.Context) error {
				ip, err := p.ensureServer(ctx, ServerSpec{
					Name:           cfg.name,
					Type:           spec.ServerType,
					Location:       spec.Location,
					Image:          spec.Image,
					Role:           spec.Role,
					Pool:           spec.Name,
					ExtraLabels:    spec.ExtraLabels,
					UserData:       spec.UserData,
					PlacementGroup: cfg.pgID,
					PrivateIP:      cfg.privateIP,
					RDNSIPv4:       spec.RDNSIPv4,
					RDNSIPv6:       spec.RDNSIPv6,
				})
				if err != nil {
					return err
				}
				mu.Lock()
				ips[cfg.name] = ip
				mu.Unlock()
				return nil
			},
		}
	}

	if err := async.RunParallel(ctx, tasks, false); err != nil {
		return nil, fmt.Errorf("failed to provision pool %s: %w", spec.Name, err)
	}

	ctx.Logger.Printf("[%s] Successfully created %d servers for pool %s", phase, spec.Count, spec.Name)
	return ips, nil
}
