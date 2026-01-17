package addons

import (
	"context"
	"fmt"
	"strings"
	"time"

	"hcloud-k8s/internal/addons/helm"
	"hcloud-k8s/internal/config"
)

// applyIngressNginx installs the NGINX Ingress Controller.
// See: terraform/ingress_nginx.tf
//
// This function handles the cert-manager integration carefully:
// 1. First creates the namespace
// 2. Applies cert-manager Certificate/Issuer resources
// 3. Waits for the admission webhook secret to be created by cert-manager
// 4. Then applies the rest of the manifests (Deployment, etc.)
//
// This two-phase approach is necessary because when using kubectl apply
// (instead of helm install), all resources are applied simultaneously.
// The Deployment would fail to start if the webhook secret doesn't exist yet.
func applyIngressNginx(ctx context.Context, kubeconfigPath string, cfg *config.Config) error {
	// Create namespace first
	namespaceYAML := createIngressNginxNamespace()
	if err := applyWithKubectl(ctx, kubeconfigPath, "ingress-nginx-namespace", []byte(namespaceYAML)); err != nil {
		return fmt.Errorf("failed to create ingress-nginx namespace: %w", err)
	}

	// Build values matching terraform configuration
	values := buildIngressNginxValues(cfg)

	// Render helm chart
	manifestBytes, err := helm.RenderChart("ingress-nginx", "ingress-nginx", values)
	if err != nil {
		return fmt.Errorf("failed to render ingress-nginx chart: %w", err)
	}

	// Split manifests into cert-manager resources and other resources
	// This is needed because cert-manager needs time to create the webhook secret
	certManagerResources, otherResources := splitIngressNginxManifests(string(manifestBytes))

	// Phase 1: Apply cert-manager resources (Issuer, Certificate)
	if len(certManagerResources) > 0 {
		// First, wait for cert-manager webhook to be ready
		// This is crucial because the webhook validates Certificate resources
		if err := waitForDeploymentReady(ctx, kubeconfigPath, "cert-manager", "cert-manager-webhook", 2*time.Minute); err != nil {
			return fmt.Errorf("failed waiting for cert-manager webhook: %w", err)
		}

		if err := applyWithKubectl(ctx, kubeconfigPath, "ingress-nginx-certmanager", []byte(certManagerResources)); err != nil {
			return fmt.Errorf("failed to apply ingress-nginx cert-manager resources: %w", err)
		}

		// The cert-manager chain needs time to process:
		// 1. self-signed-issuer → 2. root-cert (creates secret) → 3. root-issuer → 4. admission cert (creates secret)
		// First wait for the intermediate root-cert secret
		if err := waitForSecret(ctx, kubeconfigPath, "ingress-nginx", "ingress-nginx-root-cert", 3*time.Minute); err != nil {
			return fmt.Errorf("failed waiting for ingress-nginx root cert secret: %w", err)
		}

		// Then wait for the final admission webhook secret
		if err := waitForSecret(ctx, kubeconfigPath, "ingress-nginx", "ingress-nginx-admission", 3*time.Minute); err != nil {
			return fmt.Errorf("failed waiting for ingress-nginx admission secret: %w", err)
		}
	}

	// Phase 2: Apply the rest of the manifests
	if err := applyWithKubectl(ctx, kubeconfigPath, "ingress-nginx", []byte(otherResources)); err != nil {
		return fmt.Errorf("failed to apply ingress-nginx manifests: %w", err)
	}

	return nil
}

// splitIngressNginxManifests splits the rendered manifests into cert-manager resources
// and other resources. Cert-manager resources (Issuer, Certificate) need to be applied
// first and we need to wait for the secret to be created before applying other resources.
func splitIngressNginxManifests(manifests string) (certManagerResources, otherResources string) {
	var certManagerDocs []string
	var otherDocs []string

	// Split by YAML document separator
	docs := strings.Split(manifests, "\n---\n")

	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}

		// Check if this is a cert-manager resource (Issuer or Certificate)
		if isCertManagerResource(doc) {
			certManagerDocs = append(certManagerDocs, doc)
		} else {
			otherDocs = append(otherDocs, doc)
		}
	}

	return strings.Join(certManagerDocs, "\n---\n"), strings.Join(otherDocs, "\n---\n")
}

// isCertManagerResource checks if a YAML document is a cert-manager Issuer or Certificate.
func isCertManagerResource(doc string) bool {
	// Simple check for cert-manager resource types
	return strings.Contains(doc, "kind: Issuer") ||
		strings.Contains(doc, "kind: Certificate") ||
		strings.Contains(doc, "apiVersion: cert-manager.io/")
}

// buildIngressNginxValues creates helm values matching terraform configuration.
// See: terraform/ingress_nginx.tf lines 39-129
func buildIngressNginxValues(cfg *config.Config) helm.Values {
	workerCount := getWorkerCount(cfg)
	replicas := 2
	if workerCount >= 3 {
		replicas = 3
	}

	controller := buildIngressNginxController(workerCount, replicas)

	return helm.Values{
		"controller": controller,
	}
}

// buildIngressNginxController creates the controller configuration.
func buildIngressNginxController(workerCount, replicas int) helm.Values {
	controller := helm.Values{
		"admissionWebhooks": helm.Values{
			"certManager": helm.Values{
				// Use cert-manager to generate webhook certificates.
				// This avoids race conditions with kubectl apply where the
				// kube-webhook-certgen Jobs (which use Helm hooks) may not
				// complete before the controller deployment starts.
				// See: terraform/ingress_nginx.tf line 44
				"enabled": true,
			},
		},
		"kind":                       "Deployment",
		"replicaCount":               replicas,
		"minAvailable":               nil,
		"maxUnavailable":             1,
		"watchIngressWithoutClass":   true,
		"enableTopologyAwareRouting": false,
		"topologySpreadConstraints":  buildIngressNginxTopologySpread(workerCount),
		"metrics": helm.Values{
			"enabled": false,
		},
		"extraArgs": helm.Values{},
		"service": helm.Values{
			"type":                  "NodePort",
			"externalTrafficPolicy": "Local",
			"nodePorts": helm.Values{
				"http":  30000,
				"https": 30001,
			},
		},
		"config": helm.Values{
			"compute-full-forwarded-for": true,
			"use-proxy-protocol":         true,
		},
		"networkPolicy": helm.Values{
			"enabled": true,
		},
		// Add tolerations for CCM uninitialized taint
		// This allows ingress-nginx to schedule before CCM has fully initialized nodes
		"tolerations": []helm.Values{
			{
				"key":      "node.cloudprovider.kubernetes.io/uninitialized",
				"operator": "Exists",
			},
		},
	}

	return controller
}

// buildIngressNginxTopologySpread creates topology spread constraints for ingress-nginx.
// Two constraints: hostname (strict if multiple workers) and zone (soft).
func buildIngressNginxTopologySpread(workerCount int) []helm.Values {
	// Determine whenUnsatisfiable for hostname constraint
	hostnameUnsatisfiable := "ScheduleAnyway"
	if workerCount > 1 {
		hostnameUnsatisfiable = "DoNotSchedule"
	}

	labelSelector := helm.Values{
		"matchLabels": helm.Values{
			"app.kubernetes.io/instance":  "ingress-nginx",
			"app.kubernetes.io/name":      "ingress-nginx",
			"app.kubernetes.io/component": "controller",
		},
	}

	return []helm.Values{
		{
			"topologyKey":       "kubernetes.io/hostname",
			"maxSkew":           1,
			"whenUnsatisfiable": hostnameUnsatisfiable,
			"labelSelector":     labelSelector,
			"matchLabelKeys":    []string{"pod-template-hash"},
		},
		{
			"topologyKey":       "topology.kubernetes.io/zone",
			"maxSkew":           1,
			"whenUnsatisfiable": "ScheduleAnyway",
			"labelSelector":     labelSelector,
			"matchLabelKeys":    []string{"pod-template-hash"},
		},
	}
}

// createIngressNginxNamespace returns the ingress-nginx namespace manifest.
func createIngressNginxNamespace() string {
	return `apiVersion: v1
kind: Namespace
metadata:
  name: ingress-nginx
`
}
