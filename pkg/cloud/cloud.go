package cloud

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/hcloud-k8s/hcloud-k8s/pkg/config"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// Cloud provides methods to manage Hetzner Cloud infrastructure for the Kubernetes cluster.
type Cloud struct {
    Client *hcloud.Client
	Config *config.ClusterConfig

    // Services
    Action         ActionClient
    Network        NetworkClient
    Firewall       FirewallClient
    PlacementGroup PlacementGroupClient
    Server         ServerClient
    Image          ImageClient
    LoadBalancer   LoadBalancerClient
}

// NewCloud initializes a new Cloud instance with the given token and configuration.
func NewCloud(token string, cfg *config.ClusterConfig) *Cloud {
    client := hcloud.NewClient(hcloud.WithToken(token))
	return &Cloud{
		Client:         client,
		Config:         cfg,
        Action:         &client.Action,
        Network:        &client.Network,
        Firewall:       &client.Firewall,
        PlacementGroup: &client.PlacementGroup,
        Server:         &client.Server,
        Image:          &client.Image,
        LoadBalancer:   &client.LoadBalancer,
	}
}

// EnsureNetwork ensures the existence of the cluster network with configured CIDR.
func (c *Cloud) EnsureNetwork(ctx context.Context) (*hcloud.Network, error) {
	name := c.Config.ClusterName
	network, _, err := c.Network.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get network: %w", err)
	}
	if network != nil {
		return network, nil
	}

    cidr := c.Config.NetworkIPv4CIDR
    if cidr == "" {
        cidr = "10.0.0.0/16"
    }

	_, ipNet, err := net.ParseCIDR(cidr)
    if err != nil {
        return nil, fmt.Errorf("invalid network CIDR %s: %w", cidr, err)
    }

	opts := hcloud.NetworkCreateOpts{
		Name:    name,
		IPRange: ipNet,
	}

	res, _, err := c.Network.Create(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create network: %w", err)
	}
	return res, nil
}

// EnsureFirewall ensures the existence of the cluster firewall with necessary rules and configured allow lists.
func (c *Cloud) EnsureFirewall(ctx context.Context) (*hcloud.Firewall, error) {
	name := c.Config.ClusterName
	fw, _, err := c.Firewall.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get firewall: %w", err)
	}
	if fw != nil {
		return fw, nil
	}

    // Default allow all if list is empty
    apiSourceIPs := []net.IPNet{{IP: net.IPv4(0, 0, 0, 0), Mask: net.CIDRMask(0, 32)}}
    if len(c.Config.Firewall.ApiAllowList) > 0 {
        apiSourceIPs = []net.IPNet{}
        for _, cidr := range c.Config.Firewall.ApiAllowList {
            _, ipNet, err := net.ParseCIDR(cidr)
            if err != nil {
                return nil, fmt.Errorf("invalid API allow list CIDR %s: %w", cidr, err)
            }
            apiSourceIPs = append(apiSourceIPs, *ipNet)
        }
    }

	opts := hcloud.FirewallCreateOpts{
		Name: name,
		Rules: []hcloud.FirewallRule{
			{
				Direction: hcloud.FirewallRuleDirectionIn,
				Protocol:  hcloud.FirewallRuleProtocolICMP,
				SourceIPs: []net.IPNet{{IP: net.IPv4(0, 0, 0, 0), Mask: net.CIDRMask(0, 32)}},
			},
			{
				Direction: hcloud.FirewallRuleDirectionIn,
				Protocol:  hcloud.FirewallRuleProtocolTCP,
				Port:      hcloud.Ptr("6443"), // Kubernetes API
				SourceIPs: apiSourceIPs,
			},
			{
				Direction: hcloud.FirewallRuleDirectionIn,
				Protocol:  hcloud.FirewallRuleProtocolTCP,
				Port:      hcloud.Ptr("50000"), // Talos API
				SourceIPs: apiSourceIPs,
			},
		},
	}

	res, _, err := c.Firewall.Create(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create firewall: %w", err)
	}
	return res.Firewall, nil
}

// EnsurePlacementGroup ensures the existence of a placement group.
func (c *Cloud) EnsurePlacementGroup(ctx context.Context, name string, kind hcloud.PlacementGroupType) (*hcloud.PlacementGroup, error) {
	pg, _, err := c.PlacementGroup.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get placement group: %w", err)
	}
	if pg != nil {
		return pg, nil
	}

	res, _, err := c.PlacementGroup.Create(ctx, hcloud.PlacementGroupCreateOpts{
		Name: name,
		Type: kind,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create placement group: %w", err)
	}
	return res.PlacementGroup, nil
}

// EnsureServer ensures the existence of a server with the specified configuration.
func (c *Cloud) EnsureServer(ctx context.Context, name string, serverType string, location string, imageID int64, network *hcloud.Network, firewall *hcloud.Firewall, placementGroup *hcloud.PlacementGroup, userData string) (*hcloud.Server, error) {
	server, _, err := c.Server.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get server: %w", err)
	}
	if server != nil {
		return server, nil
	}

	img, _, err := c.Image.GetByID(ctx, imageID)
	if err != nil {
		return nil, fmt.Errorf("failed to get image: %w", err)
	}

	opts := hcloud.ServerCreateOpts{
		Name:             name,
		ServerType:       &hcloud.ServerType{Name: serverType},
		Image:            img,
		Location:         &hcloud.Location{Name: location},
		Networks:         []*hcloud.Network{network},
		Firewalls:        []*hcloud.ServerCreateFirewall{{Firewall: *firewall}},
		PlacementGroup:   placementGroup,
		UserData:         userData,
		StartAfterCreate: hcloud.Ptr(true),
	}

	res, _, err := c.Server.Create(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create server: %w", err)
	}

	// Wait for running
	if err := c.waitForAction(ctx, res.Action); err != nil {
		return nil, fmt.Errorf("failed to wait for server creation: %w", err)
	}

	return res.Server, nil
}

func (c *Cloud) waitForAction(ctx context.Context, action *hcloud.Action) error {
	for {
		act, _, err := c.Action.GetByID(ctx, action.ID)
		if err != nil {
			return err
		}
		if act.Status == hcloud.ActionStatusSuccess {
			return nil
		}
		if act.Status == hcloud.ActionStatusError {
			return act.Error()
		}
		time.Sleep(1 * time.Second)
	}
}

// CreateLoadBalancer creates a new Load Balancer and attaches it to the network.
func (c *Cloud) CreateLoadBalancer(ctx context.Context, name string, location string, network *hcloud.Network) (*hcloud.LoadBalancer, error) {
    lb, _, err := c.LoadBalancer.GetByName(ctx, name)
    if err != nil {
        return nil, fmt.Errorf("failed to get load balancer: %w", err)
    }
    if lb != nil {
        return lb, nil
    }

    // Create LB
    res, _, err := c.LoadBalancer.Create(ctx, hcloud.LoadBalancerCreateOpts{
        Name:             name,
        LoadBalancerType: &hcloud.LoadBalancerType{Name: "lb11"},
        Location:         &hcloud.Location{Name: location},
        Network:          network,
    })

    if err != nil {
        return nil, fmt.Errorf("failed to create load balancer: %w", err)
    }

    if err := c.waitForAction(ctx, res.Action); err != nil {
        return nil, fmt.Errorf("failed to wait for load balancer creation: %w", err)
    }

    // Add Service (Port 6443)
    _, _, err = c.LoadBalancer.AddService(ctx, res.LoadBalancer, hcloud.LoadBalancerAddServiceOpts{
        Protocol:        hcloud.LoadBalancerServiceProtocolTCP,
        ListenPort:      hcloud.Ptr(6443),
        DestinationPort: hcloud.Ptr(6443),
    })
     if err != nil {
        log.Printf("Failed to add service to LB: %v", err)
    }

    return res.LoadBalancer, nil
}
