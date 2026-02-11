package helm

import (
	"fmt"

	"github.com/imamik/k8zner/internal/config"
)

// CCMUninitializedToleration returns a toleration for the CCM uninitialized taint.
func CCMUninitializedToleration() Values {
	return Values{
		"key":      "node.cloudprovider.kubernetes.io/uninitialized",
		"operator": "Exists",
	}
}

// BootstrapTolerations returns tolerations for addons that must run on control plane
// nodes before CCM initializes them (control-plane + master + ccm-uninitialized + not-ready).
func BootstrapTolerations() []Values {
	return []Values{
		{
			"key":      "node-role.kubernetes.io/control-plane",
			"effect":   "NoSchedule",
			"operator": "Exists",
		},
		{
			"key":      "node-role.kubernetes.io/master",
			"effect":   "NoSchedule",
			"operator": "Exists",
		},
		{
			"key":    "node.cloudprovider.kubernetes.io/uninitialized",
			"value":  "true",
			"effect": "NoSchedule",
		},
		{
			"key":      "node.kubernetes.io/not-ready",
			"operator": "Exists",
		},
	}
}

// ControlPlaneNodeSelector returns a nodeSelector targeting control plane nodes.
func ControlPlaneNodeSelector() Values {
	return Values{
		"node-role.kubernetes.io/control-plane": "",
	}
}

// TopologySpread returns hostname + zone topology spread constraints.
func TopologySpread(instance, name, hostnamePolicy string) []Values {
	labelSelector := Values{
		"matchLabels": Values{
			"app.kubernetes.io/instance": instance,
			"app.kubernetes.io/name":     name,
		},
	}

	return []Values{
		{
			"topologyKey":       "kubernetes.io/hostname",
			"maxSkew":           1,
			"whenUnsatisfiable": hostnamePolicy,
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

// IngressAnnotations builds common ingress annotations for TLS and DNS.
// external-dns auto-discovers the target IP from the Ingress status
// (set by Traefik's LoadBalancer service).
func IngressAnnotations(cfg *config.Config, host string) Values {
	annotations := Values{}

	// Determine which ClusterIssuer to use based on cert-manager Cloudflare config
	clusterIssuer := "letsencrypt-prod" // Default fallback
	if cfg.Addons.CertManager.Cloudflare.Enabled {
		if cfg.Addons.CertManager.Cloudflare.Production {
			clusterIssuer = "letsencrypt-cloudflare-production"
		} else {
			clusterIssuer = "letsencrypt-cloudflare-staging"
		}
	}
	annotations["cert-manager.io/cluster-issuer"] = clusterIssuer

	// Add external-dns hostname annotation if Cloudflare/external-dns is enabled
	if cfg.Addons.Cloudflare.Enabled && cfg.Addons.ExternalDNS.Enabled {
		annotations["external-dns.alpha.kubernetes.io/hostname"] = host
	}

	return annotations
}

// NamespaceManifest generates a Namespace YAML manifest string.
func NamespaceManifest(name string, labels map[string]string) string {
	yaml := fmt.Sprintf("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: %s\n", name)
	if len(labels) > 0 {
		yaml += "  labels:\n"
		for k, v := range labels {
			yaml += fmt.Sprintf("    %s: %s\n", k, v)
		}
	}
	return yaml
}
