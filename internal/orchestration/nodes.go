package orchestration

import (
	"context"
	"fmt"
)

// provisionControlPlane creates and configures control plane nodes.
func (r *Reconciler) provisionControlPlane(ctx context.Context) (map[string]string, []string, error) {
	cpIPs, sans, err := r.computeProvisioner.ProvisionControlPlane(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to provision control plane: %w", err)
	}

	return cpIPs, sans, nil
}

// provisionWorkers creates worker nodes.
func (r *Reconciler) provisionWorkers(ctx context.Context) (map[string]string, error) {
	workerIPs, err := r.computeProvisioner.ProvisionWorkers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to provision workers: %w", err)
	}

	return workerIPs, nil
}

// bootstrapCluster generates configs and bootstraps the cluster if needed.
func (r *Reconciler) bootstrapCluster(ctx context.Context, cpIPs map[string]string, sans []string) ([]byte, []byte, error) {
	var kubeconfig []byte
	var clientCfg []byte

	if len(cpIPs) > 0 {
		var err error
		clientCfg, err = r.talosGenerator.GetClientConfig()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get client config: %w", err)
		}

		// Generate control plane machine configs
		machineConfigs, err := r.generateControlPlaneConfigs(sans, cpIPs)
		if err != nil {
			return nil, nil, err
		}

		kubeconfig, err = r.clusterProvisioner.Bootstrap(ctx, r.config.ClusterName, cpIPs, machineConfigs, clientCfg)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to bootstrap cluster: %w", err)
		}
	}

	return kubeconfig, clientCfg, nil
}

// applyWorkerConfigs generates and applies worker configurations if needed.
func (r *Reconciler) applyWorkerConfigs(ctx context.Context, workerIPs map[string]string, kubeconfig, clientCfg []byte) error {
	if len(workerIPs) == 0 || len(kubeconfig) == 0 {
		return nil
	}

	workerConfigs, err := r.generateWorkerConfigs(workerIPs)
	if err != nil {
		return err
	}

	if err := r.clusterProvisioner.ApplyWorkerConfigs(ctx, workerIPs, workerConfigs, clientCfg); err != nil {
		return fmt.Errorf("failed to apply worker configs: %w", err)
	}

	return nil
}
