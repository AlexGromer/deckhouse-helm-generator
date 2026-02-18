package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TestLoadYAMLFixture verifies fixture loading works correctly
func TestLoadYAMLFixture(t *testing.T) {
	// Test loading deployment fixture
	obj := LoadYAMLFixture(t, "deployment.yaml")

	// Verify basic fields
	if obj.GetKind() != "Deployment" {
		t.Errorf("Expected Kind=Deployment, got %s", obj.GetKind())
	}

	if obj.GetName() != "nginx-deployment" {
		t.Errorf("Expected Name=nginx-deployment, got %s", obj.GetName())
	}

	if obj.GetNamespace() != "default" {
		t.Errorf("Expected Namespace=default, got %s", obj.GetNamespace())
	}
}

// TestLoadYAMLFixtureBytes verifies raw YAML loading
func TestLoadYAMLFixtureBytes(t *testing.T) {
	data := LoadYAMLFixtureBytes(t, "service.yaml")

	if len(data) == 0 {
		t.Fatal("Expected non-empty YAML data")
	}

	// Should contain "kind: Service"
	dataStr := string(data)
	AssertContains(t, dataStr, "kind: Service", "YAML should contain Service kind")
	AssertContains(t, dataStr, "nginx-service", "YAML should contain service name")
}

// TestMockK8sObject verifies mock object creation
func TestMockK8sObject(t *testing.T) {
	spec := map[string]interface{}{
		"replicas": 3,
	}

	obj := MockK8sObject("Deployment", "test-deploy", "default", spec)

	if obj == nil {
		t.Fatal("Expected non-nil mock object")
	}

	AssertEqual(t, "Deployment", obj["kind"], "Kind should match")
	AssertEqual(t, "test-deploy", obj["metadata"].(map[string]interface{})["name"], "Name should match")
}

// TestAssertErrorContains verifies error assertion helper
func TestAssertErrorContains(t *testing.T) {
	testErr := os.ErrNotExist

	// This should NOT fail the test (os.ErrNotExist contains "file does not exist")
	AssertErrorContains(t, testErr, "does not exist")
}

// TestAssertNoError verifies nil error check
func TestAssertNoError(t *testing.T) {
	AssertNoError(t, nil)
}

// TestAssertEqual verifies equality check
func TestAssertEqual(t *testing.T) {
	AssertEqual(t, 42, 42, "Numbers should be equal")
	AssertEqual(t, "test", "test", "Strings should be equal")

	m1 := map[string]int{"a": 1, "b": 2}
	m2 := map[string]int{"a": 1, "b": 2}
	AssertEqual(t, m1, m2, "Maps should be equal")
}

// TestAssertContains verifies substring check
func TestAssertContains(t *testing.T) {
	AssertContains(t, "hello world", "world", "Should contain substring")
	AssertContains(t, "deployment.yaml", ".yaml", "Should contain extension")
}

// TestCreateUnstructured verifies unstructured object creation
func TestCreateUnstructured(t *testing.T) {
	gvk := schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}

	spec := map[string]interface{}{
		"replicas": int64(3),
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"app": "test",
			},
		},
	}

	obj := CreateUnstructured(gvk, "test-deployment", "default", spec)

	AssertEqual(t, "Deployment", obj.GetKind(), "Kind should match")
	AssertEqual(t, "test-deployment", obj.GetName(), "Name should match")
	AssertEqual(t, "default", obj.GetNamespace(), "Namespace should match")

	// Verify spec was set
	if obj.Object["spec"] == nil {
		t.Error("Spec should be set")
	}
}

// TestExtractField verifies nested field extraction
func TestExtractField(t *testing.T) {
	obj := LoadYAMLFixture(t, "deployment.yaml")

	// Extract replicas (spec.replicas)
	// ExtractField should normalize float64 → int64 for whole numbers
	replicas := ExtractField(t, obj, "spec", "replicas")
	AssertEqual(t, int64(3), replicas, "Replicas should be 3")

	// Extract selector labels
	selector := ExtractField(t, obj, "spec", "selector", "matchLabels", "app")
	AssertEqual(t, "nginx", selector, "Selector app label should be nginx")
}

// TestCreateTempDir verifies temp directory creation with cleanup
func TestCreateTempDir(t *testing.T) {
	dir := CreateTempDir(t, "dhg-test-*")

	// Directory should exist
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("Temp directory should exist: %s", dir)
	}

	// Should be able to write to it
	testFile := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Errorf("Should be able to write to temp dir: %v", err)
	}

	// After test completes, t.Cleanup() will remove the directory
}

// TestWriteFile verifies file writing in temp directory
func TestWriteFile(t *testing.T) {
	dir := CreateTempDir(t, "dhg-write-test-*")

	content := "apiVersion: v1\nkind: ConfigMap"
	filePath := WriteFile(t, dir, "test-config.yaml", content)

	// File should exist
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Written file should exist: %s", filePath)
	}

	// Content should match
	readContent, err := os.ReadFile(filePath)
	AssertNoError(t, err)
	AssertEqual(t, content, string(readContent), "File content should match")
}

// TestCompareHelmValues verifies Helm values comparison with normalization
func TestCompareHelmValues(t *testing.T) {
	values1 := `
# Comment
replicas: 3
image:
  repository: nginx
  tag: latest
`

	values2 := `
replicas: 3

image:
  repository: nginx
  tag: latest
# Another comment
`

	// Should be equal after normalization (ignoring comments and whitespace)
	CompareHelmValues(t, values1, values2)
}

// TestAllFixturesLoadable verifies all fixtures can be loaded
func TestAllFixturesLoadable(t *testing.T) {
	fixtures := []string{
		"deployment.yaml",
		"statefulset.yaml",
		"service.yaml",
		"configmap.yaml",
		"secret.yaml",
	}

	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			obj := LoadYAMLFixture(t, fixture)
			if obj == nil {
				t.Errorf("Failed to load fixture: %s", fixture)
			}

			// Verify kind is set
			if obj.GetKind() == "" {
				t.Errorf("Fixture %s should have a Kind", fixture)
			}

			// Verify name is set
			if obj.GetName() == "" {
				t.Errorf("Fixture %s should have a Name", fixture)
			}
		})
	}
}
