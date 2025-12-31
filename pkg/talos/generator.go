package talos

import (
	"fmt"

	clusterconfig "github.com/hcloud-k8s/hcloud-k8s/pkg/config"
	"github.com/siderolabs/talos/pkg/machinery/config"
	"github.com/siderolabs/talos/pkg/machinery/config/types/v1alpha1"
	"github.com/siderolabs/talos/pkg/machinery/config/generate"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
    "github.com/siderolabs/talos/pkg/machinery/config/machine"
)

type Generator struct {
	Config   *clusterconfig.ClusterConfig
    Endpoint string
}

func NewGenerator(cfg *clusterconfig.ClusterConfig) *Generator {
	return &Generator{Config: cfg}
}

func (g *Generator) Generate(role string, nodeName string, secretsBundle *secrets.Bundle) (*v1alpha1.Config, error) {
    versionContract, err := config.ParseContractFromVersion(g.Config.TalosVersion)
    if err != nil {
        return nil, fmt.Errorf("failed to parse version contract: %w", err)
    }

    var machineType machine.Type
    if role == "controlplane" {
        machineType = machine.TypeControlPlane
    } else {
        machineType = machine.TypeWorker
    }

    endpoint := g.Endpoint
    if endpoint == "" {
        endpoint = "https://127.0.0.1:6443" // Fallback
    }

    k8sVersion := g.Config.KubernetesVersion
    if k8sVersion == "" {
        k8sVersion = "v1.30.0"
    }

	input, err := generate.NewInput(
        g.Config.ClusterName,
        endpoint,
        k8sVersion,
        generate.WithSecretsBundle(secretsBundle),
        generate.WithVersionContract(versionContract),
    )
    if err != nil {
        return nil, err
    }

    cfgProvider, err := input.Config(machineType)
    if err != nil {
        return nil, err
    }

    cfg := cfgProvider.RawV1Alpha1()

    // Apply Addons (Manifests)
    // In a real scenario, we would add extra manifests here.
    // For now, we will just placeholders or minimal logic if requested.

    // Logic to add CCM/CSI manifests would go here if we had the manifests.
    // Since we don't have the full manifest content in the repo (it was fetched or templated in Terraform),
    // implementing full AddonManager might be out of scope for "Fix Bugs".
    // However, I can add a method to inject manifests.

    return cfg, nil
}

func GenerateSecrets(talosVersion string) (*secrets.Bundle, error) {
    versionContract, err := config.ParseContractFromVersion(talosVersion)
    if err != nil {
        return nil, fmt.Errorf("failed to parse version contract: %w", err)
    }

    return secrets.NewBundle(secrets.NewClock(), versionContract)
}
