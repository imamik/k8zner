# K8zner Cluster Operator Design

## Vision

A Kubernetes operator that runs inside the cluster and continuously reconciles the cluster state to match the desired configuration. Designed with **chaos resilience** in mind - any node (including control planes) can fail at any time and the system self-heals.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Kubernetes Cluster                          │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                    Control Plane (HA: 3 nodes)              │   │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐                     │   │
│  │  │  cp-1   │  │  cp-2   │  │  cp-3   │   etcd quorum: 2/3  │   │
│  │  │  etcd   │  │  etcd   │  │  etcd   │                     │   │
│  │  └─────────┘  └─────────┘  └─────────┘                     │   │
│  │       │            │            │                           │   │
│  │       └────────────┼────────────┘                           │   │
│  │                    │                                        │   │
│  │  ┌─────────────────┴─────────────────┐                     │   │
│  │  │      k8zner-operator (HA)         │                     │   │
│  │  │  - 2 replicas (leader election)   │                     │   │
│  │  │  - tolerates CP taints            │                     │   │
│  │  │  - PodAntiAffinity across CPs     │                     │   │
│  │  └─────────────────┬─────────────────┘                     │   │
│  └────────────────────┼────────────────────────────────────────┘   │
│                       │                                             │
│  ┌────────────────────┼────────────────────────────────────────┐   │
│  │                    │           Workers                      │   │
│  │  ┌─────────┐  ┌────┴────┐  ┌─────────┐                     │   │
│  │  │worker-1 │  │worker-2 │  │worker-3 │                     │   │
│  │  │ ready   │  │unhealthy│  │ ready   │                     │   │
│  │  └─────────┘  └─────────┘  └─────────┘                     │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                     │
│  ┌─────────────────────┐    ┌─────────────────────────────────┐   │
│  │   K8znerCluster     │    │   Secrets                       │   │
│  │   (CRD)             │    │   - hcloud-token                │   │
│  │                     │    │   - talos-secrets               │   │
│  │   spec:             │    │   - s3-credentials              │   │
│  │     workers: 3      │    │   - cloudflare-token            │   │
│  │     backup: true    │    └─────────────────────────────────┘   │
│  │   status:           │                                          │
│  │     ready: 2/3      │                                          │
│  └─────────────────────┘                                          │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
                    ┌───────────────────────────────┐
                    │        Hetzner Cloud          │
                    │   - Server API                │
                    │   - Load Balancer API         │
                    │   - Network API               │
                    └───────────────────────────────┘
```

## Design Principles

### 1. Chaos Resilience
- **Any node can die at any time** - system must self-heal
- **No single point of failure** - HA control planes, HA operator
- **Graceful degradation** - partial failures don't cascade
- **Idempotent operations** - safe to retry any operation

### 2. Leader Election
- Only one operator instance reconciles at a time
- Uses Kubernetes lease-based leader election
- Automatic failover if leader dies (~15s)

### 3. Stateless Operator
- All state stored in CRD status and Kubernetes objects
- Operator can restart on any node and resume
- No local state or persistent volumes

### 4. Conservative Actions
- Wait for confirmation before destructive actions
- One node replacement at a time (maintain quorum)
- Exponential backoff on failures

## CRD Design

```yaml
apiVersion: k8zner.io/v1alpha1
kind: K8znerCluster
metadata:
  name: my-cluster
  namespace: k8zner-system
spec:
  # Cluster identity (immutable after creation)
  name: my-cluster
  region: fsn1

  # Control plane configuration
  controlPlanes:
    count: 3          # 1 for dev, 3 for HA
    size: cx23

  # Worker configuration
  workers:
    count: 3
    size: cx33
    minCount: 1       # Never scale below this
    maxCount: 10      # Never scale above this

  # Features
  backup:
    enabled: true
    schedule: "0 * * * *"
    retention: 168h   # 7 days

  # Health check configuration
  healthCheck:
    nodeNotReadyThreshold: 5m    # Replace after this duration
    etcdUnhealthyThreshold: 2m   # Replace CP after this

  # Addons to reconcile
  addons:
    traefik: true
    certManager: true
    externalDns: true
    argocd: true

status:
  # Overall cluster phase
  phase: Running  # Provisioning, Running, Degraded, Healing, Failed

  # Control plane status
  controlPlanes:
    desired: 3
    ready: 3
    nodes:
      - name: my-cluster-control-plane-1
        serverID: 12345
        privateIP: 10.0.0.2
        publicIP: 1.2.3.4
        healthy: true
        etcdMemberID: "abc123"
        lastHealthCheck: "2026-01-31T14:00:00Z"
      - name: my-cluster-control-plane-2
        serverID: 12346
        privateIP: 10.0.0.3
        publicIP: 1.2.3.5
        healthy: true
        etcdMemberID: "def456"
        lastHealthCheck: "2026-01-31T14:00:00Z"
      - name: my-cluster-control-plane-3
        serverID: 12347
        privateIP: 10.0.0.4
        publicIP: 1.2.3.6
        healthy: true
        etcdMemberID: "ghi789"
        lastHealthCheck: "2026-01-31T14:00:00Z"

  # Worker status
  workers:
    desired: 3
    ready: 2
    unhealthy: 1
    nodes:
      - name: my-cluster-workers-1
        serverID: 12348
        privateIP: 10.0.1.2
        healthy: true
        lastHealthCheck: "2026-01-31T14:00:00Z"
      - name: my-cluster-workers-2
        serverID: 12349
        privateIP: 10.0.1.3
        healthy: false
        unhealthyReason: "NodeNotReady for 5m32s"
        unhealthySince: "2026-01-31T13:54:28Z"
        lastHealthCheck: "2026-01-31T14:00:00Z"
      - name: my-cluster-workers-3
        serverID: 12350
        privateIP: 10.0.1.4
        healthy: true
        lastHealthCheck: "2026-01-31T14:00:00Z"

  # Addon status
  addons:
    traefik:
      installed: true
      version: "3.3.6"
      healthy: true
    certManager:
      installed: true
      version: "1.17.2"
      healthy: true

  # Conditions (standard Kubernetes pattern)
  conditions:
    - type: Ready
      status: "False"
      reason: WorkerUnhealthy
      message: "1 of 3 workers is unhealthy"
      lastTransitionTime: "2026-01-31T13:54:28Z"
    - type: ControlPlaneReady
      status: "True"
      reason: AllNodesHealthy
      message: "3 of 3 control plane nodes are healthy"
      lastTransitionTime: "2026-01-31T12:00:00Z"
    - type: EtcdHealthy
      status: "True"
      reason: QuorumMet
      message: "etcd cluster has 3/3 healthy members"
      lastTransitionTime: "2026-01-31T12:00:00Z"

  # Reconciliation tracking
  lastReconcileTime: "2026-01-31T14:00:00Z"
  observedGeneration: 5
```

## Reconciliation Logic

### Main Reconcile Loop

```
┌──────────────────────────────────────────────────────────────┐
│                     Reconcile Loop                           │
│                                                              │
│  1. Fetch K8znerCluster CR                                   │
│              │                                               │
│              ▼                                               │
│  2. Health Check Phase                                       │
│     ├── Check all K8s nodes (kubectl get nodes)              │
│     ├── Check etcd health (talosctl etcd status)             │
│     ├── Check addon pods                                     │
│     └── Update status.nodes[].healthy                        │
│              │                                               │
│              ▼                                               │
│  3. Control Plane Reconciliation (if HA)                     │
│     ├── If CP unhealthy > threshold AND quorum maintained:   │
│     │   ├── Remove from etcd cluster                         │
│     │   ├── Delete Hetzner server                            │
│     │   ├── Create new server                                │
│     │   └── Apply Talos config (auto-joins etcd)             │
│     └── Only ONE CP at a time (preserve quorum)              │
│              │                                               │
│              ▼                                               │
│  4. Worker Reconciliation                                    │
│     ├── If worker unhealthy > threshold:                     │
│     │   ├── Cordon node                                      │
│     │   ├── Drain node (evict pods)                          │
│     │   ├── Delete Hetzner server                            │
│     │   ├── Create new server                                │
│     │   └── Apply Talos config                               │
│     ├── If workers.count changed:                            │
│     │   ├── Scale up: Create new servers                     │
│     │   └── Scale down: Drain and delete                     │
│     └── Multiple workers can be replaced in parallel         │
│              │                                               │
│              ▼                                               │
│  5. Addon Reconciliation                                     │
│     ├── For each addon in spec:                              │
│     │   ├── If not installed: Install                        │
│     │   ├── If unhealthy: Reinstall                          │
│     │   └── If version changed: Upgrade                      │
│              │                                               │
│              ▼                                               │
│  6. Update Status                                            │
│     ├── Update conditions                                    │
│     ├── Update phase                                         │
│     └── Set lastReconcileTime                                │
│              │                                               │
│              ▼                                               │
│  7. Requeue                                                  │
│     └── Requeue after 30s for continuous monitoring          │
└──────────────────────────────────────────────────────────────┘
```

### Control Plane Replacement (Critical Path)

```
IMPORTANT: Only replace ONE control plane at a time!

Preconditions:
  - etcd has quorum (2/3 or 3/5 healthy)
  - No other CP replacement in progress
  - Unhealthy for > threshold duration

Steps:
  1. Verify quorum will be maintained after removal
     └── 3 nodes: need 2 healthy, can lose 1
     └── 5 nodes: need 3 healthy, can lose 2

  2. Remove unhealthy node from etcd cluster
     └── talosctl -n <healthy-cp> etcd remove-member <unhealthy-member-id>

  3. Remove node from Kubernetes
     └── kubectl delete node <node-name>

  4. Delete Hetzner server
     └── hcloud server delete <server-id>

  5. Create new server
     └── Same as initial provisioning

  6. Apply Talos config
     └── New node auto-joins etcd cluster
     └── New node auto-joins Kubernetes

  7. Verify health
     └── Wait for node Ready
     └── Wait for etcd member healthy
```

### Worker Replacement

```
Steps:
  1. Cordon node (prevent new pods)
     └── kubectl cordon <node-name>

  2. Drain node (evict pods gracefully)
     └── kubectl drain <node-name> --ignore-daemonsets --delete-emptydir-data

  3. Delete Kubernetes node
     └── kubectl delete node <node-name>

  4. Delete Hetzner server
     └── hcloud server delete <server-id>

  5. Create new server
     └── Same as initial provisioning

  6. Apply Talos config
     └── talosctl apply-config

  7. Wait for Ready
     └── Node auto-joins cluster
```

## Failure Scenarios

### Scenario 1: Single Worker Dies
- **Detection**: Node status NotReady for 5 minutes
- **Action**: Drain, delete, replace
- **Impact**: Minimal - pods rescheduled to other workers
- **Recovery time**: ~3-5 minutes

### Scenario 2: Single Control Plane Dies (HA)
- **Detection**: Node NotReady OR etcd member unhealthy
- **Precondition**: Other 2 CPs healthy (quorum maintained)
- **Action**: Remove from etcd, delete, replace
- **Impact**: None if quorum maintained
- **Recovery time**: ~5-7 minutes

### Scenario 3: Operator Pod Dies
- **Detection**: Kubernetes restarts pod
- **Action**: Leader election, new leader takes over
- **Impact**: ~15s gap in reconciliation
- **Recovery time**: ~15-30 seconds

### Scenario 4: Two Control Planes Die Simultaneously
- **Detection**: etcd loses quorum
- **Action**: ALERT - cannot auto-recover
- **Impact**: Cluster unavailable
- **Recovery**: Manual intervention required (restore from backup)

### Scenario 5: All Workers Die
- **Detection**: All worker nodes NotReady
- **Action**: Replace all workers (sequentially or parallel)
- **Impact**: Workloads down until workers replaced
- **Recovery time**: ~5-10 minutes

## Security Considerations

### Credentials Storage
```yaml
# All credentials in Kubernetes Secrets
apiVersion: v1
kind: Secret
metadata:
  name: k8zner-credentials
  namespace: k8zner-system
type: Opaque
data:
  hcloud-token: <base64>      # Hetzner API token
  talos-secrets: <base64>     # Talos machine secrets (for new nodes)
  s3-access-key: <base64>     # Backup credentials
  s3-secret-key: <base64>
  cloudflare-token: <base64>  # DNS management
```

### RBAC
```yaml
# Operator needs extensive permissions
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: k8zner-operator
rules:
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch", "delete"]
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch", "delete"]  # For drain
  - apiGroups: [""]
    resources: ["pods/eviction"]
    verbs: ["create"]
  - apiGroups: ["k8zner.io"]
    resources: ["k8znerclusters"]
    verbs: ["get", "list", "watch", "update", "patch"]
  - apiGroups: ["k8zner.io"]
    resources: ["k8znerclusters/status"]
    verbs: ["update", "patch"]
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "create", "update"]  # Leader election
```

## Implementation Phases

### Phase 1: Foundation
- [x] CRD definition and code generation
- [x] Controller scaffolding with kubebuilder
- [x] Leader election setup
- [x] Basic reconcile loop structure
- [x] Unit tests with mocks

### Phase 2: Health Monitoring
- [x] Node health checking (NotReady detection)
- [x] etcd health checking via Talos API
- [x] Status updates in CRD
- [ ] Metrics and events

### Phase 3: Worker Scaling
- [x] Hetzner server creation/deletion
- [x] Talos config application
- [x] Scale workers up/down via CRD spec change
- [x] E2E tests for worker scaling

### Phase 4: Control Plane Scaling
- [x] CP scale-up (1 → 3 for HA)
- [x] etcd member management
- [x] Quorum-safe operations (never delete after etcd join)
- [x] CP failure recovery (delete unhealthy, replace)
- [x] E2E tests for CP operations

### Phase 5: Addon Reconciliation
- [x] Addon installation (ordered: CNI first, then rest)
- [x] Addon health checking
- [x] Bootstrap tolerations (control-plane, uninitialized, not-ready)
- [ ] Addon upgrades (version change detection)

### Phase 6: E2E Testing
- [x] Full stack addon validation (14 subtests)
- [x] HA operations (10 subtests: scale up/down, CP failure/recovery)
- [x] 24/24 tests passing
- [ ] Chaos testing (random node kills)

## Testing Strategy

### Unit Tests
- Mock Hetzner client
- Mock Kubernetes client
- Mock Talos client
- Test each reconciliation step in isolation

### Integration Tests (envtest)
- Fake Kubernetes API server
- Mock external APIs
- Test full reconciliation flow

### E2E Tests
- Real Hetzner infrastructure
- Actually kill nodes
- Verify self-healing
- Chaos testing (random node kills)

## Metrics

```prometheus
# Reconciliation metrics
k8zner_reconcile_total{result="success|error"}
k8zner_reconcile_duration_seconds

# Node health metrics
k8zner_nodes_total{role="controlplane|worker"}
k8zner_nodes_healthy{role="controlplane|worker"}
k8zner_nodes_unhealthy{role="controlplane|worker"}

# Replacement metrics
k8zner_node_replacements_total{role="controlplane|worker",reason="unhealthy|scaled"}
k8zner_node_replacement_duration_seconds

# etcd metrics
k8zner_etcd_members_total
k8zner_etcd_members_healthy
```

## Open Questions

1. **Backup verification**: Should operator periodically verify backups are valid?
2. **Scaling triggers**: Should we support HPA-like auto-scaling based on metrics?
3. **Multi-cluster**: Future support for one operator managing multiple clusters?

## Resolved Questions

- **Upgrade orchestration**: Not in scope for v1. Users update the CRD spec with new versions; operator reconciles. Rolling upgrades are a future enhancement.
- **CLI vs Operator**: Resolved — operator-first architecture. CLI bootstraps infrastructure + operator, then all reconciliation is done by the operator. The `apply` command is the single entry point.
