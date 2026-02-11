package hcloud

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateServer_NetworkParameterValidation(t *testing.T) {
	t.Parallel()
	client := NewRealClient("test-token")

	// Test that various network parameter combinations are accepted.
	// privateIP is now optional when networkID is provided - HCloud will auto-assign an IP.
	testCases := []struct {
		name      string
		networkID int64
		privateIP string
	}{
		{
			name:      "both empty - no network attachment",
			networkID: 0,
			privateIP: "",
		},
		{
			name:      "both provided - explicit IP assignment",
			networkID: 123,
			privateIP: "10.0.0.5",
		},
		{
			name:      "only networkID - HCloud auto-assigns IP",
			networkID: 123,
			privateIP: "",
		},
		{
			name:      "only privateIP - ignored when no network",
			networkID: 0,
			privateIP: "10.0.0.5",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			_, err := client.CreateServer(ctx, ServerCreateOpts{
				Name: "test", ImageType: "image", ServerType: "type", Location: "nbg1",
				NetworkID: tc.networkID, PrivateIP: tc.privateIP,
				EnablePublicIPv4: true, EnablePublicIPv6: true,
			})

			// All parameter combinations are now valid at the validation level.
			// Errors will occur downstream (missing server type, API errors, etc.)
			// but should NOT be network parameter validation errors.
			if err != nil {
				assert.NotContains(t, err.Error(), "networkID and privateIP")
			}
		})
	}
}
