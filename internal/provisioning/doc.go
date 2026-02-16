// Package provisioning provides shared types, interfaces, and orchestration for cluster provisioning.
//
// # Subpackages
//
//   - infrastructure/ — Network, Firewall, Load Balancers
//   - compute/ — Servers, Control Plane, Workers, Node Pools
//   - image/ — Talos image building and snapshot management
//   - cluster/ — Bootstrap and Talos configuration application
//   - destroy/ — Resource cleanup and teardown
//
// # Core Types
//
// Context carries configuration, state, infrastructure client, and logger.
// Phase defines a provisioning step with Name() and Provision() methods.
// State accumulates results from each phase (network, firewall, server IPs, kubeconfig).
package provisioning
