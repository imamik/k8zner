// Package controller contains the Kubernetes controllers for the k8zner operator.
package controller

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// Reconciliation metrics
	reconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "k8zner",
			Subsystem: "controller",
			Name:      "reconcile_total",
			Help:      "Total number of reconciliations by result",
		},
		[]string{"cluster", "result"},
	)

	reconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "k8zner",
			Subsystem: "controller",
			Name:      "reconcile_duration_seconds",
			Help:      "Duration of reconciliation in seconds",
			Buckets:   prometheus.ExponentialBuckets(0.01, 2, 10), // 10ms to ~10s
		},
		[]string{"cluster"},
	)

	// Node metrics
	nodesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "k8zner",
			Subsystem: "cluster",
			Name:      "nodes_total",
			Help:      "Total number of nodes by role and status",
		},
		[]string{"cluster", "role", "status"},
	)

	nodesHealthy = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "k8zner",
			Subsystem: "cluster",
			Name:      "nodes_healthy",
			Help:      "Number of healthy nodes by role",
		},
		[]string{"cluster", "role"},
	)

	nodesDesired = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "k8zner",
			Subsystem: "cluster",
			Name:      "nodes_desired",
			Help:      "Desired number of nodes by role",
		},
		[]string{"cluster", "role"},
	)

	// Node replacement metrics
	nodeReplacementsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "k8zner",
			Subsystem: "cluster",
			Name:      "node_replacements_total",
			Help:      "Total number of node replacements by role and reason",
		},
		[]string{"cluster", "role", "reason"},
	)

	nodeReplacementDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "k8zner",
			Subsystem: "cluster",
			Name:      "node_replacement_duration_seconds",
			Help:      "Duration of node replacement in seconds",
			Buckets:   prometheus.ExponentialBuckets(10, 2, 8), // 10s to ~21min
		},
		[]string{"cluster", "role"},
	)

	// Etcd metrics
	etcdMembersTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "k8zner",
			Subsystem: "cluster",
			Name:      "etcd_members_total",
			Help:      "Total number of etcd members",
		},
		[]string{"cluster"},
	)

	etcdHealthy = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "k8zner",
			Subsystem: "cluster",
			Name:      "etcd_healthy",
			Help:      "Whether the etcd cluster is healthy (1) or not (0)",
		},
		[]string{"cluster"},
	)

	// Hetzner Cloud API metrics
	hcloudAPICallsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "k8zner",
			Subsystem: "hcloud",
			Name:      "api_calls_total",
			Help:      "Total number of Hetzner Cloud API calls by operation and result",
		},
		[]string{"operation", "result"},
	)

	hcloudAPILatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "k8zner",
			Subsystem: "hcloud",
			Name:      "api_latency_seconds",
			Help:      "Latency of Hetzner Cloud API calls in seconds",
			Buckets:   prometheus.ExponentialBuckets(0.1, 2, 8), // 100ms to ~25s
		},
		[]string{"operation"},
	)
)

func init() {
	// Register metrics with controller-runtime's registry
	metrics.Registry.MustRegister(
		reconcileTotal,
		reconcileDuration,
		nodesTotal,
		nodesHealthy,
		nodesDesired,
		nodeReplacementsTotal,
		nodeReplacementDuration,
		etcdMembersTotal,
		etcdHealthy,
		hcloudAPICallsTotal,
		hcloudAPILatency,
	)
}

// recordReconcileMetric records a reconciliation result.
func recordReconcileMetric(cluster string, result string, duration float64) {
	reconcileTotal.WithLabelValues(cluster, result).Inc()
	reconcileDuration.WithLabelValues(cluster).Observe(duration)
}

// recordNodeCountsMetric records the node counts for a cluster.
func recordNodeCountsMetric(cluster, role string, total, healthy, desired int) {
	nodesTotal.WithLabelValues(cluster, role, "total").Set(float64(total))
	nodesHealthy.WithLabelValues(cluster, role).Set(float64(healthy))
	nodesDesired.WithLabelValues(cluster, role).Set(float64(desired))
}

// recordNodeReplacementMetric records a node replacement.
func recordNodeReplacementMetric(cluster, role, reason string) {
	nodeReplacementsTotal.WithLabelValues(cluster, role, reason).Inc()
}

// recordNodeReplacementDurationMetric records the duration of a node replacement.
func recordNodeReplacementDurationMetric(cluster, role string, duration float64) {
	nodeReplacementDuration.WithLabelValues(cluster, role).Observe(duration)
}

// recordEtcdStatusMetric records the etcd cluster status.
func recordEtcdStatusMetric(cluster string, members int, healthy bool) {
	etcdMembersTotal.WithLabelValues(cluster).Set(float64(members))
	if healthy {
		etcdHealthy.WithLabelValues(cluster).Set(1)
	} else {
		etcdHealthy.WithLabelValues(cluster).Set(0)
	}
}

// recordHCloudAPICallMetric records a Hetzner Cloud API call.
func recordHCloudAPICallMetric(operation, result string, latency float64) {
	hcloudAPICallsTotal.WithLabelValues(operation, result).Inc()
	hcloudAPILatency.WithLabelValues(operation).Observe(latency)
}

// Metrics helper methods that check enableMetrics before recording.
// These eliminate the repeated `if r.enableMetrics` pattern at call sites.

func (r *ClusterReconciler) recordReconcile(cluster, result string, duration float64) {
	if r.enableMetrics {
		recordReconcileMetric(cluster, result, duration)
	}
}

func (r *ClusterReconciler) recordNodeCounts(cluster, role string, total, healthy, desired int) {
	if r.enableMetrics {
		recordNodeCountsMetric(cluster, role, total, healthy, desired)
	}
}

func (r *ClusterReconciler) recordNodeReplacement(cluster, role, reason string) {
	if r.enableMetrics {
		recordNodeReplacementMetric(cluster, role, reason)
	}
}

func (r *ClusterReconciler) recordNodeReplacementDuration(cluster, role string, duration float64) {
	if r.enableMetrics {
		recordNodeReplacementDurationMetric(cluster, role, duration)
	}
}

func (r *ClusterReconciler) recordHCloudAPICall(operation, result string, latency float64) {
	if r.enableMetrics {
		recordHCloudAPICallMetric(operation, result, latency)
	}
}
