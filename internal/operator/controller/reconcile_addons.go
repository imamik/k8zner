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
)

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
	restConfig, err := clientConfigFromKubeconfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create rest config: %w", err)
	}

	k8sClient, err := client.New(restConfig, client.Options{})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-checkCtx.Done():
			return fmt.Errorf("timeout waiting for Cilium to be ready")
		case <-ticker.C:
			podList := &corev1.PodList{}
			if err := k8sClient.List(ctx, podList, client.InNamespace("kube-system"), client.MatchingLabels{"k8s-app": "cilium"}); err != nil {
				continue
			}

			if len(podList.Items) == 0 {
				continue
			}

			if allPodsReady(podList.Items) {
				return nil
			}
		}
	}
}

// allPodsReady checks if all pods in the list are running and ready.
func allPodsReady(pods []corev1.Pod) bool {
	for _, pod := range pods {
		if pod.Status.Phase != corev1.PodRunning {
			return false
		}
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status != corev1.ConditionTrue {
				return false
			}
		}
	}
	return true
}

// reconcileAddonsPhase installs remaining addons after CNI is ready.
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
func (r *ClusterReconciler) ensureWorkersReady(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (ctrl.Result, bool) {
	logger := log.FromContext(ctx)

	desiredWorkers := cluster.Spec.Workers.Count
	readyWorkers := cluster.Status.Workers.Ready
	if desiredWorkers == 0 || readyWorkers >= desiredWorkers {
		return ctrl.Result{}, false
	}

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

	if len(creds.TalosConfig) == 0 {
		return nil, fmt.Errorf("talos config not available in credentials")
	}

	talosConfig, err := talosconfig.FromString(string(creds.TalosConfig))
	if err != nil {
		return nil, fmt.Errorf("failed to parse talos config: %w", err)
	}

	endpoint := r.findTalosEndpoint(cluster)
	if endpoint == "" {
		return nil, fmt.Errorf("no control plane endpoint available")
	}

	logger.Info("retrieving kubeconfig from Talos", "endpoint", endpoint)

	talosClient, err := talosclient.New(ctx,
		talosclient.WithConfig(talosConfig),
		talosclient.WithEndpoints(endpoint),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Talos client: %w", err)
	}
	defer func() { _ = talosClient.Close() }()

	kubeconfigCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	kubeconfig, err := talosClient.Kubeconfig(kubeconfigCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve kubeconfig: %w", err)
	}

	return kubeconfig, nil
}

// findTalosEndpoint returns the best available Talos API endpoint for the cluster.
func (r *ClusterReconciler) findTalosEndpoint(cluster *k8znerv1alpha1.K8znerCluster) string {
	switch {
	case cluster.Status.ControlPlaneEndpoint != "":
		return cluster.Status.ControlPlaneEndpoint
	case cluster.Spec.Bootstrap != nil && cluster.Spec.Bootstrap.PublicIP != "":
		return cluster.Spec.Bootstrap.PublicIP
	default:
		for _, node := range cluster.Status.ControlPlanes.Nodes {
			if node.Healthy && node.PublicIP != "" {
				return node.PublicIP
			}
		}
	}
	return ""
}
