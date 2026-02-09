package addons

import (
	"bytes"
	"fmt"
	"io"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	sigsyaml "sigs.k8s.io/yaml"
)

// patchDeploymentDNSPolicy sets dnsPolicy on a named Deployment in rendered Helm YAML.
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

// patchHostNetworkAPIAccess injects Kubernetes API env vars into containers for hostNetwork pods.
func patchHostNetworkAPIAccess(manifests []byte, resourceName string) ([]byte, error) {
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

		if len(raw.Object) == 0 {
			continue
		}

		kind := raw.GetKind()
		if (kind == "DaemonSet" || kind == "Deployment") && raw.GetName() == resourceName {
			containers, found, err := unstructured.NestedSlice(raw.Object, "spec", "template", "spec", "containers")
			if err != nil || !found || len(containers) == 0 {
				return nil, fmt.Errorf("no containers found in %s/%s", kind, resourceName)
			}

			for i, c := range containers {
				container, ok := c.(map[string]interface{})
				if !ok {
					continue
				}

				// Remove existing entries to avoid duplicates (upstream manifests
				// may already define these vars, e.g., Talos CCM).
				env, _, _ := unstructured.NestedSlice(container, "env")
				filtered := make([]interface{}, 0, len(env)+2)
				for _, e := range env {
					if em, ok := e.(map[string]interface{}); ok {
						name, _ := em["name"].(string)
						if name == "KUBERNETES_SERVICE_HOST" || name == "KUBERNETES_SERVICE_PORT" {
							continue
						}
					}
					filtered = append(filtered, e)
				}
				filtered = append(filtered,
					map[string]interface{}{"name": "KUBERNETES_SERVICE_HOST", "value": "localhost"},
					map[string]interface{}{"name": "KUBERNETES_SERVICE_PORT", "value": "6443"},
				)
				container["env"] = filtered
				containers[i] = container
			}

			if err := unstructured.SetNestedSlice(raw.Object, containers, "spec", "template", "spec", "containers"); err != nil {
				return nil, fmt.Errorf("failed to set containers on %s/%s: %w", kind, resourceName, err)
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
		return nil, fmt.Errorf("%s not found in manifests", resourceName)
	}

	var buf bytes.Buffer
	for i, doc := range docs {
		if i > 0 {
			buf.WriteString("---\n")
		}
		buf.Write(doc)
	}

	return buf.Bytes(), nil
}
