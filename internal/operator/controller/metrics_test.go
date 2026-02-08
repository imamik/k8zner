package controller

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestRecordReconcile(t *testing.T) {
	// Reset metrics for testing
	reconcileTotal.Reset()
	reconcileDuration.Reset()

	RecordReconcile("test-cluster", "success", 1.5)

	// Verify counter was incremented
	counter, err := reconcileTotal.GetMetricWithLabelValues("test-cluster", "success")
	assert.NoError(t, err)
	assert.Equal(t, float64(1), testutil.ToFloat64(counter))

	// Record another reconcile
	RecordReconcile("test-cluster", "error", 0.5)

	errorCounter, err := reconcileTotal.GetMetricWithLabelValues("test-cluster", "error")
	assert.NoError(t, err)
	assert.Equal(t, float64(1), testutil.ToFloat64(errorCounter))
}

func TestRecordNodeCounts(t *testing.T) {
	// Reset metrics for testing
	nodesTotal.Reset()
	nodesHealthy.Reset()
	nodesDesired.Reset()

	RecordNodeCounts("test-cluster", "control-plane", 3, 2, 3)

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

func TestRecordNodeReplacement(t *testing.T) {
	// Reset metrics for testing
	nodeReplacementsTotal.Reset()

	RecordNodeReplacement("test-cluster", "worker", "NodeNotReady")

	counter, err := nodeReplacementsTotal.GetMetricWithLabelValues("test-cluster", "worker", "NodeNotReady")
	assert.NoError(t, err)
	assert.Equal(t, float64(1), testutil.ToFloat64(counter))

	// Record another replacement
	RecordNodeReplacement("test-cluster", "worker", "NodeNotReady")
	assert.Equal(t, float64(2), testutil.ToFloat64(counter))
}

func TestRecordNodeReplacementDuration(t *testing.T) {
	// Reset metrics for testing
	nodeReplacementDuration.Reset()

	// Just verify it doesn't panic - histograms are harder to test directly
	RecordNodeReplacementDuration("test-cluster", "control-plane", 120.0)
	RecordNodeReplacementDuration("test-cluster", "control-plane", 60.0)

	// Verify the metric exists with the label
	_, err := nodeReplacementDuration.GetMetricWithLabelValues("test-cluster", "control-plane")
	assert.NoError(t, err)
}

func TestRecordEtcdStatus(t *testing.T) {
	// Reset metrics for testing
	etcdMembersTotal.Reset()
	etcdHealthy.Reset()

	RecordEtcdStatus("test-cluster", 3, true)

	membersGauge, err := etcdMembersTotal.GetMetricWithLabelValues("test-cluster")
	assert.NoError(t, err)
	assert.Equal(t, float64(3), testutil.ToFloat64(membersGauge))

	healthyGauge, err := etcdHealthy.GetMetricWithLabelValues("test-cluster")
	assert.NoError(t, err)
	assert.Equal(t, float64(1), testutil.ToFloat64(healthyGauge))

	// Test unhealthy status
	RecordEtcdStatus("test-cluster", 3, false)
	assert.Equal(t, float64(0), testutil.ToFloat64(healthyGauge))
}

func TestRecordHCloudAPICall(t *testing.T) {
	// Reset metrics for testing
	hcloudAPICallsTotal.Reset()
	hcloudAPILatency.Reset()

	RecordHCloudAPICall("create_server", "success", 2.5)

	counter, err := hcloudAPICallsTotal.GetMetricWithLabelValues("create_server", "success")
	assert.NoError(t, err)
	assert.Equal(t, float64(1), testutil.ToFloat64(counter))

	RecordHCloudAPICall("delete_server", "error", 0.5)

	errorCounter, err := hcloudAPICallsTotal.GetMetricWithLabelValues("delete_server", "error")
	assert.NoError(t, err)
	assert.Equal(t, float64(1), testutil.ToFloat64(errorCounter))
}
