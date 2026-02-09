// Package helm provides Helm chart management for addon installation.
//
// It includes a chart registry mapping addon names to chart specifications,
// a client for downloading, rendering, and applying charts, and shared
// value builder functions for common Kubernetes constructs like tolerations,
// topology spread constraints, and namespace manifests.
//
// Charts are cached locally to avoid repeated downloads. The client uses
// Server-Side Apply to install rendered manifests into the cluster.
package helm
