// Package provisioning provides adapters for using CLI provisioners from the operator.
//
// The adapter layer wraps existing CLI provisioners (infrastructure, image, compute, cluster)
// and provides methods for the operator's state machine reconciliation. This eliminates
// code duplication by reusing the proven CLI provisioning code.
package provisioning

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/config"
	hcloudInternal "github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/provisioning/cluster"
	"github.com/imamik/k8zner/internal/provisioning/compute"
	"github.com/imamik/k8zner/internal/provisioning/image"
	"github.com/imamik/k8zner/internal/provisioning/infrastructure"
	"github.com/imamik/k8zner/internal/util/naming"
)

// Credentials holds the secrets needed for provisioning.
type Credentials struct {
	HCloudToken        string
	TalosSecrets       []byte
	TalosConfig        []byte
	CloudflareAPIToken string // Optional, for DNS/TLS integration

	// Backup S3 credentials (loaded from S3SecretRef if specified)
	BackupS3AccessKey string
	BackupS3SecretKey string
	BackupS3Endpoint  string
	BackupS3Bucket    string
	BackupS3Region    string
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
func (a *PhaseAdapter) LoadCredentials(ctx context.Context, k8sCluster *k8znerv1alpha1.K8znerCluster) (*Credentials, error) {
	logger := log.FromContext(ctx)

	if k8sCluster.Spec.CredentialsRef.Name == "" {
		return nil, fmt.Errorf("credentialsRef.name is not set")
	}

	secret := &corev1.Secret{}
	key := client.ObjectKey{
		Namespace: k8sCluster.Namespace,
		Name:      k8sCluster.Spec.CredentialsRef.Name,
	}

	if err := a.client.Get(ctx, key, secret); err != nil {
		return nil, fmt.Errorf("failed to get credentials secret %s: %w", key.Name, err)
	}

	creds, err := extractCredentials(secret)
	if err != nil {
		return nil, err
	}

	if err := a.loadBackupCredentials(ctx, k8sCluster, creds, logger); err != nil {
		logger.Error(err, "failed to load backup S3 secret, backup will be skipped")
	}

	logger.V(1).Info("loaded credentials from secret",
		"secret", key.Name,
		"hasTalosSecrets", len(creds.TalosSecrets) > 0,
		"hasTalosConfig", len(creds.TalosConfig) > 0,
		"hasCloudflareToken", len(creds.CloudflareAPIToken) > 0,
		"hasBackupS3Creds", creds.BackupS3AccessKey != "",
	)

	return creds, nil
}

// extractCredentials extracts core credentials from a Kubernetes Secret.
func extractCredentials(secret *corev1.Secret) (*Credentials, error) {
	creds := &Credentials{}

	if token, ok := secret.Data[k8znerv1alpha1.CredentialsKeyHCloudToken]; ok {
		creds.HCloudToken = string(token)
	} else {
		return nil, fmt.Errorf("credentials secret missing key %s", k8znerv1alpha1.CredentialsKeyHCloudToken)
	}

	if talosSecretData, ok := secret.Data[k8znerv1alpha1.CredentialsKeyTalosSecrets]; ok {
		creds.TalosSecrets = talosSecretData
	}

	if cfg, ok := secret.Data[k8znerv1alpha1.CredentialsKeyTalosConfig]; ok {
		creds.TalosConfig = cfg
	}

	if cfToken, ok := secret.Data[k8znerv1alpha1.CredentialsKeyCloudflareAPIToken]; ok {
		creds.CloudflareAPIToken = string(cfToken)
	}

	return creds, nil
}

// loadBackupCredentials loads backup S3 credentials from a referenced Secret.
func (a *PhaseAdapter) loadBackupCredentials(ctx context.Context, k8sCluster *k8znerv1alpha1.K8znerCluster, creds *Credentials, logger interface{ Info(string, ...interface{}) }) error {
	if k8sCluster.Spec.Backup == nil || k8sCluster.Spec.Backup.S3SecretRef == nil || k8sCluster.Spec.Backup.S3SecretRef.Name == "" {
		return nil
	}

	backupSecret := &corev1.Secret{}
	backupKey := client.ObjectKey{
		Namespace: k8sCluster.Namespace,
		Name:      k8sCluster.Spec.Backup.S3SecretRef.Name,
	}

	if err := a.client.Get(ctx, backupKey, backupSecret); err != nil {
		return err
	}

	creds.BackupS3AccessKey = string(backupSecret.Data["access-key"])
	creds.BackupS3SecretKey = string(backupSecret.Data["secret-key"])
	creds.BackupS3Endpoint = string(backupSecret.Data["endpoint"])
	creds.BackupS3Bucket = string(backupSecret.Data["bucket"])
	creds.BackupS3Region = string(backupSecret.Data["region"])
	logger.Info("loaded backup S3 credentials from secret", "secret", backupKey.Name)

	return nil
}

// BuildProvisioningContext creates a provisioning context from the CRD spec and credentials.
func (a *PhaseAdapter) BuildProvisioningContext(
	ctx context.Context,
	k8sCluster *k8znerv1alpha1.K8znerCluster,
	creds *Credentials,
	infraManager hcloudInternal.InfrastructureManager,
	talosProducer provisioning.TalosConfigProducer,
) (*provisioning.Context, error) {
	// Convert CRD spec to internal config
	cfg, err := SpecToConfig(k8sCluster, creds)
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
func (a *PhaseAdapter) ReconcileInfrastructure(pCtx *provisioning.Context, k8sCluster *k8znerv1alpha1.K8znerCluster) error {
	logger := log.FromContext(pCtx.Context)
	logger.Info("reconciling infrastructure")

	if err := a.infraProvisioner.Provision(pCtx); err != nil {
		return fmt.Errorf("infrastructure provisioning failed: %w", err)
	}

	if pCtx.State.Network != nil {
		k8sCluster.Status.Infrastructure.NetworkID = pCtx.State.Network.ID
	}
	if pCtx.State.Firewall != nil {
		k8sCluster.Status.Infrastructure.FirewallID = pCtx.State.Firewall.ID
	}
	k8sCluster.Status.Infrastructure.SSHKeyID = pCtx.State.SSHKeyID

	SetCondition(k8sCluster, k8znerv1alpha1.ConditionInfrastructureReady, metav1.ConditionTrue,
		"InfrastructureProvisioned", "Network, firewall, and load balancer created")

	return nil
}

// ReconcileImage ensures the Talos image snapshot exists.
func (a *PhaseAdapter) ReconcileImage(pCtx *provisioning.Context, k8sCluster *k8znerv1alpha1.K8znerCluster) error {
	logger := log.FromContext(pCtx.Context)
	logger.Info("reconciling image")

	if err := a.imageProvisioner.Provision(pCtx); err != nil {
		return fmt.Errorf("image provisioning failed: %w", err)
	}

	snapshot, err := pCtx.Infra.GetSnapshotByLabels(pCtx.Context, map[string]string{
		"talos_version": pCtx.Config.Talos.Version,
	})
	if err == nil && snapshot != nil {
		now := metav1.Now()
		k8sCluster.Status.ImageSnapshot = &k8znerv1alpha1.ImageStatus{
			SnapshotID:  snapshot.ID,
			Version:     pCtx.Config.Talos.Version,
			SchematicID: pCtx.Config.Talos.SchematicID,
			CreatedAt:   &now,
		}
		k8sCluster.Status.Infrastructure.SnapshotID = snapshot.ID
	}

	SetCondition(k8sCluster, k8znerv1alpha1.ConditionImageReady, metav1.ConditionTrue,
		"ImageAvailable", "Talos image snapshot is available")

	return nil
}

// ReconcileCompute provisions the remaining control plane and worker servers.
func (a *PhaseAdapter) ReconcileCompute(pCtx *provisioning.Context, k8sCluster *k8znerv1alpha1.K8znerCluster) error {
	logger := log.FromContext(pCtx.Context)
	logger.Info("reconciling compute")

	// If bootstrap node exists, we need to account for it
	if k8sCluster.Spec.Bootstrap != nil && k8sCluster.Spec.Bootstrap.Completed {
		populateBootstrapState(pCtx, k8sCluster, logger)
	}

	if err := a.computeProvisioner.Provision(pCtx); err != nil {
		return fmt.Errorf("compute provisioning failed: %w", err)
	}

	updateNodeStatuses(k8sCluster, pCtx.State)

	// Try to get placement group ID for status (optional)
	pgName := naming.PlacementGroup(k8sCluster.Name, "control-plane")
	if pg, err := pCtx.Infra.GetPlacementGroup(pCtx.Context, pgName); err == nil && pg != nil {
		k8sCluster.Status.Infrastructure.PlacementGroupID = pg.ID
	}

	return nil
}

// populateBootstrapState adds bootstrap node info to state and limits counts for CLI-bootstrapped clusters.
func populateBootstrapState(pCtx *provisioning.Context, k8sCluster *k8znerv1alpha1.K8znerCluster, logger interface{ Info(string, ...interface{}) }) {
	bootstrapName := k8sCluster.Spec.Bootstrap.BootstrapNode
	if bootstrapName != "" {
		pCtx.State.ControlPlaneIPs[bootstrapName] = k8sCluster.Spec.Bootstrap.PublicIP
		pCtx.State.ControlPlaneServerIDs[bootstrapName] = k8sCluster.Spec.Bootstrap.BootstrapNodeID
	}

	// Limit counts for CLI-bootstrapped clusters - Running-phase handles scale-up
	for i := range pCtx.Config.ControlPlane.NodePools {
		if pCtx.Config.ControlPlane.NodePools[i].Count > 1 {
			logger.Info("limiting CP count to 1 for CLI-bootstrapped cluster (Running-phase will scale up)",
				"originalCount", pCtx.Config.ControlPlane.NodePools[i].Count)
			pCtx.Config.ControlPlane.NodePools[i].Count = 1
		}
	}
	for i := range pCtx.Config.Workers {
		if pCtx.Config.Workers[i].Count > 0 {
			logger.Info("limiting worker count to 0 for CLI-bootstrapped cluster (Running-phase will scale up)",
				"originalCount", pCtx.Config.Workers[i].Count)
			pCtx.Config.Workers[i].Count = 0
		}
	}
}

// ReconcileBootstrap applies Talos configs and bootstraps the cluster.
func (a *PhaseAdapter) ReconcileBootstrap(pCtx *provisioning.Context, k8sCluster *k8znerv1alpha1.K8znerCluster) error {
	logger := log.FromContext(pCtx.Context)
	logger.Info("reconciling bootstrap")

	// Populate state from CRD status (filled during Compute phase)
	populateStateFromCRD(pCtx, k8sCluster, logger)

	if err := a.clusterProvisioner.Provision(pCtx); err != nil {
		return fmt.Errorf("cluster bootstrap failed: %w", err)
	}

	SetCondition(k8sCluster, k8znerv1alpha1.ConditionBootstrapped, metav1.ConditionTrue,
		"ClusterBootstrapped", "Cluster has been bootstrapped successfully")

	return nil
}

// populateStateFromCRD populates provisioning state from the CRD status for bootstrap.
func populateStateFromCRD(pCtx *provisioning.Context, k8sCluster *k8znerv1alpha1.K8znerCluster, logger interface{ Info(string, ...interface{}) }) {
	for _, node := range k8sCluster.Status.ControlPlanes.Nodes {
		if node.Name != "" && node.PublicIP != "" {
			pCtx.State.ControlPlaneIPs[node.Name] = node.PublicIP
			pCtx.State.ControlPlaneServerIDs[node.Name] = node.ServerID
		}
	}
	for _, node := range k8sCluster.Status.Workers.Nodes {
		if node.Name != "" && node.PublicIP != "" {
			pCtx.State.WorkerIPs[node.Name] = node.PublicIP
			pCtx.State.WorkerServerIDs[node.Name] = node.ServerID
		}
	}

	// Populate SANs from infrastructure status for valid TLS certificates
	var sans []string
	if k8sCluster.Status.Infrastructure.LoadBalancerIP != "" {
		sans = append(sans, k8sCluster.Status.Infrastructure.LoadBalancerIP)
	}
	if k8sCluster.Status.Infrastructure.LoadBalancerPrivateIP != "" {
		sans = append(sans, k8sCluster.Status.Infrastructure.LoadBalancerPrivateIP)
	}
	pCtx.State.SANs = sans

	logger.Info("populated state from CRD status for bootstrap",
		"controlPlaneCount", len(pCtx.State.ControlPlaneIPs),
		"workerCount", len(pCtx.State.WorkerIPs),
		"controlPlaneIPs", pCtx.State.ControlPlaneIPs,
		"workerIPs", pCtx.State.WorkerIPs,
		"SANs", pCtx.State.SANs,
	)
}

// AttachBootstrapNodeToInfrastructure attaches the bootstrap control plane
// to the network, firewall, and load balancer.
func (a *PhaseAdapter) AttachBootstrapNodeToInfrastructure(
	pCtx *provisioning.Context,
	k8sCluster *k8znerv1alpha1.K8znerCluster,
) error {
	logger := log.FromContext(pCtx.Context)

	if k8sCluster.Spec.Bootstrap == nil || !k8sCluster.Spec.Bootstrap.Completed {
		return fmt.Errorf("bootstrap state not available")
	}

	bootstrapName := k8sCluster.Spec.Bootstrap.BootstrapNode
	if bootstrapName == "" {
		return fmt.Errorf("bootstrap node name is not set")
	}

	logger.Info("attaching bootstrap node to infrastructure",
		"nodeName", bootstrapName,
		"serverID", k8sCluster.Spec.Bootstrap.BootstrapNodeID,
	)

	networkID := pCtx.State.Network.ID
	if networkID == 0 {
		networkID = k8sCluster.Status.Infrastructure.NetworkID
	}
	if networkID == 0 {
		return fmt.Errorf("network ID not available - infrastructure must be provisioned first")
	}

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

	privateIP, err := calculateBootstrapNodeIP(k8sCluster)
	if err != nil {
		return err
	}

	logger.Info("attaching bootstrap node to network",
		"nodeName", bootstrapName,
		"networkID", networkID,
		"privateIP", privateIP,
	)

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

// calculateBootstrapNodeIP determines the private IP for the bootstrap node.
func calculateBootstrapNodeIP(k8sCluster *k8znerv1alpha1.K8znerCluster) (string, error) {
	networkCIDR := defaultString(k8sCluster.Spec.Network.IPv4CIDR, "10.0.0.0/16")
	cpSubnet, err := config.CIDRSubnet(networkCIDR, 8, 0)
	if err != nil {
		return "", fmt.Errorf("failed to calculate control plane subnet: %w", err)
	}

	privateIP, err := config.CIDRHost(cpSubnet, 2)
	if err != nil {
		return "", fmt.Errorf("failed to calculate bootstrap node IP: %w", err)
	}

	return privateIP, nil
}

// updateNodeStatuses updates the cluster status with node information from provisioning state.
func updateNodeStatuses(k8sCluster *k8znerv1alpha1.K8znerCluster, state *provisioning.State) {
	for name, ip := range state.ControlPlaneIPs {
		serverID := state.ControlPlaneServerIDs[name]
		found := false
		for i := range k8sCluster.Status.ControlPlanes.Nodes {
			if k8sCluster.Status.ControlPlanes.Nodes[i].Name == name {
				k8sCluster.Status.ControlPlanes.Nodes[i].PublicIP = ip
				k8sCluster.Status.ControlPlanes.Nodes[i].ServerID = serverID
				found = true
				break
			}
		}
		if !found {
			k8sCluster.Status.ControlPlanes.Nodes = append(k8sCluster.Status.ControlPlanes.Nodes,
				k8znerv1alpha1.NodeStatus{
					Name:     name,
					ServerID: serverID,
					PublicIP: ip,
				})
		}
	}

	for name, ip := range state.WorkerIPs {
		serverID := state.WorkerServerIDs[name]
		found := false
		for i := range k8sCluster.Status.Workers.Nodes {
			if k8sCluster.Status.Workers.Nodes[i].Name == name {
				k8sCluster.Status.Workers.Nodes[i].PublicIP = ip
				k8sCluster.Status.Workers.Nodes[i].ServerID = serverID
				found = true
				break
			}
		}
		if !found {
			k8sCluster.Status.Workers.Nodes = append(k8sCluster.Status.Workers.Nodes,
				k8znerv1alpha1.NodeStatus{
					Name:     name,
					ServerID: serverID,
					PublicIP: ip,
				})
		}
	}
}

// SetCondition sets a condition on the cluster status.
func SetCondition(k8sCluster *k8znerv1alpha1.K8znerCluster, condType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	condition := metav1.Condition{
		Type:               condType,
		Status:             status,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}

	for i, c := range k8sCluster.Status.Conditions {
		if c.Type == condType {
			if c.Status != status {
				k8sCluster.Status.Conditions[i] = condition
			}
			return
		}
	}
	k8sCluster.Status.Conditions = append(k8sCluster.Status.Conditions, condition)
}
