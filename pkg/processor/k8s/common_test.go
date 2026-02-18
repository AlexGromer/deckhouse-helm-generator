package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Test helpers for common.go processors
// ============================================================

// makeStatefulSetObj creates an unstructured StatefulSet for testing.
func makeStatefulSetObj(name, namespace string, labels map[string]interface{}, spec map[string]interface{}) *unstructured.Unstructured {
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
			"kind":       "StatefulSet",
			"metadata":   metadata,
			"spec":       spec,
		},
	}
}

// makeDaemonSetObj creates an unstructured DaemonSet for testing.
func makeDaemonSetObj(name, namespace string, labels map[string]interface{}, spec map[string]interface{}) *unstructured.Unstructured {
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
			"kind":       "DaemonSet",
			"metadata":   metadata,
			"spec":       spec,
		},
	}
}

// makePVCObj creates an unstructured PersistentVolumeClaim for testing.
func makePVCObj(name, namespace string, labels map[string]interface{}, spec map[string]interface{}) *unstructured.Unstructured {
	metadata := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
	}
	if labels != nil {
		metadata["labels"] = labels
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "PersistentVolumeClaim",
			"metadata":   metadata,
			"spec":       spec,
		},
	}
}

// makeServiceAccountObj creates an unstructured ServiceAccount for testing.
func makeServiceAccountObj(name, namespace string, labels, annotations map[string]interface{}, fields map[string]interface{}) *unstructured.Unstructured {
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
	obj := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ServiceAccount",
		"metadata":   metadata,
	}
	for k, v := range fields {
		obj[k] = v
	}
	return &unstructured.Unstructured{Object: obj}
}

// makeWorkloadSpec creates a workload spec with containers for StatefulSet/DaemonSet testing.
func makeWorkloadSpec(containerName, image string) map[string]interface{} {
	return map[string]interface{}{
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

// ============================================================
// Subtask 1: StatefulSet — Extract serviceName
// ============================================================

func TestProcessStatefulSet_ExtractsServiceName(t *testing.T) {
	p := NewStatefulSetProcessor()
	ctx := newTestProcessorContext()

	spec := makeWorkloadSpec("myapp", "nginx:1.21")
	spec["replicas"] = int64(3)
	spec["serviceName"] = "myapp-headless"

	obj := makeStatefulSetObj("myapp", "default", nil, spec)
	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed)
	testutil.AssertEqual(t, "myapp-headless", result.Values["serviceName"])

	// Should add Service dependency
	found := false
	for _, dep := range result.Dependencies {
		if dep.GVK.Kind == "Service" && dep.Name == "myapp-headless" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected Service dependency for headless service 'myapp-headless'")
	}
}

// ============================================================
// Subtask 2: StatefulSet — Extract volumeClaimTemplates
// ============================================================

func TestProcessStatefulSet_ExtractsVolumeClaimTemplates(t *testing.T) {
	p := NewStatefulSetProcessor()
	ctx := newTestProcessorContext()

	spec := makeWorkloadSpec("db", "postgres:15")
	spec["replicas"] = int64(3)
	spec["serviceName"] = "db-headless"
	spec["volumeClaimTemplates"] = []interface{}{
		map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "data",
			},
			"spec": map[string]interface{}{
				"accessModes":      []interface{}{"ReadWriteOnce"},
				"storageClassName": "fast-ssd",
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{
						"storage": "10Gi",
					},
				},
			},
		},
	}

	obj := makeStatefulSetObj("db", "default", nil, spec)
	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	vcts, ok := result.Values["volumeClaimTemplates"].([]interface{})
	if !ok {
		t.Fatal("Expected volumeClaimTemplates to be []interface{}")
	}
	testutil.AssertEqual(t, 1, len(vcts))

	vct, ok := vcts[0].(map[string]interface{})
	if !ok {
		t.Fatal("Expected VCT element to be map")
	}
	meta, _ := vct["metadata"].(map[string]interface{})
	testutil.AssertEqual(t, "data", meta["name"])
}

// ============================================================
// Subtask 3: StatefulSet — Extract podManagementPolicy
// ============================================================

func TestProcessStatefulSet_ExtractsPodManagementPolicy(t *testing.T) {
	p := NewStatefulSetProcessor()
	ctx := newTestProcessorContext()

	spec := makeWorkloadSpec("cache", "redis:7")
	spec["replicas"] = int64(3)
	spec["serviceName"] = "cache-headless"
	spec["podManagementPolicy"] = "Parallel"

	obj := makeStatefulSetObj("cache", "default", nil, spec)
	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "Parallel", result.Values["podManagementPolicy"])
}

// ============================================================
// Subtask 4: StatefulSet — Extract updateStrategy
// ============================================================

func TestProcessStatefulSet_ExtractsUpdateStrategy(t *testing.T) {
	p := NewStatefulSetProcessor()
	ctx := newTestProcessorContext()

	spec := makeWorkloadSpec("db", "postgres:15")
	spec["replicas"] = int64(3)
	spec["serviceName"] = "db-headless"
	spec["updateStrategy"] = map[string]interface{}{
		"type": "RollingUpdate",
		"rollingUpdate": map[string]interface{}{
			"partition": int64(3),
		},
	}

	obj := makeStatefulSetObj("db", "default", nil, spec)
	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	strategy, ok := result.Values["updateStrategy"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected updateStrategy to be a map")
	}
	testutil.AssertEqual(t, "RollingUpdate", strategy["type"])
}

// ============================================================
// Subtask 5: DaemonSet — Extract updateStrategy
// ============================================================

func TestProcessDaemonSet_ExtractsUpdateStrategy(t *testing.T) {
	p := NewDaemonSetProcessor()
	ctx := newTestProcessorContext()

	spec := makeWorkloadSpec("fluentd", "fluentd:v1.16")
	spec["updateStrategy"] = map[string]interface{}{
		"type": "RollingUpdate",
		"rollingUpdate": map[string]interface{}{
			"maxUnavailable": int64(1),
		},
	}

	obj := makeDaemonSetObj("fluentd", "kube-system", nil, spec)
	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed)
	strategy, ok := result.Values["updateStrategy"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected updateStrategy to be a map")
	}
	testutil.AssertEqual(t, "RollingUpdate", strategy["type"])
}

// ============================================================
// Subtask 6: DaemonSet — Extract node selector
// ============================================================

func TestProcessDaemonSet_ExtractsNodeSelector(t *testing.T) {
	p := NewDaemonSetProcessor()
	ctx := newTestProcessorContext()

	spec := makeWorkloadSpec("monitoring-agent", "datadog:7")
	spec["template"].(map[string]interface{})["spec"].(map[string]interface{})["nodeSelector"] = map[string]interface{}{
		"disktype": "ssd",
	}

	obj := makeDaemonSetObj("monitoring-agent", "monitoring", nil, spec)
	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	ns, ok := result.Values["nodeSelector"].(map[string]string)
	if !ok {
		t.Fatal("Expected nodeSelector to be map[string]string")
	}
	testutil.AssertEqual(t, "ssd", ns["disktype"])
}

// ============================================================
// Subtask 7: PVC — Extract storageClassName
// ============================================================

func TestProcessPVC_ExtractsStorageClassName(t *testing.T) {
	p := NewPVCProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"storageClassName": "fast",
		"accessModes":      []interface{}{"ReadWriteOnce"},
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
				"storage": "10Gi",
			},
		},
	}
	obj := makePVCObj("data-vol", "default", nil, spec)
	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed)
	testutil.AssertEqual(t, "fast", result.Values["storageClassName"])
}

// ============================================================
// Subtask 8: PVC — Extract resources
// ============================================================

func TestProcessPVC_ExtractsResources(t *testing.T) {
	p := NewPVCProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"accessModes": []interface{}{"ReadWriteOnce"},
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
				"storage": "10Gi",
			},
		},
	}
	obj := makePVCObj("data-pvc", "default", nil, spec)
	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	resources, ok := result.Values["resources"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected resources to be a map")
	}
	requests, ok := resources["requests"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected resources.requests to be a map")
	}
	testutil.AssertEqual(t, "10Gi", requests["storage"])
}

// ============================================================
// Subtask 9: PVC — Extract accessModes
// ============================================================

func TestProcessPVC_ExtractsAccessModes(t *testing.T) {
	p := NewPVCProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"accessModes": []interface{}{"ReadWriteOnce"},
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
				"storage": "5Gi",
			},
		},
	}
	obj := makePVCObj("pvc-rwo", "default", nil, spec)
	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)

	// accessModes comes from NestedStringSlice — returns []string
	modes, ok := result.Values["accessModes"].([]string)
	if !ok {
		t.Fatalf("Expected accessModes to be []string, got %T", result.Values["accessModes"])
	}
	testutil.AssertEqual(t, 1, len(modes))
	testutil.AssertEqual(t, "ReadWriteOnce", modes[0])
}

// ============================================================
// Subtask 10: PVC — Extract volumeMode
// ============================================================

func TestProcessPVC_ExtractsVolumeMode(t *testing.T) {
	p := NewPVCProcessor()
	ctx := newTestProcessorContext()

	tests := []struct {
		name     string
		mode     string
		expected string
	}{
		{"Filesystem", "Filesystem", "Filesystem"},
		{"Block", "Block", "Block"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := map[string]interface{}{
				"accessModes": []interface{}{"ReadWriteOnce"},
				"volumeMode":  tt.mode,
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{
						"storage": "1Gi",
					},
				},
			}
			obj := makePVCObj("pvc-"+tt.name, "default", nil, spec)
			result, err := p.Process(ctx, obj)

			testutil.AssertNoError(t, err)
			testutil.AssertEqual(t, tt.expected, result.Values["volumeMode"])
		})
	}
}

// ============================================================
// Subtask 11: PVC — Extract dataSource (cloned from snapshot)
// ============================================================

func TestProcessPVC_ExtractsDataSource(t *testing.T) {
	p := NewPVCProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"accessModes": []interface{}{"ReadWriteOnce"},
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
				"storage": "20Gi",
			},
		},
		"dataSource": map[string]interface{}{
			"name":     "my-snapshot",
			"kind":     "VolumeSnapshot",
			"apiGroup": "snapshot.storage.k8s.io",
		},
	}
	obj := makePVCObj("clone-pvc", "default", nil, spec)
	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed)

	ds, ok := result.Values["dataSource"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected dataSource in values")
	}
	if ds["name"] != "my-snapshot" {
		t.Errorf("Expected dataSource name 'my-snapshot', got %v", ds["name"])
	}
	if ds["kind"] != "VolumeSnapshot" {
		t.Errorf("Expected dataSource kind 'VolumeSnapshot', got %v", ds["kind"])
	}
	if ds["apiGroup"] != "snapshot.storage.k8s.io" {
		t.Errorf("Expected dataSource apiGroup 'snapshot.storage.k8s.io', got %v", ds["apiGroup"])
	}
}

func TestProcessPVC_NoStorageClass(t *testing.T) {
	p := NewPVCProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"accessModes": []interface{}{"ReadWriteMany"},
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
				"storage": "50Gi",
			},
		},
	}
	obj := makePVCObj("shared-pvc", "default", nil, spec)
	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed)
	if _, exists := result.Values["storageClassName"]; exists {
		t.Error("storageClassName should not be set when not in spec")
	}
}

// ============================================================
// Subtask 12: ServiceAccount — Extract imagePullSecrets
// ============================================================

func TestProcessServiceAccount_ExtractsImagePullSecrets(t *testing.T) {
	p := NewServiceAccountProcessor()
	ctx := newTestProcessorContext()

	fields := map[string]interface{}{
		"imagePullSecrets": []interface{}{
			map[string]interface{}{
				"name": "regcred",
			},
		},
	}
	obj := makeServiceAccountObj("myapp-sa", "default", nil, nil, fields)
	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed)

	secrets, ok := result.Values["imagePullSecrets"].([]interface{})
	if !ok {
		t.Fatalf("Expected imagePullSecrets to be []interface{}, got %T", result.Values["imagePullSecrets"])
	}
	testutil.AssertEqual(t, 1, len(secrets))
	sec, _ := secrets[0].(map[string]interface{})
	testutil.AssertEqual(t, "regcred", sec["name"])
}

// ============================================================
// Subtask 13: ServiceAccount — Extract automountServiceAccountToken
// ============================================================

func TestProcessServiceAccount_ExtractsAutomountToken(t *testing.T) {
	p := NewServiceAccountProcessor()
	ctx := newTestProcessorContext()

	fields := map[string]interface{}{
		"automountServiceAccountToken": false,
	}
	obj := makeServiceAccountObj("restricted-sa", "default", nil, nil, fields)
	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, false, result.Values["automountServiceAccountToken"])
}

// ============================================================
// Subtask 14: Edge cases
// ============================================================

func TestProcessStatefulSet_EdgeCases(t *testing.T) {
	p := NewStatefulSetProcessor()
	ctx := newTestProcessorContext()

	t.Run("NilStatefulSet", func(t *testing.T) {
		_, err := p.Process(ctx, nil)
		if err == nil {
			t.Error("Expected error for nil StatefulSet")
		}
	})

	t.Run("WithoutVolumeClaimTemplates", func(t *testing.T) {
		spec := makeWorkloadSpec("cache", "redis:7")
		spec["replicas"] = int64(1)
		spec["serviceName"] = "cache-headless"

		obj := makeStatefulSetObj("cache", "default", nil, spec)
		result, err := p.Process(ctx, obj)

		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed)
		if _, exists := result.Values["volumeClaimTemplates"]; exists {
			t.Error("volumeClaimTemplates should not be set when not present")
		}
	})

	t.Run("WithServiceAccountDependency", func(t *testing.T) {
		spec := makeWorkloadSpec("app", "nginx:1.21")
		spec["replicas"] = int64(1)
		spec["serviceName"] = "app-headless"
		spec["template"].(map[string]interface{})["spec"].(map[string]interface{})["serviceAccountName"] = "app-sa"

		obj := makeStatefulSetObj("app", "default", nil, spec)
		result, err := p.Process(ctx, obj)

		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, "app-sa", result.Values["serviceAccountName"])
		found := false
		for _, dep := range result.Dependencies {
			if dep.GVK.Kind == "ServiceAccount" && dep.Name == "app-sa" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected ServiceAccount dependency")
		}
	})
}

func TestProcessDaemonSet_EdgeCases(t *testing.T) {
	p := NewDaemonSetProcessor()
	ctx := newTestProcessorContext()

	t.Run("NilDaemonSet", func(t *testing.T) {
		_, err := p.Process(ctx, nil)
		if err == nil {
			t.Error("Expected error for nil DaemonSet")
		}
	})

	t.Run("WithTolerations", func(t *testing.T) {
		spec := makeWorkloadSpec("node-exporter", "prom/node-exporter:1.7")
		spec["template"].(map[string]interface{})["spec"].(map[string]interface{})["tolerations"] = []interface{}{
			map[string]interface{}{
				"key":      "node-role.kubernetes.io/master",
				"operator": "Exists",
				"effect":   "NoSchedule",
			},
		}

		obj := makeDaemonSetObj("node-exporter", "monitoring", nil, spec)
		result, err := p.Process(ctx, obj)

		testutil.AssertNoError(t, err)
		tolerations, ok := result.Values["tolerations"].([]interface{})
		if !ok {
			t.Fatal("Expected tolerations to be []interface{}")
		}
		testutil.AssertEqual(t, 1, len(tolerations))
	})

	t.Run("WithPodAnnotations", func(t *testing.T) {
		spec := makeWorkloadSpec("agent", "agent:1.0")
		spec["template"].(map[string]interface{})["metadata"].(map[string]interface{})["annotations"] = map[string]interface{}{
			"prometheus.io/scrape": "true",
			"prometheus.io/port":   "9090",
		}

		obj := makeDaemonSetObj("agent", "monitoring", nil, spec)
		result, err := p.Process(ctx, obj)

		testutil.AssertNoError(t, err)
		annotations, ok := result.Values["podAnnotations"].(map[string]string)
		if !ok {
			t.Fatalf("Expected podAnnotations to be map[string]string, got %T", result.Values["podAnnotations"])
		}
		testutil.AssertEqual(t, "true", annotations["prometheus.io/scrape"])
	})
}

func TestProcessPVC_EdgeCases(t *testing.T) {
	p := NewPVCProcessor()
	ctx := newTestProcessorContext()

	t.Run("NilPVC", func(t *testing.T) {
		_, err := p.Process(ctx, nil)
		if err == nil {
			t.Error("Expected error for nil PVC")
		}
	})

	t.Run("MinimalPVC", func(t *testing.T) {
		spec := map[string]interface{}{
			"accessModes": []interface{}{"ReadWriteOnce"},
			"resources": map[string]interface{}{
				"requests": map[string]interface{}{
					"storage": "1Gi",
				},
			},
		}
		obj := makePVCObj("minimal", "default", nil, spec)
		result, err := p.Process(ctx, obj)

		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed)
		testutil.AssertEqual(t, "minimal", result.Values["name"])
	})

	t.Run("PVCWithSelector", func(t *testing.T) {
		spec := map[string]interface{}{
			"accessModes": []interface{}{"ReadWriteOnce"},
			"resources": map[string]interface{}{
				"requests": map[string]interface{}{
					"storage": "10Gi",
				},
			},
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"release": "stable",
					"tier":    "data",
				},
			},
		}
		obj := makePVCObj("selector-pvc", "default", nil, spec)
		result, err := p.Process(ctx, obj)

		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed)

		sel, ok := result.Values["selector"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected selector in values")
		}
		ml, ok := sel["matchLabels"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected matchLabels in selector")
		}
		if ml["release"] != "stable" {
			t.Errorf("Expected matchLabels release 'stable', got %v", ml["release"])
		}
		if ml["tier"] != "data" {
			t.Errorf("Expected matchLabels tier 'data', got %v", ml["tier"])
		}
	})
}

func TestProcessServiceAccount_EdgeCases(t *testing.T) {
	p := NewServiceAccountProcessor()
	ctx := newTestProcessorContext()

	t.Run("NilServiceAccount", func(t *testing.T) {
		_, err := p.Process(ctx, nil)
		if err == nil {
			t.Error("Expected error for nil ServiceAccount")
		}
	})

	t.Run("MinimalServiceAccount", func(t *testing.T) {
		obj := makeServiceAccountObj("default", "default", nil, nil, nil)
		result, err := p.Process(ctx, obj)

		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed)
		testutil.AssertEqual(t, "default", result.Values["name"])
		testutil.AssertEqual(t, true, result.Values["enabled"])
	})

	t.Run("WithAnnotations", func(t *testing.T) {
		annotations := map[string]interface{}{
			"eks.amazonaws.com/role-arn": "arn:aws:iam::123456789:role/my-role",
		}
		obj := makeServiceAccountObj("irsa-sa", "default", nil, annotations, nil)
		result, err := p.Process(ctx, obj)

		testutil.AssertNoError(t, err)
		ann, ok := result.Values["annotations"].(map[string]string)
		if !ok {
			t.Fatalf("Expected annotations to be map[string]string, got %T", result.Values["annotations"])
		}
		testutil.AssertEqual(t, "arn:aws:iam::123456789:role/my-role", ann["eks.amazonaws.com/role-arn"])
	})
}

// ============================================================
// Subtask 15: Result metadata, template generation, constructors
// ============================================================

func TestProcessStatefulSet_ResultMetadata(t *testing.T) {
	p := NewStatefulSetProcessor()
	ctx := newTestProcessorContext()

	spec := makeWorkloadSpec("myapp", "nginx:1.21")
	spec["replicas"] = int64(1)
	spec["serviceName"] = "myapp-headless"

	obj := makeStatefulSetObj("myapp", "default",
		map[string]interface{}{"app.kubernetes.io/name": "myapp"}, spec)
	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "myapp", result.ServiceName)
	testutil.AssertEqual(t, "templates/myapp-statefulset.yaml", result.TemplatePath)
	testutil.AssertEqual(t, "services.myapp.statefulSet", result.ValuesPath)
	testutil.AssertContains(t, result.TemplateContent, "kind: StatefulSet")
	testutil.AssertContains(t, result.TemplateContent, "serviceName")
}

func TestProcessDaemonSet_ResultMetadata(t *testing.T) {
	p := NewDaemonSetProcessor()
	ctx := newTestProcessorContext()

	spec := makeWorkloadSpec("fluentd", "fluentd:v1.16")
	obj := makeDaemonSetObj("fluentd", "kube-system",
		map[string]interface{}{"app.kubernetes.io/name": "fluentd"}, spec)
	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "fluentd", result.ServiceName)
	testutil.AssertEqual(t, "templates/fluentd-daemonset.yaml", result.TemplatePath)
	testutil.AssertEqual(t, "services.fluentd.daemonSet", result.ValuesPath)
	testutil.AssertContains(t, result.TemplateContent, "kind: DaemonSet")
}

func TestProcessPVC_ResultMetadata(t *testing.T) {
	p := NewPVCProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"accessModes": []interface{}{"ReadWriteOnce"},
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
				"storage": "1Gi",
			},
		},
	}
	obj := makePVCObj("data-pvc", "default",
		map[string]interface{}{"app.kubernetes.io/name": "myapp"}, spec)
	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "myapp", result.ServiceName)
	testutil.AssertEqual(t, "templates/myapp-pvc-data-pvc.yaml", result.TemplatePath)
	testutil.AssertEqual(t, "services.myapp.pvc", result.ValuesPath)
	testutil.AssertContains(t, result.TemplateContent, "kind: PersistentVolumeClaim")
}

func TestProcessServiceAccount_ResultMetadata(t *testing.T) {
	p := NewServiceAccountProcessor()
	ctx := newTestProcessorContext()

	obj := makeServiceAccountObj("myapp-sa", "default",
		map[string]interface{}{"app.kubernetes.io/name": "myapp"}, nil, nil)
	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "myapp", result.ServiceName)
	testutil.AssertEqual(t, "templates/myapp-serviceaccount.yaml", result.TemplatePath)
	testutil.AssertEqual(t, "services.myapp.serviceAccount", result.ValuesPath)
	testutil.AssertContains(t, result.TemplateContent, "kind: ServiceAccount")
}

// ============================================================
// Constructor tests
// ============================================================

func TestNewStatefulSetProcessor(t *testing.T) {
	p := NewStatefulSetProcessor()
	testutil.AssertEqual(t, "statefulset", p.Name())
	testutil.AssertEqual(t, 100, p.Priority())

	gvks := p.Supports()
	testutil.AssertEqual(t, 1, len(gvks))
	testutil.AssertEqual(t, schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}, gvks[0])
}

func TestNewDaemonSetProcessor(t *testing.T) {
	p := NewDaemonSetProcessor()
	testutil.AssertEqual(t, "daemonset", p.Name())
	testutil.AssertEqual(t, 100, p.Priority())

	gvks := p.Supports()
	testutil.AssertEqual(t, 1, len(gvks))
	testutil.AssertEqual(t, schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}, gvks[0])
}

func TestNewPVCProcessor(t *testing.T) {
	p := NewPVCProcessor()
	testutil.AssertEqual(t, "pvc", p.Name())
	testutil.AssertEqual(t, 100, p.Priority())

	gvks := p.Supports()
	testutil.AssertEqual(t, 1, len(gvks))
	testutil.AssertEqual(t, schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"}, gvks[0])
}

func TestNewServiceAccountProcessor(t *testing.T) {
	p := NewServiceAccountProcessor()
	testutil.AssertEqual(t, "serviceaccount", p.Name())
	testutil.AssertEqual(t, 100, p.Priority())

	gvks := p.Supports()
	testutil.AssertEqual(t, 1, len(gvks))
	testutil.AssertEqual(t, schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ServiceAccount"}, gvks[0])
}

// ============================================================
// extractWorkloadValues shared helper tests
// ============================================================

func TestExtractWorkloadValues_Containers(t *testing.T) {
	spec := makeWorkloadSpec("app", "nginx:1.21")
	spec["template"].(map[string]interface{})["spec"].(map[string]interface{})["containers"] = []interface{}{
		map[string]interface{}{
			"name":  "app",
			"image": "nginx:1.21",
			"ports": []interface{}{
				map[string]interface{}{
					"containerPort": int64(80),
					"protocol":      "TCP",
				},
			},
			"resources": map[string]interface{}{
				"limits": map[string]interface{}{
					"cpu":    "500m",
					"memory": "256Mi",
				},
			},
			"env": []interface{}{
				map[string]interface{}{
					"name":  "ENV_VAR",
					"value": "test",
				},
			},
			"volumeMounts": []interface{}{
				map[string]interface{}{
					"name":      "config",
					"mountPath": "/etc/config",
				},
			},
		},
	}

	obj := makeStatefulSetObj("app", "default", nil, spec)
	values, _ := extractWorkloadValues(obj)

	containers, ok := values["containers"].([]map[string]interface{})
	if !ok {
		t.Fatalf("Expected containers to be []map[string]interface{}, got %T", values["containers"])
	}
	testutil.AssertEqual(t, 1, len(containers))
	testutil.AssertEqual(t, "app", containers[0]["name"])

	img, ok := containers[0]["image"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected image to be a map")
	}
	testutil.AssertEqual(t, "nginx", img["repository"])
	testutil.AssertEqual(t, "1.21", img["tag"])

	if containers[0]["ports"] == nil {
		t.Error("Expected ports to be extracted")
	}
	if containers[0]["resources"] == nil {
		t.Error("Expected resources to be extracted")
	}
	if containers[0]["env"] == nil {
		t.Error("Expected env to be extracted")
	}
	if containers[0]["volumeMounts"] == nil {
		t.Error("Expected volumeMounts to be extracted")
	}
}

func TestExtractWorkloadValues_Volumes(t *testing.T) {
	spec := makeWorkloadSpec("app", "nginx:1.21")
	spec["template"].(map[string]interface{})["spec"].(map[string]interface{})["volumes"] = []interface{}{
		map[string]interface{}{
			"name": "config",
			"configMap": map[string]interface{}{
				"name": "app-config",
			},
		},
	}

	obj := makeStatefulSetObj("app", "default", nil, spec)
	values, deps := extractWorkloadValues(obj)

	if values["volumes"] == nil {
		t.Error("Expected volumes to be extracted")
	}

	// ConfigMap volume should create a dependency
	found := false
	for _, dep := range deps {
		if dep.GVK.Kind == "ConfigMap" && dep.Name == "app-config" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected ConfigMap dependency from volume")
	}
}

func TestExtractWorkloadValues_EnvDependencies(t *testing.T) {
	spec := makeWorkloadSpec("app", "nginx:1.21")
	spec["template"].(map[string]interface{})["spec"].(map[string]interface{})["containers"] = []interface{}{
		map[string]interface{}{
			"name":  "app",
			"image": "nginx:1.21",
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
	}

	obj := makeStatefulSetObj("app", "default", nil, spec)
	_, deps := extractWorkloadValues(obj)

	found := false
	for _, dep := range deps {
		if dep.GVK.Kind == "Secret" && dep.Name == "db-credentials" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected Secret dependency from env secretKeyRef")
	}
}

// ============================================================
// Template content verification
// ============================================================

func TestProcessStatefulSet_GeneratesTemplate(t *testing.T) {
	p := NewStatefulSetProcessor()
	ctx := newTestProcessorContext()

	spec := makeWorkloadSpec("db", "postgres:15")
	spec["replicas"] = int64(3)
	spec["serviceName"] = "db-headless"
	obj := makeStatefulSetObj("db", "default",
		map[string]interface{}{"app.kubernetes.io/name": "db"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tmpl := result.TemplateContent
	testutil.AssertContains(t, tmpl, "kind: StatefulSet")
	testutil.AssertContains(t, tmpl, "serviceName:")
	testutil.AssertContains(t, tmpl, "replicas:")
	testutil.AssertContains(t, tmpl, "volumeClaimTemplates")
	// Chart name should be referenced
	if !strings.Contains(tmpl, "test-chart") {
		t.Error("Expected template to reference chart name")
	}
}

func TestProcessDaemonSet_GeneratesTemplate(t *testing.T) {
	p := NewDaemonSetProcessor()
	ctx := newTestProcessorContext()

	spec := makeWorkloadSpec("agent", "agent:1.0")
	obj := makeDaemonSetObj("agent", "monitoring",
		map[string]interface{}{"app.kubernetes.io/name": "agent"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tmpl := result.TemplateContent
	testutil.AssertContains(t, tmpl, "kind: DaemonSet")
	testutil.AssertContains(t, tmpl, "updateStrategy")
	// DaemonSet has no replicas
	if strings.Contains(tmpl, "replicas:") {
		t.Error("DaemonSet template should not have replicas")
	}
}
