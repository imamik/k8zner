package controller

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	hcloudgo "github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
)

func setupTestScheme(t *testing.T) *runtime.Scheme {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, k8znerv1alpha1.AddToScheme(scheme))
	return scheme
}

// createTestNode is a helper function to create test nodes.
func createTestNode(name string, isControlPlane, isReady bool) *corev1.Node {
	labels := map[string]string{}
	if isControlPlane {
		labels["node-role.kubernetes.io/control-plane"] = ""
	}

	readyStatus := corev1.ConditionFalse
	if isReady {
		readyStatus = corev1.ConditionTrue
	}

	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Spec: corev1.NodeSpec{
			ProviderID: "hcloud://12345",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:               corev1.NodeReady,
					Status:             readyStatus,
					LastTransitionTime: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
				},
			},
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "10.0.0.1"},
				{Type: corev1.NodeExternalIP, Address: "1.2.3.4"},
			},
		},
	}
}

func TestNewClusterReconciler(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	t.Run("with default options", func(t *testing.T) {
		t.Parallel()
		r := NewClusterReconciler(client, scheme, recorder)

		assert.NotNil(t, r)
		assert.Equal(t, client, r.Client)
		assert.Equal(t, scheme, r.Scheme)
		assert.Equal(t, recorder, r.Recorder)
		assert.True(t, r.enableMetrics)
		assert.Equal(t, 1, r.maxConcurrentHeals)
	})

	t.Run("with custom options", func(t *testing.T) {
		t.Parallel()
		mockHCloud := &MockHCloudClient{}
		mockTalos := &MockTalosClient{}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithTalosClient(mockTalos),
			WithMetrics(false),
			WithMaxConcurrentHeals(3),
		)

		assert.NotNil(t, r)
		assert.Equal(t, mockHCloud, r.hcloudClient)
		assert.Equal(t, mockTalos, r.talosClient)
		assert.False(t, r.enableMetrics)
		assert.Equal(t, 3, r.maxConcurrentHeals)
	})

	t.Run("with HCloud token for lazy initialization", func(t *testing.T) {
		t.Parallel()
		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudToken("test-token"),
		)

		assert.NotNil(t, r)
		assert.Equal(t, "test-token", r.hcloudToken)
		assert.Nil(t, r.hcloudClient) // Should be nil until ensureHCloudClient is called
	})
}

func TestClusterReconciler_Reconcile(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("cluster not found returns no error", func(t *testing.T) {
		t.Parallel()
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(&MockHCloudClient{}),
		)

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "default",
				Name:      "nonexistent",
			},
		})

		assert.NoError(t, err)
		assert.Equal(t, ctrl.Result{}, result)
	})

	t.Run("paused cluster skips reconciliation", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Paused: true,
			},
		}

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(&MockHCloudClient{}),
		)

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "default",
				Name:      "test-cluster",
			},
		})

		assert.NoError(t, err)
		assert.Equal(t, defaultRequeueAfter, result.RequeueAfter)
	})

	t.Run("reconciles healthy cluster", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Region: "nbg1",
				ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{
					Count: 3,
					Size:  "cx22",
				},
				Workers: k8znerv1alpha1.WorkerSpec{
					Count: 2,
					Size:  "cx22",
				},
			},
		}

		cpNode1 := createTestNode("cp-1", true, true)
		cpNode2 := createTestNode("cp-2", true, true)
		cpNode3 := createTestNode("cp-3", true, true)

		worker1 := createTestNode("worker-1", false, true)
		worker2 := createTestNode("worker-2", false, true)

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster, cpNode1, cpNode2, cpNode3, worker1, worker2).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		mockHCloud := &MockHCloudClient{}
		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "default",
				Name:      "test-cluster",
			},
		})

		assert.NoError(t, err)
		assert.Equal(t, defaultRequeueAfter, result.RequeueAfter)

		updatedCluster := &k8znerv1alpha1.K8znerCluster{}
		err = client.Get(context.Background(), types.NamespacedName{
			Namespace: "default",
			Name:      "test-cluster",
		}, updatedCluster)
		require.NoError(t, err)

		assert.Equal(t, 3, updatedCluster.Status.ControlPlanes.Ready)
		assert.Equal(t, 2, updatedCluster.Status.Workers.Ready)
		assert.Equal(t, k8znerv1alpha1.ClusterPhaseRunning, updatedCluster.Status.Phase)
	})
}

func TestBuildClusterState(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("builds state with SSH keys from annotations", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-cluster",
				Namespace: "default",
				Annotations: map[string]string{
					"k8zner.io/ssh-keys":               "key1,key2,key3",
					"k8zner.io/control-plane-endpoint": "1.2.3.4",
				},
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Region: "fsn1",
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "cp-1", Healthy: true, PrivateIP: "10.0.0.1", PublicIP: "1.2.3.1"},
						{Name: "cp-2", Healthy: true, PrivateIP: "10.0.0.2", PublicIP: "1.2.3.2"},
					},
				},
			},
		}

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		mockHCloud := &MockHCloudClient{}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		state, err := r.buildClusterState(context.Background(), cluster)
		require.NoError(t, err)

		assert.Equal(t, "my-cluster", state.Name)
		assert.Equal(t, "fsn1", state.Region)
		assert.Equal(t, []string{"key1", "key2", "key3"}, state.SSHKeyIDs)
		assert.Equal(t, "1.2.3.4", state.ControlPlaneIP)
		assert.Contains(t, state.SANs, "10.0.0.1")
		assert.Contains(t, state.SANs, "10.0.0.2")
		assert.Contains(t, state.SANs, "1.2.3.1")
		assert.Contains(t, state.SANs, "1.2.3.2")
		assert.Equal(t, int64(1), state.NetworkID)
	})

	t.Run("uses default SSH key naming when no annotation", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Region: "nbg1",
			},
		}

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		mockHCloud := &MockHCloudClient{}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		state, err := r.buildClusterState(context.Background(), cluster)
		require.NoError(t, err)

		assert.Equal(t, []string{"my-cluster-key"}, state.SSHKeyIDs)
	})
}

func TestGenerateReplacementServerName(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(client, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-cluster",
		},
	}

	t.Run("generates new name for control plane with cp role", func(t *testing.T) {
		t.Parallel()
		name := r.generateReplacementServerName(cluster, "control-plane", "my-cluster-control-plane-3")
		assert.True(t, strings.HasPrefix(name, "my-cluster-cp-"), "expected name to start with my-cluster-cp-, got %s", name)
		parts := strings.Split(name, "-")
		assert.Equal(t, 4, len(parts), "expected 4 parts: my, cluster, cp, id")
		assert.Equal(t, 5, len(parts[3]), "expected 5-char random ID, got %s", parts[3])
	})

	t.Run("generates new name for worker with w role", func(t *testing.T) {
		t.Parallel()
		name := r.generateReplacementServerName(cluster, "worker", "my-cluster-workers-2")
		assert.True(t, strings.HasPrefix(name, "my-cluster-w-"), "expected name to start with my-cluster-w-, got %s", name)
		parts := strings.Split(name, "-")
		assert.Equal(t, 4, len(parts), "expected 4 parts: my, cluster, w, id")
		assert.Equal(t, 5, len(parts[3]), "expected 5-char random ID, got %s", parts[3])
	})

	t.Run("generates unique names each time", func(t *testing.T) {
		t.Parallel()
		name1 := r.generateReplacementServerName(cluster, "worker", "old-name")
		name2 := r.generateReplacementServerName(cluster, "worker", "old-name")
		assert.NotEqual(t, name1, name2, "expected unique names on each call")
	})

	t.Run("handles unknown role with fallback format", func(t *testing.T) {
		t.Parallel()
		name := r.generateReplacementServerName(cluster, "storage", "old-storage-node")
		assert.True(t, strings.HasPrefix(name, "my-cluster-st-"), "expected name to start with my-cluster-st-, got %s", name)
	})
}

func TestNodeEventHandler(t *testing.T) {
	t.Parallel()
	h := &nodeEventHandler{}

	t.Run("Create enqueues cluster", func(t *testing.T) {
		t.Parallel()
		q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
		defer q.ShutDown()

		h.Create(context.Background(), event.CreateEvent{}, q)
		assert.Equal(t, 1, q.Len())

		item, _ := q.Get()
		assert.Equal(t, "k8zner-system", item.Namespace)
		assert.Equal(t, "cluster", item.Name)
		q.Done(item)
	})

	t.Run("Update enqueues cluster", func(t *testing.T) {
		t.Parallel()
		q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
		defer q.ShutDown()

		h.Update(context.Background(), event.UpdateEvent{}, q)
		assert.Equal(t, 1, q.Len())

		item, _ := q.Get()
		assert.Equal(t, "k8zner-system", item.Namespace)
		assert.Equal(t, "cluster", item.Name)
		q.Done(item)
	})

	t.Run("Delete enqueues cluster", func(t *testing.T) {
		t.Parallel()
		q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
		defer q.ShutDown()

		h.Delete(context.Background(), event.DeleteEvent{}, q)
		assert.Equal(t, 1, q.Len())

		item, _ := q.Get()
		assert.Equal(t, "k8zner-system", item.Namespace)
		assert.Equal(t, "cluster", item.Name)
		q.Done(item)
	})

	t.Run("Generic enqueues cluster", func(t *testing.T) {
		t.Parallel()
		q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
		defer q.ShutDown()

		h.Generic(context.Background(), event.GenericEvent{}, q)
		assert.Equal(t, 1, q.Len())

		item, _ := q.Get()
		assert.Equal(t, "k8zner-system", item.Namespace)
		assert.Equal(t, "cluster", item.Name)
		q.Done(item)
	})
}

func TestGetPrivateIPFromServer(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("returns error when GetServerByName fails", func(t *testing.T) {
		t.Parallel()
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		mockHCloud := &MockHCloudClient{
			GetServerByNameFunc: func(_ context.Context, _ string) (*hcloudgo.Server, error) {
				return nil, assert.AnError
			},
		}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
		)

		_, err := r.getPrivateIPFromServer(context.Background(), "test-server")
		assert.Error(t, err)
	})

	t.Run("returns error when server not found", func(t *testing.T) {
		t.Parallel()
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		mockHCloud := &MockHCloudClient{
			GetServerByNameFunc: func(_ context.Context, _ string) (*hcloudgo.Server, error) {
				return nil, nil
			},
		}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
		)

		_, err := r.getPrivateIPFromServer(context.Background(), "missing-server")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("returns private IP from server", func(t *testing.T) {
		t.Parallel()
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		mockHCloud := &MockHCloudClient{
			GetServerByNameFunc: func(_ context.Context, _ string) (*hcloudgo.Server, error) {
				return &hcloudgo.Server{
					ID:   123,
					Name: "test-server",
					PrivateNet: []hcloudgo.ServerPrivateNet{
						{IP: net.ParseIP("10.0.0.5")},
					},
				}, nil
			},
		}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
		)

		ip, err := r.getPrivateIPFromServer(context.Background(), "test-server")
		require.NoError(t, err)
		assert.Equal(t, "10.0.0.5", ip)
	})

	t.Run("returns empty string when no private networks", func(t *testing.T) {
		t.Parallel()
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		mockHCloud := &MockHCloudClient{
			GetServerByNameFunc: func(_ context.Context, _ string) (*hcloudgo.Server, error) {
				return &hcloudgo.Server{
					ID:   123,
					Name: "test-server",
				}, nil
			},
		}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
		)

		ip, err := r.getPrivateIPFromServer(context.Background(), "test-server")
		require.NoError(t, err)
		assert.Equal(t, "", ip)
	})
}
