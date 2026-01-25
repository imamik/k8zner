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
