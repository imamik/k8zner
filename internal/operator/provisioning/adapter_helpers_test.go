package provisioning

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/config"
	"github.com/imamik/k8zner/internal/provisioning"
)

// --- extractCredentials tests ---

func TestExtractCredentials_AllFields(t *testing.T) {
	t.Parallel()
	secret := &corev1.Secret{
		Data: map[string][]byte{
			k8znerv1alpha1.CredentialsKeyHCloudToken:        []byte("hcloud-token-123"),
			k8znerv1alpha1.CredentialsKeyTalosSecrets:       []byte("talos-secrets-yaml"),
			k8znerv1alpha1.CredentialsKeyTalosConfig:        []byte("talos-config-yaml"),
			k8znerv1alpha1.CredentialsKeyCloudflareAPIToken: []byte("cf-token-456"),
		},
	}

	creds, err := extractCredentials(secret)
	require.NoError(t, err)
	assert.Equal(t, "hcloud-token-123", creds.HCloudToken)
	assert.Equal(t, []byte("talos-secrets-yaml"), creds.TalosSecrets)
	assert.Equal(t, []byte("talos-config-yaml"), creds.TalosConfig)
	assert.Equal(t, "cf-token-456", creds.CloudflareAPIToken)
}

func TestExtractCredentials_OnlyRequiredField(t *testing.T) {
	t.Parallel()
	secret := &corev1.Secret{
		Data: map[string][]byte{
			k8znerv1alpha1.CredentialsKeyHCloudToken: []byte("token"),
		},
	}

	creds, err := extractCredentials(secret)
	require.NoError(t, err)
	assert.Equal(t, "token", creds.HCloudToken)
	assert.Nil(t, creds.TalosSecrets)
	assert.Nil(t, creds.TalosConfig)
	assert.Empty(t, creds.CloudflareAPIToken)
}

func TestExtractCredentials_MissingHCloudToken(t *testing.T) {
	t.Parallel()
	secret := &corev1.Secret{
		Data: map[string][]byte{
			k8znerv1alpha1.CredentialsKeyTalosSecrets: []byte("secrets"),
		},
	}

	creds, err := extractCredentials(secret)
	require.Error(t, err)
	assert.Nil(t, creds)
	assert.Contains(t, err.Error(), k8znerv1alpha1.CredentialsKeyHCloudToken)
}

func TestExtractCredentials_EmptySecret(t *testing.T) {
	t.Parallel()
	secret := &corev1.Secret{
		Data: map[string][]byte{},
	}

	_, err := extractCredentials(secret)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing key")
}

// --- calculateBootstrapNodeIP tests ---

func TestCalculateBootstrapNodeIP_DefaultCIDR(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{},
	}

	ip, err := calculateBootstrapNodeIP(cluster)
	require.NoError(t, err)
	// Default 10.0.0.0/16, CP subnet = CIDRSubnet(10.0.0.0/16, 8, 0) = first /24 in the /16
	// Then CIDRHost(subnet, 2) = .2 offset
	assert.NotEmpty(t, ip)
	assert.Contains(t, ip, "10.0.")
}

func TestCalculateBootstrapNodeIP_CustomCIDR(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Network: k8znerv1alpha1.NetworkSpec{
				IPv4CIDR: "172.16.0.0/16",
			},
		},
	}

	ip, err := calculateBootstrapNodeIP(cluster)
	require.NoError(t, err)
	assert.Contains(t, ip, "172.16.")
}

func TestCalculateBootstrapNodeIP_InvalidCIDR(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Network: k8znerv1alpha1.NetworkSpec{
				IPv4CIDR: "not-a-cidr",
			},
		},
	}

	_, err := calculateBootstrapNodeIP(cluster)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subnet")
}

// --- populateStateFromCRD tests ---

func TestPopulateStateFromCRD_FullState(t *testing.T) {
	t.Parallel()
	state := provisioning.NewState()
	pCtx := &provisioning.Context{State: state}

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", PublicIP: "1.1.1.1", ServerID: 101},
					{Name: "cp-2", PublicIP: "2.2.2.2", ServerID: 102},
				},
			},
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", PublicIP: "3.3.3.3", ServerID: 201},
				},
			},
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				LoadBalancerIP:        "5.5.5.5",
				LoadBalancerPrivateIP: "10.0.0.100",
			},
		},
	}

	populateStateFromCRD(pCtx, cluster, &discardLogger{})

	assert.Len(t, pCtx.State.ControlPlaneIPs, 2)
	assert.Equal(t, "1.1.1.1", pCtx.State.ControlPlaneIPs["cp-1"])
	assert.Equal(t, "2.2.2.2", pCtx.State.ControlPlaneIPs["cp-2"])
	assert.Equal(t, int64(101), pCtx.State.ControlPlaneServerIDs["cp-1"])

	assert.Len(t, pCtx.State.WorkerIPs, 1)
	assert.Equal(t, "3.3.3.3", pCtx.State.WorkerIPs["w-1"])
	assert.Equal(t, int64(201), pCtx.State.WorkerServerIDs["w-1"])

	assert.Equal(t, []string{"5.5.5.5", "10.0.0.100"}, pCtx.State.SANs)
}

func TestPopulateStateFromCRD_EmptyNodes(t *testing.T) {
	t.Parallel()
	state := provisioning.NewState()
	pCtx := &provisioning.Context{State: state}

	cluster := &k8znerv1alpha1.K8znerCluster{}

	populateStateFromCRD(pCtx, cluster, &discardLogger{})

	assert.Empty(t, pCtx.State.ControlPlaneIPs)
	assert.Empty(t, pCtx.State.WorkerIPs)
	assert.Empty(t, pCtx.State.SANs)
}

func TestPopulateStateFromCRD_SkipsNodesWithEmptyFields(t *testing.T) {
	t.Parallel()
	state := provisioning.NewState()
	pCtx := &provisioning.Context{State: state}

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", PublicIP: "1.1.1.1", ServerID: 101},
					{Name: "", PublicIP: "2.2.2.2", ServerID: 102}, // empty name
					{Name: "cp-3", PublicIP: "", ServerID: 103},    // empty IP
				},
			},
		},
	}

	populateStateFromCRD(pCtx, cluster, &discardLogger{})

	assert.Len(t, pCtx.State.ControlPlaneIPs, 1, "only node with both name and IP should be included")
	assert.Equal(t, "1.1.1.1", pCtx.State.ControlPlaneIPs["cp-1"])
}

func TestPopulateStateFromCRD_OnlyPublicLBIP(t *testing.T) {
	t.Parallel()
	state := provisioning.NewState()
	pCtx := &provisioning.Context{State: state}

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				LoadBalancerIP: "5.5.5.5",
			},
		},
	}

	populateStateFromCRD(pCtx, cluster, &discardLogger{})

	assert.Equal(t, []string{"5.5.5.5"}, pCtx.State.SANs)
}

// --- updateNodeStatuses / mergeNodePool tests ---

func TestUpdateNodeStatuses_NewNodes(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{}

	state := provisioning.NewState()
	state.ControlPlaneIPs["cp-1"] = "1.1.1.1"
	state.ControlPlaneServerIDs["cp-1"] = 101
	state.WorkerIPs["w-1"] = "2.2.2.2"
	state.WorkerServerIDs["w-1"] = 201

	updateNodeStatuses(cluster, state)

	require.Len(t, cluster.Status.ControlPlanes.Nodes, 1)
	assert.Equal(t, "cp-1", cluster.Status.ControlPlanes.Nodes[0].Name)
	assert.Equal(t, "1.1.1.1", cluster.Status.ControlPlanes.Nodes[0].PublicIP)
	assert.Equal(t, int64(101), cluster.Status.ControlPlanes.Nodes[0].ServerID)

	require.Len(t, cluster.Status.Workers.Nodes, 1)
	assert.Equal(t, "w-1", cluster.Status.Workers.Nodes[0].Name)
	assert.Equal(t, "2.2.2.2", cluster.Status.Workers.Nodes[0].PublicIP)
	assert.Equal(t, int64(201), cluster.Status.Workers.Nodes[0].ServerID)
}

func TestUpdateNodeStatuses_UpdatesExistingNode(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", PublicIP: "old-ip", ServerID: 100},
				},
			},
		},
	}

	state := provisioning.NewState()
	state.ControlPlaneIPs["cp-1"] = "new-ip"
	state.ControlPlaneServerIDs["cp-1"] = 101

	updateNodeStatuses(cluster, state)

	require.Len(t, cluster.Status.ControlPlanes.Nodes, 1)
	assert.Equal(t, "new-ip", cluster.Status.ControlPlanes.Nodes[0].PublicIP)
	assert.Equal(t, int64(101), cluster.Status.ControlPlanes.Nodes[0].ServerID)
}

func TestUpdateNodeStatuses_MergesNewAndExisting(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", PublicIP: "1.1.1.1", ServerID: 101},
				},
			},
		},
	}

	state := provisioning.NewState()
	state.ControlPlaneIPs["cp-1"] = "1.1.1.1" // same
	state.ControlPlaneServerIDs["cp-1"] = 101
	state.ControlPlaneIPs["cp-2"] = "2.2.2.2" // new
	state.ControlPlaneServerIDs["cp-2"] = 102

	updateNodeStatuses(cluster, state)

	assert.Len(t, cluster.Status.ControlPlanes.Nodes, 2)
}

func TestMergeNodePool_EmptyInputs(t *testing.T) {
	t.Parallel()
	var nodes []k8znerv1alpha1.NodeStatus
	ips := map[string]string{}
	serverIDs := map[string]int64{}

	mergeNodePool(&nodes, ips, serverIDs)

	assert.Empty(t, nodes)
}

// --- setCondition tests ---

func TestSetCondition_NewCondition(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{}

	setCondition(cluster, k8znerv1alpha1.ConditionInfrastructureReady, metav1.ConditionTrue,
		"Provisioned", "Infrastructure ready")

	require.Len(t, cluster.Status.Conditions, 1)
	assert.Equal(t, k8znerv1alpha1.ConditionInfrastructureReady, cluster.Status.Conditions[0].Type)
	assert.Equal(t, metav1.ConditionTrue, cluster.Status.Conditions[0].Status)
	assert.Equal(t, "Provisioned", cluster.Status.Conditions[0].Reason)
	assert.Equal(t, "Infrastructure ready", cluster.Status.Conditions[0].Message)
}

func TestSetCondition_UpdatesExistingCondition(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Conditions: []metav1.Condition{
				{
					Type:   k8znerv1alpha1.ConditionInfrastructureReady,
					Status: metav1.ConditionFalse,
					Reason: "Pending",
				},
			},
		},
	}

	setCondition(cluster, k8znerv1alpha1.ConditionInfrastructureReady, metav1.ConditionTrue,
		"Provisioned", "Infrastructure ready")

	require.Len(t, cluster.Status.Conditions, 1, "should update in place, not append")
	assert.Equal(t, metav1.ConditionTrue, cluster.Status.Conditions[0].Status)
	assert.Equal(t, "Provisioned", cluster.Status.Conditions[0].Reason)
}

func TestSetCondition_NoopIfSameStatus(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Conditions: []metav1.Condition{
				{
					Type:   k8znerv1alpha1.ConditionBootstrapped,
					Status: metav1.ConditionTrue,
					Reason: "Original",
				},
			},
		},
	}

	setCondition(cluster, k8znerv1alpha1.ConditionBootstrapped, metav1.ConditionTrue,
		"NewReason", "New message")

	// Should NOT update because status is the same
	assert.Equal(t, "Original", cluster.Status.Conditions[0].Reason)
}

func TestSetCondition_MultipleConditions(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{}

	setCondition(cluster, k8znerv1alpha1.ConditionInfrastructureReady, metav1.ConditionTrue, "Ready", "infra ok")
	setCondition(cluster, k8znerv1alpha1.ConditionImageReady, metav1.ConditionTrue, "Ready", "image ok")
	setCondition(cluster, k8znerv1alpha1.ConditionBootstrapped, metav1.ConditionFalse, "Pending", "not yet")

	assert.Len(t, cluster.Status.Conditions, 3)
}

// --- populateBootstrapState additional tests ---

func TestPopulateBootstrapState_EmptyBootstrapNode(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "cp", Count: 1, ServerType: "cx23", Location: "fsn1"},
			},
		},
	}
	pCtx := &provisioning.Context{
		Config: cfg,
		State:  provisioning.NewState(),
	}

	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Bootstrap: &k8znerv1alpha1.BootstrapState{
				Completed:     true,
				BootstrapNode: "", // empty node name
			},
		},
	}

	populateBootstrapState(pCtx, cluster, &discardLogger{})

	// Should not add empty-key entries to state
	assert.Empty(t, pCtx.State.ControlPlaneIPs)
}

func TestPopulateBootstrapState_CountsAlreadyAtLimit(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "cp", Count: 1, ServerType: "cx23", Location: "fsn1"},
			},
		},
		Workers: []config.WorkerNodePool{}, // no workers
	}
	pCtx := &provisioning.Context{
		Config: cfg,
		State:  provisioning.NewState(),
	}

	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Bootstrap: &k8znerv1alpha1.BootstrapState{
				Completed:     true,
				BootstrapNode: "cp-1",
				PublicIP:      "1.2.3.4",
			},
		},
	}

	populateBootstrapState(pCtx, cluster, &discardLogger{})

	// Count already 1, should remain 1 (no limiting needed)
	assert.Equal(t, 1, pCtx.Config.ControlPlane.NodePools[0].Count)
}

// --- LoadCredentials tests ---

func newFakeScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = k8znerv1alpha1.AddToScheme(scheme)
	return scheme
}

func TestLoadCredentials_Success(t *testing.T) {
	t.Parallel()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-creds", Namespace: "default"},
		Data: map[string][]byte{
			k8znerv1alpha1.CredentialsKeyHCloudToken:        []byte("hcloud-token"),
			k8znerv1alpha1.CredentialsKeyTalosSecrets:       []byte("talos-secrets"),
			k8znerv1alpha1.CredentialsKeyTalosConfig:        []byte("talos-config"),
			k8znerv1alpha1.CredentialsKeyCloudflareAPIToken: []byte("cf-token"),
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(newFakeScheme()).WithObjects(secret).Build()
	adapter := &PhaseAdapter{client: fakeClient}

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			CredentialsRef: corev1.LocalObjectReference{Name: "my-creds"},
		},
	}

	creds, err := adapter.LoadCredentials(context.Background(), cluster)
	require.NoError(t, err)
	assert.Equal(t, "hcloud-token", creds.HCloudToken)
	assert.Equal(t, []byte("talos-secrets"), creds.TalosSecrets)
	assert.Equal(t, []byte("talos-config"), creds.TalosConfig)
	assert.Equal(t, "cf-token", creds.CloudflareAPIToken)
}

func TestLoadCredentials_EmptyCredentialsRef(t *testing.T) {
	t.Parallel()
	fakeClient := fake.NewClientBuilder().WithScheme(newFakeScheme()).Build()
	adapter := &PhaseAdapter{client: fakeClient}

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			CredentialsRef: corev1.LocalObjectReference{Name: ""}, // empty
		},
	}

	creds, err := adapter.LoadCredentials(context.Background(), cluster)
	require.Error(t, err)
	assert.Nil(t, creds)
	assert.Contains(t, err.Error(), "credentialsRef.name is not set")
}

func TestLoadCredentials_SecretNotFound(t *testing.T) {
	t.Parallel()
	fakeClient := fake.NewClientBuilder().WithScheme(newFakeScheme()).Build()
	adapter := &PhaseAdapter{client: fakeClient}

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			CredentialsRef: corev1.LocalObjectReference{Name: "nonexistent"},
		},
	}

	creds, err := adapter.LoadCredentials(context.Background(), cluster)
	require.Error(t, err)
	assert.Nil(t, creds)
	assert.Contains(t, err.Error(), "failed to get credentials secret")
}

func TestLoadCredentials_MissingRequiredField(t *testing.T) {
	t.Parallel()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "bad-creds", Namespace: "default"},
		Data:       map[string][]byte{}, // Missing hcloud-token
	}

	fakeClient := fake.NewClientBuilder().WithScheme(newFakeScheme()).WithObjects(secret).Build()
	adapter := &PhaseAdapter{client: fakeClient}

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			CredentialsRef: corev1.LocalObjectReference{Name: "bad-creds"},
		},
	}

	creds, err := adapter.LoadCredentials(context.Background(), cluster)
	require.Error(t, err)
	assert.Nil(t, creds)
	assert.Contains(t, err.Error(), k8znerv1alpha1.CredentialsKeyHCloudToken)
}

func TestLoadCredentials_WithBackupCredentials(t *testing.T) {
	t.Parallel()
	mainSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "main-creds", Namespace: "default"},
		Data: map[string][]byte{
			k8znerv1alpha1.CredentialsKeyHCloudToken: []byte("hcloud-token"),
		},
	}
	backupSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "backup-creds", Namespace: "default"},
		Data: map[string][]byte{
			"access-key": []byte("s3-ak"),
			"secret-key": []byte("s3-sk"),
			"endpoint":   []byte("s3.example.com"),
			"bucket":     []byte("my-bucket"),
			"region":     []byte("eu-central-1"),
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(newFakeScheme()).WithObjects(mainSecret, backupSecret).Build()
	adapter := &PhaseAdapter{client: fakeClient}

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			CredentialsRef: corev1.LocalObjectReference{Name: "main-creds"},
			Backup: &k8znerv1alpha1.BackupSpec{
				Enabled:     true,
				S3SecretRef: &k8znerv1alpha1.SecretReference{Name: "backup-creds"},
			},
		},
	}

	creds, err := adapter.LoadCredentials(context.Background(), cluster)
	require.NoError(t, err)
	assert.Equal(t, "hcloud-token", creds.HCloudToken)
	assert.Equal(t, "s3-ak", creds.BackupS3AccessKey)
	assert.Equal(t, "s3-sk", creds.BackupS3SecretKey)
	assert.Equal(t, "s3.example.com", creds.BackupS3Endpoint)
	assert.Equal(t, "my-bucket", creds.BackupS3Bucket)
	assert.Equal(t, "eu-central-1", creds.BackupS3Region)
}

func TestLoadCredentials_BackupSecretNotFound_NoError(t *testing.T) {
	t.Parallel()
	// Missing backup secret should log warning but not fail
	mainSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "main-creds", Namespace: "default"},
		Data: map[string][]byte{
			k8znerv1alpha1.CredentialsKeyHCloudToken: []byte("token"),
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(newFakeScheme()).WithObjects(mainSecret).Build()
	adapter := &PhaseAdapter{client: fakeClient}

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			CredentialsRef: corev1.LocalObjectReference{Name: "main-creds"},
			Backup: &k8znerv1alpha1.BackupSpec{
				Enabled:     true,
				S3SecretRef: &k8znerv1alpha1.SecretReference{Name: "nonexistent-backup"},
			},
		},
	}

	// Should succeed even though backup secret doesn't exist
	creds, err := adapter.LoadCredentials(context.Background(), cluster)
	require.NoError(t, err)
	assert.Equal(t, "token", creds.HCloudToken)
	assert.Empty(t, creds.BackupS3AccessKey) // Backup creds not loaded
}

// --- loadBackupCredentials tests ---

func TestLoadBackupCredentials_NoBackupSpec(t *testing.T) {
	t.Parallel()
	adapter := &PhaseAdapter{client: fake.NewClientBuilder().WithScheme(newFakeScheme()).Build()}
	creds := &Credentials{}

	err := adapter.loadBackupCredentials(context.Background(),
		&k8znerv1alpha1.K8znerCluster{}, creds, &discardLogger{})
	require.NoError(t, err)
	assert.Empty(t, creds.BackupS3AccessKey)
}

func TestLoadBackupCredentials_NilS3SecretRef(t *testing.T) {
	t.Parallel()
	adapter := &PhaseAdapter{client: fake.NewClientBuilder().WithScheme(newFakeScheme()).Build()}
	creds := &Credentials{}

	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Backup: &k8znerv1alpha1.BackupSpec{Enabled: true, S3SecretRef: nil},
		},
	}

	err := adapter.loadBackupCredentials(context.Background(), cluster, creds, &discardLogger{})
	require.NoError(t, err)
}

func TestLoadBackupCredentials_EmptyS3SecretRefName(t *testing.T) {
	t.Parallel()
	adapter := &PhaseAdapter{client: fake.NewClientBuilder().WithScheme(newFakeScheme()).Build()}
	creds := &Credentials{}

	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Backup: &k8znerv1alpha1.BackupSpec{
				Enabled:     true,
				S3SecretRef: &k8znerv1alpha1.SecretReference{Name: ""},
			},
		},
	}

	err := adapter.loadBackupCredentials(context.Background(), cluster, creds, &discardLogger{})
	require.NoError(t, err)
}

func TestLoadBackupCredentials_SecretNotFound(t *testing.T) {
	t.Parallel()
	adapter := &PhaseAdapter{client: fake.NewClientBuilder().WithScheme(newFakeScheme()).Build()}
	creds := &Credentials{}

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Backup: &k8znerv1alpha1.BackupSpec{
				Enabled:     true,
				S3SecretRef: &k8znerv1alpha1.SecretReference{Name: "missing-secret"},
			},
		},
	}

	err := adapter.loadBackupCredentials(context.Background(), cluster, creds, &discardLogger{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get backup S3 credentials")
}

// --- BuildProvisioningContext tests ---

func TestBuildProvisioningContext_Success(t *testing.T) {
	t.Parallel()
	cluster := newTestCluster("test-cluster", "", nil)
	creds := baseCreds()

	adapter := &PhaseAdapter{}
	pCtx, err := adapter.BuildProvisioningContext(context.Background(), cluster, creds, nil, nil)
	require.NoError(t, err)
	assert.NotNil(t, pCtx)
	assert.Equal(t, "test-cluster", pCtx.Config.ClusterName)
	assert.NotNil(t, pCtx.State)
	assert.NotNil(t, pCtx.Observer)
}

func TestBuildProvisioningContext_PassesInfraAndTalos(t *testing.T) {
	t.Parallel()
	cluster := newTestCluster("test-cluster", "", nil)
	creds := baseCreds()

	adapter := &PhaseAdapter{}
	// Pass nil infra/talos to verify they are propagated
	pCtx, err := adapter.BuildProvisioningContext(context.Background(), cluster, creds, nil, nil)
	require.NoError(t, err)
	assert.Nil(t, pCtx.Infra)
	assert.Nil(t, pCtx.Talos)
	assert.NotNil(t, pCtx.Timeouts)
}

func TestBuildProvisioningContext_NilCredentials(t *testing.T) {
	t.Parallel()
	cluster := newTestCluster("test-cluster", "", nil)
	adapter := &PhaseAdapter{}

	// Nil credentials should cause SpecToConfig to use empty token
	pCtx, err := adapter.BuildProvisioningContext(context.Background(), cluster, &Credentials{}, nil, nil)
	require.NoError(t, err)
	assert.NotNil(t, pCtx)
	assert.Empty(t, pCtx.Config.HCloudToken)
}

func TestBuildProvisioningContext_LoggerSetToObserver(t *testing.T) {
	t.Parallel()
	cluster := newTestCluster("test-cluster", "", nil)
	creds := baseCreds()

	adapter := &PhaseAdapter{}
	pCtx, err := adapter.BuildProvisioningContext(context.Background(), cluster, creds, nil, nil)
	require.NoError(t, err)
	// Logger should be set to the same Observer
	assert.NotNil(t, pCtx.Logger)
	assert.Equal(t, pCtx.Observer, pCtx.Logger)
}

// --- NewPhaseAdapter test ---

func TestNewPhaseAdapter(t *testing.T) {
	t.Parallel()
	fakeClient := fake.NewClientBuilder().WithScheme(newFakeScheme()).Build()
	adapter := NewPhaseAdapter(fakeClient)

	require.NotNil(t, adapter)
	assert.NotNil(t, adapter.client)
	assert.NotNil(t, adapter.infraProvisioner)
	assert.NotNil(t, adapter.imageProvisioner)
	assert.NotNil(t, adapter.computeProvisioner)
	assert.NotNil(t, adapter.clusterProvisioner)
}

func TestNewPhaseAdapter_NilClient(t *testing.T) {
	t.Parallel()
	adapter := NewPhaseAdapter(nil)

	require.NotNil(t, adapter)
	assert.Nil(t, adapter.client)
	assert.NotNil(t, adapter.infraProvisioner)
	assert.NotNil(t, adapter.imageProvisioner)
	assert.NotNil(t, adapter.computeProvisioner)
	assert.NotNil(t, adapter.clusterProvisioner)
}

// --- AttachBootstrapNodeToInfrastructure tests ---

// logCtx creates a context with a discard logger for tests that use log.FromContext.
func logCtx() context.Context {
	return log.IntoContext(context.Background(), logr.Discard())
}

func TestAttachBootstrapNodeToInfrastructure_NilBootstrap(t *testing.T) {
	t.Parallel()
	adapter := &PhaseAdapter{}
	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Bootstrap: nil,
		},
	}
	pCtx := &provisioning.Context{
		Context: logCtx(),
		State:   provisioning.NewState(),
	}

	err := adapter.AttachBootstrapNodeToInfrastructure(pCtx, cluster)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bootstrap state not available")
}

func TestAttachBootstrapNodeToInfrastructure_NotCompleted(t *testing.T) {
	t.Parallel()
	adapter := &PhaseAdapter{}
	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Bootstrap: &k8znerv1alpha1.BootstrapState{
				Completed: false,
			},
		},
	}
	pCtx := &provisioning.Context{
		Context: logCtx(),
		State:   provisioning.NewState(),
	}

	err := adapter.AttachBootstrapNodeToInfrastructure(pCtx, cluster)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bootstrap state not available")
}

func TestAttachBootstrapNodeToInfrastructure_EmptyBootstrapNode(t *testing.T) {
	t.Parallel()
	adapter := &PhaseAdapter{}
	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Bootstrap: &k8znerv1alpha1.BootstrapState{
				Completed:     true,
				BootstrapNode: "",
			},
		},
	}
	pCtx := &provisioning.Context{
		Context: logCtx(),
		State:   provisioning.NewState(),
	}

	err := adapter.AttachBootstrapNodeToInfrastructure(pCtx, cluster)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bootstrap node name is not set")
}

func TestAttachBootstrapNodeToInfrastructure_NoNetworkID(t *testing.T) {
	t.Parallel()
	adapter := &PhaseAdapter{}
	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Bootstrap: &k8znerv1alpha1.BootstrapState{
				Completed:       true,
				BootstrapNode:   "cp-1",
				BootstrapNodeID: 100,
			},
		},
	}
	state := provisioning.NewState()
	state.Network = &hcloud.Network{ID: 0} // Network exists but ID is 0
	pCtx := &provisioning.Context{
		Context: logCtx(),
		State:   state,
	}
	// No networkID in state or CRD status

	err := adapter.AttachBootstrapNodeToInfrastructure(pCtx, cluster)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network ID not available")
}

func TestAttachBootstrapNodeToInfrastructure_FallbackToStatusNetworkID(t *testing.T) {
	t.Parallel()
	adapter := &PhaseAdapter{}

	// Set networkID via CRD status (fallback), but no infra manager to get server
	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Bootstrap: &k8znerv1alpha1.BootstrapState{
				Completed:       true,
				BootstrapNode:   "cp-1",
				BootstrapNodeID: 100,
			},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				NetworkID: 42,
			},
		},
	}
	state := provisioning.NewState()
	state.Network = &hcloud.Network{ID: 0} // Network with ID 0, will fallback to CRD status
	pCtx := &provisioning.Context{
		Context: logCtx(),
		State:   state,
		Infra:   nil, // Will panic when GetServerByName is called
	}

	// This will fail because Infra is nil (panic on GetServerByName),
	// which proves we passed the networkID check (fallback to CRD status worked).
	assert.Panics(t, func() {
		_ = adapter.AttachBootstrapNodeToInfrastructure(pCtx, cluster)
	}, "should panic because Infra is nil - proves we passed the networkID check")
}

func TestAttachBootstrapNodeToInfrastructure_NetworkIDFromState(t *testing.T) {
	t.Parallel()
	adapter := &PhaseAdapter{}

	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Bootstrap: &k8znerv1alpha1.BootstrapState{
				Completed:       true,
				BootstrapNode:   "cp-1",
				BootstrapNodeID: 100,
			},
		},
	}
	state := provisioning.NewState()
	state.Network = &hcloud.Network{ID: 99} // Valid network ID in state
	pCtx := &provisioning.Context{
		Context: logCtx(),
		State:   state,
		Infra:   nil, // Will panic when GetServerByName is called
	}

	// Should get past networkID check (uses state.Network.ID=99) and
	// then panic on nil Infra when calling GetServerByName.
	assert.Panics(t, func() {
		_ = adapter.AttachBootstrapNodeToInfrastructure(pCtx, cluster)
	}, "should panic because Infra is nil - proves we passed the networkID check with state network")
}

// --- populateBootstrapState with multiple node pools ---

func TestPopulateBootstrapState_MultipleNodePools(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "cp-pool-1", Count: 3, ServerType: "cx23", Location: "fsn1"},
				{Name: "cp-pool-2", Count: 5, ServerType: "cx33", Location: "nbg1"},
			},
		},
		Workers: []config.WorkerNodePool{
			{Name: "workers-1", Count: 2, ServerType: "cx23", Location: "fsn1"},
			{Name: "workers-2", Count: 4, ServerType: "cx33", Location: "nbg1"},
		},
	}

	// Keep references to original slices
	origCPPools := cfg.ControlPlane.NodePools
	origWorkers := cfg.Workers

	pCtx := &provisioning.Context{
		Config: cfg,
		State:  provisioning.NewState(),
	}

	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Bootstrap: &k8znerv1alpha1.BootstrapState{
				Completed:       true,
				BootstrapNode:   "cp-abc",
				BootstrapNodeID: 111,
				PublicIP:        "5.5.5.5",
			},
		},
	}

	populateBootstrapState(pCtx, cluster, &discardLogger{})

	// Both CP pools should be limited to 1
	assert.Equal(t, 1, pCtx.Config.ControlPlane.NodePools[0].Count)
	assert.Equal(t, 1, pCtx.Config.ControlPlane.NodePools[1].Count)

	// Both worker pools should be limited to 0
	assert.Equal(t, 0, pCtx.Config.Workers[0].Count)
	assert.Equal(t, 0, pCtx.Config.Workers[1].Count)

	// Originals should not be mutated
	assert.Equal(t, 3, origCPPools[0].Count)
	assert.Equal(t, 5, origCPPools[1].Count)
	assert.Equal(t, 2, origWorkers[0].Count)
	assert.Equal(t, 4, origWorkers[1].Count)

	// Bootstrap node state
	assert.Equal(t, "5.5.5.5", pCtx.State.ControlPlaneIPs["cp-abc"])
	assert.Equal(t, int64(111), pCtx.State.ControlPlaneServerIDs["cp-abc"])
}

// --- populateStateFromCRD with only private LB IP ---

func TestPopulateStateFromCRD_OnlyPrivateLBIP(t *testing.T) {
	t.Parallel()
	state := provisioning.NewState()
	pCtx := &provisioning.Context{State: state}

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				LoadBalancerPrivateIP: "10.0.0.100",
			},
		},
	}

	populateStateFromCRD(pCtx, cluster, &discardLogger{})

	assert.Equal(t, []string{"10.0.0.100"}, pCtx.State.SANs)
}

func TestPopulateStateFromCRD_NoLBIPs(t *testing.T) {
	t.Parallel()
	state := provisioning.NewState()
	pCtx := &provisioning.Context{State: state}

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", PublicIP: "1.1.1.1", ServerID: 101},
				},
			},
		},
	}

	populateStateFromCRD(pCtx, cluster, &discardLogger{})

	// SANs should be empty (nil) when no LB IPs
	assert.Empty(t, pCtx.State.SANs)
}

// --- mergeNodePool edge cases ---

func TestMergeNodePool_NilNodes(t *testing.T) {
	t.Parallel()
	var nodes []k8znerv1alpha1.NodeStatus
	ips := map[string]string{"n1": "1.1.1.1"}
	serverIDs := map[string]int64{"n1": 1}

	mergeNodePool(&nodes, ips, serverIDs)

	require.Len(t, nodes, 1)
	assert.Equal(t, "n1", nodes[0].Name)
	assert.Equal(t, "1.1.1.1", nodes[0].PublicIP)
	assert.Equal(t, int64(1), nodes[0].ServerID)
}

func TestMergeNodePool_ServerIDNotInMap(t *testing.T) {
	t.Parallel()
	var nodes []k8znerv1alpha1.NodeStatus
	ips := map[string]string{"n1": "1.1.1.1"}
	serverIDs := map[string]int64{} // n1 not in serverIDs

	mergeNodePool(&nodes, ips, serverIDs)

	require.Len(t, nodes, 1)
	assert.Equal(t, "n1", nodes[0].Name)
	assert.Equal(t, "1.1.1.1", nodes[0].PublicIP)
	assert.Equal(t, int64(0), nodes[0].ServerID, "should be zero when not in serverIDs map")
}

// --- extractCredentials edge cases ---

func TestExtractCredentials_NilData(t *testing.T) {
	t.Parallel()
	secret := &corev1.Secret{
		Data: nil,
	}

	_, err := extractCredentials(secret)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing key")
}

func TestExtractCredentials_OnlyOptionalFields(t *testing.T) {
	t.Parallel()
	secret := &corev1.Secret{
		Data: map[string][]byte{
			k8znerv1alpha1.CredentialsKeyHCloudToken:  []byte("token"),
			k8znerv1alpha1.CredentialsKeyTalosSecrets: []byte("secrets-data"),
		},
	}

	creds, err := extractCredentials(secret)
	require.NoError(t, err)
	assert.Equal(t, "token", creds.HCloudToken)
	assert.Equal(t, []byte("secrets-data"), creds.TalosSecrets)
	assert.Nil(t, creds.TalosConfig, "TalosConfig should be nil when not present")
	assert.Empty(t, creds.CloudflareAPIToken, "CloudflareAPIToken should be empty when not present")
}

// --- setCondition edge cases ---

func TestSetCondition_TransitionFromUnknown(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Conditions: []metav1.Condition{
				{
					Type:   k8znerv1alpha1.ConditionInfrastructureReady,
					Status: metav1.ConditionUnknown,
					Reason: "Unknown",
				},
			},
		},
	}

	setCondition(cluster, k8znerv1alpha1.ConditionInfrastructureReady, metav1.ConditionTrue,
		"Provisioned", "Done")

	require.Len(t, cluster.Status.Conditions, 1)
	assert.Equal(t, metav1.ConditionTrue, cluster.Status.Conditions[0].Status)
	assert.Equal(t, "Provisioned", cluster.Status.Conditions[0].Reason)
}

func TestSetCondition_LastTransitionTimeSet(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{}

	setCondition(cluster, "TestCondition", metav1.ConditionTrue, "Ready", "all good")

	require.Len(t, cluster.Status.Conditions, 1)
	assert.False(t, cluster.Status.Conditions[0].LastTransitionTime.IsZero())
}

// --- calculateBootstrapNodeIP additional tests ---

func TestCalculateBootstrapNodeIP_ExplicitDefault(t *testing.T) {
	t.Parallel()
	// Explicitly set the same value as the default
	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Network: k8znerv1alpha1.NetworkSpec{
				IPv4CIDR: "10.0.0.0/16",
			},
		},
	}

	ipExplicit, err := calculateBootstrapNodeIP(cluster)
	require.NoError(t, err)

	// Compare with default behavior
	clusterDefault := &k8znerv1alpha1.K8znerCluster{}
	ipDefault, err := calculateBootstrapNodeIP(clusterDefault)
	require.NoError(t, err)

	assert.Equal(t, ipDefault, ipExplicit, "explicit default should match implicit default")
}

func TestCalculateBootstrapNodeIP_SmallCIDR(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Network: k8znerv1alpha1.NetworkSpec{
				IPv4CIDR: "192.168.0.0/16",
			},
		},
	}

	ip, err := calculateBootstrapNodeIP(cluster)
	require.NoError(t, err)
	assert.Contains(t, ip, "192.168.")
}
