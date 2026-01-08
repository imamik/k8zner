package orchestration

import (
	"context"
	"fmt"
	"log"

	"hcloud-k8s/internal/util/retry"
)

// ensureServer ensures a server exists and returns its IP.
func (r *Reconciler) ensureServer(
	ctx context.Context,
	serverName string,
	serverType string,
	location string,
	image string,
	role string,
	poolName string,
	extraLabels map[string]string,
	userData string,
	pgID *int64,
	privateIP string,
) (string, error) {
	// Check if exists
	serverID, err := r.serverProvisioner.GetServerID(ctx, serverName)
	if err != nil {
		return "", err
	}

	if serverID != "" {
		// Server exists, get IP
		ip, err := r.serverProvisioner.GetServerIP(ctx, serverName)
		if err != nil {
			return "", err
		}
		return ip, nil
	}

	// Create
	log.Printf("Creating %s Server %s...", role, serverName)

	// Labels
	labels := NewLabelBuilder(r.config.ClusterName).
		WithRole(role).
		WithPool(poolName).
		Merge(extraLabels).
		Build()

	// Image defaulting - if empty or "talos", ensure the versioned image exists
	if image == "" || image == "talos" {
		var err error
		image, err = r.ensureImage(ctx, serverType, location)
		if err != nil {
			return "", fmt.Errorf("failed to ensure Talos image: %w", err)
		}
		log.Printf("Using Talos image: %s", image)
	}

	// Get Network ID
	if r.network == nil {
		return "", fmt.Errorf("network not initialized")
	}
	networkID := r.network.ID

	_, err = r.serverProvisioner.CreateServer(
		ctx,
		serverName,
		image,
		serverType,
		location,
		r.config.SSHKeys,
		labels,
		userData,
		pgID,
		networkID,
		privateIP,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create server %s: %w", serverName, err)
	}

	// Get IP after creation with retry logic and configurable timeout
	ipCtx, cancel := context.WithTimeout(ctx, r.timeouts.ServerIP)
	defer cancel()

	var ip string
	err = retry.WithExponentialBackoff(ipCtx, func() error {
		var getErr error
		ip, getErr = r.serverProvisioner.GetServerIP(ctx, serverName)
		if getErr != nil {
			return getErr
		}
		if ip == "" {
			return fmt.Errorf("server IP not yet assigned")
		}
		return nil
	}, retry.WithMaxRetries(r.timeouts.RetryMaxAttempts), retry.WithInitialDelay(r.timeouts.RetryInitialDelay))

	if err != nil {
		return "", fmt.Errorf("failed to get server IP for %s: %w", serverName, err)
	}

	return ip, nil
}
