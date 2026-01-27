package wizard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Additional tests for options.go that complement wizard_test.go
// Note: TestFilterServerTypes, TestLocationsToOptions, TestServerTypesToOptions,
// and TestVersionsToOptions are already defined in wizard_test.go

func TestFilterServerTypes_X86Shared(t *testing.T) {
	filtered := FilterServerTypes(ArchX86, CategoryShared)

	require.NotEmpty(t, filtered)
	for _, st := range filtered {
		assert.Equal(t, ArchX86, st.Architecture)
		assert.Equal(t, CategoryShared, st.Category)
	}

	// Check specific types are included
	values := extractValuesFromTypes(filtered)
	assert.Contains(t, values, "cpx11")
	assert.Contains(t, values, "cpx21")
	assert.Contains(t, values, "cpx31")
}

func TestFilterServerTypes_X86Dedicated(t *testing.T) {
	filtered := FilterServerTypes(ArchX86, CategoryDedicated)

	require.NotEmpty(t, filtered)
	for _, st := range filtered {
		assert.Equal(t, ArchX86, st.Architecture)
		assert.Equal(t, CategoryDedicated, st.Category)
	}

	// Check specific types are included
	values := extractValuesFromTypes(filtered)
	assert.Contains(t, values, "ccx13")
	assert.Contains(t, values, "ccx23")
	assert.Contains(t, values, "ccx33")
}

func TestFilterServerTypes_X86CostOptimized(t *testing.T) {
	filtered := FilterServerTypes(ArchX86, CategoryCostOptimized)

	require.NotEmpty(t, filtered)
	for _, st := range filtered {
		assert.Equal(t, ArchX86, st.Architecture)
		assert.Equal(t, CategoryCostOptimized, st.Category)
	}

	// Check specific types are included
	values := extractValuesFromTypes(filtered)
	assert.Contains(t, values, "cx22")
	assert.Contains(t, values, "cx32")
}

func TestFilterServerTypes_ARMShared(t *testing.T) {
	filtered := FilterServerTypes(ArchARM, CategoryShared)

	require.NotEmpty(t, filtered)
	for _, st := range filtered {
		assert.Equal(t, ArchARM, st.Architecture)
		assert.Equal(t, CategoryShared, st.Category)
	}

	// Check specific types are included
	values := extractValuesFromTypes(filtered)
	assert.Contains(t, values, "cax11")
	assert.Contains(t, values, "cax21")
	assert.Contains(t, values, "cax31")
	assert.Contains(t, values, "cax41")
}

func TestFilterServerTypes_NoMatch(t *testing.T) {
	// ARM doesn't have dedicated or cost-optimized options
	filtered := FilterServerTypes(ArchARM, CategoryDedicated)
	assert.Empty(t, filtered)

	filtered = FilterServerTypes(ArchARM, CategoryCostOptimized)
	assert.Empty(t, filtered)
}

func TestFilterServerTypes_InvalidArchitecture(t *testing.T) {
	filtered := FilterServerTypes("invalid", CategoryShared)
	assert.Empty(t, filtered)
}

func TestFilterServerTypes_InvalidCategory(t *testing.T) {
	filtered := FilterServerTypes(ArchX86, "invalid")
	assert.Empty(t, filtered)
}

func TestServerTypesToOptions_Empty(t *testing.T) {
	opts := ServerTypesToOptions([]ServerTypeOption{})
	assert.Empty(t, opts)
}

func TestVersionsToOptions_Empty(t *testing.T) {
	opts := VersionsToOptions([]VersionOption{})
	assert.Empty(t, opts)
}

func TestAllServerTypes_Complete(t *testing.T) {
	// Verify all server types have required fields
	for _, st := range AllServerTypes {
		assert.NotEmpty(t, st.Value, "Value should not be empty")
		assert.NotEmpty(t, st.Label, "Label should not be empty")
		assert.NotEmpty(t, st.Description, "Description should not be empty")
		assert.NotEmpty(t, st.Architecture, "Architecture should not be empty")
		assert.NotEmpty(t, st.Category, "Category should not be empty")
	}
}

func TestLocations_Complete(t *testing.T) {
	for _, loc := range Locations {
		assert.NotEmpty(t, loc.Value, "Value should not be empty")
		assert.NotEmpty(t, loc.Label, "Label should not be empty")
		assert.NotEmpty(t, loc.Description, "Description should not be empty")
	}
}

func TestEULocations_Subset(t *testing.T) {
	// EU locations should be a subset of all locations
	allValues := make(map[string]bool)
	for _, loc := range Locations {
		allValues[loc.Value] = true
	}

	for _, loc := range EULocations {
		assert.True(t, allValues[loc.Value], "EU location %s should be in all locations", loc.Value)
	}
}

func TestTalosVersions_NotEmpty(t *testing.T) {
	assert.NotEmpty(t, TalosVersions)

	for _, v := range TalosVersions {
		assert.NotEmpty(t, v.Value)
		assert.NotEmpty(t, v.Label)
	}
}

func TestKubernetesVersions_NotEmpty(t *testing.T) {
	assert.NotEmpty(t, KubernetesVersions)

	for _, v := range KubernetesVersions {
		assert.NotEmpty(t, v.Value)
		assert.NotEmpty(t, v.Label)
	}
}

func TestBasicAddons_Complete(t *testing.T) {
	for _, addon := range BasicAddons {
		assert.NotEmpty(t, addon.Key, "Key should not be empty")
		assert.NotEmpty(t, addon.Label, "Label should not be empty")
		assert.NotEmpty(t, addon.Description, "Description should not be empty")
	}
}

func TestConstants(t *testing.T) {
	// Architecture constants
	assert.Equal(t, "x86", ArchX86)
	assert.Equal(t, "arm", ArchARM)

	// Category constants
	assert.Equal(t, "shared", CategoryShared)
	assert.Equal(t, "dedicated", CategoryDedicated)
	assert.Equal(t, "cost-optimized", CategoryCostOptimized)

	// CNI constants
	assert.Equal(t, "cilium", CNICilium)
	assert.Equal(t, "talos", CNITalosNative)
	assert.Equal(t, "none", CNINone)

	// Ingress constants
	assert.Equal(t, "none", IngressNone)
	assert.Equal(t, "nginx", IngressNginx)
	assert.Equal(t, "traefik", IngressTraefik)
}

// extractValuesFromTypes is a helper to get values from ServerTypeOption slice
func extractValuesFromTypes(types []ServerTypeOption) []string {
	values := make([]string, len(types))
	for i, t := range types {
		values[i] = t.Value
	}
	return values
}
