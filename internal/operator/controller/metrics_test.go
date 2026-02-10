package controller

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestRecordReconcileMetric(t *testing.T) {
	// Reset metrics for testing
	reconcileTotal.Reset()
	reconcileDuration.Reset()

	recordReconcileMetric("test-cluster", "success", 1.5)

	// Verify counter was incremented
	counter, err := reconcileTotal.GetMetricWithLabelValues("test-cluster", "success")
	assert.NoError(t, err)
	assert.Equal(t, float64(1), testutil.ToFloat64(counter))

	// Record another reconcile
	recordReconcileMetric("test-cluster", "error", 0.5)

	errorCounter, err := reconcileTotal.GetMetricWithLabelValues("test-cluster", "error")
	assert.NoError(t, err)
	assert.Equal(t, float64(1), testutil.ToFloat64(errorCounter))
}

func TestRecordNodeCountsMetric(t *testing.T) {
	// Reset metrics for testing
	nodesTotal.Reset()
	nodesHealthy.Reset()
	nodesDesired.Reset()

	recordNodeCountsMetric("test-cluster", "control-plane", 3, 2, 3)

	// Verify gauges were set
	totalGauge, err := nodesTotal.GetMetricWithLabelValues("test-cluster", "control-plane", "total")
	assert.NoError(t, err)
	assert.Equal(t, float64(3), testutil.ToFloat64(totalGauge))

	healthyGauge, err := nodesHealthy.GetMetricWithLabelValues("test-cluster", "control-plane")
	assert.NoError(t, err)
	assert.Equal(t, float64(2), testutil.ToFloat64(healthyGauge))

	desiredGauge, err := nodesDesired.GetMetricWithLabelValues("test-cluster", "control-plane")
	assert.NoError(t, err)
	assert.Equal(t, float64(3), testutil.ToFloat64(desiredGauge))
}

func TestRecordNodeReplacementMetric(t *testing.T) {
	// Reset metrics for testing
	nodeReplacementsTotal.Reset()

	recordNodeReplacementMetric("test-cluster", "worker", "NodeNotReady")

	counter, err := nodeReplacementsTotal.GetMetricWithLabelValues("test-cluster", "worker", "NodeNotReady")
	assert.NoError(t, err)
	assert.Equal(t, float64(1), testutil.ToFloat64(counter))

	// Record another replacement
	recordNodeReplacementMetric("test-cluster", "worker", "NodeNotReady")
	assert.Equal(t, float64(2), testutil.ToFloat64(counter))
}

func TestRecordNodeReplacementDurationMetric(t *testing.T) {
	// Reset metrics for testing
	nodeReplacementDuration.Reset()

	// Just verify it doesn't panic - histograms are harder to test directly
	recordNodeReplacementDurationMetric("test-cluster", "control-plane", 120.0)
	recordNodeReplacementDurationMetric("test-cluster", "control-plane", 60.0)

	// Verify the metric exists with the label
	_, err := nodeReplacementDuration.GetMetricWithLabelValues("test-cluster", "control-plane")
	assert.NoError(t, err)
}

func TestRecordEtcdStatusMetric(t *testing.T) {
	// Reset metrics for testing
	etcdMembersTotal.Reset()
	etcdHealthy.Reset()

	recordEtcdStatusMetric("test-cluster", 3, true)

	membersGauge, err := etcdMembersTotal.GetMetricWithLabelValues("test-cluster")
	assert.NoError(t, err)
	assert.Equal(t, float64(3), testutil.ToFloat64(membersGauge))

	healthyGauge, err := etcdHealthy.GetMetricWithLabelValues("test-cluster")
	assert.NoError(t, err)
	assert.Equal(t, float64(1), testutil.ToFloat64(healthyGauge))

	// Test unhealthy status
	recordEtcdStatusMetric("test-cluster", 3, false)
	assert.Equal(t, float64(0), testutil.ToFloat64(healthyGauge))
}

// --- Wrapper method tests (test enableMetrics guard) ---

func TestRecordNodeCounts_MetricsEnabled(t *testing.T) {
	nodesTotal.Reset()
	nodesHealthy.Reset()
	nodesDesired.Reset()

	r := &ClusterReconciler{enableMetrics: true}
	r.recordNodeCounts("wrapper-cluster", "worker", 5, 4, 5)

	totalGauge, err := nodesTotal.GetMetricWithLabelValues("wrapper-cluster", "worker", "total")
	assert.NoError(t, err)
	assert.Equal(t, float64(5), testutil.ToFloat64(totalGauge))

	healthyGauge, err := nodesHealthy.GetMetricWithLabelValues("wrapper-cluster", "worker")
	assert.NoError(t, err)
	assert.Equal(t, float64(4), testutil.ToFloat64(healthyGauge))
}

func TestRecordNodeCounts_MetricsDisabled(t *testing.T) {
	nodesTotal.Reset()
	nodesHealthy.Reset()
	nodesDesired.Reset()

	r := &ClusterReconciler{enableMetrics: false}
	r.recordNodeCounts("disabled-cluster", "worker", 5, 4, 5)

	// With metrics disabled, the gauge should not have been set
	totalGauge, err := nodesTotal.GetMetricWithLabelValues("disabled-cluster", "worker", "total")
	assert.NoError(t, err)
	assert.Equal(t, float64(0), testutil.ToFloat64(totalGauge))
}

func TestRecordReconcile_MetricsEnabled(t *testing.T) {
	reconcileTotal.Reset()
	reconcileDuration.Reset()

	r := &ClusterReconciler{enableMetrics: true}
	r.recordReconcile("wrapper-cluster", "success", 1.0)

	counter, err := reconcileTotal.GetMetricWithLabelValues("wrapper-cluster", "success")
	assert.NoError(t, err)
	assert.Equal(t, float64(1), testutil.ToFloat64(counter))
}

func TestRecordReconcile_MetricsDisabled(t *testing.T) {
	reconcileTotal.Reset()

	r := &ClusterReconciler{enableMetrics: false}
	r.recordReconcile("disabled-cluster", "success", 1.0)

	counter, err := reconcileTotal.GetMetricWithLabelValues("disabled-cluster", "success")
	assert.NoError(t, err)
	assert.Equal(t, float64(0), testutil.ToFloat64(counter))
}

func TestRecordNodeReplacement_MetricsEnabled(t *testing.T) {
	nodeReplacementsTotal.Reset()

	r := &ClusterReconciler{enableMetrics: true}
	r.recordNodeReplacement("wrapper-cluster", "control-plane", "StuckProvisioning")

	counter, err := nodeReplacementsTotal.GetMetricWithLabelValues("wrapper-cluster", "control-plane", "StuckProvisioning")
	assert.NoError(t, err)
	assert.Equal(t, float64(1), testutil.ToFloat64(counter))
}

func TestRecordNodeReplacementDuration_MetricsEnabled(t *testing.T) {
	nodeReplacementDuration.Reset()

	r := &ClusterReconciler{enableMetrics: true}
	r.recordNodeReplacementDuration("wrapper-cluster", "worker", 90.0)

	_, err := nodeReplacementDuration.GetMetricWithLabelValues("wrapper-cluster", "worker")
	assert.NoError(t, err)
}

func TestRecordHCloudAPICall_MetricsEnabled(t *testing.T) {
	hcloudAPICallsTotal.Reset()
	hcloudAPILatency.Reset()

	r := &ClusterReconciler{enableMetrics: true}
	r.recordHCloudAPICall("list_servers", "success", 0.5)

	counter, err := hcloudAPICallsTotal.GetMetricWithLabelValues("list_servers", "success")
	assert.NoError(t, err)
	assert.Equal(t, float64(1), testutil.ToFloat64(counter))
}

func TestRecordHCloudAPICallMetric(t *testing.T) {
	// Reset metrics for testing
	hcloudAPICallsTotal.Reset()
	hcloudAPILatency.Reset()

	recordHCloudAPICallMetric("create_server", "success", 2.5)

	counter, err := hcloudAPICallsTotal.GetMetricWithLabelValues("create_server", "success")
	assert.NoError(t, err)
	assert.Equal(t, float64(1), testutil.ToFloat64(counter))

	recordHCloudAPICallMetric("delete_server", "error", 0.5)

	errorCounter, err := hcloudAPICallsTotal.GetMetricWithLabelValues("delete_server", "error")
	assert.NoError(t, err)
	assert.Equal(t, float64(1), testutil.ToFloat64(errorCounter))
}
