# k8zner

[![CI](https://github.com/imamik/k8zner/actions/workflows/ci.yaml/badge.svg)](https://github.com/imamik/k8zner/actions/workflows/ci.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/imamik/k8zner)](https://goreportcard.com/report/github.com/imamik/k8zner)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

**k8zner** (k8s + Hetzner) — Production-ready Kubernetes clusters on Hetzner Cloud using [Talos Linux](https://www.talos.dev/).

## Features

- **Declarative Configuration** — Define your cluster in YAML, apply with a single command
- **Talos Linux** — Immutable, secure, API-managed Kubernetes OS
- **High Availability** — Multi-node control planes with automatic failover
- **Auto-scaling** — Cluster Autoscaler integration for dynamic worker pools
- **Full Addon Suite** — Cilium CNI, Hetzner CCM/CSI, cert-manager, ingress-nginx, and more
- **Snapshot-based Provisioning** — Fast node creation from pre-built Talos images
- **Idempotent Operations** — Safe to run repeatedly, only applies necessary changes
- **Self-Contained Binary** — No runtime dependencies (kubectl, talosctl not required)

## Quick Start

### Prerequisites

- [Hetzner Cloud account](https://www.hetzner.com/cloud) with API token

Optional (for manual debugging only):
- [kubectl](https://kubernetes.io/docs/tasks/tools/) — for ad-hoc cluster operations
- [talosctl](https://www.talos.dev/latest/introduction/getting-started/#talosctl) — for Talos-specific debugging

### Installation

Download the latest release from the [releases page](https://github.com/imamik/k8zner/releases), or build from source:

```bash
git clone https://github.com/imamik/k8zner.git
cd k8zner
make build
```

### Create a Cluster

1. **Set your Hetzner Cloud API token:**

```bash
export HCLOUD_TOKEN="your-api-token"
```

2. **Create a cluster configuration** (`cluster.yaml`):

```yaml
cluster_name: "my-cluster"
location: "nbg1"

ssh_keys:
  - "my-ssh-key"

control_plane:
  count: 3
  server_type: "cpx21"

worker_pools:
  - name: "default"
    count: 2
    server_type: "cpx31"

talos:
  version: "v1.9.0"
  k8s_version: "1.32.0"

addons:
  ccm:
    enabled: true
  csi:
    enabled: true
  cilium:
    enabled: true
```

3. **Build Talos image snapshot** (one-time):

```bash
./bin/k8zner image build --config cluster.yaml
```

4. **Apply the cluster:**

```bash
./bin/k8zner apply --config cluster.yaml
```

5. **Access your cluster:**

```bash
export KUBECONFIG=./secrets/my-cluster/kubeconfig
kubectl get nodes
```

## Commands

| Command | Description |
|---------|-------------|
| `k8zner apply` | Create or update cluster infrastructure and configuration |
| `k8zner destroy` | Tear down all cluster resources |
| `k8zner upgrade` | Upgrade Talos and/or Kubernetes versions |
| `k8zner image build` | Build Talos image snapshot for faster provisioning |
| `k8zner image delete` | Delete Talos image snapshots |

## Configuration Reference

See the [`examples/`](examples/) directory for configuration examples:
- [`cluster-config-minimal.yaml`](examples/cluster-config-minimal.yaml) — Simplest working configuration
- [`cluster-config.yaml`](examples/cluster-config.yaml) — Common options with explanations
- [`cluster-config-full.yaml`](examples/cluster-config-full.yaml) — All available options

### Core Settings

| Field | Description | Required |
|-------|-------------|----------|
| `cluster_name` | Unique cluster identifier | Yes |
| `location` | Hetzner datacenter (nbg1, fsn1, hel1, ash) | Yes |
| `ssh_keys` | SSH key names for server access | Yes |

### Control Plane

| Field | Description | Default |
|-------|-------------|---------|
| `control_plane.count` | Number of control plane nodes | 1 |
| `control_plane.server_type` | Hetzner server type | cpx21 |

### Worker Pools

```yaml
worker_pools:
  - name: "compute"
    count: 3
    server_type: "cpx41"
    labels:
      node-type: compute
    taints:
      - key: workload
        value: compute
        effect: NoSchedule
```

### Addons

| Addon | Description | Default |
|-------|-------------|---------|
| `ccm` | Hetzner Cloud Controller Manager | enabled |
| `csi` | Hetzner CSI Driver for volumes | enabled |
| `cilium` | CNI with network policies | enabled |
| `ingress_nginx` | Ingress controller | disabled |
| `cert_manager` | TLS certificate management | disabled |
| `cluster_autoscaler` | Auto-scaling worker pools | disabled |
| `metrics_server` | Kubernetes metrics | disabled |

## Architecture

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
│ ├─ image    │ ├─ ingress  │ └─ validate │ └─ ssh (exec)        │
│ └─ cluster  │ └─ helm     │             │                      │
└─────────────┴─────────────┴─────────────┴──────────────────────┘
```

### Pipeline Phases

1. **Validation** — Pre-flight checks for configuration
2. **Infrastructure** — Network, firewall, load balancer setup
3. **Image** — Talos snapshot creation (if needed)
4. **Compute** — Server provisioning with Talos config
5. **Cluster** — Bootstrap and addon installation

## Development

### Prerequisites

- Go 1.21+
- golangci-lint

### Build

```bash
make build      # Build binary
make test       # Run unit tests
make lint       # Run linters
make check      # Run all checks (fmt, lint, test, build)
```

### End-to-End Tests

```bash
export HCLOUD_TOKEN="your-api-token"
make e2e        # Full test suite
make e2e-fast   # Faster iteration (keeps snapshots)
```

### Project Structure

```
cmd/
├── k8zner/
│   ├── commands/     # CLI command definitions (Cobra)
│   └── handlers/     # Business logic for commands
internal/
├── config/           # Configuration types and validation
├── orchestration/    # High-level workflow coordination
├── provisioning/     # Infrastructure provisioning
│   ├── infrastructure/
│   ├── compute/
│   ├── image/
│   └── cluster/
├── addons/           # Kubernetes addon management
│   ├── k8sclient/    # Kubernetes client (replaces kubectl)
│   └── helm/         # Helm chart rendering
├── platform/         # External system integrations
│   ├── hcloud/       # Hetzner Cloud client
│   ├── talos/        # Talos configuration
│   └── ssh/          # SSH command execution
└── util/             # Shared utilities
tests/
└── e2e/              # End-to-end tests
```

## Contributing

Contributions are welcome! Please read our [Contributing Guidelines](CONTRIBUTING.md) before submitting a pull request.

## Security

For security issues, please see our [Security Policy](SECURITY.md).

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- [Talos Linux](https://www.talos.dev/) — The secure, immutable Kubernetes OS
- [Hetzner Cloud](https://www.hetzner.com/cloud) — Affordable, high-performance cloud infrastructure
- [Cilium](https://cilium.io/) — eBPF-based networking and security
