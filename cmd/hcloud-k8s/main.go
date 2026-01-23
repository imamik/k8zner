// Package main is the entry point for the hcloud-k8s CLI.
//
// hcloud-k8s is a command-line tool for provisioning production-ready
// Kubernetes clusters on Hetzner Cloud using Talos Linux. It provides
// a stateless, declarative approach to infrastructure management without
// requiring Terraform or other IaC tools.
//
// The CLI provides two main commands:
//   - apply: Provision and manage Kubernetes clusters
//   - image: Build custom Talos Linux snapshots
//
// For detailed usage information, run:
//
//	hcloud-k8s --help
package main

import (
	"fmt"
	"os"

	"hcloud-k8s/cmd/hcloud-k8s/commands"
)

// Version information set by goreleaser at build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	commands.SetVersionInfo(version, commit, date)
	if err := commands.Root().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
