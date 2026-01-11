// Package helm provides a Helm client for installing charts programmatically.
package helm

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

// InMemoryRESTClientGetter implements genericclioptions.RESTClientGetter
// using in-memory kubeconfig bytes instead of filesystem paths.
// This allows the Helm client to work with kubeconfig data directly.
type InMemoryRESTClientGetter struct {
	kubeconfig []byte
	namespace  string
	restConfig *rest.Config
}

// NewInMemoryRESTClientGetter creates a new RESTClientGetter from kubeconfig bytes.
func NewInMemoryRESTClientGetter(kubeconfig []byte, namespace string) *InMemoryRESTClientGetter {
	return &InMemoryRESTClientGetter{
		kubeconfig: kubeconfig,
		namespace:  namespace,
	}
}

// ToRESTConfig returns a REST config from the kubeconfig bytes.
func (g *InMemoryRESTClientGetter) ToRESTConfig() (*rest.Config, error) {
	if g.restConfig != nil {
		return g.restConfig, nil
	}

	clientConfig, err := clientcmd.NewClientConfigFromBytes(g.kubeconfig)
	if err != nil {
		return nil, err
	}

	g.restConfig, err = clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	return g.restConfig, nil
}

// ToDiscoveryClient returns a cached discovery client.
func (g *InMemoryRESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	restConfig, err := g.ToRESTConfig()
	if err != nil {
		return nil, err
	}

	dc, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	return memory.NewMemCacheClient(dc), nil
}

// ToRESTMapper returns a REST mapper for the cluster.
func (g *InMemoryRESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	dc, err := g.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}

	return restmapper.NewDeferredDiscoveryRESTMapper(dc), nil
}

// ToRawKubeConfigLoader returns a clientcmd.ClientConfig.
func (g *InMemoryRESTClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	clientConfig, _ := clientcmd.NewClientConfigFromBytes(g.kubeconfig)
	return clientConfig
}
