# Interactive Configuration Wizard

The `k8zner init` command provides an interactive wizard for creating cluster configurations. It guides you through all the options with sensible defaults and helpful descriptions.

## Basic Usage

```bash
k8zner init
```

This creates a `cluster.yaml` file with your configuration.

## Options

| Flag | Description |
|------|-------------|
| `-o, --output` | Output file path (default: `cluster.yaml`) |
| `-a, --advanced` | Show advanced configuration options |
| `-f, --full` | Output complete YAML with all options |

## Wizard Flow

### 1. Cluster Identity

- **Cluster Name**: 1-32 lowercase alphanumeric characters or hyphens
- **Location**: Hetzner datacenter (nbg1, fsn1, hel1, ash, hil, sin)

### 2. SSH Access (Optional)

Enter comma-separated SSH key names from your Hetzner Cloud account.

Leave empty to have SSH keys auto-generated during cluster creation.

### 3. Server Architecture

Choose your CPU architecture:

- **x86 (AMD/Intel)** — Wider software compatibility
- **ARM (Ampere Altra)** — Better price/performance ratio

### 4. Server Category (x86 only)

For x86 architecture, choose the server category:

| Category | Series | Description |
|----------|--------|-------------|
| **Shared vCPU** | CPX | AMD Genoa processors, good for most workloads |
| **Cost-Optimized** | CX | Budget-friendly, EU regions only |
| **Dedicated vCPU** | CCX | Guaranteed performance, no noisy neighbors |

ARM servers (CAX) are always shared vCPU.

### 5. Control Plane Configuration

- **Server Type**: Filtered based on your architecture/category selection
- **Node Count**: 1 (dev), 3 (recommended for HA), or 5 (large clusters)

### 6. Worker Nodes

Choose whether to add dedicated worker nodes:

- **Yes**: Configure worker server type and count
- **No**: Workloads run on control plane nodes

### 7. CNI Selection

Choose your Container Network Interface:

| Option | Description |
|--------|-------------|
| **Cilium** (Recommended) | Advanced networking with eBPF, network policies |
| **Talos Default (Flannel)** | Simple, built-in networking |
| **None** | Bring your own CNI |

### 8. Cluster Addons

Multi-select additional components:

| Addon | Description | Default |
|-------|-------------|---------|
| Hetzner CCM | Cloud Controller Manager for load balancers | On |
| Hetzner CSI | Container Storage Interface for volumes | On |
| Metrics Server | Resource metrics for HPA/VPA | On |
| Cert Manager | Automatic TLS certificate management | Off |
| Ingress NGINX | HTTP/HTTPS ingress controller | Off |
| Longhorn | Distributed block storage | Off |

### 9. Versions

- **Talos Version**: v1.9.0 (latest), v1.8.3
- **Kubernetes Version**: v1.32.0 (latest), v1.31.0

## Advanced Mode

Use `--advanced` flag for additional configuration:

```bash
k8zner init --advanced
```

### Network Configuration

- **Network CIDR**: Private network range (default: 10.0.0.0/16)
- **Pod CIDR**: IP range for pods (default: 10.244.0.0/16)
- **Service CIDR**: IP range for services (default: 10.96.0.0/12)

### Security Options

- **Disk Encryption**: Encrypt node disks with LUKS2
- **Cluster Access Mode**: Public or private API access

### Cilium Options (if Cilium selected)

- **Encryption**: Enable pod-to-pod encryption
- **Encryption Type**: WireGuard (recommended) or IPsec
- **Hubble**: Network observability and monitoring
- **Gateway API**: Kubernetes Gateway API support

## Output Modes

### Minimal Output (Default)

Only essential, non-default values are written:

```yaml
cluster_name: my-cluster
location: nbg1
control_plane:
  nodepools:
    - name: control-plane
      type: cpx21
      count: 3
talos:
  version: v1.9.0
kubernetes:
  version: v1.32.0
addons:
  cilium:
    enabled: true
  ccm:
    enabled: true
```

### Full Output

Use `--full` to include all configuration options:

```bash
k8zner init --full
```

This generates a complete YAML with all fields, useful for:
- Understanding all available options
- Manual customization
- Documentation purposes

## Examples

### Quick Development Cluster

```bash
k8zner init -o dev-cluster.yaml
# Accept defaults, single control plane, no workers
```

### Production HA Cluster

```bash
k8zner init --advanced -o prod-cluster.yaml
# 3 control plane nodes, workers, encryption enabled
```

### ARM-based Cost-Effective Cluster

```bash
k8zner init -o arm-cluster.yaml
# Select ARM architecture when prompted
# Great for ARM-compatible workloads
```

## Tips

1. **Start simple**: Use basic mode first, add advanced options later
2. **SSH keys optional**: Leave empty for auto-generation
3. **ARM for savings**: CAX servers offer excellent price/performance
4. **Dedicated for production**: CCX servers guarantee consistent performance
5. **Minimal YAML**: Default output is clean and easy to version control
