package addons

import (
	"testing"

	"github.com/imamik/k8zner/internal/addons/helm"
	"github.com/imamik/k8zner/internal/config"
)

// Package-level vars to prevent compiler optimization of benchmark results.
var benchResultHelmValues helm.Values

// --- Traefik value builder benchmarks ---

func BenchmarkBuildTraefikValues_SingleWorker(b *testing.B) {
	cfg := &config.Config{
		ClusterName: "bench-cluster",
		Location:    "fsn1",
		Workers:     []config.WorkerNodePool{{Count: 1}},
		Addons: config.AddonsConfig{
			Traefik: config.TraefikConfig{
				Enabled: true,
			},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultHelmValues = buildTraefikValues(cfg)
	}
}

func BenchmarkBuildTraefikValues_LargeCluster(b *testing.B) {
	cfg := &config.Config{
		ClusterName: "bench-cluster",
		Location:    "fsn1",
		Workers:     []config.WorkerNodePool{{Count: 5}},
		Addons: config.AddonsConfig{
			Traefik: config.TraefikConfig{
				Enabled:               true,
				Kind:                  "Deployment",
				ExternalTrafficPolicy: "Cluster",
				IngressClass:          "traefik",
			},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultHelmValues = buildTraefikValues(cfg)
	}
}

func BenchmarkBuildTraefikValues_WithCustomValues(b *testing.B) {
	cfg := &config.Config{
		ClusterName: "bench-cluster",
		Location:    "fsn1",
		Workers:     []config.WorkerNodePool{{Count: 3}},
		Addons: config.AddonsConfig{
			Traefik: config.TraefikConfig{
				Enabled:               true,
				Kind:                  "Deployment",
				ExternalTrafficPolicy: "Cluster",
				IngressClass:          "traefik",
				Helm: config.HelmChartConfig{
					Values: map[string]any{
						"additionalArguments": []string{"--ping", "--metrics.prometheus"},
						"resources": map[string]any{
							"requests": map[string]any{
								"cpu":    "200m",
								"memory": "256Mi",
							},
						},
					},
				},
			},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultHelmValues = buildTraefikValues(cfg)
	}
}

func BenchmarkBuildTraefikValues_Parallel(b *testing.B) {
	cfg := &config.Config{
		ClusterName: "bench-cluster",
		Location:    "fsn1",
		Workers:     []config.WorkerNodePool{{Count: 3}},
		Addons: config.AddonsConfig{
			Traefik: config.TraefikConfig{
				Enabled: true,
			},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = buildTraefikValues(cfg)
		}
	})
}

// --- Cilium value builder benchmarks ---

func BenchmarkBuildCiliumValues_BasicConfig(b *testing.B) {
	cfg := &config.Config{
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Count: 1},
			},
		},
		Addons: config.AddonsConfig{
			Cilium: config.CiliumConfig{
				Enabled:     true,
				RoutingMode: "tunnel",
			},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultHelmValues = buildCiliumValues(cfg)
	}
}

func BenchmarkBuildCiliumValues_FullHA(b *testing.B) {
	cfg := &config.Config{
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Count: 3},
			},
		},
		Addons: config.AddonsConfig{
			Cilium: config.CiliumConfig{
				Enabled:                     true,
				RoutingMode:                 "tunnel",
				KubeProxyReplacementEnabled: true,
				HubbleEnabled:               true,
				HubbleRelayEnabled:          true,
				HubbleUIEnabled:             true,
				EncryptionEnabled:           true,
				EncryptionType:              "wireguard",
				GatewayAPIEnabled:           true,
				ServiceMonitorEnabled:       true,
			},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultHelmValues = buildCiliumValues(cfg)
	}
}

func BenchmarkBuildCiliumValues_WithIPSec(b *testing.B) {
	cfg := &config.Config{
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Count: 3},
			},
		},
		Addons: config.AddonsConfig{
			Cilium: config.CiliumConfig{
				Enabled:                     true,
				RoutingMode:                 "native",
				KubeProxyReplacementEnabled: true,
				EncryptionEnabled:           true,
				EncryptionType:              "ipsec",
			},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultHelmValues = buildCiliumValues(cfg)
	}
}

func BenchmarkBuildCiliumValues_WithCustomValues(b *testing.B) {
	cfg := &config.Config{
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Count: 1},
			},
		},
		Addons: config.AddonsConfig{
			Cilium: config.CiliumConfig{
				Enabled:                     true,
				RoutingMode:                 "tunnel",
				KubeProxyReplacementEnabled: true,
				HubbleEnabled:               true,
				HubbleRelayEnabled:          true,
				Helm: config.HelmChartConfig{
					Values: map[string]any{
						"debug": map[string]any{
							"enabled": true,
						},
						"resources": map[string]any{
							"requests": map[string]any{
								"cpu":    "200m",
								"memory": "256Mi",
							},
						},
					},
				},
			},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultHelmValues = buildCiliumValues(cfg)
	}
}

func BenchmarkBuildCiliumValues_Parallel(b *testing.B) {
	cfg := &config.Config{
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Count: 3},
			},
		},
		Addons: config.AddonsConfig{
			Cilium: config.CiliumConfig{
				Enabled:                     true,
				RoutingMode:                 "tunnel",
				KubeProxyReplacementEnabled: true,
				HubbleEnabled:               true,
				HubbleRelayEnabled:          true,
				HubbleUIEnabled:             true,
			},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = buildCiliumValues(cfg)
		}
	})
}

// --- ArgoCD value builder benchmarks ---

func BenchmarkBuildArgoCDValues_Default(b *testing.B) {
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			ArgoCD: config.ArgoCDConfig{
				Enabled: true,
			},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultHelmValues = buildArgoCDValues(cfg)
	}
}

func BenchmarkBuildArgoCDValues_HAMode(b *testing.B) {
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			ArgoCD: config.ArgoCDConfig{
				Enabled: true,
				HA:      true,
			},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultHelmValues = buildArgoCDValues(cfg)
	}
}

func BenchmarkBuildArgoCDValues_WithIngress(b *testing.B) {
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			ArgoCD: config.ArgoCDConfig{
				Enabled:          true,
				HA:               true,
				IngressEnabled:   true,
				IngressHost:      "argocd.example.com",
				IngressClassName: "traefik",
				IngressTLS:       true,
			},
			CertManager: config.CertManagerConfig{
				Cloudflare: config.CertManagerCloudflareConfig{
					Enabled:    true,
					Production: true,
				},
			},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultHelmValues = buildArgoCDValues(cfg)
	}
}

func BenchmarkBuildArgoCDValues_WithCustomValues(b *testing.B) {
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			ArgoCD: config.ArgoCDConfig{
				Enabled:          true,
				HA:               true,
				IngressEnabled:   true,
				IngressHost:      "argocd.example.com",
				IngressClassName: "traefik",
				IngressTLS:       true,
				Helm: config.HelmChartConfig{
					Values: map[string]any{
						"server": map[string]any{
							"autoscaling": map[string]any{
								"enabled":     true,
								"minReplicas": 2,
								"maxReplicas": 5,
							},
						},
						"controller": map[string]any{
							"resources": map[string]any{
								"requests": map[string]any{
									"cpu":    "500m",
									"memory": "512Mi",
								},
							},
						},
					},
				},
			},
			CertManager: config.CertManagerConfig{
				Cloudflare: config.CertManagerCloudflareConfig{
					Enabled:    true,
					Production: true,
				},
			},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultHelmValues = buildArgoCDValues(cfg)
	}
}

func BenchmarkBuildArgoCDValues_Parallel(b *testing.B) {
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			ArgoCD: config.ArgoCDConfig{
				Enabled:          true,
				HA:               true,
				IngressEnabled:   true,
				IngressHost:      "argocd.example.com",
				IngressClassName: "traefik",
				IngressTLS:       true,
			},
			CertManager: config.CertManagerConfig{
				Cloudflare: config.CertManagerCloudflareConfig{
					Enabled:    true,
					Production: true,
				},
			},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = buildArgoCDValues(cfg)
		}
	})
}
