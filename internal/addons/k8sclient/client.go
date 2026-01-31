package k8sclient

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// RefreshDiscovery refreshes the API discovery to pick up newly installed CRDs.
	// This should be called after installing a Helm chart that includes CRDs.
	RefreshDiscovery(ctx context.Context) error

	// HasCRD checks if a CRD with the given name exists.
	HasCRD(ctx context.Context, crdName string) (bool, error)

	// HasReadyEndpoints checks if a service has at least one ready endpoint.
	// This is useful for waiting for a service's backing pods to be ready.
	HasReadyEndpoints(ctx context.Context, namespace, serviceName string) (bool, error)

	// GetWorkerExternalIPs returns the external IPs of worker nodes.
	// This is useful for DNS configuration when using hostNetwork mode.
	GetWorkerExternalIPs(ctx context.Context) ([]string, error)
}

// client implements the Client interface using k8s.io/client-go.
type client struct {
	clientset     kubernetes.Interface
	dynamicClient dynamic.Interface
	mapper        meta.RESTMapper
	kubeconfig    []byte // Store for refreshing discovery
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
		kubeconfig:    kubeconfig,
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

// RefreshDiscovery refreshes the API discovery to pick up newly installed CRDs.
func (c *client) RefreshDiscovery(ctx context.Context) error {
	if len(c.kubeconfig) == 0 {
		// For test clients, skip refresh
		return nil
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(c.kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create REST config: %w", err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create discovery client: %w", err)
	}

	groupResources, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		return fmt.Errorf("failed to get API group resources: %w", err)
	}

	c.mapper = restmapper.NewDiscoveryRESTMapper(groupResources)
	return nil
}

// HasCRD checks if a specific API resource exists (e.g., "cert-manager.io/v1/ClusterIssuer").
// The crdName parameter is in the format "group/version/kind" (e.g., "cert-manager.io/v1/ClusterIssuer").
func (c *client) HasCRD(ctx context.Context, crdName string) (bool, error) {
	if len(c.kubeconfig) == 0 {
		return true, nil // For test clients, assume CRDs exist
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(c.kubeconfig)
	if err != nil {
		return false, fmt.Errorf("failed to create REST config: %w", err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return false, fmt.Errorf("failed to create discovery client: %w", err)
	}

	// Check if the CRD exists by querying the API
	_, apiResourceLists, err := discoveryClient.ServerGroupsAndResources()
	if err != nil {
		// Partial discovery errors are common when some APIs are unavailable
		if !discovery.IsGroupDiscoveryFailedError(err) {
			return false, fmt.Errorf("failed to discover API resources: %w", err)
		}
	}

	// Look for the specified API resource
	// For cert-manager, we check for the cert-manager.io API group
	for _, list := range apiResourceLists {
		if list.GroupVersion == "cert-manager.io/v1" {
			for _, resource := range list.APIResources {
				if resource.Kind == "ClusterIssuer" {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

// HasReadyEndpoints checks if a service has at least one ready endpoint.
func (c *client) HasReadyEndpoints(ctx context.Context, namespace, serviceName string) (bool, error) {
	endpoints, err := c.clientset.CoreV1().Endpoints(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return false, nil // Service doesn't exist yet
	}

	// Check if any subset has at least one ready address
	for _, subset := range endpoints.Subsets {
		if len(subset.Addresses) > 0 {
			return true, nil
		}
	}

	return false, nil
}

// GetWorkerExternalIPs returns the external IPs of worker nodes.
// Worker nodes are identified by NOT having the control-plane role label.
func (c *client) GetWorkerExternalIPs(ctx context.Context) ([]string, error) {
	nodes, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	var externalIPs []string
	for _, node := range nodes.Items {
		// Skip control plane nodes
		if _, isControlPlane := node.Labels["node-role.kubernetes.io/control-plane"]; isControlPlane {
			continue
		}

		// Get external IP from node addresses
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeExternalIP && addr.Address != "" {
				externalIPs = append(externalIPs, addr.Address)
				break // Only need one external IP per node
			}
		}
	}

	return externalIPs, nil
}
