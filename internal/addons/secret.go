package addons

import (
	"context"
	"fmt"
	"log"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/imamik/k8zner/internal/addons/k8sclient"
)

// createHCloudSecret creates the hcloud secret that addon charts reference.
// This secret must exist before applying CCM or CSI addons.
// The secret contains:
//   - token: HCloud API token for CCM/CSI to manage cloud resources
//   - network: Network ID for CCM to configure routes and load balancers
func createHCloudSecret(ctx context.Context, client k8sclient.Client, token string, networkID int64) error {
	// Validate inputs - these are required for CCM/CSI to function
	if token == "" {
		return fmt.Errorf("hcloud token is empty - CCM/CSI will not be able to manage cloud resources")
	}
	if networkID == 0 {
		return fmt.Errorf("network ID is 0 - CCM will not be able to configure routes")
	}

	log.Printf("[addons] Creating hcloud secret in kube-system (networkID=%d, tokenLength=%d)", networkID, len(token))

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hcloud",
			Namespace: "kube-system",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "k8zner",
				"app.kubernetes.io/component":  "cloud-credentials",
			},
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

	log.Printf("[addons] Successfully created hcloud secret in kube-system")
	return nil
}
