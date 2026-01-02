package config

import (
	"fmt"
	"net"
)

// CIDRSubnet calculates a subnet address given a network address, a netmask size increase, and a subnet number.
// This mimics the behavior of Terraform's cidrsubnet function.
// prefix: The network prefix (e.g., "10.0.0.0/16")
// newbits: The number of additional bits to add to the prefix length (e.g., 8 for /24 inside /16)
// netnum: The zero-based index of the subnet to calculate.
func CIDRSubnet(prefix string, newbits int, netnum int) (string, error) {
	_, network, err := net.ParseCIDR(prefix)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR prefix: %w", err)
	}

	maskSize, totalBits := network.Mask.Size()
	newMaskSize := maskSize + newbits

	if newMaskSize > totalBits {
		return "", fmt.Errorf("prefix extension of %d bits is too large for %s", newbits, prefix)
	}

	// Calculate the max number of subnets allowed with newbits
	maxSubnets := 1 << newbits
	if netnum >= maxSubnets {
		return "", fmt.Errorf("subnet number %d exceeds max subnets %d", netnum, maxSubnets)
	}

	// Convert IP to integer
	ip := network.IP
	if ip.To4() != nil {
		ip = ip.To4()
	}

	ipInt := bigIntFromIP(ip)

	// Calculate the size of each subnet
	subnetSize := 1 << (totalBits - newMaskSize)

	// Add the offset
	offset := netnum * subnetSize

	// Add offset to IP
	ipInt = ipInt + uint64(offset)

	// Convert back to IP
	newIP := ipFromBigInt(ipInt, len(ip))

	return fmt.Sprintf("%s/%d", newIP.String(), newMaskSize), nil
}

// Helper to convert IP to uint64
func bigIntFromIP(ip net.IP) uint64 {
	var val uint64
	for _, b := range ip {
		val <<= 8
		val |= uint64(b)
	}
	return val
}

func ipFromBigInt(val uint64, len int) net.IP {
	res := make(net.IP, len)
	for i := len - 1; i >= 0; i-- {
		res[i] = byte(val & 0xff)
		val >>= 8
	}
	return res
}

// CalculateSubnets calculates the standard subnets used in hcloud-k8s based on the main network CIDR.
func (c *Config) CalculateSubnets() error {
	// Defaults if not set
	if c.Network.IPv4CIDR == "" {
		c.Network.IPv4CIDR = "10.0.0.0/16"
	}

	// Calculate Node CIDR if missing
	if c.Network.NodeIPv4CIDR == "" {
		// Default: cidrsubnet(network_ipv4_cidr, 3, 2)
		// e.g. 10.0.0.0/16 -> /19, index 2 -> 10.0.64.0/19
		subnet, err := CIDRSubnet(c.Network.IPv4CIDR, 3, 2)
		if err != nil {
			return err
		}
		c.Network.NodeIPv4CIDR = subnet
	}

	// Calculate Service CIDR if missing
	if c.Network.ServiceIPv4CIDR == "" {
		// Default: cidrsubnet(network_ipv4_cidr, 3, 3)
		subnet, err := CIDRSubnet(c.Network.IPv4CIDR, 3, 3)
		if err != nil {
			return err
		}
		c.Network.ServiceIPv4CIDR = subnet
	}

	// Calculate Pod CIDR if missing
	if c.Network.PodIPv4CIDR == "" {
		// Default: cidrsubnet(network_ipv4_cidr, 1, 1)
		subnet, err := CIDRSubnet(c.Network.IPv4CIDR, 1, 1)
		if err != nil {
			return err
		}
		c.Network.PodIPv4CIDR = subnet
	}

	// Calculate default mask size if missing
	if c.Network.NodeIPv4SubnetMask == 0 {
		// Default logic from terraform:
		// coalesce(var.network_node_ipv4_subnet_mask_size, 32 - (local.network_pod_ipv4_subnet_mask_size - split("/", local.network_pod_ipv4_cidr)[1]))
		// Assuming Pod CIDR is /17 (default), pod mask is 24 (default).
		// 32 - (24 - 17) = 32 - 7 = 25.

		_, podNet, _ := net.ParseCIDR(c.Network.PodIPv4CIDR)
		podSize, _ := podNet.Mask.Size()

		// This logic in terraform is a bit specific.
		// "network_pod_ipv4_subnet_mask_size" in TF defaults to 24.
		podSubnetMaskSize := 24
		c.Network.NodeIPv4SubnetMask = 32 - (podSubnetMaskSize - podSize)
	}

	return nil
}

// GetSubnetForRole calculates the specific subnet for a role (CP, LB, Worker).
func (c *Config) GetSubnetForRole(role string, index int) (string, error) {
	// Logic from network.tf:
	// Control Plane: index 0
	// LB: index 1
	// Worker: index 2 + index

	// We need mask_diff between NodeCIDR and NodeSubnetMask
	_, nodeNet, err := net.ParseCIDR(c.Network.NodeIPv4CIDR)
	if err != nil {
		return "", err
	}
	nodeSize, _ := nodeNet.Mask.Size()
	newBits := c.Network.NodeIPv4SubnetMask - nodeSize

	var subnetIndex int
	switch role {
	case "control-plane":
		subnetIndex = 0
	case "load-balancer":
		subnetIndex = 1
	case "worker":
		subnetIndex = 2 + index
	case "autoscaler":
		// Last available
		subnetIndex = (1 << newBits) - 1
	default:
		return "", fmt.Errorf("unknown role: %s", role)
	}

	return CIDRSubnet(c.Network.NodeIPv4CIDR, newBits, subnetIndex)
}
