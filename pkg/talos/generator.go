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

	input, err := generate.NewInput(
        g.Config.ClusterName,
        endpoint,
        "v1.30.0", // TODO: Make configurable
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

    return cfgProvider.RawV1Alpha1(), nil
}

func GenerateSecrets(talosVersion string) (*secrets.Bundle, error) {
    versionContract, err := config.ParseContractFromVersion(talosVersion)
    if err != nil {
        return nil, fmt.Errorf("failed to parse version contract: %w", err)
    }

    return secrets.NewBundle(secrets.NewClock(), versionContract)
}
