# Implementation Plan: Fill Terraform Feature Gaps

## Overview

This plan addresses 8 gaps between the Go and Terraform implementations, following existing patterns discovered during exploration.

---

## Gap 1: Add Talos CCM Addon

**Terraform Reference:** `terraform/variables.tf:945-955`, `terraform/talos_config.tf:29-31`

**Pattern:** Remote manifest download (same as PrometheusOperatorCRDs, GatewayAPICRDs)

### Files to Modify

1. **`internal/config/types.go`** - Add TalosCCMConfig struct to AddonsConfig
```go
// In AddonsConfig struct, add:
TalosCCM TalosCCMConfig `mapstructure:"talos_ccm" yaml:"talos_ccm"`

// New struct:
type TalosCCMConfig struct {
    Enabled bool   `mapstructure:"enabled" yaml:"enabled"`
    Version string `mapstructure:"version" yaml:"version"`
}
```

2. **`internal/config/defaults.go`** - Add defaults
```go
const (
    DefaultTalosCCMVersion = "v1.11.0"
)

// In applyAddonDefaults:
if cfg.Addons.TalosCCM.Enabled && cfg.Addons.TalosCCM.Version == "" {
    cfg.Addons.TalosCCM.Version = DefaultTalosCCMVersion
}
```

3. **`internal/addons/talosCCM.go`** - New file
```go
package addons

const defaultTalosCCMVersion = "v1.11.0"

func applyTalosCCM(ctx context.Context, kubeconfigPath string, cfg *config.Config) error {
    if !cfg.Addons.TalosCCM.Enabled {
        return nil
    }

    version := cfg.Addons.TalosCCM.Version
    if version == "" {
        version = defaultTalosCCMVersion
    }

    manifestURL := fmt.Sprintf(
        "https://raw.githubusercontent.com/siderolabs/talos-cloud-controller-manager/%s/docs/deploy/cloud-controller-manager-daemonset.yml",
        version,
    )

    log.Printf("Installing Talos CCM %s...", version)

    if err := applyFromURL(ctx, kubeconfigPath, "talos-ccm", manifestURL); err != nil {
        return fmt.Errorf("failed to apply Talos CCM from %s: %w", manifestURL, err)
    }

    return nil
}
```

4. **`internal/addons/apply.go`** - Add to installation order (after CRDs, before Cilium)
```go
// After PrometheusOperatorCRDs, before Cilium:
if cfg.Addons.TalosCCM.Enabled {
    if err := applyTalosCCM(ctx, tmpKubeconfig, cfg); err != nil {
        return fmt.Errorf("failed to install Talos CCM: %w", err)
    }
}
```

---

## Gap 2: Wire Upgrade Options Through

**Terraform Reference:** `terraform/variables.tf:595-628`, `terraform/talos.tf:54-66`

**Current Bug:** Config has Debug, Force, Insecure, RebootMode but they're not used. Missing Stage.

### Files to Modify

1. **`internal/config/types.go`** - Add Stage field
```go
type UpgradeConfig struct {
    Debug      bool   `mapstructure:"debug" yaml:"debug"`
    Force      bool   `mapstructure:"force" yaml:"force"`
    Insecure   bool   `mapstructure:"insecure" yaml:"insecure"`
    RebootMode string `mapstructure:"reboot_mode" yaml:"reboot_mode"`
    Stage      bool   `mapstructure:"stage" yaml:"stage"` // NEW
}
```

2. **`internal/config/validation.go`** - Add validation for RebootMode
```go
// In validateTalosConfig:
if cfg.Talos.Upgrade.RebootMode != "" &&
   cfg.Talos.Upgrade.RebootMode != "default" &&
   cfg.Talos.Upgrade.RebootMode != "powercycle" {
    errs = append(errs, fmt.Errorf("talos.upgrade.reboot_mode must be 'default' or 'powercycle'"))
}
```

3. **`internal/platform/talos/upgrade.go`** - Update UpgradeNode to accept options
```go
// Change signature:
func (g *Generator) UpgradeNode(ctx context.Context, endpoint, imageURL string, opts UpgradeOptions) error

// New options type:
type UpgradeOptions struct {
    Stage      bool
    Force      bool
    // Debug and Insecure are handled at talosctl level, not client API
}

// Update implementation:
func (g *Generator) UpgradeNode(ctx context.Context, endpoint, imageURL string, opts UpgradeOptions) error {
    talosClient, err := g.createClient(ctx, endpoint)
    if err != nil {
        return fmt.Errorf("failed to create Talos client: %w", err)
    }
    defer talosClient.Close()

    nodeCtx := client.WithNode(ctx, endpoint)
    _, err = talosClient.Upgrade(nodeCtx, imageURL, opts.Stage, opts.Force)
    if err != nil {
        return fmt.Errorf("failed to upgrade node %s: %w", endpoint, err)
    }
    return nil
}
```

4. **`internal/provisioning/upgrade/provisioner.go`** - Pass options from config
```go
func (p *Provisioner) upgradeNode(ctx *provisioning.Context, nodeIP, roleType string) error {
    // ... existing image URL building ...

    // Create upgrade options from config
    opts := talos.UpgradeOptions{
        Stage: ctx.Config.Talos.Upgrade.Stage,
        Force: ctx.Config.Talos.Upgrade.Force,
    }

    // Call with options
    if err := ctx.Talos.UpgradeNode(ctx, nodeIP, imageURL, opts); err != nil {
        return fmt.Errorf("failed to upgrade %s node %s: %w", roleType, nodeIP, err)
    }
    // ...
}
```

5. **`internal/provisioning/interfaces.go`** - Update interface
```go
type TalosConfigProducer interface {
    // ... existing methods ...
    UpgradeNode(ctx context.Context, endpoint, imageURL string, opts talos.UpgradeOptions) error
}
```

---

## Gap 3: Add Metrics Server Options

**Terraform Reference:** `terraform/variables.tf:1569-1579`, `terraform/metrics_server.tf:1-16`

### Files to Modify

1. **`internal/config/types.go`** - Add fields
```go
type MetricsServerConfig struct {
    Enabled                bool `mapstructure:"enabled" yaml:"enabled"`
    Helm                   HelmChartConfig `mapstructure:"helm" yaml:"helm"`
    ScheduleOnControlPlane *bool `mapstructure:"schedule_on_control_plane" yaml:"schedule_on_control_plane"`
    Replicas               *int  `mapstructure:"replicas" yaml:"replicas"`
}
```

2. **`internal/addons/metricsServer.go`** - Use config values in Helm values
```go
func buildMetricsServerValues(cfg *config.Config) helm.Values {
    msCfg := cfg.Addons.MetricsServer

    // Calculate defaults like Terraform does
    workerCount := getWorkerCount(cfg)
    scheduleOnCP := msCfg.ScheduleOnControlPlane
    if scheduleOnCP == nil {
        defaultScheduleOnCP := workerCount == 0
        scheduleOnCP = &defaultScheduleOnCP
    }

    // Calculate node sum for replica default
    var nodeSum int
    if *scheduleOnCP {
        nodeSum = getControlPlaneCount(cfg)
    } else if workerCount > 0 {
        nodeSum = workerCount
    } else {
        nodeSum = getAutoscalerMaxCount(cfg)
    }

    replicas := msCfg.Replicas
    if replicas == nil {
        defaultReplicas := 1
        if nodeSum > 1 {
            defaultReplicas = 2
        }
        replicas = &defaultReplicas
    }

    values := helm.Values{
        "replicas": *replicas,
        "args": []string{
            "--kubelet-insecure-tls",
            "--kubelet-preferred-address-types=InternalIP,ExternalIP,Hostname",
        },
    }

    // Add node selector and tolerations if scheduling on control plane
    if *scheduleOnCP {
        values["nodeSelector"] = helm.Values{
            "node-role.kubernetes.io/control-plane": "",
        }
        values["tolerations"] = []helm.Values{
            {
                "key":      "node-role.kubernetes.io/control-plane",
                "effect":   "NoSchedule",
                "operator": "Exists",
            },
        }
    }

    return helm.MergeCustomValues(values, msCfg.Helm.Values)
}
```

---

## Gap 4: Add Image Builder Configuration

**Terraform Reference:** `terraform/variables.tf:542-573`, `terraform/image.tf:124-196`

### Files to Modify

1. **`internal/config/types.go`** - Add ImageBuilderConfig
```go
// In TalosConfig struct, add:
ImageBuilder ImageBuilderConfig `mapstructure:"image_builder" yaml:"image_builder"`

// New struct:
type ImageBuilderConfig struct {
    AMD64 ImageBuilderArchConfig `mapstructure:"amd64" yaml:"amd64"`
    ARM64 ImageBuilderArchConfig `mapstructure:"arm64" yaml:"arm64"`
}

type ImageBuilderArchConfig struct {
    ServerType     string `mapstructure:"server_type" yaml:"server_type"`
    ServerLocation string `mapstructure:"server_location" yaml:"server_location"`
}
```

2. **`internal/config/defaults.go`** - Add defaults
```go
const (
    DefaultAMD64BuilderServerType     = "cpx11"
    DefaultAMD64BuilderServerLocation = "ash"
    DefaultARM64BuilderServerType     = "cax11"
    DefaultARM64BuilderServerLocation = "nbg1"
)

// In applyTalosDefaults:
if cfg.Talos.ImageBuilder.AMD64.ServerType == "" {
    cfg.Talos.ImageBuilder.AMD64.ServerType = DefaultAMD64BuilderServerType
}
if cfg.Talos.ImageBuilder.AMD64.ServerLocation == "" {
    cfg.Talos.ImageBuilder.AMD64.ServerLocation = DefaultAMD64BuilderServerLocation
}
if cfg.Talos.ImageBuilder.ARM64.ServerType == "" {
    cfg.Talos.ImageBuilder.ARM64.ServerType = DefaultARM64BuilderServerType
}
if cfg.Talos.ImageBuilder.ARM64.ServerLocation == "" {
    cfg.Talos.ImageBuilder.ARM64.ServerLocation = DefaultARM64BuilderServerLocation
}
```

3. **`internal/provisioning/image/builder.go`** - Use config
```go
// Update Build signature to accept config:
func (b *Builder) Build(ctx context.Context, architecture, talosVersion, schematicID, location string, builderConfig config.ImageBuilderArchConfig) (string, error) {
    // Use config values instead of defaults
    serverType := builderConfig.ServerType
    if serverType == "" {
        serverType = hcloud.GetDefaultServerType(hcloud.Architecture(architecture))
    }

    serverLocation := builderConfig.ServerLocation
    if serverLocation == "" {
        serverLocation = location
    }

    // Use serverType and serverLocation in CreateServer call
    serverID, err := b.infra.CreateServer(ctx, serverName, "debian-13", serverType, serverLocation, sshKeys, labels, "", nil, 0, "")
    // ...
}
```

4. **Update callers** in `internal/provisioning/image/provisioner.go` to pass config

---

## Gap 5: Add Talos Backup S3 Hcloud URL Helper

**Terraform Reference:** `terraform/variables.tf:829-833`, `terraform/talos_backup.tf:2-5`

**Regex Pattern:** `^(?:https?://)?(?P<bucket>[^.]+)\.(?P<region>[^.]+)\.your-objectstorage\.com\.?$`

### Files to Modify

1. **`internal/config/types.go`** - Add S3HcloudURL field
```go
type TalosBackupS3Config struct {
    // Existing fields...

    // HcloudURL is a convenience field for Hetzner Object Storage
    // Format: bucket.region.your-objectstorage.com
    // When set, automatically extracts Bucket, Region, and Endpoint
    HcloudURL string `mapstructure:"hcloud_url" yaml:"hcloud_url"`
}
```

2. **`internal/config/defaults.go`** - Parse URL and set derived values
```go
import "regexp"

var hcloudS3URLRegex = regexp.MustCompile(`^(?:https?://)?([^.]+)\.([^.]+)\.your-objectstorage\.com\.?$`)

func applyTalosBackupDefaults(cfg *Config) {
    backup := &cfg.Addons.TalosBackup

    // Parse Hcloud URL if provided
    if backup.S3.HcloudURL != "" {
        matches := hcloudS3URLRegex.FindStringSubmatch(backup.S3.HcloudURL)
        if len(matches) == 3 {
            if backup.S3.Bucket == "" {
                backup.S3.Bucket = matches[1]
            }
            if backup.S3.Region == "" {
                backup.S3.Region = matches[2]
            }
            if backup.S3.Endpoint == "" {
                backup.S3.Endpoint = fmt.Sprintf("https://%s.your-objectstorage.com", matches[2])
            }
        }
    }
}
```

3. **`internal/config/validation.go`** - Validate URL format
```go
func validateTalosBackupConfig(cfg *Config) []error {
    var errs []error
    backup := cfg.Addons.TalosBackup

    if backup.S3.HcloudURL != "" && !hcloudS3URLRegex.MatchString(backup.S3.HcloudURL) {
        errs = append(errs, fmt.Errorf("talos_backup.s3.hcloud_url must match format: bucket.region.your-objectstorage.com"))
    }

    return errs
}
```

---

## Gap 6: Add Prerequisites Check

**Terraform Reference:** `terraform/client.tf:130-212`

### Files to Create

1. **`internal/util/prerequisites/check.go`** - New file
```go
package prerequisites

import (
    "fmt"
    "os/exec"
    "regexp"
    "strings"
)

type CheckResult struct {
    Tool      string
    Available bool
    Version   string
    Error     string
}

type Options struct {
    CheckPacker   bool
    CheckJQ       bool
    CheckTalosctl bool
    CheckKubectl  bool
    TalosVersion  string // For version comparison
}

// Check verifies required tools are installed
func Check(opts Options) ([]CheckResult, error) {
    var results []CheckResult
    var missing []string

    tools := []struct {
        name    string
        check   bool
        cmd     string
        args    []string
        helpURL string
    }{
        {"packer", opts.CheckPacker, "packer", []string{"version"}, "https://developer.hashicorp.com/packer/install"},
        {"jq", opts.CheckJQ, "jq", []string{"--version"}, "https://jqlang.github.io/jq/download/"},
        {"talosctl", opts.CheckTalosctl, "talosctl", []string{"version", "--client", "--short"}, "https://www.talos.dev/latest/talos-guides/install/talosctl/"},
        {"kubectl", opts.CheckKubectl, "kubectl", []string{"version", "--client", "--short"}, "https://kubernetes.io/docs/tasks/tools/"},
    }

    for _, tool := range tools {
        if !tool.check {
            continue
        }

        result := CheckResult{Tool: tool.name}

        cmd := exec.Command(tool.cmd, tool.args...)
        output, err := cmd.Output()
        if err != nil {
            result.Available = false
            result.Error = fmt.Sprintf("not installed. Install from: %s", tool.helpURL)
            missing = append(missing, tool.name)
        } else {
            result.Available = true
            result.Version = strings.TrimSpace(string(output))
        }

        results = append(results, result)
    }

    if len(missing) > 0 {
        return results, fmt.Errorf("missing required tools: %s", strings.Join(missing, ", "))
    }

    return results, nil
}

// CheckTalosctlVersion verifies talosctl version meets minimum requirement
func CheckTalosctlVersion(minVersion string) error {
    cmd := exec.Command("talosctl", "version", "--client", "--short")
    output, err := cmd.Output()
    if err != nil {
        return fmt.Errorf("failed to get talosctl version: %w", err)
    }

    version := strings.TrimSpace(string(output))
    if !versionAtLeast(version, minVersion) {
        return fmt.Errorf("talosctl version %s is below minimum required %s", version, minVersion)
    }

    return nil
}

func versionAtLeast(current, minimum string) bool {
    // Parse versions like "v1.9.0" or "1.9.0"
    re := regexp.MustCompile(`v?(\d+)\.(\d+)\.(\d+)`)

    curMatch := re.FindStringSubmatch(current)
    minMatch := re.FindStringSubmatch(minimum)

    if len(curMatch) != 4 || len(minMatch) != 4 {
        return false
    }

    for i := 1; i <= 3; i++ {
        cur := parseInt(curMatch[i])
        min := parseInt(minMatch[i])
        if cur > min {
            return true
        }
        if cur < min {
            return false
        }
    }
    return true // equal
}
```

2. **`cmd/hcloud-k8s/handlers/apply.go`** - Add preflight check
```go
// At start of Apply handler:
if !opts.SkipPreflightCheck {
    results, err := prerequisites.Check(prerequisites.Options{
        CheckTalosctl: true,
        CheckKubectl:  true,
    })
    if err != nil {
        return fmt.Errorf("preflight check failed: %w", err)
    }

    // Check talosctl version matches target
    if err := prerequisites.CheckTalosctlVersion(cfg.Talos.Version); err != nil {
        log.Printf("Warning: %v", err)
    }
}
```

---

## Gap 7: Add Enhanced Outputs (Cilium Encryption Info)

**Terraform Reference:** `terraform/outputs.tf:72-87`

### Files to Modify

1. **`internal/provisioning/state.go`** - Add output structs
```go
// CiliumEncryptionInfo contains Cilium encryption status
type CiliumEncryptionInfo struct {
    EncryptionEnabled bool   `json:"encryption_enabled"`
    EncryptionType    string `json:"encryption_type"`
    IPSec             *IPSecInfo `json:"ipsec,omitempty"`
}

type IPSecInfo struct {
    CurrentKeyID int    `json:"current_key_id"`
    NextKeyID    int    `json:"next_key_id"`
    Algorithm    string `json:"algorithm"`
    KeySizeBits  int    `json:"key_size_bits"`
    SecretName   string `json:"secret_name"`
    Namespace    string `json:"namespace"`
}

// In State struct, add:
CiliumEncryption *CiliumEncryptionInfo `json:"cilium_encryption,omitempty"`
```

2. **`internal/addons/cilium.go`** - Populate encryption info
```go
func buildCiliumEncryptionInfo(cfg *config.Config) *provisioning.CiliumEncryptionInfo {
    ciliumCfg := cfg.Addons.Cilium

    info := &provisioning.CiliumEncryptionInfo{
        EncryptionEnabled: ciliumCfg.EncryptionEnabled,
        EncryptionType:    ciliumCfg.EncryptionType,
    }

    if ciliumCfg.EncryptionEnabled && ciliumCfg.EncryptionType == "ipsec" {
        keyID := ciliumCfg.IPSecKeyID
        if keyID == 0 {
            keyID = 1
        }
        nextKeyID := keyID + 1
        if nextKeyID > 15 {
            nextKeyID = 1
        }

        info.IPSec = &provisioning.IPSecInfo{
            CurrentKeyID: keyID,
            NextKeyID:    nextKeyID,
            Algorithm:    ciliumCfg.IPSecAlgorithm,
            KeySizeBits:  ciliumCfg.IPSecKeySize,
            SecretName:   "cilium-ipsec-keys",
            Namespace:    "kube-system",
        }
    }

    return info
}
```

3. **`cmd/hcloud-k8s/handlers/apply.go`** - Print encryption info
```go
// After addon installation:
if cfg.Addons.Cilium.Enabled && cfg.Addons.Cilium.EncryptionEnabled {
    encInfo := addons.BuildCiliumEncryptionInfo(cfg)
    log.Printf("Cilium Encryption: type=%s", encInfo.EncryptionType)
    if encInfo.IPSec != nil {
        log.Printf("  IPSec Key ID: %d (next: %d)", encInfo.IPSec.CurrentKeyID, encInfo.IPSec.NextKeyID)
    }
}
```

---

## Gap 8: Add Tests

### Test Files to Create/Modify

1. **`internal/addons/talosCCM_test.go`** - Test Talos CCM addon
2. **`internal/config/types_test.go`** - Test new config structs
3. **`internal/config/defaults_test.go`** - Test S3 URL parsing
4. **`internal/util/prerequisites/check_test.go`** - Test prerequisites
5. **`tests/e2e/addons_test.go`** - Add Talos CCM to E2E tests

---

## Implementation Order

1. **Gap 1: Talos CCM** - Self-contained, no dependencies
2. **Gap 2: Upgrade Options** - Important bug fix
3. **Gap 3: Metrics Server** - Simple config addition
4. **Gap 5: S3 Hcloud URL** - Simple convenience helper
5. **Gap 4: Image Builder** - Slightly more complex
6. **Gap 6: Prerequisites** - New utility
7. **Gap 7: Outputs** - Enhancement
8. **Gap 8: Tests** - Throughout implementation

---

## Verification Checklist

- [ ] All config changes have corresponding YAML tags
- [ ] Defaults match Terraform exactly
- [ ] Validation matches Terraform rules
- [ ] E2E tests pass
- [ ] Unit tests added for new code
- [ ] No breaking changes to existing config
