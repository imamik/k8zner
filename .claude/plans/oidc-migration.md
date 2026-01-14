# OIDC RBAC Migration Plan

## Overview
Migrate OIDC RBAC addon from terraform to Go using direct YAML generation (no helm).
This addon creates RoleBindings and ClusterRoleBindings for OIDC group mappings.

## Current State (Terraform)
- **File**: `terraform/oidc.tf`
- **Purpose**: Map OIDC groups to Kubernetes roles and cluster roles
- **Approach**: YAML generation using locals

## Key Requirements

### 1. Group Mappings
Each group mapping has:
- OIDC group name
- List of cluster roles (ClusterRole names)
- List of roles (Role name + namespace)

### 2. ClusterRoleBinding Generation
For each unique cluster role referenced across all mappings:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: oidc-<cluster_role>
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: <cluster_role>
subjects:
  - apiGroup: rbac.authorization.k8s.io
    kind: Group
    name: <groups_prefix><group>
  # ... all groups that have this cluster role
```

### 3. RoleBinding Generation
For each unique role (namespace/name combination) referenced across all mappings:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: oidc-<role_name>
  namespace: <namespace>
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: <role_name>
subjects:
  - apiGroup: rbac.authorization.k8s.io
    kind: Group
    name: <groups_prefix><group>
  # ... all groups that have this role
```

### 4. Groups Prefix
- Configurable prefix for OIDC groups
- Applied to all subject names

### 5. Manifest Combination
- Combine all ClusterRoleBindings and RoleBindings
- Separate with `---` YAML document separator
- Apply as single manifest using kubectl

## Implementation Steps

### Step 1: Extend Config Types
Add to `internal/config/types.go`:
```go
type OIDCConfig struct {
    Enabled       bool
    GroupsPrefix  string
    GroupMappings []OIDCGroupMapping
}

type OIDCGroupMapping struct {
    Group        string
    ClusterRoles []string
    Roles        []OIDCRole
}

type OIDCRole struct {
    Name      string
    Namespace string
}
```

### Step 2: Implement Addon
Create `internal/addons/oidc.go`:

```go
func applyOIDC(ctx, kubeconfigPath, cfg) error {
    if len(cfg.OIDC.GroupMappings) == 0 {
        return nil
    }

    // Collect unique cluster roles and roles
    clusterRoles := collectUniqueClusterRoles(cfg.OIDC.GroupMappings)
    roles := collectUniqueRoles(cfg.OIDC.GroupMappings)

    // Generate manifests
    manifests := []string{}

    // Generate ClusterRoleBindings
    for _, clusterRole := range clusterRoles {
        binding := generateClusterRoleBinding(
            clusterRole,
            cfg.OIDC.GroupMappings,
            cfg.OIDC.GroupsPrefix,
        )
        manifests = append(manifests, binding)
    }

    // Generate RoleBindings
    for roleKey, roleInfo := range roles {
        binding := generateRoleBinding(
            roleInfo,
            cfg.OIDC.GroupMappings,
            cfg.OIDC.GroupsPrefix,
        )
        manifests = append(manifests, binding)
    }

    // Combine and apply
    combined := strings.Join(manifests, "\n---\n")
    return applyWithKubectl(ctx, kubeconfigPath, "kube-oidc-rbac", []byte(combined))
}

func collectUniqueClusterRoles(mappings) []string {
    // Extract all cluster roles from mappings
    // Return unique set
}

func collectUniqueRoles(mappings) map[string]OIDCRole {
    // Extract all roles from mappings
    // Return map keyed by "namespace/name"
}

func generateClusterRoleBinding(clusterRole, mappings, prefix) string {
    // Find all groups that have this cluster role
    // Generate ClusterRoleBinding manifest
}

func generateRoleBinding(role, mappings, prefix) string {
    // Find all groups that have this role
    // Generate RoleBinding manifest
}
```

### Step 3: Testing
Create `internal/addons/oidc_test.go`:
- Test unique cluster role collection
- Test unique role collection
- Test ClusterRoleBinding generation with multiple groups
- Test RoleBinding generation with multiple groups
- Test groups prefix application
- Test empty config handling
- Test manifest combination

## Implementation Pattern

Similar to RBAC:
- Direct YAML generation using yaml.Marshal
- No helm chart (pure YAML addon)
- Collect unique roles first, then generate bindings
- One binding per role (not per group)

## Success Criteria
- [ ] OIDC config types added
- [ ] Unique cluster role collection works
- [ ] Unique role collection works
- [ ] ClusterRoleBinding generation works
- [ ] RoleBinding generation works
- [ ] Groups prefix applied correctly
- [ ] Manifests combined correctly
- [ ] Tests for all generation paths
- [ ] CODE_STRUCTURE.md compliant
- [ ] Integrated into apply.go

## Terraform Reference
```hcl
# terraform/oidc.tf
# Lines 1-84
```

## Notes
- More complex than RBAC due to group aggregation logic
- One binding per role (efficient), not one per group
- Need to track which groups map to which roles
- Groups prefix is prepended to all subject names
