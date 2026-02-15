// Package provisioning provides shared types, interfaces, and orchestration for cluster provisioning.
//
// # Architecture
//
// The provisioning domain is organized into focused subpackages, each with a clear single responsibility:
//
//   - infrastructure/ — Network, Firewall, Load Balancers, Floating IPs
//   - compute/ — Servers, Control Plane, Workers, Node Pools
//   - image/ — Talos image building and snapshot management
//   - cluster/ — Bootstrap and Talos configuration application
//
// # Core Concepts
//
// Context: A unified context carrying configuration, state, infrastructure client, and logger.
// All provisioning phases receive this context for accessing shared resources.
//
// Phase: Interface for provisioning phases. Each phase has a Name() and Provision() method.
// Phases are executed sequentially by RunPhases.
//
// State: Accumulates results from each phase (network, firewall, server IPs, kubeconfig).
// Later phases can access results from earlier phases via the shared state.
//
// Observer: Simple logging interface for progress and diagnostics.
//
// # Validation
//
// The validation package provides pre-flight checks via multiple Validator implementations:
//   - RequiredFieldsValidator: Ensures cluster name, location, network zone are set
//   - NetworkValidator: Validates CIDR format and size
//   - ServerTypeValidator: Validates node pool configurations
//   - SSHKeyValidator: Warns if no SSH keys are configured
//   - VersionValidator: Validates Talos and Kubernetes version formats
package provisioning
