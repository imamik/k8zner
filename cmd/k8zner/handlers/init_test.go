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
			printWelcome(false)
		})

		assert.Contains(t, output, "k8zner - Kubernetes on Hetzner Cloud")
		assert.Contains(t, output, "This wizard will help you create")
		assert.NotContains(t, output, "advanced mode")
	})

	t.Run("advanced mode", func(t *testing.T) {
		output := captureOutput(func() {
			printWelcome(true)
		})

		assert.Contains(t, output, "k8zner - Kubernetes on Hetzner Cloud")
		assert.Contains(t, output, "Running in advanced mode")
	})
}

func TestPrintInitSuccess(t *testing.T) {
	t.Run("with workers", func(t *testing.T) {
		result := &wizard.WizardResult{
			ClusterName:       "test-cluster",
			Location:          "nbg1",
			ControlPlaneType:  "cpx21",
			ControlPlaneCount: 3,
			AddWorkers:        true,
			WorkerType:        "cpx31",
			WorkerCount:       2,
			TalosVersion:      "v1.9.0",
			KubernetesVersion: "v1.32.0",
		}

		output := captureOutput(func() {
			printInitSuccess("cluster.yaml", result)
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
	})

	t.Run("without workers", func(t *testing.T) {
		result := &wizard.WizardResult{
			ClusterName:       "single-node",
			Location:          "fsn1",
			ControlPlaneType:  "cpx21",
			ControlPlaneCount: 1,
			AddWorkers:        false,
			TalosVersion:      "v1.9.0",
			KubernetesVersion: "v1.32.0",
		}

		output := captureOutput(func() {
			printInitSuccess("output.yaml", result)
		})

		assert.Contains(t, output, "single-node")
		assert.Contains(t, output, "1 x cpx21")
		assert.Contains(t, output, "None (workloads will run on control plane)")
		assert.NotContains(t, output, "Workers:         2")
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
		ControlPlaneType:  "cax21",
		ControlPlaneCount: 3,
		AddWorkers:        false,
		TalosVersion:      "v1.8.3",
		KubernetesVersion: "v1.31.0",
	}

	customPath := "/custom/path/config.yaml"
	output := captureOutput(func() {
		printInitSuccess(customPath, result)
	})

	// Verify output path appears in both the file location and the apply command
	assert.True(t, strings.Count(output, customPath) >= 2,
		"Output path should appear at least twice (file location and apply command)")
}
