//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/sak-d/hcloud-k8s/internal/cluster"
	"github.com/sak-d/hcloud-k8s/internal/config"
	hcloud_client "github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/k8s"
	"github.com/sak-d/hcloud-k8s/internal/talos"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAddonsValidation(t *testing.T) {
	t.Parallel()

	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping e2e test")
	}

	clusterName := fmt.Sprintf("e2e-addons-%d", time.Now().Unix())
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	// 1. Setup minimal config
	cfg := &config.Config{
		ClusterName: clusterName,
		HCloudToken: token,
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		Firewall: config.FirewallConfig{
			UseCurrentIPv4: true,
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{
					Name:       "control-plane",
					ServerType: "cpx22",
					Count:      1,
					Image:      "talos",
				},
			},
		},
		Workers: []config.WorkerNodePool{
			{
				Name:       "worker",
				ServerType: "cpx22",
				Count:      1,
				Image:      "talos",
			},
		},
		Talos: config.TalosConfig{
			Version: "v1.8.3",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "v1.31.0",
		},
	}

	hClient := hcloud_client.NewRealClient(token)
	cleaner := &ResourceCleaner{t: t}
	sshKeyName, _ := setupSSHKey(t, hClient, cleaner, clusterName)
	cfg.SSHKeys = []string{sshKeyName}

	// 2. Clean up before and after
	defer func() {
		cleanupCtx := context.Background()
		hClient.DeleteServer(cleanupCtx, clusterName+"-control-plane-1")
		hClient.DeleteServer(cleanupCtx, clusterName+"-worker-1")
		hClient.DeleteLoadBalancer(cleanupCtx, clusterName+"-kube-api")
		hClient.DeleteFirewall(cleanupCtx, clusterName)
		hClient.DeleteNetwork(cleanupCtx, clusterName)
		hClient.DeletePlacementGroup(cleanupCtx, clusterName+"-control-plane")
		hClient.DeleteCertificate(cleanupCtx, clusterName+"-state")
	}()

	// 3. Reconcile
	talosGen, err := talos.NewConfigGenerator(clusterName, cfg.Kubernetes.Version, cfg.Talos.Version, "", cfg.Kubernetes.CNI.Type, cfg.Talos.RegistryMirrors, "")
	require.NoError(t, err)

	reconciler := cluster.NewReconciler(hClient, talosGen, cfg)
	err = reconciler.Reconcile(ctx)
	require.NoError(t, err)

	// 4. Validate Kubernetes State
	t.Log("Reconciliation finished, validating Kubernetes state...")

	// Get Kubeconfig
	cp1IP, err := hClient.GetServerIP(ctx, clusterName+"-control-plane-1")
	require.NoError(t, err)
	kubeconfig, err := talosGen.GetKubeconfig(ctx, cp1IP)
	require.NoError(t, err)

	kClient, err := k8s.NewClient(kubeconfig)
	require.NoError(t, err)

	// A. Wait for Nodes to be Ready (requires CNI)
	t.Log("Waiting for nodes to become Ready (timeout 10m)...")
	require.Eventually(t, func() bool {
		nodes, err := kClient.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Logf("Error listing nodes: %v", err)
			return false
		}
		readyCount := 0
		for _, node := range nodes.Items {
			for _, condition := range node.Status.Conditions {
				if condition.Type == "Ready" && condition.Status == "True" {
					readyCount++
				}
			}
		}
		t.Logf("Nodes Ready: %d/%d", readyCount, len(nodes.Items))
		return readyCount == 2 // 1 CP + 1 Worker
	}, 10*time.Minute, 30*time.Second)

	// B. Verify Addon Pods
	t.Log("Verifying addon pods...")

	// Cilium
	pods, err := kClient.Clientset.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{
		LabelSelector: "k8s-app=cilium",
	})
	require.NoError(t, err)
	require.NotEmpty(t, pods.Items, "Cilium pods should exist")
	t.Logf("Found %d Cilium pods", len(pods.Items))

	// CCM
	pods, err = kClient.Clientset.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{
		LabelSelector: "app=hcloud-cloud-controller-manager",
	})
	require.NoError(t, err)
	require.NotEmpty(t, pods.Items, "CCM pods should exist")
	t.Logf("Found %d CCM pods", len(pods.Items))

	t.Log("✓ Addons validation PASSED")
}
