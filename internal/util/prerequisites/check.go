// Package prerequisites provides utilities for checking required client tools.
// This mirrors terraform/client.tf client_prerequisites_check functionality.
package prerequisites

import (
	"fmt"
	"os/exec"
	"strings"
)

// Tool represents a client tool that may be required.
type Tool struct {
	// Name is the binary name to look for in PATH.
	Name string

	// Required indicates if this tool is mandatory.
	Required bool

	// Description explains what the tool is used for.
	Description string

	// InstallURL provides a URL for installation instructions.
	InstallURL string
}

// DefaultTools returns the default set of tools to check.
// kubectl is always required for addon installation.
func DefaultTools() []Tool {
	return []Tool{
		{
			Name:        "kubectl",
			Required:    true,
			Description: "Required for applying Kubernetes manifests and managing addons",
			InstallURL:  "https://kubernetes.io/docs/tasks/tools/",
		},
	}
}

// ImageBuildTools returns additional tools needed for image building.
func ImageBuildTools() []Tool {
	return []Tool{
		{
			Name:        "packer",
			Required:    true,
			Description: "Required for building custom Talos images",
			InstallURL:  "https://developer.hashicorp.com/packer/install",
		},
	}
}

// OptionalTools returns tools that are useful but not required.
func OptionalTools() []Tool {
	return []Tool{
		{
			Name:        "talosctl",
			Required:    false,
			Description: "Useful for debugging and manual cluster operations",
			InstallURL:  "https://www.talos.dev/latest/talos-guides/install/talosctl/",
		},
	}
}

// CheckResult contains the result of checking a single tool.
type CheckResult struct {
	Tool    Tool
	Found   bool
	Path    string
	Version string
}

// CheckResults contains the results of checking multiple tools.
type CheckResults struct {
	Results []CheckResult
	Missing []Tool
}

// HasErrors returns true if any required tools are missing.
func (r *CheckResults) HasErrors() bool {
	for _, tool := range r.Missing {
		if tool.Required {
			return true
		}
	}
	return false
}

// Error returns an error if any required tools are missing.
func (r *CheckResults) Error() error {
	var missing []string
	for _, tool := range r.Missing {
		if tool.Required {
			missing = append(missing, fmt.Sprintf("%s (%s)", tool.Name, tool.InstallURL))
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("missing required tools: %s", strings.Join(missing, ", "))
}

// Check verifies that the specified tools are available.
func Check(tools []Tool) *CheckResults {
	results := &CheckResults{}

	for _, tool := range tools {
		result := CheckResult{Tool: tool}

		path, err := exec.LookPath(tool.Name)
		if err == nil {
			result.Found = true
			result.Path = path
			// Try to get version (best effort)
			result.Version = getToolVersion(tool.Name)
		} else {
			results.Missing = append(results.Missing, tool)
		}

		results.Results = append(results.Results, result)
	}

	return results
}

// CheckDefault checks the default required tools.
func CheckDefault() *CheckResults {
	return Check(DefaultTools())
}

// CheckAll checks all tools (default + optional).
func CheckAll() *CheckResults {
	defaults := DefaultTools()
	optional := OptionalTools()
	all := make([]Tool, 0, len(defaults)+len(optional))
	all = append(all, defaults...)
	all = append(all, optional...)
	return Check(all)
}

// CheckForImageBuild checks tools needed for image building.
func CheckForImageBuild() *CheckResults {
	defaults := DefaultTools()
	imageBuild := ImageBuildTools()
	all := make([]Tool, 0, len(defaults)+len(imageBuild))
	all = append(all, defaults...)
	all = append(all, imageBuild...)
	return Check(all)
}

// getToolVersion attempts to get the version of a tool.
// Returns empty string if version cannot be determined.
func getToolVersion(name string) string {
	// Common version flags to try
	versionFlags := []string{"--version", "version", "-v"}

	for _, flag := range versionFlags {
		// #nosec G204 - name comes from trusted Tool definitions, not user input
		cmd := exec.Command(name, flag)
		output, err := cmd.Output()
		if err == nil {
			// Return first line of output, trimmed
			lines := strings.Split(string(output), "\n")
			if len(lines) > 0 {
				return strings.TrimSpace(lines[0])
			}
		}
	}

	return ""
}
