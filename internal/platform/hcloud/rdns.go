package hcloud

import (
	"context"
	"fmt"
	"net"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// SetServerRDNS configures reverse DNS for a server's IP address.
func (c *RealClient) SetServerRDNS(ctx context.Context, serverID int64, ipAddress, dnsPtr string) error {
	server := &hcloud.Server{ID: serverID}
	return c.setRDNS(ctx, server, serverID, ipAddress, dnsPtr, "server")
}

// SetLoadBalancerRDNS configures reverse DNS for a load balancer's IP address.
func (c *RealClient) SetLoadBalancerRDNS(ctx context.Context, lbID int64, ipAddress, dnsPtr string) error {
	lb := &hcloud.LoadBalancer{ID: lbID}
	return c.setRDNS(ctx, lb, lbID, ipAddress, dnsPtr, "load balancer")
}

// setRDNS is a private helper that sets reverse DNS for any HCloud resource.
// It handles the common logic for both servers and load balancers.
func (c *RealClient) setRDNS(ctx context.Context, resource hcloud.RDNSSupporter, resourceID int64, ipAddress, dnsPtr, resourceType string) error {
	// Parse IP address
	ip := net.ParseIP(ipAddress)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", ipAddress)
	}

	// Call HCloud API to change DNS PTR
	action, _, err := c.client.RDNS.ChangeDNSPtr(ctx, resource, ip, hcloud.Ptr(dnsPtr))
	if err != nil {
		return fmt.Errorf("failed to set RDNS for %s %d (IP: %s → %s): %w", resourceType, resourceID, ipAddress, dnsPtr, err)
	}

	// Wait for action to complete if needed
	if action != nil {
		if err := waitForActions(ctx, c.client, action); err != nil {
			return fmt.Errorf("failed waiting for RDNS action on %s %d: %w", resourceType, resourceID, err)
		}
	}

	return nil
}
