// Package talos generates and manages Talos Linux machine configurations.
//
// It produces control plane and worker configs with Hetzner-specific patches
// including disk encryption, network interface configuration, and kernel
// parameters. The generator manages Talos secrets and produces client configs
// for authenticated API access.
package talos
