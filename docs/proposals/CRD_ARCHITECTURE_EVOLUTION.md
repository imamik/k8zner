# CRD Architecture Evolution: Single Cluster CRD vs Split Resource Model

## Context

k8zner currently exposes a single, user-facing `K8znerCluster` CRD as the declarative source of truth for infrastructure, compute, Talos/Kubernetes versions, and addon intent.

This proposal reevaluates whether k8zner should keep the single-CRD model or split responsibilities into separate resources (e.g., `Server`, `Network`, `Addon`) and controllers.

## Current Model (k8zner)

### What exists today

- Single top-level desired-state API: `K8znerCluster`.
- One operator reconciler driving provisioning phases (infrastructure → image → compute → bootstrap/CNI/addons).
- CRD spec is converted into shared internal runtime config via `SpecToConfig()`.

### Strengths

- **Simple UX:** one object to manage per cluster.
- **Operator-first cohesion:** single state machine and status story.
- **Low API surface area:** easier docs/support and fewer compatibility edges.
- **Good fit for opinionated product behavior:** central orchestration with clear phases.

### Current pressure points

- Spec now combines multiple lifecycle domains (infra + compute + addons + backup + health).
- Status contains broad concerns (infra IDs, node health, connectivity, addon health, phase history).
- Some converter logic encodes implicit behavior that is not obvious from the API surface.

## External Reference: Cluster API Provider Hetzner (CAPH)

A mature Hetzner-based project (`syself/cluster-api-provider-hetzner`) follows the split-resource pattern:

- `HetznerCluster` for cluster-scoped infrastructure.
- `HCloudMachine` (+ templates/remediation resources) for machine lifecycle.
- Multiple controllers and resources with ownership chains.

This model works because CAPH optimizes for:

- Composability with Cluster API ecosystem controllers.
- Independent machine lifecycle and remediation.
- Templated machine definitions and reusable abstractions.

## Best-Practice Guidance

There is no universal best practice in isolation; the “best” model depends on ownership boundaries and interoperability goals.

### Prefer a single top-level CRD when

- One team owns the entire lifecycle.
- Product UX simplicity is top priority.
- Subdomains are implementation details, not standalone products/APIs.
- You do not need third-party controller interoperability at per-resource granularity.

### Prefer split CRDs when

- Different controllers/teams own different domains.
- You need independent reconciliation loops and rollout cadence.
- You have high-cardinality child resources requiring their own status/conditions.
- You target ecosystem interoperability (Cluster API style machine management).

## Recommendation for k8zner

### Decision

**Keep `K8znerCluster` as the primary user-facing API for now.**

Do **not** immediately split into separate first-class CRDs for `Server`, `Network`, `Addon`.

### Why

- Current architecture is intentionally operator-first with one desired-state contract.
- Full CRD splitting introduces significant complexity (ownership, garbage collection, versioning, RBAC, condition propagation) before clear product need.
- Most current pain appears to be boundary clarity/internal contracts rather than user-facing API shape.

## Evolution Plan (staged)

### Stage 0 — harden the existing model (short-term)

1. **Clarify API semantics vs implementation semantics**
   - Ensure every spec field maps clearly to behavior.
   - Remove implicit converter surprises where practical.
2. **Improve status contract discipline**
   - Define which status fields are stable vs diagnostic.
   - Standardize condition reasons/messages across phases.
3. **Document lifecycle boundaries**
   - Explicitly document which phase/controller path owns each concern.

### Stage 1 — internal domain decomposition (no external CRD split)

1. Define explicit internal interfaces for domains:
   - Infrastructure domain
   - Compute domain
   - Addon domain
2. Reduce cross-domain coupling in conversion/reconcile paths.
3. Add invariants and tests around spec→config translation and phase ownership.

### Stage 2 — optional child CRDs (only if justified)

Introduce narrowly-scoped child resources **only where independent lifecycle is required**, likely starting with machine-level resources (highest churn).

Candidate path:

- Keep `K8znerCluster` as parent intent.
- Add internal/advanced child CRDs (e.g., `K8znerMachine`) behind feature gate.
- Parent controller orchestrates; child controller reconciles machine-specific lifecycle.

### Stage 3 — broad split (conditional)

Only pursue separate public `Network`/`Server`/`Addon` APIs if at least two of the following are true:

- Need for independent team ownership and release cadence.
- Demand for third-party integrations against sub-resources.
- Reconciler throughput/scalability bottlenecks due to monolithic loop.

## Risks and Trade-offs

### If we split too early

- Significant increase in complexity (APIs, conditions, ownership, upgrade/migration).
- Harder UX for most users.
- More moving parts to debug during failures.

### If we never split

- Potential long-term maintainability pressure in one large API/reconciler.
- Less extensibility for external ecosystem integrations.

## Migration Principles (if/when splitting)

1. **Backward compatibility first**
   - `K8znerCluster` remains valid for at least one major cycle.
2. **Progressive enhancement**
   - New child CRDs optional initially; parent API still usable alone.
3. **Clear ownership and references**
   - Parent/child ownerRefs and condition aggregation defined upfront.
4. **Observed behavior parity**
   - Existing cluster operations behave the same before and after migration.

## Proposed Immediate Deliverables

1. Accept this ADR-style direction: **single public CRD now, staged internal decomposition, conditional split later**.
2. Create follow-up implementation tasks:
   - API semantics audit (spec fields vs behavior)
   - Status contract normalization
   - Spec converter coupling cleanup
   - Domain ownership matrix in docs
3. Reevaluate after 1–2 releases using concrete metrics:
   - reconcile latency by phase
   - bug density by domain
   - operator complexity hotspots

## Appendix: Evaluation Checklist

Use this checklist during future reevaluation:

- Is there a real requirement for independent sub-resource ownership?
- Are users asking to manage machines/networks/addons independently as first-class APIs?
- Are we blocked on interoperability that requires split resources?
- Can the same goal be met by internal modularization without new CRDs?

If most answers are “no”, remain with single CRD and continue internal decomposition.
