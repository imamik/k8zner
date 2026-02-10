package addons

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/imamik/k8zner/internal/addons/k8sclient"
	"github.com/imamik/k8zner/internal/config"
	v2 "github.com/imamik/k8zner/internal/config/v2"
	"github.com/imamik/k8zner/internal/platform/s3"
)

// talosBackupVersion returns the pinned talos-backup version from the version matrix.
func talosBackupVersion() string {
	return v2.DefaultVersionMatrix().TalosBackup
}

// applyTalosBackup installs the Talos etcd backup CronJob.
func applyTalosBackup(ctx context.Context, client k8sclient.Client, cfg *config.Config) error {
	if !cfg.Addons.TalosBackup.Enabled {
		return nil
	}

	backup := cfg.Addons.TalosBackup
	if backup.S3Bucket == "" || backup.S3AccessKey == "" || backup.S3SecretKey == "" {
		return fmt.Errorf("talos backup requires s3_bucket, s3_access_key, and s3_secret_key")
	}

	// Warn if encryption is disabled
	if backup.EncryptionDisabled {
		log.Printf("WARNING: Talos backup encryption is disabled. etcd backups will be stored unencrypted in S3 bucket %q", backup.S3Bucket)
	}

	// Ensure S3 bucket exists before installing the CronJob
	if err := ensureS3Bucket(ctx, backup); err != nil {
		return fmt.Errorf("failed to ensure S3 bucket: %w", err)
	}

	manifests := generateTalosBackupManifests(cfg)
	combined := strings.Join(manifests, "\n---\n")

	if err := applyManifests(ctx, client, "talos-backup", []byte(combined)); err != nil {
		return fmt.Errorf("failed to apply Talos Backup manifests: %w", err)
	}

	return nil
}

func generateTalosBackupManifests(cfg *config.Config) []string {
	return []string{
		generateTalosServiceAccount(),
		generateTalosBackupSecret(cfg.Addons.TalosBackup),
		generateTalosBackupCronJob(cfg),
	}
}

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

func generateTalosBackupCronJob(cfg *config.Config) string {
	backup := cfg.Addons.TalosBackup

	container := map[string]any{
		"name":            "talos-backup",
		"image":           fmt.Sprintf("ghcr.io/siderolabs/talos-backup:%s", talosBackupVersion()),
		"workingDir":      "/tmp",
		"imagePullPolicy": "IfNotPresent",
		"env":             buildTalosBackupEnv(cfg),
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

	podSpec := map[string]any{
		"containers":    []map[string]any{container},
		"restartPolicy": "OnFailure",
		"volumes": []map[string]any{
			{"emptyDir": map[string]any{}, "name": "tmp"},
			{"name": "talos-secrets", "secret": map[string]any{"secretName": "talos-backup-secrets"}},
		},
		// Tolerations for control plane and uninitialized nodes.
		// The uninitialized toleration is required because CCM may not have
		// finished initializing nodes when this CronJob is first scheduled.
		"tolerations": []map[string]any{
			{"key": "node-role.kubernetes.io/control-plane", "operator": "Exists", "effect": "NoSchedule"},
			{"key": "node.cloudprovider.kubernetes.io/uninitialized", "operator": "Exists", "effect": "NoSchedule"},
		},
	}

	cronjob := map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "CronJob",
		"metadata": map[string]any{
			"name":      "talos-backup",
			"namespace": "kube-system",
		},
		"spec": map[string]any{
			"schedule":          backup.Schedule,
			"suspend":           false,
			"concurrencyPolicy": "Forbid",
			"jobTemplate": map[string]any{
				"spec": map[string]any{
					"template": map[string]any{"spec": podSpec},
				},
			},
		},
	}

	yamlBytes, _ := yaml.Marshal(cronjob)
	return string(yamlBytes)
}

func buildTalosBackupEnv(cfg *config.Config) []map[string]any {
	backup := cfg.Addons.TalosBackup

	// Determine encryption setting - default to enabled (DISABLE_ENCRYPTION=false)
	disableEncryption := "false"
	if backup.EncryptionDisabled {
		disableEncryption = "true"
	}

	return []map[string]any{
		{
			"name": "AWS_ACCESS_KEY_ID",
			"valueFrom": map[string]any{
				"secretKeyRef": map[string]any{"name": "talos-backup-s3-secrets", "key": "access_key"},
			},
		},
		{
			"name": "AWS_SECRET_ACCESS_KEY",
			"valueFrom": map[string]any{
				"secretKeyRef": map[string]any{"name": "talos-backup-s3-secrets", "key": "secret_key"},
			},
		},
		{"name": "AWS_REGION", "value": backup.S3Region},
		{"name": "CUSTOM_S3_ENDPOINT", "value": backup.S3Endpoint},
		{"name": "BUCKET", "value": backup.S3Bucket},
		{"name": "CLUSTER_NAME", "value": cfg.ClusterName},
		{"name": "S3_PREFIX", "value": "etcd-backups"},
		{"name": "USE_PATH_STYLE", "value": "false"},
		{"name": "ENABLE_COMPRESSION", "value": "true"},
		{"name": "DISABLE_ENCRYPTION", "value": disableEncryption},
	}
}

// ensureS3Bucket creates the S3 bucket if it doesn't already exist.
func ensureS3Bucket(ctx context.Context, backup config.TalosBackupConfig) error {
	client, err := s3.NewClient(backup.S3Endpoint, backup.S3Region, backup.S3AccessKey, backup.S3SecretKey)
	if err != nil {
		return fmt.Errorf("failed to create S3 client: %w", err)
	}

	exists, err := client.BucketExists(ctx, backup.S3Bucket)
	if err != nil {
		return fmt.Errorf("failed to check bucket existence: %w", err)
	}

	if exists {
		log.Printf("[talos-backup] S3 bucket already exists: %s", backup.S3Bucket)
		return nil
	}

	if err := client.CreateBucket(ctx, backup.S3Bucket); err != nil {
		return fmt.Errorf("failed to create bucket %s: %w", backup.S3Bucket, err)
	}

	log.Printf("[talos-backup] S3 bucket created: %s", backup.S3Bucket)
	return nil
}
