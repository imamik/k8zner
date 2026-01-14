// Package helm provides a lightweight abstraction for rendering Helm charts
// as Kubernetes manifests. Charts are pre-rendered at build time and embedded
// in the binary, matching Terraform's offline-first approach.
package helm

import (
	"embed"
)

//go:embed all:templates/*
var templatesFS embed.FS

// Chart represents metadata about an embedded helm chart.
type Chart struct {
	Name       string
	Version    string
	Repository string
}
