# Cert Manager Migration Plan

## Overview
Migrate cert-manager addon from terraform to Go using helm abstraction layer.

## Current State (Terraform)
- **File**: `terraform/cert_manager.tf`
- **Chart**: cert-manager v1.19.2 from https://charts.jetstack.io
- **Namespace**: cert-manager (created via manifest)
- **Components**: 3 (controller, webhook, cainjector)

## Key Requirements

### 1. Namespace Management
- Create cert-manager namespace
- Should be created before helm chart application

### 2. Replica Configuration
- Single control plane: 1 replica
- HA control plane (>1): 2 replicas
- Applies to all 3 components

### 3. High Availability
- **PodDisruptionBudget**:
  - enabled: true
  - maxUnavailable: 1
- **Topology Spread Constraints**:
  - topologyKey: kubernetes.io/hostname
  - maxSkew: 1
  - whenUnsatisfiable: DoNotSchedule
  - Separate constraints for each component (controller, webhook, cainjector)

### 4. Scheduling
- **nodeSelector**: node-role.kubernetes.io/control-plane
- **tolerations**:
  - key: node-role.kubernetes.io/control-plane
  - effect: NoSchedule
  - operator: Exists

### 5. Configuration
- **CRDs**: enabled: true
- **startupapicheck**: enabled: false (avoids Job creation)
- **Gateway API**: enableGatewayAPI: true
- **Feature Gates**:
  - ACMEHTTP01IngressPathTypeExact: Disabled if ingress-nginx is enabled (workaround for bug)

## Implementation Steps

### Step 1: Update Embed Script
- Change cert-manager version from v1.16.3 to v1.19.2
- Re-download chart with correct version

### Step 2: Add Config Type
- Add `CertManagerConfig` to `internal/config/types.go`
- Fields: Enabled bool

### Step 3: Implement Addon
Create `internal/addons/certManager.go`:
- `applyCertManager(ctx, kubeconfigPath, cfg)` - Main entry point
- `buildCertManagerValues(cfg)` - Build helm values matching terraform
- `createNamespaceManifest()` - Create namespace YAML

### Step 4: Value Builder Logic
```go
func buildCertManagerValues(cfg *config.Config) helm.Values {
    controlPlaneCount := getControlPlaneCount(cfg)
    replicas := 1
    if controlPlaneCount > 1 {
        replicas = 2
    }

    baseValues := {
        replicaCount: replicas,
        podDisruptionBudget: {...},
        topologySpreadConstraints: [...],
        nodeSelector: {...},
        tolerations: [...],
    }

    return {
        crds: {enabled: true},
        startupapicheck: {enabled: false},
        config: {
            enableGatewayAPI: true,
            featureGates: {
                ACMEHTTP01IngressPathTypeExact: !cfg.Addons.IngressNginx.Enabled,
            },
        },
        // Apply baseValues to controller
        // Apply baseValues to webhook (with updated labelSelector)
        // Apply baseValues to cainjector (with updated labelSelector)
    }
}
```

### Step 5: Integration
- Update `internal/addons/apply.go` to call `applyCertManager`
- Apply namespace first, then helm chart

### Step 6: Testing
Create `internal/addons/certManager_test.go`:
- Test replica calculation
- Test topology spread constraints for all components
- Test feature gate logic (ingress-nginx enabled/disabled)
- Test PDB configuration
- Test node selector and tolerations

## Code Structure Compliance

### File Organization
- `internal/addons/certManager.go` - camelCase filename ✅
- Functions < 50 lines where possible
- Clear separation: apply → build values → render → kubectl

### Function Naming
- `applyCertManager()` - unexported, camelCase ✅
- `buildCertManagerValues()` - unexported, camelCase ✅
- `createCertManagerNamespace()` - unexported, camelCase ✅

### Documentation
- Package comment if new package (N/A)
- Function comments: 1-3 lines max
- Reference terraform source in comments

### Error Handling
- Return errors with context
- Use `fmt.Errorf("failed to X: %w", err)` pattern

## Testing Strategy

### Unit Tests
1. **Value Building**:
   - Single control plane → 1 replica
   - HA control plane → 2 replicas
   - Correct topology spread for all 3 components

2. **Feature Gates**:
   - IngressNginx enabled → ACMEHTTP01IngressPathTypeExact = false
   - IngressNginx disabled → ACMEHTTP01IngressPathTypeExact = true

3. **Component Configuration**:
   - Controller has correct labelSelector
   - Webhook has correct labelSelector
   - Cainjector has correct labelSelector

### Integration Points
- Verify namespace creation before chart
- Verify helm rendering produces valid YAML
- Mock kubectl for apply testing

## Risks & Mitigations

### Risk 1: Complex Value Structure
Cert-manager has 3 components with shared base config but different label selectors.

**Mitigation**: Create helper function to generate topology spread constraints with component-specific labels.

### Risk 2: Namespace Timing
Namespace must exist before helm chart application.

**Mitigation**: Apply namespace manifest first, then helm chart. Both use same kubectl apply.

### Risk 3: CRD Management
Cert-manager includes CRDs that must be installed.

**Mitigation**: Helm chart handles CRDs via `crds.enabled = true`. No special handling needed.

## Success Criteria
- [ ] Chart version matches terraform (v1.19.2)
- [ ] All 3 components configured identically to terraform
- [ ] Replica count logic matches terraform
- [ ] Feature gates work correctly
- [ ] Tests cover all configuration paths
- [ ] CODE_STRUCTURE.md compliant
- [ ] Namespace created before chart application

## Terraform Reference
```hcl
# terraform/cert_manager.tf
# Lines 1-124
```
