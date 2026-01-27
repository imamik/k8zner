# Configuration Guide

This document describes all configuration options for k8zner cluster definitions.

## Quick Start

The easiest way to create a configuration is using the interactive wizard:

```bash
k8zner init
```

See [Interactive Wizard](wizard.md) for detailed wizard documentation.

## Configuration File

k8zner uses YAML configuration files. By default, it looks for `cluster.yaml` in the current directory.

```bash
k8zner apply -c cluster.yaml
```

## Core Settings

### Required Fields

```yaml
cluster_name: "my-cluster"    # 1-32 lowercase alphanumeric with hyphens
location: "nbg1"              # Hetzner datacenter
```

### Available Locations

| Location | Description |
|----------|-------------|
| `nbg1` | Nuremberg, Germany |
| `fsn1` | Falkenstein, Germany |
| `hel1` | Helsinki, Finland |
| `ash` | Ashburn, USA |
| `hil` | Hillsboro, USA |
| `sin` | Singapore |

### SSH Keys (Optional)

```yaml
ssh_keys:
  - "my-key"
  - "another-key"
```

If omitted, SSH keys are auto-generated during cluster creation.

## Control Plane

```yaml
control_plane:
  nodepools:
    - name: "control-plane"
      type: "cpx21"           # Server type
      count: 3                # 1, 3, or 5 for etcd quorum
```

### Recommended Control Plane Sizes

| Cluster Size | Count | Server Type |
|--------------|-------|-------------|
| Development | 1 | cpx21 |
| Small Production | 3 | cpx31 |
| Large Production | 3-5 | cpx41+ |

## Worker Pools

```yaml
workers:
  - name: "workers"
    type: "cpx31"
    count: 3
    labels:
      node-role: worker
    taints:
      - "workload=general:NoSchedule"
```

### Worker Pool Options

| Field | Description |
|-------|-------------|
| `name` | Pool identifier |
| `type` | Hetzner server type |
| `count` | Number of nodes |
| `labels` | Kubernetes node labels |
| `taints` | Kubernetes node taints |
| `location` | Override cluster location |

## Server Types

### x86 Shared (CPX)

| Type | vCPU | RAM | SSD |
|------|------|-----|-----|
| cpx11 | 2 | 2GB | 40GB |
| cpx21 | 3 | 4GB | 80GB |
| cpx31 | 4 | 8GB | 160GB |
| cpx41 | 8 | 16GB | 240GB |
| cpx51 | 16 | 32GB | 360GB |

### x86 Cost-Optimized (CX) — EU Only

| Type | vCPU | RAM | SSD |
|------|------|-----|-----|
| cx22 | 2 | 4GB | 40GB |
| cx32 | 4 | 8GB | 80GB |
| cx42 | 8 | 16GB | 160GB |
| cx52 | 16 | 32GB | 320GB |

### x86 Dedicated (CCX)

| Type | vCPU | RAM | SSD |
|------|------|-----|-----|
| ccx13 | 2 | 8GB | 80GB |
| ccx23 | 4 | 16GB | 160GB |
| ccx33 | 8 | 32GB | 240GB |
| ccx43 | 16 | 64GB | 360GB |
| ccx53 | 32 | 128GB | 600GB |
| ccx63 | 48 | 192GB | 960GB |

### ARM Shared (CAX) — Germany/Finland Only

| Type | vCPU | RAM | SSD |
|------|------|-----|-----|
| cax11 | 2 | 4GB | 40GB |
| cax21 | 4 | 8GB | 80GB |
| cax31 | 8 | 16GB | 160GB |
| cax41 | 16 | 32GB | 320GB |

## Talos Configuration

```yaml
talos:
  version: "v1.9.0"
  machine:
    state_encryption: true      # LUKS2 encryption
    ephemeral_encryption: true
```

## Kubernetes Configuration

```yaml
kubernetes:
  version: "v1.32.0"
  allow_scheduling_on_control_planes: false
  domain: "cluster.local"
```

## Addons

### Cilium (CNI)

```yaml
addons:
  cilium:
    enabled: true
    encryption_enabled: true
    encryption_type: "wireguard"  # or "ipsec"
    hubble_enabled: true
    hubble_relay_enabled: true
    gateway_api_enabled: false
```

### Hetzner Cloud Controller Manager

```yaml
addons:
  ccm:
    enabled: true
```

### Hetzner CSI Driver

```yaml
addons:
  csi:
    enabled: true
    default_storage_class: true
```

### Metrics Server

```yaml
addons:
  metrics_server:
    enabled: true
```

### Cert Manager

```yaml
addons:
  cert_manager:
    enabled: true
```

### Ingress NGINX

```yaml
addons:
  ingress_nginx:
    enabled: true
    replicas: 2
```

### Traefik

Alternative ingress controller to NGINX:

```yaml
addons:
  traefik:
    enabled: true
    replicas: 2
```

### ArgoCD

GitOps continuous delivery:

```yaml
addons:
  argocd:
    enabled: true
    ha: false
    ingress_enabled: true
    ingress_host: "argocd.example.com"
```

## Cloudflare DNS Integration

k8zner integrates with Cloudflare for automatic DNS management and TLS certificate provisioning.

### Prerequisites

1. A Cloudflare account with a domain
2. A Cloudflare API token with permissions:
   - **Zone > Zone > Read** (to find zone ID automatically)
   - **Zone > DNS > Edit** (to manage DNS records)

To create a token:
1. Go to [Cloudflare API Tokens](https://dash.cloudflare.com/profile/api-tokens)
2. Click "Create Token"
3. Use "Edit zone DNS" template or create custom token
4. Set Zone Resources to your specific domain (recommended) or all zones

### Environment Variables

Set your Cloudflare credentials (recommended over config file):

```bash
export CF_API_TOKEN="your-cloudflare-api-token"
export CF_DOMAIN="example.com"
```

### Cloudflare Base Configuration

```yaml
addons:
  cloudflare:
    enabled: true
    # api_token: "..."    # Or use CF_API_TOKEN env var (preferred)
    domain: "example.com" # Or use CF_DOMAIN env var
    proxied: false        # true = orange cloud (CDN), false = DNS only
```

### External-DNS

Automatically creates DNS records from Ingress resources:

```yaml
addons:
  cloudflare:
    enabled: true
    domain: "example.com"

  external_dns:
    enabled: true
    txt_owner_id: "my-cluster"  # Default: cluster_name
    policy: "sync"              # sync (deletes orphans), upsert-only (never deletes)
    sources:
      - ingress               # Watch Ingress resources
      # - service             # Watch LoadBalancer Services
```

When you create an Ingress, external-dns will automatically create DNS records:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-app
  annotations:
    external-dns.alpha.kubernetes.io/hostname: my-app.example.com
spec:
  rules:
    - host: my-app.example.com
      # ...
```

### Cert-Manager with Cloudflare DNS01

For TLS certificates via Let's Encrypt DNS01 challenge (supports wildcards):

```yaml
addons:
  cloudflare:
    enabled: true
    domain: "example.com"

  cert_manager:
    enabled: true
    cloudflare:
      enabled: true
      email: "admin@example.com"  # Required for Let's Encrypt
      production: false           # Use staging first to test
```

This creates ClusterIssuers:
- `letsencrypt-cloudflare-staging` — For testing (not browser-trusted)
- `letsencrypt-cloudflare-production` — For production (browser-trusted)

Use in Ingress:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-app
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-cloudflare-production
spec:
  tls:
    - hosts:
        - my-app.example.com
      secretName: my-app-tls
  rules:
    - host: my-app.example.com
      # ...
```

### Full Cloudflare Example

Complete configuration with DNS and TLS:

```yaml
addons:
  # Ingress controller (choose one)
  ingress_nginx:
    enabled: true

  # Cloudflare integration
  cloudflare:
    enabled: true
    domain: "example.com"
    proxied: false

  # Automatic DNS records
  external_dns:
    enabled: true
    policy: "sync"
    sources:
      - ingress

  # TLS certificates
  cert_manager:
    enabled: true
    cloudflare:
      enabled: true
      email: "admin@example.com"
      production: true
```

### Longhorn

```yaml
addons:
  longhorn:
    enabled: true
    default_storage_class: false
```

## Network Configuration

```yaml
network:
  ipv4_cidr: "10.0.0.0/16"
  pod_ipv4_cidr: "10.244.0.0/16"
  service_ipv4_cidr: "10.96.0.0/12"
```

## Cluster Access

```yaml
cluster_access: "public"  # or "private"
```

## Example Configurations

### Minimal Development

```yaml
cluster_name: dev
location: nbg1
control_plane:
  nodepools:
    - name: control-plane
      type: cpx21
      count: 1
kubernetes:
  allow_scheduling_on_control_planes: true
talos:
  version: v1.9.0
kubernetes:
  version: v1.32.0
addons:
  cilium:
    enabled: true
```

### Production HA

```yaml
cluster_name: production
location: fsn1
control_plane:
  nodepools:
    - name: control-plane
      type: cpx31
      count: 3
workers:
  - name: workers
    type: cpx41
    count: 5
talos:
  version: v1.9.0
  machine:
    state_encryption: true
    ephemeral_encryption: true
kubernetes:
  version: v1.32.0
addons:
  cilium:
    enabled: true
    encryption_enabled: true
    encryption_type: wireguard
    hubble_enabled: true
  ccm:
    enabled: true
  csi:
    enabled: true
  metrics_server:
    enabled: true
  cert_manager:
    enabled: true
  ingress_nginx:
    enabled: true
```

### ARM Cost-Effective

```yaml
cluster_name: arm-cluster
location: nbg1
control_plane:
  nodepools:
    - name: control-plane
      type: cax21
      count: 3
workers:
  - name: workers
    type: cax31
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
  csi:
    enabled: true
```

## See Also

- [Interactive Wizard](wizard.md) — Create configurations interactively
- [Examples](../examples/) — More configuration examples
