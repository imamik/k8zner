package compute

import (
	"fmt"
	"time"

	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/util/rdns"
)

const (
	// rdnsMaxRetries is the maximum number of RDNS operation retries.
	rdnsMaxRetries = 3

	// rdnsBaseDelay is the base delay for exponential backoff (2s, 4s, 8s).
	rdnsBaseDelay = 2 * time.Second
)

// retryRDNS executes a function with exponential backoff retry logic.
// Uses 3 retries with delays of 2s, 4s, 8s (total max ~14 seconds).
func retryRDNS(ctx *provisioning.Context, operation func() error, resourceType string) error {
	var lastErr error

	for attempt := 0; attempt < rdnsMaxRetries; attempt++ {
		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		// Don't sleep after the last attempt
		if attempt < rdnsMaxRetries-1 {
			delay := rdnsBaseDelay * time.Duration(1<<uint(attempt))
			ctx.Logger.Printf("[%s] RDNS operation failed for %s (attempt %d/%d), retrying in %v: %v",
				phase, resourceType, attempt+1, rdnsMaxRetries, delay, err)
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("failed after %d attempts: %w", rdnsMaxRetries, lastErr)
}

// applyServerRDNSSimple configures reverse DNS for a server using pre-resolved templates.
func (p *Provisioner) applyServerRDNSSimple(ctx *provisioning.Context, serverID int64, serverName, ipv4, rdnsIPv4, rdnsIPv6, role, pool string) error {
	// Apply IPv4 RDNS if configured
	if rdnsIPv4 != "" && ipv4 != "" {
		dnsPtr, err := rdns.RenderTemplate(rdnsIPv4, rdns.TemplateVars{
			ClusterName: ctx.Config.ClusterName,
			Hostname:    serverName,
			ID:          serverID,
			IPAddress:   ipv4,
			IPType:      "ipv4",
			Pool:        pool,
			Role:        role,
		})
		if err != nil {
			return fmt.Errorf("failed to render IPv4 RDNS template: %w", err)
		}

		// Set RDNS with retry logic
		if err := retryRDNS(ctx, func() error {
			return ctx.Infra.SetServerRDNS(ctx, serverID, ipv4, dnsPtr)
		}, fmt.Sprintf("server %d (%s)", serverID, serverName)); err != nil {
			return fmt.Errorf("failed to set IPv4 RDNS for %s (IP: %s → %s): %w", serverName, ipv4, dnsPtr, err)
		}

		ctx.Logger.Printf("[%s] Set IPv4 RDNS: %s → %s", phase, ipv4, dnsPtr)
	}

	// Apply IPv6 RDNS if configured (IPv6 support can be added later)
	if rdnsIPv6 != "" {
		// IPv6 address retrieval not yet implemented
		ctx.Logger.Printf("[%s] IPv6 RDNS configured but IPv6 address retrieval not yet implemented", phase)
	}

	return nil
}
