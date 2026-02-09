// Package ssh provides an SSH client for executing commands on remote servers.
//
// It is used during Talos image building to connect to Hetzner servers in
// rescue mode, install Talos to disk, and verify the installation. The
// client supports key-based authentication with configurable retry logic.
package ssh
