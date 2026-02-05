// Package addons provides addon management for the operator.
package addons

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/addons"
	"github.com/imamik/k8zner/internal/config"
)

// AddonInfo contains metadata about an addon.
type AddonInfo struct {
	Name         string
	InstallOrder int
	Required     bool // Whether the addon is always installed
}

// AddonOrder defines the installation order for addons.
// Lower numbers are installed first.
var AddonOrder = []AddonInfo{
	{Name: k8znerv1alpha1.AddonNameCilium, InstallOrder: k8znerv1alpha1.AddonOrderCilium, Required: true},
	{Name: k8znerv1alpha1.AddonNameCCM, InstallOrder: k8znerv1alpha1.AddonOrderCCM, Required: true},
	{Name: k8znerv1alpha1.AddonNameCSI, InstallOrder: k8znerv1alpha1.AddonOrderCSI, Required: true},
	{Name: k8znerv1alpha1.AddonNameMetricsServer, InstallOrder: k8znerv1alpha1.AddonOrderMetricsServer, Required: false},
	{Name: k8znerv1alpha1.AddonNameCertManager, InstallOrder: k8znerv1alpha1.AddonOrderCertManager, Required: false},
	{Name: k8znerv1alpha1.AddonNameTraefik, InstallOrder: k8znerv1alpha1.AddonOrderTraefik, Required: false},
	{Name: k8znerv1alpha1.AddonNameExternalDNS, InstallOrder: k8znerv1alpha1.AddonOrderExternalDNS, Required: false},
	{Name: k8znerv1alpha1.AddonNameArgoCD, InstallOrder: k8znerv1alpha1.AddonOrderArgoCD, Required: false},
	{Name: k8znerv1alpha1.AddonNameMonitoring, InstallOrder: k8znerv1alpha1.AddonOrderMonitoring, Required: false},
	{Name: k8znerv1alpha1.AddonNameTalosBackup, InstallOrder: k8znerv1alpha1.AddonOrderTalosBackup, Required: false},
}

// PhaseManager manages addon installation phases.
type PhaseManager struct {
	k8sClient client.Client
}

// NewPhaseManager creates a new addon phase manager.
func NewPhaseManager(c client.Client) *PhaseManager {
	return &PhaseManager{
		k8sClient: c,
	}
}

// InstallCilium installs Cilium CNI and waits for it to be ready.
func (m *PhaseManager) InstallCilium(ctx context.Context, cfg *config.Config, kubeconfig []byte) error {
	logger := log.FromContext(ctx)
	logger.Info("installing Cilium CNI")

	if err := addons.ApplyCilium(ctx, cfg, kubeconfig); err != nil {
		return fmt.Errorf("failed to install Cilium: %w", err)
	}

	logger.Info("waiting for Cilium to be ready")
	return m.waitForCiliumReady(ctx, kubeconfig)
}

// InstallAddons installs all remaining addons after CNI is ready.
func (m *PhaseManager) InstallAddons(ctx context.Context, cfg *config.Config, kubeconfig []byte, networkID int64) error {
	logger := log.FromContext(ctx)
	logger.Info("installing cluster addons")

	if err := addons.ApplyWithoutCilium(ctx, cfg, kubeconfig, networkID); err != nil {
		return fmt.Errorf("failed to install addons: %w", err)
	}

	return nil
}

// waitForCiliumReady waits for Cilium pods to be ready.
func (m *PhaseManager) waitForCiliumReady(ctx context.Context, kubeconfig []byte) error {
	// Create a new client from kubeconfig
	restConfig, err := clientConfigFromKubeconfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create rest config: %w", err)
	}

	k8sClient, err := client.New(restConfig, client.Options{})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Wait for Cilium pods
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-checkCtx.Done():
			return fmt.Errorf("timeout waiting for Cilium to be ready")
		case <-ticker.C:
			if ready, err := m.isCiliumReady(ctx, k8sClient); err == nil && ready {
				return nil
			}
		}
	}
}

// isCiliumReady checks if all Cilium pods are running and ready.
func (m *PhaseManager) isCiliumReady(ctx context.Context, k8sClient client.Client) (bool, error) {
	podList := &corev1.PodList{}
	if err := k8sClient.List(ctx, podList,
		client.InNamespace("kube-system"),
		client.MatchingLabels{"k8s-app": "cilium"},
	); err != nil {
		return false, err
	}

	if len(podList.Items) == 0 {
		return false, nil
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			return false, nil
		}
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status != corev1.ConditionTrue {
				return false, nil
			}
		}
	}

	return true, nil
}

// UpdateAddonStatus updates the addon status in the cluster CR.
func (m *PhaseManager) UpdateAddonStatus(cluster *k8znerv1alpha1.K8znerCluster, name string, installed, healthy bool, phase k8znerv1alpha1.AddonPhase, message string) {
	if cluster.Status.Addons == nil {
		cluster.Status.Addons = make(map[string]k8znerv1alpha1.AddonStatus)
	}

	now := metav1.Now()
	order := getAddonOrder(name)

	cluster.Status.Addons[name] = k8znerv1alpha1.AddonStatus{
		Installed:          installed,
		Healthy:            healthy,
		Phase:              phase,
		Message:            message,
		LastTransitionTime: &now,
		InstallOrder:       order,
	}
}

// getAddonOrder returns the installation order for an addon.
func getAddonOrder(name string) int {
	for _, info := range AddonOrder {
		if info.Name == name {
			return info.InstallOrder
		}
	}
	return 99 // Unknown addons last
}

// IsAddonEnabled checks if an addon is enabled in the cluster spec.
func IsAddonEnabled(cluster *k8znerv1alpha1.K8znerCluster, name string) bool {
	// Core addons are always enabled
	switch name {
	case k8znerv1alpha1.AddonNameCilium, k8znerv1alpha1.AddonNameCCM, k8znerv1alpha1.AddonNameCSI:
		return true
	}

	// TalosBackup is configured via spec.Backup, not spec.Addons
	if name == k8znerv1alpha1.AddonNameTalosBackup {
		return cluster.Spec.Backup != nil && cluster.Spec.Backup.Enabled
	}

	// All other addons require spec.Addons to be set
	if cluster.Spec.Addons == nil {
		return false
	}

	switch name {
	case k8znerv1alpha1.AddonNameMetricsServer:
		return cluster.Spec.Addons.MetricsServer
	case k8znerv1alpha1.AddonNameCertManager:
		return cluster.Spec.Addons.CertManager
	case k8znerv1alpha1.AddonNameTraefik:
		return cluster.Spec.Addons.Traefik
	case k8znerv1alpha1.AddonNameExternalDNS:
		return cluster.Spec.Addons.ExternalDNS
	case k8znerv1alpha1.AddonNameArgoCD:
		return cluster.Spec.Addons.ArgoCD
	case k8znerv1alpha1.AddonNameMonitoring:
		return cluster.Spec.Addons.Monitoring
	default:
		return false
	}
}

// clientConfigFromKubeconfig creates a rest.Config from kubeconfig bytes.
func clientConfigFromKubeconfig(kubeconfig []byte) (*rest.Config, error) {
	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create client config: %w", err)
	}
	return clientConfig.ClientConfig()
}
