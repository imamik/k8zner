package addons

import (
	"context"
	"fmt"

	"k8zner/internal/addons/helm"
	"k8zner/internal/addons/k8sclient"
	"k8zner/internal/config"
)

// applyMetricsServer installs the Kubernetes Metrics Server.
func applyMetricsServer(ctx context.Context, client k8sclient.Client, cfg *config.Config) error {
	values := buildMetricsServerValues(cfg)

	manifestBytes, err := helm.RenderChart("metrics-server", "kube-system", values)
	if err != nil {
		return fmt.Errorf("failed to render metrics-server chart: %w", err)
	}

	if err := applyManifests(ctx, client, "metrics-server", manifestBytes); err != nil {
		return fmt.Errorf("failed to apply metrics-server manifests: %w", err)
	}

	return nil
}

// buildMetricsServerValues creates helm values matching terraform configuration.
// See: terraform/metrics_server.tf
func buildMetricsServerValues(cfg *config.Config) helm.Values {
	controlPlaneCount := getControlPlaneCount(cfg)
	workerCount := getWorkerCount(cfg)

	// Calculate schedule_on_control_plane: use config value if set, otherwise auto-detect
	// Auto-detection: schedule on control plane when no workers exist
	scheduleOnControlPlane := workerCount == 0
	if cfg.Addons.MetricsServer.ScheduleOnControlPlane != nil {
		scheduleOnControlPlane = *cfg.Addons.MetricsServer.ScheduleOnControlPlane
	}

	// Calculate replicas: use config value if set, otherwise auto-calculate
	// Auto-calculation: 2 replicas if >1 node available, otherwise 1
	nodeCount := workerCount
	if scheduleOnControlPlane {
		nodeCount = controlPlaneCount
	}

	replicas := 1
	if nodeCount > 1 {
		replicas = 2
	}
	if cfg.Addons.MetricsServer.Replicas != nil {
		replicas = *cfg.Addons.MetricsServer.Replicas
	}

	values := helm.Values{
		"replicas":                  replicas,
		"podDisruptionBudget":       buildMetricsServerPDB(),
		"topologySpreadConstraints": buildMetricsServerTopologySpread(),
		// Talos-specific configuration
		// Talos uses self-signed kubelet certificates, so we need to skip TLS verification
		"args": []string{
			"--kubelet-insecure-tls",
			"--kubelet-preferred-address-types=InternalIP",
		},
	}

	if scheduleOnControlPlane {
		values["nodeSelector"] = helm.Values{"node-role.kubernetes.io/control-plane": ""}
		values["tolerations"] = buildControlPlaneTolerations()
	}

	// Merge custom Helm values from config
	return helm.MergeCustomValues(values, cfg.Addons.MetricsServer.Helm.Values)
}

// buildMetricsServerPDB creates the pod disruption budget configuration.
func buildMetricsServerPDB() helm.Values {
	return helm.Values{
		"enabled":        true,
		"minAvailable":   nil,
		"maxUnavailable": 1,
	}
}

// buildMetricsServerTopologySpread creates topology spread constraints.
func buildMetricsServerTopologySpread() []helm.Values {
	labelSelector := helm.Values{
		"matchLabels": helm.Values{
			"app.kubernetes.io/instance": "metrics-server",
			"app.kubernetes.io/name":     "metrics-server",
		},
	}

	return []helm.Values{
		{
			"topologyKey":       "kubernetes.io/hostname",
			"maxSkew":           1,
			"whenUnsatisfiable": "DoNotSchedule",
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

// buildControlPlaneTolerations creates tolerations for control plane scheduling.
// Includes tolerations for both control plane nodes and CCM uninitialized nodes.
func buildControlPlaneTolerations() []helm.Values {
	return []helm.Values{
		{
			"key":      "node-role.kubernetes.io/control-plane",
			"effect":   "NoSchedule",
			"operator": "Exists",
		},
		{
			"key":      "node.cloudprovider.kubernetes.io/uninitialized",
			"operator": "Exists",
		},
	}
}

// getWorkerCount returns the total number of worker nodes.
func getWorkerCount(cfg *config.Config) int {
	count := 0
	for _, pool := range cfg.Workers {
		count += pool.Count
	}
	return count
}
