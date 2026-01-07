package cluster

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/config"
)

// reconcileNodePool provisions a pool of servers in parallel.
func (r *Reconciler) reconcileNodePool(
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
	names := NewNames(r.config.ClusterName)

	for j := 1; j <= count; j++ {
		// Name: <cluster>-<pool>-<index> (e.g. cluster-control-plane-1)
		serverName := names.Server(poolName, j)

		// Calculate global index for subnet calculations
		// For CP: 10 * np_index + cp_index + 1
		// For Worker: wkr_index + 1
		var hostNum int
		var subnet string
		var err error

		if role == "control-plane" {
			// Terraform: ipv4_private = cidrhost(subnet, np_index * 10 + cp_index + 1)
			subnet, err = r.config.GetSubnetForRole("control-plane", 0)
			if err != nil {
				return nil, err
			}
			hostNum = poolIndex*10 + (j - 1) + 1
		} else {
			// Terraform: ipv4_private = cidrhost(subnet, wkr_index + 1)
			// Note: Terraform iterates worker nodepools and uses separate subnets for each
			// hcloud_network_subnet.worker[np.name]
			// The config.GetSubnetForRole("worker", i) handles the subnet iteration.
			subnet, err = r.config.GetSubnetForRole("worker", poolIndex)
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
			for _, p := range r.config.Workers {
				if p.Name == poolName {
					usePG = p.PlacementGroup
					break
				}
			}

			if usePG {
				pgIndex := int((j-1)/10) + 1
				pgLabels := NewLabelBuilder(r.config.ClusterName).
					WithRole("worker").
					WithPool(poolName).
					WithNodePool(poolName).
					Build()
				pg, err := r.pgManager.EnsurePlacementGroup(ctx, names.WorkerPlacementGroupShard(poolName, pgIndex), "spread", pgLabels)
				if err != nil {
					return nil, err
				}
				currentPGID = &pg.ID
			}
		} else {
			currentPGID = pgID
		}

		configs[j-1] = serverConfig{
			name:      serverName,
			privateIP: privateIP,
			pgID:      currentPGID,
		}
	}

	// Create all servers in parallel
	log.Printf("=== CREATING %d SERVERS IN PARALLEL for pool %s at %s ===", count, poolName, time.Now().Format("15:04:05"))
	type serverResult struct {
		name string
		ip   string
		err  error
	}

	resultChan := make(chan serverResult, count)

	for _, cfg := range configs {
		cfg := cfg // capture loop variable
		go func() {
			log.Printf("[server:%s] Starting at %s", cfg.name, time.Now().Format("15:04:05"))
			var fws []*hcloud.Firewall
			if r.firewall != nil {
				log.Printf("[node_pool] Attaching firewall %s to server %s", r.firewall.Name, cfg.name)
				fws = []*hcloud.Firewall{r.firewall}
			} else {
				log.Printf("[node_pool] WARNING: r.firewall is nil, no firewall attached to server %s", cfg.name)
			}
			ip, err := r.ensureServer(ctx, cfg.name, serverType, location, image, role, poolName, extraLabels, userData, cfg.pgID, cfg.privateIP, fws)
			log.Printf("[server:%s] Completed at %s", cfg.name, time.Now().Format("15:04:05"))
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

	log.Printf("=== SUCCESSFULLY CREATED %d SERVERS for pool %s at %s ===", count, poolName, time.Now().Format("15:04:05"))
	return ips, nil
}
