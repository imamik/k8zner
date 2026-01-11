# Migration Status Analysis: Terraform → Pure Go CLI

**Analysis Date:** 2026-01-10 (Updated)
**Reference Document:** [technical_design_doc.md](technical_design_doc.md)

---

## Executive Summary

The migration from Terraform to a pure Go CLI is **~70% complete**. Core infrastructure provisioning is fully functional with excellent architecture, comprehensive test coverage, and working addon framework. The primary gaps are in **Day-2 operations** (upgrade/destroy), **advanced addons**, and **config schema extensions**.

### Current State

✅ **Fully Working:**
- Core infrastructure provisioning (networks, firewalls, load balancers, placement groups, floating IPs)
- Talos image building and snapshot creation
- Control plane and worker node provisioning with placement group sharding
- Cluster bootstrap with Talos
- CCM addon installation (fully wired into reconciliation flow)
- Comprehensive E2E test suite

⚠️ **Partially Complete:**
- Addon framework (only CCM implemented, 9 addons missing)
- Config schema (missing fields for advanced addons)
- Advanced Talos configurations (encryption, registries, extra mounts)

🔴 **Missing:**
- CLI lifecycle commands (upgrade, destroy)
- Most Kubernetes addons (CSI, Cilium, Ingress NGINX, RBAC, OIDC, Autoscaler, Backups, etc.)
- RDNS configuration

---

## Detailed Feature Comparison

### ✅ Step 1: Image Builder (100% Complete)

**TDD Requirements:**
1. Provision temporary server (Debian 13)
2. Enable/Boot Rescue Mode
3. SSH and write `talos.raw.xz` to `/dev/sda`
4. Create Snapshot with labels
5. Cleanup temporary server

**Implementation Status:**
- ✅ All requirements implemented in `internal/provisioning/image/`
  - `builder.go` - Complete image build pipeline
  - `coordinator.go` - Parallel architecture detection and building
  - `provisioner.go` - Pipeline phase integration
- ✅ E2E tests passing (`tests/e2e/snapshot_build_test.go`)
- ✅ Handler in `cmd/hcloud-k8s/handlers/image.go`

**Terraform Reference:**
- ✅ `terraform/image.tf` → Fully migrated (Go actively builds, Terraform only sourced)
- ✅ `terraform/packer/` → Replaced by native Go implementation

---

### ✅ Step 2: Base Infrastructure (100% Complete)

**TDD Requirements vs Implementation:**

| Component | TDD Requirement | Implementation | Status |
|-----------|----------------|----------------|--------|
| **Network** | Private network + subnets (CP, LB, Workers, Autoscaler) | `internal/provisioning/infrastructure/network.go` | ✅ Complete |
| **Firewall** | Dynamic IP rules + ports (6443, 50000, 51820) | `internal/provisioning/infrastructure/firewall.go` | ✅ Complete |
| **Load Balancers** | Control Plane LB (6443) + optional Ingress LB | `internal/provisioning/infrastructure/load_balancer.go` | ✅ Complete |
| **Placement Groups** | Control Plane spread + Worker partitioning (10 nodes/PG) | `internal/provisioning/compute/pool.go` + `internal/platform/hcloud/placement_group.go` | ✅ Complete |
| **Floating IPs** | Control Plane VIP if configured | `internal/provisioning/infrastructure/floating_ip.go` + `internal/platform/hcloud/floating_ip.go` | ✅ Complete |

**Terraform References:**
- ✅ `terraform/network.tf` → Fully migrated
- ✅ `terraform/firewall.tf` → Fully migrated
- ✅ `terraform/load_balancer.tf` → Fully migrated
- ✅ `terraform/placement_group.tf` → Fully migrated (with sharding: 1 PG per 10 workers)
- ✅ `terraform/floating_ip.tf` → Fully migrated

---

### ⚠️ Step 3: Server Provisioning & Talos Config (75% Complete)

**TDD Requirements vs Implementation:**

| Component | TDD Requirement | Implementation | Status |
|-----------|----------------|----------------|--------|
| **Config Generation** | SANs, Network (eth0/eth1), basic patches | `internal/platform/talos/config.go` | ✅ Complete |
| **Server Creation** | From snapshot with labels, network, firewall, placement groups | `internal/provisioning/compute/` | ✅ Complete |
| **Secrets** | Generate/load Talos certs and tokens | `internal/platform/talos/config.go` | ✅ Complete |
| **Encryption** | System Disk Encryption (LUKS) for state + ephemeral | Not in config schema | 🔴 **Missing** |
| **Registries** | Registry mirrors injection | Not in config schema | 🔴 **Missing** |
| **Extra Mounts** | Kubelet mounts (e.g., `/var/lib/longhorn`) | Not in config schema | 🔴 **Missing** |
| **RDNS** | Dynamic reverse DNS entries | Not implemented in hcloud adapter | 🔴 **Missing** |

**Terraform References:**
- ✅ `terraform/server.tf` → Server creation fully migrated
- ✅ `terraform/talos_config.tf` → Basic config migrated
- ❌ `terraform/talos_config.tf` (L100-150) → **Advanced configs not migrated** (encryption, registries, mounts)
- ❌ `terraform/rdns.tf` → **Not migrated**

---

### ✅ Step 4: Bootstrap & Cluster Formation (100% Complete)

**TDD Requirements:**
1. Push config to first Control Plane node (Maintenance Mode)
2. Call `bootstrap` API
3. Push config to remaining nodes
4. Retrieve `admin.kubeconfig`
5. Wait for node readiness

**Implementation Status:**
- ✅ All requirements implemented in `internal/provisioning/cluster/bootstrap.go`
  - Insecure Talos client for initial maintenance mode config
  - mTLS switch after reboot
  - Health checks via `ClientCtx.Version()`
  - State marker via hcloud certificate (idempotency)
  - Kubeconfig retrieval with polling (10m timeout)
- ✅ E2E tests passing (`tests/e2e/cluster_test.go`)

**Terraform Reference:**
- ✅ `terraform/talos.tf` → Fully migrated

---

### ⚠️ Step 5: Features & Addons (15% Complete)

**Addon Framework Status:** ✅ **Fully Wired Up**

The addon installation is properly integrated in `internal/orchestration/reconciler.go:77-84`:
```go
if len(r.state.Kubeconfig) > 0 {
    pCtx.Logger.Printf("[%s] Installing cluster addons...", phase)
    networkID := r.state.Network.ID
    if err := addons.Apply(ctx, r.config, r.state.Kubeconfig, networkID); err != nil {
        return nil, fmt.Errorf("failed to install addons: %w", err)
    }
}
```

**TDD Requirements vs Implementation:**

| Addon | TDD Requirement | Implementation | Status |
|-------|----------------|----------------|--------|
| **Hetzner CCM** | Secret + Deployment manifest | `internal/addons/ccm.go` + `manifests/hcloud-ccm/` | ✅ Complete |
| **Hetzner CSI** | Apply manifests | Not implemented | 🔴 **Missing** |
| **Cilium CNI** | Helm chart + IPSec key generation | Not implemented | 🔴 **Missing** |
| **RBAC** | Generate Role/ClusterRole manifests | Not implemented | 🔴 **Missing** |
| **OIDC** | Generate RoleBindings/ClusterRoleBindings | Not implemented | 🔴 **Missing** |
| **Autoscaler** | Helm + config secret with full machine config | Not implemented | 🔴 **Missing** |
| **Backups** | CronJob + ServiceAccount + S3 secrets | Not implemented | 🔴 **Missing** |
| **Ingress NGINX** | Helm + Deployment/DaemonSet logic | Not implemented | 🔴 **Missing** |
| **Cert Manager** | Helm with CRDs | Not implemented | 🔴 **Missing** |
| **Metrics Server** | Helm deployment | Not implemented | 🔴 **Missing** |

**Addon Framework Architecture:**
- `internal/addons/apply.go` - Entry point, iterates enabled addons
- `internal/addons/manifests.go` - Generic templating engine (Go text/template)
- `internal/addons/kubectl.go` - Manifest application via kubectl
- `internal/addons/manifests/` - Embedded manifest templates

**Terraform References:**
- ✅ `terraform/hcloud.tf` (CCM) → Migrated
- ❌ `terraform/cilium.tf` → **Not migrated** (171 lines, complex Helm config)
- ❌ `terraform/autoscaler.tf` → **Not migrated** (131 lines, requires machine config injection)
- ❌ `terraform/rbac.tf` → **Not migrated**
- ❌ `terraform/oidc.tf` → **Not migrated**
- ❌ `terraform/talos_backup.tf` → **Not migrated**
- ❌ `terraform/ingress_nginx.tf` → **Not migrated**
- ❌ `terraform/cert_manager.tf` → **Not migrated**
- ❌ `terraform/metrics_server.tf` → **Not migrated**
- ❌ `terraform/longhorn.tf` → **Not migrated**

---

### 🔴 Step 6: Lifecycle (0% Complete)

**TDD Requirements:**
- ❌ Upgrade Logic (FSM): Check version → Loop nodes → `talosctl upgrade` → Wait for reboot → Health check
- ❌ K8s Upgrade: Call `upgrade-k8s` API endpoint
- ❌ Destroy Logic: Query by labels → Delete in order (Servers → LBs → Floating IPs → Networks → Placement Groups → Firewalls)

**Implementation Status:**
- ❌ No `upgrade` CLI command (config struct `UpgradeConfig` exists but unused)
- ❌ No `destroy` CLI command
- ❌ No scaling operations

**CLI Commands Available:**
| Command | Status |
|---------|--------|
| `apply` | ✅ Complete |
| `image build` | ✅ Complete |
| `upgrade` | 🔴 Missing |
| `destroy` | 🔴 Missing |
| `scale` | 🔴 Missing |

---

## Architecture Assessment

### ✅ Strengths

1. **Clean Domain Separation:**
   - `internal/provisioning/` → Well-structured sub-packages (infrastructure, compute, cluster, image)
   - `internal/platform/` → Clean adapters for external systems (hcloud, talos, ssh)
   - `internal/orchestration/` → High-level workflow coordination

2. **Pipeline-Based Provisioning:**
   - `internal/provisioning/pipeline.go` → Consistent 6-phase execution
   - `internal/provisioning/observability.go` → Structured logging with timing

3. **Idempotent Reconciliation:**
   - State markers via hcloud certificates prevent re-bootstrap
   - Infrastructure client uses `Ensure*` methods (create or get existing)
   - Safe to re-run `apply` on existing clusters

4. **Validation-First Approach:**
   - `internal/provisioning/validation.go` → 5 validators (fields, network, server type, SSH, version)
   - `internal/config/validate.go` → Config validation with defaults

5. **Test Coverage:**
   - Unit tests across all packages
   - Comprehensive E2E tests in `tests/e2e/`

6. **Extensible Addon Framework:**
   - Go template-based manifest processing
   - Embedded manifests at build time
   - Pattern proven with CCM, trivial to add new addons

### ⚠️ Config Schema Gaps

**Missing configuration fields vs TDD spec (Section 6):**

| Feature | TDD Spec | Current Config | Gap |
|---------|----------|----------------|-----|
| `talos.backups.s3` | Yes | No | 🔴 Missing |
| `kubernetes.oidc.groupMappings` | Yes | No | 🔴 Missing |
| `kubernetes.rbac.roles` | Yes | No | 🔴 Missing |
| `cni.type` | Yes | No (only `encryption`) | 🔴 Missing |
| `ingress.nginx.kind` | Yes | No | 🔴 Missing |
| `ingress.nginx.replicas` | Yes | No | 🔴 Missing |
| Registry mirrors | Yes | No | 🔴 Missing |
| Kubelet extra mounts | Yes | No | 🔴 Missing |
| System disk encryption | Yes | No | 🔴 Missing |

### ⚠️ Platform Adapter Gaps

| Adapter | Status | Gap |
|---------|--------|-----|
| hcloud servers | ✅ Complete | None |
| hcloud network | ✅ Complete | None |
| hcloud firewall | ✅ Complete | None |
| hcloud load balancer | ✅ Complete | None |
| hcloud placement group | ✅ Complete | None |
| hcloud floating IP | ✅ Complete | None |
| hcloud RDNS | 🔴 Missing | No `hcloud_rdns` support |
| talos config | ✅ Complete | None |
| talos upgrade | 🔴 Missing | No upgrade path |
| SSH | ✅ Complete | None |

---

## Prioritized Action Plan

### Phase 1: Addons (Estimated: 2-3 weeks)

#### 🎯 Priority 1.1: Add CSI Driver

**Goal:** Prove addon pattern with a simple manifest-based addon.

**Tasks:**
1. Add CSI manifests to `internal/addons/manifests/hcloud-csi/`
   - `daemonset.yaml` - CSI node plugin
   - `deployment.yaml` - CSI controller
   - `secret.yaml` - Token injection
   - `storageclass.yaml` - Default storage class
2. Add `CSIConfig` struct to `internal/config/types.go`
3. Implement `applyCSI()` function in `internal/addons/csi.go`
4. Wire into `addons.Apply()`
5. Add E2E test with PVC/PV verification

**Reference:** `terraform/hcloud.tf` (CSI portion)

---

#### 🎯 Priority 1.2: Add Cilium CNI

**Goal:** Implement Helm-based addon with dynamic configuration.

**Tasks:**
1. Decide on Helm approach:
   - Option A: Add `helm.sh/helm/v3` dependency for native Go Helm
   - Option B: Pre-render templates and embed as manifests
   - Option C: Shell out to `helm template`
2. Add `CNIConfig` struct with `type` field to `internal/config/types.go`
3. Port Cilium Helm values from `terraform/cilium.tf`
4. Implement IPSec key generation (if `cni.encryption: ipsec`)
5. Implement `applyCilium()` in `internal/addons/cilium.go`
6. E2E test with pod-to-pod connectivity validation

**Reference:** `terraform/cilium.tf` (171 lines)

---

#### 🎯 Priority 1.3: Add Metrics Server

**Goal:** Quick win - simple Helm addon.

**Tasks:**
1. Add metrics-server manifests or Helm template
2. Add `MetricsServerConfig` to config schema
3. Implement `applyMetricsServer()` function
4. Wire into `addons.Apply()`

**Reference:** `terraform/metrics_server.tf`

---

#### 🎯 Priority 1.4: Add Cert Manager

**Goal:** Enable certificate automation.

**Tasks:**
1. Add cert-manager CRDs + deployment manifests
2. Add `CertManagerConfig` to config schema
3. Implement `applyCertManager()` function
4. Wire into `addons.Apply()`

**Reference:** `terraform/cert_manager.tf`

---

### Phase 2: Advanced Addons (Estimated: 2-3 weeks)

#### 🎯 Priority 2.1: Add Ingress NGINX

**Tasks:**
1. Add config for `kind` (Deployment vs DaemonSet) and `replicas`
2. Port Helm values from `terraform/ingress_nginx.tf`
3. Implement `applyIngressNginx()` function

---

#### 🎯 Priority 2.2: Add RBAC Configuration

**Tasks:**
1. Add `RBACConfig` struct with role definitions to config schema
2. Implement manifest generator for Role/ClusterRole
3. Implement `applyRBAC()` function

**Reference:** `terraform/rbac.tf`

---

#### 🎯 Priority 2.3: Add OIDC Bindings

**Tasks:**
1. Add `groupMappings` to `OIDCConfig` in config schema
2. Implement manifest generator for RoleBinding/ClusterRoleBinding
3. Implement `applyOIDC()` function

**Reference:** `terraform/oidc.tf`

---

#### 🎯 Priority 2.4: Add Cluster Autoscaler

**Tasks:**
1. Add config for autoscaler node pool machine configs
2. Generate `cluster-autoscaler-hetzner-config` Secret with **full Talos machine config**
3. Port Helm values from `terraform/autoscaler.tf`
4. Implement `applyAutoscaler()` function

**Reference:** `terraform/autoscaler.tf` (131 lines)

---

#### 🎯 Priority 2.5: Add Talos Backups

**Tasks:**
1. Add `BackupsConfig` with S3 settings to config schema
2. Implement CronJob + ServiceAccount + S3 Secret manifests
3. Implement `applyBackups()` function

**Reference:** `terraform/talos_backup.tf`

---

### Phase 3: Lifecycle Operations (Estimated: 2-3 weeks)

#### 🎯 Priority 3.1: Add Destroy Command

**Goal:** Enable cluster teardown.

**Tasks:**
1. Add `destroy` command to `cmd/hcloud-k8s/commands/`
2. Implement handler that:
   - Queries all resources by `cluster=<name>` label
   - Deletes in order: Servers → LBs → Floating IPs → Networks → Placement Groups → Firewalls → Certificates
3. Add confirmation prompt with `--force` flag
4. Add E2E test for destroy

---

#### 🎯 Priority 3.2: Add Upgrade Command

**Goal:** Enable Talos and Kubernetes upgrades.

**Tasks:**
1. Add `upgrade` command to CLI
2. Implement FSM logic:
   - Compare running Talos version vs desired
   - For each node: `talosctl upgrade` → wait reboot → health check
   - Call `upgrade-k8s` API endpoint
3. Wire existing `UpgradeConfig` from config schema
4. Add E2E test for upgrade path

---

### Phase 4: Advanced Talos Config (Estimated: 1 week)

#### 🎯 Priority 4.1: Add RDNS Support

**Tasks:**
1. Implement RDNS in `internal/platform/hcloud/rdns.go`
2. Add RDNS configuration to config schema
3. Wire into compute provisioner

**Reference:** `terraform/rdns.tf`

---

#### 🎯 Priority 4.2: Add Registry Mirrors

**Tasks:**
1. Add `registries` config to Talos config schema
2. Implement registry mirror injection in `internal/platform/talos/config.go`

---

#### 🎯 Priority 4.3: Add Kubelet Extra Mounts

**Tasks:**
1. Add `extraMounts` config to schema
2. Implement mount injection in Talos config generation

---

#### 🎯 Priority 4.4: Add System Disk Encryption

**Tasks:**
1. Add `encryption.enabled` to config schema
2. Implement LUKS configuration in Talos config generation

---

## Migration Completion Summary

| Category | Completion % | Remaining Work |
|----------|--------------|----------------|
| **Step 1: Image Builder** | 100% | ✅ None |
| **Step 2: Infrastructure** | 100% | ✅ None |
| **Step 3: Server & Talos** | 75% | RDNS, Encryption, Registries, Mounts |
| **Step 4: Bootstrap** | 100% | ✅ None |
| **Step 5: Addons** | 15% | 9/10 addons missing |
| **Step 6: Lifecycle** | 0% | Upgrade + Destroy commands |
| **Overall** | **~70%** | ~30% remaining |

---

## Immediate Next Steps

### 🚀 **Recommended Starting Point: CSI Driver (Priority 1.1)**

**Why this step:**
- Addon framework is already wired up and proven with CCM
- CSI is a simple manifest-based addon (no Helm complexity)
- Enables persistent volumes for stateful workloads
- Validates the addon pattern before tackling complex addons (Cilium, Autoscaler)
- Quick win to build momentum

**Implementation Checklist:**

1. **Create manifests directory:**
   ```
   internal/addons/manifests/hcloud-csi/
   ├── secret.yaml
   ├── controller.yaml
   ├── node.yaml
   └── storageclass.yaml
   ```

2. **Add config schema:**
   ```go
   // internal/config/types.go
   type CSIConfig struct {
       Enabled bool `mapstructure:"enabled" yaml:"enabled"`
   }

   // Add to AddonsConfig
   CSI CSIConfig `mapstructure:"csi" yaml:"csi"`
   ```

3. **Implement addon:**
   ```go
   // internal/addons/csi.go
   func applyCSI(ctx context.Context, kubeconfigPath, token string) error {
       templateData := map[string]string{"Token": token}
       manifestBytes, err := readAndProcessManifests("hcloud-csi", templateData)
       if err != nil {
           return fmt.Errorf("failed to read CSI manifests: %w", err)
       }
       return applyWithKubectl(ctx, kubeconfigPath, "hcloud-csi", manifestBytes)
   }
   ```

4. **Wire into Apply:**
   ```go
   // internal/addons/apply.go (after CCM block)
   if cfg.Addons.CSI.Enabled {
       if err := applyCSI(ctx, tmpKubeconfig, cfg.HCloudToken); err != nil {
           return fmt.Errorf("failed to install CSI: %w", err)
       }
   }
   ```

5. **Test:**
   ```bash
   go test -v -tags=e2e ./tests/e2e/ -run TestClusterProvisioning
   kubectl get pods -n kube-system | grep csi
   kubectl get storageclass
   ```

---

## Conclusion

The foundation is **excellent** with a **production-ready architecture**. Infrastructure provisioning is complete, the addon framework is properly wired and extensible, and the pipeline system provides clean phase orchestration.

**Key insight:** The migration status was previously underestimated. Infrastructure is 100% complete (not 70%), and the addon framework is fully operational (not missing wiring).

**Primary remaining work:**
1. **Addon implementations** - Following the proven CCM pattern
2. **Lifecycle commands** - Upgrade and destroy CLI commands
3. **Config schema extensions** - Fields for advanced features

The recommended path is to start with **CSI Driver** as it validates the addon pattern with minimal complexity, then progressively tackle more complex addons (Cilium, Autoscaler) and lifecycle operations.
