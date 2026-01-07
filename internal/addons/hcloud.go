package addons

import (
	"context"
	"fmt"
	"log"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	hcloudChartsRepo = "https://charts.hetzner.cloud"
)

func (m *Manager) ensureHCloudSecret(ctx context.Context) error {
	log.Println("Ensuring hcloud secret...")

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hcloud",
			Namespace: "kube-system",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"token":   []byte(m.cfg.HCloudToken),
			"network": []byte(fmt.Sprintf("%d", m.networkID)),
		},
	}

	existing, err := m.k8sClient.Clientset.CoreV1().Secrets("kube-system").Get(ctx, "hcloud", metav1.GetOptions{})
	if err == nil {
		// Update if token/network changed
		if string(existing.Data["token"]) != m.cfg.HCloudToken || string(existing.Data["network"]) != fmt.Sprintf("%d", m.networkID) {
			_, err = m.k8sClient.Clientset.CoreV1().Secrets("kube-system").Update(ctx, secret, metav1.UpdateOptions{})
			return err
		}
		return nil
	}

	_, err = m.k8sClient.Clientset.CoreV1().Secrets("kube-system").Create(ctx, secret, metav1.CreateOptions{})
	return err
}

func (m *Manager) ensureCCM(ctx context.Context) error {
	log.Println("Deploying Hetzner CCM...")

	values := map[string]interface{}{
		"networking": map[string]interface{}{
			"enabled":     true,
			"clusterCIDR": m.cfg.Network.PodIPv4CIDR,
		},
		"nodeSelector": map[string]interface{}{
			"node-role.kubernetes.io/control-plane": "",
		},
	}

	// CCM chart often requires specific tolerations for control-plane
	// Most helm charts for CCM handle this via default values, but let's be sure.

	return m.helmClient.InstallOrUpgrade(
		m.kubeconfig,
		"kube-system",
		"hcloud-ccm",
		hcloudChartsRepo,
		"hcloud-cloud-controller-manager",
		"1.19.0", // Target version
		values,
	)
}

func (m *Manager) ensureCSI(ctx context.Context) error {
	log.Println("Deploying Hetzner CSI...")

	values := map[string]interface{}{
		"controller": map[string]interface{}{
			"nodeSelector": map[string]interface{}{
				"node-role.kubernetes.io/control-plane": "",
			},
			"tolerations": []interface{}{
				map[string]interface{}{
					"key":      "node-role.kubernetes.io/control-plane",
					"effect":   "NoSchedule",
					"operator": "Exists",
				},
			},
		},
		"storageClasses": []interface{}{
			map[string]interface{}{
				"name":                "hcloud-volumes",
				"reclaimPolicy":       "Delete",
				"defaultStorageClass": true,
			},
		},
	}

	return m.helmClient.InstallOrUpgrade(
		m.kubeconfig,
		"kube-system",
		"hcloud-csi",
		hcloudChartsRepo,
		"hcloud-csi",
		"2.11.0", // Target version
		values,
	)
}
