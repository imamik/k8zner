# Helm Addon Abstraction

This package provides a lightweight abstraction for rendering Helm charts as Kubernetes manifests. It matches Terraform's offline-first approach by embedding helm charts in the binary and rendering them at runtime with Go.

## Architecture

```
internal/addons/helm/
├── chart.go         # Chart file loading and discovery
├── renderer.go      # Core helm template rendering engine
├── values.go        # Value merging and YAML utilities
├── templates/       # Embedded helm charts
│   ├── metrics-server/
│   ├── cert-manager/
│   └── ...
└── README.md
```

## Design Principles

1. **Offline-First**: Helm charts are pre-downloaded and embedded at build time
2. **Terraform Parity**: Values structures match terraform locals for consistency
3. **Type-Safe**: Values use `helm.Values` type with strong typing
4. **Minimal Dependencies**: Uses official Helm v3 SDK for rendering
5. **Embedded Resources**: Charts stored in binary via `//go:embed`

## Usage

### Rendering a Chart

```go
import "hcloud-k8s/internal/addons/helm"

values := helm.Values{
    "replicas": 2,
    "nodeSelector": helm.Values{
        "node-role.kubernetes.io/control-plane": "",
    },
}

manifests, err := helm.RenderChart("metrics-server", "kube-system", values)
if err != nil {
    return fmt.Errorf("failed to render chart: %w", err)
}
```

### Merging Values

```go
defaults := helm.Values{"replicas": 1}
overrides := helm.Values{"replicas": 3}

merged := helm.Merge(defaults, overrides) // {"replicas": 3}
```

### Adding New Charts

1. Add chart to `scripts/embed-helm-charts.sh`:
   ```bash
   ["my-chart"]="https://example.com/charts my-chart v1.0.0"
   ```

2. Download the chart:
   ```bash
   ./scripts/embed-helm-charts.sh my-chart
   ```

3. Create addon implementation:
   ```go
   // internal/addons/myChart.go
   func applyMyChart(ctx context.Context, kubeconfigPath string, cfg *config.Config) error {
       values := buildMyChartValues(cfg)
       manifests, err := helm.RenderChart("my-chart", "kube-system", values)
       // ... apply manifests
   }
   ```

4. Add to `Apply()` function in `internal/addons/apply.go`

## Terraform Mapping

The helm abstraction mirrors terraform's approach:

### Terraform (Data Source)
```hcl
data "helm_template" "metrics_server" {
  name      = "metrics-server"
  namespace = "kube-system"

  repository = var.metrics_server_helm_repository
  chart      = var.metrics_server_helm_chart
  version    = var.metrics_server_helm_version

  values = [yamlencode({
    replicas = 2
  })]
}
```

### Go (Runtime Rendering)
```go
values := helm.Values{
    "replicas": 2,
}

manifests, _ := helm.RenderChart("metrics-server", "kube-system", values)
```

## Chart Versions

Chart versions are locked in `scripts/embed-helm-charts.sh` and should match terraform default variables:

- `metrics-server`: v3.12.2
- `cert-manager`: v1.16.3
- `ingress-nginx`: v4.11.3
- `longhorn`: v1.7.2
- `cluster-autoscaler`: v1.1.1

## Build Process

Charts are embedded at build time:

```bash
# Download all charts
./scripts/embed-helm-charts.sh

# Or download specific chart
./scripts/embed-helm-charts.sh metrics-server

# Build binary (charts automatically embedded via go:embed)
go build ./cmd/hcloud-k8s
```

## Testing

```bash
# Test helm abstraction
go test ./internal/addons/helm/...

# Test addon implementations
go test ./internal/addons/...
```

## Migration from Terraform

When migrating addons from terraform to Go:

1. Reference terraform file (e.g., `terraform/metrics_server.tf`)
2. Extract locals logic for value computation
3. Match data structures and defaults exactly
4. Add terraform file reference in comments:
   ```go
   // buildMetricsServerValues creates helm values matching terraform configuration.
   // See: terraform/metrics_server.tf
   ```

## Limitations

- Charts must be embedded at build time (not fetched at runtime)
- Requires helm SDK (adds ~10MB to binary)
- No CRD installation order guarantees (handled by kubectl apply)
- Values must be serializable to YAML

## Future Enhancements

- [ ] Support for subchart value overrides
- [ ] Chart dependency resolution
- [ ] Post-rendering hooks (kustomize-style)
- [ ] Values schema validation
- [ ] Dry-run mode for testing
