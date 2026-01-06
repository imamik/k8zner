// Package addons provides addon management for Kubernetes clusters.
package addons

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// Addon represents a Kubernetes addon that can be installed, verified, and managed.
type Addon interface {
	// Name returns the unique name of the addon.
	Name() string

	// Enabled returns whether this addon is enabled in the configuration.
	Enabled() bool

	// Dependencies returns a list of addon names that must be installed before this one.
	Dependencies() []string

	// GenerateManifests generates the Kubernetes manifests for this addon.
	// Returns a slice of YAML manifest strings.
	GenerateManifests(ctx context.Context) ([]string, error)

	// Verify checks if the addon is installed and running correctly.
	Verify(ctx context.Context, k8sClient K8sClient) error
}

// K8sClient defines the Kubernetes operations needed by addons.
type K8sClient interface {
	// Apply applies a YAML manifest to the cluster.
	Apply(ctx context.Context, manifest string) error

	// CreateSecret creates or updates a Kubernetes secret.
	CreateSecret(ctx context.Context, namespace, name string, data map[string][]byte) error

	// WaitForDeployment waits for a deployment to become ready.
	WaitForDeployment(ctx context.Context, namespace, name string, timeout time.Duration) error

	// WaitForDaemonSet waits for a daemonset to become ready.
	WaitForDaemonSet(ctx context.Context, namespace, name string, timeout time.Duration) error

	// WaitForPodsReady waits for all pods matching a label selector to become ready.
	WaitForPodsReady(ctx context.Context, namespace, labelSelector string, timeout time.Duration) error

	// GetPods returns pods matching a label selector in a namespace.
	GetPods(ctx context.Context, namespace, labelSelector string) ([]corev1.Pod, error)

	// SecretExists checks if a secret exists in the given namespace.
	SecretExists(ctx context.Context, namespace, name string) (bool, error)

	// CheckPodLogs retrieves logs from a pod to check for errors.
	CheckPodLogs(ctx context.Context, namespace, name string) (string, error)
}

// AddonConfig holds runtime configuration shared across all addons.
type AddonConfig struct {
	// ClusterName is the name of the cluster.
	ClusterName string

	// HCloudToken is the Hetzner Cloud API token.
	HCloudToken string

	// NetworkID is the ID of the Hetzner Cloud network.
	NetworkID string

	// NetworkCIDR is the CIDR of the pod network.
	NetworkCIDR string

	// LoadBalancerSubnet is the subnet for load balancers.
	LoadBalancerSubnet string

	// ControlPlaneCount is the number of control plane nodes.
	ControlPlaneCount int
}

// InstallOptions contains options for installing addons.
type InstallOptions struct {
	// Timeout is the maximum time to wait for addon installation.
	Timeout time.Duration

	// VerifyInstallation enables verification after installation.
	VerifyInstallation bool

	// ContinueOnError continues installing other addons if one fails.
	ContinueOnError bool
}

// DefaultInstallOptions returns default installation options.
func DefaultInstallOptions() InstallOptions {
	return InstallOptions{
		Timeout:            10 * time.Minute,
		VerifyInstallation: true,
		ContinueOnError:    false,
	}
}
