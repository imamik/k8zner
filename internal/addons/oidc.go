package addons

import (
	"context"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"hcloud-k8s/internal/config"
)

// applyOIDC installs OIDC RBAC role bindings and cluster role bindings.
// See: terraform/oidc.tf
func applyOIDC(ctx context.Context, kubeconfigPath string, cfg *config.Config) error {
	if len(cfg.Addons.OIDC.GroupMappings) == 0 {
		return nil // Nothing to apply
	}

	// Collect unique cluster roles and roles from all group mappings
	clusterRoles := collectUniqueClusterRoles(cfg.Addons.OIDC.GroupMappings)
	roles := collectUniqueRoles(cfg.Addons.OIDC.GroupMappings)

	var manifests []string

	// Generate ClusterRoleBindings (one per unique cluster role)
	for _, clusterRole := range clusterRoles {
		binding := generateClusterRoleBinding(
			clusterRole,
			cfg.Addons.OIDC.GroupMappings,
			cfg.Addons.OIDC.GroupsPrefix,
		)
		manifests = append(manifests, binding)
	}

	// Generate RoleBindings (one per unique role)
	for _, role := range roles {
		binding := generateRoleBinding(
			role,
			cfg.Addons.OIDC.GroupMappings,
			cfg.Addons.OIDC.GroupsPrefix,
		)
		manifests = append(manifests, binding)
	}

	if len(manifests) == 0 {
		return nil
	}

	// Combine and apply manifests
	combined := strings.Join(manifests, "\n---\n")
	if err := applyWithKubectl(ctx, kubeconfigPath, "kube-oidc-rbac", []byte(combined)); err != nil {
		return fmt.Errorf("failed to apply OIDC RBAC manifests: %w", err)
	}

	return nil
}

// collectUniqueClusterRoles extracts all unique cluster role names.
// See: terraform/oidc.tf lines 3-5
func collectUniqueClusterRoles(mappings []config.OIDCGroupMapping) []string {
	roleSet := make(map[string]bool)
	for _, mapping := range mappings {
		for _, role := range mapping.ClusterRoles {
			roleSet[role] = true
		}
	}

	var roles []string
	for role := range roleSet {
		roles = append(roles, role)
	}
	return roles
}

// collectUniqueRoles extracts all unique roles (by namespace/name).
// See: terraform/oidc.tf lines 8-14
func collectUniqueRoles(mappings []config.OIDCGroupMapping) []config.OIDCRole {
	roleMap := make(map[string]config.OIDCRole)
	for _, mapping := range mappings {
		for _, role := range mapping.Roles {
			key := role.Namespace + "/" + role.Name
			roleMap[key] = role
		}
	}

	var roles []config.OIDCRole
	for _, role := range roleMap {
		roles = append(roles, role)
	}
	return roles
}

// generateClusterRoleBinding creates a ClusterRoleBinding manifest.
// See: terraform/oidc.tf lines 17-42
func generateClusterRoleBinding(clusterRole string, mappings []config.OIDCGroupMapping, groupsPrefix string) string {
	// Find all groups that have this cluster role
	var subjects []map[string]any
	for _, mapping := range mappings {
		for _, role := range mapping.ClusterRoles {
			if role == clusterRole {
				subjects = append(subjects, map[string]any{
					"apiGroup": "rbac.authorization.k8s.io",
					"kind":     "Group",
					"name":     groupsPrefix + mapping.Group,
				})
				break
			}
		}
	}

	binding := map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRoleBinding",
		"metadata": map[string]any{
			"name": "oidc-" + clusterRole,
		},
		"roleRef": map[string]any{
			"apiGroup": "rbac.authorization.k8s.io",
			"kind":     "ClusterRole",
			"name":     clusterRole,
		},
		"subjects": subjects,
	}

	yamlBytes, _ := yaml.Marshal(binding)
	return string(yamlBytes)
}

// generateRoleBinding creates a RoleBinding manifest.
// See: terraform/oidc.tf lines 45-71
func generateRoleBinding(role config.OIDCRole, mappings []config.OIDCGroupMapping, groupsPrefix string) string {
	roleKey := role.Namespace + "/" + role.Name

	// Find all groups that have this role
	var subjects []map[string]any
	for _, mapping := range mappings {
		for _, r := range mapping.Roles {
			if r.Namespace == role.Namespace && r.Name == role.Name {
				subjects = append(subjects, map[string]any{
					"apiGroup": "rbac.authorization.k8s.io",
					"kind":     "Group",
					"name":     groupsPrefix + mapping.Group,
				})
				break
			}
		}
	}

	binding := map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "RoleBinding",
		"metadata": map[string]any{
			"name":      "oidc-" + role.Name,
			"namespace": role.Namespace,
		},
		"roleRef": map[string]any{
			"apiGroup": "rbac.authorization.k8s.io",
			"kind":     "Role",
			"name":     role.Name,
		},
		"subjects": subjects,
	}

	yamlBytes, _ := yaml.Marshal(binding)
	return string(yamlBytes)
}
