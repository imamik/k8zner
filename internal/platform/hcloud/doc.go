// Package hcloud provides a wrapper around the Hetzner Cloud API client with enhanced
// reliability features including retry logic, timeout management, and error handling.
//
// # Architecture
//
// The package is organized into domain-specific modules:
//
//   - client.go: Main client initialization and configuration
//   - operations.go: Generic operations for Delete and Ensure patterns
//   - server.go: Server lifecycle management (create, delete, power operations)
//   - server_helpers.go: Server creation helper functions (image resolution, network attachment)
//   - network.go: Network and subnet management
//   - firewall.go: Firewall rule management
//   - loadbalancer.go: Load balancer provisioning
//   - placementgroup.go: Placement group management for server distribution
//   - certificate.go: TLS certificate management
//   - snapshot.go: Snapshot creation and management
//   - sshkey.go: SSH key management
//   - architecture.go: Architecture detection and server type mapping
//   - errors.go: Error classification for retry logic
//
// # Generic Operations
//
// The package uses Go generics to provide consistent Delete and Ensure operations
// across all resource types, eliminating code duplication while maintaining type safety.
//
// DeleteOperation provides idempotent resource deletion with automatic retry logic:
//   - Handles resource locking with exponential backoff
//   - Returns success if resource doesn't exist
//   - Configurable timeouts and retry parameters
//
// EnsureOperation provides get-or-create semantics with optional update/validation:
//   - Simple Ensure: Get → return if exists → Create if not
//   - Ensure with Update: Get → Update if exists → Create if not
//   - Ensure with Validation: Get → Validate if exists → Create if not
//
// # Key Features
//
//   - Retry Logic: Exponential backoff with configurable parameters
//   - Timeout Management: Configurable timeouts for all operations
//   - Error Classification: Fatal vs retryable errors for smart retry behavior
//   - Parallel Safety: Thread-safe operations suitable for concurrent use
//   - Resource Idempotency: All operations check for existing resources
//
// # Retry and Timeout Configuration
//
// Timeouts and retry parameters are configurable via environment variables:
//
//   - HCLOUD_TIMEOUT_SERVER_CREATE: Server creation timeout (default: 10m)
//   - HCLOUD_TIMEOUT_DELETE: Resource deletion timeout (default: 5m)
//   - HCLOUD_TIMEOUT_IMAGE_WAIT: Image availability wait timeout (default: 15m)
//   - HCLOUD_TIMEOUT_SERVER_IP: Server IP assignment timeout (default: 5m)
//   - HCLOUD_RETRY_MAX_ATTEMPTS: Maximum retry attempts (default: 10)
//   - HCLOUD_RETRY_INITIAL_DELAY: Initial retry delay (default: 2s)
//
// # Example Usage
//
//	// Initialize client
//	client := hcloud.NewClient(token)
//
//	// Create a server with automatic retry
//	serverID, err := client.CreateServer(ctx, ServerCreateOpts{
//	    Name:             "my-server",
//	    ImageType:        "talos",
//	    ServerType:       "cpx31",
//	    Location:         "nbg1",
//	    SSHKeys:          []string{"my-key"},
//	    Labels:           map[string]string{"role": "control-plane"},
//	    UserData:         "#!/bin/bash\necho hello",
//	    PlacementGroupID: &placementGroupID,
//	    NetworkID:        networkID,
//	    PrivateIP:        "10.0.0.2",
//	    EnablePublicIPv4: true,
//	    EnablePublicIPv6: true,
//	})
//
//	// Operations automatically handle retries for transient failures
//	// and return fatal errors immediately for permanent failures
package hcloud
