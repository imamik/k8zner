package addons

import (
	"bytes"
	"fmt"
	"io"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	sigsyaml "sigs.k8s.io/yaml"
)

// patchDeploymentDNSPolicy modifies rendered Helm YAML to set dnsPolicy on
// a specific Deployment. This is a post-render patch for charts that don't
// expose dnsPolicy as a configurable value.
//
// It splits multi-doc YAML into individual documents, finds the Deployment
// matching deploymentName, sets spec.template.spec.dnsPolicy, and
// re-serializes all docs back to multi-doc YAML.
func patchDeploymentDNSPolicy(manifests []byte, deploymentName, dnsPolicy string) ([]byte, error) {
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(manifests), 4096)

	var docs [][]byte
	patched := false

	for {
		var raw unstructured.Unstructured
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to decode YAML document: %w", err)
		}

		// Skip empty documents
		if len(raw.Object) == 0 {
			continue
		}

		// Check if this is the target Deployment
		if raw.GetKind() == "Deployment" && raw.GetName() == deploymentName {
			if err := unstructured.SetNestedField(raw.Object, dnsPolicy, "spec", "template", "spec", "dnsPolicy"); err != nil {
				return nil, fmt.Errorf("failed to set dnsPolicy on %s: %w", deploymentName, err)
			}
			patched = true
		}

		out, err := sigsyaml.Marshal(raw.Object)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal YAML document: %w", err)
		}
		docs = append(docs, out)
	}

	if !patched {
		return nil, fmt.Errorf("deployment %q not found in manifests", deploymentName)
	}

	// Join documents with YAML document separator
	var buf bytes.Buffer
	for i, doc := range docs {
		if i > 0 {
			buf.WriteString("---\n")
		}
		buf.Write(doc)
	}

	return buf.Bytes(), nil
}
