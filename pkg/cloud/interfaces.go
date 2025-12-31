package cloud

import (
	"context"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// Interfaces for hcloud services to allow mocking

// ActionClient defines the interface for interacting with Hetzner Cloud Actions.
type ActionClient interface {
	GetByID(ctx context.Context, id int64) (*hcloud.Action, *hcloud.Response, error)
}

// NetworkClient defines the interface for interacting with Hetzner Cloud Networks.
type NetworkClient interface {
	GetByID(ctx context.Context, id int64) (*hcloud.Network, *hcloud.Response, error)
	GetByName(ctx context.Context, name string) (*hcloud.Network, *hcloud.Response, error)
	Create(ctx context.Context, opts hcloud.NetworkCreateOpts) (*hcloud.Network, *hcloud.Response, error)
	Delete(ctx context.Context, network *hcloud.Network) (*hcloud.Response, error)
}

// FirewallClient defines the interface for interacting with Hetzner Cloud Firewalls.
type FirewallClient interface {
	GetByID(ctx context.Context, id int64) (*hcloud.Firewall, *hcloud.Response, error)
	GetByName(ctx context.Context, name string) (*hcloud.Firewall, *hcloud.Response, error)
	// Create returns value, not pointer for FirewallCreateResult
	Create(ctx context.Context, opts hcloud.FirewallCreateOpts) (hcloud.FirewallCreateResult, *hcloud.Response, error)
	Delete(ctx context.Context, firewall *hcloud.Firewall) (*hcloud.Response, error)
}

// PlacementGroupClient defines the interface for interacting with Hetzner Cloud Placement Groups.
type PlacementGroupClient interface {
	GetByID(ctx context.Context, id int64) (*hcloud.PlacementGroup, *hcloud.Response, error)
	GetByName(ctx context.Context, name string) (*hcloud.PlacementGroup, *hcloud.Response, error)
	// Create returns value, not pointer for PlacementGroupCreateResult
	Create(ctx context.Context, opts hcloud.PlacementGroupCreateOpts) (hcloud.PlacementGroupCreateResult, *hcloud.Response, error)
}

// ServerClient defines the interface for interacting with Hetzner Cloud Servers.
type ServerClient interface {
	GetByID(ctx context.Context, id int64) (*hcloud.Server, *hcloud.Response, error)
	GetByName(ctx context.Context, name string) (*hcloud.Server, *hcloud.Response, error)
	// Create returns value, not pointer for ServerCreateResult
	Create(ctx context.Context, opts hcloud.ServerCreateOpts) (hcloud.ServerCreateResult, *hcloud.Response, error)
	Delete(ctx context.Context, server *hcloud.Server) (*hcloud.Response, error)
	EnableRescue(ctx context.Context, server *hcloud.Server, opts hcloud.ServerEnableRescueOpts) (hcloud.ServerEnableRescueResult, *hcloud.Response, error)
	Reset(ctx context.Context, server *hcloud.Server) (*hcloud.Action, *hcloud.Response, error)
	CreateImage(ctx context.Context, server *hcloud.Server, opts *hcloud.ServerCreateImageOpts) (hcloud.ServerCreateImageResult, *hcloud.Response, error)
}

// ImageClient defines the interface for interacting with Hetzner Cloud Images.
type ImageClient interface {
	GetByID(ctx context.Context, id int64) (*hcloud.Image, *hcloud.Response, error)
	GetByName(ctx context.Context, name string) (*hcloud.Image, *hcloud.Response, error)
	All(ctx context.Context) ([]*hcloud.Image, error)
    AllWithOpts(ctx context.Context, opts hcloud.ImageListOpts) ([]*hcloud.Image, error)
}

// SSHKeyClient defines the interface for interacting with Hetzner Cloud SSH Keys.
type SSHKeyClient interface {
	Create(ctx context.Context, opts hcloud.SSHKeyCreateOpts) (*hcloud.SSHKey, *hcloud.Response, error)
	Delete(ctx context.Context, sshKey *hcloud.SSHKey) (*hcloud.Response, error)
}

// LoadBalancerClient defines the interface for interacting with Hetzner Cloud Load Balancers.
type LoadBalancerClient interface {
    GetByName(ctx context.Context, name string) (*hcloud.LoadBalancer, *hcloud.Response, error)
    Create(ctx context.Context, opts hcloud.LoadBalancerCreateOpts) (hcloud.LoadBalancerCreateResult, *hcloud.Response, error)
    AddService(ctx context.Context, lb *hcloud.LoadBalancer, opts hcloud.LoadBalancerAddServiceOpts) (*hcloud.Action, *hcloud.Response, error)
    AddServerTarget(ctx context.Context, lb *hcloud.LoadBalancer, opts hcloud.LoadBalancerAddServerTargetOpts) (*hcloud.Action, *hcloud.Response, error)
}
