// Package upgrade handles Talos OS and Kubernetes version upgrades.
//
// Control plane nodes are upgraded sequentially to maintain etcd quorum.
// Worker nodes are upgraded in parallel. The upgrade process uses Talos
// API to trigger upgrades and waits for each node to become healthy
// before proceeding.
package upgrade
