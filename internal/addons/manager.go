package addons

import (
	"context"
	"fmt"
	"log"
	"sort"
)

// Manager orchestrates the installation and management of addons.
type Manager struct {
	addons    []Addon
	k8sClient K8sClient
	options   InstallOptions
}

// NewManager creates a new addon manager.
func NewManager(k8sClient K8sClient, addons []Addon, options InstallOptions) *Manager {
	return &Manager{
		addons:    addons,
		k8sClient: k8sClient,
		options:   options,
	}
}

// Install installs all enabled addons in dependency order.
func (m *Manager) Install(ctx context.Context) error {
	// Filter enabled addons
	enabled := m.getEnabledAddons()
	if len(enabled) == 0 {
		log.Println("No addons enabled, skipping addon installation")
		return nil
	}

	// Sort addons by dependencies
	sorted, err := m.topologicalSort(enabled)
	if err != nil {
		return fmt.Errorf("failed to resolve addon dependencies: %w", err)
	}

	log.Printf("Installing %d addons in order: %v", len(sorted), addonNames(sorted))

	// Install each addon
	for _, addon := range sorted {
		if err := m.installAddon(ctx, addon); err != nil {
			if m.options.ContinueOnError {
				log.Printf("Error installing addon %s: %v (continuing)", addon.Name(), err)
				continue
			}
			return fmt.Errorf("failed to install addon %s: %w", addon.Name(), err)
		}
	}

	log.Println("All addons installed successfully")
	return nil
}

// installAddon installs a single addon.
func (m *Manager) installAddon(ctx context.Context, addon Addon) error {
	log.Printf("Installing addon: %s", addon.Name())

	// Generate manifests
	manifests, err := addon.GenerateManifests(ctx)
	if err != nil {
		return fmt.Errorf("failed to generate manifests: %w", err)
	}

	// Apply each manifest
	for i, manifest := range manifests {
		if manifest == "" {
			continue
		}

		log.Printf("Applying manifest %d/%d for addon %s", i+1, len(manifests), addon.Name())
		if err := m.k8sClient.Apply(ctx, manifest); err != nil {
			return fmt.Errorf("failed to apply manifest %d: %w", i+1, err)
		}
	}

	// Verify installation if enabled
	if m.options.VerifyInstallation {
		log.Printf("Verifying addon %s installation...", addon.Name())
		if err := addon.Verify(ctx, m.k8sClient); err != nil {
			return fmt.Errorf("failed to verify installation: %w", err)
		}
		log.Printf("Addon %s verified successfully", addon.Name())
	}

	log.Printf("Addon %s installed successfully", addon.Name())
	return nil
}

// getEnabledAddons returns a list of enabled addons.
func (m *Manager) getEnabledAddons() []Addon {
	var enabled []Addon
	for _, addon := range m.addons {
		if addon.Enabled() {
			enabled = append(enabled, addon)
		}
	}
	return enabled
}

// topologicalSort sorts addons by their dependencies using Kahn's algorithm.
func (m *Manager) topologicalSort(addons []Addon) ([]Addon, error) {
	// Build adjacency list and in-degree map
	graph := make(map[string][]string)
	inDegree := make(map[string]int)
	addonMap := make(map[string]Addon)

	// Initialize
	for _, addon := range addons {
		name := addon.Name()
		addonMap[name] = addon
		inDegree[name] = 0
		graph[name] = []string{}
	}

	// Build graph
	for _, addon := range addons {
		name := addon.Name()
		deps := addon.Dependencies()

		for _, dep := range deps {
			// Verify dependency exists
			if _, exists := addonMap[dep]; !exists {
				// Dependency not enabled, skip it
				log.Printf("Warning: addon %s depends on %s which is not enabled", name, dep)
				continue
			}

			// Add edge from dependency to addon
			graph[dep] = append(graph[dep], name)
			inDegree[name]++
		}
	}

	// Find all nodes with in-degree 0
	var queue []string
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	// Sort the queue for deterministic ordering
	sort.Strings(queue)

	// Process queue
	var result []Addon
	for len(queue) > 0 {
		// Pop from queue
		current := queue[0]
		queue = queue[1:]

		result = append(result, addonMap[current])

		// Reduce in-degree of neighbors
		neighbors := graph[current]
		sort.Strings(neighbors) // Deterministic ordering

		for _, neighbor := range neighbors {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	// Check for cycles
	if len(result) != len(addons) {
		return nil, fmt.Errorf("circular dependency detected in addons")
	}

	return result, nil
}

// addonNames returns a slice of addon names for logging.
func addonNames(addons []Addon) []string {
	names := make([]string, len(addons))
	for i, addon := range addons {
		names[i] = addon.Name()
	}
	return names
}
