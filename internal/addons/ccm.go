package addons

import (
	"context"
	"fmt"
	"strconv"

	"github.com/imamik/k8zner/internal/addons/helm"
	"github.com/imamik/k8zner/internal/addons/k8sclient"
	"github.com/imamik/k8zner/internal/config"
)

// applyCCM installs the Hetzner Cloud Controller Manager.
// See: terraform/hcloud.tf (hcloud_ccm)
func applyCCM(ctx context.Context, client k8sclient.Client, cfg *config.Config, networkID int64) error {
	if !cfg.Addons.CCM.Enabled {
		return nil
	}

	// Build CCM values matching terraform configuration
	values := buildCCMValues(cfg, networkID)

	// Get chart spec with any config overrides
	spec := helm.GetChartSpec("hcloud-ccm", cfg.Addons.CCM.Helm)

	// Render helm chart with values
	manifestBytes, err := helm.RenderFromSpec(ctx, spec, "kube-system", values)
	if err != nil {
		return fmt.Errorf("failed to render CCM chart: %w", err)
	}

	// Apply manifests to cluster
	if err := applyManifests(ctx, client, "hcloud-ccm", manifestBytes); err != nil {
		return fmt.Errorf("failed to apply CCM manifests: %w", err)
	}

	return nil
}

// buildCCMValues creates helm values matching terraform configuration.
// See: terraform/hcloud.tf lines 31-57
func buildCCMValues(cfg *config.Config, _ int64) helm.Values {
	ccm := &cfg.Addons.CCM
	lb := &ccm.LoadBalancers

	// Base configuration
	values := helm.Values{
		"kind":         "DaemonSet",
		"nodeSelector": helm.ControlPlaneNodeSelector(),
		"tolerations":  helm.BootstrapTolerations(),
		"networking": helm.Values{
			"enabled":     true,
			"clusterCIDR": getClusterCIDR(cfg),
			"network": helm.Values{
				"valueFrom": helm.Values{
					"secretKeyRef": helm.Values{
						"name": "hcloud",
						"key":  "network",
					},
				},
			},
		},
	}

	// Build environment variables for load balancer configuration
	// See: terraform/hcloud.tf lines 39-54
	env := buildCCMEnvVars(cfg, lb)

	// CCM runs on control plane nodes with hostNetwork:true.
	// When kube-proxy is disabled (Cilium replaces it), the Kubernetes service IP
	// is not routable. Override to use localhost since kube-apiserver runs on the same node.
	env["KUBERNETES_SERVICE_HOST"] = helm.Values{"value": "localhost"}
	env["KUBERNETES_SERVICE_PORT"] = helm.Values{"value": "6443"}

	if len(env) > 0 {
		values["env"] = env
	}

	// Merge custom Helm values from config
	return helm.MergeCustomValues(values, ccm.Helm.Values)
}

// buildCCMEnvVars builds the environment variables for CCM load balancer configuration.
func buildCCMEnvVars(cfg *config.Config, lb *config.CCMLoadBalancerConfig) helm.Values {
	ccm := &cfg.Addons.CCM
	env := helm.Values{}

	// HCLOUD_TOKEN - Critical: CCM needs this to authenticate with Hetzner Cloud API
	// Without this, CCM cannot provision load balancers or manage routes
	env["HCLOUD_TOKEN"] = helm.Values{
		"valueFrom": helm.Values{
			"secretKeyRef": helm.Values{
				"name": "hcloud",
				"key":  "token",
			},
		},
	}

	// HCLOUD_LOAD_BALANCERS_ENABLED
	if lb.Enabled != nil {
		env["HCLOUD_LOAD_BALANCERS_ENABLED"] = helm.Values{
			"value": strconv.FormatBool(*lb.Enabled),
		}
	}

	// HCLOUD_LOAD_BALANCERS_LOCATION
	location := lb.Location
	if location == "" {
		location = cfg.Location // Fall back to cluster location
	}
	env["HCLOUD_LOAD_BALANCERS_LOCATION"] = helm.Values{
		"value": location,
	}

	// HCLOUD_LOAD_BALANCERS_TYPE
	if lb.Type != "" {
		env["HCLOUD_LOAD_BALANCERS_TYPE"] = helm.Values{
			"value": lb.Type,
		}
	}

	// HCLOUD_LOAD_BALANCERS_ALGORITHM_TYPE
	if lb.Algorithm != "" {
		env["HCLOUD_LOAD_BALANCERS_ALGORITHM_TYPE"] = helm.Values{
			"value": lb.Algorithm,
		}
	}

	// HCLOUD_LOAD_BALANCERS_USE_PRIVATE_IP
	if lb.UsePrivateIP != nil {
		env["HCLOUD_LOAD_BALANCERS_USE_PRIVATE_IP"] = helm.Values{
			"value": strconv.FormatBool(*lb.UsePrivateIP),
		}
	}

	// HCLOUD_LOAD_BALANCERS_DISABLE_PRIVATE_INGRESS
	if lb.DisablePrivateIngress != nil {
		env["HCLOUD_LOAD_BALANCERS_DISABLE_PRIVATE_INGRESS"] = helm.Values{
			"value": strconv.FormatBool(*lb.DisablePrivateIngress),
		}
	}

	// HCLOUD_LOAD_BALANCERS_DISABLE_PUBLIC_NETWORK
	if lb.DisablePublicNetwork != nil {
		env["HCLOUD_LOAD_BALANCERS_DISABLE_PUBLIC_NETWORK"] = helm.Values{
			"value": strconv.FormatBool(*lb.DisablePublicNetwork),
		}
	}

	// HCLOUD_LOAD_BALANCERS_DISABLE_IPV6
	if lb.DisableIPv6 != nil {
		env["HCLOUD_LOAD_BALANCERS_DISABLE_IPV6"] = helm.Values{
			"value": strconv.FormatBool(*lb.DisableIPv6),
		}
	}

	// HCLOUD_LOAD_BALANCERS_USES_PROXYPROTOCOL
	if lb.UsesProxyProtocol != nil {
		env["HCLOUD_LOAD_BALANCERS_USES_PROXYPROTOCOL"] = helm.Values{
			"value": strconv.FormatBool(*lb.UsesProxyProtocol),
		}
	}

	// Health check settings
	hc := &lb.HealthCheck
	if hc.Interval > 0 {
		env["HCLOUD_LOAD_BALANCERS_HEALTH_CHECK_INTERVAL"] = helm.Values{
			"value": fmt.Sprintf("%ds", hc.Interval),
		}
	}
	if hc.Timeout > 0 {
		env["HCLOUD_LOAD_BALANCERS_HEALTH_CHECK_TIMEOUT"] = helm.Values{
			"value": fmt.Sprintf("%ds", hc.Timeout),
		}
	}
	if hc.Retries > 0 {
		env["HCLOUD_LOAD_BALANCERS_HEALTH_CHECK_RETRIES"] = helm.Values{
			"value": strconv.Itoa(hc.Retries),
		}
	}

	// HCLOUD_NETWORK_ROUTES_ENABLED
	if ccm.NetworkRoutesEnabled != nil {
		env["HCLOUD_NETWORK_ROUTES_ENABLED"] = helm.Values{
			"value": strconv.FormatBool(*ccm.NetworkRoutesEnabled),
		}
	}

	// HCLOUD_LOAD_BALANCERS_PRIVATE_SUBNET_IP_RANGE
	// This is derived from the load balancer subnet, which we get from the network config
	lbSubnetRange := getLBSubnetIPRange(cfg)
	if lbSubnetRange != "" {
		env["HCLOUD_LOAD_BALANCERS_PRIVATE_SUBNET_IP_RANGE"] = helm.Values{
			"value": lbSubnetRange,
		}
	}

	return env
}

// getClusterCIDR returns the pod CIDR for CCM networking configuration.
// This is used for the clusterCIDR setting in the CCM helm values.
func getClusterCIDR(cfg *config.Config) string {
	// Use pod CIDR if explicitly set
	if cfg.Network.PodIPv4CIDR != "" {
		return cfg.Network.PodIPv4CIDR
	}

	// Use native routing CIDR if set (for Cilium native routing mode)
	if cfg.Network.NativeRoutingIPv4CIDR != "" {
		return cfg.Network.NativeRoutingIPv4CIDR
	}

	// Fall back to the main network CIDR
	if cfg.Network.IPv4CIDR != "" {
		return cfg.Network.IPv4CIDR
	}

	// Default Flannel CIDR if nothing else is set
	return "10.244.0.0/16"
}

// getLBSubnetIPRange returns the IP range for the load balancer subnet.
// This uses the same subnet calculation as the infrastructure provisioner
// to ensure CCM creates load balancers in the correct subnet.
func getLBSubnetIPRange(cfg *config.Config) string {
	// Use the config's subnet allocation to get the LB subnet
	// This ensures consistency with how infrastructure provisioner creates subnets
	lbSubnet, err := cfg.GetSubnetForRole("load-balancer", 0)
	if err != nil {
		// If we can't calculate, let CCM use its default
		return ""
	}
	return lbSubnet
}
