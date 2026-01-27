package addons

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// mockK8sClient is a local mock for testing
type mockK8sClient struct {
	mock.Mock
}

func (m *mockK8sClient) ApplyManifests(ctx context.Context, manifests []byte, fieldManager string) error {
	args := m.Called(ctx, manifests, fieldManager)
	return args.Error(0)
}

func (m *mockK8sClient) CreateSecret(ctx context.Context, secret *corev1.Secret) error {
	args := m.Called(ctx, secret)
	return args.Error(0)
}

func (m *mockK8sClient) DeleteSecret(ctx context.Context, namespace, name string) error {
	args := m.Called(ctx, namespace, name)
	return args.Error(0)
}

func TestApplyManifests(t *testing.T) {
	tests := []struct {
		name        string
		addonName   string
		manifests   []byte
		mockErr     error
		expectErr   bool
		errContains string
	}{
		{
			name:      "success",
			addonName: "test-addon",
			manifests: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test"),
			mockErr:   nil,
			expectErr: false,
		},
		{
			name:        "apply error",
			addonName:   "test-addon",
			manifests:   []byte("apiVersion: v1\nkind: ConfigMap"),
			mockErr:     errors.New("apply failed"),
			expectErr:   true,
			errContains: "failed to apply manifests for addon test-addon",
		},
		{
			name:      "empty manifest",
			addonName: "test-addon",
			manifests: []byte{},
			mockErr:   nil,
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := new(mockK8sClient)
			client.On("ApplyManifests", mock.Anything, tt.manifests, tt.addonName).Return(tt.mockErr)

			err := applyManifests(context.Background(), client, tt.addonName, tt.manifests)

			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestApplyFromURL_Success(t *testing.T) {
	manifestContent := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
  namespace: default
data:
  key: value`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(manifestContent))
	}))
	defer server.Close()

	client := new(mockK8sClient)
	client.On("ApplyManifests", mock.Anything, []byte(manifestContent), "test-addon").Return(nil)

	err := applyFromURL(context.Background(), client, "test-addon", server.URL)
	require.NoError(t, err)
	client.AssertExpectations(t)
}

func TestApplyFromURL_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := new(mockK8sClient)

	err := applyFromURL(context.Background(), client, "test-addon", server.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 404")
}

func TestApplyFromURL_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := new(mockK8sClient)

	err := applyFromURL(context.Background(), client, "test-addon", server.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 500")
}

func TestApplyFromURL_InvalidURL(t *testing.T) {
	client := new(mockK8sClient)

	err := applyFromURL(context.Background(), client, "test-addon", "http://[::1]:namedport")
	require.Error(t, err)
	// Could fail on request creation or download
	assert.True(t,
		strings.Contains(err.Error(), "failed to download manifest") ||
			strings.Contains(err.Error(), "failed to create request"),
		"Expected error about download or request creation, got: %s", err.Error())
}

func TestApplyFromURL_ApplyError(t *testing.T) {
	manifestContent := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(manifestContent))
	}))
	defer server.Close()

	client := new(mockK8sClient)
	client.On("ApplyManifests", mock.Anything, []byte(manifestContent), "test-addon").Return(errors.New("apply failed"))

	err := applyFromURL(context.Background(), client, "test-addon", server.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to apply manifests for addon test-addon")
}

func TestApplyFromURL_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow response that will be canceled
		select {
		case <-r.Context().Done():
			return
		}
	}))
	defer server.Close()

	client := new(mockK8sClient)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := applyFromURL(ctx, client, "test-addon", server.URL)
	require.Error(t, err)
}
