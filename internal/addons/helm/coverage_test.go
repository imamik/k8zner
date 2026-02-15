package helm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetCachePath_HomeDirFallback tests the fallback path when HOME is unset
// and os.UserHomeDir() may fail. On most Linux systems with /etc/passwd, this
// path is not reachable, but we test the behavior by unsetting HOME.
func TestGetCachePath_HomeDirFallback(t *testing.T) {
	// Unset both XDG_CACHE_HOME and HOME
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")

	path := getCachePath()

	// On Linux, os.UserHomeDir() may still succeed via /etc/passwd lookup.
	// Either way, the path should end with k8zner/charts and be valid.
	assert.True(t, len(path) > 0, "getCachePath should always return a non-empty path")
	assert.Contains(t, path, filepath.Join("k8zner", "charts"))
}

// TestLoadChartFromPath_InvalidPath tests loadChartFromPath with a non-existent path.
func TestLoadChartFromPath_InvalidPath(t *testing.T) {
	t.Parallel()

	_, err := loadChartFromPath("/nonexistent/chart/path")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load chart from")
}

// TestLoadChartFromPath_InvalidArchive tests loadChartFromPath with an invalid file.
func TestLoadChartFromPath_InvalidArchive(t *testing.T) {
	t.Parallel()

	// Create a temporary file that is not a valid helm chart
	tmpDir := t.TempDir()
	fakePath := filepath.Join(tmpDir, "fake-chart.tgz")
	require.NoError(t, os.WriteFile(fakePath, []byte("not a real chart"), 0600))

	_, err := loadChartFromPath(fakePath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load chart from")
}

// TestClearCache_WithExistingContent tests that clearCache removes real content.
func TestClearCache_WithExistingContent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	// Create nested structure in cache
	cachePath := getCachePath()
	require.NoError(t, os.MkdirAll(cachePath, 0750))

	file1 := filepath.Join(cachePath, "chart-a-1.0.0.tgz")
	file2 := filepath.Join(cachePath, "chart-b-2.0.0.tgz")
	require.NoError(t, os.WriteFile(file1, []byte("chart-a"), 0600))
	require.NoError(t, os.WriteFile(file2, []byte("chart-b"), 0600))

	err := clearCache()
	require.NoError(t, err)

	// Verify the directory is gone
	_, err = os.Stat(cachePath)
	assert.True(t, os.IsNotExist(err), "cache directory should be removed")
}

// TestToYAML_EmptyValues tests toYAML with empty values.
func TestToYAML_EmptyValues(t *testing.T) {
	t.Parallel()
	v := Values{}
	yaml, err := v.toYAML()
	require.NoError(t, err)
	// yaml.v3 encodes empty maps as "{}\n"
	assert.Contains(t, string(yaml), "{}")
}

// TestToYAML_NestedValues tests toYAML with nested Values.
func TestToYAML_NestedValues(t *testing.T) {
	t.Parallel()
	v := Values{
		"deployment": Values{
			"replicas": 3,
			"image": Values{
				"repository": "nginx",
				"tag":        "latest",
			},
		},
		"service": Values{
			"type": "ClusterIP",
			"port": 80,
		},
	}

	yaml, err := v.toYAML()
	require.NoError(t, err)
	yamlStr := string(yaml)
	assert.Contains(t, yamlStr, "replicas: 3")
	assert.Contains(t, yamlStr, "repository: nginx")
	assert.Contains(t, yamlStr, "type: ClusterIP")
}

// TestFromYAML_EmptyInput tests fromYAML with empty byte slice.
func TestFromYAML_EmptyInput(t *testing.T) {
	t.Parallel()
	values, err := fromYAML([]byte(""))
	require.NoError(t, err)
	assert.Nil(t, values)
}

// TestToYAML_NilValue tests toYAML with a nil value inside the map.
func TestToYAML_NilValue(t *testing.T) {
	t.Parallel()
	v := Values{
		"key":     "value",
		"nullKey": nil,
	}
	yaml, err := v.toYAML()
	require.NoError(t, err)
	assert.Contains(t, string(yaml), "key: value")
	assert.Contains(t, string(yaml), "nullKey: null")
}
