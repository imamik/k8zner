package helm

import (
	"bytes"
	"fmt"
	"text/template"

	"gopkg.in/yaml.v3"
)

// Values represents helm chart values as a map.
type Values map[string]any

// Merge combines multiple Values maps with later maps taking precedence.
func Merge(valueMaps ...Values) Values {
	result := make(Values)
	for _, m := range valueMaps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
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

// renderTemplate processes a Go template with the provided values.
func renderTemplate(name string, templateContent []byte, values Values) ([]byte, error) {
	tmpl, err := template.New(name).Parse(string(templateContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template %s: %w", name, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, values); err != nil {
		return nil, fmt.Errorf("failed to execute template %s: %w", name, err)
	}

	return buf.Bytes(), nil
}
