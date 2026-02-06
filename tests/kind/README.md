# Kind Integration Tests

Tests addon installations against a local Kubernetes cluster using [kind](https://kind.sigs.k8s.io/).

## Quick Start

```bash
make test-kind          # Full suite (~20m)
make test-kind-smoke    # CRDs only (~2m)
make test-kind-core     # CRDs + core addons (~5m)
```

## Test Layers

| Layer | Tests | Duration |
|-------|-------|----------|
| 01_CRDs | Gateway API, Prometheus Operator CRDs | ~1m |
| 02_Core | cert-manager, metrics-server | ~5m |
| 03_Ingress | Traefik | ~3m |
| 04_GitOps | ArgoCD | ~5m |
| 05_Monitoring | kube-prometheus-stack | ~8m |
| 06_Integration | Ingress, certificates, ServiceMonitors | ~2m |

## Commands

```bash
# Run specific layer
make test-kind-layer LAYER=03_Ingress

# Keep cluster for debugging
make test-kind-keep

# Clean up
make test-kind-cleanup
```

## What's Tested

**Can test** (no cloud dependencies):
- Helm installations
- CRD registration
- Pod scheduling
- Ingress resources
- Self-signed certificates
- ServiceMonitors

**Cannot test** (requires cloud APIs):
- CCM, CSI (Hetzner)
- External-DNS (Cloudflare)
- ACME certificates
- Load balancers

## Debugging

```bash
# Keep cluster after failure
KEEP_KIND_CLUSTER=1 go test -v -tags=kind ./tests/kind/...

# Access cluster
export KUBECONFIG=$(kind get kubeconfig-path --name k8zner-test)
kubectl get pods -A
```

## File Structure

```
tests/kind/
├── framework.go      # Cluster lifecycle
├── kubectl.go        # kubectl helpers
├── wait.go           # Wait conditions
├── assert.go         # Assertions
├── diagnostics.go    # Debug info collection
├── suite_test.go     # Test entry point
└── addons_test.go    # Addon tests
```
