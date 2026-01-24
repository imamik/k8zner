package addons

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/imamik/k8zner/internal/addons/k8sclient"
)

// applyManifests applies Kubernetes manifests using the k8sclient.
// It uses Server-Side Apply with the given field manager name.
func applyManifests(ctx context.Context, client k8sclient.Client, addonName string, manifestBytes []byte) error {
	if err := client.ApplyManifests(ctx, manifestBytes, addonName); err != nil {
		return fmt.Errorf("failed to apply manifests for addon %s: %w", addonName, err)
	}
	return nil
}

// applyFromURL downloads a manifest from a URL and applies it using the k8sclient.
// This is useful for applying CRDs or other manifests hosted remotely.
func applyFromURL(ctx context.Context, client k8sclient.Client, addonName, manifestURL string) error {
	// Create HTTP request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request for %s: %w", manifestURL, err)
	}

	// Download the manifest
	httpClient := &http.Client{Timeout: 60 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download manifest from %s: %w", manifestURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download manifest from %s: HTTP %d", manifestURL, resp.StatusCode)
	}

	// Read the manifest content
	manifestBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read manifest from %s: %w", manifestURL, err)
	}

	// Apply using the k8sclient
	return applyManifests(ctx, client, addonName, manifestBytes)
}
