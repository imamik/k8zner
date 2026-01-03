package config

import (
	"encoding/binary"
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
	// #nosec G115
	ipInt = ipInt + uint64(offset)

	// Convert back to IP
	newIP := ipFromBigInt(ipInt, len(ip))

	return fmt.Sprintf("%s/%d", newIP.String(), newMaskSize), nil
}

// CIDRHost calculates a full host IP address for a given network address and host number.
// This mimics the behavior of Terraform's cidrhost function.
// prefix: The network prefix (e.g., "10.0.0.0/16")
// hostnum: The host number to calculate. Can be negative.
func CIDRHost(prefix string, hostnum int) (string, error) {
	_, network, err := net.ParseCIDR(prefix)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR prefix: %w", err)
	}

	maskSize, totalBits := network.Mask.Size()

	// Number of host bits
	hostBits := totalBits - maskSize
	maxHosts := uint64(1) << hostBits

	// Handle negative hostnum
	var offset uint64
	if hostnum < 0 {
		absHostNum := uint64(-hostnum)
		if absHostNum > maxHosts {
			return "", fmt.Errorf("host number %d exceeds max hosts %d", hostnum, maxHosts)
		}
		offset = maxHosts - absHostNum
	} else {
		offset = uint64(hostnum)
		if offset >= maxHosts {
			return "", fmt.Errorf("host number %d exceeds max hosts %d", hostnum, maxHosts)
		}
	}

	ip := network.IP
	if ip.To4() != nil {
		ip = ip.To4()
	}
	ipInt := bigIntFromIP(ip)
	ipInt = ipInt + offset

	newIP := ipFromBigInt(ipInt, len(ip))
	return newIP.String(), nil
}

// Helper to convert IP to uint64.
func bigIntFromIP(ip net.IP) uint64 {
	if len(ip) == 16 {
		// Only supporting IPv4 logic for now as simplified bigInt
		// If IPv6, we need math/big, but standard Hetzner setup uses IPv4 for subnets usually.
		// For robustness, let's assume IPv4 (4 bytes) if possible or convert.
		if ip4 := ip.To4(); ip4 != nil {
			return uint64(binary.BigEndian.Uint32(ip4))
		}
		// Fallback or error for true IPv6 (128 bit) which doesn't fit in uint64.
		// NOTE: This simple implementation only supports IPv4.
		return 0
	}
	return uint64(binary.BigEndian.Uint32(ip))
}

func ipFromBigInt(val uint64, length int) net.IP {
	if length == 4 {
		ip := make(net.IP, 4)
		// #nosec G115
		binary.BigEndian.PutUint32(ip, uint32(val))
		return ip
	}
	// Simplified return for IPv4 mapped
	ip := make(net.IP, 4)
	// #nosec G115
	binary.BigEndian.PutUint32(ip, uint32(val))
	return ip
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
