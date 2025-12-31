package fakes

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// FakeNetworkClient simulates hcloud.NetworkClient
type FakeNetworkClient struct {
	mu       sync.Mutex
	Networks map[int64]*hcloud.Network
	nextID   int64
}

func NewFakeNetworkClient() *FakeNetworkClient {
	return &FakeNetworkClient{
		Networks: make(map[int64]*hcloud.Network),
		nextID:   1,
	}
}

func (f *FakeNetworkClient) GetByID(ctx context.Context, id int64) (*hcloud.Network, *hcloud.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n, ok := f.Networks[id]; ok {
		return n, nil, nil
	}
	return nil, nil, fmt.Errorf("not found")
}

func (f *FakeNetworkClient) GetByName(ctx context.Context, name string) (*hcloud.Network, *hcloud.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range f.Networks {
		if n.Name == name {
			return n, nil, nil
		}
	}
	return nil, nil, nil // Return nil if not found, like real client does usually (or err is nil)
}

func (f *FakeNetworkClient) Create(ctx context.Context, opts hcloud.NetworkCreateOpts) (*hcloud.Network, *hcloud.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.nextID
	f.nextID++
	n := &hcloud.Network{
		ID:      id,
		Name:    opts.Name,
		IPRange: opts.IPRange,
	}
	f.Networks[id] = n
	return n, nil, nil
}

func (f *FakeNetworkClient) Delete(ctx context.Context, network *hcloud.Network) (*hcloud.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Networks, network.ID)
	return nil, nil
}

// FakeLoadBalancerClient simulates hcloud.LoadBalancerClient
type FakeLoadBalancerClient struct {
	mu            sync.Mutex
	LoadBalancers map[int64]*hcloud.LoadBalancer
	nextID        int64
}

func NewFakeLoadBalancerClient() *FakeLoadBalancerClient {
	return &FakeLoadBalancerClient{
		LoadBalancers: make(map[int64]*hcloud.LoadBalancer),
		nextID:        1,
	}
}

func (f *FakeLoadBalancerClient) GetByName(ctx context.Context, name string) (*hcloud.LoadBalancer, *hcloud.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, lb := range f.LoadBalancers {
		if lb.Name == name {
			return lb, nil, nil
		}
	}
	return nil, nil, nil
}

func (f *FakeLoadBalancerClient) Create(ctx context.Context, opts hcloud.LoadBalancerCreateOpts) (hcloud.LoadBalancerCreateResult, *hcloud.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.nextID
	f.nextID++

    // Simulate public IP creation
    ipv4 := net.ParseIP("1.2.3.4")

	lb := &hcloud.LoadBalancer{
		ID:   id,
		Name: opts.Name,
        PublicNet: hcloud.LoadBalancerPublicNet{
            IPv4: hcloud.LoadBalancerPublicNetIPv4{IP: ipv4},
        },
        LoadBalancerType: opts.LoadBalancerType,
        Location: opts.Location,
	}
	f.LoadBalancers[id] = lb

    // Create action
    action := &hcloud.Action{ID: id * 100, Status: hcloud.ActionStatusSuccess}

	return hcloud.LoadBalancerCreateResult{LoadBalancer: lb, Action: action}, nil, nil
}

func (f *FakeLoadBalancerClient) AddService(ctx context.Context, lb *hcloud.LoadBalancer, opts hcloud.LoadBalancerAddServiceOpts) (*hcloud.Action, *hcloud.Response, error) {
	return &hcloud.Action{ID: 1, Status: hcloud.ActionStatusSuccess}, nil, nil
}

func (f *FakeLoadBalancerClient) AddServerTarget(ctx context.Context, lb *hcloud.LoadBalancer, opts hcloud.LoadBalancerAddServerTargetOpts) (*hcloud.Action, *hcloud.Response, error) {
	return &hcloud.Action{ID: 2, Status: hcloud.ActionStatusSuccess}, nil, nil
}

// FakeServerClient simulates hcloud.ServerClient
type FakeServerClient struct {
	mu      sync.Mutex
	Servers map[int64]*hcloud.Server
	nextID  int64
}

func NewFakeServerClient() *FakeServerClient {
	return &FakeServerClient{
		Servers: make(map[int64]*hcloud.Server),
		nextID:  1,
	}
}

func (f *FakeServerClient) GetByID(ctx context.Context, id int64) (*hcloud.Server, *hcloud.Response, error) {
    f.mu.Lock()
    defer f.mu.Unlock()
    if s, ok := f.Servers[id]; ok {
        return s, nil, nil
    }
    return nil, nil, fmt.Errorf("not found")
}

func (f *FakeServerClient) GetByName(ctx context.Context, name string) (*hcloud.Server, *hcloud.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.Servers {
		if s.Name == name {
			return s, nil, nil
		}
	}
	return nil, nil, nil
}

func (f *FakeServerClient) Create(ctx context.Context, opts hcloud.ServerCreateOpts) (hcloud.ServerCreateResult, *hcloud.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.nextID
	f.nextID++

	s := &hcloud.Server{
		ID:         id,
		Name:       opts.Name,
		ServerType: opts.ServerType,
        PublicNet: hcloud.ServerPublicNet{
            IPv4: hcloud.ServerPublicNetIPv4{
                IP: net.ParseIP("10.0.0.1"),
            },
        },
        PrivateNet: []hcloud.ServerPrivateNet{
            {IP: net.ParseIP("10.0.0.5")},
        },
	}
	f.Servers[id] = s

    action := &hcloud.Action{ID: id * 100, Status: hcloud.ActionStatusSuccess}
	return hcloud.ServerCreateResult{Server: s, Action: action}, nil, nil
}

func (f *FakeServerClient) Delete(ctx context.Context, server *hcloud.Server) (*hcloud.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Servers, server.ID)
	return nil, nil
}

func (f *FakeServerClient) EnableRescue(ctx context.Context, server *hcloud.Server, opts hcloud.ServerEnableRescueOpts) (hcloud.ServerEnableRescueResult, *hcloud.Response, error) {
    return hcloud.ServerEnableRescueResult{Action: &hcloud.Action{ID: 1, Status: hcloud.ActionStatusSuccess}}, nil, nil
}

func (f *FakeServerClient) Reset(ctx context.Context, server *hcloud.Server) (*hcloud.Action, *hcloud.Response, error) {
    return &hcloud.Action{ID: 1, Status: hcloud.ActionStatusSuccess}, nil, nil
}

func (f *FakeServerClient) CreateImage(ctx context.Context, server *hcloud.Server, opts *hcloud.ServerCreateImageOpts) (hcloud.ServerCreateImageResult, *hcloud.Response, error) {
    return hcloud.ServerCreateImageResult{
        Image: &hcloud.Image{ID: 999},
        Action: &hcloud.Action{ID: 1, Status: hcloud.ActionStatusSuccess},
    }, nil, nil
}

// FakeImageClient simulates hcloud.ImageClient
type FakeImageClient struct {
    Images []*hcloud.Image
}

func NewFakeImageClient() *FakeImageClient {
    return &FakeImageClient{
        Images: []*hcloud.Image{},
    }
}

func (f *FakeImageClient) GetByID(ctx context.Context, id int64) (*hcloud.Image, *hcloud.Response, error) {
    for _, img := range f.Images {
        if img.ID == id {
            return img, nil, nil
        }
    }
    // Return dummy image if list is empty to avoid blocking tests that need "an image"
    // Ideally we populate this in tests.
    return &hcloud.Image{ID: id}, nil, nil
}

func (f *FakeImageClient) GetByName(ctx context.Context, name string) (*hcloud.Image, *hcloud.Response, error) {
    return nil, nil, nil
}

func (f *FakeImageClient) All(ctx context.Context) ([]*hcloud.Image, error) {
    return f.Images, nil
}

func (f *FakeImageClient) AllWithOpts(ctx context.Context, opts hcloud.ImageListOpts) ([]*hcloud.Image, error) {
    return f.Images, nil
}

// FakeFirewallClient simulates hcloud.FirewallClient
type FakeFirewallClient struct {
    Firewalls map[int64]*hcloud.Firewall
    nextID int64
}

func NewFakeFirewallClient() *FakeFirewallClient {
    return &FakeFirewallClient{
        Firewalls: make(map[int64]*hcloud.Firewall),
        nextID: 1,
    }
}

func (f *FakeFirewallClient) GetByName(ctx context.Context, name string) (*hcloud.Firewall, *hcloud.Response, error) {
    for _, fw := range f.Firewalls {
        if fw.Name == name {
            return fw, nil, nil
        }
    }
    return nil, nil, nil
}

func (f *FakeFirewallClient) GetByID(ctx context.Context, id int64) (*hcloud.Firewall, *hcloud.Response, error) {
    if fw, ok := f.Firewalls[id]; ok {
        return fw, nil, nil
    }
    return nil, nil, nil
}

func (f *FakeFirewallClient) Create(ctx context.Context, opts hcloud.FirewallCreateOpts) (hcloud.FirewallCreateResult, *hcloud.Response, error) {
    id := f.nextID
    f.nextID++
    fw := &hcloud.Firewall{ID: id, Name: opts.Name}
    f.Firewalls[id] = fw
    return hcloud.FirewallCreateResult{Firewall: fw}, nil, nil
}

func (f *FakeFirewallClient) Delete(ctx context.Context, firewall *hcloud.Firewall) (*hcloud.Response, error) {
    delete(f.Firewalls, firewall.ID)
    return nil, nil
}

// FakePlacementGroupClient
type FakePlacementGroupClient struct {
    PGs map[int64]*hcloud.PlacementGroup
    nextID int64
}

func NewFakePlacementGroupClient() *FakePlacementGroupClient {
    return &FakePlacementGroupClient{
        PGs: make(map[int64]*hcloud.PlacementGroup),
        nextID: 1,
    }
}

func (f *FakePlacementGroupClient) GetByName(ctx context.Context, name string) (*hcloud.PlacementGroup, *hcloud.Response, error) {
    for _, pg := range f.PGs {
        if pg.Name == name {
            return pg, nil, nil
        }
    }
    return nil, nil, nil
}

func (f *FakePlacementGroupClient) GetByID(ctx context.Context, id int64) (*hcloud.PlacementGroup, *hcloud.Response, error) {
    if pg, ok := f.PGs[id]; ok {
        return pg, nil, nil
    }
    return nil, nil, nil
}

func (f *FakePlacementGroupClient) Create(ctx context.Context, opts hcloud.PlacementGroupCreateOpts) (hcloud.PlacementGroupCreateResult, *hcloud.Response, error) {
    id := f.nextID
    f.nextID++
    pg := &hcloud.PlacementGroup{ID: id, Name: opts.Name, Type: opts.Type}
    f.PGs[id] = pg
    return hcloud.PlacementGroupCreateResult{PlacementGroup: pg}, nil, nil
}

// FakeActionClient
type FakeActionClient struct {}

func (f *FakeActionClient) GetByID(ctx context.Context, id int64) (*hcloud.Action, *hcloud.Response, error) {
    return &hcloud.Action{ID: id, Status: hcloud.ActionStatusSuccess}, nil, nil
}
