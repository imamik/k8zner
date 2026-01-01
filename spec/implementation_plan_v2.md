# Implementation Plan - Iteration 2: Cluster Infrastructure & Bootstrapping

## 1. Overview
This plan details the second increment of the `hcloud-k8s` project. Building upon the "Image Builder" foundation, this iteration focuses on **provisioning the actual Kubernetes cluster infrastructure** and **bootstrapping the control plane**.

At the end of this iteration, the CLI will be able to:
1.  Read a full cluster configuration file.
2.  Provision all Hetzner Cloud resources (Networks, Firewalls, Load Balancers, Placement Groups, Floating IPs, Servers).
3.  Generate valid Talos machine configurations for Control Plane and Worker nodes.
4.  Bootstrap the cluster and retrieve the `kubeconfig`.

## 2. Goals
1.  **Full Infrastructure Management:** Create/Update/Delete Hetzner resources based on config.
2.  **Label-Based Reconciliation:** Implement the "Controller" logic to manage state without a local state file.
3.  **Talos Integration:** Generate production-ready machine configs using `talos/pkg/machinery`.
4.  **Cluster Bootstrap:** Automate the bootstrap process to get a working API endpoint.
5.  **E2E Verification:** Expand E2E tests to provision a full cluster and verify `kubectl get nodes`.

## 3. Architecture & Design Principles
*   **Controller Pattern:** The core logic acts as a reconciler. It queries the current state (via Hetzner API and Labels), compares it with the desired state (Config), and applies changes.
*   **Dependency Injection:** The `ClusterController` will depend on interfaces (`InfrastructureManager`, `TalosManager`, `ServerManager`) to allow for easy mocking and testing.
*   **Idempotency:** All operations must be idempotent. Running `apply` twice should result in no changes on the second run.
*   **Safety:** Operations like `Delete` should be guarded and explicit.

## 4. Implementation Steps

### Phase 1: Configuration Schema Expansion (TDD)
**Goal:** Define the full configuration structure required for a cluster.

1.  **Test (Fail):** Update `internal/config/config_test.go` to test loading a complex config with Networks, Firewalls, and NodePools.
2.  **Implement (Pass):**
    *   Expand `Config` struct to include `Network`, `Firewall`, `LoadBalancer`, `ControlPlane`, `WorkerNodePools`, `Talos`, `Kubernetes`.
    *   Implement validation logic (e.g., check for overlapping subnets, valid instance types).
    *   **Doc:** Add detailed GoDoc comments for every field.

### Phase 2: Infrastructure Primitives (TDD)
**Goal:** Add missing Hetzner Cloud interfaces and implementations.

1.  **Define Interfaces (`internal/hcloud/client.go`):**
    *   `NetworkManager`: `EnsureNetwork`, `DeleteNetwork`.
    *   `FirewallManager`: `EnsureFirewall`, `DeleteFirewall`.
    *   `LoadBalancerManager`: `EnsureLoadBalancer`, `DeleteLoadBalancer`.
    *   `PlacementGroupManager`: `EnsurePlacementGroup`.
    *   `FloatingIPManager`: `EnsureFloatingIP`.
    *   **Doc:** Document the "Ensure" semantics (Create or Update).
2.  **Test (Mock):** Update `internal/hcloud/mock_client.go`.
3.  **Implement (`internal/hcloud/real_client.go`):**
    *   Implement the interfaces using `hcloud-go/v2`.
    *   Ensure proper label handling for reconciliation.

### Phase 3: Talos Configuration Generation (TDD)
**Goal:** programmatic generation of Talos machine configs.

1.  **Setup:** Create package `internal/talos`.
2.  **Test (Fail):** Create `internal/talos/config_test.go` to verify config generation for a Control Plane node.
3.  **Implement (Pass):**
    *   `ConfigGenerator` struct.
    *   Methods to generate `MachineConfig` using `talos/pkg/machinery`.
    *   Support for: Network interfaces, Extra Mounts, Certs/Secrets generation.
4.  **Doc:** Explain how secrets are handled (e.g., in-memory vs generated).

### Phase 4: Cluster Reconciliation Engine (TDD)
**Goal:** The core "Controller" logic that orchestrates everything.

1.  **Setup:** Create package `internal/cluster`.
2.  **Define Interface:** `Reconciler`.
3.  **Test (Fail):** Create `internal/cluster/reconciler_test.go`.
    *   Mock all infrastructure managers.
    *   Test the flow: Ensure Network -> Ensure Firewall -> Ensure LB -> Ensure Servers.
4.  **Implement (Pass):**
    *   `Reconciler` struct holding references to all Managers.
    *   `Reconcile(ctx, config)` method.
    *   **Logic:**
        1.  Reconcile Base Infra (Net, FW, PG, IP).
        2.  Reconcile Load Balancers.
        3.  Generate Talos Configs & Secrets.
        4.  Reconcile Control Plane Servers (Create if missing, update labels).
        5.  Reconcile Worker Servers (NodePool logic).
5.  **Doc:** Document the reconciliation order and dependency graph.

### Phase 5: Cluster Bootstrap & Kubeconfig (TDD)
**Goal:** Bring the cluster to life.

1.  **Test (Fail):** Create `internal/cluster/bootstrap_test.go`.
2.  **Implement (Pass):**
    *   `Bootstrapper` struct using `talos/pkg/machinery/client`.
    *   `Bootstrap()` method:
        *   Wait for Control Plane Node 1 to be accessible (port 50000).
        *   Call `bootstrap` API.
        *   Retrieve `kubeconfig`.
3.  **Integration:** Integrate into the `Reconcile` loop.

### Phase 6: CLI "Apply" Command
**Goal:** Expose functionality to the user.

1.  Update `cmd/hcloud-k8s/main.go`.
2.  Add `apply` command.
    *   Flag: `--config <path>` (required).
    *   Flag: `--dry-run` (optional).
3.  **Logic:**
    *   Load Config.
    *   Instantiate `RealClient`.
    *   Instantiate `Reconciler`.
    *   Run `Reconcile`.
    *   Output `kubeconfig` to file.

### Phase 7: E2E Verification
**Goal:** Prove it works in the real world.

1.  **Setup:** Create `tests/e2e/cluster_test.go`.
2.  **Test Logic:**
    *   **Provision:** Run `apply` with a minimal config (1 CP, 1 Worker).
    *   **Verify:**
        *   Check Hetzner Console (via API) for resources.
        *   Use generated `kubeconfig` to run `kubectl get nodes`.
    *   **Cleanup:** Implement `destroy` logic (or just manually clean up for now until `destroy` command is implemented in next iteration, but ideally we implement a basic `destroy` or use `defer` cleanup in test).
    *   *Note:* Since `destroy` command is Phase 6 of the *Design Doc* but technically needed for clean tests, we will implement a basic `Destroy()` method in the Reconciler for test cleanup.

## 5. Definition of Done
*   [ ] `internal/config` supports full cluster spec.
*   [ ] `internal/hcloud` has all necessary interfaces (Network, FW, LB, etc.).
*   [ ] `internal/talos` generates valid v1.9+ configs.
*   [ ] `internal/cluster` successfully provisions and links all resources.
*   [ ] `hcloud-k8s apply` command works.
*   [ ] E2E test passes: Cluster comes up, `kubectl` works.
*   [ ] **Documentation Updated:** New packages documented, architectural decisions recorded.
