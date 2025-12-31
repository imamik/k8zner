package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "hcloud-k8s",
	Short: "A CLI to deploy Kubernetes on Hetzner Cloud using Talos",
	Long:  `hcloud-k8s replaces Terraform and Packer for provisioning Kubernetes on Hetzner Cloud with Talos Linux.`,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
