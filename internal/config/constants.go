package config

// Common port numbers used throughout the application.
const (
	// KubeAPIPort is the standard Kubernetes API server port.
	KubeAPIPort = 6443

	// TalosAPIPort is the standard Talos API port.
	TalosAPIPort = 50000

	// EtcdClientPort is the etcd client port.
	EtcdClientPort = 2379

	// EtcdPeerPort is the etcd peer communication port.
	EtcdPeerPort = 2380

	// EtcdMetricsPort is the etcd metrics port.
	EtcdMetricsPort = 2381
)

// Common timeout and retry defaults.
const (
	// DefaultRetryCount is the default number of retries for operations.
	DefaultRetryCount = 5

	// DefaultTalosctlRetryCount is the default retry count for talosctl operations.
	DefaultTalosctlRetryCount = 5
)
