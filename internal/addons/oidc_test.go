package addons

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"hcloud-k8s/internal/config"
)

func TestCollectUniqueClusterRoles(t *testing.T) {
	mappings := []config.OIDCGroupMapping{
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
	mappings := []config.OIDCGroupMapping{
		{
			Group: "team-a",
			Roles: []config.OIDCRole{
				{Name: "developer", Namespace: "team-a"},
				{Name: "viewer", Namespace: "default"},
			},
		},
		{
			Group: "team-b",
			Roles: []config.OIDCRole{
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
	mappings := []config.OIDCGroupMapping{
		{
			Group:        "admins",
			ClusterRoles: []string{"cluster-admin", "view"},
		},
		{
			Group:        "developers",
			ClusterRoles: []string{"view"},
		},
	}

	yaml := generateClusterRoleBinding("view", mappings, "oidc:")

	// Verify structure
	assert.Contains(t, yaml, "apiVersion: rbac.authorization.k8s.io/v1")
	assert.Contains(t, yaml, "kind: ClusterRoleBinding")
	assert.Contains(t, yaml, "name: oidc-view")
	assert.Contains(t, yaml, "roleRef:")
	assert.Contains(t, yaml, "kind: ClusterRole")
	assert.Contains(t, yaml, "name: view")

	// Verify subjects include both groups with prefix
	assert.Contains(t, yaml, "oidc:admins")
	assert.Contains(t, yaml, "oidc:developers")

	// Parse and verify structure
	var binding map[string]any
	err := yaml3.Unmarshal([]byte(yaml), &binding)
	require.NoError(t, err)

	subjects := binding["subjects"].([]any)
	assert.Len(t, subjects, 2)
}

func TestGenerateClusterRoleBindingSingleGroup(t *testing.T) {
	mappings := []config.OIDCGroupMapping{
		{
			Group:        "admins",
			ClusterRoles: []string{"cluster-admin"},
		},
	}

	yaml := generateClusterRoleBinding("cluster-admin", mappings, "")

	// Without prefix
	assert.Contains(t, yaml, "name: admins")
	assert.NotContains(t, yaml, "name: :admins")

	// Parse and verify single subject
	var binding map[string]any
	err := yaml3.Unmarshal([]byte(yaml), &binding)
	require.NoError(t, err)

	subjects := binding["subjects"].([]any)
	assert.Len(t, subjects, 1)
}

func TestGenerateRoleBinding(t *testing.T) {
	role := config.OIDCRole{
		Name:      "developer",
		Namespace: "team-a",
	}

	mappings := []config.OIDCGroupMapping{
		{
			Group: "team-a-devs",
			Roles: []config.OIDCRole{
				{Name: "developer", Namespace: "team-a"},
			},
		},
		{
			Group: "team-a-leads",
			Roles: []config.OIDCRole{
				{Name: "developer", Namespace: "team-a"},
			},
		},
	}

	yaml := generateRoleBinding(role, mappings, "oidc:")

	// Verify structure
	assert.Contains(t, yaml, "apiVersion: rbac.authorization.k8s.io/v1")
	assert.Contains(t, yaml, "kind: RoleBinding")
	assert.Contains(t, yaml, "name: oidc-developer")
	assert.Contains(t, yaml, "namespace: team-a")
	assert.Contains(t, yaml, "roleRef:")
	assert.Contains(t, yaml, "kind: Role")
	assert.Contains(t, yaml, "name: developer")

	// Verify subjects include both groups with prefix
	assert.Contains(t, yaml, "oidc:team-a-devs")
	assert.Contains(t, yaml, "oidc:team-a-leads")

	// Parse and verify structure
	var binding map[string]any
	err := yaml3.Unmarshal([]byte(yaml), &binding)
	require.NoError(t, err)

	subjects := binding["subjects"].([]any)
	assert.Len(t, subjects, 2)

	metadata := binding["metadata"].(map[string]any)
	assert.Equal(t, "team-a", metadata["namespace"])
}

func TestGenerateRoleBindingSingleGroup(t *testing.T) {
	role := config.OIDCRole{
		Name:      "viewer",
		Namespace: "default",
	}

	mappings := []config.OIDCGroupMapping{
		{
			Group: "viewers",
			Roles: []config.OIDCRole{
				{Name: "viewer", Namespace: "default"},
			},
		},
	}

	yaml := generateRoleBinding(role, mappings, "")

	// Without prefix
	assert.Contains(t, yaml, "name: viewers")

	// Parse and verify single subject
	var binding map[string]any
	err := yaml3.Unmarshal([]byte(yaml), &binding)
	require.NoError(t, err)

	subjects := binding["subjects"].([]any)
	assert.Len(t, subjects, 1)
}

func TestOIDCFullFlow(t *testing.T) {
	oidc := config.OIDCConfig{
		Enabled:      true,
		GroupsPrefix: "oidc:",
		GroupMappings: []config.OIDCGroupMapping{
			{
				Group:        "admins",
				ClusterRoles: []string{"cluster-admin"},
				Roles: []config.OIDCRole{
					{Name: "developer", Namespace: "default"},
				},
			},
			{
				Group:        "developers",
				ClusterRoles: []string{"view"},
				Roles: []config.OIDCRole{
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
		binding := generateClusterRoleBinding(clusterRole, oidc.GroupMappings, oidc.GroupsPrefix)
		manifests = append(manifests, binding)
	}

	for _, role := range roles {
		binding := generateRoleBinding(role, oidc.GroupMappings, oidc.GroupsPrefix)
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
	mappings := []config.OIDCGroupMapping{}

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
			mappings := []config.OIDCGroupMapping{
				{
					Group:        tt.group,
					ClusterRoles: []string{"view"},
				},
			}

			yaml := generateClusterRoleBinding("view", mappings, tt.prefix)
			assert.Contains(t, yaml, "name: "+tt.want)
		})
	}
}
