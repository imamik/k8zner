package helm

// DefaultChartSpecs contains the default chart specifications for each addon.
// These define the official Helm chart repositories, names, and versions.
// Users can override these settings via config.HelmChartConfig.
var DefaultChartSpecs = map[string]ChartSpec{
	"hcloud-ccm": {
		Repository: "https://charts.hetzner.cloud",
		Name:       "hcloud-cloud-controller-manager",
		Version:    "1.29.0",
	},
	"hcloud-csi": {
		Repository: "https://charts.hetzner.cloud",
		Name:       "hcloud-csi",
		Version:    "2.18.3",
	},
	"cilium": {
		Repository: "https://helm.cilium.io",
		Name:       "cilium",
		Version:    "1.18.5",
	},
	"cert-manager": {
		Repository: "https://charts.jetstack.io",
		Name:       "cert-manager",
		Version:    "v1.19.2",
	},
	"ingress-nginx": {
		Repository: "https://kubernetes.github.io/ingress-nginx",
		Name:       "ingress-nginx",
		Version:    "4.11.3",
	},
	"traefik": {
		Repository: "https://traefik.github.io/charts",
		Name:       "traefik",
		Version:    "39.0.0",
	},
	"metrics-server": {
		Repository: "https://kubernetes-sigs.github.io/metrics-server",
		Name:       "metrics-server",
		Version:    "3.12.2",
	},
	"cluster-autoscaler": {
		Repository: "https://kubernetes.github.io/autoscaler",
		Name:       "cluster-autoscaler",
		Version:    "9.50.1",
	},
	"longhorn": {
		Repository: "https://charts.longhorn.io",
		Name:       "longhorn",
		Version:    "1.10.1",
	},
	"argo-cd": {
		Repository: "https://argoproj.github.io/argo-helm",
		Name:       "argo-cd",
		Version:    "9.3.5",
	},
}
