# Addon Testing Guide

This guide explains how to validate that all addons work correctly in the hcloud-kubernetes cluster.

## Overview

The project has migrated from Terraform to pure Go implementation. Currently, the following addons have been migrated:

| Addon | Status | Tested in E2E |
|-------|--------|---------------|
| **CCM** (Hetzner Cloud Controller Manager) | ✅ Migrated | ✅ Yes (cluster_test.go) |
| **CSI** (Hetzner Cloud Storage) | ✅ Migrated | ✅ Yes (cluster_test.go) |
| **Metrics Server** | ✅ Migrated | ✅ Yes (addons_test.go) |
| **Cert Manager** | ✅ Migrated | ✅ Yes (addons_test.go) |
| **Longhorn** | ✅ Migrated | ✅ Yes (addons_test.go) |
| **Ingress NGINX** | ✅ Migrated | ✅ Yes (addons_test.go) |
| **RBAC** | ✅ Migrated | ✅ Yes (addons_test.go) |
| **OIDC RBAC** | ✅ Migrated | ⚠️  Requires OIDC config |

## E2E Test Structure

### Existing Tests

1. **`tests/e2e/cluster_test.go`** - `TestClusterProvisioning`
   - Tests basic cluster provisioning with CCM and CSI
   - Validates CCM functionality (node provider IDs, load balancer provisioning)
   - Validates CSI functionality (volume creation, mounting, deletion)
   - Runtime: ~15-20 minutes

2. **`tests/e2e/addons_test.go`** - `TestAddonsProvisioning` (NEW)
   - Tests all addons together in a single cluster
   - Validates each addon individually
   - Runtime: ~25-30 minutes (due to Longhorn initialization)

### Test Coverage

#### CCM (Cloud Controller Manager)
- ✅ Deployment exists
- ✅ Pod is running
- ✅ Nodes have provider IDs (`hcloud://...`)
- ✅ Cloud provider labels are set
- ✅ Load balancer provisioning works
- ✅ Load balancer deletion works

#### CSI (Container Storage Interface)
- ✅ CSIDriver resource exists
- ✅ Controller deployment exists
- ✅ Node daemonset exists
- ✅ StorageClass is created and set as default
- ✅ PVC can be created and bound
- ✅ Volume can be attached to pod
- ✅ Volume deletion works

#### Metrics Server
- ✅ Deployment exists
- ✅ Pod is running
- ✅ Metrics API is functional (`kubectl top nodes`)

#### Cert Manager
- ✅ Namespace exists
- ✅ All CRDs are installed
- ✅ Core components are running (cert-manager, webhook, cainjector)

#### Longhorn
- ✅ Namespace exists
- ✅ Deployment exists
- ✅ All pods reach Running state

#### Ingress NGINX
- ✅ Namespace exists
- ✅ Controller exists (deployment or daemonset)
- ✅ Pod is running

#### RBAC
- ✅ ClusterRoles are created
- ✅ Resources are properly labeled

## Running E2E Tests

### Prerequisites

1. **Hetzner Cloud Token**
   ```bash
   export HCLOUD_TOKEN="your-hetzner-cloud-api-token"
   ```

2. **kubectl** (for cluster validation)
   ```bash
   # Install kubectl if not present
   curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
   chmod +x kubectl
   sudo mv kubectl /usr/local/bin/
   ```

### Running All Tests

Run the complete e2e test suite:

```bash
export HCLOUD_TOKEN="your-token"
export E2E_KEEP_SNAPSHOTS=true  # Keep snapshots for faster subsequent runs
make e2e
```

This will run all tests in parallel:
- `TestInfraProvisioning` - Infrastructure reconciliation
- `TestSnapshotCreation` - Fresh snapshot build test
- `TestImageBuildLifecycle` - Snapshot boot verification
- `TestSimpleTalosNode` - Basic Talos connectivity
- `TestApplyConfig` - Talos configuration
- `TestClusterProvisioning` - Full cluster with CCM & CSI
- `TestParallelProvisioning` - Multi-node cluster
- `TestAddonsProvisioning` - All addons validation (NEW)

**Runtime:** ~25-30 minutes (first run, builds snapshots)
**Runtime:** ~15-20 minutes (subsequent runs with cached snapshots)

### Running Individual Tests

#### Test CCM and CSI Only
```bash
export HCLOUD_TOKEN="your-token"
go test -v -timeout=1h -tags=e2e ./tests/e2e/... -run TestClusterProvisioning
```

#### Test All Addons
```bash
export HCLOUD_TOKEN="your-token"
go test -v -timeout=1h -tags=e2e ./tests/e2e/... -run TestAddonsProvisioning
```

#### Test Specific Addon
To test a specific addon, you can modify the test to run only that addon's subtest:
```bash
export HCLOUD_TOKEN="your-token"
go test -v -timeout=1h -tags=e2e ./tests/e2e/... -run TestAddonsProvisioning/MetricsServer
```

### Debugging Tests

#### View Logs During Test
```bash
go test -v -timeout=1h -tags=e2e ./tests/e2e/... -run TestAddonsProvisioning 2>&1 | tee test-output.log
```

#### Keep Resources for Inspection
Modify the test to skip cleanup:
1. Comment out `defer cleanup()` in the test
2. Run the test
3. Manually inspect resources using `kubectl`
4. Clean up manually when done:
   ```bash
   kubectl --kubeconfig /tmp/kubeconfig-e2e-addons-TIMESTAMP delete all --all -n <namespace>
   ```

#### Check Addon Logs
```bash
# Get kubeconfig from test output
export KUBECONFIG=/tmp/kubeconfig-e2e-addons-TIMESTAMP

# Check CCM logs
kubectl logs -n kube-system -l app=hcloud-cloud-controller-manager

# Check CSI logs
kubectl logs -n kube-system -l app.kubernetes.io/name=hcloud-csi

# Check Metrics Server logs
kubectl logs -n kube-system -l app.kubernetes.io/name=metrics-server

# Check Cert Manager logs
kubectl logs -n cert-manager -l app.kubernetes.io/component=cert-manager

# Check Longhorn logs
kubectl logs -n longhorn-system -l app=longhorn-manager

# Check Ingress NGINX logs
kubectl logs -n ingress-nginx -l app.kubernetes.io/component=controller
```

## Comparing with Terraform Implementation

If you need to compare the Go implementation with the Terraform implementation to debug issues or ensure parity, follow this guide.

### Step 1: Set Up Terraform Environment

```bash
cd terraform/

# Initialize Terraform
terraform init

# Copy example config
cp terraform.tfvars.example terraform.tfvars

# Edit terraform.tfvars with your settings
vim terraform.tfvars
```

### Step 2: Enable Terraform Debug Logging

```bash
# Enable detailed logging
export TF_LOG=DEBUG
export TF_LOG_PATH=/tmp/terraform-debug.log

# Apply with all addons enabled
terraform apply -auto-approve
```

### Step 3: Capture Addon Manifests from Terraform

```bash
# Get kubeconfig from Terraform output
terraform output -raw kubeconfig > /tmp/terraform-kubeconfig

export KUBECONFIG=/tmp/terraform-kubeconfig

# Capture all addon manifests
mkdir -p /tmp/terraform-addons

# CCM
kubectl get deployment -n kube-system hcloud-cloud-controller-manager -o yaml > /tmp/terraform-addons/ccm-deployment.yaml
kubectl get secret -n kube-system hcloud -o yaml > /tmp/terraform-addons/ccm-secret.yaml

# CSI
kubectl get deployment -n kube-system hcloud-csi-controller -o yaml > /tmp/terraform-addons/csi-controller.yaml
kubectl get daemonset -n kube-system hcloud-csi-node -o yaml > /tmp/terraform-addons/csi-node.yaml
kubectl get storageclass hcloud-volumes -o yaml > /tmp/terraform-addons/csi-storageclass.yaml

# Metrics Server
kubectl get deployment -n kube-system metrics-server -o yaml > /tmp/terraform-addons/metrics-server.yaml

# Cert Manager
kubectl get deployment -n cert-manager cert-manager -o yaml > /tmp/terraform-addons/cert-manager.yaml
kubectl get deployment -n cert-manager cert-manager-webhook -o yaml > /tmp/terraform-addons/cert-manager-webhook.yaml

# Longhorn
kubectl get deployment -n longhorn-system longhorn-driver-deployer -o yaml > /tmp/terraform-addons/longhorn-deployer.yaml

# Ingress NGINX
kubectl get deployment -n ingress-nginx ingress-nginx-controller -o yaml > /tmp/terraform-addons/ingress-nginx.yaml
```

### Step 4: Capture Helm Values from Terraform

```bash
# Extract Helm values from Terraform state
terraform show -json > /tmp/terraform-state.json

# View Helm release values
kubectl get secret -n kube-system -l owner=helm -o json | jq '.items[] | select(.metadata.name | contains("hcloud-ccm"))' > /tmp/terraform-helm-ccm.json
kubectl get secret -n kube-system -l owner=helm -o json | jq '.items[] | select(.metadata.name | contains("hcloud-csi"))' > /tmp/terraform-helm-csi.json
kubectl get secret -n kube-system -l owner=helm -o json | jq '.items[] | select(.metadata.name | contains("metrics-server"))' > /tmp/terraform-helm-metrics.json
```

### Step 5: Compare with Go Implementation

```bash
# Run Go implementation
export HCLOUD_TOKEN="your-token"
go test -v -timeout=1h -tags=e2e ./tests/e2e/... -run TestAddonsProvisioning

# Get kubeconfig from Go test
export KUBECONFIG=/tmp/kubeconfig-e2e-addons-TIMESTAMP

# Capture addon manifests
mkdir -p /tmp/go-addons

kubectl get deployment -n kube-system hcloud-cloud-controller-manager -o yaml > /tmp/go-addons/ccm-deployment.yaml
kubectl get deployment -n kube-system hcloud-csi-controller -o yaml > /tmp/go-addons/csi-controller.yaml
# ... repeat for all addons

# Compare manifests
diff /tmp/terraform-addons/ccm-deployment.yaml /tmp/go-addons/ccm-deployment.yaml
diff /tmp/terraform-addons/csi-controller.yaml /tmp/go-addons/csi-controller.yaml
```

### Step 6: Extract Detailed Logs

#### Terraform Logs
```bash
# Terraform already logs to TF_LOG_PATH
cat /tmp/terraform-debug.log | grep -i "addon\|helm\|manifest" > /tmp/terraform-addon-logs.log
```

#### Go Implementation Logs
```bash
# Run with verbose logging
go test -v -timeout=1h -tags=e2e ./tests/e2e/... -run TestAddonsProvisioning 2>&1 | tee /tmp/go-implementation.log

# Filter addon-related logs
cat /tmp/go-implementation.log | grep -i "addon\|installing\|applying" > /tmp/go-addon-logs.log
```

### Step 7: Functional Comparison

Create a checklist to verify functional parity:

```bash
# Create comparison script
cat > /tmp/compare-addons.sh << 'EOF'
#!/bin/bash

echo "=== CCM Comparison ==="
echo "Terraform:"
kubectl --kubeconfig=/tmp/terraform-kubeconfig get nodes -o jsonpath='{.items[*].spec.providerID}'
echo ""
echo "Go:"
kubectl --kubeconfig=/tmp/kubeconfig-e2e-addons-TIMESTAMP get nodes -o jsonpath='{.items[*].spec.providerID}'
echo ""

echo "=== CSI Comparison ==="
echo "Terraform:"
kubectl --kubeconfig=/tmp/terraform-kubeconfig get storageclass
echo ""
echo "Go:"
kubectl --kubeconfig=/tmp/kubeconfig-e2e-addons-TIMESTAMP get storageclass
echo ""

echo "=== Metrics Server Comparison ==="
echo "Terraform:"
kubectl --kubeconfig=/tmp/terraform-kubeconfig top nodes
echo ""
echo "Go:"
kubectl --kubeconfig=/tmp/kubeconfig-e2e-addons-TIMESTAMP top nodes
echo ""
EOF

chmod +x /tmp/compare-addons.sh
/tmp/compare-addons.sh
```

## Expected Results

### Successful Test Output

When all tests pass, you should see output like:

```
=== RUN   TestAddonsProvisioning
=== RUN   TestAddonsProvisioning/CCM
✓ CCM deployment exists
✓ CCM pod is Running
✓ CCM addon is working
=== RUN   TestAddonsProvisioning/CSI
✓ CSIDriver resource exists
✓ CSI controller deployment exists
✓ CSI controller pod is Running
✓ CSI addon is working
=== RUN   TestAddonsProvisioning/MetricsServer
✓ Metrics Server deployment exists
✓ Metrics Server pod is Running
✓ Metrics API is working
=== RUN   TestAddonsProvisioning/CertManager
✓ cert-manager namespace exists
✓ CRD certificates.cert-manager.io exists
✓ CRD certificaterequests.cert-manager.io exists
✓ CRD issuers.cert-manager.io exists
✓ CRD clusterissuers.cert-manager.io exists
✓ cert-manager pod is Running
✓ cert-manager-webhook pod is Running
✓ cert-manager-cainjector pod is Running
✓ Cert Manager addon is working
=== RUN   TestAddonsProvisioning/Longhorn
✓ longhorn-system namespace exists
✓ Longhorn deployment exists
✓ All Longhorn pods are Running (12 pods)
✓ Longhorn addon is working
=== RUN   TestAddonsProvisioning/IngressNginx
✓ ingress-nginx namespace exists
✓ Ingress NGINX controller (deployment) exists
✓ Ingress NGINX pod is Running
✓ Ingress NGINX addon is working
=== RUN   TestAddonsProvisioning/RBAC
✓ RBAC addon resources found
✓ RBAC addon verification complete
--- PASS: TestAddonsProvisioning (1234.56s)
    --- PASS: TestAddonsProvisioning/CCM (45.23s)
    --- PASS: TestAddonsProvisioning/CSI (56.78s)
    --- PASS: TestAddonsProvisioning/MetricsServer (89.01s)
    --- PASS: TestAddonsProvisioning/CertManager (123.45s)
    --- PASS: TestAddonsProvisioning/Longhorn (567.89s)
    --- PASS: TestAddonsProvisioning/IngressNginx (234.56s)
    --- PASS: TestAddonsProvisioning/RBAC (12.34s)
PASS
```

### Common Issues and Solutions

#### Issue: Metrics Server pod is not Running
**Symptoms:** Metrics Server pod is in `CrashLoopBackOff`

**Solution:** Check if Metrics Server has proper certificates and can reach the API server:
```bash
kubectl logs -n kube-system -l app.kubernetes.io/name=metrics-server
```

#### Issue: Longhorn pods stuck in Pending
**Symptoms:** Longhorn pods don't reach Running state

**Solution:** Check if nodes have the required dependencies:
```bash
kubectl describe pod -n longhorn-system <pod-name>
# Longhorn requires open-iscsi on worker nodes
```

#### Issue: Cert Manager webhook not ready
**Symptoms:** Cert Manager webhook pod is not Running

**Solution:** Check webhook logs and verify network policies:
```bash
kubectl logs -n cert-manager -l app.kubernetes.io/component=cert-manager-webhook
```

## Performance Notes

### Test Duration

| Test | First Run | Cached Snapshots | Bottleneck |
|------|-----------|------------------|------------|
| TestClusterProvisioning | ~15-20 min | ~8-10 min | Cluster bootstrap |
| TestAddonsProvisioning | ~25-30 min | ~15-20 min | Longhorn initialization |

### Optimization Tips

1. **Use snapshot caching** for development:
   ```bash
   export E2E_KEEP_SNAPSHOTS=true
   ```

2. **Run specific addon tests** instead of full suite:
   ```bash
   go test -v -tags=e2e ./tests/e2e/... -run TestAddonsProvisioning/MetricsServer
   ```

3. **Parallelize where possible**: The test suite already runs tests in parallel where safe.

## Contributing

When adding new addons:

1. Implement the addon in `internal/addons/<addon>.go`
2. Add helm chart to `internal/addons/helm/templates/<addon>/`
3. Update `internal/addons/apply.go` to include the new addon
4. Add test function to `tests/e2e/addons_test.go`
5. Update this guide with the new addon

## References

- [E2E Testing Guide](tests/e2e/README.md)
- [Addon Refactor Summary](.claude/ADDON_REFACTOR_SUMMARY.md)
- [Migration Status Analysis](migration_status_analysis.md)
- [Technical Design Document](technical_design_doc.md)
