# Code Organization & Quality Rules

This document defines the structural patterns and quality standards for the k8zner codebase.

## 1. Architecture: Dual-Path (CLI + Operator)

The codebase has two entry points that drive the same shared layers:

- **CLI path** (`cmd/k8zner/`): On-machine provisioning via Cobra commands. Bootstraps a single CP node, installs all addons in one shot.
- **Operator path** (`cmd/operator/`): Kubernetes controller reconciling `K8znerCluster` CRDs. Scales CP from 1→N, installs addons in phases, handles self-healing.

Both paths share: `internal/addons/`, `internal/provisioning/`, `internal/platform/`, `internal/config/`.

### Package Structure

- **api/v1alpha1/**: CRD types (`K8znerCluster` spec, status, phase constants)
- **cmd/k8zner/**: CLI — split commands (Cobra definitions) from handlers (business logic)
- **cmd/operator/**: Operator entrypoint — sets up controller-runtime
- **cmd/cleanup/**: Standalone cleanup utility for destroying cluster resources
- **internal/**: Organize by domain and responsibility:
  - **operator/controller/**: CRD reconciliation (phases, scaling, healing, health checks)
  - **operator/provisioning/**: CRD spec → internal Config adapter
  - **operator/addons/**: Operator-specific addon phase manager
  - **provisioning/**: All cluster provisioning (compute, infrastructure, images, bootstrap)
  - **addons/**: Addon installation (shared by CLI and operator)
    - **addons/helm/**: Helm chart rendering, value building, and chart client
    - **addons/k8sclient/**: Kubernetes API operations (apply manifests, manage secrets)
  - **config/**: Configuration management (YAML spec, defaults, validation)
  - **platform/**: External system integrations (hcloud, talos, ssh, s3)
  - **util/**: Reusable utilities (async, keygen, labels, naming, ptr, retry)
- One package = one responsibility

## 2. Function Design

- **Flow functions**: Orchestrate steps, delegate to operations (e.g., `Apply()`)
- **Operation functions**: Do one thing, return early on errors
- Keep functions < 50 lines; split at logical boundaries if larger
- Function names describe what they do: `reconcileInfrastructure()` not `process()`

## 3. Separation of Concerns

- Infrastructure provisioning separate from addon installation
- CLI framework (cobra) isolated in commands/, never in handlers/
- Configuration loading separate from execution
- Operator controller logic separate from shared provisioning/addon layers
- CRD types (`api/v1alpha1/`) never import internal packages

## 4. Documentation

- Package-level comments explain purpose and scope (~10-15 lines max)
- Exported functions: concise godoc (1-3 lines)
- Comments explain "why", not "what"

## 5. Dependencies

- Trust dependencies to validate their inputs
- Minimal abstractions — only create interfaces when needed (2+ implementations)

## 6. Error Handling

- Wrap errors with context: `fmt.Errorf("failed to X: %w", err)`
- Return errors, don't log and continue
- No defensive checks for impossible states
- **Library code** (internal/): Return errors, let callers decide logging
- **Command handlers** (cmd/handlers/): Log significant operations and errors

## 7. Naming Conventions

### Files
- **snake_case**: `apply.go`, `load_balancer.go`, `cert_manager.go`

### Functions & Methods
- **Exported**: PascalCase — `Apply()`, `NewReconciler()`
- **Unexported**: camelCase — `loadConfig()`, `reconcileInfrastructure()`
- Verb-first naming: `reconcileCluster()` not `clusterReconcile()`

### Variables
- Descriptive names in wider scopes: `reconciler`, `kubeconfig`
- Short names in tight scopes: `for i, cfg := range configs`

### Constants
- camelCase for unexported: `secretsFile`, `defaultTimeout`
- PascalCase for exported: `DefaultRetryCount`, `MaxServerCount`

## 8. When to Create New Packages

### DO create a package:
- **Domain separation**: Infrastructure vs addons vs configuration
- **Reusable components**: Used by 2+ other packages
- **External boundary**: Interacting with external systems (hcloud, talos, ssh)

### DON'T create a package:
- Single file with 2-3 helper functions
- Only used by one parent package
- No clear domain boundary

### Size Guidelines:
- **Ideal**: 3-8 files per package
- **Too large**: 15+ files suggests multiple concerns — consider splitting

## 9. Operator Controller Patterns

The operator controller uses a single `ClusterReconciler` struct split across thematic files (phases, scaling, healing, health, etc.). Same package, same struct — no new interfaces needed.

### Config Round-Trip

Three representations must stay in sync:

1. **YAML spec** (`internal/config/spec.go`) — user-facing config file
2. **Internal Config** (`internal/config/types.go`) — runtime representation with expanded defaults
3. **CRD spec** (`api/v1alpha1/types.go`) — Kubernetes-native representation

Flows:
- **CLI**: YAML spec → `ExpandSpec()` → internal Config → provisioning/addons
- **Operator**: CRD spec → `SpecToConfig()` → internal Config → provisioning/addons

`SpecToConfig()` must replicate the same defaults that `ExpandSpec()` sets. When adding CRD fields, trace the full round-trip to avoid silent mismatches.

### Shared Addon Installation

`internal/addons/` is shared by both paths. Entry points:
- `Apply()` — CLI: installs everything including Cilium and Operator
- `ApplyWithoutCilium()` — Operator: skips Cilium (CNI phase) and Operator (already running)
- `ApplyCilium()` — Operator CNI phase: installs only Cilium

## 10. Generic Operations

Use Go generics to eliminate code duplication across 3+ resource types (e.g., `DeleteOperation[T]`, `EnsureOperation[T]` in `internal/platform/hcloud/`). Avoid generics when logic differs significantly between types or only 1-2 instances exist.

## 11. External Commands & Resources

- Use `exec.CommandContext` for external tools, with `// #nosec G204` comments
- Use `//go:embed` for manifests and templates
- Always defer cleanup of temporary files

---

*This is a living document. Update it as we discover new patterns during refactoring.*
