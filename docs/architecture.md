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

## Project Structure

```
cmd/
├── k8zner/
│   ├── commands/         # CLI command definitions (Cobra)
│   │   ├── root.go       # Root command and global flags
│   │   ├── init.go       # Interactive wizard
│   │   ├── apply.go      # Create or update cluster
│   │   ├── destroy.go    # Tear down cluster
│   │   └── doctor.go     # Cluster diagnostics
│   └── handlers/         # Business logic for commands
│
internal/
├── config/               # Configuration handling
│   ├── types.go          # Internal config struct
│   ├── v2/              # Simplified config (k8zner.yaml)
│   │   ├── config.go     # V2 config types
│   │   ├── expand.go     # V2 → internal config expansion
│   │   └── defaults.go   # Version matrix, constants
│   ├── wizard/           # Interactive configuration wizard
│   └── validate.go       # Configuration validation
│
├── operator/             # Kubernetes operator
│   ├── controller/       # Reconciler logic
│   ├── adapter.go        # Bridges operator ↔ provisioning
│   └── installer.go      # Operator deployment into cluster
│
├── provisioning/         # Infrastructure provisioning
│   ├── infrastructure/   # Network, firewall, LB
│   ├── compute/          # Server creation
│   ├── image/            # Talos image building
│   └── cluster/          # Bootstrap and config
│
├── addons/               # Kubernetes addon management
│   ├── k8sclient/        # Embedded Kubernetes client
│   └── helm/             # Helm chart rendering
│
├── platform/             # External integrations
│   ├── hcloud/           # Hetzner Cloud API client
│   ├── talos/            # Talos configuration
│   └── ssh/              # SSH command execution
│
└── util/                 # Shared utilities
    ├── retry/            # Retry with backoff
    ├── naming/           # Resource naming
    └── labels/           # Label management
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
