package orchestration

import (
	"fmt"
)

// generateControlPlaneConfigs generates machine configs for control plane nodes.
func (r *Reconciler) generateControlPlaneConfigs(sans []string, nodeIPs map[string]string) (map[string][]byte, error) {
	machineConfigs := make(map[string][]byte)
	for nodeName := range nodeIPs {
		nodeConfig, err := r.talosGenerator.GenerateControlPlaneConfig(sans, nodeName)
		if err != nil {
			return nil, fmt.Errorf("failed to generate control plane config for %s: %w", nodeName, err)
		}
		machineConfigs[nodeName] = nodeConfig
	}

	return machineConfigs, nil
}

// generateWorkerConfigs generates machine configs for worker nodes.
func (r *Reconciler) generateWorkerConfigs(nodeIPs map[string]string) (map[string][]byte, error) {
	workerConfigs := make(map[string][]byte)
	for nodeName := range nodeIPs {
		nodeConfig, err := r.talosGenerator.GenerateWorkerConfig(nodeName)
		if err != nil {
			return nil, fmt.Errorf("failed to generate worker config for %s: %w", nodeName, err)
		}
		workerConfigs[nodeName] = nodeConfig
	}

	return workerConfigs, nil
}
