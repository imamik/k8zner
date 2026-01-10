# Contributing to hcloud-k8s

Thank you for your interest in contributing to hcloud-k8s! This document provides guidelines and information for contributors.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/hcloud-k8s.git`
3. Create a feature branch: `git checkout -b feature/my-feature`
4. Make your changes
5. Run tests: `go test ./...`
6. Commit your changes with a descriptive message
7. Push to your fork and submit a pull request

## Development Setup

### Prerequisites

- Go 1.21 or later
- A Hetzner Cloud account and API token (for E2E tests)
- `golangci-lint` for linting (optional)

### Building

```bash
go build -o hcloud-k8s ./cmd/hcloud-k8s
```

### Running Tests

```bash
# Unit tests
go test ./...

# With coverage
go test -cover ./...

# E2E tests (requires HCLOUD_TOKEN)
export HCLOUD_TOKEN="your-token"
go test -v -tags=e2e ./tests/e2e/...
```

### Linting

```bash
golangci-lint run
```

## Code Style

We follow the patterns documented in [.claude/CODE_STRUCTURE.md](.claude/CODE_STRUCTURE.md). Key points:

### Package Organization

- **cmd/**: CLI commands and handlers (keep business logic in handlers)
- **internal/provisioning/**: Core provisioning domain with subpackages
- **internal/platform/**: External system integrations (hcloud, talos, ssh)
- **internal/util/**: Small, focused utility packages

### Function Design

- Keep functions under 50 lines
- Use verb-first naming: `reconcileCluster()` not `clusterReconcile()`
- Return errors, don't log and continue
- Wrap errors with context: `fmt.Errorf("failed to X: %w", err)`

### Logging

- Use the `Observer` interface for all logging in provisioning code
- Use consistent phase prefixes: `ctx.Logger.Printf("[%s] message", phase)`
- Never log and return the same error

### Documentation

- All exported functions need godoc comments
- Package-level comments in `doc.go` files
- Keep comments concise (1-3 lines for functions)

## Pull Request Guidelines

1. **Keep PRs focused**: One feature or fix per PR
2. **Write tests**: Add tests for new functionality
3. **Update documentation**: Update README or relevant docs if needed
4. **Follow commit conventions**: Use descriptive commit messages
5. **Ensure CI passes**: All tests and lints must pass

### Commit Message Format

```
<type>(<scope>): <description>

[optional body]
```

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`

Examples:
- `feat(provisioning): Add validation for network CIDR`
- `fix(compute): Handle nil pointer in server creation`
- `docs(readme): Update architecture section`

## Architecture Decisions

Major architecture decisions are documented in [.claude/CODE_STRUCTURE.md](.claude/CODE_STRUCTURE.md). When proposing significant changes:

1. Discuss in an issue first
2. Document the rationale
3. Consider backwards compatibility
4. Update relevant documentation

## Questions?

- Open an issue for bugs or feature requests
- Check existing issues before creating new ones
- Be respectful and constructive in discussions
