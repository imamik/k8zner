# E2E Test Cleanup Utility

This utility cleans up leftover resources from interrupted or failed E2E tests.

## When to Use

Use this cleanup utility when:

- E2E tests are interrupted (killed with Ctrl+C, SIGKILL, etc.)
- Tests fail and don't cleanup properly
- You want to ensure no leftover test resources exist
- You see test failures about resources already existing

## Usage

### Quick Cleanup via Makefile

```bash
export HCLOUD_TOKEN="your-token"
make e2e-cleanup
```

### Manual Cleanup

```bash
export HCLOUD_TOKEN="your-token"
go run tests/e2e/cmd/cleanup/main.go
```

## What Gets Cleaned Up

The utility searches for and deletes:

### By Name Prefix
- `e2e-seq-*` - Sequential E2E test clusters and resources
- `build-talos-*` - Snapshot build servers
- `key-build-talos-*` - Snapshot build SSH keys

### Resources Cleaned
- Servers
- Load Balancers
- Firewalls
- Networks
- Placement Groups
- SSH Keys
- Snapshots (older than 24 hours with E2E labels)

## Cleanup Strategy

The utility uses a **name-based cleanup** approach:

1. Lists ALL resources of each type
2. Filters by name prefix
3. Deletes matching resources
4. Reports count of deleted resources

This is more reliable than label-based cleanup because:
- Works even if labels are missing or malformed
- Doesn't require complex label selectors
- Catches resources from old test runs
- Simple and predictable

## Safety

The utility:
- Only deletes resources with specific E2E test prefixes
- Won't touch production or non-test resources
- Has a 15-minute timeout for safety
- Logs all actions and errors
- Only deletes snapshots older than 24 hours

## Exit Codes

- `0` - Success, all resources cleaned up
- `1` - Some errors occurred, check logs

## Cleanup Verification

After running cleanup, verify no leftover resources:

```bash
# List servers
hcloud server list | grep e2e-seq

# List load balancers
hcloud load-balancer list | grep e2e-seq

# List networks
hcloud network list | grep e2e-seq
```

## Integration with Tests

E2E tests automatically:
1. Register cleanup with `defer cleanupE2ECluster()`
2. Run cleanup even on test failures
3. Verify cleanup completed successfully
4. Report any leftover resources

However, cleanup won't run if:
- Test process is killed with SIGKILL (exit 137)
- System runs out of memory
- Test timeout is exceeded
- Process is forcefully terminated

In these cases, run this utility manually.

## Preventing Leftover Resources

To minimize leftover resources:

1. **Use proper timeouts**: Give tests enough time to cleanup
   ```bash
   go test -timeout=2h -tags=e2e ./tests/e2e/
   ```

2. **Don't kill tests**: Use Ctrl+C once and wait for cleanup

3. **Monitor resources**: Periodically run cleanup utility

4. **Use E2E_KEEP_SNAPSHOTS**: Reuse snapshots between runs
   ```bash
   E2E_KEEP_SNAPSHOTS=true make e2e-fast
   ```

## Troubleshooting

### "Failed to delete X" errors

Some resources may fail to delete due to:
- Dependencies (e.g., servers attached to networks)
- API rate limiting
- Hetzner Cloud temporary issues

The utility will continue and report all errors. Run it again if needed.

### Resources still exist after cleanup

Check if resources have different naming patterns. Update the cleanup utility's `prefixes` array if needed.

### Old snapshots accumulating

The utility only deletes snapshots older than 24 hours. To delete all E2E snapshots:

```bash
# List E2E snapshots
hcloud image list | grep talos-v

# Delete manually if needed
hcloud image delete <snapshot-id>
```
