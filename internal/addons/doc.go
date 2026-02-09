// Package addons installs and manages Kubernetes cluster addons.
//
// Addons are installed in a specific order to satisfy dependencies:
// Cilium (CNI) first, then CCM/CSI, then higher-level services like
// Traefik, cert-manager, ArgoCD, and monitoring.
//
// The package supports two installation paths:
//   - CLI path: [Apply] installs all addons sequentially
//   - Operator path: [ApplyCilium] installs CNI first, then [InstallStep]
//     installs remaining addons one at a time with status tracking
//
// Each addon is implemented as a build function that generates Helm values
// and an install function that renders and applies the chart.
package addons
