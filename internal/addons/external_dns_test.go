package addons

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/imamik/k8zner/internal/addons/helm"
	"github.com/imamik/k8zner/internal/config"
)

func TestBuildExternalDNSValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		config         *config.Config
		expectedOwner  string
		expectedPolicy string
		checkExtraArgs func(t *testing.T, extraArgs []string)
	}{
		{
			name: "default values with cluster name as owner ID",
			config: &config.Config{
				ClusterName: "my-cluster",
				Addons: config.AddonsConfig{
					ExternalDNS: config.ExternalDNSConfig{
						Enabled: true,
					},
					Cloudflare: config.CloudflareConfig{
						Domain: "example.com",
					},
				},
			},
			expectedOwner:  "my-cluster",
			expectedPolicy: "sync",
			checkExtraArgs: func(t *testing.T, extraArgs []string) {
				assert.Empty(t, extraArgs, "No extra args without proxied or zone ID")
			},
		},
		{
			name: "custom owner ID",
			config: &config.Config{
				ClusterName: "my-cluster",
				Addons: config.AddonsConfig{
					ExternalDNS: config.ExternalDNSConfig{
						Enabled:    true,
						TXTOwnerID: "custom-owner",
					},
					Cloudflare: config.CloudflareConfig{
						Domain: "example.com",
					},
				},
			},
			expectedOwner:  "custom-owner",
			expectedPolicy: "sync",
		},
		{
			name: "upsert-only policy",
			config: &config.Config{
				ClusterName: "my-cluster",
				Addons: config.AddonsConfig{
					ExternalDNS: config.ExternalDNSConfig{
						Enabled: true,
						Policy:  "upsert-only",
					},
					Cloudflare: config.CloudflareConfig{
						Domain: "example.com",
					},
				},
			},
			expectedOwner:  "my-cluster",
			expectedPolicy: "upsert-only",
		},
		{
			name: "cloudflare proxied enabled",
			config: &config.Config{
				ClusterName: "my-cluster",
				Addons: config.AddonsConfig{
					ExternalDNS: config.ExternalDNSConfig{
						Enabled: true,
					},
					Cloudflare: config.CloudflareConfig{
						Domain:  "example.com",
						Proxied: true,
					},
				},
			},
			expectedOwner:  "my-cluster",
			expectedPolicy: "sync",
			checkExtraArgs: func(t *testing.T, extraArgs []string) {
				assert.Contains(t, extraArgs, "--cloudflare-proxied")
			},
		},
		{
			name: "cloudflare zone ID specified",
			config: &config.Config{
				ClusterName: "my-cluster",
				Addons: config.AddonsConfig{
					ExternalDNS: config.ExternalDNSConfig{
						Enabled: true,
					},
					Cloudflare: config.CloudflareConfig{
						Domain: "example.com",
						ZoneID: "abc123",
					},
				},
			},
			expectedOwner:  "my-cluster",
			expectedPolicy: "sync",
			checkExtraArgs: func(t *testing.T, extraArgs []string) {
				assert.Contains(t, extraArgs, "--zone-id-filter=abc123")
			},
		},
		{
			name: "proxied and zone ID together",
			config: &config.Config{
				ClusterName: "my-cluster",
				Addons: config.AddonsConfig{
					ExternalDNS: config.ExternalDNSConfig{
						Enabled: true,
					},
					Cloudflare: config.CloudflareConfig{
						Domain:  "example.com",
						Proxied: true,
						ZoneID:  "xyz789",
					},
				},
			},
			expectedOwner:  "my-cluster",
			expectedPolicy: "sync",
			checkExtraArgs: func(t *testing.T, extraArgs []string) {
				assert.Contains(t, extraArgs, "--cloudflare-proxied")
				assert.Contains(t, extraArgs, "--zone-id-filter=xyz789")
			},
		},
		{
			name: "custom sources",
			config: &config.Config{
				ClusterName: "my-cluster",
				Addons: config.AddonsConfig{
					ExternalDNS: config.ExternalDNSConfig{
						Enabled: true,
						Sources: []string{"ingress", "service"},
					},
					Cloudflare: config.CloudflareConfig{
						Domain: "example.com",
					},
				},
			},
			expectedOwner:  "my-cluster",
			expectedPolicy: "sync",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			values := buildExternalDNSValues(tt.config)

			// Check owner ID
			assert.Equal(t, tt.expectedOwner, values["txtOwnerId"])

			// Check policy
			assert.Equal(t, tt.expectedPolicy, values["policy"])

			// Check provider
			provider := values["provider"].(helm.Values)
			assert.Equal(t, "cloudflare", provider["name"])

			// Check domain filters
			domainFilters := values["domainFilters"].([]string)
			if tt.config.Addons.Cloudflare.Domain != "" {
				assert.Contains(t, domainFilters, tt.config.Addons.Cloudflare.Domain)
			}

			// Check extra args
			if tt.checkExtraArgs != nil {
				extraArgs := values["extraArgs"].([]string)
				tt.checkExtraArgs(t, extraArgs)
			}

			// Check sources
			sources := values["sources"].([]string)
			if len(tt.config.Addons.ExternalDNS.Sources) > 0 {
				assert.Equal(t, tt.config.Addons.ExternalDNS.Sources, sources)
			} else {
				assert.Equal(t, []string{"ingress"}, sources)
			}
		})
	}
}

func TestBuildExternalDNSValues_StructureComplete(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			ExternalDNS: config.ExternalDNSConfig{
				Enabled: true,
			},
			Cloudflare: config.CloudflareConfig{
				Domain: "example.com",
			},
		},
	}

	values := buildExternalDNSValues(cfg)

	// Check env vars for CF_API_TOKEN
	env := values["env"].([]helm.Values)
	require.Len(t, env, 1)
	assert.Equal(t, "CF_API_TOKEN", env[0]["name"])

	// Check node affinity (should schedule on worker nodes, NOT control plane)
	affinity, ok := values["affinity"].(helm.Values)
	require.True(t, ok, "affinity should exist")
	nodeAffinity, ok := affinity["nodeAffinity"].(helm.Values)
	require.True(t, ok, "nodeAffinity should exist")
	_, ok = nodeAffinity["requiredDuringSchedulingIgnoredDuringExecution"]
	assert.True(t, ok, "requiredDuringSchedulingIgnoredDuringExecution should exist")

	// Check tolerations
	tolerations := values["tolerations"].([]helm.Values)
	assert.Len(t, tolerations, 1) // Only CCM uninitialized toleration

	// Check service account
	serviceAccount := values["serviceAccount"].(helm.Values)
	assert.True(t, serviceAccount["create"].(bool))
	assert.Equal(t, "external-dns", serviceAccount["name"])

	// Check RBAC
	rbac := values["rbac"].(helm.Values)
	assert.True(t, rbac["create"].(bool))

	// Check other settings
	assert.Equal(t, "info", values["logLevel"])
	assert.Equal(t, "text", values["logFormat"])
	assert.Equal(t, "1m", values["interval"])
	assert.Equal(t, "txt", values["registry"])
	assert.Equal(t, 1, values["replicaCount"])
}

func TestBuildExternalDNSValues_NoDomain(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			ExternalDNS: config.ExternalDNSConfig{
				Enabled: true,
			},
			Cloudflare: config.CloudflareConfig{
				// No domain set
			},
		},
	}

	values := buildExternalDNSValues(cfg)

	domainFilters := values["domainFilters"].([]string)
	assert.Empty(t, domainFilters)
}

func TestBuildExternalDNSValues_PDBConfig(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			ExternalDNS: config.ExternalDNSConfig{
				Enabled: true,
			},
			Cloudflare: config.CloudflareConfig{
				Domain: "example.com",
			},
		},
	}

	values := buildExternalDNSValues(cfg)

	pdb := values["podDisruptionBudget"].(helm.Values)
	assert.True(t, pdb["enabled"].(bool))
	assert.Equal(t, 1, pdb["minAvailable"])
}
