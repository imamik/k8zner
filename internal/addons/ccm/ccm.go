// Package ccm provides the Hetzner Cloud Controller Manager addon.
package ccm

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"time"

	"github.com/sak-d/hcloud-k8s/internal/addons"
	"github.com/sak-d/hcloud-k8s/internal/config"
)

const (
	addonName = "hcloud-ccm"
	namespace = "kube-system"
	secretName = "hcloud"

	// Default Helm configuration
	defaultHelmRepository = "https://charts.hetzner.cloud"
	defaultHelmChart      = "hcloud-cloud-controller-manager"
	defaultHelmVersion    = "1.21.1"
)

// CCM implements the Hetzner Cloud Controller Manager addon.
type CCM struct {
	config      *config.Config
	addonConfig *config.CCMConfig
	hcloudToken string
	networkID   string
}

// New creates a new CCM addon instance.
func New(cfg *config.Config, addonCfg *config.CCMConfig, hcloudToken, networkID string) *CCM {
	return &CCM{
		config:      cfg,
		addonConfig: addonCfg,
		hcloudToken: hcloudToken,
		networkID:   networkID,
	}
}

// Name returns the addon name.
func (c *CCM) Name() string {
	return addonName
}

// Enabled returns whether the addon is enabled.
func (c *CCM) Enabled() bool {
	return c.addonConfig.Enabled
}

// Dependencies returns addon dependencies.
func (c *CCM) Dependencies() []string {
	return []string{} // CCM has no dependencies, installs first
}

// GenerateManifests generates Kubernetes manifests for CCM.
func (c *CCM) GenerateManifests(ctx context.Context) ([]string, error) {
	manifests := []string{}

	// 1. Generate hcloud secret
	secretManifest, err := c.generateSecret()
	if err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}
	manifests = append(manifests, secretManifest)

	// 2. Generate Helm chart manifests
	helmManifest, err := c.generateHelmManifests(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate helm manifests: %w", err)
	}
	manifests = append(manifests, helmManifest)

	return manifests, nil
}

// generateSecret generates the hcloud secret manifest.
func (c *CCM) generateSecret() (string, error) {
	tokenBase64 := base64.StdEncoding.EncodeToString([]byte(c.hcloudToken))
	networkBase64 := base64.StdEncoding.EncodeToString([]byte(c.networkID))

	secret := fmt.Sprintf(`apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: %s
  namespace: %s
data:
  token: %s
  network: %s
`, secretName, namespace, tokenBase64, networkBase64)

	return secret, nil
}

// generateHelmManifests generates the Helm chart manifests for CCM.
func (c *CCM) generateHelmManifests(ctx context.Context) (string, error) {
	// Prepare Helm values
	values := c.buildHelmValues()

	// Get Helm configuration
	helmRepo := c.addonConfig.HelmRepository
	if helmRepo == "" {
		helmRepo = defaultHelmRepository
	}

	helmChart := c.addonConfig.HelmChart
	if helmChart == "" {
		helmChart = defaultHelmChart
	}

	helmVersion := c.addonConfig.HelmVersion
	if helmVersion == "" {
		helmVersion = defaultHelmVersion
	}

	// Render Helm chart
	renderer := addons.NewHelmRenderer()
	manifest, err := renderer.RenderChart(helmRepo, helmChart, helmVersion, namespace, values)
	if err != nil {
		return "", fmt.Errorf("failed to render helm chart: %w", err)
	}

	return manifest, nil
}

// buildHelmValues constructs the Helm values for CCM.
func (c *CCM) buildHelmValues() map[string]interface{} {
	values := map[string]interface{}{
		"kind": "DaemonSet",
		"nodeSelector": map[string]string{
			"node-role.kubernetes.io/control-plane": "",
		},
		"networking": map[string]interface{}{
			"enabled":     c.addonConfig.NetworkRoutesEnabled,
			"clusterCIDR": c.config.Network.NodeIPv4CIDR,
		},
		"env": c.buildEnvVars(),
	}

	// Merge custom Helm values if provided
	if len(c.addonConfig.HelmValues) > 0 {
		for k, v := range c.addonConfig.HelmValues {
			values[k] = v
		}
	}

	return values
}

// buildEnvVars constructs environment variables for CCM.
func (c *CCM) buildEnvVars() map[string]interface{} {
	env := map[string]interface{}{
		"HCLOUD_LOAD_BALANCERS_ENABLED": map[string]interface{}{
			"value": fmt.Sprintf("%t", c.addonConfig.LoadBalancersEnabled),
		},
		"HCLOUD_NETWORK_ROUTES_ENABLED": map[string]interface{}{
			"value": fmt.Sprintf("%t", c.addonConfig.NetworkRoutesEnabled),
		},
	}

	// Add load balancer specific configuration if enabled
	if c.addonConfig.LoadBalancersEnabled {
		lbAlgorithm := c.addonConfig.LoadBalancerAlgorithmType
		if lbAlgorithm == "" {
			lbAlgorithm = "round_robin"
		}

		lbType := c.addonConfig.LoadBalancerType
		if lbType == "" {
			lbType = "lb11"
		}

		lbLocation := c.addonConfig.LoadBalancerLocation
		if lbLocation == "" {
			lbLocation = c.config.Location
		}

		healthCheckInterval := c.addonConfig.LoadBalancerHealthCheckInt
		if healthCheckInterval == 0 {
			healthCheckInterval = 15
		}

		healthCheckTimeout := c.addonConfig.LoadBalancerHealthCheckTimeout
		if healthCheckTimeout == 0 {
			healthCheckTimeout = 10
		}

		healthCheckRetries := c.addonConfig.LoadBalancerHealthCheckRetry
		if healthCheckRetries == 0 {
			healthCheckRetries = 3
		}

		env["HCLOUD_LOAD_BALANCERS_ALGORITHM_TYPE"] = map[string]interface{}{
			"value": lbAlgorithm,
		}
		env["HCLOUD_LOAD_BALANCERS_TYPE"] = map[string]interface{}{
			"value": lbType,
		}
		env["HCLOUD_LOAD_BALANCERS_LOCATION"] = map[string]interface{}{
			"value": lbLocation,
		}
		env["HCLOUD_LOAD_BALANCERS_DISABLE_PRIVATE_INGRESS"] = map[string]interface{}{
			"value": fmt.Sprintf("%t", c.addonConfig.LoadBalancerDisablePrivate),
		}
		env["HCLOUD_LOAD_BALANCERS_DISABLE_PUBLIC_NETWORK"] = map[string]interface{}{
			"value": fmt.Sprintf("%t", c.addonConfig.LoadBalancerDisablePublic),
		}
		env["HCLOUD_LOAD_BALANCERS_USE_PRIVATE_IP"] = map[string]interface{}{
			"value": fmt.Sprintf("%t", c.addonConfig.LoadBalancerUsePrivateIP),
		}
		env["HCLOUD_LOAD_BALANCERS_HEALTH_CHECK_INTERVAL"] = map[string]interface{}{
			"value": fmt.Sprintf("%ds", healthCheckInterval),
		}
		env["HCLOUD_LOAD_BALANCERS_HEALTH_CHECK_TIMEOUT"] = map[string]interface{}{
			"value": fmt.Sprintf("%ds", healthCheckTimeout),
		}
		env["HCLOUD_LOAD_BALANCERS_HEALTH_CHECK_RETRIES"] = map[string]interface{}{
			"value": fmt.Sprintf("%d", healthCheckRetries),
		}

		// Add load balancer subnet if available
		if c.config.Network.IPv4CIDR != "" {
			// Calculate load balancer subnet
			// In the real implementation, you'd use the subnet calculation logic
			env["HCLOUD_LOAD_BALANCERS_PRIVATE_SUBNET_IP_RANGE"] = map[string]interface{}{
				"value": c.config.Network.IPv4CIDR, // Simplified for now
			}
		}
	}

	return env
}

// Verify checks if CCM is installed and running correctly.
func (c *CCM) Verify(ctx context.Context, k8sClient addons.K8sClient) error {
	log.Printf("Verifying %s installation...", addonName)

	// Check if secret exists
	secretExists, err := k8sClient.SecretExists(ctx, namespace, secretName)
	if err != nil {
		return fmt.Errorf("failed to check secret existence: %w", err)
	}
	if !secretExists {
		return fmt.Errorf("secret %s not found in namespace %s", secretName, namespace)
	}

	// Wait for DaemonSet to be ready
	log.Printf("Waiting for %s DaemonSet to be ready...", addonName)
	if err := k8sClient.WaitForDaemonSet(ctx, namespace, addonName, 5*time.Minute); err != nil {
		return fmt.Errorf("daemonset not ready: %w", err)
	}

	// Verify pods are running
	pods, err := k8sClient.GetPods(ctx, namespace, fmt.Sprintf("app.kubernetes.io/name=%s", addonName))
	if err != nil {
		return fmt.Errorf("failed to get pods: %w", err)
	}

	if len(pods) == 0 {
		return fmt.Errorf("no pods found for %s", addonName)
	}

	log.Printf("%s verified successfully (%d pods running)", addonName, len(pods))
	return nil
}
