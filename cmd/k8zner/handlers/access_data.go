package handlers

import (
	"context"
	"fmt"
	"log"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/imamik/k8zner/internal/config"
	"gopkg.in/yaml.v3"
)

const accessDataPath = "access-data.yaml"

type serviceAccessInfo struct {
	Enabled  bool   `yaml:"enabled"`
	URL      string `yaml:"url,omitempty"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

type clusterAccessData struct {
	ClusterName string             `yaml:"cluster_name"`
	SavedAt     string             `yaml:"saved_at"`
	TalosConfig string             `yaml:"talos_config"`
	Kubeconfig  string             `yaml:"kubeconfig"`
	ArgoCD      *serviceAccessInfo `yaml:"argocd,omitempty"`
	Grafana     *serviceAccessInfo `yaml:"grafana,omitempty"`
}

// persistAccessData writes cluster access details to disk and optionally enriches with addon credentials.
func persistAccessData(ctx context.Context, cfg *config.Config, kubeconfig []byte, includeAddonCredentials bool) error {
	data := buildAccessDataFromConfig(cfg)

	if includeAddonCredentials {
		if err := hydrateAddonCredentials(ctx, kubeconfig, data); err != nil {
			log.Printf("Warning: failed to load addon credentials: %v", err)
		}
	}

	content, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal access data: %w", err)
	}

	if err := writeFile(accessDataPath, content, 0600); err != nil {
		return fmt.Errorf("failed to write access data: %w", err)
	}

	return nil
}

func buildAccessDataFromConfig(cfg *config.Config) *clusterAccessData {
	data := &clusterAccessData{
		ClusterName: cfg.ClusterName,
		SavedAt:     time.Now().UTC().Format(time.RFC3339),
		TalosConfig: talosConfigPath,
		Kubeconfig:  kubeconfigPath,
	}

	if cfg.Addons.ArgoCD.Enabled {
		data.ArgoCD = &serviceAccessInfo{
			Enabled:  true,
			URL:      buildServiceURL(cfg.Addons.ArgoCD.IngressHost),
			Username: "admin",
		}
	}

	if cfg.Addons.KubePrometheusStack.Enabled {
		grafana := &serviceAccessInfo{
			Enabled: true,
			URL:     buildServiceURL(cfg.Addons.KubePrometheusStack.Grafana.IngressHost),
		}
		if cfg.Addons.KubePrometheusStack.Grafana.AdminPassword != "" {
			grafana.Username = "admin"
			grafana.Password = cfg.Addons.KubePrometheusStack.Grafana.AdminPassword
		}
		data.Grafana = grafana
	}

	return data
}

func buildServiceURL(host string) string {
	if host == "" {
		return ""
	}
	return "https://" + host
}

func hydrateAddonCredentials(ctx context.Context, kubeconfig []byte, data *clusterAccessData) error {
	if len(kubeconfig) == 0 {
		return nil
	}

	restCfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to parse kubeconfig: %w", err)
	}

	k8sClient, err := client.New(restCfg, client.Options{})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	if data.ArgoCD != nil {
		secret := &corev1.Secret{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: "argocd", Name: "argocd-initial-admin-secret"}, secret); err == nil {
			if password, ok := secret.Data["password"]; ok {
				data.ArgoCD.Password = string(password)
			}
		}
	}

	if data.Grafana != nil {
		secret := &corev1.Secret{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: "monitoring", Name: "kube-prometheus-stack-grafana"}, secret); err == nil {
			if username, ok := secret.Data["admin-user"]; ok {
				data.Grafana.Username = string(username)
			}
			if password, ok := secret.Data["admin-password"]; ok {
				data.Grafana.Password = string(password)
			}
		}
	}

	return nil
}
