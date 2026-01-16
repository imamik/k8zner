# E2E Testing Guide

## Overview

The E2E tests validate the entire hcloud-k8s stack, from snapshot creation to cluster provisioning, addon deployment, scaling, and upgrades. Tests are modular and can run specific phases independently for faster development cycles.

## Test Architecture

### Sequential Lifecycle Test

The main test (`TestE2ELifecycle`) runs through 5 phases:

1. **Snapshots** - Build Talos images (amd64/arm64)
2. **Cluster** - Provision infrastructure and bootstrap Kubernetes
3. **Addons** - Install and test addons (CCM, CSI, etc.)
4. **Scale** - Test cluster scaling
5. **Upgrade** - Test Talos/K8s version upgrades

Each phase can be skipped independently using environment variables for faster testing.

### Standalone Tests

- **Snapshot Build** - Dedicated snapshot verification test
- **Standalone Upgrade** - Isolated upgrade test with fresh cluster

## Quick Start

### Full Test (All Phases)

```bash
export HCLOUD_TOKEN="your-token"
cd tests/e2e
go test -v -timeout=1h -tags=e2e -run TestE2ELifecycle .
```

**Duration:** 35-45 minutes

### Quick Test (Skip Slow Phases)

```bash
# Skip scale and upgrade for faster iteration
E2E_SKIP_SCALE=true E2E_SKIP_UPGRADE=true \
go test -v -timeout=30m -tags=e2e -run TestE2ELifecycle .
```

**Duration:** 20-30 minutes

### Using Helper Script

```bash
# Full test
./run-upgrade-test.sh full

# Quick test
./run-upgrade-test.sh quick

# Upgrade only
./run-upgrade-test.sh upgrade-only

# Standalone upgrade test
./run-upgrade-test.sh standalone
```

## Modular Testing

### Skip Individual Phases

Each phase can be controlled via environment variables:

```bash
# Skip snapshot building (use existing snapshots)
E2E_SKIP_SNAPSHOTS=true

# Skip cluster provisioning (use existing cluster)
E2E_SKIP_CLUSTER=true

# Skip addon testing
E2E_SKIP_ADDONS=true

# Skip scale testing
E2E_SKIP_SCALE=true

# Skip upgrade testing
E2E_SKIP_UPGRADE=true
```

**Example:** Test only addons on existing cluster:

```bash
E2E_REUSE_CLUSTER=true \
E2E_CLUSTER_NAME=my-cluster \
E2E_KUBECONFIG_PATH=./kubeconfig \
E2E_SKIP_SNAPSHOTS=true \
E2E_SKIP_CLUSTER=true \
E2E_SKIP_SCALE=true \
E2E_SKIP_UPGRADE=true \
go test -v -timeout=15m -tags=e2e -run TestE2ELifecycle .
```

### Reuse Existing Cluster

Speed up testing by reusing an already-deployed cluster:

```bash
# 1. Deploy cluster once
./hcloud-k8s apply --config config.yaml

# 2. Run tests on existing cluster
E2E_REUSE_CLUSTER=true \
E2E_CLUSTER_NAME=my-cluster \
E2E_KUBECONFIG_PATH=./kubeconfig \
E2E_SKIP_SNAPSHOTS=true \
E2E_SKIP_CLUSTER=true \
go test -v -timeout=20m -tags=e2e -run TestE2ELifecycle .
```

## Environment Variables

### Required

| Variable | Description |
|----------|-------------|
| `HCLOUD_TOKEN` | Hetzner Cloud API token (can be in `.env` file) |

### Phase Control

| Variable | Default | Description |
|----------|---------|-------------|
| `E2E_SKIP_SNAPSHOTS` | `false` | Skip snapshot building phase |
| `E2E_SKIP_CLUSTER` | `false` | Skip cluster provisioning phase |
| `E2E_SKIP_ADDONS` | `false` | Skip addon testing phase |
| `E2E_SKIP_SCALE` | `false` | Skip scale testing phase |
| `E2E_SKIP_UPGRADE` | `false` | Skip upgrade testing phase |

### Cluster Reuse

| Variable | Default | Description |
|----------|---------|-------------|
| `E2E_REUSE_CLUSTER` | `false` | Use existing cluster instead of creating new one |
| `E2E_CLUSTER_NAME` | - | Name of cluster to reuse (required if reusing) |
| `E2E_KUBECONFIG_PATH` | - | Path to kubeconfig (required if reusing) |

### Snapshot Management

| Variable | Default | Description |
|----------|---------|-------------|
| `E2E_KEEP_SNAPSHOTS` | `false` | Keep snapshots between test runs (faster re-runs) |

### Version Control (Upgrade Tests)

| Variable | Default | Description |
|----------|---------|-------------|
| `E2E_INITIAL_TALOS_VERSION` | `v1.8.2` | Initial Talos version for upgrade tests |
| `E2E_TARGET_TALOS_VERSION` | `v1.8.3` | Target Talos version for upgrade tests |
| `E2E_INITIAL_K8S_VERSION` | `v1.30.0` | Initial K8s version for upgrade tests |
| `E2E_TARGET_K8S_VERSION` | `v1.31.0` | Target K8s version for upgrade tests |

## Use Cases

### 1. Feature Development

Iterate quickly on specific features:

```bash
# Deploy cluster once with cached snapshots
E2E_KEEP_SNAPSHOTS=true E2E_SKIP_SCALE=true E2E_SKIP_UPGRADE=true \
go test -v -timeout=30m -tags=e2e -run TestE2ELifecycle .

# Note cluster name from output
export E2E_CLUSTER_NAME=e2e-seq-1234567890
export E2E_KUBECONFIG_PATH=/tmp/kubeconfig-e2e-seq-1234567890

# Then iterate on code and retest specific phases
while [ 1 ]; do
    # Make changes
    vim internal/...

    # Rebuild
    go build -o ../../hcloud-k8s ../../cmd/hcloud-k8s

    # Test specific phase
    E2E_REUSE_CLUSTER=true \
    E2E_SKIP_SNAPSHOTS=true \
    E2E_SKIP_CLUSTER=true \
    E2E_SKIP_SCALE=true \
    E2E_SKIP_UPGRADE=true \
    go test -v -timeout=15m -tags=e2e -run TestE2ELifecycle .

    read -p "Continue? " -n 1 -r
    echo
    [[ ! $REPLY =~ ^[Yy]$ ]] && break
done
```

### 2. Upgrade Testing

Test upgrade functionality in isolation:

```bash
# Option 1: Standalone test (fresh cluster)
go test -v -timeout=45m -tags=e2e -run TestE2EUpgradeStandalone .

# Option 2: Test on existing cluster
E2E_REUSE_CLUSTER=true \
E2E_CLUSTER_NAME=my-cluster \
E2E_KUBECONFIG_PATH=./kubeconfig \
E2E_SKIP_SNAPSHOTS=true \
E2E_SKIP_CLUSTER=true \
E2E_SKIP_ADDONS=true \
E2E_SKIP_SCALE=true \
go test -v -timeout=30m -tags=e2e -run TestE2ELifecycle .
```

### 3. Addon Development

Focus on addon functionality:

```bash
# Build cluster with other phases cached
E2E_KEEP_SNAPSHOTS=true \
E2E_SKIP_SCALE=true \
E2E_SKIP_UPGRADE=true \
go test -v -timeout=30m -tags=e2e -run TestE2ELifecycle .

# Then iterate on addon code
E2E_REUSE_CLUSTER=true \
E2E_CLUSTER_NAME=<cluster-name> \
E2E_KUBECONFIG_PATH=/tmp/kubeconfig-<cluster-name> \
E2E_SKIP_SNAPSHOTS=true \
E2E_SKIP_CLUSTER=true \
E2E_SKIP_SCALE=true \
E2E_SKIP_UPGRADE=true \
go test -v -timeout=15m -tags=e2e -run TestE2ELifecycle .
```

### 4. CI/CD: Full Validation

```bash
# Always run full test in CI
export HCLOUD_TOKEN="${{ secrets.HCLOUD_TOKEN }}"
export E2E_KEEP_SNAPSHOTS=false
go test -v -timeout=1h -tags=e2e -run TestE2ELifecycle ./tests/e2e/
```

## Performance

| Configuration | Duration | Notes |
|--------------|----------|-------|
| Full (all phases, no cache) | 40-50 min | First run |
| Full (with cached snapshots) | 30-40 min | E2E_KEEP_SNAPSHOTS=true |
| Quick (skip scale/upgrade) | 20-30 min | Faster iteration |
| Upgrade only | 10-15 min | On existing cluster |
| Standalone upgrade | 25-35 min | Fresh cluster with upgrade |
| Addon testing only | 10-15 min | On existing cluster |

## Test Files

### Core Tests

- `sequential_test.go` - Main lifecycle test with modular phases
- `upgrade_standalone_test.go` - Dedicated upgrade test

### Phase Implementations

- `phase_snapshots.go` - Snapshot building
- `phase_cluster.go` - Cluster provisioning
- `phase_addons.go` - Addon testing
- `phase_scale.go` - Scaling tests
- `phase_upgrade.go` - Upgrade tests

### Infrastructure

- `suite_test.go` - Test suite setup
- `state.go` - Test state management
- `config.go` - Configuration and environment parsing
- `helpers.go` - Shared utilities
- `run-upgrade-test.sh` - Test runner script

## Debugging

### View Detailed Logs

```bash
go test -v -timeout=1h -tags=e2e -run TestE2ELifecycle . 2>&1 | tee test.log
```

### Run Specific Phase

```bash
# Only run upgrade phase
E2E_SKIP_SNAPSHOTS=true \
E2E_SKIP_CLUSTER=true \
E2E_SKIP_ADDONS=true \
E2E_SKIP_SCALE=true \
go test -v -timeout=30m -tags=e2e -run TestE2ELifecycle .
```

### Inspect Cluster After Test

```bash
# Set E2E_REUSE_CLUSTER to prevent cleanup
# Then manually inspect:
kubectl --kubeconfig /tmp/kubeconfig-<cluster-name> get nodes
hcloud server list | grep <cluster-name>
```

### Cleanup Orphaned Resources

```bash
# List E2E resources
hcloud server list | grep e2e
hcloud network list | grep e2e
hcloud load-balancer list | grep e2e

# Delete by cluster name
CLUSTER=e2e-seq-1234567890
hcloud server list -o json | \
  jq -r ".[] | select(.labels.cluster==\"$CLUSTER\") | .name" | \
  xargs -I {} hcloud server delete {}
```

## Best Practices

1. ✅ Use `.env` file for HCLOUD_TOKEN to avoid exposing in history
2. ✅ Cache snapshots locally with `E2E_KEEP_SNAPSHOTS=true`
3. ✅ Skip slow phases during development for faster iteration
4. ✅ Reuse clusters when testing specific phases
5. ✅ Run full test before creating pull requests
6. ✅ Clean up resources after testing
7. ❌ Don't skip snapshot build test in CI
8. ❌ Don't commit with development-only environment variables

## CI/CD Integration

### GitHub Actions Example

```yaml
name: E2E Tests

on: [pull_request]

jobs:
  e2e-full:
    name: Full E2E Test
    runs-on: ubuntu-latest
    timeout-minutes: 60
    steps:
      - uses: actions/checkout@v3

      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Run E2E Tests
        env:
          HCLOUD_TOKEN: ${{ secrets.HCLOUD_TOKEN }}
        run: |
          cd tests/e2e
          go test -v -timeout=1h -tags=e2e -run TestE2ELifecycle .

  e2e-upgrade:
    name: Upgrade Test
    runs-on: ubuntu-latest
    timeout-minutes: 45
    steps:
      - uses: actions/checkout@v3

      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Run Upgrade Test
        env:
          HCLOUD_TOKEN: ${{ secrets.HCLOUD_TOKEN }}
        run: |
          cd tests/e2e
          go test -v -timeout=45m -tags=e2e -run TestE2EUpgradeStandalone .
```

## Contributing

When adding new test phases:

1. Create `phase_<name>.go` with implementation
2. Add phase control to `config.go` (`E2E_SKIP_<NAME>`)
3. Integrate into `sequential_test.go`
4. Update this README with usage examples
5. Add test case to `run-upgrade-test.sh`

## Related Documentation

- [UPGRADE.md](../../UPGRADE.md) - Upgrade feature documentation
- [README.md](../../README.md) - Project documentation
- Makefile targets: `make e2e`, `make e2e-fast`
