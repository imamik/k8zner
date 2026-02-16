package config

import (
	"testing"
)

func TestVersionMatrix_Validate(t *testing.T) {
	t.Parallel()
	vm := DefaultVersionMatrix()

	if vm.Talos == "" {
		t.Error("Talos version is empty")
	}
	if vm.Kubernetes == "" {
		t.Error("Kubernetes version is empty")
	}
	if vm.Cilium == "" {
		t.Error("Cilium version is empty")
	}
	if vm.Traefik == "" {
		t.Error("Traefik version is empty")
	}
	if vm.CertManager == "" {
		t.Error("CertManager version is empty")
	}
	if vm.ExternalDNS == "" {
		t.Error("ExternalDNS version is empty")
	}
	if vm.ArgoCD == "" {
		t.Error("ArgoCD version is empty")
	}
	if vm.MetricsServer == "" {
		t.Error("MetricsServer version is empty")
	}
	if vm.HCloudCCM == "" {
		t.Error("HCloudCCM version is empty")
	}
	if vm.HCloudCSI == "" {
		t.Error("HCloudCSI version is empty")
	}
}

func TestVersionMatrix_TalosVersion(t *testing.T) {
	t.Parallel()
	vm := DefaultVersionMatrix()

	if vm.Talos[0] != 'v' {
		t.Errorf("Talos version should start with 'v', got %s", vm.Talos)
	}
}

func TestVersionMatrix_KubernetesVersion(t *testing.T) {
	t.Parallel()
	vm := DefaultVersionMatrix()

	if vm.Kubernetes[0] == 'v' {
		t.Errorf("Kubernetes version should not start with 'v', got %s", vm.Kubernetes)
	}
}

func TestVersionMatrix_HelmChartVersions(t *testing.T) {
	t.Parallel()
	vm := DefaultVersionMatrix()

	versions := []struct {
		name    string
		version string
	}{
		{"Cilium", vm.Cilium},
		{"Traefik", vm.Traefik},
		{"CertManager", vm.CertManager},
		{"ExternalDNS", vm.ExternalDNS},
		{"ArgoCD", vm.ArgoCD},
		{"MetricsServer", vm.MetricsServer},
	}

	for _, v := range versions {
		t.Run(v.name, func(t *testing.T) {
			t.Parallel()
			if v.version == "" {
				t.Errorf("%s version is empty", v.name)
			}
		})
	}
}

func TestLoadBalancerType(t *testing.T) {
	t.Parallel()
	if LoadBalancerType != "lb11" {
		t.Errorf("LoadBalancerType = %s, want lb11", LoadBalancerType)
	}
}

func TestNetworkCIDRs(t *testing.T) {
	t.Parallel()
	if NetworkCIDR == "" {
		t.Error("NetworkCIDR is empty")
	}
	if PodCIDR == "" {
		t.Error("PodCIDR is empty")
	}
	if ServiceCIDR == "" {
		t.Error("ServiceCIDR is empty")
	}
}
