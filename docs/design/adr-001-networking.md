# ADR-001: Networking Architecture on Hetzner + Talos

## Status

Accepted (2026-02-08)

## Context

Running Kubernetes on Hetzner Cloud with Talos Linux requires specific networking decisions. Key constraints:

- Talos Linux prevents non-root port binding even with `NET_BIND_SERVICE` + `allowPrivilegeEscalation`
- Hetzner uses predictable PCI interface names (`enp1s0`, `enp7s0`), not `eth0`/`eth1`
- Cilium kube-proxy replacement requires explicit device configuration
- Health checks from Hetzner LBs traverse the private network

## Decisions

### 1. Traefik: Always LoadBalancer (never hostNetwork)

**Decision**: Traefik runs as a `Deployment` with a `LoadBalancer` service. Hetzner CCM auto-creates the LB via annotations.

**Why**: Talos Linux drops all capabilities and sets `allowPrivilegeEscalation: false` in its security context. Even with `NET_BIND_SERVICE`, non-root containers cannot bind ports 80/443. All major Hetzner K8s projects (kube-hetzner, terraform-hcloud-kubernetes) use the same approach.

**Consequences**:
- No firewall rules needed for 80/443 on nodes
- external-dns discovers LB IP from Ingress `.status.loadBalancer.ingress`
- `externalTrafficPolicy: Cluster` (default) avoids health check node port issues

### 2. Cilium: Explicit Device Configuration for Hetzner+Talos

**Decision**: Cilium is configured with:
- `devices: "enp+"` — match all PCI ethernet interfaces
- `nodePort.directRoutingDevice: "enp1s0"` — required filter when `kubeProxyReplacement: true`
- `loadBalancer.acceleration: "disabled"` — XDP/native incompatible with virtio
- `routingMode: "tunnel"` — VXLAN mode

**Why**: When `kubeProxyReplacement: true`, Cilium's `AreDevicesRequired()` returns true. If the `devices` pattern matches nothing (e.g., `eth+` on Talos), Cilium CrashLoops with "unable to determine direct routing device". Talos uses predictable PCI names, not the `eth0`/`eth1` convention.

**Consequences**:
- `directRoutingDevice` acts as a FILTER, not a direct assignment
- Native routing acceleration (`loadBalancer.acceleration: "native"`) is incompatible with virtio NICs
- All Hetzner LB health checks work because eBPF handles NodePort on detected devices

### 3. TLS: cert-manager with DNS-01 (not Traefik built-in ACME)

**Decision**: cert-manager handles all TLS via Cloudflare DNS-01 challenge. Traefik's built-in ACME is disabled.

**Why**: Traefik's built-in ACME stores certificates in a local file, which only works for single-instance deployments. In HA with multiple Traefik replicas, each would independently request certificates, causing rate limit issues. cert-manager stores certificates as Kubernetes Secrets, shared across all replicas.

**Consequences**:
- cert-manager creates `kubernetes.io/tls` Secrets referenced by Ingress `spec.tls[].secretName`
- Traefik's `kubernetesIngress` provider watches Secrets for TLS termination
- kubernetesCRD provider is DISABLED (standard Ingress only, no IngressRoute CRDs)

### 4. Network CIDRs: Single Source of Truth

**Decision**: All CIDR defaults are defined as constants in `internal/config/v2/versions.go` and referenced by both `v2.Expand()` and `SpecToConfig()`.

**Why**: The project has two config paths (CLI v2 YAML and Operator CRD). Hardcoding CIDRs as string literals in both paths caused a drift bug where `buildMachineConfigOptions()` used `10.244.0.0/16` (generic k8s default) while the network config used `10.0.128.0/17` (project default).

**Consequences**:
- Pod CIDR (`10.0.128.0/17`) must be WITHIN the network CIDR (`10.0.0.0/16`) for Cilium native routing
- Service CIDR (`10.96.0.0/12`) is outside the network CIDR but handled by Cilium's kube-proxy replacement
- Changes to CIDRs only need to happen in one place

## References

- All major Hetzner K8s projects use LoadBalancer, not hostNetwork
- kube-hetzner defaults to `externalTrafficPolicy: Cluster`
- Cilium docs: [Device Detection](https://docs.cilium.io/en/stable/network/concepts/networking/native-routing/)
