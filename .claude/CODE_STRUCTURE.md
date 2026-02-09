# Code Organization & Quality Rules

This document defines the structural patterns and quality standards for the k8zner codebase. These rules emerged from active refactoring and represent our agreed-upon approach.

## 1. Architecture: Dual-Path (CLI + Operator)

The codebase has two entry points that drive the same shared layers:

- **CLI path** (`cmd/k8zner/`): On-machine provisioning via Cobra commands. Bootstraps a single CP node, installs all addons in one shot.
- **Operator path** (`cmd/operator/`): Kubernetes controller reconciling `K8znerCluster` CRDs. Scales CP from 1→N, installs addons in phases, handles self-healing.

Both paths share: `internal/addons/`, `internal/provisioning/`, `internal/platform/`, `internal/config/`.

### Package Structure

- **api/v1alpha1/**: CRD types (`K8znerCluster` spec, status, phase constants)
- **cmd/k8zner/**: CLI — split commands (Cobra definitions) from handlers (business logic)
- **cmd/operator/**: Operator entrypoint — sets up controller-runtime
- **internal/**: Organize by domain and responsibility:
  - **operator/controller/**: CRD reconciliation (phases, scaling, healing, health checks)
  - **operator/provisioning/**: CRD spec → internal Config adapter
  - **operator/addons/**: Operator-specific addon phase manager
  - **provisioning/**: All cluster provisioning (compute, infrastructure, images, bootstrap)
  - **addons/**: Addon installation (shared by CLI and operator)
  - **config/**: Configuration management; **config/v2/** for YAML spec + defaults
  - **platform/**: External system integrations (hcloud, talos, ssh, s3)
  - **util/**: Reusable utilities (async, keygen, labels, naming, rdns, retry)
- One package = one responsibility — provisioning is acceptable as a larger package when the domain is cohesive

## 2. Function Design

- **Flow functions**: Orchestrate steps, delegate to operations (e.g., `Apply()`)
- **Operation functions**: Do one thing, return early on errors
- Keep functions < 50 lines; split at logical boundaries if larger
- Function names describe what they do: `reconcileInfrastructure()` not `process()`
- When splitting large functions, extract by responsibility:
  - Read/process data → separate from write/execute
  - Template rendering → separate from applying results
  - File I/O → separate from business logic

## 3. Separation of Concerns

- Infrastructure provisioning separate from addon installation
- CLI framework (cobra) isolated in commands/, never in handlers/
- Configuration loading separate from execution
- Write critical state (secrets) before risky operations (reconciliation)
- Operator controller logic separate from shared provisioning/addon layers
- CRD types (`api/v1alpha1/`) never import internal packages

## 4. Documentation

- Package-level comments explain purpose and scope (keep to ~10-15 lines max)
- All exported functions have godoc comments with:
  - What it does (one-line summary)
  - Key behavior details (1-2 additional lines if needed)
  - Required inputs/environment variables (only if non-obvious)
- **Keep function comments concise (1-3 lines)** - verbose docs make code hard to scan
- Internal/unexported functions don't need comments unless logic is non-obvious
- Comments explain "why", not "what" (code shows what)
- Remove verbose thinking/planning comments from final code

## 5. Dependencies

- Trust dependencies to validate their inputs (don't duplicate validation)
- Simple module path (`k8zner` not `github.com/...`)
- Minimal abstractions - only create interfaces when needed (2+ implementations)

## 6. Error Handling

- Wrap errors with context: `fmt.Errorf("failed to X: %w", err)`
- Return errors, don't log and continue
- No defensive checks for impossible states
- Let clients handle validation, don't duplicate it
- Provide context in error messages: what operation failed, what was being operated on
- Example: `fmt.Errorf("failed to read manifests for addon %s at %s: %w", name, path, err)`

## 6.1. Logging

- **Library code** (internal/): Return errors, let callers decide logging
- **Command handlers** (cmd/handlers/): Log significant operations and errors
- Use standard `log` package unless structured logging is needed
- Avoid logging and returning the same error (caller will log it)

## 7. Naming Conventions

### Files
- **snake_case** for file names: `apply.go`, `load_balancer.go`, `cert_manager.go`
- Match primary type/function name when applicable: `reconciler.go` contains `Reconciler`

### Functions & Methods
- **Exported**: PascalCase - `Apply()`, `NewReconciler()`, `GetNetworkID()`
- **Unexported**: camelCase - `loadConfig()`, `reconcileInfrastructure()`
- Verb-first naming: `reconcileCluster()` not `clusterReconcile()`
- Boolean functions: `isReady()`, `hasBootstrapped()`

### Function Naming Clarity
- **Avoid nested similar terminology**: `provisionResources()` → `provisionInfrastructure()` is confusing; use distinct names like `buildImages()` → `setupFirewall()`
- **Action-based for specificity**: `buildCacheAndFetchUsers()` over `prepareUsers()`
- **Match names to purpose**: If you're validating and saving, name it `validateAndSave()` not `process()`
- **Colocate by domain**: `getNetworkID()` belongs in network file, not orchestrator file

### Variables
- Descriptive names in wider scopes: `reconciler`, `kubeconfig`, `talosGenerator`
- Short names in tight scopes: `for i, cfg := range configs`
- No single-letter vars except standard idioms: `i`, `err`, `ctx`

### Packages
- Lowercase, single word when possible: `addons`, `cluster`, `config`
- Plural for collections: `addons` (manages multiple addon types)
- Singular for single concept: `config` (configuration management)

### Constants
- camelCase for unexported: `secretsFile`, `defaultTimeout`
- PascalCase for exported: `DefaultRetryCount`, `MaxServerCount`

## 8. When to Create New Packages

Create a new package when:

### ✅ DO create a package:
- **Domain separation**: Infrastructure vs addons vs configuration
- **Reusable components**: Used by 2+ other packages (retry, netutil)
- **External boundary**: Interacting with external systems (hcloud, talos, ssh)
- **Clear responsibility**: Can describe it in one sentence without "and"

### ❌ DON'T create a package:
- Single file with 2-3 helper functions (keep in existing package)
- "Utils" or "Helpers" - these indicate unclear boundaries
- Only used by one parent package (keep as internal file)
- No clear domain boundary (probably belongs in existing package)

### Package Size Guidelines:
- **Ideal**: 3-8 files per package
- **Small is OK**: 1-2 files if clear, focused responsibility
- **Too large**: 15+ files suggests multiple concerns - consider splitting

### When to Split a Single-File Package:

Split when a single file exceeds ~200 lines AND has clear sub-responsibilities:

**✅ DO split when**:
- File has multiple distinct responsibilities
- Functions group naturally by purpose (read/process/apply, infrastructure/compute)
- Would improve readability without adding abstraction overhead
- Different functions interact with different external systems

**❌ DON'T split when**:
- File is < 150 lines and cohesive
- Splitting would create circular dependencies
- Functions are tightly coupled and frequently call each other
- Only splitting to hit an arbitrary file count target

### Examples from our codebase:
- ✅ `internal/provisioning` - Cohesive domain (cluster provisioning), all related operations together
- ✅ `internal/addons` - Clear domain (cluster addons), reusable by both CLI and operator
- ✅ `internal/platform/hcloud` - External system boundary, many operations
- ✅ `internal/operator/controller` - CRD reconciliation, thematic file split (phases, scaling, healing, health)
- ✅ `cmd/k8zner/commands` - CLI commands separate from handlers
- ✅ `cmd/k8zner/handlers` - Business logic separate from CLI framework
- ✅ `internal/util/*` - Small, focused utilities (async, naming, labels, retry)

### Provisioning subpackage structure:
The provisioning domain is organized into focused subpackages, each with a clear single responsibility:

- **`infrastructure/`** — Network, Firewall, Load Balancers, Floating IPs
- **`compute/`** — Servers, Control Plane, Workers, Node Pools
- **`image/`** — Talos image building and snapshot management
- **`cluster/`** — Bootstrap and Talos configuration application

Each subpackage interacts with the `internal/platform/hcloud` layer for cloud operations. The provisioning pipeline (`internal/provisioning/pipeline.go`) coordinates these provisioners in the correct order. Shared interfaces and state types live at the `internal/provisioning` root level.

## 9. Operator Controller Patterns

### Thematic File Splitting

The operator controller (`internal/operator/controller/`) uses a single struct (`ClusterReconciler`) split across thematic files. Same package, same struct — no new interfaces or abstractions needed.

| File | Responsibility |
|------|---------------|
| `cluster_controller.go` | Struct, constants, options, `Reconcile()` entry point, `SetupWithManager` |
| `reconcile_phases.go` | Phase state machine (Infrastructure → Image → Compute → Bootstrap → CNI → Addons → Running) |
| `reconcile_scaling.go` | Control plane and worker scaling (up/down) |
| `reconcile_healing.go` | Self-healing node replacement |
| `reconcile_health.go` | Health checks, node readiness, cluster phase updates |
| `server_provisioning.go` | Shared provisioning helpers (create server, SSH keys, snapshot lookup) |
| `node_status.go` | Node phase tracking in CRD status |

**When to split a controller file**: when a single file exceeds ~800 lines and methods group by theme. Keep the struct definition and entry point in the main file; move method groups to thematic files.

### Config Round-Trip

Three representations of cluster configuration must stay in sync:

1. **v2 YAML spec** (`internal/config/v2/types.go`) — user-facing config file
2. **Internal Config** (`internal/config/types.go`) — runtime representation with expanded defaults
3. **CRD spec** (`api/v1alpha1/types.go`) — Kubernetes-native representation

The round-trip flows:
- **CLI path**: v2 YAML → `v2.Expand()` → internal Config → provisioning/addons
- **Operator path**: CRD spec → `adapter.SpecToConfig()` → internal Config → provisioning/addons

`SpecToConfig()` must replicate the same defaults that `v2.Expand()` sets. When adding CRD fields, trace the full round-trip to avoid silent mismatches.

### Shared Addon Installation

`internal/addons/` is shared by both paths. Entry points:
- `Apply()` — CLI path: installs everything including Cilium and Operator
- `ApplyWithoutCilium()` — Operator path: skips Cilium (installed in CNI phase) and Operator (already running)
- `ApplyCilium()` — Operator CNI phase: installs only Cilium

All three delegate to a shared `applyAddons()` with options controlling inclusion.

## 10. Generic Operations & Code Reuse

### When to Use Go Generics

Use generics to eliminate code duplication when you have:
- **Identical logic** across multiple types (3+ instances)
- **Type-safe operations** that work on different resource types
- **Clear abstraction boundaries** where the generic type doesn't leak

**Example from internal/platform/hcloud:**
The `DeleteOperation[T]` generic eliminated ~150 lines of duplicated delete logic across 9 resource types (firewalls, networks, servers, etc.). Each resource now uses a simple 7-line function instead of 27 lines of boilerplate.

### Generic Operation Patterns

**Delete Pattern:**
```go
func (c *RealClient) DeleteFirewall(ctx context.Context, name string) error {
    return (&DeleteOperation[*hcloud.Firewall]{
        Name:         name,
        ResourceType: "firewall",
        Get:          c.client.Firewall.Get,
        Delete:       c.client.Firewall.Delete,
    }).Execute(ctx, c)
}
```

**Ensure Pattern (Simple):**
```go
return (&EnsureOperation[*hcloud.Certificate, hcloud.CertificateCreateOpts, any]{
    Name:         name,
    ResourceType: "certificate",
    Get:          c.client.Certificate.Get,
    Create:       func(ctx context.Context, opts hcloud.CertificateCreateOpts) (*CreateResult[*hcloud.Certificate], *hcloud.Response, error) { ... },
    CreateOptsMapper: func() hcloud.CertificateCreateOpts { ... },
}).Execute(ctx, c)
```

**Ensure Pattern (with Update):**
```go
return (&EnsureOperation[*hcloud.Firewall, hcloud.FirewallCreateOpts, hcloud.FirewallSetRulesOpts]{
    // ... fields as above
    Update: c.client.Firewall.SetRules,
    UpdateOptsMapper: func(fw *hcloud.Firewall) hcloud.FirewallSetRulesOpts { ... },
}).Execute(ctx, c)
```

**Ensure Pattern (with Validation):**
```go
return (&EnsureOperation[*hcloud.Network, hcloud.NetworkCreateOpts, any]{
    // ... fields as above
    Validate: func(network *hcloud.Network) error {
        if network.IPRange.String() != ipRange {
            return fmt.Errorf("network exists with different IP range")
        }
        return nil
    },
}).Execute(ctx, c)
```

### When NOT to Use Generics

- Logic differs significantly between types
- Only 1-2 instances of the pattern
- Generic abstraction makes code harder to understand
- Resource has unique requirements (e.g., snapshot.DeleteImage uses ID instead of name)

### Benefits of Generic Operations

1. **Consistency:** All resources use identical retry/timeout/error handling
2. **DRY:** Single source of truth for common patterns
3. **Maintainability:** Changes to retry logic apply to all resources
4. **Type Safety:** Full compile-time type checking
5. **Testability:** Generic operations can be unit tested independently

## 11. External Commands & Resources

### Executing External Commands
- Use `exec.CommandContext` for external tools (kubectl, ssh, etc.)
- Pass context for cancellation and timeouts
- Capture and include output in error messages
- Add security comments for linters: `// #nosec G204 - path is validated/internal`

**Example**:
```go
// #nosec G204 - kubeconfigPath from internal config, tmpfile from secure temp creation
cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", tmpfile)
output, err := cmd.CombinedOutput()
if err != nil {
    return fmt.Errorf("kubectl apply failed: %w\nOutput: %s", err, string(output))
}
```

### Embedded Resources
- Use `//go:embed` for manifests, templates, and config files
- Structure: `package_name/resources/` or `package_name/manifests/`
- Templates use `.tmpl` extension: `secret.yaml.tmpl`
- Include version and source in embedded manifests

**Example**:
```go
//go:embed manifests/*
var manifestsFS embed.FS
```

### Temporary Files
- Use `os.CreateTemp` with descriptive patterns
- Always defer cleanup: `defer func() { _ = os.Remove(tmpfile.Name()) }()`
- Close files before using them with external commands
- Consider extracting if pattern is reused 3+ times

---

*This is a living document. Update it as we discover new patterns during refactoring.*
