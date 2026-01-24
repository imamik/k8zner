# Security Policy

## Supported Versions

We release patches for security vulnerabilities for the following versions:

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |
| < latest | :x:               |

We recommend always using the latest release.

## Reporting a Vulnerability

We take security vulnerabilities seriously. If you discover a security issue, please report it responsibly.

### How to Report

**Please do NOT report security vulnerabilities through public GitHub issues.**

Instead, please report them via one of the following methods:

1. **GitHub Security Advisories** (preferred): Use the "Report a vulnerability" button on the Security tab of this repository.

2. **Email**: Send details to the repository maintainers (check the repository for contact information).

### What to Include

Please include as much of the following information as possible:

- Type of vulnerability (e.g., command injection, privilege escalation, information disclosure)
- Full paths of affected source files
- Location of the affected source code (tag/branch/commit or direct URL)
- Step-by-step instructions to reproduce the issue
- Proof-of-concept or exploit code (if possible)
- Impact assessment and potential attack scenarios

### Response Timeline

- **Initial Response**: Within 48 hours, we will acknowledge receipt of your report
- **Assessment**: Within 7 days, we will provide an initial assessment
- **Resolution**: We aim to release patches within 30 days for critical vulnerabilities

### Disclosure Policy

- We will work with you to understand and resolve the issue quickly
- We will keep you informed of our progress
- We will credit you in the security advisory (unless you prefer to remain anonymous)
- We ask that you give us reasonable time to address the issue before public disclosure

## Security Best Practices

When using k8zner, please follow these security recommendations:

### API Token Security

- **Never commit API tokens** to version control
- Use environment variables (`HCLOUD_TOKEN`) instead of config files
- Rotate tokens regularly
- Use tokens with minimal required permissions

### Cluster Configuration

- **SSH Keys**: Always configure SSH keys to prevent Hetzner from emailing root passwords
- **Network Isolation**: Use private networks for inter-node communication
- **Firewall Rules**: Review and restrict firewall rules to necessary traffic only

### Secrets Management

- Secrets are stored in `./secrets/<cluster-name>/` by default
- Protect these files with appropriate filesystem permissions
- Consider using external secrets management for production
- The kubeconfig and talosconfig files grant full cluster access

### Addon Security

- Keep addons updated to their latest supported versions
- Review addon configurations for security implications
- Enable network policies with Cilium for pod-level isolation

## Security Features

k8zner includes several security features:

- **Talos Linux**: Immutable, minimal OS with no SSH access by default
- **Firewall Management**: Automatic firewall configuration for cluster traffic
- **Private Networks**: Inter-node communication over private Hetzner networks
- **TLS Everywhere**: All control plane communication is encrypted

## Vulnerability Scanning

We use the following tools in our CI pipeline:

- **gosec**: Static analysis for Go security issues
- **golangci-lint**: Includes security-focused linters

## Third-Party Dependencies

We monitor our dependencies for known vulnerabilities:

- Go modules are regularly updated
- Critical dependency updates are prioritized
- See `go.mod` for current dependency versions
