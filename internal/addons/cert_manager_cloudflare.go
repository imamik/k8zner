package addons

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	"github.com/imamik/k8zner/internal/addons/k8sclient"
	"github.com/imamik/k8zner/internal/config"
)

// applyCertManagerCloudflare creates ClusterIssuers for Let's Encrypt with Cloudflare DNS01 solver.
// This is applied after cert-manager is installed and Cloudflare secrets are created.
func applyCertManagerCloudflare(ctx context.Context, client k8sclient.Client, cfg *config.Config) error {
	cfCfg := cfg.Addons.CertManager.Cloudflare

	// Create staging ClusterIssuer
	stagingManifest, err := buildClusterIssuerManifest(cfCfg.Email, false)
	if err != nil {
		return fmt.Errorf("failed to build staging ClusterIssuer manifest: %w", err)
	}
	if err := applyManifests(ctx, client, "letsencrypt-cloudflare-staging", stagingManifest); err != nil {
		return fmt.Errorf("failed to apply staging ClusterIssuer: %w", err)
	}

	// Create production ClusterIssuer
	productionManifest, err := buildClusterIssuerManifest(cfCfg.Email, true)
	if err != nil {
		return fmt.Errorf("failed to build production ClusterIssuer manifest: %w", err)
	}
	if err := applyManifests(ctx, client, "letsencrypt-cloudflare-production", productionManifest); err != nil {
		return fmt.Errorf("failed to apply production ClusterIssuer: %w", err)
	}

	return nil
}

// buildClusterIssuerManifest creates a ClusterIssuer manifest for Let's Encrypt with Cloudflare DNS01.
func buildClusterIssuerManifest(email string, production bool) ([]byte, error) {
	data := clusterIssuerData{
		Email:          email,
		SecretName:     cloudflareSecretName,
		SecretKey:      "api-token",
		Production:     production,
		Name:           "letsencrypt-cloudflare-staging",
		Server:         "https://acme-staging-v02.api.letsencrypt.org/directory",
		PrivateKeyName: "letsencrypt-cloudflare-staging-key",
	}

	if production {
		data.Name = "letsencrypt-cloudflare-production"
		data.Server = "https://acme-v02.api.letsencrypt.org/directory"
		data.PrivateKeyName = "letsencrypt-cloudflare-production-key"
	}

	tmpl, err := template.New("clusterissuer").Parse(clusterIssuerTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ClusterIssuer template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute ClusterIssuer template: %w", err)
	}

	return buf.Bytes(), nil
}

// clusterIssuerData holds the data for rendering the ClusterIssuer template.
type clusterIssuerData struct {
	Name           string
	Email          string
	Server         string
	PrivateKeyName string
	SecretName     string
	SecretKey      string
	Production     bool
}

// clusterIssuerTemplate is the YAML template for a ClusterIssuer with Cloudflare DNS01 solver.
const clusterIssuerTemplate = `apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: {{ .Name }}
spec:
  acme:
    # Email address for Let's Encrypt account
    email: {{ .Email }}
    # ACME server URL
    server: {{ .Server }}
    # Secret to store the ACME account private key
    privateKeySecretRef:
      name: {{ .PrivateKeyName }}
    # DNS01 solver using Cloudflare
    solvers:
    - dns01:
        cloudflare:
          apiTokenSecretRef:
            name: {{ .SecretName }}
            key: {{ .SecretKey }}
`
