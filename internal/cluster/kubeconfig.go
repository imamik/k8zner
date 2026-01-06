package cluster

import (
	"context"
	"fmt"
	"log"
	"os"
)

// KubeconfigExporter handles exporting the kubeconfig after cluster bootstrap.
type KubeconfigExporter struct {
	talosGenerator TalosConfigProducer
	outputPath     string
}

// NewKubeconfigExporter creates a new kubeconfig exporter.
func NewKubeconfigExporter(talosGenerator TalosConfigProducer, outputPath string) *KubeconfigExporter {
	return &KubeconfigExporter{
		talosGenerator: talosGenerator,
		outputPath:     outputPath,
	}
}

// Export retrieves the kubeconfig from Talos and saves it to a file.
func (e *KubeconfigExporter) Export(ctx context.Context) error {
	log.Println("Exporting kubeconfig...")

	// Get kubeconfig from Talos
	kubeconfig, err := e.talosGenerator.GetKubeconfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to retrieve kubeconfig: %w", err)
	}

	// Write to file with secure permissions (0600)
	if err := os.WriteFile(e.outputPath, kubeconfig, 0600); err != nil {
		return fmt.Errorf("failed to write kubeconfig to %s: %w", e.outputPath, err)
	}

	log.Printf("✓ Kubeconfig exported to: %s", e.outputPath)
	return nil
}

// GetOutputPath returns the configured output path.
func (e *KubeconfigExporter) GetOutputPath() string {
	return e.outputPath
}
