package addons

import (
	"context"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"hcloud-k8s/internal/addons/k8sclient"
	"hcloud-k8s/internal/config"
)

// applyRBAC installs RBAC roles and cluster roles.
// See: terraform/rbac.tf
func applyRBAC(ctx context.Context, client k8sclient.Client, cfg *config.Config) error {
	if len(cfg.Addons.RBAC.Roles) == 0 && len(cfg.Addons.RBAC.ClusterRoles) == 0 {
		return nil // Nothing to apply
	}

	// Generate manifests
	manifests, err := generateRBACManifests(cfg.Addons.RBAC)
	if err != nil {
		return fmt.Errorf("failed to generate RBAC manifests: %w", err)
	}

	// Combine with --- separator
	combined := strings.Join(manifests, "\n---\n")

	// Apply manifests
	if err := applyManifests(ctx, client, "kube-rbac", []byte(combined)); err != nil {
		return fmt.Errorf("failed to apply RBAC manifests: %w", err)
	}

	return nil
}

// generateRBACManifests creates YAML manifests for roles and cluster roles.
// See: terraform/rbac.tf lines 3-31
func generateRBACManifests(rbac config.RBACConfig) ([]string, error) {
	var manifests []string

	// Generate Role manifests
	for _, role := range rbac.Roles {
		manifest, err := generateRole(role)
		if err != nil {
			return nil, fmt.Errorf("failed to generate Role %s: %w", role.Name, err)
		}
		manifests = append(manifests, manifest)
	}

	// Generate ClusterRole manifests
	for _, role := range rbac.ClusterRoles {
		manifest, err := generateClusterRole(role)
		if err != nil {
			return nil, fmt.Errorf("failed to generate ClusterRole %s: %w", role.Name, err)
		}
		manifests = append(manifests, manifest)
	}

	return manifests, nil
}

// generateRole creates a YAML manifest for a namespaced Role.
// See: terraform/rbac.tf lines 5-17
func generateRole(role config.RoleConfig) (string, error) {
	r := map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "Role",
		"metadata": map[string]any{
			"name":      role.Name,
			"namespace": role.Namespace,
		},
		"rules": buildRules(role.Rules),
	}

	yamlBytes, err := yaml.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Role YAML: %w", err)
	}
	return string(yamlBytes), nil
}

// generateClusterRole creates a YAML manifest for a ClusterRole.
// See: terraform/rbac.tf lines 19-30
func generateClusterRole(role config.ClusterRoleConfig) (string, error) {
	r := map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRole",
		"metadata": map[string]any{
			"name": role.Name,
		},
		"rules": buildRules(role.Rules),
	}

	yamlBytes, err := yaml.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("failed to marshal ClusterRole YAML: %w", err)
	}
	return string(yamlBytes), nil
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
