// Package provisioning provides adapters for using CLI provisioners from the operator.
//
// The adapter layer wraps existing CLI provisioners (infrastructure, image, compute, cluster)
// and provides methods for the operator's state machine reconciliation. This eliminates
// code duplication by reusing the proven CLI provisioning code.
package provisioning

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/config"
	hcloudInternal "github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/platform/talos"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/provisioning/cluster"
	"github.com/imamik/k8zner/internal/provisioning/compute"
	"github.com/imamik/k8zner/internal/provisioning/image"
	"github.com/imamik/k8zner/internal/provisioning/infrastructure"
)

// Credentials holds the secrets needed for provisioning.
type Credentials struct {
	HCloudToken  string
	TalosSecrets []byte
	TalosConfig  []byte
}

// PhaseAdapter wraps existing CLI provisioners for operator use.
type PhaseAdapter struct {
	client client.Client

	// Provisioners (created on demand)
	infraProvisioner   *infrastructure.Provisioner
	imageProvisioner   *image.Provisioner
	computeProvisioner *compute.Provisioner
	clusterProvisioner *cluster.Provisioner
}

// NewPhaseAdapter creates a new provisioning adapter.
func NewPhaseAdapter(c client.Client) *PhaseAdapter {
	return &PhaseAdapter{
		client:             c,
		infraProvisioner:   infrastructure.NewProvisioner(),
		imageProvisioner:   image.NewProvisioner(),
		computeProvisioner: compute.NewProvisioner(),
		clusterProvisioner: cluster.NewProvisioner(),
	}
}

// LoadCredentials loads the credentials from the referenced Secret.
func (a *PhaseAdapter) LoadCredentials(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) (*Credentials, error) {
	logger := log.FromContext(ctx)

	if cluster.Spec.CredentialsRef.Name == "" {
		return nil, fmt.Errorf("credentialsRef.name is not set")
	}

	secret := &corev1.Secret{}
	key := client.ObjectKey{
		Namespace: cluster.Namespace,
		Name:      cluster.Spec.CredentialsRef.Name,
	}

	if err := a.client.Get(ctx, key, secret); err != nil {
		return nil, fmt.Errorf("failed to get credentials secret %s: %w", key.Name, err)
	}

	creds := &Credentials{}

	// HCloud token
	if token, ok := secret.Data[k8znerv1alpha1.CredentialsKeyHCloudToken]; ok {
		creds.HCloudToken = string(token)
	} else {
		return nil, fmt.Errorf("credentials secret missing key %s", k8znerv1alpha1.CredentialsKeyHCloudToken)
	}

	// Talos secrets (optional for existing clusters)
	if secrets, ok := secret.Data[k8znerv1alpha1.CredentialsKeyTalosSecrets]; ok {
		creds.TalosSecrets = secrets
	}

	// Talos config (optional for existing clusters)
	if cfg, ok := secret.Data[k8znerv1alpha1.CredentialsKeyTalosConfig]; ok {
		creds.TalosConfig = cfg
	}

	logger.V(1).Info("loaded credentials from secret",
		"secret", key.Name,
		"hasTalosSecrets", len(creds.TalosSecrets) > 0,
		"hasTalosConfig", len(creds.TalosConfig) > 0,
	)

	return creds, nil
}

// BuildProvisioningContext creates a provisioning context from the CRD spec and credentials.
func (a *PhaseAdapter) BuildProvisioningContext(
	ctx context.Context,
	cluster *k8znerv1alpha1.K8znerCluster,
	creds *Credentials,
	infraManager hcloudInternal.InfrastructureManager,
	talosProducer provisioning.TalosConfigProducer,
) (*provisioning.Context, error) {
	// Convert CRD spec to internal config
	cfg, err := SpecToConfig(cluster, creds)
	if err != nil {
		return nil, fmt.Errorf("failed to convert spec to config: %w", err)
	}

	// Create provisioning context with operator-specific observer
	pCtx := &provisioning.Context{
		Context:  ctx,
		Config:   cfg,
		State:    provisioning.NewState(),
		Infra:    infraManager,
		Talos:    talosProducer,
		Observer: NewOperatorObserver(ctx),
		Timeouts: config.LoadTimeouts(),
	}
	pCtx.Logger = pCtx.Observer

	return pCtx, nil
}

// ReconcileInfrastructure runs the infrastructure provisioning phase.
// Creates network, firewall, and load balancer.
func (a *PhaseAdapter) ReconcileInfrastructure(pCtx *provisioning.Context, cluster *k8znerv1alpha1.K8znerCluster) error {
	logger := log.FromContext(pCtx.Context)
	logger.Info("reconciling infrastructure")

	// Run the infrastructure provisioner
	if err := a.infraProvisioner.Provision(pCtx); err != nil {
		return fmt.Errorf("infrastructure provisioning failed: %w", err)
	}

	// Update CRD status with infrastructure IDs
	if pCtx.State.Network != nil {
		cluster.Status.Infrastructure.NetworkID = pCtx.State.Network.ID
	}
	if pCtx.State.Firewall != nil {
		cluster.Status.Infrastructure.FirewallID = pCtx.State.Firewall.ID
	}
	cluster.Status.Infrastructure.SSHKeyID = pCtx.State.SSHKeyID

	// Set condition
	SetCondition(cluster, k8znerv1alpha1.ConditionInfrastructureReady, metav1.ConditionTrue,
		"InfrastructureProvisioned", "Network, firewall, and load balancer created")

	return nil
}

// ReconcileImage ensures the Talos image snapshot exists.
func (a *PhaseAdapter) ReconcileImage(pCtx *provisioning.Context, cluster *k8znerv1alpha1.K8znerCluster) error {
	logger := log.FromContext(pCtx.Context)
	logger.Info("reconciling image")

	// Run the image provisioner
	if err := a.imageProvisioner.Provision(pCtx); err != nil {
		return fmt.Errorf("image provisioning failed: %w", err)
	}

	// Get snapshot info and update status
	snapshot, err := pCtx.Infra.GetSnapshotByLabels(pCtx.Context, map[string]string{
		"talos_version": pCtx.Config.Talos.Version,
	})
	if err == nil && snapshot != nil {
		now := metav1.Now()
		cluster.Status.ImageSnapshot = &k8znerv1alpha1.ImageStatus{
			SnapshotID:  snapshot.ID,
			Version:     pCtx.Config.Talos.Version,
			SchematicID: pCtx.Config.Talos.SchematicID,
			CreatedAt:   &now,
		}
	}

	// Set condition
	SetCondition(cluster, k8znerv1alpha1.ConditionImageReady, metav1.ConditionTrue,
		"ImageAvailable", "Talos image snapshot is available")

	return nil
}

// ReconcileCompute provisions the remaining control plane and worker servers.
// Skips the bootstrap node if it already exists.
func (a *PhaseAdapter) ReconcileCompute(pCtx *provisioning.Context, cluster *k8znerv1alpha1.K8znerCluster) error {
	logger := log.FromContext(pCtx.Context)
	logger.Info("reconciling compute")

	// If bootstrap node exists, we need to account for it
	if cluster.Spec.Bootstrap != nil && cluster.Spec.Bootstrap.Completed {
		// Add bootstrap node to state so compute provisioner doesn't recreate it
		bootstrapName := cluster.Spec.Bootstrap.BootstrapNode
		if bootstrapName != "" {
			pCtx.State.ControlPlaneIPs[bootstrapName] = cluster.Spec.Bootstrap.PublicIP
			pCtx.State.ControlPlaneServerIDs[bootstrapName] = cluster.Spec.Bootstrap.BootstrapNodeID
		}
	}

	// Run the compute provisioner (will create remaining nodes)
	if err := a.computeProvisioner.Provision(pCtx); err != nil {
		return fmt.Errorf("compute provisioning failed: %w", err)
	}

	// Update node statuses from provisioning results
	updateNodeStatuses(cluster, pCtx.State)

	return nil
}

// ReconcileBootstrap applies Talos configs and bootstraps the cluster.
func (a *PhaseAdapter) ReconcileBootstrap(pCtx *provisioning.Context, cluster *k8znerv1alpha1.K8znerCluster) error {
	logger := log.FromContext(pCtx.Context)
	logger.Info("reconciling bootstrap")

	// Run the cluster provisioner (applies configs, bootstraps etcd)
	if err := a.clusterProvisioner.Provision(pCtx); err != nil {
		return fmt.Errorf("cluster bootstrap failed: %w", err)
	}

	// Set condition
	SetCondition(cluster, k8znerv1alpha1.ConditionBootstrapped, metav1.ConditionTrue,
		"ClusterBootstrapped", "Cluster has been bootstrapped successfully")

	return nil
}

// AttachBootstrapNodeToInfrastructure attaches the bootstrap control plane
// to the network, firewall, and load balancer using the provisioning context.
// This is called after infrastructure is created to integrate the bootstrap node.
func (a *PhaseAdapter) AttachBootstrapNodeToInfrastructure(
	pCtx *provisioning.Context,
	cluster *k8znerv1alpha1.K8znerCluster,
) error {
	logger := log.FromContext(pCtx.Context)

	if cluster.Spec.Bootstrap == nil || !cluster.Spec.Bootstrap.Completed {
		return fmt.Errorf("bootstrap state not available")
	}

	bootstrapName := cluster.Spec.Bootstrap.BootstrapNode
	if bootstrapName == "" {
		return fmt.Errorf("bootstrap node name is not set")
	}

	logger.Info("attaching bootstrap node to infrastructure",
		"nodeName", bootstrapName,
		"serverID", cluster.Spec.Bootstrap.BootstrapNodeID,
	)

	// Get the network ID from the provisioning state or cluster status
	networkID := pCtx.State.Network.ID
	if networkID == 0 {
		networkID = cluster.Status.Infrastructure.NetworkID
	}
	if networkID == 0 {
		return fmt.Errorf("network ID not available - infrastructure must be provisioned first")
	}

	// Check if server is already attached to the network
	server, err := pCtx.Infra.GetServerByName(pCtx.Context, bootstrapName)
	if err != nil {
		return fmt.Errorf("failed to get bootstrap server: %w", err)
	}
	if server == nil {
		return fmt.Errorf("bootstrap server not found: %s", bootstrapName)
	}

	// Check if already attached
	for _, pn := range server.PrivateNet {
		if pn.Network.ID == networkID {
			logger.Info("bootstrap node already attached to network",
				"nodeName", bootstrapName,
				"networkID", networkID,
				"privateIP", pn.IP.String(),
			)
			return nil
		}
	}

	// Calculate private IP for bootstrap node (first CP in subnet)
	// Control planes use subnet index 0, which is 10.0.0.0/24 by default
	// First CP gets IP .2 (after network .0 and gateway .1)
	networkCIDR := defaultString(cluster.Spec.Network.IPv4CIDR, "10.0.0.0/16")
	cpSubnet, err := config.CIDRSubnet(networkCIDR, 8, 0) // Control plane subnet
	if err != nil {
		return fmt.Errorf("failed to calculate control plane subnet: %w", err)
	}

	privateIP, err := config.CIDRHost(cpSubnet, 2) // First CP IP (.2)
	if err != nil {
		return fmt.Errorf("failed to calculate bootstrap node IP: %w", err)
	}

	logger.Info("attaching bootstrap node to network",
		"nodeName", bootstrapName,
		"networkID", networkID,
		"privateIP", privateIP,
	)

	// Attach the server to the network
	if err := pCtx.Infra.AttachServerToNetwork(pCtx.Context, bootstrapName, networkID, privateIP); err != nil {
		return fmt.Errorf("failed to attach bootstrap node to network: %w", err)
	}

	logger.Info("bootstrap node attached to infrastructure",
		"nodeName", bootstrapName,
		"networkID", networkID,
		"privateIP", privateIP,
	)

	return nil
}

// updateNodeStatuses updates the cluster status with node information from provisioning state.
func updateNodeStatuses(cluster *k8znerv1alpha1.K8znerCluster, state *provisioning.State) {
	// Update control plane nodes
	for name, ip := range state.ControlPlaneIPs {
		serverID := state.ControlPlaneServerIDs[name]
		found := false
		for i := range cluster.Status.ControlPlanes.Nodes {
			if cluster.Status.ControlPlanes.Nodes[i].Name == name {
				cluster.Status.ControlPlanes.Nodes[i].PublicIP = ip
				cluster.Status.ControlPlanes.Nodes[i].ServerID = serverID
				found = true
				break
			}
		}
		if !found {
			cluster.Status.ControlPlanes.Nodes = append(cluster.Status.ControlPlanes.Nodes,
				k8znerv1alpha1.NodeStatus{
					Name:     name,
					ServerID: serverID,
					PublicIP: ip,
				})
		}
	}

	// Update worker nodes
	for name, ip := range state.WorkerIPs {
		serverID := state.WorkerServerIDs[name]
		found := false
		for i := range cluster.Status.Workers.Nodes {
			if cluster.Status.Workers.Nodes[i].Name == name {
				cluster.Status.Workers.Nodes[i].PublicIP = ip
				cluster.Status.Workers.Nodes[i].ServerID = serverID
				found = true
				break
			}
		}
		if !found {
			cluster.Status.Workers.Nodes = append(cluster.Status.Workers.Nodes,
				k8znerv1alpha1.NodeStatus{
					Name:     name,
					ServerID: serverID,
					PublicIP: ip,
				})
		}
	}
}

// SetCondition sets a condition on the cluster status.
func SetCondition(cluster *k8znerv1alpha1.K8znerCluster, condType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	condition := metav1.Condition{
		Type:               condType,
		Status:             status,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}

	// Find and update existing condition or append new one
	for i, c := range cluster.Status.Conditions {
		if c.Type == condType {
			if c.Status != status {
				cluster.Status.Conditions[i] = condition
			}
			return
		}
	}
	cluster.Status.Conditions = append(cluster.Status.Conditions, condition)
}

// SpecToConfig converts a K8znerCluster spec to the internal config.Config format.
func SpecToConfig(cluster *k8znerv1alpha1.K8znerCluster, creds *Credentials) (*config.Config, error) {
	spec := &cluster.Spec

	cfg := &config.Config{
		ClusterName: cluster.Name,
		HCloudToken: creds.HCloudToken,
		Location:    spec.Region,

		// Network configuration
		Network: config.NetworkConfig{
			IPv4CIDR:        defaultString(spec.Network.IPv4CIDR, "10.0.0.0/16"),
			PodIPv4CIDR:     defaultString(spec.Network.PodCIDR, "10.244.0.0/16"),
			ServiceIPv4CIDR: defaultString(spec.Network.ServiceCIDR, "10.96.0.0/16"),
		},

		// Talos configuration
		Talos: config.TalosConfig{
			Version:     spec.Talos.Version,
			SchematicID: spec.Talos.SchematicID,
			Extensions:  spec.Talos.Extensions,
		},

		// Kubernetes configuration
		Kubernetes: config.KubernetesConfig{
			Version:                spec.Kubernetes.Version,
			APILoadBalancerEnabled: true, // Always enable LB for operator-managed clusters
		},

		// Control plane configuration
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{
					Name:       "control-plane",
					Location:   spec.Region,
					ServerType: normalizeServerSize(spec.ControlPlanes.Size),
					Count:      spec.ControlPlanes.Count,
				},
			},
		},

		// Worker configuration
		Workers: []config.WorkerNodePool{
			{
				Name:       "worker",
				Location:   spec.Region,
				ServerType: normalizeServerSize(spec.Workers.Size),
				Count:      spec.Workers.Count,
			},
		},

		// Enable essential addons
		Addons: config.AddonsConfig{
			Cilium: config.CiliumConfig{
				Enabled: true,
			},
			CCM: config.CCMConfig{
				Enabled: true,
			},
			CSI: config.CSIConfig{
				Enabled: true,
			},
			MetricsServer: config.MetricsServerConfig{
				Enabled: spec.Addons != nil && spec.Addons.MetricsServer,
			},
			CertManager: config.CertManagerConfig{
				Enabled: spec.Addons != nil && spec.Addons.CertManager,
			},
			Traefik: config.TraefikConfig{
				Enabled: spec.Addons != nil && spec.Addons.Traefik,
			},
			ArgoCD: config.ArgoCDConfig{
				Enabled: spec.Addons != nil && spec.Addons.ArgoCD,
			},
		},
	}

	return cfg, nil
}

// normalizeServerSize converts legacy server type names to current Hetzner names.
func normalizeServerSize(size string) string {
	// Map legacy types to current types
	legacyMap := map[string]string{
		"cx22": "cx23",
		"cx32": "cx33",
		"cx42": "cx43",
		"cx52": "cx53",
	}
	if newSize, ok := legacyMap[strings.ToLower(size)]; ok {
		return newSize
	}
	return size
}

// defaultString returns the value if non-empty, otherwise the default.
func defaultString(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

// ParseSecretsFromBytes parses a Talos secrets bundle from YAML bytes.
// This is used when loading secrets from a Kubernetes Secret instead of a file.
func ParseSecretsFromBytes(data []byte) (*secrets.Bundle, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty secrets data")
	}

	var sb secrets.Bundle
	if err := yaml.Unmarshal(data, &sb); err != nil {
		return nil, fmt.Errorf("failed to unmarshal secrets bundle: %w", err)
	}

	// Re-inject clock (required for certificate generation)
	sb.Clock = secrets.NewFixedClock(time.Now())

	return &sb, nil
}

// CreateTalosGenerator creates a TalosConfigProducer from the cluster spec and credentials.
// This is used by the operator to generate Talos configs for new nodes.
func (a *PhaseAdapter) CreateTalosGenerator(
	cluster *k8znerv1alpha1.K8znerCluster,
	creds *Credentials,
) (provisioning.TalosConfigProducer, error) {
	// Parse secrets from the credential data
	sb, err := ParseSecretsFromBytes(creds.TalosSecrets)
	if err != nil {
		return nil, fmt.Errorf("failed to parse talos secrets: %w", err)
	}

	// Determine the endpoint
	// Priority: 1. Load balancer IP (if exists), 2. Bootstrap public IP, 3. Cluster name
	endpoint := cluster.Name
	if cluster.Status.ControlPlaneEndpoint != "" {
		endpoint = cluster.Status.ControlPlaneEndpoint
	} else if cluster.Spec.Bootstrap != nil && cluster.Spec.Bootstrap.PublicIP != "" {
		endpoint = cluster.Spec.Bootstrap.PublicIP
	}

	// Create the generator
	generator := talos.NewGenerator(
		cluster.Name,
		cluster.Spec.Kubernetes.Version,
		cluster.Spec.Talos.Version,
		endpoint,
		sb,
	)

	// Set machine config options from spec
	machineOpts := &talos.MachineConfigOptions{
		SchematicID: cluster.Spec.Talos.SchematicID,
		// Set defaults for operator-managed clusters
		StateEncryption:         true,
		EphemeralEncryption:     true,
		IPv6Enabled:             true,
		PublicIPv4Enabled:       true,
		PublicIPv6Enabled:       true,
		CoreDNSEnabled:          true,
		DiscoveryServiceEnabled: true,
		KubeProxyReplacement:    true, // Cilium replaces kube-proxy
		// Network context from spec
		NodeIPv4CIDR:    defaultString(cluster.Spec.Network.IPv4CIDR, "10.0.0.0/16"),
		PodIPv4CIDR:     defaultString(cluster.Spec.Network.PodCIDR, "10.244.0.0/16"),
		ServiceIPv4CIDR: defaultString(cluster.Spec.Network.ServiceCIDR, "10.96.0.0/16"),
		EtcdSubnet:      defaultString(cluster.Spec.Network.IPv4CIDR, "10.0.0.0/16"),
	}

	generator.SetMachineConfigOptions(machineOpts)

	return generator, nil
}

// OperatorObserver implements provisioning.Observer for operator context.
type OperatorObserver struct {
	ctx    context.Context
	fields map[string]string
}

// NewOperatorObserver creates a new operator observer.
func NewOperatorObserver(ctx context.Context) *OperatorObserver {
	return &OperatorObserver{
		ctx:    ctx,
		fields: make(map[string]string),
	}
}

// Printf implements the Logger interface.
func (o *OperatorObserver) Printf(format string, v ...interface{}) {
	logger := log.FromContext(o.ctx)
	logger.Info(fmt.Sprintf(format, v...))
}

// Event implements provisioning.Observer.
func (o *OperatorObserver) Event(event provisioning.Event) {
	logger := log.FromContext(o.ctx)

	// Merge context fields with event fields
	fields := make(map[string]string)
	for k, v := range o.fields {
		fields[k] = v
	}
	for k, v := range event.Fields {
		fields[k] = v
	}

	// Convert to key-value pairs for structured logging
	keysAndValues := make([]interface{}, 0, len(fields)*2+4)
	keysAndValues = append(keysAndValues, "eventType", string(event.Type))
	if event.Phase != "" {
		keysAndValues = append(keysAndValues, "phase", event.Phase)
	}
	if event.Resource != "" {
		keysAndValues = append(keysAndValues, "resource", event.Resource)
	}
	for k, v := range fields {
		keysAndValues = append(keysAndValues, k, v)
	}

	switch event.Type {
	case provisioning.EventPhaseFailed, provisioning.EventResourceFailed, provisioning.EventValidationError:
		logger.Error(nil, event.Message, keysAndValues...)
	default:
		logger.Info(event.Message, keysAndValues...)
	}
}

// Progress implements provisioning.Observer.
func (o *OperatorObserver) Progress(phase string, current, total int) {
	logger := log.FromContext(o.ctx)
	logger.V(1).Info("progress", "phase", phase, "current", current, "total", total)
}

// WithFields implements provisioning.Observer.
func (o *OperatorObserver) WithFields(fields map[string]string) provisioning.Observer {
	newFields := make(map[string]string)
	for k, v := range o.fields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}
	return &OperatorObserver{
		ctx:    o.ctx,
		fields: newFields,
	}
}
