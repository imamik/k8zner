# Longhorn Migration Plan

## Overview
Migrate longhorn storage addon from terraform to Go using helm abstraction.

## Current State (Terraform)
- **File**: `terraform/longhorn.tf`
- **Chart**: longhorn v1.10.1 from https://charts.longhorn.io
- **Namespace**: longhorn-system (with pod security labels)
- **Purpose**: Distributed block storage for Kubernetes

## Key Requirements

### 1. Namespace with Pod Security
- Name: longhorn-system
- Labels:
  - pod-security.kubernetes.io/enforce: privileged
  - pod-security.kubernetes.io/audit: privileged
  - pod-security.kubernetes.io/warn: privileged

### 2. Manager Image Hotfix
- Override manager image tag: v1.10.1-hotfix-1
- Workaround for: https://github.com/longhorn/longhorn/issues/12259

### 3. Upgrade Configuration
- preUpgradeChecker.upgradeVersionCheck: false

### 4. Default Settings
- allowCollectingLonghornUsageMetrics: false
- kubernetesClusterAutoscalerEnabled: Based on cluster config
- upgradeChecker: false

### 5. Network Policies
- enabled: true
- type: "rke1" (ingress-nginx compatible)

### 6. Storage Class
- persistence.defaultClass: From config

## Implementation Steps

### Step 1: Update Chart Version
- Chart version in embed script: 1.7.2 → 1.10.1
- Re-download chart

### Step 2: Add Config Type
- Add `LonghornConfig` to types.go
- Fields: Enabled, DefaultStorageClass

### Step 3: Implement Addon
Create `internal/addons/longhorn.go`:
- `applyLonghorn(ctx, kubeconfigPath, cfg)` - Entry point
- `buildLonghornValues(cfg)` - Build values
- `createLonghornNamespace()` - Namespace with PSP labels

### Step 4: Value Builder Logic
```go
func buildLonghornValues(cfg *config.Config) helm.Values {
    // Check if cluster autoscaler is enabled
    clusterAutoscalerEnabled := hasClusterAutoscaler(cfg)

    return {
        image: {
            longhorn: {
                manager: {
                    tag: "v1.10.1-hotfix-1",
                },
            },
        },
        preUpgradeChecker: {
            upgradeVersionCheck: false,
        },
        defaultSettings: {
            allowCollectingLonghornUsageMetrics: false,
            kubernetesClusterAutoscalerEnabled: clusterAutoscalerEnabled,
            upgradeChecker: false,
        },
        networkPolicies: {
            enabled: true,
            type: "rke1",
        },
        persistence: {
            defaultClass: cfg.Addons.Longhorn.DefaultStorageClass,
        },
    }
}
```

### Step 5: Integration
- Update `apply.go` to call `applyLonghorn`
- Create namespace first, then apply helm chart

### Step 6: Testing
Create `internal/addons/longhorn_test.go`:
- Test manager image tag override
- Test cluster autoscaler detection
- Test default settings
- Test network policies
- Test storage class configuration

## Code Structure Compliance
- File: longhorn.go (camelCase) ✅
- Functions < 50 lines ✅
- Clear error messages ✅
- 1-3 line comments ✅
- Returns errors, no logging ✅

## Success Criteria
- [ ] Chart version matches terraform (v1.10.1)
- [ ] Namespace has pod security labels
- [ ] Manager image tag hotfix applied
- [ ] Cluster autoscaler detection works
- [ ] Network policies configured
- [ ] Default storage class configurable
- [ ] Tests cover all paths
- [ ] CODE_STRUCTURE.md compliant

## Terraform Reference
```hcl
# terraform/longhorn.tf
# Lines 1-69
```
