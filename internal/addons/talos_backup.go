package addons

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/imamik/k8zner/internal/addons/k8sclient"
	"github.com/imamik/k8zner/internal/config"
)

// applyTalosBackup installs the Talos etcd backup CronJob.
// See: terraform/talos_backup.tf
func applyTalosBackup(ctx context.Context, client k8sclient.Client, cfg *config.Config) error {
	if !cfg.Addons.TalosBackup.Enabled {
		return nil
	}

	// Validate configuration
	if err := validateTalosBackupConfig(cfg.Addons.TalosBackup); err != nil {
		return fmt.Errorf("invalid talos backup configuration: %w", err)
	}

	// Generate manifests
	manifests := generateTalosBackupManifests(cfg)

	// Combine with --- separator
	combined := strings.Join(manifests, "\n---\n")

	// Apply manifests
	if err := applyManifests(ctx, client, "talos-backup", []byte(combined)); err != nil {
		return fmt.Errorf("failed to apply Talos Backup manifests: %w", err)
	}

	return nil
}

// generateTalosBackupManifests creates YAML manifests for Talos backup.
// See: terraform/talos_backup.tf
func generateTalosBackupManifests(cfg *config.Config) []string {
	var manifests []string

	// 1. ServiceAccount (Talos-specific)
	manifests = append(manifests, generateTalosServiceAccount())

	// 2. Secret (S3 credentials)
	manifests = append(manifests, generateTalosBackupSecret(cfg.Addons.TalosBackup))

	// 3. CronJob
	manifests = append(manifests, generateTalosBackupCronJob(cfg))

	return manifests
}

// generateTalosServiceAccount creates the Talos ServiceAccount manifest.
// See: terraform/talos_backup.tf lines 7-19
func generateTalosServiceAccount() string {
	sa := map[string]any{
		"apiVersion": "talos.dev/v1alpha1",
		"kind":       "ServiceAccount",
		"metadata": map[string]any{
			"name":      "talos-backup-secrets",
			"namespace": "kube-system",
		},
		"spec": map[string]any{
			"roles": []string{"os:etcd:backup"},
		},
	}

	yamlBytes, _ := yaml.Marshal(sa)
	return string(yamlBytes)
}

// generateTalosBackupSecret creates the S3 credentials secret.
// See: terraform/talos_backup.tf lines 21-33
func generateTalosBackupSecret(backup config.TalosBackupConfig) string {
	secret := map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]any{
			"name":      "talos-backup-s3-secrets",
			"namespace": "kube-system",
		},
		"type": "Opaque",
		"data": map[string]any{
			"access_key": base64.StdEncoding.EncodeToString([]byte(backup.S3AccessKey)),
			"secret_key": base64.StdEncoding.EncodeToString([]byte(backup.S3SecretKey)),
		},
	}

	yamlBytes, _ := yaml.Marshal(secret)
	return string(yamlBytes)
}

// generateTalosBackupCronJob creates the backup CronJob manifest.
// See: terraform/talos_backup.tf lines 35-98
func generateTalosBackupCronJob(cfg *config.Config) string {
	backup := cfg.Addons.TalosBackup

	// Default version
	version := backup.Version
	if version == "" {
		version = "v0.1.0-beta.3-3-g38dad7c"
	}

	// Default schedule
	schedule := backup.Schedule
	if schedule == "" {
		schedule = "0 * * * *" // Hourly
	}

	// Build environment variables
	envVars := buildTalosBackupEnv(cfg)

	// Build container spec
	container := map[string]any{
		"name":            "talos-backup",
		"image":           fmt.Sprintf("ghcr.io/siderolabs/talos-backup:%s", version),
		"workingDir":      "/tmp",
		"imagePullPolicy": "IfNotPresent",
		"env":             envVars,
		"volumeMounts": []map[string]any{
			{"name": "tmp", "mountPath": "/tmp"},
			{"name": "talos-secrets", "mountPath": "/var/run/secrets/talos.dev"},
		},
		"resources": map[string]any{
			"requests": map[string]string{"memory": "128Mi", "cpu": "250m"},
			"limits":   map[string]string{"memory": "256Mi", "cpu": "500m"},
		},
		"securityContext": map[string]any{
			"runAsUser":                1000,
			"runAsGroup":               1000,
			"allowPrivilegeEscalation": false,
			"runAsNonRoot":             true,
			"capabilities":             map[string]any{"drop": []string{"ALL"}},
			"seccompProfile":           map[string]any{"type": "RuntimeDefault"},
		},
	}

	// Build pod spec
	podSpec := map[string]any{
		"containers":    []map[string]any{container},
		"restartPolicy": "OnFailure",
		"volumes": []map[string]any{
			{"emptyDir": map[string]any{}, "name": "tmp"},
			{"name": "talos-secrets", "secret": map[string]any{"secretName": "talos-backup-secrets"}},
		},
		"tolerations": []map[string]any{
			{"key": "node-role.kubernetes.io/control-plane", "operator": "Exists", "effect": "NoSchedule"},
		},
	}

	// Build CronJob
	cronjob := map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "CronJob",
		"metadata": map[string]any{
			"name":      "talos-backup",
			"namespace": "kube-system",
		},
		"spec": map[string]any{
			"schedule":          schedule,
			"suspend":           false,
			"concurrencyPolicy": "Forbid",
			"jobTemplate": map[string]any{
				"spec": map[string]any{
					"template": map[string]any{
						"spec": podSpec,
					},
				},
			},
		},
	}

	yamlBytes, _ := yaml.Marshal(cronjob)
	return string(yamlBytes)
}

// buildTalosBackupEnv builds the environment variables for the backup container.
// See: terraform/talos_backup.tf lines 55-66
func buildTalosBackupEnv(cfg *config.Config) []map[string]any {
	backup := cfg.Addons.TalosBackup

	envVars := []map[string]any{
		{
			"name": "AWS_ACCESS_KEY_ID",
			"valueFrom": map[string]any{
				"secretKeyRef": map[string]any{
					"name": "talos-backup-s3-secrets",
					"key":  "access_key",
				},
			},
		},
		{
			"name": "AWS_SECRET_ACCESS_KEY",
			"valueFrom": map[string]any{
				"secretKeyRef": map[string]any{
					"name": "talos-backup-s3-secrets",
					"key":  "secret_key",
				},
			},
		},
		{"name": "AWS_REGION", "value": backup.S3Region},
		{"name": "CUSTOM_S3_ENDPOINT", "value": backup.S3Endpoint},
		{"name": "BUCKET", "value": backup.S3Bucket},
		{"name": "CLUSTER_NAME", "value": cfg.ClusterName},
		{"name": "S3_PREFIX", "value": backup.S3Prefix},
		{"name": "USE_PATH_STYLE", "value": fmt.Sprintf("%t", backup.S3PathStyle)},
		{"name": "ENABLE_COMPRESSION", "value": fmt.Sprintf("%t", backup.EnableCompression)},
	}

	// AGE encryption (optional)
	disableEncryption := backup.AGEX25519PublicKey == ""
	envVars = append(envVars, map[string]any{
		"name":  "AGE_X25519_PUBLIC_KEY",
		"value": backup.AGEX25519PublicKey,
	})
	envVars = append(envVars, map[string]any{
		"name":  "DISABLE_ENCRYPTION",
		"value": fmt.Sprintf("%t", disableEncryption),
	})

	return envVars
}

// validateTalosBackupConfig validates the Talos backup configuration.
func validateTalosBackupConfig(backup config.TalosBackupConfig) error {
	if backup.S3Bucket == "" {
		return fmt.Errorf("s3_bucket is required")
	}
	if backup.S3AccessKey == "" {
		return fmt.Errorf("s3_access_key is required")
	}
	if backup.S3SecretKey == "" {
		return fmt.Errorf("s3_secret_key is required")
	}
	return nil
}
