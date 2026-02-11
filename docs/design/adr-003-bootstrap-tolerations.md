# ADR-003: Bootstrap Tolerations for Control Plane Addons

## Status

Accepted (2026-02-05)

## Context

During cluster bootstrap, addons must run on control plane nodes before the cluster is fully initialized. Three conditions block pod scheduling:

1. Control plane taint prevents workload scheduling
2. Cloud provider hasn't initialized the node yet
3. Node isn't Ready (CNI not installed)

This creates a chicken-and-egg problem: CNI and CCM need to run to make nodes ready, but pods can't schedule until nodes are ready.

## Decision

All addons that run during bootstrap must include three tolerations:

```yaml
tolerations:
  - key: node-role.kubernetes.io/control-plane
    effect: NoSchedule
  - key: node.cloudprovider.kubernetes.io/uninitialized
    value: "true"
    effect: NoSchedule
  - key: node.kubernetes.io/not-ready
    effect: NoSchedule
```

Additionally, the k8zner operator itself requires a fourth toleration:

```yaml
- key: node.cilium.io/agent-not-ready
  effect: NoSchedule
```

This is because the operator installs Cilium — it cannot depend on Cilium being ready.

## Consequences

- Without all three base tolerations, pods get stuck in Pending during bootstrap
- Tests validate toleration keys by count and index order
- When adding new tolerations, corresponding tests must be updated
