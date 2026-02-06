//go:build kind

package kind

import (
	"fmt"
	"testing"
)

// CollectDiagnostics gathers debug info for a failing resource.
func (f *Framework) CollectDiagnostics(t *testing.T, namespace, kind, name string) {
	t.Helper()
	t.Logf("\n=== Diagnostics: %s/%s/%s ===", namespace, kind, name)

	output, _ := f.Kubectl("-n", namespace, "describe", kind, name)
	t.Logf("Describe:\n%s", output)

	output, _ = f.Kubectl("-n", namespace, "get", "events", "--sort-by=.lastTimestamp",
		"--field-selector", fmt.Sprintf("involvedObject.name=%s", name))
	t.Logf("Events:\n%s", output)

	if kind == "deployment" {
		output, _ = f.Kubectl("-n", namespace, "get", "pods", "-o", "wide")
		t.Logf("Pods:\n%s", output)
	}
}

// CollectNamespaceDiagnostics gathers debug info for an entire namespace.
func (f *Framework) CollectNamespaceDiagnostics(t *testing.T, namespace string) {
	t.Helper()
	t.Logf("\n=== Namespace %s ===", namespace)

	output, _ := f.Kubectl("-n", namespace, "get", "all", "-o", "wide")
	t.Logf("Resources:\n%s", output)

	output, _ = f.Kubectl("-n", namespace, "get", "events", "--sort-by=.lastTimestamp")
	t.Logf("Events:\n%s", output)
}

// CollectPodLogs gets logs from pods matching a label.
func (f *Framework) CollectPodLogs(t *testing.T, namespace, label string, tailLines int) {
	t.Helper()

	podName, err := f.Kubectl("-n", namespace, "get", "pods", "-l", label,
		"-o", "jsonpath={.items[0].metadata.name}")
	if err != nil || podName == "" {
		return
	}

	output, _ := f.Kubectl("-n", namespace, "logs", podName, fmt.Sprintf("--tail=%d", tailLines))
	t.Logf("Logs from %s:\n%s", podName, output)
}
