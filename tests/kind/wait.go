//go:build kind

package kind

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// WaitForCondition polls until condition returns true or timeout is reached.
func (f *Framework) WaitForCondition(t *testing.T, desc string, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Second)
	}
	t.Fatalf("timeout waiting for %s", desc)
}

// WaitForDeployment waits for a deployment to have all replicas ready.
func (f *Framework) WaitForDeployment(t *testing.T, namespace, name string, timeout time.Duration) {
	t.Helper()
	t.Logf("Waiting for deployment %s/%s...", namespace, name)

	f.WaitForCondition(t, fmt.Sprintf("deployment %s/%s ready", namespace, name), timeout, func() bool {
		output, err := f.Kubectl("-n", namespace, "get", "deployment", name,
			"-o", "jsonpath={.status.readyReplicas}/{.status.replicas}")
		if err != nil {
			return false
		}
		parts := strings.Split(output, "/")
		if len(parts) == 2 && parts[0] != "" && parts[0] != "0" && parts[0] == parts[1] {
			t.Logf("  ✓ %s/%s ready (%s)", namespace, name, output)
			return true
		}
		return false
	})
}

// WaitForPod waits for a pod matching the label to be Running.
func (f *Framework) WaitForPod(t *testing.T, namespace, label string, timeout time.Duration) {
	t.Helper()
	t.Logf("Waiting for pod %s in %s...", label, namespace)

	f.WaitForCondition(t, fmt.Sprintf("pod %s running", label), timeout, func() bool {
		output, err := f.Kubectl("-n", namespace, "get", "pods", "-l", label,
			"-o", "jsonpath={.items[*].status.phase}")
		if err != nil {
			return false
		}
		if strings.Contains(output, "Running") {
			t.Logf("  ✓ Pod %s running", label)
			return true
		}
		return false
	})
}

// WaitForCRD waits for a CRD to be registered.
func (f *Framework) WaitForCRD(t *testing.T, name string, timeout time.Duration) {
	t.Helper()

	f.WaitForCondition(t, fmt.Sprintf("CRD %s", name), timeout, func() bool {
		_, err := f.Kubectl("get", "crd", name)
		if err == nil {
			t.Logf("  ✓ CRD %s", name)
			return true
		}
		return false
	})
}

// WaitForNamespace waits for a namespace to exist.
func (f *Framework) WaitForNamespace(t *testing.T, name string, timeout time.Duration) {
	t.Helper()

	f.WaitForCondition(t, fmt.Sprintf("namespace %s", name), timeout, func() bool {
		_, err := f.Kubectl("get", "namespace", name)
		return err == nil
	})
}

// WaitForCertificateReady waits for a cert-manager Certificate to be Ready.
func (f *Framework) WaitForCertificateReady(t *testing.T, namespace, name string, timeout time.Duration) {
	t.Helper()
	t.Logf("Waiting for certificate %s/%s...", namespace, name)

	f.WaitForCondition(t, fmt.Sprintf("certificate %s ready", name), timeout, func() bool {
		output, err := f.Kubectl("-n", namespace, "get", "certificate", name,
			"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
		return err == nil && output == "True"
	})
}
