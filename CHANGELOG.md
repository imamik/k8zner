# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.0] - 2025-01-26

### Added

- **Traefik Ingress Controller** - Alternative ingress controller option
  - Full Traefik v3 support with Helm chart integration
  - Configurable via `addons.traefik` in cluster config
  - Proxy protocol support for preserving client IPs
  - Prometheus metrics integration
  - Interactive wizard updated to offer Traefik as an ingress option

- **Runtime Helm Chart Downloading** - Charts downloaded at runtime instead of embedded
  - Removes ~4.9MB of embedded chart templates (464 files)
  - Charts cached in `~/.cache/k8zner/charts/` for faster subsequent runs
  - Users can override chart versions via `helm.version` in addon config
  - Automatic chart updates without rebuilding the binary

### Changed

- Updated addon chart versions to latest stable releases:
  - Cilium: 1.18.5
  - cert-manager: v1.19.2
  - cluster-autoscaler: 9.50.1
  - hcloud-ccm: 1.29.0
  - hcloud-csi: 2.18.3
  - ingress-nginx: 4.11.3
  - longhorn: 1.10.1
  - metrics-server: 3.12.2
  - traefik: 39.0.0

### Fixed

- JSON schema validation for Cilium chart with slice type values
- YAML rendering now correctly skips whitespace-only template output

## [0.2.1] - 2025-01-25

### Added

- Comprehensive wizard documentation (`docs/wizard.md`)
- Full configuration reference (`docs/configuration.md`)
- System architecture documentation (`docs/architecture.md`)

### Changed

- Streamlined README with focus on interactive wizard as primary setup method
- Updated server types documentation with all Hetzner families

## [0.2.0] - 2025-01-25

### Added

- **Interactive Config Builder** (`k8zner init`) - New wizard for creating cluster configurations
  - Hierarchical server selection: choose architecture (x86/ARM) first, then category (shared/dedicated/cost-optimized)
  - Updated Hetzner instance types with current pricing data (CX, CPX, CCX, CAX families)
  - CNI selection as dedicated choice: Cilium, Talos default (Flannel), or none
  - Optional SSH keys - auto-generated if not provided
  - Minimal YAML output by default (only essential values)
  - `--full` flag for complete YAML with all configuration options
  - `--advanced` flag for network, security, and Cilium customization
- Configurable timeouts for cluster operations
- CI pipeline optimization with parallel test jobs
- Codecov integration for test coverage reporting

### Changed

- Cluster bootstrap now uses configurable timeouts instead of hardcoded values
- Network configuration uses improved retry logic

## [0.1.1] - 2025-01-20

### Fixed

- Minor bug fixes and stability improvements

## [0.1.0] - 2025-01-15

### Added

- Initial public release
- Declarative YAML configuration for cluster definition
- Talos Linux support for immutable, secure Kubernetes nodes
- High availability control plane with automatic failover
- Worker pool management with labels and taints
- Auto-scaling support via Cluster Autoscaler integration
- Snapshot-based provisioning for fast node creation
- Full addon suite:
  - Cilium CNI with network policies
  - Hetzner Cloud Controller Manager (CCM)
  - Hetzner CSI Driver for persistent volumes
  - cert-manager for TLS certificate management
  - ingress-nginx for ingress routing
  - metrics-server for Kubernetes metrics
- Self-contained binary (no kubectl or talosctl required at runtime)
- Shell completions for bash, zsh, fish, and PowerShell
- Commands:
  - `apply` - Create or update cluster infrastructure
  - `destroy` - Tear down all cluster resources
  - `upgrade` - Upgrade Talos and/or Kubernetes versions
  - `image build` - Build Talos image snapshots
  - `image delete` - Delete Talos image snapshots
  - `completion` - Generate shell completion scripts
  - `version` - Print version information

### Security

- Private network isolation for inter-node communication
- Firewall rules for control plane and worker nodes
- Secrets stored locally in `./secrets/` directory
- No credentials stored in cluster state

[Unreleased]: https://github.com/imamik/k8zner/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/imamik/k8zner/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/imamik/k8zner/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/imamik/k8zner/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/imamik/k8zner/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/imamik/k8zner/releases/tag/v0.1.0
