# Test Coverage Plan

Current state (updated): critical gaps called out in early versions of this document have been substantially reduced.

This document now serves as a **living hardening backlog** focused on the remaining high-risk areas and ongoing depth improvements.

---

## Phase 1: Operator Spec Converter (HIGH — config round-trip safety)

**Package:** `internal/operator/provisioning/`
**Status:** `spec_converter.go` now has dedicated unit coverage (`spec_converter_test.go`). Remaining work is depth/edge-case expansion, not baseline coverage.

This was previously the single most impactful gap. Baseline coverage is now in place; focus should shift to regression-proofing edge cases and fuzz/property checks.

### Coverage expansion targets (`spec_converter_test.go`)

| Test | What it verifies |
|------|-----------------|
| `TestSpecToConfig_BasicFields` | ClusterName, region, location, network CIDRs mapped correctly |
| `TestSpecToConfig_NetworkDefaults` | IPv4CIDR=10.0.0.0/16, NodeIPv4CIDR=10.0.128.0/17, CalculateSubnets called |
| `TestSpecToConfig_TalosConfig` | Version, SchematicID, Extensions preserved |
| `TestSpecToConfig_WorkerCountZero` | Workers.Count = 0 (operator creates workers via reconciliation, not compute provisioner) |
| `TestSpecToConfig_CiliumDefaults` | KubeProxyReplacement=true, RoutingMode=tunnel, HubbleEnabled=true |
| `TestSpecToConfig_TraefikDefaults` | Kind=Deployment, ExternalTrafficPolicy=Cluster, IngressClass=traefik |
| `TestBuildAddonsConfig_AlwaysEnabled` | GatewayAPICRDs, PrometheusOperatorCRDs, TalosCCM, Cilium, CCM, CSI always on |
| `TestBuildAddonsConfig_Conditional` | MetricsServer, CertManager, Traefik, ExternalDNS, ArgoCD honor spec flags |
| `TestConfigureBackup_Disabled` | No backup config when spec.Backup disabled |
| `TestConfigureBackup_Enabled` | S3 config populated, EncryptionDisabled=true |
| `TestConfigureCloudflare` | ExternalDNS token, TXTOwnerID, CertManager DNS-01 enabled when domain set |
| `TestExpandArgoCDFromSpec` | Disabled/enabled/domain/subdomain combinations |
| `TestExpandMonitoringFromSpec` | Disabled/enabled/custom subdomain |
| `TestExpandExternalDNSFromSpec` | Disabled vs enabled (policy=sync, sources=ingress) |
| `TestNormalizeServerSize` | cx22→cx23, cx32→cx33, current sizes pass through |
| `TestResolveEndpoint` | Precedence: LBPrivateIP > ControlPlaneEndpoint > LBIP > CP node IP |
| `TestBuildMachineConfigOptions` | All flags set, KubeProxyReplacement=true |

**Mocking:** Pure functions + `config.Config` structs — no external deps needed.

---

## Phase 2: HCloud Generic Operations (HIGH — foundation for all resource tests)

**Package:** `internal/platform/hcloud/`
**Status:** `operations.go` now has dedicated tests (`operations_test.go`) for generic delete/ensure behavior. Focus here is now resilience scenarios and richer failure-mode assertions.

Every resource operation delegates to these generics; with baseline tests in place, this area is now about strengthening confidence with additional concurrency/retry edge cases.

### Coverage expansion targets (`operations_test.go`)

| Test | What it verifies |
|------|-----------------|
| `TestDeleteOperation_ResourceExists` | Get returns resource, Delete called, nil error |
| `TestDeleteOperation_ResourceNotFound` | Get returns nil, Delete NOT called, nil error (idempotent) |
| `TestDeleteOperation_GetError` | Get fails → wrapped error returned |
| `TestDeleteOperation_DeleteError` | Delete fails → wrapped error returned |
| `TestEnsureOperation_CreateNew` | Get returns nil → Create called, resource returned |
| `TestEnsureOperation_ReturnExisting` | Get returns resource → no Create, resource returned |
| `TestEnsureOperation_ExistingWithValidation` | Get returns resource, Validate passes → resource returned |
| `TestEnsureOperation_ExistingValidationFails` | Get returns resource, Validate fails → error |
| `TestEnsureOperation_ExistingWithUpdate` | Get returns resource, Update called with UpdateOptsMapper |
| `TestEnsureOperation_CreateError` | Create fails → wrapped error |
| `TestWaitForActions_Nil` | Nil actions → no-op |
| `TestWaitForActions_Single` | Single action waited on |
| `TestWaitForActions_Multiple` | Multiple actions all waited on |
| `TestSimpleCreate_Wrapping` | Wraps create function into CreateResult correctly |

**Mocking:** Mock `Get`, `Create`, `Delete`, `Update` function fields. Use MockClient already in `mock_client_test.go`.

---

## Phase 3: HCloud Resource Operations (HIGH — server creation is critical path)

**Package:** `internal/platform/hcloud/`
**Gap:** `server.go` (315 lines), `network.go` (85 lines), `snapshot.go` (82 lines), `rdns.go` (46 lines), `certificate.go` (42 lines), `placement_group.go` (48 lines), `ssh_key.go` (32 lines).

### Tests to write per file:

**`server_create_test.go`** (server.go covers the hottest path)

| Test | What it verifies |
|------|-----------------|
| `TestBuildServerCreateOpts_AllFields` | Image, server type, location, SSH keys resolved; labels, user data, placement group set |
| `TestBuildServerCreateOpts_PublicNetConfig` | IPv4/IPv6 enable flags map to PublicNet struct |
| `TestBuildServerCreateOpts_NetworkAttachment` | Private IP and network ID attached when provided |
| `TestCreateServer_Success` | Full flow: build opts → create → attach to network → return ID |
| `TestCreateServer_AttachNetwork` | Network attachment called when networkID > 0 |
| `TestGetServerIP_IPv4` | Prefers IPv4 when available |
| `TestGetServerIP_IPv6Fallback` | Constructs IPv6 address with ::1 suffix when no IPv4 |
| `TestGetServerIP_NoIP` | Error when neither IPv4 nor IPv6 available |
| `TestGetServerID_Exists` | Returns server ID as string |
| `TestGetServerID_NotFound` | Returns empty string, no error |
| `TestGetServersByLabel` | Label selector built correctly, results returned |
| `TestDeleteServer_Idempotent` | Succeeds when server doesn't exist |
| `TestAttachServerToNetwork` | Network attachment, private IP, power-on after attach |

**`network_test.go`**

| Test | What it verifies |
|------|-----------------|
| `TestEnsureNetwork_Create` | Creates with parsed CIDR, location, labels |
| `TestEnsureNetwork_ExistingMatchingCIDR` | Returns existing, no error |
| `TestEnsureNetwork_ExistingDifferentCIDR` | Returns validation error |
| `TestEnsureSubnet_Create` | Creates subnet with type, zone, parsed CIDR |
| `TestEnsureSubnet_AlreadyExists` | Idempotent, no error |
| `TestDeleteNetwork_Idempotent` | Succeeds when not found |

**`snapshot_test.go`**

| Test | What it verifies |
|------|-----------------|
| `TestCreateSnapshot` | Server ID parsed, snapshot type set, labels applied, image ID returned |
| `TestDeleteImage_Success` | Image ID parsed, retry on locked |
| `TestDeleteImage_InvalidID` | Parse error returned |
| `TestGetSnapshotByLabels_Found` | Label selector built, snapshot type filtered, most recent returned |
| `TestGetSnapshotByLabels_NotFound` | Returns nil, no error |

**`rdns_test.go`**, **`certificate_test.go`**, **`placement_group_test.go`**, **`ssh_key_test.go`** — similar pattern, 2-5 tests each.

**Mocking:** Extend existing `MockClient` — it already has function fields for all methods.

---

## Phase 4: Operator Controller — Addon Reconciliation (HIGH — cluster lifecycle)

**Package:** `internal/operator/controller/`
**Gap:** `reconcile_addons.go` (378 lines, 0 tests) — CNI installation and addon phase orchestration.

### Tests to write: `reconcile_addons_test.go`

| Test | What it verifies |
|------|-----------------|
| `TestReconcileCNIPhase_Success` | Credentials loaded, SpecToConfig called, Cilium installed, status updated, phase transitions to Compute |
| `TestReconcileCNIPhase_CredentialsFailure` | Error event recorded, requeue |
| `TestReconcileCNIPhase_CLIBootstrap` | Phase transitions to Addons (skips Compute for CLI-bootstrapped clusters) |
| `TestAllPodsReady_Empty` | Returns true (vacuous truth) |
| `TestAllPodsReady_MixedStates` | Returns false if any pod not Running+Ready |
| `TestAllPodsReady_AllHealthy` | Returns true when all pods Running+Ready=True |
| `TestReconcileAddonsPhase_WorkersNotReady` | Requeue after 15sec, workers created if hcloudClient available |
| `TestReconcileAddonsPhase_AllWorkersReady` | Addons installed, phase transitions |
| `TestEnsureWorkersReady_DesiredZero` | Immediate return, no wait |
| `TestEnsureWorkersReady_ScaleUp` | scaleUpWorkers called with correct count |
| `TestResolveNetworkID_FromStatus` | Returns cached ID immediately |
| `TestResolveNetworkID_FromHCloud` | Queries HCloud, persists to status |
| `TestInstallNextAddon_SkipsInstalled` | Skips addons already in status.Addons |
| `TestInstallNextAddon_InstallsNext` | Installs first uninstalled addon, adds status entry, requeues |
| `TestInstallNextAddon_AllComplete` | Phase=Complete, ClusterPhase=Running |
| `TestFindTalosEndpoint_Precedence` | ControlPlaneEndpoint > Bootstrap.PublicIP > healthy CP IP |
| `TestGetKubeconfigFromTalos_NoEndpoint` | Error when no endpoint available |

**Mocking:** Use existing `MockHCloudClient`, `MockTalosClient`, `MockTalosConfigGenerator` from `mocks_test.go`.

---

## Phase 5: Operator Controller — Phase State Machine (HIGH — cluster lifecycle)

**Package:** `internal/operator/controller/`
**Gap:** `reconcile_phases.go` (310 lines, 0 tests) — all provisioning phase orchestration.

### Tests to write: `reconcile_phases_test.go`

| Test | What it verifies |
|------|-----------------|
| `TestReconcileInfrastructurePhase_CLIBootstrap` | Skips when all IDs present, sets endpoint from LB, transitions to Image |
| `TestReconcileInfrastructurePhase_Fresh` | Calls ReconcileInfrastructure, transitions to Image |
| `TestReconcileInfrastructurePhase_CredentialsError` | Error event, requeue |
| `TestReconcileImagePhase_Success` | Calls ReconcileImage, transitions to Compute |
| `TestReconcileComputePhase_CLIBootstrap` | spec.Bootstrap.Completed=true → skip to Addons |
| `TestReconcileComputePhase_Fresh` | Transitions to Bootstrap |
| `TestReconcileBootstrapPhase_Success` | Calls ReconcileBootstrap, event recorded |
| `TestReconcileRunningPhase` | Health check → CP reconcile → worker reconcile → requeue 30sec |
| `TestBuildProvisioningContext` | HCloud client created, Talos generator set, network state populated |
| `TestDiscoverInfrastructure_LBFound` | Status updated with LB ID, public IP, private IP |
| `TestDiscoverInfrastructure_LBNotFound` | Continues gracefully, no error |
| `TestDiscoverInfrastructure_FirewallFound` | Status updated with firewall ID |

**Mocking:** Same mocks as Phase 4 + mock `phaseAdapter` interface.

---

## Phase 6: Operator Controller — Scaling (HIGH — etcd safety)

**Package:** `internal/operator/controller/`
**Gap:** `reconcile_scaling_cp.go` (358 lines), `reconcile_scaling_workers.go` (483 lines).

### Tests to write: `reconcile_scaling_cp_test.go`

| Test | What it verifies |
|------|-----------------|
| `TestReconcileControlPlanes_AlreadyProvisioning` | Requeue 30sec when CP in WaitingForTalosAPI phase |
| `TestReconcileControlPlanes_ScaleUp` | Triggers scale-up when current < desired |
| `TestReconcileControlPlanes_SingleCPSkipsReplacement` | No replacement attempted with 1 CP |
| `TestFindUnhealthyNode_None` | Returns nil when all healthy |
| `TestFindUnhealthyNode_ThresholdNotReached` | Returns nil when unhealthy duration < threshold |
| `TestFindUnhealthyNode_PastThreshold` | Returns node when duration > threshold |
| `TestReplaceUnhealthyCPIfNeeded_QuorumSafe` | Replacement happens when quorum maintained |
| `TestReplaceUnhealthyCPIfNeeded_QuorumUnsafe` | Condition set, event recorded, no replacement |
| `TestScaleUpControlPlanes_PartialSuccess` | Creates 1 of 2 CPs, returns partial error |
| `TestConfigureAndWaitForCP_EtcdSafety` | After config applied, server NOT deleted on timeout (phase=WaitingForK8s) |

### Tests to write: `reconcile_scaling_workers_test.go`

| Test | What it verifies |
|------|-----------------|
| `TestReconcileWorkers_NoAction` | Returns nil when count correct, all healthy |
| `TestFindUnhealthyWorkers` | Returns all workers past threshold |
| `TestReplaceUnhealthyWorkers_Limit` | Respects maxConcurrentHeals |
| `TestScaleWorkers_Up` | Phase set to Healing, scaleUpWorkers called |
| `TestScaleWorkers_Down` | Phase set to Healing, scaleDownWorkers called |
| `TestScaleDownWorkers_UnhealthyFirst` | Unhealthy workers removed before healthy ones |
| `TestSelectWorkersForRemoval_UnhealthyFirst` | Selection prioritizes unhealthy, then newest |
| `TestSelectWorkersForRemoval_AllHealthy` | Newest workers selected |
| `TestDecommissionWorker` | Cordon → drain → delete K8s node → delete HCloud server |
| `TestDecommissionWorker_NodeNotFound` | Continues to delete HCloud server |
| `TestRemoveWorkersFromStatus` | Filters removed workers from status slice |

**Mocking:** `MockHCloudClient`, `MockTalosClient`, fake K8s client (controller-runtime).

---

## Phase 7: Operator Controller — Cluster State & Talos Client (MEDIUM)

**Package:** `internal/operator/controller/`
**Gap:** `cluster_state.go` (209 lines), `talos_client.go` (242 lines).

### Tests to write: `cluster_state_test.go`

| Test | What it verifies |
|------|-----------------|
| `TestFindHealthyControlPlaneIP_NoNodes` | Returns "" |
| `TestFindHealthyControlPlaneIP_AllUnhealthy` | Returns "" |
| `TestFindHealthyControlPlaneIP_HasHealthy` | Returns first healthy CP's private IP |
| `TestBuildClusterSANs_AllSources` | Includes endpoint, LB IP, annotation, CP node IPs |
| `TestBuildClusterSANs_MinimalSources` | Only CP node IPs when no LB or endpoint |
| `TestResolveSSHKeyIDs_Annotation` | Splits comma-separated annotation |
| `TestResolveSSHKeyIDs_Default` | Returns `{clusterName}-key` |
| `TestResolveControlPlaneIP_Precedence` | ControlPlaneEndpoint > LB IP > annotation > healthy CP |
| `TestGenerateReplacementServerName` | CP uses `naming.ControlPlane()`, worker uses `naming.Worker()` |
| `TestWaitForServerIP_Immediate` | Returns when IP already assigned |
| `TestWaitForServerIP_Timeout` | Error after timeout |
| `TestWaitForK8sNodeReady_BecomesReady` | Returns nil when node Ready=True |
| `TestWaitForK8sNodeReady_Timeout` | Error when node never becomes Ready |

### Tests to write: `talos_client_test.go`

| Test | What it verifies |
|------|-----------------|
| `TestNewRealTalosClient_InvalidYAML` | Returns error |
| `TestNewRealTalosClient_ValidConfig` | Returns client |
| `TestIsNodeInMaintenanceMode_True` | Insecure connection succeeds → true |
| `TestIsNodeInMaintenanceMode_False` | Insecure connection fails → false |
| `TestGetEtcdMembers` | Maps member fields correctly (check IsLeader vs IsLearner bug) |
| `TestRemoveEtcdMember_InvalidID` | Parse error |
| `TestWaitForTalosAPI_MaintenanceMode` | Error containing "maintenance mode" treated as ready |

**Note:** `talos_client.go` tests require either a Talos mock server or integration test tags. Recommend testing the logic functions (parsing, error classification) as unit tests and the client interactions as `//go:build integration`.

---

## Phase 8: Provisioning — Infrastructure (MEDIUM)

**Package:** `internal/provisioning/infrastructure/`
**Gap:** `load_balancer.go` (232 lines), `firewall.go` (135 lines), `network.go` (91 lines), `rdns.go` (71 lines).

### Tests to write: `load_balancer_test.go`

| Test | What it verifies |
|------|-----------------|
| `TestProvisionLoadBalancers_APIOnly` | API LB created with lb11, services: 6443 + 50000, network attached |
| `TestProvisionLoadBalancers_WithIngress` | Both API and Ingress LBs created |
| `TestProvisionLoadBalancers_IngressDisabled` | Only API LB created |
| `TestNewIngressService_Defaults` | TCP, proxyprotocol, health check interval=15s, timeout=10s, retries=3 |
| `TestNewIngressService_CustomConfig` | Config overrides defaults |

### Tests to write: `firewall_test.go`

| Test | What it verifies |
|------|-----------------|
| `TestProvisionFirewall_BasicRules` | KubeAPI (6443) + TalosAPI (50000) rules created |
| `TestCollectAPISources_Precedence` | Specific > fallback > currentIP |
| `TestCollectAPISources_CurrentIP` | Appended as /32 when UseCurrentIPv4=true |
| `TestParseCIDRs_Valid` | Parses IPv4 and IPv6 CIDRs |
| `TestParseCIDRs_Invalid` | Skips invalid entries silently |
| `TestBuildFirewallRule` | Direction, protocol, ports, CIDRs mapped |
| `TestParseProtocol` | tcp/udp/icmp/gre/esp + unknown defaults to TCP |

### Tests to write: `network_test.go`

| Test | What it verifies |
|------|-----------------|
| `TestProvisionNetwork_AllSubnets` | Network + CP + LB + worker + autoscaler subnets |
| `TestProvisionNetwork_PublicIPDetection` | GetPublicIP called when UseCurrentIPv4=true |
| `TestProvisionNetwork_NoAutoscaler` | Autoscaler subnet skipped when disabled |

**Mocking:** Mock `provisioning.Context` with mock `Infra` (hcloud.Client interface).

---

## Phase 9: Provisioning — Compute (MEDIUM)

**Package:** `internal/provisioning/compute/`
**Gap:** `pool.go` (185 lines), `control_plane.go` (92 lines), `workers.go` (80 lines), `server.go` (195 lines), `rdns.go` (71 lines).

### Tests to write: `pool_test.go`

| Test | What it verifies |
|------|-----------------|
| `TestReconcileNodePool_PrivateIPCalculation` | CP: 10*poolIndex + j + 2, Worker: j + 2 |
| `TestReconcileNodePool_PlacementGroupSharding` | Worker pools sharded at 10 nodes |
| `TestReconcileNodePool_ParallelCreation` | All servers created concurrently via async.RunParallel |
| `TestReconcileNodePool_ErrorPropagation` | Failed server → error returned with server name |

### Tests to write: `control_plane_test.go`

| Test | What it verifies |
|------|-----------------|
| `TestProvisionControlPlane_EndpointSetup` | Endpoint set from LB IP |
| `TestProvisionControlPlane_SANCollection` | SANs from LB private networks |
| `TestProvisionControlPlane_AllPoolsProcessed` | Each CP pool processed sequentially |

### Tests to write: `workers_test.go`

| Test | What it verifies |
|------|-----------------|
| `TestProvisionWorkers_EmptyConfig` | Early return, no error |
| `TestProvisionWorkers_ParallelPools` | Pools run via async.RunParallel |
| `TestProvisionWorkers_ThreadSafeIPMerge` | Results from multiple pools merged safely |

### Tests to write: `server_test.go`

| Test | What it verifies |
|------|-----------------|
| `TestEnsureServer_Exists` | Returns existing IP and ID |
| `TestEnsureServer_Creates` | Server created with all options, IP returned |
| `TestEnsureServer_ImageDetection` | Architecture-based snapshot lookup |
| `TestEnsureImage_VersionDefaults` | Default Talos/K8s versions when not specified |

**Mocking:** Mock `provisioning.Context.Infra` interface.

---

## Phase 10: CLI Handlers (MEDIUM)

**Package:** `cmd/k8zner/handlers/`
**Gap:** `cluster_crd.go` (368 lines), `bootstrap.go` (224 lines).

### Tests to write: `bootstrap_test.go` (expand existing)

| Test | What it verifies |
|------|-----------------|
| `TestProvisionFirstControlPlane_RestoresCounts` | CP count set to 1, worker to 0, restored on error |
| `TestInstallOperatorOnly_RetryLogic` | Retries up to 10 times on transient errors |
| `TestIsTransientError` | EOF, connection refused/reset, timeout, no such host, TLS timeout |
| `TestIsTransientError_FatalErrors` | Non-transient errors return false |
| `TestCleanupOnFailure` | Destroy provisioner called |

### Tests to write: `cluster_crd_test.go`

| Test | What it verifies |
|------|-----------------|
| `TestBuildK8znerCluster` | Labels, spec fields, status fields from config/infra |
| `TestBuildClusterSpec` | All config fields mapped to CRD spec |
| `TestEnsureNamespace_AlreadyExists` | Idempotent, no error |

**Mocking:** Fake K8s client, mock file reads.

---

## Phase 11: Provisioning — Cluster Bootstrap Helpers (LOW)

**Package:** `internal/provisioning/cluster/`
**Gap:** `bootstrap_helpers.go` (330 lines), `network.go` (42 lines).

### Tests to write: `bootstrap_helpers_test.go` (expand existing)

| Test | What it verifies |
|------|-----------------|
| `TestGenerateDummyCert` | Returns valid PEM cert+key, RSA 2048, 10yr validity |
| `TestWaitForPort_Success` | Returns nil when port opens |
| `TestWaitForPort_Timeout` | Returns error when port never opens |
| `TestDetectMaintenanceModeNodes` | Separates maintenance from configured nodes |
| `TestConfigureNewNodes_NoneFound` | Returns nil early |

**Note:** Functions like `isNodeInMaintenanceMode`, `applyMachineConfig`, `waitForNodeReady` require Talos API mocking. Tag as `//go:build integration` or add a Talos client interface for testability.

---

## Phase 12: Fuzz Tests (LOW — polish)

**Package:** `internal/config/`

### Tests to write: `fuzz_test.go`

| Test | What it verifies |
|------|-----------------|
| `FuzzExpandConfig` | `ExpandSpec()` doesn't panic on arbitrary YAML |
| `FuzzParseConfig` | Config loader handles malformed input gracefully |

**Package:** `internal/operator/provisioning/`

| Test | What it verifies |
|------|-----------------|
| `FuzzSpecToConfig` | `SpecToConfig()` doesn't panic on arbitrary CRD specs |

---

## Summary

| Phase | Package | Files | Est. Tests | Priority |
|-------|---------|-------|------------|----------|
| 1 | operator/provisioning | spec_converter.go | 17 | HIGH |
| 2 | platform/hcloud | operations.go | 14 | HIGH |
| 3 | platform/hcloud | server.go + 6 resource files | 35 | HIGH |
| 4 | operator/controller | reconcile_addons.go | 17 | HIGH |
| 5 | operator/controller | reconcile_phases.go | 12 | HIGH |
| 6 | operator/controller | reconcile_scaling_*.go | 21 | HIGH |
| 7 | operator/controller | cluster_state.go + talos_client.go | 20 | MEDIUM |
| 8 | provisioning/infrastructure | load_balancer + firewall + network + rdns | 18 | MEDIUM |
| 9 | provisioning/compute | pool + cp + workers + server | 15 | MEDIUM |
| 10 | cmd/handlers | cluster_crd + bootstrap | 11 | MEDIUM |
| 11 | provisioning/cluster | bootstrap_helpers + network | 5 | LOW |
| 12 | config + operator/provisioning | fuzz tests | 3 | LOW |
| | | **Total** | **~188** | |

### Existing Mock Infrastructure

All mocks already exist and are ready to use:

| Mock | Location | Methods |
|------|----------|---------|
| `MockHCloudClient` | `operator/controller/mocks_test.go` | 10 methods with call tracking |
| `MockTalosClient` | `operator/controller/mocks_test.go` | 5 methods with call tracking |
| `MockTalosConfigGenerator` | `operator/controller/mocks_test.go` | 4 methods, thread-safe |
| `MockClient` | `platform/hcloud/mock_client_test.go` | Full InfrastructureManager interface |

### Testing Patterns to Follow

- Table-driven tests with `t.Parallel()` (75% adoption, extend to 100%)
- `require` for preconditions, `assert` for verifications (testify)
- Benchmark hot paths with `b.ReportAllocs()` + `b.Loop()`
- `//go:build integration` for tests requiring external services (Talos, HCloud)
