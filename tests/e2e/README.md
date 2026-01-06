# E2E Testing Guide

## Overview

The E2E tests validate the entire hcloud-k8s stack, from snapshot creation to cluster provisioning.

## Test Strategy

### Fast Tests with Cached Snapshots

Most tests use **cached snapshots** for speed:
- `TestImageBuildLifecycle` - Verifies servers boot from snapshots
- `TestApplyConfig` - Tests Talos configuration
- `TestClusterProvisioning` - Tests full cluster setup
- `TestParallelProvisioning` - Tests parallel resource creation
- `TestSimpleTalosNode` - Basic connectivity tests

### Snapshot Build Verification

**`TestSnapshotCreation`** - Dedicated test that:
- ✅ **ALWAYS** builds fresh snapshots (ignores cache)
- ✅ Tests the complete snapshot build process
- ✅ Verifies snapshots boot correctly
- ✅ Catches regressions in image builder
- ✅ Should ALWAYS run in CI

## Running Tests

### Full Test Run (Default)
```bash
export HCLOUD_TOKEN="your-token"
make e2e
```
**Time:** ~11-12 minutes
- Builds snapshots once
- Runs all tests in parallel
- Tests snapshot build process
- Cleans up snapshots

### Fast Local Development
```bash
export HCLOUD_TOKEN="your-token"
export E2E_KEEP_SNAPSHOTS=true          # Keep snapshots between runs
export E2E_SKIP_SNAPSHOT_BUILD_TEST=true # Skip slow snapshot build test
make e2e
```
**First run:** ~11-12 minutes (builds snapshots)
**Subsequent runs:** ~8 minutes (reuses snapshots)

⚠️ **WARNING:** Don't use `E2E_SKIP_SNAPSHOT_BUILD_TEST=true` in CI!

### CI Configuration
```bash
export HCLOUD_TOKEN="${{ secrets.HCLOUD_TOKEN }}"
export E2E_KEEP_SNAPSHOTS=false  # Always clean up in CI
# Never set E2E_SKIP_SNAPSHOT_BUILD_TEST in CI!
make e2e
```

## Test Parallelization

Tests run in parallel for speed:
- Each test gets a unique cluster name (timestamp-based)
- Resources are isolated by labels
- Cleanup is automatic via `t.Cleanup()`

**Non-parallel tests:**
- `TestInfraProvisioning` - Runs first, not parallel
- `TestSnapshotCreation` - Resource-intensive, runs sequentially

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `HCLOUD_TOKEN` | (required) | Hetzner Cloud API token |
| `E2E_KEEP_SNAPSHOTS` | `false` | Keep snapshots between runs for speed |
| `E2E_SKIP_SNAPSHOT_BUILD_TEST` | `false` | Skip snapshot build test (⚠️ NOT for CI!) |

## Test Coverage

### What Each Test Validates

1. **TestSnapshotCreation** ⚡ Critical
   - Snapshot build process works
   - Talos images are correctly provisioned
   - Servers boot from fresh snapshots
   - **Purpose:** Catch snapshot build regressions

2. **TestImageBuildLifecycle**
   - Servers boot from cached snapshots
   - Talos API becomes accessible
   - Basic snapshot functionality

3. **TestSimpleTalosNode**
   - Minimal Talos deployment
   - Port connectivity
   - Maintenance mode behavior

4. **TestApplyConfig**
   - Machine configuration generation
   - Config application to nodes
   - Node reboots and authentication

5. **TestClusterProvisioning**
   - Full cluster creation
   - Network, firewall, load balancer setup
   - Control plane + worker provisioning
   - API connectivity

6. **TestParallelProvisioning**
   - Multiple control plane nodes
   - Multiple worker pools
   - Parallel resource creation
   - Etcd bootstrapping

7. **TestInfraProvisioning**
   - Infrastructure reconciliation
   - Resource verification
   - Error handling

## Performance

| Configuration | Time | Use Case |
|--------------|------|----------|
| Full run (no cache) | ~11-12 min | CI, first run |
| With cached snapshots | ~8 min | Local dev, subsequent runs |
| Sequential (old) | ~26 min | Legacy (before parallelization) |

**Speedup:** 57% faster with parallelization + caching

## Architecture

```
TestMain
  ├─ Build snapshots (parallel: amd64 + arm64)
  ├─ Check for existing snapshots (if E2E_KEEP_SNAPSHOTS=true)
  └─ Run tests in parallel
     ├─ TestSnapshotCreation (always builds fresh)
     ├─ TestImageBuildLifecycle (uses cached)
     ├─ TestApplyConfig (uses cached)
     ├─ TestClusterProvisioning (uses cached)
     ├─ TestParallelProvisioning (uses cached)
     └─ TestSimpleTalosNode (uses cached)
```

## Debugging

### View test logs
```bash
go test -v -timeout=1h -tags=e2e ./tests/e2e/...
```

### Run specific test
```bash
go test -v -timeout=1h -tags=e2e -run TestSnapshotCreation ./tests/e2e/...
```

### Keep resources for inspection
```bash
# Comment out cleanup in test
# cleaner.Add(func() { ... })
```

## Best Practices

1. ✅ **Always run `TestSnapshotCreation` in CI** to catch build regressions
2. ✅ Use cached snapshots for local development speed
3. ✅ Clean up snapshots in CI to avoid cost
4. ✅ Use unique names (timestamps) to avoid resource conflicts
5. ✅ Register cleanup handlers immediately after creating resources
6. ❌ Don't skip `TestSnapshotCreation` in CI
7. ❌ Don't commit with `E2E_SKIP_SNAPSHOT_BUILD_TEST=true` in workflows
