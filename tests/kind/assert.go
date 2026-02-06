//go:build kind

package kind

import (
	"strings"
	"testing"
)

// AssertCRDsExist verifies all CRDs in the list exist.
func (f *Framework) AssertCRDsExist(t *testing.T, crds []string) {
	t.Helper()
	for _, crd := range crds {
		if !f.ResourceExists("crd", "", crd) {
			t.Errorf("CRD %s not found", crd)
		}
	}
}

// AssertDeploymentReady verifies a deployment has all replicas ready.
func (f *Framework) AssertDeploymentReady(t *testing.T, namespace, name string) {
	t.Helper()
	output, err := f.Kubectl("-n", namespace, "get", "deployment", name,
		"-o", "jsonpath={.status.readyReplicas}/{.status.replicas}")
	if err != nil {
		t.Errorf("deployment %s/%s: %v", namespace, name, err)
		return
	}
	parts := strings.Split(output, "/")
	if len(parts) != 2 || parts[0] == "" || parts[0] == "0" || parts[0] != parts[1] {
		t.Errorf("deployment %s/%s not ready: %s", namespace, name, output)
	}
}

// AssertPodRunning verifies at least one pod with label is Running.
func (f *Framework) AssertPodRunning(t *testing.T, namespace, label string) {
	t.Helper()
	output, err := f.Kubectl("-n", namespace, "get", "pods", "-l", label,
		"-o", "jsonpath={.items[*].status.phase}")
	if err != nil {
		t.Errorf("pods %s in %s: %v", label, namespace, err)
		return
	}
	if !strings.Contains(output, "Running") {
		t.Errorf("no running pods with label %s in %s", label, namespace)
	}
}

// AssertServiceExists verifies a service exists.
func (f *Framework) AssertServiceExists(t *testing.T, namespace, name string) {
	t.Helper()
	if !f.ResourceExists("service", namespace, name) {
		t.Errorf("service %s/%s not found", namespace, name)
	}
}

// AssertSecretExists verifies a secret exists.
func (f *Framework) AssertSecretExists(t *testing.T, namespace, name string) {
	t.Helper()
	if !f.ResourceExists("secret", namespace, name) {
		t.Errorf("secret %s/%s not found", namespace, name)
	}
}
