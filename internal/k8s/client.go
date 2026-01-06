// Package k8s provides a Kubernetes client wrapper for addon management.
package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Client wraps Kubernetes API operations for addon installation.
type Client struct {
	clientset *kubernetes.Clientset
	dynamic   dynamic.Interface
}

// NewClient creates a new Kubernetes client from a kubeconfig file.
func NewClient(kubeconfigPath string) (*Client, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &Client{
		clientset: clientset,
		dynamic:   dynamicClient,
	}, nil
}

// NewClientFromBytes creates a new Kubernetes client from kubeconfig bytes.
func NewClientFromBytes(kubeconfigData []byte) (*Client, error) {
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigData)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig from bytes: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &Client{
		clientset: clientset,
		dynamic:   dynamicClient,
	}, nil
}

// Apply applies a YAML manifest to the cluster using server-side apply.
// This ensures idempotent operations.
func (c *Client) Apply(ctx context.Context, manifest string) error {
	// Parse YAML into unstructured objects
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)

	for {
		var obj unstructured.Unstructured
		err := decoder.Decode(&obj)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return fmt.Errorf("failed to decode manifest: %w", err)
		}

		// Skip empty objects
		if obj.Object == nil || len(obj.Object) == 0 {
			continue
		}

		gvk := obj.GroupVersionKind()
		gvr := schema.GroupVersionResource{
			Group:    gvk.Group,
			Version:  gvk.Version,
			Resource: resourceForKind(gvk.Kind),
		}

		// Get namespace (default to "default" if not specified)
		namespace := obj.GetNamespace()
		if namespace == "" {
			namespace = "default"
		}

		// Use dynamic client to apply the resource
		var result *unstructured.Unstructured
		if namespace != "" && namespace != "default" {
			result, err = c.dynamic.Resource(gvr).Namespace(namespace).
				Create(ctx, &obj, metav1.CreateOptions{})
		} else {
			result, err = c.dynamic.Resource(gvr).
				Create(ctx, &obj, metav1.CreateOptions{})
		}

		if err != nil {
			// If resource already exists, try to update it
			if namespace != "" && namespace != "default" {
				result, err = c.dynamic.Resource(gvr).Namespace(namespace).
					Update(ctx, &obj, metav1.UpdateOptions{})
			} else {
				result, err = c.dynamic.Resource(gvr).
					Update(ctx, &obj, metav1.UpdateOptions{})
			}

			if err != nil {
				return fmt.Errorf("failed to apply resource %s/%s: %w",
					obj.GetKind(), obj.GetName(), err)
			}
		}

		fmt.Printf("Applied %s/%s in namespace %s\n",
			result.GetKind(), result.GetName(), namespace)
	}

	return nil
}

// CreateSecret creates or updates a Kubernetes secret.
func (c *Client) CreateSecret(ctx context.Context, namespace, name string, data map[string][]byte) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
		Type: corev1.SecretTypeOpaque,
	}

	// Try to create the secret
	_, err := c.clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		// If it already exists, update it
		_, err = c.clientset.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create or update secret %s/%s: %w", namespace, name, err)
		}
		fmt.Printf("Updated secret %s in namespace %s\n", name, namespace)
		return nil
	}

	fmt.Printf("Created secret %s in namespace %s\n", name, namespace)
	return nil
}

// WaitForDeployment waits for a deployment to become ready.
func (c *Client) WaitForDeployment(ctx context.Context, namespace, name string, timeout time.Duration) error {
	return wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		deployment, err := c.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}

		return isDeploymentReady(deployment), nil
	})
}

// WaitForDaemonSet waits for a daemonset to become ready.
func (c *Client) WaitForDaemonSet(ctx context.Context, namespace, name string, timeout time.Duration) error {
	return wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		daemonSet, err := c.clientset.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}

		return isDaemonSetReady(daemonSet), nil
	})
}

// GetPods returns pods matching a label selector in a namespace.
func (c *Client) GetPods(ctx context.Context, namespace, labelSelector string) ([]corev1.Pod, error) {
	podList, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	return podList.Items, nil
}

// isDeploymentReady checks if a deployment is ready.
func isDeploymentReady(deployment *appsv1.Deployment) bool {
	if deployment.Status.UpdatedReplicas != *deployment.Spec.Replicas {
		return false
	}
	if deployment.Status.Replicas != *deployment.Spec.Replicas {
		return false
	}
	if deployment.Status.AvailableReplicas != *deployment.Spec.Replicas {
		return false
	}

	// Check for available condition
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable &&
			condition.Status == corev1.ConditionTrue {
			return true
		}
	}

	return false
}

// isDaemonSetReady checks if a daemonset is ready.
func isDaemonSetReady(daemonSet *appsv1.DaemonSet) bool {
	return daemonSet.Status.DesiredNumberScheduled > 0 &&
		daemonSet.Status.NumberReady == daemonSet.Status.DesiredNumberScheduled &&
		daemonSet.Status.NumberAvailable == daemonSet.Status.DesiredNumberScheduled
}

// resourceForKind maps a Kubernetes kind to its resource name.
// This is a simplified mapping for common resources.
func resourceForKind(kind string) string {
	switch kind {
	case "Deployment":
		return "deployments"
	case "Service":
		return "services"
	case "ConfigMap":
		return "configmaps"
	case "Secret":
		return "secrets"
	case "DaemonSet":
		return "daemonsets"
	case "StatefulSet":
		return "statefulsets"
	case "ServiceAccount":
		return "serviceaccounts"
	case "Role":
		return "roles"
	case "RoleBinding":
		return "rolebindings"
	case "ClusterRole":
		return "clusterroles"
	case "ClusterRoleBinding":
		return "clusterrolebindings"
	case "Namespace":
		return "namespaces"
	case "PersistentVolumeClaim":
		return "persistentvolumeclaims"
	case "PersistentVolume":
		return "persistentvolumes"
	case "StorageClass":
		return "storageclasses"
	default:
		// Default: lowercase kind + 's'
		return kind + "s"
	}
}
