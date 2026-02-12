# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.8.0] - 2026-02-12

### Added

- **TUI dashboard** for real-time provisioning observability (#138)
  - Progress bar with ETA, node status table, addon grid with timing and retry info
  - Phase history timeline with durations, error ring buffer (last 10)
  - `apply` shows TUI during bootstrap; `doctor --watch` uses TUI; `--ci` flag for plain log output
- **Operator addon health probes** — core addon and connectivity checks (DNS, TLS, HTTPS) reported in CRD status (#139)
- **Cloudflare DNS cleanup** on `destroy` — removes DNS records created by external-dns (#139)

### Changed

- **E2E test refactor** — shared addon verification functions across FullStack and HA test suites (#139)
- **Operator hardening** — phase transition recording, addon retry with exponential backoff (10s/30s/60s), timeout detection with K8s Warning events (#138)
- `doctor` health check timeout increased to 10m (#139)

## [0.7.0] - 2026-02-11

### Added

- **Operator-first architecture** — `apply` is now the single entry point for cluster lifecycle
  - New clusters: bootstraps infrastructure, deploys operator, creates CRD
  - Existing clusters: updates CRD spec, operator reconciles changes
  - `--wait` flag to wait for operator to complete provisioning
- **`K8znerCluster` CRD** for declarative cluster management via Kubernetes-native API
- **`doctor` command** — Diagnose cluster configuration and live status
  - Pre-cluster mode: validates config, shows what would be created
  - Cluster mode: ASCII table with emoji indicators for infrastructure, nodes, and addons
  - `--watch` flag for continuous updates, `--json` for machine-readable output
- **Control plane self-healing** — Detect failure, remove etcd member, replace node automatically
- **Worker and CP scaling** via CRD updates (`k8zner apply` updates CRD, operator reconciles)
- **kube-prometheus-stack** addon (Prometheus, Grafana, Alertmanager) via `monitoring: true`
- **Operator Helm chart** for in-cluster deployment with leader election and metrics endpoint
- **KIND-based integration tests** for operator reconciliation logic
- **3 Architecture Decision Records**: networking, dual-path architecture, bootstrap tolerations
- **Comprehensive test coverage improvements** (~30k lines of tests across 142 test files)

### Changed

- **Traefik**: always LoadBalancer service (was DaemonSet with hostNetwork) — **breaking**
- **cert-manager**: DNS-01 via Cloudflare (was HTTP-01) for wildcard certificate support
- **Cilium**: kube-proxy replacement enabled by default, tunnel mode, Hetzner-specific device config (`enp+`)
- `config/v2` package consolidated into `config` (internal refactor, no user-facing config change)
- E2E tests rewritten for operator-based lifecycle (24/24 passing)
- `destroy` command config flag is now optional (auto-detects k8zner.yaml)

### Removed

- **7 CLI commands** removed in favor of operator-first approach:
  - `create` — absorbed into `apply`
  - `bootstrap` — replaced by operator
  - `upgrade` — handled by operator via CRD update
  - `health` — replaced by `doctor`
  - `image` — image management is automatic
  - `cost` — cost estimation removed
  - `migrate` — migration path no longer needed
- **`internal/orchestration`** — CLI-only reconciler replaced by operator
- **`internal/pricing`** — CLI-only cost estimation
- **`internal/util/prerequisites`** — not needed with operator approach
- **`PrerequisitesCheckEnabled`** config field
- **Floating IP support** — LoadBalancer-only approach
- **ingress-nginx support** — Traefik is the sole ingress controller

### Fixed

- CSI CrashLoopBackOff during bootstrap (DNS resolution timing)
- Cilium device detection on Talos (`enp+` pattern, not `eth0`)
- ArgoCD Redis secret initialization (`redisSecretInit.enabled` must be true)
- Bootstrap toleration gaps (3 tolerations needed for all bootstrap-critical pods)
- CIDR drift between CLI and operator config paths

## [0.6.0] - 2026-01-31

### Added

- **Simplified Configuration (v2)** - Opinionated, production-ready defaults in ~12 lines
  - New `k8zner.yaml` format with only 5 fields: name, region, mode, workers, domain
  - Two cluster modes: `dev` (1 CP, 1 shared LB) and `ha` (3 CPs, 2 separate LBs)
  - IPv6-only nodes by default (saves IPv4 costs, better security)
  - Hardcoded addon stack: Cilium, Traefik, cert-manager, ArgoCD, metrics-server
  - Pinned, tested version matrix for Talos and Kubernetes
  - Auto-detection of k8zner.yaml in current directory
- **Cost Estimator** (`k8zner cost`) - Calculate monthly cluster costs
  - Fetches live pricing from Hetzner API
  - Shows line-item breakdown with VAT calculation
  - JSON and compact output formats
- **Simplified Wizard** - Only 5-6 questions to create a cluster
  - Cluster name, region, mode, worker count, worker size, domain (optional)
  - Shows cost estimate after configuration
- **Automatic etcd Backups** (`backup: true`) - S3-based etcd snapshots
  - Backs up etcd to Hetzner Object Storage every hour
  - Bucket auto-created during provisioning: `{cluster-name}-etcd-backups`
  - Requires `HETZNER_S3_ACCESS_KEY` and `HETZNER_S3_SECRET_KEY` environment variables
  - Uses talos-backup with compression enabled

### Changed

- `k8zner init` now creates `k8zner.yaml` (was `cluster.yaml`)
- `k8zner apply` auto-detects config file in current directory
- Removed `--advanced` and `--full` flags (config is now always minimal)
- Traefik now uses DaemonSet with hostNetwork for direct port binding
- All clusters use Traefik instead of ingress-nginx by default

### Removed

- Legacy complex wizard with 20+ questions
- Server architecture/category selection (now uses standard cx22 for control planes)
- Manual addon configuration (all addons are now automatic)

## [0.5.0] - 2025-01-27

### Added

- **Cloudflare DNS Integration** - Automatic DNS record management and TLS certificates
  - **external-dns addon** - Automatically creates DNS records from Ingress annotations
    - Cloudflare provider with API token authentication
    - Configurable sync policy (sync or upsert-only)
    - TXT record ownership for safe multi-cluster deployments
    - Support for Ingress and Service sources
  - **cert-manager Cloudflare DNS01 solver** - Wildcard and DNS-validated certificates
    - Let's Encrypt staging and production support
    - ClusterIssuer automatically created: `letsencrypt-cloudflare-staging` / `letsencrypt-cloudflare-production`
    - Works with Cloudflare proxied and non-proxied records
  - Environment variable support: `CF_API_TOKEN` and `CF_DOMAIN`
  - Full E2E test coverage with real DNS validation

### Changed

- **Ingress controllers now use LoadBalancer by default** for proper external IP allocation
  - ingress-nginx: Changed from NodePort to LoadBalancer service type
  - Traefik: Enabled LoadBalancer service for web entrypoint
- cert-manager ingress-shim enabled for automatic Certificate creation from Ingress annotations
- Improved E2E cleanup to wait for server deletion before removing firewalls

### Fixed

- cert-manager Gateway API disabled by default (requires Gateway API CRDs to be installed)
- Firewall deletion now retries when resources are still in use

## [0.4.0] - 2025-01-26

### Added

- **ArgoCD GitOps Addon** - Continuous delivery for Kubernetes (CNCF Graduated project)
  - Full ArgoCD server, application controller, and repo server deployment
  - Configurable via `addons.argocd` in cluster config
  - HA mode support with multiple replicas
  - Optional Dex SSO integration
  - ApplicationSet controller for multi-cluster deployments
  - Notifications controller for alerts and webhooks
  - Custom Helm values override support
  - E2E tested with full lifecycle validation

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

[0.8.0]: https://github.com/imamik/k8zner/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/imamik/k8zner/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/imamik/k8zner/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/imamik/k8zner/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/imamik/k8zner/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/imamik/k8zner/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/imamik/k8zner/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/imamik/k8zner/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/imamik/k8zner/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/imamik/k8zner/releases/tag/v0.1.0
