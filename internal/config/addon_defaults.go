package config

// DefaultCilium returns the opinionated Cilium CNI configuration.
// Tunnel mode avoids direct routing device detection issues on Hetzner Cloud
// where multiple interfaces confuse Cilium's native routing.
func DefaultCilium() CiliumConfig {
	return CiliumConfig{
		Enabled:                     true,
		KubeProxyReplacementEnabled: true,
		RoutingMode:                 "tunnel",
		HubbleEnabled:               true,
		HubbleRelayEnabled:          true,
		HubbleUIEnabled:             true,
	}
}

// DefaultTraefik returns the opinionated Traefik ingress configuration.
// Uses Deployment with LoadBalancer service; CCM creates a Hetzner LB
// automatically via annotations. No hostNetwork needed.
func DefaultTraefik(enabled bool) TraefikConfig {
	return TraefikConfig{
		Enabled:               enabled,
		ExternalTrafficPolicy: "Cluster",
		IngressClass:          "traefik",
	}
}

// DefaultCCM returns the opinionated Hetzner Cloud Controller Manager configuration.
func DefaultCCM() CCMConfig {
	return CCMConfig{Enabled: true}
}

// DefaultCSI returns the opinionated Hetzner CSI driver configuration.
func DefaultCSI() CSIConfig {
	return CSIConfig{Enabled: true, DefaultStorageClass: true}
}

// DefaultGatewayAPICRDs returns the opinionated Gateway API CRDs configuration.
func DefaultGatewayAPICRDs() GatewayAPICRDsConfig {
	return GatewayAPICRDsConfig{Enabled: true}
}

// DefaultPrometheusOperatorCRDs returns the opinionated Prometheus Operator CRDs configuration.
func DefaultPrometheusOperatorCRDs() PrometheusOperatorCRDsConfig {
	return PrometheusOperatorCRDsConfig{Enabled: true}
}
