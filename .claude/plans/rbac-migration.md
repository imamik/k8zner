# RBAC Migration Plan

## Overview
Migrate RBAC addon from terraform to Go using direct YAML generation (no helm).
This is a simple addon that generates Role and ClusterRole manifests from config.

## Current State (Terraform)
- **File**: `terraform/rbac.tf`
- **Purpose**: Generate Kubernetes RBAC manifests
- **Approach**: YAML generation using locals

## Key Requirements

### 1. Role Generation
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: <role.name>
  namespace: <role.namespace>
rules:
  - apiGroups: [...]
    resources: [...]
    verbs: [...]
```

### 2. ClusterRole Generation
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: <role.name>
rules:
  - apiGroups: [...]
    resources: [...]
    verbs: [...]
```

### 3. Manifest Combination
- Combine all Role and ClusterRole manifests
- Separate with `---` YAML document separator
- Apply as single manifest using kubectl

## Implementation Steps

### Step 1: Extend Config Types
Add to `internal/config/types.go`:
```go
type RBACConfig struct {
    Enabled      bool
    Roles        []RoleConfig
    ClusterRoles []ClusterRoleConfig
}

type RoleConfig struct {
    Name      string
    Namespace string
    Rules     []RBACRuleConfig
}

type ClusterRoleConfig struct {
    Name  string
    Rules []RBACRuleConfig
}

type RBACRuleConfig struct {
    APIGroups []string
    Resources []string
    Verbs     []string
}
```

### Step 2: Implement Addon
Create `internal/addons/rbac.go`:

```go
func applyRBAC(ctx, kubeconfigPath, cfg) error {
    if len(cfg.RBAC.Roles) == 0 && len(cfg.RBAC.ClusterRoles) == 0 {
        return nil // Nothing to apply
    }

    // Generate manifests
    manifests := generateRBACManifests(cfg.RBAC)

    // Combine with --- separator
    combined := strings.Join(manifests, "\n---\n")

    // Apply
    return applyWithKubectl(ctx, kubeconfigPath, "kube-rbac", []byte(combined))
}

func generateRBACManifests(rbac RBACConfig) []string {
    var manifests []string

    // Generate Role manifests
    for _, role := range rbac.Roles {
        manifests = append(manifests, generateRole(role))
    }

    // Generate ClusterRole manifests
    for _, role := range rbac.ClusterRoles {
        manifests = append(manifests, generateClusterRole(role))
    }

    return manifests
}

func generateRole(role RoleConfig) string {
    // Build Role manifest
}

func generateClusterRole(role ClusterRoleConfig) string {
    // Build ClusterRole manifest
}
```

### Step 3: Testing
Create `internal/addons/rbac_test.go`:
- Test Role generation with various rules
- Test ClusterRole generation
- Test manifest combination
- Test empty config handling

## Implementation Pattern

Since this doesn't use helm, follow the namespace creation pattern:
- Direct YAML string building
- Use `yamlencode` equivalent (yaml.Marshal)
- Combine manifests with `---` separator
- Apply via kubectl

## Success Criteria
- [ ] RBAC config types added
- [ ] Role generation works
- [ ] ClusterRole generation works
- [ ] Manifests combined correctly
- [ ] Tests for all generation paths
- [ ] CODE_STRUCTURE.md compliant
- [ ] Integrated into apply.go

## Terraform Reference
```hcl
# terraform/rbac.tf
# Lines 1-38
```

## Notes
- No helm chart needed - pure YAML generation
- Similar to createIngressNginxNamespace() pattern
- Use yaml.Marshal for generating YAML
- Simpler than helm addons
