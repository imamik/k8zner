// Package naming provides consistent naming functions for Hetzner Cloud resources.
//
// Resource names follow the pattern {cluster}-{type} for infrastructure
// (networks, firewalls, load balancers) and {cluster}-{role}-{5char} for
// nodes (control planes, workers). The random suffix prevents naming
// conflicts during scaling and replacement operations.
package naming
