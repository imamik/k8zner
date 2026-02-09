// Package compute provisions control plane and worker servers on Hetzner Cloud.
//
// Servers are created in parallel within each node pool, attached to the
// cluster network, and labeled for identification. The package handles SSH
// key assignment, placement group distribution, and IP allocation.
package compute
