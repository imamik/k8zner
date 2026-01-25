package wizard

import (
	"context"
	"net"
	"regexp"
	"strings"

	"github.com/charmbracelet/huh"
)

// clusterNameRegex validates cluster name format: 1-32 lowercase alphanumeric with hyphens.
var clusterNameRegex = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,30}[a-z0-9])?$`)

// runClusterIdentityGroup prompts for cluster name and location.
func runClusterIdentityGroup(ctx context.Context, result *WizardResult) error {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Cluster Name").
				Description("1-32 lowercase alphanumeric characters or hyphens").
				Placeholder("my-cluster").
				Value(&result.ClusterName).
				Validate(validateClusterName),
			huh.NewSelect[string]().
				Title("Location").
				Description("Hetzner Cloud datacenter").
				Options(LocationsToOptions()...).
				Value(&result.Location),
		).Title("Cluster Identity"),
	).RunWithContext(ctx)
}

// runSSHAccessGroup prompts for SSH key names.
func runSSHAccessGroup(ctx context.Context, result *WizardResult) error {
	var sshKeysInput string

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("SSH Key Names").
				Description("Comma-separated list of SSH key names from Hetzner Cloud").
				Placeholder("my-key, another-key").
				Value(&sshKeysInput).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return errSSHKeysRequired
					}
					return nil
				}),
		).Title("SSH Access"),
	).RunWithContext(ctx)

	if err != nil {
		return err
	}

	result.SSHKeys = parseSSHKeys(sshKeysInput)
	return nil
}

// runControlPlaneGroup prompts for control plane configuration.
func runControlPlaneGroup(ctx context.Context, result *WizardResult) error {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Server Type").
				Description("Choose the server type for control plane nodes").
				Options(ServerTypesToOptions(ControlPlaneServerTypes)...).
				Value(&result.ControlPlaneType),
			huh.NewSelect[int]().
				Title("Node Count").
				Description("Odd numbers required for etcd quorum (HA)").
				Options(ControlPlaneCountOptions...).
				Value(&result.ControlPlaneCount),
		).Title("Control Plane"),
	).RunWithContext(ctx)
}

// runWorkersGroup prompts for worker node configuration.
func runWorkersGroup(ctx context.Context, result *WizardResult) error {
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Add Worker Nodes?").
				Description("Worker nodes run your application workloads").
				Value(&result.AddWorkers),
		).Title("Workers"),
	).RunWithContext(ctx)

	if err != nil {
		return err
	}

	if result.AddWorkers {
		return huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Server Type").
					Description("Choose the server type for worker nodes").
					Options(ServerTypesToOptions(WorkerServerTypes)...).
					Value(&result.WorkerType),
				huh.NewSelect[int]().
					Title("Node Count").
					Description("Number of worker nodes").
					Options(WorkerCountOptions...).
					Value(&result.WorkerCount),
			).Title("Worker Configuration"),
		).RunWithContext(ctx)
	}

	return nil
}

// runAddonsGroup prompts for addon selection.
func runAddonsGroup(ctx context.Context, result *WizardResult) error {
	options := make([]huh.Option[string], len(BasicAddons))
	defaultSelected := []string{}

	for i, addon := range BasicAddons {
		options[i] = huh.NewOption(addon.Label+" - "+addon.Description, addon.Key)
		if addon.Default {
			defaultSelected = append(defaultSelected, addon.Key)
		}
	}

	result.EnabledAddons = defaultSelected

	return huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Cluster Addons").
				Description("Select addons to install").
				Options(options...).
				Value(&result.EnabledAddons),
		).Title("Addons"),
	).RunWithContext(ctx)
}

// runVersionsGroup prompts for Talos and Kubernetes versions.
func runVersionsGroup(ctx context.Context, result *WizardResult) error {
	result.TalosVersion = TalosVersions[0].Value
	result.KubernetesVersion = KubernetesVersions[0].Value

	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Talos Version").
				Description("Talos Linux operating system version").
				Options(VersionsToOptions(TalosVersions)...).
				Value(&result.TalosVersion),
			huh.NewSelect[string]().
				Title("Kubernetes Version").
				Description("Kubernetes cluster version").
				Options(VersionsToOptions(KubernetesVersions)...).
				Value(&result.KubernetesVersion),
		).Title("Versions"),
	).RunWithContext(ctx)
}

// runNetworkGroup prompts for network configuration (advanced mode).
func runNetworkGroup(ctx context.Context, opts *AdvancedOptions) error {
	opts.NetworkCIDR = "10.0.0.0/16"
	opts.PodCIDR = "10.244.0.0/16"
	opts.ServiceCIDR = "10.96.0.0/12"

	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Network CIDR").
				Description("Private network range for Hetzner Cloud network").
				Value(&opts.NetworkCIDR).
				Validate(validateCIDR),
			huh.NewInput().
				Title("Pod CIDR").
				Description("IP range for Kubernetes pods").
				Value(&opts.PodCIDR).
				Validate(validateCIDR),
			huh.NewInput().
				Title("Service CIDR").
				Description("IP range for Kubernetes services").
				Value(&opts.ServiceCIDR).
				Validate(validateCIDR),
		).Title("Network Configuration"),
	).RunWithContext(ctx)
}

// runSecurityGroup prompts for security options (advanced mode).
func runSecurityGroup(ctx context.Context, opts *AdvancedOptions) error {
	opts.DiskEncryption = true
	opts.ClusterAccess = "public"

	return huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Enable Disk Encryption").
				Description("Encrypt node disks with LUKS2").
				Value(&opts.DiskEncryption),
			huh.NewSelect[string]().
				Title("Cluster Access Mode").
				Description("How to access the cluster API").
				Options(ClusterAccessModes...).
				Value(&opts.ClusterAccess),
		).Title("Security Options"),
	).RunWithContext(ctx)
}

// runCiliumGroup prompts for Cilium configuration (advanced mode).
func runCiliumGroup(ctx context.Context, opts *AdvancedOptions) error {
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Enable Encryption").
				Description("Encrypt pod-to-pod traffic").
				Value(&opts.CiliumEncryption),
		).Title("Cilium Options"),
	).RunWithContext(ctx)

	if err != nil {
		return err
	}

	if opts.CiliumEncryption {
		opts.CiliumEncryptionType = "wireguard"

		err = huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Encryption Type").
					Description("Choose encryption method for pod traffic").
					Options(CiliumEncryptionTypes...).
					Value(&opts.CiliumEncryptionType),
			).Title("Cilium Encryption"),
		).RunWithContext(ctx)

		if err != nil {
			return err
		}
	}

	return huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Enable Hubble Observability").
				Description("Network observability and monitoring").
				Value(&opts.HubbleEnabled),
			huh.NewConfirm().
				Title("Enable Gateway API").
				Description("Kubernetes Gateway API support").
				Value(&opts.GatewayAPIEnabled),
		).Title("Cilium Features"),
	).RunWithContext(ctx)
}

// validateClusterName validates the cluster name format.
func validateClusterName(s string) error {
	if s == "" {
		return errClusterNameRequired
	}
	if !clusterNameRegex.MatchString(s) {
		return errClusterNameInvalid
	}
	return nil
}

// validateCIDR validates a CIDR notation string using net.ParseCIDR.
func validateCIDR(s string) error {
	if s == "" {
		return errCIDRRequired
	}
	if _, _, err := net.ParseCIDR(s); err != nil {
		return errCIDRInvalid
	}
	return nil
}

// parseSSHKeys parses a comma-separated list of SSH key names.
func parseSSHKeys(input string) []string {
	parts := strings.Split(input, ",")
	keys := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			keys = append(keys, trimmed)
		}
	}
	return keys
}
