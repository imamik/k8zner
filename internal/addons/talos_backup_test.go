package addons

import (
	"strings"
	"testing"

	"github.com/imamik/k8zner/internal/config"
)

func TestGenerateTalosBackupManifests(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			TalosBackup: config.TalosBackupConfig{
				Enabled:           true,
				Schedule:          "0 * * * *",
				S3Bucket:          "test-bucket",
				S3Region:          "fsn1",
				S3Endpoint:        "https://fsn1.your-objectstorage.com",
				S3AccessKey:       "test-access-key",
				S3SecretKey:       "test-secret-key",
				S3Prefix:          "etcd-backups",
				EnableCompression: true,
			},
		},
	}

	manifests := generateTalosBackupManifests(cfg)

	if len(manifests) != 3 {
		t.Errorf("Expected 3 manifests (ServiceAccount, Secret, CronJob), got %d", len(manifests))
	}
}

func TestGenerateTalosServiceAccount(t *testing.T) {
	t.Parallel()
	sa := generateTalosServiceAccount()

	if !strings.Contains(sa, "kind: ServiceAccount") {
		t.Error("Expected ServiceAccount kind")
	}
	if !strings.Contains(sa, "apiVersion: talos.dev/v1alpha1") {
		t.Error("Expected talos.dev/v1alpha1 apiVersion")
	}
	if !strings.Contains(sa, "name: talos-backup-secrets") {
		t.Error("Expected name talos-backup-secrets")
	}
	if !strings.Contains(sa, "namespace: kube-system") {
		t.Error("Expected namespace kube-system")
	}
	if !strings.Contains(sa, "os:etcd:backup") {
		t.Error("Expected os:etcd:backup role")
	}
}

func TestGenerateTalosBackupSecret(t *testing.T) {
	t.Parallel()
	backup := config.TalosBackupConfig{
		S3AccessKey: "test-access-key",
		S3SecretKey: "test-secret-key",
	}

	secret := generateTalosBackupSecret(backup)

	if !strings.Contains(secret, "kind: Secret") {
		t.Error("Expected Secret kind")
	}
	if !strings.Contains(secret, "name: talos-backup-s3-secrets") {
		t.Error("Expected name talos-backup-s3-secrets")
	}
	if !strings.Contains(secret, "namespace: kube-system") {
		t.Error("Expected namespace kube-system")
	}
	if !strings.Contains(secret, "type: Opaque") {
		t.Error("Expected Opaque type")
	}
}

func TestGenerateTalosBackupCronJob(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			TalosBackup: config.TalosBackupConfig{
				Enabled:           true,
				Schedule:          "0 2 * * *",
				S3Bucket:          "test-bucket",
				S3Region:          "fsn1",
				S3Endpoint:        "https://fsn1.your-objectstorage.com",
				S3AccessKey:       "test-access-key",
				S3SecretKey:       "test-secret-key",
				S3Prefix:          "etcd-backups",
				EnableCompression: true,
			},
		},
	}

	cronjob := generateTalosBackupCronJob(cfg)

	if !strings.Contains(cronjob, "kind: CronJob") {
		t.Error("Expected CronJob kind")
	}
	if !strings.Contains(cronjob, "name: talos-backup") {
		t.Error("Expected name talos-backup")
	}
	if !strings.Contains(cronjob, "namespace: kube-system") {
		t.Error("Expected namespace kube-system")
	}
	if !strings.Contains(cronjob, "schedule: 0 2 * * *") {
		t.Error("Expected schedule '0 2 * * *'")
	}
	if !strings.Contains(cronjob, "ghcr.io/siderolabs/talos-backup:") {
		t.Error("Expected talos-backup image from ghcr.io/siderolabs")
	}
}

func TestBuildTalosBackupEnv_EncryptionEnabled(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			TalosBackup: config.TalosBackupConfig{
				Enabled:            true,
				S3Bucket:           "test-bucket",
				S3Region:           "fsn1",
				S3Endpoint:         "https://fsn1.your-objectstorage.com",
				EnableCompression:  true,
				EncryptionDisabled: false, // Encryption enabled (default)
			},
		},
	}

	env := buildTalosBackupEnv(cfg)

	// Find DISABLE_ENCRYPTION env var
	var disableEncryption string
	for _, e := range env {
		if name, ok := e["name"].(string); ok && name == "DISABLE_ENCRYPTION" {
			disableEncryption = e["value"].(string)
			break
		}
	}

	if disableEncryption != "false" {
		t.Errorf("Expected DISABLE_ENCRYPTION=false when encryption is enabled, got %s", disableEncryption)
	}
}

func TestBuildTalosBackupEnv_EncryptionDisabled(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			TalosBackup: config.TalosBackupConfig{
				Enabled:            true,
				S3Bucket:           "test-bucket",
				S3Region:           "fsn1",
				S3Endpoint:         "https://fsn1.your-objectstorage.com",
				EnableCompression:  true,
				EncryptionDisabled: true, // Encryption disabled
			},
		},
	}

	env := buildTalosBackupEnv(cfg)

	// Find DISABLE_ENCRYPTION env var
	var disableEncryption string
	for _, e := range env {
		if name, ok := e["name"].(string); ok && name == "DISABLE_ENCRYPTION" {
			disableEncryption = e["value"].(string)
			break
		}
	}

	if disableEncryption != "true" {
		t.Errorf("Expected DISABLE_ENCRYPTION=true when encryption is disabled, got %s", disableEncryption)
	}
}

func TestBuildTalosBackupEnv_AllEnvVars(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			TalosBackup: config.TalosBackupConfig{
				Enabled:            true,
				S3Bucket:           "my-bucket",
				S3Region:           "fsn1",
				S3Endpoint:         "https://fsn1.your-objectstorage.com",
				EnableCompression:  true,
				EncryptionDisabled: false,
			},
		},
	}

	env := buildTalosBackupEnv(cfg)

	// Expected env vars
	expectedEnvVars := map[string]interface{}{
		"AWS_ACCESS_KEY_ID":     true, // secretKeyRef
		"AWS_SECRET_ACCESS_KEY": true, // secretKeyRef
		"AWS_REGION":            "fsn1",
		"CUSTOM_S3_ENDPOINT":    "https://fsn1.your-objectstorage.com",
		"BUCKET":                "my-bucket",
		"CLUSTER_NAME":          "test-cluster",
		"S3_PREFIX":             "etcd-backups",
		"USE_PATH_STYLE":        "false",
		"ENABLE_COMPRESSION":    "true",
		"DISABLE_ENCRYPTION":    "false",
	}

	for _, e := range env {
		name, ok := e["name"].(string)
		if !ok {
			t.Error("Expected name to be a string")
			continue
		}

		expected, exists := expectedEnvVars[name]
		if !exists {
			t.Errorf("Unexpected env var: %s", name)
			continue
		}

		// For secret refs, just verify they exist
		if expected == true {
			if _, hasValueFrom := e["valueFrom"]; !hasValueFrom {
				t.Errorf("Expected %s to have valueFrom (secretKeyRef)", name)
			}
		} else {
			// For regular values, verify the value
			if value, hasValue := e["value"].(string); hasValue {
				if value != expected.(string) {
					t.Errorf("Expected %s=%s, got %s", name, expected, value)
				}
			} else {
				t.Errorf("Expected %s to have a value", name)
			}
		}
	}
}

func TestTalosBackupVersion(t *testing.T) {
	t.Parallel()
	version := talosBackupVersion()

	// Version should not be empty
	if version == "" {
		t.Error("Expected talos backup version to be non-empty")
	}

	// Version should start with 'v'
	if !strings.HasPrefix(version, "v") {
		t.Errorf("Expected talos backup version to start with 'v', got %s", version)
	}
}

func TestGenerateTalosBackupCronJob_Tolerations(t *testing.T) {
	t.Parallel()
	// Tolerations are critical for the backup CronJob to schedule on control plane nodes.
	// Without the uninitialized toleration, the job may fail to schedule during bootstrap.

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			TalosBackup: config.TalosBackupConfig{
				Enabled:    true,
				Schedule:   "0 * * * *",
				S3Bucket:   "test-bucket",
				S3Region:   "fsn1",
				S3Endpoint: "https://fsn1.your-objectstorage.com",
			},
		},
	}

	cronjob := generateTalosBackupCronJob(cfg)

	// Verify control-plane toleration exists
	if !strings.Contains(cronjob, "node-role.kubernetes.io/control-plane") {
		t.Error("Expected control-plane toleration")
	}

	// Verify uninitialized toleration exists (critical for bootstrap)
	if !strings.Contains(cronjob, "node.cloudprovider.kubernetes.io/uninitialized") {
		t.Error("Expected uninitialized node toleration for CCM bootstrap compatibility")
	}
}
