// Package addons defines addon metadata and installation ordering for
// the operator's phase-based addon reconciliation.
//
// Each addon has a name, install order, and enabled flag derived from
// the cluster spec. The operator installs addons one at a time in order,
// tracking status in the cluster resource after each step.
package addons
