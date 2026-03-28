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
// Test helpers for GenericCRD processor
// ============================================================

func makeGenericCRDObj(apiVersion, kind, name, namespace string, spec map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app": name,
				},
			},
		},
	}
	if spec != nil {
		obj.Object["spec"] = spec
	}
	return obj
}

func makeCRDDefinitionObj(name string, versions []interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": map[string]interface{}{
				"versions": versions,
			},
		},
	}
}

// ============================================================
// 5.2.1: Generic CRD processing
// ============================================================

func TestGenericCRDProcessor_Constructor(t *testing.T) {
	proc := NewGenericCRDProcessor()

	testutil.AssertEqual(t, "genericcrd", proc.Name(), "processor name")
	testutil.AssertEqual(t, 1, proc.Priority(), "processor priority (lowest)")

	gvks := proc.Supports()
	if len(gvks) != 0 {
		t.Errorf("Expected 0 supported GVKs (fallback), got %d", len(gvks))
	}
}

func TestGenericCRDProcessor_ProcessesUnknownCRD(t *testing.T) {
	proc := NewGenericCRDProcessor()
	ctx := newTestProcessorContext()

	obj := makeGenericCRDObj(
		"example.io/v1alpha1", "MyCustomResource",
		"my-resource", "default",
		map[string]interface{}{
			"replicas": int64(3),
			"config": map[string]interface{}{
				"key": "value",
			},
		},
	)

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")
	testutil.AssertEqual(t, "myResource", result.ServiceName, "service name")

	// Check GVK in values
	gvkValues, ok := result.Values["gvk"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected gvk in values")
	}
	testutil.AssertEqual(t, "example.io", gvkValues["group"], "group")
	testutil.AssertEqual(t, "v1alpha1", gvkValues["version"], "version")
	testutil.AssertEqual(t, "MyCustomResource", gvkValues["kind"], "kind")

	// Check spec extracted
	spec, ok := result.Values["spec"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected spec in values")
	}
	config, ok := spec["config"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected config in spec")
	}
	testutil.AssertEqual(t, "value", config["key"], "spec.config.key")
}

func TestGenericCRDProcessor_ExtractsStatus(t *testing.T) {
	proc := NewGenericCRDProcessor()
	ctx := newTestProcessorContext()

	obj := makeGenericCRDObj(
		"example.io/v1", "StatusResource",
		"status-res", "default", nil,
	)
	obj.Object["status"] = map[string]interface{}{
		"phase":   "Running",
		"replicas": int64(2),
	}

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	status, ok := result.Values["status"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected status in values")
	}
	testutil.AssertEqual(t, "Running", status["phase"], "status.phase")
}

func TestGenericCRDProcessor_NilObject(t *testing.T) {
	proc := NewGenericCRDProcessor()
	ctx := newTestProcessorContext()

	result, err := proc.Process(ctx, nil)
	if err == nil {
		t.Fatal("Expected error for nil object")
	}
	if result != nil {
		t.Error("Expected nil result")
	}
}

func TestGenericCRDProcessor_Template(t *testing.T) {
	proc := NewGenericCRDProcessor()
	ctx := processor.Context{ChartName: "myapp"}

	obj := makeGenericCRDObj(
		"stable.example.com/v1", "Widget",
		"my-widget", "production",
		map[string]interface{}{"color": "blue"},
	)

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tpl := result.TemplateContent
	testutil.AssertContains(t, tpl, "apiVersion: stable.example.com/v1", "apiVersion in template")
	testutil.AssertContains(t, tpl, "kind: Widget", "kind in template")
	testutil.AssertContains(t, tpl, "{{ $.Release.Namespace }}", "namespace in template")
	testutil.AssertContains(t, tpl, `include "myapp.labels"`, "labels helper")
	testutil.AssertContains(t, tpl, "toYaml . | nindent 2", "spec toYaml")
}

func TestGenericCRDProcessor_ClusterScoped(t *testing.T) {
	proc := NewGenericCRDProcessor()
	ctx := newTestProcessorContext()

	obj := makeGenericCRDObj(
		"example.io/v1", "ClusterWidget",
		"global-widget", "", // no namespace
		map[string]interface{}{"scope": "cluster"},
	)

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	// Cluster-scoped: template should NOT contain namespace
	if strings.Contains(result.TemplateContent, "Release.Namespace") {
		t.Error("Cluster-scoped resource should not have namespace in template")
	}
}

func TestGenericCRDProcessor_Metadata(t *testing.T) {
	proc := NewGenericCRDProcessor()
	ctx := newTestProcessorContext()

	obj := makeGenericCRDObj(
		"apps.example.io/v2beta1", "AppConfig",
		"my-app-config", "staging",
		nil,
	)

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	testutil.AssertEqual(t, "my-app-config", result.Metadata["name"], "metadata name")
	testutil.AssertEqual(t, "staging", result.Metadata["namespace"], "metadata namespace")
	testutil.AssertEqual(t, true, result.Metadata["isCRD"], "isCRD flag")
}

func TestGenericCRDProcessor_TemplatePath(t *testing.T) {
	proc := NewGenericCRDProcessor()
	ctx := newTestProcessorContext()

	obj := makeGenericCRDObj(
		"example.io/v1", "MyThing",
		"thing-one", "default",
		nil,
	)

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	testutil.AssertContains(t, result.TemplatePath, "mything", "template path has lowercase kind")
	testutil.AssertContains(t, result.ValuesPath, "mything", "values path has lowercase kind")
}

// ============================================================
// 5.2.2: CRD schema extraction
// ============================================================

func TestExtractCRDValuesSchema_Basic(t *testing.T) {
	obj := makeCRDDefinitionObj("widgets.example.io", []interface{}{
		map[string]interface{}{
			"name": "v1",
			"schema": map[string]interface{}{
				"openAPIV3Schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"spec": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"replicas": map[string]interface{}{
									"type":    "integer",
									"default": int64(1),
								},
								"image": map[string]interface{}{
									"type": "string",
								},
								"enabled": map[string]interface{}{
									"type":    "boolean",
									"default": true,
								},
							},
						},
					},
				},
			},
		},
	})

	schema := extractCRDValuesSchema(obj)
	if schema == nil {
		t.Fatal("Expected non-nil schema")
	}

	v1Schema, ok := schema["v1"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected v1 schema")
	}

	spec, ok := v1Schema["spec"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected spec in schema")
	}

	testutil.AssertEqual(t, int64(1), spec["replicas"], "default replicas")
	testutil.AssertEqual(t, "", spec["image"], "default string")
	testutil.AssertEqual(t, true, spec["enabled"], "default enabled")
}

func TestExtractCRDValuesSchema_NotCRD(t *testing.T) {
	obj := makeGenericCRDObj("apps/v1", "Deployment", "test", "default", nil)
	schema := extractCRDValuesSchema(obj)
	if schema != nil {
		t.Error("Expected nil schema for non-CRD resource")
	}
}

func TestExtractCRDValuesSchema_MultipleVersions(t *testing.T) {
	obj := makeCRDDefinitionObj("things.example.io", []interface{}{
		map[string]interface{}{
			"name": "v1alpha1",
			"schema": map[string]interface{}{
				"openAPIV3Schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type": "string",
						},
					},
				},
			},
		},
		map[string]interface{}{
			"name": "v1",
			"schema": map[string]interface{}{
				"openAPIV3Schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type": "string",
						},
						"count": map[string]interface{}{
							"type": "integer",
						},
					},
				},
			},
		},
	})

	schema := extractCRDValuesSchema(obj)
	if schema == nil {
		t.Fatal("Expected non-nil schema")
	}

	if _, ok := schema["v1alpha1"]; !ok {
		t.Error("Expected v1alpha1 in schema")
	}
	if _, ok := schema["v1"]; !ok {
		t.Error("Expected v1 in schema")
	}
}

func TestExtractCRDValuesSchema_NestedObjects(t *testing.T) {
	obj := makeCRDDefinitionObj("nested.example.io", []interface{}{
		map[string]interface{}{
			"name": "v1",
			"schema": map[string]interface{}{
				"openAPIV3Schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"config": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"timeout": map[string]interface{}{
									"type":    "integer",
									"default": int64(30),
								},
								"retries": map[string]interface{}{
									"type": "integer",
								},
							},
						},
						"tags": map[string]interface{}{
							"type": "array",
						},
					},
				},
			},
		},
	})

	schema := extractCRDValuesSchema(obj)
	if schema == nil {
		t.Fatal("Expected non-nil schema")
	}

	v1, ok := schema["v1"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected v1 schema")
	}

	config, ok := v1["config"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected config as nested object")
	}
	testutil.AssertEqual(t, int64(30), config["timeout"], "nested default")
	testutil.AssertEqual(t, int64(0), config["retries"], "nested integer default")
}

func TestExtractCRDValuesSchema_NoVersions(t *testing.T) {
	obj := makeCRDDefinitionObj("empty.example.io", []interface{}{})
	schema := extractCRDValuesSchema(obj)
	if schema != nil {
		t.Error("Expected nil schema for CRD with no versions")
	}
}

// ============================================================
// 5.2.3: CRD installation (crds/ directory)
// ============================================================

func TestGenerateCRDInstallFiles_Basic(t *testing.T) {
	crd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"name": "widgets.example.io",
			},
			"spec": map[string]interface{}{
				"group": "example.io",
				"names": map[string]interface{}{
					"kind":   "Widget",
					"plural": "widgets",
				},
			},
		},
	}

	files := GenerateCRDInstallFiles([]*unstructured.Unstructured{crd})

	if len(files) != 1 {
		t.Fatalf("Expected 1 CRD file, got %d", len(files))
	}

	content, ok := files["crds/widgets.example.io.yaml"]
	if !ok {
		t.Fatal("Expected crds/widgets.example.io.yaml")
	}

	testutil.AssertContains(t, content, "WARNING", "should have warning comment")
	testutil.AssertContains(t, content, "Helm does NOT manage CRD updates", "should warn about CRD updates")
	testutil.AssertContains(t, content, "apiVersion: apiextensions.k8s.io/v1", "apiVersion")
	testutil.AssertContains(t, content, "kind: CustomResourceDefinition", "kind")
	testutil.AssertContains(t, content, "name: widgets.example.io", "name")
}

func TestGenerateCRDInstallFiles_MultipleCRDs(t *testing.T) {
	crds := []*unstructured.Unstructured{
		{Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata":   map[string]interface{}{"name": "foos.example.io"},
			"spec":       map[string]interface{}{"group": "example.io"},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata":   map[string]interface{}{"name": "bars.example.io"},
			"spec":       map[string]interface{}{"group": "example.io"},
		}},
	}

	files := GenerateCRDInstallFiles(crds)
	if len(files) != 2 {
		t.Fatalf("Expected 2 CRD files, got %d", len(files))
	}

	if _, ok := files["crds/foos.example.io.yaml"]; !ok {
		t.Error("Expected crds/foos.example.io.yaml")
	}
	if _, ok := files["crds/bars.example.io.yaml"]; !ok {
		t.Error("Expected crds/bars.example.io.yaml")
	}
}

func TestGenerateCRDInstallFiles_NonCRDSkipped(t *testing.T) {
	resources := []*unstructured.Unstructured{
		{Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]interface{}{"name": "my-deploy"},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata":   map[string]interface{}{"name": "widgets.example.io"},
			"spec":       map[string]interface{}{"group": "example.io"},
		}},
	}

	files := GenerateCRDInstallFiles(resources)
	if len(files) != 1 {
		t.Fatalf("Expected 1 CRD file (non-CRD skipped), got %d", len(files))
	}
}

func TestGenerateCRDInstallFiles_Empty(t *testing.T) {
	files := GenerateCRDInstallFiles(nil)
	if len(files) != 0 {
		t.Errorf("Expected 0 files for nil input, got %d", len(files))
	}
}

func TestGenerateCRDInstallFiles_WithLabelsAndAnnotations(t *testing.T) {
	crd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"name": "widgets.example.io",
				"labels": map[string]interface{}{
					"app.kubernetes.io/managed-by": "dhg",
				},
				"annotations": map[string]interface{}{
					"meta.helm.sh/release-name": "my-release",
				},
			},
			"spec": map[string]interface{}{"group": "example.io"},
		},
	}

	files := GenerateCRDInstallFiles([]*unstructured.Unstructured{crd})
	content := files["crds/widgets.example.io.yaml"]

	testutil.AssertContains(t, content, "app.kubernetes.io/managed-by", "labels preserved")
	testutil.AssertContains(t, content, "meta.helm.sh/release-name", "annotations preserved")
}

// ============================================================
// Registry fallback test
// ============================================================

func TestGenericCRDProcessor_FallbackPriority(t *testing.T) {
	r := processor.NewRegistry()
	RegisterAll(r)

	// Known GVK should have a processor
	deployGVK := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	p, ok := r.GetProcessor(deployGVK)
	if !ok {
		t.Fatal("Expected processor for Deployment")
	}
	if p.Priority() <= 1 {
		t.Error("Known processor should have higher priority than generic")
	}

	// Unknown GVK should NOT have a processor in the registry
	unknownGVK := schema.GroupVersionKind{Group: "unknown.io", Version: "v1", Kind: "UnknownThing"}
	_, ok = r.GetProcessor(unknownGVK)
	if ok {
		t.Error("Unknown GVK should not match any registered processor")
	}

	// The GenericCRDProcessor can handle it explicitly
	generic := NewGenericCRDProcessor()
	ctx := newTestProcessorContext()
	obj := makeGenericCRDObj("unknown.io/v1", "UnknownThing", "test", "default", nil)
	result, err := generic.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "generic should process unknown CRD")
}

// ============================================================
// parseOpenAPISchemaToValues tests
// ============================================================

func TestParseOpenAPISchemaToValues_AllTypes(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type": "string",
			},
			"count": map[string]interface{}{
				"type": "integer",
			},
			"enabled": map[string]interface{}{
				"type": "boolean",
			},
			"weight": map[string]interface{}{
				"type": "number",
			},
			"items": map[string]interface{}{
				"type": "array",
			},
			"meta": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"key": map[string]interface{}{
						"type": "string",
					},
				},
			},
		},
	}

	result := parseOpenAPISchemaToValues(schema, "")

	testutil.AssertEqual(t, "", result["name"], "string default")
	testutil.AssertEqual(t, int64(0), result["count"], "integer default")
	testutil.AssertEqual(t, false, result["enabled"], "boolean default")
	testutil.AssertEqual(t, float64(0), result["weight"], "number default")

	items, ok := result["items"].([]interface{})
	if !ok {
		t.Fatal("Expected items as slice")
	}
	if len(items) != 0 {
		t.Error("Expected empty array default")
	}

	meta, ok := result["meta"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected meta as map")
	}
	testutil.AssertEqual(t, "", meta["key"], "nested string default")
}
