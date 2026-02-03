// Package controller contains the Kubernetes controllers for the k8zner operator.
package controller

import (
	"context"
	"crypto/tls"
	"fmt"
	"strconv"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/client/config"
)

// RealTalosClient implements TalosClient using the actual Talos API.
type RealTalosClient struct {
	// talosConfig is the client configuration for authenticated connections
	talosConfig *config.Config
}

// NewRealTalosClient creates a new TalosClient from talosconfig bytes.
func NewRealTalosClient(talosconfigBytes []byte) (*RealTalosClient, error) {
	cfg, err := config.FromString(string(talosconfigBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse talosconfig: %w", err)
	}
	return &RealTalosClient{talosConfig: cfg}, nil
}

// ApplyConfig applies a machine configuration to a node.
// For nodes in maintenance mode (unconfigured), this uses an insecure connection.
func (c *RealTalosClient) ApplyConfig(ctx context.Context, nodeIP string, configData []byte) error {
	// Wait for Talos API to be available
	if err := waitForTalosAPI(ctx, nodeIP, 2*time.Minute); err != nil {
		return fmt.Errorf("failed to wait for Talos API: %w", err)
	}

	// Create insecure client for maintenance mode
	// Fresh Talos nodes from snapshots boot into maintenance mode and don't have
	// credentials yet, so we must use an insecure connection to apply the initial config.
	talosClient, err := client.New(ctx,
		client.WithEndpoints(nodeIP),
		//nolint:gosec // InsecureSkipVerify is required for Talos maintenance mode
		client.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}),
	)
	if err != nil {
		return fmt.Errorf("failed to create talos client: %w", err)
	}
	defer func() { _ = talosClient.Close() }()

	// Apply the configuration with REBOOT mode for pre-installed Talos
	applyReq := &machine.ApplyConfigurationRequest{
		Data: configData,
		Mode: machine.ApplyConfigurationRequest_REBOOT,
	}

	_, err = talosClient.ApplyConfiguration(ctx, applyReq)
	if err != nil {
		return fmt.Errorf("failed to apply configuration: %w", err)
	}

	return nil
}

// IsNodeInMaintenanceMode checks if a node is unconfigured (in maintenance mode).
func (c *RealTalosClient) IsNodeInMaintenanceMode(ctx context.Context, nodeIP string) (bool, error) {
	// Try to connect with insecure client
	talosClient, err := client.New(ctx,
		client.WithEndpoints(nodeIP),
		//nolint:gosec // InsecureSkipVerify is required for checking maintenance mode
		client.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}),
	)
	if err != nil {
		return false, fmt.Errorf("failed to create talos client: %w", err)
	}
	defer func() { _ = talosClient.Close() }()

	// Try to get version - if this works without auth, node is in maintenance mode
	_, err = talosClient.Version(ctx)
	if err != nil {
		// Connection failed - likely not in maintenance mode or not reachable
		return false, nil
	}

	return true, nil
}

// GetEtcdMembers returns the list of etcd members.
func (c *RealTalosClient) GetEtcdMembers(ctx context.Context, nodeIP string) ([]EtcdMember, error) {
	talosClient, err := client.New(ctx,
		client.WithConfig(c.talosConfig),
		client.WithEndpoints(nodeIP),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create talos client: %w", err)
	}
	defer func() { _ = talosClient.Close() }()

	resp, err := talosClient.EtcdMemberList(ctx, &machine.EtcdMemberListRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list etcd members: %w", err)
	}

	var members []EtcdMember
	for _, msg := range resp.Messages {
		for _, member := range msg.Members {
			members = append(members, EtcdMember{
				ID:       fmt.Sprintf("%d", member.Id),
				Name:     member.Hostname,
				Endpoint: member.Hostname,
				IsLeader: member.IsLearner, // Note: this maps learner, not leader
			})
		}
	}

	return members, nil
}

// RemoveEtcdMember removes a member from the etcd cluster.
func (c *RealTalosClient) RemoveEtcdMember(ctx context.Context, nodeIP string, memberID string) error {
	talosClient, err := client.New(ctx,
		client.WithConfig(c.talosConfig),
		client.WithEndpoints(nodeIP),
	)
	if err != nil {
		return fmt.Errorf("failed to create talos client: %w", err)
	}
	defer func() { _ = talosClient.Close() }()

	// Convert string memberID to uint64
	memberIDUint, err := strconv.ParseUint(memberID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid member ID %q: %w", memberID, err)
	}

	err = talosClient.EtcdRemoveMemberByID(ctx, &machine.EtcdRemoveMemberByIDRequest{
		MemberId: memberIDUint,
	})
	if err != nil {
		return fmt.Errorf("failed to remove etcd member: %w", err)
	}

	return nil
}

// WaitForNodeReady waits for a node to become ready after configuration.
func (c *RealTalosClient) WaitForNodeReady(ctx context.Context, nodeIP string, timeoutSec int) error {
	// Initial wait for reboot to begin
	time.Sleep(5 * time.Second)

	// Wait for Talos API to come back up
	if err := waitForTalosAPI(ctx, nodeIP, time.Duration(timeoutSec)*time.Second); err != nil {
		return fmt.Errorf("node did not come back online: %w", err)
	}

	// Create authenticated client
	talosClient, err := client.New(ctx,
		client.WithConfig(c.talosConfig),
		client.WithEndpoints(nodeIP),
	)
	if err != nil {
		return fmt.Errorf("failed to create talos client: %w", err)
	}
	defer func() { _ = talosClient.Close() }()

	// Poll for node ready status
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	timeout := time.After(time.Duration(timeoutSec) * time.Second)

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for node to become ready")
		case <-ticker.C:
			// Check if node is ready by getting service status
			resp, err := talosClient.ServiceList(ctx)
			if err != nil {
				continue // Not ready yet
			}

			// Check for kubelet running
			for _, msg := range resp.Messages {
				for _, svc := range msg.Services {
					if svc.Id == "kubelet" && svc.State == "Running" {
						return nil
					}
				}
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// waitForTalosAPI waits for the Talos API port to become available.
func waitForTalosAPI(ctx context.Context, nodeIP string, timeout time.Duration) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	timeoutCh := time.After(timeout)

	for {
		select {
		case <-timeoutCh:
			return fmt.Errorf("timeout waiting for Talos API on %s", nodeIP)
		case <-ticker.C:
			// Try to connect
			talosClient, err := client.New(ctx,
				client.WithEndpoints(nodeIP),
				//nolint:gosec // InsecureSkipVerify for connectivity check
				client.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}),
			)
			if err != nil {
				continue
			}

			// Try to get version
			_, err = talosClient.Version(ctx)
			_ = talosClient.Close()
			if err == nil {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
