package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ExternalDNS tests verify detection of external-dns annotations on Ingress/Service.

// ============================================================
// Test 1: Ingress with ExternalDNS annotation — detected
// ============================================================

func TestExternalDNS_Detect_Ingress_WithAnnotation(t *testing.T) {
	proc := NewIngressProcessor()
	ctx := newTestProcessorContext()

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "Ingress",
			"metadata": map[string]interface{}{
				"name":      "myapp-ingress",
				"namespace": "default",
				"annotations": map[string]interface{}{
					"external-dns.alpha.kubernetes.io/hostname": "app.example.com",
					"external-dns.alpha.kubernetes.io/ttl":      "300",
				},
			},
			"spec": map[string]interface{}{
				"rules": []interface{}{
					map[string]interface{}{
						"host": "app.example.com",
						"http": map[string]interface{}{
							"paths": []interface{}{
								map[string]interface{}{
									"path":     "/",
									"pathType": "Prefix",
									"backend": map[string]interface{}{
										"service": map[string]interface{}{
											"name": "myapp",
											"port": map[string]interface{}{
												"number": int64(80),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")

	// ExternalDNS data should be captured in metadata
	externalDNS, ok := result.Metadata["external_dns"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected external_dns in metadata")
	}
	testutil.AssertEqual(t, "app.example.com", externalDNS["hostname"], "hostname")
}

// ============================================================
// Test 2: Service with ExternalDNS annotation — detected
// ============================================================

func TestExternalDNS_Detect_Service_WithAnnotation(t *testing.T) {
	proc := NewServiceProcessor()
	ctx := newTestProcessorContext()

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "myapp-svc",
				"namespace": "default",
				"annotations": map[string]interface{}{
					"external-dns.alpha.kubernetes.io/hostname": "svc.example.com",
				},
			},
			"spec": map[string]interface{}{
				"type": "LoadBalancer",
				"ports": []interface{}{
					map[string]interface{}{
						"port":       int64(80),
						"targetPort": int64(8080),
					},
				},
				"selector": map[string]interface{}{
					"app": "myapp",
				},
			},
		},
	}

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	externalDNS, ok := result.Metadata["external_dns"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected external_dns in metadata")
	}
	testutil.AssertEqual(t, "svc.example.com", externalDNS["hostname"], "hostname")
}

// ============================================================
// Test 3: Service without ExternalDNS annotation — not detected
// ============================================================

func TestExternalDNS_Detect_NoAnnotation(t *testing.T) {
	proc := NewServiceProcessor()
	ctx := newTestProcessorContext()

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "myapp-svc",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"type": "ClusterIP",
				"ports": []interface{}{
					map[string]interface{}{
						"port":       int64(80),
						"targetPort": int64(8080),
					},
				},
				"selector": map[string]interface{}{
					"app": "myapp",
				},
			},
		},
	}

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	if _, ok := result.Metadata["external_dns"]; ok {
		t.Error("Should NOT have external_dns metadata without annotation")
	}
}

// ============================================================
// Test 4: Hostname extraction from annotation
// ============================================================

func TestExternalDNS_Hostname_Extraction(t *testing.T) {
	proc := NewIngressProcessor()
	ctx := newTestProcessorContext()

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "Ingress",
			"metadata": map[string]interface{}{
				"name":      "multi-host",
				"namespace": "default",
				"annotations": map[string]interface{}{
					"external-dns.alpha.kubernetes.io/hostname": "a.example.com,b.example.com",
				},
			},
			"spec": map[string]interface{}{
				"rules": []interface{}{
					map[string]interface{}{
						"host": "a.example.com",
					},
				},
			},
		},
	}

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	externalDNS := result.Metadata["external_dns"].(map[string]interface{})
	testutil.AssertEqual(t, "a.example.com,b.example.com", externalDNS["hostname"], "hostname should include all values")
}
