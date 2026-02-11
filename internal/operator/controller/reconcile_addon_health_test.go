package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
)

func TestReconcileAddonHealth(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	require.NoError(t, k8znerv1alpha1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, batchv1.AddToScheme(scheme))

	t.Run("healthy deployment addon", func(t *testing.T) {
		t.Parallel()

		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "hcloud-cloud-controller-manager", Namespace: "kube-system"},
			Status:     appsv1.DeploymentStatus{Replicas: 1, ReadyReplicas: 1},
		}

		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dep).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(k8sClient, scheme, recorder)

		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Addons: map[string]k8znerv1alpha1.AddonStatus{
					k8znerv1alpha1.AddonNameCCM: {Installed: true},
				},
			},
		}

		r.reconcileAddonHealth(context.Background(), cluster)

		ccm := cluster.Status.Addons[k8znerv1alpha1.AddonNameCCM]
		assert.True(t, ccm.Healthy)
		assert.Contains(t, ccm.Message, "1/1 ready")
		assert.NotNil(t, ccm.LastHealthCheck)
	})

	t.Run("unhealthy deployment addon", func(t *testing.T) {
		t.Parallel()

		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "hcloud-cloud-controller-manager", Namespace: "kube-system"},
			Status:     appsv1.DeploymentStatus{Replicas: 1, ReadyReplicas: 0},
		}

		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dep).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(k8sClient, scheme, recorder)

		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Addons: map[string]k8znerv1alpha1.AddonStatus{
					k8znerv1alpha1.AddonNameCCM: {Installed: true},
				},
			},
		}

		r.reconcileAddonHealth(context.Background(), cluster)

		ccm := cluster.Status.Addons[k8znerv1alpha1.AddonNameCCM]
		assert.False(t, ccm.Healthy)
		assert.Contains(t, ccm.Message, "0/1 ready")
	})

	t.Run("healthy daemonset addon", func(t *testing.T) {
		t.Parallel()

		ds := &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "cilium", Namespace: "kube-system"},
			Status:     appsv1.DaemonSetStatus{NumberReady: 3},
		}

		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ds).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(k8sClient, scheme, recorder)

		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Addons: map[string]k8znerv1alpha1.AddonStatus{
					k8znerv1alpha1.AddonNameCilium: {Installed: true},
				},
			},
		}

		r.reconcileAddonHealth(context.Background(), cluster)

		cilium := cluster.Status.Addons[k8znerv1alpha1.AddonNameCilium]
		assert.True(t, cilium.Healthy)
		assert.Contains(t, cilium.Message, "3 ready")
	})

	t.Run("cronjob addon exists", func(t *testing.T) {
		t.Parallel()

		cj := &batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{Name: "talos-backup", Namespace: "kube-system"},
		}

		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cj).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(k8sClient, scheme, recorder)

		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Addons: map[string]k8znerv1alpha1.AddonStatus{
					k8znerv1alpha1.AddonNameTalosBackup: {Installed: true},
				},
			},
		}

		r.reconcileAddonHealth(context.Background(), cluster)

		backup := cluster.Status.Addons[k8znerv1alpha1.AddonNameTalosBackup]
		assert.True(t, backup.Healthy)
	})

	t.Run("skips uninstalled addons", func(t *testing.T) {
		t.Parallel()

		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(k8sClient, scheme, recorder)

		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Addons: map[string]k8znerv1alpha1.AddonStatus{
					k8znerv1alpha1.AddonNameCCM: {Installed: false},
				},
			},
		}

		r.reconcileAddonHealth(context.Background(), cluster)

		ccm := cluster.Status.Addons[k8znerv1alpha1.AddonNameCCM]
		assert.False(t, ccm.Healthy) // Not changed
		assert.Nil(t, ccm.LastHealthCheck)
	})

	t.Run("sets AddonsHealthy condition", func(t *testing.T) {
		t.Parallel()

		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "hcloud-cloud-controller-manager", Namespace: "kube-system"},
			Status:     appsv1.DeploymentStatus{Replicas: 1, ReadyReplicas: 1},
		}

		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dep).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(k8sClient, scheme, recorder)

		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Addons: map[string]k8znerv1alpha1.AddonStatus{
					k8znerv1alpha1.AddonNameCCM: {Installed: true},
				},
			},
		}

		r.reconcileAddonHealth(context.Background(), cluster)

		// Check condition
		var found bool
		for _, cond := range cluster.Status.Conditions {
			if cond.Type == k8znerv1alpha1.ConditionAddonsHealthy {
				found = true
				assert.Equal(t, metav1.ConditionTrue, cond.Status)
			}
		}
		assert.True(t, found, "AddonsHealthy condition should be set")
	})
}
