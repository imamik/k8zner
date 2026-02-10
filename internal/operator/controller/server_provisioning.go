package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"

	hcloudgo "github.com/hetznercloud/hcloud-go/v2/hcloud"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/util/keygen"
	"github.com/imamik/k8zner/internal/util/labels"
	"github.com/imamik/k8zner/internal/util/naming"
)

// talosClients holds the Talos API clients resolved from injected mocks or credentials.
type talosClients struct {
	configGen talosConfigGenerator
	client    talosClient
}

// loadTalosClients resolves Talos clients from injected mocks or cluster credentials.
// Returns nil clients (not error) if credentials are unavailable — callers skip Talos steps.
func (r *ClusterReconciler) loadTalosClients(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) talosClients {
	logger := log.FromContext(ctx)

	tc := talosClients{
		configGen: r.talosConfigGen,
		client:    r.talosClient,
	}

	if tc.client != nil {
		// Already injected (e.g. tests) — nothing to load.
		return tc
	}

	if cluster.Spec.CredentialsRef.Name == "" {
		return tc
	}

	creds, err := r.phaseAdapter.LoadCredentials(ctx, cluster)
	if err != nil {
		logger.Error(err, "failed to load credentials for Talos operations")
		return tc
	}

	if tc.configGen == nil {
		generator, err := r.phaseAdapter.CreateTalosGenerator(cluster, creds)
		if err != nil {
			logger.Error(err, "failed to create Talos config generator")
		} else {
			tc.configGen = generator
		}
	}

	if len(creds.TalosConfig) > 0 {
		talosClientInstance, err := newRealTalosClient(creds.TalosConfig)
		if err != nil {
			logger.Error(err, "failed to create Talos client")
		} else {
			tc.client = talosClientInstance
		}
	}

	return tc
}

// discoverLoadBalancerInfo discovers LB IP/ID from HCloud if not already in cluster status.
// This is needed before creating Talos config generator (which needs the CP endpoint).
func (r *ClusterReconciler) discoverLoadBalancerInfo(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, hcloudToken string) {
	logger := log.FromContext(ctx)

	if cluster.Status.Infrastructure.LoadBalancerID != 0 && cluster.Status.Infrastructure.LoadBalancerIP != "" {
		return
	}

	infraManager := hcloud.NewRealClient(hcloudToken)
	lbName := naming.KubeAPILoadBalancer(cluster.Name)
	lb, lbErr := infraManager.GetLoadBalancer(ctx, lbName)
	if lbErr != nil || lb == nil {
		return
	}

	cluster.Status.Infrastructure.LoadBalancerID = lb.ID
	if lb.PublicNet.Enabled && lb.PublicNet.IPv4.IP.String() != "<nil>" {
		cluster.Status.Infrastructure.LoadBalancerIP = lb.PublicNet.IPv4.IP.String()
	}
	if len(lb.PrivateNet) > 0 && lb.PrivateNet[0].IP != nil {
		cluster.Status.Infrastructure.LoadBalancerPrivateIP = lb.PrivateNet[0].IP.String()
	}
	logger.Info("discovered LB info",
		"lbID", lb.ID,
		"lbIP", cluster.Status.Infrastructure.LoadBalancerIP,
		"lbPrivateIP", cluster.Status.Infrastructure.LoadBalancerPrivateIP,
	)
}

// provisioningPrereqs holds the common prerequisites for provisioning new nodes.
type provisioningPrereqs struct {
	ClusterState *clusterState
	TC           talosClients
	SnapshotID   int64
	SSHKeyName   string
}

// prepareForProvisioning gathers all prerequisites for provisioning a new node:
// cluster state, Talos clients, snapshot, and an ephemeral SSH key.
// Returns a cleanup function that must be deferred by the caller.
func (r *ClusterReconciler) prepareForProvisioning(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, role string) (*provisioningPrereqs, func(), error) {
	clusterState, err := r.buildClusterState(ctx, cluster)
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to build cluster state: %v", err)
		return nil, nil, fmt.Errorf("failed to build cluster state: %w", err)
	}

	// Discover LB info if needed, then load Talos clients.
	// LB discovery must happen BEFORE loadTalosClients because the Talos generator
	// needs the control plane endpoint (LB IP) to generate valid configs.
	if r.talosClient == nil && cluster.Spec.CredentialsRef.Name != "" {
		creds, err := r.phaseAdapter.LoadCredentials(ctx, cluster)
		if err == nil {
			r.discoverLoadBalancerInfo(ctx, cluster, creds.HCloudToken)
		}
	}
	tc := r.loadTalosClients(ctx, cluster)

	snapshot, err := r.getSnapshot(ctx)
	if err != nil {
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Talos snapshot not found: %v", err)
		return nil, nil, err
	}

	sshKeyName, cleanup, err := r.createEphemeralSSHKey(ctx, cluster, role)
	if err != nil {
		return nil, nil, err
	}

	return &provisioningPrereqs{
		ClusterState: clusterState,
		TC:           tc,
		SnapshotID:   snapshot.ID,
		SSHKeyName:   sshKeyName,
	}, cleanup, nil
}

// getSnapshot retrieves the Talos OS snapshot for server creation.
func (r *ClusterReconciler) getSnapshot(ctx context.Context) (*hcloudgo.Image, error) {
	snapshotLabels := map[string]string{"os": "talos"}
	snapshot, err := r.hcloudClient.GetSnapshotByLabels(ctx, snapshotLabels)
	if err != nil {
		return nil, fmt.Errorf("failed to get Talos snapshot: %w", err)
	}
	if snapshot == nil {
		return nil, fmt.Errorf("no Talos snapshot found with labels %v", snapshotLabels)
	}
	return snapshot, nil
}

// createEphemeralSSHKey creates a temporary SSH key to avoid Hetzner password emails.
// Returns the key name and a cleanup function that deletes the key.
func (r *ClusterReconciler) createEphemeralSSHKey(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, role string) (keyName string, cleanup func(), err error) {
	logger := log.FromContext(ctx)

	keyName = fmt.Sprintf("ephemeral-%s-%s-%d", cluster.Name, role, time.Now().Unix())
	logger.Info("creating ephemeral SSH key", "keyName", keyName)

	keyPair, err := keygen.GenerateRSAKeyPair(2048)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate ephemeral SSH key: %w", err)
	}

	sshKeyLabels := map[string]string{
		"cluster": cluster.Name,
		"type":    fmt.Sprintf("ephemeral-%s", role),
	}

	_, err = r.hcloudClient.CreateSSHKey(ctx, keyName, string(keyPair.PublicKey), sshKeyLabels)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create ephemeral SSH key: %w", err)
	}

	cleanup = func() {
		logger.Info("cleaning up ephemeral SSH key", "keyName", keyName)
		if err := r.hcloudClient.DeleteSSHKey(ctx, keyName); err != nil {
			logger.Error(err, "failed to delete ephemeral SSH key", "keyName", keyName)
		}
	}

	return keyName, cleanup, nil
}

// serverCreateOpts holds the options for creating a server.
type serverCreateOpts struct {
	Name       string
	SnapshotID int64
	ServerType string
	Region     string
	SSHKeyName string
	Labels     map[string]string
	NetworkID  int64
	Role       string // "control-plane" or "worker" - for phase tracking
}

// serverProvisionResult holds the results of server creation.
type serverProvisionResult struct {
	Name      string
	ServerID  int64
	PublicIP  string
	PrivateIP string
	TalosIP   string // Private IP if available, else Public IP
}

// provisionServer creates a server and waits for IP assignment and server ID.
// On failure after server creation, it cleans up the orphaned server.
func (r *ClusterReconciler) provisionServer(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, opts serverCreateOpts) (*serverProvisionResult, error) {
	logger := log.FromContext(ctx)

	// Create the server
	r.updateNodePhase(ctx, cluster, opts.Role, nodeStatusUpdate{
		Name:   opts.Name,
		Phase:  k8znerv1alpha1.NodePhaseCreatingServer,
		Reason: fmt.Sprintf("Creating HCloud server with snapshot %d", opts.SnapshotID),
	})

	startTime := time.Now()
	_, err := r.hcloudClient.CreateServer(ctx, hcloud.ServerCreateOpts{
		Name:             opts.Name,
		ImageType:        fmt.Sprintf("%d", opts.SnapshotID),
		ServerType:       opts.ServerType,
		Location:         opts.Region,
		SSHKeys:          []string{opts.SSHKeyName},
		Labels:           opts.Labels,
		NetworkID:        opts.NetworkID,
		EnablePublicIPv4: true,
		EnablePublicIPv6: true,
	})
	if err != nil {
		r.recordHCloudAPICall("create_server", "error", time.Since(startTime).Seconds())
		r.updateNodePhase(ctx, cluster, opts.Role, nodeStatusUpdate{
			Name:   opts.Name,
			Phase:  k8znerv1alpha1.NodePhaseFailed,
			Reason: fmt.Sprintf("Failed to create server: %v", err),
		})
		return nil, fmt.Errorf("failed to create server: %w", err)
	}
	r.recordHCloudAPICall("create_server", "success", time.Since(startTime).Seconds())
	logger.Info("created server", "name", opts.Name)

	// Wait for server IP assignment
	r.updateNodePhase(ctx, cluster, opts.Role, nodeStatusUpdate{
		Name:   opts.Name,
		Phase:  k8znerv1alpha1.NodePhaseWaitingForIP,
		Reason: "Waiting for HCloud to assign IP address",
	})

	serverIP, err := r.waitForServerIP(ctx, opts.Name, serverIPTimeout)
	if err != nil {
		r.handleProvisioningFailure(ctx, cluster, opts.Role, opts.Name,
			fmt.Sprintf("Failed to get IP: %v", err))
		return nil, fmt.Errorf("failed to get server IP: %w", err)
	}
	logger.Info("server IP assigned", "name", opts.Name, "ip", serverIP)

	// Get server ID
	serverIDStr, err := r.hcloudClient.GetServerID(ctx, opts.Name)
	if err != nil {
		r.handleProvisioningFailure(ctx, cluster, opts.Role, opts.Name,
			fmt.Sprintf("Failed to get server ID: %v", err))
		return nil, fmt.Errorf("failed to get server ID: %w", err)
	}
	var serverID int64
	if _, err := fmt.Sscanf(serverIDStr, "%d", &serverID); err != nil {
		r.handleProvisioningFailure(ctx, cluster, opts.Role, opts.Name,
			fmt.Sprintf("Failed to parse server ID: %v", err))
		return nil, fmt.Errorf("failed to parse server ID: %w", err)
	}

	// Get private IP from server
	privateIP, _ := r.getPrivateIPFromServer(ctx, opts.Name)

	// Use private IP for Talos communication if available (bypasses firewall restrictions)
	talosIP := serverIP
	if privateIP != "" {
		talosIP = privateIP
		logger.Info("using private IP for Talos communication", "name", opts.Name, "privateIP", privateIP)
	}

	return &serverProvisionResult{
		Name:      opts.Name,
		ServerID:  serverID,
		PublicIP:  serverIP,
		PrivateIP: privateIP,
		TalosIP:   talosIP,
	}, nil
}

// configureNodeFunc is called after server provisioning to apply Talos config and wait for readiness.
type configureNodeFunc func(serverName string, result *serverProvisionResult) error

// nodeProvisionParams describes the parameters for provisioning a new node.
type nodeProvisionParams struct {
	Name          string
	Role          string // "control-plane" or "worker"
	Pool          string // "control-plane" or "workers"
	ServerType    string
	SnapshotID    int64
	SSHKeyName    string
	NetworkID     int64
	Configure     configureNodeFunc
	MetricsReason string // e.g. "scale-up"; empty to skip metrics (caller records them)
}

// provisionAndConfigureNode provisions a server, applies Talos config via the Configure callback,
// persists status, and records metrics. This is the unified implementation for both CP and worker provisioning.
func (r *ClusterReconciler) provisionAndConfigureNode(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, params nodeProvisionParams) error {
	logger := log.FromContext(ctx)

	serverLabels := labels.NewLabelBuilder(cluster.Name).
		WithRole(params.Role).
		WithPool(params.Pool).
		WithManagedBy(labels.ManagedByOperator).
		Build()

	logger.Info("creating new server",
		"name", params.Name, "role", params.Role, "snapshot", params.SnapshotID, "serverType", params.ServerType)

	startTime := time.Now()

	result, err := r.provisionServer(ctx, cluster, serverCreateOpts{
		Name:       params.Name,
		SnapshotID: params.SnapshotID,
		ServerType: params.ServerType,
		Region:     cluster.Spec.Region,
		SSHKeyName: params.SSHKeyName,
		Labels:     serverLabels,
		NetworkID:  params.NetworkID,
		Role:       params.Role,
	})
	if err != nil {
		logger.Error(err, "failed to provision server", "name", params.Name, "role", params.Role)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonServerCreationError,
			"Failed to create %s server %s: %v", params.Role, params.Name, err)
		return err
	}

	// Persist status to prevent duplicate server creation
	if err := r.persistClusterStatus(ctx, cluster); err != nil {
		logger.Error(err, "failed to persist status after server creation", "name", params.Name)
	}

	if err := r.updateNodePhaseAndPersist(ctx, cluster, params.Role, nodeStatusUpdate{
		Name:      params.Name,
		ServerID:  result.ServerID,
		PublicIP:  result.PublicIP,
		PrivateIP: result.PrivateIP,
		Phase:     k8znerv1alpha1.NodePhaseWaitingForTalosAPI,
		Reason:    fmt.Sprintf("Waiting for Talos API on %s:50000", result.TalosIP),
	}); err != nil {
		logger.Error(err, "failed to persist node status", "name", params.Name)
	}

	if err := params.Configure(params.Name, result); err != nil {
		return err
	}

	if err := r.persistClusterStatus(ctx, cluster); err != nil {
		logger.Error(err, "failed to persist cluster status", "name", params.Name)
	}

	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReasonScalingUp,
		"Successfully created %s %s", params.Role, params.Name)

	if params.MetricsReason != "" {
		r.recordNodeReplacement(cluster.Name, params.Role, params.MetricsReason)
		r.recordNodeReplacementDuration(cluster.Name, params.Role, time.Since(startTime).Seconds())
	}

	return nil
}

// handleProvisioningFailure marks a node as failed, deletes its orphaned server, and removes it from status.
func (r *ClusterReconciler) handleProvisioningFailure(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, role, serverName, reason string) {
	logger := log.FromContext(ctx)

	r.updateNodePhase(ctx, cluster, role, nodeStatusUpdate{
		Name:   serverName,
		Phase:  k8znerv1alpha1.NodePhaseFailed,
		Reason: reason,
	})

	if delErr := r.hcloudClient.DeleteServer(ctx, serverName); delErr != nil {
		logger.Error(delErr, "failed to delete orphaned server", "name", serverName)
	}

	r.removeNodeFromStatus(cluster, role, serverName)
}
