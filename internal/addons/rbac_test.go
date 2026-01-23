package addons

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"hcloud-k8s/internal/config"
)

func TestGenerateRBACManifests(t *testing.T) {
	rbac := config.RBACConfig{
		Enabled: true,
		Roles: []config.RoleConfig{
			{
				Name:      "developer",
				Namespace: "dev",
				Rules: []config.RBACRuleConfig{
					{
						APIGroups: []string{""},
						Resources: []string{"pods", "services"},
						Verbs:     []string{"get", "list", "watch"},
					},
				},
			},
		},
		ClusterRoles: []config.ClusterRoleConfig{
			{
				Name: "cluster-viewer",
				Rules: []config.RBACRuleConfig{
					{
						APIGroups: []string{""},
						Resources: []string{"nodes"},
						Verbs:     []string{"get", "list"},
					},
				},
			},
		},
	}

	manifests, err := generateRBACManifests(rbac)
	require.NoError(t, err)

	assert.Len(t, manifests, 2)

	// First manifest should be the Role
	var roleManifest map[string]any
	err = yaml.Unmarshal([]byte(manifests[0]), &roleManifest)
	require.NoError(t, err)
	assert.Equal(t, "rbac.authorization.k8s.io/v1", roleManifest["apiVersion"])
	assert.Equal(t, "Role", roleManifest["kind"])

	metadata := roleManifest["metadata"].(map[string]any)
	assert.Equal(t, "developer", metadata["name"])
	assert.Equal(t, "dev", metadata["namespace"])

	// Second manifest should be the ClusterRole
	var clusterRoleManifest map[string]any
	err = yaml.Unmarshal([]byte(manifests[1]), &clusterRoleManifest)
	require.NoError(t, err)
	assert.Equal(t, "rbac.authorization.k8s.io/v1", clusterRoleManifest["apiVersion"])
	assert.Equal(t, "ClusterRole", clusterRoleManifest["kind"])

	clusterMetadata := clusterRoleManifest["metadata"].(map[string]any)
	assert.Equal(t, "cluster-viewer", clusterMetadata["name"])
}

func TestGenerateRole(t *testing.T) {
	role := config.RoleConfig{
		Name:      "pod-reader",
		Namespace: "default",
		Rules: []config.RBACRuleConfig{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"apps"},
				Resources: []string{"deployments"},
				Verbs:     []string{"get"},
			},
		},
	}

	yamlStr, err := generateRole(role)
	require.NoError(t, err)

	// Verify structure
	assert.Contains(t, yamlStr, "apiVersion: rbac.authorization.k8s.io/v1")
	assert.Contains(t, yamlStr, "kind: Role")
	assert.Contains(t, yamlStr, "name: pod-reader")
	assert.Contains(t, yamlStr, "namespace: default")
	assert.Contains(t, yamlStr, "rules:")
	assert.Contains(t, yamlStr, "pods")
	assert.Contains(t, yamlStr, "deployments")
	assert.Contains(t, yamlStr, "get")
	assert.Contains(t, yamlStr, "list")
	assert.Contains(t, yamlStr, "watch")
}

func TestGenerateClusterRole(t *testing.T) {
	role := config.ClusterRoleConfig{
		Name: "node-reader",
		Rules: []config.RBACRuleConfig{
			{
				APIGroups: []string{""},
				Resources: []string{"nodes"},
				Verbs:     []string{"get", "list"},
			},
		},
	}

	yamlStr, err := generateClusterRole(role)
	require.NoError(t, err)

	// Verify structure
	assert.Contains(t, yamlStr, "apiVersion: rbac.authorization.k8s.io/v1")
	assert.Contains(t, yamlStr, "kind: ClusterRole")
	assert.Contains(t, yamlStr, "name: node-reader")
	assert.Contains(t, yamlStr, "rules:")
	assert.Contains(t, yamlStr, "nodes")
	assert.Contains(t, yamlStr, "get")
	assert.Contains(t, yamlStr, "list")

	// ClusterRole should not have namespace
	assert.NotContains(t, yamlStr, "namespace:")
}

func TestBuildRules(t *testing.T) {
	rules := []config.RBACRuleConfig{
		{
			APIGroups: []string{""},
			Resources: []string{"pods", "services"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"apps", "extensions"},
			Resources: []string{"deployments", "replicasets"},
			Verbs:     []string{"create", "update", "delete"},
		},
	}

	result := buildRules(rules)

	assert.Len(t, result, 2)

	// First rule
	assert.Equal(t, []string{""}, result[0]["apiGroups"])
	assert.Equal(t, []string{"pods", "services"}, result[0]["resources"])
	assert.Equal(t, []string{"get", "list", "watch"}, result[0]["verbs"])

	// Second rule
	assert.Equal(t, []string{"apps", "extensions"}, result[1]["apiGroups"])
	assert.Equal(t, []string{"deployments", "replicasets"}, result[1]["resources"])
	assert.Equal(t, []string{"create", "update", "delete"}, result[1]["verbs"])
}

func TestGenerateRBACManifestsEmpty(t *testing.T) {
	rbac := config.RBACConfig{
		Enabled:      true,
		Roles:        []config.RoleConfig{},
		ClusterRoles: []config.ClusterRoleConfig{},
	}

	manifests, err := generateRBACManifests(rbac)
	require.NoError(t, err)

	assert.Len(t, manifests, 0)
}

func TestGenerateRBACManifestsCombination(t *testing.T) {
	tests := []struct {
		name         string
		roleCount    int
		clusterCount int
	}{
		{
			name:         "only roles",
			roleCount:    3,
			clusterCount: 0,
		},
		{
			name:         "only cluster roles",
			roleCount:    0,
			clusterCount: 2,
		},
		{
			name:         "mixed",
			roleCount:    2,
			clusterCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rbac := config.RBACConfig{
				Enabled: true,
			}

			// Add roles
			for i := 0; i < tt.roleCount; i++ {
				rbac.Roles = append(rbac.Roles, config.RoleConfig{
					Name:      "role-" + string(rune(i)),
					Namespace: "default",
					Rules: []config.RBACRuleConfig{
						{
							APIGroups: []string{""},
							Resources: []string{"pods"},
							Verbs:     []string{"get"},
						},
					},
				})
			}

			// Add cluster roles
			for i := 0; i < tt.clusterCount; i++ {
				rbac.ClusterRoles = append(rbac.ClusterRoles, config.ClusterRoleConfig{
					Name: "cluster-role-" + string(rune(i)),
					Rules: []config.RBACRuleConfig{
						{
							APIGroups: []string{""},
							Resources: []string{"nodes"},
							Verbs:     []string{"get"},
						},
					},
				})
			}

			manifests, err := generateRBACManifests(rbac)
			require.NoError(t, err)
			expectedCount := tt.roleCount + tt.clusterCount
			assert.Len(t, manifests, expectedCount)

			// Verify combined manifest would work
			combined := strings.Join(manifests, "\n---\n")
			assert.Contains(t, combined, "apiVersion: rbac.authorization.k8s.io/v1")

			// Count separators
			separatorCount := strings.Count(combined, "\n---\n")
			assert.Equal(t, expectedCount-1, separatorCount)
		})
	}
}
