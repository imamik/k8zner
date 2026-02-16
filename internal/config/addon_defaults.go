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

// BuildIngressHost returns "customSubdomain.domain" if customSubdomain is set,
// otherwise "defaultSubdomain.domain". Returns empty string if domain is empty.
func BuildIngressHost(domain, defaultSubdomain, customSubdomain string) string {
	if domain == "" {
		return ""
	}
	sub := defaultSubdomain
	if customSubdomain != "" {
		sub = customSubdomain
	}
	return sub + "." + domain
}

// DefaultExternalDNS returns ExternalDNS config with standard policy and sources.
func DefaultExternalDNS(enabled bool) ExternalDNSConfig {
	if !enabled {
		return ExternalDNSConfig{}
	}
	return ExternalDNSConfig{
		Enabled: true,
		Policy:  "sync",
		Sources: []string{"ingress"},
	}
}

// DefaultTalosBackup returns TalosBackupConfig with standard prefix and compression settings.
// Callers must still set schedule, S3 credentials, bucket, region, and endpoint.
func DefaultTalosBackup() TalosBackupConfig {
	return TalosBackupConfig{
		S3Prefix:           "etcd-backups",
		EnableCompression:  true,
		EncryptionDisabled: true,
	}
}

// DefaultPrometheusPersistence returns the default Prometheus persistence configuration.
func DefaultPrometheusPersistence() KubePrometheusPersistenceConfig {
	return KubePrometheusPersistenceConfig{
		Enabled: true,
		Size:    "50Gi",
	}
}
