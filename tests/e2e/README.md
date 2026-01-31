# E2E Tests

End-to-end tests for k8zner that deploy real infrastructure on Hetzner Cloud.

## Prerequisites

All tests require:
```bash
export HCLOUD_TOKEN="your-hetzner-api-token"
```

## Running Tests

### Basic Dev Cluster Test

Deploys a minimal dev cluster and verifies all core addons:

```bash
go test -v -timeout=45m -tags=e2e -run TestE2EDevCluster ./tests/e2e/
```

### HA Cluster Test

Deploys a full HA cluster with 3 control planes:

```bash
go test -v -timeout=60m -tags=e2e -run TestE2EHACluster ./tests/e2e/
```

### ArgoCD Dashboard Test

Tests ArgoCD dashboard with DNS and TLS (requires Cloudflare):

```bash
export CF_API_TOKEN="your-cloudflare-token"
export CF_DOMAIN="your-domain.com"

go test -v -timeout=45m -tags=e2e -run TestE2EDevClusterWithArgoCD ./tests/e2e/
```

### S3 Backup Test

Tests S3 bucket operations with Hetzner Object Storage:

```bash
export HETZNER_S3_ACCESS_KEY="your-s3-access-key"
export HETZNER_S3_SECRET_KEY="your-s3-secret-key"

go test -v -timeout=10m -tags=e2e -run TestE2EBackup ./tests/e2e/
```

## Environment Variables

| Variable | Required For | Description |
|----------|--------------|-------------|
| `HCLOUD_TOKEN` | All tests | Hetzner Cloud API token |
| `CF_API_TOKEN` | DNS/TLS tests | Cloudflare API token with Zone:Read and DNS:Edit permissions |
| `CF_DOMAIN` | DNS/TLS tests | Domain managed by Cloudflare (e.g., `example.com`) |
| `HETZNER_S3_ACCESS_KEY` | Backup tests | Hetzner Object Storage access key |
| `HETZNER_S3_SECRET_KEY` | Backup tests | Hetzner Object Storage secret key |
| `E2E_KEEP_SNAPSHOTS` | Optional | Set to `true` to cache Talos snapshots between test runs |

## Getting Credentials

### Hetzner Cloud Token

1. Go to [Hetzner Cloud Console](https://console.hetzner.cloud/)
2. Select your project → Security → API Tokens
3. Generate a new token with Read/Write permissions

### Cloudflare API Token

1. Go to [Cloudflare API Tokens](https://dash.cloudflare.com/profile/api-tokens)
2. Create Token → Use "Edit zone DNS" template
3. Set Zone Resources to your domain
4. Required permissions: `Zone > Zone > Read`, `Zone > DNS > Edit`

### Hetzner Object Storage Credentials

1. Go to [Hetzner Cloud Console](https://console.hetzner.cloud/)
2. Navigate to Object Storage → Security Credentials
3. Generate new credentials

## Test Resources

### What Gets Created

| Test | Resources |
|------|-----------|
| Dev Cluster | 1 CP node, 1 worker, 1 LB, network, firewall |
| HA Cluster | 3 CP nodes, 3 workers, 2 LBs, network, firewall |
| ArgoCD | Same as Dev + DNS record + TLS certificate |
| Backup | S3 bucket only (no cluster) |

### Resource Cleanup

Tests automatically clean up resources when they complete (pass or fail).

**S3 Buckets**: The backup test cleans up its test bucket. However, when using `backup: true` in a real cluster, the bucket is intentionally NOT deleted on `k8zner destroy` to preserve backups for disaster recovery.

### Estimated Costs

- Dev cluster test (~45 min): ~€0.50
- HA cluster test (~60 min): ~€1.50
- S3 backup test (~10 min): ~€0.01

## Troubleshooting

### Test Timeout

Increase the timeout:
```bash
go test -v -timeout=90m -tags=e2e ...
```

### Resource Cleanup Failed

If tests leave orphaned resources, clean them manually:
```bash
# List resources with the test prefix
hcloud server list | grep e2e-
hcloud network list | grep e2e-
hcloud load-balancer list | grep e2e-

# Delete manually if needed
hcloud server delete <name>
```

### S3 Bucket Cleanup

```bash
# Using AWS CLI configured for Hetzner
aws s3 rb s3://e2e-backup-*-etcd-backups --force \
  --endpoint-url https://fsn1.your-objectstorage.com
```
