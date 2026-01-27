package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImage(t *testing.T) {
	cmd := Image()

	require.NotNil(t, cmd)
	assert.Equal(t, "image", cmd.Use)
	assert.Equal(t, "Manage Talos images", cmd.Short)
}

func TestImage_HasBuildSubcommand(t *testing.T) {
	cmd := Image()

	subcommands := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subcommands[sub.Use] = true
	}

	assert.True(t, subcommands["build"], "Expected build subcommand")
}

func TestBuild(t *testing.T) {
	cmd := Build()

	require.NotNil(t, cmd)
	assert.Equal(t, "build", cmd.Use)
	assert.Equal(t, "Build a new Talos image", cmd.Short)
}

func TestBuild_NameFlag(t *testing.T) {
	cmd := Build()

	flag := cmd.Flags().Lookup("name")
	require.NotNil(t, flag, "name flag should exist")
	assert.Equal(t, "", flag.Shorthand)
	assert.Equal(t, "talos", flag.DefValue)
	assert.Equal(t, "Name of the image to create", flag.Usage)
}

func TestBuild_VersionFlag(t *testing.T) {
	cmd := Build()

	flag := cmd.Flags().Lookup("version")
	require.NotNil(t, flag, "version flag should exist")
	assert.Equal(t, "", flag.Shorthand)
	assert.Equal(t, "v1.7.0", flag.DefValue)
	assert.Equal(t, "Talos version to install", flag.Usage)
}

func TestBuild_LocationFlag(t *testing.T) {
	cmd := Build()

	flag := cmd.Flags().Lookup("location")
	require.NotNil(t, flag, "location flag should exist")
	assert.Equal(t, "", flag.Shorthand)
	assert.Equal(t, "nbg1", flag.DefValue)
	assert.Contains(t, flag.Usage, "Hetzner datacenter location")
}

func TestBuild_ArchFlag(t *testing.T) {
	cmd := Build()

	flag := cmd.Flags().Lookup("arch")
	require.NotNil(t, flag, "arch flag should exist")
	assert.Equal(t, "", flag.Shorthand)
	assert.Equal(t, "amd64", flag.DefValue)
	assert.Contains(t, flag.Usage, "Architecture")
}

func TestBuild_RunE(t *testing.T) {
	cmd := Build()
	assert.NotNil(t, cmd.RunE, "Build command should have RunE function")
}
