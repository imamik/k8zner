// Package controller implements the Kubernetes controller for K8znerCluster
// custom resources.
//
// The controller uses a phase-based state machine to drive cluster lifecycle:
// Infrastructure -> Image -> Compute -> Bootstrap -> CNI -> Addons -> Running.
//
// Once running, the controller continuously monitors node health, replaces
// unhealthy nodes (respecting etcd quorum for control planes), and scales
// workers to match the desired count.
//
// The controller is split across several files by concern:
//   - cluster_controller.go: Entry point, reconcile loop, setup
//   - cluster_state.go: Cluster state resolution helpers
//   - reconcile_phases.go: Provisioning phase handlers
//   - reconcile_addons.go: CNI and addon phase handlers
//   - reconcile_scaling_cp.go: Control plane scaling and replacement
//   - reconcile_scaling_workers.go: Worker scaling and replacement
//   - reconcile_health.go: Node health monitoring
//   - reconcile_healing.go: Unhealthy node replacement
//   - reconcile_infra_health.go: Infrastructure health checks (LB, network, firewall)
//   - reconcile_addon_health.go: Addon deployment status monitoring
//   - reconcile_connectivity.go: Cluster connectivity probes
package controller
