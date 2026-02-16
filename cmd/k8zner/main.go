// Package main is the entry point for the k8zner CLI.
//
// k8zner is a command-line tool for provisioning production-ready
// Kubernetes clusters on Hetzner Cloud using Talos Linux. It provides
// a stateless, declarative approach to infrastructure management without
// requiring Terraform or other IaC tools.
//
// Commands: init, apply, destroy, doctor, cost, secrets.
//
// For detailed usage information, run:
//
//	k8zner --help
package main

import (
	"fmt"
	"os"

	"github.com/imamik/k8zner/cmd/k8zner/commands"
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
