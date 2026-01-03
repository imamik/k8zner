package hcloud

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// CreateSnapshot creates a snapshot of the server.
func (c *RealClient) CreateSnapshot(ctx context.Context, serverID, snapshotDescription string) (string, error) {
	id, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid server id: %s", serverID)
	}
	server := &hcloud.Server{ID: id}

	result, _, err := c.client.Server.CreateImage(ctx, server, &hcloud.ServerCreateImageOpts{
		Type:        hcloud.ImageTypeSnapshot,
		Description: &snapshotDescription,
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
	id, err := strconv.ParseInt(imageID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid image id: %s", imageID)
	}
	image := &hcloud.Image{ID: id}

	_, err = c.client.Image.Delete(ctx, image)
	if err != nil {
		return fmt.Errorf("failed to delete image: %w", err)
	}
	return nil
}
