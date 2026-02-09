package controller

import (
	"context"
	"fmt"
	"time"

	talosclient "github.com/siderolabs/talos/pkg/machinery/client"
	talosconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/addons"
	"github.com/imamik/k8zner/internal/config"
	operatorprov "github.com/imamik/k8zner/internal/operator/provisioning"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/util/naming"
)

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
		r.logAndRecordError(ctx, cluster, err, EventReasonCredentialsError, "Failed to load credentials")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Build provisioning context
	pCtx, err := r.buildProvisioningContext(ctx, cluster, creds)
	if err != nil {
		r.logAndRecordError(ctx, cluster, err, EventReasonInfrastructureFailed, "Failed to build provisioning context")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Run infrastructure provisioning
	if err := r.phaseAdapter.ReconcileInfrastructure(pCtx, cluster); err != nil {
		r.logAndRecordError(ctx, cluster, err, EventReasonInfrastructureFailed, "Infrastructure provisioning failed")
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
		r.logAndRecordError(ctx, cluster, err, EventReasonCredentialsError, "Failed to load credentials")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Build provisioning context
	pCtx, err := r.buildProvisioningContext(ctx, cluster, creds)
	if err != nil {
		r.logAndRecordError(ctx, cluster, err, EventReasonImageFailed, "Failed to build provisioning context")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Run image provisioning
	if err := r.phaseAdapter.ReconcileImage(pCtx, cluster); err != nil {
		r.logAndRecordError(ctx, cluster, err, EventReasonImageFailed, "Image provisioning failed")
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
		r.logAndRecordError(ctx, cluster, err, EventReasonCredentialsError, "Failed to load credentials")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Build provisioning context
	pCtx, err := r.buildProvisioningContext(ctx, cluster, creds)
	if err != nil {
		r.logAndRecordError(ctx, cluster, err, EventReasonComputeFailed, "Failed to build provisioning context")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Run compute provisioning
	if err := r.phaseAdapter.ReconcileCompute(pCtx, cluster); err != nil {
		r.logAndRecordError(ctx, cluster, err, EventReasonComputeFailed, "Compute provisioning failed")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonComputeProvisioned,
		"Compute resources provisioned")

	// For CLI-bootstrapped clusters, the cluster is already bootstrapped and CNI is installed.
	// Skip Bootstrap phase (which can't run from inside the cluster due to TLS cert SANs)
	// and go directly to Addons. Any new CP/worker servers created during Compute will be
	// configured by the Running-phase scale-up logic (scaleUpControlPlanes/scaleUpWorkers).
	if cluster.Spec.Bootstrap != nil && cluster.Spec.Bootstrap.Completed {
		cluster.Status.ProvisioningPhase = k8znerv1alpha1.PhaseAddons
	} else {
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
		r.logAndRecordError(ctx, cluster, err, EventReasonCredentialsError, "Failed to load credentials")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Build provisioning context
	pCtx, err := r.buildProvisioningContext(ctx, cluster, creds)
	if err != nil {
		r.logAndRecordError(ctx, cluster, err, EventReasonBootstrapFailed, "Failed to build provisioning context")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Run cluster bootstrap
	// For CLI-bootstrapped clusters, this will detect the state marker and only configure new nodes
	// For fresh clusters, this will do the full bootstrap sequence
	if err := r.phaseAdapter.ReconcileBootstrap(pCtx, cluster); err != nil {
		r.logAndRecordError(ctx, cluster, err, EventReasonBootstrapFailed, "Cluster bootstrap failed")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonBootstrapComplete,
		"Cluster bootstrapped successfully")

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
		r.logAndRecordError(ctx, cluster, err, EventReasonCredentialsError, "Failed to load credentials")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Convert CRD spec to config for addon installation
	cfg, err := operatorprov.SpecToConfig(cluster, creds)
	if err != nil {
		r.logAndRecordError(ctx, cluster, err, EventReasonConfiguringFailed, "Failed to convert spec to config")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Ensure HCloud token is set (required for CCM/CSI)
	cfg.HCloudToken = creds.HCloudToken

	// Get kubeconfig from Talos
	kubeconfig, err := r.getKubeconfigFromTalos(ctx, cluster, creds)
	if err != nil {
		r.logAndRecordError(ctx, cluster, err, EventReasonConfiguringFailed, "Failed to get kubeconfig")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// Get network ID from status, or look it up from HCloud if not set
	networkID := cluster.Status.Infrastructure.NetworkID
	if networkID == 0 {
		// Network ID not in status - look it up from HCloud by cluster name
		logger.Info("networkID not in status, looking up from HCloud", "clusterName", cluster.Name)
		network, err := r.hcloudClient.GetNetwork(ctx, cluster.Name)
		if err != nil {
			r.logAndRecordError(ctx, cluster, err, EventReasonConfiguringFailed, "Failed to get network from HCloud")
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
		r.logAndRecordError(ctx, cluster, err, EventReasonConfiguringFailed, "Failed to install addons")
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

	logger.V(1).Info("loading credentials for CNI installation")
	creds, err := r.phaseAdapter.LoadCredentials(ctx, cluster)
	if err != nil {
		r.logAndRecordError(ctx, cluster, err, EventReasonCredentialsError, "Failed to load credentials")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}
	logger.V(1).Info("credentials loaded successfully",
		"hasTalosSecrets", len(creds.TalosSecrets) > 0,
		"hasTalosConfig", len(creds.TalosConfig) > 0,
	)

	cfg, err := operatorprov.SpecToConfig(cluster, creds)
	if err != nil {
		r.logAndRecordError(ctx, cluster, err, EventReasonCNIFailed, "Failed to convert spec to config")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	logger.V(1).Info("getting kubeconfig from Talos")
	kubeconfig, err := r.getKubeconfigFromTalos(ctx, cluster, creds)
	if err != nil {
		r.logAndRecordError(ctx, cluster, err, EventReasonCNIFailed, "Failed to get kubeconfig")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}
	logger.V(1).Info("kubeconfig retrieved successfully", "kubeconfigLength", len(kubeconfig))

	if result, err := r.installAndWaitForCNI(ctx, cluster, cfg, kubeconfig); err != nil || result.RequeueAfter > 0 {
		return result, err
	}

	// For CLI-bootstrapped clusters, workers don't exist yet - go through compute/bootstrap first
	if cluster.Spec.Bootstrap != nil && cluster.Spec.Bootstrap.Completed {
		cluster.Status.ProvisioningPhase = k8znerv1alpha1.PhaseCompute
	} else {
		// For operator-managed clusters, compute/bootstrap already ran - proceed to addons
		cluster.Status.ProvisioningPhase = k8znerv1alpha1.PhaseAddons
	}
	return ctrl.Result{Requeue: true}, nil
}

// installAndWaitForCNI installs Cilium CNI and waits for it to become ready.
func (r *ClusterReconciler) installAndWaitForCNI(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, cfg *config.Config, kubeconfig []byte) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.Info("installing Cilium CNI")
	if err := addons.ApplyCilium(ctx, cfg, kubeconfig); err != nil {
		r.logAndRecordError(ctx, cluster, err, EventReasonCNIFailed, "Failed to install Cilium")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	now := metav1.Now()
	if cluster.Status.Addons == nil {
		cluster.Status.Addons = make(map[string]k8znerv1alpha1.AddonStatus)
	}
	cluster.Status.Addons[k8znerv1alpha1.AddonNameCilium] = k8znerv1alpha1.AddonStatus{
		Installed:          true,
		Healthy:            false,
		Phase:              k8znerv1alpha1.AddonPhaseInstalling,
		LastTransitionTime: &now,
		InstallOrder:       k8znerv1alpha1.AddonOrderCilium,
	}

	logger.Info("waiting for Cilium to be ready")
	if err := r.waitForCiliumReady(ctx, kubeconfig); err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonCNIFailed,
			"Cilium not ready: %v", err)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	cluster.Status.Addons[k8znerv1alpha1.AddonNameCilium] = k8znerv1alpha1.AddonStatus{
		Installed:          true,
		Healthy:            true,
		Phase:              k8znerv1alpha1.AddonPhaseInstalled,
		LastTransitionTime: &now,
		InstallOrder:       k8znerv1alpha1.AddonOrderCilium,
	}

	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonCNIReady,
		"Cilium CNI is ready")

	return ctrl.Result{}, nil
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
// Addons are installed one-at-a-time per reconcile cycle, updating the CRD
// status after each so that progress is visible externally.
func (r *ClusterReconciler) reconcileAddonsPhase(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling addons phase")

	// Ensure workers are ready before installing addons
	if result, waiting := r.ensureWorkersReady(ctx, cluster); waiting {
		return result, nil
	}

	creds, err := r.phaseAdapter.LoadCredentials(ctx, cluster)
	if err != nil {
		r.logAndRecordError(ctx, cluster, err, EventReasonCredentialsError, "Failed to load credentials")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	cfg, err := operatorprov.SpecToConfig(cluster, creds)
	if err != nil {
		r.logAndRecordError(ctx, cluster, err, EventReasonAddonsFailed, "Failed to convert spec to config")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}
	cfg.HCloudToken = creds.HCloudToken

	kubeconfig, err := r.getKubeconfigFromTalos(ctx, cluster, creds)
	if err != nil {
		r.logAndRecordError(ctx, cluster, err, EventReasonAddonsFailed, "Failed to get kubeconfig")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	networkID, err := r.resolveNetworkID(ctx, cluster)
	if err != nil {
		r.logAndRecordError(ctx, cluster, err, EventReasonAddonsFailed, "Failed to resolve network ID")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	if cfg.HCloudToken == "" {
		err := fmt.Errorf("HCloud token is empty")
		r.logAndRecordError(ctx, cluster, err, EventReasonAddonsFailed, "CCM/CSI addons require valid credentials")
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	return r.installNextAddon(ctx, cluster, cfg, kubeconfig, networkID)
}

// ensureWorkersReady checks if workers are ready, creating them if needed.
// Returns (result, true) if workers are still being created/not ready and caller should return.
func (r *ClusterReconciler) ensureWorkersReady(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, bool) {
	logger := log.FromContext(ctx)

	desiredWorkers := cluster.Spec.Workers.Count
	readyWorkers := cluster.Status.Workers.Ready
	if desiredWorkers == 0 || readyWorkers >= desiredWorkers {
		return ctrl.Result{}, false
	}

	// Trigger worker creation if not already done
	currentWorkerNodes := len(cluster.Status.Workers.Nodes)
	if currentWorkerNodes < desiredWorkers && r.hcloudClient != nil {
		toCreate := desiredWorkers - currentWorkerNodes
		logger.Info("creating workers before addon installation",
			"desired", desiredWorkers, "current", currentWorkerNodes, "toCreate", toCreate)
		if err := r.scaleUpWorkers(ctx, cluster, toCreate); err != nil {
			logger.Error(err, "failed to create workers for addon phase")
		}
	}

	logger.Info("waiting for workers to be ready before installing addons",
		"ready", readyWorkers, "desired", desiredWorkers)
	return ctrl.Result{RequeueAfter: 15 * time.Second}, true
}

// resolveNetworkID returns the network ID from status, or looks it up from HCloud.
func (r *ClusterReconciler) resolveNetworkID(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (int64, error) {
	logger := log.FromContext(ctx)

	networkID := cluster.Status.Infrastructure.NetworkID
	if networkID != 0 {
		return networkID, nil
	}

	logger.Info("networkID not in status, looking up from HCloud", "clusterName", cluster.Name)
	network, err := r.hcloudClient.GetNetwork(ctx, cluster.Name)
	if err != nil {
		return 0, err
	}
	if network == nil {
		r.Recorder.Event(cluster, corev1.EventTypeWarning, EventReasonAddonsFailed,
			"Network not found in HCloud - waiting for infrastructure")
		return 0, fmt.Errorf("network not found in HCloud")
	}

	cluster.Status.Infrastructure.NetworkID = network.ID
	logger.Info("found network ID from HCloud", "networkID", network.ID)
	return network.ID, nil
}

// installNextAddon installs the next pending addon from the enabled steps list.
func (r *ClusterReconciler) installNextAddon(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, cfg *config.Config, kubeconfig []byte, networkID int64) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if cluster.Status.Addons == nil {
		cluster.Status.Addons = make(map[string]k8znerv1alpha1.AddonStatus)
	}

	steps := addons.EnabledSteps(cfg)
	for _, step := range steps {
		if _, installed := cluster.Status.Addons[step.Name]; installed {
			continue
		}

		logger.Info("installing addon", "addon", step.Name, "order", step.Order)
		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonAddonsInstalling,
			"Installing addon: %s", step.Name)

		if err := addons.InstallStep(ctx, step.Name, cfg, kubeconfig, networkID); err != nil {
			r.logAndRecordError(ctx, cluster, err, EventReasonAddonsFailed,
				fmt.Sprintf("Failed to install addon: %s", step.Name))
			return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
		}

		now := metav1.Now()
		cluster.Status.Addons[step.Name] = k8znerv1alpha1.AddonStatus{
			Installed:          true,
			Healthy:            true,
			Phase:              k8znerv1alpha1.AddonPhaseInstalled,
			LastTransitionTime: &now,
			InstallOrder:       step.Order,
		}

		logger.Info("addon installed successfully", "addon", step.Name)
		return ctrl.Result{Requeue: true}, nil
	}

	// All addons installed
	r.Recorder.Event(cluster, corev1.EventTypeNormal, EventReasonAddonsReady,
		"All addons installed successfully")

	cluster.Status.PhaseStartedAt = nil
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
	infraManager := hcloud.NewRealClient(creds.HCloudToken)

	// Discover infrastructure from HCloud BEFORE creating Talos generator.
	// The Talos generator needs the control plane endpoint (LB IP) to generate configs.
	r.discoverInfrastructure(ctx, cluster, infraManager)

	talosProducer, err := r.phaseAdapter.CreateTalosGenerator(cluster, creds)
	if err != nil {
		return nil, fmt.Errorf("failed to create talos generator: %w", err)
	}

	pCtx, err := r.phaseAdapter.BuildProvisioningContext(ctx, cluster, creds, infraManager, talosProducer)
	if err != nil {
		return nil, err
	}

	// Populate network state for CLI bootstrap clusters
	if pCtx.State.Network == nil {
		network, err := infraManager.GetNetwork(ctx, cluster.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get network: %w", err)
		}
		pCtx.State.Network = network
		if cluster.Status.Infrastructure.NetworkID == 0 && network != nil {
			cluster.Status.Infrastructure.NetworkID = network.ID
		}
	}

	return pCtx, nil
}

// discoverInfrastructure populates missing infrastructure IDs by querying HCloud.
// This is critical for CLI-bootstrapped clusters where the CRD status may not have all infra info.
func (r *ClusterReconciler) discoverInfrastructure(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, infraManager *hcloud.RealClient) {
	if cluster.Status.Infrastructure.LoadBalancerID == 0 {
		lbName := naming.KubeAPILoadBalancer(cluster.Name)
		lb, err := infraManager.GetLoadBalancer(ctx, lbName)
		if err == nil && lb != nil {
			cluster.Status.Infrastructure.LoadBalancerID = lb.ID
			if lb.PublicNet.Enabled && lb.PublicNet.IPv4.IP.String() != "<nil>" {
				cluster.Status.Infrastructure.LoadBalancerIP = lb.PublicNet.IPv4.IP.String()
			}
			if len(lb.PrivateNet) > 0 && lb.PrivateNet[0].IP != nil {
				cluster.Status.Infrastructure.LoadBalancerPrivateIP = lb.PrivateNet[0].IP.String()
			}
		}
	}

	if cluster.Status.Infrastructure.FirewallID == 0 {
		fwName := naming.Firewall(cluster.Name)
		fw, err := infraManager.GetFirewall(ctx, fwName)
		if err == nil && fw != nil {
			cluster.Status.Infrastructure.FirewallID = fw.ID
		}
	}
}
