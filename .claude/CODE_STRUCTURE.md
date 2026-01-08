# Code Organization & Quality Rules

This document defines the structural patterns and quality standards for the hcloud-k8s codebase. These rules emerged from active refactoring and represent our agreed-upon approach.

## 1. Package Structure

- **cmd/**: Split commands (CLI definitions) from handlers (business logic)
- **internal/**: Organize by domain and responsibility:
  - **provisioning/**: All cluster provisioning (compute, infrastructure, images, bootstrap)
  - **addons/**: Addon installation
  - **config/**: Configuration management
  - **platform/**: External system integrations (hcloud, talos, ssh)
  - **util/**: Reusable utilities (async, labels, naming, retry, etc.)
- One package = one responsibility - provisioning is acceptable as a larger package when the domain is cohesive

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
- Provide context in error messages: what operation failed, what was being operated on
- Example: `fmt.Errorf("failed to read manifests for addon %s at %s: %w", name, path, err)`

## 6.1. Logging

- **Library code** (internal/): Return errors, let callers decide logging
- **Command handlers** (cmd/handlers/): Log significant operations and errors
- Use standard `log` package unless structured logging is needed
- Avoid logging and returning the same error (caller will log it)

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
- ✅ `internal/addons` - Clear domain (cluster addons), reusable, external boundary (kubectl)
- ✅ `internal/platform/hcloud` - External system boundary, many operations
- ✅ `cmd/hcloud-k8s/commands` - CLI commands separate from handlers
- ✅ `cmd/hcloud-k8s/handlers` - Business logic separate from CLI framework
- ✅ `internal/util/*` - Small, focused utilities (async, naming, labels, retry)

### Why provisioning/ is a flat package:
The provisioning package contains ~17 files but represents a single cohesive domain: **cluster provisioning**. All files work together to provision infrastructure, compute resources, images, and bootstrap Talos clusters. Splitting into subpackages (compute/, infrastructure/, etc.) would require creating separate types and complex cross-package dependencies due to Go's package system. The flat structure keeps the codebase simple while maintaining clear file organization through descriptive naming (control_plane.go, network.go, bootstrap.go, etc.).

## 9. External Commands & Resources

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
