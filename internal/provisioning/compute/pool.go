package compute

import (
	"context"
	"fmt"
	"sync"

	"github.com/imamik/k8zner/internal/config"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/util/async"
	"github.com/imamik/k8zner/internal/util/labels"
	"github.com/imamik/k8zner/internal/util/naming"
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
	EnablePublicIPv4 bool // Enable public IPv4 (set from config.ShouldEnablePublicIPv4())
	EnablePublicIPv6 bool // Enable public IPv6 (set from config.ShouldEnablePublicIPv6())
}

// NodePoolResult holds the results of provisioning a node pool.
type NodePoolResult struct {
	IPs       map[string]string // nodeName -> publicIP
	ServerIDs map[string]int64  // nodeName -> serverID
}

// reconcileNodePool provisions a pool of servers in parallel.
// Returns both server IPs and server IDs for use in machine config generation.
func (p *Provisioner) reconcileNodePool(ctx *provisioning.Context, spec NodePoolSpec) (NodePoolResult, error) {
	// Pre-calculate all server configurations
	type serverConfig struct {
		name      string
		privateIP string
		pgID      *int64
	}

	// Discover existing servers for this pool to support idempotent re-runs.
	// Random names (cp-{5char}, w-{5char}) require label-based discovery.
	existingNames, err := p.getExistingServerNames(ctx, spec.Role, spec.Name)
	if err != nil {
		return NodePoolResult{}, fmt.Errorf("failed to discover existing servers: %w", err)
	}

	configs := make([]serverConfig, spec.Count)

	for j := 1; j <= spec.Count; j++ {
		// Reuse existing server names for idempotency, generate random names for new servers.
		// Format: {cluster}-cp-{5char} or {cluster}-w-{5char}
		var srvName string
		idx := j - 1
		switch {
		case idx < len(existingNames):
			srvName = existingNames[idx]
		case spec.Role == "control-plane":
			srvName = naming.ControlPlane(ctx.Config.ClusterName)
		default:
			srvName = naming.Worker(ctx.Config.ClusterName)
		}

		// Calculate global index for subnet calculations
		// For CP: 10 * np_index + cp_index + 1
		// For Worker: wkr_index + 1
		var hostNum int
		var subnet string
		var err error

		if spec.Role == "control-plane" {
			// IP allocation: host = pool_index * 10 + node_index + 2
			// Hetzner Cloud reserves .1 for the gateway, so we start at .2
			subnet, err = ctx.Config.GetSubnetForRole("control-plane", 0)
			if err != nil {
				return NodePoolResult{}, err
			}
			hostNum = spec.PoolIndex*10 + (j - 1) + 2
		} else {
			// IP allocation: host = node_index + 2 (each worker pool has its own subnet)
			// Hetzner Cloud reserves .1 for the gateway, so we start at .2
			subnet, err = ctx.Config.GetSubnetForRole("worker", spec.PoolIndex)
			if err != nil {
				return NodePoolResult{}, err
			}
			hostNum = (j - 1) + 2
		}

		privateIP, err := config.CIDRHost(subnet, hostNum)
		if err != nil {
			return NodePoolResult{}, fmt.Errorf("failed to calculate private ip: %w", err)
		}

		// Placement Group Sharding for Workers
		// Name pattern: {cluster}-{pool}-pg-{ceil((index+1)/10)}
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
				pgIndex := (j-1)/10 + 1
				pgLabels := labels.NewLabelBuilder(ctx.Config.ClusterName).
					WithRole("worker").
					WithPool(spec.Name).
					WithNodePool(spec.Name).
					WithTestIDIfSet(ctx.Config.TestID).
					Build()
				pg, err := ctx.Infra.EnsurePlacementGroup(ctx, naming.WorkerPlacementGroupShard(ctx.Config.ClusterName, spec.Name, pgIndex), "spread", pgLabels)
				if err != nil {
					return NodePoolResult{}, err
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
	ctx.Observer.Printf("[%s] Creating %d servers for pool %s...", phase, spec.Count, spec.Name)

	// Collect IPs and server IDs in a thread-safe way
	var mu sync.Mutex
	result := NodePoolResult{
		IPs:       make(map[string]string),
		ServerIDs: make(map[string]int64),
	}

	tasks := make([]async.Task, len(configs))
	for i, cfg := range configs {
		cfg := cfg // capture loop variable
		tasks[i] = async.Task{
			Name: fmt.Sprintf("server-%s", cfg.name),
			Func: func(_ context.Context) error {
				info, err := p.ensureServer(ctx, ServerSpec{
					Name:             cfg.name,
					Type:             spec.ServerType,
					Location:         spec.Location,
					Image:            spec.Image,
					Role:             spec.Role,
					Pool:             spec.Name,
					ExtraLabels:      spec.ExtraLabels,
					UserData:         spec.UserData,
					PlacementGroup:   cfg.pgID,
					PrivateIP:        cfg.privateIP,
					EnablePublicIPv4: spec.EnablePublicIPv4,
					EnablePublicIPv6: spec.EnablePublicIPv6,
				})
				if err != nil {
					return err
				}
				mu.Lock()
				result.IPs[cfg.name] = info.IP
				result.ServerIDs[cfg.name] = info.ServerID
				mu.Unlock()
				return nil
			},
		}
	}

	if err := async.RunParallel(ctx, tasks, false); err != nil {
		return NodePoolResult{}, fmt.Errorf("failed to provision pool %s: %w", spec.Name, err)
	}

	ctx.Observer.Printf("[%s] Successfully created %d servers for pool %s", phase, spec.Count, spec.Name)
	return result, nil
}

// getExistingServerNames returns names of servers already provisioned for this pool.
// Used to maintain idempotency with random server names across re-runs.
func (p *Provisioner) getExistingServerNames(ctx *provisioning.Context, role, pool string) ([]string, error) {
	servers, err := ctx.Infra.GetServersByLabel(ctx, map[string]string{
		labels.KeyCluster: ctx.Config.ClusterName,
		labels.KeyRole:    role,
		labels.KeyPool:    pool,
	})
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(servers))
	for _, s := range servers {
		names = append(names, s.Name)
	}
	return names, nil
}
