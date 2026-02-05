# Plan: Fix E2E Addon Deployment Issues

## Overview

This plan addresses all addon deployment issues discovered during E2E testing investigation. Issues are grouped by priority and include specific file changes, implementation details, and validation steps.

---

## Phase 1: Monitoring Stack (DONE)

**Status: Implemented in commit cfad2a7**

- Added `Monitoring` field to CRD AddonSpec
- Updated `buildAddonSpec()` and `SpecToConfig()` to map monitoring
- Enabled PrometheusOperatorCRDs, GatewayAPICRDs, TalosCCM as dependencies

---

## Phase 2: Backup Credential Flow (CRITICAL)

### Problem
TalosBackup addon requires S3 credentials, but the CRD-to-operator flow loses them:
1. v2.Expand() reads S3 credentials from environment variables
2. CRD BackupSpec only stores: Enabled, Schedule, Retention
3. SpecToConfig() creates TalosBackup config without credentials
4. Addon installation fails validation

### Solution: Use SecretRef Pattern

Instead of storing credentials in the CRD spec, reference a Secret that contains them.

### Files to Modify

#### 1. `api/v1alpha1/types.go`
```go
// BackupSpec configures automated etcd backups.
type BackupSpec struct {
    Enabled bool `json:"enabled"`

    // Schedule is the cron schedule for backups
    // +kubebuilder:default="0 * * * *"
    // +optional
    Schedule string `json:"schedule,omitempty"`

    // Retention is how long to keep backups
    // +kubebuilder:default="168h"
    // +optional
    Retention string `json:"retention,omitempty"`

    // S3SecretRef references a Secret containing S3 credentials
    // Required keys: access-key, secret-key, endpoint, bucket, region
    // +optional
    S3SecretRef *SecretReference `json:"s3SecretRef,omitempty"`
}

// SecretReference references a Secret in the same namespace
type SecretReference struct {
    Name string `json:"name"`
}
```

#### 2. `cmd/k8zner/handlers/create.go`
```go
// In buildBackupSpec() or wherever backup is configured:
func buildBackupSpec(cfg *config.Config, clusterName string) *k8znerv1alpha1.BackupSpec {
    if !cfg.Addons.TalosBackup.Enabled {
        return nil
    }

    return &k8znerv1alpha1.BackupSpec{
        Enabled:   true,
        Schedule:  cfg.Addons.TalosBackup.Schedule,
        Retention: "168h", // Default
        S3SecretRef: &k8znerv1alpha1.SecretReference{
            Name: clusterName + "-backup-s3",
        },
    }
}

// Also create the Secret with S3 credentials
func createBackupSecret(ctx context.Context, client k8sclient.Client, cfg *config.Config, clusterName string) error {
    secret := &corev1.Secret{
        ObjectMeta: metav1.ObjectMeta{
            Name:      clusterName + "-backup-s3",
            Namespace: k8znerNamespace,
        },
        StringData: map[string]string{
            "access-key": cfg.Addons.TalosBackup.S3AccessKey,
            "secret-key": cfg.Addons.TalosBackup.S3SecretKey,
            "endpoint":   cfg.Addons.TalosBackup.S3Endpoint,
            "bucket":     cfg.Addons.TalosBackup.S3Bucket,
            "region":     cfg.Addons.TalosBackup.S3Region,
        },
    }
    return client.Create(ctx, secret)
}
```

#### 3. `internal/operator/provisioning/adapter.go`
```go
// In SpecToConfig(), load S3 credentials from referenced Secret
func (a *PhaseAdapter) loadBackupCredentials(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (*BackupCredentials, error) {
    if cluster.Spec.Backup == nil || cluster.Spec.Backup.S3SecretRef == nil {
        return nil, nil
    }

    secret := &corev1.Secret{}
    key := client.ObjectKey{
        Namespace: cluster.Namespace,
        Name:      cluster.Spec.Backup.S3SecretRef.Name,
    }

    if err := a.client.Get(ctx, key, secret); err != nil {
        return nil, fmt.Errorf("failed to get backup S3 secret: %w", err)
    }

    return &BackupCredentials{
        AccessKey: string(secret.Data["access-key"]),
        SecretKey: string(secret.Data["secret-key"]),
        Endpoint:  string(secret.Data["endpoint"]),
        Bucket:    string(secret.Data["bucket"]),
        Region:    string(secret.Data["region"]),
    }, nil
}

// Update SpecToConfig to use loaded credentials
if spec.Backup != nil && spec.Backup.Enabled {
    backupCreds, err := a.loadBackupCredentials(ctx, k8sCluster)
    if err != nil {
        return nil, err
    }
    if backupCreds != nil {
        cfg.Addons.TalosBackup = config.TalosBackupConfig{
            Enabled:     true,
            Schedule:    spec.Backup.Schedule,
            S3AccessKey: backupCreds.AccessKey,
            S3SecretKey: backupCreds.SecretKey,
            S3Endpoint:  backupCreds.Endpoint,
            S3Bucket:    backupCreds.Bucket,
            S3Region:    backupCreds.Region,
        }
    }
}
```

### Validation
- Create cluster with backup enabled
- Verify Secret `{cluster}-backup-s3` exists in k8zner-system namespace
- Verify TalosBackup CronJob is created
- Verify backup runs successfully

---

## Phase 3: Talos CRD Wait Timeout (HIGH)

### Problem
Backup addon silently skips installation if Talos API CRD isn't registered within 2 minutes.

### Solution: Configurable Timeout + Better Status Reporting

### Files to Modify

#### 1. `internal/addons/apply.go`
```go
const (
    defaultTalosCRDWaitTime = 2 * time.Minute
    maxTalosCRDWaitTime     = 10 * time.Minute
)

// Add parameter to control wait time
func waitForTalosCRD(ctx context.Context, client k8sclient.Client, timeout time.Duration) error {
    if timeout == 0 {
        timeout = defaultTalosCRDWaitTime
    }
    if timeout > maxTalosCRDWaitTime {
        timeout = maxTalosCRDWaitTime
    }

    // ... existing wait logic with configurable timeout
}

// In ApplyWithoutCilium, return explicit status instead of silent skip
type AddonInstallResult struct {
    Name      string
    Installed bool
    Skipped   bool
    Reason    string
    Error     error
}

func ApplyWithoutCilium(...) ([]AddonInstallResult, error) {
    var results []AddonInstallResult

    // ... for TalosBackup:
    if cfg.Addons.TalosBackup.Enabled {
        if err := waitForTalosCRD(ctx, client, 5*time.Minute); err != nil {
            results = append(results, AddonInstallResult{
                Name:    "talos-backup",
                Skipped: true,
                Reason:  fmt.Sprintf("Talos API CRD not available: %v", err),
            })
            // Log warning but don't fail
            log.Printf("[addons] WARNING: Skipping Talos Backup - %v", err)
        } else {
            if err := applyTalosBackup(ctx, client, cfg); err != nil {
                results = append(results, AddonInstallResult{
                    Name:  "talos-backup",
                    Error: err,
                })
                return results, err
            }
            results = append(results, AddonInstallResult{
                Name:      "talos-backup",
                Installed: true,
            })
        }
    }

    return results, nil
}
```

#### 2. `internal/operator/controller/cluster_controller.go`
```go
// Update addon status to reflect skipped addons
func (r *ClusterReconciler) reconcileAddonsPhase(...) {
    // ... after ApplyWithoutCilium
    results, err := addons.ApplyWithoutCilium(...)

    // Update status for each addon
    for _, result := range results {
        status := k8znerv1alpha1.AddonStatus{
            LastTransitionTime: &now,
        }

        if result.Installed {
            status.Installed = true
            status.Healthy = true
            status.Phase = k8znerv1alpha1.AddonPhaseInstalled
        } else if result.Skipped {
            status.Installed = false
            status.Phase = k8znerv1alpha1.AddonPhasePending
            status.Message = result.Reason
        } else if result.Error != nil {
            status.Phase = k8znerv1alpha1.AddonPhaseFailed
            status.Message = result.Error.Error()
        }

        cluster.Status.Addons[result.Name] = status
    }
}
```

### Validation
- Check addon status shows "Pending" with reason when CRD unavailable
- Kubernetes events show warning about skipped addon
- CRD status reflects actual installation state

---

## Phase 4: ArgoCD Ingress Dependencies (MEDIUM)

### Problem
ArgoCD ingress may fail if Traefik IngressClass isn't ready.

### Solution: Add Dependency Checks Before Installation

### Files to Modify

#### 1. `internal/addons/argocd.go`
```go
func applyArgoCD(ctx context.Context, client k8sclient.Client, cfg *config.Config) error {
    // Wait for IngressClass if ingress is enabled
    if cfg.Addons.ArgoCD.IngressEnabled {
        ingressClass := cfg.Addons.ArgoCD.IngressClassName
        if ingressClass == "" {
            ingressClass = "traefik"
        }

        if err := waitForIngressClass(ctx, client, ingressClass, 2*time.Minute); err != nil {
            log.Printf("[ArgoCD] Warning: IngressClass %s not ready, ingress may not work: %v", ingressClass, err)
            // Continue anyway - ingress can be created later
        }
    }

    // ... existing installation logic
}

func waitForIngressClass(ctx context.Context, client k8sclient.Client, name string, timeout time.Duration) error {
    deadline := time.Now().Add(timeout)

    for time.Now().Before(deadline) {
        exists, err := client.HasIngressClass(ctx, name)
        if err == nil && exists {
            return nil
        }

        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(5 * time.Second):
        }
    }

    return fmt.Errorf("timeout waiting for IngressClass %s", name)
}
```

#### 2. `internal/addons/k8sclient/client.go`
```go
// Add method to check IngressClass existence
func (c *Client) HasIngressClass(ctx context.Context, name string) (bool, error) {
    // Use discovery client or direct API call
    gvr := schema.GroupVersionResource{
        Group:    "networking.k8s.io",
        Version:  "v1",
        Resource: "ingressclasses",
    }

    _, err := c.dynamic.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
    if err != nil {
        if apierrors.IsNotFound(err) {
            return false, nil
        }
        return false, err
    }
    return true, nil
}
```

### Validation
- ArgoCD ingress created successfully when Traefik is ready
- Warning logged if IngressClass not found but installation continues
- Ingress works after Traefik becomes ready

---

## Phase 5: E2E Test Timing Improvements (MEDIUM)

### Problem
Tests check addon functionality before pods are ready.

### Solution: Add Explicit Waits in E2E Tests

### Files to Modify

#### 1. `tests/e2e/functional_tests.go`
```go
// testCSIVolume - add wait for CSI controller
func testCSIVolume(t *testing.T, state *E2EState) {
    // Wait for CSI controller to be ready
    t.Log("Waiting for CSI controller pods...")
    waitCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
    defer cancel()

    if err := waitForCSIReady(waitCtx, state.KubeconfigPath); err != nil {
        t.Fatalf("CSI controller not ready: %v", err)
    }

    // Now test volume creation
    // ... existing test
}

func waitForCSIReady(ctx context.Context, kubeconfigPath string) error {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            // Check if hcloud-csi-controller deployment is ready
            cmd := exec.CommandContext(ctx, "kubectl",
                "--kubeconfig", kubeconfigPath,
                "get", "deployment", "hcloud-csi-controller",
                "-n", "kube-system",
                "-o", "jsonpath={.status.readyReplicas}")
            output, err := cmd.CombinedOutput()
            if err == nil && strings.TrimSpace(string(output)) != "0" {
                return nil
            }
        }
    }
}
```

#### 2. `tests/e2e/addon_tests.go` (new file)
```go
// waitForMetricsAPI waits for metrics server API to be available
func waitForMetricsAPI(ctx context.Context, kubeconfigPath string, timeout time.Duration) error {
    deadline := time.Now().Add(timeout)

    for time.Now().Before(deadline) {
        cmd := exec.CommandContext(ctx, "kubectl",
            "--kubeconfig", kubeconfigPath,
            "top", "nodes", "--no-headers")
        if err := cmd.Run(); err == nil {
            return nil
        }

        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(10 * time.Second):
        }
    }

    return fmt.Errorf("timeout waiting for metrics API")
}

// waitForMonitoringReady waits for Prometheus and Grafana pods
func waitForMonitoringReady(ctx context.Context, kubeconfigPath string, timeout time.Duration) error {
    deadline := time.Now().Add(timeout)

    for time.Now().Before(deadline) {
        // Check Prometheus
        promReady := checkDeploymentReady(ctx, kubeconfigPath, "monitoring", "prometheus-kube-prometheus-stack-prometheus")
        // Check Grafana
        grafanaReady := checkDeploymentReady(ctx, kubeconfigPath, "monitoring", "kube-prometheus-stack-grafana")

        if promReady && grafanaReady {
            return nil
        }

        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(15 * time.Second):
        }
    }

    return fmt.Errorf("timeout waiting for monitoring stack")
}
```

#### 3. `tests/e2e/full_stack_dev_test.go`
```go
// Update Monitoring subtest
t.Run("10_Verify_Monitoring", func(t *testing.T) {
    // First wait for monitoring to be ready
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer cancel()

    if err := waitForMonitoringReady(ctx, state.KubeconfigPath, 5*time.Minute); err != nil {
        t.Fatalf("Monitoring stack not ready: %v", err)
    }

    // Now verify components
    verifyGrafana(t, state.KubeconfigPath)
    verifyPrometheus(t, state.KubeconfigPath)
    verifyAlertmanager(t, state.KubeconfigPath)
})

// Update MetricsServer subtest
t.Run("07_Verify_MetricsServer", func(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
    defer cancel()

    if err := waitForMetricsAPI(ctx, state.KubeconfigPath, 3*time.Minute); err != nil {
        t.Fatalf("Metrics API not available: %v", err)
    }

    verifyMetricsServer(t, state.KubeconfigPath)
})
```

### Validation
- E2E tests pass consistently
- No flaky failures due to timing
- Tests properly wait for addons before validation

---

## Implementation Order

| Phase | Description | Priority | Effort | Dependencies |
|-------|-------------|----------|--------|--------------|
| 1 | Monitoring Stack | CRITICAL | DONE | - |
| 2 | Backup Credentials | CRITICAL | Medium | Phase 1 |
| 3 | Talos CRD Timeout | HIGH | Low | Phase 2 |
| 4 | ArgoCD Dependencies | MEDIUM | Low | Phase 1 |
| 5 | E2E Test Timing | MEDIUM | Low | All above |

---

## Testing Plan

### Unit Tests
1. Test `buildBackupSpec()` creates correct Secret reference
2. Test `loadBackupCredentials()` retrieves credentials from Secret
3. Test `waitForIngressClass()` timeout behavior
4. Test addon install result tracking

### Integration Tests
1. Create cluster with backup enabled → verify Secret created
2. Create cluster with monitoring enabled → verify Prometheus/Grafana deployed
3. Simulate slow Talos CRD registration → verify backup skipped with status

### E2E Tests
1. Full stack test with all addons
2. Verify all addon statuses in CRD
3. Verify functional tests pass with new waits

---

## Rollback Plan

If issues arise:
1. Revert CRD changes requires careful handling of existing clusters
2. New fields are optional, so old CRDs continue to work
3. Secret-based credentials are backwards compatible

---

## Success Criteria

- [ ] Monitoring stack deploys when `monitoring: true` in v2 config
- [ ] Backup CronJob created when backup enabled with S3 credentials
- [ ] Addon status in CRD accurately reflects installation state
- [ ] E2E tests pass consistently without timing failures
- [ ] No silent addon installation failures
