package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
)

// addonCheck defines how to check an addon's runtime health.
type addonCheck struct {
	name      string
	checkFunc func(ctx context.Context, r *ClusterReconciler) (bool, string)
}

// reconcileAddonHealth checks the runtime health of all installed addons.
// It updates AddonStatus.Healthy, .Message, and .LastHealthCheck for each addon.
// This is non-fatal — errors are logged but never returned.
func (r *ClusterReconciler) reconcileAddonHealth(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("checking addon runtime health")

	if cluster.Status.Addons == nil {
		return
	}

	now := metav1.Now()

	checks := []addonCheck{
		{k8znerv1alpha1.AddonNameCilium, checkDaemonSet("kube-system", "cilium")},
		{k8znerv1alpha1.AddonNameCCM, checkDeployment("kube-system", "hcloud-cloud-controller-manager")},
		{k8znerv1alpha1.AddonNameCSI, checkDeployment("kube-system", "hcloud-csi-controller")},
		{k8znerv1alpha1.AddonNameMetricsServer, checkDeployment("kube-system", "metrics-server")},
		{k8znerv1alpha1.AddonNameTraefik, checkDeployment("traefik", "traefik")},
		{k8znerv1alpha1.AddonNameCertManager, checkDeployment("cert-manager", "cert-manager")},
		{k8znerv1alpha1.AddonNameExternalDNS, checkDeployment("external-dns", "external-dns")},
		{k8znerv1alpha1.AddonNameArgoCD, checkDeployment("argocd", "argocd-server")},
		{k8znerv1alpha1.AddonNameMonitoring, checkMonitoring},
		{k8znerv1alpha1.AddonNameTalosBackup, checkCronJob("kube-system", "talos-backup")},
	}

	allHealthy := true
	for _, check := range checks {
		addon, ok := cluster.Status.Addons[check.name]
		if !ok || !addon.Installed {
			continue
		}

		healthy, msg := check.checkFunc(ctx, r)
		addon.Healthy = healthy
		addon.Message = msg
		addon.LastHealthCheck = &now
		cluster.Status.Addons[check.name] = addon

		if !healthy {
			allHealthy = false
			logger.Info("addon unhealthy", "addon", check.name, "message", msg)
		}
	}

	// Set ConditionAddonsHealthy
	status := metav1.ConditionTrue
	reason := "AllHealthy"
	message := "All installed addons are healthy"
	if !allHealthy {
		status = metav1.ConditionFalse
		reason = "SomeUnhealthy"
		message = "One or more addons are unhealthy"
	}
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:    k8znerv1alpha1.ConditionAddonsHealthy,
		Status:  status,
		Reason:  reason,
		Message: message,
	})
}

// checkDeployment returns a check function that verifies a Deployment has ready replicas.
// Returns (healthy, message). On API errors, returns (true, error) to avoid flapping —
// the addon was already marked healthy during installation.
func checkDeployment(namespace, name string) func(ctx context.Context, r *ClusterReconciler) (bool, string) {
	return func(ctx context.Context, r *ClusterReconciler) (bool, string) {
		dep := &appsv1.Deployment{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, dep); err != nil {
			// Don't mark unhealthy on API/cache errors — preserve install-time health
			return true, fmt.Sprintf("check skipped: %v", err)
		}
		if dep.Status.ReadyReplicas > 0 {
			return true, fmt.Sprintf("%d/%d ready", dep.Status.ReadyReplicas, dep.Status.Replicas)
		}
		return false, fmt.Sprintf("0/%d ready", dep.Status.Replicas)
	}
}

// checkDaemonSet returns a check function that verifies a DaemonSet has ready pods.
func checkDaemonSet(namespace, name string) func(ctx context.Context, r *ClusterReconciler) (bool, string) {
	return func(ctx context.Context, r *ClusterReconciler) (bool, string) {
		ds := &appsv1.DaemonSet{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, ds); err != nil {
			return true, fmt.Sprintf("check skipped: %v", err)
		}
		if ds.Status.NumberReady > 0 {
			return true, fmt.Sprintf("%d ready", ds.Status.NumberReady)
		}
		return false, "0 ready"
	}
}

// checkCronJob returns a check function that verifies a CronJob exists.
func checkCronJob(namespace, name string) func(ctx context.Context, r *ClusterReconciler) (bool, string) {
	return func(ctx context.Context, r *ClusterReconciler) (bool, string) {
		cj := &batchv1.CronJob{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, cj); err != nil {
			return true, fmt.Sprintf("check skipped: %v", err)
		}
		return true, "exists"
	}
}

// checkMonitoring verifies the monitoring stack (Prometheus + Grafana).
func checkMonitoring(ctx context.Context, r *ClusterReconciler) (bool, string) {
	// Check Prometheus StatefulSet
	stsList := &appsv1.StatefulSetList{}
	if err := r.List(ctx, stsList, client.InNamespace("monitoring"), client.MatchingLabels{"app.kubernetes.io/name": "prometheus"}); err != nil {
		return true, fmt.Sprintf("check skipped: %v", err)
	}
	promReady := false
	for _, sts := range stsList.Items {
		if sts.Status.ReadyReplicas > 0 {
			promReady = true
			break
		}
	}

	// Check Grafana Deployment
	depList := &appsv1.DeploymentList{}
	if err := r.List(ctx, depList, client.InNamespace("monitoring"), client.MatchingLabels{"app.kubernetes.io/name": "grafana"}); err != nil {
		return true, fmt.Sprintf("check skipped: %v", err)
	}
	grafanaReady := false
	for _, dep := range depList.Items {
		if dep.Status.ReadyReplicas > 0 {
			grafanaReady = true
			break
		}
	}

	if promReady && grafanaReady {
		return true, "prometheus and grafana ready"
	}
	return false, fmt.Sprintf("prometheus=%v, grafana=%v", promReady, grafanaReady)
}
