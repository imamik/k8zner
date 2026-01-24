// Package main provides a standalone cleanup utility for Hetzner Cloud resources.
//
// This command can be used to clean up resources by label, which is especially
// useful for cleaning up E2E test resources or orphaned infrastructure.
//
// Usage:
//
//	# Clean up all resources with test-id=e2e-seq-123456
//	cleanup -test-id e2e-seq-123456
//
//	# Clean up all resources for a cluster
//	cleanup -cluster my-cluster-name
//
//	# Clean up with multiple labels
//	cleanup -label cluster=my-cluster -label environment=test
//
//	# Dry run (list resources without deleting)
//	cleanup -test-id e2e-seq-123456 -dry-run
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/imamik/k8zner/internal/platform/hcloud"
)

type labelFlags []string

func (lf *labelFlags) String() string {
	return strings.Join(*lf, ",")
}

func (lf *labelFlags) Set(value string) error {
	*lf = append(*lf, value)
	return nil
}

func main() {
	var (
		testID      = flag.String("test-id", "", "Test ID label to clean up (e.g., e2e-seq-123456)")
		clusterName = flag.String("cluster", "", "Cluster name to clean up")
		dryRun      = flag.Bool("dry-run", false, "List resources without deleting them")
		labels      labelFlags
	)

	flag.Var(&labels, "label", "Additional labels in key=value format (can be specified multiple times)")
	flag.Parse()

	// Build label selector
	labelSelector := make(map[string]string)

	if *testID != "" {
		labelSelector["test-id"] = *testID
	}

	if *clusterName != "" {
		labelSelector["cluster"] = *clusterName
	}

	for _, label := range labels {
		parts := strings.SplitN(label, "=", 2)
		if len(parts) != 2 {
			log.Fatalf("Invalid label format: %s (expected key=value)", label)
		}
		labelSelector[parts[0]] = parts[1]
	}

	if len(labelSelector) == 0 {
		log.Fatal("Error: At least one label must be specified (--test-id, --cluster, or --label)")
	}

	// Get Hetzner Cloud token
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		log.Fatal("Error: HCLOUD_TOKEN environment variable not set")
	}

	// Create client
	client := hcloud.NewRealClient(token)

	ctx := context.Background()

	// Log what we're doing
	log.Printf("Cleanup utility starting...")
	log.Printf("Label selector: %v", labelSelector)
	if *dryRun {
		log.Printf("DRY RUN MODE: No resources will be deleted")
	}

	// In dry-run mode, we'd need to implement a list-only version
	// For now, we'll just show what would be deleted
	if *dryRun {
		log.Fatal("Error: Dry-run mode not yet implemented. Remove -dry-run flag to proceed with cleanup.")
	}

	// Perform cleanup
	if err := client.CleanupByLabel(ctx, labelSelector); err != nil {
		log.Fatalf("Cleanup failed: %v", err)
	}

	fmt.Println("\n✅ Cleanup completed successfully!")
}
