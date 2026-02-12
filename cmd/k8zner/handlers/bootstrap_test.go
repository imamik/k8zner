package handlers

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/imamik/k8zner/internal/config"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
)

// --- writeTalosFiles tests ---

func TestWriteTalosFiles(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		origWrite := writeFile
		defer func() { writeFile = origWrite }()

		var writtenPath string
		var writtenPerm os.FileMode
		writeFile = func(path string, _ []byte, perm os.FileMode) error {
			writtenPath = path
			writtenPerm = perm
			return nil
		}

		mock := &mockTalosProducer{clientConfig: []byte("talos-config-data")}
		err := writeTalosFiles(mock)
		require.NoError(t, err)
		assert.Equal(t, talosConfigPath, writtenPath)
		assert.Equal(t, os.FileMode(0600), writtenPerm)
	})

	t.Run("GetClientConfig error", func(t *testing.T) {
		t.Parallel()
		mock := &mockTalosProducer{clientConfigErr: errors.New("generation failed")}
		err := writeTalosFiles(mock)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to generate talosconfig")
	})

	t.Run("writeFile error", func(t *testing.T) {
		t.Parallel()
		origWrite := writeFile
		defer func() { writeFile = origWrite }()

		writeFile = func(_ string, _ []byte, _ os.FileMode) error {
			return errors.New("permission denied")
		}

		mock := &mockTalosProducer{clientConfig: []byte("data")}
		err := writeTalosFiles(mock)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to write talosconfig")
	})
}

// --- writeKubeconfig tests ---

func TestWriteKubeconfig(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		origWrite := writeFile
		defer func() { writeFile = origWrite }()

		var writtenPath string
		var writtenData []byte
		writeFile = func(path string, data []byte, _ os.FileMode) error {
			writtenPath = path
			writtenData = data
			return nil
		}

		err := writeKubeconfig([]byte("kubeconfig-data"))
		require.NoError(t, err)
		assert.Equal(t, kubeconfigPath, writtenPath)
		assert.Equal(t, []byte("kubeconfig-data"), writtenData)
	})

	t.Run("empty kubeconfig skips write", func(t *testing.T) {
		t.Parallel()
		err := writeKubeconfig(nil)
		require.NoError(t, err)

		err = writeKubeconfig([]byte{})
		require.NoError(t, err)
	})

	t.Run("writeFile error", func(t *testing.T) {
		t.Parallel()
		origWrite := writeFile
		defer func() { writeFile = origWrite }()

		writeFile = func(_ string, _ []byte, _ os.FileMode) error {
			return errors.New("disk full")
		}

		err := writeKubeconfig([]byte("data"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to write kubeconfig")
	})
}

// --- initializeTalosGenerator tests ---

func TestInitializeTalosGenerator(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		origSecrets := getOrGenerateSecrets
		origTalos := newTalosGenerator
		defer func() {
			getOrGenerateSecrets = origSecrets
			newTalosGenerator = origTalos
		}()

		getOrGenerateSecrets = func(_, _ string) (*secrets.Bundle, error) {
			return &secrets.Bundle{}, nil
		}

		var capturedEndpoint string
		newTalosGenerator = func(_, _, _, endpoint string, _ *secrets.Bundle) provisioning.TalosConfigProducer {
			capturedEndpoint = endpoint
			return &mockTalosProducer{}
		}

		cfg := &config.Config{
			ClusterName: "my-cluster",
			Kubernetes:  config.KubernetesConfig{Version: "1.31.0"},
			Talos:       config.TalosConfig{Version: "1.8.3"},
		}

		gen, err := initializeTalosGenerator(cfg)
		require.NoError(t, err)
		assert.NotNil(t, gen)
		assert.Equal(t, "https://my-cluster-kube-api:6443", capturedEndpoint)
	})

	t.Run("secrets error", func(t *testing.T) {
		t.Parallel()
		origSecrets := getOrGenerateSecrets
		defer func() { getOrGenerateSecrets = origSecrets }()

		getOrGenerateSecrets = func(_, _ string) (*secrets.Bundle, error) {
			return nil, errors.New("cannot read secrets")
		}

		cfg := &config.Config{
			ClusterName: "test",
			Talos:       config.TalosConfig{Version: "1.8.3"},
		}

		gen, err := initializeTalosGenerator(cfg)
		require.Error(t, err)
		assert.Nil(t, gen)
		assert.Contains(t, err.Error(), "failed to initialize secrets")
	})
}

// --- printApplySuccess tests ---

func TestPrintApplySuccess(t *testing.T) {
	t.Run("without wait", func(t *testing.T) {
		cfg := &config.Config{ClusterName: "test"}
		output := captureOutput(func() {
			printApplySuccess(cfg, false)
		})
		assert.Contains(t, output, "Bootstrap complete!")
		assert.Contains(t, output, "Secrets saved to")
		assert.Contains(t, output, "Talos config saved to")
		assert.Contains(t, output, "Kubeconfig saved to")
		assert.Contains(t, output, "Access data saved to")
		assert.Contains(t, output, "operator is now provisioning")
		assert.Contains(t, output, "k8zner doctor --watch")
		assert.Contains(t, output, "kubectl get nodes")
	})

	t.Run("with wait", func(t *testing.T) {
		cfg := &config.Config{ClusterName: "test"}
		output := captureOutput(func() {
			printApplySuccess(cfg, true)
		})
		assert.Contains(t, output, "Bootstrap complete!")
		assert.NotContains(t, output, "operator is now provisioning")
	})
}

// --- cleanupOnFailure tests ---

func TestCleanupOnFailure(t *testing.T) {
	t.Parallel()

	t.Run("success with mock client", func(t *testing.T) {
		t.Parallel()
		cfg := &config.Config{ClusterName: "test"}
		mockClient := &hcloud.MockClient{}
		err := cleanupOnFailure(context.Background(), cfg, mockClient)
		require.NoError(t, err)
	})
}

func TestIsTransientError(t *testing.T) {
	t.Parallel()

	t.Run("recognizes EOF", func(t *testing.T) {
		t.Parallel()
		assert.True(t, isTransientError("unexpected EOF"))
	})

	t.Run("recognizes connection refused", func(t *testing.T) {
		t.Parallel()
		assert.True(t, isTransientError("dial tcp 10.0.0.1:6443: connection refused"))
	})

	t.Run("recognizes connection reset", func(t *testing.T) {
		t.Parallel()
		assert.True(t, isTransientError("read: connection reset by peer"))
	})

	t.Run("recognizes i/o timeout", func(t *testing.T) {
		t.Parallel()
		assert.True(t, isTransientError("i/o timeout"))
	})

	t.Run("recognizes no such host", func(t *testing.T) {
		t.Parallel()
		assert.True(t, isTransientError("dial tcp: lookup foo.bar: no such host"))
	})

	t.Run("recognizes TLS handshake timeout", func(t *testing.T) {
		t.Parallel()
		assert.True(t, isTransientError("net/http: TLS handshake timeout"))
	})

	t.Run("recognizes context deadline exceeded", func(t *testing.T) {
		t.Parallel()
		assert.True(t, isTransientError("context deadline exceeded"))
	})

	t.Run("rejects unknown errors", func(t *testing.T) {
		t.Parallel()
		assert.False(t, isTransientError("permission denied"))
		assert.False(t, isTransientError("resource not found"))
		assert.False(t, isTransientError("invalid configuration"))
		assert.False(t, isTransientError(""))
	})
}
