# Migration Status Analysis: Terraform → Pure Go CLI

**Analysis Date:** 2026-07-21 (Updated)
**Reference Document:** [technical_design_doc.md](technical_design_doc.md)

---

## Executive Summary

The migration from Terraform to a pure Go CLI is **~92% complete**. Core infrastructure, Talos configuration, and Addons are fully implemented.

**Current Build Status:** ⚠️ **BROKEN**
The `hcloud-k8s` binary fails to compile because `cmd/hcloud-k8s/commands/upgrade.go` is missing, despite `Root` command trying to register it.

### Critical Gaps for MVP
1.  **Upgrade Command Wiring:** The upgrade logic exists in `internal/provisioning/upgrade/` (comprehensive FSM implementation), but the CLI command wrapper is missing.
2.  **Scale Down:** The reconciler can create new nodes (Scale Up) but ignores servers that should be removed (Scale Down).

---

## MVP Feature Overview Table

| Phase | Feature | Status | Notes |
| :--- | :--- | :--- | :--- |
| **1. Foundation** | Image Builder | ✅ 100% | Full replacement for Packer. |
| **2. Infrastructure** | Network & Subnets | ✅ 100% | Includes correct CIDR calculations. |
| | Firewalls | ✅ 100% | Dynamic IP allow-listing implemented. |
| | Load Balancers | ✅ 100% | Control Plane & Ingress LB support. |
| | Placement Groups | ✅ 100% | Spread topology & sharding (1 PG/10 nodes). |
| **3. Talos Config** | Config Generation | ✅ 100% | |
| | Advanced Configs | ✅ 100% | Encryption, Registries, Kernel Args, Mounts. |
| | RDNS | ✅ 100% | Template support for IPv4/IPv6. |
| **4. Provisioning** | Server Creation | ✅ 100% | |
| | Bootstrap | ✅ 100% | State markers & kubeconfig retrieval. |
| | Scale Up | ✅ 100% | Implicitly handled by reconciler. |
| | **Scale Down** | 🔴 0% | Logic to cordon/drain/delete excess nodes missing. |
| **5. Addons** | CCM & CSI | ✅ 100% | Includes encryption secret generation. |
| | Cilium CNI | ✅ 100% | Includes IPSec & Hubble support. |
| | OIDC & RBAC | ✅ 100% | Dynamic RoleBinding generation. |
| | Talos Backups | ✅ 100% | S3 Backups & CronJob. |
| | Standard Addons | ✅ 100% | Ingress, CertManager, Metrics, Longhorn wired. |
| **6. Lifecycle** | Destroy Command | ✅ 100% | Full teardown dependency order implemented. |
| | **Upgrade Command** | ⚠️ 90% | **Logic exists** in `internal/provisioning/upgrade/`, but **CLI command is missing**. |

**Total Estimated MVP Completion:** **92%**

---

## Detailed Gap Analysis

### ⚠️ Upgrade Command (Logic vs Wiring)
You asked: *"I thought Talos upgrade was completely migrated?!"*
**Answer:** The **logic** is migrated, but the **CLI command** is missing.

-   **Logic (✅ Present):** `internal/provisioning/upgrade/provisioner.go` contains a complete upgrade provisioner:
    -   Control Plane sequential upgrade loop.
    -   Worker upgrade loop.
    -   Kubernetes version upgrade.
    -   Health checks & Dry Run mode.
    -   `internal/platform/talos/upgrade.go` handles the low-level API calls.
-   **CLI (🔴 Missing):** `cmd/hcloud-k8s/commands/upgrade.go` does not exist.
    -   `cmd/hcloud-k8s/commands/root.go` calls `cmd.AddCommand(Upgrade())`, causing a build error.

### 🔴 Scale Down (Missing Logic)
The `reconcileNodePool` function in `internal/provisioning/compute/pool.go` iterates from `1` to `Count` to ensure servers exist.
-   It **does not** list existing servers to check if any indices > `Count` exist.
-   **Impact:** If you reduce `count` in `cluster.yaml` from 5 to 3, the CLI will do nothing. The 2 extra nodes will remain running and joined to the cluster.

---

## Recommended Next Steps

1.  **Fix Build / Wire Upgrade:** Create `cmd/hcloud-k8s/commands/upgrade.go` to expose the existing upgrade logic.
2.  **Implement Scale Down:** Update `reconcileNodePool` to identify and remove excess nodes (Cordon -> Drain -> Delete).

