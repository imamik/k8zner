package hcloud

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// RealClient implements InfrastructureManager using the Hetzner Cloud API.
type RealClient struct {
	client *hcloud.Client
}

// NewRealClient creates a new RealClient.
func NewRealClient(token string) *RealClient {
	return &RealClient{
		client: hcloud.NewClient(hcloud.WithToken(token)),
	}
}

// --- ServerProvisioner ---

// CreateServer creates a new server with the given specifications.
func (c *RealClient) CreateServer(ctx context.Context, name, imageType, serverType, location string, sshKeys []string, labels map[string]string, userData string) (string, error) {
	serverTypeObj, _, err := c.client.ServerType.Get(ctx, serverType)
	if err != nil {
		return "", fmt.Errorf("failed to get server type: %w", err)
	}
	if serverTypeObj == nil {
		return "", fmt.Errorf("server type not found: %s", serverType)
	}

	// Try to get image by name first.
	imageObj, _, err := c.client.Image.Get(ctx, imageType) //nolint:staticcheck
	if err != nil {
		return "", fmt.Errorf("failed to get image: %w", err)
	}

	// Check if image architecture matches server type architecture.
	if imageObj != nil && imageObj.Architecture != serverTypeObj.Architecture {
		images, _, err := c.client.Image.List(ctx, hcloud.ImageListOpts{
			Name:         imageType,
			Architecture: []hcloud.Architecture{serverTypeObj.Architecture},
		})
		if err != nil {
			return "", fmt.Errorf("failed to list images: %w", err)
		}
		if len(images) > 0 {
			imageObj = images[0]
		}
	}

	// Special handling for debian-12
	if imageObj == nil {
		if imageType == "debian-12" {
			images, _, err := c.client.Image.List(ctx, hcloud.ImageListOpts{
				Name:         "debian-12",
				Architecture: []hcloud.Architecture{serverTypeObj.Architecture},
			})
			if err != nil {
				return "", fmt.Errorf("failed to list images: %w", err)
			}
			if len(images) > 0 {
				imageObj = images[0]
			}
		}
	}

	if imageObj == nil {
		return "", fmt.Errorf("image not found: %s", imageType)
	}

	var sshKeyObjs []*hcloud.SSHKey
	for _, key := range sshKeys {
		keyObj, _, err := c.client.SSHKey.Get(ctx, key)
		if err != nil {
			return "", fmt.Errorf("failed to get ssh key %s: %w", key, err)
		}
		if keyObj == nil {
			return "", fmt.Errorf("ssh key not found: %s", key)
		}
		sshKeyObjs = append(sshKeyObjs, keyObj)
	}

	var locObj *hcloud.Location
	if location != "" {
		locObj, _, err = c.client.Location.Get(ctx, location)
		if err != nil {
			return "", fmt.Errorf("failed to get location %s: %w", location, err)
		}
		if locObj == nil {
			return "", fmt.Errorf("location not found: %s", location)
		}
	}

	opts := hcloud.ServerCreateOpts{
		Name:       name,
		ServerType: serverTypeObj,
		Image:      imageObj,
		SSHKeys:    sshKeyObjs,
		Labels:     labels,
		UserData:   userData,
		Location:   locObj,
	}

	result, _, err := c.client.Server.Create(ctx, opts)
	if err != nil {
		return "", fmt.Errorf("failed to create server: %w", err)
	}

	if err := c.client.Action.WaitFor(ctx, result.Action); err != nil {
		return "", fmt.Errorf("failed to wait for server creation: %w", err)
	}

	return fmt.Sprintf("%d", result.Server.ID), nil
}

// DeleteServer deletes the server with the given name.
func (c *RealClient) DeleteServer(ctx context.Context, name string) error {
	server, _, err := c.client.Server.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get server: %w", err)
	}
	if server == nil {
		return nil // Server already deleted.
	}

	_, err = c.client.Server.Delete(ctx, server) //nolint:staticcheck
	if err != nil {
		return fmt.Errorf("failed to delete server: %w", err)
	}

	return nil
}

// GetServerIP returns the public IP of the server.
func (c *RealClient) GetServerIP(ctx context.Context, name string) (string, error) {
	server, _, err := c.client.Server.Get(ctx, name)
	if err != nil {
		return "", fmt.Errorf("failed to get server: %w", err)
	}
	if server == nil {
		return "", fmt.Errorf("server not found: %s", name)
	}

	if server.PublicNet.IPv4.IP == nil {
		return "", fmt.Errorf("server has no public IPv4")
	}

	return server.PublicNet.IPv4.IP.String(), nil
}

// EnableRescue enables rescue mode for the server.
func (c *RealClient) EnableRescue(ctx context.Context, serverID string, sshKeyIDs []string) (string, error) {
	id, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid server id: %s", serverID)
	}
	server := &hcloud.Server{ID: id}

	var sshKeys []*hcloud.SSHKey
	for _, kid := range sshKeyIDs {
		kidInt, err := strconv.ParseInt(kid, 10, 64)
		if err != nil {
			continue // Ignore invalid.
		}
		sshKeys = append(sshKeys, &hcloud.SSHKey{ID: kidInt})
	}

	result, _, err := c.client.Server.EnableRescue(ctx, server, hcloud.ServerEnableRescueOpts{
		SSHKeys: sshKeys,
	})
	if err != nil {
		return "", fmt.Errorf("failed to enable rescue: %w", err)
	}

	if err := c.client.Action.WaitFor(ctx, result.Action); err != nil {
		return "", fmt.Errorf("failed to wait for rescue enable: %w", err)
	}

	return result.RootPassword, nil
}

// ResetServer resets (reboots) the server.
func (c *RealClient) ResetServer(ctx context.Context, serverID string) error {
	id, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid server id: %s", serverID)
	}
	server := &hcloud.Server{ID: id}

	result, _, err := c.client.Server.Reset(ctx, server)
	if err != nil {
		return fmt.Errorf("failed to reset server: %w", err)
	}

	if err := c.client.Action.WaitFor(ctx, result); err != nil {
		return fmt.Errorf("failed to wait for reset: %w", err)
	}
	return nil
}

// PoweroffServer shuts down the server.
func (c *RealClient) PoweroffServer(ctx context.Context, serverID string) error {
	id, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid server id: %s", serverID)
	}
	server := &hcloud.Server{ID: id}

	result, _, err := c.client.Server.Poweroff(ctx, server)
	if err != nil {
		return fmt.Errorf("failed to poweroff server: %w", err)
	}

	if err := c.client.Action.WaitFor(ctx, result); err != nil {
		return fmt.Errorf("failed to wait for poweroff: %w", err)
	}
	return nil
}

// GetServerID returns the ID of the server by name.
func (c *RealClient) GetServerID(ctx context.Context, name string) (string, error) {
	server, _, err := c.client.Server.Get(ctx, name)
	if err != nil {
		return "", fmt.Errorf("failed to get server: %w", err)
	}
	if server == nil {
		return "", fmt.Errorf("server not found: %s", name)
	}
	return fmt.Sprintf("%d", server.ID), nil
}

// --- SnapshotManager ---

// CreateSnapshot creates a snapshot of the server.
func (c *RealClient) CreateSnapshot(ctx context.Context, serverID, snapshotDescription string) (string, error) {
	id, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid server id: %s", serverID)
	}
	server := &hcloud.Server{ID: id}

	result, _, err := c.client.Server.CreateImage(ctx, server, &hcloud.ServerCreateImageOpts{
		Type:        hcloud.ImageTypeSnapshot,
		Description: &snapshotDescription,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create snapshot: %w", err)
	}

	if err := c.client.Action.WaitFor(ctx, result.Action); err != nil {
		return "", fmt.Errorf("failed to wait for snapshot creation: %w", err)
	}

	return fmt.Sprintf("%d", result.Image.ID), nil
}

// DeleteImage deletes an image by ID.
func (c *RealClient) DeleteImage(ctx context.Context, imageID string) error {
	id, err := strconv.ParseInt(imageID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid image id: %s", imageID)
	}
	image := &hcloud.Image{ID: id}

	_, err = c.client.Image.Delete(ctx, image)
	if err != nil {
		return fmt.Errorf("failed to delete image: %w", err)
	}
	return nil
}

// --- SSHKeyManager ---

// CreateSSHKey creates a new SSH key.
func (c *RealClient) CreateSSHKey(ctx context.Context, name, publicKey string) (string, error) {
	opts := hcloud.SSHKeyCreateOpts{
		Name:      name,
		PublicKey: publicKey,
	}
	key, _, err := c.client.SSHKey.Create(ctx, opts)
	if err != nil {
		return "", fmt.Errorf("failed to create ssh key: %w", err)
	}
	return fmt.Sprintf("%d", key.ID), nil
}

// DeleteSSHKey deletes the SSH key with the given name.
func (c *RealClient) DeleteSSHKey(ctx context.Context, name string) error {
	key, _, err := c.client.SSHKey.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get ssh key: %w", err)
	}
	if key == nil {
		return nil
	}
	_, err = c.client.SSHKey.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to delete ssh key: %w", err)
	}
	return nil
}

// --- NetworkManager ---

func (c *RealClient) EnsureNetwork(ctx context.Context, name, ipRange, zone string, labels map[string]string) (*hcloud.Network, error) {
	network, _, err := c.client.Network.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get network: %w", err)
	}

	if network != nil {
		// Verify IP Range matches
		if network.IPRange.String() != ipRange {
			return nil, fmt.Errorf("network %s exists but with different IP range %s (expected %s)", name, network.IPRange.String(), ipRange)
		}
		// TODO: Update labels if needed
		return network, nil
	}

	// Create
	_, ipNet, err := net.ParseCIDR(ipRange)
	if err != nil {
		return nil, fmt.Errorf("invalid ip range: %w", err)
	}

	opts := hcloud.NetworkCreateOpts{
		Name:    name,
		IPRange: ipNet,
		Labels:  labels,
	}
	network, _, err = c.client.Network.Create(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create network: %w", err)
	}

	return network, nil
}

func (c *RealClient) EnsureSubnet(ctx context.Context, network *hcloud.Network, ipRange, networkZone string, subnetType hcloud.NetworkSubnetType) error {
	// Check if subnet exists
	for _, subnet := range network.Subnets {
		if subnet.IPRange.String() == ipRange {
			return nil // Exists
		}
	}

	// Create Subnet
	_, ipNet, err := net.ParseCIDR(ipRange)
	if err != nil {
		return fmt.Errorf("invalid subnet ip range: %w", err)
	}

	opts := hcloud.NetworkAddSubnetOpts{
		Subnet: hcloud.NetworkSubnet{
			Type:        subnetType,
			IPRange:     ipNet,
			NetworkZone: hcloud.NetworkZone(networkZone),
		},
	}

	action, _, err := c.client.Network.AddSubnet(ctx, network, opts)
	if err != nil {
		return fmt.Errorf("failed to add subnet: %w", err)
	}

	if err := c.client.Action.WaitFor(ctx, action); err != nil {
		return fmt.Errorf("failed to wait for subnet creation: %w", err)
	}

	return nil
}

func (c *RealClient) DeleteNetwork(ctx context.Context, name string) error {
	network, _, err := c.client.Network.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get network: %w", err)
	}
	if network == nil {
		return nil
	}
	_, err = c.client.Network.Delete(ctx, network)
	return err
}

func (c *RealClient) GetNetwork(ctx context.Context, name string) (*hcloud.Network, error) {
	network, _, err := c.client.Network.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	return network, nil
}


// --- FirewallManager ---

func (c *RealClient) EnsureFirewall(ctx context.Context, name string, rules []hcloud.FirewallRule, labels map[string]string) (*hcloud.Firewall, error) {
	fw, _, err := c.client.Firewall.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get firewall: %w", err)
	}

	if fw != nil {
		// Update Rules
		// We use SetRules to overwrite existing rules with the desired state
		actions, _, err := c.client.Firewall.SetRules(ctx, fw, hcloud.FirewallSetRulesOpts{
			Rules: rules,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to set firewall rules: %w", err)
		}
		if err := c.client.Action.WaitFor(ctx, actions...); err != nil {
			return nil, fmt.Errorf("failed to wait for firewall rules update: %w", err)
		}
		return fw, nil
	}

	// Create
	opts := hcloud.FirewallCreateOpts{
		Name:   name,
		Rules:  rules,
		Labels: labels,
	}
	res, _, err := c.client.Firewall.Create(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create firewall: %w", err)
	}

	if err := c.client.Action.WaitFor(ctx, res.Actions...); err != nil {
		return nil, fmt.Errorf("failed to wait for firewall creation: %w", err)
	}

	return res.Firewall, nil
}

func (c *RealClient) DeleteFirewall(ctx context.Context, name string) error {
	fw, _, err := c.client.Firewall.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get firewall: %w", err)
	}
	if fw == nil {
		return nil
	}
	_, err = c.client.Firewall.Delete(ctx, fw)
	return err
}

func (c *RealClient) GetFirewall(ctx context.Context, name string) (*hcloud.Firewall, error) {
	fw, _, err := c.client.Firewall.Get(ctx, name)
	return fw, err
}

// --- LoadBalancerManager ---

func (c *RealClient) EnsureLoadBalancer(ctx context.Context, name, location, lbType string, algorithm hcloud.LoadBalancerAlgorithmType, labels map[string]string) (*hcloud.LoadBalancer, error) {
	lb, _, err := c.client.LoadBalancer.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get lb: %w", err)
	}

	if lb != nil {
		// Check if updates needed (omitted for brevity, can implement update logic)
		return lb, nil
	}

	// Create
	lbTypeObj, _, err := c.client.LoadBalancerType.Get(ctx, lbType)
	if err != nil {
		return nil, fmt.Errorf("failed to get lb type: %w", err)
	}
	locObj, _, err := c.client.Location.Get(ctx, location)
	if err != nil {
		return nil, fmt.Errorf("failed to get location: %w", err)
	}

	opts := hcloud.LoadBalancerCreateOpts{
		Name:             name,
		LoadBalancerType: lbTypeObj,
		Location:         locObj,
		Algorithm:        &hcloud.LoadBalancerAlgorithm{Type: algorithm},
		Labels:           labels,
	}

	res, _, err := c.client.LoadBalancer.Create(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create lb: %w", err)
	}
	if err := c.client.Action.WaitFor(ctx, res.Action); err != nil {
		return nil, fmt.Errorf("failed to wait for lb creation: %w", err)
	}

	return res.LoadBalancer, nil
}

func (c *RealClient) ConfigureService(ctx context.Context, lb *hcloud.LoadBalancer, service hcloud.LoadBalancerAddServiceOpts) error {
	// Check if service exists
	if service.ListenPort == nil {
		return fmt.Errorf("listen port is nil")
	}

	for _, s := range lb.Services {
		if s.ListenPort == *service.ListenPort {
			// Update? For now we assume idempotency means "ensure it matches".
			return nil
		}
	}

	action, _, err := c.client.LoadBalancer.AddService(ctx, lb, service)
	if err != nil {
		return fmt.Errorf("failed to add service: %w", err)
	}
	return c.client.Action.WaitFor(ctx, action)
}

func (c *RealClient) AttachToNetwork(ctx context.Context, lb *hcloud.LoadBalancer, network *hcloud.Network, ip net.IP) error {
	// Check if already attached
	for _, privateNet := range lb.PrivateNet {
		if privateNet.Network.ID == network.ID {
			return nil
		}
	}

	opts := hcloud.LoadBalancerAttachToNetworkOpts{
		Network: network,
		IP:      ip,
	}
	action, _, err := c.client.LoadBalancer.AttachToNetwork(ctx, lb, opts)
	if err != nil {
		return fmt.Errorf("failed to attach lb to network: %w", err)
	}
	return c.client.Action.WaitFor(ctx, action)
}

func (c *RealClient) DeleteLoadBalancer(ctx context.Context, name string) error {
	lb, _, err := c.client.LoadBalancer.Get(ctx, name)
	if err != nil {
		return err
	}
	if lb == nil {
		return nil
	}
	_, err = c.client.LoadBalancer.Delete(ctx, lb)
	return err
}

func (c *RealClient) GetLoadBalancer(ctx context.Context, name string) (*hcloud.LoadBalancer, error) {
	lb, _, err := c.client.LoadBalancer.Get(ctx, name)
	return lb, err
}

// --- PlacementGroupManager ---

func (c *RealClient) EnsurePlacementGroup(ctx context.Context, name, pgType string, labels map[string]string) (*hcloud.PlacementGroup, error) {
	pg, _, err := c.client.PlacementGroup.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if pg != nil {
		return pg, nil
	}

	opts := hcloud.PlacementGroupCreateOpts{
		Name:   name,
		Type:   hcloud.PlacementGroupType(pgType),
		Labels: labels,
	}
	res, _, err := c.client.PlacementGroup.Create(ctx, opts)
	if err != nil {
		return nil, err
	}
	return res.PlacementGroup, nil
}

func (c *RealClient) DeletePlacementGroup(ctx context.Context, name string) error {
	pg, _, err := c.client.PlacementGroup.Get(ctx, name)
	if err != nil {
		return err
	}
	if pg == nil {
		return nil
	}
	_, err = c.client.PlacementGroup.Delete(ctx, pg)
	return err
}

func (c *RealClient) GetPlacementGroup(ctx context.Context, name string) (*hcloud.PlacementGroup, error) {
	pg, _, err := c.client.PlacementGroup.Get(ctx, name)
	return pg, err
}

// --- FloatingIPManager ---

func (c *RealClient) EnsureFloatingIP(ctx context.Context, name, homeLocation, ipType string, labels map[string]string) (*hcloud.FloatingIP, error) {
	fip, _, err := c.client.FloatingIP.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if fip != nil {
		return fip, nil
	}

	loc, _, err := c.client.Location.Get(ctx, homeLocation)
	if err != nil {
		return nil, err
	}

	opts := hcloud.FloatingIPCreateOpts{
		Name:         &name,
		Type:         hcloud.FloatingIPType(ipType),
		HomeLocation: loc,
		Labels:       labels,
	}
	res, _, err := c.client.FloatingIP.Create(ctx, opts)
	if err != nil {
		return nil, err
	}
	return res.FloatingIP, nil
}

func (c *RealClient) DeleteFloatingIP(ctx context.Context, name string) error {
	fip, _, err := c.client.FloatingIP.Get(ctx, name)
	if err != nil {
		return err
	}
	if fip == nil {
		return nil
	}
	_, err = c.client.FloatingIP.Delete(ctx, fip)
	return err
}

func (c *RealClient) GetFloatingIP(ctx context.Context, name string) (*hcloud.FloatingIP, error) {
	fip, _, err := c.client.FloatingIP.Get(ctx, name)
	return fip, err
}

// Helper for public IP detection
func (c *RealClient) GetPublicIP(ctx context.Context) (string, error) {
	// Simple HTTP request to icanhazip.com
	req, err := http.NewRequestWithContext(ctx, "GET", "https://ipv4.icanhazip.com", nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}
