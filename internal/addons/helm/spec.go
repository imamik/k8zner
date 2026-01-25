package helm

import "github.com/imamik/k8zner/internal/config"

// GetChartSpec returns the chart spec for the given addon name,
// applying any overrides from the HelmChartConfig.
// This allows users to customize repository, chart name, and version via config.
func GetChartSpec(name string, helmCfg config.HelmChartConfig) ChartSpec {
	spec, ok := DefaultChartSpecs[name]
	if !ok {
		// Return empty spec if addon not found - caller should handle this
		return ChartSpec{}
	}

	// Apply config overrides
	if helmCfg.Repository != "" {
		spec.Repository = helmCfg.Repository
	}
	if helmCfg.Chart != "" {
		spec.Name = helmCfg.Chart
	}
	if helmCfg.Version != "" {
		spec.Version = helmCfg.Version
	}

	return spec
}
