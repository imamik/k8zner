# Troubleshooting

Common issues and solutions for k8zner clusters on Hetzner Cloud.

## CSI Driver CrashLoopBackOff

**Symptom**: `hcloud-csi-controller` or `hcloud-csi-node` pods crash with DNS resolution errors.

**Root cause**: During bootstrap, CoreDNS is not yet running because it requires the CNI (Cilium) to be ready. The CSI driver tries to resolve `api.hetzner.cloud` and fails.

**Solution**: This resolves automatically once Cilium is installed and CoreDNS pods start. The CSI pods will recover after a few restart cycles. No manual intervention needed.

If CSI pods remain in CrashLoopBackOff after 5+ minutes:

```bash
# Check if Cilium is running
kubectl get pods -n kube-system -l k8s-app=cilium

# Check CoreDNS
kubectl get pods -n kube-system -l k8s-app=kube-dns

# Restart CSI pods if DNS is working
kubectl rollout restart deployment hcloud-csi-controller -n kube-system
kubectl rollout restart daemonset hcloud-csi-node -n kube-system
```

## Cilium Not Starting / Device Detection Failure

**Symptom**: Cilium agent crashes with `unable to determine direct routing device` or `no devices matched`.

**Root cause**: Talos Linux uses predictable PCI interface names (`enp1s0`, `enp7s0`), not `eth0`/`eth1`. Cilium's device auto-detection fails if configured for the wrong interface names.

**What k8zner configures**: k8zner sets `devices: "enp+"` to match all PCI ethernet interfaces on Hetzner Cloud. If you're customizing Cilium settings, ensure the device pattern matches your interfaces.

**Verify interface names**:

```bash
# Via Talos API
talosctl get links --nodes <node-ip>

# Expected output on Hetzner Cloud:
#   enp1s0 (public interface)
#   enp7s0 (private network interface)
```

**Key Cilium settings for Hetzner Cloud**:
- `devices: "enp+"` - matches PCI ethernet interfaces
- `nodePort.directRoutingDevice: "enp1s0"` - required with kube-proxy replacement
- `loadBalancer.acceleration: "disabled"` - XDP incompatible with virtio
- `routingMode: "tunnel"` - VXLAN mode for reliable pod networking

## Load Balancer Health Checks Failing

**Symptom**: Hetzner Load Balancer shows targets as unhealthy.

**Root cause**: The LB sends health checks via the private network to NodePort services. Cilium's kube-proxy replacement must be enabled to handle NodePort traffic via eBPF.

**Check LB status**:

```bash
# Via hcloud CLI
hcloud load-balancer describe <cluster-name>-api
hcloud load-balancer describe <cluster-name>-ingress
```

**Common causes**:
1. **Cilium not ready**: Wait for Cilium pods to be running. LB health checks will pass once eBPF programs are loaded.
2. **Wrong `externalTrafficPolicy`**: k8zner uses `Cluster` (default). Changing to `Local` can break health checks if the target pod isn't on every node.

## Pods Stuck in Pending During Bootstrap

**Symptom**: Addon pods (CCM, CSI, metrics-server) stuck in `Pending` state during initial cluster setup.

**Root cause**: Missing tolerations. During bootstrap, control plane nodes have three taints:
1. `node-role.kubernetes.io/control-plane` - standard control plane taint
2. `node.cloudprovider.kubernetes.io/uninitialized` - applied before CCM initializes nodes
3. `node.kubernetes.io/not-ready` - applied before node passes readiness checks

**Solution**: k8zner automatically adds all three tolerations to bootstrap-critical addons (CCM, CSI, metrics-server). If you're deploying custom workloads during bootstrap, add these tolerations:

```yaml
tolerations:
  - key: node-role.kubernetes.io/control-plane
    effect: NoSchedule
  - key: node.cloudprovider.kubernetes.io/uninitialized
    value: "true"
    effect: NoSchedule
  - key: node.kubernetes.io/not-ready
    effect: NoSchedule
```

## ArgoCD Redis Pods Failing

**Symptom**: ArgoCD Redis pods fail with `secret argocd-redis not found`.

**Root cause**: The `redisSecretInit` job was disabled. This init job creates the `argocd-redis` secret that Redis pods reference for authentication.

**Solution**: k8zner sets `redisSecretInit.enabled: true` by default. If you see this error after manual changes:

```bash
# Check if the init job ran
kubectl get jobs -n argocd -l app.kubernetes.io/component=redis-secret-init

# If the job doesn't exist, restart ArgoCD
kubectl rollout restart statefulset argocd-redis -n argocd
```

## Control Plane Scale-Up Stuck

**Symptom**: New control plane node shows `WaitingForK8s` phase and doesn't become ready.

**Root cause**: After Talos config is applied, the node joins etcd and starts kubelet. This can take several minutes, especially if the node needs to download container images.

**Important**: Do NOT delete a control plane server that has had Talos config applied. The etcd member has already been added. Deleting the server would break etcd quorum. k8zner will automatically retry on the next reconciliation cycle.

**Check node status**:

```bash
# Check Talos node health
talosctl health --nodes <node-ip>

# Check etcd member list
talosctl etcd members --nodes <existing-cp-ip>

# Check kubelet status
talosctl service kubelet --nodes <node-ip>
```

## TLS Certificate Not Issued

**Symptom**: Ingress shows TLS errors, certificate not found.

**Root cause**: cert-manager uses Cloudflare DNS-01 challenge to issue certificates. Several things can prevent this:

**Check certificate status**:

```bash
# Check Certificate resources
kubectl get certificates -A

# Check CertificateRequest and Challenge
kubectl get certificaterequests -A
kubectl get challenges -A

# Check cert-manager logs
kubectl logs -n cert-manager deployment/cert-manager
```

**Common causes**:
1. **Invalid Cloudflare API token**: Ensure `CF_API_TOKEN` has DNS edit permissions for the domain.
2. **DNS propagation delay**: DNS-01 challenges require DNS propagation. Wait a few minutes and check again.
3. **Wrong domain**: Verify the Ingress host matches a domain managed by Cloudflare.
4. **ClusterIssuer not ready**: Check `kubectl get clusterissuers` for status.

## Backup CronJob Not Running

**Symptom**: Talos backup CronJob exists but no backups appear in S3.

**Root cause**: The backup requires the Talos API CRD (`talos.dev/v1alpha1`) to be registered, which only happens when `kubernetesTalosAPIAccess` is enabled in the Talos machine config.

**Check status**:

```bash
# Check if the CRD exists
kubectl get crd | grep talos

# Check CronJob status
kubectl get cronjobs -n kube-system

# Check recent job runs
kubectl get jobs -n kube-system -l app=talos-backup

# Check S3 credentials
kubectl get secret -n kube-system talos-backup-s3
```

If the Talos CRD is not present, backup installation is skipped during addon setup. Re-run `k8zner apply` after ensuring the machine config includes `kubernetesTalosAPIAccess`.

## Node Stuck in Maintenance Mode

**Symptom**: A new node boots but doesn't join the cluster.

**Root cause**: Talos nodes boot from a snapshot into maintenance mode. They accept insecure connections until a machine config is applied. If config application fails, the node stays in maintenance mode.

**Detect maintenance mode**:

```bash
# Try insecure connection (should respond in maintenance mode)
talosctl version --insecure --nodes <node-ip>

# Try authenticated connection (should fail in maintenance mode)
talosctl version --nodes <node-ip>
```

**Solution**: Re-run `k8zner apply` to trigger the operator to detect and configure the node, or manually apply the config:

```bash
talosctl apply-config --insecure --nodes <node-ip> --file machineconfig.yaml
```

## Getting Help

If none of the above resolves your issue:

1. Check cluster events: `kubectl get events -A --sort-by=.lastTimestamp`
2. Check operator logs: `kubectl logs -n k8zner-system deployment/k8zner-operator`
3. Check Talos node health: `talosctl health --nodes <node-ip>`
4. Open an issue with logs and cluster status at the project repository.
