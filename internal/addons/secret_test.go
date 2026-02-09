package addons

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// Note: Uses mockK8sClient from kubectl_test.go (same package)

func TestCreateHCloudSecret(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		token       string
		networkID   int64
		mockErr     error
		expectErr   bool
		errContains string
	}{
		{
			name:      "success",
			token:     "test-token",
			networkID: 12345,
			mockErr:   nil,
			expectErr: false,
		},
		{
			name:        "client error",
			token:       "test-token",
			networkID:   12345,
			mockErr:     errors.New("create failed"),
			expectErr:   true,
			errContains: "failed to create hcloud secret",
		},
		{
			name:      "large network ID",
			token:     "test-token",
			networkID: 9999999999,
			mockErr:   nil,
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := new(mockK8sClient)

			// Capture the secret to verify its structure
			var capturedSecret *corev1.Secret
			client.On("CreateSecret", mock.Anything, mock.AnythingOfType("*v1.Secret")).Run(func(args mock.Arguments) {
				capturedSecret = args.Get(1).(*corev1.Secret)
			}).Return(tt.mockErr)

			err := createHCloudSecret(context.Background(), client, tt.token, tt.networkID)

			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)

				// Verify secret structure
				require.NotNil(t, capturedSecret)
				assert.Equal(t, "hcloud", capturedSecret.Name)
				assert.Equal(t, "kube-system", capturedSecret.Namespace)
				assert.Equal(t, corev1.SecretTypeOpaque, capturedSecret.Type)
				assert.Equal(t, tt.token, capturedSecret.StringData["token"])
				assert.Contains(t, capturedSecret.StringData["network"], "")
			}

			client.AssertExpectations(t)
		})
	}
}

func TestCreateHCloudSecret_NetworkIDFormat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		networkID      int64
		expectedString string
	}{
		{
			name:           "single digit",
			networkID:      1,
			expectedString: "1",
		},
		{
			name:           "multiple digits",
			networkID:      12345,
			expectedString: "12345",
		},
		{
			name:           "large number",
			networkID:      1234567890,
			expectedString: "1234567890",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := new(mockK8sClient)

			var capturedSecret *corev1.Secret
			client.On("CreateSecret", mock.Anything, mock.AnythingOfType("*v1.Secret")).Run(func(args mock.Arguments) {
				capturedSecret = args.Get(1).(*corev1.Secret)
			}).Return(nil)

			err := createHCloudSecret(context.Background(), client, "token", tt.networkID)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedString, capturedSecret.StringData["network"])
		})
	}
}
