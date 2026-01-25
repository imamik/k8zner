# Architecture

This document describes the internal architecture of k8zner.

## Overview

k8zner is a single binary that orchestrates the creation and management of Kubernetes clusters on Hetzner Cloud using Talos Linux.

```
┌─────────────────────────────────────────────────────────────────┐
│                         k8zner CLI                              │
├─────────────────────────────────────────────────────────────────┤
│                      Orchestration Layer                        │
│              (Reconciler, Pipeline, Phases)                     │
├─────────────┬─────────────┬─────────────┬──────────────────────┤
│ Provisioning│   Addons    │   Config    │      Platform        │
│ ├─ infra    │ ├─ cilium   │ ├─ types    │ ├─ hcloud (client)   │
│ ├─ compute  │ ├─ ccm/csi  │ ├─ load     │ ├─ talos (config)    │
│ ├─ image    │ ├─ ingress  │ ├─ wizard   │ └─ ssh (exec)        │
│ └─ cluster  │ └─ helm     │ └─ validate │                      │
└─────────────┴─────────────┴─────────────┴──────────────────────┘
```

## Pipeline Phases

The reconciliation pipeline executes phases in order:

### 1. Validation Phase

Pre-flight checks ensure:
- Configuration is valid
- Required API tokens are present
- Server types and locations exist
- No conflicting resources

### 2. Infrastructure Phase

Creates Hetzner Cloud resources:
- Private network and subnets
- Firewall rules
- Load balancers (if HA enabled)
- Placement groups

### 3. Image Phase

Handles Talos OS images:
- Checks for existing snapshots
- Builds new images if needed
- Supports custom extensions

### 4. Compute Phase

Provisions servers:
- Creates control plane nodes
- Creates worker nodes
- Attaches to private network
- Applies Talos configuration

### 5. Cluster Phase

Bootstraps Kubernetes:
- Initializes etcd cluster
- Waits for control plane ready
- Installs configured addons
- Generates kubeconfig

## Project Structure

```
cmd/
├── k8zner/
│   ├── commands/         # CLI command definitions (Cobra)
│   │   ├── root.go       # Root command and global flags
│   │   ├── init.go       # Interactive wizard command
│   │   ├── apply.go      # Apply cluster configuration
│   │   ├── destroy.go    # Destroy cluster
│   │   └── upgrade.go    # Upgrade Talos/K8s
│   └── handlers/         # Business logic for commands
│
internal/
├── config/               # Configuration handling
│   ├── types.go          # Config struct definitions
│   ├── wizard/           # Interactive configuration wizard
│   │   ├── wizard.go     # Wizard orchestration
│   │   ├── questions.go  # Interactive prompts
│   │   ├── options.go    # Server types, locations, etc.
│   │   ├── builder.go    # Config struct builder
│   │   └── writer.go     # YAML output
│   └── validate.go       # Configuration validation
│
├── orchestration/        # High-level workflow
│   ├── reconciler.go     # Main reconciliation loop
│   └── pipeline.go       # Phase execution
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
User Config (YAML)
       │
       ▼
┌─────────────────┐
│ Config Loader   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   Validator     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   Reconciler    │◄──────────────┐
└────────┬────────┘               │
         │                        │
    ┌────┴────┐              ┌────┴────┐
    ▼         ▼              │ Hetzner │
┌───────┐ ┌───────┐          │  Cloud  │
│ Infra │ │Compute│          │   API   │
└───┬───┘ └───┬───┘          └─────────┘
    │         │
    ▼         ▼
┌─────────────────┐
│    Cluster      │
│   Bootstrap     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│     Addons      │
└────────┬────────┘
         │
         ▼
    kubeconfig
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
