// Package destroy handles cluster teardown and resource cleanup.
//
// It removes all Hetzner Cloud resources associated with a cluster by
// querying resources with the cluster label. Resources are deleted in
// dependency order: servers first, then load balancers, firewalls,
// networks, snapshots, SSH keys, and certificates.
package destroy
