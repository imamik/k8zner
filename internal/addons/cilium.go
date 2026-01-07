package addons

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ciliumChartsRepo = "https://helm.cilium.io/"
)

func (m *Manager) ensureCilium(ctx context.Context) error {
	log.Println("Deploying Cilium CNI...")

	values := map[string]interface{}{
		"ipam": map[string]interface{}{
			"mode": "kubernetes",
		},
		"k8sServiceHost":       "127.0.0.1",
		"k8sServicePort":       "7445",
		"kubeProxyReplacement": true,
		"securityContext": map[string]interface{}{
			"capabilities": map[string]interface{}{
				"ciliumAgent":      []string{"CHOWN", "KILL", "NET_ADMIN", "NET_RAW", "IPC_LOCK", "SYS_ADMIN", "SYS_RESOURCE", "DAC_OVERRIDE", "FOWNER", "SETGID", "SETUID"},
				"cleanCiliumState": []string{"NET_ADMIN", "SYS_ADMIN", "SYS_RESOURCE"},
			},
		},
		"cgroup": map[string]interface{}{
			"autoMount": map[string]interface{}{
				"enabled": false,
			},
			"hostRoot": "/sys/fs/cgroup",
		},
	}

	// Handle Encryption
	encryption := m.cfg.Kubernetes.CNI.Encryption
	if encryption == "ipsec" || encryption == "wireguard" {
		values["encryption"] = map[string]interface{}{
			"enabled": true,
			"type":    encryption,
		}

		if encryption == "ipsec" {
			if err := m.ensureCiliumIPSecSecret(ctx); err != nil {
				return fmt.Errorf("failed to ensure cilium ipsec secret: %w", err)
			}
		}
	}

	return m.helmClient.InstallOrUpgrade(
		m.kubeconfig,
		"kube-system",
		"cilium",
		ciliumChartsRepo,
		"cilium",
		"1.16.1", // Target version
		values,
	)
}

func (m *Manager) ensureCiliumIPSecSecret(ctx context.Context) error {
	log.Println("Ensuring Cilium IPSec secret...")

	// Check if exists
	_, err := m.k8sClient.Clientset.CoreV1().Secrets("kube-system").Get(ctx, "cilium-ipsec-keys", metav1.GetOptions{})
	if err == nil {
		return nil
	}

	// Generate key (20 bytes for AES-GCM-128)
	key := make([]byte, 20)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	keyHex := hex.EncodeToString(key)

	// Format: "ID+ algorithm key"
	// Default ID 3 is often used in terraform examples
	ipsecKey := fmt.Sprintf("3+ rfc4106(gcm(aes)) %s 128", keyHex)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cilium-ipsec-keys",
			Namespace: "kube-system",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"keys": []byte(ipsecKey),
		},
	}

	_, err = m.k8sClient.Clientset.CoreV1().Secrets("kube-system").Create(ctx, secret, metav1.CreateOptions{})
	return err
}
