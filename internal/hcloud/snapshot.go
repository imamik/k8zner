package hcloud

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// CreateSnapshot creates a snapshot of the server.
func (c *RealClient) CreateSnapshot(ctx context.Context, serverID, snapshotDescription string, labels map[string]string) (string, error) {
	id, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid server id: %s", serverID)
	}
	server := &hcloud.Server{ID: id}

	result, _, err := c.client.Server.CreateImage(ctx, server, &hcloud.ServerCreateImageOpts{
		Type:        hcloud.ImageTypeSnapshot,
		Description: &snapshotDescription,
		Labels:      labels,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create snapshot: %w", err)
	}

	if err := c.client.Action.WaitFor(ctx, result.Action); err != nil {
		return "", fmt.Errorf("failed to wait for snapshot creation: %w", err)
	}

	return fmt.Sprintf("%d", result.Image.ID), nil
}

// DeleteImage deletes an image by ID.
func (c *RealClient) DeleteImage(ctx context.Context, imageID string) error {
	return deleteResource(ctx, imageID, DeleteFuncs[hcloud.Image]{
		Get: func(ctx context.Context, idStr string) (*hcloud.Image, error) {
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid image id: %s", idStr)
			}
			img, _, err := c.client.Image.GetByID(ctx, id)
			return img, err
		},
		Delete: func(ctx context.Context, img *hcloud.Image) error {
			_, err := c.client.Image.Delete(ctx, img)
			return err
		},
	}, c.getGenericTimeouts())
}

// GetSnapshotByLabels finds a snapshot matching the given labels.
func (c *RealClient) GetSnapshotByLabels(ctx context.Context, labels map[string]string) (*hcloud.Image, error) {
	opts := hcloud.ImageListOpts{
		Type: []hcloud.ImageType{hcloud.ImageTypeSnapshot},
	}
	opts.LabelSelector = buildLabelSelector(labels)

	images, err := c.client.Image.AllWithOpts(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list images: %w", err)
	}

	if len(images) == 0 {
		return nil, nil // No matching snapshot found
	}

	// Return the most recent snapshot
	return images[0], nil
}

// buildLabelSelector creates a label selector string from a map of labels.
func buildLabelSelector(labels map[string]string) string {
	var selectors []string
	for k, v := range labels {
		selectors = append(selectors, fmt.Sprintf("%s=%s", k, v))
	}
	// Join with comma for AND logic
	result := ""
	for i, s := range selectors {
		if i > 0 {
			result += ","
		}
		result += s
	}
	return result
}
