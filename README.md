# k8zner

[![CI](https://github.com/imamik/k8zner/actions/workflows/ci.yaml/badge.svg)](https://github.com/imamik/k8zner/actions/workflows/ci.yaml)
[![codecov](https://codecov.io/gh/imamik/k8zner/branch/main/graph/badge.svg)](https://codecov.io/gh/imamik/k8zner)
[![Go Report Card](https://goreportcard.com/badge/github.com/imamik/k8zner)](https://goreportcard.com/report/github.com/imamik/k8zner)
[![Release](https://img.shields.io/github/v/release/imamik/k8zner)](https://github.com/imamik/k8zner/releases/latest)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

**k8zner** (k8s + Hetzner) — Production-ready Kubernetes clusters on [Hetzner Cloud](https://www.hetzner.com/cloud) using [Talos Linux](https://www.talos.dev/).

A self-contained Go CLI that provisions secure, highly available Kubernetes clusters with batteries included. No Terraform, kubectl, or talosctl required — just download and run.

### Get Started in Minutes

```bash
export HCLOUD_TOKEN="your-token"
k8zner init        # Interactive wizard
k8zner apply       # Deploy cluster
```

### Batteries Included

k8zner comes with pre-configured integrations for a complete production setup:

| Category | Integrations |
|----------|-------------|
| **Networking** | [Cilium](https://cilium.io/) (eBPF CNI), [Gateway API](https://gateway-api.sigs.k8s.io/), [ingress-nginx](https://kubernetes.github.io/ingress-nginx/), [Traefik](https://traefik.io/) |
| **Cloud Integration** | [Hetzner CCM](https://github.com/hetznercloud/hcloud-cloud-controller-manager), [Hetzner CSI](https://github.com/hetznercloud/csi-driver) |
| **DNS & TLS** | [Cloudflare](https://www.cloudflare.com/) DNS, [external-dns](https://github.com/kubernetes-sigs/external-dns), [cert-manager](https://cert-manager.io/) with Let's Encrypt |
| **GitOps** | [ArgoCD](https://argo-cd.readthedocs.io/) |
| **Storage** | [Longhorn](https://longhorn.io/) distributed storage |
| **Scaling** | [Cluster Autoscaler](https://github.com/kubernetes/autoscaler) |
| **Monitoring** | [Metrics Server](https://github.com/kubernetes-sigs/metrics-server), [Prometheus Operator](https://prometheus-operator.dev/) CRDs |
| **Auth** | OIDC integration |

---

## Acknowledgments

This project is a **direct Go port** of the excellent [terraform-hcloud-kubernetes](https://github.com/hcloud-k8s/terraform-hcloud-kubernetes) Terraform module. We extend our sincere gratitude to the original authors and contributors of that project for their pioneering work in making production-grade Kubernetes accessible on Hetzner Cloud.

The architectural decisions, security model, and component integrations in this project are heavily inspired by their Terraform implementation. This Go port aims to provide the same production-ready experience while offering the benefits of a single self-contained binary and native Go tooling.

**If you prefer Infrastructure-as-Code with Terraform, we highly recommend the original module.**

---

## Why This Project?

### The Go Advantage

While the original Terraform module is excellent, a native Go implementation offers distinct benefits:

| Aspect | Terraform Module | Go CLI |
|--------|------------------|--------|
| **Dependencies** | Terraform, kubectl, talosctl | Single binary (104MB) |
| **State Management** | Terraform state files | Stateless, idempotent operations |
| **Portability** | Requires Terraform installation | Download and run |
| **CI/CD Integration** | Terraform workflows | Simple binary execution |
| **Extensibility** | HCL modules | Go code, easy to fork |
| **Interactive Setup** | Manual HCL writing | Guided wizard |

### When to Use This Tool

- You want a **self-contained CLI** without managing Terraform state
- You prefer **interactive cluster configuration** over writing HCL
- You need **simple CI/CD integration** with a single binary
- You want to **customize the provisioning logic** in Go
- You're already in a **Go-centric development environment**

### When to Use Terraform Instead

- You already use **Terraform for infrastructure management**
- You need **drift detection and plan/apply workflows**
- You require **Terraform state for compliance/audit**
- You want to **integrate with other Terraform modules**

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Hetzner Cloud                                   │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                         Private Network                                 │ │
│  │   10.0.0.0/8 (configurable)                                            │ │
│  │  ┌─────────────────────────────────────────────────────────────────┐   │ │
│  │  │                     Node Subnet (10.0.1.0/24)                    │   │ │
│  │  │                                                                   │   │ │
│  │  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │   │ │
│  │  │  │Control Plane│  │Control Plane│  │Control Plane│              │   │ │
│  │  │  │   Node 1    │  │   Node 2    │  │   Node 3    │              │   │ │
│  │  │  │  (Talos)    │  │  (Talos)    │  │  (Talos)    │              │   │ │
│  │  │  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘              │   │ │
│  │  │         │                │                │                      │   │ │
│  │  │         └────────────────┼────────────────┘                      │   │ │
│  │  │                          │                                       │   │ │
│  │  │                    ┌─────┴─────┐                                 │   │ │
│  │  │                    │   etcd    │                                 │   │ │
│  │  │                    │  cluster  │                                 │   │ │
│  │  │                    └───────────┘                                 │   │ │
│  │  │                                                                   │   │ │
│  │  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌────────┐  │   │ │
│  │  │  │   Worker    │  │   Worker    │  │   Worker    │  │  ...   │  │   │ │
│  │  │  │   Node 1    │  │   Node 2    │  │   Node N    │  │        │  │   │ │
│  │  │  │  (Talos)    │  │  (Talos)    │  │  (Talos)    │  │        │  │   │ │
│  │  │  └─────────────┘  └─────────────┘  └─────────────┘  └────────┘  │   │ │
│  │  │                                                                   │   │ │
│  │  └───────────────────────────────────────────────────────────────────┘   │ │
│  │                                                                          │ │
│  │  ┌──────────────────┐    ┌──────────────────┐                           │ │
│  │  │  Load Balancer   │    │  Load Balancer   │                           │ │
│  │  │   (Kube API)     │    │    (Ingress)     │                           │ │
│  │  └──────────────────┘    └──────────────────┘                           │ │
│  │                                                                          │ │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │ │
│  │  │                        Firewall Rules                             │   │ │
│  │  │  • Kube API (6443) - configurable source IPs                     │   │ │
│  │  │  • Talos API (50000) - configurable source IPs                   │   │ │
│  │  │  • Node communication - internal only                             │   │ │
│  │  └──────────────────────────────────────────────────────────────────┘   │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│                          Kubernetes Components                               │
│                                                                              │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐             │
│  │     Cilium      │  │   Hetzner CCM   │  │   Hetzner CSI   │             │
│  │  (CNI + eBPF)   │  │ (Cloud Control) │  │    (Storage)    │             │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘             │
│                                                                              │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐             │
│  │  cert-manager   │  │  external-dns   │  │ metrics-server  │             │
│  │ (Certificates)  │  │ (DNS Records)   │  │  (Monitoring)   │             │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘             │
│                                                                              │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐             │
│  │  ingress-nginx  │  │    Traefik      │  │ Cluster Autoscaler            │
│  │   (Ingress)     │  │   (Ingress)     │  │   (Scaling)     │             │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Talos Linux: The Foundation

This project uses [Talos Linux](https://www.talos.dev/) as the operating system for all cluster nodes. Talos is purpose-built for Kubernetes:

### Why Talos?

- **Immutable**: The OS is read-only. No SSH, no shell, no package manager. Configuration only via API.
- **Minimal**: Contains only what's needed to run Kubernetes. Reduced attack surface.
- **Secure**: API-driven management with mutual TLS. No traditional Linux userland vulnerabilities.
- **Declarative**: Machine configuration is versioned and reproducible.
- **Fast**: Boots in seconds. Minimal overhead.

### Talos vs Traditional Linux

| Aspect | Traditional Linux | Talos Linux |
|--------|-------------------|-------------|
| **Access** | SSH, shell, sudo | API only (talosctl) |
| **Updates** | Package manager | Atomic image replacement |
| **Configuration** | Files, scripts | Declarative YAML |
| **Attack Surface** | Large (systemd, sshd, etc.) | Minimal (Kubernetes only) |
| **Drift** | Possible (manual changes) | Impossible (immutable) |

---

## Security Architecture

### Defense in Depth

```
┌─────────────────────────────────────────────────────────────────┐
│                      Layer 1: Network Perimeter                  │
│  • Hetzner Cloud Firewall                                       │
│  • Configurable source IP allowlists                            │
│  • Separate rules for Kube API vs Talos API                     │
└─────────────────────────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Layer 2: Network Isolation                  │
│  • Private network for node communication                        │
│  • Pod network isolated from node network                        │
│  • Cilium NetworkPolicies                                        │
└─────────────────────────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Layer 3: Pod Network Encryption             │
│  • Cilium WireGuard or IPsec encryption                         │
│  • Transparent pod-to-pod encryption                             │
│  • No application changes required                               │
└─────────────────────────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Layer 4: OS Security                        │
│  • Talos immutable filesystem                                    │
│  • No SSH, no shell access                                       │
│  • API-only management with mTLS                                 │
│  • Minimal attack surface                                        │
└─────────────────────────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Layer 5: Storage Encryption                 │
│  • LUKS2 disk encryption (optional)                             │
│  • Encrypted persistent volumes via CSI                          │
└─────────────────────────────────────────────────────────────────┘
```

### Network Encryption with Cilium

Pod-to-pod traffic can be encrypted transparently using Cilium's built-in encryption:

```yaml
addons:
  cilium:
    enabled: true
    encryption:
      enabled: true
      type: "wireguard"  # or "ipsec"
```

- **WireGuard**: Modern, fast, kernel-level encryption
- **IPsec**: Standards-compliant, FIPS-compatible option

---

## Bundled Components

### Container Networking: Cilium

[Cilium](https://cilium.io/) provides advanced networking using eBPF:

- **eBPF-based datapath**: High-performance packet processing in the kernel
- **Network Policies**: L3/L4/L7 policy enforcement
- **Transparent Encryption**: WireGuard or IPsec for pod traffic
- **Hubble**: Network observability and flow visualization
- **Gateway API**: Native Kubernetes Gateway implementation

### Cloud Integration: Hetzner CCM

The [Hetzner Cloud Controller Manager](https://github.com/hetznercloud/hcloud-cloud-controller-manager):

- **Node lifecycle management**: Automatic node registration and cleanup
- **Load Balancer provisioning**: Native Hetzner Load Balancers for Services
- **Node metadata**: Zone and region information for scheduling

### Storage: Hetzner CSI

The [Hetzner CSI Driver](https://github.com/hetznercloud/csi-driver):

- **Dynamic provisioning**: Automatic volume creation
- **Volume expansion**: Resize without downtime
- **Encryption**: Optional volume encryption
- **Snapshots**: Point-in-time volume snapshots

### Additional Components

| Component | Purpose |
|-----------|---------|
| **cert-manager** | Automated TLS certificate management with Let's Encrypt |
| **external-dns** | Automatic DNS record creation from Ingress/Service |
| **ingress-nginx** | Production-grade ingress controller |
| **Traefik** | Alternative ingress with automatic HTTPS |
| **metrics-server** | Resource metrics for HPA and kubectl top |
| **Cluster Autoscaler** | Automatic node scaling based on demand |
| **Longhorn** | Distributed block storage (alternative to Hetzner volumes) |
| **ArgoCD** | GitOps continuous delivery |

---

## Quick Start

### Prerequisites

- Hetzner Cloud account with API token
- (Optional) Cloudflare account for DNS integration

### Installation

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

### 1. Set Environment Variables

```bash
export HCLOUD_TOKEN="your-hetzner-api-token"

# Optional: For Cloudflare DNS integration
export CF_API_TOKEN="your-cloudflare-api-token"
export CF_DOMAIN="example.com"
```

### 2. Create Cluster Configuration

**Interactive Wizard (Recommended):**
```bash
k8zner init
```

The wizard guides you through:
- Cluster name and Hetzner location
- Server architecture (x86 or ARM)
- Control plane sizing (1 or 3+ nodes for HA)
- Worker pool configuration
- CNI selection and encryption
- Addon selection

**Advanced options:**
```bash
k8zner init --advanced
```

### 3. Build Talos Image

```bash
k8zner image build -c cluster.yaml
```

This creates a snapshot of the Talos Linux image in your Hetzner account for fast node provisioning.

### 4. Deploy the Cluster

```bash
k8zner apply -c cluster.yaml
```

The provisioning pipeline:
1. **Validation**: Verify configuration and API access
2. **Infrastructure**: Create network, firewall, load balancers
3. **Compute**: Provision control plane and worker nodes
4. **Cluster**: Bootstrap Kubernetes and install addons

### 5. Access Your Cluster

```bash
export KUBECONFIG=./secrets/my-cluster/kubeconfig
kubectl get nodes
```

---

## Configuration Reference

### Minimal Configuration

```yaml
cluster_name: "my-cluster"
location: "nbg1"

control_plane:
  nodepools:
    - name: "control"
      type: "cpx21"
      count: 3

workers:
  - name: "general"
    type: "cpx31"
    count: 2
```

### Full Configuration Example

```yaml
# Cluster identity
cluster_name: "production"
location: "nbg1"          # Primary location
ssh_keys: ["my-ssh-key"]  # For Talos image building

# Kubernetes and Talos versions
talos:
  version: "v1.9.0"
kubernetes:
  version: "1.32.0"

# Network configuration
network:
  ipv4_cidr: "10.0.0.0/8"
  node_ipv4_cidr: "10.0.1.0/24"
  pod_ipv4_cidr: "10.244.0.0/16"
  service_ipv4_cidr: "10.96.0.0/12"

  # Existing network (optional)
  # existing_network_id: 12345

# Firewall rules
firewall:
  # IPs allowed to access Kubernetes API
  kube_api_allowed_sources:
    - "0.0.0.0/0"  # All IPs (restrict in production!)

  # IPs allowed to access Talos API
  talos_api_allowed_sources:
    - "1.2.3.4/32"  # Your IP only

# Control plane (HA with 3 nodes)
control_plane:
  nodepools:
    - name: "control"
      type: "cpx21"       # 3 vCPU, 4 GB RAM
      count: 3
      location: "nbg1"
      labels:
        node.kubernetes.io/control-plane: "true"

# Worker pools
workers:
  - name: "general"
    type: "cpx31"         # 4 vCPU, 8 GB RAM
    count: 3
    location: "nbg1"
    labels:
      workload-type: "general"

  - name: "memory"
    type: "cpx41"         # 8 vCPU, 16 GB RAM
    count: 2
    location: "fsn1"
    labels:
      workload-type: "memory-intensive"
    taints:
      - key: "workload"
        value: "memory"
        effect: "NoSchedule"

# Auto-scaling configuration
autoscaler:
  enabled: true
  nodepools:
    - name: "general"
      min: 2
      max: 10

# Load balancer configuration
load_balancer:
  enabled: true
  type: "lb11"
  location: "nbg1"

# Ingress load balancer
ingress_load_balancer:
  enabled: true
  type: "lb11"

# Addons
addons:
  # Container networking
  cilium:
    enabled: true
    version: "1.16.0"
    hubble:
      enabled: true       # Network observability
    encryption:
      enabled: true
      type: "wireguard"   # or "ipsec"
    gateway_api:
      enabled: true

  # Cloud integration
  ccm:
    enabled: true
  csi:
    enabled: true
    encryption: true      # Encrypted volumes

  # Certificate management
  cert_manager:
    enabled: true
    cloudflare:
      enabled: true
      email: "admin@example.com"

  # DNS automation
  external_dns:
    enabled: true
    cloudflare:
      proxied: false

  # Ingress controllers (choose one)
  ingress_nginx:
    enabled: true
  traefik:
    enabled: false

  # Monitoring
  metrics_server:
    enabled: true

  # Auto-scaling
  cluster_autoscaler:
    enabled: true

  # Storage
  longhorn:
    enabled: false        # Alternative to Hetzner CSI

  # GitOps
  argocd:
    enabled: false
```

---

## Hetzner Server Types

### x86 Servers

| Family | Type | vCPU | RAM | Description |
|--------|------|------|-----|-------------|
| **CX** | cx22 | 2 | 4 GB | Shared, cost-optimized (EU only) |
| | cx32 | 4 | 8 GB | |
| | cx42 | 8 | 16 GB | |
| | cx52 | 16 | 32 GB | |
| **CPX** | cpx11 | 2 | 2 GB | Shared, AMD EPYC |
| | cpx21 | 3 | 4 GB | |
| | cpx31 | 4 | 8 GB | |
| | cpx41 | 8 | 16 GB | |
| | cpx51 | 16 | 32 GB | |
| **CCX** | ccx13 | 2 | 8 GB | Dedicated vCPUs |
| | ccx23 | 4 | 16 GB | |
| | ccx33 | 8 | 32 GB | |
| | ccx43 | 16 | 64 GB | |
| | ccx53 | 32 | 128 GB | |
| | ccx63 | 48 | 192 GB | |

### ARM Servers (CAX)

| Type | vCPU | RAM | Notes |
|------|------|-----|-------|
| cax11 | 2 | 4 GB | Ampere Altra |
| cax21 | 4 | 8 GB | |
| cax31 | 8 | 16 GB | |
| cax41 | 16 | 32 GB | |

**Note**: ARM servers are only available in Germany (fsn1, nbg1) and Finland (hel1).

### Location Codes

| Code | Location | Country |
|------|----------|---------|
| fsn1 | Falkenstein | Germany |
| nbg1 | Nuremberg | Germany |
| hel1 | Helsinki | Finland |
| ash | Ashburn | USA |
| hil | Hillsboro | USA |

---

## Cloudflare Integration

Automatic DNS and TLS certificate management via Cloudflare:

### Setup

1. Create a Cloudflare API token with DNS edit permissions
2. Set environment variables:
   ```bash
   export CF_API_TOKEN="your-token"
   export CF_DOMAIN="example.com"
   ```
3. Enable in configuration:
   ```yaml
   addons:
     cloudflare:
       enabled: true
     external_dns:
       enabled: true
     cert_manager:
       enabled: true
       cloudflare:
         enabled: true
         email: "admin@example.com"
   ```

### How It Works

1. **external-dns** watches Ingress resources and creates DNS records
2. **cert-manager** uses DNS01 challenge via Cloudflare for Let's Encrypt
3. TLS certificates are automatically issued and renewed

### Example Ingress

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-app
  annotations:
    external-dns.alpha.kubernetes.io/hostname: app.example.com
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  tls:
    - hosts:
        - app.example.com
      secretName: app-tls
  rules:
    - host: app.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: my-app
                port:
                  number: 80
```

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `k8zner init` | Interactive wizard to create cluster.yaml |
| `k8zner init --advanced` | Wizard with advanced networking/security options |
| `k8zner apply -c cluster.yaml` | Create or update cluster |
| `k8zner destroy -c cluster.yaml` | Tear down all resources |
| `k8zner upgrade -c cluster.yaml` | Upgrade Talos/Kubernetes |
| `k8zner image build -c cluster.yaml` | Build Talos image snapshot |
| `k8zner image delete` | Delete image snapshots |
| `k8zner version` | Show version information |

---

## Provisioning Pipeline

The `apply` command executes a multi-phase pipeline:

```
┌─────────────────────────────────────────────────────────────────┐
│ Phase 1: VALIDATION                                              │
│ • Parse and validate configuration                               │
│ • Verify Hetzner API token                                       │
│ • Check server type and location availability                    │
│ • Detect resource conflicts                                      │
└─────────────────────────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│ Phase 2: INFRASTRUCTURE                                          │
│ • Create/verify private network and subnets                     │
│ • Configure firewall rules                                       │
│ • Provision load balancers (API, ingress)                       │
│ • Allocate floating IPs (if configured)                         │
│ • Setup placement groups                                         │
└─────────────────────────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│ Phase 3: IMAGE                                                   │
│ • Check for existing Talos snapshot                             │
│ • Build custom image if needed                                   │
│ • Upload and cache snapshot                                      │
└─────────────────────────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│ Phase 4: COMPUTE                                                 │
│ • Provision control plane nodes                                  │
│ • Generate and apply Talos machine configs                      │
│ • Provision worker nodes                                         │
│ • Configure reverse DNS                                          │
└─────────────────────────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│ Phase 5: CLUSTER                                                 │
│ • Initialize etcd on first control plane                        │
│ • Bootstrap Kubernetes                                           │
│ • Join remaining control plane nodes                             │
│ • Generate kubeconfig                                            │
│ • Install configured addons                                      │
│   └─ CNI (Cilium)                                               │
│   └─ Cloud controller manager                                    │
│   └─ CSI driver                                                  │
│   └─ Ingress, cert-manager, etc.                                │
└─────────────────────────────────────────────────────────────────┘
```

### Idempotency

All operations are idempotent. Re-running `apply` on an existing cluster:
- Skips resources that already exist
- Updates configurations if changed
- Adds new nodes if pool sizes increased

---

## Project Structure

```
cmd/
└── k8zner/
    ├── commands/         # CLI command definitions (Cobra)
    └── handlers/         # Business logic

internal/
├── config/              # Configuration parsing and validation
│   └── wizard/          # Interactive setup wizard
├── orchestration/       # Provisioning workflow coordination
├── provisioning/        # Infrastructure and cluster provisioning
│   ├── infrastructure/  # Network, firewall, load balancers
│   ├── compute/         # Server provisioning
│   ├── cluster/         # Kubernetes bootstrap
│   ├── image/           # Talos image building
│   ├── destroy/         # Resource cleanup
│   └── upgrade/         # Version upgrades
├── addons/              # Kubernetes addon installation
│   ├── helm/            # Helm chart rendering
│   └── k8sclient/       # Embedded Kubernetes client
├── platform/
│   ├── hcloud/          # Hetzner Cloud API wrapper
│   ├── talos/           # Talos configuration generation
│   └── ssh/             # SSH client for image building
└── util/                # Shared utilities
```

---

## Development

```bash
# Build
make build

# Run tests
make test

# Run linters
make lint

# Run all checks
make check

# Build for all platforms
make build-all
```

### Testing

The project includes comprehensive tests:
- Unit tests with >80% coverage
- Mock Hetzner client for isolated testing
- E2E tests against real Hetzner infrastructure

```bash
# Run unit tests
make test

# Run with coverage
make test-coverage

# Run E2E tests (requires HCLOUD_TOKEN)
make test-e2e
```

---

## Upgrading

### Kubernetes Version

```bash
# Edit cluster.yaml
kubernetes:
  version: "1.33.0"

# Apply upgrade
k8zner upgrade -c cluster.yaml
```

### Talos Version

```bash
# Edit cluster.yaml
talos:
  version: "v1.10.0"

# Rebuild image
k8zner image build -c cluster.yaml

# Apply upgrade (rolling update)
k8zner upgrade -c cluster.yaml
```

---

## Troubleshooting

### Common Issues

**API token errors:**
```bash
# Verify token is set
echo $HCLOUD_TOKEN

# Test API access
curl -H "Authorization: Bearer $HCLOUD_TOKEN" \
  https://api.hetzner.cloud/v1/servers
```

**Node not joining cluster:**
- Check firewall rules allow Talos API (port 50000)
- Verify network connectivity between nodes
- Check Talos machine config with `talosctl`

**Addon installation failures:**
- Ensure cluster is fully bootstrapped
- Check addon prerequisites (e.g., CRDs)
- Review addon logs in cluster

### Getting Cluster Credentials

```bash
# Kubeconfig is saved to secrets directory
export KUBECONFIG=./secrets/<cluster-name>/kubeconfig

# Talos config for node management
export TALOSCONFIG=./secrets/<cluster-name>/talosconfig
```

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development guidelines.

---

## License

Apache License 2.0 - see [LICENSE](LICENSE)

---

## Related Projects

- [terraform-hcloud-kubernetes](https://github.com/hcloud-k8s/terraform-hcloud-kubernetes) - The original Terraform module this project is based on
- [Talos Linux](https://www.talos.dev/) - The secure, immutable Kubernetes OS
- [Cilium](https://cilium.io/) - eBPF-based networking
- [Hetzner Cloud](https://www.hetzner.com/cloud) - Affordable cloud infrastructure
