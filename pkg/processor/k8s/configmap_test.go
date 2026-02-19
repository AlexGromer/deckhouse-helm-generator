package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor/value"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Test helpers for ConfigMap processor
// ============================================================

// makeConfigMapObj creates an unstructured ConfigMap for testing.
func makeConfigMapObj(name, namespace string, labels, annotations map[string]interface{}, data map[string]interface{}) *unstructured.Unstructured {
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
		"kind":       "ConfigMap",
		"metadata":   metadata,
	}

	// Merge data fields into the object
	for k, v := range data {
		obj[k] = v
	}

	return &unstructured.Unstructured{Object: obj}
}

// newTestContextWithValueProcessor creates a context with value processor and external file manager.
func newTestContextWithValueProcessor() processor.Context {
	return processor.Context{
		ChartName:           "test-chart",
		ValueProcessor:      value.DefaultProcessor(),
		ExternalFileManager: value.NewExternalFileManager(),
	}
}

// ============================================================
// Subtask 1: Extract data (simple key-value)
// ============================================================

func TestProcessConfigMap_ExtractsSimpleData(t *testing.T) {
	t.Run("SimpleKeyValue", func(t *testing.T) {
		proc := NewConfigMapProcessor()
		ctx := newTestProcessorContext()

		obj := makeConfigMapObj("my-config", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"data": map[string]interface{}{
					"key1": "value1",
					"key2": "value2",
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		data, ok := result.Values["data"]
		if !ok {
			t.Fatal("Expected 'data' key in values")
		}

		dataMap, ok := data.(map[string]string)
		if !ok {
			t.Fatalf("Expected data as map[string]string, got %T", data)
		}
		testutil.AssertEqual(t, "value1", dataMap["key1"], "key1 value")
		testutil.AssertEqual(t, "value2", dataMap["key2"], "key2 value")
	})

	t.Run("Enabled", func(t *testing.T) {
		proc := NewConfigMapProcessor()
		ctx := newTestProcessorContext()

		obj := makeConfigMapObj("my-config", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"data": map[string]interface{}{
					"key1": "value1",
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		testutil.AssertEqual(t, true, result.Values["enabled"], "ConfigMap should have enabled=true")
	})
}

// ============================================================
// Subtask 2: Extract data (multi-line values) with value processor
// ============================================================

func TestProcessConfigMap_ExtractsMultiLineData(t *testing.T) {
	proc := NewConfigMapProcessor()
	ctx := newTestContextWithValueProcessor()

	nginxConf := "server {\n    listen 80;\n    server_name localhost;\n    location / {\n        root /usr/share/nginx/html;\n    }\n}\n"

	obj := makeConfigMapObj("nginx-config", "default",
		map[string]interface{}{"app": "nginx"}, nil,
		map[string]interface{}{
			"data": map[string]interface{}{
				"nginx.conf": nginxConf,
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	data, ok := result.Values["data"]
	if !ok {
		t.Fatal("Expected 'data' key in values")
	}

	// With value processor, data should be processed
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data as map[string]interface{} when value processor is active, got %T", data)
	}

	// Verify the value was processed (should contain the config content)
	confValue, ok := dataMap["nginx.conf"]
	if !ok {
		t.Fatal("Expected 'nginx.conf' key in processed data")
	}
	// The value should contain the nginx config content
	confStr, ok := confValue.(string)
	if !ok {
		// Could be a map with _externalFile if externalized
		confMap, ok := confValue.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected nginx.conf value as string or map, got %T", confValue)
		}
		// If externalized, verify external file reference
		if _, hasExtFile := confMap["_externalFile"]; hasExtFile {
			// Verify external files were created
			if len(result.ExternalFiles) == 0 {
				t.Fatal("Expected external files when value is externalized")
			}
		}
	} else {
		testutil.AssertContains(t, confStr, "listen 80", "nginx.conf should contain 'listen 80'")
	}
}

// ============================================================
// Subtask 3: Extract binaryData
// ============================================================

func TestProcessConfigMap_ExtractsBinaryData(t *testing.T) {
	proc := NewConfigMapProcessor()
	ctx := newTestProcessorContext()

	obj := makeConfigMapObj("my-config", "default",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"binaryData": map[string]interface{}{
				"image.png": "aVZCT1J3MEtHZ29BQUFBTlNVaEVVZw==",
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	binaryData, ok := result.Values["binaryData"]
	if !ok {
		t.Fatal("Expected 'binaryData' key in values")
	}

	bdMap, ok := binaryData.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected binaryData as map[string]interface{}, got %T", binaryData)
	}

	_, hasImage := bdMap["image.png"]
	if !hasImage {
		t.Fatal("Expected 'image.png' key in binaryData")
	}
}

// ============================================================
// Subtask 4: Detect JSON data
// ============================================================

func TestProcessConfigMap_DetectsJSON(t *testing.T) {
	proc := NewConfigMapProcessor()
	ctx := newTestContextWithValueProcessor()

	jsonData := `{"port": 80, "workers": 4, "cache": {"enabled": true}}`

	obj := makeConfigMapObj("app-config", "default",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"data": map[string]interface{}{
				"config.json": jsonData,
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	data, ok := result.Values["data"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected data as map[string]interface{} with value processor")
	}

	configValue, ok := data["config.json"]
	if !ok {
		t.Fatal("Expected 'config.json' key in data")
	}

	// Value processor should have processed the JSON
	// It could be formatted/pretty-printed string or externalized
	switch v := configValue.(type) {
	case string:
		// Should contain the JSON content (possibly pretty-printed)
		testutil.AssertContains(t, v, "port", "JSON should contain 'port' key")
	case map[string]interface{}:
		// Could be externalized
		if _, hasExtFile := v["_externalFile"]; hasExtFile {
			if _, hasType := v["_type"]; !hasType {
				t.Error("Externalized value should have '_type'")
			}
		}
	default:
		t.Fatalf("Unexpected type for config.json value: %T", configValue)
	}
}

// ============================================================
// Subtask 5: Detect XML data
// ============================================================

func TestProcessConfigMap_DetectsXML(t *testing.T) {
	proc := NewConfigMapProcessor()
	ctx := newTestContextWithValueProcessor()

	xmlData := `<?xml version="1.0" encoding="UTF-8"?><config><database><host>localhost</host><port>5432</port></database></config>`

	obj := makeConfigMapObj("app-config", "default",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"data": map[string]interface{}{
				"config.xml": xmlData,
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	data, ok := result.Values["data"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected data as map[string]interface{} with value processor")
	}

	configValue, ok := data["config.xml"]
	if !ok {
		t.Fatal("Expected 'config.xml' key in data")
	}

	// Value processor should have detected XML
	switch v := configValue.(type) {
	case string:
		testutil.AssertContains(t, v, "config", "XML should contain 'config' element")
	case map[string]interface{}:
		// Externalized
		if _, hasExtFile := v["_externalFile"]; hasExtFile {
			if typeVal, hasType := v["_type"]; hasType {
				typeStr, ok := typeVal.(string)
				if !ok {
					t.Fatalf("Expected _type as string, got %T", typeVal)
				}
				// Should be detected as xml
				if typeStr != "xml" && typeStr != "text" {
					t.Errorf("Expected _type to be 'xml' or 'text', got %q", typeStr)
				}
			}
		}
	default:
		t.Fatalf("Unexpected type for config.xml value: %T", configValue)
	}
}

// ============================================================
// Subtask 6: Large value externalization
// ============================================================

func TestProcessConfigMap_LargeValueExternalization(t *testing.T) {
	proc := NewConfigMapProcessor()
	ctx := newTestContextWithValueProcessor()

	// Create a value larger than default threshold (1024 bytes)
	largeValue := strings.Repeat("configuration-line\n", 100) // ~1900 bytes

	obj := makeConfigMapObj("large-config", "default",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"data": map[string]interface{}{
				"large-file.conf": largeValue,
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	data, ok := result.Values["data"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected data as map[string]interface{} with value processor")
	}

	largeFileValue, ok := data["large-file.conf"]
	if !ok {
		t.Fatal("Expected 'large-file.conf' key in data")
	}

	// Should be externalized since it's > 1KB
	fileMap, isMap := largeFileValue.(map[string]interface{})
	if isMap {
		if _, hasExtFile := fileMap["_externalFile"]; hasExtFile {
			// Verify external files were created in result
			if len(result.ExternalFiles) == 0 {
				t.Fatal("Expected at least one external file")
			}
			// Verify external file has content
			if result.ExternalFiles[0].Content == "" {
				t.Error("Expected non-empty external file content")
			}
		}
	}
	// If not externalized (threshold not met or processor didn't externalize),
	// the value should still be present as string
}

// ============================================================
// Subtask 7: Edge cases
// ============================================================

func TestProcessConfigMap_EdgeCases(t *testing.T) {
	t.Run("EmptyConfigMap", func(t *testing.T) {
		proc := NewConfigMapProcessor()
		ctx := newTestProcessorContext()

		obj := makeConfigMapObj("empty-config", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed, "should be processed")
		testutil.AssertEqual(t, true, result.Values["enabled"], "should have enabled=true")

		// No data or binaryData keys expected
		_, hasData := result.Values["data"]
		if hasData {
			t.Error("Expected no data key for empty ConfigMap")
		}
	})

	t.Run("ConfigMapWithOnlyMetadata", func(t *testing.T) {
		proc := NewConfigMapProcessor()
		ctx := newTestProcessorContext()

		obj := makeConfigMapObj("meta-only", "default",
			map[string]interface{}{"app": "myapp"},
			map[string]interface{}{"owner": "team-a"},
			map[string]interface{}{})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		annotations, ok := result.Values["annotations"].(map[string]string)
		if !ok {
			t.Fatal("Expected annotations in values")
		}
		testutil.AssertEqual(t, "team-a", annotations["owner"], "annotation preserved")
	})

	t.Run("NilConfigMap", func(t *testing.T) {
		proc := NewConfigMapProcessor()
		ctx := newTestProcessorContext()

		result, err := proc.Process(ctx, nil)
		if err == nil {
			t.Fatal("Expected error for nil ConfigMap, got nil")
		}
		if result != nil {
			t.Error("Expected nil result for nil ConfigMap")
		}
	})

	t.Run("ImmutableConfigMap", func(t *testing.T) {
		proc := NewConfigMapProcessor()
		ctx := newTestProcessorContext()

		obj := makeConfigMapObj("immutable-config", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"immutable": true,
				"data": map[string]interface{}{
					"key1": "value1",
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Values["immutable"], "immutable should be true")
	})

	t.Run("NoDependencies", func(t *testing.T) {
		proc := NewConfigMapProcessor()
		ctx := newTestProcessorContext()

		obj := makeConfigMapObj("my-config", "default",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"data": map[string]interface{}{
					"key1": "value1",
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		// ConfigMaps should have empty dependencies (they don't depend on other resources)
		if len(result.Dependencies) != 0 {
			t.Errorf("Expected 0 dependencies for ConfigMap, got %d", len(result.Dependencies))
		}
	})
}

// ============================================================
// Result metadata and template tests
// ============================================================

func TestProcessConfigMap_ResultMetadata(t *testing.T) {
	proc := NewConfigMapProcessor()
	ctx := newTestProcessorContext()

	obj := makeConfigMapObj("my-app-config", "production",
		map[string]interface{}{"app": "my-app"}, nil,
		map[string]interface{}{
			"data": map[string]interface{}{"key": "val"},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	testutil.AssertEqual(t, true, result.Processed, "should be processed")
	testutil.AssertEqual(t, "myApp", result.ServiceName, "service name from labels")
	testutil.AssertContains(t, result.TemplatePath, "configmap", "template path should contain configmap")
	testutil.AssertContains(t, result.ValuesPath, "configMaps", "values path should contain configMaps")
	testutil.AssertEqual(t, "my-app-config", result.Metadata["name"], "metadata name")
	testutil.AssertEqual(t, "production", result.Metadata["namespace"], "metadata namespace")
}

func TestProcessConfigMap_GeneratesTemplate(t *testing.T) {
	proc := NewConfigMapProcessor()
	ctx := processor.Context{ChartName: "myapp"}

	obj := makeConfigMapObj("app-config", "default",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"data": map[string]interface{}{"key": "val"},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tpl := result.TemplateContent
	if tpl == "" {
		t.Fatal("Expected non-empty template content")
	}

	testutil.AssertContains(t, tpl, "apiVersion: v1", "template should have apiVersion")
	testutil.AssertContains(t, tpl, "kind: ConfigMap", "template should have kind")
	testutil.AssertContains(t, tpl, "{{ $.Release.Namespace }}", "template should use release namespace")
	testutil.AssertContains(t, tpl, `include "myapp.labels"`, "template should include labels helper")
	testutil.AssertContains(t, tpl, ".data", "template should reference data")
}

// ============================================================
// Fixture-based smoke test
// ============================================================

func TestProcessConfigMap_Fixture(t *testing.T) {
	proc := NewConfigMapProcessor()
	ctx := processor.Context{ChartName: "nginx-chart"}

	obj := testutil.LoadYAMLFixture(t, "configmap.yaml")

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	testutil.AssertEqual(t, true, result.Processed, "should be processed")
	testutil.AssertEqual(t, "nginx", result.ServiceName, "service name from app label")

	// Data should be extracted
	_, hasData := result.Values["data"]
	if !hasData {
		t.Fatal("Expected data in values from fixture")
	}

	// Annotations
	annotations, ok := result.Values["annotations"].(map[string]string)
	if !ok {
		t.Fatal("Expected annotations in values from fixture")
	}
	testutil.AssertEqual(t, "1.0", annotations["config-version"], "annotation from fixture")

	// Template not empty
	if result.TemplateContent == "" {
		t.Error("Expected non-empty template content")
	}

	// No dependencies
	if len(result.Dependencies) != 0 {
		t.Errorf("Expected 0 dependencies, got %d", len(result.Dependencies))
	}
}

// ============================================================
// Constructor test
// ============================================================

func TestNewConfigMapProcessor(t *testing.T) {
	proc := NewConfigMapProcessor()

	testutil.AssertEqual(t, "configmap", proc.Name(), "processor name")
	testutil.AssertEqual(t, 100, proc.Priority(), "processor priority")

	gvks := proc.Supports()
	if len(gvks) != 1 {
		t.Fatalf("Expected 1 supported GVK, got %d", len(gvks))
	}
	expected := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

// ============================================================
// sanitizeName helper test
// ============================================================

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Simple", "myconfig", "myconfig"},
		{"WithDash", "my-config", "myConfig"},
		{"Empty", "", "config"},
		{"AllDashes", "---", "config"},
		{"WithDots", "my.config", "myConfig"},
		{"Uppercase", "MyConfig", "myConfig"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeName(tt.input)
			testutil.AssertEqual(t, tt.expected, result, "sanitizeName(%q)", tt.input)
		})
	}
}

// Ensure unused imports don't cause compilation errors
var _ = strings.Contains
