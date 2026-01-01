// Package config provides configuration loading and validation logic.
package config

import (
	"errors"
	"fmt"
	"math"
	"net"

	"github.com/mitchellh/mapstructure"
)

// Config holds the application configuration.
type Config struct {
	HCloudToken  string           `mapstructure:"hcloud_token"`
	ClusterName  string           `mapstructure:"cluster_name"`
	Network      NetworkConfig    `mapstructure:"network"`
	Firewall     FirewallConfig   `mapstructure:"firewall"`
	ControlPlane ControlPlane     `mapstructure:"control_plane"`
	Workers      []WorkerNodePool `mapstructure:"workers"`
	Autoscaler   AutoscalerConfig `mapstructure:"autoscaler"`
}

// NetworkConfig defines network settings.
type NetworkConfig struct {
	CIDR                         string `mapstructure:"cidr"`
	Zone                         string `mapstructure:"zone"`
	NodeIPv4CIDR                 string `mapstructure:"node_ipv4_cidr"`
	NodeIPv4SubnetMaskSize       int    `mapstructure:"node_ipv4_subnet_mask_size"`
	PodIPv4CIDR                  string `mapstructure:"pod_ipv4_cidr"`
	ServiceIPv4CIDR              string `mapstructure:"service_ipv4_cidr"`
	NativeRoutingIPv4CIDR        string `mapstructure:"native_routing_ipv4_cidr"`
	ControlPlaneSubnet           string // Calculated
	LoadBalancerSubnet           string // Calculated
	AutoscalerSubnet             string // Calculated
	SkipFirstSubnet              bool   // Calculated
}

// FirewallConfig defines firewall settings.
type FirewallConfig struct {
	APIAllowCIDR []string `mapstructure:"api_allow_cidr"`
}

// ControlPlane defines control plane settings.
type ControlPlane struct {
	ServerType string `mapstructure:"server_type"`
	Count      int    `mapstructure:"count"`
	Location   string `mapstructure:"location"`
	Image      string `mapstructure:"image"`
}

// WorkerNodePool defines a worker node pool.
type WorkerNodePool struct {
	Name       string `mapstructure:"name"`
	ServerType string `mapstructure:"server_type"`
	Count      int    `mapstructure:"count"`
	Location   string `mapstructure:"location"`
	Image      string `mapstructure:"image"`
	Subnet     string // Calculated
}

// AutoscalerConfig defines settings for the cluster autoscaler.
type AutoscalerConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

// Load parses the configuration from a map.
func Load(input map[string]interface{}) (*Config, error) {
	var cfg Config
	if err := mapstructure.Decode(input, &cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	// Set defaults if missing
	if cfg.Network.CIDR == "" {
		cfg.Network.CIDR = "10.0.0.0/16"
	}
	// Add more defaults as per network.tf logic if needed

	return &cfg, nil
}

// Validate checks the configuration for errors.
func Validate(cfg *Config) error {
	if cfg.HCloudToken == "" {
		return errors.New("hcloud_token is required")
	}
	if cfg.ClusterName == "" {
		return errors.New("cluster_name is required")
	}
	return nil
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	return Validate(c)
}

// CalculateSubnets computes the subnets for control plane, LB, and workers based on network.tf logic.
func (c *Config) CalculateSubnets() error {
	// Replicating network.tf logic
	// local.network_ipv4_cidr = var.network_ipv4_cidr (defaults to 10.0.0.0/16)
	networkCIDR := c.Network.CIDR

	// network_node_ipv4_cidr = coalesce(var.network_node_ipv4_cidr, cidrsubnet(local.network_ipv4_cidr, 3, 2))
	nodeCIDR := c.Network.NodeIPv4CIDR
	if nodeCIDR == "" {
		var err error
		nodeCIDR, err = cidrSubnet(networkCIDR, 3, 2)
		if err != nil {
			return fmt.Errorf("failed to calculate node cidr: %w", err)
		}
	}
	c.Network.NodeIPv4CIDR = nodeCIDR

	// network_node_ipv4_cidr_skip_first_subnet = cidrhost(local.network_ipv4_cidr, 0) == cidrhost(local.network_node_ipv4_cidr, 0)
	skipFirstSubnet, err := cidrHostEqual(networkCIDR, nodeCIDR, 0)
	if err != nil {
		return err
	}
	c.Network.SkipFirstSubnet = skipFirstSubnet

	// network_pod_ipv4_subnet_mask_size = 24
	// network_node_ipv4_subnet_mask_size = coalesce(..., 32 - (24 - split("/", local.network_pod_ipv4_cidr)[1]))
	// For simplicity, assuming pod cidr is /16, so 24 - 16 = 8. 32 - 8 = 24.
	// If pod cidr is 10.244.0.0/16, mask is 16. 24 - 16 = 8. 32 - 8 = 24.
	// So node subnets are /24.
	nodeSubnetMaskSize := c.Network.NodeIPv4SubnetMaskSize
	if nodeSubnetMaskSize == 0 {
		nodeSubnetMaskSize = 24 // Simplified default, assuming standard setup
	}

	_, nodeNet, err := net.ParseCIDR(nodeCIDR)
	if err != nil {
		return err
	}
	nodeMaskSize, _ := nodeNet.Mask.Size()
	newBits := nodeSubnetMaskSize - nodeMaskSize

	skipOffset := 0
	if skipFirstSubnet {
		skipOffset = 1
	}

	// Control Plane: index 0
	cpSubnet, err := cidrSubnet(nodeCIDR, newBits, 0+skipOffset)
	if err != nil {
		return err
	}
	c.Network.ControlPlaneSubnet = cpSubnet

	// Load Balancer: index 1
	lbSubnet, err := cidrSubnet(nodeCIDR, newBits, 1+skipOffset)
	if err != nil {
		return err
	}
	c.Network.LoadBalancerSubnet = lbSubnet

	// Workers: index 2 + index
	for i := range c.Workers {
		workerSubnet, err := cidrSubnet(nodeCIDR, newBits, 2+skipOffset+i)
		if err != nil {
			return err
		}
		c.Workers[i].Subnet = workerSubnet
	}

	// Autoscaler: last available subnet
	// pow(2, newBits) - 1
	lastIndex := int(math.Pow(2, float64(newBits))) - 1
	asSubnet, err := cidrSubnet(nodeCIDR, newBits, lastIndex)
	if err != nil {
		return err
	}
	c.Network.AutoscalerSubnet = asSubnet

	return nil
}

// cidrSubnet mimics Terraform's cidrsubnet function.
func cidrSubnet(baseCIDR string, newBits int, num int) (string, error) {
	_, ipNet, err := net.ParseCIDR(baseCIDR)
	if err != nil {
		return "", err
	}

	maskSize, totalBits := ipNet.Mask.Size()
	newMaskSize := maskSize + newBits
	if newMaskSize > totalBits {
		return "", fmt.Errorf("new mask size %d exceeds total bits %d", newMaskSize, totalBits)
	}

	// IPv4 is 32 bits.
	if len(ipNet.IP) == 16 {
		// handle ipv6? Terraform cidrsubnet handles both.
		// For now, assuming IPv4 as per task description.
		return "", errors.New("ipv6 not fully supported in this helper yet")
	}

	ip := ipToInt(ipNet.IP)

	// Calculate the size of the subnet
	subnetSize := uint32(1) << (32 - newMaskSize)

	// Add the offset
	ip += uint32(num) * subnetSize

	newIP := intToIP(ip)
	return fmt.Sprintf("%s/%d", newIP.String(), newMaskSize), nil
}

func ipToInt(ip net.IP) uint32 {
	ip = ip.To4()
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func intToIP(nn uint32) net.IP {
	ip := make(net.IP, 4)
	ip[0] = byte(nn >> 24)
	ip[1] = byte(nn >> 16)
	ip[2] = byte(nn >> 8)
	ip[3] = byte(nn)
	return ip
}

// cidrHostEqual checks if the host at index `hostNum` in cidr1 is the same IP as in cidr2.
func cidrHostEqual(cidr1, cidr2 string, hostNum int) (bool, error) {
	ip1, err := cidrHost(cidr1, hostNum)
	if err != nil {
		return false, err
	}
	ip2, err := cidrHost(cidr2, hostNum)
	if err != nil {
		return false, err
	}
	return ip1.Equal(ip2), nil
}

// cidrHost mimics Terraform's cidrhost function.
func cidrHost(cidr string, hostNum int) (net.IP, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	ip := ipToInt(ipNet.IP)

	// hostNum can be negative
	if hostNum < 0 {
		// Determine size of subnet
		maskSize, _ := ipNet.Mask.Size()
		subnetSize := uint32(1) << (32 - maskSize)
		ip += subnetSize + uint32(hostNum)
	} else {
		ip += uint32(hostNum)
	}

	return intToIP(ip), nil
}
