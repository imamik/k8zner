package addons

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"hcloud-k8s/internal/config"
)

func TestGenerateTalosBackupManifests(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			TalosBackup: config.TalosBackupConfig{
				Enabled:            true,
				Version:            "v0.1.0",
				Schedule:           "0 * * * *",
				S3Bucket:           "my-bucket",
				S3Region:           "us-east-1",
				S3Endpoint:         "https://s3.example.com",
				S3Prefix:           "backups/",
				S3AccessKey:        "test-access-key",
				S3SecretKey:        "test-secret-key",
				S3PathStyle:        false,
				AGEX25519PublicKey: "age1test...",
				EnableCompression:  true,
			},
		},
	}

	manifests := generateTalosBackupManifests(cfg)

	// Should generate 3 manifests
	assert.Len(t, manifests, 3)

	// Verify ServiceAccount
	var sa map[string]any
	err := yaml.Unmarshal([]byte(manifests[0]), &sa)
	require.NoError(t, err)
	assert.Equal(t, "talos.dev/v1alpha1", sa["apiVersion"])
	assert.Equal(t, "ServiceAccount", sa["kind"])

	// Verify Secret
	var secret map[string]any
	err = yaml.Unmarshal([]byte(manifests[1]), &secret)
	require.NoError(t, err)
	assert.Equal(t, "v1", secret["apiVersion"])
	assert.Equal(t, "Secret", secret["kind"])

	data := secret["data"].(map[string]any)
	assert.NotEmpty(t, data["access_key"])
	assert.NotEmpty(t, data["secret_key"])

	// Verify CronJob
	var cronjob map[string]any
	err = yaml.Unmarshal([]byte(manifests[2]), &cronjob)
	require.NoError(t, err)
	assert.Equal(t, "batch/v1", cronjob["apiVersion"])
	assert.Equal(t, "CronJob", cronjob["kind"])

	spec := cronjob["spec"].(map[string]any)
	assert.Equal(t, "0 * * * *", spec["schedule"])
	assert.Equal(t, "Forbid", spec["concurrencyPolicy"])
}

func TestGenerateTalosServiceAccount(t *testing.T) {
	manifestYAML := generateTalosServiceAccount()

	assert.Contains(t, manifestYAML, "apiVersion: talos.dev/v1alpha1")
	assert.Contains(t, manifestYAML, "kind: ServiceAccount")
	assert.Contains(t, manifestYAML, "name: talos-backup-secrets")
	assert.Contains(t, manifestYAML, "namespace: kube-system")
	assert.Contains(t, manifestYAML, "os:etcd:backup")

	// Parse and verify structure
	var sa map[string]any
	err := yaml.Unmarshal([]byte(manifestYAML), &sa)
	require.NoError(t, err)

	metadata := sa["metadata"].(map[string]any)
	assert.Equal(t, "talos-backup-secrets", metadata["name"])
	assert.Equal(t, "kube-system", metadata["namespace"])

	spec := sa["spec"].(map[string]any)
	roles := spec["roles"].([]any)
	assert.Len(t, roles, 1)
	assert.Equal(t, "os:etcd:backup", roles[0])
}

func TestGenerateTalosBackupSecret(t *testing.T) {
	backup := config.TalosBackupConfig{
		S3AccessKey: "test-access",
		S3SecretKey: "test-secret",
	}

	manifestYAML := generateTalosBackupSecret(backup)

	assert.Contains(t, manifestYAML, "apiVersion: v1")
	assert.Contains(t, manifestYAML, "kind: Secret")
	assert.Contains(t, manifestYAML, "name: talos-backup-s3-secrets")
	assert.Contains(t, manifestYAML, "type: Opaque")

	// Parse and verify
	var secret map[string]any
	err := yaml.Unmarshal([]byte(manifestYAML), &secret)
	require.NoError(t, err)

	data := secret["data"].(map[string]any)
	assert.NotEmpty(t, data["access_key"])
	assert.NotEmpty(t, data["secret_key"])

	// Verify base64 encoding
	accessKey := data["access_key"].(string)
	secretKey := data["secret_key"].(string)
	assert.NotEqual(t, "test-access", accessKey) // Should be base64 encoded
	assert.NotEqual(t, "test-secret", secretKey)
}

func TestGenerateTalosBackupCronJob(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			TalosBackup: config.TalosBackupConfig{
				Version:            "v0.1.0",
				Schedule:           "0 */6 * * *",
				S3Bucket:           "backups",
				S3Region:           "eu-central-1",
				S3Endpoint:         "https://s3.hetzner.com",
				S3Prefix:           "etcd/",
				S3PathStyle:        true,
				AGEX25519PublicKey: "",
				EnableCompression:  false,
			},
		},
	}

	manifestYAML := generateTalosBackupCronJob(cfg)

	// Verify structure
	assert.Contains(t, manifestYAML, "apiVersion: batch/v1")
	assert.Contains(t, manifestYAML, "kind: CronJob")
	assert.Contains(t, manifestYAML, "schedule: 0 */6 * * *")
	assert.Contains(t, manifestYAML, "concurrencyPolicy: Forbid")
	assert.Contains(t, manifestYAML, "restartPolicy: OnFailure")

	// Verify container settings
	assert.Contains(t, manifestYAML, "ghcr.io/siderolabs/talos-backup:v0.1.0")
	assert.Contains(t, manifestYAML, "workingDir: /tmp")

	// Verify environment variables
	assert.Contains(t, manifestYAML, "AWS_ACCESS_KEY_ID")
	assert.Contains(t, manifestYAML, "AWS_SECRET_ACCESS_KEY")
	assert.Contains(t, manifestYAML, "BUCKET")
	assert.Contains(t, manifestYAML, "CLUSTER_NAME")

	// Verify security context
	assert.Contains(t, manifestYAML, "runAsUser: 1000")
	assert.Contains(t, manifestYAML, "runAsNonRoot: true")
	assert.Contains(t, manifestYAML, "allowPrivilegeEscalation: false")

	// Verify volumes
	assert.Contains(t, manifestYAML, "talos-secrets")
	assert.Contains(t, manifestYAML, "emptyDir")

	// Verify tolerations
	assert.Contains(t, manifestYAML, "node-role.kubernetes.io/control-plane")

	// Parse and verify detailed structure
	var cronjob map[string]any
	err := yaml.Unmarshal([]byte(manifestYAML), &cronjob)
	require.NoError(t, err)

	metadata := cronjob["metadata"].(map[string]any)
	assert.Equal(t, "talos-backup", metadata["name"])
	assert.Equal(t, "kube-system", metadata["namespace"])

	spec := cronjob["spec"].(map[string]any)
	assert.Equal(t, "0 */6 * * *", spec["schedule"])
	assert.False(t, spec["suspend"].(bool))
}

func TestGenerateTalosBackupCronJobDefaultVersion(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			TalosBackup: config.TalosBackupConfig{
				Version:  "", // Empty, should use default
				S3Bucket: "test",
			},
		},
	}

	manifestYAML := generateTalosBackupCronJob(cfg)
	assert.Contains(t, manifestYAML, "ghcr.io/siderolabs/talos-backup:v0.1.0-beta.3-3-g38dad7c")
}

func TestGenerateTalosBackupCronJobDefaultSchedule(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			TalosBackup: config.TalosBackupConfig{
				Schedule: "", // Empty, should use default
				S3Bucket: "test",
			},
		},
	}

	manifestYAML := generateTalosBackupCronJob(cfg)
	assert.Contains(t, manifestYAML, "schedule: 0 * * * *") // Hourly
}

func TestBuildTalosBackupEnv(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "prod-cluster",
		Addons: config.AddonsConfig{
			TalosBackup: config.TalosBackupConfig{
				S3Bucket:           "prod-backups",
				S3Region:           "us-west-2",
				S3Endpoint:         "https://s3.amazonaws.com",
				S3Prefix:           "etcd-backups/",
				S3PathStyle:        false,
				AGEX25519PublicKey: "age1abc123...",
				EnableCompression:  true,
			},
		},
	}

	env := buildTalosBackupEnv(cfg)

	// Should have all required env vars
	assert.GreaterOrEqual(t, len(env), 9)

	// Verify key environment variables exist
	envMap := make(map[string]string)
	for _, e := range env {
		if name, ok := e["name"].(string); ok {
			if value, ok := e["value"].(string); ok {
				envMap[name] = value
			}
		}
	}

	assert.Equal(t, "prod-backups", envMap["BUCKET"])
	assert.Equal(t, "prod-cluster", envMap["CLUSTER_NAME"])
	assert.Equal(t, "us-west-2", envMap["AWS_REGION"])
	assert.Equal(t, "https://s3.amazonaws.com", envMap["CUSTOM_S3_ENDPOINT"])
	assert.Equal(t, "etcd-backups/", envMap["S3_PREFIX"])
	assert.Equal(t, "false", envMap["USE_PATH_STYLE"])
	assert.Equal(t, "true", envMap["ENABLE_COMPRESSION"])
	assert.Equal(t, "age1abc123...", envMap["AGE_X25519_PUBLIC_KEY"])
	assert.Equal(t, "false", envMap["DISABLE_ENCRYPTION"])
}

func TestBuildTalosBackupEnvNoEncryption(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			TalosBackup: config.TalosBackupConfig{
				S3Bucket:           "test-bucket",
				AGEX25519PublicKey: "", // No encryption key
			},
		},
	}

	env := buildTalosBackupEnv(cfg)

	envMap := make(map[string]string)
	for _, e := range env {
		if name, ok := e["name"].(string); ok {
			if value, ok := e["value"].(string); ok {
				envMap[name] = value
			}
		}
	}

	assert.Equal(t, "", envMap["AGE_X25519_PUBLIC_KEY"])
	assert.Equal(t, "true", envMap["DISABLE_ENCRYPTION"])
}

func TestBuildTalosBackupEnvSecretRefs(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			TalosBackup: config.TalosBackupConfig{
				S3Bucket: "test",
			},
		},
	}

	env := buildTalosBackupEnv(cfg)

	// Find AWS credential env vars
	var accessKeyEnv, secretKeyEnv map[string]any
	for _, e := range env {
		if name, ok := e["name"].(string); ok {
			if name == "AWS_ACCESS_KEY_ID" {
				accessKeyEnv = e
			}
			if name == "AWS_SECRET_ACCESS_KEY" {
				secretKeyEnv = e
			}
		}
	}

	// Verify they use secretKeyRef
	require.NotNil(t, accessKeyEnv)
	require.NotNil(t, secretKeyEnv)

	accessKeyRef := accessKeyEnv["valueFrom"].(map[string]any)["secretKeyRef"].(map[string]any)
	assert.Equal(t, "talos-backup-s3-secrets", accessKeyRef["name"])
	assert.Equal(t, "access_key", accessKeyRef["key"])

	secretKeyRef := secretKeyEnv["valueFrom"].(map[string]any)["secretKeyRef"].(map[string]any)
	assert.Equal(t, "talos-backup-s3-secrets", secretKeyRef["name"])
	assert.Equal(t, "secret_key", secretKeyRef["key"])
}

func TestValidateTalosBackupConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  config.TalosBackupConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: config.TalosBackupConfig{
				S3Bucket:    "valid-bucket",
				S3AccessKey: "access-key",
				S3SecretKey: "secret-key",
			},
			wantErr: false,
		},
		{
			name: "missing bucket",
			config: config.TalosBackupConfig{
				S3AccessKey: "access-key",
				S3SecretKey: "secret-key",
			},
			wantErr: true,
			errMsg:  "s3_bucket is required",
		},
		{
			name: "missing access key",
			config: config.TalosBackupConfig{
				S3Bucket:    "valid-bucket",
				S3SecretKey: "secret-key",
			},
			wantErr: true,
			errMsg:  "s3_access_key is required",
		},
		{
			name: "missing secret key",
			config: config.TalosBackupConfig{
				S3Bucket:    "valid-bucket",
				S3AccessKey: "access-key",
			},
			wantErr: true,
			errMsg:  "s3_secret_key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTalosBackupConfig(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGenerateTalosBackupManifestsCombination(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test",
		Addons: config.AddonsConfig{
			TalosBackup: config.TalosBackupConfig{
				Enabled:     true,
				Version:     "v0.1.0",
				Schedule:    "0 * * * *",
				S3Bucket:    "test-bucket",
				S3AccessKey: "key",
				S3SecretKey: "secret",
			},
		},
	}

	manifests := generateTalosBackupManifests(cfg)
	combined := strings.Join(manifests, "\n---\n")

	// Verify separators
	separatorCount := strings.Count(combined, "\n---\n")
	assert.Equal(t, 2, separatorCount)

	// Verify all three resource types present
	assert.Contains(t, combined, "kind: ServiceAccount")
	assert.Contains(t, combined, "kind: Secret")
	assert.Contains(t, combined, "kind: CronJob")

	// Verify Talos-specific API version
	assert.Contains(t, combined, "apiVersion: talos.dev/v1alpha1")
}

func TestGenerateTalosBackupCronJobResourceLimits(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test",
		Addons: config.AddonsConfig{
			TalosBackup: config.TalosBackupConfig{
				S3Bucket: "test",
			},
		},
	}

	manifestYAML := generateTalosBackupCronJob(cfg)

	// Verify resource requests and limits
	assert.Contains(t, manifestYAML, "requests:")
	assert.Contains(t, manifestYAML, "memory: 128Mi")
	assert.Contains(t, manifestYAML, "cpu: 250m")
	assert.Contains(t, manifestYAML, "limits:")
	assert.Contains(t, manifestYAML, "memory: 256Mi")
	assert.Contains(t, manifestYAML, "cpu: 500m")
}

func TestGenerateTalosBackupCronJobSecurityContext(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test",
		Addons: config.AddonsConfig{
			TalosBackup: config.TalosBackupConfig{
				S3Bucket: "test",
			},
		},
	}

	manifestYAML := generateTalosBackupCronJob(cfg)

	var cronjob map[string]any
	err := yaml.Unmarshal([]byte(manifestYAML), &cronjob)
	require.NoError(t, err)

	// Navigate to container security context
	spec := cronjob["spec"].(map[string]any)
	jobTemplate := spec["jobTemplate"].(map[string]any)
	jobSpec := jobTemplate["spec"].(map[string]any)
	podTemplate := jobSpec["template"].(map[string]any)
	podSpec := podTemplate["spec"].(map[string]any)
	containers := podSpec["containers"].([]any)
	container := containers[0].(map[string]any)
	securityContext := container["securityContext"].(map[string]any)

	// Verify security settings
	assert.Equal(t, 1000, securityContext["runAsUser"])
	assert.Equal(t, 1000, securityContext["runAsGroup"])
	assert.False(t, securityContext["allowPrivilegeEscalation"].(bool))
	assert.True(t, securityContext["runAsNonRoot"].(bool))

	capabilities := securityContext["capabilities"].(map[string]any)
	drop := capabilities["drop"].([]any)
	assert.Contains(t, drop, "ALL")

	seccompProfile := securityContext["seccompProfile"].(map[string]any)
	assert.Equal(t, "RuntimeDefault", seccompProfile["type"])
}
