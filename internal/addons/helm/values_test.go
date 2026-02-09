package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMerge(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			result := Merge(tt.input...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToYAML(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	t.Run("shallow merge - same as Merge", func(t *testing.T) {
		t.Parallel()
		result := DeepMerge(
			Values{"key1": "value1", "key2": "value2"},
			Values{"key2": "override", "key3": "value3"},
		)
		assert.Equal(t, "value1", result["key1"])
		assert.Equal(t, "override", result["key2"])
		assert.Equal(t, "value3", result["key3"])
	})

	t.Run("deep merge - nested maps", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
		result := DeepMerge(
			Values{"args": []string{"--flag1", "--flag2"}},
			Values{"args": []string{"--flag3"}},
		)
		assert.Equal(t, []string{"--flag3"}, result["args"])
	})

	t.Run("non-map values override maps", func(t *testing.T) {
		t.Parallel()
		result := DeepMerge(
			Values{"config": map[string]any{"key": "value"}},
			Values{"config": "simple string"},
		)
		assert.Equal(t, "simple string", result["config"])
	})

	t.Run("multiple merges", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
		result := DeepMerge(Values{}, Values{}, Values{})
		assert.Empty(t, result)
	})
}

func TestDeepMerge_RealWorldCSICase(t *testing.T) {
	t.Parallel()
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

func TestToMap(t *testing.T) {
	t.Parallel()
	t.Run("simple values", func(t *testing.T) {
		t.Parallel()
		v := Values{
			"string": "value",
			"int":    42,
			"bool":   true,
		}
		result := v.ToMap()
		assert.Equal(t, "value", result["string"])
		assert.Equal(t, 42, result["int"])
		assert.Equal(t, true, result["bool"])
	})

	t.Run("nested Values", func(t *testing.T) {
		t.Parallel()
		v := Values{
			"outer": Values{
				"inner": Values{
					"value": "deep",
				},
			},
		}
		result := v.ToMap()

		outer, ok := result["outer"].(map[string]interface{})
		require.True(t, ok, "outer should be map[string]interface{}")

		inner, ok := outer["inner"].(map[string]interface{})
		require.True(t, ok, "inner should be map[string]interface{}")

		assert.Equal(t, "deep", inner["value"])
	})

	t.Run("nested map[string]any", func(t *testing.T) {
		t.Parallel()
		v := Values{
			"outer": map[string]any{
				"inner": map[string]any{
					"value": "deep",
				},
			},
		}
		result := v.ToMap()

		outer, ok := result["outer"].(map[string]interface{})
		require.True(t, ok, "outer should be map[string]interface{}")

		inner, ok := outer["inner"].(map[string]interface{})
		require.True(t, ok, "inner should be map[string]interface{}")

		assert.Equal(t, "deep", inner["value"])
	})

	t.Run("arrays with nested maps", func(t *testing.T) {
		t.Parallel()
		v := Values{
			"items": []any{
				Values{"name": "item1"},
				map[string]any{"name": "item2"},
				"plain string",
			},
		}
		result := v.ToMap()

		items, ok := result["items"].([]any)
		require.True(t, ok, "items should be []any")
		require.Len(t, items, 3)

		item1, ok := items[0].(map[string]interface{})
		require.True(t, ok, "item1 should be map[string]interface{}")
		assert.Equal(t, "item1", item1["name"])

		item2, ok := items[1].(map[string]interface{})
		require.True(t, ok, "item2 should be map[string]interface{}")
		assert.Equal(t, "item2", item2["name"])

		assert.Equal(t, "plain string", items[2])
	})

	t.Run("empty values", func(t *testing.T) {
		t.Parallel()
		v := Values{}
		result := v.ToMap()
		assert.Empty(t, result)
	})
}

func TestToValuesMap(t *testing.T) {
	t.Parallel()
	t.Run("Values type", func(t *testing.T) {
		t.Parallel()
		v := Values{"key": "value"}
		result := toValuesMap(v)
		assert.Equal(t, Values{"key": "value"}, result)
	})

	t.Run("map[string]any type", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{"key": "value"}
		result := toValuesMap(m)
		assert.Equal(t, Values{"key": "value"}, result)
	})

	t.Run("non-map type returns nil", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, toValuesMap("string"))
		assert.Nil(t, toValuesMap(42))
		assert.Nil(t, toValuesMap([]string{"a", "b"}))
		assert.Nil(t, toValuesMap(nil))
	})
}

func TestFromYAML_Errors(t *testing.T) {
	t.Parallel()
	t.Run("invalid yaml - tabs in content", func(t *testing.T) {
		t.Parallel()
		// YAML doesn't allow tabs for indentation - this should fail

		invalidYAML := []byte("key:\n\t- invalid")
		_, err := FromYAML(invalidYAML)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse YAML")
	})

	t.Run("invalid yaml - unclosed bracket", func(t *testing.T) {
		t.Parallel()
		invalidYAML := []byte("key: [unclosed")
		_, err := FromYAML(invalidYAML)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse YAML")
	})
}

func TestConvertToInterface_SliceTypes(t *testing.T) {
	t.Parallel()
	t.Run("[]Values slice", func(t *testing.T) {
		t.Parallel()
		input := []Values{
			{"name": "item1", "value": 1},
			{"name": "item2", "value": 2},
		}
		result := convertToInterface(input)

		arr, ok := result.([]any)
		require.True(t, ok, "result should be []any")
		require.Len(t, arr, 2)

		item1, ok := arr[0].(map[string]interface{})
		require.True(t, ok, "item should be map[string]interface{}")
		assert.Equal(t, "item1", item1["name"])
		assert.Equal(t, 1, item1["value"])
	})

	t.Run("[]map[string]any slice", func(t *testing.T) {
		t.Parallel()
		input := []map[string]any{
			{"name": "item1", "value": 1},
			{"name": "item2", "value": 2},
		}
		result := convertToInterface(input)

		arr, ok := result.([]any)
		require.True(t, ok, "result should be []any")
		require.Len(t, arr, 2)

		item1, ok := arr[0].(map[string]interface{})
		require.True(t, ok, "item should be map[string]interface{}")
		assert.Equal(t, "item1", item1["name"])
	})

	t.Run("[]string slice", func(t *testing.T) {
		t.Parallel()
		input := []string{"a", "b", "c"}
		result := convertToInterface(input)

		arr, ok := result.([]any)
		require.True(t, ok, "result should be []any")
		require.Len(t, arr, 3)
		assert.Equal(t, "a", arr[0])
		assert.Equal(t, "b", arr[1])
		assert.Equal(t, "c", arr[2])
	})

	t.Run("[]int slice", func(t *testing.T) {
		t.Parallel()
		input := []int{1, 2, 3}
		result := convertToInterface(input)

		arr, ok := result.([]any)
		require.True(t, ok, "result should be []any")
		require.Len(t, arr, 3)
		assert.Equal(t, 1, arr[0])
		assert.Equal(t, 2, arr[1])
		assert.Equal(t, 3, arr[2])
	})

	t.Run("default case - primitives pass through", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "string", convertToInterface("string"))
		assert.Equal(t, 42, convertToInterface(42))
		assert.Equal(t, true, convertToInterface(true))
		assert.Equal(t, 3.14, convertToInterface(3.14))
		assert.Nil(t, convertToInterface(nil))
	})
}

func TestMergeCustomValues(t *testing.T) {
	t.Parallel()
	t.Run("nil custom values returns base unchanged", func(t *testing.T) {
		t.Parallel()
		base := Values{"replicas": 2, "image": "nginx"}
		result := MergeCustomValues(base, nil)
		assert.Equal(t, base, result)
	})

	t.Run("empty custom values returns base unchanged", func(t *testing.T) {
		t.Parallel()
		base := Values{"replicas": 2, "image": "nginx"}
		result := MergeCustomValues(base, map[string]any{})
		assert.Equal(t, base, result)
	})

	t.Run("custom values override base values", func(t *testing.T) {
		t.Parallel()
		base := Values{"replicas": 2, "image": "nginx"}
		custom := map[string]any{"replicas": 5}
		result := MergeCustomValues(base, custom)
		assert.Equal(t, 5, result["replicas"])
		assert.Equal(t, "nginx", result["image"])
	})

	t.Run("deep merge with nested custom values", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
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
