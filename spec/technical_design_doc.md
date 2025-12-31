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
*   **Kubernetes:** `k8s.io/client-go` (for applying manifests/charts)
*   **YAML:** `sigs.k8s.io/yaml`
*   **SSH:** `golang.org/x/crypto/ssh`

### 3.3. State Management Strategy: Label-Based Reconciliation

Unlike Terraform, we will **not** maintain a local state file (`.tfstate`). Instead, we will treat the Hetzner Cloud API as the source of truth.

*   **Discovery:** Resources will be identified using deterministic labels:
    *   `cluster=<cluster_name>`
    *   `role=<control-plane|worker>`
    *   `component=<load-balancer|firewall|network>`
*   **Reconciliation:**
    *   **Create:** Check if resource with labels exists. If not, create it.
    *   **Update:** Check if resource properties match config (e.g., Firewall rules). If not, update it.
    *   **Destroy:** List all resources with `cluster=<cluster_name>` and delete them.

This approach ensures the CLI is robust and stateless (on the client side).

## 4. Implementation Steps

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
    2.  **Firewall:** Create/Update Firewalls.
        *   *Control Plane:* Allow API (6443), Talos API (50000), Etcd (2379-2380) within private net.
        *   *Workers:* Allow Node ports, Cilium/VXLAN ports.
    3.  **Load Balancer:** Provision LB for Control Plane (Port 6443 -> Nodes 6443).
    4.  **Placement Groups:** Create spread groups for HA.
*   **Validation:** Verify resources exist in Hetzner via API or Console.

### Step 3: Server Provisioning & Talos Config (Replaces Server Terraform & Talos Config)

**Goal:** Create servers and generate machine configurations.

*   **Logic:**
    1.  **Config Generation:** Use `talos/pkg/machinery` to generate `controlplane.yaml` and `worker.yaml`.
        *   Configure SANs (LB IP, Floating IPs).
        *   Configure Cluster Discovery.
        *   Inject secrets (generated in memory or loaded from a secure file).
    2.  **Provisioning:** Create servers using the Snapshots from Step 1.
        *   Attach to Network, Firewall, and Placement Groups.
        *   *Note:* We cannot easily inject user-data into the raw image snapshot on Hetzner for Talos to auto-consume without a config partition.
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

### Step 5: Addons & Features (Replaces Addon Terraform)

**Goal:** Install CCM, CNI, and other components.

*   **Logic:**
    1.  **Hetzner CCM:** Apply the Secret (HCloud Token) and the Deployment manifests using `client-go`.
    2.  **CSI:** Apply the Hetzner CSI driver manifests.
    3.  **CNI (Cilium):** Install Cilium (via Helm Charts or manifests). *Note: Ensure strict compatibility with Talos (e.g., /run/cilium/cgroupv2 mounts).*
    4.  **CSR Approver:** Talos auto-approves Kubelet CSRs, but verify if `kubelet-serving` certs need the HCloud CCM or a separate approver.
*   **Validation:** `kubectl get pods -A` shows all system pods running. PVC creation test for CSI.

### Step 6: Full E2E & Lifecycle

**Goal:** Complete CLI with destroy and upgrade commands.

*   **Logic:**
    1.  **Destroy:** Query all resources with cluster label and delete them in dependency order (Servers -> LBs -> Networks).
    2.  **Upgrade:** Implement rolling upgrade logic:
        *   Cordon/Drain node.
        *   Call Talos Upgrade API (image update).
        *   Wait for reboot and health.
        *   Uncordon.
*   **Validation:** Run the full suite: Create -> Validate -> Destroy.

## 5. Testing Strategy

*   **Unit Tests:**
    *   Test Config generation (YAML output).
    *   Test Label selector logic.
*   **Integration Tests:**
    *   Requires `HCLOUD_TOKEN`.
    *   **Test 1: Image Build:** Build a snapshot, verify it exists, delete it.
    *   **Test 2: Minimal Cluster:** Build 1 CP node, bootstrap, verify K8s API, destroy.
    *   **Test 3: Full Cluster:** HA Control Plane (3 nodes), Workers, CCM/CNI, destroy.

## 6. Configuration

The CLI will accept a configuration file (YAML).

```yaml
clusterName: "talos-k8s"
hetzner:
  region: "hel1"
  networkZone: "eu-central"
  sshKeys:
    - "my-key"
nodes:
  controlPlane:
    count: 3
    type: "cpx21"
    arch: "amd64"
  workers:
    count: 3
    type: "cpx21"
    arch: "amd64"
talos:
  version: "v1.9.0"
kubernetes:
  version: "1.32.0"
cni:
  type: "cilium"
```
