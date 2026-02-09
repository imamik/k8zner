// Package image builds Talos Linux disk images as Hetzner Cloud snapshots.
//
// The build process creates a temporary server in rescue mode, installs
// Talos to disk via SSH, creates a snapshot, and cleans up the temporary
// server. The resulting snapshot is used as the boot image for all cluster
// nodes.
package image
