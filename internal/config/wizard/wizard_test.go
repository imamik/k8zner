package wizard

import (
	"os"
	"strings"
	"testing"
)

func TestBuildConfig(t *testing.T) {
	result := &WizardResult{
		ClusterName:       "my-cluster",
		Location:          "nbg1",
		SSHKeys:           []string{"my-key"},
		Architecture:      ArchX86,
		ServerCategory:    CategoryShared,
		ControlPlaneType:  "cpx21",
		ControlPlaneCount: 3,
		AddWorkers:        true,
		WorkerType:        "cpx21",
		WorkerCount:       2,
		CNIChoice:         CNICilium,
		EnabledAddons:     []string{"ccm", "csi", "metrics_server"},
		TalosVersion:      "v1.9.0",
		KubernetesVersion: "v1.32.0",
	}

	cfg := BuildConfig(result)

	// Verify basic fields
	if cfg.ClusterName != "my-cluster" {
		t.Errorf("ClusterName = %q, want %q", cfg.ClusterName, "my-cluster")
	}
	if cfg.Location != "nbg1" {
		t.Errorf("Location = %q, want %q", cfg.Location, "nbg1")
	}
	if len(cfg.SSHKeys) != 1 || cfg.SSHKeys[0] != "my-key" {
		t.Errorf("SSHKeys = %v, want [my-key]", cfg.SSHKeys)
	}

	// Verify control plane
	if len(cfg.ControlPlane.NodePools) != 1 {
		t.Fatalf("ControlPlane.NodePools length = %d, want 1", len(cfg.ControlPlane.NodePools))
	}
	cp := cfg.ControlPlane.NodePools[0]
	if cp.Name != "control-plane" {
		t.Errorf("ControlPlane name = %q, want %q", cp.Name, "control-plane")
	}
	if cp.ServerType != "cpx21" {
		t.Errorf("ControlPlane type = %q, want %q", cp.ServerType, "cpx21")
	}
	if cp.Count != 3 {
		t.Errorf("ControlPlane count = %d, want 3", cp.Count)
	}

	// Verify workers
	if len(cfg.Workers) != 1 {
		t.Fatalf("Workers length = %d, want 1", len(cfg.Workers))
	}
	w := cfg.Workers[0]
	if w.Name != "workers" {
		t.Errorf("Worker name = %q, want %q", w.Name, "workers")
	}
	if w.ServerType != "cpx21" {
		t.Errorf("Worker type = %q, want %q", w.ServerType, "cpx21")
	}
	if w.Count != 2 {
		t.Errorf("Worker count = %d, want 2", w.Count)
	}

	// Verify addons
	if !cfg.Addons.Cilium.Enabled {
		t.Error("Cilium should be enabled")
	}
	if !cfg.Addons.CCM.Enabled {
		t.Error("CCM should be enabled")
	}
	if !cfg.Addons.CSI.Enabled {
		t.Error("CSI should be enabled")
	}
	if !cfg.Addons.MetricsServer.Enabled {
		t.Error("MetricsServer should be enabled")
	}

	// Verify versions
	if cfg.Talos.Version != "v1.9.0" {
		t.Errorf("Talos.Version = %q, want %q", cfg.Talos.Version, "v1.9.0")
	}
	if cfg.Kubernetes.Version != "v1.32.0" {
		t.Errorf("Kubernetes.Version = %q, want %q", cfg.Kubernetes.Version, "v1.32.0")
	}
}

func TestBuildConfigWithAdvancedOptions(t *testing.T) {
	result := &WizardResult{
		ClusterName:       "advanced-cluster",
		Location:          "fsn1",
		SSHKeys:           []string{"key1", "key2"},
		Architecture:      ArchX86,
		ServerCategory:    CategoryShared,
		ControlPlaneType:  "cpx31",
		ControlPlaneCount: 5,
		AddWorkers:        false,
		CNIChoice:         CNICilium,
		EnabledAddons:     []string{},
		TalosVersion:      "v1.8.3",
		KubernetesVersion: "v1.31.0",
		AdvancedOptions: &AdvancedOptions{
			NetworkCIDR:          "10.10.0.0/16",
			PodCIDR:              "10.200.0.0/16",
			ServiceCIDR:          "10.100.0.0/12",
			DiskEncryption:       true,
			ClusterAccess:        "private",
			CiliumEncryption:     true,
			CiliumEncryptionType: "wireguard",
			HubbleEnabled:        true,
			GatewayAPIEnabled:    true,
		},
	}

	cfg := BuildConfig(result)

	// Verify advanced network options
	if cfg.Network.IPv4CIDR != "10.10.0.0/16" {
		t.Errorf("Network.IPv4CIDR = %q, want %q", cfg.Network.IPv4CIDR, "10.10.0.0/16")
	}
	if cfg.Network.PodIPv4CIDR != "10.200.0.0/16" {
		t.Errorf("Network.PodIPv4CIDR = %q, want %q", cfg.Network.PodIPv4CIDR, "10.200.0.0/16")
	}
	if cfg.Network.ServiceIPv4CIDR != "10.100.0.0/12" {
		t.Errorf("Network.ServiceIPv4CIDR = %q, want %q", cfg.Network.ServiceIPv4CIDR, "10.100.0.0/12")
	}

	// Verify security options
	if cfg.Talos.Machine.StateEncryption == nil || !*cfg.Talos.Machine.StateEncryption {
		t.Error("StateEncryption should be true")
	}
	if cfg.Talos.Machine.EphemeralEncryption == nil || !*cfg.Talos.Machine.EphemeralEncryption {
		t.Error("EphemeralEncryption should be true")
	}
	if cfg.ClusterAccess != "private" {
		t.Errorf("ClusterAccess = %q, want %q", cfg.ClusterAccess, "private")
	}

	// Verify Cilium options
	if !cfg.Addons.Cilium.EncryptionEnabled {
		t.Error("Cilium encryption should be enabled")
	}
	if cfg.Addons.Cilium.EncryptionType != "wireguard" {
		t.Errorf("Cilium.EncryptionType = %q, want %q", cfg.Addons.Cilium.EncryptionType, "wireguard")
	}
	if !cfg.Addons.Cilium.HubbleEnabled {
		t.Error("Hubble should be enabled")
	}
	if !cfg.Addons.Cilium.HubbleRelayEnabled {
		t.Error("HubbleRelay should be enabled")
	}
	if !cfg.Addons.Cilium.GatewayAPIEnabled {
		t.Error("GatewayAPI should be enabled")
	}

	// Verify no workers
	if len(cfg.Workers) != 0 {
		t.Errorf("Workers length = %d, want 0", len(cfg.Workers))
	}

	// Verify scheduling on control plane is enabled when no workers
	if cfg.Kubernetes.AllowSchedulingOnCP == nil || !*cfg.Kubernetes.AllowSchedulingOnCP {
		t.Error("AllowSchedulingOnCP should be true when no workers are configured")
	}
}

func TestBuildConfigWithWorkersDisablesSchedulingOnCP(t *testing.T) {
	result := &WizardResult{
		ClusterName:       "test-cluster",
		Location:          "nbg1",
		SSHKeys:           []string{"my-key"},
		Architecture:      ArchX86,
		ServerCategory:    CategoryShared,
		ControlPlaneType:  "cpx21",
		ControlPlaneCount: 3,
		AddWorkers:        true,
		WorkerType:        "cpx31",
		WorkerCount:       2,
		CNIChoice:         CNICilium,
		EnabledAddons:     []string{},
		TalosVersion:      "v1.9.0",
		KubernetesVersion: "v1.32.0",
	}

	cfg := BuildConfig(result)

	// AllowSchedulingOnCP should not be set when workers are present
	if cfg.Kubernetes.AllowSchedulingOnCP != nil {
		t.Error("AllowSchedulingOnCP should not be set when workers are configured")
	}
}

func TestWriteConfig(t *testing.T) {
	result := &WizardResult{
		ClusterName:       "test-cluster",
		Location:          "nbg1",
		SSHKeys:           []string{"my-key"},
		Architecture:      ArchX86,
		ServerCategory:    CategoryShared,
		ControlPlaneType:  "cpx21",
		ControlPlaneCount: 3,
		AddWorkers:        true,
		WorkerType:        "cpx21",
		WorkerCount:       2,
		CNIChoice:         CNICilium,
		EnabledAddons:     []string{"ccm", "csi", "metrics_server"},
		TalosVersion:      "v1.9.0",
		KubernetesVersion: "v1.32.0",
	}

	cfg := BuildConfig(result)

	// Write to temp file
	tmpFile, err := os.CreateTemp("", "test-cluster-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	if err := WriteConfig(cfg, tmpFile.Name(), false); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

	// Read back and verify
	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	// Check for expected content
	s := string(content)
	if !containsString(s, "cluster_name: test-cluster") {
		t.Error("Missing cluster_name in output")
	}
	if !containsString(s, "location: nbg1") {
		t.Error("Missing location in output")
	}
	if !containsString(s, "# k8zner cluster configuration") {
		t.Error("Missing header comment in output")
	}
	// Verify the header contains the actual output path, not hardcoded "cluster.yaml"
	if !containsString(s, tmpFile.Name()) {
		t.Errorf("Header should contain output path %q", tmpFile.Name())
	}

	t.Logf("Generated config:\n%s", s)
}

func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestParseSSHKeys(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"key1", []string{"key1"}},
		{"key1, key2", []string{"key1", "key2"}},
		{"key1,key2,key3", []string{"key1", "key2", "key3"}},
		{"  key1  ,  key2  ", []string{"key1", "key2"}},
		{"key1,,key2", []string{"key1", "key2"}},
		{"", []string{}},
	}

	for _, tt := range tests {
		result := parseSSHKeys(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("parseSSHKeys(%q) = %v, want %v", tt.input, result, tt.expected)
			continue
		}
		for i, v := range result {
			if v != tt.expected[i] {
				t.Errorf("parseSSHKeys(%q)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
			}
		}
	}
}

func TestValidateClusterName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"my-cluster", false},
		{"cluster1", false},
		{"a", false},
		{"my-production-cluster-2024", false},
		{"", true},               // empty
		{"-invalid", true},       // starts with hyphen
		{"invalid-", true},       // ends with hyphen
		{"UPPERCASE", true},      // uppercase
		{"has_underscore", true}, // underscore
		{"has.dot", true},        // dot
		{"this-is-a-very-long-cluster-name-that-exceeds-limit", true}, // too long
	}

	for _, tt := range tests {
		err := validateClusterName(tt.name)
		if (err != nil) != tt.wantErr {
			t.Errorf("validateClusterName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
	}
}

func TestValidateCIDR(t *testing.T) {
	tests := []struct {
		cidr    string
		wantErr bool
	}{
		// Valid CIDRs
		{"10.0.0.0/16", false},
		{"192.168.1.0/24", false},
		{"10.244.0.0/16", false},
		{"10.96.0.0/12", false},
		{"172.16.0.0/12", false},
		{"0.0.0.0/0", false},

		// Invalid CIDRs
		{"", true},                   // empty
		{"10.0.0.0", true},           // missing mask
		{"invalid", true},            // invalid format
		{"10.0.0.0/", true},          // missing mask number
		{"999.999.999.999/24", true}, // invalid IP octets
		{"10.0.0.0/33", true},        // mask too large
		{"10.0.0.0/-1", true},        // negative mask
		{"10.0.0.256/24", true},      // octet out of range
		{"10.0.0/24", true},          // incomplete IP
		{"10.0.0.0.0/24", true},      // too many octets
		{"10.0.0.0/24/extra", true},  // extra slash
		{"  10.0.0.0/16  ", true},    // whitespace
	}

	for _, tt := range tests {
		err := validateCIDR(tt.cidr)
		if (err != nil) != tt.wantErr {
			t.Errorf("validateCIDR(%q) error = %v, wantErr %v", tt.cidr, err, tt.wantErr)
		}
	}
}

func TestContainsAddon(t *testing.T) {
	addons := []string{"cilium", "ccm", "csi"}

	tests := []struct {
		addon string
		want  bool
	}{
		{"cilium", true},
		{"ccm", true},
		{"csi", true},
		{"metrics_server", false},
		{"", false},
	}

	for _, tt := range tests {
		got := containsAddon(addons, tt.addon)
		if got != tt.want {
			t.Errorf("containsAddon(%v, %q) = %v, want %v", addons, tt.addon, got, tt.want)
		}
	}
}

func TestFileExists(t *testing.T) {
	// Test with existing file
	tmpFile, err := os.CreateTemp("", "test-exists-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	if !FileExists(tmpFile.Name()) {
		t.Errorf("FileExists(%q) = false, want true", tmpFile.Name())
	}

	// Test with non-existing file
	if FileExists("/nonexistent/path/file.txt") {
		t.Error("FileExists(/nonexistent/path/file.txt) = true, want false")
	}
}

func TestLocationsToOptions(t *testing.T) {
	opts := LocationsToOptions()
	if len(opts) != len(Locations) {
		t.Errorf("LocationsToOptions() returned %d options, want %d", len(opts), len(Locations))
	}
}

func TestServerTypesToOptions(t *testing.T) {
	opts := ServerTypesToOptions(ControlPlaneServerTypes)
	if len(opts) != len(ControlPlaneServerTypes) {
		t.Errorf("ServerTypesToOptions() returned %d options, want %d", len(opts), len(ControlPlaneServerTypes))
	}
}

func TestVersionsToOptions(t *testing.T) {
	opts := VersionsToOptions(TalosVersions)
	if len(opts) != len(TalosVersions) {
		t.Errorf("VersionsToOptions() returned %d options, want %d", len(opts), len(TalosVersions))
	}
}

func TestBuildAddonsConfigAllTypes(t *testing.T) {
	// Test all addon types with Cilium CNI and Nginx ingress
	allAddons := []string{"ccm", "csi", "metrics_server", "cert_manager", "longhorn", "argocd"}
	addons := buildAddonsConfig(allAddons, CNICilium, IngressNginx)

	if !addons.Cilium.Enabled {
		t.Error("Cilium should be enabled when CNI is cilium")
	}
	if !addons.CCM.Enabled {
		t.Error("CCM should be enabled")
	}
	if !addons.CSI.Enabled {
		t.Error("CSI should be enabled")
	}
	if !addons.MetricsServer.Enabled {
		t.Error("MetricsServer should be enabled")
	}
	if !addons.CertManager.Enabled {
		t.Error("CertManager should be enabled")
	}
	if !addons.IngressNginx.Enabled {
		t.Error("IngressNginx should be enabled when IngressNginx is selected")
	}
	if addons.Traefik.Enabled {
		t.Error("Traefik should not be enabled when IngressNginx is selected")
	}
	if !addons.Longhorn.Enabled {
		t.Error("Longhorn should be enabled")
	}
	if !addons.ArgoCD.Enabled {
		t.Error("ArgoCD should be enabled")
	}

	// Test with Traefik ingress controller
	traefikAddons := buildAddonsConfig([]string{"ccm"}, CNICilium, IngressTraefik)
	if !traefikAddons.Traefik.Enabled {
		t.Error("Traefik should be enabled when IngressTraefik is selected")
	}
	if traefikAddons.IngressNginx.Enabled {
		t.Error("IngressNginx should not be enabled when IngressTraefik is selected")
	}

	// Test with no ingress controller
	noIngressAddons := buildAddonsConfig([]string{"ccm"}, CNICilium, IngressNone)
	if noIngressAddons.IngressNginx.Enabled {
		t.Error("IngressNginx should not be enabled when IngressNone is selected")
	}
	if noIngressAddons.Traefik.Enabled {
		t.Error("Traefik should not be enabled when IngressNone is selected")
	}

	// Test with empty addons and Talos native CNI
	emptyAddons := buildAddonsConfig([]string{}, CNITalosNative, IngressNone)
	if emptyAddons.Cilium.Enabled {
		t.Error("Cilium should not be enabled with Talos native CNI")
	}

	// Test with unknown addon (should not panic)
	unknownAddons := buildAddonsConfig([]string{"unknown_addon"}, CNINone, IngressNone)
	if unknownAddons.Cilium.Enabled {
		t.Error("Cilium should not be enabled with CNI none")
	}
}

func TestFilterServerTypes(t *testing.T) {
	// Test filtering x86 shared
	x86Shared := FilterServerTypes(ArchX86, CategoryShared)
	if len(x86Shared) == 0 {
		t.Error("Expected x86 shared server types")
	}
	for _, st := range x86Shared {
		if st.Architecture != ArchX86 || st.Category != CategoryShared {
			t.Errorf("Got server type with wrong arch/category: %s (%s/%s)", st.Value, st.Architecture, st.Category)
		}
	}

	// Test filtering ARM shared
	armShared := FilterServerTypes(ArchARM, CategoryShared)
	if len(armShared) == 0 {
		t.Error("Expected ARM shared server types")
	}
	for _, st := range armShared {
		if st.Architecture != ArchARM || st.Category != CategoryShared {
			t.Errorf("Got server type with wrong arch/category: %s (%s/%s)", st.Value, st.Architecture, st.Category)
		}
	}

	// Test filtering x86 dedicated
	x86Dedicated := FilterServerTypes(ArchX86, CategoryDedicated)
	if len(x86Dedicated) == 0 {
		t.Error("Expected x86 dedicated server types")
	}
}
