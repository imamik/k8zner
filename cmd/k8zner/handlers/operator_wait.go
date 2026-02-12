package handlers

import (
	"context"
	"fmt"
	"log"
	"time"

	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
)

const (
	// operatorPollInterval is the interval between status checks when waiting for the operator.
	operatorPollInterval = 10 * time.Second

	// operatorWaitTimeout is the maximum time to wait for the operator to complete provisioning.
	operatorWaitTimeout = 30 * time.Minute
)

// waitForOperatorComplete polls the K8znerCluster status until provisioning completes.
func waitForOperatorComplete(ctx context.Context, clusterName string, kubeconfig []byte) error {
	log.Println("Waiting for operator to complete provisioning...")

	kubecfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to parse kubeconfig: %w", err)
	}

	scheme := k8znerv1alpha1.Scheme
	k8sClient, err := client.New(kubecfg, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	ticker := time.NewTicker(operatorPollInterval)
	defer ticker.Stop()

	deadline := time.Now().Add(operatorWaitTimeout)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for operator to complete after %v", operatorWaitTimeout)
		}

		phase, clusterPhase, err := getClusterPhase(ctx, k8sClient, clusterName)
		if err != nil {
			log.Printf("Warning: failed to get cluster status: %v", err)
			continue
		}

		// Show doctor status summary for richer monitoring
		if status, statusErr := getClusterStatus(ctx, k8sClient, clusterName); statusErr == nil {
			healthySummary := doctorSummaryLine(status)
			log.Printf("Status: %s (phase: %s) %s", clusterPhase, phase, healthySummary)
		} else {
			log.Printf("Status: %s (phase: %s)", clusterPhase, phase)
		}

		if phase == k8znerv1alpha1.PhaseComplete && clusterPhase == k8znerv1alpha1.ClusterPhaseRunning {
			log.Println("Cluster provisioning complete!")
			return nil
		}
	}
}

// getClusterPhase retrieves the current provisioning and cluster phases.
func getClusterPhase(ctx context.Context, k8sClient client.Client, clusterName string) (k8znerv1alpha1.ProvisioningPhase, k8znerv1alpha1.ClusterPhase, error) {
	getCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	k8zCluster := &k8znerv1alpha1.K8znerCluster{}
	key := client.ObjectKey{
		Namespace: k8znerNamespace,
		Name:      clusterName,
	}

	if err := k8sClient.Get(getCtx, key, k8zCluster); err != nil {
		return "", "", err
	}

	return k8zCluster.Status.ProvisioningPhase, k8zCluster.Status.Phase, nil
}
