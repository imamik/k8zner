// Package v1alpha1 contains API Schema definitions for the k8zner.io v1alpha1 API group
// +kubebuilder:object:generate=true
// +groupName=k8zner.io
package v1alpha1

import (
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
}

// ControlPlaneSpec defines the control plane configuration.
type ControlPlaneSpec struct {
	// Count is the number of control plane nodes (1 for dev, 3 for HA)
	// +kubebuilder:validation:Enum=1;3;5
	// +kubebuilder:default=1
	Count int `json:"count"`

	// Size is the Hetzner server type (e.g., cx22, cx32)
	// +kubebuilder:default="cx22"
	Size string `json:"size"`
}

// WorkerSpec defines the worker node configuration.
type WorkerSpec struct {
	// Count is the desired number of worker nodes
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	Count int `json:"count"`

	// Size is the Hetzner server type (e.g., cx22, cx32, cx42)
	// +kubebuilder:default="cx22"
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
	// +kubebuilder:default="5m"
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

	// Healthy indicates if the node is healthy
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
)
