package k8s

import (
	"fmt"
	"log"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

// HelmClient handles Helm operations.
type HelmClient struct {
	settings *cli.EnvSettings
}

// NewHelmClient creates a new HelmClient.
func NewHelmClient() *HelmClient {
	return &HelmClient{
		settings: cli.New(),
	}
}

// InstallOrUpgrade installs or upgrades a Helm chart.
func (h *HelmClient) InstallOrUpgrade(kubeconfig []byte, namespace, releaseName, repoURL, chartName, version string, values map[string]interface{}) error {
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create rest config: %w", err)
	}

	actionConfig := new(action.Configuration)
	clientGetter := &genericRESTClientGetter{
		config:    restConfig,
		namespace: namespace,
	}

	if err := actionConfig.Init(clientGetter, namespace, os.Getenv("HELM_DRIVER"), log.Printf); err != nil {
		return fmt.Errorf("failed to init action config: %w", err)
	}

	cp := &action.ChartPathOptions{}
	cp.RepoURL = repoURL
	cp.Version = version

	chartPath, err := cp.LocateChart(chartName, h.settings)
	if err != nil {
		return fmt.Errorf("failed to locate chart: %w", err)
	}

	chart, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("failed to load chart: %w", err)
	}

	// Check if already installed
	histClient := action.NewHistory(actionConfig)
	histClient.Max = 1
	if _, err := histClient.Run(releaseName); err == nil {
		upgrade := action.NewUpgrade(actionConfig)
		upgrade.Namespace = namespace
		upgrade.Wait = true
		upgrade.Timeout = 5 * time.Minute
		_, err = upgrade.Run(releaseName, chart, values)
		if err != nil {
			return fmt.Errorf("helm upgrade failed: %w", err)
		}
		return nil
	}

	install := action.NewInstall(actionConfig)
	install.Namespace = namespace
	install.ReleaseName = releaseName
	install.CreateNamespace = true
	install.Wait = true
	install.Timeout = 5 * time.Minute
	_, err = install.Run(chart, values)
	if err != nil {
		return fmt.Errorf("helm install failed: %w", err)
	}

	return nil
}

// AddRepo adds a repository to the helm settings.
func (h *HelmClient) AddRepo(name, url string) error {
	f, err := repo.LoadFile(h.settings.RepositoryConfig)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if os.IsNotExist(err) {
		f = repo.NewFile()
	}

	c := repo.Entry{
		Name: name,
		URL:  url,
	}

	r, err := repo.NewChartRepository(&c, getter.All(h.settings))
	if err != nil {
		return err
	}

	if _, err := r.DownloadIndexFile(); err != nil {
		return err
	}

	f.Update(&c)
	return f.WriteFile(h.settings.RepositoryConfig, 0644)
}

// genericRESTClientGetter implements basic RESTClientGetter for Helm.
type genericRESTClientGetter struct {
	config    *rest.Config
	namespace string
}

func (g *genericRESTClientGetter) ToRESTConfig() (*rest.Config, error) {
	return g.config, nil
}

func (g *genericRESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	config := g.config
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(discoveryClient), nil
}

func (g *genericRESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	discoveryClient, err := g.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient), nil
}

func (g *genericRESTClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return clientcmd.NewDefaultClientConfig(*clientcmdapi.NewConfig(), &clientcmd.ConfigOverrides{})
}
