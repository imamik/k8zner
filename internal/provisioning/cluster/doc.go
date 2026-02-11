// Package cluster handles cluster bootstrap and Talos configuration.
//
// The bootstrap process applies machine configs to nodes (directly or via
// load balancer in private-first mode), waits for readiness, initializes
// etcd, and retrieves the kubeconfig. It also detects and configures new
// nodes in maintenance mode during scaling operations.
package cluster
