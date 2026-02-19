package k8s

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const externalDNSHostnameAnnotation = "external-dns.alpha.kubernetes.io/hostname"

// detectExternalDNS checks for external-dns annotations and returns metadata if present.
func detectExternalDNS(obj *unstructured.Unstructured) map[string]interface{} {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return nil
	}

	hostname, ok := annotations[externalDNSHostnameAnnotation]
	if !ok || hostname == "" {
		return nil
	}

	result := map[string]interface{}{
		"hostname": hostname,
		"enabled":  true,
	}

	// Optional TTL annotation
	if ttl, ok := annotations["external-dns.alpha.kubernetes.io/ttl"]; ok {
		result["ttl"] = ttl
	}

	return result
}
