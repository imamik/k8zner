# Implementation Plan: Simplified Config v2

## Overview

Complete overhaul of k8zner to implement the simplified, opinionated configuration schema.
This is a breaking change that prioritizes simplicity, security, and maintainability.

## Guiding Principles

1. **Test-Driven Development** — Write tests first, then implementation
2. **Incremental Progress** — Each iteration produces working code
3. **Backwards Incompatible** — Clean break, no legacy support
4. **Delete More Than Add** — Simplification means removal

## Implementation Phases

### Phase 1: Foundation (New Config Types + Validation)

**Goal:** New config types with comprehensive validation and tests.

**Tasks:**
1. Create `internal/config/v2/types.go` with new simplified types
2. Create `internal/config/v2/types_test.go` with exhaustive validation tests
3. Create `internal/config/v2/versions.go` with pinned version matrix
4. Create `internal/config/v2/versions_test.go`
5. Create `internal/config/v2/loader.go` for YAML loading
6. Create `internal/config/v2/loader_test.go`
7. Create `internal/config/v2/defaults.go` for computed defaults
8. Create `internal/config/v2/defaults_test.go`

**Deliverable:** Fully tested new config package that can load and validate simplified configs.

### Phase 2: Cost Calculator

**Goal:** `k8zner cost` command with live Hetzner pricing.

**Tasks:**
1. Create `internal/pricing/client.go` — Hetzner pricing API client
2. Create `internal/pricing/client_test.go`
3. Create `internal/pricing/calculator.go` — Cost calculation logic
4. Create `internal/pricing/calculator_test.go`
5. Create `internal/pricing/formatter.go` — Pretty terminal output
6. Create `internal/pricing/formatter_test.go`
7. Create `cmd/k8zner/commands/cost.go` — CLI command
8. Create `cmd/k8zner/commands/cost_test.go`

**Deliverable:** Working `k8zner cost` command that shows real-time pricing.

### Phase 3: Config Expansion (v2 → Internal)

**Goal:** Expand simplified config to full internal representation for provisioning.

**Tasks:**
1. Create `internal/config/v2/expand.go` — Expands v2 config to provisioning config
2. Create `internal/config/v2/expand_test.go`
3. Map Mode → control plane count, LB count
4. Map Workers → single worker pool
5. Generate hardcoded addon configs (Cilium, Traefik, cert-manager, etc.)
6. Generate network config (IPv6-only, firewall rules)
7. Generate all computed values (names, labels, etc.)

**Deliverable:** Function that converts 5-field config to full provisioning spec.

### Phase 4: Infrastructure Provisioning Updates

**Goal:** Update provisioning to support IPv6-only nodes and new LB topology.

**Tasks:**
1. Update `internal/platform/hcloud/server.go` — IPv6-only server creation
2. Update `internal/platform/hcloud/server_test.go`
3. Update `internal/provisioning/infrastructure/network.go` — Firewall rules
4. Update `internal/provisioning/infrastructure/network_test.go`
5. Update `internal/provisioning/infrastructure/loadbalancer.go` — Shared/separate LB logic
6. Update `internal/provisioning/infrastructure/loadbalancer_test.go`
7. Update `internal/platform/talos/config.go` — IPv6 network config
8. Update `internal/platform/talos/config_test.go`

**Deliverable:** Infrastructure provisioning works with new IPv6-only, firewall-locked architecture.

### Phase 5: Addon Hardcoding

**Goal:** Remove addon configurability, hardcode best practices.

**Tasks:**
1. Create `internal/addons/v2/stack.go` — Defines the fixed addon stack
2. Create `internal/addons/v2/stack_test.go`
3. Create `internal/addons/v2/cilium.go` — Hardcoded Cilium config
4. Create `internal/addons/v2/cilium_test.go`
5. Create `internal/addons/v2/traefik.go` — Hardcoded Traefik config
6. Create `internal/addons/v2/traefik_test.go`
7. Create `internal/addons/v2/certmanager.go` — Hardcoded cert-manager config
8. Create `internal/addons/v2/certmanager_test.go`
9. Create `internal/addons/v2/externaldns.go` — Hardcoded external-dns (when domain set)
10. Create `internal/addons/v2/externaldns_test.go`
11. Create `internal/addons/v2/argocd.go` — Hardcoded ArgoCD config
12. Create `internal/addons/v2/argocd_test.go`
13. Create `internal/addons/v2/metrics.go` — Hardcoded metrics-server config
14. Create `internal/addons/v2/metrics_test.go`

**Deliverable:** Fixed addon stack with zero configurability, all best practices baked in.

### Phase 6: CLI Overhaul

**Goal:** Simplified CLI commands using new config.

**Tasks:**
1. Update `cmd/k8zner/commands/init.go` — Simplified wizard (5 questions)
2. Update `cmd/k8zner/commands/init_test.go`
3. Update `cmd/k8zner/commands/up.go` — Uses v2 config
4. Update `cmd/k8zner/commands/up_test.go`
5. Update `cmd/k8zner/commands/down.go` — Uses v2 config
6. Update `cmd/k8zner/commands/down_test.go`
7. Update `cmd/k8zner/commands/status.go` — Simplified status output
8. Update `cmd/k8zner/commands/status_test.go`
9. Remove obsolete commands/flags

**Deliverable:** Clean CLI with minimal commands, all using v2 config.

### Phase 7: Orchestration Updates

**Goal:** Update orchestration layer to use v2 config expansion.

**Tasks:**
1. Update `internal/orchestration/provisioner.go` — Use expanded v2 config
2. Update `internal/orchestration/provisioner_test.go`
3. Update `internal/orchestration/phases.go` — Simplified phase list
4. Update `internal/orchestration/phases_test.go`
5. Remove unused orchestration code

**Deliverable:** Orchestration works end-to-end with v2 config.

### Phase 8: Cleanup & Deletion

**Goal:** Remove all legacy code, reduce codebase size.

**Tasks:**
1. Delete `internal/config/types.go` (old 1000+ line types)
2. Delete `internal/config/validate.go` (old validation)
3. Delete `internal/config/wizard/` (old wizard)
4. Delete unused addon configs in `internal/addons/`
5. Delete ARM64 support code
6. Delete multi-pool worker code
7. Delete ingress-nginx code (Traefik only)
8. Delete OIDC configuration code
9. Delete autoscaler configuration code
10. Update all imports
11. Remove dead code detected by linter

**Deliverable:** Lean codebase with ~40% less code.

### Phase 9: Documentation & Examples

**Goal:** Update all documentation for v2.

**Tasks:**
1. Rewrite `README.md` — Simplified quick start
2. Rewrite `docs/configuration.md` — 5-field config reference
3. Update `docs/architecture.md` — IPv6-only, firewall architecture
4. Update `examples/` — New minimal examples
5. Delete obsolete documentation
6. Update CHANGELOG.md

**Deliverable:** Complete, accurate documentation for v2.

### Phase 10: Integration & E2E Testing

**Goal:** Verify everything works end-to-end.

**Tasks:**
1. Update E2E test configs to v2 format
2. Run full E2E test suite
3. Test dev mode (1 CP, shared LB)
4. Test HA mode (3 CP, separate LBs)
5. Test with domain (DNS + TLS)
6. Test without domain
7. Verify IPv6-only connectivity
8. Verify firewall blocks inbound
9. Verify addon stack works

**Deliverable:** Passing E2E tests, production-ready release.

## File Changes Summary

### New Files (~15 files)
- `internal/config/v2/types.go`
- `internal/config/v2/types_test.go`
- `internal/config/v2/versions.go`
- `internal/config/v2/versions_test.go`
- `internal/config/v2/loader.go`
- `internal/config/v2/loader_test.go`
- `internal/config/v2/defaults.go`
- `internal/config/v2/defaults_test.go`
- `internal/config/v2/expand.go`
- `internal/config/v2/expand_test.go`
- `internal/pricing/client.go`
- `internal/pricing/client_test.go`
- `internal/pricing/calculator.go`
- `internal/pricing/calculator_test.go`
- `cmd/k8zner/commands/cost.go`

### Modified Files (~20 files)
- `internal/platform/hcloud/server.go`
- `internal/provisioning/infrastructure/*.go`
- `internal/platform/talos/config.go`
- `internal/orchestration/*.go`
- `cmd/k8zner/commands/*.go`
- `README.md`
- `docs/*.md`

### Deleted Files (~30+ files)
- `internal/config/types.go`
- `internal/config/validate.go`
- `internal/config/wizard/*.go`
- Various addon config files
- ARM64 support files
- Legacy test files

## Success Criteria

1. **Config simplicity**: Full cluster in 12 lines of YAML
2. **Test coverage**: >90% on new code
3. **Code reduction**: ~40% less code than before
4. **Working E2E**: Both dev and HA modes deploy successfully
5. **IPv6-only**: Nodes have no IPv4, only reachable via LB
6. **Cost calculator**: Accurate pricing with live Hetzner data

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| IPv6-only breaks image pulls | Test major registries (Docker Hub, ghcr.io, quay.io) |
| Shared LB port conflicts | Use different ports (6443 for API, 80/443 for ingress) |
| Breaking existing users | Clear migration docs, no auto-upgrade |
| Cilium IPv6 issues | Test Cilium thoroughly in IPv6-only mode |

## Timeline

- Phase 1-2: Foundation + Cost (Day 1)
- Phase 3-4: Expansion + Infrastructure (Day 1-2)
- Phase 5-6: Addons + CLI (Day 2)
- Phase 7-8: Orchestration + Cleanup (Day 2-3)
- Phase 9-10: Docs + E2E (Day 3)

Total: ~3 days of focused work
