package helm

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
)

// Client provides Helm operations using in-memory kubeconfig.
type Client struct {
	kubeconfig   []byte
	namespace    string
	actionConfig *action.Configuration
}

// NewClient creates a Helm client from kubeconfig bytes.
func NewClient(kubeconfig []byte, namespace string) (*Client, error) {
	c := &Client{
		kubeconfig: kubeconfig,
		namespace:  namespace,
	}

	actionConfig := new(action.Configuration)
	restGetter := NewInMemoryRESTClientGetter(kubeconfig, namespace)

	// Initialize with a no-op logger (suppress debug output)
	if err := actionConfig.Init(restGetter, namespace, "secret", func(format string, v ...interface{}) {}); err != nil {
		return nil, fmt.Errorf("failed to initialize helm action config: %w", err)
	}

	c.actionConfig = actionConfig
	return c, nil
}

// InstallOrUpgrade installs a chart or upgrades if already installed.
func (c *Client) InstallOrUpgrade(ctx context.Context, releaseName, repoURL, chartName, version string, values map[string]interface{}) (*release.Release, error) {
	// Check if release exists
	histClient := action.NewHistory(c.actionConfig)
	histClient.Max = 1
	_, err := histClient.Run(releaseName)

	if err != nil {
		// Release doesn't exist, install
		return c.install(ctx, releaseName, repoURL, chartName, version, values)
	}
	// Release exists, upgrade
	return c.upgrade(ctx, releaseName, repoURL, chartName, version, values)
}

func (c *Client) install(ctx context.Context, releaseName, repoURL, chartName, version string, values map[string]interface{}) (*release.Release, error) {
	installClient := action.NewInstall(c.actionConfig)
	installClient.ReleaseName = releaseName
	installClient.Namespace = c.namespace
	installClient.CreateNamespace = true
	installClient.Version = version
	installClient.Wait = true
	installClient.Timeout = 10 * time.Minute

	chart, err := c.loadChart(repoURL, chartName, version)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart: %w", err)
	}

	return installClient.RunWithContext(ctx, chart, values)
}

func (c *Client) upgrade(ctx context.Context, releaseName, repoURL, chartName, version string, values map[string]interface{}) (*release.Release, error) {
	upgradeClient := action.NewUpgrade(c.actionConfig)
	upgradeClient.Namespace = c.namespace
	upgradeClient.Version = version
	upgradeClient.Wait = true
	upgradeClient.Timeout = 10 * time.Minute
	upgradeClient.ReuseValues = false // Use new values

	chart, err := c.loadChart(repoURL, chartName, version)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart: %w", err)
	}

	return upgradeClient.RunWithContext(ctx, releaseName, chart, values)
}

func (c *Client) loadChart(repoURL, chartName, version string) (*chart.Chart, error) {
	settings := cli.New()

	// Create registry client for OCI support
	registryClient, err := registry.NewClient(
		registry.ClientOptDebug(false),
		registry.ClientOptWriter(io.Discard),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create registry client: %w", err)
	}

	// Find the chart in the repository
	chartPath, err := repo.FindChartInRepoURL(
		repoURL,
		chartName,
		version,
		"", "", "",
		getter.All(settings),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find chart %s in repo %s: %w", chartName, repoURL, err)
	}

	// Clean up the downloaded chart after loading
	defer func() {
		_ = os.Remove(chartPath)
	}()

	// Suppress registry client warning by setting it
	_ = registryClient

	return loader.Load(chartPath)
}

// Uninstall removes a Helm release.
func (c *Client) Uninstall(releaseName string) error {
	uninstallClient := action.NewUninstall(c.actionConfig)
	uninstallClient.Wait = true
	uninstallClient.Timeout = 5 * time.Minute

	_, err := uninstallClient.Run(releaseName)
	return err
}

// ReleaseExists checks if a release exists.
func (c *Client) ReleaseExists(releaseName string) (bool, error) {
	histClient := action.NewHistory(c.actionConfig)
	histClient.Max = 1
	_, err := histClient.Run(releaseName)
	if err != nil {
		return false, nil
	}
	return true, nil
}
