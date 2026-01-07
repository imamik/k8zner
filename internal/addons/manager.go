package addons

import (
	"context"
	"fmt"
	"log"

	"github.com/sak-d/hcloud-k8s/internal/config"
	"github.com/sak-d/hcloud-k8s/internal/k8s"
)

// AddonManager defines the interface for managing Kubernetes addons.
type AddonManager interface {
	EnsureAddons(ctx context.Context) error
}

// Manager orchestrates addon deployment.
type Manager struct {
	k8sClient  *k8s.Client
	helmClient *k8s.HelmClient
	cfg        *config.Config
	kubeconfig []byte
	networkID  int64
}

// NewManager creates a new addon manager.
func NewManager(kubeconfig []byte, cfg *config.Config, networkID int64) (*Manager, error) {
	kClient, err := k8s.NewClient(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	hClient := k8s.NewHelmClient()

	return &Manager{
		k8sClient:  kClient,
		helmClient: hClient,
		cfg:        cfg,
		kubeconfig: kubeconfig,
		networkID:  networkID,
	}, nil
}

// EnsureAddons ensures all required addons are deployed.
func (m *Manager) EnsureAddons(ctx context.Context) error {
	log.Println("--- Ensuring Addons ---")

	// 1. Core Secret (Hetzner Token & Network)
	if err := m.ensureHCloudSecret(ctx); err != nil {
		return fmt.Errorf("failed to ensure hcloud secret: %w", err)
	}

	// 2. CNI (Cilium) - Needed for other pods to reach Ready
	if err := m.ensureCilium(ctx); err != nil {
		return fmt.Errorf("failed to ensure Cilium: %w", err)
	}

	// 3. Cloud Controller Manager (CCM)
	if err := m.ensureCCM(ctx); err != nil {
		return fmt.Errorf("failed to ensure CCM: %w", err)
	}

	// 4. CSI Driver
	if err := m.ensureCSI(ctx); err != nil {
		return fmt.Errorf("failed to ensure CSI: %w", err)
	}

	log.Println("--- All Addons Ensured ---")
	return nil
}
