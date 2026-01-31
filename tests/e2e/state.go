//go:build e2e

package e2e

import (
	"github.com/imamik/k8zner/internal/platform/hcloud"
)

// SharedTestContext holds shared state across all E2E tests in a test run.
// This is used to share pre-built resources (like snapshots) across tests.
type SharedTestContext struct {
	SnapshotAMD64 string
	SnapshotARM64 string
	Client        *hcloud.RealClient
}

// sharedCtx is the global shared test context, initialized by TestMain.
var sharedCtx *SharedTestContext

// E2EState holds the cluster state as it progresses through test phases.
// Each phase reads from and updates this state, building on previous phases.
type E2EState struct {
	// Cluster Identity
	ClusterName string
	TestID      string // Unique test ID for resource labeling

	// Client
	Client *hcloud.RealClient

	// Snapshots (Phase 1)
	SnapshotAMD64 string
	SnapshotARM64 string

	// Infrastructure (Phase 2)
	NetworkID        string
	FirewallID       string
	LoadBalancerIP   string
	ControlPlanePGID string
	SSHKeyName       string
	SSHPrivateKey    []byte

	// Control Plane (Phase 3)
	ControlPlaneIPs  []string
	TalosConfig      []byte
	Kubeconfig       []byte
	KubeconfigPath   string
	TalosSecretsPath string

	// Workers (Phase 4)
	WorkerIPs       []string
	WorkerPoolNames []string

	// Addons (Phase 5-6)
	AddonsInstalled map[string]bool

	// Scale (Phase 7)
	ScaledOut bool
}

// NewE2EState creates a new state object for the E2E test lifecycle.
func NewE2EState(clusterName string, client *hcloud.RealClient) *E2EState {
	return &E2EState{
		ClusterName:     clusterName,
		TestID:          clusterName, // Use cluster name as test ID for labeling
		Client:          client,
		AddonsInstalled: make(map[string]bool),
		WorkerPoolNames: []string{},
		ControlPlaneIPs: []string{},
		WorkerIPs:       []string{},
	}
}
