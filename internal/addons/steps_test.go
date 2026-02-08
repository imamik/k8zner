package addons

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/imamik/k8zner/internal/config"
)

func TestEnabledSteps(t *testing.T) {
	t.Run("all addons enabled", func(t *testing.T) {
		cfg := &config.Config{
			Addons: config.AddonsConfig{
				CCM:                 config.CCMConfig{Enabled: true},
				CSI:                 config.CSIConfig{Enabled: true},
				MetricsServer:       config.MetricsServerConfig{Enabled: true},
				CertManager:         config.CertManagerConfig{Enabled: true},
				Traefik:             config.TraefikConfig{Enabled: true},
				ExternalDNS:         config.ExternalDNSConfig{Enabled: true},
				ArgoCD:              config.ArgoCDConfig{Enabled: true},
				KubePrometheusStack: config.KubePrometheusStackConfig{Enabled: true},
				TalosBackup:         config.TalosBackupConfig{Enabled: true},
			},
		}

		steps := EnabledSteps(cfg)

		require.Len(t, steps, 9)

		// Verify names
		names := make([]string, len(steps))
		for i, s := range steps {
			names[i] = s.Name
		}
		assert.Equal(t, []string{
			StepCCM,
			StepCSI,
			StepMetricsServer,
			StepCertManager,
			StepTraefik,
			StepExternalDNS,
			StepArgoCD,
			StepMonitoring,
			StepTalosBackup,
		}, names)
	})

	t.Run("minimal config - only CCM and CSI", func(t *testing.T) {
		cfg := &config.Config{
			Addons: config.AddonsConfig{
				CCM: config.CCMConfig{Enabled: true},
				CSI: config.CSIConfig{Enabled: true},
			},
		}

		steps := EnabledSteps(cfg)

		require.Len(t, steps, 2)
		assert.Equal(t, StepCCM, steps[0].Name)
		assert.Equal(t, StepCSI, steps[1].Name)
	})

	t.Run("no addons enabled", func(t *testing.T) {
		cfg := &config.Config{}

		steps := EnabledSteps(cfg)

		assert.Empty(t, steps)
	})

	t.Run("order values are ascending", func(t *testing.T) {
		cfg := &config.Config{
			Addons: config.AddonsConfig{
				CCM:                 config.CCMConfig{Enabled: true},
				CSI:                 config.CSIConfig{Enabled: true},
				MetricsServer:       config.MetricsServerConfig{Enabled: true},
				CertManager:         config.CertManagerConfig{Enabled: true},
				Traefik:             config.TraefikConfig{Enabled: true},
				ExternalDNS:         config.ExternalDNSConfig{Enabled: true},
				ArgoCD:              config.ArgoCDConfig{Enabled: true},
				KubePrometheusStack: config.KubePrometheusStackConfig{Enabled: true},
				TalosBackup:         config.TalosBackupConfig{Enabled: true},
			},
		}

		steps := EnabledSteps(cfg)

		for i := 1; i < len(steps); i++ {
			assert.Greater(t, steps[i].Order, steps[i-1].Order,
				"step %s (order %d) should be after step %s (order %d)",
				steps[i].Name, steps[i].Order, steps[i-1].Name, steps[i-1].Order)
		}
	})

	t.Run("CCM is always first when enabled", func(t *testing.T) {
		cfg := &config.Config{
			Addons: config.AddonsConfig{
				CCM:     config.CCMConfig{Enabled: true},
				ArgoCD:  config.ArgoCDConfig{Enabled: true},
				Traefik: config.TraefikConfig{Enabled: true},
			},
		}

		steps := EnabledSteps(cfg)

		require.Len(t, steps, 3)
		assert.Equal(t, StepCCM, steps[0].Name)
	})

	t.Run("ArgoCD and monitoring are last", func(t *testing.T) {
		cfg := &config.Config{
			Addons: config.AddonsConfig{
				CCM:                 config.CCMConfig{Enabled: true},
				ArgoCD:              config.ArgoCDConfig{Enabled: true},
				KubePrometheusStack: config.KubePrometheusStackConfig{Enabled: true},
				TalosBackup:         config.TalosBackupConfig{Enabled: true},
			},
		}

		steps := EnabledSteps(cfg)

		require.Len(t, steps, 4)
		assert.Equal(t, StepCCM, steps[0].Name)
		assert.Equal(t, StepArgoCD, steps[1].Name)
		assert.Equal(t, StepMonitoring, steps[2].Name)
		assert.Equal(t, StepTalosBackup, steps[3].Name)
	})
}

func TestInstallStep_UnknownStep(t *testing.T) {
	err := InstallStep(t.Context(), "nonexistent-addon", &config.Config{}, []byte("fake-kubeconfig"), 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create kubernetes client")
}

func TestStepConstants(t *testing.T) {
	// Verify step constants match expected addon names
	assert.Equal(t, "hcloud-ccm", StepCCM)
	assert.Equal(t, "hcloud-csi", StepCSI)
	assert.Equal(t, "metrics-server", StepMetricsServer)
	assert.Equal(t, "cert-manager", StepCertManager)
	assert.Equal(t, "traefik", StepTraefik)
	assert.Equal(t, "external-dns", StepExternalDNS)
	assert.Equal(t, "argocd", StepArgoCD)
	assert.Equal(t, "monitoring", StepMonitoring)
	assert.Equal(t, "talos-backup", StepTalosBackup)
}
