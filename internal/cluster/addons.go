package cluster

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sak-d/hcloud-k8s/internal/addons"
	"github.com/sak-d/hcloud-k8s/internal/addons/ccm"
	"github.com/sak-d/hcloud-k8s/internal/addons/cilium"
	"github.com/sak-d/hcloud-k8s/internal/addons/csi"
	"github.com/sak-d/hcloud-k8s/internal/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// reconcileAddons installs and verifies Kubernetes addons.
func (r *Reconciler) reconcileAddons(ctx context.Context, kubeconfigPath string) error {
	log.Println("=== RECONCILING ADDONS ===")

	// Check if any addons are enabled
	if !r.hasEnabledAddons() {
		log.Println("No addons enabled, skipping addon installation")
		return nil
	}

	// Create Kubernetes client
	k8sClient, err := k8s.NewClient(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Note: We don't wait for full API server readiness here because Talos clusters
	// often need a CNI (Cilium) to be installed before the API server becomes fully
	// operational. The addon installation will retry on connection errors.
	log.Println("Note: API server may not be fully ready until CNI (Cilium) is installed")

	// Create addon instances
	addonList := r.createAddonList()

	// Create addon manager
	manager := addons.NewManager(k8sClient, addonList, addons.DefaultInstallOptions())

	// Install addons
	if err := manager.Install(ctx); err != nil {
		return fmt.Errorf("failed to install addons: %w", err)
	}

	log.Println("=== ADDONS RECONCILIATION COMPLETE ===")
	return nil
}

// hasEnabledAddons checks if any addons are enabled.
func (r *Reconciler) hasEnabledAddons() bool {
	return r.config.Addons.CCM.Enabled ||
		r.config.Addons.CSI.Enabled ||
		r.config.Addons.Cilium.Enabled
}

// createAddonList creates a list of addon instances based on configuration.
func (r *Reconciler) createAddonList() []addons.Addon {
	var addonList []addons.Addon

	// Get network ID for addon configuration
	networkID := ""
	if r.network != nil {
		networkID = fmt.Sprintf("%d", r.network.ID)
	}

	// CCM (Hetzner Cloud Controller Manager)
	if r.config.Addons.CCM.Enabled {
		ccmAddon := ccm.New(
			r.config,
			&r.config.Addons.CCM,
			r.config.HCloudToken,
			networkID,
		)
		addonList = append(addonList, ccmAddon)
	}

	// CSI (Hetzner CSI Driver)
	if r.config.Addons.CSI.Enabled {
		csiAddon := csi.New(
			r.config,
			&r.config.Addons.CSI,
			r.config.HCloudToken,
		)
		addonList = append(addonList, csiAddon)
	}

	// Cilium (CNI)
	if r.config.Addons.Cilium.Enabled {
		ciliumAddon := cilium.New(
			r.config,
			&r.config.Addons.Cilium,
		)
		addonList = append(addonList, ciliumAddon)
	}

	return addonList
}

// waitForAPIServer waits for the Kubernetes API server to be responsive and ready.
func waitForAPIServer(ctx context.Context, k8sClient *k8s.Client, timeout time.Duration) error {
	clientset := k8sClient.GetClientset()
	deadline := time.Now().Add(timeout)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for API server to be ready")
		}

		// Try to list nodes as a health check
		_, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 1})
		if err == nil {
			// API server is responsive
			return nil
		}

		log.Printf("API server not yet ready: %v (retrying in 5s)", err)
		time.Sleep(5 * time.Second)
	}
}
