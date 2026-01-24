package k8sclient

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// ApplyManifests applies multi-document YAML using Server-Side Apply.
// Each document in the YAML is parsed and applied separately.
// Empty documents are skipped.
func (c *client) ApplyManifests(ctx context.Context, manifests []byte, fieldManager string) error {
	// Create YAML decoder for multi-document parsing
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(manifests), 4096)

	docIndex := 0
	for {
		// Decode next document into unstructured object
		var obj unstructured.Unstructured
		if err := decoder.Decode(&obj); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to decode manifest document %d: %w", docIndex, err)
		}

		// Skip empty documents (common in multi-doc YAML)
		if len(obj.Object) == 0 {
			docIndex++
			continue
		}

		// Apply the object using Server-Side Apply
		if err := c.applyObject(ctx, &obj, fieldManager); err != nil {
			kind := obj.GetKind()
			name := obj.GetName()
			namespace := obj.GetNamespace()
			return fmt.Errorf("failed to apply %s %s/%s: %w", kind, namespace, name, err)
		}

		docIndex++
	}

	return nil
}

// applyObject applies a single unstructured object using Server-Side Apply.
func (c *client) applyObject(ctx context.Context, obj *unstructured.Unstructured, fieldManager string) error {
	// Get GVK from the object
	gvk := obj.GroupVersionKind()
	if gvk.Kind == "" {
		return fmt.Errorf("object has no kind set")
	}

	// Map GVK to GVR (Group/Version/Resource)
	mapping, err := c.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return fmt.Errorf("failed to get REST mapping for %v: %w", gvk, err)
	}

	// Get the resource interface (namespaced or cluster-scoped)
	var resourceInterface = c.dynamicClient.Resource(mapping.Resource)

	// Determine if resource is namespaced
	namespace := obj.GetNamespace()
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		if namespace == "" {
			namespace = "default"
		}
	}

	// Convert object to JSON for the patch
	data, err := obj.MarshalJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal object to JSON: %w", err)
	}

	// Apply using Server-Side Apply (Patch with ApplyPatchType)
	opts := metav1.PatchOptions{
		FieldManager: fieldManager,
	}

	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		_, err = resourceInterface.Namespace(namespace).Patch(
			ctx,
			obj.GetName(),
			types.ApplyPatchType,
			data,
			opts,
		)
	} else {
		_, err = resourceInterface.Patch(
			ctx,
			obj.GetName(),
			types.ApplyPatchType,
			data,
			opts,
		)
	}

	if err != nil {
		return fmt.Errorf("server-side apply failed: %w", err)
	}

	return nil
}
