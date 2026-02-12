package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/imamik/k8zner/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// saveAndRestoreInitFactories saves and restores init factory functions.
func saveAndRestoreInitFactories(t *testing.T) {
	origFileExists := fileExists
	origRunV2Wizard := runV2Wizard
	origWriteV2Config := writeV2Config

	t.Cleanup(func() {
		fileExists = origFileExists
		runV2Wizard = origRunV2Wizard
		writeV2Config = origWriteV2Config
	})
}

func TestPrintWelcome(t *testing.T) {
	output := captureOutput(func() {
		printWelcome()
	})

	assert.Contains(t, output, "k8zner - Kubernetes on Hetzner Cloud")
	assert.Contains(t, output, "6 simple questions")
	assert.Contains(t, output, "fully expanded and explicit")
}

func TestPrintInitSuccess(t *testing.T) {
	t.Run("dev mode", func(t *testing.T) {
		cfg := &config.Spec{
			Name:   "test-cluster",
			Region: config.RegionFalkenstein,
			Mode:   config.ModeDev,
			Workers: config.WorkerSpec{
				Count: 2,
				Size:  config.SizeCX32,
			},
		}

		output := captureOutput(func() {
			printInitSuccess("k8zner.yaml", cfg)
		})

		assert.Contains(t, output, "Configuration saved!")
		assert.Contains(t, output, "k8zner.yaml")
		assert.Contains(t, output, "test-cluster")
		assert.Contains(t, output, "fsn1")
		assert.Contains(t, output, "dev")
		assert.Contains(t, output, "2 x cx32")
		assert.Contains(t, output, "Next Steps")
		assert.Contains(t, output, "HCLOUD_TOKEN")
		assert.Contains(t, output, "k8zner apply")
	})

	t.Run("ha mode", func(t *testing.T) {
		cfg := &config.Spec{
			Name:   "production",
			Region: config.RegionNuremberg,
			Mode:   config.ModeHA,
			Workers: config.WorkerSpec{
				Count: 3,
				Size:  config.SizeCX42,
			},
			Domain: "example.com",
		}

		output := captureOutput(func() {
			printInitSuccess("prod.yaml", cfg)
		})

		assert.Contains(t, output, "production")
		assert.Contains(t, output, "nbg1")
		assert.Contains(t, output, "ha")
		assert.Contains(t, output, "3 x cx42")
		assert.Contains(t, output, "example.com")
		// HA mode has 3 control planes (cx23 is the default type)
		assert.Contains(t, output, "3 x cx23")
	})

	t.Run("shows included features", func(t *testing.T) {
		cfg := &config.Spec{
			Name:   "test",
			Region: config.RegionFalkenstein,
			Mode:   config.ModeDev,
			Workers: config.WorkerSpec{
				Count: 1,
				Size:  config.SizeCX22,
			},
		}

		output := captureOutput(func() {
			printInitSuccess("k8zner.yaml", cfg)
		})

		assert.Contains(t, output, "Included Features")
		assert.Contains(t, output, "Talos Linux")
		assert.Contains(t, output, "Cilium CNI")
		assert.Contains(t, output, "Traefik Ingress")
		assert.Contains(t, output, "ArgoCD")
		assert.Contains(t, output, "cert-manager")
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

func TestInit_WithInjection(t *testing.T) {
	saveAndRestoreInitFactories(t)

	validResult := &config.WizardResult{
		Name:        "test-cluster",
		Region:      config.RegionFalkenstein,
		Mode:        config.ModeDev,
		WorkerCount: 2,
		WorkerSize:  config.SizeCX32,
	}

	t.Run("success flow - new file", func(t *testing.T) {
		fileExists = func(_ string) bool {
			return false
		}

		runV2Wizard = func(_ context.Context) (*config.WizardResult, error) {
			return validResult, nil
		}

		writeV2Config = func(_ *config.Spec, _ string) error {
			return nil
		}

		// Capture output to suppress printing
		_ = captureOutput(func() {
			err := Init(context.Background(), "output.yaml")
			require.NoError(t, err)
		})
	})

	t.Run("success flow - overwrites existing file with warning", func(t *testing.T) {
		fileExists = func(_ string) bool {
			return true
		}

		runV2Wizard = func(_ context.Context) (*config.WizardResult, error) {
			return validResult, nil
		}

		writeV2Config = func(_ *config.Spec, _ string) error {
			return nil
		}

		output := captureOutput(func() {
			err := Init(context.Background(), "existing.yaml")
			require.NoError(t, err)
		})

		// Should show warning about overwrite
		assert.Contains(t, output, "Warning")
		assert.Contains(t, output, "existing.yaml")
		assert.Contains(t, output, "overwritten")
	})

	t.Run("wizard error", func(t *testing.T) {
		fileExists = func(_ string) bool {
			return false
		}

		runV2Wizard = func(_ context.Context) (*config.WizardResult, error) {
			return nil, errors.New("user canceled")
		}

		_ = captureOutput(func() {
			err := Init(context.Background(), "output.yaml")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "wizard canceled")
		})
	})

	t.Run("write config error", func(t *testing.T) {
		fileExists = func(_ string) bool {
			return false
		}

		runV2Wizard = func(_ context.Context) (*config.WizardResult, error) {
			return validResult, nil
		}

		writeV2Config = func(_ *config.Spec, _ string) error {
			return errors.New("permission denied")
		}

		_ = captureOutput(func() {
			err := Init(context.Background(), "/readonly/output.yaml")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to write config")
		})
	})

	t.Run("converts wizard result to config", func(t *testing.T) {
		var capturedConfig *config.Spec

		fileExists = func(_ string) bool {
			return false
		}

		runV2Wizard = func(_ context.Context) (*config.WizardResult, error) {
			return &config.WizardResult{
				Name:        "my-cluster",
				Region:      config.RegionHelsinki,
				Mode:        config.ModeHA,
				WorkerCount: 5,
				WorkerSize:  config.SizeCX52,
				Domain:      "test.example.com",
			}, nil
		}

		writeV2Config = func(cfg *config.Spec, _ string) error {
			capturedConfig = cfg
			return nil
		}

		_ = captureOutput(func() {
			err := Init(context.Background(), "output.yaml")
			require.NoError(t, err)
		})

		require.NotNil(t, capturedConfig)
		assert.Equal(t, "my-cluster", capturedConfig.Name)
		assert.Equal(t, config.RegionHelsinki, capturedConfig.Region)
		assert.Equal(t, config.ModeHA, capturedConfig.Mode)
		assert.Equal(t, 5, capturedConfig.Workers.Count)
		assert.Equal(t, config.SizeCX52, capturedConfig.Workers.Size)
		assert.Equal(t, "test.example.com", capturedConfig.Domain)
	})
}
