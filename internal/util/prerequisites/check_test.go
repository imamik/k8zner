package prerequisites

import (
	"testing"
)

func TestCheck(t *testing.T) {
	// Test with a tool that definitely exists - try multiple common tools
	// because different environments have different tools available
	possibleTools := []string{"go", "bash", "sh", "ls", "cat"}

	var foundTool string
	for _, tool := range possibleTools {
		results := Check([]Tool{{Name: tool, Required: false}})
		if len(results.Results) > 0 && results.Results[0].Found {
			foundTool = tool
			break
		}
	}

	if foundTool == "" {
		t.Skip("no common tools found in PATH, skipping test")
	}

	tools := []Tool{
		{
			Name:        foundTool,
			Required:    true,
			Description: "Test tool",
			InstallURL:  "https://example.com",
		},
	}

	results := Check(tools)

	if len(results.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results.Results))
	}

	if !results.Results[0].Found {
		t.Errorf("expected %s to be found", foundTool)
	}

	if results.Results[0].Path == "" {
		t.Errorf("expected path to be set")
	}

	if results.HasErrors() {
		t.Errorf("expected no errors")
	}
}

func TestCheckMissingTool(t *testing.T) {
	tools := []Tool{
		{
			Name:        "nonexistent-tool-xyz123",
			Required:    true,
			Description: "A tool that does not exist",
			InstallURL:  "https://example.com",
		},
	}

	results := Check(tools)

	if len(results.Missing) != 1 {
		t.Errorf("expected 1 missing tool, got %d", len(results.Missing))
	}

	if !results.HasErrors() {
		t.Errorf("expected HasErrors to be true")
	}

	err := results.Error()
	if err == nil {
		t.Errorf("expected Error to return an error")
	}
}

func TestCheckOptionalMissing(t *testing.T) {
	tools := []Tool{
		{
			Name:        "nonexistent-tool-xyz123",
			Required:    false, // optional
			Description: "An optional tool that does not exist",
			InstallURL:  "https://example.com",
		},
	}

	results := Check(tools)

	if len(results.Missing) != 1 {
		t.Errorf("expected 1 missing tool, got %d", len(results.Missing))
	}

	// Optional tools don't cause errors
	if results.HasErrors() {
		t.Errorf("expected HasErrors to be false for optional tools")
	}

	err := results.Error()
	if err != nil {
		t.Errorf("expected Error to return nil for optional tools, got %v", err)
	}
}

func TestDefaultTools(t *testing.T) {
	tools := DefaultTools()

	// The main binary is self-contained and requires no external CLI tools at runtime
	if len(tools) != 0 {
		t.Errorf("expected DefaultTools to return empty list, got %d tools", len(tools))
	}
}

func TestImageBuildTools(t *testing.T) {
	tools := ImageBuildTools()

	if len(tools) == 0 {
		t.Error("expected ImageBuildTools to return at least one tool")
	}

	// packer should be in image build tools
	found := false
	for _, tool := range tools {
		if tool.Name == "packer" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected packer in ImageBuildTools")
	}
}

func TestOptionalTools(t *testing.T) {
	tools := OptionalTools()

	if len(tools) == 0 {
		t.Error("expected OptionalTools to return at least one tool")
	}

	// All optional tools should have Required = false
	for _, tool := range tools {
		if tool.Required {
			t.Errorf("optional tool %s should have Required = false", tool.Name)
		}
	}

	// kubectl should be in optional tools
	foundKubectl := false
	foundTalosctl := false
	for _, tool := range tools {
		if tool.Name == "kubectl" {
			foundKubectl = true
		}
		if tool.Name == "talosctl" {
			foundTalosctl = true
		}
	}

	if !foundKubectl {
		t.Error("expected kubectl in OptionalTools")
	}
	if !foundTalosctl {
		t.Error("expected talosctl in OptionalTools")
	}
}
