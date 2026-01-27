# k8zner

[![CI](https://github.com/imamik/k8zner/actions/workflows/ci.yaml/badge.svg)](https://github.com/imamik/k8zner/actions/workflows/ci.yaml)
[![codecov](https://codecov.io/gh/imamik/k8zner/branch/main/graph/badge.svg)](https://codecov.io/gh/imamik/k8zner)
[![Go Report Card](https://goreportcard.com/badge/github.com/imamik/k8zner)](https://goreportcard.com/report/github.com/imamik/k8zner)
[![Release](https://img.shields.io/github/v/release/imamik/k8zner)](https://github.com/imamik/k8zner/releases/latest)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

**k8zner** (k8s + Hetzner) — Production-ready Kubernetes on [Hetzner Cloud](https://www.hetzner.com/cloud), the cost-effective way.

### Why k8zner?

Running Kubernetes shouldn't require a dedicated platform team. k8zner enables engineers to deploy **production-ready, highly available clusters** on Hetzner Cloud — one of the most cost-effective cloud providers — without the complexity.

- **From zero to production cluster** in minutes, not days
- **Day-one operations solved**: networking, storage, TLS, DNS, GitOps — all pre-configured
- **Bridge to application deployment**: built-in ArgoCD, ingress, and cert-manager get your apps running fast
- **Single binary**: No Terraform, kubectl, or talosctl required — just download and run

Built on [Talos Linux](https://www.talos.dev/), the secure and immutable Kubernetes OS.

## Quick Start

```bash
# 1. Install
brew install imamik/tap/k8zner   # or: go install github.com/imamik/k8zner/cmd/k8zner@latest

# 2. Set your Hetzner Cloud API token
export HCLOUD_TOKEN="your-token"

# 3. Create and deploy
k8zner init              # Interactive wizard guides you through setup
k8zner image build       # Build Talos Linux image (one-time)
k8zner apply             # Deploy your cluster

# 4. Access
export KUBECONFIG=./secrets/my-cluster/kubeconfig
kubectl get nodes
```

### Optional: Cloudflare DNS & TLS

For automatic DNS records and Let's Encrypt certificates, set up Cloudflare:

1. **Create a Cloudflare API Token** at [dash.cloudflare.com/profile/api-tokens](https://dash.cloudflare.com/profile/api-tokens):
   - Click "Create Token" → Use "Edit zone DNS" template
   - Set Zone Resources to your domain
   - Required permissions: `Zone > Zone > Read` and `Zone > DNS > Edit`

2. **Set environment variables:**
   ```bash
   export CF_API_TOKEN="your-cloudflare-api-token"
   export CF_DOMAIN="example.com"
   ```

3. **Enable in wizard** or add to config:
   ```yaml
   addons:
     cloudflare: { enabled: true }
     external_dns: { enabled: true }
     cert_manager: { enabled: true, cloudflare: { enabled: true, email: "you@example.com" } }
   ```

See [Cloudflare DNS Integration](docs/configuration.md#cloudflare-dns-integration) for full setup guide.

## Batteries Included

k8zner comes with pre-configured integrations — enable what you need:

| Category | Integrations |
|----------|-------------|
| **Networking** | [Cilium](https://cilium.io/) (eBPF CNI with WireGuard/IPsec encryption), [Gateway API](https://gateway-api.sigs.k8s.io/), [ingress-nginx](https://kubernetes.github.io/ingress-nginx/), [Traefik](https://traefik.io/) |
| **Cloud** | [Hetzner CCM](https://github.com/hetznercloud/hcloud-cloud-controller-manager) (load balancers, node lifecycle), [Hetzner CSI](https://github.com/hetznercloud/csi-driver) (volumes) |
| **DNS & TLS** | [Cloudflare](https://www.cloudflare.com/) integration, [external-dns](https://github.com/kubernetes-sigs/external-dns), [cert-manager](https://cert-manager.io/) with Let's Encrypt |
| **GitOps** | [ArgoCD](https://argo-cd.readthedocs.io/) |
| **Storage** | [Longhorn](https://longhorn.io/) distributed storage |
| **Scaling** | [Cluster Autoscaler](https://github.com/kubernetes/autoscaler) |
| **Monitoring** | [Metrics Server](https://github.com/kubernetes-sigs/metrics-server), [Prometheus Operator](https://prometheus-operator.dev/) CRDs, [Hubble](https://docs.cilium.io/en/stable/observability/hubble/) |
| **Auth** | OIDC integration |

## Origin & Vision

k8zner started as a **Go port** of the excellent [terraform-hcloud-kubernetes](https://github.com/hcloud-k8s/terraform-hcloud-kubernetes) Terraform module. We extend our sincere gratitude to the original authors for their pioneering work in making production-grade Kubernetes accessible on Hetzner Cloud.

**Where we're headed:** Beyond cluster provisioning, k8zner aims to become a broader toolkit that simplifies the entire Kubernetes journey — from infrastructure to application deployment. Our goal is to make highly available, production-ready Kubernetes accessible to every engineer, not just platform teams.

**Prefer Terraform?** The [original module](https://github.com/hcloud-k8s/terraform-hcloud-kubernetes) is excellent for IaC workflows.

---

<details>
<summary><strong>k8zner vs Terraform Module</strong></summary>

### Comparison

| Aspect | Terraform Module | k8zner |
|--------|------------------|--------|
| **Dependencies** | Terraform, kubectl, talosctl | Single binary |
| **State** | Terraform state files | Stateless, idempotent |
| **Setup** | Write HCL manually | Interactive wizard |
| **CI/CD** | Terraform workflows | Simple binary execution |
| **Extensibility** | HCL modules | Fork and modify Go code |
| **Day-one ops** | Manual addon setup | Pre-configured & integrated |

### Choose k8zner when you want to

- Get a **production cluster running fast** without IaC expertise
- Have **day-one operations pre-configured** (DNS, TLS, GitOps, monitoring)
- Use a **self-contained CLI** without managing Terraform state
- **Simplify CI/CD** with a single binary

### Choose Terraform when you need

- **Infrastructure-as-Code workflows** with plan/apply
- **Drift detection** and state management
- **Integration with other Terraform modules**
- **Compliance/audit trails** via Terraform state

</details>

<details>
<summary><strong>Architecture Overview</strong></summary>

Full documentation: **[docs/architecture.md](docs/architecture.md)**

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
│  │  │  • Kube API (6443) • Talos API (50000) • Internal only           │   │ │
│  │  └──────────────────────────────────────────────────────────────────┘   │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Provisioning Pipeline

The `apply` command executes these phases:

1. **Validation** — Verify configuration and API access
2. **Infrastructure** — Create network, firewall, load balancers
3. **Image** — Build/cache Talos Linux snapshot
4. **Compute** — Provision control plane and worker nodes
5. **Cluster** — Bootstrap Kubernetes and install addons

All operations are **idempotent** — re-running `apply` safely skips existing resources.

</details>

<details>
<summary><strong>Why Talos Linux?</strong></summary>

[Talos Linux](https://www.talos.dev/) is a secure, immutable OS purpose-built for Kubernetes:

| Aspect | Traditional Linux | Talos Linux |
|--------|-------------------|-------------|
| **Access** | SSH, shell, sudo | API only (talosctl) |
| **Updates** | Package manager | Atomic image replacement |
| **Configuration** | Files, scripts | Declarative YAML |
| **Attack Surface** | Large (systemd, sshd, etc.) | Minimal (Kubernetes only) |
| **Drift** | Possible (manual changes) | Impossible (immutable) |

**Key benefits:**
- **Immutable** — Read-only filesystem, no SSH, no shell
- **Minimal** — Only what's needed for Kubernetes
- **Secure** — API-driven with mutual TLS
- **Fast** — Boots in seconds

</details>

<details>
<summary><strong>Security Architecture</strong></summary>

### Defense in Depth

```
┌─────────────────────────────────────────────────────────────────┐
│  Layer 1: Network Perimeter                                      │
│  • Hetzner Cloud Firewall with configurable IP allowlists       │
└─────────────────────────────────────────────────────────────────┘
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│  Layer 2: Network Isolation                                      │
│  • Private network for nodes • Cilium NetworkPolicies           │
└─────────────────────────────────────────────────────────────────┘
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│  Layer 3: Pod Network Encryption                                 │
│  • Cilium WireGuard or IPsec • Transparent pod-to-pod encryption│
└─────────────────────────────────────────────────────────────────┘
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│  Layer 4: OS Security                                            │
│  • Talos immutable filesystem • No SSH • API-only with mTLS     │
└─────────────────────────────────────────────────────────────────┘
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│  Layer 5: Storage Encryption                                     │
│  • LUKS2 disk encryption • Encrypted volumes via CSI            │
└─────────────────────────────────────────────────────────────────┘
```

### Enable Network Encryption

```yaml
addons:
  cilium:
    enabled: true
    encryption:
      enabled: true
      type: "wireguard"  # or "ipsec"
```

</details>

<details>
<summary><strong>Configuration Reference</strong></summary>

Full documentation: **[docs/configuration.md](docs/configuration.md)** | Interactive setup: **[docs/wizard.md](docs/wizard.md)**

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
cluster_name: "production"
location: "nbg1"
ssh_keys: ["my-ssh-key"]

talos:
  version: "v1.9.0"
kubernetes:
  version: "1.32.0"

# Network
network:
  ipv4_cidr: "10.0.0.0/8"
  node_ipv4_cidr: "10.0.1.0/24"
  pod_ipv4_cidr: "10.244.0.0/16"
  service_ipv4_cidr: "10.96.0.0/12"

# Firewall
firewall:
  kube_api_allowed_sources: ["0.0.0.0/0"]
  talos_api_allowed_sources: ["1.2.3.4/32"]

# Control plane (HA)
control_plane:
  nodepools:
    - name: "control"
      type: "cpx21"
      count: 3
      location: "nbg1"

# Workers
workers:
  - name: "general"
    type: "cpx31"
    count: 3
    location: "nbg1"
  - name: "memory"
    type: "cpx41"
    count: 2
    location: "fsn1"
    taints:
      - key: "workload"
        value: "memory"
        effect: "NoSchedule"

# Auto-scaling
autoscaler:
  enabled: true
  nodepools:
    - name: "general"
      min: 2
      max: 10

# Load balancers
load_balancer:
  enabled: true
  type: "lb11"
ingress_load_balancer:
  enabled: true
  type: "lb11"

# Addons
addons:
  cilium:
    enabled: true
    hubble:
      enabled: true
    encryption:
      enabled: true
      type: "wireguard"
    gateway_api:
      enabled: true
  ccm:
    enabled: true
  csi:
    enabled: true
    encryption: true
  cert_manager:
    enabled: true
    cloudflare:
      enabled: true
      email: "admin@example.com"
  external_dns:
    enabled: true
  ingress_nginx:
    enabled: true
  metrics_server:
    enabled: true
  cluster_autoscaler:
    enabled: true
  argocd:
    enabled: false
  longhorn:
    enabled: false
```

</details>

<details>
<summary><strong>Hetzner Server Types</strong></summary>

### x86 Servers

| Family | Type | vCPU | RAM | Description |
|--------|------|------|-----|-------------|
| **CX** | cx22-cx52 | 2-16 | 4-32 GB | Shared, cost-optimized (EU only) |
| **CPX** | cpx11-cpx51 | 2-16 | 2-32 GB | Shared, AMD EPYC |
| **CCX** | ccx13-ccx63 | 2-48 | 8-192 GB | Dedicated vCPUs |

### ARM Servers

| Type | vCPU | RAM | Notes |
|------|------|-----|-------|
| cax11-cax41 | 2-16 | 4-32 GB | Ampere Altra (Germany, Finland only) |

### Locations

| Code | Location |
|------|----------|
| fsn1 | Falkenstein, Germany |
| nbg1 | Nuremberg, Germany |
| hel1 | Helsinki, Finland |
| ash | Ashburn, USA |
| hil | Hillsboro, USA |

</details>

<details>
<summary><strong>Cloudflare DNS Integration</strong></summary>

Full documentation: **[docs/configuration.md#cloudflare-dns-integration](docs/configuration.md#cloudflare-dns-integration)**

### Creating a Cloudflare API Token

1. Go to [Cloudflare API Tokens](https://dash.cloudflare.com/profile/api-tokens)
2. Click **"Create Token"**
3. Use the **"Edit zone DNS"** template (or create custom)
4. Configure permissions:
   - `Zone > Zone > Read` — to find zone ID
   - `Zone > DNS > Edit` — to manage records
5. Set **Zone Resources** to your specific domain (recommended)

**For teams/CI:** Use Account Owned Tokens at *Manage Account → Account API Tokens* (persists when members leave)

### Configuration

```bash
export CF_API_TOKEN="your-cloudflare-token"
export CF_DOMAIN="example.com"
```

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

1. **external-dns** watches Ingress resources → creates DNS A records
2. **cert-manager** uses DNS01 challenge → issues Let's Encrypt certificates
3. Certificates auto-renew

### Example Ingress

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-app
  annotations:
    external-dns.alpha.kubernetes.io/hostname: app.example.com
    cert-manager.io/cluster-issuer: letsencrypt-cloudflare-production
spec:
  tls:
    - hosts: ["app.example.com"]
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

</details>

<details>
<summary><strong>CLI Commands</strong></summary>

| Command | Description |
|---------|-------------|
| `k8zner init` | Interactive wizard to create cluster.yaml |
| `k8zner init --advanced` | Wizard with networking/security options |
| `k8zner apply -c cluster.yaml` | Create or update cluster |
| `k8zner destroy -c cluster.yaml` | Tear down all resources |
| `k8zner upgrade -c cluster.yaml` | Upgrade Talos/Kubernetes |
| `k8zner image build -c cluster.yaml` | Build Talos image snapshot |
| `k8zner image delete` | Delete image snapshots |
| `k8zner version` | Show version information |

</details>

<details>
<summary><strong>Upgrading</strong></summary>

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

# Rebuild image and apply
k8zner image build -c cluster.yaml
k8zner upgrade -c cluster.yaml
```

</details>

<details>
<summary><strong>Troubleshooting</strong></summary>

### API Token Errors

```bash
echo $HCLOUD_TOKEN
curl -H "Authorization: Bearer $HCLOUD_TOKEN" https://api.hetzner.cloud/v1/servers
```

### Node Not Joining

- Check firewall allows Talos API (port 50000)
- Verify network connectivity between nodes
- Inspect with `talosctl`

### Cluster Credentials

```bash
export KUBECONFIG=./secrets/<cluster-name>/kubeconfig
export TALOSCONFIG=./secrets/<cluster-name>/talosconfig
```

</details>

<details>
<summary><strong>Project Structure</strong></summary>

```
cmd/k8zner/
├── commands/         # CLI commands (Cobra)
└── handlers/         # Business logic

internal/
├── config/           # Configuration & wizard
├── orchestration/    # Workflow coordination
├── provisioning/     # Infrastructure provisioning
│   ├── infrastructure/   # Network, firewall, LBs
│   ├── compute/          # Servers
│   ├── cluster/          # K8s bootstrap
│   └── image/            # Talos images
├── addons/           # K8s addon installation
│   ├── helm/             # Chart rendering
│   └── k8sclient/        # Embedded K8s client
├── platform/
│   ├── hcloud/           # Hetzner API
│   ├── talos/            # Talos config
│   └── ssh/              # SSH client
└── util/             # Shared utilities
```

</details>

<details>
<summary><strong>Development</strong></summary>

```bash
make build          # Build binary
make test           # Run unit tests
make test-coverage  # With coverage
make lint           # Run linters
make check          # All checks
make test-e2e       # E2E tests (requires HCLOUD_TOKEN)
```

</details>

---

## Related Projects

- [terraform-hcloud-kubernetes](https://github.com/hcloud-k8s/terraform-hcloud-kubernetes) — Original Terraform module
- [Talos Linux](https://www.talos.dev/) — Secure, immutable Kubernetes OS
- [Cilium](https://cilium.io/) — eBPF-based networking
- [Hetzner Cloud](https://www.hetzner.com/cloud) — Affordable cloud infrastructure

## License

Apache License 2.0 — see [LICENSE](LICENSE)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.
