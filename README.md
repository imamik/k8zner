# hcloud-k8s

![Build Status](https://github.com/sak-d/hcloud-k8s/actions/workflows/ci.yaml/badge.svg)
![Go Report Card](https://goreportcard.com/badge/github.com/sak-d/hcloud-k8s)

`hcloud-k8s` is a CLI tool designed to replace Terraform and Packer for provisioning Kubernetes on Hetzner Cloud using Talos Linux. It provides a stateless, label-based reconciliation strategy for managing infrastructure and generating node configurations.

## Features

- **Image Builder:** Create custom Talos snapshots on Hetzner Cloud.
- **Cluster Reconciliation:** Provision and manage Control Plane servers (idempotent).
- **Talos Config Generation:** Automatically generates secure Talos machine configurations and client secrets.
- **Cluster Upgrades:** Safe, rolling upgrades of Talos OS and Kubernetes with automatic health checks.
- **Persistence:** Saves cluster secrets and `talosconfig` locally for ongoing management.
- **Privacy:** Enforces SSH key usage to prevent root password transmission via email.
- **Reverse DNS (RDNS):** Automatic configuration of PTR records for servers and load balancers with template support.

## Quick Start

### 1. Build the Binary

```bash
git clone https://github.com/sak-d/hcloud-k8s.git
cd hcloud-k8s
go build -o hcloud-k8s ./cmd/hcloud-k8s
```

### 2. Build a Talos Image

Create a custom snapshot for your cluster nodes.

```bash
export HCLOUD_TOKEN="your-token"
./hcloud-k8s image build --name "talos-v1.9.0" --version "v1.9.0" --arch "amd64"
```

### 3. Create a Cluster Configuration

Create a `config.yaml` file:

```yaml
cluster_name: "my-talos-cluster"
location: "nbg1" # nbg1, fsn1, hel1, ash, etc.
ssh_keys:
  - "my-ssh-key-name" # Must exist in Hetzner Cloud

control_plane:
  count: 3
  server_type: "cpx22" # cpx22 for AMD64, cax11 for ARM
  image: "talos-v1.9.0" # Matches the image name built above
  endpoint: "https://<LOAD_BALANCER_IP>:6443" # VIP or LB IP

talos:
  version: "v1.9.0"
  k8s_version: "1.32.0"
```

### 4. Apply Configuration

Provision the servers and generate configs.

```bash
./hcloud-k8s apply --config config.yaml
```

This will:
1.  Verify/Create the SSH keys and resources.
2.  Provision 3 Control Plane servers.
3.  Generate `secrets.yaml` (CA keys) and `talosconfig` in the current directory.

### 5. Upgrade Cluster (Optional)

Upgrade Talos OS and Kubernetes versions safely with rolling upgrades.

```bash
# Update versions in config.yaml
vim config.yaml

# Test what will be upgraded (recommended)
./hcloud-k8s upgrade --config config.yaml --dry-run

# Execute upgrade
./hcloud-k8s upgrade --config config.yaml
```

**Features:**
- Sequential control plane upgrades (maintains quorum)
- Automatic health checks between upgrades
- Version checking (skips nodes already at target version)
- Kubernetes control plane upgrade after Talos upgrade

**See [UPGRADE.md](UPGRADE.md) for detailed upgrade guide.**

### 6. Destroy Cluster

Remove all cluster resources when no longer needed.

```bash
./hcloud-k8s destroy --config config.yaml
```

## Configuration

### Reverse DNS (RDNS)

Configure automatic PTR records for servers and load balancers. RDNS templates support dynamic variable substitution:

**Available Template Variables:**
- `{{ cluster-name }}` - Cluster name
- `{{ hostname }}` - Server/Load balancer name
- `{{ id }}` - Resource ID
- `{{ ip-labels }}` - Reverse IP notation (e.g., `1.2.3.4` → `4.3.2.1`)
- `{{ ip-type }}` - IP version (`ipv4` or `ipv6`)
- `{{ role }}` - Resource role (`control-plane`, `worker`, `kube-api`, `ingress`)
- `{{ pool }}` - Node pool name

**Example Configuration:**

```yaml
cluster_name: "my-cluster"

# Global RDNS defaults (applies to all resources)
rdns:
  cluster: "{{ hostname }}.{{ cluster-name }}.example.com"
  cluster_ipv4: "{{ ip-labels }}.nodes.example.com"
  cluster_ipv6: "{{ ip-labels }}.ipv6.example.com"
  ingress_ipv4: "{{ cluster-name }}-ingress.example.com"
  ingress_ipv6: "{{ cluster-name }}-ingress-v6.example.com"

control_plane:
  node_pools:
    - name: "control-plane"
      count: 3
      server_type: "cpx22"
      # Override RDNS for this pool (optional)
      rdns_ipv4: "{{ hostname }}.cp.example.com"
      rdns_ipv6: "{{ hostname }}.cp-v6.example.com"

workers:
  - name: "worker"
    count: 2
    server_type: "cpx32"
    # Override RDNS for this pool (optional)
    rdns_ipv4: "{{ hostname }}.workers.example.com"

ingress:
  enabled: true
  # Override RDNS for ingress load balancer (optional)
  rdns_ipv4: "ingress.example.com"
  rdns_ipv6: "ingress-v6.example.com"
```

**RDNS Resolution Order (Fallback Chain):**

1. **Servers (Control Plane/Worker):**
   - Pool-specific: `node_pools[].rdns_ipv4`
   - Cluster default: `rdns.cluster_rdns_ipv4`
   - Generic default: `rdns.cluster`

2. **Kube API Load Balancer:**
   - Cluster default: `rdns.cluster_rdns_ipv4`
   - Generic default: `rdns.cluster`

3. **Ingress Load Balancer:**
   - Ingress-specific: `ingress.rdns_ipv4`
   - Cluster ingress default: `rdns.ingress_rdns_ipv4`
   - Cluster default: `rdns.cluster_rdns_ipv4`
   - Generic default: `rdns.cluster`

**Notes:**
- RDNS failures are logged as warnings and don't block provisioning
- IPv6 support requires IPv6-enabled resources
- PTR records are automatically updated when resources are recreated

## Development

### Architecture

The project follows a domain-driven Go layout:

```
cmd/
├── hcloud-k8s/
│   ├── commands/     # CLI command definitions (cobra)
│   └── handlers/     # Business logic for commands

internal/
├── config/           # Configuration loading and validation
├── orchestration/    # High-level workflow coordination
├── provisioning/     # Cluster provisioning domain
│   ├── infrastructure/  # Network, Firewall, Load Balancers
│   ├── compute/         # Servers, Control Plane, Workers
│   ├── image/           # Talos image building
│   └── cluster/         # Bootstrap and configuration
├── platform/         # External system integrations
│   ├── hcloud/          # Hetzner Cloud API client
│   ├── talos/           # Talos configuration generator
│   └── ssh/             # SSH connectivity
├── addons/           # Kubernetes addon installation
└── util/             # Reusable utilities (async, naming, retry)
```

Key design principles:
- **Pipeline-based provisioning**: Phases execute sequentially with shared context
- **Structured observability**: Consistent logging via Observer interface
- **Idempotent operations**: Safe to run multiple times
- **Validation-first**: Pre-flight checks before resource creation

### Testing

Run unit tests:
```bash
go test ./...
```

Run End-to-End (E2E) tests (requires `HCLOUD_TOKEN`):
```bash
export HCLOUD_TOKEN="your-token"
go test -v -tags=e2e ./tests/e2e/...
```

### Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development guidelines and code style.
