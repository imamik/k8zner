# Terraform vs Go Implementation Comparison

This document compares the Terraform implementation with the Go implementation to identify discrepancies.

## Cilium Configuration Differences

### 1. `policyCIDRMatchMode`

**Terraform (`terraform/cilium.tf:61` and `terraform/variables.tf:1381`):**
```terraform
policyCIDRMatchMode = var.cilium_policy_cidr_match_mode  # default: "" (empty string)
```

**Go (`internal/addons/cilium.go:82`):**
```go
"policyCIDRMatchMode": []string{"nodes"},  // Always set to ["nodes"]
```

**Impact:**
- Terraform default is empty (no CIDR match mode)
- Go always enables "nodes" mode
- This allows targeting kube-api server with NetworkPolicy
- **Recommendation:** Go implementation should match Terraform default (empty) or make it configurable

---

### 2. `hostLegacyRouting`

**Terraform (`terraform/cilium.tf:65`):**
```terraform
hostLegacyRouting = local.cilium_ipsec_enabled  # Only true when IPSec is used
```

**Go (`internal/addons/cilium.go:86-88`):**
```go
// hostLegacyRouting MUST be true on Talos 1.8+ due to DNS forwarding
// See: https://github.com/siderolabs/talos/issues/9132
"hostLegacyRouting": true,  // Always true
```

**Impact:**
- Terraform only enables when IPSec encryption is used
- Go always enables due to Talos 1.8+ DNS forwarding requirement
- **Recommendation:** Go implementation is correct for Talos 1.8+. Document this difference.

---

### 3. `socketLB.hostNamespaceOnly`

**Terraform (`terraform/cilium.tf:79-80` and `terraform/variables.tf:1441-1444`):**
```terraform
socketLB = {
  hostNamespaceOnly = var.cilium_socket_lb_host_namespace_only_enabled  # default: false
}
```

**Go (`internal/addons/cilium.go:101-103`):**
```go
"socketLB": helm.Values{
	"hostNamespaceOnly": false,  // Hardcoded false
},
```

**Impact:**
- Both default to false
- Terraform allows configuration via variable
- Go is hardcoded
- **Recommendation:** Consider making this configurable in Go if needed

---

### 4. Gateway API Configuration

**Terraform (`terraform/cilium.tf:101-110`):**
```terraform
gatewayAPI = {
  enabled               = var.cilium_gateway_api_enabled
  enableProxyProtocol   = var.cilium_gateway_api_proxy_protocol_enabled
  enableAppProtocol     = true
  enableAlpn            = true
  externalTrafficPolicy = var.cilium_gateway_api_external_traffic_policy
  gatewayClass = {
    create = tostring(var.cilium_gateway_api_enabled)
  }
}
```

**Go (`internal/addons/cilium.go:134-138`):**
```go
if cfg.Addons.Cilium.GatewayAPIEnabled {
	values["gatewayAPI"] = helm.Values{
		"enabled": true,
	}
}
```

**Impact:**
- Terraform has more Gateway API options (proxy protocol, ALPN, traffic policy, gateway class)
- Go only sets `enabled: true`
- **Recommendation:** Consider adding these options to Go config if Gateway API is used

---

### 5. Prometheus/ServiceMonitor Configuration

**Terraform (`terraform/cilium.tf:119-154`):**
```terraform
prometheus = {
  enabled = true
  serviceMonitor = {
    enabled        = var.cilium_service_monitor_enabled
    trustCRDsExist = var.cilium_service_monitor_enabled
    interval       = "15s"
  }
}
operator = {
  ...
  prometheus = {
    enabled = true
    serviceMonitor = {
      enabled  = var.cilium_service_monitor_enabled
      interval = "15s"
    }
  }
}
```

**Go (`internal/addons/cilium.go`):**
```go
// No Prometheus configuration present
```

**Impact:**
- Terraform enables Prometheus metrics and ServiceMonitor support
- Go doesn't configure Prometheus at all
- **Recommendation:** Add Prometheus configuration if metrics are needed

---

### 6. Operator `topologySpreadConstraints`

**Terraform (`terraform/cilium.tf:135-147`):**
```terraform
topologySpreadConstraints = [
  {
    topologyKey       = "kubernetes.io/hostname"
    maxSkew           = 1
    whenUnsatisfiable = "DoNotSchedule"
    labelSelector = {
      matchLabels = {
        "app.kubernetes.io/name" = "cilium-operator"
      }
    }
    matchLabelKeys = ["pod-template-hash"]
  }
]
```

**Go (`internal/addons/cilium.go:180-205`):**
```go
operatorConfig["topologySpreadConstraints"] = []helm.Values{
	{
		"topologyKey":       "kubernetes.io/hostname",
		"maxSkew":           1,
		"whenUnsatisfiable": "DoNotSchedule",
		"labelSelector": helm.Values{
			"matchLabels": helm.Values{
				"app.kubernetes.io/part-of": "cilium",
				"app.kubernetes.io/name":    "cilium-operator",
			},
		},
		"matchLabelKeys": []string{"pod-template-hash"},
	},
	{
		"topologyKey":       "topology.kubernetes.io/zone",
		"maxSkew":           1,
		"whenUnsatisfiable": "ScheduleAnyway",
		"labelSelector": helm.Values{
			"matchLabels": helm.Values{
				"app.kubernetes.io/part-of": "cilium",
				"app.kubernetes.io/name":    "cilium-operator",
			},
		},
		"matchLabelKeys": []string{"pod-template-hash"},
	},
}
```

**Impact:**
- Terraform has ONE topology constraint (hostname)
- Go has TWO topology constraints (hostname + zone)
- Go adds `app.kubernetes.io/part-of: "cilium"` label selector
- Go adds zone-based constraint with "ScheduleAnyway"
- **Recommendation:** Go implementation is more robust for multi-zone deployments

---

### 7. Operator `podDisruptionBudget`

**Terraform (`terraform/cilium.tf:130-134`):**
```terraform
podDisruptionBudget = {
  enabled        = true
  minAvailable   = null
  maxUnavailable = 1
}
```

**Go (`internal/addons/cilium.go:175-178`):**
```go
if controlPlaneCount > 1 {
	operatorConfig["podDisruptionBudget"] = helm.Values{
		"enabled":        true,
		"maxUnavailable": 1,
	}
}
```

**Impact:**
- Terraform always enables PDB (even for single control plane)
- Go only enables PDB when `controlPlaneCount > 1`
- Both set `maxUnavailable: 1`
- Terraform explicitly sets `minAvailable: null`
- **Recommendation:** Go implementation is more correct (no need for PDB with single replica)

---

### 8. `agentNotReadyTaintKey`

**Terraform (`terraform/cilium.tf`):**
```terraform
# Not configured
```

**Go (`internal/addons/cilium.go:125`):**
```go
"agentNotReadyTaintKey": "node.cilium.io/agent-not-ready",
```

**Impact:**
- Terraform doesn't set this (uses Helm chart default)
- Go explicitly sets it
- This is a Helm chart default value anyway
- **Recommendation:** Go implementation is fine (explicit is better than implicit). See https://github.com/cilium/cilium/issues/40312

---

## Summary

### Critical Differences
1. **`policyCIDRMatchMode`** - Go always sets to ["nodes"], Terraform defaults to empty
2. **`hostLegacyRouting`** - Go always true (for Talos 1.8+), Terraform conditional on IPSec

### Feature Gaps in Go
1. **Gateway API** - Limited configuration (missing proxy protocol, ALPN, traffic policy)
2. **Prometheus/ServiceMonitor** - Not configured at all
3. **Socket LB** - Not configurable (hardcoded to false)

### Go Enhancements
1. **Topology Spread** - Two constraints (hostname + zone) vs Terraform's one
2. **PDB Logic** - Only enabled for HA setups (more correct)
3. **Agent Taint** - Explicitly configured for better startup behavior

### Recommendations

1. **Make `policyCIDRMatchMode` configurable** - Default to empty string to match Terraform
2. **Document `hostLegacyRouting` requirement** - This is a Talos 1.8+ requirement, not a bug
3. **Consider adding Prometheus configuration** - If metrics/monitoring is needed
4. **Gateway API enhancements** - Add if advanced Gateway API features are required
5. **Keep topology spread enhancements** - Multi-zone is better than Terraform's approach

## Validation Status

- ✅ Core Cilium functionality matches Terraform
- ⚠️  Some configuration differences (mostly minor)
- ✅ Go implementation has some improvements (topology, PDB logic)
- ⚠️  Missing some advanced features (Prometheus, full Gateway API config)
