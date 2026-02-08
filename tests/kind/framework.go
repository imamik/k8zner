//go:build kind

// Package kind provides integration tests against a local Kubernetes cluster.
// Uses kind (Kubernetes in Docker) to test addon installations without cloud infrastructure.
package kind

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

const clusterName = "k8zner-test"

// Framework manages the kind cluster lifecycle for tests.
type Framework struct {
	mu             sync.RWMutex
	kubeconfig     []byte
	kubeconfigPath string
	clusterReady   bool
	installed      map[string]bool
}

// NewFramework creates a test framework instance.
func NewFramework() *Framework {
	return &Framework{
		installed: make(map[string]bool),
	}
}

// Setup creates the kind cluster or reuses an existing one.
func (f *Framework) Setup() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.checkPrerequisites(); err != nil {
		return err
	}

	if f.clusterExists() {
		fmt.Printf("Using existing kind cluster: %s\n", clusterName)
		f.clusterReady = true
		return f.loadKubeconfig()
	}

	fmt.Printf("Creating kind cluster: %s\n", clusterName)
	if err := f.createCluster(); err != nil {
		return fmt.Errorf("create cluster: %w", err)
	}

	f.clusterReady = true
	return f.loadKubeconfig()
}

// Teardown deletes the kind cluster unless KEEP_KIND_CLUSTER is set.
func (f *Framework) Teardown() {
	f.mu.Lock()
	defer f.mu.Unlock()

	if os.Getenv("KEEP_KIND_CLUSTER") != "" {
		fmt.Printf("\nCluster preserved: %s\n", clusterName)
		fmt.Printf("  Kubeconfig: %s\n", f.kubeconfigPath)
		fmt.Printf("  Delete: kind delete cluster --name %s\n", clusterName)
		return
	}

	if f.kubeconfigPath != "" {
		_ = os.Remove(f.kubeconfigPath)
	}

	if f.clusterReady {
		fmt.Printf("Deleting kind cluster: %s\n", clusterName)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_ = exec.CommandContext(ctx, "kind", "delete", "cluster", "--name", clusterName).Run()
	}
}

func (f *Framework) checkPrerequisites() error {
	if _, err := exec.LookPath("kind"); err != nil {
		return fmt.Errorf("kind not found: install with 'go install sigs.k8s.io/kind@latest'")
	}
	if _, err := exec.LookPath("kubectl"); err != nil {
		return fmt.Errorf("kubectl not found")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		return fmt.Errorf("docker not running")
	}
	return nil
}

func (f *Framework) clusterExists() bool {
	output, err := exec.Command("kind", "get", "clusters").Output()
	return err == nil && strings.Contains(string(output), clusterName)
}

func (f *Framework) createCluster() error {
	// KIND_WORKERS controls number of worker nodes (default: 2, set to 0 for single-node CI)
	numWorkers := 2
	if w := os.Getenv("KIND_WORKERS"); w != "" {
		if n, err := strconv.Atoi(w); err == nil && n >= 0 {
			numWorkers = n
		}
	}

	var config strings.Builder
	config.WriteString(`kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 80
    hostPort: 8080
  - containerPort: 443
    hostPort: 8443
`)
	for i := 0; i < numWorkers; i++ {
		config.WriteString("- role: worker\n")
	}
	configStr := config.String()
	configFile, err := os.CreateTemp("", "kind-config-*.yaml")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(configFile.Name()) }()

	if _, err := configFile.WriteString(configStr); err != nil {
		return err
	}
	if err := configFile.Close(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// #nosec G204 -- test code with controlled command arguments
	cmd := exec.CommandContext(ctx, "kind", "create", "cluster",
		"--name", clusterName,
		"--config", configFile.Name(),
		"--wait", "120s",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (f *Framework) loadKubeconfig() error {
	output, err := exec.Command("kind", "get", "kubeconfig", "--name", clusterName).Output()
	if err != nil {
		return fmt.Errorf("get kubeconfig: %w", err)
	}
	f.kubeconfig = output

	kubeconfigFile, err := os.CreateTemp("", "kind-kubeconfig-*.yaml")
	if err != nil {
		return err
	}
	if _, err := kubeconfigFile.Write(output); err != nil {
		return err
	}
	if err := kubeconfigFile.Close(); err != nil {
		return err
	}
	f.kubeconfigPath = kubeconfigFile.Name()
	return nil
}

// Kubeconfig returns the kubeconfig bytes for the cluster.
func (f *Framework) Kubeconfig() []byte {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.kubeconfig
}

// KubeconfigPath returns the path to the kubeconfig file.
func (f *Framework) KubeconfigPath() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.kubeconfigPath
}

// MarkInstalled records that a component has been installed.
func (f *Framework) MarkInstalled(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installed[name] = true
}

// IsInstalled checks if a component was already installed.
func (f *Framework) IsInstalled(name string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.installed[name]
}
