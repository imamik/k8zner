# Technical Design Document: Refactoring to Pure Go CLI

## 1. Introduction

This document describes the design for refactoring the current Terraform and Packer-based repository into a pure Go CLI tool. The goal is to create a self-contained binary that provisions a Talos Kubernetes cluster on Hetzner Cloud without external dependencies (except for the CLI itself).

## 2. Goals

*   **Self-Contained:** A single binary with no external runtime dependencies (terraform, packer, hcloud-cli, etc.).
*   **Pure Go:** All logic implemented in Go.
*   **Hetzner & Talos:** Full support for provisioning infrastructure on Hetzner and bootstrapping Talos Linux.
*   **E2E Testing:** Integrated end-to-end testing capabilities using `HCLOUD_TOKEN`.
*   **Modular Architecture:** Clean separation of concerns for maintainability and testing.
*   **Idempotency:** The CLI should act as a reconciler, ensuring the actual state matches the desired state without relying on a fragile local state file.
*   **Full Feature Parity:** Support all features currently handled by Terraform, including OIDC, RBAC, Autoscaler, Backups, and advanced Network configuration.

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
│   ├── manifests/        # Logic to generate K8s manifests (RBAC, OIDC, etc.)
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
*   **Kubernetes:** `k8s.io/client-go` (for applying manifests/charts), `k8s.io/api` (for types)
*   **YAML:** `sigs.k8s.io/yaml`
*   **SSH:** `golang.org/x/crypto/ssh`

### 3.3. State Management Strategy: Label-Based Reconciliation

Unlike Terraform, we will **not** maintain a local state file (`.tfstate`). Instead, we will treat the Hetzner Cloud API as the source of truth.

*   **Discovery:** Resources will be identified using deterministic labels:
    *   `cluster=<cluster_name>`
    *   `role=<control-plane|worker>`
    *   `component=<load-balancer|firewall|network|placement-group>`
    *   `nodepool=<nodepool_name>`
*   **Reconciliation:**
    *   **Create:** Check if resource with labels exists. If not, create it.
    *   **Update:** Check if resource properties match config (e.g., Firewall rules). If not, update it.
    *   **Destroy:** List all resources with `cluster=<cluster_name>` and delete them.

## 4. Implementation Steps & Feature Parity

The refactoring will be executed in the following phases. Each phase includes validation steps.

### Step 1: Image Builder (Replaces Packer)

**Goal:** Create a Talos snapshot on Hetzner for both AMD64 and ARM64 architectures.

*   **Logic:**
    1.  Provision a temporary server (e.g., `cpx11` for AMD64, `cax11` for ARM64) with a stock Linux image (Debian).
    2.  Enable Rescue Mode via Hetzner API.
    3.  Reset the server to boot into Rescue Mode.
    4.  Establish SSH connection.
    5.  Download the specific Talos raw disk image (`.raw.xz`) corresponding to the architecture.
    6.  Write image to disk: `xz -d -c talos.raw.xz | dd of=/dev/sda && sync`.
    7.  Create a Snapshot of the server with labels `os=talos`, `arch=<amd64|arm64>`, `version=<talos_version>`.
    8.  Delete the temporary server.
*   **Validation:** Create a server from the snapshot and verify it boots (e.g., via Ping or opening port 50000).

### Step 2: Base Infrastructure (Replaces Infrastructure Terraform)

**Goal:** Provision networking and security resources.

*   **Logic:**
    1.  **Network:** Ensure a private network exists with the defined CIDR.
    2.  **Firewall:** Create/Update Firewalls based on `firewall.tf` logic.
        *   Dynamic rule generation for "current IP" (public access).
        *   Rules for Kube API (6443), Talos API (50000).
        *   Wireguard/Cilium ports.
    3.  **Load Balancer:** Provision LB for Control Plane (Port 6443 -> Nodes 6443).
    4.  **Placement Groups:** Implement the spread logic from `placement_group.tf`.
        *   One PG for Control Plane (Spread).
        *   Multiple PGs for Workers (1 per 10 nodes) to avoid Hetzner limits.
    5.  **Floating IPs:** Create/Assign Floating IPs if `control_plane_public_vip_ipv4_enabled` is true.
*   **Validation:** Verify resources exist in Hetzner via API or Console.

### Step 3: Server Provisioning & Talos Config (Replaces Server Terraform & Talos Config)

**Goal:** Create servers and generate machine configurations.

*   **Logic:**
    1.  **Config Generation:** Use `talos/pkg/machinery`.
        *   **Patches:** Apply config patches for:
            *   Network interfaces (VIPs, Routes).
            *   Kernel modules/args.
            *   Kubelet extra mounts (e.g., Longhorn).
            *   Sysctls.
            *   Cluster Discovery (K8s vs Service).
    2.  **Provisioning:** Create servers using the Snapshots from Step 1.
        *   Attach to Network, Firewall, and correct Placement Group.
        *   Generate and assign RDNS entries (reverse DNS) based on `rdns.tf` patterns.
    3.  **Bootstrap Config:**
        *   Wait for server to boot.
        *   Use `talosctl` (via Go library) to push the generated config to the node in "Maintenance Mode" (Port 50000).
*   **Validation:** Servers are running, configured, and `talosctl health` reports distinct status (e.g., "Ready" or "Booting").

### Step 4: Bootstrap & Cluster Formation (Replaces Talos Bootstrap)

**Goal:** Form a working Kubernetes cluster.

*   **Logic:**
    1.  Identify the first Control Plane node.
    2.  **Bootstrap:** Call the Talos Bootstrap API on this node.
    3.  **Kubeconfig:** Retrieve the `admin.kubeconfig` from the node once bootstrapped.
    4.  **Wait:** Poll `client-go` until all nodes appear in `Ready` state (or at least registered).
*   **Validation:** `kubectl get nodes` returns the list of nodes.

### Step 5: Features & Addons (Replaces Addon Terraform)

**Goal:** Install all complex components managed by Terraform.

*   **Logic:**
    1.  **RBAC:**
        *   Generate Manifests from config (Roles/ClusterRoles).
        *   Apply using `client-go` dynamic client.
    2.  **OIDC:**
        *   Configure `kube-apiserver` flags in Talos config.
        *   Generate `ClusterRoleBinding` and `RoleBinding` manifests mapping OIDC groups to K8s roles (logic from `oidc.tf`).
    3.  **Autoscaler:**
        *   Generate `cluster-config` Secret containing cloud-init for new nodes.
        *   Deploy Cluster Autoscaler Helm Chart (or Manifests).
    4.  **Backups (Talos Backup):**
        *   Deploy `talos-backup` CronJob, ServiceAccount, and S3 Secrets.
        *   Inject `os:etcd:backup` role into Talos machine config.
    5.  **Hetzner Integrations:**
        *   **CCM:** Apply Secret (Token) and Deployment.
        *   **CSI:** Apply CSI Driver manifests.
    6.  **CNI (Cilium):** Install via Helm Chart.
*   **Validation:** `kubectl get pods -A` shows all system pods running. Verify RBAC bindings exist.

### Step 6: Full E2E & Lifecycle

**Goal:** Complete CLI with destroy and upgrade commands.

*   **Logic:**
    1.  **Destroy:** Query all resources with cluster label and delete them in dependency order (Servers -> LBs -> Networks).
    2.  **Upgrade:** Implement rolling upgrade logic:
        *   Cordon/Drain node.
        *   Call Talos Upgrade API (image update).
        *   Wait for reboot and health.
        *   Uncordon.
        *   *Edge Case:* Handle "force" upgrades or skipping health checks if configured.
*   **Validation:** Run the full suite: Create -> Validate -> Upgrade -> Destroy.

## 5. Testing Strategy

*   **Unit Tests:**
    *   Test Config generation (YAML output).
    *   Test Label selector logic.
    *   Test RBAC/OIDC manifest generation logic.
*   **Integration Tests:**
    *   Requires `HCLOUD_TOKEN`.
    *   **Test 1: Image Build:** Build a snapshot, verify it exists, delete it.
    *   **Test 2: Minimal Cluster:** Build 1 CP node, bootstrap, verify K8s API, destroy.
    *   **Test 3: Full Cluster:** HA Control Plane (3 nodes), Workers, OIDC, Autoscaler, destroy.

## 6. Configuration

The CLI will accept a configuration file (YAML) that maps to the Terraform variables.

```yaml
clusterName: "talos-k8s"
hetzner:
  region: "hel1"
  networkZone: "eu-central"
  sshKeys: ["my-key"]
  firewall:
    apiSource: ["0.0.0.0/0"] # Replaces firewall_kube_api_source
nodes:
  controlPlane:
    count: 3
    type: "cpx21"
    arch: "amd64"
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
      # Secrets loaded from env vars or separate file
kubernetes:
  version: "1.32.0"
  oidc:
    enabled: true
    issuerUrl: "https://accounts.google.com"
    clientId: "..."
    groupsPrefix: "oidc:"
    groupMappings:
      - group: "devs"
        clusterRoles: ["view"]
  rbac:
    roles:
      - name: "pod-reader"
        namespace: "default"
        rules: ...
cni:
  type: "cilium"
autoscaler:
  enabled: true
  nodepools: ...
```
