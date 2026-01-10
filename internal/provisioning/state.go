package provisioning

import "github.com/hetznercloud/hcloud-go/v2/hcloud"

// State holds the shared results of provisioning phases.
// It is progressively populated as each phase completes and is passed
// to subsequent phases that need earlier results.
type State struct {
	// Infrastructure results (populated by infrastructure provisioner)
	Network  *hcloud.Network
	Firewall *hcloud.Firewall

	// Compute results (populated by compute provisioner)
	ControlPlaneIPs map[string]string // nodeName -> publicIP
	WorkerIPs       map[string]string // nodeName -> publicIP
	SANs            []string          // Subject Alternative Names for certs

	// Cluster results (populated by cluster bootstrapper)
	Kubeconfig  []byte
	TalosConfig []byte
}

// NewState creates an empty provisioning state.
func NewState() *State {
	return &State{
		ControlPlaneIPs: make(map[string]string),
		WorkerIPs:       make(map[string]string),
	}
}
