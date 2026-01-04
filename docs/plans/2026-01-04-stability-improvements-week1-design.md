# Stability Improvements (Week 1) - Design Document

**Date**: 2026-01-04
**Status**: Approved
**Priority**: High

## Executive Summary

This design adds robust retry logic and configurable timeouts to critical infrastructure operations, addressing stability issues identified in the code review. These improvements prevent unnecessary failures during transient API issues, rate limits, and resource contention.

## Goals

1. Add retry logic with exponential backoff to all critical operations
2. Make timeouts configurable via environment variables
3. Extend server IP retrieval timeout from 20s to 60s
4. Add context timeouts to all delete operations
5. Maintain backward compatibility

## Non-Goals

- Cleanup command (Week 4)
- Label-based resource discovery (Week 2)
- Observability/metrics (Week 3)
- Dry-run mode (Week 4)

---

## Architecture Overview

### New Components

```
internal/
├── retry/              # NEW: Retry utilities package
│   ├── retry.go        # Exponential backoff implementation
│   └── retry_test.go   # Comprehensive unit tests
├── config/
│   ├── timeouts.go     # NEW: Timeout configuration
│   └── timeouts_test.go
├── hcloud/
│   └── server.go       # MODIFIED: Add retries + timeouts
├── cluster/
│   └── reconciler.go   # MODIFIED: Use timeout config
└── image/
    └── builder.go      # MODIFIED: Use retry helper
```

### Key Principles

- **Backward Compatible**: No breaking API changes
- **Fail-Safe Defaults**: Sensible defaults if env vars not set
- **Context-Aware**: All operations respect context cancellation
- **Logging**: Log retry attempts for debugging
- **Testable**: Mock-friendly design

---

## Detailed Design

### 1. Retry Helper (`internal/retry/retry.go`)

#### Core Function

```go
package retry

import (
    "context"
    "fmt"
    "log"
    "time"
)

// Config holds retry configuration
type Config struct {
    MaxRetries   int
    InitialDelay time.Duration
    MaxDelay     time.Duration
    Multiplier   float64
}

// Option is a functional option for retry configuration
type Option func(*Config)

// WithExponentialBackoff executes the operation with exponential backoff retry
func WithExponentialBackoff(ctx context.Context, operation func() error, opts ...Option) error {
    cfg := &Config{
        MaxRetries:   5,
        InitialDelay: 1 * time.Second,
        MaxDelay:     30 * time.Second,
        Multiplier:   2.0,
    }

    for _, opt := range opts {
        opt(cfg)
    }

    delay := cfg.InitialDelay

    for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
        err := operation()
        if err == nil {
            return nil
        }

        // Check if error is fatal (non-retryable)
        if IsFatal(err) {
            return fmt.Errorf("fatal error (not retrying): %w", err)
        }

        if attempt < cfg.MaxRetries {
            log.Printf("Retry attempt %d/%d after error: %v (waiting %s)",
                attempt+1, cfg.MaxRetries, err, delay)

            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-time.After(delay):
                delay = time.Duration(float64(delay) * cfg.Multiplier)
                if delay > cfg.MaxDelay {
                    delay = cfg.MaxDelay
                }
            }
        }
    }

    return fmt.Errorf("operation failed after %d retries", cfg.MaxRetries)
}

// WithMaxRetries sets the maximum number of retries
func WithMaxRetries(n int) Option {
    return func(c *Config) {
        c.MaxRetries = n
    }
}

// WithInitialDelay sets the initial delay between retries
func WithInitialDelay(d time.Duration) Option {
    return func(c *Config) {
        c.InitialDelay = d
    }
}

// FatalError wraps an error to mark it as fatal (non-retryable)
type FatalError struct {
    Err error
}

func (e *FatalError) Error() string {
    return e.Err.Error()
}

func (e *FatalError) Unwrap() error {
    return e.Err
}

// Fatal marks an error as fatal (non-retryable)
func Fatal(err error) error {
    if err == nil {
        return nil
    }
    return &FatalError{Err: err}
}

// IsFatal checks if an error is fatal (non-retryable)
func IsFatal(err error) bool {
    var fatalErr *FatalError
    return errors.As(err, &fatalErr)
}
```

#### Error Classification

**Retryable Errors**:
- Network timeouts
- Rate limits (HTTP 429)
- Resource locked (e.g., snapshot in progress)
- Temporary API unavailability (HTTP 502, 503)

**Fatal Errors** (wrap with `retry.Fatal()`):
- Invalid credentials (HTTP 401)
- Resource not found (HTTP 404) - after initial check
- Invalid parameters (HTTP 400)
- Validation errors

---

### 2. Timeout Configuration (`internal/config/timeouts.go`)

#### Environment Variables

```bash
# Server operations
HCLOUD_TIMEOUT_SERVER_CREATE=10m      # Server creation timeout
HCLOUD_TIMEOUT_SERVER_IP=60s          # Wait for server IP timeout
HCLOUD_TIMEOUT_IMAGE_WAIT=5m          # Image availability wait

# Delete operations
HCLOUD_TIMEOUT_DELETE=5m              # All delete operations

# Bootstrap operations
HCLOUD_TIMEOUT_BOOTSTRAP=10m          # Bootstrap wait timeout

# Retry configuration
HCLOUD_RETRY_MAX_ATTEMPTS=5           # Max retry attempts
HCLOUD_RETRY_INITIAL_DELAY=1s         # Initial retry delay
```

#### Implementation

```go
package config

import (
    "os"
    "strconv"
    "time"
)

// Timeouts holds all configurable timeout values
type Timeouts struct {
    ServerCreate      time.Duration
    ServerIP          time.Duration
    Delete            time.Duration
    Bootstrap         time.Duration
    ImageWait         time.Duration
    RetryMaxAttempts  int
    RetryInitialDelay time.Duration
}

// LoadTimeouts loads timeout configuration from environment variables
func LoadTimeouts() *Timeouts {
    return &Timeouts{
        ServerCreate:      parseDuration("HCLOUD_TIMEOUT_SERVER_CREATE", 10*time.Minute),
        ServerIP:          parseDuration("HCLOUD_TIMEOUT_SERVER_IP", 60*time.Second),
        Delete:            parseDuration("HCLOUD_TIMEOUT_DELETE", 5*time.Minute),
        Bootstrap:         parseDuration("HCLOUD_TIMEOUT_BOOTSTRAP", 10*time.Minute),
        ImageWait:         parseDuration("HCLOUD_TIMEOUT_IMAGE_WAIT", 5*time.Minute),
        RetryMaxAttempts:  parseInt("HCLOUD_RETRY_MAX_ATTEMPTS", 5),
        RetryInitialDelay: parseDuration("HCLOUD_RETRY_INITIAL_DELAY", 1*time.Second),
    }
}

func parseDuration(envVar string, defaultVal time.Duration) time.Duration {
    val := os.Getenv(envVar)
    if val == "" {
        return defaultVal
    }
    d, err := time.ParseDuration(val)
    if err != nil {
        log.Printf("Warning: Invalid duration for %s: %v, using default %s", envVar, err, defaultVal)
        return defaultVal
    }
    return d
}

func parseInt(envVar string, defaultVal int) int {
    val := os.Getenv(envVar)
    if val == "" {
        return defaultVal
    }
    i, err := strconv.Atoi(val)
    if err != nil {
        log.Printf("Warning: Invalid integer for %s: %v, using default %d", envVar, err, defaultVal)
        return defaultVal
    }
    return i
}
```

#### Default Values

| Timeout | Default | Rationale |
|---------|---------|-----------|
| ServerCreate | 10 minutes | Server creation + network attachment |
| ServerIP | 60 seconds | Up from 20s; handles peak times |
| Delete | 5 minutes | Allow time for resource cleanup |
| Bootstrap | 10 minutes | Talos boot + etcd initialization |
| ImageWait | 5 minutes | Current value, now configurable |
| RetryMaxAttempts | 5 | Reasonable for transient failures |
| RetryInitialDelay | 1 second | Exponential backoff starting point |

---

### 3. Server Creation Improvements

#### Changes to `internal/hcloud/server.go`

**Inject Timeouts into Client**:
```go
type RealClient struct {
    client   *hcloud.Client
    timeouts *config.Timeouts  // NEW
}

func NewRealClient(token string) *RealClient {
    return &RealClient{
        client:   hcloud.NewClient(hcloud.WithToken(token)),
        timeouts: config.LoadTimeouts(),  // NEW
    }
}
```

**Modify CreateServer (lines 15-176)**:

1. **Wrap with timeout context**:
```go
func (c *RealClient) CreateServer(ctx context.Context, ...) (string, error) {
    ctx, cancel := context.WithTimeout(ctx, c.timeouts.ServerCreate)
    defer cancel()

    // ... existing logic
}
```

2. **Add retry to server creation API call** (around line 137):
```go
var result hcloud.ServerCreateResult

err = retry.WithExponentialBackoff(ctx, func() error {
    res, _, err := c.client.Server.Create(ctx, opts)
    if err != nil {
        // Check for fatal errors
        if isInvalidParameter(err) {
            return retry.Fatal(err)
        }
        return err
    }
    result = *res
    return nil
}, retry.WithMaxRetries(c.timeouts.RetryMaxAttempts))

if err != nil {
    return "", fmt.Errorf("failed to create server: %w", err)
}
```

3. **Update image wait timeout** (line 56):
```go
timeout := time.After(c.timeouts.ImageWait)
```

4. **Add retry to network attachment** (lines 157-163):
```go
err = retry.WithExponentialBackoff(ctx, func() error {
    action, _, err := c.client.Server.AttachToNetwork(ctx, result.Server, attachOpts)
    if err != nil {
        return err
    }
    return c.client.Action.WaitFor(ctx, action)
})
```

---

### 4. Server IP Retrieval Improvements

#### Changes to `internal/cluster/reconciler.go:399-410`

**Current Implementation**:
```go
// Fixed 10 retries with 2-second sleep = 20 seconds max
for i := 0; i < 10; i++ {
    ip, err = r.serverProvisioner.GetServerIP(ctx, serverName)
    if err == nil && ip != "" {
        break
    }
    time.Sleep(2 * time.Second)
}
```

**New Implementation**:
```go
// Use configurable timeout with exponential backoff
ipCtx, cancel := context.WithTimeout(ctx, r.timeouts.ServerIP)
defer cancel()

err = retry.WithExponentialBackoff(ipCtx, func() error {
    ip, err = r.serverProvisioner.GetServerIP(ctx, serverName)
    if err != nil {
        return err
    }
    if ip == "" {
        return fmt.Errorf("server IP not yet assigned")
    }
    return nil
}, retry.WithInitialDelay(1*time.Second))

if err != nil {
    return "", fmt.Errorf("failed to get server IP for %s after timeout: %w", serverName, err)
}
```

**Benefits**:
- Timeout increases from 20s to 60s (configurable)
- Exponential backoff reduces API calls
- More resilient during peak times

---

### 5. Delete Operation Improvements

#### Add Timeouts and Retries to All Delete Methods

**Pattern to apply**:

```go
func (c *RealClient) Delete<Resource>(ctx context.Context, name string) error {
    ctx, cancel := context.WithTimeout(ctx, c.timeouts.Delete)
    defer cancel()

    return retry.WithExponentialBackoff(ctx, func() error {
        resource, _, err := c.client.<Resource>.Get(ctx, name)
        if err != nil {
            return retry.Fatal(fmt.Errorf("failed to get resource: %w", err))
        }
        if resource == nil {
            return nil // Already deleted
        }

        _, err = c.client.<Resource>.Delete(ctx, resource)
        if err != nil {
            // Check if locked (retryable) vs not found (fatal)
            if isResourceLocked(err) {
                return err // Retry
            }
            return retry.Fatal(err)
        }
        return nil
    }, retry.WithMaxRetries(c.timeouts.RetryMaxAttempts))
}
```

**Files to modify**:
- `internal/hcloud/server.go:178-194` - DeleteServer
- `internal/hcloud/network.go:82-92` - DeleteNetwork
- `internal/hcloud/firewall.go:51-61` - DeleteFirewall
- `internal/hcloud/load_balancer.go:118-128` - DeleteLoadBalancer
- `internal/hcloud/placement_group.go:32-42` - DeletePlacementGroup
- `internal/hcloud/floating_ip.go:38-48` - DeleteFloatingIP
- `internal/hcloud/snapshot.go:36-48` - DeleteImage
- `internal/hcloud/ssh_key.go:24-37` - DeleteSSHKey

**Error Classification Helper**:
```go
// internal/hcloud/errors.go (NEW)
func isResourceLocked(err error) bool {
    // Hetzner returns specific error codes for locked resources
    return strings.Contains(err.Error(), "locked") ||
           strings.Contains(err.Error(), "conflict")
}

func isInvalidParameter(err error) bool {
    return strings.Contains(err.Error(), "invalid") ||
           strings.Contains(err.Error(), "not found")
}
```

---

### 6. Update Image Builder

#### Simplify Cleanup Using Retry Helper

**Current**: `internal/image/builder.go:158-215` has manual retry logic

**New**: Replace with retry helper

```go
func (b *Builder) cleanupServer(serverName string) {
    log.Printf("Cleaning up server %s...", serverName)
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
    defer cancel()

    err := retry.WithExponentialBackoff(ctx, func() error {
        return b.provisioner.DeleteServer(ctx, serverName)
    })

    if err != nil {
        log.Printf("Failed to delete server %s: %v", serverName, err)
    } else {
        log.Printf("Server %s deleted successfully", serverName)
    }
}

func (b *Builder) cleanupSSHKey(keyName string) {
    log.Printf("Cleaning up SSH key %s...", keyName)
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer cancel()

    err := retry.WithExponentialBackoff(ctx, func() error {
        return b.sshKeyManager.DeleteSSHKey(ctx, keyName)
    })

    if err != nil {
        log.Printf("Failed to delete SSH key %s: %v", keyName, err)
    } else {
        log.Printf("SSH key %s deleted successfully", keyName)
    }
}
```

**Benefits**:
- Reduces code duplication
- Consistent retry behavior across codebase
- Easier to test

---

### 7. Integration with Reconciler

#### Modify `internal/cluster/reconciler.go`

**Inject timeouts**:
```go
type Reconciler struct {
    // ... existing fields
    timeouts *config.Timeouts  // NEW
}

func NewReconciler(
    infra hcloud_internal.InfrastructureManager,
    talosGenerator TalosConfigProducer,
    cfg *config.Config,
) *Reconciler {
    return &Reconciler{
        // ... existing initialization
        timeouts: config.LoadTimeouts(),  // NEW
    }
}
```

**Use in reconciliation**:
- Pass timeouts to operations that need them
- Context propagation for cancellation

---

## Testing Strategy

### Unit Tests

#### `internal/retry/retry_test.go`

```go
func TestWithExponentialBackoff_Success(t *testing.T)
func TestWithExponentialBackoff_MaxRetries(t *testing.T)
func TestWithExponentialBackoff_ContextCancellation(t *testing.T)
func TestWithExponentialBackoff_FatalError(t *testing.T)
func TestWithExponentialBackoff_BackoffTiming(t *testing.T)
```

#### `internal/config/timeouts_test.go`

```go
func TestLoadTimeouts_Defaults(t *testing.T)
func TestLoadTimeouts_EnvVars(t *testing.T)
func TestLoadTimeouts_InvalidEnvVars(t *testing.T)
```

#### Modified Existing Tests

- Update server creation tests to work with retry logic
- Update reconciler tests to mock timeouts
- Ensure backward compatibility in all tests

### Integration Tests

- Run existing E2E tests with default configuration
- Run E2E tests with custom timeouts
- Verify retry logs appear correctly

### Manual Testing Checklist

- [ ] Set `HCLOUD_TIMEOUT_SERVER_IP=30s` and verify behavior
- [ ] Set `HCLOUD_RETRY_MAX_ATTEMPTS=3` and verify retry count
- [ ] Test with invalid `HCLOUD_TOKEN` to verify retries and failure
- [ ] Monitor logs for retry attempt messages
- [ ] Verify context cancellation works (Ctrl+C during operation)

---

## Migration Guide

### For Users

**No action required** - all changes are backward compatible.

**Optional**: Set environment variables to customize timeouts:
```bash
export HCLOUD_TIMEOUT_SERVER_IP=90s  # For slow environments
export HCLOUD_RETRY_MAX_ATTEMPTS=10  # For flaky networks
```

### For Developers

**When adding new operations**:
1. Wrap with appropriate timeout context
2. Use `retry.WithExponentialBackoff` for retryable operations
3. Mark fatal errors with `retry.Fatal()`
4. Add unit tests for retry scenarios

---

## Rollout Plan

### Phase 1: Implementation (This Session)
1. Create `internal/retry` package
2. Create `internal/config/timeouts.go`
3. Update `internal/hcloud/server.go`
4. Update `internal/cluster/reconciler.go`
5. Update all delete operations
6. Update `internal/image/builder.go`
7. Write unit tests

### Phase 2: Testing
1. Run unit tests
2. Run existing E2E tests (if HCLOUD_TOKEN available)
3. Manual testing with various timeout configurations

### Phase 3: Documentation
1. Update README with environment variable documentation
2. Add troubleshooting guide for timeout tuning
3. Commit design document

### Phase 4: Review & Merge
1. Create PR with all changes
2. Code review
3. Merge to main branch

---

## Success Metrics

- [ ] All unit tests pass
- [ ] All E2E tests pass with default configuration
- [ ] Server creation succeeds with simulated transient failures
- [ ] Server IP retrieval handles 60-second delays
- [ ] Delete operations retry on locked resources
- [ ] Logs show retry attempts with timing information
- [ ] No breaking changes to existing APIs

---

## Future Enhancements (Out of Scope)

These will be addressed in subsequent weeks:

- **Week 2**: Enhanced E2E cleanup with label-based discovery
- **Week 3**: Observability (metrics, structured logging)
- **Week 4**: Cleanup command, dry-run mode

---

## References

- Code Review: Initial analysis of stability issues
- Hetzner Cloud API: https://docs.hetzner.cloud/
- Go Context Package: https://golang.org/pkg/context/
- Exponential Backoff: https://en.wikipedia.org/wiki/Exponential_backoff
