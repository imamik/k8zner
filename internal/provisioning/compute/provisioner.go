package compute

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/imamik/k8zner/internal/config"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/util/async"
	"github.com/imamik/k8zner/internal/util/keygen"
	"github.com/imamik/k8zner/internal/util/labels"
	"github.com/imamik/k8zner/internal/util/naming"
)

// Provisioner handles compute resource provisioning (servers, node pools).
type Provisioner struct{}

// NewProvisioner creates a new compute provisioner.
func NewProvisioner() *Provisioner {
	return &Provisioner{}
}

// Name implements the provisioning.Phase interface.
func (p *Provisioner) Name() string {
	return "compute"
}

// Provision implements the provisioning.Phase interface.
// This method creates ALL servers (control plane + workers) in parallel
// for maximum provisioning speed.
// Uses ephemeral SSH keys to avoid Hetzner password emails.
func (p *Provisioner) Provision(ctx *provisioning.Context) error {
	// 0. Create ephemeral SSH key to avoid Hetzner password emails
	sshKeyName := fmt.Sprintf("ephemeral-%s-compute-%d", ctx.Config.ClusterName, time.Now().Unix())
	ctx.Observer.Printf("[%s] Creating ephemeral SSH key: %s", phase, sshKeyName)

	keyPair, err := keygen.GenerateRSAKeyPair(2048)
	if err != nil {
		return fmt.Errorf("failed to generate ephemeral SSH key: %w", err)
	}

	sshKeyLabels := labels.NewLabelBuilder(ctx.Config.ClusterName).
		WithTestIDIfSet(ctx.Config.TestID).
		Build()
	sshKeyLabels["type"] = "ephemeral-compute"

	_, err = ctx.Infra.CreateSSHKey(ctx, sshKeyName, string(keyPair.PublicKey), sshKeyLabels)
	if err != nil {
		return fmt.Errorf("failed to create ephemeral SSH key: %w", err)
	}

	// Schedule cleanup of the ephemeral SSH key (always runs)
	defer func() {
		ctx.Observer.Printf("[%s] Cleaning up ephemeral SSH key: %s", phase, sshKeyName)
		if err := ctx.Infra.DeleteSSHKey(ctx, sshKeyName); err != nil {
			ctx.Observer.Printf("[%s] Warning: failed to delete ephemeral SSH key %s: %v", phase, sshKeyName, err)
		}
	}()

	// Replace user SSH keys with ephemeral key for this provisioning session
	originalSSHKeys := ctx.Config.SSHKeys
	ctx.Config.SSHKeys = []string{sshKeyName}
	defer func() {
		ctx.Config.SSHKeys = originalSSHKeys
	}()

	// 1. Pre-compute: Setup LB endpoint and collect SANs
	if err := p.prepareControlPlaneEndpoint(ctx); err != nil {
		return err
	}

	// 2. Create ALL servers in parallel (CP + Workers together)
	if err := p.provisionAllServers(ctx); err != nil {
		return err
	}

	return nil
}

// prepareControlPlaneEndpoint sets up the Talos endpoint from the Load Balancer.
// This must run before server creation so Talos configs have the right endpoint.
func (p *Provisioner) prepareControlPlaneEndpoint(ctx *provisioning.Context) error {
	ctx.Observer.Printf("[%s] Preparing control plane endpoint...", phase)

	var sans []string

	// Get LB IP for endpoint
	lb, err := ctx.Infra.GetLoadBalancer(ctx, naming.KubeAPILoadBalancer(ctx.Config.ClusterName))
	if err != nil {
		return fmt.Errorf("failed to get load balancer: %w", err)
	}
	if lb != nil {
		if lbIP := hcloud.LoadBalancerIPv4(lb); lbIP != "" {
			sans = append(sans, lbIP)
			endpoint := fmt.Sprintf("https://%s:%d", lbIP, config.KubeAPIPort)
			ctx.Observer.Printf("[%s] Setting Talos endpoint to: %s", phase, endpoint)
			ctx.Talos.SetEndpoint(endpoint)
		}

		for _, net := range lb.PrivateNet {
			sans = append(sans, net.IP.String())
		}
	}

	ctx.State.SANs = sans
	return nil
}

// provisionAllServers creates all control plane and worker servers in parallel.
// This significantly reduces total provisioning time by overlapping server creation.
func (p *Provisioner) provisionAllServers(ctx *provisioning.Context) error {
	var tasks []async.Task
	var mu sync.Mutex

	// Build tasks for control plane pools
	for i, pool := range ctx.Config.ControlPlane.NodePools {
		pool := pool
		poolIndex := i
		tasks = append(tasks, async.Task{
			Name: fmt.Sprintf("cp-pool-%s", pool.Name),
			Func: func(_ context.Context) error {
				// Ensure placement group
				pgLabels := labels.NewLabelBuilder(ctx.Config.ClusterName).
					WithPool(pool.Name).
					WithTestIDIfSet(ctx.Config.TestID).
					Build()

				pg, err := ctx.Infra.EnsurePlacementGroup(ctx, naming.PlacementGroup(ctx.Config.ClusterName, pool.Name), "spread", pgLabels)
				if err != nil {
					return fmt.Errorf("failed to ensure placement group for pool %s: %w", pool.Name, err)
				}

				poolResult, err := p.reconcileNodePool(ctx, NodePoolSpec{
					Name:             pool.Name,
					Count:            pool.Count,
					ServerType:       pool.ServerType,
					Location:         pool.Location,
					Image:            pool.Image,
					Role:             "control-plane",
					ExtraLabels:      pool.Labels,
					PlacementGroupID: &pg.ID,
					PoolIndex:        poolIndex,
				})
				if err != nil {
					return err
				}

				mu.Lock()
				for k, v := range poolResult.IPs {
					ctx.State.ControlPlaneIPs[k] = v
				}
				for k, v := range poolResult.ServerIDs {
					ctx.State.ControlPlaneServerIDs[k] = v
				}
				mu.Unlock()
				return nil
			},
		})
	}

	// Build tasks for worker pools
	for i, pool := range ctx.Config.Workers {
		pool := pool
		poolIndex := i
		tasks = append(tasks, async.Task{
			Name: fmt.Sprintf("worker-pool-%s", pool.Name),
			Func: func(_ context.Context) error {
				poolResult, err := p.reconcileNodePool(ctx, NodePoolSpec{
					Name:        pool.Name,
					Count:       pool.Count,
					ServerType:  pool.ServerType,
					Location:    pool.Location,
					Image:       pool.Image,
					Role:        "worker",
					ExtraLabels: pool.Labels,
					PoolIndex:   poolIndex,
				})
				if err != nil {
					return err
				}

				mu.Lock()
				for name, ip := range poolResult.IPs {
					ctx.State.WorkerIPs[name] = ip
				}
				for name, id := range poolResult.ServerIDs {
					ctx.State.WorkerServerIDs[name] = id
				}
				mu.Unlock()
				return nil
			},
		})
	}

	if len(tasks) == 0 {
		return nil
	}

	totalCPs := 0
	for _, pool := range ctx.Config.ControlPlane.NodePools {
		totalCPs += pool.Count
	}
	totalWorkers := 0
	for _, pool := range ctx.Config.Workers {
		totalWorkers += pool.Count
	}

	ctx.Observer.Printf("[%s] Creating %d control plane + %d worker servers in parallel...", phase, totalCPs, totalWorkers)

	if err := async.RunParallel(ctx, tasks, true); err != nil {
		return fmt.Errorf("failed to provision servers: %w", err)
	}

	ctx.Observer.Printf("[%s] Successfully created all %d servers", phase, totalCPs+totalWorkers)
	return nil
}
