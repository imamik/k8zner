# Ingress NGINX Migration Plan

## Overview
Migrate ingress-nginx addon from terraform to Go using helm abstraction.
This is the most complex addon due to load balancer integration and service type logic.

## Current State (Terraform)
- **File**: `terraform/ingress_nginx.tf`
- **Chart**: ingress-nginx v4.11.3
- **Namespace**: ingress-nginx
- **Purpose**: HTTP/HTTPS ingress controller

## Key Requirements

### 1. Replica Calculation
```
replicas = user_configured OR (worker_count < 3 ? 2 : 3)
```

### 2. Service Type Logic
**Critical**: Determines LoadBalancer vs NodePort
```
service_type = length(ingress_load_balancer_pools) == 0 ? "LoadBalancer" : "NodePort"
```
- If no external LB pools configured → Use Hetzner LoadBalancer
- If external LB pools exist → Use NodePort (external LB points to nodes)

### 3. Service Configuration

#### NodePort Mode
```yaml
service:
  type: NodePort
  nodePorts:
    http: 30000
    https: 30001
```

#### LoadBalancer Mode
```yaml
service:
  type: LoadBalancer
  annotations:
    load-balancer.hetzner.cloud/algorithm-type: round_robin
    load-balancer.hetzner.cloud/disable-private-ingress: "true"
    load-balancer.hetzner.cloud/disable-public-network: "false"
    load-balancer.hetzner.cloud/health-check-interval: "15s"
    load-balancer.hetzner.cloud/health-check-retries: "3"
    load-balancer.hetzner.cloud/health-check-timeout: "10s"
    load-balancer.hetzner.cloud/hostname: ""
    load-balancer.hetzner.cloud/ipv6-disabled: "false"
    load-balancer.hetzner.cloud/location: ""
    load-balancer.hetzner.cloud/name: ""
    load-balancer.hetzner.cloud/type: "lb11"
    load-balancer.hetzner.cloud/use-private-ip: "true"
    load-balancer.hetzner.cloud/uses-proxyprotocol: "true"
```

### 4. Topology Spread Constraints
Two constraints (only if kind=Deployment):
1. **Hostname** spread:
   - maxSkew: 1
   - whenUnsatisfiable: DoNotSchedule (if worker_sum > 1) OR ScheduleAnyway
2. **Zone** spread:
   - maxSkew: 1
   - whenUnsatisfiable: ScheduleAnyway

### 5. Controller Configuration
- **Admission Webhooks**: cert-manager integration enabled
- **Kind**: Deployment or DaemonSet (from config)
- **minAvailable**: null
- **maxUnavailable**: 1
- **watchIngressWithoutClass**: true
- **enableTopologyAwareRouting**: configurable

### 6. Proxy Configuration
```yaml
config:
  proxy-real-ip-cidr: <depends on external traffic policy>
    # If externalTrafficPolicy=Local: load_balancer_subnet
    # Otherwise: node_ipv4_cidr
  compute-full-forwarded-for: true
  use-proxy-protocol: true
```

### 7. Network Policy
```yaml
networkPolicy:
  enabled: true
```

## Implementation Steps

### Step 1: Download Chart
```bash
./scripts/embed-helm-charts.sh ingress-nginx
```

### Step 2: Extend Config Types
Add to `internal/config/types.go`:
```go
type IngressNginxConfig struct {
    Enabled                  bool
    Kind                     string  // "Deployment" or "DaemonSet"
    Replicas                 int     // Optional, calculated if 0
    TopologyAwareRouting     bool
    ExternalTrafficPolicy    string  // "Local" or "Cluster"
    UseLoadBalancer          bool    // If false, use NodePort
    // LoadBalancer settings (if UseLoadBalancer=true)
    LoadBalancerType         string
    LoadBalancerLocation     string
    // Add more as needed
}
```

### Step 3: Implement Addon
Create `internal/addons/ingressNginx.go`:

```go
func applyIngressNginx(ctx, kubeconfigPath, cfg) error {
    // Create namespace
    // Build values
    // Render chart
    // Apply
}

func buildIngressNginxValues(cfg) helm.Values {
    // Calculate replicas
    replicas := calculateIngressNginxReplicas(cfg)

    // Determine service type
    serviceType := determineServiceType(cfg)

    // Build base controller config
    controller := helm.Values{
        "admissionWebhooks": helm.Values{
            "certManager": helm.Values{"enabled": true},
        },
        "kind": cfg.Addons.IngressNginx.Kind,
        "replicaCount": replicas,
        "minAvailable": nil,
        "maxUnavailable": 1,
        "watchIngressWithoutClass": true,
        "enableTopologyAwareRouting": cfg.Addons.IngressNginx.TopologyAwareRouting,
    }

    // Add topology spread if Deployment
    if cfg.Addons.IngressNginx.Kind == "Deployment" {
        controller["topologySpreadConstraints"] = buildTopologySpread(cfg)
    }

    // Build service config
    controller["service"] = buildServiceConfig(serviceType, cfg)

    // Build proxy config
    controller["config"] = buildProxyConfig(cfg)

    // Network policy
    controller["networkPolicy"] = helm.Values{"enabled": true}

    return helm.Values{"controller": controller}
}

func determineServiceType(cfg) string {
    if cfg.Addons.IngressNginx.UseLoadBalancer {
        return "LoadBalancer"
    }
    return "NodePort"
}

func buildServiceConfig(serviceType, cfg) helm.Values {
    service := helm.Values{
        "type": serviceType,
        "externalTrafficPolicy": cfg.Addons.IngressNginx.ExternalTrafficPolicy,
    }

    if serviceType == "NodePort" {
        service["nodePorts"] = helm.Values{
            "http": 30000,
            "https": 30001,
        }
    } else {
        service["annotations"] = buildLoadBalancerAnnotations(cfg)
    }

    return service
}

func buildLoadBalancerAnnotations(cfg) helm.Values {
    // Return all Hetzner LB annotations
}

func buildTopologySpread(cfg) []helm.Values {
    workerCount := getWorkerCount(cfg)

    hostnameConstraint := helm.Values{
        "topologyKey": "kubernetes.io/hostname",
        "maxSkew": 1,
        "whenUnsatisfiable": workerCount > 1 ? "DoNotSchedule" : "ScheduleAnyway",
        "labelSelector": helm.Values{
            "matchLabels": helm.Values{
                "app.kubernetes.io/instance": "ingress-nginx",
                "app.kubernetes.io/name": "ingress-nginx",
                "app.kubernetes.io/component": "controller",
            },
        },
        "matchLabelKeys": []string{"pod-template-hash"},
    }

    zoneConstraint := helm.Values{
        // Same but zone key, always ScheduleAnyway
    }

    return []helm.Values{hostnameConstraint, zoneConstraint}
}

func buildProxyConfig(cfg) helm.Values {
    // TODO: Need network subnet info for proxy-real-ip-cidr
    // For now, use sensible defaults
    return helm.Values{
        "compute-full-forwarded-for": true,
        "use-proxy-protocol": true,
    }
}
```

### Step 4: Testing
Create `internal/addons/ingressNginx_test.go`:
- Test replica calculation
- Test service type logic
- Test NodePort configuration
- Test LoadBalancer annotations
- Test topology spread (Deployment vs DaemonSet)
- Test proxy config

## Challenges & Solutions

### Challenge 1: Service Type Decision
**Problem**: Needs to know if external load balancer pools exist
**Solution**: Add `UseLoadBalancer` bool to config. Simplifies logic.

### Challenge 2: Load Balancer Annotations
**Problem**: Many annotations depend on terraform locals (location, name, etc.)
**Solution**: Use sensible defaults or make configurable. Can be extended later.

### Challenge 3: proxy-real-ip-cidr
**Problem**: Depends on network subnet info not in addon config
**Solution**: Skip for v1, or add network CIDR to config struct

### Challenge 4: Topology Spread Complexity
**Problem**: 2 constraints with different whenUnsatisfiable logic
**Solution**: Helper function to build both, reuse label selector

## Simplified Implementation Strategy

**Phase 1: Basic Implementation**
- Support Deployment kind only (most common)
- UseLoadBalancer = false (NodePort mode)
- Skip load balancer annotations (NodePort doesn't need them)
- Fixed replicas (2)
- Basic proxy config

**Phase 2: Full Implementation** (if needed)
- Add DaemonSet support
- Add LoadBalancer mode with annotations
- Add dynamic replica calculation
- Add full proxy config with CIDR

## Success Criteria
- [ ] Chart v4.11.3 embedded
- [ ] Namespace created
- [ ] Service type logic works
- [ ] NodePort mode configured correctly
- [ ] LoadBalancer mode with annotations (if needed)
- [ ] Topology spread for Deployment kind
- [ ] Cert-manager webhook integration
- [ ] Network policy enabled
- [ ] Tests for all modes
- [ ] CODE_STRUCTURE.md compliant

## Terraform Reference
```hcl
# terraform/ingress_nginx.tf
# Lines 1-144
```

## Notes
- Most complex addon due to service type conditional logic
- Start with simplified version (NodePort only)
- Can extend to LoadBalancer mode later if needed
- Focus on getting the pattern right
