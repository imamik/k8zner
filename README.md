# k8zner

[![CI](https://github.com/imamik/k8zner/actions/workflows/ci.yaml/badge.svg)](https://github.com/imamik/k8zner/actions/workflows/ci.yaml)
[![codecov](https://codecov.io/gh/imamik/k8zner/branch/main/graph/badge.svg)](https://codecov.io/gh/imamik/k8zner)
[![Go Report Card](https://goreportcard.com/badge/github.com/imamik/k8zner)](https://goreportcard.com/report/github.com/imamik/k8zner)
[![Release](https://img.shields.io/github/v/release/imamik/k8zner)](https://github.com/imamik/k8zner/releases/latest)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

**k8zner** (k8s + Hetzner) — Operator-driven Kubernetes on [Hetzner Cloud](https://www.hetzner.com/cloud), designed for practical reliability and fast onboarding.

### Why k8zner?

Running Kubernetes shouldn't require a dedicated platform team. k8zner enables engineers to deploy **high-availability clusters with strong defaults** on Hetzner Cloud — one of the most cost-effective cloud providers — without heavy platform overhead.

- **From zero to HA-capable cluster** in minutes, not days
- **Day-one operations covered**: networking, storage, TLS, DNS, GitOps — all pre-configured
- **Bridge to application deployment**: built-in ArgoCD, ingress, and cert-manager get your apps running fast
- **Single binary**: No Terraform, kubectl, or talosctl required — just download and run
- **Opinionated defaults**: tested version matrix, x86-64 architecture, EU regions — fewer choices, more confidence

Built on [Talos Linux](https://www.talos.dev/), the secure and immutable Kubernetes OS.

> **Maturity statement:** k8zner is actively hardened with broad automated testing and operator-based reconciliation. It is suitable for serious non-trivial workloads, but we avoid claiming universal “battle-proof production” coverage until we publish longer-horizon reliability benchmarks and failure-injection evidence.

## Quick Start

```bash
# 1. Install
brew install imamik/tap/k8zner   # or: go install github.com/imamik/k8zner/cmd/k8zner@latest

# 2. Set your Hetzner Cloud API token
export HCLOUD_TOKEN="your-token"

# 3. Create and deploy
k8zner init              # Interactive wizard creates k8zner.yaml
k8zner apply             # Deploy your cluster (builds image automatically)

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

2. **Set environment variable:**
   ```bash
   export CF_API_TOKEN="your-cloudflare-api-token"
   ```

3. **Enter domain in wizard** or add to config:
   ```yaml
   domain: example.com
   ```

The simplified config automatically enables external-dns and cert-manager with Cloudflare when a domain is specified.

## Batteries Included

Every k8zner cluster includes production-ready components — no configuration needed:

| Category | Always Included |
|----------|-----------------|
| **Networking** | [Cilium](https://cilium.io/) (eBPF CNI, kube-proxy replacement, Hubble observability) |
| **Ingress** | [Traefik](https://traefik.io/) with [Gateway API](https://gateway-api.sigs.k8s.io/) support |
| **Cloud** | [Hetzner CCM](https://github.com/hetznercloud/hcloud-cloud-controller-manager) + [CSI](https://github.com/hetznercloud/csi-driver) (load balancers, volumes) |
| **TLS** | [cert-manager](https://cert-manager.io/) with Let's Encrypt |
| **GitOps** | [ArgoCD](https://argo-cd.readthedocs.io/) |
| **Metrics** | [Metrics Server](https://github.com/kubernetes-sigs/metrics-server) for HPA/VPA |

**Optional** (enabled via config):

| Feature | How to Enable |
|---------|---------------|
| **DNS automation** | Set `domain: example.com` + `CF_API_TOKEN` |
| **Monitoring stack** | Set `monitoring: true` (Prometheus, Grafana, Alertmanager) |
| **etcd backups** | Set `backup: true` + S3 credentials |

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
│  │   10.0.0.0/16                                                          │ │
│  │  ┌─────────────────────────────────────────────────────────────────┐   │ │
│  │  │                     Node Subnet (10.0.0.0/17)                    │   │ │
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

1. **Image** — Build/cache Talos Linux snapshot
2. **Infrastructure** — Create network, firewall, load balancers
3. **Bootstrap** — Provision first control plane and bootstrap Kubernetes
4. **Operator** — Install k8zner operator into the cluster
5. **CRD** — Create K8znerCluster resource (operator takes over)
6. **Reconcile** — Operator installs CNI, addons, scales control planes and workers

All operations are **idempotent** — re-running `apply` on an existing cluster updates the CRD spec.

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

### Network Encryption

Cilium with WireGuard encryption is enabled by default for all pod-to-pod traffic.

</details>

<details>
<summary><strong>Configuration Reference</strong></summary>

Full documentation: **[docs/configuration.md](docs/configuration.md)** | Interactive setup: **[docs/wizard.md](docs/wizard.md)**

k8zner uses a **simplified, opinionated configuration** — just 5 fields for a production-ready cluster:

### Minimal Configuration

```yaml
name: my-cluster
region: nbg1
mode: dev
workers:
  count: 1
  size: cx23
```

### Production HA Configuration

```yaml
name: production
region: fsn1
mode: ha
workers:
  count: 3
  size: cx33
domain: example.com      # Enables DNS + TLS via Cloudflare
```

### Full Configuration (all options)

```yaml
name: production
region: fsn1
mode: ha
workers:
  count: 3
  size: cx33

# Optional: Control plane size (defaults to cx23)
control_plane:
  size: cx23

# Optional: Cloudflare DNS & TLS
domain: example.com
cert_email: ops@example.com     # Let's Encrypt notifications
argo_subdomain: argocd          # ArgoCD at argocd.example.com

# Optional: Monitoring stack
monitoring: true                # Prometheus, Grafana, Alertmanager
grafana_subdomain: grafana      # Grafana at grafana.example.com

# Optional: etcd backups
backup: true                    # Requires HETZNER_S3_ACCESS_KEY/SECRET_KEY
```

### Configuration Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Cluster name (DNS-safe: lowercase, alphanumeric, hyphens) |
| `region` | Yes | Datacenter: `nbg1`, `fsn1`, or `hel1` |
| `mode` | Yes | `dev` (1 CP, 1 LB) or `ha` (3 CP, 2 LBs) |
| `workers.count` | Yes | Number of workers (1-5) |
| `workers.size` | Yes | Server type (see table below) |
| `control_plane.size` | No | Control plane server type (default: `cx23`) |
| `domain` | No | Cloudflare domain for DNS/TLS |
| `monitoring` | No | Enable Prometheus/Grafana stack |
| `backup` | No | Enable etcd backups to S3 |

All infrastructure settings (versions, networking, addons) use tested, production-ready defaults.

</details>

<details>
<summary><strong>Hetzner Server Types</strong></summary>

### Server Sizes

k8zner supports both dedicated vCPU (CX) and shared vCPU (CPX) instances:

#### CX Series - Dedicated vCPU (Default)
Consistent performance, recommended for production:

| Size | vCPU | RAM | Disk | Price |
|------|------|-----|------|-------|
| `cx23` | 2 | 4 GB | 40 GB | ~€4/mo |
| `cx33` | 4 | 8 GB | 80 GB | ~€8/mo |
| `cx43` | 8 | 16 GB | 160 GB | ~€16/mo |
| `cx53` | 16 | 32 GB | 320 GB | ~€30/mo |

#### CPX Series - Shared vCPU
Better availability, suitable for dev/test:

| Size | vCPU | RAM | Disk | Price |
|------|------|-----|------|-------|
| `cpx22` | 2 | 4 GB | 40 GB | ~€4.50/mo |
| `cpx32` | 4 | 8 GB | 80 GB | ~€8.50/mo |
| `cpx42` | 8 | 16 GB | 160 GB | ~€15.50/mo |
| `cpx52` | 16 | 32 GB | 320 GB | ~€29.50/mo |

Control planes default to `cx23` (2 dedicated vCPU, 4GB RAM - sufficient for etcd + API server).

**Note:** k8zner supports x86-64 (amd64) architecture only. ARM servers (CAX) are not supported.

### Regions

k8zner supports EU regions only (where CX instances are available):

| Code | Location |
|------|----------|
| `fsn1` | Falkenstein, Germany |
| `nbg1` | Nuremberg, Germany |
| `hel1` | Helsinki, Finland |

US regions (Ashburn, Hillsboro) are not supported as they lack CX instance types.

</details>

<details>
<summary><strong>Cloudflare DNS Integration</strong></summary>

Full documentation: **[docs/configuration.md](docs/configuration.md)**

### Setup

1. Go to [Cloudflare API Tokens](https://dash.cloudflare.com/profile/api-tokens)
2. Click **"Create Token"** → Use **"Edit zone DNS"** template
3. Set permissions: `Zone > Zone > Read` + `Zone > DNS > Edit`
4. Scope to your specific domain

### Configuration

```bash
export CF_API_TOKEN="your-cloudflare-token"
```

```yaml
name: my-cluster
region: fsn1
mode: ha
workers:
  count: 3
  size: cx33
domain: example.com  # Just add this — DNS and TLS are automatic
```

### What You Get

When `domain` is set, k8zner automatically enables:
- **external-dns**: Creates DNS records from Ingress/Gateway resources
- **cert-manager + Cloudflare DNS01**: Issues Let's Encrypt certificates
- **ArgoCD dashboard**: Accessible at `argo.{domain}` with TLS

### Example Ingress

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-app
  annotations:
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

DNS records are created automatically via external-dns.

</details>

<details>
<summary><strong>CLI Commands</strong></summary>

| Command | Description |
|---------|-------------|
| `k8zner init` | Interactive wizard to create k8zner.yaml |
| `k8zner apply` | Create or update cluster (operator-managed) |
| `k8zner destroy` | Tear down all resources |
| `k8zner doctor` | Diagnose cluster configuration and status |
| `k8zner secrets` | Retrieve cluster credentials (kubeconfig, ArgoCD, Grafana) |
| `k8zner cost` | Calculate monthly cluster costs with Hetzner pricing |
| `k8zner version` | Show version information |

</details>

<details>
<summary><strong>Upgrading</strong></summary>

k8zner uses a pinned, tested version matrix (currently Talos v1.9.0, Kubernetes v1.32.0).

To upgrade your cluster, update your config and re-apply:

```bash
# 1. Update k8zner binary
brew upgrade k8zner  # or reinstall

# 2. Re-apply to update cluster (operator handles rolling upgrades)
k8zner apply
```

The operator handles version updates with rolling node upgrades.

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
cmd/
├── k8zner/
│   ├── commands/         # CLI commands (Cobra): init, apply, destroy, doctor
│   └── handlers/         # Business logic for each command
├── operator/             # Kubernetes operator entrypoint
└── cleanup/              # Standalone cleanup utility

internal/
├── operator/             # Kubernetes operator
│   ├── controller/       # CRD reconciliation (phases, scaling, healing)
│   ├── provisioning/     # CRD spec → config adapter
│   └── addons/           # Operator addon phase manager
├── config/               # Configuration handling
├── provisioning/         # Infrastructure provisioning (shared by CLI + operator)
│   ├── infrastructure/   # Network, firewall, LBs
│   ├── compute/          # Servers, node pools
│   ├── image/            # Talos image building
│   ├── cluster/          # K8s bootstrap
│   └── destroy/          # Resource teardown
├── addons/               # K8s addon installation (shared by CLI + operator)
│   ├── helm/             # Chart rendering and value building
│   └── k8sclient/        # Kubernetes API operations
├── platform/
│   ├── hcloud/           # Hetzner API (generic Delete/Ensure operations)
│   ├── talos/            # Talos config and patches
│   ├── ssh/              # SSH client
│   └── s3/               # S3/backup integration
└── util/                 # Shared utilities (async, naming, labels, retry, rdns, keygen)

api/v1alpha1/             # CRD types (K8znerCluster spec, status, phases)
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
make e2e            # E2E tests (requires HCLOUD_TOKEN)
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
