# hcloud-k8s

![Build Status](https://github.com/sak-d/hcloud-k8s/actions/workflows/ci.yaml/badge.svg)
![Go Report Card](https://goreportcard.com/badge/github.com/sak-d/hcloud-k8s)

`hcloud-k8s` is a CLI tool designed to replace Terraform and Packer for provisioning Kubernetes on Hetzner Cloud using Talos Linux. It provides a stateless, label-based reconciliation strategy for managing infrastructure and generating node configurations.

## Features

- **Image Builder:** Create custom Talos snapshots on Hetzner Cloud.
- **Cluster Reconciliation:** Provision and manage Control Plane servers (idempotent).
- **Talos Config Generation:** Automatically generates secure Talos machine configurations and client secrets.
- **Persistence:** Saves cluster secrets and `talosconfig` locally for ongoing management.
- **Privacy:** Enforces SSH key usage to prevent root password transmission via email.

## Quick Start

### 1. Build the Binary

```bash
git clone https://github.com/sak-d/hcloud-k8s.git
cd hcloud-k8s
go build -o hcloud-k8s ./cmd/hcloud-k8s
```

### 2. Build a Talos Image

Create a custom snapshot for your cluster nodes.

```bash
export HCLOUD_TOKEN="your-token"
./hcloud-k8s image build --name "talos-v1.9.0" --version "v1.9.0" --arch "amd64"
```

### 3. Create a Cluster Configuration

Create a `config.yaml` file:

```yaml
cluster_name: "my-talos-cluster"
location: "nbg1" # nbg1, fsn1, hel1, ash, etc.
ssh_keys:
  - "my-ssh-key-name" # Must exist in Hetzner Cloud

control_plane:
  count: 3
  server_type: "cpx22" # cpx22 for AMD64, cax11 for ARM
  image: "talos-v1.9.0" # Matches the image name built above
  endpoint: "https://<LOAD_BALANCER_IP>:6443" # VIP or LB IP

talos:
  version: "v1.9.0"
  k8s_version: "1.32.0"
```

### 4. Apply Configuration

Provision the servers and generate configs.

```bash
./hcloud-k8s apply --config config.yaml
```

This will:
1.  Verify/Create the SSH keys and resources.
2.  Provision 3 Control Plane servers.
3.  Generate `secrets.yaml` (CA keys) and `talosconfig` in the current directory.

## Development

### Architecture

The project follows a standard Go layout:
- `cmd/`: Entry points for the application.
- `internal/`: Private application logic (`cluster`, `hcloud`, `talos`, `config`).
- `pkg/`: Shared library code.

### Testing

Run unit tests:
```bash
go test ./...
```

Run End-to-End (E2E) tests (requires `HCLOUD_TOKEN`):
```bash
export HCLOUD_TOKEN="your-token"
go test -v -tags=e2e ./tests/e2e/...
```
