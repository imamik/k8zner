# Architecture

This document describes the internal architecture of k8zner.

## Overview

k8zner uses an **operator-first architecture**. The CLI bootstraps infrastructure and deploys a Kubernetes operator that continuously reconciles the cluster to match the desired state defined in a `K8znerCluster` CRD.

```
┌─────────────────────────────────────────────────────────────────┐
│                         k8zner CLI                              │
│              (init, apply, destroy, doctor)                     │
├─────────────────────────────────────────────────────────────────┤
│                      Bootstrap Layer                            │
│         (Image, Infrastructure, 1st CP, Operator)               │
├─────────────┬─────────────┬─────────────┬──────────────────────┤
│ Provisioning│   Config    │   Platform  │     Operator         │
│ ├─ infra    │ ├─ types    │ ├─ hcloud   │ ├─ controller        │
│ ├─ compute  │ ├─ v2       │ ├─ talos    │ ├─ adapter           │
│ ├─ image    │ ├─ wizard   │ └─ ssh      │ ├─ addons            │
│ └─ cluster  │ └─ validate │             │ └─ CRD (K8znerCluster)│
└─────────────┴─────────────┴─────────────┴──────────────────────┘
```

## Operator-First Flow

### New Cluster (`k8zner apply`)

1. **Image** — Build/reuse Talos OS snapshot
2. **Infrastructure** — Create network, firewall, placement groups, load balancer (HA)
3. **First Control Plane** — Create 1 CP server, bootstrap etcd + Kubernetes
4. **Operator** — Deploy k8zner-operator into the cluster
5. **CRD** — Create `K8znerCluster` resource with full desired state
6. **Reconcile** — Operator takes over: installs CNI, addons, scales up remaining CPs/workers

### Existing Cluster (`k8zner apply`)

1. Update `K8znerCluster` CRD spec
2. Operator detects change and reconciles (scale workers, update addons, etc.)

### Operator Reconciliation Phases

The operator reconciles through ordered phases:

| Phase | Description |
|-------|-------------|
| Infrastructure | Ensure network, firewall, LB exist |
| Compute | Scale control planes and workers to desired count |
| WaitForK8s | Wait for all nodes to join Kubernetes |
| CNI | Install Cilium (must be first addon) |
| Addons | Install remaining addons (CCM, CSI, Traefik, cert-manager, etc.) |
| Running | Cluster is healthy, continuous monitoring |

## Dual-Path Architecture

The codebase has two entry points that share the same core layers:

```
                    ┌──────────────────────────────────────────────────────┐
                    │                   Entry Points                       │
                    │                                                      │
                    │   ┌──────────────┐        ┌───────────────────┐     │
                    │   │  CLI (k8zner)│        │ Operator (CRD)    │     │
                    │   │  k8zner.yaml │        │ K8znerCluster     │     │
                    │   └──────┬───────┘        └────────┬──────────┘     │
                    │          │                         │                 │
                    │          ▼                         ▼                 │
                    │   ┌──────────────┐        ┌───────────────────┐     │
                    │   │ v2.Expand()  │        │ SpecToConfig()    │     │
                    │   └──────┬───────┘        └────────┬──────────┘     │
                    │          │                         │                 │
                    │          └────────────┬────────────┘                 │
                    │                       ▼                              │
                    │              ┌─────────────────┐                    │
                    │              │  config.Config   │  (shared runtime  │
                    │              │  (internal)      │   representation) │
                    │              └────────┬────────┘                    │
                    │                       │                              │
                    │          ┌────────────┼────────────┐                │
                    │          ▼            ▼            ▼                │
                    │   ┌────────────┐ ┌─────────┐ ┌──────────┐         │
                    │   │provisioning│ │ addons  │ │ platform │         │
                    │   │ (shared)   │ │(shared) │ │ (shared) │         │
                    │   └────────────┘ └─────────┘ └──────────┘         │
                    └──────────────────────────────────────────────────────┘
```

**CLI path** (`cmd/k8zner/`): Bootstraps a single CP node, installs all addons in one shot.
**Operator path** (`cmd/operator/`): Reconciles `K8znerCluster` CRDs, scales CP 1→N, installs addons in phases.

### Addon Entry Points

The shared `internal/addons/` package has three entry points to support both paths:

| Function | Used by | Installs |
|----------|---------|----------|
| `Apply()` | CLI | Everything (Cilium + addons + operator) |
| `ApplyCilium()` | Operator CNI phase | Cilium only |
| `ApplyWithoutCilium()` | Operator addons phase | All addons except Cilium and operator |

All three delegate to a shared `applyAddons()` with options controlling inclusion.

### Config Round-Trip

Three representations of cluster configuration must stay in sync:

1. **v2 YAML spec** (`internal/config/v2/types.go`) — user-facing config file
2. **Internal Config** (`internal/config/types.go`) — runtime representation with expanded defaults
3. **CRD spec** (`api/v1alpha1/types.go`) — Kubernetes-native representation

```
CLI:      k8zner.yaml  ──▶  v2.Expand()     ──▶  config.Config  ──▶  provisioning/addons
Operator: CRD spec     ──▶  SpecToConfig()  ──▶  config.Config  ──▶  provisioning/addons
```

`SpecToConfig()` references the same constants from `config/v2/versions.go` that `v2.Expand()` uses,
ensuring both paths produce identical `config.Config` for equivalent inputs.

## Project Structure

```
cmd/
├── k8zner/
│   ├── commands/         # CLI command definitions (Cobra)
│   └── handlers/         # Business logic for commands
├── operator/             # Operator entrypoint (controller-runtime)
└── cleanup/              # Standalone cleanup utility
│
internal/
├── operator/             # Kubernetes operator
│   ├── controller/       # CRD reconciliation (phases, scaling, healing, health)
│   ├── provisioning/     # CRD spec → internal Config adapter
│   └── addons/           # Operator-specific addon phase manager
│
├── config/               # Configuration handling
│   ├── types.go          # Internal config struct
│   └── v2/               # Simplified config (k8zner.yaml)
│       ├── types.go       # V2 config types
│       ├── expand.go      # V2 → internal config expansion
│       └── versions.go    # Version matrix, network CIDRs, constants
│
├── provisioning/         # Infrastructure provisioning
│   ├── infrastructure/   # Network, firewall, LB
│   ├── compute/          # Servers, node pools
│   ├── image/            # Talos image building
│   ├── cluster/          # Bootstrap and Talos config
│   ├── destroy/          # Resource cleanup and teardown
│   └── upgrade/          # Node upgrade provisioning
│
├── addons/               # Kubernetes addon management (shared by CLI and operator)
│   ├── helm/             # Helm chart rendering and value building
│   └── k8sclient/        # Kubernetes API operations
│
├── platform/             # External integrations
│   ├── hcloud/           # Hetzner Cloud API client (generic operations)
│   ├── talos/            # Talos configuration and patches
│   ├── ssh/              # SSH command execution
│   └── s3/               # S3/backup integration
│
└── util/                 # Shared utilities
    ├── async/            # Async operations
    ├── keygen/           # SSH key generation
    ├── labels/           # Label management
    ├── naming/           # Resource naming
    ├── rdns/             # Reverse DNS
    └── retry/            # Retry with backoff

api/v1alpha1/             # CRD types (zero internal imports)
tests/
├── e2e/                  # End-to-end tests (real Hetzner infrastructure)
└── kind/                 # KinD integration tests (local)
```

## Key Design Decisions

### Self-Contained Binary

k8zner embeds all necessary functionality:
- Kubernetes client (no kubectl required)
- Talos configuration (no talosctl required)
- Helm chart rendering

### Idempotent Operations

All operations are safe to run repeatedly:
- Resources are created if missing
- Existing resources are updated if needed
- No duplicate resources created

### Declarative Configuration

Users define desired state in YAML:
- k8zner reconciles actual state to match
- Changes are detected and applied
- Rollback by reverting configuration

### Snapshot-Based Provisioning

Talos images are built once and reused:
- Reduces provisioning time
- Ensures consistent node configuration
- Supports custom extensions

## Data Flow

```
k8zner.yaml (v2 config)
       │
       ▼
┌─────────────────┐
│  Config Loader  │  v2 → internal config expansion
└────────┬────────┘
         │
         ▼
┌─────────────────┐     ┌─────────────┐
│  CLI Bootstrap  │────▶│ Hetzner     │
│  (first time)   │     │ Cloud API   │
└────────┬────────┘     └─────────────┘
         │
         │  Image → Infra → 1 CP → Bootstrap
         │
         ▼
┌─────────────────┐
│ Install Operator│  Deploy k8zner-operator
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Create CRD     │  K8znerCluster with full spec
└────────┬────────┘
         │
         ▼
┌─────────────────────────────────────────┐
│         Operator Reconciler             │
│                                         │
│  CRD Spec ──▶ Desired State            │
│       │                                 │
│  ┌────┴────┐  ┌────────┐  ┌────────┐  │
│  │ Compute │  │ Addons │  │ Health │  │
│  │ (scale) │  │(install)│  │(monitor)│  │
│  └─────────┘  └────────┘  └────────┘  │
│                                         │
│  Continuous reconciliation (30s loop)   │
└─────────────────────────────────────────┘
```

## Error Handling

### Retry Logic

Operations use exponential backoff:
- Initial delay: 1 second
- Maximum delay: 30 seconds
- Maximum retries: 5

### Fatal Errors

Some errors are unrecoverable:
- Invalid configuration
- Missing API tokens
- Resource conflicts

### Graceful Degradation

Non-critical failures don't stop the pipeline:
- Optional addons can fail
- Warnings are logged
- Core cluster remains functional
