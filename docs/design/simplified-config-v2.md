# Simplified Config Schema v2

## Design Principles

1. **One validated path** — No choices that require expertise
2. **Secure by default** — Firewall blocks all inbound except LB, TLS everywhere
3. **Batteries included** — All addons always installed
4. **Minimal surface area** — Fewer options = fewer bugs = easier maintenance

## User-Facing Config

```yaml
# k8zner.yaml — The entire config file
name: my-cluster

# Region: where to deploy
# Options: nbg1 (Nuremberg), fsn1 (Falkenstein), hel1 (Helsinki)
region: fsn1

# Mode: cluster topology
# - dev: 1 control plane, 1 shared LB (cheap, for development)
# - ha:  3 control planes, 2 separate LBs (production, highly available)
mode: ha

# Workers: compute capacity
workers:
  count: 3          # 1-5 nodes
  size: cx32        # cx22 | cx32 | cx42 | cx52

# Domain: enables automatic DNS + TLS (optional)
# Requires CF_API_TOKEN environment variable
domain: example.com
```

That's it. **12 lines** for a production-ready HA Kubernetes cluster.

## What Gets Hardcoded (Best Practices)

### Infrastructure
- **Control plane size**: CX22 (2 vCPU, 4GB) — sufficient for etcd + API server
- **Architecture**: AMD64 only — no ARM complexity
- **Node networking**: IPv6-only (no IPv4) — saves cost, smaller attack surface
- **Load balancer type**: LB11 — sufficient for most workloads
- **Load balancer networking**: IPv4 + IPv6 — accessible to all users
- **Load balancer topology**:
  - `dev` mode: 1 shared LB (K8s API on :6443, ingress on :80/:443)
  - `ha` mode: 2 separate LBs (dedicated API + dedicated ingress)

### Network Security (Automatic)
- **Nodes are IPv6-only** — No public IPv4 (saves cost, reduces attack surface)
- **Private network for cluster traffic** — Node-to-node, pod-to-pod (IPv4 10.x.x.x)
- **IPv6 for outbound internet** — Pulling images, external APIs
- **Firewall blocks ALL inbound** — On both IPv4 and IPv6
- **Load balancer is the only entry point** — Has IPv4+IPv6 for users, reaches nodes via private network

```
┌─────────────────────────────────────────────────────────────┐
│  Internet                                                    │
│    │                                                         │
│    ▼ (IPv4 + IPv6)                                          │
│  ┌─────────────────┐                                        │
│  │  Load Balancer  │  ◄── Only public entry point           │
│  │  (IPv4 + IPv6)  │                                        │
│  └────────┬────────┘                                        │
│           │ (private network)                               │
│           ▼                                                 │
│  ┌─────────────────────────────────────────────┐            │
│  │         Private Network (10.0.0.0/16)       │            │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐     │            │
│  │  │  Node   │  │  Node   │  │  Node   │     │            │
│  │  │ (IPv6)  │  │ (IPv6)  │  │ (IPv6)  │     │            │
│  │  │ no IPv4 │  │ no IPv4 │  │ no IPv4 │     │            │
│  │  └────┬────┘  └────┬────┘  └────┬────┘     │            │
│  │       │            │            │           │            │
│  │       └────────────┴────────────┘           │            │
│  │              (cluster traffic)              │            │
│  └─────────────────────────────────────────────┘            │
│                        │                                     │
│                        ▼ (IPv6 outbound only)               │
│                   Container Registries, APIs                 │
└─────────────────────────────────────────────────────────────┘
```

This is the most secure and cost-effective pattern.

### Kubernetes Stack (Always Installed)
- **CNI**: Cilium with kube-proxy replacement, Hubble observability
- **Ingress**: Traefik with automatic TLS
- **Storage**: Hetzner CSI with encrypted default StorageClass
- **DNS**: external-dns (when domain configured)
- **TLS**: cert-manager with Let's Encrypt (when domain configured)
- **Observability**: metrics-server, Prometheus Operator CRDs
- **GitOps**: ArgoCD (HA mode when cluster is HA)
- **Cloud integration**: Hetzner CCM, Talos CCM

### Version Matrix (Pinned & Tested)
```yaml
# internal/config/versions.go — Single source of truth
versions:
  talos: v1.9.0
  kubernetes: v1.32.0
  cilium: v1.16.5
  traefik: v3.2.0
  cert_manager: v1.16.0
  argocd: v2.13.0
  external_dns: v0.15.0
  metrics_server: v0.7.2
```

## Go Types

```go
// internal/config/types_v2.go

// Config is the simplified, opinionated configuration.
type Config struct {
    Name    string `yaml:"name"`             // Cluster name (required)
    Region  Region `yaml:"region"`           // Hetzner region (required)
    Mode    Mode   `yaml:"mode"`             // dev or ha (required)
    Workers Worker `yaml:"workers"`          // Worker configuration (required)
    Domain  string `yaml:"domain,omitempty"` // Base domain for DNS/TLS (optional)
}

// Region is a Hetzner datacenter location.
type Region string

const (
    RegionNuremberg   Region = "nbg1" // Nuremberg, Germany
    RegionFalkenstein Region = "fsn1" // Falkenstein, Germany
    RegionHelsinki    Region = "hel1" // Helsinki, Finland
)

// Mode defines the cluster topology and infrastructure choices.
type Mode string

const (
    // ModeDev: 1 control plane, 1 shared LB (K8s API + ingress on same LB)
    // Best for: development, testing, side projects
    // Cost: ~€15-25/mo
    ModeDev Mode = "dev"

    // ModeHA: 3 control planes, 2 separate LBs (dedicated API + ingress)
    // Best for: production workloads requiring high availability
    // Cost: ~€45-70/mo
    ModeHA Mode = "ha"
)

// Worker defines the worker pool configuration.
type Worker struct {
    Count int        `yaml:"count"` // 1-5 nodes
    Size  ServerSize `yaml:"size"`  // Instance type
}

// ServerSize is a Hetzner shared instance type (CX = shared vCPU).
type ServerSize string

const (
    SizeCX22 ServerSize = "cx22" // 2 vCPU,  4GB RAM,  40GB disk (~€4.35/mo)
    SizeCX32 ServerSize = "cx32" // 4 vCPU,  8GB RAM,  80GB disk (~€8.09/mo)
    SizeCX42 ServerSize = "cx42" // 8 vCPU, 16GB RAM, 160GB disk (~€15.59/mo)
    SizeCX52 ServerSize = "cx52" // 16 vCPU, 32GB RAM, 320GB disk (~€29.59/mo)
)

// Derived configuration (computed from Mode)
func (c *Config) ControlPlaneCount() int {
    if c.Mode == ModeDev {
        return 1
    }
    return 3
}

func (c *Config) LoadBalancerCount() int {
    if c.Mode == ModeDev {
        return 1 // Shared: API on :6443, ingress on :80/:443
    }
    return 2 // Separate: dedicated API LB + dedicated ingress LB
}
```

## Validation Rules

```go
func (c *Config) Validate() error {
    var errs []error

    // Name: required, DNS-safe (used for resource naming)
    if c.Name == "" {
        errs = append(errs, errors.New("name is required"))
    } else if !isValidDNSName(c.Name) {
        errs = append(errs, errors.New("name must be DNS-safe (lowercase, alphanumeric, hyphens)"))
    }

    // Region: must be valid Hetzner location
    if !c.Region.IsValid() {
        errs = append(errs, fmt.Errorf("region must be one of: nbg1, fsn1, hel1"))
    }

    // Mode: must be dev or ha
    if !c.Mode.IsValid() {
        errs = append(errs, fmt.Errorf("mode must be 'dev' or 'ha'"))
    }

    // Workers: count 1-5, valid size
    if c.Workers.Count < 1 || c.Workers.Count > 5 {
        errs = append(errs, errors.New("workers.count must be 1-5"))
    }
    if !c.Workers.Size.IsValid() {
        errs = append(errs, fmt.Errorf("workers.size must be one of: cx22, cx32, cx42, cx52"))
    }

    // Domain: if set, CF_API_TOKEN required
    if c.Domain != "" {
        if !isValidDomain(c.Domain) {
            errs = append(errs, errors.New("domain must be a valid domain name"))
        }
        if os.Getenv("CF_API_TOKEN") == "" {
            errs = append(errs, errors.New("CF_API_TOKEN environment variable required when domain is set"))
        }
    }

    return errors.Join(errs...)
}
```

Validation is trivial — only 5 fields to check.

## Cost Calculator Output

```
$ k8zner cost

┌─────────────────────────────────────────────────────────────┐
│  k8zner Cost Estimate                                       │
│  Cluster: my-cluster                                        │
├─────────────────────────────────────────────────────────────┤
│  Mode: ha                                                   │
│    • 3 control planes (CX22, IPv6-only)                     │
│    • 2 load balancers (API + ingress, IPv4+IPv6)            │
│  Workers: 3× CX32 (4 vCPU, 8GB each, IPv6-only)             │
│  Region: fsn1 (Falkenstein, Germany)                        │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Control Planes    3× CX22           €13.05/mo              │
│  Workers           3× CX32           €24.27/mo              │
│  Load Balancers    2× LB11           €12.82/mo              │
│  ───────────────────────────────────────────────────────────│
│  Subtotal                            €50.14/mo              │
│  VAT (19% DE)                         €9.53/mo              │
│  ───────────────────────────────────────────────────────────│
│  Total                               €59.67/mo              │
│                                                             │
│  Annual estimate: €716.04                                   │
│                                                             │
│  IPv6-only nodes save ~€3/mo vs IPv4                        │
└─────────────────────────────────────────────────────────────┘

  Prices from Hetzner API • EUR • Updated: just now

  💡 Tip: Use 'mode: dev' for development (~€21/mo with same workers)
```

## Migration Path

The new config is NOT backwards compatible. This is intentional.

For existing users:
1. Provide a `k8zner migrate` command that reads old config and outputs new format
2. Warn about removed options
3. Document the hardcoded best practices

## Comparison

| Aspect | Current | Simplified |
|--------|---------|------------|
| Config lines (minimal) | 28 | 8 |
| Config lines (full) | 250+ | 12 |
| Type definitions | 1042 lines | ~80 lines |
| User decisions | 50+ | 5 |
| Code paths to test | Many | Few |
| Documentation needed | Extensive | Minimal |

## What Users Lose

1. **ARM64 support** — Rarely needed, adds CI complexity
2. **Multiple worker pools** — One pool covers 95% of use cases
3. **Custom CNI choice** — Cilium is the modern standard
4. **Ingress choice** — Traefik is simpler, modern, auto-TLS
5. **Fine-grained addon config** — Best practices are baked in
6. **Custom network CIDRs** — Defaults work for everyone
7. **OIDC configuration** — Can be added post-install if needed
8. **Autoscaler** — Out of scope for v1, manual scaling 1-5
9. **LB topology choice** — Mode determines this (dev=shared, ha=separate)
10. **IPv4 on nodes** — IPv6-only is more secure and cheaper

## What Users Gain

1. **Simplicity** — 12 lines to production cluster
2. **Confidence** — One tested, hardened path
3. **Speed** — Less to configure = faster setup
4. **Security** — Best practices by default (IPv6-only nodes, firewall, TLS)
5. **Maintainability** — Easier upgrades, fewer breaking changes
6. **Cost transparency** — Built-in calculator
7. **Lower cost** — IPv6-only nodes save ~€0.50/node/mo on IPv4 addresses

## Environment Variables

```bash
# Required
HCLOUD_TOKEN=xxx      # Hetzner Cloud API token

# Optional (required if domain is set)
CF_API_TOKEN=xxx      # Cloudflare API token for DNS/TLS
```

## Example Configs

### Minimal Dev Cluster (~€15/mo)
```yaml
name: dev
region: fsn1
mode: dev
workers:
  count: 1
  size: cx22
```
- 1 control plane (CX22, IPv6-only)
- 1 worker (CX22, IPv6-only)
- 1 shared load balancer (IPv4+IPv6)
- Firewall: all inbound blocked except via LB

### Standard Dev Cluster (~€21/mo)
```yaml
name: my-project
region: fsn1
mode: dev
workers:
  count: 2
  size: cx22
domain: myproject.dev
```
- 1 control plane + 2 workers (all IPv6-only)
- 1 shared load balancer (IPv4+IPv6)
- Automatic DNS + TLS via Cloudflare

### Production HA Cluster (~€60/mo)
```yaml
name: production
region: fsn1
mode: ha
workers:
  count: 3
  size: cx32
domain: mycompany.com
```
- 3 control planes + 3 workers (all IPv6-only)
- 2 load balancers (IPv4+IPv6)
- HA etcd quorum, automatic DNS + TLS

### High-Performance HA (~€175/mo)
```yaml
name: enterprise
region: fsn1
mode: ha
workers:
  count: 5
  size: cx52
domain: bigco.io
```
- 3 control planes + 5 workers (all IPv6-only)
- 2 load balancers (IPv4+IPv6)
- Total: 80 vCPU, 160GB RAM across workers
