//go:build e2e

package e2e

import (
	"testing"
)

// TestE2EOperatorScale tests the operator-centric scaling flow.
// This test validates that the operator correctly scales workers
// when the K8znerCluster CRD spec is modified.
//
// This test should be run AFTER a cluster has been created.
// It uses the shared E2E state from the test suite.
//
// Run the full E2E suite: go test -tags=e2e -v ./tests/e2e/...
// Or run after creating a cluster manually.
func TestE2EOperatorScale(t *testing.T) {
	// This test is integrated into the E2E lifecycle via phaseOperatorScale
	// and is called from the main E2E tests (TestE2EDevCluster, TestE2EHACluster)
	// after the cluster is created.
	//
	// To run standalone, you need to have a cluster already running and
	// provide the state. This is typically done via the sequential test.
	t.Skip("Run via TestE2ELifecycle or TestE2EDevCluster/TestE2EHACluster with operator scaling enabled")
}
