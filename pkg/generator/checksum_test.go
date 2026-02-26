package generator

import (
	"fmt"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// GenerateChecksumAnnotations Tests
// ============================================================

func TestChecksumAnnotations_NoDependencies_ReturnsEmpty(t *testing.T) {
	annotations := GenerateChecksumAnnotations("myapp", nil)

	if len(annotations) != 0 {
		t.Errorf("expected 0 annotations, got %d", len(annotations))
	}
}

func TestChecksumAnnotations_EmptyDependencies_ReturnsEmpty(t *testing.T) {
	annotations := GenerateChecksumAnnotations("myapp", []types.ResourceKey{})

	if len(annotations) != 0 {
		t.Errorf("expected 0 annotations, got %d", len(annotations))
	}
}

func TestChecksumAnnotations_ConfigMapDependency_GeneratesConfigChecksum(t *testing.T) {
	deps := []types.ResourceKey{
		{
			GVK:  schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"},
			Name: "myapp-config",
		},
	}

	annotations := GenerateChecksumAnnotations("myapp", deps)

	if len(annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(annotations))
	}

	ann := annotations[0]
	expectedKey := "checksum/config-myapp-config"
	if ann.Key != expectedKey {
		t.Errorf("expected key %q, got %q", expectedKey, ann.Key)
	}

	expectedTemplatePath := "myapp-configmap.yaml"
	if ann.TemplatePath != expectedTemplatePath {
		t.Errorf("expected template path %q, got %q", expectedTemplatePath, ann.TemplatePath)
	}

	expectedExpr := fmt.Sprintf(`{{ include (print $.Template.BasePath "/%s") . | sha256sum }}`, expectedTemplatePath)
	if ann.Expression != expectedExpr {
		t.Errorf("expected expression %q, got %q", expectedExpr, ann.Expression)
	}
}

func TestChecksumAnnotations_SecretDependency_GeneratesSecretChecksum(t *testing.T) {
	deps := []types.ResourceKey{
		{
			GVK:  schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"},
			Name: "myapp-creds",
		},
	}

	annotations := GenerateChecksumAnnotations("myapp", deps)

	if len(annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(annotations))
	}

	ann := annotations[0]
	expectedKey := "checksum/secret-myapp-creds"
	if ann.Key != expectedKey {
		t.Errorf("expected key %q, got %q", expectedKey, ann.Key)
	}

	expectedTemplatePath := "myapp-secret.yaml"
	if ann.TemplatePath != expectedTemplatePath {
		t.Errorf("expected template path %q, got %q", expectedTemplatePath, ann.TemplatePath)
	}

	if !strings.Contains(ann.Expression, "sha256sum") {
		t.Errorf("expression should contain 'sha256sum', got %q", ann.Expression)
	}
	if !strings.Contains(ann.Expression, expectedTemplatePath) {
		t.Errorf("expression should reference template path %q, got %q", expectedTemplatePath, ann.Expression)
	}
}

func TestChecksumAnnotations_BothConfigMapAndSecret_GeneratesBoth(t *testing.T) {
	deps := []types.ResourceKey{
		{
			GVK:  schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"},
			Name: "app-config",
		},
		{
			GVK:  schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"},
			Name: "app-secret",
		},
	}

	annotations := GenerateChecksumAnnotations("myapp", deps)

	if len(annotations) != 2 {
		t.Fatalf("expected 2 annotations, got %d", len(annotations))
	}

	keys := make(map[string]bool)
	for _, ann := range annotations {
		keys[ann.Key] = true
	}

	if !keys["checksum/config-app-config"] {
		t.Error("expected 'checksum/config-app-config' annotation")
	}
	if !keys["checksum/secret-app-secret"] {
		t.Error("expected 'checksum/secret-app-secret' annotation")
	}
}

func TestChecksumAnnotations_DuplicateDependencies_Deduplicated(t *testing.T) {
	deps := []types.ResourceKey{
		{
			GVK:  schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"},
			Name: "myapp-config",
		},
		{
			GVK:  schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"},
			Name: "myapp-config", // duplicate
		},
		{
			GVK:  schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"},
			Name: "myapp-secret",
		},
		{
			GVK:  schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"},
			Name: "myapp-secret", // duplicate
		},
	}

	annotations := GenerateChecksumAnnotations("myapp", deps)

	if len(annotations) != 2 {
		t.Errorf("expected 2 deduplicated annotations, got %d", len(annotations))
	}
}

func TestChecksumAnnotations_UnrelatedKinds_Ignored(t *testing.T) {
	deps := []types.ResourceKey{
		{
			GVK:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			Name: "myapp",
		},
		{
			GVK:  schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"},
			Name: "myapp-svc",
		},
	}

	annotations := GenerateChecksumAnnotations("myapp", deps)

	if len(annotations) != 0 {
		t.Errorf("expected 0 annotations for non-ConfigMap/Secret kinds, got %d", len(annotations))
	}
}

func TestChecksumAnnotations_ServiceNameUsedInTemplatePath(t *testing.T) {
	deps := []types.ResourceKey{
		{
			GVK:  schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"},
			Name: "config",
		},
	}

	annotations := GenerateChecksumAnnotations("frontend", deps)

	if len(annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(annotations))
	}

	if annotations[0].TemplatePath != "frontend-configmap.yaml" {
		t.Errorf("expected template path 'frontend-configmap.yaml', got %q", annotations[0].TemplatePath)
	}
}

// ============================================================
// FormatChecksumAnnotations Tests
// ============================================================

func TestFormatChecksumAnnotations_Empty_ReturnsEmptyString(t *testing.T) {
	result := FormatChecksumAnnotations(nil)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestFormatChecksumAnnotations_EmptySlice_ReturnsEmptyString(t *testing.T) {
	result := FormatChecksumAnnotations([]ChecksumAnnotation{})
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestFormatChecksumAnnotations_SingleAnnotation_CorrectYAML(t *testing.T) {
	annotations := []ChecksumAnnotation{
		{
			Key:        "checksum/config-myapp",
			Expression: `{{ include (print $.Template.BasePath "/myapp-configmap.yaml") . | sha256sum }}`,
		},
	}

	result := FormatChecksumAnnotations(annotations)

	if !strings.Contains(result, "checksum/config-myapp:") {
		t.Errorf("result should contain annotation key, got: %q", result)
	}
	if !strings.Contains(result, "sha256sum") {
		t.Errorf("result should contain sha256sum expression, got: %q", result)
	}
	if !strings.HasSuffix(result, "\n") {
		t.Errorf("result should end with newline, got: %q", result)
	}
	// Check the 8-space indentation
	if !strings.HasPrefix(result, "        checksum/") {
		t.Errorf("result should start with 8-space indentation, got: %q", result)
	}
}

func TestFormatChecksumAnnotations_MultipleAnnotations_AllIncluded(t *testing.T) {
	annotations := []ChecksumAnnotation{
		{
			Key:        "checksum/config-myapp",
			Expression: `{{ include (print $.Template.BasePath "/myapp-configmap.yaml") . | sha256sum }}`,
		},
		{
			Key:        "checksum/secret-myapp",
			Expression: `{{ include (print $.Template.BasePath "/myapp-secret.yaml") . | sha256sum }}`,
		},
	}

	result := FormatChecksumAnnotations(annotations)

	if !strings.Contains(result, "checksum/config-myapp:") {
		t.Error("result should contain config annotation")
	}
	if !strings.Contains(result, "checksum/secret-myapp:") {
		t.Error("result should contain secret annotation")
	}

	lines := strings.Split(strings.TrimRight(result, "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d: %q", len(lines), result)
	}
}
