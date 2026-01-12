package addons

import (
	"context"
	"fmt"

	"hcloud-k8s/internal/addons/helm"
)

const (
	csiReleaseName  = "hcloud-csi"
	csiNamespace    = "kube-system"
	csiRepoURL      = "https://charts.hetzner.cloud"
	csiChartName    = "hcloud-csi"
	csiDefaultVersion = "2.18.3" // Must match version available in charts.hetzner.cloud
)

// applyCSI installs the Hetzner Cloud CSI driver using Helm.
// It expects the 'hcloud' secret to already exist in kube-system namespace
// (created by the CCM addon).
func applyCSI(ctx context.Context, kubeconfig []byte, controlPlaneCount int, defaultStorageClass bool) error {
	// Create Helm client
	helmClient, err := helm.NewClient(kubeconfig, csiNamespace)
	if err != nil {
		return fmt.Errorf("failed to create helm client: %w", err)
	}

	// Build Helm values
	values := buildCSIValues(controlPlaneCount, defaultStorageClass)

	// Install or upgrade
	_, err = helmClient.InstallOrUpgrade(ctx, csiReleaseName, csiRepoURL, csiChartName, csiDefaultVersion, values)
	if err != nil {
		return fmt.Errorf("failed to install/upgrade CSI driver: %w", err)
	}

	return nil
}

func buildCSIValues(controlPlaneCount int, defaultStorageClass bool) map[string]any {
	// Calculate controller replicas based on control plane count
	controllerReplicas := 1
	if controlPlaneCount > 1 {
		controllerReplicas = 2
	}

	// DNS readiness init container to handle Cilium CNI initialization race
	dnsInitContainer := map[string]any{
		"name":            "wait-for-dns",
		"image":           "busybox:1.36",
		"imagePullPolicy": "IfNotPresent",
		"command": []string{
			"sh",
			"-c",
			`echo "Waiting for DNS to be ready..."
MAX_RETRIES=60
RETRY_COUNT=0

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
  if nslookup kubernetes.default.svc.cluster.local > /dev/null 2>&1; then
    echo "DNS is ready!"
    exit 0
  fi
  echo "DNS not ready yet, waiting... (attempt $((RETRY_COUNT + 1))/$MAX_RETRIES)"
  RETRY_COUNT=$((RETRY_COUNT + 1))
  sleep 2
done

echo "ERROR: DNS did not become ready after $MAX_RETRIES attempts"
exit 1`,
		},
	}

	values := map[string]any{
		"controller": map[string]any{
			"replicaCount": controllerReplicas,
			"podDisruptionBudget": map[string]any{
				"create":         true,
				"minAvailable":   nil,
				"maxUnavailable": "1",
			},
			"topologySpreadConstraints": []map[string]any{
				{
					"topologyKey":       "kubernetes.io/hostname",
					"maxSkew":           1,
					"whenUnsatisfiable": "DoNotSchedule",
					"labelSelector": map[string]any{
						"matchLabels": map[string]string{
							"app.kubernetes.io/name":      "hcloud-csi",
							"app.kubernetes.io/instance":  "hcloud-csi",
							"app.kubernetes.io/component": "controller",
						},
					},
					"matchLabelKeys": []string{"pod-template-hash"},
				},
			},
			"nodeSelector": map[string]string{
				"node-role.kubernetes.io/control-plane": "",
			},
			"tolerations": []map[string]any{
				{
					"key":      "node-role.kubernetes.io/control-plane",
					"effect":   "NoSchedule",
					"operator": "Exists",
				},
			},
			"initContainers": []map[string]any{dnsInitContainer},
		},
		"node": map[string]any{
			"initContainers": []map[string]any{dnsInitContainer},
		},
		"storageClasses": []map[string]any{
			{
				"name":                "hcloud-volumes",
				"defaultStorageClass": defaultStorageClass,
				"reclaimPolicy":       "Delete",
			},
		},
	}

	return values
}
