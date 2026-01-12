# Addon Refactoring Summary

This document summarizes the refactoring of CCM and CSI addons to use the new helm abstraction layer.

## Overview

Migrated existing CCM and CSI addons from static YAML templates to helm charts, matching terraform's approach and unlocking full configuration capabilities.

## Changes Per Addon

### Hetzner Cloud Controller Manager (CCM)

**Before:**
- Static YAML templates in `internal/addons/manifests/hcloud-ccm/`
- Version: v1.20.0 (outdated)
- Limited configuration: Only token and networkID templating
- Manual version updates required

**After:**
- Helm chart from `https://charts.hetzner.cloud`
- Version: v1.29.0 (latest, matching terraform)
- Full configuration support via `buildCCMValues()`
- Auto-updates via embed script

**Key Features:**
- DaemonSet deployment on control plane nodes
- Environment variables reference existing hcloud secret
- Load balancer configuration support
- Matches terraform/hcloud.tf lines 31-57

**Files:**
- `internal/addons/ccm.go` - Refactored to use helm.RenderChart()
- `internal/addons/ccm_test.go` - New comprehensive tests
- `internal/addons/helm/templates/hcloud-ccm/` - Embedded helm chart

### Hetzner Cloud CSI Driver (CSI)

**Before:**
- Static YAML templates in `internal/addons/manifests/hcloud-csi/`
- Manual template copies from upstream
- Limited configuration: replicas, token, storage class flag
- No PDB, topology spread, or advanced features

**After:**
- Helm chart from `https://charts.hetzner.cloud`
- Version: v2.18.3 (latest, matching terraform)
- Full configuration support via `buildCSIValues()`
- Rich HA and scheduling features

**Key Features:**
- HA support: 2 controller replicas for multi-CP clusters
- PodDisruptionBudget with maxUnavailable=1
- Topology spread constraints across hostname
- Control plane scheduling (nodeSelector + tolerations)
- Dynamic encryption key generation (32 bytes hex)
- Storage class configuration

**Files:**
- `internal/addons/csi.go` - Refactored to use helm.RenderChart()
- `internal/addons/csi_test.go` - Rewritten with comprehensive tests
- `internal/addons/helm/templates/hcloud-csi/` - Embedded helm chart

## Infrastructure Changes

### Build Tooling
- `scripts/embed-helm-charts.sh` - Added hcloud-ccm and hcloud-csi
  - CCM: v1.29.0 from https://charts.hetzner.cloud
  - CSI: v2.18.3 from https://charts.hetzner.cloud

### Removed Files
- `internal/addons/manifests.go` - No longer needed
- `internal/addons/manifests/hcloud-ccm/` - Replaced by helm chart
- `internal/addons/manifests/hcloud-csi/` - Replaced by helm chart

### Test Coverage
- `internal/addons/ccm_test.go` - Tests CCM value building
  - Node selector verification
  - Environment variable structure
  - Secret reference validation

- `internal/addons/csi_test.go` - Tests CSI value building
  - Replica count based on control plane count
  - PDB configuration
  - Topology spread constraints
  - Node selector and tolerations
  - Storage class configuration
  - Encryption key generation

## Terraform Parity

Both addons now match their terraform counterparts:

| Feature | Terraform | Go (Before) | Go (After) |
|---------|-----------|-------------|------------|
| **Chart Source** | Helm | Static YAML | Helm ✅ |
| **CCM Version** | v1.29.0 | v1.20.0 | v1.29.0 ✅ |
| **CSI Version** | v2.18.3 | Unknown | v2.18.3 ✅ |
| **Dynamic Config** | Yes | No | Yes ✅ |
| **HA Support** | Yes | No | Yes ✅ |
| **PDB** | Yes | No | Yes ✅ |
| **Topology Spread** | Yes | No | Yes ✅ |
| **Easy Updates** | Yes | No | Yes ✅ |

## Benefits

### 1. Terraform Parity
- Both platforms now use identical helm charts
- Same version numbers
- Same configuration structure

### 2. Easier Maintenance
- Version updates: Change one line in embed script
- Configuration changes: Modify value builders
- No manual YAML copying from upstream

### 3. Better Testing
- Unit tests for value builders
- Clear separation of concerns
- Easy to verify configuration logic

### 4. Full Feature Support
- All helm chart features available
- Can inject any terraform values
- Future-proof for new features

### 5. Consistent Pattern
- All addons use same helm abstraction
- Predictable code structure
- Easy to add new addons

## Migration Path for Remaining Addons

Based on this refactoring, here's the pattern for remaining addons:

1. **Add to embed script**: Update `scripts/embed-helm-charts.sh`
2. **Download chart**: Run `./scripts/embed-helm-charts.sh <addon-name>`
3. **Create addon file**: `internal/addons/<addonName>.go`
4. **Implement value builder**: `build<Addon>Values()` matching terraform
5. **Update Apply()**: Add conditional installation
6. **Write tests**: `internal/addons/<addonName>_test.go`

## Commits

### Commit 1: c47e202
**Title:** feat: Add helm abstraction layer for addon management

**Summary:** Created core helm rendering infrastructure
- Helm chart loading and discovery
- Template rendering with Helm v3 SDK
- Value merging utilities
- Metrics-server proof of concept

**Stats:** 38 files changed, +2061 lines

### Commit 2: ac485c0
**Title:** refactor: Migrate CCM and CSI to helm abstraction

**Summary:** Refactored CCM and CSI to use new helm abstraction
- Upgraded CCM v1.20.0 → v1.29.0
- Upgraded CSI to v2.18.3 helm chart
- Added comprehensive tests
- Removed old YAML templates

**Stats:** 51 files changed, +3873/-754 lines

## Next Steps

Following the established pattern, migrate remaining addons in priority order:

1. ✅ **Metrics Server** (Done - proof of concept)
2. ✅ **CCM** (Done - this refactor)
3. ✅ **CSI** (Done - this refactor)
4. **Cert Manager** - Medium complexity
5. **Longhorn** - Storage configuration
6. **Ingress NGINX** - Complex annotations
7. **Cluster Autoscaler** - Hetzner-specific
8. **Cilium** - Most complex, save for last

## Conclusion

The refactoring successfully brings CCM and CSI up to terraform parity while establishing a clear pattern for migrating remaining addons. The helm abstraction layer provides a robust foundation for managing all cluster addons consistently.
