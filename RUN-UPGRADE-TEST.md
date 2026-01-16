# Running the Upgrade E2E Test

This guide walks you through running the actual upgrade E2E test to verify the feature works end-to-end.

## Prerequisites Checklist

- [ ] Go 1.21+ installed
- [ ] kubectl installed
- [ ] HCLOUD_TOKEN set (in .env file or environment)
- [ ] Sufficient Hetzner Cloud quota (2+ servers)
- [ ] ~30-45 minutes of free time

## Quick Test (Recommended First)

This tests the upgrade functionality in isolation:

```bash
# 1. Navigate to E2E test directory
cd /home/agent/hcloud-kubernetes/tests/e2e

# 2. Load environment from .env
export $(cat ../../.env | grep -v '^#' | xargs)

# 3. Verify token is set
echo "HCLOUD_TOKEN=$HCLOUD_TOKEN"

# 4. Build the binary first
cd /home/agent/hcloud-kubernetes
go build -o hcloud-k8s ./cmd/hcloud-k8s

# 5. Run standalone upgrade test
cd tests/e2e
go test -v -timeout=45m -tags=e2e -run TestE2EUpgradeStandalone .
```

**Duration:** 25-35 minutes

## What the Test Does

1. **Build Initial Snapshots** (v1.8.2 / v1.30.0)
   - Creates Talos image for older version
   - Takes ~7-10 minutes

2. **Deploy Initial Cluster**
   - 1 control plane + 1 worker
   - Bootstrap Kubernetes v1.30.0
   - Verify cluster health
   - Takes ~5-8 minutes

3. **Verify Initial State**
   - Check nodes are Ready
   - Verify K8s version is v1.30.0
   - Check system pods

4. **Build Target Snapshots** (v1.8.3 / v1.31.0)
   - Creates Talos image for newer version
   - May use cached snapshot if available
   - Takes ~7-10 minutes (or 0 if cached)

5. **Execute Upgrade**
   - Dry run first
   - Upgrade control plane (sequential)
   - Upgrade worker
   - Upgrade Kubernetes control plane
   - Health checks between steps
   - Takes ~8-12 minutes

6. **Verify Upgraded State**
   - Wait for nodes to stabilize
   - Check nodes are Ready
   - Verify K8s version is v1.31.0
   - Check system pods

7. **Test Workload**
   - Deploy nginx
   - Verify deployment works
   - Cleanup

8. **Cleanup**
   - Destroy cluster resources

## Expected Output

### Successful Test Output

```
=== RUN   TestE2EUpgradeStandalone
    upgrade_standalone_test.go:XX: Starting E2E Upgrade Test
    upgrade_standalone_test.go:XX: Cluster: e2e-upg-1234567890
    upgrade_standalone_test.go:XX: Initial: Talos v1.8.2 / K8s v1.30.0
    upgrade_standalone_test.go:XX: Target:  Talos v1.8.3 / K8s v1.31.0
=== RUN   TestE2EUpgradeStandalone/BuildInitialSnapshots
    upgrade_standalone_test.go:XX: Building snapshots for Talos v1.8.2 / K8s v1.30.0...
    upgrade_standalone_test.go:XX: ✓ Snapshot built: 12345678
=== RUN   TestE2EUpgradeStandalone/DeployInitialCluster
    upgrade_standalone_test.go:XX: Deploying cluster with Talos v1.8.2 / K8s v1.30.0...
    upgrade_standalone_test.go:XX: ✓ Cluster provisioned in 6m30s
=== RUN   TestE2EUpgradeStandalone/VerifyInitialCluster
    upgrade_standalone_test.go:XX: Verifying cluster (expected K8s version: v1.30.0)...
    upgrade_standalone_test.go:XX: ✓ Cluster verification passed
=== RUN   TestE2EUpgradeStandalone/BuildTargetSnapshots
    upgrade_standalone_test.go:XX: Building snapshots for Talos v1.8.3 / K8s v1.31.0...
    upgrade_standalone_test.go:XX: ✓ Snapshot built: 12345679
=== RUN   TestE2EUpgradeStandalone/UpgradeCluster
    upgrade_standalone_test.go:XX: Upgrading cluster to Talos v1.8.3 / K8s v1.31.0...
    upgrade_standalone_test.go:XX: Upgrade output:
    [Upgrade] Starting cluster upgrade for: e2e-upg-1234567890
    [Upgrade] Validating configuration...
    [Upgrade] Found 2 nodes in cluster
    [Upgrade] Upgrading control plane nodes...
    [Upgrade] Found 1 control plane nodes
    [Upgrade] Upgrading control plane node 1/1 (10.0.0.10)...
    [Upgrade] Node 10.0.0.10: v1.8.2 → v1.8.3
    [Upgrade] Waiting for node 10.0.0.10 to reboot...
    [Upgrade] Node 10.0.0.10 upgraded successfully
    [Upgrade] Checking cluster health after node 10.0.0.10...
    [Upgrade] Cluster health check passed
    [Upgrade] Control plane upgrade completed
    [Upgrade] Upgrading worker nodes...
    [Upgrade] Found 1 worker nodes
    [Upgrade] Upgrading worker node 1/1 (10.0.1.10)...
    [Upgrade] Node 10.0.1.10: v1.8.2 → v1.8.3
    [Upgrade] Waiting for node 10.0.1.10 to reboot...
    [Upgrade] Node 10.0.1.10 upgraded successfully
    [Upgrade] Worker upgrade completed
    [Upgrade] Upgrading Kubernetes to version v1.31.0...
    [Upgrade] Kubernetes upgrade completed
    [Upgrade] Performing final health check...
    [Upgrade] Cluster health check passed
    [Upgrade] Cluster upgrade completed successfully
    upgrade_standalone_test.go:XX: ✓ Upgrade completed in 10m15s
=== RUN   TestE2EUpgradeStandalone/VerifyUpgradedCluster
    upgrade_standalone_test.go:XX: Verifying cluster (expected K8s version: v1.31.0)...
    upgrade_standalone_test.go:XX: ✓ Cluster verification passed
=== RUN   TestE2EUpgradeStandalone/TestWorkloadAfterUpgrade
    upgrade_standalone_test.go:XX: Testing workload deployment...
    upgrade_standalone_test.go:XX: ✓ Workload deployment successful
    upgrade_standalone_test.go:XX: ✓ E2E Upgrade Test Completed Successfully
--- PASS: TestE2EUpgradeStandalone (28m15s)
    --- PASS: TestE2EUpgradeStandalone/BuildInitialSnapshots (9m12s)
    --- PASS: TestE2EUpgradeStandalone/DeployInitialCluster (6m30s)
    --- PASS: TestE2EUpgradeStandalone/VerifyInitialCluster (1m05s)
    --- PASS: TestE2EUpgradeStandalone/BuildTargetSnapshots (0m15s)
    --- PASS: TestE2EUpgradeStandalone/UpgradeCluster (10m15s)
    --- PASS: TestE2EUpgradeStandalone/VerifyUpgradedCluster (45s)
    --- PASS: TestE2EUpgradeStandalone/TestWorkloadAfterUpgrade (13s)
PASS
ok      hcloud-k8s/tests/e2e    1695.123s
```

## Alternative: Test as Part of Full Lifecycle

This tests upgrade as part of the complete lifecycle:

```bash
cd /home/agent/hcloud-kubernetes/tests/e2e

# Full test (all phases)
go test -v -timeout=1h -tags=e2e -run TestE2ELifecycle .

# Quick test (skip scale, include upgrade)
E2E_SKIP_SCALE=true \
go test -v -timeout=40m -tags=e2e -run TestE2ELifecycle .
```

## Troubleshooting

### Issue: "go: command not found"

```bash
# Install Go
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
go version
```

### Issue: "HCLOUD_TOKEN not set"

```bash
# Check .env file
cat /home/agent/hcloud-kubernetes/.env

# Or export manually
export HCLOUD_TOKEN=your-token-here
```

### Issue: Test hangs or times out

```bash
# Check Hetzner Cloud resources
hcloud server list | grep e2e-upg

# Check network connectivity
ping 8.8.8.8

# Increase timeout
go test -v -timeout=60m -tags=e2e -run TestE2EUpgradeStandalone .
```

### Issue: Snapshot build fails

```bash
# Check Hetzner Cloud quota
hcloud server list
hcloud snapshot list

# Clean up old snapshots
hcloud snapshot list | grep e2e | awk '{print $1}' | xargs -I {} hcloud snapshot delete {}
```

### Issue: Upgrade fails

Check upgrade logs in test output. Common issues:
- Network connectivity during image download
- Insufficient node resources
- Version compatibility issues

## Verify Results Manually

After test completes, you can manually verify (if resources not cleaned up):

```bash
# Set cluster name from test output
export CLUSTER=e2e-upg-1234567890

# Check servers exist
hcloud server list | grep $CLUSTER

# Get kubeconfig
export KUBECONFIG=/tmp/kubeconfig-$CLUSTER

# Check nodes
kubectl get nodes -o wide

# Check versions
kubectl version

# Cleanup manually
./hcloud-k8s destroy --config /tmp/upgrade-config-$CLUSTER.yaml
```

## Performance Expectations

| Phase | Expected Duration | Notes |
|-------|-------------------|-------|
| Initial snapshots | 7-10 min | First build |
| Initial cluster | 5-8 min | Provisioning |
| Initial verification | 1-2 min | Health checks |
| Target snapshots | 0-10 min | May use cache |
| Upgrade execution | 8-12 min | Node reboots |
| Upgrade verification | 1-2 min | Health checks |
| Workload test | <1 min | Quick deploy |
| Cleanup | 1-2 min | Resource deletion |
| **Total** | **25-35 min** | End-to-end |

## Success Criteria

The test passes if:

1. ✅ Initial cluster deploys with v1.8.2 / v1.30.0
2. ✅ Initial cluster nodes are Ready
3. ✅ Upgrade command executes without errors
4. ✅ Control plane upgrades sequentially
5. ✅ Health checks pass after each CP node
6. ✅ Worker nodes upgrade successfully
7. ✅ Kubernetes version updates to v1.31.0
8. ✅ Upgraded cluster nodes return to Ready
9. ✅ System pods restart successfully
10. ✅ New workload deploys after upgrade
11. ✅ No errors in test output
12. ✅ Resources cleanup successfully

## Next Steps After Successful Test

1. Review test output and timing
2. Update PR with test results
3. Consider running full lifecycle test
4. Merge feature branch
5. Close related issues

## Need Help?

If you encounter issues:

1. Check test output for error messages
2. Review Hetzner Cloud console for resource status
3. Check `kubectl` and `talosctl` commands manually
4. Review logs in `/tmp/` directory
5. Ask for help with specific error messages

## Related Documentation

- [tests/e2e/README.md](tests/e2e/README.md) - Comprehensive E2E testing guide
- [UPGRADE.md](UPGRADE.md) - Upgrade feature documentation
- [README.md](README.md) - Project documentation
