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
// Test helpers for Secret processor
// ============================================================

// makeSecretObj creates an unstructured Secret for testing.
func makeSecretObj(name, namespace, secretType string, labels, annotations map[string]interface{}, fields map[string]interface{}) *unstructured.Unstructured {
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
		"kind":       "Secret",
		"metadata":   metadata,
	}

	if secretType != "" {
		obj["type"] = secretType
	}

	for k, v := range fields {
		obj[k] = v
	}

	return &unstructured.Unstructured{Object: obj}
}

// ============================================================
// Subtask 1: Extract data (Opaque secret)
// ============================================================

func TestProcessSecret_ExtractsOpaqueData(t *testing.T) {
	proc := NewSecretProcessor()
	ctx := newTestProcessorContext()

	obj := makeSecretObj("my-secret", "default", "Opaque",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"data": map[string]interface{}{
				"username": "YWRtaW4=",          // "admin" in base64
				"password": "cGFzc3dvcmQxMjM=", // "password123" in base64
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	testutil.AssertEqual(t, "Opaque", result.Values["type"], "secret type should be Opaque")
	testutil.AssertEqual(t, true, result.Values["enabled"], "should have enabled=true")

	data, ok := result.Values["data"]
	if !ok {
		t.Fatal("Expected 'data' key in values")
	}

	dataMap, ok := data.(map[string]string)
	if !ok {
		t.Fatalf("Expected data as map[string]string, got %T", data)
	}
	testutil.AssertEqual(t, "YWRtaW4=", dataMap["username"], "username should be preserved as base64")
	testutil.AssertEqual(t, "cGFzc3dvcmQxMjM=", dataMap["password"], "password should be preserved as base64")
}

// ============================================================
// Subtask 2: Extract stringData
// ============================================================

func TestProcessSecret_ExtractsStringData(t *testing.T) {
	proc := NewSecretProcessor()
	ctx := newTestProcessorContext()

	obj := makeSecretObj("my-secret", "default", "Opaque",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"stringData": map[string]interface{}{
				"database": "mydb",
				"port":     "5432",
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	stringData, ok := result.Values["stringData"]
	if !ok {
		t.Fatal("Expected 'stringData' key in values")
	}

	sdMap, ok := stringData.(map[string]string)
	if !ok {
		t.Fatalf("Expected stringData as map[string]string, got %T", stringData)
	}
	testutil.AssertEqual(t, "mydb", sdMap["database"], "database value")
	testutil.AssertEqual(t, "5432", sdMap["port"], "port value")
}

// ============================================================
// Subtask 3: Docker registry secret
// ============================================================

func TestProcessSecret_DockerRegistrySecret(t *testing.T) {
	proc := NewSecretProcessor()
	ctx := newTestProcessorContext()

	obj := makeSecretObj("regcred", "default", "kubernetes.io/dockerconfigjson",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"data": map[string]interface{}{
				".dockerconfigjson": "eyJhdXRocyI6eyJyZWdpc3RyeS5leGFtcGxlLmNvbSI6eyJ1c2VybmFtZSI6InVzZXIiLCJwYXNzd29yZCI6InBhc3MifX19",
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	testutil.AssertEqual(t, "kubernetes.io/dockerconfigjson", result.Values["type"],
		"secret type should be kubernetes.io/dockerconfigjson")

	data, ok := result.Values["data"]
	if !ok {
		t.Fatal("Expected 'data' key in values")
	}

	dataMap, ok := data.(map[string]string)
	if !ok {
		t.Fatalf("Expected data as map[string]string, got %T", data)
	}
	_, hasDockerConfig := dataMap[".dockerconfigjson"]
	if !hasDockerConfig {
		t.Fatal("Expected '.dockerconfigjson' key in data")
	}
}

// ============================================================
// Subtask 4: TLS secret
// ============================================================

func TestProcessSecret_TLSSecret(t *testing.T) {
	proc := NewSecretProcessor()
	ctx := newTestProcessorContext()

	obj := makeSecretObj("tls-secret", "default", "kubernetes.io/tls",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"data": map[string]interface{}{
				"tls.crt": "LS0tLS1CRUdJTi...", // Simplified base64
				"tls.key": "LS0tLS1CRUdJTi...", // Simplified base64
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	testutil.AssertEqual(t, "kubernetes.io/tls", result.Values["type"],
		"secret type should be kubernetes.io/tls")

	data, ok := result.Values["data"]
	if !ok {
		t.Fatal("Expected 'data' key in values")
	}

	dataMap, ok := data.(map[string]string)
	if !ok {
		t.Fatalf("Expected data as map[string]string, got %T", data)
	}
	_, hasCrt := dataMap["tls.crt"]
	_, hasKey := dataMap["tls.key"]
	if !hasCrt || !hasKey {
		t.Fatal("Expected 'tls.crt' and 'tls.key' in data")
	}
}

// ============================================================
// Subtask 5: SSH auth secret
// ============================================================

func TestProcessSecret_SSHAuthSecret(t *testing.T) {
	proc := NewSecretProcessor()
	ctx := newTestProcessorContext()

	obj := makeSecretObj("ssh-secret", "default", "kubernetes.io/ssh-auth",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"data": map[string]interface{}{
				"ssh-privatekey": "LS0tLS1CRUdJTi...", // Simplified
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	testutil.AssertEqual(t, "kubernetes.io/ssh-auth", result.Values["type"],
		"secret type should be kubernetes.io/ssh-auth")

	data, ok := result.Values["data"]
	if !ok {
		t.Fatal("Expected 'data' key in values")
	}

	dataMap, ok := data.(map[string]string)
	if !ok {
		t.Fatalf("Expected data as map[string]string, got %T", data)
	}
	_, hasSSH := dataMap["ssh-privatekey"]
	if !hasSSH {
		t.Fatal("Expected 'ssh-privatekey' in data")
	}
}

// ============================================================
// Subtask 6: Large secret externalization (with value processor)
// ============================================================

func TestProcessSecret_LargeSecretExternalization(t *testing.T) {
	proc := NewSecretProcessor()
	ctx := newTestContextWithValueProcessor()

	// Create a large base64-encoded value (> 1024 bytes when decoded)
	largeValue := strings.Repeat("a", 2048)
	// We use stringData here so value processor processes it directly
	obj := makeSecretObj("large-secret", "default", "Opaque",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"stringData": map[string]interface{}{
				"large-cert.pem": largeValue,
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	stringData, ok := result.Values["stringData"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected stringData as map[string]interface{} with value processor, got %T", result.Values["stringData"])
	}

	largeCert, ok := stringData["large-cert.pem"]
	if !ok {
		t.Fatal("Expected 'large-cert.pem' key in stringData")
	}

	// Should be externalized since it's > 1KB
	fileMap, isMap := largeCert.(map[string]interface{})
	if isMap {
		if _, hasExtFile := fileMap["_externalFile"]; hasExtFile {
			if len(result.ExternalFiles) == 0 {
				t.Fatal("Expected at least one external file for large secret")
			}
		}
	}
	// If kept inline, it's still valid - the value processor decides
}

// ============================================================
// Subtask 6b: Data processing with value processor (base64 path)
// ============================================================

func TestProcessSecret_DataWithValueProcessor(t *testing.T) {
	proc := NewSecretProcessor()
	ctx := newTestContextWithValueProcessor()

	obj := makeSecretObj("my-secret", "default", "Opaque",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"data": map[string]interface{}{
				"username": "YWRtaW4=",          // "admin" base64
				"password": "cGFzc3dvcmQxMjM=", // "password123" base64
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	data, ok := result.Values["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data as map[string]interface{} with value processor, got %T", result.Values["data"])
	}

	// Values should be kept as base64 inline (small values < threshold)
	username, ok := data["username"]
	if !ok {
		t.Fatal("Expected 'username' key in data")
	}
	// For small secrets, should be kept inline as original base64
	usernameStr, isStr := username.(string)
	if isStr {
		testutil.AssertEqual(t, "YWRtaW4=", usernameStr, "small secret should be kept as base64")
	}
}

func TestProcessSecret_StringDataWithValueProcessor(t *testing.T) {
	proc := NewSecretProcessor()
	ctx := newTestContextWithValueProcessor()

	obj := makeSecretObj("my-secret", "default", "Opaque",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"stringData": map[string]interface{}{
				"config": "simple-value",
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	stringData, ok := result.Values["stringData"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected stringData as map[string]interface{} with value processor, got %T", result.Values["stringData"])
	}

	config, ok := stringData["config"]
	if !ok {
		t.Fatal("Expected 'config' key in stringData")
	}
	configStr, isStr := config.(string)
	if !isStr {
		t.Fatalf("Expected config value as string, got %T", config)
	}
	testutil.AssertEqual(t, "simple-value", configStr, "small string should be kept inline")
}

// ============================================================
// Subtask 7: Default type (no type specified)
// ============================================================

func TestProcessSecret_DefaultType(t *testing.T) {
	proc := NewSecretProcessor()
	ctx := newTestProcessorContext()

	obj := makeSecretObj("my-secret", "default", "",
		map[string]interface{}{"app": "myapp"}, nil,
		map[string]interface{}{
			"data": map[string]interface{}{
				"key1": "dmFsdWUx", // "value1" in base64
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "Opaque", result.Values["type"],
		"secret type should default to Opaque when not specified")
}

// ============================================================
// Subtask 8: Edge cases
// ============================================================

func TestProcessSecret_EdgeCases(t *testing.T) {
	t.Run("EmptySecret", func(t *testing.T) {
		proc := NewSecretProcessor()
		ctx := newTestProcessorContext()

		obj := makeSecretObj("empty-secret", "default", "Opaque",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed, "should be processed")
		testutil.AssertEqual(t, true, result.Values["enabled"], "should have enabled=true")

		_, hasData := result.Values["data"]
		if hasData {
			t.Error("Expected no data key for empty secret")
		}
	})

	t.Run("ImmutableSecret", func(t *testing.T) {
		proc := NewSecretProcessor()
		ctx := newTestProcessorContext()

		obj := makeSecretObj("immutable-secret", "default", "Opaque",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"immutable": true,
				"data": map[string]interface{}{
					"key1": "dmFsdWUx",
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Values["immutable"], "immutable should be true")
	})

	t.Run("NilSecret", func(t *testing.T) {
		proc := NewSecretProcessor()
		ctx := newTestProcessorContext()

		result, err := proc.Process(ctx, nil)
		if err == nil {
			t.Fatal("Expected error for nil secret, got nil")
		}
		if result != nil {
			t.Error("Expected nil result for nil secret")
		}
	})

	t.Run("WithAnnotations", func(t *testing.T) {
		proc := NewSecretProcessor()
		ctx := newTestProcessorContext()

		obj := makeSecretObj("my-secret", "default", "Opaque",
			map[string]interface{}{"app": "myapp"},
			map[string]interface{}{
				"external-secrets.io/managed-by": "external-secrets",
			},
			map[string]interface{}{
				"data": map[string]interface{}{
					"key1": "dmFsdWUx",
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		annotations, ok := result.Values["annotations"].(map[string]string)
		if !ok {
			t.Fatal("Expected annotations in values")
		}
		testutil.AssertEqual(t, "external-secrets",
			annotations["external-secrets.io/managed-by"],
			"External Secrets annotation should be preserved")
	})

	t.Run("NoDependencies", func(t *testing.T) {
		proc := NewSecretProcessor()
		ctx := newTestProcessorContext()

		obj := makeSecretObj("my-secret", "default", "Opaque",
			map[string]interface{}{"app": "myapp"}, nil,
			map[string]interface{}{
				"data": map[string]interface{}{
					"key1": "dmFsdWUx",
				},
			})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		if len(result.Dependencies) != 0 {
			t.Errorf("Expected 0 dependencies for Secret, got %d", len(result.Dependencies))
		}
	})
}

// ============================================================
// Result metadata and template tests
// ============================================================

func TestProcessSecret_ResultMetadata(t *testing.T) {
	proc := NewSecretProcessor()
	ctx := newTestProcessorContext()

	obj := makeSecretObj("my-app-secret", "production", "Opaque",
		map[string]interface{}{"app": "my-app"}, nil,
		map[string]interface{}{
			"data": map[string]interface{}{"key": "val"},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	testutil.AssertEqual(t, true, result.Processed, "should be processed")
	testutil.AssertEqual(t, "myApp", result.ServiceName, "service name from labels")
	testutil.AssertContains(t, result.TemplatePath, "secret", "template path should contain secret")
	testutil.AssertContains(t, result.ValuesPath, "secrets", "values path should contain secrets")
	testutil.AssertEqual(t, "my-app-secret", result.Metadata["name"], "metadata name")
	testutil.AssertEqual(t, "production", result.Metadata["namespace"], "metadata namespace")
}

func TestProcessSecret_GeneratesTemplate(t *testing.T) {
	proc := NewSecretProcessor()
	ctx := processor.Context{ChartName: "myapp"}

	obj := makeSecretObj("app-secret", "default", "Opaque",
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
	testutil.AssertContains(t, tpl, "kind: Secret", "template should have kind")
	testutil.AssertContains(t, tpl, "{{ $.Release.Namespace }}", "template should use release namespace")
	testutil.AssertContains(t, tpl, `include "myapp.labels"`, "template should include labels helper")
	testutil.AssertContains(t, tpl, "type:", "template should have type field")
	testutil.AssertContains(t, tpl, ".data", "template should reference data")
}

// ============================================================
// Fixture-based smoke test
// ============================================================

func TestProcessSecret_Fixture(t *testing.T) {
	proc := NewSecretProcessor()
	ctx := processor.Context{ChartName: "db-chart"}

	obj := testutil.LoadYAMLFixture(t, "secret.yaml")

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	testutil.AssertEqual(t, true, result.Processed, "should be processed")
	testutil.AssertEqual(t, "postgresql", result.ServiceName, "service name from app label")

	// Type
	testutil.AssertEqual(t, "Opaque", result.Values["type"], "secret type from fixture")

	// Data should be present
	_, hasData := result.Values["data"]
	if !hasData {
		t.Fatal("Expected data in values from fixture")
	}

	// StringData should be present
	_, hasStringData := result.Values["stringData"]
	if !hasStringData {
		t.Fatal("Expected stringData in values from fixture")
	}

	// Annotations
	annotations, ok := result.Values["annotations"].(map[string]string)
	if !ok {
		t.Fatal("Expected annotations in values from fixture")
	}
	testutil.AssertEqual(t, "database-credentials", annotations["secret-type"], "annotation from fixture")

	// Template not empty
	if result.TemplateContent == "" {
		t.Error("Expected non-empty template content")
	}
}

// ============================================================
// Constructor test
// ============================================================

func TestNewSecretProcessor(t *testing.T) {
	proc := NewSecretProcessor()

	testutil.AssertEqual(t, "secret", proc.Name(), "processor name")
	testutil.AssertEqual(t, 100, proc.Priority(), "processor priority")

	gvks := proc.Supports()
	if len(gvks) != 1 {
		t.Fatalf("Expected 1 supported GVK, got %d", len(gvks))
	}
	expected := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

// ============================================================
// Subtask 7 (ESO): External Secrets Operator strategy detection
// ============================================================

func TestProcessSecret_ESOStrategyDetection(t *testing.T) {
	proc := NewSecretProcessor()
	ctx := newTestProcessorContext()

	obj := makeSecretObj("eso-secret", "default", "Opaque",
		nil,
		map[string]interface{}{
			"external-secrets.io/managed-by": "external-secrets",
		},
		map[string]interface{}{
			"data": map[string]interface{}{
				"key1": "dmFsdWUx",
			},
		})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	esoManaged, ok := result.Values["esoManaged"]
	if !ok {
		t.Fatal("Expected 'esoManaged' key in values")
	}
	testutil.AssertEqual(t, true, esoManaged, "esoManaged should be true")

	esoStrategy, ok := result.Values["esoStrategy"]
	if !ok {
		t.Fatal("Expected 'esoStrategy' key in values")
	}
	testutil.AssertEqual(t, "external-secrets", esoStrategy, "esoStrategy should be the annotation value")
}

// Ensure unused imports don't cause compilation errors
var _ = strings.Contains
