package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// Test helpers
// ============================================================

func newTestProcessorContext() processor.Context {
	return processor.Context{
		ChartName: "test-chart",
	}
}

// makeDeploymentObj creates an unstructured Deployment for testing.
func makeDeploymentObj(name, namespace string, labels map[string]interface{}, spec map[string]interface{}) *unstructured.Unstructured {
	metadata := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
	}
	if labels != nil {
		metadata["labels"] = labels
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   metadata,
			"spec":       spec,
		},
	}
}

// makeBasicSpec creates a minimal deployment spec with a single container.
func makeBasicSpec(replicas int64, containerName, image string) map[string]interface{} {
	return map[string]interface{}{
		"replicas": replicas,
		"template": map[string]interface{}{
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"app": containerName,
				},
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  containerName,
						"image": image,
					},
				},
			},
		},
	}
}

// hasDependency checks if a dependency with given kind, namespace, name exists.
func hasDependency(deps []types.ResourceKey, kind, namespace, name string) bool {
	for _, dep := range deps {
		if dep.GVK.Kind == kind && dep.Namespace == namespace && dep.Name == name {
			return true
		}
	}
	return false
}

// ============================================================
// Subtask 1: Extract replicas field
// ============================================================

func TestProcessDeployment_ExtractsReplicas(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		proc := NewDeploymentProcessor()
		ctx := newTestProcessorContext()

		spec := makeBasicSpec(3, "nginx", "nginx:1.21")
		obj := makeDeploymentObj("test-deploy", "default",
			map[string]interface{}{"app": "test-deploy"}, spec)

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		replicas, ok := result.Values["replicas"]
		if !ok {
			t.Fatal("Expected 'replicas' key in values")
		}
		testutil.AssertEqual(t, int64(3), replicas, "replicas should be 3")
	})

	t.Run("Nil", func(t *testing.T) {
		proc := NewDeploymentProcessor()
		ctx := newTestProcessorContext()

		// Deployment without replicas field
		obj := makeDeploymentObj("test-deploy", "default",
			map[string]interface{}{"app": "test-deploy"},
			map[string]interface{}{
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{"app": "test-deploy"},
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "nginx",
								"image": "nginx:latest",
							},
						},
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		// When replicas is not specified, expect default value of 1
		replicas, ok := result.Values["replicas"]
		if !ok {
			t.Fatal("Expected 'replicas' key with default value when not specified")
		}
		testutil.AssertEqual(t, int64(1), replicas, "replicas should default to 1")
	})
}

// ============================================================
// Subtask 2: Extract container image
// ============================================================

func TestProcessDeployment_ExtractsImage(t *testing.T) {
	t.Run("SingleContainer", func(t *testing.T) {
		proc := NewDeploymentProcessor()
		ctx := newTestProcessorContext()

		spec := makeBasicSpec(1, "nginx", "nginx:1.21")
		obj := makeDeploymentObj("test-deploy", "default",
			map[string]interface{}{"app": "test-deploy"}, spec)

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		containers, ok := result.Values["containers"].([]map[string]interface{})
		if !ok || len(containers) == 0 {
			t.Fatal("Expected containers array in values")
		}

		img, ok := containers[0]["image"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected image map in first container")
		}
		testutil.AssertEqual(t, "nginx", img["repository"], "image repository")
		testutil.AssertEqual(t, "1.21", img["tag"], "image tag")
	})

	t.Run("MultipleContainers", func(t *testing.T) {
		proc := NewDeploymentProcessor()
		ctx := newTestProcessorContext()

		obj := makeDeploymentObj("test-deploy", "default",
			map[string]interface{}{"app": "test-deploy"},
			map[string]interface{}{
				"replicas": int64(1),
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{"app": "test-deploy"},
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "app",
								"image": "myapp:v2.0",
							},
							map[string]interface{}{
								"name":  "sidecar",
								"image": "envoy:1.28",
							},
						},
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		containers, ok := result.Values["containers"].([]map[string]interface{})
		if !ok {
			t.Fatal("Expected containers array in values")
		}
		if len(containers) != 2 {
			t.Fatalf("Expected 2 containers, got %d", len(containers))
		}

		img1 := containers[0]["image"].(map[string]interface{})
		testutil.AssertEqual(t, "myapp", img1["repository"], "first container image repo")
		testutil.AssertEqual(t, "v2.0", img1["tag"], "first container image tag")

		img2 := containers[1]["image"].(map[string]interface{})
		testutil.AssertEqual(t, "envoy", img2["repository"], "second container image repo")
		testutil.AssertEqual(t, "1.28", img2["tag"], "second container image tag")
	})

	t.Run("NoTag", func(t *testing.T) {
		proc := NewDeploymentProcessor()
		ctx := newTestProcessorContext()

		spec := makeBasicSpec(1, "nginx", "nginx")
		obj := makeDeploymentObj("test-deploy", "default",
			map[string]interface{}{"app": "test-deploy"}, spec)

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		containers := result.Values["containers"].([]map[string]interface{})
		img := containers[0]["image"].(map[string]interface{})
		testutil.AssertEqual(t, "nginx", img["repository"], "image repository")
		testutil.AssertEqual(t, "latest", img["tag"], "image tag should default to latest")
	})
}

// ============================================================
// Subtask 3: Extract resource limits/requests
// ============================================================

func TestProcessDeployment_ExtractsResources(t *testing.T) {
	t.Run("LimitsAndRequests", func(t *testing.T) {
		proc := NewDeploymentProcessor()
		ctx := newTestProcessorContext()

		obj := makeDeploymentObj("test-deploy", "default",
			map[string]interface{}{"app": "test-deploy"},
			map[string]interface{}{
				"replicas": int64(1),
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{"app": "test-deploy"},
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "nginx",
								"image": "nginx:1.21",
								"resources": map[string]interface{}{
									"limits": map[string]interface{}{
										"cpu":    "1",
										"memory": "1Gi",
									},
									"requests": map[string]interface{}{
										"cpu":    "500m",
										"memory": "512Mi",
									},
								},
							},
						},
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		containers := result.Values["containers"].([]map[string]interface{})
		resources, ok := containers[0]["resources"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected resources in container values")
		}

		limits := resources["limits"].(map[string]interface{})
		testutil.AssertEqual(t, "1", limits["cpu"], "CPU limit")
		testutil.AssertEqual(t, "1Gi", limits["memory"], "memory limit")

		requests := resources["requests"].(map[string]interface{})
		testutil.AssertEqual(t, "500m", requests["cpu"], "CPU request")
		testutil.AssertEqual(t, "512Mi", requests["memory"], "memory request")
	})

	t.Run("MissingLimits", func(t *testing.T) {
		proc := NewDeploymentProcessor()
		ctx := newTestProcessorContext()

		obj := makeDeploymentObj("test-deploy", "default",
			map[string]interface{}{"app": "test-deploy"},
			map[string]interface{}{
				"replicas": int64(1),
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{"app": "test-deploy"},
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "nginx",
								"image": "nginx:1.21",
								"resources": map[string]interface{}{
									"requests": map[string]interface{}{
										"cpu":    "500m",
										"memory": "512Mi",
									},
								},
							},
						},
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		containers := result.Values["containers"].([]map[string]interface{})
		resources := containers[0]["resources"].(map[string]interface{})

		_, hasLimits := resources["limits"]
		if hasLimits {
			t.Error("Expected no 'limits' key when limits not specified")
		}

		_, hasRequests := resources["requests"]
		if !hasRequests {
			t.Error("Expected 'requests' to be present")
		}
	})
}

// ============================================================
// Subtask 4: Extract pod labels
// ============================================================

func TestProcessDeployment_ExtractsPodLabels(t *testing.T) {
	proc := NewDeploymentProcessor()
	ctx := newTestProcessorContext()

	obj := makeDeploymentObj("test-deploy", "default",
		map[string]interface{}{"app": "test-deploy"},
		map[string]interface{}{
			"replicas": int64(1),
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"app":     "myapp",
						"version": "v1",
						"tier":    "frontend",
					},
				},
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "nginx",
							"image": "nginx:latest",
						},
					},
				},
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	podLabels, ok := result.Values["podLabels"].(map[string]string)
	if !ok {
		t.Fatal("Expected podLabels as map[string]string in values")
	}
	testutil.AssertEqual(t, "myapp", podLabels["app"], "pod label 'app'")
	testutil.AssertEqual(t, "v1", podLabels["version"], "pod label 'version'")
	testutil.AssertEqual(t, "frontend", podLabels["tier"], "pod label 'tier'")
}

// ============================================================
// Subtask 5: Extract pod annotations
// ============================================================

func TestProcessDeployment_ExtractsPodAnnotations(t *testing.T) {
	proc := NewDeploymentProcessor()
	ctx := newTestProcessorContext()

	obj := makeDeploymentObj("test-deploy", "default",
		map[string]interface{}{"app": "test-deploy"},
		map[string]interface{}{
			"replicas": int64(1),
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{"app": "test-deploy"},
					"annotations": map[string]interface{}{
						"prometheus.io/scrape": "true",
						"prometheus.io/port":   "9090",
					},
				},
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "nginx",
							"image": "nginx:latest",
						},
					},
				},
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	podAnnotations, ok := result.Values["podAnnotations"].(map[string]string)
	if !ok {
		t.Fatal("Expected podAnnotations as map[string]string in values")
	}
	testutil.AssertEqual(t, "true", podAnnotations["prometheus.io/scrape"], "prometheus scrape")
	testutil.AssertEqual(t, "9090", podAnnotations["prometheus.io/port"], "prometheus port")
}

// ============================================================
// Subtask 6: Extract node affinity
// ============================================================

func TestProcessDeployment_ExtractsAffinity(t *testing.T) {
	t.Run("NodeAffinity", func(t *testing.T) {
		proc := NewDeploymentProcessor()
		ctx := newTestProcessorContext()

		affinity := map[string]interface{}{
			"nodeAffinity": map[string]interface{}{
				"requiredDuringSchedulingIgnoredDuringExecution": map[string]interface{}{
					"nodeSelectorTerms": []interface{}{
						map[string]interface{}{
							"matchExpressions": []interface{}{
								map[string]interface{}{
									"key":      "kubernetes.io/os",
									"operator": "In",
									"values":   []interface{}{"linux"},
								},
							},
						},
					},
				},
			},
		}

		obj := makeDeploymentObj("test-deploy", "default",
			map[string]interface{}{"app": "test-deploy"},
			map[string]interface{}{
				"replicas": int64(1),
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{"app": "test-deploy"},
					},
					"spec": map[string]interface{}{
						"affinity": affinity,
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "nginx",
								"image": "nginx:latest",
							},
						},
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		aff, ok := result.Values["affinity"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected affinity in values")
		}
		if _, ok := aff["nodeAffinity"]; !ok {
			t.Fatal("Expected nodeAffinity in affinity structure")
		}
	})

	// ============================================================
	// Subtask 7: Extract pod affinity/anti-affinity
	// ============================================================

	t.Run("PodAntiAffinity", func(t *testing.T) {
		proc := NewDeploymentProcessor()
		ctx := newTestProcessorContext()

		affinity := map[string]interface{}{
			"podAntiAffinity": map[string]interface{}{
				"preferredDuringSchedulingIgnoredDuringExecution": []interface{}{
					map[string]interface{}{
						"weight": int64(100),
						"podAffinityTerm": map[string]interface{}{
							"labelSelector": map[string]interface{}{
								"matchExpressions": []interface{}{
									map[string]interface{}{
										"key":      "app",
										"operator": "In",
										"values":   []interface{}{"myapp"},
									},
								},
							},
							"topologyKey": "kubernetes.io/hostname",
						},
					},
				},
			},
		}

		obj := makeDeploymentObj("test-deploy", "default",
			map[string]interface{}{"app": "test-deploy"},
			map[string]interface{}{
				"replicas": int64(1),
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{"app": "test-deploy"},
					},
					"spec": map[string]interface{}{
						"affinity": affinity,
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "nginx",
								"image": "nginx:latest",
							},
						},
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		aff, ok := result.Values["affinity"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected affinity in values")
		}
		if _, ok := aff["podAntiAffinity"]; !ok {
			t.Fatal("Expected podAntiAffinity in affinity structure")
		}
	})
}

// ============================================================
// Subtask 8: Extract tolerations
// ============================================================

func TestProcessDeployment_ExtractsTolerations(t *testing.T) {
	proc := NewDeploymentProcessor()
	ctx := newTestProcessorContext()

	tolerations := []interface{}{
		map[string]interface{}{
			"key":      "node-role.kubernetes.io/master",
			"operator": "Exists",
			"effect":   "NoSchedule",
		},
		map[string]interface{}{
			"key":      "dedicated",
			"operator": "Equal",
			"value":    "special-user",
			"effect":   "NoSchedule",
		},
	}

	obj := makeDeploymentObj("test-deploy", "default",
		map[string]interface{}{"app": "test-deploy"},
		map[string]interface{}{
			"replicas": int64(1),
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{"app": "test-deploy"},
				},
				"spec": map[string]interface{}{
					"tolerations": tolerations,
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "nginx",
							"image": "nginx:latest",
						},
					},
				},
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tols, ok := result.Values["tolerations"].([]interface{})
	if !ok {
		t.Fatal("Expected tolerations as slice in values")
	}
	if len(tols) != 2 {
		t.Fatalf("Expected 2 tolerations, got %d", len(tols))
	}

	firstTol := tols[0].(map[string]interface{})
	testutil.AssertEqual(t, "node-role.kubernetes.io/master", firstTol["key"], "toleration key")
	testutil.AssertEqual(t, "Exists", firstTol["operator"], "toleration operator")
}

// ============================================================
// Subtask 9: Extract securityContext
// ============================================================

func TestProcessDeployment_ExtractsSecurityContext(t *testing.T) {
	t.Run("Pod", func(t *testing.T) {
		proc := NewDeploymentProcessor()
		ctx := newTestProcessorContext()

		obj := makeDeploymentObj("test-deploy", "default",
			map[string]interface{}{"app": "test-deploy"},
			map[string]interface{}{
				"replicas": int64(1),
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{"app": "test-deploy"},
					},
					"spec": map[string]interface{}{
						"securityContext": map[string]interface{}{
							"runAsUser":    int64(1000),
							"runAsGroup":   int64(3000),
							"fsGroup":      int64(2000),
							"runAsNonRoot": true,
						},
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "nginx",
								"image": "nginx:latest",
							},
						},
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		sc, ok := result.Values["podSecurityContext"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected 'podSecurityContext' in values")
		}
		testutil.AssertEqual(t, int64(1000), sc["runAsUser"], "runAsUser")
		testutil.AssertEqual(t, int64(3000), sc["runAsGroup"], "runAsGroup")
		testutil.AssertEqual(t, int64(2000), sc["fsGroup"], "fsGroup")
		testutil.AssertEqual(t, true, sc["runAsNonRoot"], "runAsNonRoot")
	})

	t.Run("Container", func(t *testing.T) {
		proc := NewDeploymentProcessor()
		ctx := newTestProcessorContext()

		obj := makeDeploymentObj("test-deploy", "default",
			map[string]interface{}{"app": "test-deploy"},
			map[string]interface{}{
				"replicas": int64(1),
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{"app": "test-deploy"},
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "nginx",
								"image": "nginx:latest",
								"securityContext": map[string]interface{}{
									"runAsUser":                int64(1000),
									"readOnlyRootFilesystem":   true,
									"allowPrivilegeEscalation": false,
								},
							},
						},
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		containers := result.Values["containers"].([]map[string]interface{})
		sc, ok := containers[0]["securityContext"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected 'securityContext' in container values")
		}
		testutil.AssertEqual(t, int64(1000), sc["runAsUser"], "container runAsUser")
		testutil.AssertEqual(t, true, sc["readOnlyRootFilesystem"], "readOnlyRootFilesystem")
		testutil.AssertEqual(t, false, sc["allowPrivilegeEscalation"], "allowPrivilegeEscalation")
	})
}

// ============================================================
// Subtask 10: Extract volumes and volumeMounts
// ============================================================

func TestProcessDeployment_ExtractsVolumes(t *testing.T) {
	t.Run("ConfigMap", func(t *testing.T) {
		proc := NewDeploymentProcessor()
		ctx := newTestProcessorContext()

		obj := makeDeploymentObj("test-deploy", "default",
			map[string]interface{}{"app": "test-deploy"},
			map[string]interface{}{
				"replicas": int64(1),
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{"app": "test-deploy"},
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "nginx",
								"image": "nginx:latest",
								"volumeMounts": []interface{}{
									map[string]interface{}{
										"name":      "config-vol",
										"mountPath": "/etc/config",
									},
								},
							},
						},
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "config-vol",
								"configMap": map[string]interface{}{
									"name": "my-config",
								},
							},
						},
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		// Verify volumes extracted
		volumes, ok := result.Values["volumes"].([]interface{})
		if !ok {
			t.Fatal("Expected volumes in values")
		}
		if len(volumes) != 1 {
			t.Fatalf("Expected 1 volume, got %d", len(volumes))
		}

		vol := volumes[0].(map[string]interface{})
		cm := vol["configMap"].(map[string]interface{})
		testutil.AssertEqual(t, "my-config", cm["name"], "ConfigMap volume name")

		// Verify volumeMounts in container
		containers := result.Values["containers"].([]map[string]interface{})
		mounts := containers[0]["volumeMounts"].([]interface{})
		mount := mounts[0].(map[string]interface{})
		testutil.AssertEqual(t, "config-vol", mount["name"], "volumeMount name")
		testutil.AssertEqual(t, "/etc/config", mount["mountPath"], "volumeMount mountPath")

		// Verify ConfigMap dependency detected
		if !hasDependency(result.Dependencies, "ConfigMap", "default", "my-config") {
			t.Error("Expected ConfigMap dependency for 'my-config'")
		}
	})

	t.Run("Secret", func(t *testing.T) {
		proc := NewDeploymentProcessor()
		ctx := newTestProcessorContext()

		obj := makeDeploymentObj("test-deploy", "default",
			map[string]interface{}{"app": "test-deploy"},
			map[string]interface{}{
				"replicas": int64(1),
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{"app": "test-deploy"},
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "nginx",
								"image": "nginx:latest",
								"volumeMounts": []interface{}{
									map[string]interface{}{
										"name":      "secret-vol",
										"mountPath": "/etc/secrets",
										"readOnly":  true,
									},
								},
							},
						},
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "secret-vol",
								"secret": map[string]interface{}{
									"secretName": "my-secret",
								},
							},
						},
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		// Verify Secret dependency detected
		if !hasDependency(result.Dependencies, "Secret", "default", "my-secret") {
			t.Error("Expected Secret dependency for 'my-secret'")
		}
	})
}

// ============================================================
// Subtask 11: Extract probes (liveness, readiness, startup)
// ============================================================

func TestProcessDeployment_ExtractsProbes(t *testing.T) {
	proc := NewDeploymentProcessor()
	ctx := newTestProcessorContext()

	obj := makeDeploymentObj("test-deploy", "default",
		map[string]interface{}{"app": "test-deploy"},
		map[string]interface{}{
			"replicas": int64(1),
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{"app": "test-deploy"},
				},
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "nginx",
							"image": "nginx:latest",
							"livenessProbe": map[string]interface{}{
								"httpGet": map[string]interface{}{
									"path": "/healthz",
									"port": int64(8080),
								},
								"initialDelaySeconds": int64(30),
								"periodSeconds":       int64(10),
							},
							"readinessProbe": map[string]interface{}{
								"httpGet": map[string]interface{}{
									"path": "/ready",
									"port": int64(8080),
								},
								"initialDelaySeconds": int64(5),
								"periodSeconds":       int64(5),
							},
							"startupProbe": map[string]interface{}{
								"httpGet": map[string]interface{}{
									"path": "/startup",
									"port": int64(8080),
								},
								"failureThreshold": int64(30),
								"periodSeconds":    int64(10),
							},
						},
					},
				},
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	containers := result.Values["containers"].([]map[string]interface{})

	// Liveness probe
	lp, ok := containers[0]["livenessProbe"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected livenessProbe in container values")
	}
	lpHTTP := lp["httpGet"].(map[string]interface{})
	testutil.AssertEqual(t, "/healthz", lpHTTP["path"], "liveness probe path")

	// Readiness probe
	rp, ok := containers[0]["readinessProbe"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected readinessProbe in container values")
	}
	rpHTTP := rp["httpGet"].(map[string]interface{})
	testutil.AssertEqual(t, "/ready", rpHTTP["path"], "readiness probe path")

	// Startup probe
	sp, ok := containers[0]["startupProbe"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected startupProbe in container values")
	}
	spHTTP := sp["httpGet"].(map[string]interface{})
	testutil.AssertEqual(t, "/startup", spHTTP["path"], "startup probe path")
}

// ============================================================
// Subtask 12: Edge cases
// ============================================================

func TestProcessDeployment_EdgeCases(t *testing.T) {
	t.Run("NilDeployment", func(t *testing.T) {
		proc := NewDeploymentProcessor()
		ctx := newTestProcessorContext()

		result, err := proc.Process(ctx, nil)
		if err == nil {
			t.Fatal("Expected error for nil deployment, got nil")
		}
		if result != nil {
			t.Error("Expected nil result for nil deployment")
		}
	})

	t.Run("EmptySpec", func(t *testing.T) {
		proc := NewDeploymentProcessor()
		ctx := newTestProcessorContext()

		obj := makeDeploymentObj("test-deploy", "default",
			map[string]interface{}{"app": "test-deploy"},
			map[string]interface{}{})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed, "should be processed")

		// Empty spec should have no containers
		_, hasContainers := result.Values["containers"]
		if hasContainers {
			t.Error("Expected no containers in empty spec")
		}
	})

	t.Run("NoContainers", func(t *testing.T) {
		proc := NewDeploymentProcessor()
		ctx := newTestProcessorContext()

		obj := makeDeploymentObj("test-deploy", "default",
			map[string]interface{}{"app": "test-deploy"},
			map[string]interface{}{
				"replicas": int64(1),
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{"app": "test-deploy"},
					},
					"spec": map[string]interface{}{},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		_, hasContainers := result.Values["containers"]
		if hasContainers {
			t.Error("Expected no containers key when no containers defined")
		}
	})
}

// ============================================================
// Additional tests: Strategy, NodeSelector, Dependencies
// ============================================================

func TestProcessDeployment_ExtractsStrategy(t *testing.T) {
	proc := NewDeploymentProcessor()
	ctx := newTestProcessorContext()

	obj := makeDeploymentObj("test-deploy", "default",
		map[string]interface{}{"app": "test-deploy"},
		map[string]interface{}{
			"replicas": int64(1),
			"strategy": map[string]interface{}{
				"type": "RollingUpdate",
				"rollingUpdate": map[string]interface{}{
					"maxSurge":       "25%",
					"maxUnavailable": "25%",
				},
			},
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{"app": "test-deploy"},
				},
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "nginx",
							"image": "nginx:latest",
						},
					},
				},
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	strategy, ok := result.Values["strategy"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected strategy in values")
	}
	testutil.AssertEqual(t, "RollingUpdate", strategy["type"], "strategy type")

	ru, ok := strategy["rollingUpdate"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected rollingUpdate in strategy")
	}
	testutil.AssertEqual(t, "25%", ru["maxSurge"], "maxSurge")
}

func TestProcessDeployment_ExtractsNodeSelector(t *testing.T) {
	proc := NewDeploymentProcessor()
	ctx := newTestProcessorContext()

	obj := makeDeploymentObj("test-deploy", "default",
		map[string]interface{}{"app": "test-deploy"},
		map[string]interface{}{
			"replicas": int64(1),
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{"app": "test-deploy"},
				},
				"spec": map[string]interface{}{
					"nodeSelector": map[string]interface{}{
						"disktype": "ssd",
						"region":   "us-east",
					},
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "nginx",
							"image": "nginx:latest",
						},
					},
				},
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	ns, ok := result.Values["nodeSelector"].(map[string]string)
	if !ok {
		t.Fatal("Expected nodeSelector as map[string]string in values")
	}
	testutil.AssertEqual(t, "ssd", ns["disktype"], "nodeSelector disktype")
	testutil.AssertEqual(t, "us-east", ns["region"], "nodeSelector region")
}

// ============================================================
// Dependency detection tests
// ============================================================

func TestProcessDeployment_DetectsDependencies(t *testing.T) {
	t.Run("ServiceAccount", func(t *testing.T) {
		proc := NewDeploymentProcessor()
		ctx := newTestProcessorContext()

		obj := makeDeploymentObj("test-deploy", "default",
			map[string]interface{}{"app": "test-deploy"},
			map[string]interface{}{
				"replicas": int64(1),
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{"app": "test-deploy"},
					},
					"spec": map[string]interface{}{
						"serviceAccountName": "my-sa",
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "nginx",
								"image": "nginx:latest",
							},
						},
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		testutil.AssertEqual(t, "my-sa", result.Values["serviceAccountName"], "serviceAccountName")
		if !hasDependency(result.Dependencies, "ServiceAccount", "default", "my-sa") {
			t.Error("Expected ServiceAccount dependency for 'my-sa'")
		}
	})

	t.Run("ImagePullSecrets", func(t *testing.T) {
		proc := NewDeploymentProcessor()
		ctx := newTestProcessorContext()

		obj := makeDeploymentObj("test-deploy", "default",
			map[string]interface{}{"app": "test-deploy"},
			map[string]interface{}{
				"replicas": int64(1),
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{"app": "test-deploy"},
					},
					"spec": map[string]interface{}{
						"imagePullSecrets": []interface{}{
							map[string]interface{}{"name": "regcred"},
							map[string]interface{}{"name": "docker-registry"},
						},
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "nginx",
								"image": "nginx:latest",
							},
						},
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		if !hasDependency(result.Dependencies, "Secret", "default", "regcred") {
			t.Error("Expected Secret dependency for 'regcred'")
		}
		if !hasDependency(result.Dependencies, "Secret", "default", "docker-registry") {
			t.Error("Expected Secret dependency for 'docker-registry'")
		}
	})

	t.Run("EnvConfigMap", func(t *testing.T) {
		proc := NewDeploymentProcessor()
		ctx := newTestProcessorContext()

		obj := makeDeploymentObj("test-deploy", "default",
			map[string]interface{}{"app": "test-deploy"},
			map[string]interface{}{
				"replicas": int64(1),
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{"app": "test-deploy"},
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "nginx",
								"image": "nginx:latest",
								"env": []interface{}{
									map[string]interface{}{
										"name": "CONFIG_VAR",
										"valueFrom": map[string]interface{}{
											"configMapKeyRef": map[string]interface{}{
												"name": "app-config",
												"key":  "config-key",
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

		if !hasDependency(result.Dependencies, "ConfigMap", "default", "app-config") {
			t.Error("Expected ConfigMap dependency for 'app-config'")
		}
	})

	t.Run("EnvSecret", func(t *testing.T) {
		proc := NewDeploymentProcessor()
		ctx := newTestProcessorContext()

		obj := makeDeploymentObj("test-deploy", "default",
			map[string]interface{}{"app": "test-deploy"},
			map[string]interface{}{
				"replicas": int64(1),
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{"app": "test-deploy"},
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "nginx",
								"image": "nginx:latest",
								"env": []interface{}{
									map[string]interface{}{
										"name": "DB_PASSWORD",
										"valueFrom": map[string]interface{}{
											"secretKeyRef": map[string]interface{}{
												"name": "db-credentials",
												"key":  "password",
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

		if !hasDependency(result.Dependencies, "Secret", "default", "db-credentials") {
			t.Error("Expected Secret dependency for 'db-credentials'")
		}
	})

	t.Run("EnvFrom", func(t *testing.T) {
		proc := NewDeploymentProcessor()
		ctx := newTestProcessorContext()

		obj := makeDeploymentObj("test-deploy", "default",
			map[string]interface{}{"app": "test-deploy"},
			map[string]interface{}{
				"replicas": int64(1),
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{"app": "test-deploy"},
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "nginx",
								"image": "nginx:latest",
								"envFrom": []interface{}{
									map[string]interface{}{
										"configMapRef": map[string]interface{}{
											"name": "env-config",
										},
									},
									map[string]interface{}{
										"secretRef": map[string]interface{}{
											"name": "env-secrets",
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

		if !hasDependency(result.Dependencies, "ConfigMap", "default", "env-config") {
			t.Error("Expected ConfigMap dependency for 'env-config'")
		}
		if !hasDependency(result.Dependencies, "Secret", "default", "env-secrets") {
			t.Error("Expected Secret dependency for 'env-secrets'")
		}
	})

	t.Run("PVC", func(t *testing.T) {
		proc := NewDeploymentProcessor()
		ctx := newTestProcessorContext()

		obj := makeDeploymentObj("test-deploy", "default",
			map[string]interface{}{"app": "test-deploy"},
			map[string]interface{}{
				"replicas": int64(1),
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{"app": "test-deploy"},
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "nginx",
								"image": "nginx:latest",
							},
						},
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "data-vol",
								"persistentVolumeClaim": map[string]interface{}{
									"claimName": "my-pvc",
								},
							},
						},
					},
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		if !hasDependency(result.Dependencies, "PersistentVolumeClaim", "default", "my-pvc") {
			t.Error("Expected PVC dependency for 'my-pvc'")
		}
	})
}

// ============================================================
// Result metadata and template tests
// ============================================================

func TestProcessDeployment_ResultMetadata(t *testing.T) {
	proc := NewDeploymentProcessor()
	ctx := newTestProcessorContext()

	spec := makeBasicSpec(1, "nginx", "nginx:latest")
	obj := makeDeploymentObj("my-app-deployment", "production",
		map[string]interface{}{"app": "my-app"}, spec)

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	testutil.AssertEqual(t, true, result.Processed, "should be processed")
	testutil.AssertEqual(t, "my-app", result.ServiceName, "service name from labels")
	testutil.AssertEqual(t, "templates/my-app-deployment.yaml", result.TemplatePath, "template path")
	testutil.AssertEqual(t, "services.my-app.deployment", result.ValuesPath, "values path")
	testutil.AssertEqual(t, "my-app-deployment", result.Metadata["name"], "metadata name")
	testutil.AssertEqual(t, "production", result.Metadata["namespace"], "metadata namespace")
}

func TestProcessDeployment_GeneratesTemplate(t *testing.T) {
	proc := NewDeploymentProcessor()
	ctx := processor.Context{ChartName: "myapp"}

	spec := makeBasicSpec(1, "nginx", "nginx:latest")
	obj := makeDeploymentObj("test-deploy", "default",
		map[string]interface{}{"app": "test-deploy"}, spec)

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tpl := result.TemplateContent
	if tpl == "" {
		t.Fatal("Expected non-empty template content")
	}

	// Verify template structure
	testutil.AssertContains(t, tpl, "apiVersion: apps/v1", "template should have apiVersion")
	testutil.AssertContains(t, tpl, "kind: Deployment", "template should have kind")
	testutil.AssertContains(t, tpl, "{{ $.Release.Namespace }}", "template should use release namespace")
	testutil.AssertContains(t, tpl, `include "myapp.labels"`, "template should include labels helper")
	testutil.AssertContains(t, tpl, `include "myapp.selectorLabels"`, "template should include selectorLabels helper")
	testutil.AssertContains(t, tpl, `include "myapp.fullname"`, "template should include fullname helper")
	testutil.AssertContains(t, tpl, ".replicas", "template should reference replicas")
	testutil.AssertContains(t, tpl, ".image.repository", "template should reference image repo")
	testutil.AssertContains(t, tpl, ".image.tag", "template should reference image tag")
}

// ============================================================
// Fixture-based smoke test
// ============================================================

func TestProcessDeployment_Fixture(t *testing.T) {
	proc := NewDeploymentProcessor()
	ctx := processor.Context{ChartName: "nginx-chart"}

	obj := testutil.LoadYAMLFixture(t, "deployment.yaml")

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	// Basic result checks
	testutil.AssertEqual(t, true, result.Processed, "should be processed")
	testutil.AssertEqual(t, "nginx", result.ServiceName, "service name from app label")

	// Replicas
	_, hasReplicas := result.Values["replicas"]
	if !hasReplicas {
		t.Error("Expected replicas in values")
	}

	// Containers
	containers, ok := result.Values["containers"].([]map[string]interface{})
	if !ok || len(containers) == 0 {
		t.Fatal("Expected at least 1 container in values")
	}
	testutil.AssertEqual(t, "nginx", containers[0]["name"], "container name")

	img := containers[0]["image"].(map[string]interface{})
	testutil.AssertEqual(t, "nginx", img["repository"], "image repository from fixture")
	testutil.AssertEqual(t, "1.25.0", img["tag"], "image tag from fixture")

	// Dependencies (ConfigMap nginx-config from env + volume)
	if !hasDependency(result.Dependencies, "ConfigMap", "default", "nginx-config") {
		t.Error("Expected ConfigMap dependency for 'nginx-config'")
	}

	// Template not empty
	if result.TemplateContent == "" {
		t.Error("Expected non-empty template content")
	}
}

// ============================================================
// Constructor test
// ============================================================

func TestNewDeploymentProcessor(t *testing.T) {
	proc := NewDeploymentProcessor()

	testutil.AssertEqual(t, "deployment", proc.Name(), "processor name")
	testutil.AssertEqual(t, 100, proc.Priority(), "processor priority")

	gvks := proc.Supports()
	if len(gvks) != 1 {
		t.Fatalf("Expected 1 supported GVK, got %d", len(gvks))
	}
	expected := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

// ============================================================
// Direct tests for helper functions
// ============================================================

func TestParseImage(t *testing.T) {
	tests := []struct {
		name       string
		image      string
		wantRepo   string
		wantTag    string
	}{
		{"WithTag", "nginx:1.21", "nginx", "1.21"},
		{"NoTag", "nginx", "nginx", "latest"},
		{"LatestTag", "nginx:latest", "nginx", "latest"},
		{"WithRegistry", "gcr.io/my-project/app:v2", "gcr.io/my-project/app", "v2"},
		{"Digest", "nginx@sha256:abc123", "nginx", "sha256:abc123"},
		{"RegistryPort", "registry:5000/myapp", "registry:5000/myapp", "latest"},
		{"RegistryPortWithTag", "registry:5000/myapp:v1", "registry:5000/myapp", "v1"},
		{"PrivateRegistry", "my.registry.io/org/image:1.0", "my.registry.io/org/image", "1.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, tag := parseImage(tt.image)
			testutil.AssertEqual(t, tt.wantRepo, repo, "repository for %q", tt.image)
			testutil.AssertEqual(t, tt.wantTag, tag, "tag for %q", tt.image)
		})
	}
}

func TestExtractEnvDependencies(t *testing.T) {
	t.Run("ConfigMapRef", func(t *testing.T) {
		env := []interface{}{
			map[string]interface{}{
				"name": "MY_VAR",
				"valueFrom": map[string]interface{}{
					"configMapKeyRef": map[string]interface{}{
						"name": "test-cm",
						"key":  "my-key",
					},
				},
			},
		}

		deps := extractEnvDependencies(env, "default")
		if len(deps) != 1 {
			t.Fatalf("Expected 1 dependency, got %d", len(deps))
		}
		testutil.AssertEqual(t, "ConfigMap", deps[0].GVK.Kind, "dependency kind")
		testutil.AssertEqual(t, "test-cm", deps[0].Name, "dependency name")
		testutil.AssertEqual(t, "default", deps[0].Namespace, "dependency namespace")
	})

	t.Run("SecretRef", func(t *testing.T) {
		env := []interface{}{
			map[string]interface{}{
				"name": "MY_SECRET",
				"valueFrom": map[string]interface{}{
					"secretKeyRef": map[string]interface{}{
						"name": "test-secret",
						"key":  "secret-key",
					},
				},
			},
		}

		deps := extractEnvDependencies(env, "prod")
		if len(deps) != 1 {
			t.Fatalf("Expected 1 dependency, got %d", len(deps))
		}
		testutil.AssertEqual(t, "Secret", deps[0].GVK.Kind, "dependency kind")
		testutil.AssertEqual(t, "test-secret", deps[0].Name, "dependency name")
		testutil.AssertEqual(t, "prod", deps[0].Namespace, "dependency namespace")
	})

	t.Run("PlainValue_NoDeps", func(t *testing.T) {
		env := []interface{}{
			map[string]interface{}{
				"name":  "PLAIN",
				"value": "hello",
			},
		}

		deps := extractEnvDependencies(env, "default")
		if len(deps) != 0 {
			t.Errorf("Expected 0 dependencies for plain value, got %d", len(deps))
		}
	})

	t.Run("InvalidType_NoDeps", func(t *testing.T) {
		env := []interface{}{
			"not-a-map",
		}

		deps := extractEnvDependencies(env, "default")
		if len(deps) != 0 {
			t.Errorf("Expected 0 dependencies for invalid type, got %d", len(deps))
		}
	})
}

func TestExtractEnvFromDependencies(t *testing.T) {
	t.Run("ConfigMapRef", func(t *testing.T) {
		envFrom := []interface{}{
			map[string]interface{}{
				"configMapRef": map[string]interface{}{
					"name": "env-cm",
				},
			},
		}

		deps := extractEnvFromDependencies(envFrom, "default")
		if len(deps) != 1 {
			t.Fatalf("Expected 1 dependency, got %d", len(deps))
		}
		testutil.AssertEqual(t, "ConfigMap", deps[0].GVK.Kind, "dependency kind")
		testutil.AssertEqual(t, "env-cm", deps[0].Name, "dependency name")
	})

	t.Run("SecretRef", func(t *testing.T) {
		envFrom := []interface{}{
			map[string]interface{}{
				"secretRef": map[string]interface{}{
					"name": "env-secret",
				},
			},
		}

		deps := extractEnvFromDependencies(envFrom, "default")
		if len(deps) != 1 {
			t.Fatalf("Expected 1 dependency, got %d", len(deps))
		}
		testutil.AssertEqual(t, "Secret", deps[0].GVK.Kind, "dependency kind")
		testutil.AssertEqual(t, "env-secret", deps[0].Name, "dependency name")
	})

	t.Run("Mixed", func(t *testing.T) {
		envFrom := []interface{}{
			map[string]interface{}{
				"configMapRef": map[string]interface{}{"name": "cm1"},
			},
			map[string]interface{}{
				"secretRef": map[string]interface{}{"name": "s1"},
			},
		}

		deps := extractEnvFromDependencies(envFrom, "ns1")
		if len(deps) != 2 {
			t.Fatalf("Expected 2 dependencies, got %d", len(deps))
		}
	})
}

func TestExtractVolumeDependencies(t *testing.T) {
	t.Run("ConfigMap", func(t *testing.T) {
		volumes := []interface{}{
			map[string]interface{}{
				"name": "vol1",
				"configMap": map[string]interface{}{
					"name": "vol-cm",
				},
			},
		}

		deps := extractVolumeDependencies(volumes, "default")
		if len(deps) != 1 {
			t.Fatalf("Expected 1 dependency, got %d", len(deps))
		}
		testutil.AssertEqual(t, "ConfigMap", deps[0].GVK.Kind, "dependency kind")
		testutil.AssertEqual(t, "vol-cm", deps[0].Name, "dependency name")
	})

	t.Run("Secret", func(t *testing.T) {
		volumes := []interface{}{
			map[string]interface{}{
				"name": "vol1",
				"secret": map[string]interface{}{
					"secretName": "vol-secret",
				},
			},
		}

		deps := extractVolumeDependencies(volumes, "default")
		if len(deps) != 1 {
			t.Fatalf("Expected 1 dependency, got %d", len(deps))
		}
		testutil.AssertEqual(t, "Secret", deps[0].GVK.Kind, "dependency kind")
		testutil.AssertEqual(t, "vol-secret", deps[0].Name, "dependency name")
	})

	t.Run("PVC", func(t *testing.T) {
		volumes := []interface{}{
			map[string]interface{}{
				"name": "vol1",
				"persistentVolumeClaim": map[string]interface{}{
					"claimName": "my-pvc",
				},
			},
		}

		deps := extractVolumeDependencies(volumes, "default")
		if len(deps) != 1 {
			t.Fatalf("Expected 1 dependency, got %d", len(deps))
		}
		testutil.AssertEqual(t, "PersistentVolumeClaim", deps[0].GVK.Kind, "dependency kind")
		testutil.AssertEqual(t, "my-pvc", deps[0].Name, "dependency name")
	})

	t.Run("EmptyDir_NoDeps", func(t *testing.T) {
		volumes := []interface{}{
			map[string]interface{}{
				"name":     "cache",
				"emptyDir": map[string]interface{}{},
			},
		}

		deps := extractVolumeDependencies(volumes, "default")
		if len(deps) != 0 {
			t.Errorf("Expected 0 dependencies for emptyDir, got %d", len(deps))
		}
	})

	t.Run("Mixed", func(t *testing.T) {
		volumes := []interface{}{
			map[string]interface{}{
				"name":     "cache",
				"emptyDir": map[string]interface{}{},
			},
			map[string]interface{}{
				"name":     "config",
				"configMap": map[string]interface{}{"name": "cfg"},
			},
			map[string]interface{}{
				"name":   "creds",
				"secret": map[string]interface{}{"secretName": "sec"},
			},
			map[string]interface{}{
				"name":                  "data",
				"persistentVolumeClaim": map[string]interface{}{"claimName": "pvc1"},
			},
		}

		deps := extractVolumeDependencies(volumes, "default")
		if len(deps) != 3 {
			t.Fatalf("Expected 3 dependencies (cm+secret+pvc, not emptyDir), got %d", len(deps))
		}
	})
}

// Ensure unused imports don't cause compilation errors
var _ = strings.Contains
