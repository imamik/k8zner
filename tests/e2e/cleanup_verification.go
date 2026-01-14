//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"hcloud-k8s/internal/platform/hcloud"
)

// verifyCleanupComplete checks if all resources for the test have been deleted.
// This is called after cleanup to ensure nothing was left behind.
func verifyCleanupComplete(t *testing.T, state *E2EState) {
	if state == nil || state.Client == nil || state.ClusterName == "" {
		return
	}

	t.Log("[Cleanup Verification] Checking for leftover resources...")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	leftover := []string{}

	// Check servers
	if servers, err := state.Client.ListServers(ctx); err == nil {
		for _, server := range servers {
			if strings.HasPrefix(server.Name, state.ClusterName) {
				leftover = append(leftover, "server:"+server.Name)
			}
		}
	}

	// Check load balancers
	if lbs, err := state.Client.ListLoadBalancers(ctx); err == nil {
		for _, lb := range lbs {
			if strings.HasPrefix(lb.Name, state.ClusterName) {
				leftover = append(leftover, "loadbalancer:"+lb.Name)
			}
		}
	}

	// Check networks
	if networks, err := state.Client.ListNetworks(ctx); err == nil {
		for _, net := range networks {
			if strings.HasPrefix(net.Name, state.ClusterName) {
				leftover = append(leftover, "network:"+net.Name)
			}
		}
	}

	// Check firewalls
	if firewalls, err := state.Client.ListFirewalls(ctx); err == nil {
		for _, fw := range firewalls {
			if strings.HasPrefix(fw.Name, state.ClusterName) {
				leftover = append(leftover, "firewall:"+fw.Name)
			}
		}
	}

	// Check placement groups
	if pgs, err := state.Client.ListPlacementGroups(ctx); err == nil {
		for _, pg := range pgs {
			if strings.HasPrefix(pg.Name, state.ClusterName) {
				leftover = append(leftover, "placement-group:"+pg.Name)
			}
		}
	}

	// Check SSH keys
	if keys, err := state.Client.ListSSHKeys(ctx); err == nil {
		for _, key := range keys {
			if strings.HasPrefix(key.Name, state.ClusterName) {
				leftover = append(leftover, "ssh-key:"+key.Name)
			}
		}
	}

	if len(leftover) > 0 {
		t.Errorf("[Cleanup Verification] Found %d leftover resources: %v", len(leftover), leftover)
		t.Log("[Cleanup Verification] Run 'make e2e-cleanup' to clean them up manually")
	} else {
		t.Log("[Cleanup Verification] ✓ No leftover resources found")
	}
}
