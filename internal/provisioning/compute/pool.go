package compute

import (
	"hcloud-k8s/internal/util/labels"
	"context"
	"fmt"
	"log"

	"hcloud-k8s/internal/config"
	"hcloud-k8s/internal/util/naming"
)

// reconcileNodePool provisions a pool of servers in parallel.
func (p *Provisioner) reconcileNodePool(
	ctx context.Context,
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
		srvName := naming.Server(p.config.ClusterName, poolName, j)

		// Calculate global index for subnet calculations
		// For CP: 10 * np_index + cp_index + 1
		// For Worker: wkr_index + 1
		var hostNum int
		var subnet string
		var err error

		if role == "control-plane" {
			// Terraform: ipv4_private = cidrhost(subnet, np_index * 10 + cp_index + 1)
			subnet, err = p.config.GetSubnetForRole("control-plane", 0)
			if err != nil {
				return nil, err
			}
			hostNum = poolIndex*10 + (j - 1) + 1
		} else {
			// Terraform: ipv4_private = cidrhost(subnet, wkr_index + 1)
			// Note: Terraform iterates worker nodepools and uses separate subnets for each
			// hcloud_network_subnet.worker[np.name]
			// The config.GetSubnetForRole("worker", i) handles the subnet iteration.
			subnet, err = p.config.GetSubnetForRole("worker", poolIndex)
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
			for _, p := range p.config.Workers {
				if p.Name == poolName {
					usePG = p.PlacementGroup
					break
				}
			}

			if usePG {
				pgIndex := int((j-1)/10) + 1
				pgLabels := labels.NewLabelBuilder(p.config.ClusterName).
					WithRole("worker").
					WithPool(poolName).
					WithNodePool(poolName).
					Build()
				pg, err := p.pgManager.EnsurePlacementGroup(ctx, naming.WorkerPlacementGroupShard(p.config.ClusterName, poolName, pgIndex), "spread", pgLabels)
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
	log.Printf("Creating %d servers for pool %s...", count, poolName)
	type serverResult struct {
		name string
		ip   string
		err  error
	}

	resultChan := make(chan serverResult, count)

	for _, cfg := range configs {
		cfg := cfg // capture loop variable
		go func() {
			ip, err := p.ensureServer(ctx, cfg.name, serverType, location, image, role, poolName, extraLabels, userData, cfg.pgID, cfg.privateIP)
			resultChan <- serverResult{name: cfg.name, ip: ip, err: err}
		}()
	}

	// Collect results
	ips := make(map[string]string)
	for i := 0; i < count; i++ {
		result := <-resultChan
		if result.err != nil {
			return nil, fmt.Errorf("failed to create server %s: %w", result.name, result.err)
		}
		ips[result.name] = result.ip
	}

	log.Printf("Successfully created %d servers for pool %s", count, poolName)
	return ips, nil
}
