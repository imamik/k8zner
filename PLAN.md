# Implementation Plan: E2E Test Critical Fixes

## Executive Summary

The E2E test failures revealed **4 critical issues** that need to be fixed:

1. **Health checks not running during provisioning** - Ready counts stay at 0
2. **CCM/CSI not functioning** - hcloud secret may be missing or misconfigured
3. **Addon installation blocking** - No timeout, failures cause infinite retry
4. **HA bootstrap failures** - Additional CPs turn off after creation

---

## Issue 1: Health Check Not Running During Provisioning Phases

### Root Cause

The `reconcileHealthCheck()` function is only called in:
- `reconcileLegacy()` (line 421) - for clusters without credentialsRef
- `reconcileRunningPhase()` (line 1088) - for completed provisioning

During state machine phases (CNI, Addons, Compute), health checks are **never executed**, so:
- `cluster.Status.ControlPlanes.Ready` stays at 0
- `cluster.Status.Workers.Ready` stays at 0
- `updateClusterPhase()` sets phase to `Degraded` (Ready != Desired)

### Evidence

From test logs:
```
Cluster phase: Degraded, provisioning: Addons, CP ready: 0, workers ready: 0
```
The cluster showed `CP ready: 0` for 30+ minutes even though nodes were running.

### Fix

**File:** `internal/operator/controller/cluster_controller.go`

Add health check execution at the start of `reconcile()` function (after line 346):

```go
func (r *ClusterReconciler) reconcile(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
    logger := log.FromContext(ctx)
    defer r.updateClusterPhase(cluster)

    // Keep status.Desired in sync with spec counts
    cluster.Status.ControlPlanes.Desired = cluster.Spec.ControlPlanes.Count
    cluster.Status.Workers.Desired = cluster.Spec.Workers.Count

    // Step 1: Check for stuck nodes...
    // Step 2: Verify and update node states...

    // NEW: Step 3: Run health check to update Ready counts
    // This ensures the cluster phase is accurate during all provisioning stages
    if err := r.reconcileHealthCheck(ctx, cluster); err != nil {
        logger.Error(err, "health check failed during reconciliation")
        // Continue - don't block provisioning on health check failures
    }

    // Check if this cluster needs provisioning...
}
```

### Impact

- Ready counts will be updated every reconciliation cycle
- Cluster phase will accurately reflect actual node state
- Tests can reliably wait for Ready counts to match Desired

---

## Issue 2: CCM/CSI Not Functioning (Load Balancer and Volume Provisioning)

### Root Cause

The CCM and CSI addons require the `hcloud` secret in `kube-system` namespace with:
- `token`: HCloud API token
- `network`: Network ID as string

The secret is created by `createHCloudSecret()` in `internal/addons/secret.go`, but there are two potential issues:

1. **Network ID may be 0** when addon installation runs (not populated in status)
2. **Secret creation may fail silently** or use stale data

### Evidence

From test logs:
```
[330s] Waiting for LB external IP...
CCM failed to provision load balancer
Waiting for CSI volume (attempt 37/48, phase: Pending)...
```

### Fix

**File:** `internal/operator/controller/cluster_controller.go` (reconcileAddonsPhase)

1. Add validation that network ID is valid before calling `ApplyWithoutCilium`:

```go
func (r *ClusterReconciler) reconcileAddonsPhase(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
    // ... existing code to get networkID ...

    // NEW: Validate network ID is set
    if networkID == 0 {
        r.Recorder.Event(cluster, corev1.EventTypeWarning, EventReasonAddonsFailed,
            "Network ID is 0 - cannot install CCM/CSI without valid network ID")
        return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
    }

    // NEW: Validate HCloud token is set
    if cfg.HCloudToken == "" {
        r.Recorder.Event(cluster, corev1.EventTypeWarning, EventReasonAddonsFailed,
            "HCloud token is empty - cannot install CCM/CSI")
        return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
    }

    logger.Info("installing addons",
        "networkID", networkID,
        "hasToken", cfg.HCloudToken != "", // Log token presence, not value
    )
    // ... rest of function ...
}
```

**File:** `internal/addons/secret.go`

2. Add logging to secret creation and ensure it reports failures:

```go
func createHCloudSecret(ctx context.Context, client k8sclient.Client, token string, networkID int64) error {
    // NEW: Validate inputs
    if token == "" {
        return fmt.Errorf("hcloud token is empty")
    }
    if networkID == 0 {
        return fmt.Errorf("network ID is 0")
    }

    log.Printf("[addons] Creating hcloud secret with network ID: %d", networkID)

    secret := &corev1.Secret{
        // ... existing code ...
    }

    if err := client.CreateSecret(ctx, secret); err != nil {
        return fmt.Errorf("failed to create hcloud secret: %w", err)
    }

    log.Printf("[addons] Successfully created hcloud secret in kube-system")
    return nil
}
```

### Impact

- CCM will have valid HCloud credentials
- CSI will be able to provision volumes
- Errors will be logged and visible

---

## Issue 3: Addon Installation Blocking Without Timeout

### Root Cause

When addon installation fails (e.g., ArgoCD chart fails), `reconcileAddonsPhase` returns:
```go
return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
```

This causes infinite retry with no escalation or timeout. The cluster stays stuck in "Addons" phase forever.

### Evidence

ArgoCD test stayed in `provisioning: Addons` for 30 minutes with no progress.

### Fix

**File:** `internal/operator/controller/cluster_controller.go`

Add addon installation timeout tracking:

```go
const (
    // Addon installation timeout
    addonInstallationTimeout = 10 * time.Minute
)

func (r *ClusterReconciler) reconcileAddonsPhase(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
    logger := log.FromContext(ctx)

    // NEW: Check if addon installation has been running too long
    if cluster.Status.AddonInstallationStarted != nil {
        elapsed := time.Since(cluster.Status.AddonInstallationStarted.Time)
        if elapsed > addonInstallationTimeout {
            logger.Error(nil, "addon installation timeout exceeded, proceeding to compute phase",
                "elapsed", elapsed, "timeout", addonInstallationTimeout)
            r.Recorder.Eventf(cluster, corev1.EventTypeWarning, "AddonTimeout",
                "Addon installation exceeded %s timeout, proceeding with degraded addons", addonInstallationTimeout)

            // Proceed to compute phase anyway - let the cluster continue
            cluster.Status.ProvisioningPhase = k8znerv1alpha1.PhaseCompute
            return ctrl.Result{Requeue: true}, nil
        }
    } else {
        // NEW: Track when addon installation started
        now := metav1.Now()
        cluster.Status.AddonInstallationStarted = &now
    }

    // ... rest of existing function ...
}
```

**File:** `api/v1alpha1/types.go`

Add field to track addon installation start time:

```go
type K8znerClusterStatus struct {
    // ... existing fields ...

    // AddonInstallationStarted tracks when addon installation began (for timeout)
    // +optional
    AddonInstallationStarted *metav1.Time `json:"addonInstallationStarted,omitempty"`
}
```

### Impact

- Addon installation failures won't block cluster provisioning forever
- Cluster will proceed to Compute phase after timeout
- Users can see warning events about addon failures

---

## Issue 4: HA Cluster Bootstrap - Additional CPs Turning Off

### Root Cause

Looking at the test logs, additional control planes (CP 2, CP 3) were created but immediately turned off. This suggests:

1. The servers booted from Talos snapshot
2. They didn't receive valid Talos configuration
3. They powered off (Talos behavior when stuck in maintenance without config)

The issue is that the **CLI creates only 1 CP during initial bootstrap** (see `create.go` line 140-162), and the operator is supposed to create the remaining CPs. But the operator's `reconcileComputePhase` may not be properly configuring additional CPs.

### Evidence

From test logs:
```
[compute] Creating 1 control plane + 0 worker servers in parallel...
```
And later, servers e2e-ha-*-control-plane-2 and control-plane-3 were found in "off" state.

### Fix

**File:** `internal/operator/controller/cluster_controller.go`

The issue is in the compute phase - when it creates additional CPs, they need to be configured with Talos. Currently, `ReconcileCompute` creates servers but doesn't apply Talos configs.

1. Add explicit Talos config application for new CPs in compute phase:

```go
func (r *ClusterReconciler) reconcileComputePhase(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
    // ... existing code to create servers ...

    // After creating servers, apply Talos configs to any new nodes
    // NEW: For CLI-bootstrapped clusters, configure any new nodes
    if cluster.Spec.Bootstrap != nil && cluster.Spec.Bootstrap.Completed {
        // Get all servers that need configuration
        newNodes := r.findNodesNeedingConfiguration(ctx, cluster)
        if len(newNodes) > 0 {
            logger.Info("configuring new nodes created by operator", "count", len(newNodes))

            // Load credentials for Talos config generation
            creds, err := r.phaseAdapter.LoadCredentials(ctx, cluster)
            if err != nil {
                return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
            }

            // Configure each new node
            for _, node := range newNodes {
                if err := r.configureNode(ctx, cluster, creds, node); err != nil {
                    logger.Error(err, "failed to configure node", "node", node.Name)
                    // Continue with other nodes
                }
            }
        }
    }

    // ... rest of function ...
}
```

2. Add helper function to configure a new node:

```go
func (r *ClusterReconciler) configureNode(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, creds *operatorprov.Credentials, node NodeInfo) error {
    logger := log.FromContext(ctx)

    // Create Talos config generator
    generator, err := r.phaseAdapter.CreateTalosGenerator(cluster, creds)
    if err != nil {
        return fmt.Errorf("failed to create Talos generator: %w", err)
    }

    // Generate config based on role
    var config []byte
    if node.Role == "control-plane" {
        sans := []string{cluster.Status.Infrastructure.LoadBalancerIP}
        config, err = generator.GenerateControlPlaneConfig(sans, node.Name)
    } else {
        config, err = generator.GenerateWorkerConfig(node.Name)
    }
    if err != nil {
        return fmt.Errorf("failed to generate config: %w", err)
    }

    // Apply config via insecure Talos API (node is in maintenance mode)
    if err := r.applyTalosConfigInsecure(ctx, node.IP, config); err != nil {
        return fmt.Errorf("failed to apply config: %w", err)
    }

    // Wait for node to be ready
    if err := r.nodeReadyWaiter(ctx, node.Name, nodeReadyTimeout); err != nil {
        return fmt.Errorf("node not ready after config: %w", err)
    }

    logger.Info("node configured successfully", "node", node.Name)
    return nil
}
```

### Impact

- Additional CPs will receive Talos configuration
- Nodes won't power off due to being stuck in maintenance mode
- HA clusters will properly scale to 3 or 5 CPs

---

## Implementation Order

1. **Issue 1 (Health Check)** - Highest priority, affects all tests
   - Small change, low risk
   - Unblocks accurate cluster phase reporting

2. **Issue 2 (CCM/CSI)** - High priority, affects functional tests
   - Add validation and logging
   - Ensures cloud resources work

3. **Issue 3 (Addon Timeout)** - Medium priority, affects ArgoCD test
   - Add timeout mechanism
   - Requires new status field

4. **Issue 4 (HA Bootstrap)** - Medium priority, affects HA test
   - Most complex change
   - Requires new helper functions

---

## Testing Strategy

After implementing fixes:

1. Run `TestE2EBackup` - Should pass (already passes, verify no regression)
2. Run `TestE2EDevCluster` - Should see CCM/CSI working
3. Run `TestE2EDevClusterWithArgoCD` - Should complete or timeout gracefully
4. Run `TestE2EHACluster` - Should see 3 CPs running

---

## Files to Modify

| File | Changes |
|------|---------|
| `internal/operator/controller/cluster_controller.go` | Add health check to reconcile(), add addon timeout, add node configuration |
| `internal/addons/secret.go` | Add validation and logging |
| `api/v1alpha1/types.go` | Add AddonInstallationStarted field |
| `api/v1alpha1/zz_generated.deepcopy.go` | Regenerate after types.go change |
