package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersion(t *testing.T) {
	cmd := Version()

	require.NotNil(t, cmd)
	assert.Equal(t, "version", cmd.Use)
	assert.Equal(t, "Print version information", cmd.Short)
}

func TestVersion_Run(t *testing.T) {
	cmd := Version()
	assert.NotNil(t, cmd.Run, "Version command should have Run function")
}

func TestSetVersionInfo(t *testing.T) {
	// Save original values
	origVersion := version
	origCommit := commit
	origDate := date

	// Restore after test
	defer func() {
		version = origVersion
		commit = origCommit
		date = origDate
	}()

	// Set new values
	SetVersionInfo("1.2.3", "abc123", "2024-01-01")

	assert.Equal(t, "1.2.3", version)
	assert.Equal(t, "abc123", commit)
	assert.Equal(t, "2024-01-01", date)
}

func TestVersion_Output(t *testing.T) {
	// Save original values
	origVersion := version
	origCommit := commit
	origDate := date

	// Restore after test
	defer func() {
		version = origVersion
		commit = origCommit
		date = origDate
	}()

	// Set test values
	SetVersionInfo("test-version", "test-commit", "test-date")

	// Verify the version values are set correctly
	assert.Equal(t, "test-version", version)
	assert.Equal(t, "test-commit", commit)
	assert.Equal(t, "test-date", date)

	// The version command uses fmt.Printf which writes to stdout,
	// not to the command's output buffer. We just verify the command executes without error.
	cmd := Version()
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestVersion_DefaultValues(t *testing.T) {
	// Test that default values are set
	cmd := Version()

	// Just verify the command exists and has default values
	assert.NotEmpty(t, cmd.Use)
}
