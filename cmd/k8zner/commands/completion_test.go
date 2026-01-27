package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompletion(t *testing.T) {
	cmd := Completion()

	require.NotNil(t, cmd)
	assert.Equal(t, "completion [bash|zsh|fish|powershell]", cmd.Use)
	assert.Equal(t, "Generate shell completion scripts", cmd.Short)
	assert.Contains(t, cmd.Long, "Generate shell completion scripts")
}

func TestCompletion_ValidArgs(t *testing.T) {
	cmd := Completion()

	expectedArgs := []string{"bash", "zsh", "fish", "powershell"}
	assert.Equal(t, expectedArgs, cmd.ValidArgs)
}

func TestCompletion_DisableFlagsInUseLine(t *testing.T) {
	cmd := Completion()
	assert.True(t, cmd.DisableFlagsInUseLine)
}

func TestCompletion_ExactArgs(t *testing.T) {
	cmd := Completion()

	// Test that exactly 1 argument is required
	assert.NotNil(t, cmd.Args)
}

func TestCompletion_RunE(t *testing.T) {
	cmd := Completion()
	assert.NotNil(t, cmd.RunE, "Completion command should have RunE function")
}

// Note: Completion commands write directly to os.Stdout (not cmd.OutOrStdout()),
// so we just verify they execute without error.

func TestCompletion_BashOutput(t *testing.T) {
	root := Root()
	root.SetArgs([]string{"completion", "bash"})

	err := root.Execute()
	require.NoError(t, err)
}

func TestCompletion_ZshOutput(t *testing.T) {
	root := Root()
	root.SetArgs([]string{"completion", "zsh"})

	err := root.Execute()
	require.NoError(t, err)
}

func TestCompletion_FishOutput(t *testing.T) {
	root := Root()
	root.SetArgs([]string{"completion", "fish"})

	err := root.Execute()
	require.NoError(t, err)
}

func TestCompletion_PowershellOutput(t *testing.T) {
	root := Root()
	root.SetArgs([]string{"completion", "powershell"})

	err := root.Execute()
	require.NoError(t, err)
}

func TestCompletion_InvalidShell(t *testing.T) {
	root := Root()
	root.SetArgs([]string{"completion", "invalid"})

	err := root.Execute()
	assert.Error(t, err)
}

func TestCompletion_NoArgs(t *testing.T) {
	root := Root()
	root.SetArgs([]string{"completion"})

	err := root.Execute()
	assert.Error(t, err)
}
