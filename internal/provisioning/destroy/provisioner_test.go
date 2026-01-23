package destroy

import (
	"context"
	"testing"

	"k8zner/internal/config"
	"k8zner/internal/platform/hcloud"
	"k8zner/internal/provisioning"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvisionerName(t *testing.T) {
	p := NewProvisioner()
	assert.Equal(t, "Destroy", p.Name())
}

func TestProvision(t *testing.T) {
	tests := []struct {
		name          string
		clusterName   string
		testID        string
		setupMock     func(*hcloud.MockClient)
		expectError   bool
		errorContains string
	}{
		{
			name:        "successful destroy",
			clusterName: "test-cluster",
			setupMock: func(m *hcloud.MockClient) {
				m.CleanupByLabelFunc = func(_ context.Context, labels map[string]string) error {
					// Verify cluster label is present
					clusterLabel, ok := labels["cluster"]
					if !ok {
						t.Error("cluster label not found in CleanupByLabel call")
					}
					if clusterLabel != "test-cluster" {
						t.Errorf("expected cluster=test-cluster, got cluster=%s", clusterLabel)
					}
					return nil
				}
			},
			expectError: false,
		},
		{
			name:        "successful destroy with test ID",
			clusterName: "test-cluster",
			testID:      "e2e-12345",
			setupMock: func(m *hcloud.MockClient) {
				m.CleanupByLabelFunc = func(_ context.Context, labels map[string]string) error {
					// Verify both cluster and test-id labels are present
					if labels["cluster"] != "test-cluster" {
						t.Error("cluster label mismatch")
					}
					if labels["test-id"] != "e2e-12345" {
						t.Error("test-id label mismatch")
					}
					return nil
				}
			},
			expectError: false,
		},
		{
			name:        "cleanup fails",
			clusterName: "test-cluster",
			setupMock: func(m *hcloud.MockClient) {
				m.CleanupByLabelFunc = func(_ context.Context, _ map[string]string) error {
					return assert.AnError
				}
			},
			expectError:   true,
			errorContains: "failed to cleanup cluster resources",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock client
			mockClient := &hcloud.MockClient{}
			tt.setupMock(mockClient)

			// Create config
			cfg := &config.Config{
				ClusterName: tt.clusterName,
				TestID:      tt.testID,
			}

			// Create provisioning context
			pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, nil)

			// Create provisioner and run
			provisioner := NewProvisioner()
			err := provisioner.Provision(pCtx)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestProvisionCallsCleanupWithCorrectLabels(t *testing.T) {
	var capturedLabels map[string]string

	mockClient := &hcloud.MockClient{
		CleanupByLabelFunc: func(_ context.Context, labels map[string]string) error {
			capturedLabels = labels
			return nil
		},
	}

	cfg := &config.Config{
		ClusterName: "production-cluster",
		TestID:      "",
	}

	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, nil)
	provisioner := NewProvisioner()

	err := provisioner.Provision(pCtx)
	require.NoError(t, err)

	// Verify the captured labels
	require.NotNil(t, capturedLabels)
	assert.Equal(t, "production-cluster", capturedLabels["cluster"])

	// Test ID should not be present
	_, hasTestID := capturedLabels["test-id"]
	assert.False(t, hasTestID, "test-id should not be present when not configured")
}
