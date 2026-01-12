package addons

import (
	"context"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"hcloud-k8s/internal/config"
)

// applyRBAC installs RBAC roles and cluster roles.
// See: terraform/rbac.tf
func applyRBAC(ctx context.Context, kubeconfigPath string, cfg *config.Config) error {
	if len(cfg.Addons.RBAC.Roles) == 0 && len(cfg.Addons.RBAC.ClusterRoles) == 0 {
		return nil // Nothing to apply
	}

	// Generate manifests
	manifests := generateRBACManifests(cfg.Addons.RBAC)

	// Combine with --- separator
	combined := strings.Join(manifests, "\n---\n")

	// Apply manifests
	if err := applyWithKubectl(ctx, kubeconfigPath, "kube-rbac", []byte(combined)); err != nil {
		return fmt.Errorf("failed to apply RBAC manifests: %w", err)
	}

	return nil
}

// generateRBACManifests creates YAML manifests for roles and cluster roles.
// See: terraform/rbac.tf lines 3-31
func generateRBACManifests(rbac config.RBACConfig) []string {
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

// generateRole creates a YAML manifest for a namespaced Role.
// See: terraform/rbac.tf lines 5-17
func generateRole(role config.RoleConfig) string {
	r := map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "Role",
		"metadata": map[string]any{
			"name":      role.Name,
			"namespace": role.Namespace,
		},
		"rules": buildRules(role.Rules),
	}

	yamlBytes, _ := yaml.Marshal(r)
	return string(yamlBytes)
}

// generateClusterRole creates a YAML manifest for a ClusterRole.
// See: terraform/rbac.tf lines 19-30
func generateClusterRole(role config.ClusterRoleConfig) string {
	r := map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRole",
		"metadata": map[string]any{
			"name": role.Name,
		},
		"rules": buildRules(role.Rules),
	}

	yamlBytes, _ := yaml.Marshal(r)
	return string(yamlBytes)
}

// buildRules converts config rules to RBAC rule format.
// See: terraform/rbac.tf lines 12-16, 25-29
func buildRules(rules []config.RBACRuleConfig) []map[string]any {
	result := make([]map[string]any, len(rules))
	for i, rule := range rules {
		result[i] = map[string]any{
			"apiGroups": rule.APIGroups,
			"resources": rule.Resources,
			"verbs":     rule.Verbs,
		}
	}
	return result
}
