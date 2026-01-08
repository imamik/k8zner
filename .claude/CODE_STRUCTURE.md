# Code Organization & Quality Rules

This document defines the structural patterns and quality standards for the hcloud-k8s codebase. These rules emerged from active refactoring and represent our agreed-upon approach.

## 1. Package Structure

- **cmd/**: Split commands (CLI definitions) from handlers (business logic)
- **internal/**: Organize by domain (cluster, addons, hcloud, config) not by layer
- One package = one responsibility (infrastructure ≠ addons)

## 2. Function Design

- **Flow functions**: Orchestrate steps, delegate to operations (e.g., `Apply()`)
- **Operation functions**: Do one thing, return early on errors
- Keep functions < 50 lines; split at logical boundaries if larger
- Function names describe what they do: `reconcileInfrastructure()` not `process()`

## 3. Separation of Concerns

- Infrastructure provisioning separate from addon installation
- CLI framework (cobra) isolated in commands/, never in handlers/
- Configuration loading separate from execution
- Write critical state (secrets) before risky operations (reconciliation)

## 4. Documentation

- Package-level comments explain purpose and scope
- All exported functions have godoc comments with:
  - What it does (one-line summary)
  - Key behavior details
  - Required inputs/environment variables
- Comments explain "why", not "what" (code shows what)
- Remove verbose thinking/planning comments from final code

## 5. Dependencies

- Trust dependencies to validate their inputs (don't duplicate validation)
- Simple module path (`hcloud-k8s` not `github.com/...`)
- Minimal abstractions - only create interfaces when needed (2+ implementations)

## 6. Error Handling

- Wrap errors with context: `fmt.Errorf("failed to X: %w", err)`
- Return errors, don't log and continue
- No defensive checks for impossible states
- Let clients handle validation, don't duplicate it

## 7. Naming Conventions

### Files
- **camelCase** for file names: `apply.go`, `applyHandler.go`
- Match primary type/function name when applicable: `reconciler.go` contains `Reconciler`

### Functions & Methods
- **Exported**: PascalCase - `Apply()`, `NewReconciler()`, `GetNetworkID()`
- **Unexported**: camelCase - `loadConfig()`, `reconcileInfrastructure()`
- Verb-first naming: `reconcileCluster()` not `clusterReconcile()`
- Boolean functions: `isReady()`, `hasBootstrapped()`

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

### Examples from our codebase:
- ✅ `internal/addons` - Clear domain (cluster addons), reusable, external boundary (kubectl)
- ✅ `internal/hcloud` - External system boundary, many operations
- ✅ `cmd/hcloud-k8s/commands` - CLI commands separate from handlers
- ✅ `cmd/hcloud-k8s/handlers` - Business logic separate from CLI framework

---

*This is a living document. Update it as we discover new patterns during refactoring.*
