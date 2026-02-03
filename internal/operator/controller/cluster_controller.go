// Package controller contains the Kubernetes controllers for the k8zner operator.
package controller

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	talosclient "github.com/siderolabs/talos/pkg/machinery/client"
	talosconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/addons"
	configv2 "github.com/imamik/k8zner/internal/config/v2"
	operatorprov "github.com/imamik/k8zner/internal/operator/provisioning"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/util/keygen"
	"github.com/imamik/k8zner/internal/util/naming"
)

const (
	// Default reconciliation interval.
	defaultRequeueAfter = 30 * time.Second

	// Default health check thresholds.
	defaultNodeNotReadyThreshold  = 3 * time.Minute
	defaultEtcdUnhealthyThreshold = 2 * time.Minute

	// Server creation timeouts.
	serverIPTimeout  = 2 * time.Minute
	nodeReadyTimeout = 5 * time.Minute

	// Status update retry settings.
	statusUpdateRetries = 3
	statusRetryInterval = 100 * time.Millisecond
	serverIPRetryDelay  = 5 * time.Second

	// Event reasons.
	EventReasonReconciling         = "Reconciling"
	EventReasonReconcileSucceeded  = "ReconcileSucceeded"
	EventReasonReconcileFailed     = "ReconcileFailed"
	EventReasonNodeUnhealthy       = "NodeUnhealthy"
	EventReasonNodeReplacing       = "NodeReplacing"
	EventReasonNodeReplaced        = "NodeReplaced"
	EventReasonQuorumLost          = "QuorumLost"
	EventReasonScalingUp           = "ScalingUp"
	EventReasonScalingDown         = "ScalingDown"
	EventReasonServerCreationError = "ServerCreationError"
	EventReasonConfigApplyError    = "ConfigApplyError"
	EventReasonNodeReadyTimeout    = "NodeReadyTimeout"

	// Provisioning event reasons.
	EventReasonProvisioningPhase     = "ProvisioningPhase"
	EventReasonInfrastructureCreated = "InfrastructureCreated"
	EventReasonInfrastructureFailed  = "InfrastructureFailed"
	EventReasonImageReady            = "ImageReady"
	EventReasonImageFailed           = "ImageFailed"
	EventReasonComputeProvisioned    = "ComputeProvisioned"
	EventReasonComputeFailed         = "ComputeFailed"
	EventReasonBootstrapComplete     = "BootstrapComplete"
	EventReasonBootstrapFailed       = "BootstrapFailed"
	EventReasonCNIInstalling         = "CNIInstalling"
	EventReasonCNIReady              = "CNIReady"
	EventReasonCNIFailed             = "CNIFailed"
	EventReasonAddonsInstalling      = "AddonsInstalling"
	EventReasonAddonsReady           = "AddonsReady"
	EventReasonAddonsFailed          = "AddonsFailed"
	EventReasonConfiguringComplete   = "ConfiguringComplete"
	EventReasonConfiguringFailed     = "ConfiguringFailed"
	EventReasonProvisioningComplete  = "ProvisioningComplete"
	EventReasonCredentialsError      = "CredentialsError"
)

// normalizeServerSize converts legacy server type names to current Hetzner names.
// For example, cx22 → cx23 (Hetzner renamed types in 2024).
func normalizeServerSize(size string) string {
	return string(configv2.ServerSize(size).Normalize())
}

// ClusterReconciler reconciles a K8znerCluster object.
type ClusterReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Dependencies (injected via options).
	hcloudClient       HCloudClient
	talosClient        TalosClient
	talosConfigGen     TalosConfigGenerator
	hcloudToken        string
	enableMetrics      bool
	maxConcurrentHeals int

	// Provisioning adapter for operator-driven provisioning.
	phaseAdapter *operatorprov.PhaseAdapter
}

// Option configures a ClusterReconciler.
type Option func(*ClusterReconciler)

// WithHCloudClient sets a custom HCloud client.
func WithHCloudClient(c HCloudClient) Option {
	return func(r *ClusterReconciler) {
		r.hcloudClient = c
	}
}

// WithTalosClient sets a custom Talos client.
func WithTalosClient(c TalosClient) Option {
	return func(r *ClusterReconciler) {
		r.talosClient = c
	}
}

// WithTalosConfigGenerator sets a custom Talos config generator.
func WithTalosConfigGenerator(g TalosConfigGenerator) Option {
	return func(r *ClusterReconciler) {
		r.talosConfigGen = g
	}
}

// WithHCloudToken sets the Hetzner Cloud API token for lazy client creation.
func WithHCloudToken(token string) Option {
	return func(r *ClusterReconciler) {
		r.hcloudToken = token
	}
}

// WithMetrics enables Prometheus metrics.
func WithMetrics(enable bool) Option {
	return func(r *ClusterReconciler) {
		r.enableMetrics = enable
	}
}

// WithMaxConcurrentHeals sets the maximum concurrent healing operations.
func WithMaxConcurrentHeals(maxHeals int) Option {
	return func(r *ClusterReconciler) {
		r.maxConcurrentHeals = maxHeals
	}
}

// NewClusterReconciler creates a new ClusterReconciler with the given options.
func NewClusterReconciler(c client.Client, scheme *runtime.Scheme, recorder record.EventRecorder, opts ...Option) *ClusterReconciler {
	r := &ClusterReconciler{
		Client:             c,
		Scheme:             scheme,
		Recorder:           recorder,
		enableMetrics:      true,
		maxConcurrentHeals: 1,
		phaseAdapter:       operatorprov.NewPhaseAdapter(c),
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// ensureHCloudClient ensures the HCloud client is initialized.
func (r *ClusterReconciler) ensureHCloudClient() error {
	if r.hcloudClient != nil {
		return nil
	}
	if r.hcloudToken == "" {
		return fmt.Errorf("HCloud token not configured")
	}
	r.hcloudClient = hcloud.NewRealClient(r.hcloudToken)
	return nil
}

// +kubebuilder:rbac:groups=k8zner.io,resources=k8znerclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=k8zner.io,resources=k8znerclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=k8zner.io,resources=k8znerclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=pods/eviction,verbs=create
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;create;update

// Reconcile handles the reconciliation loop for K8znerCluster resources.
func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("cluster", req.Name)
	ctx = log.IntoContext(ctx, logger)

	startTime := time.Now()
	defer func() {
		if r.enableMetrics {
			duration := time.Since(startTime).Seconds()
			RecordReconcile(req.Name, "completed", duration)
		}
	}()

	// Fetch the K8znerCluster
	cluster := &k8znerv1alpha1.K8znerCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("cluster resource not found, likely deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch K8znerCluster")
		return ctrl.Result{}, err
	}

	// Check if paused
	if cluster.Spec.Paused {
		logger.Info("cluster is paused, skipping reconciliation")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Ensure HCloud client is initialized
	if err := r.ensureHCloudClient(); err != nil {
		logger.Error(err, "failed to initialize HCloud client")
		r.Recorder.Event(cluster, corev1.EventTypeWarning, EventReasonReconcileFailed, err.Error())
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Record reconciliation start
	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonReconciling, "Starting reconciliation")

	// Run the reconciliation phases
	result, err := r.reconcile(ctx, cluster)

	// Update status with retry for conflict handling
	cluster.Status.LastReconcileTime = &metav1.Time{Time: time.Now()}
	cluster.Status.ObservedGeneration = cluster.Generation

	if statusErr := r.updateStatusWithRetry(ctx, cluster); statusErr != nil {
		logger.Error(statusErr, "failed to update status")
		if err == nil {
			err = statusErr
		}
	}

	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonReconcileFailed,
			"Reconciliation failed: %v", err)
		if r.enableMetrics {
			RecordReconcile(req.Name, "error", time.Since(startTime).Seconds())
		}
	} else {
		r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonReconcileSucceeded,
			"Reconciliation completed successfully")
	}

	return result, err
}

// updateStatusWithRetry updates the cluster status with retry on conflict.
// This handles the case where another reconcile has modified the object.
func (r *ClusterReconciler) updateStatusWithRetry(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) error {
	logger := log.FromContext(ctx)

	for i := 0; i < statusUpdateRetries; i++ {
		err := r.Status().Update(ctx, cluster)
		if err == nil {
			return nil
		}

		if !apierrors.IsConflict(err) {
			return err
		}

		// On conflict, re-fetch the latest version and re-apply our status changes
		logger.V(1).Info("status update conflict, retrying", "attempt", i+1)

		latest := &k8znerv1alpha1.K8znerCluster{}
		if getErr := r.Get(ctx, client.ObjectKeyFromObject(cluster), latest); getErr != nil {
			return fmt.Errorf("failed to get latest cluster for retry: %w", getErr)
		}

		// Preserve our status changes on the latest version
		latest.Status = cluster.Status
		cluster = latest

		time.Sleep(statusRetryInterval)
	}

	return fmt.Errorf("failed to update status after %d retries", statusUpdateRetries)
}

// reconcile runs the main reconciliation logic.
// It uses a state machine based on ProvisioningPhase for new clusters,
// and falls back to health-check-only mode for existing clusters without provisioning state.
func (r *ClusterReconciler) reconcile(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Always update the cluster phase before returning
	defer r.updateClusterPhase(cluster)

	// Always keep status.Desired in sync with spec counts
	// This ensures status reflects the desired state for both legacy and state-machine modes
	cluster.Status.ControlPlanes.Desired = cluster.Spec.ControlPlanes.Count
	cluster.Status.Workers.Desired = cluster.Spec.Workers.Count

	// Step 1: Check for stuck nodes and clean them up
	stuckNodes := r.checkStuckNodes(ctx, cluster)
	for _, stuck := range stuckNodes {
		if err := r.handleStuckNode(ctx, cluster, stuck); err != nil {
			logger.Error(err, "failed to handle stuck node", "node", stuck.Name)
		}
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, "NodeStuck",
			"Node %s stuck in %s phase for %s, cleaning up", stuck.Name, stuck.Phase, stuck.Elapsed.Round(time.Second))
	}

	// Step 2: Verify and update node states from external APIs
	// This catches nodes that have progressed without operator involvement
	if err := r.verifyAndUpdateNodeStates(ctx, cluster); err != nil {
		logger.Error(err, "failed to verify node states")
		// Continue with reconciliation - this is not fatal
	}

	// Check if this cluster needs provisioning (has credentialsRef set)
	if cluster.Spec.CredentialsRef.Name != "" {
		// This is an operator-managed cluster - use state machine
		return r.reconcileWithStateMachine(ctx, cluster)
	}

	// Legacy mode: health check + healing only (for clusters created before operator-centric architecture)
	return r.reconcileLegacy(ctx, cluster)
}

// reconcileWithStateMachine handles provisioning and ongoing management using a phase-based state machine.
func (r *ClusterReconciler) reconcileWithStateMachine(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Determine current phase (empty means start from beginning)
	currentPhase := cluster.Status.ProvisioningPhase
	if currentPhase == "" {
		// Check if CLI bootstrap completed - if so, start from CNI phase
		if cluster.Spec.Bootstrap.Completed {
			currentPhase = k8znerv1alpha1.PhaseCNI
			cluster.Status.ProvisioningPhase = k8znerv1alpha1.PhaseCNI
			logger.Info("bootstrap completed by CLI, starting from CNI phase")
		} else {
			// New cluster - start with infrastructure phase
			currentPhase = k8znerv1alpha1.PhaseInfrastructure
		}
	}

	logger.Info("reconciling with state machine", "phase", currentPhase)

	switch currentPhase {
	case k8znerv1alpha1.PhaseInfrastructure:
		return r.reconcileInfrastructurePhase(ctx, cluster)

	case k8znerv1alpha1.PhaseImage:
		return r.reconcileImagePhase(ctx, cluster)

	case k8znerv1alpha1.PhaseCompute:
		return r.reconcileComputePhase(ctx, cluster)

	case k8znerv1alpha1.PhaseBootstrap:
		return r.reconcileBootstrapPhase(ctx, cluster)

	case k8znerv1alpha1.PhaseCNI:
		return r.reconcileCNIPhase(ctx, cluster)

	case k8znerv1alpha1.PhaseAddons:
		return r.reconcileAddonsPhase(ctx, cluster)

	case k8znerv1alpha1.PhaseConfiguring:
		// Legacy phase - redirect to CNI for new operator-centric flow
		return r.reconcileConfiguringPhase(ctx, cluster)

	case k8znerv1alpha1.PhaseComplete:
		// Provisioning complete - switch to health monitoring mode
		return r.reconcileRunningPhase(ctx, cluster)

	default:
		logger.Info("unknown provisioning phase, resetting to infrastructure", "phase", currentPhase)
		cluster.Status.ProvisioningPhase = k8znerv1alpha1.PhaseInfrastructure
		return ctrl.Result{Requeue: true}, nil
	}
}

// reconcileLegacy handles health check and healing for legacy clusters.
func (r *ClusterReconciler) reconcileLegacy(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Phase 1: Health Check
	logger.V(1).Info("running health check phase")
	if err := r.reconcileHealthCheck(ctx, cluster); err != nil {
		return ctrl.Result{}, fmt.Errorf("health check failed: %w", err)
	}

	// Phase 2: Control Plane Reconciliation
	logger.V(1).Info("running control plane reconciliation")
	if result, err := r.reconcileControlPlanes(ctx, cluster); err != nil || result.Requeue || result.RequeueAfter > 0 {
		return result, err
	}

	// Phase 3: Worker Reconciliation
	logger.V(1).Info("running worker reconciliation")
	if result, err := r.reconcileWorkers(ctx, cluster); err != nil || result.Requeue || result.RequeueAfter > 0 {
		return result, err
	}

	// Requeue for continuous monitoring
	return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
}

// reconcileInfrastructurePhase creates network, firewall, and load balancer.
// If infrastructure already exists (from CLI bootstrap), it skips creation and proceeds to the next phase.
func (r *ClusterReconciler) reconcileInfrastructurePhase(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling infrastructure phase")

	cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseProvisioning
	cluster.Status.ProvisioningPhase = k8znerv1alpha1.PhaseInfrastructure

	// Check if infrastructure already exists (from CLI bootstrap)
	infra := cluster.Status.Infrastructure
	if infra.NetworkID != 0 && infra.LoadBalancerID != 0 && infra.FirewallID != 0 {
		logger.Info("infrastructure already exists from CLI bootstrap, skipping creation",
			"networkID", infra.NetworkID,
			"loadBalancerID", infra.LoadBalancerID,
			"firewallID", infra.FirewallID,
		)

		// Set the control plane endpoint from the LB IP if not already set
		if cluster.Status.ControlPlaneEndpoint == "" && infra.LoadBalancerIP != "" {
			cluster.Status.ControlPlaneEndpoint = infra.LoadBalancerIP
		}

		r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonInfrastructureCreated,
			"Using existing infrastructure from CLI bootstrap")

		// Transition to image phase
		cluster.Status.ProvisioningPhase = k8znerv1alpha1.PhaseImage
		return ctrl.Result{Requeue: true}, nil
	}

	// Infrastructure doesn't exist - need to create it
	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonProvisioningPhase,
		"Starting infrastructure provisioning")

	// Load credentials
	creds, err := r.phaseAdapter.LoadCredentials(ctx, cluster)
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonCredentialsError,
			"Failed to load credentials: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Build provisioning context
	pCtx, err := r.buildProvisioningContext(ctx, cluster, creds)
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonInfrastructureFailed,
			"Failed to build provisioning context: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Run infrastructure provisioning
	if err := r.phaseAdapter.ReconcileInfrastructure(pCtx, cluster); err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonInfrastructureFailed,
			"Infrastructure provisioning failed: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// If bootstrap node exists, attach it to infrastructure
	if cluster.Spec.Bootstrap != nil && cluster.Spec.Bootstrap.Completed {
		if err := r.phaseAdapter.AttachBootstrapNodeToInfrastructure(pCtx, cluster); err != nil {
			logger.Error(err, "failed to attach bootstrap node to infrastructure")
			// Non-fatal - continue to next phase
		}
	}

	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonInfrastructureCreated,
		"Infrastructure provisioned successfully")

	// Transition to image phase
	cluster.Status.ProvisioningPhase = k8znerv1alpha1.PhaseImage
	return ctrl.Result{Requeue: true}, nil
}

// reconcileImagePhase ensures the Talos image snapshot exists.
func (r *ClusterReconciler) reconcileImagePhase(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling image phase")

	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonProvisioningPhase,
		"Ensuring Talos image is available")

	// Load credentials
	creds, err := r.phaseAdapter.LoadCredentials(ctx, cluster)
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonCredentialsError,
			"Failed to load credentials: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Build provisioning context
	pCtx, err := r.buildProvisioningContext(ctx, cluster, creds)
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonImageFailed,
			"Failed to build provisioning context: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Run image provisioning
	if err := r.phaseAdapter.ReconcileImage(pCtx, cluster); err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonImageFailed,
			"Image provisioning failed: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonImageReady,
		"Talos image is available")

	// Transition to compute phase
	cluster.Status.ProvisioningPhase = k8znerv1alpha1.PhaseCompute
	return ctrl.Result{Requeue: true}, nil
}

// reconcileComputePhase provisions control plane and worker servers.
func (r *ClusterReconciler) reconcileComputePhase(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling compute phase")

	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonProvisioningPhase,
		"Provisioning compute resources")

	// Load credentials
	creds, err := r.phaseAdapter.LoadCredentials(ctx, cluster)
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonCredentialsError,
			"Failed to load credentials: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Build provisioning context
	pCtx, err := r.buildProvisioningContext(ctx, cluster, creds)
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonComputeFailed,
			"Failed to build provisioning context: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Run compute provisioning
	if err := r.phaseAdapter.ReconcileCompute(pCtx, cluster); err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonComputeFailed,
			"Compute provisioning failed: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonComputeProvisioned,
		"Compute resources provisioned")

	// For CLI-bootstrapped clusters (coming from Addons phase), go directly to Complete
	// For operator-managed clusters (coming from Image phase), go to Bootstrap
	if cluster.Spec.Bootstrap != nil && cluster.Spec.Bootstrap.Completed {
		// CLI already bootstrapped, we just created additional nodes
		cluster.Status.ProvisioningPhase = k8znerv1alpha1.PhaseComplete
		cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseRunning
		r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonProvisioningComplete,
			"Cluster provisioning complete")
	} else {
		// Transition to bootstrap phase for operator-managed clusters
		cluster.Status.ProvisioningPhase = k8znerv1alpha1.PhaseBootstrap
	}
	return ctrl.Result{Requeue: true}, nil
}

// reconcileBootstrapPhase applies Talos configs and bootstraps the cluster.
func (r *ClusterReconciler) reconcileBootstrapPhase(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling bootstrap phase")

	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonProvisioningPhase,
		"Bootstrapping cluster")

	// Load credentials
	creds, err := r.phaseAdapter.LoadCredentials(ctx, cluster)
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonCredentialsError,
			"Failed to load credentials: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Build provisioning context
	pCtx, err := r.buildProvisioningContext(ctx, cluster, creds)
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonBootstrapFailed,
			"Failed to build provisioning context: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Run cluster bootstrap
	if err := r.phaseAdapter.ReconcileBootstrap(pCtx, cluster); err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonBootstrapFailed,
			"Cluster bootstrap failed: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonBootstrapComplete,
		"Cluster bootstrapped successfully")

	// Transition to CNI phase (operator-centric flow installs Cilium first)
	cluster.Status.ProvisioningPhase = k8znerv1alpha1.PhaseCNI
	return ctrl.Result{Requeue: true}, nil
}

// reconcileConfiguringPhase installs addons and finalizes cluster configuration.
func (r *ClusterReconciler) reconcileConfiguringPhase(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling configuring phase")

	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonProvisioningPhase,
		"Configuring cluster (installing addons)")

	// Load credentials
	creds, err := r.phaseAdapter.LoadCredentials(ctx, cluster)
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonCredentialsError,
			"Failed to load credentials: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Convert CRD spec to config for addon installation
	cfg, err := operatorprov.SpecToConfig(cluster, creds)
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfiguringFailed,
			"Failed to convert spec to config: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Ensure HCloud token is set (required for CCM/CSI)
	cfg.HCloudToken = creds.HCloudToken

	// Get kubeconfig from Talos
	kubeconfig, err := r.getKubeconfigFromTalos(ctx, cluster, creds)
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfiguringFailed,
			"Failed to get kubeconfig: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Get network ID from status, or look it up from HCloud if not set
	networkID := cluster.Status.Infrastructure.NetworkID
	if networkID == 0 {
		// Network ID not in status - look it up from HCloud by cluster name
		logger.Info("networkID not in status, looking up from HCloud", "clusterName", cluster.Name)
		network, err := r.hcloudClient.GetNetwork(ctx, cluster.Name)
		if err != nil {
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfiguringFailed,
				"Failed to get network from HCloud: %v", err)
			return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
		}
		if network == nil {
			r.Recorder.Event(cluster, corev1.EventTypeWarning, EventReasonConfiguringFailed,
				"Network not found in HCloud - waiting for infrastructure")
			return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
		}
		networkID = network.ID
		// Update the status with the network ID for future reconciles
		cluster.Status.Infrastructure.NetworkID = networkID
		logger.Info("found network ID from HCloud", "networkID", networkID)
	}

	// Install addons
	logger.Info("installing addons", "networkID", networkID)
	if err := addons.Apply(ctx, cfg, kubeconfig, networkID); err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfiguringFailed,
			"Failed to install addons: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonConfiguringComplete,
		"Cluster configuration complete")

	// Transition to complete phase
	cluster.Status.ProvisioningPhase = k8znerv1alpha1.PhaseComplete
	cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseRunning

	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonProvisioningComplete,
		"Cluster provisioning complete")

	return ctrl.Result{Requeue: true}, nil
}

// reconcileCNIPhase installs Cilium CNI as the first addon.
// This must complete before any other pods can be scheduled (except hostNetwork pods).
func (r *ClusterReconciler) reconcileCNIPhase(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling CNI phase")

	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonCNIInstalling,
		"Installing Cilium CNI")

	// Load credentials
	creds, err := r.phaseAdapter.LoadCredentials(ctx, cluster)
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonCredentialsError,
			"Failed to load credentials: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Convert CRD spec to config for CNI installation
	cfg, err := operatorprov.SpecToConfig(cluster, creds)
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonCNIFailed,
			"Failed to convert spec to config: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Get kubeconfig from Talos
	kubeconfig, err := r.getKubeconfigFromTalos(ctx, cluster, creds)
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonCNIFailed,
			"Failed to get kubeconfig: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Install Cilium CNI only
	logger.Info("installing Cilium CNI")
	if err := addons.ApplyCilium(ctx, cfg, kubeconfig); err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonCNIFailed,
			"Failed to install Cilium: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Update addon status
	now := metav1.Now()
	if cluster.Status.Addons == nil {
		cluster.Status.Addons = make(map[string]k8znerv1alpha1.AddonStatus)
	}
	cluster.Status.Addons[k8znerv1alpha1.AddonNameCilium] = k8znerv1alpha1.AddonStatus{
		Installed:          true,
		Healthy:            false, // Will be updated when we verify readiness
		Phase:              k8znerv1alpha1.AddonPhaseInstalling,
		LastTransitionTime: &now,
		InstallOrder:       k8znerv1alpha1.AddonOrderCilium,
	}

	// Wait for Cilium to be ready
	logger.Info("waiting for Cilium to be ready")
	if err := r.waitForCiliumReady(ctx, kubeconfig); err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonCNIFailed,
			"Cilium not ready: %v", err)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Update status to installed
	cluster.Status.Addons[k8znerv1alpha1.AddonNameCilium] = k8znerv1alpha1.AddonStatus{
		Installed:          true,
		Healthy:            true,
		Phase:              k8znerv1alpha1.AddonPhaseInstalled,
		LastTransitionTime: &now,
		InstallOrder:       k8znerv1alpha1.AddonOrderCilium,
	}

	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonCNIReady,
		"Cilium CNI is ready")

	// Transition to addons phase
	cluster.Status.ProvisioningPhase = k8znerv1alpha1.PhaseAddons
	return ctrl.Result{Requeue: true}, nil
}

// waitForCiliumReady waits for Cilium pods to be ready.
func (r *ClusterReconciler) waitForCiliumReady(ctx context.Context, kubeconfig []byte) error {
	// Create a client from kubeconfig
	restConfig, err := clientConfigFromKubeconfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create rest config: %w", err)
	}

	k8sClient, err := client.New(restConfig, client.Options{})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Check for Cilium daemonset readiness
	// Use a timeout context
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-checkCtx.Done():
			return fmt.Errorf("timeout waiting for Cilium to be ready")
		case <-ticker.C:
			// Check Cilium pods in kube-system namespace
			podList := &corev1.PodList{}
			if err := k8sClient.List(ctx, podList, client.InNamespace("kube-system"), client.MatchingLabels{"k8s-app": "cilium"}); err != nil {
				continue // Retry on error
			}

			if len(podList.Items) == 0 {
				continue // No pods yet
			}

			// Check if all Cilium pods are ready
			allReady := true
			for _, pod := range podList.Items {
				if pod.Status.Phase != corev1.PodRunning {
					allReady = false
					break
				}
				for _, cond := range pod.Status.Conditions {
					if cond.Type == corev1.PodReady && cond.Status != corev1.ConditionTrue {
						allReady = false
						break
					}
				}
			}

			if allReady {
				return nil
			}
		}
	}
}

// reconcileAddonsPhase installs remaining addons after CNI is ready.
func (r *ClusterReconciler) reconcileAddonsPhase(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling addons phase")

	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonAddonsInstalling,
		"Installing cluster addons")

	// Load credentials
	creds, err := r.phaseAdapter.LoadCredentials(ctx, cluster)
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonCredentialsError,
			"Failed to load credentials: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Convert CRD spec to config for addon installation
	cfg, err := operatorprov.SpecToConfig(cluster, creds)
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonAddonsFailed,
			"Failed to convert spec to config: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Ensure HCloud token is set (required for CCM/CSI)
	cfg.HCloudToken = creds.HCloudToken

	// Get kubeconfig from Talos
	kubeconfig, err := r.getKubeconfigFromTalos(ctx, cluster, creds)
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonAddonsFailed,
			"Failed to get kubeconfig: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Get network ID from status, or look it up from HCloud if not set
	networkID := cluster.Status.Infrastructure.NetworkID
	if networkID == 0 {
		// Network ID not in status - look it up from HCloud by cluster name
		logger.Info("networkID not in status, looking up from HCloud", "clusterName", cluster.Name)
		network, err := r.hcloudClient.GetNetwork(ctx, cluster.Name)
		if err != nil {
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonAddonsFailed,
				"Failed to get network from HCloud: %v", err)
			return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
		}
		if network == nil {
			r.Recorder.Event(cluster, corev1.EventTypeWarning, EventReasonAddonsFailed,
				"Network not found in HCloud - waiting for infrastructure")
			return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
		}
		networkID = network.ID
		// Update the status with the network ID for future reconciles
		cluster.Status.Infrastructure.NetworkID = networkID
		logger.Info("found network ID from HCloud", "networkID", networkID)
	}

	// Install remaining addons (Cilium already installed in CNI phase)
	logger.Info("installing addons", "networkID", networkID)
	if err := addons.ApplyWithoutCilium(ctx, cfg, kubeconfig, networkID); err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonAddonsFailed,
			"Failed to install addons: %v", err)
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Update addon statuses
	now := metav1.Now()
	if cluster.Status.Addons == nil {
		cluster.Status.Addons = make(map[string]k8znerv1alpha1.AddonStatus)
	}

	// Mark core addons as installed
	cluster.Status.Addons[k8znerv1alpha1.AddonNameCCM] = k8znerv1alpha1.AddonStatus{
		Installed:          true,
		Healthy:            true,
		Phase:              k8znerv1alpha1.AddonPhaseInstalled,
		LastTransitionTime: &now,
		InstallOrder:       k8znerv1alpha1.AddonOrderCCM,
	}
	cluster.Status.Addons[k8znerv1alpha1.AddonNameCSI] = k8znerv1alpha1.AddonStatus{
		Installed:          true,
		Healthy:            true,
		Phase:              k8znerv1alpha1.AddonPhaseInstalled,
		LastTransitionTime: &now,
		InstallOrder:       k8znerv1alpha1.AddonOrderCSI,
	}

	// Update optional addons based on spec
	if cfg.Addons.MetricsServer.Enabled {
		cluster.Status.Addons[k8znerv1alpha1.AddonNameMetricsServer] = k8znerv1alpha1.AddonStatus{
			Installed:          true,
			Healthy:            true,
			Phase:              k8znerv1alpha1.AddonPhaseInstalled,
			LastTransitionTime: &now,
			InstallOrder:       k8znerv1alpha1.AddonOrderMetricsServer,
		}
	}
	if cfg.Addons.CertManager.Enabled {
		cluster.Status.Addons[k8znerv1alpha1.AddonNameCertManager] = k8znerv1alpha1.AddonStatus{
			Installed:          true,
			Healthy:            true,
			Phase:              k8znerv1alpha1.AddonPhaseInstalled,
			LastTransitionTime: &now,
			InstallOrder:       k8znerv1alpha1.AddonOrderCertManager,
		}
	}
	if cfg.Addons.Traefik.Enabled {
		cluster.Status.Addons[k8znerv1alpha1.AddonNameTraefik] = k8znerv1alpha1.AddonStatus{
			Installed:          true,
			Healthy:            true,
			Phase:              k8znerv1alpha1.AddonPhaseInstalled,
			LastTransitionTime: &now,
			InstallOrder:       k8znerv1alpha1.AddonOrderTraefik,
		}
	}
	if cfg.Addons.ExternalDNS.Enabled {
		cluster.Status.Addons[k8znerv1alpha1.AddonNameExternalDNS] = k8znerv1alpha1.AddonStatus{
			Installed:          true,
			Healthy:            true,
			Phase:              k8znerv1alpha1.AddonPhaseInstalled,
			LastTransitionTime: &now,
			InstallOrder:       k8znerv1alpha1.AddonOrderExternalDNS,
		}
	}
	if cfg.Addons.ArgoCD.Enabled {
		cluster.Status.Addons[k8znerv1alpha1.AddonNameArgoCD] = k8znerv1alpha1.AddonStatus{
			Installed:          true,
			Healthy:            true,
			Phase:              k8znerv1alpha1.AddonPhaseInstalled,
			LastTransitionTime: &now,
			InstallOrder:       k8znerv1alpha1.AddonOrderArgoCD,
		}
	}

	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonAddonsReady,
		"All addons installed successfully")

	// Check if we need to create additional nodes (CLI-bootstrapped clusters start with 1 CP)
	needsMoreNodes := cluster.Spec.ControlPlanes.Count > len(cluster.Status.ControlPlanes.Nodes) ||
		cluster.Spec.Workers.Count > len(cluster.Status.Workers.Nodes)

	if needsMoreNodes {
		// Transition to compute phase to create remaining CPs and workers
		cluster.Status.ProvisioningPhase = k8znerv1alpha1.PhaseCompute
		return ctrl.Result{Requeue: true}, nil
	}

	// All nodes already exist, transition to complete phase
	cluster.Status.ProvisioningPhase = k8znerv1alpha1.PhaseComplete
	cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseRunning

	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonProvisioningComplete,
		"Cluster provisioning complete")

	return ctrl.Result{Requeue: true}, nil
}

// clientConfigFromKubeconfig creates a rest.Config from kubeconfig bytes.
func clientConfigFromKubeconfig(kubeconfig []byte) (*rest.Config, error) {
	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create client config: %w", err)
	}
	return clientConfig.ClientConfig()
}

// getKubeconfigFromTalos retrieves the kubeconfig from the Talos API.
func (r *ClusterReconciler) getKubeconfigFromTalos(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, creds *operatorprov.Credentials) ([]byte, error) {
	logger := log.FromContext(ctx)

	// Parse Talos client config from credentials
	if len(creds.TalosConfig) == 0 {
		return nil, fmt.Errorf("talos config not available in credentials")
	}

	talosConfig, err := talosconfig.FromString(string(creds.TalosConfig))
	if err != nil {
		return nil, fmt.Errorf("failed to parse talos config: %w", err)
	}

	// Find a healthy control plane IP to connect to
	var endpoint string
	switch {
	case cluster.Status.ControlPlaneEndpoint != "":
		endpoint = cluster.Status.ControlPlaneEndpoint
	case cluster.Spec.Bootstrap != nil && cluster.Spec.Bootstrap.PublicIP != "":
		endpoint = cluster.Spec.Bootstrap.PublicIP
	default:
		// Try to find from control plane nodes
		for _, node := range cluster.Status.ControlPlanes.Nodes {
			if node.Healthy && node.PublicIP != "" {
				endpoint = node.PublicIP
				break
			}
		}
	}

	if endpoint == "" {
		return nil, fmt.Errorf("no control plane endpoint available")
	}

	logger.Info("retrieving kubeconfig from Talos", "endpoint", endpoint)

	// Create Talos client
	talosClient, err := talosclient.New(ctx,
		talosclient.WithConfig(talosConfig),
		talosclient.WithEndpoints(endpoint),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Talos client: %w", err)
	}
	defer func() { _ = talosClient.Close() }()

	// Get kubeconfig with timeout
	kubeconfigCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	kubeconfig, err := talosClient.Kubeconfig(kubeconfigCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve kubeconfig: %w", err)
	}

	return kubeconfig, nil
}

// reconcileRunningPhase handles health monitoring and scaling for a running cluster.
func (r *ClusterReconciler) reconcileRunningPhase(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("reconciling running phase (health monitoring)")

	// Run health check
	if err := r.reconcileHealthCheck(ctx, cluster); err != nil {
		return ctrl.Result{}, fmt.Errorf("health check failed: %w", err)
	}

	// Control plane reconciliation
	if result, err := r.reconcileControlPlanes(ctx, cluster); err != nil || result.Requeue || result.RequeueAfter > 0 {
		return result, err
	}

	// Worker reconciliation
	if result, err := r.reconcileWorkers(ctx, cluster); err != nil || result.Requeue || result.RequeueAfter > 0 {
		return result, err
	}

	// Requeue for continuous monitoring
	return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
}

// buildProvisioningContext creates a provisioning context for phase adapter methods.
func (r *ClusterReconciler) buildProvisioningContext(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, creds *operatorprov.Credentials) (*provisioning.Context, error) {
	// Create HCloud infrastructure manager
	infraManager := hcloud.NewRealClient(creds.HCloudToken)

	// Create Talos config producer from stored secrets
	talosProducer, err := r.phaseAdapter.CreateTalosGenerator(cluster, creds)
	if err != nil {
		return nil, fmt.Errorf("failed to create talos generator: %w", err)
	}

	pCtx, err := r.phaseAdapter.BuildProvisioningContext(ctx, cluster, creds, infraManager, talosProducer)
	if err != nil {
		return nil, err
	}

	// Populate network state by looking up network by cluster name
	// This is required when the operator is picking up from CLI bootstrap
	// Network name always follows the cluster naming convention: {clusterName}
	if pCtx.State.Network == nil {
		network, err := infraManager.GetNetwork(ctx, cluster.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get network: %w", err)
		}
		pCtx.State.Network = network

		// Also populate the infrastructure status if it's missing
		if cluster.Status.Infrastructure.NetworkID == 0 && network != nil {
			cluster.Status.Infrastructure.NetworkID = network.ID
		}
	}

	return pCtx, nil
}

// reconcileHealthCheck checks the health of all nodes and updates status.
func (r *ClusterReconciler) reconcileHealthCheck(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) error {
	logger := log.FromContext(ctx)

	// List all nodes in the cluster
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	// Categorize nodes by role
	var cpNodes, workerNodes []corev1.Node
	for _, node := range nodeList.Items {
		if _, isCP := node.Labels["node-role.kubernetes.io/control-plane"]; isCP {
			cpNodes = append(cpNodes, node)
		} else {
			workerNodes = append(workerNodes, node)
		}
	}

	// Update control plane status
	cluster.Status.ControlPlanes = r.buildNodeGroupStatus(ctx, cluster, cpNodes, cluster.Spec.ControlPlanes.Count, "control-plane")

	// Update worker status
	cluster.Status.Workers = r.buildNodeGroupStatus(ctx, cluster, workerNodes, cluster.Spec.Workers.Count, "worker")

	// Record metrics
	if r.enableMetrics {
		RecordNodeCounts(cluster.Name, "control-plane",
			len(cpNodes), cluster.Status.ControlPlanes.Ready, cluster.Spec.ControlPlanes.Count)
		RecordNodeCounts(cluster.Name, "worker",
			len(workerNodes), cluster.Status.Workers.Ready, cluster.Spec.Workers.Count)
	}

	logger.Info("health check complete",
		"controlPlanes", fmt.Sprintf("%d/%d", cluster.Status.ControlPlanes.Ready, cluster.Status.ControlPlanes.Desired),
		"workers", fmt.Sprintf("%d/%d", cluster.Status.Workers.Ready, cluster.Status.Workers.Desired),
	)

	return nil
}

// buildNodeGroupStatus builds the status for a group of nodes.
func (r *ClusterReconciler) buildNodeGroupStatus(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, nodes []corev1.Node, desired int, role string) k8znerv1alpha1.NodeGroupStatus {
	status := k8znerv1alpha1.NodeGroupStatus{
		Desired: desired,
		Nodes:   make([]k8znerv1alpha1.NodeStatus, 0, len(nodes)),
	}

	now := metav1.Now()

	for _, node := range nodes {
		nodeStatus := k8znerv1alpha1.NodeStatus{
			Name:            node.Name,
			LastHealthCheck: &now,
		}

		// Extract server ID from provider ID (format: hcloud://12345)
		if node.Spec.ProviderID != "" {
			var serverID int64
			if _, err := fmt.Sscanf(node.Spec.ProviderID, "hcloud://%d", &serverID); err == nil {
				nodeStatus.ServerID = serverID
			}
		}

		// Get IPs
		for _, addr := range node.Status.Addresses {
			switch addr.Type {
			case corev1.NodeInternalIP:
				nodeStatus.PrivateIP = addr.Address
			case corev1.NodeExternalIP:
				nodeStatus.PublicIP = addr.Address
			}
		}

		// Check health
		nodeStatus.Healthy = isNodeReady(&node)
		if !nodeStatus.Healthy {
			nodeStatus.UnhealthyReason = getNodeUnhealthyReason(&node)
			// Track when node became unhealthy
			for _, cond := range node.Status.Conditions {
				if cond.Type == corev1.NodeReady && cond.Status != corev1.ConditionTrue {
					nodeStatus.UnhealthySince = &cond.LastTransitionTime
					break
				}
			}
			status.Unhealthy++

			// Record event for newly unhealthy nodes
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonNodeUnhealthy,
				"%s node %s is unhealthy: %s", role, node.Name, nodeStatus.UnhealthyReason)
		} else {
			status.Ready++
		}

		status.Nodes = append(status.Nodes, nodeStatus)
	}

	return status
}

// reconcileControlPlanes ensures control planes are healthy.
func (r *ClusterReconciler) reconcileControlPlanes(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Skip if single CP (no HA replacement possible)
	if cluster.Spec.ControlPlanes.Count == 1 {
		return ctrl.Result{}, nil
	}

	// Find unhealthy control planes
	threshold := parseThreshold(cluster.Spec.HealthCheck, "etcd")
	var unhealthyCP *k8znerv1alpha1.NodeStatus

	for i := range cluster.Status.ControlPlanes.Nodes {
		node := &cluster.Status.ControlPlanes.Nodes[i]
		if !node.Healthy && node.UnhealthySince != nil {
			if time.Since(node.UnhealthySince.Time) > threshold {
				unhealthyCP = node
				break // Only handle one at a time
			}
		}
	}

	if unhealthyCP == nil {
		return ctrl.Result{}, nil
	}

	// Check if we have quorum to replace
	healthyCPs := cluster.Status.ControlPlanes.Ready
	totalCPs := cluster.Spec.ControlPlanes.Count
	quorumNeeded := (totalCPs / 2) + 1

	if healthyCPs < quorumNeeded {
		logger.Error(nil, "cannot replace control plane - quorum would be lost",
			"healthy", healthyCPs,
			"needed", quorumNeeded,
		)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonQuorumLost,
			"Cannot replace control plane %s: only %d/%d healthy, need %d for quorum",
			unhealthyCP.Name, healthyCPs, totalCPs, quorumNeeded)

		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:    k8znerv1alpha1.ConditionControlPlaneReady,
			Status:  metav1.ConditionFalse,
			Reason:  "QuorumLost",
			Message: fmt.Sprintf("Only %d/%d control planes healthy, need %d for quorum", healthyCPs, totalCPs, quorumNeeded),
		})
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Replace the unhealthy control plane
	logger.Info("replacing unhealthy control plane",
		"node", unhealthyCP.Name,
		"serverID", unhealthyCP.ServerID,
		"unhealthySince", unhealthyCP.UnhealthySince,
	)

	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonNodeReplacing,
		"Replacing unhealthy control plane %s (unhealthy since %v)",
		unhealthyCP.Name, unhealthyCP.UnhealthySince)

	cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseHealing

	startTime := time.Now()
	if err := r.replaceControlPlane(ctx, cluster, unhealthyCP); err != nil {
		if r.enableMetrics {
			RecordNodeReplacement(cluster.Name, "control-plane", unhealthyCP.UnhealthyReason)
		}
		return ctrl.Result{}, fmt.Errorf("failed to replace control plane: %w", err)
	}

	if r.enableMetrics {
		RecordNodeReplacement(cluster.Name, "control-plane", unhealthyCP.UnhealthyReason)
		RecordNodeReplacementDuration(cluster.Name, "control-plane", time.Since(startTime).Seconds())
	}

	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonNodeReplaced,
		"Successfully replaced control plane %s", unhealthyCP.Name)

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// reconcileWorkers ensures workers are healthy and at the desired count.
func (r *ClusterReconciler) reconcileWorkers(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	threshold := parseThreshold(cluster.Spec.HealthCheck, "node")

	// Find unhealthy workers that need replacement
	var unhealthyWorkers []*k8znerv1alpha1.NodeStatus
	for i := range cluster.Status.Workers.Nodes {
		node := &cluster.Status.Workers.Nodes[i]
		if !node.Healthy && node.UnhealthySince != nil {
			if time.Since(node.UnhealthySince.Time) > threshold {
				unhealthyWorkers = append(unhealthyWorkers, node)
			}
		}
	}

	// Replace unhealthy workers (respect maxConcurrentHeals)
	replaced := 0
	for _, worker := range unhealthyWorkers {
		if replaced >= r.maxConcurrentHeals {
			break
		}

		logger.Info("replacing unhealthy worker",
			"node", worker.Name,
			"serverID", worker.ServerID,
			"unhealthySince", worker.UnhealthySince,
		)

		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonNodeReplacing,
			"Replacing unhealthy worker %s (unhealthy since %v)",
			worker.Name, worker.UnhealthySince)

		cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseHealing

		startTime := time.Now()
		if err := r.replaceWorker(ctx, cluster, worker); err != nil {
			logger.Error(err, "failed to replace worker", "node", worker.Name)
			continue
		}

		if r.enableMetrics {
			RecordNodeReplacement(cluster.Name, "worker", worker.UnhealthyReason)
			RecordNodeReplacementDuration(cluster.Name, "worker", time.Since(startTime).Seconds())
		}

		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonNodeReplaced,
			"Successfully replaced worker %s", worker.Name)
		replaced++
	}

	// Check if scaling is needed
	currentCount := len(cluster.Status.Workers.Nodes)
	provisioningCount := countWorkersInEarlyProvisioning(cluster.Status.Workers.Nodes)
	desiredCount := cluster.Spec.Workers.Count

	// Skip scaling if workers are already provisioning to prevent duplicate server creation
	// from concurrent reconciles seeing stale status
	if provisioningCount > 0 {
		logger.Info("workers currently provisioning, skipping scaling check",
			"provisioning", provisioningCount,
			"current", currentCount,
			"desired", desiredCount,
		)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if currentCount < desiredCount {
		logger.Info("scaling up workers", "current", currentCount, "desired", desiredCount)
		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonScalingUp,
			"Scaling up workers: %d -> %d", currentCount, desiredCount)

		// Only attempt scaling if HCloud client is configured
		if r.hcloudClient != nil {
			cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseHealing
			toCreate := desiredCount - currentCount
			if err := r.scaleUpWorkers(ctx, cluster, toCreate); err != nil {
				logger.Error(err, "failed to scale up workers")
				// Continue to allow status update, will retry on next reconcile
			}
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	} else if currentCount > desiredCount {
		logger.Info("scaling down workers", "current", currentCount, "desired", desiredCount)
		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonScalingDown,
			"Scaling down workers: %d -> %d", currentCount, desiredCount)

		// Only attempt scaling if HCloud client is configured
		if r.hcloudClient != nil {
			cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseHealing
			toRemove := currentCount - desiredCount
			if err := r.scaleDownWorkers(ctx, cluster, toRemove); err != nil {
				logger.Error(err, "failed to scale down workers")
				// Continue to allow status update, will retry on next reconcile
			}
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if len(unhealthyWorkers) > 0 {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// scaleUpWorkers creates new worker nodes to reach the desired count.
// Uses ephemeral SSH keys to avoid Hetzner password emails.
func (r *ClusterReconciler) scaleUpWorkers(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, count int) error {
	logger := log.FromContext(ctx)

	// Step 1: Build cluster state for server creation
	clusterState, err := r.buildClusterState(ctx, cluster)
	if err != nil {
		logger.Error(err, "failed to build cluster state")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to build cluster state for worker scaling: %v", err)
		return fmt.Errorf("failed to build cluster state: %w", err)
	}

	// Step 1b: Use injected clients if available (for testing), otherwise load from credentials
	talosConfigGen := r.talosConfigGen
	talosClient := r.talosClient
	if talosClient == nil && cluster.Spec.CredentialsRef.Name != "" {
		creds, err := r.phaseAdapter.LoadCredentials(ctx, cluster)
		if err != nil {
			logger.Error(err, "failed to load credentials for Talos config generation")
			// Continue without Talos config - server will be created but not configured
		} else {
			// Create Talos config generator if not injected
			if talosConfigGen == nil {
				generator, err := r.phaseAdapter.CreateTalosGenerator(cluster, creds)
				if err != nil {
					logger.Error(err, "failed to create Talos config generator")
				} else {
					talosConfigGen = generator
				}
			}

			// Create Talos client if we have talosconfig
			if len(creds.TalosConfig) > 0 {
				talosClientInstance, err := NewRealTalosClient(creds.TalosConfig)
				if err != nil {
					logger.Error(err, "failed to create Talos client")
				} else {
					talosClient = talosClientInstance
				}
			}
		}
	}

	// Step 2: Get Talos snapshot for server creation
	snapshotLabels := map[string]string{"os": "talos"}
	snapshot, err := r.hcloudClient.GetSnapshotByLabels(ctx, snapshotLabels)
	if err != nil {
		logger.Error(err, "failed to get Talos snapshot")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to get Talos snapshot: %v", err)
		return fmt.Errorf("failed to get Talos snapshot: %w", err)
	}
	if snapshot == nil {
		err := fmt.Errorf("no Talos snapshot found with labels %v", snapshotLabels)
		logger.Error(err, "Talos snapshot not found")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Talos snapshot not found for worker scaling")
		return err
	}

	// Step 3: Create ephemeral SSH key for this batch of workers
	// This avoids Hetzner sending password emails
	sshKeyName := fmt.Sprintf("ephemeral-%s-worker-%d", cluster.Name, time.Now().Unix())
	logger.Info("creating ephemeral SSH key for worker scaling", "keyName", sshKeyName)

	keyPair, err := keygen.GenerateRSAKeyPair(2048)
	if err != nil {
		logger.Error(err, "failed to generate ephemeral SSH key")
		return fmt.Errorf("failed to generate ephemeral SSH key: %w", err)
	}

	sshKeyLabels := map[string]string{
		"cluster": cluster.Name,
		"type":    "ephemeral-worker",
	}

	_, err = r.hcloudClient.CreateSSHKey(ctx, sshKeyName, string(keyPair.PublicKey), sshKeyLabels)
	if err != nil {
		logger.Error(err, "failed to create ephemeral SSH key")
		return fmt.Errorf("failed to create ephemeral SSH key: %w", err)
	}

	// Schedule cleanup of the ephemeral SSH key (always runs)
	defer func() {
		logger.Info("cleaning up ephemeral SSH key", "keyName", sshKeyName)
		if err := r.hcloudClient.DeleteSSHKey(ctx, sshKeyName); err != nil {
			logger.Error(err, "failed to delete ephemeral SSH key", "keyName", sshKeyName)
		}
	}()

	// Step 4: Create workers (respect maxConcurrentHeals)
	// With random IDs, we don't need to track indexes - each name is unique
	created := 0
	for i := 0; i < count && created < r.maxConcurrentHeals; i++ {
		// Generate a unique server name with random ID
		newServerName := naming.Worker(cluster.Name)

		serverLabels := map[string]string{
			"cluster": cluster.Name,
			"role":    "worker",
			"pool":    "workers",
		}

		serverType := normalizeServerSize(cluster.Spec.Workers.Size)
		logger.Info("creating new worker server",
			"name", newServerName,
			"snapshot", snapshot.ID,
			"serverType", serverType,
		)

		// Track node phase: CreatingServer
		r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
			Name:   newServerName,
			Phase:  k8znerv1alpha1.NodePhaseCreatingServer,
			Reason: fmt.Sprintf("Creating HCloud server with snapshot %d", snapshot.ID),
		})

		startTime := time.Now()
		_, err = r.hcloudClient.CreateServer(
			ctx,
			newServerName,
			fmt.Sprintf("%d", snapshot.ID),
			serverType,
			cluster.Spec.Region,
			[]string{sshKeyName}, // Use ephemeral SSH key instead of clusterState.SSHKeyIDs
			serverLabels,
			"",  // userData
			nil, // placementGroupID
			clusterState.NetworkID,
			"",   // privateIP - let HCloud assign
			true, // enablePublicIPv4
			true, // enablePublicIPv6
		)
		if err != nil {
			if r.enableMetrics {
				RecordHCloudAPICall("create_server", "error", time.Since(startTime).Seconds())
			}
			logger.Error(err, "failed to create worker server", "name", newServerName)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
				"Failed to create worker server %s: %v", newServerName, err)
			// Update phase to Failed and remove from tracking
			r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
				Name:   newServerName,
				Phase:  k8znerv1alpha1.NodePhaseFailed,
				Reason: fmt.Sprintf("Failed to create server: %v", err),
			})
			// Continue trying to create remaining workers
			continue
		}
		if r.enableMetrics {
			RecordHCloudAPICall("create_server", "success", time.Since(startTime).Seconds())
		}
		logger.Info("created worker server", "name", newServerName)

		// CRITICAL: Persist status immediately to prevent duplicate server creation
		// from concurrent reconciles seeing stale status without this node
		if err := r.persistClusterStatus(ctx, cluster); err != nil {
			logger.Error(err, "failed to persist status after server creation", "name", newServerName)
		}

		// Step 5: Wait for server IP assignment
		r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
			Name:   newServerName,
			Phase:  k8znerv1alpha1.NodePhaseWaitingForIP,
			Reason: "Waiting for HCloud to assign IP address",
		})

		serverIP, err := r.waitForServerIP(ctx, newServerName, serverIPTimeout)
		if err != nil {
			logger.Error(err, "failed to get server IP", "name", newServerName)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
				"Failed to get IP for worker server %s: %v", newServerName, err)
			r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
				Name:   newServerName,
				Phase:  k8znerv1alpha1.NodePhaseFailed,
				Reason: fmt.Sprintf("Failed to get IP: %v", err),
			})
			// Clean up orphaned server
			if delErr := r.hcloudClient.DeleteServer(ctx, newServerName); delErr != nil {
				logger.Error(delErr, "failed to delete orphaned server", "name", newServerName)
			}
			r.removeNodeFromStatus(cluster, "worker", newServerName)
			continue
		}
		logger.Info("server IP assigned", "name", newServerName, "ip", serverIP)

		// Step 6: Get server ID for config generation
		serverIDStr, err := r.hcloudClient.GetServerID(ctx, newServerName)
		if err != nil {
			logger.Error(err, "failed to get server ID", "name", newServerName)
			r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
				Name:   newServerName,
				Phase:  k8znerv1alpha1.NodePhaseFailed,
				Reason: fmt.Sprintf("Failed to get server ID: %v", err),
			})
			// Clean up orphaned server
			if delErr := r.hcloudClient.DeleteServer(ctx, newServerName); delErr != nil {
				logger.Error(delErr, "failed to delete orphaned server", "name", newServerName)
			}
			r.removeNodeFromStatus(cluster, "worker", newServerName)
			continue
		}
		var serverID int64
		if _, err := fmt.Sscanf(serverIDStr, "%d", &serverID); err != nil {
			logger.Error(err, "failed to parse server ID", "name", newServerName)
			r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
				Name:   newServerName,
				Phase:  k8znerv1alpha1.NodePhaseFailed,
				Reason: fmt.Sprintf("Failed to parse server ID: %v", err),
			})
			// Clean up orphaned server
			if delErr := r.hcloudClient.DeleteServer(ctx, newServerName); delErr != nil {
				logger.Error(delErr, "failed to delete orphaned server", "name", newServerName)
			}
			r.removeNodeFromStatus(cluster, "worker", newServerName)
			continue
		}

		// Get private IP from server
		privateIP, _ := r.getPrivateIPFromServer(ctx, newServerName)

		// Use private IP for Talos communication if available (bypasses firewall restrictions)
		// This is important for operator-centric flow where the operator runs inside the cluster
		talosIP := serverIP
		if privateIP != "" {
			talosIP = privateIP
			logger.Info("using private IP for Talos communication", "name", newServerName, "privateIP", privateIP)
		}

		// Update status with server ID and IPs, persist to CRD
		if err := r.updateNodePhaseAndPersist(ctx, cluster, "worker", NodeStatusUpdate{
			Name:      newServerName,
			ServerID:  serverID,
			PublicIP:  serverIP,
			PrivateIP: privateIP,
			Phase:     k8znerv1alpha1.NodePhaseWaitingForTalosAPI,
			Reason:    fmt.Sprintf("Waiting for Talos API on %s:50000", talosIP),
		}); err != nil {
			logger.Error(err, "failed to persist node status", "name", newServerName)
		}

		// Step 7: Generate and apply Talos config (if talos clients are available)
		if talosConfigGen != nil && talosClient != nil {
			r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
				Name:   newServerName,
				Phase:  k8znerv1alpha1.NodePhaseApplyingTalosConfig,
				Reason: "Generating and applying Talos machine configuration",
			})

			machineConfig, err := talosConfigGen.GenerateWorkerConfig(newServerName, serverID)
			if err != nil {
				logger.Error(err, "failed to generate worker config", "name", newServerName)
				r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
					"Failed to generate config for worker %s: %v", newServerName, err)
				r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
					Name:   newServerName,
					Phase:  k8znerv1alpha1.NodePhaseFailed,
					Reason: fmt.Sprintf("Failed to generate Talos config: %v", err),
				})
				// Clean up orphaned server
				if delErr := r.hcloudClient.DeleteServer(ctx, newServerName); delErr != nil {
					logger.Error(delErr, "failed to delete orphaned server", "name", newServerName)
				}
				r.removeNodeFromStatus(cluster, "worker", newServerName)
				continue
			}

			logger.Info("applying Talos config to worker", "name", newServerName, "ip", talosIP)
			if err := talosClient.ApplyConfig(ctx, talosIP, machineConfig); err != nil {
				logger.Error(err, "failed to apply Talos config", "name", newServerName)
				r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
					"Failed to apply config to worker %s: %v", newServerName, err)
				r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
					Name:   newServerName,
					Phase:  k8znerv1alpha1.NodePhaseFailed,
					Reason: fmt.Sprintf("Failed to apply Talos config: %v", err),
				})
				// Clean up orphaned server
				if delErr := r.hcloudClient.DeleteServer(ctx, newServerName); delErr != nil {
					logger.Error(delErr, "failed to delete orphaned server", "name", newServerName)
				}
				r.removeNodeFromStatus(cluster, "worker", newServerName)
				continue
			}

			// Step 8: Wait for node to be ready
			r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
				Name:   newServerName,
				Phase:  k8znerv1alpha1.NodePhaseRebootingWithConfig,
				Reason: "Talos config applied, node is rebooting with new configuration",
			})

			logger.Info("waiting for worker node to become ready", "name", newServerName, "ip", talosIP)
			r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
				Name:   newServerName,
				Phase:  k8znerv1alpha1.NodePhaseWaitingForK8s,
				Reason: "Waiting for kubelet to register node with Kubernetes",
			})

			if err := talosClient.WaitForNodeReady(ctx, talosIP, int(nodeReadyTimeout.Seconds())); err != nil {
				logger.Error(err, "worker node not ready in time", "name", newServerName)
				r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonNodeReadyTimeout,
					"Worker node %s not ready in time: %v", newServerName, err)
				r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
					Name:   newServerName,
					Phase:  k8znerv1alpha1.NodePhaseFailed,
					Reason: fmt.Sprintf("Node not ready in time: %v", err),
				})
				// Clean up orphaned server
				if delErr := r.hcloudClient.DeleteServer(ctx, newServerName); delErr != nil {
					logger.Error(delErr, "failed to delete orphaned server", "name", newServerName)
				}
				r.removeNodeFromStatus(cluster, "worker", newServerName)
				continue
			}

			// Node kubelet is running - transition to NodeInitializing
			// The state verifier will promote to Ready once K8s node is fully ready
			r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
				Name:   newServerName,
				Phase:  k8znerv1alpha1.NodePhaseNodeInitializing,
				Reason: "Kubelet running, waiting for CNI and system pods",
			})
		} else {
			logger.Info("skipping Talos config application (no credentials available)", "name", newServerName)
			r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
				Name:   newServerName,
				Phase:  k8znerv1alpha1.NodePhaseWaitingForK8s,
				Reason: "Waiting for node to join cluster (no Talos credentials)",
			})
		}

		// Persist final status for this worker
		if err := r.persistClusterStatus(ctx, cluster); err != nil {
			logger.Error(err, "failed to persist cluster status", "name", newServerName)
		}

		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonScalingUp,
			"Successfully created worker %s", newServerName)
		created++

		if r.enableMetrics {
			RecordNodeReplacement(cluster.Name, "worker", "scale-up")
			RecordNodeReplacementDuration(cluster.Name, "worker", time.Since(startTime).Seconds())
		}
	}

	if created < count {
		return fmt.Errorf("only created %d of %d requested workers", created, count)
	}

	return nil
}

// scaleDownWorkers removes excess worker nodes to reach the desired count.
// Workers are selected for removal based on: unhealthy first, then newest.
func (r *ClusterReconciler) scaleDownWorkers(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, count int) error {
	logger := log.FromContext(ctx)

	// Select workers to remove (prefer unhealthy, then newest)
	workersToRemove := r.selectWorkersForRemoval(cluster, count)

	if len(workersToRemove) == 0 {
		logger.Info("no workers to remove")
		return nil
	}

	logger.Info("removing workers", "count", len(workersToRemove))

	removed := 0
	for _, worker := range workersToRemove {
		if removed >= r.maxConcurrentHeals {
			break
		}

		logger.Info("removing worker",
			"name", worker.Name,
			"serverID", worker.ServerID,
		)

		startTime := time.Now()

		// Step 1: Cordon the node
		k8sNode := &corev1.Node{}
		if err := r.Get(ctx, types.NamespacedName{Name: worker.Name}, k8sNode); err == nil {
			if !k8sNode.Spec.Unschedulable {
				k8sNode.Spec.Unschedulable = true
				if err := r.Update(ctx, k8sNode); err != nil {
					logger.Error(err, "failed to cordon node", "node", worker.Name)
				} else {
					logger.Info("cordoned node", "node", worker.Name)
				}
			}
		}

		// Step 2: Drain the node (evict pods)
		if err := r.drainNode(ctx, worker.Name); err != nil {
			logger.Error(err, "failed to drain node", "node", worker.Name)
			// Continue with removal anyway - pods will be rescheduled
		}

		// Step 3: Delete the Kubernetes node object
		if err := r.Get(ctx, types.NamespacedName{Name: worker.Name}, k8sNode); err == nil {
			if err := r.Delete(ctx, k8sNode); err != nil && !apierrors.IsNotFound(err) {
				logger.Error(err, "failed to delete k8s node", "node", worker.Name)
			} else {
				logger.Info("deleted kubernetes node", "node", worker.Name)
			}
		}

		// Step 4: Delete the Hetzner server
		if worker.Name != "" {
			if err := r.hcloudClient.DeleteServer(ctx, worker.Name); err != nil {
				logger.Error(err, "failed to delete hetzner server", "name", worker.Name)
				if r.enableMetrics {
					RecordHCloudAPICall("delete_server", "error", time.Since(startTime).Seconds())
				}
				// Continue with next worker
				continue
			}
			logger.Info("deleted hetzner server", "name", worker.Name, "serverID", worker.ServerID)
			if r.enableMetrics {
				RecordHCloudAPICall("delete_server", "success", time.Since(startTime).Seconds())
			}
		}

		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonScalingDown,
			"Successfully removed worker %s", worker.Name)
		removed++

		if r.enableMetrics {
			RecordNodeReplacement(cluster.Name, "worker", "scale-down")
			RecordNodeReplacementDuration(cluster.Name, "worker", time.Since(startTime).Seconds())
		}
	}

	// Update the status to remove the deleted workers
	r.removeWorkersFromStatus(cluster, workersToRemove[:removed])

	if removed < count {
		return fmt.Errorf("only removed %d of %d workers", removed, count)
	}

	return nil
}

// selectWorkersForRemoval selects workers to remove during scale-down.
// Priority: 1. Unhealthy workers, 2. Newest workers (by name, assuming newer names sort last)
func (r *ClusterReconciler) selectWorkersForRemoval(cluster *k8znerv1alpha1.K8znerCluster, count int) []*k8znerv1alpha1.NodeStatus {
	if count <= 0 || len(cluster.Status.Workers.Nodes) == 0 {
		return nil
	}

	// First, collect unhealthy workers
	var unhealthy []*k8znerv1alpha1.NodeStatus
	var healthy []*k8znerv1alpha1.NodeStatus

	for i := range cluster.Status.Workers.Nodes {
		node := &cluster.Status.Workers.Nodes[i]
		if !node.Healthy {
			unhealthy = append(unhealthy, node)
		} else {
			healthy = append(healthy, node)
		}
	}

	// Select from unhealthy first, then from healthy (newest first)
	// For healthy workers, we pick from the end of the list (newest)
	var selected []*k8znerv1alpha1.NodeStatus

	// Add unhealthy workers first
	for _, node := range unhealthy {
		if len(selected) >= count {
			break
		}
		selected = append(selected, node)
	}

	// If we still need more, select from healthy workers (newest first)
	for i := len(healthy) - 1; i >= 0 && len(selected) < count; i-- {
		selected = append(selected, healthy[i])
	}

	return selected
}

// removeWorkersFromStatus removes the specified workers from the cluster status.
func (r *ClusterReconciler) removeWorkersFromStatus(cluster *k8znerv1alpha1.K8znerCluster, removed []*k8znerv1alpha1.NodeStatus) {
	if len(removed) == 0 {
		return
	}

	// Create a set of names to remove
	toRemove := make(map[string]bool)
	for _, w := range removed {
		toRemove[w.Name] = true
	}

	// Filter out removed workers
	var remaining []k8znerv1alpha1.NodeStatus
	for _, node := range cluster.Status.Workers.Nodes {
		if !toRemove[node.Name] {
			remaining = append(remaining, node)
		}
	}

	cluster.Status.Workers.Nodes = remaining
}

// findNextWorkerIndex finds the next available worker index for the cluster.
func (r *ClusterReconciler) findNextWorkerIndex(cluster *k8znerv1alpha1.K8znerCluster) int {
	maxIndex := 0
	for _, node := range cluster.Status.Workers.Nodes {
		// Extract index from name like "{cluster}-workers-{index}"
		parts := strings.Split(node.Name, "-")
		if len(parts) >= 1 {
			if idx, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
				if idx >= maxIndex {
					maxIndex = idx + 1
				}
			}
		}
	}
	// If no workers exist, start at 1
	if maxIndex == 0 {
		maxIndex = 1
	}
	return maxIndex
}

// replaceControlPlane replaces an unhealthy control plane node.
func (r *ClusterReconciler) replaceControlPlane(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, node *k8znerv1alpha1.NodeStatus) error {
	logger := log.FromContext(ctx)

	// Use injected clients if available (for testing), otherwise load from credentials
	talosConfigGen := r.talosConfigGen
	talosClient := r.talosClient
	if talosClient == nil && cluster.Spec.CredentialsRef.Name != "" {
		creds, err := r.phaseAdapter.LoadCredentials(ctx, cluster)
		if err != nil {
			logger.Error(err, "failed to load credentials for Talos operations")
		} else {
			if talosConfigGen == nil {
				generator, err := r.phaseAdapter.CreateTalosGenerator(cluster, creds)
				if err != nil {
					logger.Error(err, "failed to create Talos config generator")
				} else {
					talosConfigGen = generator
				}
			}
			if len(creds.TalosConfig) > 0 {
				talosClientInstance, err := NewRealTalosClient(creds.TalosConfig)
				if err != nil {
					logger.Error(err, "failed to create Talos client")
				} else {
					talosClient = talosClientInstance
				}
			}
		}
	}

	// Step 1: Remove from etcd cluster (via Talos API)
	r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
		Name:   node.Name,
		Phase:  k8znerv1alpha1.NodePhaseRemovingFromEtcd,
		Reason: "Removing etcd member before server deletion",
	})

	if talosClient != nil && node.PrivateIP != "" {
		// Get etcd members from a healthy control plane
		healthyIP := r.findHealthyControlPlaneIP(cluster)
		if healthyIP != "" {
			members, err := talosClient.GetEtcdMembers(ctx, healthyIP)
			if err != nil {
				logger.Error(err, "failed to get etcd members")
			} else {
				for _, member := range members {
					if member.Name == node.Name || member.Endpoint == node.PrivateIP {
						if err := talosClient.RemoveEtcdMember(ctx, healthyIP, member.ID); err != nil {
							logger.Error(err, "failed to remove etcd member", "member", member.Name)
						} else {
							logger.Info("removed etcd member", "member", member.Name)
						}
						break
					}
				}
			}
		}
	} else {
		logger.Info("skipping etcd member removal (no credentials or no IP)")
	}

	// Step 2: Delete the Kubernetes node
	r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
		Name:   node.Name,
		Phase:  k8znerv1alpha1.NodePhaseDeletingServer,
		Reason: "Deleting Kubernetes node and HCloud server",
	})

	k8sNode := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: node.Name}, k8sNode); err == nil {
		if err := r.Delete(ctx, k8sNode); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete k8s node: %w", err)
		}
		logger.Info("deleted kubernetes node", "node", node.Name)
	}

	// Step 3: Delete the Hetzner server
	if node.Name != "" {
		startTime := time.Now()
		if err := r.hcloudClient.DeleteServer(ctx, node.Name); err != nil {
			logger.Error(err, "failed to delete hetzner server", "name", node.Name)
			if r.enableMetrics {
				RecordHCloudAPICall("delete_server", "error", time.Since(startTime).Seconds())
			}
			// Continue anyway - server might already be gone
		} else {
			logger.Info("deleted hetzner server", "name", node.Name, "serverID", node.ServerID)
			if r.enableMetrics {
				RecordHCloudAPICall("delete_server", "success", time.Since(startTime).Seconds())
			}
		}
	}

	// Remove old node from status
	r.removeNodeFromStatus(cluster, "control-plane", node.Name)

	// Step 4: Build cluster state for server creation
	clusterState, err := r.buildClusterState(ctx, cluster)
	if err != nil {
		logger.Error(err, "failed to build cluster state")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to build cluster state for control plane replacement: %v", err)
		return fmt.Errorf("failed to build cluster state: %w", err)
	}

	// Step 5: Get Talos snapshot for server creation
	snapshotLabels := map[string]string{"os": "talos"}
	snapshot, err := r.hcloudClient.GetSnapshotByLabels(ctx, snapshotLabels)
	if err != nil {
		logger.Error(err, "failed to get Talos snapshot")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to get Talos snapshot: %v", err)
		return fmt.Errorf("failed to get Talos snapshot: %w", err)
	}
	if snapshot == nil {
		err := fmt.Errorf("no Talos snapshot found with labels %v", snapshotLabels)
		logger.Error(err, "Talos snapshot not found")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Talos snapshot not found for control plane replacement")
		return err
	}

	// Step 5.5: Create ephemeral SSH key to avoid Hetzner password emails
	sshKeyName := fmt.Sprintf("ephemeral-%s-cp-%d", cluster.Name, time.Now().Unix())
	logger.Info("creating ephemeral SSH key for control plane replacement", "keyName", sshKeyName)

	keyPair, err := keygen.GenerateRSAKeyPair(2048)
	if err != nil {
		logger.Error(err, "failed to generate ephemeral SSH key")
		return fmt.Errorf("failed to generate ephemeral SSH key: %w", err)
	}

	sshKeyLabels := map[string]string{
		"cluster": cluster.Name,
		"type":    "ephemeral-cp",
	}

	_, err = r.hcloudClient.CreateSSHKey(ctx, sshKeyName, string(keyPair.PublicKey), sshKeyLabels)
	if err != nil {
		logger.Error(err, "failed to create ephemeral SSH key")
		return fmt.Errorf("failed to create ephemeral SSH key: %w", err)
	}

	// Schedule cleanup of the ephemeral SSH key
	defer func() {
		logger.Info("cleaning up ephemeral SSH key", "keyName", sshKeyName)
		if err := r.hcloudClient.DeleteSSHKey(ctx, sshKeyName); err != nil {
			logger.Error(err, "failed to delete ephemeral SSH key", "keyName", sshKeyName)
		}
	}()

	// Step 6: Create new server
	newServerName := r.generateReplacementServerName(cluster, "control-plane", node.Name)
	serverLabels := map[string]string{
		"cluster": cluster.Name,
		"role":    "control-plane",
		"pool":    "control-plane",
	}

	// Track new node phase: CreatingServer
	r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
		Name:   newServerName,
		Phase:  k8znerv1alpha1.NodePhaseCreatingServer,
		Reason: fmt.Sprintf("Creating replacement HCloud server with snapshot %d", snapshot.ID),
	})

	serverType := normalizeServerSize(cluster.Spec.ControlPlanes.Size)
	logger.Info("creating replacement control plane server",
		"name", newServerName,
		"snapshot", snapshot.ID,
		"serverType", serverType,
	)

	startTime := time.Now()
	_, err = r.hcloudClient.CreateServer(
		ctx,
		newServerName,
		fmt.Sprintf("%d", snapshot.ID), // image ID as string
		serverType,
		cluster.Spec.Region,
		[]string{sshKeyName}, // Use ephemeral SSH key
		serverLabels,
		"",  // userData
		nil, // placementGroupID
		clusterState.NetworkID,
		"",   // privateIP - let HCloud assign
		true, // enablePublicIPv4
		true, // enablePublicIPv6
	)
	if err != nil {
		if r.enableMetrics {
			RecordHCloudAPICall("create_server", "error", time.Since(startTime).Seconds())
		}
		logger.Error(err, "failed to create replacement control plane server", "name", newServerName)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to create replacement control plane server %s: %v", newServerName, err)
		r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
			Name:   newServerName,
			Phase:  k8znerv1alpha1.NodePhaseFailed,
			Reason: fmt.Sprintf("Failed to create server: %v", err),
		})
		return fmt.Errorf("failed to create server: %w", err)
	}
	if r.enableMetrics {
		RecordHCloudAPICall("create_server", "success", time.Since(startTime).Seconds())
	}
	logger.Info("created replacement control plane server", "name", newServerName)

	// Step 7: Wait for server IP assignment
	r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
		Name:   newServerName,
		Phase:  k8znerv1alpha1.NodePhaseWaitingForIP,
		Reason: "Waiting for HCloud to assign IP address",
	})

	serverIP, err := r.waitForServerIP(ctx, newServerName, serverIPTimeout)
	if err != nil {
		logger.Error(err, "failed to get server IP", "name", newServerName)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to get IP for replacement control plane server %s: %v", newServerName, err)
		r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
			Name:   newServerName,
			Phase:  k8znerv1alpha1.NodePhaseFailed,
			Reason: fmt.Sprintf("Failed to get IP: %v", err),
		})
		// Clean up orphaned server
		if delErr := r.hcloudClient.DeleteServer(ctx, newServerName); delErr != nil {
			logger.Error(delErr, "failed to delete orphaned server", "name", newServerName)
		}
		r.removeNodeFromStatus(cluster, "control-plane", newServerName)
		return fmt.Errorf("failed to get server IP: %w", err)
	}
	logger.Info("server IP assigned", "name", newServerName, "ip", serverIP)

	// Step 8: Get server ID for config generation
	serverIDStr, err := r.hcloudClient.GetServerID(ctx, newServerName)
	if err != nil {
		logger.Error(err, "failed to get server ID", "name", newServerName)
		r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
			Name:   newServerName,
			Phase:  k8znerv1alpha1.NodePhaseFailed,
			Reason: fmt.Sprintf("Failed to get server ID: %v", err),
		})
		// Clean up orphaned server
		if delErr := r.hcloudClient.DeleteServer(ctx, newServerName); delErr != nil {
			logger.Error(delErr, "failed to delete orphaned server", "name", newServerName)
		}
		r.removeNodeFromStatus(cluster, "control-plane", newServerName)
		return fmt.Errorf("failed to get server ID: %w", err)
	}
	var serverID int64
	if _, err := fmt.Sscanf(serverIDStr, "%d", &serverID); err != nil {
		r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
			Name:   newServerName,
			Phase:  k8znerv1alpha1.NodePhaseFailed,
			Reason: fmt.Sprintf("Failed to parse server ID: %v", err),
		})
		// Clean up orphaned server
		if delErr := r.hcloudClient.DeleteServer(ctx, newServerName); delErr != nil {
			logger.Error(delErr, "failed to delete orphaned server", "name", newServerName)
		}
		r.removeNodeFromStatus(cluster, "control-plane", newServerName)
		return fmt.Errorf("failed to parse server ID: %w", err)
	}

	// Get private IP from server
	privateIP, _ := r.getPrivateIPFromServer(ctx, newServerName)

	// Use private IP for Talos communication if available (bypasses firewall restrictions)
	// This is important for operator-centric flow where the operator runs inside the cluster
	talosIP := serverIP
	if privateIP != "" {
		talosIP = privateIP
		logger.Info("using private IP for Talos communication", "name", newServerName, "privateIP", privateIP)
	}

	// Update status with server ID and IPs, persist to CRD
	if err := r.updateNodePhaseAndPersist(ctx, cluster, "control-plane", NodeStatusUpdate{
		Name:      newServerName,
		ServerID:  serverID,
		PublicIP:  serverIP,
		PrivateIP: privateIP,
		Phase:     k8znerv1alpha1.NodePhaseWaitingForTalosAPI,
		Reason:    fmt.Sprintf("Waiting for Talos API on %s:50000", talosIP),
	}); err != nil {
		logger.Error(err, "failed to persist node status", "name", newServerName)
	}

	// Step 9: Generate and apply Talos config (if talos clients are available)
	if talosConfigGen != nil && talosClient != nil {
		r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
			Name:   newServerName,
			Phase:  k8znerv1alpha1.NodePhaseApplyingTalosConfig,
			Reason: "Generating and applying Talos machine configuration",
		})

		// Update SANs with new server IP
		sans := append([]string{}, clusterState.SANs...)
		sans = append(sans, serverIP)

		machineConfig, err := talosConfigGen.GenerateControlPlaneConfig(sans, newServerName, serverID)
		if err != nil {
			logger.Error(err, "failed to generate control plane config", "name", newServerName)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
				"Failed to generate config for control plane %s: %v", newServerName, err)
			r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
				Name:   newServerName,
				Phase:  k8znerv1alpha1.NodePhaseFailed,
				Reason: fmt.Sprintf("Failed to generate Talos config: %v", err),
			})
			// Clean up orphaned server
			if delErr := r.hcloudClient.DeleteServer(ctx, newServerName); delErr != nil {
				logger.Error(delErr, "failed to delete orphaned server", "name", newServerName)
			}
			r.removeNodeFromStatus(cluster, "control-plane", newServerName)
			return fmt.Errorf("failed to generate config: %w", err)
		}

		logger.Info("applying Talos config to control plane", "name", newServerName, "ip", talosIP)
		if err := talosClient.ApplyConfig(ctx, talosIP, machineConfig); err != nil {
			logger.Error(err, "failed to apply Talos config", "name", newServerName)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
				"Failed to apply config to control plane %s: %v", newServerName, err)
			r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
				Name:   newServerName,
				Phase:  k8znerv1alpha1.NodePhaseFailed,
				Reason: fmt.Sprintf("Failed to apply Talos config: %v", err),
			})
			// Clean up orphaned server
			if delErr := r.hcloudClient.DeleteServer(ctx, newServerName); delErr != nil {
				logger.Error(delErr, "failed to delete orphaned server", "name", newServerName)
			}
			r.removeNodeFromStatus(cluster, "control-plane", newServerName)
			return fmt.Errorf("failed to apply config: %w", err)
		}

		// Step 10: Wait for node to become ready
		r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
			Name:   newServerName,
			Phase:  k8znerv1alpha1.NodePhaseRebootingWithConfig,
			Reason: "Talos config applied, node is rebooting with new configuration",
		})

		logger.Info("waiting for control plane node to become ready", "name", newServerName, "ip", talosIP)
		r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
			Name:   newServerName,
			Phase:  k8znerv1alpha1.NodePhaseWaitingForK8s,
			Reason: "Waiting for kubelet to register node with Kubernetes",
		})

		if err := talosClient.WaitForNodeReady(ctx, talosIP, int(nodeReadyTimeout.Seconds())); err != nil {
			logger.Error(err, "control plane node failed to become ready", "name", newServerName)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonNodeReadyTimeout,
				"Control plane %s failed to become ready: %v", newServerName, err)
			r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
				Name:   newServerName,
				Phase:  k8znerv1alpha1.NodePhaseFailed,
				Reason: fmt.Sprintf("Node not ready in time: %v", err),
			})
			// Clean up orphaned server
			if delErr := r.hcloudClient.DeleteServer(ctx, newServerName); delErr != nil {
				logger.Error(delErr, "failed to delete orphaned server", "name", newServerName)
			}
			r.removeNodeFromStatus(cluster, "control-plane", newServerName)
			return fmt.Errorf("node failed to become ready: %w", err)
		}

		// Node kubelet is running - transition to NodeInitializing
		// The state verifier will promote to Ready once K8s node is fully ready
		r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
			Name:   newServerName,
			Phase:  k8znerv1alpha1.NodePhaseNodeInitializing,
			Reason: "Kubelet running, waiting for CNI and system pods",
		})

		logger.Info("control plane node kubelet is running", "name", newServerName)
	} else {
		logger.Info("skipping Talos config application (no credentials available)", "name", newServerName)
		r.updateNodePhase(ctx, cluster, "control-plane", NodeStatusUpdate{
			Name:   newServerName,
			Phase:  k8znerv1alpha1.NodePhaseWaitingForK8s,
			Reason: "Waiting for node to join cluster (no Talos credentials)",
		})
	}

	// Persist final status
	if err := r.persistClusterStatus(ctx, cluster); err != nil {
		logger.Error(err, "failed to persist cluster status", "name", newServerName)
	}

	return nil
}

// replaceWorker replaces an unhealthy worker node.
func (r *ClusterReconciler) replaceWorker(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, node *k8znerv1alpha1.NodeStatus) error {
	logger := log.FromContext(ctx)

	// Step 1: Cordon and drain the node
	r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
		Name:   node.Name,
		Phase:  k8znerv1alpha1.NodePhaseDraining,
		Reason: "Cordoning and draining node before replacement",
	})

	k8sNode := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: node.Name}, k8sNode); err == nil {
		if !k8sNode.Spec.Unschedulable {
			k8sNode.Spec.Unschedulable = true
			if err := r.Update(ctx, k8sNode); err != nil {
				logger.Error(err, "failed to cordon node", "node", node.Name)
			} else {
				logger.Info("cordoned node", "node", node.Name)
			}
		}
	}

	// Step 2: Drain the node (evict pods)
	if err := r.drainNode(ctx, node.Name); err != nil {
		logger.Error(err, "failed to drain node", "node", node.Name)
		// Continue with replacement anyway
	}

	// Step 3: Delete the Kubernetes node and server
	r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
		Name:   node.Name,
		Phase:  k8znerv1alpha1.NodePhaseDeletingServer,
		Reason: "Deleting Kubernetes node and HCloud server",
	})

	if err := r.Get(ctx, types.NamespacedName{Name: node.Name}, k8sNode); err == nil {
		if err := r.Delete(ctx, k8sNode); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete k8s node: %w", err)
		}
		logger.Info("deleted kubernetes node", "node", node.Name)
	}

	// Step 4: Delete the Hetzner server
	if node.Name != "" {
		startTime := time.Now()
		if err := r.hcloudClient.DeleteServer(ctx, node.Name); err != nil {
			logger.Error(err, "failed to delete hetzner server", "name", node.Name)
			if r.enableMetrics {
				RecordHCloudAPICall("delete_server", "error", time.Since(startTime).Seconds())
			}
			// Continue anyway - server might already be gone
		} else {
			logger.Info("deleted hetzner server", "name", node.Name, "serverID", node.ServerID)
			if r.enableMetrics {
				RecordHCloudAPICall("delete_server", "success", time.Since(startTime).Seconds())
			}
		}
	}

	// Remove old node from status
	r.removeNodeFromStatus(cluster, "worker", node.Name)

	// Step 5: Build cluster state for server creation
	clusterState, err := r.buildClusterState(ctx, cluster)
	if err != nil {
		logger.Error(err, "failed to build cluster state")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to build cluster state for worker replacement: %v", err)
		return fmt.Errorf("failed to build cluster state: %w", err)
	}

	// Step 5b: Use injected clients if available (for testing), otherwise load from credentials
	talosConfigGen := r.talosConfigGen
	talosClient := r.talosClient
	if talosClient == nil && cluster.Spec.CredentialsRef.Name != "" {
		creds, err := r.phaseAdapter.LoadCredentials(ctx, cluster)
		if err != nil {
			logger.Error(err, "failed to load credentials for Talos config generation")
		} else {
			if talosConfigGen == nil {
				generator, err := r.phaseAdapter.CreateTalosGenerator(cluster, creds)
				if err != nil {
					logger.Error(err, "failed to create Talos config generator")
				} else {
					talosConfigGen = generator
				}
			}
			if len(creds.TalosConfig) > 0 {
				talosClientInstance, err := NewRealTalosClient(creds.TalosConfig)
				if err != nil {
					logger.Error(err, "failed to create Talos client")
				} else {
					talosClient = talosClientInstance
				}
			}
		}
	}

	// Step 6: Get Talos snapshot for server creation
	snapshotLabels := map[string]string{"os": "talos"}
	snapshot, err := r.hcloudClient.GetSnapshotByLabels(ctx, snapshotLabels)
	if err != nil {
		logger.Error(err, "failed to get Talos snapshot")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to get Talos snapshot: %v", err)
		return fmt.Errorf("failed to get Talos snapshot: %w", err)
	}
	if snapshot == nil {
		err := fmt.Errorf("no Talos snapshot found with labels %v", snapshotLabels)
		logger.Error(err, "Talos snapshot not found")
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Talos snapshot not found for worker replacement")
		return err
	}

	// Step 6.5: Create ephemeral SSH key to avoid Hetzner password emails
	sshKeyName := fmt.Sprintf("ephemeral-%s-worker-%d", cluster.Name, time.Now().Unix())
	logger.Info("creating ephemeral SSH key for worker replacement", "keyName", sshKeyName)

	keyPair, err := keygen.GenerateRSAKeyPair(2048)
	if err != nil {
		logger.Error(err, "failed to generate ephemeral SSH key")
		return fmt.Errorf("failed to generate ephemeral SSH key: %w", err)
	}

	sshKeyLabels := map[string]string{
		"cluster": cluster.Name,
		"type":    "ephemeral-worker",
	}

	_, err = r.hcloudClient.CreateSSHKey(ctx, sshKeyName, string(keyPair.PublicKey), sshKeyLabels)
	if err != nil {
		logger.Error(err, "failed to create ephemeral SSH key")
		return fmt.Errorf("failed to create ephemeral SSH key: %w", err)
	}

	// Schedule cleanup of the ephemeral SSH key
	defer func() {
		logger.Info("cleaning up ephemeral SSH key", "keyName", sshKeyName)
		if err := r.hcloudClient.DeleteSSHKey(ctx, sshKeyName); err != nil {
			logger.Error(err, "failed to delete ephemeral SSH key", "keyName", sshKeyName)
		}
	}()

	// Step 7: Create new server
	newServerName := r.generateReplacementServerName(cluster, "worker", node.Name)
	serverLabels := map[string]string{
		"cluster": cluster.Name,
		"role":    "worker",
		"pool":    "workers",
	}

	// Track new node phase: CreatingServer
	r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
		Name:   newServerName,
		Phase:  k8znerv1alpha1.NodePhaseCreatingServer,
		Reason: fmt.Sprintf("Creating replacement HCloud server with snapshot %d", snapshot.ID),
	})

	serverType := normalizeServerSize(cluster.Spec.Workers.Size)
	logger.Info("creating replacement worker server",
		"name", newServerName,
		"snapshot", snapshot.ID,
		"serverType", serverType,
	)

	startTime := time.Now()
	_, err = r.hcloudClient.CreateServer(
		ctx,
		newServerName,
		fmt.Sprintf("%d", snapshot.ID), // image ID as string
		serverType,
		cluster.Spec.Region,
		[]string{sshKeyName}, // Use ephemeral SSH key
		serverLabels,
		"",  // userData
		nil, // placementGroupID
		clusterState.NetworkID,
		"",   // privateIP - let HCloud assign
		true, // enablePublicIPv4
		true, // enablePublicIPv6
	)
	if err != nil {
		if r.enableMetrics {
			RecordHCloudAPICall("create_server", "error", time.Since(startTime).Seconds())
		}
		logger.Error(err, "failed to create replacement worker server", "name", newServerName)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to create replacement worker server %s: %v", newServerName, err)
		r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
			Name:   newServerName,
			Phase:  k8znerv1alpha1.NodePhaseFailed,
			Reason: fmt.Sprintf("Failed to create server: %v", err),
		})
		return fmt.Errorf("failed to create server: %w", err)
	}
	if r.enableMetrics {
		RecordHCloudAPICall("create_server", "success", time.Since(startTime).Seconds())
	}
	logger.Info("created replacement worker server", "name", newServerName)

	// Step 8: Wait for server IP assignment
	r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
		Name:   newServerName,
		Phase:  k8znerv1alpha1.NodePhaseWaitingForIP,
		Reason: "Waiting for HCloud to assign IP address",
	})

	serverIP, err := r.waitForServerIP(ctx, newServerName, serverIPTimeout)
	if err != nil {
		logger.Error(err, "failed to get server IP", "name", newServerName)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to get IP for replacement worker server %s: %v", newServerName, err)
		r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
			Name:   newServerName,
			Phase:  k8znerv1alpha1.NodePhaseFailed,
			Reason: fmt.Sprintf("Failed to get IP: %v", err),
		})
		// Clean up orphaned server
		if delErr := r.hcloudClient.DeleteServer(ctx, newServerName); delErr != nil {
			logger.Error(delErr, "failed to delete orphaned server", "name", newServerName)
		}
		r.removeNodeFromStatus(cluster, "worker", newServerName)
		return fmt.Errorf("failed to get server IP: %w", err)
	}
	logger.Info("server IP assigned", "name", newServerName, "ip", serverIP)

	// Step 9: Get server ID for config generation
	serverIDStr, err := r.hcloudClient.GetServerID(ctx, newServerName)
	if err != nil {
		logger.Error(err, "failed to get server ID", "name", newServerName)
		r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
			Name:   newServerName,
			Phase:  k8znerv1alpha1.NodePhaseFailed,
			Reason: fmt.Sprintf("Failed to get server ID: %v", err),
		})
		// Clean up orphaned server
		if delErr := r.hcloudClient.DeleteServer(ctx, newServerName); delErr != nil {
			logger.Error(delErr, "failed to delete orphaned server", "name", newServerName)
		}
		r.removeNodeFromStatus(cluster, "worker", newServerName)
		return fmt.Errorf("failed to get server ID: %w", err)
	}
	var serverID int64
	if _, err := fmt.Sscanf(serverIDStr, "%d", &serverID); err != nil {
		r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
			Name:   newServerName,
			Phase:  k8znerv1alpha1.NodePhaseFailed,
			Reason: fmt.Sprintf("Failed to parse server ID: %v", err),
		})
		// Clean up orphaned server
		if delErr := r.hcloudClient.DeleteServer(ctx, newServerName); delErr != nil {
			logger.Error(delErr, "failed to delete orphaned server", "name", newServerName)
		}
		r.removeNodeFromStatus(cluster, "worker", newServerName)
		return fmt.Errorf("failed to parse server ID: %w", err)
	}

	// Get private IP from server
	privateIP, _ := r.getPrivateIPFromServer(ctx, newServerName)

	// Use private IP for Talos communication if available (bypasses firewall restrictions)
	// This is important for operator-centric flow where the operator runs inside the cluster
	talosIP := serverIP
	if privateIP != "" {
		talosIP = privateIP
		logger.Info("using private IP for Talos communication", "name", newServerName, "privateIP", privateIP)
	}

	// Update status with server ID and IPs, persist to CRD
	if err := r.updateNodePhaseAndPersist(ctx, cluster, "worker", NodeStatusUpdate{
		Name:      newServerName,
		ServerID:  serverID,
		PublicIP:  serverIP,
		PrivateIP: privateIP,
		Phase:     k8znerv1alpha1.NodePhaseWaitingForTalosAPI,
		Reason:    fmt.Sprintf("Waiting for Talos API on %s:50000", talosIP),
	}); err != nil {
		logger.Error(err, "failed to persist node status", "name", newServerName)
	}

	// Step 10: Generate and apply Talos config (if talos clients are available)
	if talosConfigGen != nil && talosClient != nil {
		r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
			Name:   newServerName,
			Phase:  k8znerv1alpha1.NodePhaseApplyingTalosConfig,
			Reason: "Generating and applying Talos machine configuration",
		})

		machineConfig, err := talosConfigGen.GenerateWorkerConfig(newServerName, serverID)
		if err != nil {
			logger.Error(err, "failed to generate worker config", "name", newServerName)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
				"Failed to generate config for worker %s: %v", newServerName, err)
			r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
				Name:   newServerName,
				Phase:  k8znerv1alpha1.NodePhaseFailed,
				Reason: fmt.Sprintf("Failed to generate Talos config: %v", err),
			})
			// Clean up orphaned server
			if delErr := r.hcloudClient.DeleteServer(ctx, newServerName); delErr != nil {
				logger.Error(delErr, "failed to delete orphaned server", "name", newServerName)
			}
			r.removeNodeFromStatus(cluster, "worker", newServerName)
			return fmt.Errorf("failed to generate config: %w", err)
		}

		logger.Info("applying Talos config to worker", "name", newServerName, "ip", talosIP)
		if err := talosClient.ApplyConfig(ctx, talosIP, machineConfig); err != nil {
			logger.Error(err, "failed to apply Talos config", "name", newServerName)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonConfigApplyError,
				"Failed to apply config to worker %s: %v", newServerName, err)
			r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
				Name:   newServerName,
				Phase:  k8znerv1alpha1.NodePhaseFailed,
				Reason: fmt.Sprintf("Failed to apply Talos config: %v", err),
			})
			// Clean up orphaned server
			if delErr := r.hcloudClient.DeleteServer(ctx, newServerName); delErr != nil {
				logger.Error(delErr, "failed to delete orphaned server", "name", newServerName)
			}
			r.removeNodeFromStatus(cluster, "worker", newServerName)
			return fmt.Errorf("failed to apply config: %w", err)
		}

		// Step 11: Wait for node to become ready
		r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
			Name:   newServerName,
			Phase:  k8znerv1alpha1.NodePhaseRebootingWithConfig,
			Reason: "Talos config applied, node is rebooting with new configuration",
		})

		logger.Info("waiting for worker node to become ready", "name", newServerName, "ip", talosIP)
		r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
			Name:   newServerName,
			Phase:  k8znerv1alpha1.NodePhaseWaitingForK8s,
			Reason: "Waiting for kubelet to register node with Kubernetes",
		})

		if err := talosClient.WaitForNodeReady(ctx, talosIP, int(nodeReadyTimeout.Seconds())); err != nil {
			logger.Error(err, "worker node failed to become ready", "name", newServerName)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonNodeReadyTimeout,
				"Worker %s failed to become ready: %v", newServerName, err)
			r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
				Name:   newServerName,
				Phase:  k8znerv1alpha1.NodePhaseFailed,
				Reason: fmt.Sprintf("Node not ready in time: %v", err),
			})
			// Clean up orphaned server
			if delErr := r.hcloudClient.DeleteServer(ctx, newServerName); delErr != nil {
				logger.Error(delErr, "failed to delete orphaned server", "name", newServerName)
			}
			r.removeNodeFromStatus(cluster, "worker", newServerName)
			return fmt.Errorf("node failed to become ready: %w", err)
		}

		// Node kubelet is running - transition to NodeInitializing
		// The state verifier will promote to Ready once K8s node is fully ready
		r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
			Name:   newServerName,
			Phase:  k8znerv1alpha1.NodePhaseNodeInitializing,
			Reason: "Kubelet running, waiting for CNI and system pods",
		})

		logger.Info("worker node kubelet is running", "name", newServerName)
	} else {
		logger.Info("skipping Talos config application (no credentials available)", "name", newServerName)
		r.updateNodePhase(ctx, cluster, "worker", NodeStatusUpdate{
			Name:   newServerName,
			Phase:  k8znerv1alpha1.NodePhaseWaitingForK8s,
			Reason: "Waiting for node to join cluster (no Talos credentials)",
		})
	}

	// Persist final status
	if err := r.persistClusterStatus(ctx, cluster); err != nil {
		logger.Error(err, "failed to persist cluster status", "name", newServerName)
	}

	return nil
}

// drainNode evicts all pods from a node.
func (r *ClusterReconciler) drainNode(ctx context.Context, nodeName string) error {
	logger := log.FromContext(ctx)

	// List pods on this node
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.MatchingFields{"spec.nodeName": nodeName}); err != nil {
		return fmt.Errorf("failed to list pods on node: %w", err)
	}

	// Evict each pod (skip DaemonSet pods and mirror pods)
	for _, pod := range podList.Items {
		// Skip mirror pods (static pods)
		if _, isMirror := pod.Annotations[corev1.MirrorPodAnnotationKey]; isMirror {
			continue
		}

		// Skip DaemonSet pods
		isDaemonSet := false
		for _, ref := range pod.OwnerReferences {
			if ref.Kind == "DaemonSet" {
				isDaemonSet = true
				break
			}
		}
		if isDaemonSet {
			continue
		}

		// Create eviction
		eviction := &policyv1.Eviction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
		}
		if err := r.SubResource("eviction").Create(ctx, &pod, eviction); err != nil {
			if !apierrors.IsNotFound(err) && !apierrors.IsTooManyRequests(err) {
				logger.Error(err, "failed to evict pod", "pod", pod.Name, "namespace", pod.Namespace)
			}
		} else {
			logger.V(1).Info("evicted pod", "pod", pod.Name, "namespace", pod.Namespace)
		}
	}

	return nil
}

// findHealthyControlPlaneIP finds the IP of a healthy control plane for API operations.
func (r *ClusterReconciler) findHealthyControlPlaneIP(cluster *k8znerv1alpha1.K8znerCluster) string {
	for _, node := range cluster.Status.ControlPlanes.Nodes {
		if node.Healthy && node.PrivateIP != "" {
			return node.PrivateIP
		}
	}
	return ""
}

// buildClusterState extracts cluster metadata needed for server creation.
func (r *ClusterReconciler) buildClusterState(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (*ClusterState, error) {
	logger := log.FromContext(ctx)

	state := &ClusterState{
		Name:   cluster.Name,
		Region: cluster.Spec.Region,
		Labels: map[string]string{
			"cluster": cluster.Name,
		},
	}

	// First, try to use infrastructure IDs from CRD status (set by CLI bootstrap)
	if cluster.Status.Infrastructure.NetworkID != 0 {
		state.NetworkID = cluster.Status.Infrastructure.NetworkID
	} else {
		// Fall back to looking up network by name
		networkName := fmt.Sprintf("%s-network", cluster.Name)
		network, err := r.hcloudClient.GetNetwork(ctx, networkName)
		if err != nil {
			return nil, fmt.Errorf("failed to get network %s: %w", networkName, err)
		}
		if network != nil {
			state.NetworkID = network.ID
		}
	}

	// Build SANs from existing healthy control plane IPs
	var sans []string

	// Add load balancer IP to SANs (from status, infrastructure, or annotation)
	//nolint:gocritic // ifElseChain is appropriate here as we're checking different sources with fallback logic
	if cluster.Status.ControlPlaneEndpoint != "" {
		sans = append(sans, cluster.Status.ControlPlaneEndpoint)
	} else if cluster.Status.Infrastructure.LoadBalancerIP != "" {
		sans = append(sans, cluster.Status.Infrastructure.LoadBalancerIP)
	} else if cluster.Annotations != nil {
		if endpoint, ok := cluster.Annotations["k8zner.io/control-plane-endpoint"]; ok {
			sans = append(sans, endpoint)
		}
	}

	// Add control plane node IPs to SANs
	for _, node := range cluster.Status.ControlPlanes.Nodes {
		if node.PrivateIP != "" {
			sans = append(sans, node.PrivateIP)
		}
		if node.PublicIP != "" {
			sans = append(sans, node.PublicIP)
		}
	}
	state.SANs = sans

	// Get SSH keys from cluster annotations or use default naming convention
	if cluster.Annotations != nil {
		if sshKeys, ok := cluster.Annotations["k8zner.io/ssh-keys"]; ok {
			state.SSHKeyIDs = strings.Split(sshKeys, ",")
		}
	}
	if len(state.SSHKeyIDs) == 0 {
		// Use default SSH key naming convention
		state.SSHKeyIDs = []string{fmt.Sprintf("%s-key", cluster.Name)}
	}

	// Get control plane endpoint (load balancer IP)
	// Priority: 1. CRD status endpoint, 2. Infrastructure LB IP, 3. Annotation, 4. Healthy CP IP
	//nolint:gocritic // ifElseChain is appropriate here as we're checking different sources with fallback logic
	if cluster.Status.ControlPlaneEndpoint != "" {
		state.ControlPlaneIP = cluster.Status.ControlPlaneEndpoint
	} else if cluster.Status.Infrastructure.LoadBalancerIP != "" {
		state.ControlPlaneIP = cluster.Status.Infrastructure.LoadBalancerIP
	} else if cluster.Annotations != nil {
		if endpoint, ok := cluster.Annotations["k8zner.io/control-plane-endpoint"]; ok {
			state.ControlPlaneIP = endpoint
		}
	}
	// Final fallback: use a healthy control plane IP
	if state.ControlPlaneIP == "" {
		state.ControlPlaneIP = r.findHealthyControlPlaneIP(cluster)
	}

	logger.V(1).Info("built cluster state",
		"networkID", state.NetworkID,
		"sans", len(state.SANs),
		"sshKeys", len(state.SSHKeyIDs),
		"controlPlaneIP", state.ControlPlaneIP,
	)

	return state, nil
}

// generateReplacementServerName generates a new server name for a replacement node.
// Uses the new naming convention: {cluster}-{role}-{5char} where role is cp or w.
func (r *ClusterReconciler) generateReplacementServerName(cluster *k8znerv1alpha1.K8znerCluster, role string, oldName string) string {
	// Always generate a new name with random ID for replacements
	// This avoids naming conflicts and makes it clear it's a new server
	switch role {
	case "control-plane":
		return naming.ControlPlane(cluster.Name)
	case "worker":
		return naming.Worker(cluster.Name)
	default:
		// Fallback for unknown roles
		return fmt.Sprintf("%s-%s-%s", cluster.Name, role[:2], naming.GenerateID(naming.IDLength))
	}
}

// waitForServerIP waits for a server to have an IP assigned.
func (r *ClusterReconciler) waitForServerIP(ctx context.Context, serverName string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Check immediately first (helps with mocks and already-assigned IPs)
	ip, err := r.hcloudClient.GetServerIP(ctx, serverName)
	if err == nil && ip != "" {
		return ip, nil
	}

	ticker := time.NewTicker(serverIPRetryDelay)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("timeout waiting for server IP: %w", ctx.Err())
		case <-ticker.C:
			ip, err := r.hcloudClient.GetServerIP(ctx, serverName)
			if err != nil {
				continue
			}
			if ip != "" {
				return ip, nil
			}
		}
	}
}

// updateClusterPhase updates the overall cluster phase based on status.
func (r *ClusterReconciler) updateClusterPhase(cluster *k8znerv1alpha1.K8znerCluster) {
	cpReady := cluster.Status.ControlPlanes.Ready == cluster.Status.ControlPlanes.Desired
	workersReady := cluster.Status.Workers.Ready == cluster.Status.Workers.Desired

	switch {
	case cpReady && workersReady:
		cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseRunning
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:    k8znerv1alpha1.ConditionReady,
			Status:  metav1.ConditionTrue,
			Reason:  "AllHealthy",
			Message: "All nodes are healthy",
		})
	case cluster.Status.Phase != k8znerv1alpha1.ClusterPhaseHealing:
		cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseDegraded
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:   k8znerv1alpha1.ConditionReady,
			Status: metav1.ConditionFalse,
			Reason: "NodesUnhealthy",
			Message: fmt.Sprintf("Control planes: %d/%d, Workers: %d/%d",
				cluster.Status.ControlPlanes.Ready, cluster.Status.ControlPlanes.Desired,
				cluster.Status.Workers.Ready, cluster.Status.Workers.Desired),
		})
	}

	// Update individual conditions
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:   k8znerv1alpha1.ConditionControlPlaneReady,
		Status: conditionStatus(cpReady),
		Reason: conditionReason(cpReady, "AllHealthy", "SomeUnhealthy"),
		Message: fmt.Sprintf("%d/%d control planes ready",
			cluster.Status.ControlPlanes.Ready, cluster.Status.ControlPlanes.Desired),
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:   k8znerv1alpha1.ConditionWorkersReady,
		Status: conditionStatus(workersReady),
		Reason: conditionReason(workersReady, "AllHealthy", "SomeUnhealthy"),
		Message: fmt.Sprintf("%d/%d workers ready",
			cluster.Status.Workers.Ready, cluster.Status.Workers.Desired),
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Index pods by node name for efficient drain operations
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Pod{}, "spec.nodeName", func(o client.Object) []string {
		pod := o.(*corev1.Pod)
		return []string{pod.Spec.NodeName}
	}); err != nil {
		return fmt.Errorf("failed to create pod node name index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&k8znerv1alpha1.K8znerCluster{}).
		Watches(&corev1.Node{}, &nodeEventHandler{}).
		Complete(r)
}

// Helper functions

func isNodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

func getNodeUnhealthyReason(node *corev1.Node) string {
	for _, cond := range node.Status.Conditions {
		switch {
		case cond.Type == corev1.NodeReady && cond.Status != corev1.ConditionTrue:
			return fmt.Sprintf("NodeNotReady: %s", cond.Message)
		case cond.Type == corev1.NodeMemoryPressure && cond.Status == corev1.ConditionTrue:
			return "MemoryPressure"
		case cond.Type == corev1.NodeDiskPressure && cond.Status == corev1.ConditionTrue:
			return "DiskPressure"
		case cond.Type == corev1.NodePIDPressure && cond.Status == corev1.ConditionTrue:
			return "PIDPressure"
		}
	}
	return "Unknown"
}

func parseThreshold(healthCheck *k8znerv1alpha1.HealthCheckSpec, thresholdType string) time.Duration {
	if healthCheck == nil {
		if thresholdType == "etcd" {
			return defaultEtcdUnhealthyThreshold
		}
		return defaultNodeNotReadyThreshold
	}

	var durationStr string
	switch thresholdType {
	case "etcd":
		durationStr = healthCheck.EtcdUnhealthyThreshold
		if durationStr == "" {
			return defaultEtcdUnhealthyThreshold
		}
	default:
		durationStr = healthCheck.NodeNotReadyThreshold
		if durationStr == "" {
			return defaultNodeNotReadyThreshold
		}
	}

	d, err := time.ParseDuration(durationStr)
	if err != nil {
		if thresholdType == "etcd" {
			return defaultEtcdUnhealthyThreshold
		}
		return defaultNodeNotReadyThreshold
	}
	return d
}

func conditionStatus(ready bool) metav1.ConditionStatus {
	if ready {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

func conditionReason(ready bool, trueReason, falseReason string) string {
	if ready {
		return trueReason
	}
	return falseReason
}

// nodeEventHandler handles node events and triggers reconciliation.
type nodeEventHandler struct{}

func (h *nodeEventHandler) Create(ctx context.Context, e event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueCluster(ctx, q)
}

func (h *nodeEventHandler) Update(ctx context.Context, e event.UpdateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueCluster(ctx, q)
}

func (h *nodeEventHandler) Delete(ctx context.Context, e event.DeleteEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueCluster(ctx, q)
}

func (h *nodeEventHandler) Generic(ctx context.Context, e event.GenericEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueCluster(ctx, q)
}

func (h *nodeEventHandler) enqueueCluster(ctx context.Context, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// Enqueue all K8znerCluster resources in the k8zner-system namespace
	// In a real implementation, we'd look up which cluster owns this node
	q.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "k8zner-system",
			Name:      "cluster",
		},
	})
}
