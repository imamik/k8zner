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
