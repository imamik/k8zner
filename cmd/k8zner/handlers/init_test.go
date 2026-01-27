package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/imamik/k8zner/internal/config"
	"github.com/imamik/k8zner/internal/config/wizard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// saveAndRestoreInitFactories saves and restores init factory functions.
func saveAndRestoreInitFactories(t *testing.T) {
	origFileExists := wizardFileExists
	origConfirmOverwrite := wizardConfirmOverwrite
	origRunWizard := wizardRunWizard
	origBuildConfig := wizardBuildConfig
	origWriteConfig := wizardWriteConfig

	t.Cleanup(func() {
		wizardFileExists = origFileExists
		wizardConfirmOverwrite = origConfirmOverwrite
		wizardRunWizard = origRunWizard
		wizardBuildConfig = origBuildConfig
		wizardWriteConfig = origWriteConfig
	})
}

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

	t.Run("with ssh keys", func(t *testing.T) {
		result := &wizard.WizardResult{
			ClusterName:       "test-cluster",
			Location:          "nbg1",
			Architecture:      wizard.ArchX86,
			ServerCategory:    wizard.CategoryShared,
			ControlPlaneType:  "cpx21",
			ControlPlaneCount: 1,
			AddWorkers:        false,
			CNIChoice:         wizard.CNICilium,
			TalosVersion:      "v1.9.0",
			KubernetesVersion: "v1.32.0",
			SSHKeys:           []string{"my-key-1", "my-key-2"},
		}

		output := captureOutput(func() {
			printInitSuccess("output.yaml", result, false)
		})

		assert.Contains(t, output, "my-key-1")
		assert.Contains(t, output, "my-key-2")
		assert.NotContains(t, output, "will be auto-generated")
	})

	t.Run("full output mode", func(t *testing.T) {
		result := &wizard.WizardResult{
			ClusterName:       "test-cluster",
			Location:          "nbg1",
			Architecture:      wizard.ArchX86,
			ServerCategory:    wizard.CategoryShared,
			ControlPlaneType:  "cpx21",
			ControlPlaneCount: 1,
			AddWorkers:        false,
			CNIChoice:         wizard.CNICilium,
			TalosVersion:      "v1.9.0",
			KubernetesVersion: "v1.32.0",
		}

		output := captureOutput(func() {
			printInitSuccess("output.yaml", result, true) // fullOutput = true
		})

		// Should NOT contain the minimal output hint when fullOutput is true
		assert.NotContains(t, output, "minimal output")
		assert.NotContains(t, output, "--full")
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

func TestInit_WithInjection(t *testing.T) {
	saveAndRestoreInitFactories(t)

	validResult := &wizard.WizardResult{
		ClusterName:       "test-cluster",
		Location:          "nbg1",
		Architecture:      wizard.ArchX86,
		ServerCategory:    wizard.CategoryShared,
		ControlPlaneType:  "cpx21",
		ControlPlaneCount: 3,
		AddWorkers:        false,
		CNIChoice:         wizard.CNICilium,
		TalosVersion:      "v1.9.0",
		KubernetesVersion: "v1.32.0",
	}

	t.Run("success flow - new file", func(t *testing.T) {
		wizardFileExists = func(_ string) bool {
			return false
		}

		wizardRunWizard = func(_ context.Context, _ bool) (*wizard.WizardResult, error) {
			return validResult, nil
		}

		wizardBuildConfig = func(_ *wizard.WizardResult) *config.Config {
			return &config.Config{ClusterName: "test-cluster"}
		}

		wizardWriteConfig = func(_ *config.Config, _ string, _ bool) error {
			return nil
		}

		// Capture output to suppress printing
		_ = captureOutput(func() {
			err := Init(context.Background(), "output.yaml", false, false)
			require.NoError(t, err)
		})
	})

	t.Run("success flow - overwrite confirmed", func(t *testing.T) {
		wizardFileExists = func(_ string) bool {
			return true
		}

		wizardConfirmOverwrite = func(_ string) (bool, error) {
			return true, nil
		}

		wizardRunWizard = func(_ context.Context, _ bool) (*wizard.WizardResult, error) {
			return validResult, nil
		}

		wizardBuildConfig = func(_ *wizard.WizardResult) *config.Config {
			return &config.Config{ClusterName: "test-cluster"}
		}

		wizardWriteConfig = func(_ *config.Config, _ string, _ bool) error {
			return nil
		}

		_ = captureOutput(func() {
			err := Init(context.Background(), "existing.yaml", false, false)
			require.NoError(t, err)
		})
	})

	t.Run("user aborts overwrite", func(t *testing.T) {
		wizardFileExists = func(_ string) bool {
			return true
		}

		wizardConfirmOverwrite = func(_ string) (bool, error) {
			return false, nil // User says no
		}

		output := captureOutput(func() {
			err := Init(context.Background(), "existing.yaml", false, false)
			require.NoError(t, err) // Abort is not an error
		})

		assert.Contains(t, output, "Aborted")
	})

	t.Run("confirm overwrite error", func(t *testing.T) {
		wizardFileExists = func(_ string) bool {
			return true
		}

		wizardConfirmOverwrite = func(_ string) (bool, error) {
			return false, errors.New("terminal not interactive")
		}

		_ = captureOutput(func() {
			err := Init(context.Background(), "existing.yaml", false, false)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to prompt for confirmation")
		})
	})

	t.Run("wizard error", func(t *testing.T) {
		wizardFileExists = func(_ string) bool {
			return false
		}

		wizardRunWizard = func(_ context.Context, _ bool) (*wizard.WizardResult, error) {
			return nil, errors.New("user cancelled")
		}

		_ = captureOutput(func() {
			err := Init(context.Background(), "output.yaml", false, false)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "wizard failed")
		})
	})

	t.Run("write config error", func(t *testing.T) {
		wizardFileExists = func(_ string) bool {
			return false
		}

		wizardRunWizard = func(_ context.Context, _ bool) (*wizard.WizardResult, error) {
			return validResult, nil
		}

		wizardBuildConfig = func(_ *wizard.WizardResult) *config.Config {
			return &config.Config{ClusterName: "test-cluster"}
		}

		wizardWriteConfig = func(_ *config.Config, _ string, _ bool) error {
			return errors.New("permission denied")
		}

		_ = captureOutput(func() {
			err := Init(context.Background(), "/readonly/output.yaml", false, false)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to write config")
		})
	})

	t.Run("advanced mode passes through", func(t *testing.T) {
		var capturedAdvanced bool

		wizardFileExists = func(_ string) bool {
			return false
		}

		wizardRunWizard = func(_ context.Context, advanced bool) (*wizard.WizardResult, error) {
			capturedAdvanced = advanced
			return validResult, nil
		}

		wizardBuildConfig = func(_ *wizard.WizardResult) *config.Config {
			return &config.Config{ClusterName: "test-cluster"}
		}

		wizardWriteConfig = func(_ *config.Config, _ string, _ bool) error {
			return nil
		}

		_ = captureOutput(func() {
			err := Init(context.Background(), "output.yaml", true, false)
			require.NoError(t, err)
		})

		assert.True(t, capturedAdvanced)
	})

	t.Run("full output mode passes through", func(t *testing.T) {
		var capturedFullOutput bool

		wizardFileExists = func(_ string) bool {
			return false
		}

		wizardRunWizard = func(_ context.Context, _ bool) (*wizard.WizardResult, error) {
			return validResult, nil
		}

		wizardBuildConfig = func(_ *wizard.WizardResult) *config.Config {
			return &config.Config{ClusterName: "test-cluster"}
		}

		wizardWriteConfig = func(_ *config.Config, _ string, fullOutput bool) error {
			capturedFullOutput = fullOutput
			return nil
		}

		_ = captureOutput(func() {
			err := Init(context.Background(), "output.yaml", false, true)
			require.NoError(t, err)
		})

		assert.True(t, capturedFullOutput)
	})
}
