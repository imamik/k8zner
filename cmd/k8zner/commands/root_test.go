package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoot(t *testing.T) {
	cmd := Root()

	require.NotNil(t, cmd)
	assert.Equal(t, "k8zner", cmd.Use)
	assert.Equal(t, "Provision Kubernetes on Hetzner Cloud using Talos", cmd.Short)
}

func TestRoot_HasSubcommands(t *testing.T) {
	cmd := Root()

	expectedSubcommands := []string{
		"init",
		"apply",
		"image",
		"destroy",
		"upgrade",
		"cost",
		"version",
		"completion",
	}

	// Get subcommand names (first word of Use string)
	subcommands := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		// Use string might include args like "completion [bash|zsh|fish|powershell]"
		// So we extract just the command name
		name := sub.Name()
		subcommands[name] = true
	}

	for _, expected := range expectedSubcommands {
		assert.True(t, subcommands[expected], "Expected subcommand %s not found", expected)
	}
}

func TestRoot_SubcommandCount(t *testing.T) {
	cmd := Root()
	assert.Len(t, cmd.Commands(), 8, "Expected 8 subcommands")
}
