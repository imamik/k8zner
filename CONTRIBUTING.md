# Contributing to k8zner

Thank you for your interest in contributing to k8zner! This document provides guidelines and information for contributors.

## Code of Conduct

This project adheres to the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code.

## How to Contribute

### Reporting Issues

Before creating an issue, please:

1. Check existing issues to avoid duplicates
2. Use the issue templates when available
3. Include relevant details:
   - k8zner version
   - Go version
   - Talos/Kubernetes versions
   - Hetzner Cloud location
   - Steps to reproduce
   - Expected vs actual behavior

### Suggesting Features

Feature requests are welcome! Please:

1. Check existing issues for similar requests
2. Describe the use case and problem it solves
3. Consider if this aligns with the project's scope

### Pull Requests

1. **Fork and clone** the repository
2. **Create a branch** from `main` for your changes
3. **Follow the code style** (see below)
4. **Write tests** for new functionality
5. **Run checks** before submitting: `make check`
6. **Submit a PR** with a clear description

## Development Setup

### Prerequisites

- Go 1.25 or later
- golangci-lint
- make

Optional (for E2E tests and manual debugging):
- kubectl (for cluster operations)
- talosctl (for Talos-specific operations)

### Building

```bash
git clone https://github.com/imamik/k8zner.git
cd k8zner
make build
```

### Running Tests

```bash
make test       # Unit tests
make lint       # Linting
make check      # All checks
```

### End-to-End Tests

E2E tests require a Hetzner Cloud API token:

```bash
export HCLOUD_TOKEN="your-api-token"
make e2e
```

## Code Style

### General Guidelines

- Follow standard Go conventions and idioms
- Keep functions focused and under 50 lines when possible
- Prefer clarity over cleverness
- Write self-documenting code; add comments for non-obvious logic

### Package Organization

- `cmd/` — CLI commands (Cobra definitions) and handlers (business logic)
- `internal/` — Internal packages organized by domain
- One package = one responsibility

### Naming Conventions

| Type | Convention | Example |
|------|------------|---------|
| Files | snake_case | `load_balancer.go` |
| Exported functions | PascalCase | `NewReconciler()` |
| Unexported functions | camelCase | `loadConfig()` |
| Constants (exported) | PascalCase | `DefaultTimeout` |
| Constants (unexported) | camelCase | `defaultRetryCount` |

### Error Handling

- Wrap errors with context: `fmt.Errorf("failed to X: %w", err)`
- Return errors, don't log and continue
- Provide actionable context in error messages

### Documentation

- All exported functions need godoc comments
- Keep comments concise (1-3 lines)
- Package-level `doc.go` for significant packages

### Testing

- Use table-driven tests where appropriate
- Test file names: `*_test.go`
- Use `github.com/stretchr/testify` for assertions
- Mock external dependencies using interfaces

## Commit Messages

Follow conventional commit format:

```
type(scope): description

[optional body]

[optional footer]
```

Types:
- `feat` — New feature
- `fix` — Bug fix
- `docs` — Documentation only
- `refactor` — Code change that neither fixes a bug nor adds a feature
- `test` — Adding or updating tests
- `chore` — Maintenance tasks

Examples:
```
feat(addons): add support for external-dns addon
fix(provisioning): handle network creation race condition
docs: update configuration reference
refactor(hcloud): extract common delete operation logic
```

## Architecture Overview

Understanding the codebase structure helps when contributing:

```
k8zner
├── cmd/k8zner/
│   ├── commands/     # CLI definitions (Cobra)
│   └── handlers/     # Command business logic
├── internal/
│   ├── config/       # Configuration parsing and validation
│   ├── operator/     # Kubernetes operator (controller, provisioning adapter)
│   ├── provisioning/ # Infrastructure provisioning phases
│   │   ├── infrastructure/  # Network, firewall, LB
│   │   ├── compute/         # Servers
│   │   ├── image/           # Talos snapshots
│   │   └── cluster/         # Bootstrap
│   ├── addons/       # Kubernetes addon installation
│   │   └── k8sclient/       # Kubernetes client (replaces kubectl)
│   ├── platform/     # External integrations
│   │   ├── hcloud/   # Hetzner Cloud API client
│   │   ├── talos/    # Talos configuration
│   │   └── ssh/      # SSH execution
│   └── util/         # Shared utilities
└── tests/            # End-to-end and integration tests
```

### Key Patterns

- **Phase Pattern** — Provisioning uses sequential phases via `RunPhases`
- **Generic Operations** — `DeleteOperation[T]` and `EnsureOperation[T]` reduce boilerplate
- **Interface-Based Design** — External dependencies use interfaces for testability

## Getting Help

- Open an issue for questions or problems
- Check existing documentation and code comments
- Review similar PRs for patterns and conventions

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
