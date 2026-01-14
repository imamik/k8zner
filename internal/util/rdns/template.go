// Package rdns provides utilities for Reverse DNS (RDNS) template rendering.
package rdns

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

// TemplateVars holds variables for RDNS template substitution.
type TemplateVars struct {
	ClusterDomain string // {{ cluster-domain }}
	ClusterName   string // {{ cluster-name }}
	Hostname      string // {{ hostname }}
	ID            int64  // {{ id }}
	IPAddress     string // Used to generate {{ ip-labels }}
	IPType        string // {{ ip-type }}: "ipv4" or "ipv6"
	Pool          string // {{ pool }}
	Role          string // {{ role }}
}

// RenderTemplate applies template variable substitution to an RDNS template.
func RenderTemplate(template string, vars TemplateVars) (string, error) {
	if template == "" {
		return "", nil
	}

	result := template

	// Generate IP labels if IP address is provided
	ipLabels := ""
	if vars.IPAddress != "" {
		var err error
		ipLabels, err = generateIPLabels(vars.IPAddress)
		if err != nil {
			return "", fmt.Errorf("failed to generate IP labels: %w", err)
		}
	}

	// Define replacement patterns
	replacements := map[string]string{
		"{{ cluster-domain }}": vars.ClusterDomain,
		"{{ cluster-name }}":   vars.ClusterName,
		"{{ hostname }}":       vars.Hostname,
		"{{ id }}":             fmt.Sprintf("%d", vars.ID),
		"{{ ip-labels }}":      ipLabels,
		"{{ ip-type }}":        vars.IPType,
		"{{ pool }}":           vars.Pool,
		"{{ role }}":           vars.Role,
	}

	// Apply all replacements
	for pattern, value := range replacements {
		result = strings.ReplaceAll(result, pattern, value)
	}

	// Check for any remaining template variables (indicates missing var)
	if hasUnresolvedTemplates(result) {
		return "", fmt.Errorf("unresolved template variables in: %s", result)
	}

	return result, nil
}

// generateIPLabels creates reverse IP label notation for PTR records.
// IPv4: 1.2.3.4 → 4.3.2.1
// IPv6: 2001:db8::1 → 1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2
func generateIPLabels(ipAddr string) (string, error) {
	ip := net.ParseIP(ipAddr)
	if ip == nil {
		return "", fmt.Errorf("invalid IP address: %s", ipAddr)
	}

	// Check if IPv4
	if ip.To4() != nil {
		return reverseIPv4(ip.To4().String()), nil
	}

	// IPv6
	return reverseIPv6(ip), nil
}

// reverseIPv4 reverses IPv4 octets.
// Example: 1.2.3.4 → 4.3.2.1
func reverseIPv4(ipv4 string) string {
	parts := strings.Split(ipv4, ".")
	// Reverse the slice
	for i := 0; i < len(parts)/2; i++ {
		j := len(parts) - 1 - i
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, ".")
}

// reverseIPv6 expands and reverses IPv6 address into nibble notation.
// Example: 2001:db8::1 → 1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2
func reverseIPv6(ip net.IP) string {
	// Expand IPv6 to full 16-byte representation
	expanded := expandIPv6(ip)

	// Convert to nibble notation (each hex digit)
	var nibbles []string
	for _, hexChar := range expanded {
		if hexChar != ':' {
			nibbles = append(nibbles, string(hexChar))
		}
	}

	// Reverse the nibbles
	for i := 0; i < len(nibbles)/2; i++ {
		j := len(nibbles) - 1 - i
		nibbles[i], nibbles[j] = nibbles[j], nibbles[i]
	}

	return strings.Join(nibbles, ".")
}

// expandIPv6 expands IPv6 address to full form without :: abbreviation.
// Example: 2001:db8::1 → 2001:0db8:0000:0000:0000:0000:0000:0001
func expandIPv6(ip net.IP) string {
	// Convert to 16-byte representation
	ipv6 := ip.To16()
	if ipv6 == nil {
		return ""
	}

	// Format as 8 groups of 4 hex digits
	parts := make([]string, 8)
	for i := 0; i < 8; i++ {
		parts[i] = fmt.Sprintf("%04x", uint16(ipv6[i*2])<<8|uint16(ipv6[i*2+1]))
	}

	return strings.Join(parts, ":")
}

// hasUnresolvedTemplates checks if there are any {{ }} patterns left.
func hasUnresolvedTemplates(s string) bool {
	pattern := regexp.MustCompile(`\{\{\s*[a-z-]+\s*\}\}`)
	return pattern.MatchString(s)
}
