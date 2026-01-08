package hcloud

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"hcloud-k8s/internal/util/retry"
)

// resolveImage resolves and waits for an image to become available.
func (c *RealClient) resolveImage(ctx context.Context, imageType string, serverTypeObj *hcloud.ServerType) (*hcloud.Image, error) {
	var imageObj *hcloud.Image

	// Special handling for talos
	if imageType == "talos" {
		images, _, err := c.client.Image.List(ctx, hcloud.ImageListOpts{
			ListOpts: hcloud.ListOpts{
				LabelSelector: "os=talos",
			},
			Architecture: []hcloud.Architecture{serverTypeObj.Architecture},
			Sort:         []string{"created:desc"},
		})
		if err == nil && len(images) > 0 {
			imageObj = images[0]
		}
	}

	// Try to get image by name if not found via labels or if another image was requested.
	if imageObj == nil {
		var err error
		imageObj, _, err = c.client.Image.Get(ctx, imageType) //nolint:staticcheck
		if err != nil {
			return nil, fmt.Errorf("failed to get image: %w", err)
		}
	}

	if imageObj == nil {
		return nil, fmt.Errorf("image not found: %s", imageType)
	}

	// Wait for image to be available if it's still creating
	if imageObj.Status != hcloud.ImageStatusAvailable {
		if err := c.waitForImageAvailability(ctx, imageObj); err != nil {
			return nil, err
		}
	}

	// Check if image architecture matches server type architecture.
	if imageObj.Architecture != serverTypeObj.Architecture {
		images, _, err := c.client.Image.List(ctx, hcloud.ImageListOpts{
			Name:         imageType,
			Architecture: []hcloud.Architecture{serverTypeObj.Architecture},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list images: %w", err)
		}
		if len(images) > 0 {
			imageObj = images[0]
		}
	}

	return imageObj, nil
}

// waitForImageAvailability waits for an image to become available.
func (c *RealClient) waitForImageAvailability(ctx context.Context, imageObj *hcloud.Image) error {
	log.Printf("Image %s (%d) is in status %s, waiting for it to become available...", imageObj.Name, imageObj.ID, imageObj.Status)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	timeout := time.After(c.timeouts.ImageWait)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for image %d to become available", imageObj.ID)
		case <-ticker.C:
			img, _, err := c.client.Image.GetByID(ctx, imageObj.ID)
			if err != nil {
				return fmt.Errorf("failed to get image status: %w", err)
			}
			if img.Status == hcloud.ImageStatusAvailable {
				return nil
			}
			log.Printf("Still waiting for image %d (status: %s)...", imageObj.ID, img.Status)
		}
	}
}

// resolveSSHKeys resolves SSH key names/IDs to SSH key objects.
func (c *RealClient) resolveSSHKeys(ctx context.Context, sshKeys []string) ([]*hcloud.SSHKey, error) {
	var sshKeyObjs []*hcloud.SSHKey
	for _, key := range sshKeys {
		keyObj, _, err := c.client.SSHKey.Get(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("failed to get ssh key %s: %w", key, err)
		}
		if keyObj == nil {
			return nil, fmt.Errorf("ssh key not found: %s", key)
		}
		sshKeyObjs = append(sshKeyObjs, keyObj)
	}
	return sshKeyObjs, nil
}

// resolveLocation resolves a location name to a location object.
func (c *RealClient) resolveLocation(ctx context.Context, location string) (*hcloud.Location, error) {
	if location == "" {
		return nil, nil
	}

	locObj, _, err := c.client.Location.Get(ctx, location)
	if err != nil {
		return nil, fmt.Errorf("failed to get location %s: %w", location, err)
	}
	if locObj == nil {
		return nil, fmt.Errorf("location not found: %s", location)
	}
	return locObj, nil
}

// resolvePlacementGroup converts a placement group ID to a placement group object.
func resolvePlacementGroup(placementGroupID *int64) *hcloud.PlacementGroup {
	if placementGroupID == nil {
		return nil
	}
	return &hcloud.PlacementGroup{ID: *placementGroupID}
}

// attachServerToNetwork attaches a server to a network with the specified private IP and powers it on.
func (c *RealClient) attachServerToNetwork(ctx context.Context, server *hcloud.Server, networkID int64, privateIP string) error {
	ip := net.ParseIP(privateIP)
	if ip == nil {
		return fmt.Errorf("invalid private ip: %s", privateIP)
	}

	attachOpts := hcloud.ServerAttachToNetworkOpts{
		Network: &hcloud.Network{ID: networkID},
		IP:      ip,
	}

	// Attach to network with retry logic (network might not be ready immediately)
	err := retry.WithExponentialBackoff(ctx, func() error {
		action, _, err := c.client.Server.AttachToNetwork(ctx, server, attachOpts)
		if err != nil {
			return err
		}
		return c.client.Action.WaitFor(ctx, action)
	}, retry.WithMaxRetries(c.timeouts.RetryMaxAttempts), retry.WithInitialDelay(c.timeouts.RetryInitialDelay))

	if err != nil {
		return fmt.Errorf("failed to attach server to network: %w", err)
	}

	// Power On
	action, _, err := c.client.Server.Poweron(ctx, server)
	if err != nil {
		return fmt.Errorf("failed to power on server: %w", err)
	}
	if err := c.client.Action.WaitFor(ctx, action); err != nil {
		return fmt.Errorf("failed to wait for server power on: %w", err)
	}

	return nil
}
