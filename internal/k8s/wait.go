package k8s

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// WaitForPodReady waits for a pod to become ready.
func (c *Client) WaitForPodReady(ctx context.Context, namespace, name string, timeout time.Duration) error {
	return wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}

		return isPodReady(pod), nil
	})
}

// WaitForPodsReady waits for all pods matching a label selector to become ready.
func (c *Client) WaitForPodsReady(ctx context.Context, namespace, labelSelector string, timeout time.Duration) error {
	return wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		pods, err := c.GetPods(ctx, namespace, labelSelector)
		if err != nil {
			return false, nil
		}

		if len(pods) == 0 {
			return false, nil
		}

		for _, pod := range pods {
			if !isPodReady(&pod) {
				return false, nil
			}
		}

		return true, nil
	})
}

// WaitForServiceEndpoints waits for a service to have endpoints.
func (c *Client) WaitForServiceEndpoints(ctx context.Context, namespace, name string, timeout time.Duration) error {
	return wait.PollImmediate(5*time.Second, timeout, func() (bool, error) {
		endpoints, err := c.clientset.CoreV1().Endpoints(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}

		for _, subset := range endpoints.Subsets {
			if len(subset.Addresses) > 0 {
				return true, nil
			}
		}

		return false, nil
	})
}

// WaitForNamespace waits for a namespace to exist.
func (c *Client) WaitForNamespace(ctx context.Context, name string, timeout time.Duration) error {
	return wait.PollImmediate(2*time.Second, timeout, func() (bool, error) {
		_, err := c.clientset.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		return true, nil
	})
}

// CheckPodLogs retrieves logs from a pod to check for errors.
func (c *Client) CheckPodLogs(ctx context.Context, namespace, name string) (string, error) {
	logOptions := &corev1.PodLogOptions{
		TailLines: int64Ptr(100),
	}

	req := c.clientset.CoreV1().Pods(namespace).GetLogs(name, logOptions)
	logs, err := req.DoRaw(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get pod logs: %w", err)
	}

	return string(logs), nil
}

// isPodReady checks if a pod is ready.
func isPodReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady &&
			condition.Status == corev1.ConditionTrue {
			return true
		}
	}

	return false
}

// int64Ptr returns a pointer to an int64 value.
func int64Ptr(i int64) *int64 {
	return &i
}
