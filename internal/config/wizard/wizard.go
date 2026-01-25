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

	// SSH Access (optional - if empty, SSH key will be generated)
	SSHKeys []string

	// Architecture & Server Category
	Architecture   string // "x86" or "arm"
	ServerCategory string // "shared", "dedicated", or "cost-optimized"

	// Control Plane
	ControlPlaneType  string
	ControlPlaneCount int

	// Workers
	AddWorkers  bool
	WorkerType  string
	WorkerCount int

	// CNI Selection
	CNIChoice string // "cilium", "talos", or "none"

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
// The context is used for cancellation support (e.g., Ctrl+C).
func RunWizard(ctx context.Context, advanced bool) (*WizardResult, error) {
	result := &WizardResult{}

	if err := runClusterIdentityGroup(ctx, result); err != nil {
		return nil, fmt.Errorf("cluster identity: %w", err)
	}

	if err := runSSHAccessGroup(ctx, result); err != nil {
		return nil, fmt.Errorf("ssh access: %w", err)
	}

	// Architecture and server category selection (narrows down server types)
	if err := runArchitectureGroup(ctx, result); err != nil {
		return nil, fmt.Errorf("architecture: %w", err)
	}

	if err := runControlPlaneGroup(ctx, result); err != nil {
		return nil, fmt.Errorf("control plane: %w", err)
	}

	if err := runWorkersGroup(ctx, result); err != nil {
		return nil, fmt.Errorf("workers: %w", err)
	}

	// CNI selection (separate from other addons)
	if err := runCNIGroup(ctx, result); err != nil {
		return nil, fmt.Errorf("cni: %w", err)
	}

	if err := runAddonsGroup(ctx, result); err != nil {
		return nil, fmt.Errorf("addons: %w", err)
	}

	if err := runVersionsGroup(ctx, result); err != nil {
		return nil, fmt.Errorf("versions: %w", err)
	}

	if advanced {
		advOpts := &AdvancedOptions{}

		if err := runNetworkGroup(ctx, advOpts); err != nil {
			return nil, fmt.Errorf("network: %w", err)
		}

		if err := runSecurityGroup(ctx, advOpts); err != nil {
			return nil, fmt.Errorf("security: %w", err)
		}

		// Only show Cilium options if Cilium was selected as CNI
		if result.CNIChoice == CNICilium {
			if err := runCiliumAdvancedGroup(ctx, advOpts); err != nil {
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
