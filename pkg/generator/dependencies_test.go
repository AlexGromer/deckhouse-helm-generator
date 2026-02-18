package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// Subtask 1: Detect cross-chart dependencies from relationships
// ============================================================

func TestInterChartDeps_DetectCrossChart_ServiceToDeployment(t *testing.T) {
	// Input: Frontend group (Ingress) -> Backend group (Service) via relationship
	// Expected: Frontend has dependency on backend
	frontIngress := makeProcessedResource("Ingress", "frontend-ingress", "default",
		map[string]string{"app.kubernetes.io/name": "frontend"})
	backSvc := makeProcessedResource("Service", "backend-svc", "default",
		map[string]string{"app.kubernetes.io/name": "backend"})

	relationships := []types.Relationship{
		{From: resourceKey(frontIngress), To: resourceKey(backSvc), Type: types.RelationNameReference},
	}

	graph := buildGraph([]*types.ProcessedResource{frontIngress, backSvc}, relationships)

	groupResult, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	deps, err := DetectCrossChartDeps(groupResult.Groups, graph, "0.1.0")
	if err != nil {
		t.Fatalf("DetectCrossChartDeps returned error: %v", err)
	}

	// Frontend should depend on backend
	frontDeps := deps["frontend"]
	if len(frontDeps) == 0 {
		t.Fatal("expected frontend to have dependencies on backend")
	}

	found := false
	for _, dep := range frontDeps {
		if dep.Name == "backend" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("frontend dependencies don't include 'backend': %+v", frontDeps)
	}
}

func TestInterChartDeps_DetectCrossChart_MultipleRelationships(t *testing.T) {
	// Input: Frontend -> Backend, Backend -> Database
	// Expected: Frontend depends on backend; backend depends on database
	frontDeploy := makeProcessedResource("Deployment", "frontend", "default",
		map[string]string{"app.kubernetes.io/name": "frontend"})
	backDeploy := makeProcessedResource("Deployment", "backend", "default",
		map[string]string{"app.kubernetes.io/name": "backend"})
	backSvc := makeProcessedResource("Service", "backend-svc", "default",
		map[string]string{"app.kubernetes.io/name": "backend"})
	dbDeploy := makeProcessedResource("Deployment", "database", "default",
		map[string]string{"app.kubernetes.io/name": "database"})
	dbSvc := makeProcessedResource("Service", "database-svc", "default",
		map[string]string{"app.kubernetes.io/name": "database"})

	relationships := []types.Relationship{
		{From: resourceKey(frontDeploy), To: resourceKey(backSvc), Type: types.RelationNameReference},
		{From: resourceKey(backDeploy), To: resourceKey(dbSvc), Type: types.RelationNameReference},
	}

	graph := buildGraph([]*types.ProcessedResource{frontDeploy, backDeploy, backSvc, dbDeploy, dbSvc}, relationships)

	groupResult, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	deps, err := DetectCrossChartDeps(groupResult.Groups, graph, "0.1.0")
	if err != nil {
		t.Fatalf("DetectCrossChartDeps returned error: %v", err)
	}

	// Frontend -> backend
	frontDeps := deps["frontend"]
	foundBackend := false
	for _, dep := range frontDeps {
		if dep.Name == "backend" {
			foundBackend = true
		}
	}
	if !foundBackend {
		t.Errorf("frontend should depend on backend, got: %+v", frontDeps)
	}

	// Backend -> database
	backDeps := deps["backend"]
	foundDB := false
	for _, dep := range backDeps {
		if dep.Name == "database" {
			foundDB = true
		}
	}
	if !foundDB {
		t.Errorf("backend should depend on database, got: %+v", backDeps)
	}
}

// ============================================================
// Subtask 2: file:// repository references
// ============================================================

func TestInterChartDeps_FileRepository_LocalPath(t *testing.T) {
	// Expected: repository: file://../backend in dependency spec
	frontDeploy := makeProcessedResource("Deployment", "frontend", "default",
		map[string]string{"app.kubernetes.io/name": "frontend"})
	backSvc := makeProcessedResource("Service", "backend-svc", "default",
		map[string]string{"app.kubernetes.io/name": "backend"})

	relationships := []types.Relationship{
		{From: resourceKey(frontDeploy), To: resourceKey(backSvc), Type: types.RelationNameReference},
	}

	graph := buildGraph([]*types.ProcessedResource{frontDeploy, backSvc}, relationships)

	groupResult, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	deps, err := DetectCrossChartDeps(groupResult.Groups, graph, "0.1.0")
	if err != nil {
		t.Fatalf("DetectCrossChartDeps returned error: %v", err)
	}

	frontDeps := deps["frontend"]
	if len(frontDeps) == 0 {
		t.Fatal("expected frontend to have dependencies")
	}

	for _, dep := range frontDeps {
		if dep.Name == "backend" {
			if !strings.HasPrefix(dep.Repository, "file://") {
				t.Errorf("expected file:// repository, got '%s'", dep.Repository)
			}
			if !strings.Contains(dep.Repository, "backend") {
				t.Errorf("expected repository to reference 'backend', got '%s'", dep.Repository)
			}
			return
		}
	}
	t.Error("backend dependency not found in frontend deps")
}

func TestInterChartDeps_FileRepository_RelativeToChart(t *testing.T) {
	// Expected: Relative path from frontend to backend
	frontDeploy := makeProcessedResource("Deployment", "frontend", "default",
		map[string]string{"app.kubernetes.io/name": "frontend"})
	backSvc := makeProcessedResource("Service", "backend-svc", "default",
		map[string]string{"app.kubernetes.io/name": "backend"})

	relationships := []types.Relationship{
		{From: resourceKey(frontDeploy), To: resourceKey(backSvc), Type: types.RelationNameReference},
	}

	graph := buildGraph([]*types.ProcessedResource{frontDeploy, backSvc}, relationships)

	groupResult, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	deps, err := DetectCrossChartDeps(groupResult.Groups, graph, "0.1.0")
	if err != nil {
		t.Fatalf("DetectCrossChartDeps returned error: %v", err)
	}

	frontDeps := deps["frontend"]
	for _, dep := range frontDeps {
		if dep.Name == "backend" {
			expected := "file://../backend"
			if dep.Repository != expected {
				t.Errorf("expected repository '%s', got '%s'", expected, dep.Repository)
			}
			return
		}
	}
	t.Error("backend dependency not found")
}

// ============================================================
// Subtask 3: Condition field in dependencies
// ============================================================

func TestInterChartDeps_Condition_DefaultPattern(t *testing.T) {
	// Expected: condition: backend.enabled in dependency
	frontDeploy := makeProcessedResource("Deployment", "frontend", "default",
		map[string]string{"app.kubernetes.io/name": "frontend"})
	backSvc := makeProcessedResource("Service", "backend-svc", "default",
		map[string]string{"app.kubernetes.io/name": "backend"})

	relationships := []types.Relationship{
		{From: resourceKey(frontDeploy), To: resourceKey(backSvc), Type: types.RelationNameReference},
	}

	graph := buildGraph([]*types.ProcessedResource{frontDeploy, backSvc}, relationships)

	groupResult, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	deps, err := DetectCrossChartDeps(groupResult.Groups, graph, "0.1.0")
	if err != nil {
		t.Fatalf("DetectCrossChartDeps returned error: %v", err)
	}

	frontDeps := deps["frontend"]
	for _, dep := range frontDeps {
		if dep.Name == "backend" {
			expected := "backend.enabled"
			if dep.Condition != expected {
				t.Errorf("expected condition '%s', got '%s'", expected, dep.Condition)
			}
			return
		}
	}
	t.Error("backend dependency not found")
}

// ============================================================
// Subtask 4: Circular dependency detection
// ============================================================

func TestInterChartDeps_CircularDependency_Detection(t *testing.T) {
	// Input: A -> B -> C -> A
	// Expected: Error returned containing "circular dependency"
	resA := makeProcessedResource("Deployment", "app-a", "default",
		map[string]string{"app.kubernetes.io/name": "app-a"})
	resB := makeProcessedResource("Deployment", "app-b", "default",
		map[string]string{"app.kubernetes.io/name": "app-b"})
	resC := makeProcessedResource("Deployment", "app-c", "default",
		map[string]string{"app.kubernetes.io/name": "app-c"})

	// Cross-group relationships forming a cycle
	relationships := []types.Relationship{
		{From: resourceKey(resA), To: resourceKey(resB), Type: types.RelationNameReference},
		{From: resourceKey(resB), To: resourceKey(resC), Type: types.RelationNameReference},
		{From: resourceKey(resC), To: resourceKey(resA), Type: types.RelationNameReference},
	}

	graph := buildGraph([]*types.ProcessedResource{resA, resB, resC}, relationships)

	groupResult, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	_, err = DetectCrossChartDeps(groupResult.Groups, graph, "0.1.0")
	if err == nil {
		t.Fatal("expected error for circular dependency, got nil")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected error to contain 'circular', got: %s", err.Error())
	}
}

func TestInterChartDeps_CircularDependency_SelfReference(t *testing.T) {
	// Input: A -> A
	// Expected: Error (but only if it's cross-chart; self-references within a chart are OK)
	// Since this is within the same group, it should NOT cause an error
	resA := makeProcessedResource("Deployment", "app-a", "default",
		map[string]string{"app.kubernetes.io/name": "app-a"})
	resSvcA := makeProcessedResource("Service", "svc-a", "default",
		map[string]string{"app.kubernetes.io/name": "app-a"})

	relationships := []types.Relationship{
		{From: resourceKey(resA), To: resourceKey(resSvcA), Type: types.RelationLabelSelector},
	}

	graph := buildGraph([]*types.ProcessedResource{resA, resSvcA}, relationships)

	groupResult, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	deps, err := DetectCrossChartDeps(groupResult.Groups, graph, "0.1.0")
	if err != nil {
		t.Fatalf("unexpected error for intra-chart reference: %v", err)
	}

	// No cross-chart dependencies (both resources in same group)
	totalDeps := 0
	for _, d := range deps {
		totalDeps += len(d)
	}
	if totalDeps != 0 {
		t.Errorf("expected 0 cross-chart deps for intra-chart reference, got %d", totalDeps)
	}
}

// ============================================================
// Subtask 5: No cross-chart dependencies (independent charts)
// ============================================================

func TestInterChartDeps_NoCrossDeps_IndependentCharts(t *testing.T) {
	// Input: 2 groups with no cross-group relationships
	// Expected: No dependencies in either Chart.yaml
	frontDeploy := makeProcessedResource("Deployment", "frontend", "default",
		map[string]string{"app.kubernetes.io/name": "frontend"})
	backDeploy := makeProcessedResource("Deployment", "backend", "default",
		map[string]string{"app.kubernetes.io/name": "backend"})

	// No relationships
	graph := buildGraph([]*types.ProcessedResource{frontDeploy, backDeploy}, nil)

	groupResult, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	deps, err := DetectCrossChartDeps(groupResult.Groups, graph, "0.1.0")
	if err != nil {
		t.Fatalf("DetectCrossChartDeps returned error: %v", err)
	}

	totalDeps := 0
	for _, d := range deps {
		totalDeps += len(d)
	}
	if totalDeps != 0 {
		t.Errorf("expected 0 dependencies for independent charts, got %d", totalDeps)
	}
}

// ============================================================
// Subtask 6: Edge cases
// ============================================================

func TestInterChartDeps_Edge_SingleChart(t *testing.T) {
	// Input: 1 group
	// Expected: No dependencies section
	deploy := makeProcessedResource("Deployment", "myapp", "default",
		map[string]string{"app.kubernetes.io/name": "myapp"})

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	groupResult, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	deps, err := DetectCrossChartDeps(groupResult.Groups, graph, "0.1.0")
	if err != nil {
		t.Fatalf("DetectCrossChartDeps returned error: %v", err)
	}

	totalDeps := 0
	for _, d := range deps {
		totalDeps += len(d)
	}
	if totalDeps != 0 {
		t.Errorf("expected 0 dependencies for single chart, got %d", totalDeps)
	}
}

func TestInterChartDeps_Edge_EmptyGroups(t *testing.T) {
	// Input: 0 groups
	// Expected: No error
	deps, err := DetectCrossChartDeps([]*ServiceGroup{}, types.NewResourceGraph(), "0.1.0")
	if err != nil {
		t.Fatalf("unexpected error for empty groups: %v", err)
	}

	if len(deps) != 0 {
		t.Errorf("expected empty deps map, got %d entries", len(deps))
	}
}
