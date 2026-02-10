package addons

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
)

func TestNewPhaseManager(t *testing.T) {
	t.Parallel()

	t.Run("creates with client", func(t *testing.T) {
		t.Parallel()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		pm := NewPhaseManager(k8sClient)
		require.NotNil(t, pm)
		assert.NotNil(t, pm.k8sClient)
	})

	t.Run("creates with nil client", func(t *testing.T) {
		t.Parallel()
		pm := NewPhaseManager(nil)
		require.NotNil(t, pm)
		assert.Nil(t, pm.k8sClient)
	})
}

func TestClientConfigFromKubeconfig_InvalidBytes(t *testing.T) {
	t.Parallel()
	_, err := clientConfigFromKubeconfig([]byte("not-valid-kubeconfig"))
	assert.Error(t, err)
}

func TestClientConfigFromKubeconfig_EmptyBytes(t *testing.T) {
	t.Parallel()
	_, err := clientConfigFromKubeconfig(nil)
	assert.Error(t, err)
}

func TestIsAddonEnabled(t *testing.T) {
	t.Parallel()

	t.Run("core addons always enabled regardless of spec", func(t *testing.T) {
		t.Parallel()

		coreAddons := []string{
			k8znerv1alpha1.AddonNameCilium,
			k8znerv1alpha1.AddonNameCCM,
			k8znerv1alpha1.AddonNameCSI,
		}

		for _, addon := range coreAddons {
			t.Run(addon, func(t *testing.T) {
				t.Parallel()

				// With nil Addons spec
				cluster := &k8znerv1alpha1.K8znerCluster{}
				assert.True(t, IsAddonEnabled(cluster, addon))

				// With empty Addons spec
				cluster.Spec.Addons = &k8znerv1alpha1.AddonSpec{}
				assert.True(t, IsAddonEnabled(cluster, addon))

				// With nil Backup spec
				cluster.Spec.Backup = nil
				assert.True(t, IsAddonEnabled(cluster, addon))
			})
		}
	})

	t.Run("talos-backup enabled when spec.Backup is set and Enabled is true", func(t *testing.T) {
		t.Parallel()

		cluster := &k8znerv1alpha1.K8znerCluster{
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Backup: &k8znerv1alpha1.BackupSpec{
					Enabled: true,
				},
			},
		}
		assert.True(t, IsAddonEnabled(cluster, k8znerv1alpha1.AddonNameTalosBackup))
	})

	t.Run("talos-backup disabled when spec.Backup is nil", func(t *testing.T) {
		t.Parallel()

		cluster := &k8znerv1alpha1.K8znerCluster{}
		assert.False(t, IsAddonEnabled(cluster, k8znerv1alpha1.AddonNameTalosBackup))
	})

	t.Run("talos-backup disabled when spec.Backup.Enabled is false", func(t *testing.T) {
		t.Parallel()

		cluster := &k8znerv1alpha1.K8znerCluster{
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Backup: &k8znerv1alpha1.BackupSpec{
					Enabled: false,
				},
			},
		}
		assert.False(t, IsAddonEnabled(cluster, k8znerv1alpha1.AddonNameTalosBackup))
	})

	t.Run("non-core addons return false when spec.Addons is nil", func(t *testing.T) {
		t.Parallel()

		nonCoreAddons := []string{
			k8znerv1alpha1.AddonNameMetricsServer,
			k8znerv1alpha1.AddonNameCertManager,
			k8znerv1alpha1.AddonNameTraefik,
			k8znerv1alpha1.AddonNameExternalDNS,
			k8znerv1alpha1.AddonNameArgoCD,
			k8znerv1alpha1.AddonNameMonitoring,
		}

		cluster := &k8znerv1alpha1.K8znerCluster{}

		for _, addon := range nonCoreAddons {
			t.Run(addon, func(t *testing.T) {
				t.Parallel()
				assert.False(t, IsAddonEnabled(cluster, addon))
			})
		}
	})

	t.Run("non-core addons enabled and disabled correctly", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			addon    string
			spec     k8znerv1alpha1.AddonSpec
			expected bool
		}{
			{
				name:     "metrics-server enabled",
				addon:    k8znerv1alpha1.AddonNameMetricsServer,
				spec:     k8znerv1alpha1.AddonSpec{MetricsServer: true},
				expected: true,
			},
			{
				name:     "metrics-server disabled",
				addon:    k8znerv1alpha1.AddonNameMetricsServer,
				spec:     k8znerv1alpha1.AddonSpec{MetricsServer: false},
				expected: false,
			},
			{
				name:     "cert-manager enabled",
				addon:    k8znerv1alpha1.AddonNameCertManager,
				spec:     k8znerv1alpha1.AddonSpec{CertManager: true},
				expected: true,
			},
			{
				name:     "cert-manager disabled",
				addon:    k8znerv1alpha1.AddonNameCertManager,
				spec:     k8znerv1alpha1.AddonSpec{CertManager: false},
				expected: false,
			},
			{
				name:     "traefik enabled",
				addon:    k8znerv1alpha1.AddonNameTraefik,
				spec:     k8znerv1alpha1.AddonSpec{Traefik: true},
				expected: true,
			},
			{
				name:     "traefik disabled",
				addon:    k8znerv1alpha1.AddonNameTraefik,
				spec:     k8znerv1alpha1.AddonSpec{Traefik: false},
				expected: false,
			},
			{
				name:     "external-dns enabled",
				addon:    k8znerv1alpha1.AddonNameExternalDNS,
				spec:     k8znerv1alpha1.AddonSpec{ExternalDNS: true},
				expected: true,
			},
			{
				name:     "external-dns disabled",
				addon:    k8znerv1alpha1.AddonNameExternalDNS,
				spec:     k8znerv1alpha1.AddonSpec{ExternalDNS: false},
				expected: false,
			},
			{
				name:     "argocd enabled",
				addon:    k8znerv1alpha1.AddonNameArgoCD,
				spec:     k8znerv1alpha1.AddonSpec{ArgoCD: true},
				expected: true,
			},
			{
				name:     "argocd disabled",
				addon:    k8znerv1alpha1.AddonNameArgoCD,
				spec:     k8znerv1alpha1.AddonSpec{ArgoCD: false},
				expected: false,
			},
			{
				name:     "monitoring enabled",
				addon:    k8znerv1alpha1.AddonNameMonitoring,
				spec:     k8znerv1alpha1.AddonSpec{Monitoring: true},
				expected: true,
			},
			{
				name:     "monitoring disabled",
				addon:    k8znerv1alpha1.AddonNameMonitoring,
				spec:     k8znerv1alpha1.AddonSpec{Monitoring: false},
				expected: false,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				cluster := &k8znerv1alpha1.K8znerCluster{
					Spec: k8znerv1alpha1.K8znerClusterSpec{
						Addons: &tc.spec,
					},
				}
				assert.Equal(t, tc.expected, IsAddonEnabled(cluster, tc.addon))
			})
		}
	})

	t.Run("unknown addon name returns false", func(t *testing.T) {
		t.Parallel()

		cluster := &k8znerv1alpha1.K8znerCluster{
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Addons: &k8znerv1alpha1.AddonSpec{
					Traefik:       true,
					CertManager:   true,
					MetricsServer: true,
					ExternalDNS:   true,
					ArgoCD:        true,
					Monitoring:    true,
				},
				Backup: &k8znerv1alpha1.BackupSpec{Enabled: true},
			},
		}
		assert.False(t, IsAddonEnabled(cluster, "unknown-addon"))
		assert.False(t, IsAddonEnabled(cluster, ""))
		assert.False(t, IsAddonEnabled(cluster, "nginx"))
	})
}

func TestUpdateAddonStatus(t *testing.T) {
	t.Parallel()

	t.Run("initializes Addons map when nil", func(t *testing.T) {
		t.Parallel()

		pm := NewPhaseManager(nil)
		cluster := &k8znerv1alpha1.K8znerCluster{}
		assert.Nil(t, cluster.Status.Addons)

		pm.UpdateAddonStatus(cluster, k8znerv1alpha1.AddonNameCilium, true, true, k8znerv1alpha1.AddonPhaseInstalled, "installed")

		require.NotNil(t, cluster.Status.Addons)
		assert.Len(t, cluster.Status.Addons, 1)
	})

	t.Run("sets all fields correctly", func(t *testing.T) {
		t.Parallel()

		pm := NewPhaseManager(nil)
		cluster := &k8znerv1alpha1.K8znerCluster{}

		pm.UpdateAddonStatus(cluster, k8znerv1alpha1.AddonNameCilium, true, true, k8znerv1alpha1.AddonPhaseInstalled, "cilium ready")

		status, ok := cluster.Status.Addons[k8znerv1alpha1.AddonNameCilium]
		require.True(t, ok)
		assert.True(t, status.Installed)
		assert.True(t, status.Healthy)
		assert.Equal(t, k8znerv1alpha1.AddonPhaseInstalled, status.Phase)
		assert.Equal(t, "cilium ready", status.Message)
		assert.Equal(t, k8znerv1alpha1.AddonOrderCilium, status.InstallOrder)
		assert.NotNil(t, status.LastTransitionTime)
	})

	t.Run("overwrites existing entry", func(t *testing.T) {
		t.Parallel()

		pm := NewPhaseManager(nil)
		cluster := &k8znerv1alpha1.K8znerCluster{}

		// First update: installing
		pm.UpdateAddonStatus(cluster, k8znerv1alpha1.AddonNameTraefik, false, false, k8znerv1alpha1.AddonPhaseInstalling, "installing traefik")

		status := cluster.Status.Addons[k8znerv1alpha1.AddonNameTraefik]
		assert.False(t, status.Installed)
		assert.False(t, status.Healthy)
		assert.Equal(t, k8znerv1alpha1.AddonPhaseInstalling, status.Phase)

		// Second update: installed
		pm.UpdateAddonStatus(cluster, k8znerv1alpha1.AddonNameTraefik, true, true, k8znerv1alpha1.AddonPhaseInstalled, "traefik ready")

		status = cluster.Status.Addons[k8znerv1alpha1.AddonNameTraefik]
		assert.True(t, status.Installed)
		assert.True(t, status.Healthy)
		assert.Equal(t, k8znerv1alpha1.AddonPhaseInstalled, status.Phase)
		assert.Equal(t, "traefik ready", status.Message)
		assert.Equal(t, k8znerv1alpha1.AddonOrderTraefik, status.InstallOrder)
	})

	t.Run("LastTransitionTime is set", func(t *testing.T) {
		t.Parallel()

		pm := NewPhaseManager(nil)
		cluster := &k8znerv1alpha1.K8znerCluster{}

		pm.UpdateAddonStatus(cluster, k8znerv1alpha1.AddonNameCSI, true, false, k8znerv1alpha1.AddonPhaseFailed, "csi error")

		status := cluster.Status.Addons[k8znerv1alpha1.AddonNameCSI]
		require.NotNil(t, status.LastTransitionTime)
		assert.False(t, status.LastTransitionTime.IsZero())
	})
}

func TestGetAddonOrder(t *testing.T) {
	t.Parallel()

	t.Run("known addons return correct order", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name          string
			expectedOrder int
		}{
			{name: k8znerv1alpha1.AddonNameCilium, expectedOrder: k8znerv1alpha1.AddonOrderCilium},
			{name: k8znerv1alpha1.AddonNameCCM, expectedOrder: k8znerv1alpha1.AddonOrderCCM},
			{name: k8znerv1alpha1.AddonNameCSI, expectedOrder: k8znerv1alpha1.AddonOrderCSI},
			{name: k8znerv1alpha1.AddonNameMetricsServer, expectedOrder: k8znerv1alpha1.AddonOrderMetricsServer},
			{name: k8znerv1alpha1.AddonNameCertManager, expectedOrder: k8znerv1alpha1.AddonOrderCertManager},
			{name: k8znerv1alpha1.AddonNameTraefik, expectedOrder: k8znerv1alpha1.AddonOrderTraefik},
			{name: k8znerv1alpha1.AddonNameExternalDNS, expectedOrder: k8znerv1alpha1.AddonOrderExternalDNS},
			{name: k8znerv1alpha1.AddonNameArgoCD, expectedOrder: k8znerv1alpha1.AddonOrderArgoCD},
			{name: k8znerv1alpha1.AddonNameMonitoring, expectedOrder: k8znerv1alpha1.AddonOrderMonitoring},
			{name: k8znerv1alpha1.AddonNameTalosBackup, expectedOrder: k8znerv1alpha1.AddonOrderTalosBackup},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				assert.Equal(t, tc.expectedOrder, getAddonOrder(tc.name))
			})
		}
	})

	t.Run("unknown addon returns 99", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, 99, getAddonOrder("unknown-addon"))
		assert.Equal(t, 99, getAddonOrder(""))
		assert.Equal(t, 99, getAddonOrder("nginx-ingress"))
	})
}

func TestAddonOrder(t *testing.T) {
	t.Parallel()

	t.Run("has exactly 10 entries", func(t *testing.T) {
		t.Parallel()
		assert.Len(t, AddonOrder, 10)
	})

	t.Run("first 3 are required core addons", func(t *testing.T) {
		t.Parallel()

		for i := 0; i < 3; i++ {
			assert.True(t, AddonOrder[i].Required, "AddonOrder[%d] (%s) should be required", i, AddonOrder[i].Name)
		}

		assert.Equal(t, k8znerv1alpha1.AddonNameCilium, AddonOrder[0].Name)
		assert.Equal(t, k8znerv1alpha1.AddonNameCCM, AddonOrder[1].Name)
		assert.Equal(t, k8znerv1alpha1.AddonNameCSI, AddonOrder[2].Name)
	})

	t.Run("remaining 7 are not required", func(t *testing.T) {
		t.Parallel()

		for i := 3; i < len(AddonOrder); i++ {
			assert.False(t, AddonOrder[i].Required, "AddonOrder[%d] (%s) should not be required", i, AddonOrder[i].Name)
		}
	})

	t.Run("orders are sequential 1 through 10", func(t *testing.T) {
		t.Parallel()

		for i, info := range AddonOrder {
			assert.Equal(t, i+1, info.InstallOrder, "AddonOrder[%d] (%s) should have order %d", i, info.Name, i+1)
		}
	})
}

func TestIsCiliumReady(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	t.Run("returns false when no pods exist", func(t *testing.T) {
		t.Parallel()

		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		pm := NewPhaseManager(nil)

		ready, err := pm.isCiliumReady(context.Background(), k8sClient)
		require.NoError(t, err)
		assert.False(t, ready)
	})

	t.Run("returns false when pod is not Running", func(t *testing.T) {
		t.Parallel()

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cilium-abc12",
				Namespace: "kube-system",
				Labels:    map[string]string{"k8s-app": "cilium"},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
			},
		}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(pod).
			Build()
		pm := NewPhaseManager(nil)

		ready, err := pm.isCiliumReady(context.Background(), k8sClient)
		require.NoError(t, err)
		assert.False(t, ready)
	})

	t.Run("returns false when pod is Running but not Ready", func(t *testing.T) {
		t.Parallel()

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cilium-def34",
				Namespace: "kube-system",
				Labels:    map[string]string{"k8s-app": "cilium"},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
		}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(pod).
			Build()
		pm := NewPhaseManager(nil)

		ready, err := pm.isCiliumReady(context.Background(), k8sClient)
		require.NoError(t, err)
		assert.False(t, ready)
	})

	t.Run("returns true when all pods are Running and Ready", func(t *testing.T) {
		t.Parallel()

		pod1 := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cilium-node1",
				Namespace: "kube-system",
				Labels:    map[string]string{"k8s-app": "cilium"},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		pod2 := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cilium-node2",
				Namespace: "kube-system",
				Labels:    map[string]string{"k8s-app": "cilium"},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(pod1, pod2).
			Build()
		pm := NewPhaseManager(nil)

		ready, err := pm.isCiliumReady(context.Background(), k8sClient)
		require.NoError(t, err)
		assert.True(t, ready)
	})

	t.Run("returns false when one pod is Ready and another is not", func(t *testing.T) {
		t.Parallel()

		readyPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cilium-ready",
				Namespace: "kube-system",
				Labels:    map[string]string{"k8s-app": "cilium"},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		notReadyPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cilium-notready",
				Namespace: "kube-system",
				Labels:    map[string]string{"k8s-app": "cilium"},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
		}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(readyPod, notReadyPod).
			Build()
		pm := NewPhaseManager(nil)

		ready, err := pm.isCiliumReady(context.Background(), k8sClient)
		require.NoError(t, err)
		assert.False(t, ready)
	})

	t.Run("ignores pods without matching label", func(t *testing.T) {
		t.Parallel()

		// Pod in kube-system but without the cilium label
		otherPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns-abc12",
				Namespace: "kube-system",
				Labels:    map[string]string{"k8s-app": "kube-dns"},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(otherPod).
			Build()
		pm := NewPhaseManager(nil)

		ready, err := pm.isCiliumReady(context.Background(), k8sClient)
		require.NoError(t, err)
		assert.False(t, ready, "should return false when no cilium pods exist")
	})

	t.Run("returns error when List fails", func(t *testing.T) {
		t.Parallel()

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithInterceptorFuncs(interceptor.Funcs{
				List: func(_ context.Context, _ client.WithWatch, _ client.ObjectList, _ ...client.ListOption) error {
					return fmt.Errorf("api server unavailable")
				},
			}).
			Build()
		pm := NewPhaseManager(nil)

		ready, err := pm.isCiliumReady(context.Background(), k8sClient)
		require.Error(t, err)
		assert.False(t, ready)
		assert.Contains(t, err.Error(), "api server unavailable")
	})

	t.Run("returns true for pod running with multiple conditions", func(t *testing.T) {
		t.Parallel()

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cilium-multi",
				Namespace: "kube-system",
				Labels:    map[string]string{"k8s-app": "cilium"},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodScheduled,
						Status: corev1.ConditionTrue,
					},
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					},
					{
						Type:   corev1.ContainersReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(pod).
			Build()
		pm := NewPhaseManager(nil)

		ready, err := pm.isCiliumReady(context.Background(), k8sClient)
		require.NoError(t, err)
		assert.True(t, ready)
	})
}

func TestWaitForCiliumReady_InvalidKubeconfig(t *testing.T) {
	t.Parallel()

	pm := NewPhaseManager(nil)
	err := pm.waitForCiliumReady(context.Background(), []byte("not-valid-kubeconfig"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create rest config")
}

func TestWaitForCiliumReady_NilKubeconfig(t *testing.T) {
	t.Parallel()

	pm := NewPhaseManager(nil)
	err := pm.waitForCiliumReady(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create rest config")
}

func TestWaitForCiliumReady_EmptyKubeconfig(t *testing.T) {
	t.Parallel()

	pm := NewPhaseManager(nil)
	err := pm.waitForCiliumReady(context.Background(), []byte{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create rest config")
}

func TestInstallAddons_ErrorWrapping(t *testing.T) {
	t.Parallel()

	// InstallAddons wraps errors from ApplyWithoutCilium. We cannot directly test
	// the success path without a real K8s cluster, but we can verify the method
	// exists and returns nil when the underlying function succeeds.
	// Since ApplyWithoutCilium will fail with nil kubeconfig, test error wrapping.
	pm := NewPhaseManager(nil)
	err := pm.InstallAddons(context.Background(), nil, nil, 0)
	// Should fail because cfg and kubeconfig are nil
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to install addons")
}

func TestInstallCilium_ErrorWrapping(t *testing.T) {
	t.Parallel()

	// InstallCilium wraps errors from ApplyCilium. Test that it wraps correctly.
	pm := NewPhaseManager(nil)
	err := pm.InstallCilium(context.Background(), nil, nil)
	// Should fail because cfg and kubeconfig are nil
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to install Cilium")
}

func TestUpdateAddonStatus_UnknownAddonOrder(t *testing.T) {
	t.Parallel()

	pm := NewPhaseManager(nil)
	cluster := &k8znerv1alpha1.K8znerCluster{}

	pm.UpdateAddonStatus(cluster, "custom-addon", true, true, k8znerv1alpha1.AddonPhaseInstalled, "ok")

	status, ok := cluster.Status.Addons["custom-addon"]
	require.True(t, ok)
	assert.Equal(t, 99, status.InstallOrder, "unknown addon should get order 99")
	assert.True(t, status.Installed)
	assert.True(t, status.Healthy)
}

func TestUpdateAddonStatus_MultipleAddons(t *testing.T) {
	t.Parallel()

	pm := NewPhaseManager(nil)
	cluster := &k8znerv1alpha1.K8znerCluster{}

	pm.UpdateAddonStatus(cluster, k8znerv1alpha1.AddonNameCilium, true, true, k8znerv1alpha1.AddonPhaseInstalled, "ok")
	pm.UpdateAddonStatus(cluster, k8znerv1alpha1.AddonNameCCM, true, true, k8znerv1alpha1.AddonPhaseInstalled, "ok")
	pm.UpdateAddonStatus(cluster, k8znerv1alpha1.AddonNameCSI, true, false, k8znerv1alpha1.AddonPhaseFailed, "error")

	require.Len(t, cluster.Status.Addons, 3)

	cilium := cluster.Status.Addons[k8znerv1alpha1.AddonNameCilium]
	assert.Equal(t, k8znerv1alpha1.AddonOrderCilium, cilium.InstallOrder)

	ccm := cluster.Status.Addons[k8znerv1alpha1.AddonNameCCM]
	assert.Equal(t, k8znerv1alpha1.AddonOrderCCM, ccm.InstallOrder)

	csi := cluster.Status.Addons[k8znerv1alpha1.AddonNameCSI]
	assert.Equal(t, k8znerv1alpha1.AddonOrderCSI, csi.InstallOrder)
	assert.False(t, csi.Healthy)
	assert.Equal(t, k8znerv1alpha1.AddonPhaseFailed, csi.Phase)
}

func TestUpdateAddonStatus_AllAddonPhases(t *testing.T) {
	t.Parallel()

	phases := []k8znerv1alpha1.AddonPhase{
		k8znerv1alpha1.AddonPhasePending,
		k8znerv1alpha1.AddonPhaseInstalling,
		k8znerv1alpha1.AddonPhaseInstalled,
		k8znerv1alpha1.AddonPhaseFailed,
		k8znerv1alpha1.AddonPhaseUpgrading,
	}

	for _, phase := range phases {
		t.Run(string(phase), func(t *testing.T) {
			t.Parallel()

			pm := NewPhaseManager(nil)
			cluster := &k8znerv1alpha1.K8znerCluster{}

			pm.UpdateAddonStatus(cluster, k8znerv1alpha1.AddonNameCilium, false, false, phase, "msg")

			status := cluster.Status.Addons[k8znerv1alpha1.AddonNameCilium]
			assert.Equal(t, phase, status.Phase)
		})
	}
}
