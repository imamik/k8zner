package k8sclient

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

// Client provides Kubernetes operations for addon installation.
type Client interface {
	// ApplyManifests applies multi-document YAML using Server-Side Apply.
	// The fieldManager identifies the actor applying the configuration.
	ApplyManifests(ctx context.Context, manifests []byte, fieldManager string) error

	// CreateSecret creates or replaces a secret in the specified namespace.
	// If the secret already exists, it will be deleted and recreated.
	CreateSecret(ctx context.Context, secret *corev1.Secret) error

	// DeleteSecret deletes a secret, returning nil if not found.
	DeleteSecret(ctx context.Context, namespace, name string) error
}

// client implements the Client interface using k8s.io/client-go.
type client struct {
	clientset     kubernetes.Interface
	dynamicClient dynamic.Interface
	mapper        meta.RESTMapper
}

// NewFromKubeconfig creates a Client from kubeconfig bytes.
// This avoids the need to write kubeconfig to a temporary file.
func NewFromKubeconfig(kubeconfig []byte) (Client, error) {
	// Create REST config directly from kubeconfig bytes
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create REST config from kubeconfig: %w", err)
	}

	// Create typed clientset for secrets and other typed operations
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	// Create dynamic client for applying arbitrary manifests
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	// Create discovery client for REST mapping
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %w", err)
	}

	// Create REST mapper for GVK to GVR conversion
	groupResources, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get API group resources: %w", err)
	}
	mapper := restmapper.NewDiscoveryRESTMapper(groupResources)

	return &client{
		clientset:     clientset,
		dynamicClient: dynamicClient,
		mapper:        mapper,
	}, nil
}

// NewFromClients creates a Client from pre-configured clients.
// This is useful for testing with fake clients.
func NewFromClients(
	clientset kubernetes.Interface,
	dynamicClient dynamic.Interface,
	mapper meta.RESTMapper,
) Client {
	return &client{
		clientset:     clientset,
		dynamicClient: dynamicClient,
		mapper:        mapper,
	}
}
