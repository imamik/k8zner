package cloud

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/hcloud-k8s/hcloud-k8s/pkg/config"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

type Cloud struct {
	Client *hcloud.Client
	Config *config.ClusterConfig
}

func NewCloud(token string, cfg *config.ClusterConfig) *Cloud {
	return &Cloud{
		Client: hcloud.NewClient(hcloud.WithToken(token)),
		Config: cfg,
	}
}

func (c *Cloud) EnsureNetwork(ctx context.Context) (*hcloud.Network, error) {
	name := c.Config.ClusterName
	network, _, err := c.Client.Network.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if network != nil {
		return network, nil
	}

	_, ipNet, _ := net.ParseCIDR("10.0.0.0/16")
	opts := hcloud.NetworkCreateOpts{
		Name:    name,
		IPRange: ipNet,
	}

	res, _, err := c.Client.Network.Create(ctx, opts)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (c *Cloud) EnsureFirewall(ctx context.Context) (*hcloud.Firewall, error) {
	name := c.Config.ClusterName
	fw, _, err := c.Client.Firewall.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if fw != nil {
		return fw, nil
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
				SourceIPs: []net.IPNet{{IP: net.IPv4(0, 0, 0, 0), Mask: net.CIDRMask(0, 32)}},
			},
			{
				Direction: hcloud.FirewallRuleDirectionIn,
				Protocol:  hcloud.FirewallRuleProtocolTCP,
				Port:      hcloud.Ptr("50000"), // Talos API
				SourceIPs: []net.IPNet{{IP: net.IPv4(0, 0, 0, 0), Mask: net.CIDRMask(0, 32)}},
			},
		},
	}

	res, _, err := c.Client.Firewall.Create(ctx, opts)
	if err != nil {
		return nil, err
	}
	return res.Firewall, nil
}

func (c *Cloud) EnsurePlacementGroup(ctx context.Context, name string, kind hcloud.PlacementGroupType) (*hcloud.PlacementGroup, error) {
	pg, _, err := c.Client.PlacementGroup.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if pg != nil {
		return pg, nil
	}

	res, _, err := c.Client.PlacementGroup.Create(ctx, hcloud.PlacementGroupCreateOpts{
		Name: name,
		Type: kind,
	})
	if err != nil {
		return nil, err
	}
	return res.PlacementGroup, nil
}

func (c *Cloud) EnsureServer(ctx context.Context, name string, serverType string, location string, imageID int64, network *hcloud.Network, firewall *hcloud.Firewall, placementGroup *hcloud.PlacementGroup, userData string) (*hcloud.Server, error) {
	server, _, err := c.Client.Server.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if server != nil {
		return server, nil
	}

	img, _, err := c.Client.Image.GetByID(ctx, imageID)
	if err != nil {
		return nil, err
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

	res, _, err := c.Client.Server.Create(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Wait for running
	if err := c.waitForAction(ctx, res.Action); err != nil {
		return nil, err
	}

	return res.Server, nil
}

func (c *Cloud) waitForAction(ctx context.Context, action *hcloud.Action) error {
	for {
		act, _, err := c.Client.Action.GetByID(ctx, action.ID)
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

func (c *Cloud) GetLoadBalancer(ctx context.Context, name string) (*hcloud.LoadBalancer, error) {
	lb, _, err := c.Client.LoadBalancer.GetByName(ctx, name)
	return lb, err
}

func (c *Cloud) CreateLoadBalancer(ctx context.Context, name string, location string, network *hcloud.Network) (*hcloud.LoadBalancer, error) {
    lb, _, err := c.Client.LoadBalancer.GetByName(ctx, name)
    if err != nil {
        return nil, err
    }
    if lb != nil {
        return lb, nil
    }

    // Create LB
    res, _, err := c.Client.LoadBalancer.Create(ctx, hcloud.LoadBalancerCreateOpts{
        Name:             name,
        LoadBalancerType: &hcloud.LoadBalancerType{Name: "lb11"},
        Location:         &hcloud.Location{Name: location},
        Network:          network,
    })

    if err != nil {
        return nil, err
    }

    if err := c.waitForAction(ctx, res.Action); err != nil {
        return nil, err
    }

    // Add Service (Port 6443)
    _, _, err = c.Client.LoadBalancer.AddService(ctx, res.LoadBalancer, hcloud.LoadBalancerAddServiceOpts{
        Protocol:        hcloud.LoadBalancerServiceProtocolTCP,
        ListenPort:      hcloud.Ptr(6443),
        DestinationPort: hcloud.Ptr(6443),
    })
     if err != nil {
        log.Printf("Failed to add service to LB: %v", err)
        // Ignoring error as it might already exist or be handled differently
    }

    return res.LoadBalancer, nil
}
