package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

// LoadYAMLFixture loads a YAML file from fixtures/ directory and unmarshals it
// Returns unstructured.Unstructured object for flexible K8s resource handling
func LoadYAMLFixture(t *testing.T, filename string) *unstructured.Unstructured {
	t.Helper()

	data := LoadYAMLFixtureBytes(t, filename)

	obj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(data, &obj.Object); err != nil {
		t.Fatalf("Failed to unmarshal fixture %s: %v", filename, err)
	}

	return obj
}

// LoadYAMLFixtureBytes loads raw YAML bytes from fixtures/ directory
func LoadYAMLFixtureBytes(t *testing.T, filename string) []byte {
	t.Helper()

	// Try multiple possible paths (from project root or from test location at various depths)
	possiblePaths := []string{
		filepath.Join("pkg", "testutil", "fixtures", filename),
		filepath.Join("fixtures", filename),
		filepath.Join("..", "..", "pkg", "testutil", "fixtures", filename),
		filepath.Join("..", "..", "..", "pkg", "testutil", "fixtures", filename),
	}

	var data []byte
	var lastErr error

	for _, path := range possiblePaths {
		data, lastErr = os.ReadFile(path)
		if lastErr == nil {
			return data
		}
	}

	// If all paths failed, report the error
	t.Fatalf("Failed to read fixture %s from any of: %v (last error: %v)",
		filename, possiblePaths, lastErr)
	return nil
}

// MockK8sObject creates a mock K8s object for testing
// Returns a map that can be used in tests
func MockK8sObject(kind, name, namespace string, spec map[string]interface{}) map[string]interface{} {
	obj := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       kind,
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}

	if spec != nil {
		obj["spec"] = spec
	}

	return obj
}

// CompareHelm performs deep equality check for Helm chart structures
// Ignores order of arrays and whitespace differences in templates
func CompareHelm(t *testing.T, expected, actual map[string]interface{}) {
	t.Helper()

	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Helm chart mismatch:\nExpected: %+v\nActual: %+v", expected, actual)
	}
}

// CompareHelmValues compares Helm values.yaml content with normalization
func CompareHelmValues(t *testing.T, expected, actual string) {
	t.Helper()

	// Normalize: trim whitespace, remove empty lines
	normalizeYAML := func(s string) string {
		lines := strings.Split(s, "\n")
		var normalized []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				normalized = append(normalized, trimmed)
			}
		}
		return strings.Join(normalized, "\n")
	}

	expNorm := normalizeYAML(expected)
	actNorm := normalizeYAML(actual)

	if expNorm != actNorm {
		t.Errorf("Helm values mismatch:\nExpected:\n%s\n\nActual:\n%s", expected, actual)
	}
}

// AssertErrorContains checks if error message contains expected substring
func AssertErrorContains(t *testing.T, err error, expectedMsg string) {
	t.Helper()

	if err == nil {
		t.Fatalf("Expected error containing '%s', got nil", expectedMsg)
	}

	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Error message '%s' does not contain '%s'", err.Error(), expectedMsg)
	}
}

// AssertNoError is a convenience wrapper for checking nil errors
func AssertNoError(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
}

// AssertEqual performs deep equality check with custom error message
func AssertEqual(t *testing.T, expected, actual interface{}, msgAndArgs ...interface{}) {
	t.Helper()

	if !reflect.DeepEqual(expected, actual) {
		var msg string
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(msgAndArgs[0].(string), msgAndArgs[1:]...)
		}
		t.Errorf("%s\nExpected: %+v\nActual: %+v", msg, expected, actual)
	}
}

// AssertContains checks if a string contains a substring
func AssertContains(t *testing.T, haystack, needle string, msgAndArgs ...interface{}) {
	t.Helper()

	if !strings.Contains(haystack, needle) {
		var msg string
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(msgAndArgs[0].(string), msgAndArgs[1:]...)
		}
		t.Errorf("%s\nString '%s' does not contain '%s'", msg, haystack, needle)
	}
}

// CreateUnstructured creates an unstructured K8s object from GVK and spec
func CreateUnstructured(gvk schema.GroupVersionKind, name, namespace string, spec map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetName(name)
	obj.SetNamespace(namespace)

	if spec != nil {
		obj.Object["spec"] = spec
	}

	return obj
}

// ExtractField safely extracts a nested field from unstructured object
// Normalizes numeric types: YAML unmarshaling returns float64 for numbers,
// but we convert to int64 for integer values to match expected types in tests
func ExtractField(t *testing.T, obj *unstructured.Unstructured, fields ...string) interface{} {
	t.Helper()

	value, found, err := unstructured.NestedFieldCopy(obj.Object, fields...)
	if err != nil {
		t.Fatalf("Failed to extract field %v: %v", fields, err)
	}
	if !found {
		t.Fatalf("Field %v not found in object", fields)
	}

	// Normalize numeric types: YAML unmarshaling returns float64 for all numbers
	// Convert to int64 if it's a whole number (e.g., 3.0 → 3)
	if f, ok := value.(float64); ok {
		if f == float64(int64(f)) {
			return int64(f)
		}
	}

	return value
}

// MustConvert converts unstructured to typed object, fails test on error
func MustConvert(t *testing.T, obj *unstructured.Unstructured, target runtime.Object) {
	t.Helper()

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, target); err != nil {
		t.Fatalf("Failed to convert unstructured to %T: %v", target, err)
	}
}

// CreateTempDir creates a temporary directory for test artifacts
func CreateTempDir(t *testing.T, pattern string) string {
	t.Helper()

	dir, err := os.MkdirTemp("", pattern)
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Cleanup on test completion
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})

	return dir
}

// WriteFile is a test helper to write content to a file in temp directory
func WriteFile(t *testing.T, dir, filename, content string) string {
	t.Helper()

	filePath := filepath.Join(dir, filename)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write file %s: %v", filename, err)
	}

	return filePath
}
