package addons

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestBuildClusterIssuerManifest_Staging(t *testing.T) {
	t.Parallel()
	email := "test@example.com"

	manifest, err := buildClusterIssuerManifest(email, false)
	require.NoError(t, err)
	require.NotEmpty(t, manifest)

	// Parse the YAML
	var issuer map[string]any
	err = yaml.Unmarshal(manifest, &issuer)
	require.NoError(t, err)

	// Check metadata
	assert.Equal(t, "cert-manager.io/v1", issuer["apiVersion"])
	assert.Equal(t, "ClusterIssuer", issuer["kind"])

	metadata := issuer["metadata"].(map[string]any)
	assert.Equal(t, "letsencrypt-cloudflare-staging", metadata["name"])

	// Check spec
	spec := issuer["spec"].(map[string]any)
	acme := spec["acme"].(map[string]any)
	assert.Equal(t, email, acme["email"])
	assert.Equal(t, "https://acme-staging-v02.api.letsencrypt.org/directory", acme["server"])

	// Check private key ref
	privateKeyRef := acme["privateKeySecretRef"].(map[string]any)
	assert.Equal(t, "letsencrypt-cloudflare-staging-key", privateKeyRef["name"])

	// Check solvers
	solvers := acme["solvers"].([]any)
	require.Len(t, solvers, 1)

	solver := solvers[0].(map[string]any)
	dns01 := solver["dns01"].(map[string]any)
	cloudflare := dns01["cloudflare"].(map[string]any)

	apiTokenRef := cloudflare["apiTokenSecretRef"].(map[string]any)
	assert.Equal(t, cloudflareSecretName, apiTokenRef["name"])
	assert.Equal(t, "api-token", apiTokenRef["key"])
}

func TestBuildClusterIssuerManifest_Production(t *testing.T) {
	t.Parallel()
	email := "admin@example.org"

	manifest, err := buildClusterIssuerManifest(email, true)
	require.NoError(t, err)
	require.NotEmpty(t, manifest)

	// Parse the YAML
	var issuer map[string]any
	err = yaml.Unmarshal(manifest, &issuer)
	require.NoError(t, err)

	// Check metadata
	metadata := issuer["metadata"].(map[string]any)
	assert.Equal(t, "letsencrypt-cloudflare-production", metadata["name"])

	// Check spec
	spec := issuer["spec"].(map[string]any)
	acme := spec["acme"].(map[string]any)
	assert.Equal(t, email, acme["email"])
	assert.Equal(t, "https://acme-v02.api.letsencrypt.org/directory", acme["server"])

	// Check private key ref
	privateKeyRef := acme["privateKeySecretRef"].(map[string]any)
	assert.Equal(t, "letsencrypt-cloudflare-production-key", privateKeyRef["name"])
}

func TestBuildClusterIssuerManifest_DifferentEmails(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		email string
	}{
		{"simple email", "test@example.com"},
		{"with subdomain", "admin@mail.example.com"},
		{"with plus", "test+cert@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			manifest, err := buildClusterIssuerManifest(tt.email, false)
			require.NoError(t, err)

			var issuer map[string]any
			err = yaml.Unmarshal(manifest, &issuer)
			require.NoError(t, err)

			spec := issuer["spec"].(map[string]any)
			acme := spec["acme"].(map[string]any)
			assert.Equal(t, tt.email, acme["email"])
		})
	}
}

func TestBuildClusterIssuerManifest_ValidYAML(t *testing.T) {
	t.Parallel()
	manifest, err := buildClusterIssuerManifest("test@example.com", false)
	require.NoError(t, err)

	// Verify it's valid YAML by unmarshaling
	var parsed any
	err = yaml.Unmarshal(manifest, &parsed)
	assert.NoError(t, err, "Generated manifest should be valid YAML")
}

func TestBuildClusterIssuerManifest_BothEnvironments(t *testing.T) {
	t.Parallel()
	email := "test@example.com"

	stagingManifest, err := buildClusterIssuerManifest(email, false)
	require.NoError(t, err)

	productionManifest, err := buildClusterIssuerManifest(email, true)
	require.NoError(t, err)

	// Verify they are different
	assert.NotEqual(t, string(stagingManifest), string(productionManifest))

	// Verify staging contains staging URLs
	assert.Contains(t, string(stagingManifest), "staging")
	assert.Contains(t, string(stagingManifest), "letsencrypt-cloudflare-staging")

	// Verify production contains production URLs
	assert.Contains(t, string(productionManifest), "production")
	assert.Contains(t, string(productionManifest), "letsencrypt-cloudflare-production")
	assert.NotContains(t, string(productionManifest), "staging")
}

func TestClusterIssuerData_Fields(t *testing.T) {
	t.Parallel()
	// Test the structure directly

	data := clusterIssuerData{
		Name:           "test-issuer",
		Email:          "test@example.com",
		Server:         "https://acme.example.com",
		PrivateKeyName: "test-key",
		SecretName:     "test-secret",
		SecretKey:      "api-token",
		Production:     true,
	}

	assert.Equal(t, "test-issuer", data.Name)
	assert.Equal(t, "test@example.com", data.Email)
	assert.Equal(t, "https://acme.example.com", data.Server)
	assert.Equal(t, "test-key", data.PrivateKeyName)
	assert.Equal(t, "test-secret", data.SecretName)
	assert.Equal(t, "api-token", data.SecretKey)
	assert.True(t, data.Production)
}
