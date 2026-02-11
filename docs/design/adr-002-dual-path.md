# ADR-002: Dual-Path Architecture (CLI + Operator)

## Status

Accepted (2026-02-06)

## Context

k8zner needs two operational modes:
1. **CLI**: On-machine provisioning for initial cluster creation
2. **Operator**: In-cluster controller for ongoing reconciliation, scaling, and healing

Both modes perform the same core operations (provisioning infrastructure, installing addons) but from different execution contexts.

## Decision

### Shared Core, Separate Entry Points

Both paths share the same internal packages (`provisioning/`, `addons/`, `platform/`, `config/`) and converge through a single runtime representation (`config.Config`).

```
CLI:      k8zner.yaml  -->  v2.Expand()     -->  config.Config  -->  provisioning/addons
Operator: CRD spec     -->  SpecToConfig()  -->  config.Config  -->  provisioning/addons
```

### Config Round-Trip Safety

`SpecToConfig()` must replicate the same defaults that `v2.Expand()` sets. Both reference shared constants from `config/v2/versions.go` for network CIDRs, version pins, and addon defaults. Shared helper functions (`config.DefaultCilium()`, `config.DefaultTraefik()`, etc.) ensure addon configurations are identical.

### Adapter Pattern for Operator

The operator doesn't reimplements provisioning logic. Instead, `PhaseAdapter` wraps the existing CLI provisioners and adds operator-specific concerns (CRD status updates, logging):

```go
func (a *PhaseAdapter) ReconcileInfrastructure(pCtx, cluster) error {
    err := a.infraProvisioner.Provision(pCtx)  // Reuse CLI provisioner
    cluster.Status.Infrastructure.NetworkID = pCtx.State.Network.ID  // Operator-specific
    return err
}
```

### Three Addon Entry Points

`internal/addons/` exposes three functions that all delegate to the same `applyAddons()`:
- `Apply()` — CLI: installs everything
- `ApplyCilium()` — Operator CNI phase: Cilium only
- `ApplyWithoutCilium()` — Operator addons phase: everything except Cilium and operator

## Consequences

### Benefits
- **Zero duplication**: Provisioning logic exists once, used by both paths
- **Testability**: Handlers are framework-agnostic; provisioners tested independently of operator
- **Maintainability**: Config changes automatically apply to both paths
- **Extensibility**: Adding operator phases only requires new reconciliation methods

### Risks
- **Default synchronization**: When adding CRD fields, must trace the full round-trip through both `v2.Expand()` and `SpecToConfig()`. This has caused bugs twice (Traefik settings, Cilium settings).
- **Mitigation**: Shared `config.Default*()` functions and `configv2` constants minimize drift. Tests validate both paths produce equivalent configs.

### Intentional Differences

Some defaults intentionally differ between paths:
- `APILoadBalancerEnabled`: CLI sets based on mode (HA only); operator always enables (manages scaling)
- `Workers.Count`: Operator sets to 0 (reconciler creates workers); CLI uses actual count
- `ControlPlane.Count`: Operator limits to 1 during bootstrap (scales up in Running phase)
