package config

import (
	"fmt"
	"net"
)

const (
	// defaultPodSubnetMaskSize is the default pod subnet mask size.
	// Used when calculating the node subnet mask size.
	defaultPodSubnetMaskSize = 24
)

// Node role constants for subnet allocation.
const (
	// RoleControlPlane identifies control plane nodes in subnet allocation.
	RoleControlPlane = "control-plane"
	// RoleLoadBalancer identifies load balancer nodes in subnet allocation.
	RoleLoadBalancer = "load-balancer"
	// RoleWorker identifies worker nodes in subnet allocation.
	RoleWorker = "worker"
	// RoleAutoscaler identifies autoscaler nodes in subnet allocation.
	RoleAutoscaler = "autoscaler"
)

// CalculateSubnets calculates the standard subnets used in k8zner based on the main network CIDR.
func (c *Config) CalculateSubnets() error {
	if c.Network.IPv4CIDR == "" {
		c.Network.IPv4CIDR = "10.0.0.0/16"
	}

	if err := c.calculateNodeSubnet(); err != nil {
		return err
	}

	if err := c.calculateServiceSubnet(); err != nil {
		return err
	}

	if err := c.calculatePodSubnet(); err != nil {
		return err
	}

	if err := c.calculateNodeSubnetMask(); err != nil {
		return err
	}

	return nil
}

// calculateNodeSubnet calculates the node subnet if not already set.
// Default: cidrsubnet(network_ipv4_cidr, 3, 2)
// e.g. 10.0.0.0/16 -> /19, index 2 -> 10.0.64.0/19
func (c *Config) calculateNodeSubnet() error {
	if c.Network.NodeIPv4CIDR != "" {
		return nil
	}

	subnet, err := CIDRSubnet(c.Network.IPv4CIDR, 3, 2)
	if err != nil {
		return fmt.Errorf("failed to calculate node subnet: %w", err)
	}
	c.Network.NodeIPv4CIDR = subnet
	return nil
}

// calculateServiceSubnet calculates the service subnet if not already set.
// Default: cidrsubnet(network_ipv4_cidr, 3, 3)
func (c *Config) calculateServiceSubnet() error {
	if c.Network.ServiceIPv4CIDR != "" {
		return nil
	}

	subnet, err := CIDRSubnet(c.Network.IPv4CIDR, 3, 3)
	if err != nil {
		return fmt.Errorf("failed to calculate service subnet: %w", err)
	}
	c.Network.ServiceIPv4CIDR = subnet
	return nil
}

// calculatePodSubnet calculates the pod subnet if not already set.
// Default: cidrsubnet(network_ipv4_cidr, 1, 1)
func (c *Config) calculatePodSubnet() error {
	if c.Network.PodIPv4CIDR != "" {
		return nil
	}

	subnet, err := CIDRSubnet(c.Network.IPv4CIDR, 1, 1)
	if err != nil {
		return fmt.Errorf("failed to calculate pod subnet: %w", err)
	}
	c.Network.PodIPv4CIDR = subnet
	return nil
}

// calculateNodeSubnetMask calculates the default node subnet mask size if not already set.
// Formula: 32 - (pod_subnet_mask_size - pod_cidr_prefix_length)
// With defaults (/24 pod subnet, /17 pod CIDR): 32 - (24 - 17) = 25
func (c *Config) calculateNodeSubnetMask() error {
	if c.Network.NodeIPv4SubnetMask != 0 {
		return nil
	}

	_, podNet, err := net.ParseCIDR(c.Network.PodIPv4CIDR)
	if err != nil {
		return fmt.Errorf("failed to parse pod CIDR: %w", err)
	}

	podSize, _ := podNet.Mask.Size()
	c.Network.NodeIPv4SubnetMask = 32 - (defaultPodSubnetMaskSize - podSize)

	return nil
}

// GetSubnetForRole calculates the specific subnet for a role (CP, LB, Worker).
func (c *Config) GetSubnetForRole(role string, index int) (string, error) {
	_, nodeNet, err := net.ParseCIDR(c.Network.NodeIPv4CIDR)
	if err != nil {
		return "", fmt.Errorf("failed to parse node CIDR: %w", err)
	}

	nodeSize, _ := nodeNet.Mask.Size()
	newBits := c.Network.NodeIPv4SubnetMask - nodeSize

	subnetIndex, err := calculateSubnetIndex(role, index, newBits)
	if err != nil {
		return "", err
	}

	return CIDRSubnet(c.Network.NodeIPv4CIDR, newBits, subnetIndex)
}

// calculateSubnetIndex determines the subnet index based on role.
// Control Plane: index 0
// Load Balancer: index 1
// Worker: index 2 + index
// Autoscaler: last available subnet
func calculateSubnetIndex(role string, index int, newBits int) (int, error) {
	switch role {
	case RoleControlPlane:
		return 0, nil
	case RoleLoadBalancer:
		return 1, nil
	case RoleWorker:
		return 2 + index, nil
	case RoleAutoscaler:
		return (1 << newBits) - 1, nil
	default:
		return 0, fmt.Errorf("unknown role: %s (valid roles: %s, %s, %s, %s)",
			role, RoleControlPlane, RoleLoadBalancer, RoleWorker, RoleAutoscaler)
	}
}
