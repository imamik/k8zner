package handlers

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/imamik/k8zner/internal/config/wizard"
	"github.com/stretchr/testify/assert"
)

func TestPrintWelcome(t *testing.T) {
	t.Run("basic mode", func(t *testing.T) {
		output := captureOutput(func() {
			printWelcome(false, false)
		})

		assert.Contains(t, output, "k8zner - Kubernetes on Hetzner Cloud")
		assert.Contains(t, output, "This wizard will help you create")
		assert.NotContains(t, output, "advanced mode")
		assert.Contains(t, output, "Minimal output mode")
	})

	t.Run("advanced mode", func(t *testing.T) {
		output := captureOutput(func() {
			printWelcome(true, false)
		})

		assert.Contains(t, output, "k8zner - Kubernetes on Hetzner Cloud")
		assert.Contains(t, output, "Running in advanced mode")
	})

	t.Run("full output mode", func(t *testing.T) {
		output := captureOutput(func() {
			printWelcome(false, true)
		})

		assert.Contains(t, output, "Full output mode")
	})
}

func TestPrintInitSuccess(t *testing.T) {
	t.Run("with workers", func(t *testing.T) {
		result := &wizard.WizardResult{
			ClusterName:       "test-cluster",
			Location:          "nbg1",
			Architecture:      wizard.ArchX86,
			ServerCategory:    wizard.CategoryShared,
			ControlPlaneType:  "cpx21",
			ControlPlaneCount: 3,
			AddWorkers:        true,
			WorkerType:        "cpx31",
			WorkerCount:       2,
			CNIChoice:         wizard.CNICilium,
			TalosVersion:      "v1.9.0",
			KubernetesVersion: "v1.32.0",
		}

		output := captureOutput(func() {
			printInitSuccess("cluster.yaml", result, false)
		})

		assert.Contains(t, output, "Configuration saved successfully")
		assert.Contains(t, output, "cluster.yaml")
		assert.Contains(t, output, "test-cluster")
		assert.Contains(t, output, "nbg1")
		assert.Contains(t, output, "3 x cpx21")
		assert.Contains(t, output, "2 x cpx31")
		assert.Contains(t, output, "v1.9.0")
		assert.Contains(t, output, "v1.32.0")
		assert.Contains(t, output, "k8zner apply")
		assert.Contains(t, output, "Cilium")
	})

	t.Run("without workers", func(t *testing.T) {
		result := &wizard.WizardResult{
			ClusterName:       "single-node",
			Location:          "fsn1",
			Architecture:      wizard.ArchARM,
			ServerCategory:    wizard.CategoryShared,
			ControlPlaneType:  "cpx21",
			ControlPlaneCount: 1,
			AddWorkers:        false,
			CNIChoice:         wizard.CNITalosNative,
			TalosVersion:      "v1.9.0",
			KubernetesVersion: "v1.32.0",
		}

		output := captureOutput(func() {
			printInitSuccess("output.yaml", result, false)
		})

		assert.Contains(t, output, "single-node")
		assert.Contains(t, output, "1 x cpx21")
		assert.Contains(t, output, "None (workloads will run on control plane)")
		assert.NotContains(t, output, "Workers:         2")
		assert.Contains(t, output, "Talos Default")
	})
}

// captureOutput captures stdout during function execution.
func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestPrintInitSuccess_OutputPath(t *testing.T) {
	result := &wizard.WizardResult{
		ClusterName:       "my-cluster",
		Location:          "hel1",
		Architecture:      wizard.ArchARM,
		ServerCategory:    wizard.CategoryShared,
		ControlPlaneType:  "cax21",
		ControlPlaneCount: 3,
		AddWorkers:        false,
		CNIChoice:         wizard.CNICilium,
		TalosVersion:      "v1.8.3",
		KubernetesVersion: "v1.31.0",
	}

	customPath := "/custom/path/config.yaml"
	output := captureOutput(func() {
		printInitSuccess(customPath, result, false)
	})

	// Verify output path appears in both the file location and the apply command
	assert.True(t, strings.Count(output, customPath) >= 2,
		"Output path should appear at least twice (file location and apply command)")
}

func TestFormatCNIChoice(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "cilium",
			input:    wizard.CNICilium,
			expected: "Cilium",
		},
		{
			name:     "talos native",
			input:    wizard.CNITalosNative,
			expected: "Talos Default (Flannel)",
		},
		{
			name:     "none",
			input:    wizard.CNINone,
			expected: "None (user-managed)",
		},
		{
			name:     "unknown returns as-is",
			input:    "custom-cni",
			expected: "custom-cni",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatCNIChoice(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
