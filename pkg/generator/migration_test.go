package generator

import (
	"fmt"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// DetectDrift Tests
// ============================================================

func TestDetectDrift_IdenticalCharts(t *testing.T) {
	chart := &types.GeneratedChart{
		Name:       "myapp",
		ChartYAML:  "apiVersion: v2\nname: myapp\nversion: 0.1.0\n",
		ValuesYAML: "replicaCount: 1\n",
		Templates: map[string]string{
			"templates/deployment.yaml": "kind: Deployment\nmetadata:\n  name: myapp\n",
		},
		Helpers: "{{- define \"myapp.name\" -}}myapp{{- end -}}",
	}

	report := DetectDrift(chart, chart)

	if report.HasDrift() {
		t.Errorf("expected no drift for identical charts, got %d items", report.TotalItems())
	}
}

func TestDetectDrift_BothNil(t *testing.T) {
	report := DetectDrift(nil, nil)
	if report.HasDrift() {
		t.Error("expected no drift for nil charts")
	}
}

func TestDetectDrift_TemplateAdded(t *testing.T) {
	existing := &types.GeneratedChart{
		Name: "myapp",
		Templates: map[string]string{
			"templates/deployment.yaml": "kind: Deployment",
		},
	}
	newChart := &types.GeneratedChart{
		Name: "myapp",
		Templates: map[string]string{
			"templates/deployment.yaml": "kind: Deployment",
			"templates/service.yaml":    "kind: Service",
		},
	}

	report := DetectDrift(existing, newChart)

	if !report.HasDrift() {
		t.Fatal("expected drift")
	}
	if len(report.Templates) != 1 {
		t.Fatalf("expected 1 template drift, got %d", len(report.Templates))
	}
	if report.Templates[0].Category != DriftAdded {
		t.Errorf("expected added, got %s", report.Templates[0].Category)
	}
	if report.Templates[0].Path != "templates/service.yaml" {
		t.Errorf("expected templates/service.yaml, got %s", report.Templates[0].Path)
	}
}

func TestDetectDrift_TemplateRemoved(t *testing.T) {
	existing := &types.GeneratedChart{
		Name: "myapp",
		Templates: map[string]string{
			"templates/deployment.yaml": "kind: Deployment",
			"templates/service.yaml":    "kind: Service",
		},
	}
	newChart := &types.GeneratedChart{
		Name: "myapp",
		Templates: map[string]string{
			"templates/deployment.yaml": "kind: Deployment",
		},
	}

	report := DetectDrift(existing, newChart)

	if len(report.Templates) != 1 {
		t.Fatalf("expected 1 template drift, got %d", len(report.Templates))
	}
	if report.Templates[0].Category != DriftRemoved {
		t.Errorf("expected removed, got %s", report.Templates[0].Category)
	}
}

func TestDetectDrift_TemplateChanged(t *testing.T) {
	existing := &types.GeneratedChart{
		Name: "myapp",
		Templates: map[string]string{
			"templates/deployment.yaml": "replicas: 1",
		},
	}
	newChart := &types.GeneratedChart{
		Name: "myapp",
		Templates: map[string]string{
			"templates/deployment.yaml": "replicas: 3",
		},
	}

	report := DetectDrift(existing, newChart)

	if len(report.Templates) != 1 {
		t.Fatalf("expected 1 template drift, got %d", len(report.Templates))
	}
	if report.Templates[0].Category != DriftChanged {
		t.Errorf("expected changed, got %s", report.Templates[0].Category)
	}
}

func TestDetectDrift_ValuesAdded(t *testing.T) {
	existing := &types.GeneratedChart{
		Name:       "myapp",
		ValuesYAML: "replicaCount: 1\n",
		Templates:  map[string]string{},
	}
	newChart := &types.GeneratedChart{
		Name:       "myapp",
		ValuesYAML: "replicaCount: 1\nimage:\n  repository: nginx\n",
		Templates:  map[string]string{},
	}

	report := DetectDrift(existing, newChart)

	if len(report.Values) != 1 {
		t.Fatalf("expected 1 values drift, got %d", len(report.Values))
	}
	if report.Values[0].Category != DriftAdded {
		t.Errorf("expected added, got %s", report.Values[0].Category)
	}
	if report.Values[0].Path != "image.repository" {
		t.Errorf("expected image.repository, got %s", report.Values[0].Path)
	}
}

func TestDetectDrift_ValuesRemoved(t *testing.T) {
	existing := &types.GeneratedChart{
		Name:       "myapp",
		ValuesYAML: "replicaCount: 1\noldKey: value\n",
		Templates:  map[string]string{},
	}
	newChart := &types.GeneratedChart{
		Name:       "myapp",
		ValuesYAML: "replicaCount: 1\n",
		Templates:  map[string]string{},
	}

	report := DetectDrift(existing, newChart)

	found := false
	for _, item := range report.Values {
		if item.Path == "oldKey" && item.Category == DriftRemoved {
			found = true
		}
	}
	if !found {
		t.Error("expected removed drift for oldKey")
	}
}

func TestDetectDrift_ValuesChanged(t *testing.T) {
	existing := &types.GeneratedChart{
		Name:       "myapp",
		ValuesYAML: "replicaCount: 1\n",
		Templates:  map[string]string{},
	}
	newChart := &types.GeneratedChart{
		Name:       "myapp",
		ValuesYAML: "replicaCount: 3\n",
		Templates:  map[string]string{},
	}

	report := DetectDrift(existing, newChart)

	if len(report.Values) != 1 {
		t.Fatalf("expected 1 values drift, got %d", len(report.Values))
	}
	if report.Values[0].Category != DriftChanged {
		t.Errorf("expected changed, got %s", report.Values[0].Category)
	}
}

func TestDetectDrift_HelpersAdded(t *testing.T) {
	existing := &types.GeneratedChart{
		Name:      "myapp",
		Templates: map[string]string{},
	}
	newChart := &types.GeneratedChart{
		Name:      "myapp",
		Helpers:   "{{- define \"myapp.name\" -}}myapp{{- end -}}",
		Templates: map[string]string{},
	}

	report := DetectDrift(existing, newChart)

	if len(report.Helpers) != 1 {
		t.Fatalf("expected 1 helpers drift, got %d", len(report.Helpers))
	}
	if report.Helpers[0].Category != DriftAdded {
		t.Errorf("expected added, got %s", report.Helpers[0].Category)
	}
}

func TestDetectDrift_HelpersRemoved(t *testing.T) {
	existing := &types.GeneratedChart{
		Name:      "myapp",
		Helpers:   "{{- define \"myapp.name\" -}}myapp{{- end -}}",
		Templates: map[string]string{},
	}
	newChart := &types.GeneratedChart{
		Name:      "myapp",
		Templates: map[string]string{},
	}

	report := DetectDrift(existing, newChart)

	if len(report.Helpers) != 1 {
		t.Fatalf("expected 1 helpers drift, got %d", len(report.Helpers))
	}
	if report.Helpers[0].Category != DriftRemoved {
		t.Errorf("expected removed, got %s", report.Helpers[0].Category)
	}
}

func TestDetectDrift_HelpersChanged(t *testing.T) {
	existing := &types.GeneratedChart{
		Name:      "myapp",
		Helpers:   "{{- define \"myapp.name\" -}}myapp{{- end -}}",
		Templates: map[string]string{},
	}
	newChart := &types.GeneratedChart{
		Name:      "myapp",
		Helpers:   "{{- define \"myapp.name\" -}}myapp-v2{{- end -}}",
		Templates: map[string]string{},
	}

	report := DetectDrift(existing, newChart)

	if len(report.Helpers) != 1 {
		t.Fatalf("expected 1 helpers drift, got %d", len(report.Helpers))
	}
	if report.Helpers[0].Category != DriftChanged {
		t.Errorf("expected changed, got %s", report.Helpers[0].Category)
	}
}

func TestDetectDrift_ComprehensiveDrift(t *testing.T) {
	existing := &types.GeneratedChart{
		Name:       "myapp",
		ValuesYAML: "replicaCount: 1\noldSetting: true\n",
		Templates: map[string]string{
			"templates/deployment.yaml": "replicas: 1",
			"templates/old.yaml":        "kind: ConfigMap",
		},
		Helpers: "old helpers",
	}
	newChart := &types.GeneratedChart{
		Name:       "myapp",
		ValuesYAML: "replicaCount: 3\nnewSetting: enabled\n",
		Templates: map[string]string{
			"templates/deployment.yaml": "replicas: 3",
			"templates/service.yaml":    "kind: Service",
		},
		Helpers: "new helpers",
	}

	report := DetectDrift(existing, newChart)

	if !report.HasDrift() {
		t.Fatal("expected drift")
	}
	// Templates: deployment changed, old removed, service added = 3
	if len(report.Templates) != 3 {
		t.Errorf("expected 3 template drifts, got %d", len(report.Templates))
	}
	// Values: replicaCount changed, oldSetting removed, newSetting added = 3
	if len(report.Values) != 3 {
		t.Errorf("expected 3 values drifts, got %d", len(report.Values))
	}
	// Helpers: changed = 1
	if len(report.Helpers) != 1 {
		t.Errorf("expected 1 helpers drift, got %d", len(report.Helpers))
	}
}

func TestDetectDrift_NilExisting(t *testing.T) {
	newChart := &types.GeneratedChart{
		Name: "myapp",
		Templates: map[string]string{
			"templates/deployment.yaml": "kind: Deployment",
		},
		ValuesYAML: "replicaCount: 1\n",
		Helpers:    "some helpers",
	}

	report := DetectDrift(nil, newChart)

	if !report.HasDrift() {
		t.Fatal("expected drift when existing is nil")
	}
	if len(report.Templates) != 1 {
		t.Errorf("expected 1 template added, got %d", len(report.Templates))
	}
	if report.Templates[0].Category != DriftAdded {
		t.Errorf("expected added, got %s", report.Templates[0].Category)
	}
}

// ============================================================
// GenerateMigrationPlan Tests
// ============================================================

func TestGenerateMigrationPlan_NoDrift(t *testing.T) {
	plan := GenerateMigrationPlan(DriftReport{})

	if !strings.Contains(plan, "No changes detected") {
		t.Error("expected 'No changes detected' message")
	}
}

func TestGenerateMigrationPlan_OrdersPhases(t *testing.T) {
	drift := DriftReport{
		Templates: []DriftItem{
			{Category: DriftRemoved, Path: "templates/old.yaml", Detail: "template removed"},
			{Category: DriftAdded, Path: "templates/new.yaml", Detail: "new template"},
			{Category: DriftChanged, Path: "templates/deploy.yaml", Detail: "content changed"},
		},
	}

	plan := GenerateMigrationPlan(drift)

	addIdx := strings.Index(plan, "Phase 1: Additions")
	modIdx := strings.Index(plan, "Phase 2: Modifications")
	remIdx := strings.Index(plan, "Phase 3: Removals")

	if addIdx == -1 || modIdx == -1 || remIdx == -1 {
		t.Fatalf("missing phases in plan:\n%s", plan)
	}

	if addIdx >= modIdx || modIdx >= remIdx {
		t.Error("phases not in correct order: additions < modifications < removals")
	}
}

func TestGenerateMigrationPlan_TotalCount(t *testing.T) {
	drift := DriftReport{
		Templates: []DriftItem{
			{Category: DriftAdded, Path: "a", Detail: "added"},
		},
		Values: []DriftItem{
			{Category: DriftChanged, Path: "b", Detail: "changed"},
		},
		Helpers: []DriftItem{
			{Category: DriftRemoved, Path: "c", Detail: "removed"},
		},
	}

	plan := GenerateMigrationPlan(drift)

	if !strings.Contains(plan, "Total changes: 3") {
		t.Errorf("expected 'Total changes: 3' in plan:\n%s", plan)
	}
}

func TestGenerateMigrationPlan_OnlyAdditions(t *testing.T) {
	drift := DriftReport{
		Templates: []DriftItem{
			{Category: DriftAdded, Path: "templates/new.yaml", Detail: "new template"},
		},
	}

	plan := GenerateMigrationPlan(drift)

	if !strings.Contains(plan, "Phase 1: Additions") {
		t.Error("expected Phase 1")
	}
	if strings.Contains(plan, "Phase 2") || strings.Contains(plan, "Phase 3") {
		t.Error("should not contain Phase 2 or Phase 3 when only additions")
	}
}

// ============================================================
// GenerateValuesMigration Tests
// ============================================================

func TestGenerateValuesMigration_NoRenames(t *testing.T) {
	old := "replicaCount: 1\n"
	new := "replicaCount: 1\n"

	result := GenerateValuesMigration(old, new)

	if !strings.Contains(result, "No value migrations needed") {
		t.Errorf("expected no migrations message, got:\n%s", result)
	}
}

func TestGenerateValuesMigration_DetectsRenamedKey(t *testing.T) {
	old := "image:\n  repo: nginx\n"
	new := "image:\n  repository: nginx\n"

	result := GenerateValuesMigration(old, new)

	if !strings.Contains(result, "coalesce") {
		t.Errorf("expected coalesce in migration template, got:\n%s", result)
	}
	if !strings.Contains(result, ".Values.image.repository") {
		t.Errorf("expected new path .Values.image.repository, got:\n%s", result)
	}
	if !strings.Contains(result, ".Values.image.repo") {
		t.Errorf("expected old path .Values.image.repo, got:\n%s", result)
	}
}

func TestGenerateValuesMigration_SameLeafName(t *testing.T) {
	old := "old:\n  port: 8080\n"
	new := "new:\n  port: 8080\n"

	result := GenerateValuesMigration(old, new)

	if !strings.Contains(result, "coalesce") {
		t.Errorf("expected coalesce for renamed key, got:\n%s", result)
	}
	if !strings.Contains(result, ".Values.new.port") {
		t.Errorf("expected .Values.new.port, got:\n%s", result)
	}
	if !strings.Contains(result, ".Values.old.port") {
		t.Errorf("expected .Values.old.port, got:\n%s", result)
	}
}

func TestGenerateValuesMigration_EmptyInputs(t *testing.T) {
	result := GenerateValuesMigration("", "")
	if !strings.Contains(result, "No value migrations needed") {
		t.Errorf("expected no migrations for empty inputs, got:\n%s", result)
	}
}

func TestGenerateValuesMigration_ContainsDefine(t *testing.T) {
	old := "old:\n  name: test\n"
	new := "new:\n  name: test\n"

	result := GenerateValuesMigration(old, new)

	if !strings.Contains(result, "{{- define \"migrate.values\" -}}") {
		t.Errorf("expected define block, got:\n%s", result)
	}
	if !strings.Contains(result, "{{- end -}}") {
		t.Errorf("expected end block, got:\n%s", result)
	}
}

// ============================================================
// Helper function tests
// ============================================================

func TestFlattenYAML_Nested(t *testing.T) {
	yamlStr := "image:\n  repository: nginx\n  tag: latest\nreplicaCount: 1\n"
	flat := flattenYAML(yamlStr)

	expected := map[string]string{
		"image.repository": "nginx",
		"image.tag":        "latest",
		"replicaCount":     "1",
	}

	for k, v := range expected {
		got, ok := flat[k]
		if !ok {
			t.Errorf("missing key %s", k)
			continue
		}
		if strings.TrimSpace(v) != strings.TrimSpace(fmt.Sprintf("%v", got)) {
			t.Errorf("key %s: expected %s, got %v", k, v, got)
		}
	}
}

func TestFlattenYAML_Invalid(t *testing.T) {
	flat := flattenYAML("{{invalid yaml")
	if len(flat) != 0 {
		t.Errorf("expected empty map for invalid YAML, got %d entries", len(flat))
	}
}

func TestFlattenYAML_Empty(t *testing.T) {
	flat := flattenYAML("")
	if len(flat) != 0 {
		t.Errorf("expected empty map for empty string, got %d entries", len(flat))
	}
}

func TestLeafName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"image.repository", "repository"},
		{"replicaCount", "replicaCount"},
		{"a.b.c.d", "d"},
	}
	for _, tt := range tests {
		got := leafName(tt.input)
		if got != tt.want {
			t.Errorf("leafName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestToValuesPath(t *testing.T) {
	got := toValuesPath("image.repository")
	want := ".Values.image.repository"
	if got != want {
		t.Errorf("toValuesPath: got %q, want %q", got, want)
	}
}

func TestSanitizeVarName(t *testing.T) {
	got := sanitizeVarName("image.repository")
	want := "image_repository"
	if got != want {
		t.Errorf("sanitizeVarName: got %q, want %q", got, want)
	}
}
