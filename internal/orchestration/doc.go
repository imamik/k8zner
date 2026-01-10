// Package orchestration provides high-level workflow coordination for cluster provisioning.
//
// This package orchestrates the provisioning workflow by delegating to specialized
// provisioners in the internal/provisioning subpackages. It defines the execution order
// and coordinates state flow between provisioning phases.
//
// # Workflow
//
// The Reconciler executes the following phases in order:
//  1. Validation - Pre-flight configuration validation
//  2. Infrastructure - Network, firewall, load balancers, floating IPs
//  3. Image - Talos image building and snapshot management
//  4. Compute - Server provisioning for control plane and workers
//  5. Cluster - Bootstrap and Talos configuration application
//  6. Addons - Post-bootstrap addon installation (CCM, CNI, etc.)
//
// # Usage
//
// The Reconciler is the main entry point:
//
//	reconciler := orchestration.NewReconciler(infraClient, talosGenerator, config)
//	kubeconfig, err := reconciler.Reconcile(ctx)
//
// The reconciler is idempotent - it can be run multiple times and will only
// make changes necessary to reach the desired state.
package orchestration
