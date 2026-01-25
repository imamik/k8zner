package wizard

import (
	"context"
	"fmt"
)

// WizardResult holds all the answers from the interactive wizard.
type WizardResult struct {
	// Cluster Identity
	ClusterName string
	Location    string

	// SSH Access
	SSHKeys []string

	// Control Plane
	ControlPlaneType  string
	ControlPlaneCount int

	// Workers
	AddWorkers  bool
	WorkerType  string
	WorkerCount int

	// Addons
	EnabledAddons []string

	// Versions
	TalosVersion      string
	KubernetesVersion string

	// Advanced options (only set in advanced mode)
	AdvancedOptions *AdvancedOptions
}

// AdvancedOptions holds advanced configuration options.
type AdvancedOptions struct {
	// Network
	NetworkCIDR string
	PodCIDR     string
	ServiceCIDR string

	// Security
	DiskEncryption bool
	ClusterAccess  string

	// Cilium options
	CiliumEncryption     bool
	CiliumEncryptionType string
	HubbleEnabled        bool
	GatewayAPIEnabled    bool
}

// RunWizard runs the interactive configuration wizard.
// If advanced is true, additional configuration options are shown.
func RunWizard(ctx context.Context, advanced bool) (*WizardResult, error) {
	result := &WizardResult{}

	// Group 1: Cluster Identity
	if err := runClusterIdentityGroup(result); err != nil {
		return nil, fmt.Errorf("cluster identity: %w", err)
	}

	// Group 2: SSH Access
	if err := runSSHAccessGroup(result); err != nil {
		return nil, fmt.Errorf("ssh access: %w", err)
	}

	// Group 3: Control Plane
	if err := runControlPlaneGroup(result); err != nil {
		return nil, fmt.Errorf("control plane: %w", err)
	}

	// Group 4: Workers
	if err := runWorkersGroup(result); err != nil {
		return nil, fmt.Errorf("workers: %w", err)
	}

	// Group 5: Addons
	if err := runAddonsGroup(result); err != nil {
		return nil, fmt.Errorf("addons: %w", err)
	}

	// Group 6: Versions
	if err := runVersionsGroup(result); err != nil {
		return nil, fmt.Errorf("versions: %w", err)
	}

	// Advanced mode: additional configuration
	if advanced {
		advOpts := &AdvancedOptions{}

		if err := runNetworkGroup(advOpts); err != nil {
			return nil, fmt.Errorf("network: %w", err)
		}

		if err := runSecurityGroup(advOpts); err != nil {
			return nil, fmt.Errorf("security: %w", err)
		}

		// Cilium options if Cilium is enabled
		if containsAddon(result.EnabledAddons, "cilium") {
			if err := runCiliumGroup(advOpts); err != nil {
				return nil, fmt.Errorf("cilium: %w", err)
			}
		}

		result.AdvancedOptions = advOpts
	}

	return result, nil
}

// containsAddon checks if an addon is in the enabled list.
func containsAddon(addons []string, addon string) bool {
	for _, a := range addons {
		if a == addon {
			return true
		}
	}
	return false
}
