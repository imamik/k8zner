package addons

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yamlpkg "gopkg.in/yaml.v3"

	"github.com/imamik/k8zner/internal/config"
)

func TestCollectUniqueClusterRoles(t *testing.T) {
	mappings := []config.OIDCRBACGroupMapping{
		{
			Group:        "admins",
			ClusterRoles: []string{"cluster-admin", "view"},
		},
		{
			Group:        "developers",
			ClusterRoles: []string{"view", "edit"},
		},
		{
			Group:        "viewers",
			ClusterRoles: []string{"view"},
		},
	}

	roles := collectUniqueClusterRoles(mappings)

	// Should have unique roles only
	assert.Len(t, roles, 3)
	assert.Contains(t, roles, "cluster-admin")
	assert.Contains(t, roles, "view")
	assert.Contains(t, roles, "edit")
}

func TestCollectUniqueRoles(t *testing.T) {
	mappings := []config.OIDCRBACGroupMapping{
		{
			Group: "team-a",
			Roles: []config.OIDCRBACRole{
				{Name: "developer", Namespace: "team-a"},
				{Name: "viewer", Namespace: "default"},
			},
		},
		{
			Group: "team-b",
			Roles: []config.OIDCRBACRole{
				{Name: "developer", Namespace: "team-b"},
				{Name: "viewer", Namespace: "default"}, // Duplicate
			},
		},
	}

	roles := collectUniqueRoles(mappings)

	// Should have 3 unique roles (viewer in default is shared)
	assert.Len(t, roles, 3)

	// Check roles exist
	roleNames := make(map[string]string)
	for _, role := range roles {
		roleNames[role.Namespace+"/"+role.Name] = role.Name
	}

	assert.Contains(t, roleNames, "team-a/developer")
	assert.Contains(t, roleNames, "team-b/developer")
	assert.Contains(t, roleNames, "default/viewer")
}

func TestGenerateClusterRoleBinding(t *testing.T) {
	mappings := []config.OIDCRBACGroupMapping{
		{
			Group:        "admins",
			ClusterRoles: []string{"cluster-admin", "view"},
		},
		{
			Group:        "developers",
			ClusterRoles: []string{"view"},
		},
	}

	yamlStr, err := generateClusterRoleBinding("view", mappings, "oidc:")
	require.NoError(t, err)

	// Verify structure
	assert.Contains(t, yamlStr, "apiVersion: rbac.authorization.k8s.io/v1")
	assert.Contains(t, yamlStr, "kind: ClusterRoleBinding")
	assert.Contains(t, yamlStr, "name: oidc-view")
	assert.Contains(t, yamlStr, "roleRef:")
	assert.Contains(t, yamlStr, "kind: ClusterRole")
	assert.Contains(t, yamlStr, "name: view")

	// Verify subjects include both groups with prefix
	assert.Contains(t, yamlStr, "oidc:admins")
	assert.Contains(t, yamlStr, "oidc:developers")

	// Parse and verify structure
	var binding map[string]any
	err = yamlpkg.Unmarshal([]byte(yamlStr), &binding)
	require.NoError(t, err)

	subjects := binding["subjects"].([]any)
	assert.Len(t, subjects, 2)
}

func TestGenerateClusterRoleBindingSingleGroup(t *testing.T) {
	mappings := []config.OIDCRBACGroupMapping{
		{
			Group:        "admins",
			ClusterRoles: []string{"cluster-admin"},
		},
	}

	yamlStr, err := generateClusterRoleBinding("cluster-admin", mappings, "")
	require.NoError(t, err)

	// Without prefix
	assert.Contains(t, yamlStr, "name: admins")
	assert.NotContains(t, yamlStr, "name: :admins")

	// Parse and verify single subject
	var binding map[string]any
	err = yamlpkg.Unmarshal([]byte(yamlStr), &binding)
	require.NoError(t, err)

	subjects := binding["subjects"].([]any)
	assert.Len(t, subjects, 1)
}

func TestGenerateRoleBinding(t *testing.T) {
	role := config.OIDCRBACRole{
		Name:      "developer",
		Namespace: "team-a",
	}

	mappings := []config.OIDCRBACGroupMapping{
		{
			Group: "team-a-devs",
			Roles: []config.OIDCRBACRole{
				{Name: "developer", Namespace: "team-a"},
			},
		},
		{
			Group: "team-a-leads",
			Roles: []config.OIDCRBACRole{
				{Name: "developer", Namespace: "team-a"},
			},
		},
	}

	yamlStr, err := generateRoleBinding(role, mappings, "oidc:")
	require.NoError(t, err)

	// Verify structure
	assert.Contains(t, yamlStr, "apiVersion: rbac.authorization.k8s.io/v1")
	assert.Contains(t, yamlStr, "kind: RoleBinding")
	assert.Contains(t, yamlStr, "name: oidc-developer")
	assert.Contains(t, yamlStr, "namespace: team-a")
	assert.Contains(t, yamlStr, "roleRef:")
	assert.Contains(t, yamlStr, "kind: Role")
	assert.Contains(t, yamlStr, "name: developer")

	// Verify subjects include both groups with prefix
	assert.Contains(t, yamlStr, "oidc:team-a-devs")
	assert.Contains(t, yamlStr, "oidc:team-a-leads")

	// Parse and verify structure
	var binding map[string]any
	err = yamlpkg.Unmarshal([]byte(yamlStr), &binding)
	require.NoError(t, err)

	subjects := binding["subjects"].([]any)
	assert.Len(t, subjects, 2)

	metadata := binding["metadata"].(map[string]any)
	assert.Equal(t, "team-a", metadata["namespace"])
}

func TestGenerateRoleBindingSingleGroup(t *testing.T) {
	role := config.OIDCRBACRole{
		Name:      "viewer",
		Namespace: "default",
	}

	mappings := []config.OIDCRBACGroupMapping{
		{
			Group: "viewers",
			Roles: []config.OIDCRBACRole{
				{Name: "viewer", Namespace: "default"},
			},
		},
	}

	yamlStr, err := generateRoleBinding(role, mappings, "")
	require.NoError(t, err)

	// Without prefix
	assert.Contains(t, yamlStr, "name: viewers")

	// Parse and verify single subject
	var binding map[string]any
	err = yamlpkg.Unmarshal([]byte(yamlStr), &binding)
	require.NoError(t, err)

	subjects := binding["subjects"].([]any)
	assert.Len(t, subjects, 1)
}

func TestOIDCFullFlow(t *testing.T) {
	oidc := config.OIDCRBACConfig{
		Enabled:      true,
		GroupsPrefix: "oidc:",
		GroupMappings: []config.OIDCRBACGroupMapping{
			{
				Group:        "admins",
				ClusterRoles: []string{"cluster-admin"},
				Roles: []config.OIDCRBACRole{
					{Name: "developer", Namespace: "default"},
				},
			},
			{
				Group:        "developers",
				ClusterRoles: []string{"view"},
				Roles: []config.OIDCRBACRole{
					{Name: "developer", Namespace: "default"},
					{Name: "viewer", Namespace: "team-a"},
				},
			},
		},
	}

	// Collect unique cluster roles
	clusterRoles := collectUniqueClusterRoles(oidc.GroupMappings)
	assert.Len(t, clusterRoles, 2) // cluster-admin, view

	// Collect unique roles
	roles := collectUniqueRoles(oidc.GroupMappings)
	assert.Len(t, roles, 2) // default/developer, team-a/viewer

	// Generate all manifests
	var manifests []string

	for _, clusterRole := range clusterRoles {
		binding, err := generateClusterRoleBinding(clusterRole, oidc.GroupMappings, oidc.GroupsPrefix)
		require.NoError(t, err)
		manifests = append(manifests, binding)
	}

	for _, role := range roles {
		binding, err := generateRoleBinding(role, oidc.GroupMappings, oidc.GroupsPrefix)
		require.NoError(t, err)
		manifests = append(manifests, binding)
	}

	assert.Len(t, manifests, 4) // 2 ClusterRoleBindings + 2 RoleBindings

	// Verify combined manifest
	combined := strings.Join(manifests, "\n---\n")
	assert.Contains(t, combined, "ClusterRoleBinding")
	assert.Contains(t, combined, "RoleBinding")

	// Count separators
	separatorCount := strings.Count(combined, "\n---\n")
	assert.Equal(t, 3, separatorCount)
}

func TestOIDCEmptyMappings(t *testing.T) {
	mappings := []config.OIDCRBACGroupMapping{}

	clusterRoles := collectUniqueClusterRoles(mappings)
	assert.Len(t, clusterRoles, 0)

	roles := collectUniqueRoles(mappings)
	assert.Len(t, roles, 0)
}

func TestOIDCGroupsPrefix(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		group  string
		want   string
	}{
		{
			name:   "with prefix",
			prefix: "oidc:",
			group:  "admins",
			want:   "oidc:admins",
		},
		{
			name:   "without prefix",
			prefix: "",
			group:  "admins",
			want:   "admins",
		},
		{
			name:   "custom prefix",
			prefix: "company/",
			group:  "team-a",
			want:   "company/team-a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mappings := []config.OIDCRBACGroupMapping{
				{
					Group:        tt.group,
					ClusterRoles: []string{"view"},
				},
			}

			yamlStr, err := generateClusterRoleBinding("view", mappings, tt.prefix)
			require.NoError(t, err)
			assert.Contains(t, yamlStr, "name: "+tt.want)
		})
	}
}
