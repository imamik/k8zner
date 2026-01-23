package addons

import (
	"context"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"hcloud-k8s/internal/addons/k8sclient"
	"hcloud-k8s/internal/config"
)

// applyOIDC installs OIDC RBAC role bindings and cluster role bindings.
// See: terraform/oidc.tf
func applyOIDC(ctx context.Context, client k8sclient.Client, cfg *config.Config) error {
	if len(cfg.Addons.OIDCRBAC.GroupMappings) == 0 {
		return nil // Nothing to apply
	}

	// Collect unique cluster roles and roles from all group mappings
	clusterRoles := collectUniqueClusterRoles(cfg.Addons.OIDCRBAC.GroupMappings)
	roles := collectUniqueRoles(cfg.Addons.OIDCRBAC.GroupMappings)

	var manifests []string

	// Generate ClusterRoleBindings (one per unique cluster role)
	for _, clusterRole := range clusterRoles {
		binding, err := generateClusterRoleBinding(
			clusterRole,
			cfg.Addons.OIDCRBAC.GroupMappings,
			cfg.Addons.OIDCRBAC.GroupsPrefix,
		)
		if err != nil {
			return fmt.Errorf("failed to generate ClusterRoleBinding for %s: %w", clusterRole, err)
		}
		manifests = append(manifests, binding)
	}

	// Generate RoleBindings (one per unique role)
	for _, role := range roles {
		binding, err := generateRoleBinding(
			role,
			cfg.Addons.OIDCRBAC.GroupMappings,
			cfg.Addons.OIDCRBAC.GroupsPrefix,
		)
		if err != nil {
			return fmt.Errorf("failed to generate RoleBinding for %s/%s: %w", role.Namespace, role.Name, err)
		}
		manifests = append(manifests, binding)
	}

	if len(manifests) == 0 {
		return nil
	}

	// Combine and apply manifests
	combined := strings.Join(manifests, "\n---\n")
	if err := applyManifests(ctx, client, "kube-oidc-rbac", []byte(combined)); err != nil {
		return fmt.Errorf("failed to apply OIDC RBAC manifests: %w", err)
	}

	return nil
}

// collectUniqueClusterRoles extracts all unique cluster role names.
// See: terraform/oidc.tf lines 3-5
func collectUniqueClusterRoles(mappings []config.OIDCRBACGroupMapping) []string {
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
func collectUniqueRoles(mappings []config.OIDCRBACGroupMapping) []config.OIDCRBACRole {
	roleMap := make(map[string]config.OIDCRBACRole)
	for _, mapping := range mappings {
		for _, role := range mapping.Roles {
			key := role.Namespace + "/" + role.Name
			roleMap[key] = role
		}
	}

	var roles []config.OIDCRBACRole
	for _, role := range roleMap {
		roles = append(roles, role)
	}
	return roles
}

// generateClusterRoleBinding creates a ClusterRoleBinding manifest.
// See: terraform/oidc.tf lines 17-42
func generateClusterRoleBinding(clusterRole string, mappings []config.OIDCRBACGroupMapping, groupsPrefix string) (string, error) {
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

	yamlBytes, err := yaml.Marshal(binding)
	if err != nil {
		return "", fmt.Errorf("failed to marshal ClusterRoleBinding YAML: %w", err)
	}
	return string(yamlBytes), nil
}

// generateRoleBinding creates a RoleBinding manifest.
// See: terraform/oidc.tf lines 45-71
func generateRoleBinding(role config.OIDCRBACRole, mappings []config.OIDCRBACGroupMapping, groupsPrefix string) (string, error) {
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

	yamlBytes, err := yaml.Marshal(binding)
	if err != nil {
		return "", fmt.Errorf("failed to marshal RoleBinding YAML: %w", err)
	}
	return string(yamlBytes), nil
}
