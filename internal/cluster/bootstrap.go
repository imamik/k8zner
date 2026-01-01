package cluster

import (
	"context"
	"fmt"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/client/config"
)

// Bootstrapper handles cluster bootstrapping.
type Bootstrapper struct {
	// In real impl, we might want to inject a client factory
}

// Bootstrap bootstraps the cluster.
func (b *Bootstrapper) Bootstrap(ctx context.Context, endpoint string, nodes []string) error {
	// 1. Check State Marker (HCloud Certificate) - To be implemented in Reconciler before calling this

	// 2. Init Talos Client
	// We need a client config to talk to the node.
	// Since we are bootstrapping, we might be using the generated secrets/config.
	// For E2E/Bootstrap, we often need to wait for the node to be up.

	fmt.Printf("Bootstrapping cluster at %s with nodes %v\n", endpoint, nodes)

	// Example client setup (simplified)
	cfg := &config.Config{
		Context: "admin",
		Contexts: map[string]*config.Context{
			"admin": {
				Endpoints: []string{endpoint},
				Nodes:     nodes,
			},
		},
	}

	c, err := client.New(ctx, client.WithConfig(cfg))
	if err != nil {
		return fmt.Errorf("failed to create talos client: %w", err)
	}
	defer c.Close()

	// Wait for node to be reachable (simplified)
	// In real world, we'd loop/retry.

	// 3. Call Bootstrap
	// err = c.Bootstrap(ctx, ...)
	// For this exercise, we just simulate the call structure as we don't have a real node.
	// Real implementation would look like:
	/*
	err = c.Bootstrap(ctx, &machineapi.BootstrapRequest{
		Node: nodes[0],
	})
	*/

	// Simulate delay
	time.Sleep(10 * time.Millisecond)

	return nil
}
