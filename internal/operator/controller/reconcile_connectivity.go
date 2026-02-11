package controller

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
)

// reconcileConnectivityHealth probes DNS, TLS, and HTTPS for external endpoints.
// Only runs when spec.domain is set. Non-fatal — errors logged, never returned.
func (r *ClusterReconciler) reconcileConnectivityHealth(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("checking connectivity health")

	now := metav1.Now()
	conn := &cluster.Status.Connectivity

	// KubeAPI: always true if we're reconciling
	conn.KubeAPIReady = true

	// MetricsAPI: check APIService v1beta1.metrics.k8s.io
	conn.MetricsAPIReady = r.checkMetricsAPI(ctx)

	// External endpoints only if domain is set
	conn.Endpoints = nil
	if cluster.Spec.Domain != "" {
		var endpoints []k8znerv1alpha1.EndpointHealth

		// ArgoCD
		if cluster.Spec.Addons != nil && cluster.Spec.Addons.ArgoCD {
			subdomain := cluster.Spec.Addons.ArgoSubdomain
			if subdomain == "" {
				subdomain = "argo"
			}
			host := fmt.Sprintf("%s.%s", subdomain, cluster.Spec.Domain)
			endpoints = append(endpoints, r.probeEndpoint(ctx, host))
		}

		// Grafana
		if cluster.Spec.Addons != nil && cluster.Spec.Addons.Monitoring {
			subdomain := cluster.Spec.Addons.GrafanaSubdomain
			if subdomain == "" {
				subdomain = "grafana"
			}
			host := fmt.Sprintf("%s.%s", subdomain, cluster.Spec.Domain)
			endpoints = append(endpoints, r.probeEndpoint(ctx, host))
		}

		// Discover additional endpoints from Ingress objects (Prometheus, Alertmanager, etc.)
		endpoints = append(endpoints, r.discoverIngressEndpoints(ctx, cluster.Spec.Domain, endpoints)...)

		conn.Endpoints = endpoints
	}

	conn.LastCheck = &now
}

// checkMetricsAPI checks if the metrics APIService is available using unstructured client.
func (r *ClusterReconciler) checkMetricsAPI(ctx context.Context) bool {
	apiService := &unstructured.Unstructured{}
	apiService.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apiregistration.k8s.io",
		Version: "v1",
		Kind:    "APIService",
	})

	if err := r.Get(ctx, client.ObjectKey{Name: "v1beta1.metrics.k8s.io"}, apiService); err != nil {
		return false
	}

	conditions, found, err := unstructured.NestedSlice(apiService.Object, "status", "conditions")
	if err != nil || !found {
		return false
	}

	for _, c := range conditions {
		condMap, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if condMap["type"] == "Available" && condMap["status"] == "True" {
			return true
		}
	}
	return false
}

// probeEndpoint performs DNS, TLS, and HTTPS probes for a host.
func (r *ClusterReconciler) probeEndpoint(ctx context.Context, host string) k8znerv1alpha1.EndpointHealth {
	logger := log.FromContext(ctx)
	ep := k8znerv1alpha1.EndpointHealth{Host: host}

	// DNS probe
	ips, err := net.LookupHost(host)
	if err != nil || len(ips) == 0 {
		ep.Message = "DNS resolution failed"
		logger.V(1).Info("endpoint DNS failed", "host", host, "error", err)
		return ep
	}
	ep.DNSReady = true

	// TLS probe (connect and check handshake)
	tlsConn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", host+":443", &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		ep.Message = "TLS handshake failed"
		logger.V(1).Info("endpoint TLS failed", "host", host, "error", err)
		return ep
	}
	tlsConn.Close()
	ep.TLSReady = true

	// HTTPS probe
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := httpClient.Get(fmt.Sprintf("https://%s/", host))
	if err != nil {
		ep.Message = fmt.Sprintf("HTTPS request failed: %v", err)
		logger.V(1).Info("endpoint HTTPS failed", "host", host, "error", err)
		return ep
	}
	resp.Body.Close()

	if resp.StatusCode < 500 {
		ep.HTTPReady = true
		ep.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
	} else {
		ep.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	return ep
}

// discoverIngressEndpoints finds Ingress objects in the monitoring namespace
// and probes any hosts not already covered by the explicit endpoint list.
func (r *ClusterReconciler) discoverIngressEndpoints(ctx context.Context, domain string, existing []k8znerv1alpha1.EndpointHealth) []k8znerv1alpha1.EndpointHealth {
	logger := log.FromContext(ctx)

	// Build set of already-probed hosts
	probed := make(map[string]bool, len(existing))
	for _, ep := range existing {
		probed[ep.Host] = true
	}

	var extra []k8znerv1alpha1.EndpointHealth

	// Scan monitoring namespace for Prometheus/Alertmanager ingresses
	ingressList := &unstructured.UnstructuredList{}
	ingressList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "networking.k8s.io",
		Version: "v1",
		Kind:    "IngressList",
	})
	if err := r.List(ctx, ingressList, client.InNamespace("monitoring")); err != nil {
		logger.V(1).Info("failed to list monitoring ingresses", "error", err)
		return nil
	}

	for _, ing := range ingressList.Items {
		rules, found, err := unstructured.NestedSlice(ing.Object, "spec", "rules")
		if err != nil || !found {
			continue
		}
		for _, rule := range rules {
			ruleMap, ok := rule.(map[string]interface{})
			if !ok {
				continue
			}
			host, _, _ := unstructured.NestedString(ruleMap, "host")
			if host == "" || probed[host] {
				continue
			}
			// Only probe hosts under the cluster domain
			if !strings.HasSuffix(host, "."+domain) {
				continue
			}
			probed[host] = true
			extra = append(extra, r.probeEndpoint(ctx, host))
		}
	}

	return extra
}
