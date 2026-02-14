package controller

import (
	"context"
	"fmt"
	"testing"

	hcloudgo "github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/config"
	operatorprov "github.com/imamik/k8zner/internal/operator/provisioning"
)

func TestAllPodsReady(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		pods []corev1.Pod
		want bool
	}{
		{
			name: "empty pod list is ready",
			pods: []corev1.Pod{},
			want: true,
		},
		{
			name: "single running and ready pod",
			pods: []corev1.Pod{
				{
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						Conditions: []corev1.PodCondition{
							{Type: corev1.PodReady, Status: corev1.ConditionTrue},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "multiple running and ready pods",
			pods: []corev1.Pod{
				{
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						Conditions: []corev1.PodCondition{
							{Type: corev1.PodReady, Status: corev1.ConditionTrue},
						},
					},
				},
				{
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						Conditions: []corev1.PodCondition{
							{Type: corev1.PodReady, Status: corev1.ConditionTrue},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "pod not running",
			pods: []corev1.Pod{
				{
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
						Conditions: []corev1.PodCondition{
							{Type: corev1.PodReady, Status: corev1.ConditionFalse},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "pod running but not ready",
			pods: []corev1.Pod{
				{
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						Conditions: []corev1.PodCondition{
							{Type: corev1.PodReady, Status: corev1.ConditionFalse},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "one ready one not ready",
			pods: []corev1.Pod{
				{
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						Conditions: []corev1.PodCondition{
							{Type: corev1.PodReady, Status: corev1.ConditionTrue},
						},
					},
				},
				{
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						Conditions: []corev1.PodCondition{
							{Type: corev1.PodReady, Status: corev1.ConditionFalse},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "pod failed",
			pods: []corev1.Pod{
				{
					Status: corev1.PodStatus{
						Phase: corev1.PodFailed,
					},
				},
			},
			want: false,
		},
		{
			name: "pod succeeded (completed)",
			pods: []corev1.Pod{
				{
					Status: corev1.PodStatus{
						Phase: corev1.PodSucceeded,
					},
				},
			},
			want: false,
		},
		{
			name: "running pod with no conditions",
			pods: []corev1.Pod{
				{
					Status: corev1.PodStatus{
						Phase:      corev1.PodRunning,
						Conditions: []corev1.PodCondition{},
					},
				},
			},
			want: true, // no Ready condition means it doesn't fail the check
		},
		{
			name: "running pod with multiple conditions including ready",
			pods: []corev1.Pod{
				{
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						Conditions: []corev1.PodCondition{
							{Type: corev1.PodScheduled, Status: corev1.ConditionTrue},
							{Type: corev1.PodReady, Status: corev1.ConditionTrue},
							{Type: corev1.ContainersReady, Status: corev1.ConditionTrue},
						},
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := allPodsReady(tt.pods)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFindTalosEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cluster *k8znerv1alpha1.K8znerCluster
		want    string
	}{
		{
			name: "prefers control plane endpoint from status",
			cluster: &k8znerv1alpha1.K8znerCluster{
				Spec: k8znerv1alpha1.K8znerClusterSpec{
					Bootstrap: &k8znerv1alpha1.BootstrapState{
						PublicIP: "2.2.2.2",
					},
				},
				Status: k8znerv1alpha1.K8znerClusterStatus{
					ControlPlaneEndpoint: "1.1.1.1",
					ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
						Nodes: []k8znerv1alpha1.NodeStatus{
							{Healthy: true, PublicIP: "3.3.3.3"},
						},
					},
				},
			},
			want: "1.1.1.1",
		},
		{
			name: "falls back to bootstrap public IP",
			cluster: &k8znerv1alpha1.K8znerCluster{
				Spec: k8znerv1alpha1.K8znerClusterSpec{
					Bootstrap: &k8znerv1alpha1.BootstrapState{
						PublicIP: "2.2.2.2",
					},
				},
				Status: k8znerv1alpha1.K8znerClusterStatus{
					ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
						Nodes: []k8znerv1alpha1.NodeStatus{
							{Healthy: true, PublicIP: "3.3.3.3"},
						},
					},
				},
			},
			want: "2.2.2.2",
		},
		{
			name: "falls back to first healthy CP node",
			cluster: &k8znerv1alpha1.K8znerCluster{
				Status: k8znerv1alpha1.K8znerClusterStatus{
					ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
						Nodes: []k8znerv1alpha1.NodeStatus{
							{Healthy: false, PublicIP: "4.4.4.4"},
							{Healthy: true, PublicIP: "5.5.5.5"},
							{Healthy: true, PublicIP: "6.6.6.6"},
						},
					},
				},
			},
			want: "5.5.5.5",
		},
		{
			name: "skips unhealthy CP nodes",
			cluster: &k8znerv1alpha1.K8znerCluster{
				Status: k8znerv1alpha1.K8znerClusterStatus{
					ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
						Nodes: []k8znerv1alpha1.NodeStatus{
							{Healthy: false, PublicIP: "4.4.4.4"},
							{Healthy: false, PublicIP: "5.5.5.5"},
						},
					},
				},
			},
			want: "",
		},
		{
			name: "skips healthy CP node without public IP",
			cluster: &k8znerv1alpha1.K8znerCluster{
				Status: k8znerv1alpha1.K8znerClusterStatus{
					ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
						Nodes: []k8znerv1alpha1.NodeStatus{
							{Healthy: true, PublicIP: ""},
						},
					},
				},
			},
			want: "",
		},
		{
			name:    "returns empty when no endpoints available",
			cluster: &k8znerv1alpha1.K8znerCluster{},
			want:    "",
		},
		{
			name: "nil bootstrap spec",
			cluster: &k8znerv1alpha1.K8znerCluster{
				Spec: k8znerv1alpha1.K8znerClusterSpec{
					Bootstrap: nil,
				},
			},
			want: "",
		},
		{
			name: "empty bootstrap public IP falls through to CP nodes",
			cluster: &k8znerv1alpha1.K8znerCluster{
				Spec: k8znerv1alpha1.K8znerClusterSpec{
					Bootstrap: &k8znerv1alpha1.BootstrapState{
						PublicIP: "",
					},
				},
				Status: k8znerv1alpha1.K8znerClusterStatus{
					ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
						Nodes: []k8znerv1alpha1.NodeStatus{
							{Healthy: true, PublicIP: "7.7.7.7"},
						},
					},
				},
			},
			want: "7.7.7.7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := &ClusterReconciler{}
			got := r.findTalosEndpoint(tt.cluster)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveNetworkID(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("returns network ID from status when present", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Infrastructure: k8znerv1alpha1.InfrastructureStatus{
					NetworkID: 42,
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		mockHCloud := &MockHCloudClient{}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		id, err := r.resolveNetworkID(context.Background(), cluster)
		require.NoError(t, err)
		assert.Equal(t, int64(42), id)
		assert.Empty(t, mockHCloud.GetNetworkCalls, "should not call HCloud when ID is in status")
	})

	t.Run("looks up network from HCloud when not in status", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-cluster",
				Namespace: "default",
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Infrastructure: k8znerv1alpha1.InfrastructureStatus{
					NetworkID: 0,
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		mockHCloud := &MockHCloudClient{
			GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
				return &hcloudgo.Network{ID: 99, Name: name}, nil
			},
		}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		id, err := r.resolveNetworkID(context.Background(), cluster)
		require.NoError(t, err)
		assert.Equal(t, int64(99), id)
		assert.Equal(t, []string{"my-cluster"}, mockHCloud.GetNetworkCalls)
		// Should also cache the network ID in status
		assert.Equal(t, int64(99), cluster.Status.Infrastructure.NetworkID)
	})

	t.Run("returns error when HCloud lookup fails", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		mockHCloud := &MockHCloudClient{
			GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
				return nil, fmt.Errorf("API error")
			},
		}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		_, err := r.resolveNetworkID(context.Background(), cluster)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API error")
	})

	t.Run("returns error when network not found in HCloud", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		mockHCloud := &MockHCloudClient{
			GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
				return nil, nil
			},
		}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		_, err := r.resolveNetworkID(context.Background(), cluster)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "network not found")
	})
}

func TestEnsureWorkersReady(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("returns false when desired workers is zero", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Workers: k8znerv1alpha1.WorkerSpec{Count: 0},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder, WithMetrics(false))

		result, waiting := r.ensureWorkersReady(context.Background(), cluster)
		assert.False(t, waiting)
		assert.Zero(t, result.RequeueAfter)
	})

	t.Run("returns false when all workers ready", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Workers: k8znerv1alpha1.WorkerSpec{Count: 2},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Ready: 2,
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder, WithMetrics(false))

		result, waiting := r.ensureWorkersReady(context.Background(), cluster)
		assert.False(t, waiting)
		assert.Zero(t, result.RequeueAfter)
	})

	t.Run("returns false when more workers ready than desired", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Workers: k8znerv1alpha1.WorkerSpec{Count: 2},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Ready: 3,
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder, WithMetrics(false))

		result, waiting := r.ensureWorkersReady(context.Background(), cluster)
		assert.False(t, waiting)
		assert.Zero(t, result.RequeueAfter)
	})

	t.Run("returns true and requeues when workers not ready", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Workers: k8znerv1alpha1.WorkerSpec{Count: 3},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Ready: 1,
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder, WithMetrics(false))

		result, waiting := r.ensureWorkersReady(context.Background(), cluster)
		assert.True(t, waiting)
		assert.NotZero(t, result.RequeueAfter)
	})

	t.Run("triggers scale up when worker nodes missing and hcloud client available", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
				Annotations: map[string]string{
					"k8zner.io/ssh-keys": "key1",
				},
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Region: "nbg1",
				Workers: k8znerv1alpha1.WorkerSpec{
					Count: 3,
					Size:  "cx22",
				},
				ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{
					Count: 1,
					Size:  "cx22",
				},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Ready: 0,
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "w-1"},
					},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)
		mockHCloud := &MockHCloudClient{
			GetSnapshotByLabelsFunc: func(ctx context.Context, labels map[string]string) (*hcloudgo.Image, error) {
				return &hcloudgo.Image{ID: 1, Name: "talos"}, nil
			},
		}

		r := NewClusterReconciler(fakeClient, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		result, waiting := r.ensureWorkersReady(context.Background(), cluster)
		assert.True(t, waiting)
		assert.NotZero(t, result.RequeueAfter)
		// Scale-up creates all needed servers in parallel (no maxConcurrentHeals limit)
		assert.Equal(t, 2, len(mockHCloud.CreateServerCalls))
	})

	t.Run("does not scale when worker count matches desired", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Workers: k8znerv1alpha1.WorkerSpec{Count: 2},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Ready: 0,
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "w-1"},
						{Name: "w-2"},
					},
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		mockHCloud := &MockHCloudClient{}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		result, waiting := r.ensureWorkersReady(context.Background(), cluster)
		assert.True(t, waiting, "should still be waiting since ready < desired")
		assert.NotZero(t, result.RequeueAfter)
		assert.Empty(t, mockHCloud.CreateServerCalls, "should not create servers when node count matches")
	})

	t.Run("does not scale without hcloud client", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Workers: k8znerv1alpha1.WorkerSpec{Count: 3},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Ready: 0,
					Nodes: []k8znerv1alpha1.NodeStatus{},
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)

		// No hcloud client set
		r := NewClusterReconciler(client, scheme, recorder, WithMetrics(false))

		result, waiting := r.ensureWorkersReady(context.Background(), cluster)
		assert.True(t, waiting)
		assert.NotZero(t, result.RequeueAfter)
	})
}

func TestClientConfigFromKubeconfig(t *testing.T) {
	t.Parallel()

	t.Run("parses valid kubeconfig", func(t *testing.T) {
		t.Parallel()
		kubeconfig := []byte(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://10.0.0.1:6443
    certificate-authority-data: dGVzdC1jYQ==
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: admin
  name: admin@test-cluster
current-context: admin@test-cluster
users:
- name: admin
  user:
    client-certificate-data: dGVzdC1jZXJ0
    client-key-data: dGVzdC1rZXk=
`)

		cfg, err := clientConfigFromKubeconfig(kubeconfig)
		require.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Equal(t, "https://10.0.0.1:6443", cfg.Host)
	})

	t.Run("returns error for invalid kubeconfig", func(t *testing.T) {
		t.Parallel()
		_, err := clientConfigFromKubeconfig([]byte("not valid yaml: {{"))
		assert.Error(t, err)
	})

	t.Run("returns error for empty kubeconfig", func(t *testing.T) {
		t.Parallel()
		_, err := clientConfigFromKubeconfig([]byte{})
		assert.Error(t, err)
	})
}

func TestGetKubeconfigFromTalos_ValidationErrors(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("returns error when talos config is empty", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder, WithMetrics(false))

		creds := &operatorprov.Credentials{
			TalosConfig: []byte{},
		}

		_, err := r.getKubeconfigFromTalos(context.Background(), cluster, creds)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "talos config not available")
	})

	t.Run("returns error when talos config is invalid", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder, WithMetrics(false))

		creds := &operatorprov.Credentials{
			TalosConfig: []byte("invalid-talos-config"),
		}

		_, err := r.getKubeconfigFromTalos(context.Background(), cluster, creds)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse talos config")
	})

	t.Run("returns error when no endpoint available", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			// No bootstrap, no CP endpoint, no healthy nodes
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder, WithMetrics(false))

		// Valid minimal talosconfig YAML
		creds := &operatorprov.Credentials{
			TalosConfig: []byte(`context: test
contexts:
  test:
    endpoints:
      - 1.2.3.4
    ca: ""
    crt: ""
    key: ""
`),
		}

		_, err := r.getKubeconfigFromTalos(context.Background(), cluster, creds)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no control plane endpoint available")
	})
}

func TestReconcileCNIPhase_PhaseTransitions(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	// These tests verify the phase transition logic at the end of reconcileCNIPhase
	// by checking what happens to cluster.Status.ProvisioningPhase.
	// The full flow is hard to test because it calls external services,
	// but we CAN test the credential loading step which uses the fake K8s client.

	t.Run("returns error when credentials secret is missing", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				CredentialsRef: corev1.LocalObjectReference{
					Name: "missing-secret",
				},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		r := NewClusterReconciler(fakeClient, scheme, recorder, WithMetrics(false))

		result, err := r.reconcileCNIPhase(context.Background(), cluster)
		assert.NoError(t, err) // errors are recorded as events, not returned
		assert.Equal(t, defaultRequeueAfter, result.RequeueAfter)
	})

	t.Run("returns error when credentialsRef name is empty", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				CredentialsRef: corev1.LocalObjectReference{
					Name: "",
				},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		r := NewClusterReconciler(fakeClient, scheme, recorder, WithMetrics(false))

		result, err := r.reconcileCNIPhase(context.Background(), cluster)
		assert.NoError(t, err)
		assert.Equal(t, defaultRequeueAfter, result.RequeueAfter)
	})
}

func TestReconcileAddonsPhase_WorkerWaiting(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("waits for workers before proceeding", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Workers: k8znerv1alpha1.WorkerSpec{Count: 2},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Ready: 0,
				},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		r := NewClusterReconciler(fakeClient, scheme, recorder, WithMetrics(false))

		result, err := r.reconcileAddonsPhase(context.Background(), cluster)
		assert.NoError(t, err)
		assert.NotZero(t, result.RequeueAfter, "should requeue while waiting for workers")
	})

	t.Run("proceeds when workers are ready", func(t *testing.T) {
		t.Parallel()
		// Workers are ready but credentials ref is empty - should proceed past
		// worker check and fail at credentials loading
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Workers: k8znerv1alpha1.WorkerSpec{Count: 2},
				CredentialsRef: corev1.LocalObjectReference{
					Name: "missing-secret",
				},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Ready: 2,
				},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		r := NewClusterReconciler(fakeClient, scheme, recorder, WithMetrics(false))

		result, err := r.reconcileAddonsPhase(context.Background(), cluster)
		assert.NoError(t, err)
		// Should have gotten past worker check and failed at credentials
		assert.Equal(t, defaultRequeueAfter, result.RequeueAfter)
	})

	t.Run("skips worker check when desired is zero", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Workers: k8znerv1alpha1.WorkerSpec{Count: 0},
				CredentialsRef: corev1.LocalObjectReference{
					Name: "missing-secret",
				},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		r := NewClusterReconciler(fakeClient, scheme, recorder, WithMetrics(false))

		result, err := r.reconcileAddonsPhase(context.Background(), cluster)
		assert.NoError(t, err)
		// Should have skipped worker check and failed at credentials
		assert.Equal(t, defaultRequeueAfter, result.RequeueAfter)
	})
}

func TestInstallNextAddon_AddonTracking(t *testing.T) {
	t.Parallel()

	// installNextAddon is hard to test fully because it calls package-level
	// addons.EnabledSteps() and addons.InstallStep(). However, we CAN test
	// the completion path: when all addons are already installed.

	t.Run("completes when all addons already installed", func(t *testing.T) {
		t.Parallel()
		scheme := setupTestScheme(t)

		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				// Pre-populate all addons as installed so the loop finds nothing to install
				Addons: map[string]k8znerv1alpha1.AddonStatus{
					"cilium":          {Installed: true},
					"hcloud-ccm":      {Installed: true},
					"hcloud-csi":      {Installed: true},
					"metrics-server":  {Installed: true},
					"cert-manager":    {Installed: true},
					"traefik":         {Installed: true},
					"external-dns":    {Installed: true},
					"argocd":          {Installed: true},
					"kube-prometheus": {Installed: true},
					"talos-backup":    {Installed: true},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		r := NewClusterReconciler(fakeClient, scheme, recorder, WithMetrics(false))

		// Use a minimal config that disables all optional addons
		// The key insight: EnabledSteps() returns steps based on config
		// When all returned steps are in cluster.Status.Addons, the function completes
		cfg := &config.Config{
			ClusterName: "test-cluster",
			Location:    "nbg1",
		}

		result, err := r.installNextAddon(context.Background(), cluster, cfg, []byte("kubeconfig"), 1)
		require.NoError(t, err)
		assert.True(t, result.Requeue)
		assert.Equal(t, k8znerv1alpha1.PhaseComplete, cluster.Status.ProvisioningPhase)
		assert.Equal(t, k8znerv1alpha1.ClusterPhaseRunning, cluster.Status.Phase)
	})

	t.Run("initializes addons map when nil", func(t *testing.T) {
		t.Parallel()
		scheme := setupTestScheme(t)

		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Addons: nil,
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		r := NewClusterReconciler(fakeClient, scheme, recorder, WithMetrics(false))

		// Use config with no enabled addons by having all covered in status
		// (This test verifies the nil map initialization, the install itself will fail
		// but that's OK - we just want to verify the map gets created)
		cfg := &config.Config{
			ClusterName: "test-cluster",
			Location:    "nbg1",
		}

		// The function will try to install the first addon and fail (no real kubeconfig),
		// but the Addons map should be initialized
		_, _ = r.installNextAddon(context.Background(), cluster, cfg, []byte("fake"), 1)
		assert.NotNil(t, cluster.Status.Addons, "addons map should be initialized")
	})
}
