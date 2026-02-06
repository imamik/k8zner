# Investigation: E2E Addon Deployment Issues

## Executive Summary

Investigation of why multiple addons fail to deploy in E2E tests. The root cause is a **configuration mapping gap** between the v2 simplified config, the CRD spec, and the operator's SpecToConfig function.

## Issues Identified

### 1. Monitoring Stack Not Deployed (CRITICAL)

**Root Cause:** The CRD `AddonSpec` struct does NOT include a Monitoring/KubePrometheusStack field.

**Evidence:**
- `api/v1alpha1/types.go` lines 230-254: AddonSpec only has Traefik, CertManager, ExternalDNS, ArgoCD, MetricsServer
- `cmd/k8zner/handlers/create.go` line 486-494: `buildAddonSpec()` only maps these 5 addons
- `internal/operator/provisioning/adapter.go`: `SpecToConfig()` never enables KubePrometheusStack

**Flow Breakdown:**
```
v2 Config (monitoring: true)
    ↓ v2.Expand()
Internal Config (KubePrometheusStack.Enabled: true)
    ↓ buildAddonSpec() ← LOST HERE
CRD Spec (no Monitoring field!)
    ↓ SpecToConfig()
Operator Config (KubePrometheusStack.Enabled: false)
    ↓ ApplyWithoutCilium()
Result: Monitoring NOT installed
```

**Fix Required:**
1. Add `Monitoring bool` to `api/v1alpha1/types.go` AddonSpec
2. Update `buildAddonSpec()` in create.go to map KubePrometheusStack
3. Update `SpecToConfig()` in adapter.go to read and enable monitoring

---

### 2. Backup CronJob Not Found (CRITICAL)

**Root Causes (2 issues):**

#### A. Silent Skip Due to Missing Talos CRD
- `internal/addons/apply.go` lines 330-343: TalosBackup is SILENTLY SKIPPED if Talos API CRD is not registered within 2 minutes
- The Talos CRD requires `kubernetesTalosAPIAccess` in machine config
- If CRD registration is slow, backup addon is never installed

#### B. S3 Credentials Not Passed Through CRD
- v2.Expand() reads S3 credentials from environment variables
- CRD BackupSpec only has: Enabled, Schedule, Retention (no S3 credentials)
- SpecToConfig() creates TalosBackup config without S3 access/secret keys
- Backup installation fails credential validation

**Evidence:**
```go
// internal/addons/apply.go:330-334
if cfg.Addons.TalosBackup.Enabled {
    log.Printf("[addons] Waiting for Talos API CRD to be available...")
    if err := waitForTalosCRD(ctx, client); err != nil {
        log.Printf("[addons] Warning: Talos API CRD not available after waiting: %v")
        log.Printf("[addons] Skipping Talos Backup installation - CRD may be registered later")
    }
}
```

**Fix Required:**
1. Add S3 credential fields to CRD BackupSpec OR pass via Secret reference
2. Make Talos CRD wait time configurable
3. Add addon status to reflect "skipped" vs "failed"

---

### 3. CSI Volume Stuck Pending (TIMING)

**Likely Cause:** Test runs before CSI controller pods are fully ready.

**Evidence:**
- CSI is installed by ApplyWithoutCilium()
- CSI controller pods need time to start and become healthy
- StorageClass creation happens during chart installation
- Test may check volume provisioning before CSI is ready

**Fix Required:**
- Add wait for CSI controller to be running before testing volume creation
- E2E test should wait for StorageClass to exist

---

### 4. MetricsServer Not Available (TIMING/SCHEDULING)

**Possible Causes:**
1. Metrics-server pod not scheduled (taint tolerance issue)
2. Not enough time for aggregated API to register
3. Kubelet TLS verification issues with Talos

**Evidence:**
- MetricsServer addon IS enabled in SpecToConfig
- Uses `--kubelet-insecure-tls` for Talos compatibility
- Has tolerations for control-plane taints

**Likely Fix:**
- Add explicit wait for metrics API availability in E2E test
- Current: `kubectl top nodes` immediately fails
- Should: Wait up to 2 minutes for metrics API

---

### 5. ArgoCD Ingress Not Created (DEPENDENCY CHAIN)

**Root Cause:** ArgoCD ingress depends on:
1. Traefik (ingress controller) being ready
2. Cert-manager (for TLS certificates) being ready
3. External-DNS (for DNS records) being ready
4. IngressClass "traefik" existing

**Evidence:**
- ArgoCD is installed after Traefik and External-DNS in apply.go
- But no explicit wait for Traefik to be ready
- Ingress creation may fail silently

**Fix Required:**
- Wait for IngressClass "traefik" before installing ArgoCD
- Verify Traefik pods are Running before proceeding

---

### 6. PrometheusOperatorCRDs Not Enabled (CRITICAL)

**Root Cause:** PrometheusOperatorCRDs not enabled in SpecToConfig

**Evidence:**
```go
// internal/operator/provisioning/adapter.go - NOT present:
// PrometheusOperatorCRDs: config.PrometheusOperatorCRDsConfig{Enabled: true}
```

**Impact:** Without CRDs, kube-prometheus-stack cannot create:
- ServiceMonitor resources
- PrometheusRule resources
- PodMonitor resources

**Fix Required:**
- Always enable PrometheusOperatorCRDs in SpecToConfig (dependency of monitoring)

---

## Root Cause Summary

| Issue | Category | Severity | Status |
|-------|----------|----------|--------|
| Monitoring not mapped in CRD | Config Gap | CRITICAL | NEEDS FIX |
| Backup S3 credentials lost | Config Gap | CRITICAL | NEEDS FIX |
| Backup skipped silently | Silent Failure | HIGH | NEEDS FIX |
| CSI timing | Test Timing | MEDIUM | TEST FIX |
| MetricsServer timing | Test Timing | MEDIUM | TEST FIX |
| ArgoCD ingress deps | Dependency Chain | MEDIUM | NEEDS FIX |
| PrometheusOperatorCRDs missing | Config Gap | CRITICAL | NEEDS FIX |

---

## Recommended Fixes (Priority Order)

### Priority 1: CRD Schema Updates
1. Add `Monitoring bool` to AddonSpec
2. Add S3 credential fields to BackupSpec (or use SecretRef pattern)
3. Regenerate CRD manifests

### Priority 2: Config Mapping
1. Update `buildAddonSpec()` to map monitoring
2. Update `SpecToConfig()` to:
   - Enable PrometheusOperatorCRDs when monitoring enabled
   - Enable KubePrometheusStack from spec
   - Map backup S3 credentials

### Priority 3: Silent Failure Handling
1. Add "skipped" state to addon status
2. Log warnings when addons are skipped due to missing dependencies
3. Increase Talos CRD wait timeout or make configurable

### Priority 4: E2E Test Improvements
1. Add explicit waits for CSI controller ready
2. Add waits for metrics API availability
3. Add waits for IngressClass before checking ingress

---

## Files Requiring Changes

### CRD Schema
- `api/v1alpha1/types.go` - Add Monitoring to AddonSpec

### CLI Handlers
- `cmd/k8zner/handlers/create.go` - Update buildAddonSpec()
- `cmd/k8zner/handlers/apply.go` - Update addon mapping

### Operator Adapter
- `internal/operator/provisioning/adapter.go` - Update SpecToConfig()

### E2E Tests
- `tests/e2e/full_stack_dev_test.go` - Add waits for addon readiness

---

## Test Validation Plan

After fixes:
1. Verify `kubectl get prometheus -A` returns resources
2. Verify `kubectl get grafana -A` returns pods running
3. Verify `kubectl get cronjob -n talos-backup` exists
4. Verify `kubectl top nodes` returns metrics
5. Verify ArgoCD ingress has external IP
