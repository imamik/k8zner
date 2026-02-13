package handlers

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/imamik/k8zner/internal/config"
)

func TestBuildAccessDataFromConfig(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		ClusterName: "demo",
		Addons: config.AddonsConfig{
			ArgoCD: config.ArgoCDConfig{
				Enabled:     true,
				IngressHost: "argo.example.com",
			},
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Enabled: true,
				Grafana: config.KubePrometheusGrafanaConfig{
					IngressHost:   "grafana.example.com",
					AdminPassword: "grafana-pass",
				},
			},
		},
	}

	data := buildAccessDataFromConfig(cfg)
	require.NotNil(t, data)
	assert.Equal(t, "demo", data.ClusterName)
	assert.Equal(t, talosConfigPath, data.TalosConfig)
	assert.Equal(t, kubeconfigPath, data.Kubeconfig)
	require.NotNil(t, data.ArgoCD)
	assert.Equal(t, "https://argo.example.com", data.ArgoCD.URL)
	assert.Equal(t, "admin", data.ArgoCD.Username)
	require.NotNil(t, data.Grafana)
	assert.Equal(t, "https://grafana.example.com", data.Grafana.URL)
	assert.Equal(t, "admin", data.Grafana.Username)
	assert.Equal(t, "grafana-pass", data.Grafana.Password)
}

func TestPersistAccessData(t *testing.T) {
	t.Parallel()

	t.Run("writes file", func(t *testing.T) {
		t.Parallel()
		origWrite := writeFile
		defer func() { writeFile = origWrite }()

		var (
			path string
			perm os.FileMode
		)
		writeFile = func(p string, b []byte, m os.FileMode) error {
			path = p
			perm = m
			assert.Contains(t, string(b), "cluster_name: demo")
			assert.Contains(t, string(b), "argocd:")
			assert.Contains(t, string(b), "grafana:")
			return nil
		}

		cfg := &config.Config{
			ClusterName: "demo",
			Addons: config.AddonsConfig{
				ArgoCD:              config.ArgoCDConfig{Enabled: true},
				KubePrometheusStack: config.KubePrometheusStackConfig{Enabled: true},
			},
		}

		err := persistAccessData(context.Background(), cfg, nil, false)
		require.NoError(t, err)
		assert.Equal(t, accessDataPath, path)
		assert.Equal(t, os.FileMode(0600), perm)
	})

	t.Run("write error", func(t *testing.T) {
		t.Parallel()
		origWrite := writeFile
		defer func() { writeFile = origWrite }()
		writeFile = func(_ string, _ []byte, _ os.FileMode) error { return errors.New("disk full") }

		cfg := &config.Config{ClusterName: "demo"}
		err := persistAccessData(context.Background(), cfg, nil, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to write access data")
	})
}

func TestBuildServiceURL(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", buildServiceURL(""))
	assert.Equal(t, "https://argo.example.com", buildServiceURL("argo.example.com"))
}
