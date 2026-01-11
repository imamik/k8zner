package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInMemoryRESTClientGetter(t *testing.T) {
	kubeconfig := []byte(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: test-token
`)

	getter := NewInMemoryRESTClientGetter(kubeconfig, "test-namespace")

	require.NotNil(t, getter)
	assert.Equal(t, kubeconfig, getter.kubeconfig)
	assert.Equal(t, "test-namespace", getter.namespace)
}

func TestInMemoryRESTClientGetter_ToRESTConfig(t *testing.T) {
	kubeconfig := []byte(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: test-token
`)

	getter := NewInMemoryRESTClientGetter(kubeconfig, "default")

	restConfig, err := getter.ToRESTConfig()
	require.NoError(t, err)
	require.NotNil(t, restConfig)

	assert.Equal(t, "https://127.0.0.1:6443", restConfig.Host)
	assert.Equal(t, "test-token", restConfig.BearerToken)
}

func TestInMemoryRESTClientGetter_ToRESTConfig_Caching(t *testing.T) {
	kubeconfig := []byte(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: test-token
`)

	getter := NewInMemoryRESTClientGetter(kubeconfig, "default")

	// First call should create the config
	config1, err := getter.ToRESTConfig()
	require.NoError(t, err)

	// Second call should return cached config
	config2, err := getter.ToRESTConfig()
	require.NoError(t, err)

	// Should be the same instance (cached)
	assert.Same(t, config1, config2)
}

func TestInMemoryRESTClientGetter_ToRawKubeConfigLoader(t *testing.T) {
	kubeconfig := []byte(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
  name: test-context
current-context: test-context
`)

	getter := NewInMemoryRESTClientGetter(kubeconfig, "default")

	loader := getter.ToRawKubeConfigLoader()
	require.NotNil(t, loader)

	namespace, _, err := loader.Namespace()
	require.NoError(t, err)
	// Should return default namespace from kubeconfig or empty
	assert.NotNil(t, namespace)
}

func TestInMemoryRESTClientGetter_InvalidKubeconfig(t *testing.T) {
	invalidKubeconfig := []byte(`not valid yaml: {{{{`)

	getter := NewInMemoryRESTClientGetter(invalidKubeconfig, "default")

	_, err := getter.ToRESTConfig()
	assert.Error(t, err)
}
