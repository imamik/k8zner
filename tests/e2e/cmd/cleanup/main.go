//go:build e2e

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

func main() {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		log.Fatal("HCLOUD_TOKEN environment variable is required")
	}

	client := hcloud.NewClient(hcloud.WithToken(token))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	fmt.Println("=== E2E Test Resource Cleanup ===")
	fmt.Println("Searching for leftover E2E test resources...")

	hasErrors := false

	// Clean up all resources with e2e-related names
	prefixes := []string{"e2e-seq-", "build-talos-", "key-build-talos-"}

	for _, prefix := range prefixes {
		fmt.Printf("\n--- Cleaning resources with prefix: %s ---\n", prefix)

		// Servers
		if err := cleanupServers(ctx, client, prefix); err != nil {
			log.Printf("Error cleaning servers: %v", err)
			hasErrors = true
		}

		// Load Balancers
		if err := cleanupLoadBalancers(ctx, client, prefix); err != nil {
			log.Printf("Error cleaning load balancers: %v", err)
			hasErrors = true
		}

		// Firewalls
		if err := cleanupFirewalls(ctx, client, prefix); err != nil {
			log.Printf("Error cleaning firewalls: %v", err)
			hasErrors = true
		}

		// Networks
		if err := cleanupNetworks(ctx, client, prefix); err != nil {
			log.Printf("Error cleaning networks: %v", err)
			hasErrors = true
		}

		// Placement Groups
		if err := cleanupPlacementGroups(ctx, client, prefix); err != nil {
			log.Printf("Error cleaning placement groups: %v", err)
			hasErrors = true
		}

		// SSH Keys
		if err := cleanupSSHKeys(ctx, client, prefix); err != nil {
			log.Printf("Error cleaning SSH keys: %v", err)
			hasErrors = true
		}
	}

	// Cleanup old snapshots
	fmt.Println("\n--- Cleaning old E2E snapshots ---")
	if err := cleanupOldSnapshots(ctx, client); err != nil {
		log.Printf("Error cleaning snapshots: %v", err)
		hasErrors = true
	}

	fmt.Println("\n=== Cleanup Complete ===")
	if hasErrors {
		fmt.Println("⚠ Some resources may not have been cleaned up. Check logs above.")
		os.Exit(1)
	}
	fmt.Println("✓ All E2E test resources cleaned up successfully.")
}

func cleanupServers(ctx context.Context, client *hcloud.Client, prefix string) error {
	servers, err := client.Server.All(ctx)
	if err != nil {
		return fmt.Errorf("failed to list servers: %w", err)
	}

	count := 0
	for _, server := range servers {
		if strings.HasPrefix(server.Name, prefix) {
			fmt.Printf("  Deleting server: %s (ID: %d)\n", server.Name, server.ID)
			if _, _, err := client.Server.DeleteWithResult(ctx, server); err != nil {
				log.Printf("  Warning: Failed to delete server %s: %v", server.Name, err)
			} else {
				count++
			}
		}
	}
	if count > 0 {
		fmt.Printf("  ✓ Deleted %d servers\n", count)
	}
	return nil
}

func cleanupLoadBalancers(ctx context.Context, client *hcloud.Client, prefix string) error {
	lbs, err := client.LoadBalancer.All(ctx)
	if err != nil {
		return fmt.Errorf("failed to list load balancers: %w", err)
	}

	count := 0
	for _, lb := range lbs {
		if strings.HasPrefix(lb.Name, prefix) {
			fmt.Printf("  Deleting load balancer: %s (ID: %d)\n", lb.Name, lb.ID)
			if _, err := client.LoadBalancer.Delete(ctx, lb); err != nil {
				log.Printf("  Warning: Failed to delete load balancer %s: %v", lb.Name, err)
			} else {
				count++
			}
		}
	}
	if count > 0 {
		fmt.Printf("  ✓ Deleted %d load balancers\n", count)
	}
	return nil
}

func cleanupFirewalls(ctx context.Context, client *hcloud.Client, prefix string) error {
	firewalls, err := client.Firewall.All(ctx)
	if err != nil {
		return fmt.Errorf("failed to list firewalls: %w", err)
	}

	count := 0
	for _, fw := range firewalls {
		if strings.HasPrefix(fw.Name, prefix) {
			fmt.Printf("  Deleting firewall: %s (ID: %d)\n", fw.Name, fw.ID)
			if _, err := client.Firewall.Delete(ctx, fw); err != nil {
				log.Printf("  Warning: Failed to delete firewall %s: %v", fw.Name, err)
			} else {
				count++
			}
		}
	}
	if count > 0 {
		fmt.Printf("  ✓ Deleted %d firewalls\n", count)
	}
	return nil
}

func cleanupNetworks(ctx context.Context, client *hcloud.Client, prefix string) error {
	networks, err := client.Network.All(ctx)
	if err != nil {
		return fmt.Errorf("failed to list networks: %w", err)
	}

	count := 0
	for _, network := range networks {
		if strings.HasPrefix(network.Name, prefix) {
			fmt.Printf("  Deleting network: %s (ID: %d)\n", network.Name, network.ID)
			if _, err := client.Network.Delete(ctx, network); err != nil {
				log.Printf("  Warning: Failed to delete network %s: %v", network.Name, err)
			} else {
				count++
			}
		}
	}
	if count > 0 {
		fmt.Printf("  ✓ Deleted %d networks\n", count)
	}
	return nil
}

func cleanupPlacementGroups(ctx context.Context, client *hcloud.Client, prefix string) error {
	pgs, err := client.PlacementGroup.All(ctx)
	if err != nil {
		return fmt.Errorf("failed to list placement groups: %w", err)
	}

	count := 0
	for _, pg := range pgs {
		if strings.HasPrefix(pg.Name, prefix) {
			fmt.Printf("  Deleting placement group: %s (ID: %d)\n", pg.Name, pg.ID)
			if _, err := client.PlacementGroup.Delete(ctx, pg); err != nil {
				log.Printf("  Warning: Failed to delete placement group %s: %v", pg.Name, err)
			} else {
				count++
			}
		}
	}
	if count > 0 {
		fmt.Printf("  ✓ Deleted %d placement groups\n", count)
	}
	return nil
}

func cleanupSSHKeys(ctx context.Context, client *hcloud.Client, prefix string) error {
	keys, err := client.SSHKey.All(ctx)
	if err != nil {
		return fmt.Errorf("failed to list SSH keys: %w", err)
	}

	count := 0
	for _, key := range keys {
		if strings.HasPrefix(key.Name, prefix) {
			fmt.Printf("  Deleting SSH key: %s (ID: %d)\n", key.Name, key.ID)
			if _, err := client.SSHKey.Delete(ctx, key); err != nil {
				log.Printf("  Warning: Failed to delete SSH key %s: %v", key.Name, err)
			} else {
				count++
			}
		}
	}
	if count > 0 {
		fmt.Printf("  ✓ Deleted %d SSH keys\n", count)
	}
	return nil
}

func cleanupOldSnapshots(ctx context.Context, client *hcloud.Client) error {
	images, err := client.Image.All(ctx)
	if err != nil {
		return fmt.Errorf("failed to list images: %w", err)
	}

	count := 0
	cutoff := time.Now().Add(-24 * time.Hour)

	for _, img := range images {
		// Only cleanup snapshots (not system images)
		if img.Type != hcloud.ImageTypeSnapshot {
			continue
		}

		// Check if it's an E2E test snapshot (has test-id label or e2e- prefix in description)
		isE2E := false
		if _, hasTestID := img.Labels["test-id"]; hasTestID {
			isE2E = true
		}
		if strings.Contains(img.Description, "e2e-") || strings.Contains(img.Description, "talos-v") {
			isE2E = true
		}

		if isE2E && img.Created.Before(cutoff) {
			fmt.Printf("  Deleting old snapshot: %s (ID: %d, Created: %s)\n",
				img.Description, img.ID, img.Created.Format("2006-01-02 15:04"))
			if _, err := client.Image.Delete(ctx, img); err != nil {
				log.Printf("  Warning: Failed to delete snapshot %d: %v", img.ID, err)
			} else {
				count++
			}
		}
	}
	if count > 0 {
		fmt.Printf("  ✓ Deleted %d old snapshots\n", count)
	} else {
		fmt.Println("  No old snapshots to clean up")
	}
	return nil
}
