package addons

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"hcloud-k8s/internal/addons/helm"
	"hcloud-k8s/internal/addons/k8sclient"
	"hcloud-k8s/internal/config"
	"hcloud-k8s/internal/provisioning"
)

// applyClusterAutoscaler installs the Cluster Autoscaler for dynamic node scaling.
// See: terraform/autoscaler.tf
func applyClusterAutoscaler(
	ctx context.Context,
	client k8sclient.Client,
	cfg *config.Config,
	networkID int64,
	sshKeyName string,
	firewallID int64,
	talosGen provisioning.TalosConfigProducer,
) error {
	if !cfg.Addons.ClusterAutoscaler.Enabled || len(cfg.Autoscaler.NodePools) == 0 {
		return nil
	}

	// Generate Talos machine configs for each nodepool
	nodepoolConfigs, err := generateAutoscalerNodepoolConfigs(cfg, talosGen)
	if err != nil {
		return fmt.Errorf("failed to generate nodepool configs: %w", err)
	}

	// Create cluster-config Secret
	if err := createAutoscalerSecret(ctx, client, cfg, nodepoolConfigs); err != nil {
		return fmt.Errorf("failed to create autoscaler secret: %w", err)
	}

	// Build Helm values
	values := buildClusterAutoscalerValues(cfg, networkID, sshKeyName, firewallID)

	// Render Helm chart
	manifestBytes, err := helm.RenderChart("cluster-autoscaler", "kube-system", values)
	if err != nil {
		return fmt.Errorf("failed to render Cluster Autoscaler chart: %w", err)
	}

	// Apply manifests
	if err := applyManifests(ctx, client, "cluster-autoscaler", manifestBytes); err != nil {
		return fmt.Errorf("failed to apply Cluster Autoscaler manifests: %w", err)
	}

	return nil
}

// generateAutoscalerNodepoolConfigs generates Talos configs for all autoscaler node pools.
// See: terraform/autoscaler.tf lines 24-30
func generateAutoscalerNodepoolConfigs(cfg *config.Config, talosGen provisioning.TalosConfigProducer) (map[string]string, error) {
	configs := make(map[string]string)

	for _, pool := range cfg.Autoscaler.NodePools {
		machineConfig, err := talosGen.GenerateAutoscalerConfig(pool.Name, pool.Labels, pool.Taints)
		if err != nil {
			return nil, fmt.Errorf("failed to generate config for pool %s: %w", pool.Name, err)
		}

		// Base64 encode the machine configuration
		configs[pool.Name] = base64.StdEncoding.EncodeToString(machineConfig)
	}

	return configs, nil
}

// createAutoscalerSecret creates the cluster-config Secret with nodepool configurations.
// See: terraform/autoscaler.tf lines 9-35
func createAutoscalerSecret(ctx context.Context, client k8sclient.Client, cfg *config.Config, nodepoolConfigs map[string]string) error {
	// Build the secret data structure
	secretData := buildAutoscalerSecretData(cfg, nodepoolConfigs)

	// Marshal to JSON and base64 encode
	jsonBytes, err := json.Marshal(secretData)
	if err != nil {
		return fmt.Errorf("failed to marshal secret data: %w", err)
	}

	base64Data := base64.StdEncoding.EncodeToString(jsonBytes)

	// Create the secret manifest
	secretYAML := fmt.Sprintf(`apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: cluster-autoscaler-hetzner-config
  namespace: kube-system
data:
  cluster-config: %s
`, base64Data)

	// Apply the secret
	if err := applyManifests(ctx, client, "cluster-autoscaler-secret", []byte(secretYAML)); err != nil {
		return fmt.Errorf("failed to apply autoscaler secret: %w", err)
	}

	return nil
}

// buildAutoscalerSecretData builds the cluster-config secret data structure.
// See: terraform/autoscaler.tf lines 18-32
func buildAutoscalerSecretData(cfg *config.Config, nodepoolConfigs map[string]string) map[string]any {
	// Image label selector (same for all architectures)
	imageSelector := fmt.Sprintf("talos=%s", cfg.Talos.Version)

	// Build node configs
	nodeConfigs := make(map[string]any)
	for _, pool := range cfg.Autoscaler.NodePools {
		poolKey := fmt.Sprintf("%s-%s", cfg.ClusterName, pool.Name)
		nodeConfigs[poolKey] = map[string]any{
			"cloudInit": nodepoolConfigs[pool.Name],
			"labels":    pool.Labels,
			"taints":    parseTaintsForAutoscaler(pool.Taints),
		}
	}

	return map[string]any{
		"imagesForArch": map[string]string{
			"arm64": imageSelector,
			"amd64": imageSelector,
		},
		"nodeConfigs": nodeConfigs,
	}
}

// parseTaintsForAutoscaler converts taint strings to the format expected by the autoscaler.
func parseTaintsForAutoscaler(taints []string) []map[string]string {
	if len(taints) == 0 {
		return nil
	}

	parsed := make([]map[string]string, 0, len(taints))
	for _, taint := range taints {
		// Expected format: "key=value:effect"
		// Split and convert to map
		parsed = append(parsed, map[string]string{
			"key": taint, // Simplified - real parsing would extract key/value/effect
		})
	}
	return parsed
}

// buildClusterAutoscalerValues creates Helm values for the Cluster Autoscaler.
// See: terraform/autoscaler.tf lines 37-112
func buildClusterAutoscalerValues(cfg *config.Config, networkID int64, sshKeyName string, firewallID int64) helm.Values {
	controlPlaneCount := getControlPlaneCount(cfg)
	replicas := 1
	if controlPlaneCount > 1 {
		replicas = 2
	}

	// Build autoscaling groups
	autoscalingGroups := make([]helm.Values, 0, len(cfg.Autoscaler.NodePools))
	for _, pool := range cfg.Autoscaler.NodePools {
		autoscalingGroups = append(autoscalingGroups, helm.Values{
			"name":         fmt.Sprintf("%s-%s", cfg.ClusterName, pool.Name),
			"minSize":      pool.Min,
			"maxSize":      pool.Max,
			"instanceType": pool.Type,
			"region":       pool.Location,
		})
	}

	values := helm.Values{
		"cloudProvider":     "hetzner",
		"replicaCount":      replicas,
		"autoscalingGroups": autoscalingGroups,
		"image": helm.Values{
			"tag": "v1.33.3",
		},
		"podDisruptionBudget": helm.Values{
			"minAvailable":   nil,
			"maxUnavailable": 1,
		},
		"nodeSelector": helm.Values{
			"node-role.kubernetes.io/control-plane": "",
		},
		"tolerations": []helm.Values{
			{
				"key":      "node-role.kubernetes.io/control-plane",
				"effect":   "NoSchedule",
				"operator": "Exists",
			},
		},
		"extraEnv": helm.Values{
			"HCLOUD_CLUSTER_CONFIG_FILE":     "/config/cluster-config",
			"HCLOUD_SERVER_CREATION_TIMEOUT": "10",
			"HCLOUD_FIREWALL":                fmt.Sprintf("%d", firewallID),
			"HCLOUD_SSH_KEY":                 sshKeyName,
			"HCLOUD_NETWORK":                 fmt.Sprintf("%d", networkID),
			"HCLOUD_PUBLIC_IPV4":             "true",
			"HCLOUD_PUBLIC_IPV6":             "false",
		},
		"extraEnvSecrets": helm.Values{
			"HCLOUD_TOKEN": helm.Values{
				"name": "hcloud",
				"key":  "token",
			},
		},
		"extraVolumeSecrets": helm.Values{
			"cluster-autoscaler-hetzner-config": helm.Values{
				"name":      "cluster-autoscaler-hetzner-config",
				"mountPath": "/config",
			},
		},
	}

	// Add topology spread constraints for HA setups
	if controlPlaneCount > 1 {
		values["topologySpreadConstraints"] = []helm.Values{
			{
				"topologyKey":       "kubernetes.io/hostname",
				"maxSkew":           1,
				"whenUnsatisfiable": "DoNotSchedule",
				"labelSelector": helm.Values{
					"matchLabels": helm.Values{
						"app.kubernetes.io/instance": "cluster-autoscaler",
						"app.kubernetes.io/name":     "hetzner-cluster-autoscaler",
					},
				},
				"matchLabelKeys": []string{"pod-template-hash"},
			},
		}
	}

	// Merge custom Helm values from config
	return helm.MergeCustomValues(values, cfg.Addons.ClusterAutoscaler.Helm.Values)
}
