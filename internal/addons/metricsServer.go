package addons

import (
	"context"
	"fmt"

	"hcloud-k8s/internal/addons/helm"
	"hcloud-k8s/internal/config"
)

// applyMetricsServer installs the Kubernetes Metrics Server.
func applyMetricsServer(ctx context.Context, kubeconfigPath string, cfg *config.Config) error {
	values := buildMetricsServerValues(cfg)

	manifestBytes, err := helm.RenderChart("metrics-server", "kube-system", values)
	if err != nil {
		return fmt.Errorf("failed to render metrics-server chart: %w", err)
	}

	if err := applyWithKubectl(ctx, kubeconfigPath, "metrics-server", manifestBytes); err != nil {
		return fmt.Errorf("failed to apply metrics-server manifests: %w", err)
	}

	return nil
}

// buildMetricsServerValues creates helm values matching terraform configuration.
// See: terraform/metrics_server.tf
func buildMetricsServerValues(cfg *config.Config) helm.Values {
	controlPlaneCount := getControlPlaneCount(cfg)
	workerCount := getWorkerCount(cfg)
	scheduleOnControlPlane := workerCount == 0

	nodeCount := workerCount
	if scheduleOnControlPlane {
		nodeCount = controlPlaneCount
	}

	replicas := 1
	if nodeCount > 1 {
		replicas = 2
	}

	values := helm.Values{
		"replicas":                  replicas,
		"podDisruptionBudget":       buildMetricsServerPDB(),
		"topologySpreadConstraints": buildMetricsServerTopologySpread(),
	}

	if scheduleOnControlPlane {
		values["nodeSelector"] = helm.Values{"node-role.kubernetes.io/control-plane": ""}
		values["tolerations"] = buildControlPlaneTolerations()
	}

	return values
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
func buildControlPlaneTolerations() []helm.Values {
	return []helm.Values{
		{
			"key":      "node-role.kubernetes.io/control-plane",
			"effect":   "NoSchedule",
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
