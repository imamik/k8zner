// Package v1alpha1 contains API Schema definitions for the k8zner.io v1alpha1 API group
// +kubebuilder:object:generate=true
// +groupName=k8zner.io
package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// K8znerClusterSpec defines the desired state of a K8zner-managed cluster.
type K8znerClusterSpec struct {
	// Region is the Hetzner Cloud region (e.g., fsn1, nbg1, hel1)
	// +kubebuilder:validation:Enum=fsn1;nbg1;hel1;ash;hil
	Region string `json:"region"`

	// ControlPlanes defines the control plane configuration
	ControlPlanes ControlPlaneSpec `json:"controlPlanes"`

	// Workers defines the worker node configuration
	Workers WorkerSpec `json:"workers"`

	// Backup configures automated etcd backups
	// +optional
	Backup *BackupSpec `json:"backup,omitempty"`

	// HealthCheck configures health monitoring thresholds
	// +optional
	HealthCheck *HealthCheckSpec `json:"healthCheck,omitempty"`

	// Addons specifies which addons should be installed
	// +optional
	Addons *AddonSpec `json:"addons,omitempty"`

	// Paused stops the operator from reconciling this cluster
	// +optional
	Paused bool `json:"paused,omitempty"`

	// Network configures the cluster networking
	// +optional
	Network NetworkSpec `json:"network,omitempty"`

	// Firewall configures the cluster firewall rules
	// +optional
	Firewall FirewallSpec `json:"firewall,omitempty"`

	// PlacementGroup configures server placement strategy
	// +optional
	PlacementGroup *PlacementGroupSpec `json:"placementGroup,omitempty"`

	// Kubernetes specifies the Kubernetes version
	Kubernetes KubernetesSpec `json:"kubernetes"`

	// Talos specifies the Talos configuration
	Talos TalosSpec `json:"talos"`

	// CredentialsRef references the Secret containing HCloud token and Talos secrets
	CredentialsRef corev1.LocalObjectReference `json:"credentialsRef"`

	// Bootstrap contains the state from CLI bootstrap (if applicable)
	// +optional
	Bootstrap *BootstrapState `json:"bootstrap,omitempty"`
}

// PlacementGroupSpec configures server placement strategy.
type PlacementGroupSpec struct {
	// Enabled determines if a placement group should be created
	// +kubebuilder:default=true
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Type is the placement group type (spread)
	// +kubebuilder:validation:Enum=spread
	// +kubebuilder:default="spread"
	// +optional
	Type string `json:"type,omitempty"`
}

// NetworkSpec configures the cluster networking.
type NetworkSpec struct {
	// IPv4CIDR is the network CIDR for the Hetzner private network
	// +kubebuilder:default="10.0.0.0/16"
	// +optional
	IPv4CIDR string `json:"ipv4CIDR,omitempty"`

	// PodCIDR is the CIDR range for pod IPs
	// +kubebuilder:default="10.244.0.0/16"
	// +optional
	PodCIDR string `json:"podCIDR,omitempty"`

	// ServiceCIDR is the CIDR range for service IPs
	// +kubebuilder:default="10.96.0.0/16"
	// +optional
	ServiceCIDR string `json:"serviceCIDR,omitempty"`
}

// FirewallSpec configures firewall rules for the cluster.
type FirewallSpec struct {
	// Enabled determines if a firewall should be created
	// +kubebuilder:default=true
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// AllowedSSHIPs is a list of CIDR ranges allowed to SSH to nodes
	// +optional
	AllowedSSHIPs []string `json:"allowedSSHIPs,omitempty"`

	// AllowedAPIIPs is a list of CIDR ranges allowed to access the Kubernetes API
	// +optional
	AllowedAPIIPs []string `json:"allowedAPIIPs,omitempty"`
}

// KubernetesSpec specifies the Kubernetes version.
type KubernetesSpec struct {
	// Version is the Kubernetes version (e.g., "1.32.2")
	// +kubebuilder:validation:Pattern=`^\d+\.\d+\.\d+$`
	Version string `json:"version"`
}

// TalosSpec specifies the Talos configuration.
type TalosSpec struct {
	// Version is the Talos version (e.g., "v1.10.2")
	// +kubebuilder:validation:Pattern=`^v\d+\.\d+\.\d+$`
	Version string `json:"version"`

	// SchematicID is the Talos Factory schematic ID for custom images
	// +optional
	SchematicID string `json:"schematicID,omitempty"`

	// Extensions is a list of Talos system extensions to include
	// +optional
	Extensions []string `json:"extensions,omitempty"`
}

// BootstrapState contains the state from CLI bootstrap.
type BootstrapState struct {
	// Completed indicates the CLI bootstrap has finished
	Completed bool `json:"completed"`

	// CompletedAt is when bootstrap completed
	// +optional
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`

	// BootstrapNode is the name of the first control plane server
	// +optional
	BootstrapNode string `json:"bootstrapNode,omitempty"`

	// BootstrapNodeID is the Hetzner server ID of the first control plane
	// +optional
	BootstrapNodeID int64 `json:"bootstrapNodeID,omitempty"`

	// PublicIP is the public IP of the bootstrap node (before LB)
	// +optional
	PublicIP string `json:"publicIP,omitempty"`
}

// ControlPlaneSpec defines the control plane configuration.
type ControlPlaneSpec struct {
	// Count is the number of control plane nodes (1 for dev, 3 for HA)
	// +kubebuilder:validation:Enum=1;3;5
	// +kubebuilder:default=1
	Count int `json:"count"`

	// Size is the Hetzner server type (e.g., cx23, cx33, cpx21, cpx31)
	// CX types (dedicated vCPU) have consistent performance, CPX types (shared vCPU) have better availability
	// +kubebuilder:default="cx23"
	Size string `json:"size"`
}

// WorkerSpec defines the worker node configuration.
type WorkerSpec struct {
	// Count is the desired number of worker nodes
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	Count int `json:"count"`

	// Size is the Hetzner server type (e.g., cx23, cx33, cpx21, cpx31)
	// CX types (dedicated vCPU) have consistent performance, CPX types (shared vCPU) have better availability
	// +kubebuilder:default="cx23"
	Size string `json:"size"`

	// MinCount is the minimum number of workers (for safety)
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	// +optional
	MinCount int `json:"minCount,omitempty"`

	// MaxCount is the maximum number of workers (for safety)
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=10
	// +optional
	MaxCount int `json:"maxCount,omitempty"`
}

// BackupSpec configures automated etcd backups.
type BackupSpec struct {
	// Enabled turns on automated backups
	Enabled bool `json:"enabled"`

	// Schedule is the cron schedule for backups (default: hourly)
	// +kubebuilder:default="0 * * * *"
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// Retention is how long to keep backups (default: 168h = 7 days)
	// +kubebuilder:default="168h"
	// +optional
	Retention string `json:"retention,omitempty"`
}

// HealthCheckSpec configures health monitoring thresholds.
type HealthCheckSpec struct {
	// NodeNotReadyThreshold is how long a node can be NotReady before replacement
	// +kubebuilder:default="3m"
	// +optional
	NodeNotReadyThreshold string `json:"nodeNotReadyThreshold,omitempty"`

	// EtcdUnhealthyThreshold is how long etcd can be unhealthy before CP replacement
	// +kubebuilder:default="2m"
	// +optional
	EtcdUnhealthyThreshold string `json:"etcdUnhealthyThreshold,omitempty"`
}

// AddonSpec specifies which addons should be installed.
type AddonSpec struct {
	// Traefik ingress controller
	// +kubebuilder:default=true
	// +optional
	Traefik bool `json:"traefik,omitempty"`

	// CertManager for TLS certificates
	// +kubebuilder:default=true
	// +optional
	CertManager bool `json:"certManager,omitempty"`

	// ExternalDNS for automatic DNS management
	// +optional
	ExternalDNS bool `json:"externalDns,omitempty"`

	// ArgoCD for GitOps
	// +optional
	ArgoCD bool `json:"argocd,omitempty"`

	// MetricsServer for resource metrics
	// +kubebuilder:default=true
	// +optional
	MetricsServer bool `json:"metricsServer,omitempty"`
}

// K8znerClusterStatus defines the observed state of K8znerCluster.
type K8znerClusterStatus struct {
	// Phase is the overall cluster phase
	// +kubebuilder:validation:Enum=Provisioning;Running;Degraded;Healing;Failed
	Phase ClusterPhase `json:"phase,omitempty"`

	// ControlPlanes shows the status of control plane nodes
	ControlPlanes NodeGroupStatus `json:"controlPlanes,omitempty"`

	// Workers shows the status of worker nodes
	Workers NodeGroupStatus `json:"workers,omitempty"`

	// Addons shows the status of installed addons
	// +optional
	Addons map[string]AddonStatus `json:"addons,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastReconcileTime is when the operator last reconciled this cluster
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// ObservedGeneration is the last observed generation
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ProvisioningPhase tracks the current provisioning stage
	// +optional
	ProvisioningPhase ProvisioningPhase `json:"provisioningPhase,omitempty"`

	// Infrastructure tracks the provisioned infrastructure resources
	// +optional
	Infrastructure InfrastructureStatus `json:"infrastructure,omitempty"`

	// ControlPlaneEndpoint is the endpoint for the Kubernetes API
	// +optional
	ControlPlaneEndpoint string `json:"controlPlaneEndpoint,omitempty"`

	// ImageSnapshot tracks the Talos image snapshot
	// +optional
	ImageSnapshot *ImageStatus `json:"imageSnapshot,omitempty"`
}

// ProvisioningPhase represents the current stage of cluster provisioning.
type ProvisioningPhase string

const (
	// PhaseInfrastructure means the operator is creating infrastructure (network, firewall, LB)
	PhaseInfrastructure ProvisioningPhase = "Infrastructure"
	// PhaseImage means the operator is ensuring the Talos image snapshot exists
	PhaseImage ProvisioningPhase = "Image"
	// PhaseCompute means the operator is provisioning compute resources (servers)
	PhaseCompute ProvisioningPhase = "Compute"
	// PhaseBootstrap means the operator is bootstrapping the cluster
	PhaseBootstrap ProvisioningPhase = "Bootstrap"
	// PhaseCNI means the operator is installing Cilium CNI
	PhaseCNI ProvisioningPhase = "CNI"
	// PhaseAddons means the operator is installing other addons (after CNI is ready)
	PhaseAddons ProvisioningPhase = "Addons"
	// PhaseConfiguring means the operator is configuring the cluster (legacy, use PhaseCNI/PhaseAddons)
	PhaseConfiguring ProvisioningPhase = "Configuring"
	// PhaseComplete means provisioning is complete and the cluster is running
	PhaseComplete ProvisioningPhase = "Complete"
)

// InfrastructureStatus tracks the provisioned infrastructure resources.
type InfrastructureStatus struct {
	// NetworkID is the Hetzner network ID
	// +optional
	NetworkID int64 `json:"networkID,omitempty"`

	// SubnetID is the Hetzner subnet ID
	// +optional
	SubnetID string `json:"subnetID,omitempty"`

	// FirewallID is the Hetzner firewall ID
	// +optional
	FirewallID int64 `json:"firewallID,omitempty"`

	// LoadBalancerID is the Hetzner load balancer ID
	// +optional
	LoadBalancerID int64 `json:"loadBalancerID,omitempty"`

	// LoadBalancerIP is the public IP of the load balancer
	// +optional
	LoadBalancerIP string `json:"loadBalancerIP,omitempty"`

	// SSHKeyID is the Hetzner SSH key ID used for nodes
	// +optional
	SSHKeyID int64 `json:"sshKeyID,omitempty"`

	// PlacementGroupID is the Hetzner placement group ID
	// +optional
	PlacementGroupID int64 `json:"placementGroupID,omitempty"`

	// SnapshotID is the Hetzner Talos image snapshot ID
	// +optional
	SnapshotID int64 `json:"snapshotID,omitempty"`
}

// ImageStatus tracks the Talos image snapshot.
type ImageStatus struct {
	// SnapshotID is the Hetzner snapshot ID
	SnapshotID int64 `json:"snapshotID,omitempty"`

	// Version is the Talos version of the snapshot
	// +optional
	Version string `json:"version,omitempty"`

	// SchematicID is the Talos Factory schematic ID
	// +optional
	SchematicID string `json:"schematicID,omitempty"`

	// CreatedAt is when the snapshot was created
	// +optional
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`
}

// ClusterPhase represents the overall cluster state.
type ClusterPhase string

const (
	// ClusterPhaseProvisioning means the cluster is being created
	ClusterPhaseProvisioning ClusterPhase = "Provisioning"
	// ClusterPhaseRunning means the cluster is healthy
	ClusterPhaseRunning ClusterPhase = "Running"
	// ClusterPhaseDegraded means some components are unhealthy
	ClusterPhaseDegraded ClusterPhase = "Degraded"
	// ClusterPhaseHealing means the operator is replacing unhealthy nodes
	ClusterPhaseHealing ClusterPhase = "Healing"
	// ClusterPhaseFailed means the cluster cannot self-heal
	ClusterPhaseFailed ClusterPhase = "Failed"
)

// NodeGroupStatus represents the status of a group of nodes.
type NodeGroupStatus struct {
	// Desired is the desired number of nodes
	Desired int `json:"desired"`

	// Ready is the number of healthy nodes
	Ready int `json:"ready"`

	// Unhealthy is the number of unhealthy nodes
	// +optional
	Unhealthy int `json:"unhealthy,omitempty"`

	// Nodes is the detailed status of each node
	// +optional
	Nodes []NodeStatus `json:"nodes,omitempty"`
}

// NodePhase represents the lifecycle phase of a node.
type NodePhase string

const (
	// Server provisioning lifecycle phases
	// NodePhaseCreatingServer means HCloud CreateServer has been called
	NodePhaseCreatingServer NodePhase = "CreatingServer"
	// NodePhaseWaitingForIP means server is created, waiting for IP assignment
	NodePhaseWaitingForIP NodePhase = "WaitingForIP"
	// NodePhaseWaitingForTalosAPI means IP is assigned, waiting for Talos API to respond
	NodePhaseWaitingForTalosAPI NodePhase = "WaitingForTalosAPI"
	// NodePhaseApplyingTalosConfig means Talos API is ready, applying machine configuration
	NodePhaseApplyingTalosConfig NodePhase = "ApplyingTalosConfig"
	// NodePhaseRebootingWithConfig means config is applied, node is rebooting with new configuration
	NodePhaseRebootingWithConfig NodePhase = "RebootingWithConfig"
	// NodePhaseWaitingForK8s means waiting for kubelet to register the node with Kubernetes
	NodePhaseWaitingForK8s NodePhase = "WaitingForK8s"
	// NodePhaseReady means the node is healthy and serving workloads
	NodePhaseReady NodePhase = "Ready"

	// Unhealthy and removal phases
	// NodePhaseUnhealthy means health checks are failing
	NodePhaseUnhealthy NodePhase = "Unhealthy"
	// NodePhaseDraining means the node is being drained before replacement
	NodePhaseDraining NodePhase = "Draining"
	// NodePhaseRemovingFromEtcd means the etcd member is being removed (control plane only)
	NodePhaseRemovingFromEtcd NodePhase = "RemovingFromEtcd"
	// NodePhaseDeletingServer means the HCloud server is being deleted
	NodePhaseDeletingServer NodePhase = "DeletingServer"

	// Error phase
	// NodePhaseFailed means provisioning failed and the node cannot be recovered automatically
	NodePhaseFailed NodePhase = "Failed"
)

// NodeStatus represents the status of a single node.
type NodeStatus struct {
	// Name is the Kubernetes node name
	Name string `json:"name"`

	// ServerID is the Hetzner server ID
	ServerID int64 `json:"serverID"`

	// PrivateIP is the private network IP
	PrivateIP string `json:"privateIP,omitempty"`

	// PublicIP is the public IP (if any)
	// +optional
	PublicIP string `json:"publicIP,omitempty"`

	// Phase is the lifecycle phase of this node
	// +kubebuilder:validation:Enum=CreatingServer;WaitingForIP;WaitingForTalosAPI;ApplyingTalosConfig;RebootingWithConfig;WaitingForK8s;Ready;Unhealthy;Draining;RemovingFromEtcd;DeletingServer;Failed
	// +optional
	Phase NodePhase `json:"phase,omitempty"`

	// PhaseReason provides additional context for the current phase
	// +optional
	PhaseReason string `json:"phaseReason,omitempty"`

	// PhaseTransitionTime is when the node transitioned to the current phase
	// +optional
	PhaseTransitionTime *metav1.Time `json:"phaseTransitionTime,omitempty"`

	// Healthy indicates if the node is healthy (deprecated, use Phase)
	Healthy bool `json:"healthy"`

	// UnhealthyReason explains why the node is unhealthy
	// +optional
	UnhealthyReason string `json:"unhealthyReason,omitempty"`

	// UnhealthySince is when the node became unhealthy
	// +optional
	UnhealthySince *metav1.Time `json:"unhealthySince,omitempty"`

	// EtcdMemberID is the etcd member ID (for control planes)
	// +optional
	EtcdMemberID string `json:"etcdMemberID,omitempty"`

	// LastHealthCheck is when health was last checked
	// +optional
	LastHealthCheck *metav1.Time `json:"lastHealthCheck,omitempty"`
}

// AddonPhase represents the installation phase of an addon.
type AddonPhase string

const (
	// AddonPhasePending means the addon is waiting to be installed
	AddonPhasePending AddonPhase = "Pending"
	// AddonPhaseInstalling means the addon is being installed
	AddonPhaseInstalling AddonPhase = "Installing"
	// AddonPhaseInstalled means the addon is installed and healthy
	AddonPhaseInstalled AddonPhase = "Installed"
	// AddonPhaseFailed means the addon installation failed
	AddonPhaseFailed AddonPhase = "Failed"
	// AddonPhaseUpgrading means the addon is being upgraded
	AddonPhaseUpgrading AddonPhase = "Upgrading"
)

// AddonStatus represents the status of an installed addon.
type AddonStatus struct {
	// Installed indicates if the addon is installed
	Installed bool `json:"installed"`

	// Version is the installed version
	// +optional
	Version string `json:"version,omitempty"`

	// Healthy indicates if the addon is healthy
	Healthy bool `json:"healthy"`

	// Message provides additional status information
	// +optional
	Message string `json:"message,omitempty"`

	// Phase is the current installation phase of the addon
	// +kubebuilder:validation:Enum=Pending;Installing;Installed;Failed;Upgrading
	// +optional
	Phase AddonPhase `json:"phase,omitempty"`

	// LastTransitionTime is when the addon phase last changed
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`

	// InstallOrder is the order in which this addon should be installed
	// +optional
	InstallOrder int `json:"installOrder,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=k8z
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="CPs",type=string,JSONPath=`.status.controlPlanes.ready`
// +kubebuilder:printcolumn:name="Workers",type=string,JSONPath=`.status.workers.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// K8znerCluster is the Schema for the k8znerclusters API.
type K8znerCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   K8znerClusterSpec   `json:"spec,omitempty"`
	Status K8znerClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// K8znerClusterList contains a list of K8znerCluster.
type K8znerClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []K8znerCluster `json:"items"`
}

// Condition types for K8znerCluster
const (
	// ConditionReady indicates the cluster is fully healthy
	ConditionReady = "Ready"
	// ConditionControlPlaneReady indicates all control planes are healthy
	ConditionControlPlaneReady = "ControlPlaneReady"
	// ConditionWorkersReady indicates all workers are healthy
	ConditionWorkersReady = "WorkersReady"
	// ConditionEtcdHealthy indicates etcd cluster is healthy
	ConditionEtcdHealthy = "EtcdHealthy"
	// ConditionAddonsHealthy indicates all addons are healthy
	ConditionAddonsHealthy = "AddonsHealthy"
	// ConditionInfrastructureReady indicates infrastructure is provisioned
	ConditionInfrastructureReady = "InfrastructureReady"
	// ConditionImageReady indicates Talos image is available
	ConditionImageReady = "ImageReady"
	// ConditionBootstrapped indicates the cluster has been bootstrapped
	ConditionBootstrapped = "Bootstrapped"
)

// Credentials Secret keys
const (
	// CredentialsKeyHCloudToken is the key for the HCloud API token in the credentials Secret
	CredentialsKeyHCloudToken = "hcloud-token"
	// CredentialsKeyTalosSecrets is the key for the Talos secrets.yaml in the credentials Secret
	CredentialsKeyTalosSecrets = "talos-secrets"
	// CredentialsKeyTalosConfig is the key for the talosconfig in the credentials Secret
	CredentialsKeyTalosConfig = "talosconfig"
)

// Addon names used for status tracking
const (
	AddonNameCilium        = "cilium"
	AddonNameCCM           = "hcloud-ccm"
	AddonNameCSI           = "hcloud-csi"
	AddonNameMetricsServer = "metrics-server"
	AddonNameCertManager   = "cert-manager"
	AddonNameTraefik       = "traefik"
	AddonNameExternalDNS   = "external-dns"
	AddonNameArgoCD        = "argocd"
	AddonNameMonitoring    = "monitoring"
	AddonNameTalosBackup   = "talos-backup"
)

// Addon install order (lower = earlier)
const (
	AddonOrderCilium        = 1  // CNI foundation - REQUIRED FIRST
	AddonOrderCCM           = 2  // Hetzner cloud controller
	AddonOrderCSI           = 3  // Hetzner storage
	AddonOrderMetricsServer = 4  // Resource metrics
	AddonOrderCertManager   = 5  // TLS certificates
	AddonOrderTraefik       = 6  // Ingress controller
	AddonOrderExternalDNS   = 7  // DNS management
	AddonOrderArgoCD        = 8  // GitOps
	AddonOrderMonitoring    = 9  // Prometheus/Grafana
	AddonOrderTalosBackup   = 10 // Backup controller
)
