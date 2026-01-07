// Package main is the entry point for the hcloud-k8s CLI.
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
