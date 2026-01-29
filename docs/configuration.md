# Configuration Guide

This document describes the simplified configuration format for k8zner clusters.

## Quick Start

Create a configuration using the interactive wizard:

```bash
k8zner init
```

This generates a `k8zner.yaml` file. See [Interactive Wizard](wizard.md) for details.

## Configuration File

k8zner uses YAML configuration with an opinionated, minimal format:

```yaml
name: my-cluster
region: fsn1
mode: ha
workers:
  count: 3
  size: cx32
domain: example.com  # optional
```

That's it! All other settings use production-ready defaults.

## Configuration Fields

### name (required)

Cluster name. Must be 1-32 lowercase alphanumeric characters or hyphens.

```yaml
name: my-cluster
```

### region (required)

Hetzner datacenter region for all resources.

| Region | Code | Location |
|--------|------|----------|
| Falkenstein | `fsn1` | Germany |
| Nuremberg | `nbg1` | Germany |
| Helsinki | `hel1` | Finland |

```yaml
region: fsn1
```

### mode (required)

Cluster mode determines the topology:

| Mode | Control Planes | Load Balancers | Best For |
|------|----------------|----------------|----------|
| `dev` | 1 | 1 (shared API/ingress) | Development, testing |
| `ha` | 3 | 2 (dedicated API + ingress) | Production |

```yaml
mode: ha
```

### workers (required)

Worker node configuration:

| Field | Description | Valid Values |
|-------|-------------|--------------|
| `count` | Number of worker nodes | 1-5 |
| `size` | Worker server size | cx22, cx32, cx42, cx52 |

```yaml
workers:
  count: 3
  size: cx32
```

#### Available Sizes

| Size | vCPU | RAM | Best For |
|------|------|-----|----------|
| `cx22` | 2 | 4GB | Small workloads |
| `cx32` | 4 | 8GB | Medium workloads |
| `cx42` | 8 | 16GB | Larger workloads |
| `cx52` | 16 | 32GB | Heavy workloads |

### domain (optional)

Cloudflare-managed domain for automatic DNS and TLS certificates.

```yaml
domain: example.com
```

When set, this automatically enables:
- **external-dns**: Creates DNS records from Ingress resources
- **cert-manager Cloudflare DNS01**: Issues Let's Encrypt certificates

Requires `CF_API_TOKEN` environment variable.

## Opinionated Defaults

The simplified config automatically includes production-ready settings:

### Infrastructure
- **IPv6-only nodes**: No public IPv4 (saves costs, better security)
- **Control planes**: cx22 servers (cost-effective for control plane workloads)
- **Disk encryption**: LUKS2 encryption for state and ephemeral partitions

### Networking
- **Cilium CNI**: eBPF-based networking with kube-proxy replacement
- **Native routing**: Direct pod-to-pod communication without tunneling
- **Hubble**: Network observability and monitoring

### Ingress
- **Traefik**: Modern ingress controller with Gateway API support
- **hostNetwork mode**: Direct port binding for efficiency
- **cert-manager**: Automatic TLS certificate provisioning

### Load Balancers
- **API LB**: Always provisioned for Kubernetes API access
- **Ingress LB**: Additional LB in HA mode for HTTP/HTTPS traffic

### Addons (always enabled)
- Hetzner Cloud Controller Manager (CCM)
- Hetzner CSI Driver (volumes)
- Cilium (CNI)
- Traefik (ingress)
- cert-manager (TLS)
- Metrics Server (HPA/VPA)
- ArgoCD (GitOps)
- Gateway API CRDs
- Prometheus Operator CRDs

### Versions
The config uses a pinned, tested version matrix:
- Talos Linux: v1.9.0
- Kubernetes: v1.32.0

## Environment Variables

Required:
```bash
export HCLOUD_TOKEN="your-hetzner-api-token"
```

Optional (for DNS/TLS):
```bash
export CF_API_TOKEN="your-cloudflare-api-token"
```

## Cost Estimation

Before applying, estimate your monthly costs:

```bash
k8zner cost
```

Output shows:
- Line-item breakdown (control planes, workers, load balancers)
- Subtotal, VAT (19% for Germany), and total
- IPv6 savings (what you save by not using IPv4)

## Example Configurations

### Development Cluster

Minimal cluster for testing:

```yaml
name: dev
region: fsn1
mode: dev
workers:
  count: 1
  size: cx22
```

**Cost**: ~€18/month (1 CP + 1 worker + 1 LB)

### Production HA Cluster

High-availability setup for production:

```yaml
name: production
region: fsn1
mode: ha
workers:
  count: 3
  size: cx32
domain: example.com
```

**Cost**: ~€64/month (3 CP + 3 workers + 2 LBs)

### Large Production Cluster

For heavy workloads:

```yaml
name: large-prod
region: fsn1
mode: ha
workers:
  count: 5
  size: cx52
domain: example.com
```

**Cost**: ~€180/month (3 CP + 5 workers + 2 LBs)

## File Location

k8zner auto-detects configuration in the current directory:

```bash
# Uses ./k8zner.yaml automatically
k8zner apply

# Or specify explicitly
k8zner apply -c /path/to/config.yaml
```

## See Also

- [Interactive Wizard](wizard.md)
- [Architecture](architecture.md)
