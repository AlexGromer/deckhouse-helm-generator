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
// Test helpers for Service processor
// ============================================================

// makeServiceObj creates an unstructured Service for testing.
func makeServiceObj(name, namespace string, labels, annotations map[string]interface{}, spec map[string]interface{}) *unstructured.Unstructured {
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
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata":   metadata,
			"spec":       spec,
		},
	}
}

// makeBasicServiceSpec creates a minimal service spec with ClusterIP type and one port.
func makeBasicServiceSpec() map[string]interface{} {
	return map[string]interface{}{
		"type": "ClusterIP",
		"selector": map[string]interface{}{
			"app": "myapp",
		},
		"ports": []interface{}{
			map[string]interface{}{
				"name":       "http",
				"port":       int64(80),
				"targetPort": int64(8080),
				"protocol":   "TCP",
			},
		},
	}
}

// ============================================================
// Subtask 1: Extract service type
// ============================================================

func TestProcessService_ExtractsType(t *testing.T) {
	t.Run("ClusterIP", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"type": "ClusterIP",
				"ports": []interface{}{
					map[string]interface{}{
						"port":     int64(80),
						"protocol": "TCP",
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, "ClusterIP", result.Values["type"], "service type should be ClusterIP")
	})

	t.Run("NodePort", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"type": "NodePort",
				"ports": []interface{}{
					map[string]interface{}{
						"port":     int64(80),
						"nodePort": int64(30080),
						"protocol": "TCP",
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, "NodePort", result.Values["type"], "service type should be NodePort")
	})

	t.Run("LoadBalancer", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"type": "LoadBalancer",
				"ports": []interface{}{
					map[string]interface{}{
						"port":     int64(443),
						"protocol": "TCP",
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, "LoadBalancer", result.Values["type"], "service type should be LoadBalancer")
	})

	t.Run("ExternalName", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"type":         "ExternalName",
				"externalName": "my.external.service.example.com",
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, "ExternalName", result.Values["type"], "service type should be ExternalName")
		testutil.AssertEqual(t, "my.external.service.example.com", result.Values["externalName"],
			"externalName should be extracted")
	})

	t.Run("Default_NoType", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"ports": []interface{}{
					map[string]interface{}{
						"port":     int64(80),
						"protocol": "TCP",
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, "ClusterIP", result.Values["type"],
			"service type should default to ClusterIP when not specified")
	})
}

// ============================================================
// Subtask 2: Extract ports
// ============================================================

func TestProcessService_ExtractsPorts(t *testing.T) {
	t.Run("SinglePort", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"type": "ClusterIP",
				"ports": []interface{}{
					map[string]interface{}{
						"name":       "http",
						"port":       int64(80),
						"targetPort": int64(8080),
						"protocol":   "TCP",
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		ports, ok := result.Values["ports"].([]map[string]interface{})
		if !ok {
			t.Fatal("Expected ports as []map[string]interface{} in values")
		}
		if len(ports) != 1 {
			t.Fatalf("Expected 1 port, got %d", len(ports))
		}

		testutil.AssertEqual(t, "http", ports[0]["name"], "port name")
		testutil.AssertEqual(t, int64(80), ports[0]["port"], "port number")
		testutil.AssertEqual(t, int64(8080), ports[0]["targetPort"], "target port")
		testutil.AssertEqual(t, "TCP", ports[0]["protocol"], "protocol")
	})

	t.Run("MultiplePorts", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"type": "ClusterIP",
				"ports": []interface{}{
					map[string]interface{}{
						"name":       "http",
						"port":       int64(80),
						"targetPort": int64(8080),
						"protocol":   "TCP",
					},
					map[string]interface{}{
						"name":       "https",
						"port":       int64(443),
						"targetPort": int64(8443),
						"protocol":   "TCP",
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		ports, ok := result.Values["ports"].([]map[string]interface{})
		if !ok {
			t.Fatal("Expected ports array in values")
		}
		if len(ports) != 2 {
			t.Fatalf("Expected 2 ports, got %d", len(ports))
		}

		testutil.AssertEqual(t, "http", ports[0]["name"], "first port name")
		testutil.AssertEqual(t, int64(80), ports[0]["port"], "first port number")
		testutil.AssertEqual(t, "https", ports[1]["name"], "second port name")
		testutil.AssertEqual(t, int64(443), ports[1]["port"], "second port number")
	})

	t.Run("NamedTargetPort", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"type": "ClusterIP",
				"ports": []interface{}{
					map[string]interface{}{
						"name":       "http",
						"port":       int64(80),
						"targetPort": "http-web",
						"protocol":   "TCP",
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		ports := result.Values["ports"].([]map[string]interface{})
		testutil.AssertEqual(t, "http-web", ports[0]["targetPort"], "named target port should be preserved")
	})

	t.Run("NodePort", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"type": "NodePort",
				"ports": []interface{}{
					map[string]interface{}{
						"name":       "http",
						"port":       int64(80),
						"targetPort": int64(8080),
						"nodePort":   int64(30080),
						"protocol":   "TCP",
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		ports := result.Values["ports"].([]map[string]interface{})
		testutil.AssertEqual(t, int64(30080), ports[0]["nodePort"], "nodePort should be extracted")
	})
}

// ============================================================
// Subtask 3: Extract selectors
// ============================================================

func TestProcessService_ExtractsSelector(t *testing.T) {
	t.Run("StandardSelector", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"type": "ClusterIP",
				"selector": map[string]interface{}{
					"app":     "myapp",
					"version": "v1",
				},
				"ports": []interface{}{
					map[string]interface{}{
						"port":     int64(80),
						"protocol": "TCP",
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		selector, ok := result.Values["selector"].(map[string]string)
		if !ok {
			t.Fatal("Expected selector as map[string]string in values")
		}
		testutil.AssertEqual(t, "myapp", selector["app"], "selector 'app'")
		testutil.AssertEqual(t, "v1", selector["version"], "selector 'version'")
	})

	t.Run("SelectorDetectsDeploymentDep_App", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"type": "ClusterIP",
				"selector": map[string]interface{}{
					"app": "backend",
				},
				"ports": []interface{}{
					map[string]interface{}{
						"port":     int64(80),
						"protocol": "TCP",
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		if !hasDependency(result.Dependencies, "Deployment", "default", "backend") {
			t.Error("Expected Deployment dependency for 'backend' detected from selector app label")
		}
	})

	t.Run("SelectorDetectsDeploymentDep_K8sName", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "production",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"type": "ClusterIP",
				"selector": map[string]interface{}{
					"app.kubernetes.io/name": "frontend",
				},
				"ports": []interface{}{
					map[string]interface{}{
						"port":     int64(80),
						"protocol": "TCP",
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		if !hasDependency(result.Dependencies, "Deployment", "production", "frontend") {
			t.Error("Expected Deployment dependency for 'frontend' detected from app.kubernetes.io/name selector")
		}
	})
}

// ============================================================
// Subtask 4: Extract session affinity
// ============================================================

func TestProcessService_ExtractsSessionAffinity(t *testing.T) {
	t.Run("ClientIP", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"type":            "ClusterIP",
				"sessionAffinity": "ClientIP",
				"ports": []interface{}{
					map[string]interface{}{
						"port":     int64(80),
						"protocol": "TCP",
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, "ClientIP", result.Values["sessionAffinity"], "session affinity should be ClientIP")
	})

	t.Run("None", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"type":            "ClusterIP",
				"sessionAffinity": "None",
				"ports": []interface{}{
					map[string]interface{}{
						"port":     int64(80),
						"protocol": "TCP",
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, "None", result.Values["sessionAffinity"], "session affinity should be None")
	})

	t.Run("NotSet", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			makeBasicServiceSpec())

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		_, hasSessionAffinity := result.Values["sessionAffinity"]
		if hasSessionAffinity {
			t.Error("Expected no sessionAffinity key when not specified")
		}
	})
}

// ============================================================
// Subtask 5: Extract loadBalancerIP
// ============================================================

func TestProcessService_ExtractsLoadBalancer(t *testing.T) {
	t.Run("LoadBalancerIP", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"type":           "LoadBalancer",
				"loadBalancerIP": "10.0.0.100",
				"ports": []interface{}{
					map[string]interface{}{
						"port":     int64(443),
						"protocol": "TCP",
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, "10.0.0.100", result.Values["loadBalancerIP"],
			"loadBalancerIP should be extracted")
	})

	t.Run("LoadBalancerSourceRanges", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"type": "LoadBalancer",
				"loadBalancerSourceRanges": []interface{}{
					"10.0.0.0/8",
					"192.168.0.0/16",
				},
				"ports": []interface{}{
					map[string]interface{}{
						"port":     int64(443),
						"protocol": "TCP",
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		ranges, ok := result.Values["loadBalancerSourceRanges"].([]string)
		if !ok {
			t.Fatal("Expected loadBalancerSourceRanges as []string in values")
		}
		if len(ranges) != 2 {
			t.Fatalf("Expected 2 source ranges, got %d", len(ranges))
		}
		testutil.AssertEqual(t, "10.0.0.0/8", ranges[0], "first source range")
		testutil.AssertEqual(t, "192.168.0.0/16", ranges[1], "second source range")
	})
}

// ============================================================
// Subtask 6: Extract annotations
// ============================================================

func TestProcessService_ExtractsAnnotations(t *testing.T) {
	t.Run("CloudProviderAnnotations", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"},
			map[string]interface{}{
				"service.beta.kubernetes.io/aws-load-balancer-type":            "nlb",
				"service.beta.kubernetes.io/aws-load-balancer-internal":        "true",
				"service.beta.kubernetes.io/aws-load-balancer-ssl-cert":        "arn:aws:acm:us-east-1:123456:certificate/abc-123",
				"service.beta.kubernetes.io/aws-load-balancer-ssl-ports":       "443",
			},
			map[string]interface{}{
				"type": "LoadBalancer",
				"ports": []interface{}{
					map[string]interface{}{
						"port":     int64(443),
						"protocol": "TCP",
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		annotations, ok := result.Values["annotations"].(map[string]string)
		if !ok {
			t.Fatal("Expected annotations as map[string]string in values")
		}
		testutil.AssertEqual(t, "nlb",
			annotations["service.beta.kubernetes.io/aws-load-balancer-type"],
			"AWS LB type annotation")
		testutil.AssertEqual(t, "true",
			annotations["service.beta.kubernetes.io/aws-load-balancer-internal"],
			"AWS LB internal annotation")
	})

	t.Run("NoAnnotations", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			makeBasicServiceSpec())

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		_, hasAnnotations := result.Values["annotations"]
		if hasAnnotations {
			t.Error("Expected no annotations key when no annotations on resource")
		}
	})
}

// ============================================================
// Subtask 7: Extract externalTrafficPolicy
// ============================================================

func TestProcessService_ExtractsExternalTrafficPolicy(t *testing.T) {
	t.Run("Local", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"type":                  "LoadBalancer",
				"externalTrafficPolicy": "Local",
				"ports": []interface{}{
					map[string]interface{}{
						"port":     int64(80),
						"protocol": "TCP",
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, "Local", result.Values["externalTrafficPolicy"],
			"externalTrafficPolicy should be Local")
	})

	t.Run("Cluster", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"type":                  "NodePort",
				"externalTrafficPolicy": "Cluster",
				"ports": []interface{}{
					map[string]interface{}{
						"port":     int64(80),
						"nodePort": int64(30080),
						"protocol": "TCP",
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, "Cluster", result.Values["externalTrafficPolicy"],
			"externalTrafficPolicy should be Cluster")
	})

	t.Run("NotSet", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			makeBasicServiceSpec())

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		_, hasPolicy := result.Values["externalTrafficPolicy"]
		if hasPolicy {
			t.Error("Expected no externalTrafficPolicy when not specified")
		}
	})
}

// ============================================================
// Subtask 8: Extract healthCheckNodePort
// ============================================================

func TestProcessService_ExtractsHealthCheckNodePort(t *testing.T) {
	proc := NewServiceProcessor()
	ctx := newTestProcessorContext()

	obj := makeServiceObj("my-svc", "default",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"type":                  "LoadBalancer",
			"externalTrafficPolicy": "Local",
			"healthCheckNodePort":   int64(31234),
			"ports": []interface{}{
				map[string]interface{}{
					"port":     int64(80),
					"protocol": "TCP",
				},
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, int64(31234), result.Values["healthCheckNodePort"],
		"healthCheckNodePort should be extracted")
}

// ============================================================
// Subtask 9: Edge cases
// ============================================================

func TestProcessService_EdgeCases(t *testing.T) {
	t.Run("HeadlessService", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"type":      "ClusterIP",
				"clusterIP": "None",
				"selector": map[string]interface{}{
					"app": "myapp",
				},
				"ports": []interface{}{
					map[string]interface{}{
						"port":     int64(80),
						"protocol": "TCP",
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, "None", result.Values["clusterIP"],
			"headless service should have clusterIP=None")
	})

	t.Run("ServiceWithoutSelectors", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		// Service without selectors (for manual Endpoints)
		obj := makeServiceObj("external-svc", "default",
			map[string]interface{}{"app": "external"}, nil,
			map[string]interface{}{
				"type": "ClusterIP",
				"ports": []interface{}{
					map[string]interface{}{
						"port":     int64(3306),
						"protocol": "TCP",
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		_, hasSelector := result.Values["selector"]
		if hasSelector {
			t.Error("Expected no selector key when service has no selectors")
		}

		// No Deployment dependencies when no selectors
		if len(result.Dependencies) > 0 {
			t.Errorf("Expected 0 dependencies for service without selectors, got %d", len(result.Dependencies))
		}
	})

	t.Run("NilService", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		result, err := proc.Process(ctx, nil)
		if err == nil {
			t.Fatal("Expected error for nil service, got nil")
		}
		if result != nil {
			t.Error("Expected nil result for nil service")
		}
	})

	t.Run("EmptySpec", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed, "should be processed")
		// Default type should be ClusterIP
		testutil.AssertEqual(t, "ClusterIP", result.Values["type"], "default type should be ClusterIP")
	})

	t.Run("ExternalIPs", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"type": "ClusterIP",
				"externalIPs": []interface{}{
					"80.11.12.10",
					"80.11.12.11",
				},
				"ports": []interface{}{
					map[string]interface{}{
						"port":     int64(80),
						"protocol": "TCP",
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		externalIPs, ok := result.Values["externalIPs"].([]string)
		if !ok {
			t.Fatal("Expected externalIPs as []string in values")
		}
		if len(externalIPs) != 2 {
			t.Fatalf("Expected 2 external IPs, got %d", len(externalIPs))
		}
		testutil.AssertEqual(t, "80.11.12.10", externalIPs[0], "first external IP")
		testutil.AssertEqual(t, "80.11.12.11", externalIPs[1], "second external IP")
	})

	t.Run("ClusterIP_Specific", func(t *testing.T) {
		proc := NewServiceProcessor()
		ctx := newTestProcessorContext()

		obj := makeServiceObj("my-svc", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"type":      "ClusterIP",
				"clusterIP": "10.96.0.100",
				"ports": []interface{}{
					map[string]interface{}{
						"port":     int64(80),
						"protocol": "TCP",
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, "10.96.0.100", result.Values["clusterIP"],
			"specific clusterIP should be preserved")
	})
}

// ============================================================
// Result metadata and template tests
// ============================================================

func TestProcessService_ResultMetadata(t *testing.T) {
	proc := NewServiceProcessor()
	ctx := newTestProcessorContext()

	obj := makeServiceObj("my-app-service", "production",
		map[string]interface{}{"app": "my-app"}, nil,
		makeBasicServiceSpec())

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	testutil.AssertEqual(t, true, result.Processed, "should be processed")
	testutil.AssertEqual(t, "my-app", result.ServiceName, "service name from labels")
	testutil.AssertEqual(t, "templates/my-app-service.yaml", result.TemplatePath, "template path")
	testutil.AssertEqual(t, "services.my-app.service", result.ValuesPath, "values path")
	testutil.AssertEqual(t, "my-app-service", result.Metadata["name"], "metadata name")
	testutil.AssertEqual(t, "production", result.Metadata["namespace"], "metadata namespace")
}

func TestProcessService_GeneratesTemplate(t *testing.T) {
	proc := NewServiceProcessor()
	ctx := processor.Context{ChartName: "myapp"}

	obj := makeServiceObj("my-svc", "default",
		map[string]interface{}{"app": "myapp"}, nil,
		makeBasicServiceSpec())

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tpl := result.TemplateContent
	if tpl == "" {
		t.Fatal("Expected non-empty template content")
	}

	// Verify template structure
	testutil.AssertContains(t, tpl, "apiVersion: v1", "template should have apiVersion")
	testutil.AssertContains(t, tpl, "kind: Service", "template should have kind")
	testutil.AssertContains(t, tpl, "{{ $.Release.Namespace }}", "template should use release namespace")
	testutil.AssertContains(t, tpl, `include "myapp.labels"`, "template should include labels helper")
	testutil.AssertContains(t, tpl, `include "myapp.selectorLabels"`, "template should include selectorLabels helper")
	testutil.AssertContains(t, tpl, `.type`, "template should reference type")
	testutil.AssertContains(t, tpl, `.ports`, "template should reference ports")
}

// ============================================================
// Fixture-based smoke test
// ============================================================

func TestProcessService_Fixture(t *testing.T) {
	proc := NewServiceProcessor()
	ctx := processor.Context{ChartName: "nginx-chart"}

	obj := testutil.LoadYAMLFixture(t, "service.yaml")

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	// Basic result checks
	testutil.AssertEqual(t, true, result.Processed, "should be processed")
	testutil.AssertEqual(t, "nginx", result.ServiceName, "service name from app label")

	// Type
	testutil.AssertEqual(t, "ClusterIP", result.Values["type"], "service type from fixture")

	// Ports
	ports, ok := result.Values["ports"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected ports array in values")
	}
	if len(ports) != 2 {
		t.Fatalf("Expected 2 ports in fixture, got %d", len(ports))
	}
	testutil.AssertEqual(t, "http", ports[0]["name"], "first port name from fixture")
	testutil.AssertEqual(t, int64(80), ports[0]["port"], "first port number from fixture")
	testutil.AssertEqual(t, "metrics", ports[1]["name"], "second port name from fixture")
	testutil.AssertEqual(t, int64(9113), ports[1]["port"], "second port number from fixture")

	// Selector
	selector, ok := result.Values["selector"].(map[string]string)
	if !ok {
		t.Fatal("Expected selector in values")
	}
	testutil.AssertEqual(t, "nginx", selector["app"], "selector from fixture")

	// Session affinity
	testutil.AssertEqual(t, "ClientIP", result.Values["sessionAffinity"], "session affinity from fixture")

	// Annotations
	annotations, ok := result.Values["annotations"].(map[string]string)
	if !ok {
		t.Fatal("Expected annotations in values")
	}
	testutil.AssertEqual(t, "true", annotations["prometheus.io/scrape"], "annotation from fixture")

	// Dependencies (app=nginx should create Deployment dependency)
	if !hasDependency(result.Dependencies, "Deployment", "default", "nginx") {
		t.Error("Expected Deployment dependency for 'nginx' from selector")
	}

	// Template not empty
	if result.TemplateContent == "" {
		t.Error("Expected non-empty template content")
	}
}

// ============================================================
// Constructor test
// ============================================================

func TestNewServiceProcessor(t *testing.T) {
	proc := NewServiceProcessor()

	testutil.AssertEqual(t, "service", proc.Name(), "processor name")
	testutil.AssertEqual(t, 100, proc.Priority(), "processor priority")

	gvks := proc.Supports()
	if len(gvks) != 1 {
		t.Fatalf("Expected 1 supported GVK, got %d", len(gvks))
	}
	expected := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

// Ensure unused imports don't cause compilation errors
var _ = strings.Contains
