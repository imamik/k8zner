package controller

import (
	"context"
	"fmt"
	"time"

	hcloudgo "github.com/hetznercloud/hcloud-go/v2/hcloud"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/util/keygen"
	"github.com/imamik/k8zner/internal/util/naming"
)

// talosClients holds the Talos API clients resolved from injected mocks or credentials.
type talosClients struct {
	configGen TalosConfigGenerator
	client    TalosClient
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
		talosClientInstance, err := NewRealTalosClient(creds.TalosConfig)
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
	r.updateNodePhase(ctx, cluster, opts.Role, NodeStatusUpdate{
		Name:   opts.Name,
		Phase:  k8znerv1alpha1.NodePhaseCreatingServer,
		Reason: fmt.Sprintf("Creating HCloud server with snapshot %d", opts.SnapshotID),
	})

	startTime := time.Now()
	_, err := r.hcloudClient.CreateServer(
		ctx,
		opts.Name,
		fmt.Sprintf("%d", opts.SnapshotID),
		opts.ServerType,
		opts.Region,
		[]string{opts.SSHKeyName},
		opts.Labels,
		"",  // userData
		nil, // placementGroupID
		opts.NetworkID,
		"",   // privateIP - let HCloud assign
		true, // enablePublicIPv4
		true, // enablePublicIPv6
	)
	if err != nil {
		if r.enableMetrics {
			RecordHCloudAPICall("create_server", "error", time.Since(startTime).Seconds())
		}
		r.updateNodePhase(ctx, cluster, opts.Role, NodeStatusUpdate{
			Name:   opts.Name,
			Phase:  k8znerv1alpha1.NodePhaseFailed,
			Reason: fmt.Sprintf("Failed to create server: %v", err),
		})
		return nil, fmt.Errorf("failed to create server: %w", err)
	}
	if r.enableMetrics {
		RecordHCloudAPICall("create_server", "success", time.Since(startTime).Seconds())
	}
	logger.Info("created server", "name", opts.Name)

	// Wait for server IP assignment
	r.updateNodePhase(ctx, cluster, opts.Role, NodeStatusUpdate{
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

// handleProvisioningFailure marks a node as failed, deletes its orphaned server, and removes it from status.
func (r *ClusterReconciler) handleProvisioningFailure(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster, role, serverName, reason string) {
	logger := log.FromContext(ctx)

	r.updateNodePhase(ctx, cluster, role, NodeStatusUpdate{
		Name:   serverName,
		Phase:  k8znerv1alpha1.NodePhaseFailed,
		Reason: reason,
	})

	if delErr := r.hcloudClient.DeleteServer(ctx, serverName); delErr != nil {
		logger.Error(delErr, "failed to delete orphaned server", "name", serverName)
	}

	r.removeNodeFromStatus(cluster, role, serverName)
}
