# Interactive Configuration Wizard

The `k8zner init` command provides a simple interactive wizard for creating cluster configurations. It guides you through 5-6 questions to set up an opinionated, production-ready cluster.

## Basic Usage

```bash
k8zner init
```

This creates a `k8zner.yaml` file with your configuration.

## Options

| Flag | Description |
|------|-------------|
| `-o, --output` | Output file path (default: `k8zner.yaml`) |

## Wizard Flow

The wizard asks only the essential questions:

### 1. Cluster Name

Enter a name for your cluster (1-32 lowercase alphanumeric characters or hyphens).

```
? Cluster name: my-cluster
```

### 2. Region

Select your preferred Hetzner datacenter region:

| Region | Code | Location |
|--------|------|----------|
| Falkenstein | fsn1 | Germany |
| Nuremberg | nbg1 | Germany |
| Helsinki | hel1 | Finland |

```
? Region: [Use arrows to move, type to filter]
> fsn1 (Falkenstein, Germany)
  nbg1 (Nuremberg, Germany)
  hel1 (Helsinki, Finland)
```

### 3. Mode

Choose between development and production modes:

| Mode | Control Planes | Load Balancers | Best For |
|------|----------------|----------------|----------|
| **dev** | 1 | 1 (shared) | Development, testing |
| **ha** | 3 | 2 (dedicated) | Production |

```
? Mode: [Use arrows to move, type to filter]
> dev (1 control plane, 1 shared LB)
  ha (3 control planes, 2 separate LBs)
```

### 4. Worker Count

Enter the number of worker nodes (1-10):

```
? Worker count: 3
```

### 5. Worker Size

Select worker node size:

**CX Series - Dedicated vCPU (Default)**
| Size | vCPU | RAM | Best For |
|------|------|-----|----------|
| cx23 | 2 | 4GB | Small workloads |
| cx33 | 4 | 8GB | Medium workloads |
| cx43 | 8 | 16GB | Larger workloads |
| cx53 | 16 | 32GB | Heavy workloads |

**CPX Series - Shared vCPU**
| Size | vCPU | RAM | Best For |
|------|------|-----|----------|
| cpx22 | 2 | 4GB | Dev/test |
| cpx32 | 4 | 8GB | Medium workloads |
| cpx42 | 8 | 16GB | Larger workloads |
| cpx52 | 16 | 32GB | Heavy workloads |

```
? Worker size: [Use arrows to move, type to filter]
  cx23 (2 vCPU, 4GB RAM)
> cx33 (4 vCPU, 8GB RAM)
  cx43 (8 vCPU, 16GB RAM)
  cpx32 (4 vCPU, 8GB RAM, shared)
```

### 6. Domain (Optional)

If you have a domain managed by Cloudflare, enter it to enable:
- Automatic DNS records via external-dns
- TLS certificates via cert-manager

```
? Domain (optional): example.com
```

Leave empty if you don't need DNS/TLS integration.

## Output

The wizard generates a minimal configuration file:

```yaml
name: my-cluster
region: fsn1
mode: ha
workers:
  count: 3
  size: cx33
domain: example.com
```

## Opinionated Defaults

The simplified config automatically includes:

### Infrastructure
- IPv6-only nodes (saves IPv4 costs, better security)
- Dedicated vCPU control plane servers (cx23 by default)
- Hetzner Cloud integration (CCM, CSI)

### Networking
- Cilium CNI with kube-proxy replacement
- Tunnel mode (VXLAN) for reliable pod-to-pod communication
- Hubble for network observability

### Ingress
- Traefik as ingress controller
- Automatic TLS with cert-manager
- Gateway API support

### Observability
- Metrics Server for HPA/VPA
- Hubble UI for network visualization

### GitOps
- ArgoCD for continuous deployment
- HA mode in production clusters

## Examples

### Development Cluster

```bash
k8zner init -o dev.yaml
# Select: dev mode, 1 worker, cx23
```

### Production Cluster

```bash
k8zner init -o production.yaml
# Select: ha mode, 3 workers, cx33, add domain
```

## Next Steps

After generating your config:

1. Set your Hetzner API token:
   ```bash
   export HCLOUD_TOKEN="your-token"
   ```

2. If using a domain, set Cloudflare token:
   ```bash
   export CF_API_TOKEN="your-cf-token"
   ```

3. Create the cluster:
   ```bash
   k8zner apply
   ```

## Tips

1. **Start with dev mode** for testing, upgrade to HA for production
2. **Use a domain** for automatic TLS certificates and DNS
3. **Review your config** in `k8zner.yaml` before applying
4. **IPv6 saves money** - all nodes use IPv6-only by default
