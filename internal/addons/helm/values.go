package helm

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Values represents helm chart values as a map.
type Values map[string]any

// Merge combines multiple Values maps with later maps taking precedence.
// This performs a shallow merge - top-level keys are replaced entirely.
func Merge(valueMaps ...Values) Values {
	result := make(Values)
	for _, m := range valueMaps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

// DeepMerge recursively merges multiple Values maps with later maps taking precedence.
// Unlike Merge, this preserves nested structures by recursively merging maps.
// Non-map values and arrays are replaced (not merged).
func DeepMerge(valueMaps ...Values) Values {
	result := make(Values)
	for _, m := range valueMaps {
		result = deepMergeTwoMaps(result, m)
	}
	return result
}

// deepMergeTwoMaps recursively merges two Values maps.
func deepMergeTwoMaps(base, override Values) Values {
	result := make(Values)

	// Copy all keys from base
	for k, v := range base {
		result[k] = v
	}

	// Merge or replace with override values
	for k, v := range override {
		if existingVal, exists := result[k]; exists {
			// Try to merge if both are map-like structures
			// Handle both Values type and map[string]any type
			existingMap := toValuesMap(existingVal)
			overrideMap := toValuesMap(v)

			if existingMap != nil && overrideMap != nil {
				result[k] = deepMergeTwoMaps(existingMap, overrideMap)
				continue
			}
		}
		// For non-map values or if key doesn't exist, replace/set the value
		result[k] = v
	}

	return result
}

// toValuesMap converts various map types to Values.
// Returns nil if the value is not a map.
func toValuesMap(v any) Values {
	switch m := v.(type) {
	case Values:
		return m
	case map[string]any:
		return Values(m)
	default:
		return nil
	}
}

// ToYAML converts values to YAML bytes.
func (v Values) ToYAML() ([]byte, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)

	if err := encoder.Encode(v); err != nil {
		return nil, fmt.Errorf("failed to encode values to YAML: %w", err)
	}

	return buf.Bytes(), nil
}

// FromYAML parses YAML bytes into Values.
func FromYAML(data []byte) (Values, error) {
	var values Values
	if err := yaml.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("failed to parse YAML values: %w", err)
	}
	return values, nil
}
