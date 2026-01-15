# Cluster Upgrade Guide

This guide explains how to upgrade your Talos Kubernetes cluster to new versions of Talos OS and Kubernetes.

## Overview

The `hcloud-k8s upgrade` command orchestrates a safe, rolling upgrade of your cluster:

1. **Validates** configuration and compatibility
2. **Upgrades control plane** nodes sequentially (maintains quorum)
3. **Upgrades worker** nodes (can be done in parallel)
4. **Upgrades Kubernetes** control plane components
5. **Performs health checks** between each step

## Prerequisites

- Existing cluster provisioned with `hcloud-k8s apply`
- `secrets.yaml` file in current directory (generated during initial provisioning)
- Updated cluster configuration with target versions
- `HCLOUD_TOKEN` environment variable set

## Usage

### Basic Upgrade

```bash
# Update your config.yaml with new versions
vim config.yaml

# Run the upgrade
hcloud-k8s upgrade --config config.yaml
```

### Dry Run (Recommended First)

Test what will be upgraded without making changes:

```bash
hcloud-k8s upgrade --config config.yaml --dry-run
```

This will show:
- Current cluster state
- Target versions
- Nodes that will be upgraded
- Estimated upgrade sequence

### Command Options

```bash
hcloud-k8s upgrade [flags]

Flags:
  -c, --config string       Path to configuration file (required)
      --dry-run             Show what would be upgraded without executing
      --skip-health-check   Skip health checks between upgrades (dangerous)
      --k8s-version string  Override Kubernetes version from config
```

## Configuration

### Talos OS Version

Update the Talos version in your `config.yaml`:

```yaml
talos:
  version: v1.8.3
  schematic_id: ce4c980550dd2ab1b17bbf2b08801c7eb59418eafe8f279833297925d67c7515
```

**Important:**
- Always upgrade incrementally (e.g., v1.8.1 → v1.8.2 → v1.8.3)
- Do not skip minor versions
- Check [Talos release notes](https://github.com/siderolabs/talos/releases) for compatibility

### Kubernetes Version

Update the Kubernetes version in your `config.yaml`:

```yaml
kubernetes:
  version: v1.32.1
```

**Important:**
- Kubernetes version must be compatible with Talos version
- Only upgrade one minor version at a time (e.g., v1.31.0 → v1.32.0)
- Check [Kubernetes version skew policy](https://kubernetes.io/releases/version-skew-policy/)

### Schematic ID

If you use custom Talos extensions, update the schematic ID:

```yaml
talos:
  version: v1.8.3
  schematic_id: <your-schematic-id>
  extensions:
    - siderolabs/qemu-guest-agent
```

Generate a new schematic at [Talos Image Factory](https://factory.talos.dev/).

## Upgrade Process

### 1. Preparation

```bash
# Backup your cluster (optional but recommended)
# Export resources
kubectl get all --all-namespaces -o yaml > backup.yaml

# Backup etcd (if Talos Backup addon enabled, this is automatic)
```

### 2. Update Configuration

```yaml
# config.yaml
talos:
  version: v1.8.3  # Target Talos version
  schematic_id: ce4c980550dd2ab1b17bbf2b08801c7eb59418eafe8f279833297925d67c7515

kubernetes:
  version: v1.32.1  # Target Kubernetes version
```

### 3. Dry Run

```bash
hcloud-k8s upgrade --config config.yaml --dry-run
```

Review the output:
```
[Upgrade] === DRY RUN REPORT ===
[Upgrade] Target Talos version: v1.8.3
[Upgrade] Target Kubernetes version: v1.32.1
[Upgrade] Nodes to check: 4
[Upgrade]
[Upgrade] This is a dry run. No changes will be made.
[Upgrade] Run without --dry-run to perform the actual upgrade.
[Upgrade] === END REPORT ===
```

### 4. Execute Upgrade

```bash
hcloud-k8s upgrade --config config.yaml
```

Expected output:
```
[Upgrade] Starting cluster upgrade for: my-cluster
[Upgrade] Validating configuration...
[Upgrade] Found 4 nodes in cluster
[Upgrade] Upgrading control plane nodes...
[Upgrade] Found 3 control plane nodes
[Upgrade] Upgrading control plane node 1/3 (10.0.0.10)...
[Upgrade] Node 10.0.0.10: v1.8.2 → v1.8.3
[Upgrade] Waiting for node 10.0.0.10 to reboot...
[Upgrade] Node 10.0.0.10 upgraded successfully
[Upgrade] Checking cluster health after node 10.0.0.10...
[Upgrade] Cluster health check passed
...
[Upgrade] Control plane upgrade completed
[Upgrade] Upgrading worker nodes...
[Upgrade] Worker upgrade completed
[Upgrade] Upgrading Kubernetes to version v1.32.1...
[Upgrade] Kubernetes upgrade completed
[Upgrade] Performing final health check...
[Upgrade] Cluster health check passed
[Upgrade] Cluster upgrade completed successfully
```

### 5. Verification

```bash
# Check node versions
kubectl get nodes

# Check system pods
kubectl get pods -n kube-system

# Check cluster health
kubectl cluster-info

# Verify Talos version on nodes
talosctl version --nodes <node-ip>
```

## Upgrade Behavior

### Control Plane Upgrades

Control plane nodes are upgraded **sequentially** to maintain etcd quorum:

1. Upgrade first control plane node
2. Wait for node to reboot and become ready
3. Perform health check
4. Proceed to next control plane node
5. Repeat until all control plane nodes upgraded

**Timeout:** 10 minutes per node

### Worker Upgrades

Worker nodes are upgraded **sequentially** (parallel support planned):

1. Check current version
2. Skip if already at target version
3. Upgrade node
4. Wait for node to become ready
5. Proceed to next worker

**Timeout:** 10 minutes per node

### Kubernetes Upgrade

After all nodes are upgraded, Kubernetes control plane components are upgraded:

1. Connect to first control plane node
2. Trigger Kubernetes upgrade via Talos API
3. Talos orchestrates upgrade of:
   - kube-apiserver
   - kube-controller-manager
   - kube-scheduler
   - kube-proxy (if not replaced)

## Version Compatibility

### Talos ↔ Kubernetes

| Talos Version | Supported Kubernetes Versions |
|---------------|-------------------------------|
| v1.8.x        | v1.30.x, v1.31.x, v1.32.x    |
| v1.9.x        | v1.31.x, v1.32.x, v1.33.x    |

Check [Talos support matrix](https://www.talos.dev/latest/introduction/support-matrix/) for latest compatibility.

### Upgrade Paths

**Safe upgrade paths:**
- Talos: v1.8.1 → v1.8.2 → v1.8.3 (patch upgrades)
- Talos: v1.8.x → v1.9.x (minor upgrade)
- Kubernetes: v1.31.x → v1.32.x (one minor version)

**Unsafe upgrade paths:**
- Skipping Talos minor versions (v1.8.x → v1.10.x)
- Skipping Kubernetes minor versions (v1.30.x → v1.32.x)
- Downgrading versions

## Troubleshooting

### Node Fails to Come Back After Upgrade

**Symptoms:** Node stuck in "NotReady" state after upgrade

**Solutions:**
1. Check node logs via Hetzner Console
2. Verify network connectivity
3. Check Talos logs: `talosctl logs --nodes <ip>`
4. If necessary, manually reboot via Hetzner Cloud console

### Health Check Fails

**Symptoms:** Upgrade stops with "health check failed" error

**Solutions:**
1. Check cluster state: `kubectl get nodes`
2. Check pod status: `kubectl get pods --all-namespaces`
3. Review Talos health: `talosctl health --nodes <ip>`
4. Use `--skip-health-check` flag (use with caution)

### Upgrade Times Out

**Symptoms:** Upgrade exceeds 10 minute timeout per node

**Solutions:**
1. Check network speed to Hetzner Cloud
2. Check node internet connectivity (image download)
3. Verify sufficient CPU/memory on nodes
4. Check for stuck processes on node

### Version Mismatch After Upgrade

**Symptoms:** Nodes report different versions than expected

**Solutions:**
1. Verify schematic ID in config matches
2. Check image URL in Talos logs
3. Manually trigger upgrade: `talosctl upgrade --nodes <ip> --image <url>`

## Safety Features

### Health Checks

Health checks are performed:
- After each control plane node upgrade (critical for quorum)
- After all worker upgrades (optional)
- Before completing upgrade

**Retry logic:** 3 attempts with 10 second delays

**Skip health checks:** Use `--skip-health-check` flag (not recommended)

### Version Checking

Before upgrading each node:
- Current version is queried
- If already at target version, upgrade is skipped
- Prevents unnecessary reboots

### Dry Run Mode

Always test with `--dry-run` first:
- Shows what will be upgraded
- Validates configuration
- No changes made to cluster

## Advanced Usage

### Upgrade Kubernetes Version Only

If Talos is already at the target version:

```bash
# Only K8s will be upgraded (Talos nodes skipped)
hcloud-k8s upgrade --config config.yaml
```

### Override Kubernetes Version

Upgrade K8s version without changing config:

```bash
hcloud-k8s upgrade --config config.yaml --k8s-version v1.32.1
```

### Skip Health Checks (Dangerous)

Use only if you understand the risks:

```bash
hcloud-k8s upgrade --config config.yaml --skip-health-check
```

**Warning:** This can lead to:
- Loss of etcd quorum
- Cluster unavailability
- Data loss

## Rollback

If upgrade fails and cluster is non-functional:

1. **Restore from backup:**
   ```bash
   kubectl apply -f backup.yaml
   ```

2. **Revert configuration:**
   ```bash
   # Update config.yaml to previous versions
   hcloud-k8s upgrade --config config.yaml
   ```

3. **Manual node recovery:**
   ```bash
   # For each failed node
   talosctl reset --nodes <ip> --graceful=false
   # Then re-provision via hcloud-k8s apply
   ```

## Best Practices

1. **Always test in non-production first**
2. **Use dry run mode** before actual upgrade
3. **Backup cluster** before upgrading
4. **Upgrade incrementally** (one version at a time)
5. **Monitor during upgrade** (kubectl get nodes, logs)
6. **Upgrade during maintenance window** (expect brief API unavailability)
7. **Keep secrets.yaml** secure and backed up
8. **Test applications** after upgrade
9. **Document upgrade** (versions, date, issues)
10. **Have rollback plan** ready

## References

- [Talos Upgrade Guide](https://www.talos.dev/latest/talos-guides/upgrading-talos/)
- [Kubernetes Upgrade Guide](https://kubernetes.io/docs/tasks/administer-cluster/cluster-upgrade/)
- [Talos Release Notes](https://github.com/siderolabs/talos/releases)
- [Kubernetes Release Notes](https://kubernetes.io/releases/)
