# hcloud-k8s

![Build Status](https://github.com/sak-d/hcloud-k8s/actions/workflows/ci.yaml/badge.svg)
![Go Report Card](https://goreportcard.com/badge/github.com/sak-d/hcloud-k8s)

`hcloud-k8s` is a CLI tool designed to replace Terraform and Packer for provisioning Kubernetes on Hetzner Cloud using Talos Linux. It aims to provide a stateless, label-based reconciliation strategy for managing resources.

## Quick Start

```bash
# Clone the repository
git clone https://github.com/sak-d/hcloud-k8s.git
cd hcloud-k8s

# Build the binary
make build

# Run the help command
./bin/hcloud-k8s --help
```

## Architecture

The project follows a standard Go layout:
- `cmd/`: Entry points for the application.
- `internal/`: Private application logic.
- `pkg/`: Shared library code.
- `deploy/`: Deployment configurations.

See `spec/implementation_plan_v1.md` and `spec/technical_design_doc.md` for more details.
