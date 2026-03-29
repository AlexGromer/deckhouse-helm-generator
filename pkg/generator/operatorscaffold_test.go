package generator

// Test Plan: operatorscaffold_test.go
//
// | #  | Test Name                                              | Category    | Input                                             | Expected Output                                       | Notes                                  |
// |----|--------------------------------------------------------|-------------|---------------------------------------------------|-------------------------------------------------------|----------------------------------------|
// | 1  | GenerateScaffold_NilOptsName_ReturnsError              | error       | empty OperatorName                                | error returned                                        | required field guard                   |
// | 2  | GenerateScaffold_MinimalOpts_Succeeds                  | happy       | minimal valid opts                                | non-nil result, no error                              | baseline                               |
// | 3  | GenerateScaffold_ResultContainsCRDTemplate             | happy       | valid opts                                        | Result.Files contains CRD template                    | CRD file present                       |
// | 4  | GenerateScaffold_ResultContainsControllerDeployment    | happy       | valid opts                                        | Result.Files contains Deployment template             | controller file present                |
// | 5  | GenerateScaffold_ResultContainsClusterRole             | happy       | opts with ClusterScope                            | Result.Files contains ClusterRole template            | RBAC file present                      |
// | 6  | GenerateCRDTemplate_NameAndGroupPresent                | happy       | opts with CRDKind "FooBar", Group "foo.io"        | template contains "FooBar" and "foo.io"               | CRD identity in template               |
// | 7  | GenerateCRDTemplate_NamespacedScope                   | happy       | opts with CRDScope=Namespaced                     | template contains "Namespaced"                        | scope reflects in CRD                  |
// | 8  | GenerateCRDTemplate_ClusterScope                      | happy       | opts with CRDScope=Cluster                        | template contains "Cluster"                           | cluster scope CRD                      |
// | 9  | GenerateCRDTemplate_SpecFieldsPresent                 | happy       | opts with SpecFields populated                    | template contains field names                         | spec field rendering                   |
// | 10 | GenerateControllerDeployment_ContainsOperatorName     | happy       | opts with OperatorName "my-op"                    | deployment template contains "my-op"                  | naming in deployment                   |
// | 11 | GenerateControllerDeployment_LeaderElectionFlag       | happy       | opts with EnableLeaderElection=true               | deployment template contains "leader-elect"           | leader election arg                    |
// | 12 | GenerateClusterRole_ContainsCRDGroup                  | happy       | opts with CRDGroup "batch.io"                     | ClusterRole template contains "batch.io"              | RBAC group reference                   |
// | 13 | InjectScaffold_NilChart_ReturnsError                  | error       | nil chart                                         | error, count=0                                        | nil safety                             |
// | 14 | InjectScaffold_NilResult_ReturnsError                 | error       | nil result                                        | error, count=0                                        | nil safety                             |
// | 15 | InjectScaffold_FilesAddedToTemplates                  | happy       | valid chart + result                              | chart.Templates len increases                         | inject populates templates             |
// | 16 | ValidateOpts_EmptyCRDKind_ReturnsError                | error       | opts with empty CRDKind                           | ValidateOperatorScaffoldOptions returns error         | required field                         |
// | 17 | ValidateOpts_InvalidScope_ReturnsError                | error       | opts with scope "Unknown"                         | ValidateOperatorScaffoldOptions returns error         | scope must be Namespaced or Cluster    |

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func makeOperatorChart(name string) *types.GeneratedChart {
	return &types.GeneratedChart{
		Name:       name,
		ChartYAML:  "apiVersion: v2\nname: " + name + "\nversion: 0.1.0\n",
		ValuesYAML: "replicaCount: 1\n",
		Templates:  map[string]string{},
	}
}

func makeMinimalOperatorOpts(name string) OperatorScaffoldOptions {
	return OperatorScaffoldOptions{
		OperatorName: name,
		Namespace:    "default",
		CRDGroup:     "example.io",
		CRDKind:      "FooBar",
		CRDScope:     OperatorScopeNamespaced,
	}
}

// ── 1. GenerateScaffold_NilOptsName_ReturnsError ──────────────────────────────

func TestOperatorScaffold_GenerateScaffold_EmptyOperatorName_ReturnsError(t *testing.T) {
	opts := OperatorScaffoldOptions{
		OperatorName: "",
		Namespace:    "default",
		CRDGroup:     "example.io",
		CRDKind:      "FooBar",
		CRDScope:     OperatorScopeNamespaced,
	}

	result, err := GenerateOperatorScaffold(opts)
	if err == nil {
		t.Error("expected error for empty OperatorName, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result on error, got %+v", result)
	}
}

// ── 2. GenerateScaffold_MinimalOpts_Succeeds ──────────────────────────────────

func TestOperatorScaffold_GenerateScaffold_MinimalOpts_Succeeds(t *testing.T) {
	opts := makeMinimalOperatorOpts("my-operator")

	result, err := GenerateOperatorScaffold(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// ── 3. GenerateScaffold_ResultContainsCRDTemplate ────────────────────────────

func TestOperatorScaffold_GenerateScaffold_ResultContainsCRDTemplate(t *testing.T) {
	opts := makeMinimalOperatorOpts("my-operator")

	result, err := GenerateOperatorScaffold(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundCRD := false
	for path := range result.Files {
		if strings.Contains(path, "crd") || strings.Contains(path, "CRD") ||
			strings.Contains(strings.ToLower(path), "customresourcedefinition") {
			foundCRD = true
			break
		}
	}
	if !foundCRD {
		t.Errorf("expected a CRD template file in result.Files, got keys: %v", fileKeys(result.Files))
	}
}

// ── 4. GenerateScaffold_ResultContainsControllerDeployment ────────────────────

func TestOperatorScaffold_GenerateScaffold_ResultContainsControllerDeployment(t *testing.T) {
	opts := makeMinimalOperatorOpts("my-operator")

	result, err := GenerateOperatorScaffold(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundDeployment := false
	for path, content := range result.Files {
		if strings.Contains(strings.ToLower(path), "deployment") ||
			strings.Contains(content, "kind: Deployment") {
			foundDeployment = true
			break
		}
	}
	if !foundDeployment {
		t.Errorf("expected a Deployment template in result.Files, got keys: %v", fileKeys(result.Files))
	}
}

// ── 5. GenerateScaffold_ResultContainsClusterRole ────────────────────────────

func TestOperatorScaffold_GenerateScaffold_ResultContainsClusterRole(t *testing.T) {
	opts := makeMinimalOperatorOpts("my-operator")
	opts.CRDScope = OperatorScopeCluster

	result, err := GenerateOperatorScaffold(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundRBAC := false
	for path, content := range result.Files {
		if strings.Contains(strings.ToLower(path), "clusterrole") ||
			strings.Contains(strings.ToLower(path), "rbac") ||
			strings.Contains(content, "ClusterRole") {
			foundRBAC = true
			break
		}
	}
	if !foundRBAC {
		t.Errorf("expected a ClusterRole template in result.Files, got keys: %v", fileKeys(result.Files))
	}
}

// ── 6. GenerateCRDTemplate_NameAndGroupPresent ────────────────────────────────

func TestOperatorScaffold_GenerateCRDTemplate_NameAndGroupPresent(t *testing.T) {
	opts := makeMinimalOperatorOpts("my-operator")
	opts.CRDKind = "FooBar"
	opts.CRDGroup = "foo.io"

	tmpl := GenerateCRDTemplate(opts)

	if !strings.Contains(tmpl, "FooBar") {
		t.Errorf("expected CRDKind 'FooBar' in CRD template, got:\n%s", tmpl)
	}
	if !strings.Contains(tmpl, "foo.io") {
		t.Errorf("expected CRDGroup 'foo.io' in CRD template, got:\n%s", tmpl)
	}
}

// ── 7. GenerateCRDTemplate_NamespacedScope ────────────────────────────────────

func TestOperatorScaffold_GenerateCRDTemplate_NamespacedScope(t *testing.T) {
	opts := makeMinimalOperatorOpts("my-operator")
	opts.CRDScope = OperatorScopeNamespaced

	tmpl := GenerateCRDTemplate(opts)

	if !strings.Contains(tmpl, "Namespaced") {
		t.Errorf("expected 'Namespaced' scope in CRD template, got:\n%s", tmpl)
	}
}

// ── 8. GenerateCRDTemplate_ClusterScope ──────────────────────────────────────

func TestOperatorScaffold_GenerateCRDTemplate_ClusterScope(t *testing.T) {
	opts := makeMinimalOperatorOpts("my-operator")
	opts.CRDScope = OperatorScopeCluster

	tmpl := GenerateCRDTemplate(opts)

	if !strings.Contains(tmpl, "Cluster") {
		t.Errorf("expected 'Cluster' scope in CRD template, got:\n%s", tmpl)
	}
}

// ── 9. GenerateCRDTemplate_SpecFieldsPresent ──────────────────────────────────

func TestOperatorScaffold_GenerateCRDTemplate_SpecFieldsPresent(t *testing.T) {
	opts := makeMinimalOperatorOpts("my-operator")
	opts.SpecFields = []CRDSpecField{
		{Name: "replicas", Type: "integer", Description: "replica count", Required: true},
		{Name: "image", Type: "string", Description: "container image", Required: true},
	}

	tmpl := GenerateCRDTemplate(opts)

	for _, field := range opts.SpecFields {
		if !strings.Contains(tmpl, field.Name) {
			t.Errorf("expected spec field %q in CRD template, got:\n%s", field.Name, tmpl)
		}
	}
}

// ── 10. GenerateControllerDeployment_ContainsOperatorName ────────────────────

func TestOperatorScaffold_GenerateControllerDeployment_ContainsOperatorName(t *testing.T) {
	opts := makeMinimalOperatorOpts("my-op")

	tmpl := GenerateControllerDeploymentTemplate(opts)

	if !strings.Contains(tmpl, "my-op") {
		t.Errorf("expected operator name 'my-op' in controller deployment template, got:\n%s", tmpl)
	}
}

// ── 11. GenerateControllerDeployment_LeaderElectionFlag ──────────────────────

func TestOperatorScaffold_GenerateControllerDeployment_LeaderElectionFlag(t *testing.T) {
	opts := makeMinimalOperatorOpts("my-operator")
	opts.EnableLeaderElection = true

	tmpl := GenerateControllerDeploymentTemplate(opts)

	if !strings.Contains(tmpl, "leader-elect") && !strings.Contains(tmpl, "leaderElection") {
		t.Errorf("expected leader election flag in controller deployment template, got:\n%s", tmpl)
	}
}

// ── 12. GenerateClusterRole_ContainsCRDGroup ──────────────────────────────────

func TestOperatorScaffold_GenerateClusterRole_ContainsCRDGroup(t *testing.T) {
	opts := makeMinimalOperatorOpts("my-operator")
	opts.CRDGroup = "batch.io"

	tmpl := GenerateClusterRoleTemplate(opts)

	if !strings.Contains(tmpl, "batch.io") {
		t.Errorf("expected CRDGroup 'batch.io' in ClusterRole template, got:\n%s", tmpl)
	}
}

// ── 13. InjectScaffold_NilChart_ReturnsError ─────────────────────────────────

func TestOperatorScaffold_InjectScaffold_NilChart_ReturnsError(t *testing.T) {
	opts := makeMinimalOperatorOpts("my-operator")
	result, err := GenerateOperatorScaffold(opts)
	if err != nil {
		t.Fatalf("setup: unexpected error: %v", err)
	}

	updated, count, err := InjectOperatorScaffold(nil, result, opts)
	if err == nil {
		t.Error("expected error for nil chart, got nil")
	}
	if updated != nil {
		t.Errorf("expected nil chart on error, got %+v", updated)
	}
	if count != 0 {
		t.Errorf("expected count=0 on error, got %d", count)
	}
}

// ── 14. InjectScaffold_NilResult_ReturnsError ────────────────────────────────

func TestOperatorScaffold_InjectScaffold_NilResult_ReturnsError(t *testing.T) {
	chart := makeOperatorChart("my-operator")
	opts := makeMinimalOperatorOpts("my-operator")

	updated, count, err := InjectOperatorScaffold(chart, nil, opts)
	if err == nil {
		t.Error("expected error for nil result, got nil")
	}
	if updated != nil {
		t.Errorf("expected nil chart on error, got %+v", updated)
	}
	if count != 0 {
		t.Errorf("expected count=0 on error, got %d", count)
	}
}

// ── 15. InjectScaffold_FilesAddedToTemplates ──────────────────────────────────

func TestOperatorScaffold_InjectScaffold_FilesAddedToTemplates(t *testing.T) {
	chart := makeOperatorChart("my-operator")
	initialTemplateCount := len(chart.Templates)

	opts := makeMinimalOperatorOpts("my-operator")
	result, err := GenerateOperatorScaffold(opts)
	if err != nil {
		t.Fatalf("setup: unexpected error: %v", err)
	}

	updated, count, err := InjectOperatorScaffold(chart, result, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count <= 0 {
		t.Errorf("expected count > 0, got %d", count)
	}
	if len(updated.Templates) <= initialTemplateCount {
		t.Errorf("expected Templates to grow from %d, got %d", initialTemplateCount, len(updated.Templates))
	}
}

// ── 16. ValidateOpts_EmptyCRDKind_ReturnsError ───────────────────────────────

func TestOperatorScaffold_ValidateOpts_EmptyCRDKind_ReturnsError(t *testing.T) {
	opts := OperatorScaffoldOptions{
		OperatorName: "my-operator",
		Namespace:    "default",
		CRDGroup:     "example.io",
		CRDKind:      "",
		CRDScope:     OperatorScopeNamespaced,
	}

	errs := ValidateOperatorScaffoldOptions(opts)
	if len(errs) == 0 {
		t.Error("expected ValidateOperatorScaffoldOptions to return at least 1 error for empty CRDKind, got none")
	}

	mentionsKind := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "CRDKind") || strings.Contains(e.Error(), "kind") ||
			strings.Contains(e.Error(), "Kind") {
			mentionsKind = true
			break
		}
	}
	if !mentionsKind {
		t.Errorf("expected an error message referencing CRDKind, got: %v", errs)
	}
}

// ── 17. ValidateOpts_InvalidScope_ReturnsError ───────────────────────────────

func TestOperatorScaffold_ValidateOpts_InvalidScope_ReturnsError(t *testing.T) {
	opts := OperatorScaffoldOptions{
		OperatorName: "my-operator",
		Namespace:    "default",
		CRDGroup:     "example.io",
		CRDKind:      "FooBar",
		CRDScope:     OperatorScope("Unknown"),
	}

	errs := ValidateOperatorScaffoldOptions(opts)
	if len(errs) == 0 {
		t.Error("expected ValidateOperatorScaffoldOptions to return at least 1 error for unknown scope, got none")
	}

	mentionsScope := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "scope") || strings.Contains(e.Error(), "Scope") ||
			strings.Contains(e.Error(), "CRDScope") {
			mentionsScope = true
			break
		}
	}
	if !mentionsScope {
		t.Errorf("expected an error message referencing scope, got: %v", errs)
	}
}

// ── local helper ──────────────────────────────────────────────────────────────

func fileKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
