package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMerge(t *testing.T) {
	tests := []struct {
		name     string
		input    []Values
		expected Values
	}{
		{
			name: "merge two maps",
			input: []Values{
				{"key1": "value1", "key2": "value2"},
				{"key2": "override", "key3": "value3"},
			},
			expected: Values{"key1": "value1", "key2": "override", "key3": "value3"},
		},
		{
			name:     "merge empty maps",
			input:    []Values{{}, {}},
			expected: Values{},
		},
		{
			name: "later maps take precedence",
			input: []Values{
				{"replicas": 1},
				{"replicas": 2},
				{"replicas": 3},
			},
			expected: Values{"replicas": 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Merge(tt.input...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToYAML(t *testing.T) {
	values := Values{
		"replicas": 2,
		"image": Values{
			"repository": "metrics-server",
			"tag":        "v0.7.2",
		},
	}

	yaml, err := values.ToYAML()
	require.NoError(t, err)
	assert.Contains(t, string(yaml), "replicas: 2")
	assert.Contains(t, string(yaml), "repository: metrics-server")
	assert.Contains(t, string(yaml), "tag: v0.7.2")
}

func TestFromYAML(t *testing.T) {
	yamlData := []byte(`
replicas: 2
nodeSelector:
  node-role.kubernetes.io/control-plane: ""
`)

	values, err := FromYAML(yamlData)
	require.NoError(t, err)
	assert.Equal(t, 2, values["replicas"])
	assert.NotNil(t, values["nodeSelector"])
}

func TestDeepMerge(t *testing.T) {
	t.Run("shallow merge - same as Merge", func(t *testing.T) {
		result := DeepMerge(
			Values{"key1": "value1", "key2": "value2"},
			Values{"key2": "override", "key3": "value3"},
		)
		assert.Equal(t, "value1", result["key1"])
		assert.Equal(t, "override", result["key2"])
		assert.Equal(t, "value3", result["key3"])
	})

	t.Run("deep merge - nested maps", func(t *testing.T) {
		result := DeepMerge(
			Values{
				"controller": map[string]any{
					"replicas": 1,
					"podSecurityContext": map[string]any{
						"enabled": true,
						"fsGroup": 1001,
					},
				},
			},
			Values{
				"controller": map[string]any{
					"replicas": 2,
					"nodeSelector": map[string]any{
						"node-role.kubernetes.io/control-plane": "",
					},
				},
			},
		)

		controller := toValuesMap(result["controller"])
		require.NotNil(t, controller)
		assert.Equal(t, 2, controller["replicas"])

		podSec := toValuesMap(controller["podSecurityContext"])
		require.NotNil(t, podSec, "podSecurityContext should be preserved")
		assert.Equal(t, true, podSec["enabled"])
		assert.Equal(t, 1001, podSec["fsGroup"])

		nodeSelector := toValuesMap(controller["nodeSelector"])
		require.NotNil(t, nodeSelector)
		assert.Equal(t, "", nodeSelector["node-role.kubernetes.io/control-plane"])
	})

	t.Run("deep merge - three levels deep", func(t *testing.T) {
		result := DeepMerge(
			Values{
				"controller": map[string]any{
					"image": map[string]any{
						"repository": "hcloud-csi",
						"tag":        "v2.0.0",
						"pullPolicy": "IfNotPresent",
					},
					"replicas": 1,
				},
			},
			Values{
				"controller": map[string]any{
					"image": map[string]any{
						"tag": "v2.1.0",
					},
					"nodeSelector": map[string]any{
						"disk": "ssd",
					},
				},
			},
		)

		controller := toValuesMap(result["controller"])
		require.NotNil(t, controller)
		assert.Equal(t, 1, controller["replicas"])

		image := toValuesMap(controller["image"])
		require.NotNil(t, image)
		assert.Equal(t, "hcloud-csi", image["repository"], "repository should be preserved")
		assert.Equal(t, "v2.1.0", image["tag"], "tag should be overridden")
		assert.Equal(t, "IfNotPresent", image["pullPolicy"], "pullPolicy should be preserved")
	})

	t.Run("arrays are replaced not merged", func(t *testing.T) {
		result := DeepMerge(
			Values{"args": []string{"--flag1", "--flag2"}},
			Values{"args": []string{"--flag3"}},
		)
		assert.Equal(t, []string{"--flag3"}, result["args"])
	})

	t.Run("non-map values override maps", func(t *testing.T) {
		result := DeepMerge(
			Values{"config": map[string]any{"key": "value"}},
			Values{"config": "simple string"},
		)
		assert.Equal(t, "simple string", result["config"])
	})

	t.Run("multiple merges", func(t *testing.T) {
		result := DeepMerge(
			Values{"a": map[string]any{"x": 1}},
			Values{"a": map[string]any{"y": 2}},
			Values{"a": map[string]any{"z": 3}},
		)

		a := toValuesMap(result["a"])
		require.NotNil(t, a)
		assert.Equal(t, 1, a["x"], "x should be preserved from first merge")
		assert.Equal(t, 2, a["y"], "y should be preserved from second merge")
		assert.Equal(t, 3, a["z"], "z should be added from third merge")
	})

	t.Run("empty maps", func(t *testing.T) {
		result := DeepMerge(Values{}, Values{}, Values{})
		assert.Empty(t, result)
	})
}

func TestDeepMerge_RealWorldCSICase(t *testing.T) {
	// This simulates the actual CSI addon scenario where chart defaults
	// contain podSecurityContext but our custom values don't

	chartDefaults := Values{
		"controller": map[string]any{
			"replicas": 1,
			"podSecurityContext": map[string]any{
				"enabled": true,
				"fsGroup": 1001,
			},
			"image": map[string]any{
				"repository": "registry.k8s.io/sig-storage/csi-attacher",
				"tag":        "v4.10.0",
			},
		},
	}

	customValues := Values{
		"controller": map[string]any{
			"replicas": 2,
			"hcloudToken": map[string]any{
				"existingSecret": map[string]any{
					"name": "hcloud",
					"key":  "token",
				},
			},
		},
	}

	result := DeepMerge(chartDefaults, customValues)

	// Check that replicas was overridden
	controller := toValuesMap(result["controller"])
	require.NotNil(t, controller)
	assert.Equal(t, 2, controller["replicas"], "replicas should be overridden to 2")

	// Check that podSecurityContext was preserved from defaults
	podSecurityContext := toValuesMap(controller["podSecurityContext"])
	require.NotNil(t, podSecurityContext, "podSecurityContext must be preserved from chart defaults")
	assert.Equal(t, true, podSecurityContext["enabled"])
	assert.Equal(t, 1001, podSecurityContext["fsGroup"])

	// Check that image was preserved from defaults
	image := toValuesMap(controller["image"])
	require.NotNil(t, image, "image config must be preserved from chart defaults")
	assert.Equal(t, "registry.k8s.io/sig-storage/csi-attacher", image["repository"])
	assert.Equal(t, "v4.10.0", image["tag"])

	// Check that hcloudToken was added from custom values
	hcloudToken := toValuesMap(controller["hcloudToken"])
	require.NotNil(t, hcloudToken, "hcloudToken must be added from custom values")
	existingSecret := toValuesMap(hcloudToken["existingSecret"])
	require.NotNil(t, existingSecret)
	assert.Equal(t, "hcloud", existingSecret["name"])
	assert.Equal(t, "token", existingSecret["key"])
}

func TestMergeCustomValues(t *testing.T) {
	t.Run("nil custom values returns base unchanged", func(t *testing.T) {
		base := Values{"replicas": 2, "image": "nginx"}
		result := MergeCustomValues(base, nil)
		assert.Equal(t, base, result)
	})

	t.Run("empty custom values returns base unchanged", func(t *testing.T) {
		base := Values{"replicas": 2, "image": "nginx"}
		result := MergeCustomValues(base, map[string]any{})
		assert.Equal(t, base, result)
	})

	t.Run("custom values override base values", func(t *testing.T) {
		base := Values{"replicas": 2, "image": "nginx"}
		custom := map[string]any{"replicas": 5}
		result := MergeCustomValues(base, custom)
		assert.Equal(t, 5, result["replicas"])
		assert.Equal(t, "nginx", result["image"])
	})

	t.Run("deep merge with nested custom values", func(t *testing.T) {
		base := Values{
			"controller": Values{
				"replicas": 2,
				"config": Values{
					"setting1": "value1",
					"setting2": "value2",
				},
			},
		}
		custom := map[string]any{
			"controller": map[string]any{
				"config": map[string]any{
					"setting2": "override",
					"setting3": "new",
				},
			},
		}
		result := MergeCustomValues(base, custom)

		controller := toValuesMap(result["controller"])
		require.NotNil(t, controller)
		assert.Equal(t, 2, controller["replicas"], "replicas should be preserved")

		config := toValuesMap(controller["config"])
		require.NotNil(t, config)
		assert.Equal(t, "value1", config["setting1"], "setting1 should be preserved")
		assert.Equal(t, "override", config["setting2"], "setting2 should be overridden")
		assert.Equal(t, "new", config["setting3"], "setting3 should be added")
	})

	t.Run("real-world helm override scenario", func(t *testing.T) {
		// Simulates user overriding Cilium helm values
		base := Values{
			"ipam": Values{"mode": "kubernetes"},
			"operator": Values{
				"replicas": 1,
				"nodeSelector": Values{
					"node-role.kubernetes.io/control-plane": "",
				},
			},
		}
		custom := map[string]any{
			"operator": map[string]any{
				"replicas": 3,
				"resources": map[string]any{
					"limits": map[string]any{
						"memory": "512Mi",
					},
				},
			},
		}
		result := MergeCustomValues(base, custom)

		// Check ipam preserved
		ipam := toValuesMap(result["ipam"])
		require.NotNil(t, ipam)
		assert.Equal(t, "kubernetes", ipam["mode"])

		// Check operator merged
		operator := toValuesMap(result["operator"])
		require.NotNil(t, operator)
		assert.Equal(t, 3, operator["replicas"], "replicas should be overridden")

		nodeSelector := toValuesMap(operator["nodeSelector"])
		require.NotNil(t, nodeSelector, "nodeSelector should be preserved")
		assert.Equal(t, "", nodeSelector["node-role.kubernetes.io/control-plane"])

		resources := toValuesMap(operator["resources"])
		require.NotNil(t, resources, "resources should be added")
	})
}
