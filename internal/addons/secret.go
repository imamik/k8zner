package addons

import (
	"context"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"hcloud-k8s/internal/addons/k8sclient"
)

// createHCloudSecret creates the hcloud secret that addon charts reference.
// This secret must exist before applying CCM or CSI addons.
func createHCloudSecret(ctx context.Context, client k8sclient.Client, token string, networkID int64) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hcloud",
			Namespace: "kube-system",
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"token":   token,
			"network": strconv.FormatInt(networkID, 10),
		},
	}

	if err := client.CreateSecret(ctx, secret); err != nil {
		return fmt.Errorf("failed to create hcloud secret: %w", err)
	}

	return nil
}
