package provisioning

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
