package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Test helpers for Ingress processor
// ============================================================

func makeIngressObj(name, namespace string, labels, annotations map[string]interface{}, spec map[string]interface{}) *unstructured.Unstructured {
	metadata := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
	}
	if labels != nil {
		metadata["labels"] = labels
	}
	if annotations != nil {
		metadata["annotations"] = annotations
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "Ingress",
			"metadata":   metadata,
			"spec":       spec,
		},
	}
}

func makeBasicIngressRule(host, path, pathType, svcName string, svcPort int64) map[string]interface{} {
	return map[string]interface{}{
		"host": host,
		"http": map[string]interface{}{
			"paths": []interface{}{
				map[string]interface{}{
					"path":     path,
					"pathType": pathType,
					"backend": map[string]interface{}{
						"service": map[string]interface{}{
							"name": svcName,
							"port": map[string]interface{}{
								"number": svcPort,
							},
						},
					},
				},
			},
		},
	}
}

// ============================================================
// Subtask 1: Extract ingressClassName
// ============================================================

func TestProcessIngress_ExtractsClassName(t *testing.T) {
	t.Run("Nginx", func(t *testing.T) {
		proc := NewIngressProcessor()
		ctx := newTestProcessorContext()

		obj := makeIngressObj("my-ingress", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"ingressClassName": "nginx",
				"rules": []interface{}{
					makeBasicIngressRule("example.com", "/", "Prefix", "my-svc", int64(80)),
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, "nginx", result.Values["className"], "ingressClassName should be nginx")
	})

	t.Run("NoClassName", func(t *testing.T) {
		proc := NewIngressProcessor()
		ctx := newTestProcessorContext()

		obj := makeIngressObj("my-ingress", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"rules": []interface{}{
					makeBasicIngressRule("example.com", "/", "Prefix", "my-svc", int64(80)),
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		_, hasClassName := result.Values["className"]
		if hasClassName {
			t.Error("Expected no className when not specified")
		}
	})
}

// ============================================================
// Subtask 2: Extract rules (single host)
// ============================================================

func TestProcessIngress_ExtractsRulesSingleHost(t *testing.T) {
	proc := NewIngressProcessor()
	ctx := newTestProcessorContext()

	obj := makeIngressObj("my-ingress", "default",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"rules": []interface{}{
				makeBasicIngressRule("example.com", "/api", "Prefix", "api-svc", int64(8080)),
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	hosts, ok := result.Values["hosts"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected hosts as []map[string]interface{}")
	}
	if len(hosts) != 1 {
		t.Fatalf("Expected 1 host, got %d", len(hosts))
	}

	testutil.AssertEqual(t, "example.com", hosts[0]["host"], "host name")

	paths, ok := hosts[0]["paths"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected paths in host entry")
	}
	if len(paths) != 1 {
		t.Fatalf("Expected 1 path, got %d", len(paths))
	}

	testutil.AssertEqual(t, "/api", paths[0]["path"], "path")
	testutil.AssertEqual(t, "Prefix", paths[0]["pathType"], "pathType")

	svc, ok := paths[0]["service"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected service backend in path")
	}
	testutil.AssertEqual(t, "api-svc", svc["name"], "backend service name")
	testutil.AssertEqual(t, int64(8080), svc["port"], "backend service port")
}

// ============================================================
// Subtask 3: Extract rules (multiple hosts)
// ============================================================

func TestProcessIngress_ExtractsRulesMultipleHosts(t *testing.T) {
	proc := NewIngressProcessor()
	ctx := newTestProcessorContext()

	obj := makeIngressObj("my-ingress", "default",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"rules": []interface{}{
				makeBasicIngressRule("api.example.com", "/", "Prefix", "api-svc", int64(8080)),
				map[string]interface{}{
					"host": "web.example.com",
					"http": map[string]interface{}{
						"paths": []interface{}{
							map[string]interface{}{
								"path":     "/",
								"pathType": "Prefix",
								"backend": map[string]interface{}{
									"service": map[string]interface{}{
										"name": "web-svc",
										"port": map[string]interface{}{
											"number": int64(80),
										},
									},
								},
							},
							map[string]interface{}{
								"path":     "/assets",
								"pathType": "Prefix",
								"backend": map[string]interface{}{
									"service": map[string]interface{}{
										"name": "static-svc",
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
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	hosts := result.Values["hosts"].([]map[string]interface{})
	if len(hosts) != 2 {
		t.Fatalf("Expected 2 hosts, got %d", len(hosts))
	}

	testutil.AssertEqual(t, "api.example.com", hosts[0]["host"], "first host")
	testutil.AssertEqual(t, "web.example.com", hosts[1]["host"], "second host")

	webPaths := hosts[1]["paths"].([]map[string]interface{})
	if len(webPaths) != 2 {
		t.Fatalf("Expected 2 paths for web host, got %d", len(webPaths))
	}
}

// ============================================================
// Subtask 4: Extract TLS configuration
// ============================================================

func TestProcessIngress_ExtractsTLS(t *testing.T) {
	proc := NewIngressProcessor()
	ctx := newTestProcessorContext()

	obj := makeIngressObj("my-ingress", "default",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"tls": []interface{}{
				map[string]interface{}{
					"hosts":      []interface{}{"example.com", "www.example.com"},
					"secretName": "tls-secret",
				},
			},
			"rules": []interface{}{
				makeBasicIngressRule("example.com", "/", "Prefix", "my-svc", int64(80)),
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tls, ok := result.Values["tls"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected tls as []map[string]interface{}")
	}
	if len(tls) != 1 {
		t.Fatalf("Expected 1 TLS entry, got %d", len(tls))
	}

	tlsHosts := tls[0]["hosts"].([]string)
	if len(tlsHosts) != 2 {
		t.Fatalf("Expected 2 TLS hosts, got %d", len(tlsHosts))
	}
	testutil.AssertEqual(t, "example.com", tlsHosts[0], "first TLS host")
	testutil.AssertEqual(t, "www.example.com", tlsHosts[1], "second TLS host")
	testutil.AssertEqual(t, "tls-secret", tls[0]["secretName"], "TLS secret name")

	// Verify Secret dependency
	if !hasDependency(result.Dependencies, "Secret", "default", "tls-secret") {
		t.Error("Expected Secret dependency for TLS secret")
	}
}

// ============================================================
// Subtask 5: Extract backend service dependencies
// ============================================================

func TestProcessIngress_DetectsServiceDependencies(t *testing.T) {
	proc := NewIngressProcessor()
	ctx := newTestProcessorContext()

	obj := makeIngressObj("my-ingress", "default",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"rules": []interface{}{
				makeBasicIngressRule("example.com", "/api", "Prefix", "api-svc", int64(8080)),
				makeBasicIngressRule("example.com", "/web", "Prefix", "web-svc", int64(80)),
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	if !hasDependency(result.Dependencies, "Service", "default", "api-svc") {
		t.Error("Expected Service dependency for 'api-svc'")
	}
	if !hasDependency(result.Dependencies, "Service", "default", "web-svc") {
		t.Error("Expected Service dependency for 'web-svc'")
	}
}

// ============================================================
// Subtask 6: Extract annotations
// ============================================================

func TestProcessIngress_ExtractsAnnotations(t *testing.T) {
	t.Run("NginxAnnotations", func(t *testing.T) {
		proc := NewIngressProcessor()
		ctx := newTestProcessorContext()

		obj := makeIngressObj("my-ingress", "default",
			map[string]interface{}{"app": "myapp"},
			map[string]interface{}{
				"nginx.ingress.kubernetes.io/rewrite-target":   "/",
				"nginx.ingress.kubernetes.io/ssl-redirect":     "true",
				"nginx.ingress.kubernetes.io/proxy-body-size":  "50m",
			},
			map[string]interface{}{
				"rules": []interface{}{
					makeBasicIngressRule("example.com", "/", "Prefix", "my-svc", int64(80)),
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		annotations := result.Values["annotations"].(map[string]string)
		testutil.AssertEqual(t, "/", annotations["nginx.ingress.kubernetes.io/rewrite-target"], "rewrite-target")
		testutil.AssertEqual(t, "true", annotations["nginx.ingress.kubernetes.io/ssl-redirect"], "ssl-redirect")
	})

	t.Run("CertManagerAnnotation_ClusterIssuer", func(t *testing.T) {
		proc := NewIngressProcessor()
		ctx := newTestProcessorContext()

		obj := makeIngressObj("my-ingress", "default",
			map[string]interface{}{"app": "myapp"},
			map[string]interface{}{
				"cert-manager.io/cluster-issuer": "letsencrypt-prod",
			},
			map[string]interface{}{
				"rules": []interface{}{
					makeBasicIngressRule("example.com", "/", "Prefix", "my-svc", int64(80)),
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		if !hasDependency(result.Dependencies, "ClusterIssuer", "", "letsencrypt-prod") {
			t.Error("Expected ClusterIssuer dependency for cert-manager annotation")
		}
	})

	t.Run("CertManagerAnnotation_Issuer", func(t *testing.T) {
		proc := NewIngressProcessor()
		ctx := newTestProcessorContext()

		obj := makeIngressObj("my-ingress", "staging",
			map[string]interface{}{"app": "myapp"},
			map[string]interface{}{
				"cert-manager.io/issuer": "letsencrypt-staging",
			},
			map[string]interface{}{
				"rules": []interface{}{
					makeBasicIngressRule("example.com", "/", "Prefix", "my-svc", int64(80)),
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		if !hasDependency(result.Dependencies, "Issuer", "staging", "letsencrypt-staging") {
			t.Error("Expected Issuer dependency for cert-manager annotation")
		}
	})
}

// ============================================================
// Subtask 7: Path type detection
// ============================================================

func TestProcessIngress_PathTypes(t *testing.T) {
	proc := NewIngressProcessor()
	ctx := newTestProcessorContext()

	obj := makeIngressObj("my-ingress", "default",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{
					"host": "example.com",
					"http": map[string]interface{}{
						"paths": []interface{}{
							map[string]interface{}{
								"path":     "/api",
								"pathType": "Prefix",
								"backend": map[string]interface{}{
									"service": map[string]interface{}{
										"name": "api-svc",
										"port": map[string]interface{}{"number": int64(80)},
									},
								},
							},
							map[string]interface{}{
								"path":     "/healthz",
								"pathType": "Exact",
								"backend": map[string]interface{}{
									"service": map[string]interface{}{
										"name": "health-svc",
										"port": map[string]interface{}{"number": int64(80)},
									},
								},
							},
							map[string]interface{}{
								"path":     "/legacy",
								"pathType": "ImplementationSpecific",
								"backend": map[string]interface{}{
									"service": map[string]interface{}{
										"name": "legacy-svc",
										"port": map[string]interface{}{"number": int64(80)},
									},
								},
							},
						},
					},
				},
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	hosts := result.Values["hosts"].([]map[string]interface{})
	paths := hosts[0]["paths"].([]map[string]interface{})

	testutil.AssertEqual(t, "Prefix", paths[0]["pathType"], "first pathType")
	testutil.AssertEqual(t, "Exact", paths[1]["pathType"], "second pathType")
	testutil.AssertEqual(t, "ImplementationSpecific", paths[2]["pathType"], "third pathType")
}

// ============================================================
// Subtask 8: Named port backend
// ============================================================

func TestProcessIngress_NamedPortBackend(t *testing.T) {
	proc := NewIngressProcessor()
	ctx := newTestProcessorContext()

	obj := makeIngressObj("my-ingress", "default",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{
					"host": "example.com",
					"http": map[string]interface{}{
						"paths": []interface{}{
							map[string]interface{}{
								"path":     "/",
								"pathType": "Prefix",
								"backend": map[string]interface{}{
									"service": map[string]interface{}{
										"name": "my-svc",
										"port": map[string]interface{}{
											"name": "http",
										},
									},
								},
							},
						},
					},
				},
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	hosts := result.Values["hosts"].([]map[string]interface{})
	paths := hosts[0]["paths"].([]map[string]interface{})
	svc := paths[0]["service"].(map[string]interface{})

	testutil.AssertEqual(t, "http", svc["portName"], "named port should be preserved as portName")
}

// ============================================================
// Subtask 9: Wildcard hosts
// ============================================================

func TestProcessIngress_WildcardHost(t *testing.T) {
	proc := NewIngressProcessor()
	ctx := newTestProcessorContext()

	obj := makeIngressObj("my-ingress", "default",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"rules": []interface{}{
				makeBasicIngressRule("*.example.com", "/", "Prefix", "my-svc", int64(80)),
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	hosts := result.Values["hosts"].([]map[string]interface{})
	testutil.AssertEqual(t, "*.example.com", hosts[0]["host"], "wildcard host should be preserved")
}

// ============================================================
// Subtask 10: Edge cases
// ============================================================

func TestProcessIngress_EdgeCases(t *testing.T) {
	t.Run("NilIngress", func(t *testing.T) {
		proc := NewIngressProcessor()
		ctx := newTestProcessorContext()

		result, err := proc.Process(ctx, nil)
		if err == nil {
			t.Fatal("Expected error for nil ingress, got nil")
		}
		if result != nil {
			t.Error("Expected nil result for nil ingress")
		}
	})

	t.Run("EmptyHost", func(t *testing.T) {
		proc := NewIngressProcessor()
		ctx := newTestProcessorContext()

		obj := makeIngressObj("my-ingress", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"rules": []interface{}{
					map[string]interface{}{
						"http": map[string]interface{}{
							"paths": []interface{}{
								map[string]interface{}{
									"path":     "/",
									"pathType": "Prefix",
									"backend": map[string]interface{}{
										"service": map[string]interface{}{
											"name": "my-svc",
											"port": map[string]interface{}{"number": int64(80)},
										},
									},
								},
							},
						},
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		hosts := result.Values["hosts"].([]map[string]interface{})
		// Empty host should still have paths
		_, hasHost := hosts[0]["host"]
		if hasHost {
			t.Error("Expected no host key when host is empty")
		}
		_, hasPaths := hosts[0]["paths"]
		if !hasPaths {
			t.Error("Expected paths even without host")
		}
	})

	t.Run("NoAnnotations", func(t *testing.T) {
		proc := NewIngressProcessor()
		ctx := newTestProcessorContext()

		obj := makeIngressObj("my-ingress", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"rules": []interface{}{
					makeBasicIngressRule("example.com", "/", "Prefix", "my-svc", int64(80)),
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		_, hasAnnotations := result.Values["annotations"]
		if hasAnnotations {
			t.Error("Expected no annotations key when none specified")
		}
	})
}

// ============================================================
// Subtask 8: Default backend
// ============================================================

func TestProcessIngress_DefaultBackend(t *testing.T) {
	proc := NewIngressProcessor()
	ctx := newTestProcessorContext()

	obj := makeIngressObj("my-ingress", "default",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"defaultBackend": map[string]interface{}{
				"service": map[string]interface{}{
					"name": "fallback-svc",
					"port": map[string]interface{}{
						"number": int64(8080),
					},
				},
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	// Verify defaultBackend is extracted
	db, ok := result.Values["defaultBackend"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected defaultBackend in values")
	}

	svc, ok := db["service"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected service in defaultBackend")
	}

	if svc["name"] != "fallback-svc" {
		t.Errorf("Expected service name 'fallback-svc', got %v", svc["name"])
	}
	if svc["port"] != int64(8080) {
		t.Errorf("Expected port 8080, got %v", svc["port"])
	}

	// Verify Service dependency is registered
	foundDep := false
	for _, dep := range result.Dependencies {
		if dep.GVK.Kind == "Service" && dep.Name == "fallback-svc" {
			foundDep = true
			break
		}
	}
	if !foundDep {
		t.Error("Expected Service dependency for defaultBackend service 'fallback-svc'")
	}
}

// ============================================================
// Result metadata and template tests
// ============================================================

func TestProcessIngress_ResultMetadata(t *testing.T) {
	proc := NewIngressProcessor()
	ctx := newTestProcessorContext()

	obj := makeIngressObj("my-app-ingress", "production",
		map[string]interface{}{"app": "my-app"}, nil,
		map[string]interface{}{
			"rules": []interface{}{
				makeBasicIngressRule("example.com", "/", "Prefix", "my-svc", int64(80)),
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	testutil.AssertEqual(t, true, result.Processed, "should be processed")
	testutil.AssertEqual(t, "myApp", result.ServiceName, "service name from labels")
	testutil.AssertEqual(t, "templates/myApp-ingress.yaml", result.TemplatePath, "template path")
	testutil.AssertEqual(t, "services.myApp.ingress", result.ValuesPath, "values path")
	testutil.AssertEqual(t, "my-app-ingress", result.Metadata["name"], "metadata name")
	testutil.AssertEqual(t, "production", result.Metadata["namespace"], "metadata namespace")
}

func TestProcessIngress_GeneratesTemplate(t *testing.T) {
	proc := NewIngressProcessor()
	ctx := processor.Context{ChartName: "myapp"}

	obj := makeIngressObj("my-ingress", "default",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"rules": []interface{}{
				makeBasicIngressRule("example.com", "/", "Prefix", "my-svc", int64(80)),
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tpl := result.TemplateContent
	if tpl == "" {
		t.Fatal("Expected non-empty template content")
	}

	testutil.AssertContains(t, tpl, "apiVersion: networking.k8s.io/v1", "template should have apiVersion")
	testutil.AssertContains(t, tpl, "kind: Ingress", "template should have kind")
	testutil.AssertContains(t, tpl, "{{ $.Release.Namespace }}", "template should use release namespace")
	testutil.AssertContains(t, tpl, `include "myapp.labels"`, "template should include labels helper")
	testutil.AssertContains(t, tpl, "ingressClassName", "template should reference className")
	testutil.AssertContains(t, tpl, ".hosts", "template should reference hosts")
}

// ============================================================
// Constructor test
// ============================================================

func TestNewIngressProcessor(t *testing.T) {
	proc := NewIngressProcessor()

	testutil.AssertEqual(t, "ingress", proc.Name(), "processor name")
	testutil.AssertEqual(t, 100, proc.Priority(), "processor priority")

	gvks := proc.Supports()
	if len(gvks) != 1 {
		t.Fatalf("Expected 1 supported GVK, got %d", len(gvks))
	}
	expected := schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

// Ensure unused imports don't cause compilation errors
var _ = strings.Contains
