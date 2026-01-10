package hcloud

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateServer_NetworkParameterValidation(t *testing.T) {
	client := NewRealClient("test-token")

	testCases := []struct {
		name      string
		networkID int64
		privateIP string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "both empty - valid",
			networkID: 0,
			privateIP: "",
			wantErr:   false, // Will fail later due to missing server type, but validation passes
		},
		{
			name:      "both provided - valid",
			networkID: 123,
			privateIP: "10.0.0.5",
			wantErr:   false, // Will fail later due to missing server type, but validation passes
		},
		{
			name:      "only networkID - invalid",
			networkID: 123,
			privateIP: "",
			wantErr:   true,
			errMsg:    "networkID and privateIP must both be provided or both be empty",
		},
		{
			name:      "only privateIP - invalid",
			networkID: 0,
			privateIP: "10.0.0.5",
			wantErr:   true,
			errMsg:    "networkID and privateIP must both be provided or both be empty",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			_, err := client.CreateServer(ctx, "test", "image", "type", "nbg1", nil, nil, "", nil, tc.networkID, tc.privateIP)

			if tc.wantErr {
				assert.Error(t, err)
				if tc.errMsg != "" {
					assert.Contains(t, err.Error(), tc.errMsg)
				}
			}
			// Note: For valid cases, we expect errors from downstream (missing server type, etc.)
			// but the validation error should not be present
			if !tc.wantErr && err != nil {
				assert.NotContains(t, err.Error(), "networkID and privateIP")
			}
		})
	}
}
