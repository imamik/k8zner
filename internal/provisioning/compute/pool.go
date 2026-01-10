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

// reconcileNodePool provisions a pool of servers in parallel.
func (p *Provisioner) reconcileNodePool(
	ctx *provisioning.Context,
	poolName string,
	count int,
	serverType string,
	location string,
	image string,
	role string,
	extraLabels map[string]string,
	userData string,
	pgID *int64,
	poolIndex int,
) (map[string]string, error) {
	// Pre-calculate all server configurations
	type serverConfig struct {
		name      string
		privateIP string
		pgID      *int64
	}

	configs := make([]serverConfig, count)

	for j := 1; j <= count; j++ {
		// Name: <cluster>-<pool>-<index> (e.g. cluster-control-plane-1)
		srvName := naming.Server(ctx.Config.ClusterName, poolName, j)

		// Calculate global index for subnet calculations
		// For CP: 10 * np_index + cp_index + 1
		// For Worker: wkr_index + 1
		var hostNum int
		var subnet string
		var err error

		if role == "control-plane" {
			// Terraform: ipv4_private = cidrhost(subnet, np_index * 10 + cp_index + 1)
			subnet, err = ctx.Config.GetSubnetForRole("control-plane", 0)
			if err != nil {
				return nil, err
			}
			hostNum = poolIndex*10 + (j - 1) + 1
		} else {
			// Terraform: ipv4_private = cidrhost(subnet, wkr_index + 1)
			// Note: Terraform iterates worker nodepools and uses separate subnets for each
			// hcloud_network_subnet.worker[np.name]
			// The config.GetSubnetForRole("worker", i) handles the subnet iteration.
			subnet, err = ctx.Config.GetSubnetForRole("worker", poolIndex)
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
		if role == "worker" && pgID == nil { // Workers manage their own PGs if enabled
			// Check if enabled in config for this pool
			usePG := false
			// Find the pool config again (slightly inefficient but safe)
			for _, pool := range ctx.Config.Workers {
				if pool.Name == poolName {
					usePG = pool.PlacementGroup
					break
				}
			}

			if usePG {
				pgIndex := int((j-1)/10) + 1
				pgLabels := labels.NewLabelBuilder(ctx.Config.ClusterName).
					WithRole("worker").
					WithPool(poolName).
					WithNodePool(poolName).
					Build()
				pg, err := ctx.Infra.EnsurePlacementGroup(ctx, naming.WorkerPlacementGroupShard(ctx.Config.ClusterName, poolName, pgIndex), "spread", pgLabels)
				if err != nil {
					return nil, err
				}
				currentPGID = &pg.ID
			}
		} else {
			currentPGID = pgID
		}

		configs[j-1] = serverConfig{
			name:      srvName,
			privateIP: privateIP,
			pgID:      currentPGID,
		}
	}

	// Create all servers in parallel
	ctx.Logger.Printf("[%s] Creating %d servers for pool %s...", phase, count, poolName)

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
					Type:           serverType,
					Location:       location,
					Image:          image,
					Role:           role,
					Pool:           poolName,
					ExtraLabels:    extraLabels,
					UserData:       userData,
					PlacementGroup: cfg.pgID,
					PrivateIP:      cfg.privateIP,
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
		return nil, fmt.Errorf("failed to provision pool %s: %w", poolName, err)
	}

	ctx.Logger.Printf("[%s] Successfully created %d servers for pool %s", phase, count, poolName)
	return ips, nil
}
