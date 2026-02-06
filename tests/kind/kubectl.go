//go:build kind

package kind

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// Kubectl executes a kubectl command and returns output.
func (f *Framework) Kubectl(args ...string) (string, error) {
	fullArgs := append([]string{"--kubeconfig", f.KubeconfigPath()}, args...)
	// #nosec G204 -- test code with controlled command arguments
	cmd := exec.Command("kubectl", fullArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v: %s", err, stderr.String())
	}
	return stdout.String(), nil
}

// KubectlMust executes kubectl and fails the test on error.
func (f *Framework) KubectlMust(t *testing.T, args ...string) string {
	t.Helper()
	output, err := f.Kubectl(args...)
	if err != nil {
		t.Fatalf("kubectl %s: %v", strings.Join(args, " "), err)
	}
	return output
}

// KubectlApply applies a manifest string.
func (f *Framework) KubectlApply(t *testing.T, manifest string) {
	t.Helper()

	// #nosec G204 -- test code with controlled command arguments
	cmd := exec.Command("kubectl", "--kubeconfig", f.KubeconfigPath(), "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("kubectl apply: %v\n%s", err, stderr.String())
	}
}

// KubectlDelete deletes a resource (non-fatal, returns error).
func (f *Framework) KubectlDelete(namespace, kind, name string) error {
	args := []string{"delete", kind, name, "--ignore-not-found"}
	if namespace != "" {
		args = append([]string{"-n", namespace}, args...)
	}
	_, err := f.Kubectl(args...)
	return err
}

// ResourceExists checks if a resource exists.
func (f *Framework) ResourceExists(kind, namespace, name string) bool {
	args := []string{"get", kind}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	args = append(args, name)
	_, err := f.Kubectl(args...)
	return err == nil
}

// NamespaceExists checks if a namespace exists.
func (f *Framework) NamespaceExists(name string) bool {
	return f.ResourceExists("namespace", "", name)
}
