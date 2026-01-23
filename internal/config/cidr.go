package config

import (
	"encoding/binary"
	"fmt"
	"net"
)

// CIDRSubnet calculates a subnet address given a network address, a netmask size increase, and a subnet number.
// This mimics the behavior of Terraform's cidrsubnet function.
//
// Parameters:
//   - prefix: The network prefix (e.g., "10.0.0.0/16")
//   - newbits: The number of additional bits to add to the prefix length (e.g., 8 for /24 inside /16)
//   - netnum: The zero-based index of the subnet to calculate
//
// Note: Only IPv4 addresses are supported. IPv6 addresses will return an error.
func CIDRSubnet(prefix string, newbits int, netnum int) (string, error) {
	_, network, err := net.ParseCIDR(prefix)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR prefix: %w", err)
	}

	// Validate IPv4
	if network.IP.To4() == nil {
		return "", fmt.Errorf("only IPv4 addresses are supported, got IPv6: %s", prefix)
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
	ipInt += uint64(offset)

	// Convert back to IP
	newIP := ipFromBigInt(ipInt)

	return fmt.Sprintf("%s/%d", newIP.String(), newMaskSize), nil
}

// CIDRHost calculates a full host IP address for a given network address and host number.
// This mimics the behavior of Terraform's cidrhost function.
//
// Parameters:
//   - prefix: The network prefix (e.g., "10.0.0.0/16")
//   - hostnum: The host number to calculate. Can be negative to count from the end
//
// Note: Only IPv4 addresses are supported. IPv6 addresses will return an error.
func CIDRHost(prefix string, hostnum int) (string, error) {
	_, network, err := net.ParseCIDR(prefix)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR prefix: %w", err)
	}

	// Validate IPv4
	if network.IP.To4() == nil {
		return "", fmt.Errorf("only IPv4 addresses are supported, got IPv6: %s", prefix)
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
	ipInt += offset

	newIP := ipFromBigInt(ipInt)
	return newIP.String(), nil
}

// bigIntFromIP converts an IP address to uint64.
// Only supports IPv4 addresses.
func bigIntFromIP(ip net.IP) uint64 {
	if len(ip) == 16 {
		if ip4 := ip.To4(); ip4 != nil {
			return uint64(binary.BigEndian.Uint32(ip4))
		}
		return 0
	}
	return uint64(binary.BigEndian.Uint32(ip))
}

// ipFromBigInt converts a uint64 value back to an IPv4 address.
func ipFromBigInt(val uint64) net.IP {
	ip := make(net.IP, 4)
	// #nosec G115
	binary.BigEndian.PutUint32(ip, uint32(val))
	return ip
}
