package hcloud

import (
	"context"
	"fmt"
	"net"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// SetServerRDNS sets reverse DNS for a server's IP address.
func (c *RealClient) SetServerRDNS(ctx context.Context, serverID int64, ipAddress, dnsPtr string) error {
	server := &hcloud.Server{ID: serverID}

	// Parse IP address
	ip := net.ParseIP(ipAddress)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", ipAddress)
	}

	// Set RDNS
	action, _, err := c.client.RDNS.ChangeDNSPtr(ctx, server, ip, hcloud.Ptr(dnsPtr))
	if err != nil {
		return fmt.Errorf("failed to set server RDNS: %w", err)
	}

	// Wait for action to complete
	if action != nil {
		if err := waitForActions(ctx, c.client, action); err != nil {
			return fmt.Errorf("failed waiting for server RDNS action: %w", err)
		}
	}

	return nil
}

// SetLoadBalancerRDNS sets reverse DNS for a load balancer's IP address.
func (c *RealClient) SetLoadBalancerRDNS(ctx context.Context, lbID int64, ipAddress, dnsPtr string) error {
	lb := &hcloud.LoadBalancer{ID: lbID}

	// Parse IP address
	ip := net.ParseIP(ipAddress)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", ipAddress)
	}

	// Set RDNS
	action, _, err := c.client.RDNS.ChangeDNSPtr(ctx, lb, ip, hcloud.Ptr(dnsPtr))
	if err != nil {
		return fmt.Errorf("failed to set load balancer RDNS: %w", err)
	}

	// Wait for action to complete
	if action != nil {
		if err := waitForActions(ctx, c.client, action); err != nil {
			return fmt.Errorf("failed waiting for load balancer RDNS action: %w", err)
		}
	}

	return nil
}
