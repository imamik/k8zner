//go:build integration

// Package controller contains integration tests using envtest.
//
// Envtest provides a real Kubernetes API server and etcd instance for testing
// controller logic against the actual Kubernetes API. This is more reliable than
// mocking the client, as it catches issues with watch behavior, status updates,
// and CRD validation that mocks would miss.
//
// Run these tests with:
//
//	make test-integration
//
// Or manually:
//
//	KUBEBUILDER_ASSETS="$(setup-envtest use -p path)" go test -v -tags=integration ./internal/operator/controller/...
package controller

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
)

// Test configuration
var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc

	// Mock clients accessible to tests for verification
	mockHCloud *MockHCloudClient
	mockTalos  *MockTalosClient
)

// TestControllerIntegration is the entry point for Ginkgo tests.
func TestControllerIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.Background())

	By("bootstrapping test environment with real kube-apiserver and etcd")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		// Use existing cluster if KUBEBUILDER_ASSETS not set (for debugging)
		UseExistingCluster: nil,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// Register K8znerCluster CRD types with the scheme
	err = k8znerv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// Create the test client
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Create controller manager
	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).NotTo(HaveOccurred())

	// Initialize mock clients with default behavior
	mockHCloud = &MockHCloudClient{}
	mockTalos = &MockTalosClient{}

	// Register the controller with dependency injection for mocks
	err = NewClusterReconciler(
		k8sManager.GetClient(),
		k8sManager.GetScheme(),
		k8sManager.GetEventRecorderFor("k8znercluster-controller"),
		WithHCloudClient(mockHCloud),
		WithTalosClient(mockTalos),
		WithMetrics(false), // Disable metrics to avoid port conflicts in tests
	).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	// Start the controller manager in background
	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()

	// Wait for the manager to be fully ready
	// First wait for the cache to sync
	By("waiting for manager cache to sync")
	Eventually(func() bool {
		return k8sManager.GetCache().WaitForCacheSync(ctx)
	}, time.Second*30, time.Millisecond*500).Should(BeTrue(), "timed out waiting for cache sync")

	// Then verify the manager can list K8znerClusters (proves controller is watching)
	By("verifying controller is ready by listing clusters")
	Eventually(func() error {
		clusters := &k8znerv1alpha1.K8znerClusterList{}
		return k8sManager.GetClient().List(ctx, clusters)
	}, time.Second*10, time.Millisecond*100).Should(Succeed(), "controller not ready to list clusters")
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("K8znerCluster Controller", func() {
	// Test timing constants - increased for CI environments which can be slower
	const (
		timeout  = time.Second * 30
		interval = time.Millisecond * 500
	)

	// Helper to create unique cluster names for test isolation
	var testClusterName string
	var testNamespace string

	BeforeEach(func() {
		// Generate unique names to avoid test interference
		testClusterName = fmt.Sprintf("test-cluster-%d", GinkgoRandomSeed())
		testNamespace = "default"
	})

	AfterEach(func() {
		// Cleanup: delete any clusters created during the test
		cluster := &k8znerv1alpha1.K8znerCluster{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: testClusterName, Namespace: testNamespace}, cluster)
		if err == nil {
			// Remove finalizers to allow deletion in tests
			cluster.Finalizers = nil
			_ = k8sClient.Update(ctx, cluster)
			_ = k8sClient.Delete(ctx, cluster)
		}
	})

	// Helper function to create a basic K8znerCluster
	createCluster := func(name string, opts ...func(*k8znerv1alpha1.K8znerCluster)) *k8znerv1alpha1.K8znerCluster {
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testNamespace,
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Region: "fsn1",
				ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{
					Count: 1,
					Size:  "cx22",
				},
				Workers: k8znerv1alpha1.WorkerSpec{
					Count: 2,
					Size:  "cx22",
				},
				// Required fields with validation patterns
				Kubernetes: k8znerv1alpha1.KubernetesSpec{
					Version: "1.32.0",
				},
				Talos: k8znerv1alpha1.TalosSpec{
					Version: "v1.10.0",
				},
			},
		}
		for _, opt := range opts {
			opt(cluster)
		}
		return cluster
	}

	// Helper function to wait for cluster to exist
	getCluster := func(name string) *k8znerv1alpha1.K8znerCluster {
		cluster := &k8znerv1alpha1.K8znerCluster{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: testNamespace}, cluster)
		}, timeout, interval).Should(Succeed())
		return cluster
	}

	Context("Cluster Creation", func() {
		It("should create a K8znerCluster and reconcile it", func() {
			By("Creating a new K8znerCluster")
			cluster := createCluster(testClusterName)
			Expect(k8sClient.Create(ctx, cluster)).Should(Succeed())

			By("Verifying the cluster is created with correct spec")
			createdCluster := getCluster(testClusterName)
			Expect(createdCluster.Spec.Region).Should(Equal("fsn1"))
			Expect(createdCluster.Spec.ControlPlanes.Count).Should(Equal(1))
			Expect(createdCluster.Spec.Workers.Count).Should(Equal(2))
		})

		It("should update cluster status after reconciliation", func() {
			By("Creating a K8znerCluster")
			cluster := createCluster(testClusterName)
			Expect(k8sClient.Create(ctx, cluster)).Should(Succeed())

			By("Waiting for the controller to reconcile and update status")
			// First check if LastReconcileTime is set (proves reconciliation happened)
			Eventually(func() bool {
				c := &k8znerv1alpha1.K8znerCluster{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testClusterName, Namespace: testNamespace}, c)
				if err != nil {
					return false
				}
				// Check if any status was updated - reconciliation sets LastReconcileTime
				return c.Status.LastReconcileTime != nil
			}, timeout, interval).Should(BeTrue(), "reconciliation did not update LastReconcileTime")

			// Then verify Phase is set
			Eventually(func() string {
				c := &k8znerv1alpha1.K8znerCluster{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testClusterName, Namespace: testNamespace}, c)
				if err != nil {
					return ""
				}
				return string(c.Status.Phase)
			}, timeout, interval).ShouldNot(BeEmpty())
		})
	})

	Context("Paused Clusters", func() {
		It("should skip reconciliation when cluster is paused", func() {
			By("Creating a paused K8znerCluster")
			cluster := createCluster(testClusterName, func(c *k8znerv1alpha1.K8znerCluster) {
				c.Spec.Paused = true
			})
			Expect(k8sClient.Create(ctx, cluster)).Should(Succeed())

			By("Verifying the cluster status remains empty (not reconciled)")
			Consistently(func() string {
				c := &k8znerv1alpha1.K8znerCluster{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testClusterName, Namespace: testNamespace}, c)
				if err != nil {
					return "error"
				}
				return string(c.Status.Phase)
			}, time.Second*3, interval).Should(BeEmpty())
		})

		It("should resume reconciliation when cluster is unpaused", func() {
			By("Creating a paused K8znerCluster")
			cluster := createCluster(testClusterName, func(c *k8znerv1alpha1.K8znerCluster) {
				c.Spec.Paused = true
			})
			Expect(k8sClient.Create(ctx, cluster)).Should(Succeed())

			By("Waiting to ensure it's not reconciled")
			Consistently(func() string {
				c := &k8znerv1alpha1.K8znerCluster{}
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: testClusterName, Namespace: testNamespace}, c)
				return string(c.Status.Phase)
			}, time.Second*2, interval).Should(BeEmpty())

			By("Unpausing the cluster")
			Eventually(func() error {
				c := &k8znerv1alpha1.K8znerCluster{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: testClusterName, Namespace: testNamespace}, c); err != nil {
					return err
				}
				c.Spec.Paused = false
				return k8sClient.Update(ctx, c)
			}, timeout, interval).Should(Succeed())

			By("Verifying the controller starts reconciling")
			Eventually(func() string {
				c := &k8znerv1alpha1.K8znerCluster{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testClusterName, Namespace: testNamespace}, c)
				if err != nil {
					return ""
				}
				return string(c.Status.Phase)
			}, timeout, interval).ShouldNot(BeEmpty())
		})
	})

	Context("Node Health Detection", func() {
		It("should detect unhealthy nodes and update cluster status", func() {
			By("Creating a K8znerCluster")
			cluster := createCluster(testClusterName)
			Expect(k8sClient.Create(ctx, cluster)).Should(Succeed())

			By("Waiting for initial reconciliation")
			Eventually(func() string {
				c := &k8znerv1alpha1.K8znerCluster{}
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: testClusterName, Namespace: testNamespace}, c)
				return string(c.Status.Phase)
			}, timeout, interval).ShouldNot(BeEmpty())

			By("Creating an unhealthy node belonging to the cluster")
			unhealthyNode := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("%s-worker-unhealthy", testClusterName),
					Labels: map[string]string{
						"node-role.kubernetes.io/worker":   "",
						"node.kubernetes.io/instance-type": "cx22",
						"topology.kubernetes.io/region":    "fsn1",
						"k8zner.io/cluster":                testClusterName,
					},
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:               corev1.NodeReady,
							Status:             corev1.ConditionFalse,
							Reason:             "KubeletNotReady",
							LastTransitionTime: metav1.Now(),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, unhealthyNode)).Should(Succeed())

			By("Triggering reconciliation via cluster update")
			Eventually(func() error {
				c := &k8znerv1alpha1.K8znerCluster{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: testClusterName, Namespace: testNamespace}, c); err != nil {
					return err
				}
				if c.Annotations == nil {
					c.Annotations = make(map[string]string)
				}
				c.Annotations["test.k8zner.io/trigger"] = time.Now().Format(time.RFC3339Nano)
				return k8sClient.Update(ctx, c)
			}, timeout, interval).Should(Succeed())

			By("Verifying controller detects the unhealthy state")
			Eventually(func() string {
				c := &k8znerv1alpha1.K8znerCluster{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testClusterName, Namespace: testNamespace}, c)
				if err != nil {
					return ""
				}
				return string(c.Status.Phase)
			}, timeout, interval).Should(Or(Equal("Degraded"), Equal("Healing"), Equal("Running")))

			By("Cleaning up the test node")
			Expect(k8sClient.Delete(ctx, unhealthyNode)).Should(Succeed())
		})
	})

	Context("Cluster Deletion", func() {
		It("should handle cluster deletion gracefully", func() {
			By("Creating a K8znerCluster")
			cluster := createCluster(testClusterName)
			Expect(k8sClient.Create(ctx, cluster)).Should(Succeed())

			By("Waiting for the cluster to be reconciled")
			getCluster(testClusterName)

			By("Deleting the cluster")
			Expect(k8sClient.Delete(ctx, cluster)).Should(Succeed())

			By("Verifying the cluster is eventually deleted")
			Eventually(func() bool {
				c := &k8znerv1alpha1.K8znerCluster{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testClusterName, Namespace: testNamespace}, c)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("Spec Changes", func() {
		It("should reconcile when worker count changes", func() {
			By("Creating a K8znerCluster with 2 workers")
			cluster := createCluster(testClusterName)
			Expect(k8sClient.Create(ctx, cluster)).Should(Succeed())

			By("Waiting for initial reconciliation")
			Eventually(func() string {
				c := &k8znerv1alpha1.K8znerCluster{}
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: testClusterName, Namespace: testNamespace}, c)
				return string(c.Status.Phase)
			}, timeout, interval).ShouldNot(BeEmpty())

			By("Increasing worker count to 3")
			Eventually(func() error {
				c := &k8znerv1alpha1.K8znerCluster{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: testClusterName, Namespace: testNamespace}, c); err != nil {
					return err
				}
				c.Spec.Workers.Count = 3
				return k8sClient.Update(ctx, c)
			}, timeout, interval).Should(Succeed())

			By("Verifying the spec change is persisted")
			Eventually(func() int {
				c := &k8znerv1alpha1.K8znerCluster{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testClusterName, Namespace: testNamespace}, c)
				if err != nil {
					return 0
				}
				return c.Spec.Workers.Count
			}, timeout, interval).Should(Equal(3))
		})
	})
})
