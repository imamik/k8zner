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
//   hcloud-k8s --help
package main

import (
	"fmt"
	"os"

	"github.com/sak-d/hcloud-k8s/cmd/hcloud-k8s/commands"
)

func main() {
	if err := commands.Root().Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
