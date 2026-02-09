// Package infrastructure provisions cloud networking resources on Hetzner Cloud.
//
// It creates and manages networks with subnets, firewalls with ingress/egress
// rules, and load balancers for the Kubernetes API. All resources are created
// idempotently and labeled for cluster association.
package infrastructure
