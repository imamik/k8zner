# k8zner

[![CI](https://github.com/imamik/k8zner/actions/workflows/ci.yaml/badge.svg)](https://github.com/imamik/k8zner/actions/workflows/ci.yaml)
[![codecov](https://codecov.io/gh/imamik/k8zner/branch/main/graph/badge.svg)](https://codecov.io/gh/imamik/k8zner)
[![Go Report Card](https://goreportcard.com/badge/github.com/imamik/k8zner)](https://goreportcard.com/report/github.com/imamik/k8zner)
[![Release](https://img.shields.io/github/v/release/imamik/k8zner)](https://github.com/imamik/k8zner/releases/latest)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

**k8zner** (k8s + Hetzner) — Production-ready Kubernetes clusters on Hetzner Cloud using [Talos Linux](https://www.talos.dev/).

## Features

- **Interactive Setup Wizard** — Create cluster configurations with guided prompts
- **Declarative Configuration** — Define your cluster in YAML, apply with a single command
- **Talos Linux** — Immutable, secure, API-managed Kubernetes OS
- **High Availability** — Multi-node control planes with automatic failover
- **Auto-scaling** — Cluster Autoscaler integration for dynamic worker pools
- **Full Addon Suite** — Cilium CNI, Hetzner CCM/CSI, cert-manager, ingress-nginx, and more
- **Snapshot-based Provisioning** — Fast node creation from pre-built Talos images
- **Self-Contained Binary** — No runtime dependencies (kubectl, talosctl not required)

## Quick Start

### 1. Install k8zner

**Homebrew (macOS/Linux):**
```bash
brew tap imamik/tap
brew install k8zner
```

**Go install:**
```bash
go install github.com/imamik/k8zner/cmd/k8zner@latest
```

**Download binary:** See [releases page](https://github.com/imamik/k8zner/releases)

### 2. Set your Hetzner Cloud API token

```bash
export HCLOUD_TOKEN="your-api-token"
```

### 3. Create cluster configuration

**Option A: Interactive Wizard (Recommended)**

```bash
k8zner init
```

The wizard guides you through:
- Cluster name and location
- Server architecture (x86 or ARM)
- Server category (shared, dedicated, or cost-optimized)
- Control plane and worker configuration
- CNI selection (Cilium, Talos default, or bring your own)
- Cluster addons

Use `--advanced` for network, security, and Cilium customization options.

**Option B: Manual Configuration**

Create `cluster.yaml` manually — see [Configuration Guide](docs/configuration.md) for all options.

### 4. Build Talos image (one-time)

```bash
k8zner image build -c cluster.yaml
```

### 5. Deploy your cluster

```bash
k8zner apply -c cluster.yaml
```

### 6. Access your cluster

```bash
export KUBECONFIG=./secrets/my-cluster/kubeconfig
kubectl get nodes
```

## Commands

| Command | Description |
|---------|-------------|
| `k8zner init` | Interactive wizard to create cluster configuration |
| `k8zner apply` | Create or update cluster infrastructure |
| `k8zner destroy` | Tear down all cluster resources |
| `k8zner upgrade` | Upgrade Talos and/or Kubernetes versions |
| `k8zner image build` | Build Talos image snapshot |
| `k8zner image delete` | Delete Talos image snapshots |

## Documentation

- [Configuration Guide](docs/configuration.md) — Full configuration reference
- [Interactive Wizard](docs/wizard.md) — Detailed wizard documentation
- [Examples](examples/) — Sample configurations for common scenarios
- [Architecture](docs/architecture.md) — How k8zner works

## Hetzner Server Types

k8zner supports all Hetzner Cloud server families:

| Family | Type | Description | Regions |
|--------|------|-------------|---------|
| **CX** | Shared | Cost-optimized (Intel/AMD mix) | EU only |
| **CPX** | Shared | AMD Genoa processors | All |
| **CCX** | Dedicated | Dedicated AMD EPYC vCPUs | All |
| **CAX** | Shared | ARM (Ampere Altra) | Germany, Finland |

The wizard helps you choose the right server type based on architecture and performance needs.

## Development

```bash
make build      # Build binary
make test       # Run unit tests
make lint       # Run linters
make check      # Run all checks
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for development guidelines.

## License

Apache License 2.0 - see [LICENSE](LICENSE)

## Acknowledgments

- [Talos Linux](https://www.talos.dev/) — Secure, immutable Kubernetes OS
- [Hetzner Cloud](https://www.hetzner.com/cloud) — Affordable cloud infrastructure
- [Cilium](https://cilium.io/) — eBPF-based networking
