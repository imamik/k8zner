# Technical Design Document: Refactoring to Pure Go CLI

## 1. Introduction

This document describes the design for refactoring the current infrastructure-as-code repository into a pure Go CLI tool. The goal is to create a self-contained binary that provisions a Talos Kubernetes cluster on Hetzner Cloud without external dependencies (except for the CLI itself).

## 2. Goals

*   **Self-Contained:** A single binary with no external runtime dependencies (terraform, packer, hcloud-cli, etc.).
*   **Pure Go:** All logic implemented in Go.
*   **Hetzner & Talos:** Full support for provisioning infrastructure on Hetzner and bootstrapping Talos Linux.
*   **E2E Testing:** Integrated end-to-end testing capabilities using `HCLOUD_TOKEN`.
*   **Modular Architecture:** Clean separation of concerns for maintainability and testing.
*   **Idempotency:** The CLI should act as a reconciler, ensuring the actual state matches the desired state without relying on a fragile local state file.
*   **Feature Parity:** Full support for all current Terraform capabilities (RBAC, OIDC, Autoscaler, Backups, Ingress, Cilium, etc.).

## 3. Architecture

The CLI will be built using the `cobra` framework. The architecture allows for distinct packages handling different domains.

### 3.1. Directory Structure

```
.
├── cmd/
│   └── hcloud-k8s/
│       └── main.go       # Entry point
├── internal/
│   ├── cluster/          # Cluster orchestration logic (The "Controller")
│   ├── config/           # Configuration parsing and validation
│   ├── hcloud/           # Wrapper around hcloud-go (Infrastructure Provider)
│   ├── talos/            # Wrapper around talos/pkg/machinery and client (OS Provisioner)
│   ├── k8s/              # Kubernetes client wrapper (Addon Manager)
│   ├── manifests/        # Generators for K8s resources (RBAC, OIDC, Autoscaler secrets)
│   ├── addons/           # Logic for specific addons (Cilium, Longhorn, etc.)
│   ├── ssh/              # SSH utilities
│   └── utils/            # Helper functions
├── pkg/
│   └── ...               # Exportable packages (if any)
├── go.mod
└── go.sum
```

### 3.2. Key Libraries

*   **CLI Framework:** `github.com/spf13/cobra`
*   **Hetzner Cloud:** `github.com/hetznercloud/hcloud-go/v2/hcloud`
*   **Talos Config:** `github.com/siderolabs/talos/pkg/machinery`
*   **Kubernetes:** `k8s.io/client-go` (dynamic client), `k8s.io/api` (types)
*   **YAML:** `sigs.k8s.io/yaml`
*   **SSH:** `golang.org/x/crypto/ssh`

### 3.3. State Management Strategy: Label-Based Reconciliation

Resources will be identified using deterministic labels to avoid local state files:
*   `cluster=<cluster_name>`
*   `role=<control-plane|worker|ingress|kube-api>`
*   `component=<load-balancer|firewall|network|placement-group>`
*   `nodepool=<nodepool_name>`

Reconciliation Logic:
*   **Create:** If label query returns 0 items, create resource.
*   **Update:** If resource exists, compare properties (e.g., Firewall rules, LB targets) and update if drifted.
*   **Delete:** Find resources by `cluster` label and delete.

## 4. Implementation Steps & Feature Parity

### Step 1: Image Builder (Replaces Packer)

**Goal:** Create a Talos snapshot on Hetzner.

*   **Logic:**
    1.  Provision temporary server (Debian).
    2.  Enable/Boot Rescue Mode.
    3.  SSH and write `talos.raw.xz` to `/dev/sda`.
    4.  Create Snapshot with labels `os=talos`, `arch=<amd64|arm64>`, `version=<talos_version>`.
    5.  Cleanup temporary server.

### Step 2: Base Infrastructure (Replaces Infrastructure Terraform)

**Goal:** Provision networking, security, and load balancing.

*   **Logic:**
    1.  **Network:** Ensure private network and subnets exist.
        *   Subnets: Control Plane, Load Balancer, Workers (1 per nodepool), Autoscaler.
    2.  **Firewall:**
        *   Dynamic "Current IP" rules (detect local public IP via `icanhazip.com`).
        *   Allow rules: Kube API (6443), Talos API (50000), Wireguard (51820/udp).
    3.  **Load Balancers:**
        *   **Control Plane LB:** Port 6443 -> Nodes 6443. Private Network enabled.
        *   **Ingress LB:** (Optional) Managed via `hcloud-ccm` annotations or explicitly provisioned if configured (Pools logic from `load_balancer.tf`).
    4.  **Placement Groups:**
        *   Control Plane: Type `spread`.
        *   Workers: Partitioned (1 PG per 10 nodes) to avoid Hetzner limits.
    5.  **Floating IPs:** Create if `control_plane_public_vip_ipv4_enabled` is true.

### Step 3: Server Provisioning & Talos Config (Replaces Server Terraform & Talos Config)

**Goal:** Create servers and generate machine configurations.

*   **Logic:**
    1.  **Config Generation (`talos/pkg/machinery`):**
        *   **SANs:** Generate dynamic Subject Alternative Names including LB IPs, Floating IPs, and Node IPs.
        *   **Network:** Configure eth0 (Public) and eth1 (Private) interfaces.
        *   **Encryption:** Enable System Disk Encryption (`state`, `ephemeral`) via LUKS.
        *   **Registries:** Inject Registry Mirrors if configured.
        *   **Extra Mounts:** Inject Kubelet mounts (e.g., `/var/lib/longhorn`).
    2.  **Provisioning:**
        *   Create servers from Step 1 Snapshot.
        *   Attach to Network, Firewall, and Placement Groups.
        *   Generate RDNS entries based on templates (`{{ hostname }}`, `{{ role }}`).
    3.  **Secrets Management:**
        *   Generate Talos secrets (certs, tokens) in-memory or load from file.
        *   *No Terraform State:* Secrets must be saved to a secure local file (e.g., `cluster-secrets.yaml`) or Keyring.

### Step 4: Bootstrap & Cluster Formation (Replaces Talos Bootstrap)

**Goal:** Form a working Kubernetes cluster.

*   **Logic:**
    1.  Push config to first Control Plane node (Maintenance Mode).
    2.  Call `bootstrap` API.
    3.  Push config to remaining nodes.
    4.  Retrieve `admin.kubeconfig` and save locally.
    5.  Wait for node readiness.

### Step 5: Features & Addons (Replaces Addon Terraform)

**Goal:** Install complex components via `client-go` (Manifests/Helm).

*   **Logic:**
    1.  **Hetzner CCM:**
        *   Create Secret `hcloud` with token.
        *   Apply Deployment manifest.
    2.  **Hetzner CSI:** Apply Manifests.
    3.  **Cilium CNI:**
        *   Install via Helm Chart.
        *   Handle IPSec Key generation (random bytes) and Secret creation if encryption enabled.
    4.  **RBAC:**
        *   Generate `Role` and `ClusterRole` manifests from config.
    5.  **OIDC:**
        *   Generate `ClusterRoleBinding` and `RoleBinding` manifests mapping OIDC groups.
    6.  **Autoscaler:**
        *   Generate `cluster-autoscaler-hetzner-config` Secret.
        *   *Crucial Detail:* This secret must contain the **full Talos Machine Config** (cloud-init) for future autoscaled nodes, generated dynamically for the autoscaler nodepools.
        *   Deploy Autoscaler Helm Chart.
    7.  **Backups:**
        *   Deploy `talos-backup` CronJob, ServiceAccount, and S3 Secrets.
    8.  **Ingress NGINX:**
        *   Deploy via Helm.
        *   Handle logic for `Deployment` vs `DaemonSet` and Load Balancer annotations.
    9.  **Cert Manager:** Deploy via Helm (CRDs enabled).
    10. **Metrics Server:** Deploy via Helm.

### Step 6: Lifecycle (Upgrade & Destroy)

**Goal:** Robust Day-2 operations.

*   **Upgrade Logic (Finite State Machine):**
    1.  **Check:** Compare running Talos version/schematic vs desired.
    2.  **Loop:** For each node:
        *   `talosctl upgrade` (API call).
        *   Wait for reboot.
        *   Wait for `Ready` state and health check success.
    3.  **K8s Upgrade:** Call `upgrade-k8s` API endpoint on controller.
*   **Destroy Logic:**
    *   Query all resources by `cluster=<name>` label.
    *   Delete in order: Servers -> LBs -> Floating IPs -> Networks -> Placement Groups -> Firewalls.

## 5. Testing Strategy

*   **Unit Tests:** Config generation, Manifest generation.
*   **Integration Tests (E2E):**
    *   Use `HCLOUD_TOKEN`.
    *   Provisions real resources.
    *   Flow: Build Image -> Create Minimal Cluster -> Verify API -> Upgrade -> Destroy.

## 6. Configuration Schema

```yaml
clusterName: "talos-k8s"
hetzner:
  region: "hel1"
  networkZone: "eu-central"
  sshKeys: ["my-key"]
  firewall:
    apiSource: ["0.0.0.0/0"]
nodes:
  controlPlane:
    count: 3
    type: "cpx21"
    floatingIp: true
  workers:
    nodepools:
      - name: "worker-1"
        count: 3
        type: "cpx21"
        placementGroup: true
talos:
  version: "v1.9.0"
  extensions: []
  backups:
    s3:
      enabled: true
      bucket: "my-backup"
kubernetes:
  version: "1.32.0"
  oidc:
    enabled: true
    issuerUrl: "..."
    groupMappings:
      - group: "admins"
        clusterRoles: ["cluster-admin"]
  rbac:
    roles: ...
cni:
  type: "cilium"
  encryption: "ipsec" # Generates keys automatically
autoscaler:
  enabled: true
  nodepools:
    - name: "autoscaler-1"
      min: 0
      max: 5
      type: "cpx21"
ingress:
  nginx:
    enabled: true
    kind: "Deployment"
    replicas: 2
```
