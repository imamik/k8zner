package main

import (
	"context"
	"fmt"
	"os"

	"github.com/hcloud-k8s/hcloud-k8s/pkg/cloud"
	"github.com/hcloud-k8s/hcloud-k8s/pkg/cluster"
	"github.com/hcloud-k8s/hcloud-k8s/pkg/config"
	"github.com/hcloud-k8s/hcloud-k8s/pkg/image"
	"github.com/hcloud-k8s/hcloud-k8s/pkg/talos"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	configPath string
    token      string
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "config.yaml", "Path to configuration file")
    rootCmd.PersistentFlags().StringVarP(&token, "token", "t", "", "Hetzner Cloud Token")

	rootCmd.AddCommand(applyCmd)
}

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply configuration to create/update cluster",
	RunE: func(cmd *cobra.Command, args []string) error {
        if token == "" {
            token = os.Getenv("HCLOUD_TOKEN")
        }
        if token == "" {
            return fmt.Errorf("HCLOUD_TOKEN is required")
        }

		// 1. Load Config
		data, err := os.ReadFile(configPath)
		if err != nil {
			return fmt.Errorf("failed to read config: %w", err)
		}
		var cfg config.ClusterConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
		}

		ctx := context.Background()
		c := cloud.NewCloud(token, &cfg)
		imgBuilder := image.NewBuilder(token)
		talosGen := talos.NewGenerator(&cfg)

        applier := cluster.NewApplier(&cfg, c, imgBuilder, talosGen)

		return applier.Apply(ctx)
	},
}
