package provisioning

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
)

// FuzzSpecToConfig ensures SpecToConfig never panics on arbitrary CRD field values.
func FuzzSpecToConfig(f *testing.F) {
	// Seed: valid cluster
	f.Add("test", "fsn1", "example.com", 1, "cx23", 2, "cx23", "1.32.0", "v1.9.0", true, true, true, true, true, true, true, "cd", "metrics")
	// Seed: minimal
	f.Add("x", "nbg1", "", 1, "cx23", 0, "cpx22", "1.30.0", "v1.7.0", false, false, false, false, false, false, false, "", "")
	// Seed: empty strings
	f.Add("", "", "", 0, "", 0, "", "", "", false, false, false, false, false, false, false, "", "")
	// Seed: legacy server sizes
	f.Add("legacy", "hel1", "test.dev", 3, "cx22", 5, "cx52", "1.31.0", "v1.8.0", true, true, true, true, true, true, true, "argo", "grafana")

	f.Fuzz(func(t *testing.T, name, region, domain string, cpCount int, cpSize string, workerCount int, workerSize, k8sVersion, talosVersion string, traefik, certMgr, extDNS, argocd, metricsServer, monitoring, backup bool, argoSub, grafanaSub string) {
		addons := &k8znerv1alpha1.AddonSpec{
			Traefik:          traefik,
			CertManager:      certMgr,
			ExternalDNS:      extDNS,
			ArgoCD:           argocd,
			MetricsServer:    metricsServer,
			Monitoring:       monitoring,
			ArgoSubdomain:    argoSub,
			GrafanaSubdomain: grafanaSub,
		}

		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Region: region,
				Domain: domain,
				ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{
					Count: cpCount,
					Size:  cpSize,
				},
				Workers: k8znerv1alpha1.WorkerSpec{
					Count: workerCount,
					Size:  workerSize,
				},
				Kubernetes: k8znerv1alpha1.KubernetesSpec{Version: k8sVersion},
				Talos:      k8znerv1alpha1.TalosSpec{Version: talosVersion},
				Addons:     addons,
			},
		}

		var backupSpec *k8znerv1alpha1.BackupSpec
		if backup {
			backupSpec = &k8znerv1alpha1.BackupSpec{
				Enabled:  true,
				Schedule: "0 * * * *",
			}
		}
		cluster.Spec.Backup = backupSpec

		creds := &Credentials{
			HCloudToken:        "fuzz-token",
			CloudflareAPIToken: "fuzz-cf-token",
			BackupS3AccessKey:  "fuzz-s3-key",
			BackupS3SecretKey:  "fuzz-s3-secret",
			BackupS3Endpoint:   "https://fuzz.endpoint",
			BackupS3Bucket:     "fuzz-bucket",
			BackupS3Region:     "fuzz-region",
		}

		// Must not panic — errors are fine (e.g., invalid CIDRs in CalculateSubnets)
		result, err := SpecToConfig(cluster, creds)
		if err != nil {
			return
		}

		// If conversion succeeded, verify basic invariants
		if result.ClusterName != name {
			t.Errorf("ClusterName = %q, want %q", result.ClusterName, name)
		}
		if result.Location != region {
			t.Errorf("Location = %q, want %q", result.Location, region)
		}
	})
}

// FuzzNormalizeServerSize ensures the internal normalizeServerSize never panics.
func FuzzNormalizeServerSize(f *testing.F) {
	f.Add("cx22")
	f.Add("cx32")
	f.Add("CX22")
	f.Add("")
	f.Add("arbitrary")
	f.Add("cpx22")

	f.Fuzz(func(t *testing.T, s string) {
		// Must not panic
		result := normalizeServerSize(s)
		// Result must not be empty if input was non-empty (passthrough behavior)
		if s != "" && result == "" {
			t.Errorf("normalizeServerSize(%q) returned empty string", s)
		}
	})
}
