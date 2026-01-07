// Package cluster provides the core reconciliation logic for provisioning and managing
// Kubernetes clusters on Hetzner Cloud using Talos Linux.
//
// # Architecture
//
// The cluster package is organized into focused modules:
//
//   - reconciler.go: Main reconciliation orchestration
//   - control_plane.go: Control plane node provisioning
//   - workers.go: Worker node provisioning with parallel pool support
//   - node_pool.go: Generic node pool provisioning with placement group sharding
//   - image_management.go: Talos image building and caching
//   - server_provisioning.go: Individual server creation and configuration
//   - bootstrap.go: Talos cluster bootstrapping
//   - async.go: Parallel task execution helpers
//   - names.go: Resource naming conventions
//   - labels.go: Label builder with fluent interface
//
// # Reconciliation Flow
//
// 1. Network setup (VPC, subnets)
// 2. Parallel image building for all required architectures
// 3. Parallel infrastructure provisioning (firewall, load balancers, floating IPs)
// 4. Control plane provisioning with placement groups
// 5. Talos cluster bootstrap
// 6. Worker pool provisioning with parallel execution and placement group sharding
//
// # Key Design Principles
//
//   - Parallel execution: Infrastructure components and server pools are provisioned in parallel
//   - Idempotency: All operations can be safely retried
//   - Resource efficiency: Images are pre-built once and reused across all nodes
//   - Placement groups: Workers are sharded across placement groups (10 servers per group)
//   - Timeout configuration: All operations have configurable timeouts
//
// # Example Usage
//
//	config := &config.Config{
//	    ClusterName: "my-cluster",
//	    ControlPlane: config.ControlPlaneConfig{
//	        NodePools: []config.NodePoolConfig{
//	            {Name: "control-plane", Count: 3, ServerType: "cpx31", Location: "nbg1"},
//	        },
//	    },
//	    Workers: []config.NodePoolConfig{
//	        {Name: "worker", Count: 3, ServerType: "cpx21", Location: "nbg1"},
//	    },
//	}
//
//	infra := hcloud.NewClient(token)
//	talosGen := talos.NewGenerator(config)
//	reconciler := cluster.NewReconciler(infra, talosGen, config)
//
//	kubeconfig, err := reconciler.Reconcile(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Save kubeconfig
//	os.WriteFile("kubeconfig", kubeconfig, 0600)
package cluster
