package pattern

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func makeGraph() *types.ResourceGraph {
	return types.NewResourceGraph()
}

func addResource(g *types.ResourceGraph, group, version, kind, name, namespace, serviceName string) *types.ProcessedResource {
	obj := &unstructured.Unstructured{}
	obj.SetKind(kind)
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetAPIVersion(group + "/" + version)
	if group == "" {
		obj.SetAPIVersion(version)
	}
	pr := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    schema.GroupVersionKind{Group: group, Version: version, Kind: kind},
		},
		ServiceName: serviceName,
		Values:      make(map[string]interface{}),
	}
	g.AddResource(pr)
	return pr
}

func addGroup(g *types.ResourceGraph, name string, resources ...*types.ProcessedResource) {
	grp := &types.ResourceGroup{
		Name:      name,
		Resources: resources,
		Namespace: "default",
	}
	g.AddGroup(grp)
}

// addWorkloadWithContainers creates a Deployment with explicit container specs
// in Values so that checker container-loop code paths are exercised.
func addWorkloadWithContainers(g *types.ResourceGraph, kind, name, serviceName string, containers []map[string]interface{}) *types.ProcessedResource {
	obj := &unstructured.Unstructured{}
	obj.SetKind(kind)
	obj.SetName(name)
	obj.SetNamespace("default")
	obj.SetAPIVersion("apps/v1")
	pr := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kind},
		},
		ServiceName: serviceName,
		Values: map[string]interface{}{
			"containers": containers,
		},
	}
	g.AddResource(pr)
	return pr
}

// ── NewAnalyzer / AddDetector / AddChecker ───────────────────────────────────

func TestNewAnalyzer(t *testing.T) {
	a := NewAnalyzer()
	if a == nil {
		t.Fatal("NewAnalyzer returned nil")
	}
	if len(a.detectors) != 0 || len(a.checkers) != 0 {
		t.Error("new analyzer should have empty detectors and checkers")
	}
}

func TestAddDetector(t *testing.T) {
	a := NewAnalyzer()
	a.AddDetector(NewMicroservicesDetector())
	if len(a.detectors) != 1 {
		t.Errorf("detectors = %d; want 1", len(a.detectors))
	}
}

func TestAddChecker(t *testing.T) {
	a := NewAnalyzer()
	a.AddChecker(NewResourceLimitsChecker())
	if len(a.checkers) != 1 {
		t.Errorf("checkers = %d; want 1", len(a.checkers))
	}
}

func TestDefaultAnalyzer_HasDetectorsAndCheckers(t *testing.T) {
	a := DefaultAnalyzer()
	if len(a.detectors) != 5 {
		t.Errorf("DefaultAnalyzer detectors = %d; want 5", len(a.detectors))
	}
	if len(a.checkers) != 9 {
		t.Errorf("DefaultAnalyzer checkers = %d; want 9", len(a.checkers))
	}
}

// ── Analyze ──────────────────────────────────────────────────────────────────

func TestAnalyze_EmptyGraph(t *testing.T) {
	a := DefaultAnalyzer()
	g := makeGraph()
	result := a.Analyze(g)

	if result == nil {
		t.Fatal("Analyze returned nil")
	}
	if result.Metrics.TotalResources != 0 {
		t.Error("empty graph should have 0 resources")
	}
}

func TestAnalyze_MonolithPattern(t *testing.T) {
	a := DefaultAnalyzer()
	g := makeGraph()
	r1 := addResource(g, "apps", "v1", "Deployment", "app", "default", "app")
	r2 := addResource(g, "", "v1", "Service", "svc", "default", "app")
	addGroup(g, "app", r1, r2)

	result := a.Analyze(g)
	if result.PrimaryPattern != PatternMonolith {
		t.Errorf("PrimaryPattern = %q; want monolith", result.PrimaryPattern)
	}
	if result.RecommendedStrategy != StrategyUniversal {
		t.Errorf("RecommendedStrategy = %q; want universal", result.RecommendedStrategy)
	}
}

func TestAnalyze_StatefulPattern(t *testing.T) {
	a := DefaultAnalyzer()
	g := makeGraph()
	r1 := addResource(g, "apps", "v1", "StatefulSet", "db", "default", "db")
	r2 := addResource(g, "", "v1", "PersistentVolumeClaim", "data", "default", "db")
	addGroup(g, "db", r1, r2)

	result := a.Analyze(g)
	found := false
	for _, p := range result.DetectedPatterns {
		if p == PatternStateful {
			found = true
		}
	}
	if !found {
		t.Errorf("expected PatternStateful in %v", result.DetectedPatterns)
	}
}

func TestAnalyze_DeckhousePattern(t *testing.T) {
	a := DefaultAnalyzer()
	g := makeGraph()
	r1 := addResource(g, "deckhouse.io", "v1", "ModuleConfig", "mc", "default", "dh")
	addGroup(g, "dh", r1)

	result := a.Analyze(g)
	found := false
	for _, p := range result.DetectedPatterns {
		if p == PatternDeckhouse {
			found = true
		}
	}
	if !found {
		t.Errorf("expected PatternDeckhouse in %v", result.DetectedPatterns)
	}
}

func TestAnalyze_Confidence(t *testing.T) {
	a := DefaultAnalyzer()
	g := makeGraph()
	r := addResource(g, "apps", "v1", "Deployment", "app", "default", "app")
	addGroup(g, "app", r)

	result := a.Analyze(g)
	if result.Confidence < 0 || result.Confidence > 100 {
		t.Errorf("Confidence = %d; should be 0-100", result.Confidence)
	}
}

// TestAnalyze_DuplicatePatternDeduplication verifies that when multiple detectors
// return the same pattern, the pattern appears only once in DetectedPatterns.
func TestAnalyze_DuplicatePatternDeduplication(t *testing.T) {
	a := NewAnalyzer()
	// Add the same detector twice to guarantee the same pattern is returned twice.
	a.AddDetector(NewStatefulDetector())
	a.AddDetector(NewStatefulDetector())

	g := makeGraph()
	r := addResource(g, "apps", "v1", "StatefulSet", "db", "default", "db")
	addGroup(g, "db", r)

	result := a.Analyze(g)

	count := 0
	for _, p := range result.DetectedPatterns {
		if p == PatternStateful {
			count++
		}
	}
	if count != 1 {
		t.Errorf("PatternStateful appears %d times; want exactly 1 (deduplication)", count)
	}
}

// TestAnalyze_MultipleDetectorsAggregateCounts verifies patternCounts accumulation
// across multiple detectors, so determinePrimaryPattern picks the highest-count pattern.
func TestAnalyze_MultipleDetectorsAggregateCounts(t *testing.T) {
	a := NewAnalyzer()
	// Three detectors all return Deckhouse; one returns Stateful.
	// Primary pattern must be Deckhouse.
	a.AddDetector(NewDeckhouseDetector())
	a.AddDetector(NewDeckhouseDetector())
	a.AddDetector(NewDeckhouseDetector())
	a.AddDetector(NewStatefulDetector())

	g := makeGraph()
	r1 := addResource(g, "deckhouse.io", "v1", "ModuleConfig", "mc", "default", "dh")
	r2 := addResource(g, "apps", "v1", "StatefulSet", "db", "default", "dh")
	addGroup(g, "dh", r1, r2)

	result := a.Analyze(g)
	if result.PrimaryPattern != PatternDeckhouse {
		t.Errorf("PrimaryPattern = %q; want deckhouse (highest detector count)", result.PrimaryPattern)
	}
}

// ── MicroservicesDetector ────────────────────────────────────────────────────

func TestMicroservicesDetector_Name(t *testing.T) {
	d := NewMicroservicesDetector()
	if d.Name() != "microservices" {
		t.Errorf("Name() = %q", d.Name())
	}
}

func TestMicroservicesDetector_LessThan3Groups(t *testing.T) {
	d := NewMicroservicesDetector()
	g := makeGraph()
	addResource(g, "apps", "v1", "Deployment", "a", "default", "a")
	addGroup(g, "a", &types.ProcessedResource{})

	patterns := d.Detect(g)
	for _, p := range patterns {
		if p == PatternMicroservices {
			t.Error("should not detect microservices with <3 groups")
		}
	}
}

func TestMicroservicesDetector_ManyServices(t *testing.T) {
	d := NewMicroservicesDetector()
	g := makeGraph()

	for _, name := range []string{"svc1", "svc2", "svc3", "svc4"} {
		r := addResource(g, "apps", "v1", "Deployment", name, "default", name)
		addGroup(g, name, r)
	}

	patterns := d.Detect(g)
	found := false
	for _, p := range patterns {
		if p == PatternMicroservices {
			found = true
		}
	}
	if !found {
		t.Error("should detect microservices with 4+ services with deployments")
	}
}

// TestMicroservicesDetector_WithStatefulSet verifies that when StatefulSets exist
// across many services, the hasPVC/hasStatefulSet branch suppresses PatternStateless.
func TestMicroservicesDetector_WithStatefulSet(t *testing.T) {
	d := NewMicroservicesDetector()
	g := makeGraph()

	// 3 services, each with a StatefulSet — hasPVC/hasStatefulSet will be true
	for _, name := range []string{"svc1", "svc2", "svc3"} {
		r := addResource(g, "apps", "v1", "StatefulSet", name, "default", name)
		addGroup(g, name, r)
	}

	patterns := d.Detect(g)
	for _, p := range patterns {
		if p == PatternStateless {
			t.Error("should not detect stateless when StatefulSets are present")
		}
	}
}

// TestMicroservicesDetector_WithPVC verifies that PVCs suppress PatternStateless.
func TestMicroservicesDetector_WithPVC(t *testing.T) {
	d := NewMicroservicesDetector()
	g := makeGraph()

	for _, name := range []string{"svc1", "svc2", "svc3"} {
		r := addResource(g, "apps", "v1", "Deployment", name, "default", name)
		addGroup(g, name, r)
	}
	// Add a PVC that sets hasPVC = true
	addResource(g, "", "v1", "PersistentVolumeClaim", "data", "default", "svc1")

	patterns := d.Detect(g)
	for _, p := range patterns {
		if p == PatternStateless {
			t.Error("should not detect stateless when PVCs are present")
		}
	}
}

// TestMicroservicesDetector_WithIngress exercises the hasIngress=true and
// servicesWithIngress++ branches in MicroservicesDetector.Detect (detectors.go:44-54).
func TestMicroservicesDetector_WithIngress(t *testing.T) {
	d := NewMicroservicesDetector()
	g := makeGraph()

	// Build 3 service groups, each with a Deployment and an Ingress resource.
	for _, name := range []string{"svc1", "svc2", "svc3"} {
		dep := addResource(g, "apps", "v1", "Deployment", name, "default", name)
		ing := addResource(g, "networking.k8s.io", "v1", "Ingress", name+"-ing", "default", name)
		addGroup(g, name, dep, ing)
	}

	patterns := d.Detect(g)
	found := false
	for _, p := range patterns {
		if p == PatternMicroservices {
			found = true
		}
	}
	if !found {
		t.Error("should detect microservices with 3+ services that have Ingress resources")
	}
}

// ── StatefulDetector ─────────────────────────────────────────────────────────

func TestStatefulDetector_Name(t *testing.T) {
	d := NewStatefulDetector()
	if d.Name() != "stateful" {
		t.Errorf("Name() = %q", d.Name())
	}
}

func TestStatefulDetector_WithPVC(t *testing.T) {
	d := NewStatefulDetector()
	g := makeGraph()
	addResource(g, "", "v1", "PersistentVolumeClaim", "data", "default", "db")

	patterns := d.Detect(g)
	found := false
	for _, p := range patterns {
		if p == PatternStateful {
			found = true
		}
	}
	if !found {
		t.Error("should detect stateful with PVC")
	}
}

func TestStatefulDetector_WithDaemonSet(t *testing.T) {
	d := NewStatefulDetector()
	g := makeGraph()
	addResource(g, "apps", "v1", "DaemonSet", "agent", "default", "agent")

	patterns := d.Detect(g)
	found := false
	for _, p := range patterns {
		if p == PatternDaemonSet {
			found = true
		}
	}
	if !found {
		t.Error("should detect daemonset pattern")
	}
}

// ── DeckhouseDetector ────────────────────────────────────────────────────────

func TestDeckhouseDetector_Name(t *testing.T) {
	d := NewDeckhouseDetector()
	if d.Name() != "deckhouse" {
		t.Errorf("Name() = %q", d.Name())
	}
}

func TestDeckhouseDetector_WithDeckhouseResource(t *testing.T) {
	d := NewDeckhouseDetector()
	g := makeGraph()
	addResource(g, "deckhouse.io", "v1", "ModuleConfig", "mc", "default", "dh")

	patterns := d.Detect(g)
	found := false
	for _, p := range patterns {
		if p == PatternDeckhouse {
			found = true
		}
	}
	if !found {
		t.Error("should detect deckhouse pattern")
	}
}

func TestDeckhouseDetector_NoDeckhouseResources(t *testing.T) {
	d := NewDeckhouseDetector()
	g := makeGraph()
	addResource(g, "apps", "v1", "Deployment", "app", "default", "app")

	patterns := d.Detect(g)
	for _, p := range patterns {
		if p == PatternDeckhouse {
			t.Error("should not detect deckhouse without deckhouse.io resources")
		}
	}
}

// TestDeckhouseDetector_SidecarPattern verifies that a Deployment with multiple
// containers triggers PatternSidecar detection.
func TestDeckhouseDetector_SidecarPattern(t *testing.T) {
	d := NewDeckhouseDetector()
	g := makeGraph()

	// Add a Deployment with two containers in Values
	pr := addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{"name": "main"},
		{"name": "sidecar"},
	})
	addGroup(g, "app", pr)

	patterns := d.Detect(g)
	found := false
	for _, p := range patterns {
		if p == PatternSidecar {
			found = true
		}
	}
	if !found {
		t.Error("should detect sidecar pattern when Deployment has >1 containers")
	}
}

// TestDeckhouseDetector_SingleContainerNoSidecar verifies that a Deployment
// with only one container does NOT trigger PatternSidecar.
func TestDeckhouseDetector_SingleContainerNoSidecar(t *testing.T) {
	d := NewDeckhouseDetector()
	g := makeGraph()

	pr := addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{"name": "main"},
	})
	addGroup(g, "app", pr)

	patterns := d.Detect(g)
	for _, p := range patterns {
		if p == PatternSidecar {
			t.Error("should not detect sidecar with only 1 container")
		}
	}
}

// ── ResourceLimitsChecker ────────────────────────────────────────────────────

func TestResourceLimitsChecker_Name(t *testing.T) {
	c := NewResourceLimitsChecker()
	if c.Name() != "resource-limits" {
		t.Errorf("Name() = %q", c.Name())
	}
	if c.Category() != "Resource Management" {
		t.Errorf("Category() = %q", c.Category())
	}
}

func TestResourceLimitsChecker_NoWorkloads(t *testing.T) {
	c := NewResourceLimitsChecker()
	g := makeGraph()
	addResource(g, "", "v1", "ConfigMap", "cfg", "default", "app")

	practices := c.Check(g)
	// Should return compliant practice since there are no workloads
	for _, p := range practices {
		if !p.Compliant && p.ID == "BP-001" {
			t.Error("should be compliant when no workloads exist")
		}
	}
}

// TestResourceLimitsChecker_MissingLimitsAndRequests exercises the container-loop
// code paths: workload with containers that lack limits and requests.
func TestResourceLimitsChecker_MissingLimitsAndRequests(t *testing.T) {
	c := NewResourceLimitsChecker()
	g := makeGraph()

	// Container with no resources key at all
	pr := addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{"name": "main"},
	})
	addGroup(g, "app", pr)

	practices := c.Check(g)
	hasBP001 := false
	hasBP002 := false
	for _, p := range practices {
		if p.ID == "BP-001" && !p.Compliant {
			hasBP001 = true
		}
		if p.ID == "BP-002" && !p.Compliant {
			hasBP002 = true
		}
	}
	if !hasBP001 {
		t.Error("should report BP-001 (missing limits) for workload without container resources")
	}
	if !hasBP002 {
		t.Error("should report BP-002 (missing requests) for workload without container resources")
	}
}

// TestResourceLimitsChecker_WithLimitsAndRequests exercises the compliant branch:
// container has both limits and requests set.
func TestResourceLimitsChecker_WithLimitsAndRequests(t *testing.T) {
	c := NewResourceLimitsChecker()
	g := makeGraph()

	pr := addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{
			"name": "main",
			"resources": map[string]interface{}{
				"limits":   map[string]interface{}{"cpu": "500m", "memory": "128Mi"},
				"requests": map[string]interface{}{"cpu": "100m", "memory": "64Mi"},
			},
		},
	})
	addGroup(g, "app", pr)

	practices := c.Check(g)
	for _, p := range practices {
		if (p.ID == "BP-001" || p.ID == "BP-002") && !p.Compliant {
			t.Errorf("should be compliant when container has limits and requests; got %s non-compliant", p.ID)
		}
	}
	// Should have the compliant BP-001 practice
	found := false
	for _, p := range practices {
		if p.Compliant && p.ID == "BP-001" {
			found = true
		}
	}
	if !found {
		t.Error("should have compliant BP-001 when all containers have limits and requests")
	}
}

// TestResourceLimitsChecker_ContainersWrongType exercises the type-assertion failure
// branch in ResourceLimitsChecker.Check (checkers.go:46-47): when the "containers"
// value is present but is not []map[string]interface{}, the checker skips the resource.
func TestResourceLimitsChecker_ContainersWrongType(t *testing.T) {
	c := NewResourceLimitsChecker()
	g := makeGraph()

	obj := &unstructured.Unstructured{}
	obj.SetKind("Deployment")
	obj.SetName("app")
	obj.SetNamespace("default")
	obj.SetAPIVersion("apps/v1")
	pr := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		},
		ServiceName: "app",
		// Set containers to a wrong type — string instead of []map[string]interface{}
		Values: map[string]interface{}{
			"containers": "not-a-list",
		},
	}
	g.AddResource(pr)
	addGroup(g, "app", pr)

	// When containers is wrong type, the checker skips the resource entirely.
	// No workloads with valid containers means both missing sets are empty
	// and the compliant BP-001 is returned.
	practices := c.Check(g)
	for _, p := range practices {
		if !p.Compliant && (p.ID == "BP-001" || p.ID == "BP-002") {
			t.Error("wrong-type containers should be skipped; no violation should be reported")
		}
	}
}

// ── SecurityChecker ──────────────────────────────────────────────────────────

func TestSecurityChecker_Name(t *testing.T) {
	c := NewSecurityChecker()
	if c.Name() != "security" {
		t.Errorf("Name() = %q", c.Name())
	}
	if c.Category() != "Security" {
		t.Errorf("Category() = %q", c.Category())
	}
}

// TestSecurityChecker_WorkloadNoContainers covers the nil-containers branch in SecurityChecker:
// workload resource with no "containers" key → both runAsNonRoot and readOnlyRootFS flagged.
func TestSecurityChecker_WorkloadNoContainers(t *testing.T) {
	c := NewSecurityChecker()
	g := makeGraph()
	addResource(g, "apps", "v1", "Deployment", "app", "default", "app")

	practices := c.Check(g)
	hasSEC001 := false
	hasSEC002 := false
	for _, p := range practices {
		if p.ID == "BP-SEC-001" {
			hasSEC001 = true
		}
		if p.ID == "BP-SEC-002" {
			hasSEC002 = true
		}
	}
	if !hasSEC001 {
		t.Error("should report BP-SEC-001 for workload without containers")
	}
	if !hasSEC002 {
		t.Error("should report BP-SEC-002 for workload without containers")
	}
}

// TestSecurityChecker_ContainerWithGoodSecurityContext verifies that containers
// with runAsNonRoot:true and readOnlyRootFilesystem:true do NOT generate SEC-001/002.
func TestSecurityChecker_ContainerWithGoodSecurityContext(t *testing.T) {
	c := NewSecurityChecker()
	g := makeGraph()

	pr := addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{
			"name": "main",
			"securityContext": map[string]interface{}{
				"runAsNonRoot":           true,
				"readOnlyRootFilesystem": true,
			},
		},
	})
	addGroup(g, "app", pr)

	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-SEC-001" {
			t.Error("should not report SEC-001 when runAsNonRoot is true")
		}
		if p.ID == "BP-SEC-002" {
			t.Error("should not report SEC-002 when readOnlyRootFilesystem is true")
		}
	}
}

// TestSecurityChecker_PrivilegedContainer verifies that containers with
// securityContext.privileged:true generate BP-SEC-003.
func TestSecurityChecker_PrivilegedContainer(t *testing.T) {
	c := NewSecurityChecker()
	g := makeGraph()

	pr := addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{
			"name": "main",
			"securityContext": map[string]interface{}{
				"runAsNonRoot":           true,
				"readOnlyRootFilesystem": true,
				"privileged":             true,
			},
		},
	})
	addGroup(g, "app", pr)

	practices := c.Check(g)
	found := false
	for _, p := range practices {
		if p.ID == "BP-SEC-003" {
			found = true
		}
	}
	if !found {
		t.Error("should report BP-SEC-003 for privileged container")
	}
}

// TestSecurityChecker_ContainerNoSecurityContext exercises the no-securityContext branch:
// container list present but container has no securityContext.
func TestSecurityChecker_ContainerNoSecurityContext(t *testing.T) {
	c := NewSecurityChecker()
	g := makeGraph()

	pr := addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{"name": "main"},
	})
	addGroup(g, "app", pr)

	practices := c.Check(g)
	hasSEC001 := false
	hasSEC002 := false
	for _, p := range practices {
		if p.ID == "BP-SEC-001" {
			hasSEC001 = true
		}
		if p.ID == "BP-SEC-002" {
			hasSEC002 = true
		}
	}
	if !hasSEC001 {
		t.Error("should report SEC-001 when container has no securityContext")
	}
	if !hasSEC002 {
		t.Error("should report SEC-002 when container has no securityContext")
	}
}

// TestSecurityChecker_ContainersWrongType exercises the type-assertion failure branch
// in SecurityChecker.Check (checkers.go:175-176): when the "containers" value is present
// but is not []map[string]interface{}, the checker skips the workload.
func TestSecurityChecker_ContainersWrongType(t *testing.T) {
	c := NewSecurityChecker()
	g := makeGraph()

	obj := &unstructured.Unstructured{}
	obj.SetKind("Deployment")
	obj.SetName("app")
	obj.SetNamespace("default")
	obj.SetAPIVersion("apps/v1")
	pr := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		},
		ServiceName: "app",
		// containers present but wrong type — skips the type-assertion guard
		Values: map[string]interface{}{
			"containers": 42,
		},
	}
	g.AddResource(pr)
	addGroup(g, "app", pr)

	// When containers has wrong type the resource is skipped entirely —
	// no violations should be reported.
	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-SEC-001" || p.ID == "BP-SEC-002" || p.ID == "BP-SEC-003" {
			t.Errorf("wrong-type containers should be skipped; unexpected %s reported", p.ID)
		}
	}
}

// TestSecurityChecker_RunAsNonRootFalse exercises the branch where securityContext
// is present but runAsNonRoot is explicitly false (checkers.go:188-190).
// Similarly tests readOnlyRootFilesystem:false (checkers.go:193-195).
func TestSecurityChecker_RunAsNonRootFalse(t *testing.T) {
	c := NewSecurityChecker()
	g := makeGraph()

	pr := addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{
			"name": "main",
			"securityContext": map[string]interface{}{
				"runAsNonRoot":           false,
				"readOnlyRootFilesystem": false,
			},
		},
	})
	addGroup(g, "app", pr)

	practices := c.Check(g)
	hasSEC001 := false
	hasSEC002 := false
	for _, p := range practices {
		if p.ID == "BP-SEC-001" {
			hasSEC001 = true
		}
		if p.ID == "BP-SEC-002" {
			hasSEC002 = true
		}
	}
	if !hasSEC001 {
		t.Error("should report BP-SEC-001 when runAsNonRoot is false")
	}
	if !hasSEC002 {
		t.Error("should report BP-SEC-002 when readOnlyRootFilesystem is false")
	}
}

// ── HighAvailabilityChecker ──────────────────────────────────────────────────

func TestHighAvailabilityChecker_Name(t *testing.T) {
	c := NewHighAvailabilityChecker()
	if c.Name() != "high-availability" {
		t.Errorf("Name() = %q", c.Name())
	}
	if c.Category() != "High Availability" {
		t.Errorf("Category() = %q", c.Category())
	}
}

// TestHighAvailabilityChecker_SingleReplica exercises the replicas==1 branch.
func TestHighAvailabilityChecker_SingleReplica(t *testing.T) {
	c := NewHighAvailabilityChecker()
	g := makeGraph()

	pr := addWorkloadWithContainers(g, "Deployment", "app", "app", nil)
	// Set replicas explicitly in Values
	pr.Values["replicas"] = int64(1)
	addGroup(g, "app", pr)

	practices := c.Check(g)
	found := false
	for _, p := range practices {
		if p.ID == "BP-HA-001" && !p.Compliant {
			found = true
		}
	}
	if !found {
		t.Error("should report BP-HA-001 for single-replica deployment")
	}
}

// TestHighAvailabilityChecker_MissingProbes exercises the missing-probes branch.
func TestHighAvailabilityChecker_MissingProbes(t *testing.T) {
	c := NewHighAvailabilityChecker()
	g := makeGraph()

	// Deployment with containers but no probes
	pr := addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{"name": "main"},
	})
	addGroup(g, "app", pr)

	practices := c.Check(g)
	found := false
	for _, p := range practices {
		if p.ID == "BP-HA-002" && !p.Compliant {
			found = true
		}
	}
	if !found {
		t.Error("should report BP-HA-002 for deployment without health probes")
	}
}

// TestHighAvailabilityChecker_WithProbes verifies containers that have probes
// do NOT generate BP-HA-002.
func TestHighAvailabilityChecker_WithProbes(t *testing.T) {
	c := NewHighAvailabilityChecker()
	g := makeGraph()

	pr := addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{
			"name":           "main",
			"livenessProbe":  map[string]interface{}{"httpGet": map[string]interface{}{"path": "/health"}},
			"readinessProbe": map[string]interface{}{"httpGet": map[string]interface{}{"path": "/ready"}},
		},
	})
	addGroup(g, "app", pr)

	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-HA-002" {
			t.Error("should not report BP-HA-002 when container has probes")
		}
	}
}

// TestHighAvailabilityChecker_MissingPDB exercises the missing-PDB branch
// (multiple deployments, no PodDisruptionBudget).
func TestHighAvailabilityChecker_MissingPDB(t *testing.T) {
	c := NewHighAvailabilityChecker()
	g := makeGraph()

	// Two deployments, no PDB
	pr1 := addWorkloadWithContainers(g, "Deployment", "app1", "app1", nil)
	pr2 := addWorkloadWithContainers(g, "Deployment", "app2", "app2", nil)
	addGroup(g, "app1", pr1)
	addGroup(g, "app2", pr2)

	practices := c.Check(g)
	found := false
	for _, p := range practices {
		if p.ID == "BP-HA-003" && !p.Compliant {
			found = true
		}
	}
	if !found {
		t.Error("should report BP-HA-003 when multiple deployments exist without PDB")
	}
}

// TestHighAvailabilityChecker_WithPDB verifies that when a PodDisruptionBudget
// exists, BP-HA-003 is not reported.
func TestHighAvailabilityChecker_WithPDB(t *testing.T) {
	c := NewHighAvailabilityChecker()
	g := makeGraph()

	pr1 := addWorkloadWithContainers(g, "Deployment", "app1", "app1", nil)
	pr2 := addWorkloadWithContainers(g, "Deployment", "app2", "app2", nil)
	addResource(g, "policy", "v1", "PodDisruptionBudget", "pdb", "default", "app1")
	addGroup(g, "app1", pr1)
	addGroup(g, "app2", pr2)

	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-HA-003" {
			t.Error("should not report BP-HA-003 when PDB exists")
		}
	}
}

// ── computeMetrics ───────────────────────────────────────────────────────────

func TestComputeMetrics(t *testing.T) {
	a := NewAnalyzer()
	g := makeGraph()
	r1 := addResource(g, "apps", "v1", "Deployment", "web", "default", "web")
	r2 := addResource(g, "", "v1", "Service", "svc", "default", "web")
	addGroup(g, "web", r1, r2)

	metrics := a.computeMetrics(g)
	if metrics.TotalResources != 2 {
		t.Errorf("TotalResources = %d; want 2", metrics.TotalResources)
	}
	if metrics.TotalServices != 1 {
		t.Errorf("TotalServices = %d; want 1", metrics.TotalServices)
	}
	if metrics.ResourcesByKind["Deployment"] != 1 {
		t.Error("ResourcesByKind should count Deployment")
	}
}

func TestComputeMetrics_Deckhouse(t *testing.T) {
	a := NewAnalyzer()
	g := makeGraph()
	addResource(g, "deckhouse.io", "v1", "ModuleConfig", "mc", "default", "dh")

	metrics := a.computeMetrics(g)
	if metrics.DeckhouseResourceCount != 1 {
		t.Errorf("DeckhouseResourceCount = %d; want 1", metrics.DeckhouseResourceCount)
	}
}

func TestComputeMetrics_StatefulAndIngress(t *testing.T) {
	a := NewAnalyzer()
	g := makeGraph()
	r1 := addResource(g, "apps", "v1", "StatefulSet", "db", "default", "db")
	r2 := addResource(g, "networking.k8s.io", "v1", "Ingress", "ing", "default", "web")
	r3 := addResource(g, "", "v1", "Secret", "s", "default", "web")
	addGroup(g, "db", r1)
	addGroup(g, "web", r2, r3)

	metrics := a.computeMetrics(g)
	if metrics.StatefulServices != 1 {
		t.Errorf("StatefulServices = %d; want 1", metrics.StatefulServices)
	}
	if metrics.ServicesWithIngress != 1 {
		t.Errorf("ServicesWithIngress = %d; want 1", metrics.ServicesWithIngress)
	}
	if metrics.ServicesWithSecrets != 1 {
		t.Errorf("ServicesWithSecrets = %d; want 1", metrics.ServicesWithSecrets)
	}
}

// TestComputeMetrics_ZeroServices verifies AverageResourcesPerService is 0 when no groups.
func TestComputeMetrics_ZeroServices(t *testing.T) {
	a := NewAnalyzer()
	g := makeGraph()
	metrics := a.computeMetrics(g)
	if metrics.AverageResourcesPerService != 0 {
		t.Errorf("AverageResourcesPerService = %f; want 0 for empty graph", metrics.AverageResourcesPerService)
	}
}

// ── calculateComplexityScore ─────────────────────────────────────────────────

func TestCalculateComplexityScore_Low(t *testing.T) {
	a := NewAnalyzer()
	metrics := AnalysisMetrics{TotalResources: 5, TotalServices: 1, ResourcesByKind: map[string]int{"Deployment": 1}}
	score := a.calculateComplexityScore(metrics)
	if score > 30 {
		t.Errorf("low complexity score = %d; want <= 30", score)
	}
}

func TestCalculateComplexityScore_High(t *testing.T) {
	a := NewAnalyzer()
	metrics := AnalysisMetrics{
		TotalResources:  60,
		TotalServices:   15,
		StatefulServices: 3,
		ResourcesByKind: map[string]int{
			"Deployment": 10, "Service": 10, "ConfigMap": 10,
			"Secret": 10, "Ingress": 5, "PVC": 5, "HPA": 5, "PDB": 5,
		},
	}
	score := a.calculateComplexityScore(metrics)
	if score < 70 {
		t.Errorf("high complexity score = %d; want >= 70", score)
	}
}

// TestCalculateComplexityScore_MidResources covers the 20 < TotalResources <= 50 branch (+20)
// and the 5 < TotalServices <= 10 branch (+20).
func TestCalculateComplexityScore_MidResources(t *testing.T) {
	a := NewAnalyzer()
	metrics := AnalysisMetrics{
		TotalResources: 30,
		TotalServices:  7,
		ResourcesByKind: map[string]int{"Deployment": 1},
	}
	score := a.calculateComplexityScore(metrics)
	// +20 (resources 21-50) + +20 (services 6-10) + 0 stateful + 2 kinds = 42
	if score < 40 {
		t.Errorf("mid-resource complexity score = %d; want >= 40", score)
	}
}

// TestCalculateComplexityScore_LowMidServices covers the 2 < TotalServices <= 5 branch (+10).
func TestCalculateComplexityScore_LowMidServices(t *testing.T) {
	a := NewAnalyzer()
	metrics := AnalysisMetrics{
		TotalResources: 5,
		TotalServices:  3,
		ResourcesByKind: map[string]int{"Deployment": 1},
	}
	score := a.calculateComplexityScore(metrics)
	// +0 (resources <= 10) + +10 (services 3-5) + 0 stateful + 2 kinds = 12
	if score < 10 {
		t.Errorf("low-mid services complexity score = %d; want >= 10", score)
	}
}

// TestCalculateComplexityScore_ResourcesBetween11And20 covers +10 branch for resources.
func TestCalculateComplexityScore_ResourcesBetween11And20(t *testing.T) {
	a := NewAnalyzer()
	metrics := AnalysisMetrics{
		TotalResources: 15,
		TotalServices:  1,
		ResourcesByKind: map[string]int{"Deployment": 1},
	}
	score := a.calculateComplexityScore(metrics)
	// +10 (resources 11-20) + +0 (services <= 2) + 0 stateful + 2 kinds = 12
	if score < 10 {
		t.Errorf("mid-resource (11-20) complexity score = %d; want >= 10", score)
	}
}

// TestCalculateComplexityScore_Cap verifies score is capped at 100.
func TestCalculateComplexityScore_Cap(t *testing.T) {
	a := NewAnalyzer()
	metrics := AnalysisMetrics{
		TotalResources:  100,
		TotalServices:   20,
		StatefulServices: 10,
		ResourcesByKind: map[string]int{
			"A": 1, "B": 1, "C": 1, "D": 1, "E": 1,
			"F": 1, "G": 1, "H": 1, "I": 1, "J": 1,
		},
	}
	score := a.calculateComplexityScore(metrics)
	if score != 100 {
		t.Errorf("capped complexity score = %d; want 100", score)
	}
}

// ── calculateCouplingScore ───────────────────────────────────────────────────

func TestCalculateCouplingScore_SingleGroup(t *testing.T) {
	a := NewAnalyzer()
	g := makeGraph()
	r := addResource(g, "apps", "v1", "Deployment", "app", "default", "app")
	addGroup(g, "app", r)

	score := a.calculateCouplingScore(g)
	if score != 0 {
		t.Errorf("single group coupling = %d; want 0", score)
	}
}

// TestCalculateCouplingScore_NoRelationships covers the totalRels == 0 branch:
// two groups but no relationships → coupling = 0.
func TestCalculateCouplingScore_NoRelationships(t *testing.T) {
	a := NewAnalyzer()
	g := makeGraph()
	r1 := addResource(g, "apps", "v1", "Deployment", "svc-a", "default", "svc-a")
	r2 := addResource(g, "apps", "v1", "Deployment", "svc-b", "default", "svc-b")
	addGroup(g, "svc-a", r1)
	addGroup(g, "svc-b", r2)

	// No relationships added — totalRels == 0
	score := a.calculateCouplingScore(g)
	if score != 0 {
		t.Errorf("no relationships coupling = %d; want 0", score)
	}
}

// TestCalculateCouplingScore_AllCrossService covers 100% cross-service case.
func TestCalculateCouplingScore_AllCrossService(t *testing.T) {
	a := NewAnalyzer()
	g := makeGraph()

	r1 := addResource(g, "apps", "v1", "Deployment", "svc-a", "default", "svc-a")
	r2 := addResource(g, "apps", "v1", "Deployment", "svc-b", "default", "svc-b")
	addGroup(g, "svc-a", r1)
	addGroup(g, "svc-b", r2)

	// Add a cross-service relationship: svc-a → svc-b
	fromKey := r1.Original.ResourceKey()
	toKey := r2.Original.ResourceKey()
	g.AddRelationship(types.Relationship{
		From: fromKey,
		To:   toKey,
		Type: types.RelationNameReference,
	})

	score := a.calculateCouplingScore(g)
	if score != 100 {
		t.Errorf("all cross-service coupling = %d; want 100", score)
	}
}

// TestCalculateCouplingScore_AllSameService covers 0% cross-service:
// both endpoints of a relationship belong to the same service.
func TestCalculateCouplingScore_AllSameService(t *testing.T) {
	a := NewAnalyzer()
	g := makeGraph()

	r1 := addResource(g, "apps", "v1", "Deployment", "svc-a", "default", "svc-a")
	r2 := addResource(g, "", "v1", "Service", "svc-a-svc", "default", "svc-a")
	r3 := addResource(g, "apps", "v1", "Deployment", "svc-b", "default", "svc-b")
	addGroup(g, "svc-a", r1, r2)
	addGroup(g, "svc-b", r3)

	// Intra-service relationship (svc-a Deployment → svc-a Service)
	fromKey := r1.Original.ResourceKey()
	toKey := r2.Original.ResourceKey()
	g.AddRelationship(types.Relationship{
		From: fromKey,
		To:   toKey,
		Type: types.RelationLabelSelector,
	})

	score := a.calculateCouplingScore(g)
	if score != 0 {
		t.Errorf("all same-service coupling = %d; want 0", score)
	}
}

// TestCalculateCouplingScore_MixedRelationships covers partial cross-service coupling.
func TestCalculateCouplingScore_MixedRelationships(t *testing.T) {
	a := NewAnalyzer()
	g := makeGraph()

	r1 := addResource(g, "apps", "v1", "Deployment", "svc-a", "default", "svc-a")
	r2 := addResource(g, "", "v1", "Service", "svc-a-svc", "default", "svc-a")
	r3 := addResource(g, "apps", "v1", "Deployment", "svc-b", "default", "svc-b")
	addGroup(g, "svc-a", r1, r2)
	addGroup(g, "svc-b", r3)

	keyA := r1.Original.ResourceKey()
	keyASvc := r2.Original.ResourceKey()
	keyB := r3.Original.ResourceKey()

	// 1 intra-service (svc-a → svc-a-svc)
	g.AddRelationship(types.Relationship{From: keyA, To: keyASvc, Type: types.RelationLabelSelector})
	// 1 cross-service (svc-a → svc-b)
	g.AddRelationship(types.Relationship{From: keyA, To: keyB, Type: types.RelationNameReference})

	score := a.calculateCouplingScore(g)
	// 1 cross / 2 total = 50%
	if score != 50 {
		t.Errorf("mixed coupling = %d; want 50", score)
	}
}

// TestCalculateCouplingScore_RelFromMissingInGraph covers the branch where
// rel.From is not found in graph.Resources (fromService stays "").
func TestCalculateCouplingScore_RelFromMissingInGraph(t *testing.T) {
	a := NewAnalyzer()
	g := makeGraph()

	r1 := addResource(g, "apps", "v1", "Deployment", "svc-a", "default", "svc-a")
	r2 := addResource(g, "apps", "v1", "Deployment", "svc-b", "default", "svc-b")
	addGroup(g, "svc-a", r1)
	addGroup(g, "svc-b", r2)

	// Build a key that is NOT in graph.Resources (unknown resource)
	unknownKey := types.ResourceKey{
		GVK:       schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"},
		Namespace: "default",
		Name:      "ghost",
	}
	toKey := r2.Original.ResourceKey()

	// From is unknown → fromService = "" → cross-service condition false → not counted
	g.AddRelationship(types.Relationship{
		From: unknownKey,
		To:   toKey,
		Type: types.RelationOwnerReference,
	})

	score := a.calculateCouplingScore(g)
	// crossServiceRels=0, totalRels=1 → (0*100)/1 = 0
	if score != 0 {
		t.Errorf("missing-from coupling = %d; want 0", score)
	}
}

// TestCalculateCouplingScore_RelToMissingInGraph covers the branch where
// rel.To is not found in graph.Resources (toService stays "").
func TestCalculateCouplingScore_RelToMissingInGraph(t *testing.T) {
	a := NewAnalyzer()
	g := makeGraph()

	r1 := addResource(g, "apps", "v1", "Deployment", "svc-a", "default", "svc-a")
	r2 := addResource(g, "apps", "v1", "Deployment", "svc-b", "default", "svc-b")
	addGroup(g, "svc-a", r1)
	addGroup(g, "svc-b", r2)

	fromKey := r1.Original.ResourceKey()
	unknownKey := types.ResourceKey{
		GVK:       schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"},
		Namespace: "default",
		Name:      "ghost",
	}

	// To is unknown → toService = "" → cross-service condition false → not counted
	g.AddRelationship(types.Relationship{
		From: fromKey,
		To:   unknownKey,
		Type: types.RelationOwnerReference,
	})

	score := a.calculateCouplingScore(g)
	// crossServiceRels=0, totalRels=1 → 0
	if score != 0 {
		t.Errorf("missing-to coupling = %d; want 0", score)
	}
}

// ── determinePrimaryPattern ──────────────────────────────────────────────────

func TestDeterminePrimaryPattern_FromCounts(t *testing.T) {
	a := NewAnalyzer()
	counts := map[ArchitecturePattern]int{
		PatternMicroservices: 3,
		PatternStateless:     1,
	}
	pattern := a.determinePrimaryPattern(counts, AnalysisMetrics{})
	if pattern != PatternMicroservices {
		t.Errorf("got %q; want microservices", pattern)
	}
}

func TestDeterminePrimaryPattern_Heuristic_Deckhouse(t *testing.T) {
	a := NewAnalyzer()
	pattern := a.determinePrimaryPattern(map[ArchitecturePattern]int{}, AnalysisMetrics{DeckhouseResourceCount: 1})
	if pattern != PatternDeckhouse {
		t.Errorf("got %q; want deckhouse", pattern)
	}
}

func TestDeterminePrimaryPattern_Heuristic_Microservices(t *testing.T) {
	a := NewAnalyzer()
	pattern := a.determinePrimaryPattern(map[ArchitecturePattern]int{}, AnalysisMetrics{TotalServices: 5, CouplingScore: 10})
	if pattern != PatternMicroservices {
		t.Errorf("got %q; want microservices", pattern)
	}
}

func TestDeterminePrimaryPattern_Heuristic_Monolith(t *testing.T) {
	a := NewAnalyzer()
	pattern := a.determinePrimaryPattern(map[ArchitecturePattern]int{}, AnalysisMetrics{TotalServices: 1, CouplingScore: 0})
	if pattern != PatternMonolith {
		t.Errorf("got %q; want monolith", pattern)
	}
}

func TestDeterminePrimaryPattern_Heuristic_Stateful(t *testing.T) {
	a := NewAnalyzer()
	pattern := a.determinePrimaryPattern(map[ArchitecturePattern]int{}, AnalysisMetrics{TotalServices: 3, CouplingScore: 50, StatefulServices: 1})
	if pattern != PatternStateful {
		t.Errorf("got %q; want stateful", pattern)
	}
}

func TestDeterminePrimaryPattern_Heuristic_Stateless(t *testing.T) {
	a := NewAnalyzer()
	pattern := a.determinePrimaryPattern(map[ArchitecturePattern]int{}, AnalysisMetrics{TotalServices: 3, CouplingScore: 50})
	if pattern != PatternStateless {
		t.Errorf("got %q; want stateless", pattern)
	}
}

// TestDeterminePrimaryPattern_HighCouplingMonolith covers the
// CouplingScore > 70 branch of the monolith heuristic.
func TestDeterminePrimaryPattern_HighCouplingMonolith(t *testing.T) {
	a := NewAnalyzer()
	// TotalServices=4 > 3, CouplingScore=80 > 70 → Monolith
	pattern := a.determinePrimaryPattern(map[ArchitecturePattern]int{}, AnalysisMetrics{TotalServices: 4, CouplingScore: 80})
	if pattern != PatternMonolith {
		t.Errorf("got %q; want monolith (high coupling)", pattern)
	}
}

// ── recommendStrategy ────────────────────────────────────────────────────────

func TestRecommendStrategy_Deckhouse(t *testing.T) {
	a := NewAnalyzer()
	s := a.recommendStrategy(PatternDeckhouse, AnalysisMetrics{TotalServices: 3})
	if s != StrategyHybrid {
		t.Errorf("got %q; want hybrid", s)
	}
}

func TestRecommendStrategy_DeckhouseSingle(t *testing.T) {
	a := NewAnalyzer()
	s := a.recommendStrategy(PatternDeckhouse, AnalysisMetrics{TotalServices: 1})
	if s != StrategyUniversal {
		t.Errorf("got %q; want universal", s)
	}
}

func TestRecommendStrategy_MicroservicesMany(t *testing.T) {
	a := NewAnalyzer()
	s := a.recommendStrategy(PatternMicroservices, AnalysisMetrics{TotalServices: 8})
	if s != StrategySeparate {
		t.Errorf("got %q; want separate", s)
	}
}

func TestRecommendStrategy_Monolith(t *testing.T) {
	a := NewAnalyzer()
	s := a.recommendStrategy(PatternMonolith, AnalysisMetrics{TotalServices: 1})
	if s != StrategyUniversal {
		t.Errorf("got %q; want universal", s)
	}
}

func TestRecommendStrategy_Operator(t *testing.T) {
	a := NewAnalyzer()
	s := a.recommendStrategy(PatternOperator, AnalysisMetrics{TotalServices: 3})
	if s != StrategyLibrary {
		t.Errorf("got %q; want library", s)
	}
}

func TestRecommendStrategy_HighComplexity(t *testing.T) {
	a := NewAnalyzer()
	s := a.recommendStrategy(PatternStateless, AnalysisMetrics{TotalServices: 5, ComplexityScore: 80})
	if s != StrategyUmbrella {
		t.Errorf("got %q; want umbrella", s)
	}
}

// TestRecommendStrategy_MicroservicesUmbrella covers 2 < services <= 5 AND coupling < 20 → Umbrella.
func TestRecommendStrategy_MicroservicesUmbrella(t *testing.T) {
	a := NewAnalyzer()
	s := a.recommendStrategy(PatternMicroservices, AnalysisMetrics{TotalServices: 4, CouplingScore: 10})
	if s != StrategyUmbrella {
		t.Errorf("got %q; want umbrella (microservices, low coupling, 3-5 services)", s)
	}
}

// TestRecommendStrategy_MicroservicesFallthrough covers 2 < services <= 5 AND coupling >= 20
// which falls through all microservices branches and reaches the default universal.
func TestRecommendStrategy_MicroservicesFallthrough(t *testing.T) {
	a := NewAnalyzer()
	// TotalServices=4 (2 < 4 <= 5), CouplingScore=50 (>= 20) → no umbrella
	// pattern != Monolith, TotalServices=4 > 2 → not universal from that branch
	// pattern != Operator → not library
	// ComplexityScore=0 <= 70 → not umbrella
	// Falls to default universal
	s := a.recommendStrategy(PatternMicroservices, AnalysisMetrics{TotalServices: 4, CouplingScore: 50, ComplexityScore: 0})
	if s != StrategyUniversal {
		t.Errorf("got %q; want universal (microservices fallthrough)", s)
	}
}

// TestRecommendStrategy_FewServicesNotMonolith covers TotalServices <= 2 with a non-monolith pattern.
func TestRecommendStrategy_FewServicesNotMonolith(t *testing.T) {
	a := NewAnalyzer()
	// pattern != Monolith, but TotalServices=2 <= 2 → universal
	s := a.recommendStrategy(PatternStateful, AnalysisMetrics{TotalServices: 2})
	if s != StrategyUniversal {
		t.Errorf("got %q; want universal (few services, non-monolith)", s)
	}
}

// TestRecommendStrategy_DefaultFallback covers the final default return.
// Pattern that matches none of the specific branches and complexity <= 70.
func TestRecommendStrategy_DefaultFallback(t *testing.T) {
	a := NewAnalyzer()
	// PatternStateless, TotalServices=4 > 2, ComplexityScore=10 <= 70 → universal default
	s := a.recommendStrategy(PatternStateless, AnalysisMetrics{TotalServices: 4, ComplexityScore: 10})
	if s != StrategyUniversal {
		t.Errorf("got %q; want universal (default fallback)", s)
	}
}

// ── calculateConfidence ───────────────────────────────────────────────────────

// TestCalculateConfidence_HighResources covers TotalResources > 20 → +20.
func TestCalculateConfidence_HighResources(t *testing.T) {
	a := NewAnalyzer()
	result := &AnalysisResult{
		Metrics:          AnalysisMetrics{TotalResources: 25},
		DetectedPatterns: []ArchitecturePattern{PatternStateless},
		PrimaryPattern:   PatternStateless,
	}
	c := a.calculateConfidence(result)
	// 50 + 20 (resources>20) + 15 (exactly 1 pattern) = 85
	if c != 85 {
		t.Errorf("confidence = %d; want 85 (50+20+15)", c)
	}
}

// TestCalculateConfidence_MidResources covers 10 < TotalResources <= 20 → +10.
func TestCalculateConfidence_MidResources(t *testing.T) {
	a := NewAnalyzer()
	result := &AnalysisResult{
		Metrics:          AnalysisMetrics{TotalResources: 15},
		DetectedPatterns: []ArchitecturePattern{PatternStateless},
		PrimaryPattern:   PatternStateless,
	}
	c := a.calculateConfidence(result)
	// 50 + 10 (resources 11-20) + 15 (exactly 1 pattern) = 75
	if c != 75 {
		t.Errorf("confidence = %d; want 75 (50+10+15)", c)
	}
}

// TestCalculateConfidence_LowResources covers TotalResources <= 10 (no bonus).
func TestCalculateConfidence_LowResources(t *testing.T) {
	a := NewAnalyzer()
	result := &AnalysisResult{
		Metrics:          AnalysisMetrics{TotalResources: 5},
		DetectedPatterns: []ArchitecturePattern{PatternStateless},
		PrimaryPattern:   PatternStateless,
	}
	c := a.calculateConfidence(result)
	// 50 + 0 (resources<=10) + 15 (exactly 1 pattern) = 65
	if c != 65 {
		t.Errorf("confidence = %d; want 65 (50+0+15)", c)
	}
}

// TestCalculateConfidence_ManyPatterns covers len(DetectedPatterns) > 3 → -15.
func TestCalculateConfidence_ManyPatterns(t *testing.T) {
	a := NewAnalyzer()
	result := &AnalysisResult{
		Metrics: AnalysisMetrics{TotalResources: 5},
		DetectedPatterns: []ArchitecturePattern{
			PatternMicroservices, PatternStateless, PatternStateful, PatternDeckhouse,
		},
		PrimaryPattern: PatternMicroservices,
	}
	c := a.calculateConfidence(result)
	// 50 + 0 (resources<=10) - 15 (>3 patterns) = 35
	if c != 35 {
		t.Errorf("confidence = %d; want 35 (50-15)", c)
	}
}

// TestCalculateConfidence_OnePattern covers len == 1 → +15.
func TestCalculateConfidence_OnePattern(t *testing.T) {
	a := NewAnalyzer()
	result := &AnalysisResult{
		Metrics:          AnalysisMetrics{TotalResources: 0},
		DetectedPatterns: []ArchitecturePattern{PatternMonolith},
		PrimaryPattern:   PatternMonolith,
	}
	c := a.calculateConfidence(result)
	// 50 + 0 (resources=0) + 15 (exactly 1 pattern) = 65
	if c != 65 {
		t.Errorf("confidence = %d; want 65 (50+15)", c)
	}
}

// TestCalculateConfidence_NoPatternsNoBonusPenalty covers len == 0 → no adjustment.
func TestCalculateConfidence_NoPatternsNoBonusPenalty(t *testing.T) {
	a := NewAnalyzer()
	result := &AnalysisResult{
		Metrics:          AnalysisMetrics{TotalResources: 0},
		DetectedPatterns: []ArchitecturePattern{},
		PrimaryPattern:   PatternStateless,
	}
	c := a.calculateConfidence(result)
	// 50 + 0 + 0 = 50
	if c != 50 {
		t.Errorf("confidence = %d; want 50 (base, no pattern adjustment)", c)
	}
}

// TestCalculateConfidence_TwoPatternsNoBonusPenalty covers 1 < len <= 3 → no adjustment.
func TestCalculateConfidence_TwoPatternsNoBonusPenalty(t *testing.T) {
	a := NewAnalyzer()
	result := &AnalysisResult{
		Metrics:          AnalysisMetrics{TotalResources: 0},
		DetectedPatterns: []ArchitecturePattern{PatternStateless, PatternMicroservices},
		PrimaryPattern:   PatternStateless,
	}
	c := a.calculateConfidence(result)
	// 50 + 0 + 0 (2 patterns: no bonus, no penalty) = 50
	if c != 50 {
		t.Errorf("confidence = %d; want 50 (2 patterns, base only)", c)
	}
}

// TestCalculateConfidence_DeckhouseBonus covers Deckhouse pattern + DeckhouseResourceCount > 0 → +15.
func TestCalculateConfidence_DeckhouseBonus(t *testing.T) {
	a := NewAnalyzer()
	result := &AnalysisResult{
		Metrics: AnalysisMetrics{
			TotalResources:         5,
			DeckhouseResourceCount: 2,
		},
		DetectedPatterns: []ArchitecturePattern{PatternDeckhouse},
		PrimaryPattern:   PatternDeckhouse,
	}
	c := a.calculateConfidence(result)
	// 50 + 0 (resources<=10) + 15 (1 pattern) + 15 (deckhouse bonus) = 80
	if c != 80 {
		t.Errorf("confidence = %d; want 80 (50+15+15)", c)
	}
}

// TestCalculateConfidence_DeckhouseNoBonusWhenCountZero verifies no bonus when
// PrimaryPattern == Deckhouse but DeckhouseResourceCount == 0.
func TestCalculateConfidence_DeckhouseNoBonusWhenCountZero(t *testing.T) {
	a := NewAnalyzer()
	result := &AnalysisResult{
		Metrics: AnalysisMetrics{
			TotalResources:         0,
			DeckhouseResourceCount: 0,
		},
		DetectedPatterns: []ArchitecturePattern{PatternDeckhouse},
		PrimaryPattern:   PatternDeckhouse,
	}
	c := a.calculateConfidence(result)
	// 50 + 0 + 15 (1 pattern) + 0 (no deckhouse count) = 65
	if c != 65 {
		t.Errorf("confidence = %d; want 65 (no deckhouse bonus when count=0)", c)
	}
}

// TestCalculateConfidence_NonDeckhousePatternNoBonus verifies +15 deckhouse bonus
// is NOT applied when PrimaryPattern != Deckhouse even if DeckhouseResourceCount > 0.
func TestCalculateConfidence_NonDeckhousePatternNoBonus(t *testing.T) {
	a := NewAnalyzer()
	result := &AnalysisResult{
		Metrics: AnalysisMetrics{
			TotalResources:         0,
			DeckhouseResourceCount: 3,
		},
		DetectedPatterns: []ArchitecturePattern{PatternStateless},
		PrimaryPattern:   PatternStateless,
	}
	c := a.calculateConfidence(result)
	// 50 + 0 + 15 (1 pattern) = 65 (no deckhouse bonus)
	if c != 65 {
		t.Errorf("confidence = %d; want 65 (no deckhouse bonus for non-deckhouse primary)", c)
	}
}

// TestCalculateConfidence_ClampAbove100 covers the upper cap branch.
// Requires enough resources + deckhouse + single pattern to exceed 100.
func TestCalculateConfidence_ClampAbove100(t *testing.T) {
	a := NewAnalyzer()
	result := &AnalysisResult{
		Metrics: AnalysisMetrics{
			TotalResources:         100,
			DeckhouseResourceCount: 5,
		},
		DetectedPatterns: []ArchitecturePattern{PatternDeckhouse},
		PrimaryPattern:   PatternDeckhouse,
	}
	c := a.calculateConfidence(result)
	// 50 + 20 + 15 + 15 = 100 exactly; still valid cap test
	if c > 100 {
		t.Errorf("confidence = %d; must not exceed 100", c)
	}
	if c != 100 {
		t.Errorf("confidence = %d; want 100 (capped)", c)
	}
}

// TestCalculateConfidence_ClampBelow0_NeverNegative verifies the lower-bound clamp.
// With current scoring parameters the minimum achievable is 50-15=35 which is
// already positive; the < 0 guard exists defensively.
func TestCalculateConfidence_ClampBelow0_NeverNegative(t *testing.T) {
	a := NewAnalyzer()
	result := &AnalysisResult{
		Metrics: AnalysisMetrics{TotalResources: 0},
		DetectedPatterns: []ArchitecturePattern{
			PatternMicroservices, PatternStateless, PatternStateful, PatternDeckhouse,
		},
		PrimaryPattern: PatternMicroservices,
	}
	c := a.calculateConfidence(result)
	// 50 - 15 = 35; still >= 0. Verify the clamp doesn't accidentally go negative.
	if c < 0 {
		t.Errorf("confidence = %d; must not be negative", c)
	}
}

// ── generateRecommendations ───────────────────────────────────────────────────

// TestGenerateRecommendations_NoViolations verifies that when all best practices
// are compliant, no "Address Best Practice Violations" recommendation is added.
func TestGenerateRecommendations_NoViolations(t *testing.T) {
	a := NewAnalyzer()
	g := makeGraph()
	result := &AnalysisResult{
		PrimaryPattern:      PatternMonolith,
		RecommendedStrategy: StrategyUniversal,
		Metrics:             AnalysisMetrics{TotalServices: 1, ComplexityScore: 10},
		BestPractices: []BestPractice{
			{Compliant: true, Severity: SeverityInfo},
		},
	}
	recs := a.generateRecommendations(result, g)

	for _, rec := range recs {
		if rec.Title == "Address Best Practice Violations" {
			t.Error("should not add violation recommendation when all practices are compliant")
		}
	}
	// Must still have at least the chart strategy recommendation
	if len(recs) == 0 {
		t.Error("should always have at least the chart strategy recommendation")
	}
}

// TestGenerateRecommendations_WithViolations verifies violation recommendation is added.
func TestGenerateRecommendations_WithViolations(t *testing.T) {
	a := NewAnalyzer()
	g := makeGraph()
	result := &AnalysisResult{
		PrimaryPattern:      PatternMonolith,
		RecommendedStrategy: StrategyUniversal,
		Metrics:             AnalysisMetrics{TotalServices: 1, ComplexityScore: 10},
		BestPractices: []BestPractice{
			{Compliant: false, Severity: SeverityWarning},
		},
	}
	recs := a.generateRecommendations(result, g)

	found := false
	for _, rec := range recs {
		if rec.Title == "Address Best Practice Violations" {
			found = true
		}
	}
	if !found {
		t.Error("should add violation recommendation when non-info violations exist")
	}
}

// TestGenerateRecommendations_InfoViolationSkipped verifies that Info-severity
// non-compliant practices do NOT trigger the violation recommendation.
func TestGenerateRecommendations_InfoViolationSkipped(t *testing.T) {
	a := NewAnalyzer()
	g := makeGraph()
	result := &AnalysisResult{
		PrimaryPattern:      PatternMonolith,
		RecommendedStrategy: StrategyUniversal,
		Metrics:             AnalysisMetrics{TotalServices: 1, ComplexityScore: 10},
		BestPractices: []BestPractice{
			{Compliant: false, Severity: SeverityInfo},
		},
	}
	recs := a.generateRecommendations(result, g)

	for _, rec := range recs {
		if rec.Title == "Address Best Practice Violations" {
			t.Error("info-severity violations should not trigger the violation recommendation")
		}
	}
}

// TestGenerateRecommendations_LowComplexityNoComplexityRec verifies that when
// ComplexityScore <= 60, no complexity reduction recommendation is added.
func TestGenerateRecommendations_LowComplexityNoComplexityRec(t *testing.T) {
	a := NewAnalyzer()
	g := makeGraph()
	result := &AnalysisResult{
		PrimaryPattern:      PatternMonolith,
		RecommendedStrategy: StrategyUniversal,
		Metrics:             AnalysisMetrics{TotalServices: 1, ComplexityScore: 60},
		BestPractices:       []BestPractice{},
	}
	recs := a.generateRecommendations(result, g)

	for _, rec := range recs {
		if rec.Title == "Consider Complexity Reduction" {
			t.Error("should not add complexity recommendation when score <= 60")
		}
	}
}

// TestGenerateRecommendations_HighComplexityAddsRec verifies that when
// ComplexityScore > 60, the complexity reduction recommendation is added.
func TestGenerateRecommendations_HighComplexityAddsRec(t *testing.T) {
	a := NewAnalyzer()
	g := makeGraph()
	result := &AnalysisResult{
		PrimaryPattern:      PatternStateless,
		RecommendedStrategy: StrategyUniversal,
		Metrics:             AnalysisMetrics{TotalServices: 4, ComplexityScore: 61},
		BestPractices:       []BestPractice{},
	}
	recs := a.generateRecommendations(result, g)

	found := false
	for _, rec := range recs {
		if rec.Title == "Consider Complexity Reduction" {
			found = true
		}
	}
	if !found {
		t.Error("should add complexity recommendation when score > 60")
	}
}

// TestGenerateRecommendations_SortedByPriority verifies recommendations are
// returned in ascending priority order.
func TestGenerateRecommendations_SortedByPriority(t *testing.T) {
	a := NewAnalyzer()
	g := makeGraph()
	result := &AnalysisResult{
		PrimaryPattern:      PatternStateless,
		RecommendedStrategy: StrategyUniversal,
		Metrics:             AnalysisMetrics{TotalServices: 4, ComplexityScore: 80},
		BestPractices: []BestPractice{
			{Compliant: false, Severity: SeverityError},
		},
	}
	recs := a.generateRecommendations(result, g)

	for i := 1; i < len(recs); i++ {
		if recs[i-1].Priority > recs[i].Priority {
			t.Errorf("recommendations not sorted: priority[%d]=%d > priority[%d]=%d",
				i-1, recs[i-1].Priority, i, recs[i].Priority)
		}
	}
}

// ── getStrategyRationale branch coverage ─────────────────────────────────────

func TestGetStrategyDescription(t *testing.T) {
	desc := getStrategyDescription(StrategyUniversal)
	if desc == "" {
		t.Error("universal description should not be empty")
	}
}

func TestGetStrategyRationale(t *testing.T) {
	r := getStrategyRationale(StrategyUniversal, PatternMonolith, AnalysisMetrics{TotalServices: 1})
	if r == "" {
		t.Error("rationale should not be empty")
	}
	r = getStrategyRationale(StrategyLibrary, PatternOperator, AnalysisMetrics{})
	if !strings.Contains(r, "Operator") {
		t.Error("library rationale should mention operator")
	}
}

// TestGetStrategyRationale_AllBranches exercises every case in getStrategyRationale.
func TestGetStrategyRationale_AllBranches(t *testing.T) {
	tests := []struct {
		strategy ChartStrategy
		pattern  ArchitecturePattern
		metrics  AnalysisMetrics
		wantSub  string
	}{
		{StrategySeparate, PatternMicroservices, AnalysisMetrics{TotalServices: 6, CouplingScore: 15}, "separate"},
		{StrategyUmbrella, PatternMicroservices, AnalysisMetrics{TotalServices: 4, CouplingScore: 30}, "umbrella"},
		{StrategyLibrary, PatternOperator, AnalysisMetrics{}, "Operator"},
		{StrategyHybrid, PatternDeckhouse, AnalysisMetrics{}, "Deckhouse"},
		{"unknown-strategy", PatternStateless, AnalysisMetrics{}, "Recommended"},
	}

	for _, tt := range tests {
		r := getStrategyRationale(tt.strategy, tt.pattern, tt.metrics)
		if !strings.Contains(r, tt.wantSub) {
			t.Errorf("getStrategyRationale(%s) = %q; want substring %q", tt.strategy, r, tt.wantSub)
		}
	}
}

func TestGetStrategySteps(t *testing.T) {
	steps := getStrategySteps(StrategyUniversal)
	if len(steps) == 0 {
		t.Error("universal should have steps")
	}
	steps = getStrategySteps(StrategyHybrid)
	if steps != nil {
		t.Error("hybrid has no steps defined, should be nil")
	}
}

// ── Formatter ────────────────────────────────────────────────────────────────

func TestNewFormatter(t *testing.T) {
	f := NewFormatter(false)
	if f.ColorEnabled {
		t.Error("should not have color enabled")
	}
}

func TestFormatter_FormatReport(t *testing.T) {
	f := NewFormatter(false)
	report := &Report{
		Sections: []ReportSection{
			{
				Title:       "Test Section",
				Description: "Desc",
				Items: []ReportItem{
					{Title: "Item 1", Content: "Content 1", Level: "info"},
				},
			},
		},
	}
	out := f.FormatReport(report)
	if !strings.Contains(out, "Test Section") {
		t.Error("report should contain section title")
	}
	if !strings.Contains(out, "Item 1") {
		t.Error("report should contain item title")
	}
}

// TestFormatter_FormatReport_MultipleSections verifies the inter-section newline
// branch (i > 0) in FormatReport is exercised.
func TestFormatter_FormatReport_MultipleSections(t *testing.T) {
	f := NewFormatter(false)
	report := &Report{
		Sections: []ReportSection{
			{Title: "Section A", Items: []ReportItem{{Title: "A", Content: "a", Level: "info"}}},
			{Title: "Section B", Items: []ReportItem{{Title: "B", Content: "b", Level: "info"}}},
		},
	}
	out := f.FormatReport(report)
	if !strings.Contains(out, "Section A") || !strings.Contains(out, "Section B") {
		t.Error("report should contain both sections")
	}
}

func TestFormatter_FormatJSON(t *testing.T) {
	f := NewFormatter(false)
	report := &Report{Sections: []ReportSection{}}
	json, err := f.FormatJSON(report)
	if err != nil {
		t.Fatalf("FormatJSON error: %v", err)
	}
	if !strings.Contains(json, "Sections") {
		t.Error("JSON should contain Sections key")
	}
}

// TestFormatter_FormatJSON_WithAnalysisResult exercises the JSON marshalling of
// a Report with an AnalysisResult populated — ensures more code paths in FormatJSON
// are exercised and the output is valid JSON.
func TestFormatter_FormatJSON_WithAnalysisResult(t *testing.T) {
	f := NewFormatter(false)
	report := &Report{
		AnalysisResult: &AnalysisResult{
			PrimaryPattern:      PatternMicroservices,
			RecommendedStrategy: StrategyUmbrella,
			Confidence:          80,
			Metrics: AnalysisMetrics{
				TotalServices:  4,
				TotalResources: 20,
			},
			DetectedPatterns: []ArchitecturePattern{PatternMicroservices, PatternStateless},
			BestPractices:    []BestPractice{},
		},
		Sections: []ReportSection{
			{
				Title: "Test",
				Items: []ReportItem{
					{Title: "item", Content: "content", Level: "info"},
				},
			},
		},
	}

	out, err := f.FormatJSON(report)
	if err != nil {
		t.Fatalf("FormatJSON error: %v", err)
	}
	if !strings.Contains(out, "microservices") {
		t.Error("JSON output should contain primary pattern")
	}
	if !strings.Contains(out, "Sections") {
		t.Error("JSON output should contain sections")
	}
}

func TestFormatter_FormatMarkdown(t *testing.T) {
	f := NewFormatter(false)
	report := &Report{
		Sections: []ReportSection{
			{
				Title:       "Section",
				Description: "Desc",
				Items: []ReportItem{
					{Title: "Item", Content: "Content", Level: "info"},
				},
			},
		},
	}
	md := f.FormatMarkdown(report)
	if !strings.Contains(md, "## Section") {
		t.Error("markdown should contain section heading")
	}
}

func TestFormatter_FormatSummary(t *testing.T) {
	f := NewFormatter(false)
	result := &AnalysisResult{
		PrimaryPattern:      PatternMonolith,
		RecommendedStrategy: StrategyUniversal,
		Confidence:          75,
		Metrics: AnalysisMetrics{
			TotalServices:  1,
			TotalResources: 3,
		},
		BestPractices: []BestPractice{},
	}
	out := f.FormatSummary(result)
	if !strings.Contains(out, "monolith") {
		t.Error("summary should contain pattern name")
	}
	if !strings.Contains(out, "passed") {
		t.Error("summary should show all passed when no violations")
	}
}

func TestFormatter_FormatSummary_WithViolations(t *testing.T) {
	f := NewFormatter(false)
	result := &AnalysisResult{
		BestPractices: []BestPractice{
			{Compliant: false, Severity: SeverityWarning},
		},
		Metrics: AnalysisMetrics{},
	}
	out := f.FormatSummary(result)
	if !strings.Contains(out, "violation") {
		t.Error("summary should mention violations")
	}
}

func TestFormatter_Colorize_Disabled(t *testing.T) {
	f := NewFormatter(false)
	out := f.colorize("test", "red", true)
	if strings.Contains(out, "\033[") {
		t.Error("should not contain ANSI codes when color disabled")
	}
}

func TestFormatter_Colorize_Enabled(t *testing.T) {
	f := NewFormatter(true)
	out := f.colorize("test", "red", true)
	if !strings.Contains(out, "\033[") {
		t.Error("should contain ANSI codes when color enabled")
	}
}

// TestFormatter_Colorize_NonBold covers the non-bold ANSI branch.
func TestFormatter_Colorize_NonBold(t *testing.T) {
	f := NewFormatter(true)
	out := f.colorize("text", "green", false)
	// Non-bold: should have \033[3Xm not \033[1;3Xm
	if strings.Contains(out, "1;") {
		t.Error("non-bold colorize should not produce bold ANSI code")
	}
	if !strings.Contains(out, "\033[") {
		t.Error("non-bold colorize should still produce ANSI codes when enabled")
	}
}

// TestFormatter_Colorize_UnknownColor covers the unknown-color early-return path.
func TestFormatter_Colorize_UnknownColor(t *testing.T) {
	f := NewFormatter(true)
	// "purple" is not in the codes map → should return text unchanged
	out := f.colorize("test", "purple", true)
	if out != "test" {
		t.Errorf("unknown color should return text unchanged; got %q", out)
	}
}

// TestFormatter_Colorize_AllKnownColors exercises all color codes in the map
// to ensure no color key is misspelled and each produces ANSI output.
func TestFormatter_Colorize_AllKnownColors(t *testing.T) {
	f := NewFormatter(true)
	colors := []string{"red", "green", "yellow", "blue", "magenta", "cyan", "gray", "white"}
	for _, color := range colors {
		out := f.colorize("text", color, true)
		if !strings.Contains(out, "\033[") {
			t.Errorf("color %q should produce ANSI codes", color)
		}
	}
}

func TestFormatter_GetLevelIcon(t *testing.T) {
	f := NewFormatter(false)
	if f.getLevelIcon("success") != "✓" {
		t.Error("wrong icon for success")
	}
	if f.getLevelIcon("unknown") != "•" {
		t.Error("wrong default icon")
	}
}

func TestFormatter_GetLevelColor(t *testing.T) {
	f := NewFormatter(false)
	if f.getLevelColor("error") != "red" {
		t.Error("wrong color for error")
	}
	if f.getLevelColor("unknown") != "white" {
		t.Error("wrong default color")
	}
}

// ── Recommender ──────────────────────────────────────────────────────────────

func TestNewRecommender(t *testing.T) {
	a := DefaultAnalyzer()
	r := NewRecommender(a)
	if r == nil {
		t.Fatal("NewRecommender returned nil")
	}
}

func TestRecommender_GenerateReport(t *testing.T) {
	a := DefaultAnalyzer()
	r := NewRecommender(a)
	g := makeGraph()
	res := addResource(g, "apps", "v1", "Deployment", "app", "default", "app")
	addGroup(g, "app", res)

	report := r.GenerateReport(g)
	if report == nil {
		t.Fatal("GenerateReport returned nil")
	}
	if len(report.Sections) != 5 {
		t.Errorf("expected 5 sections, got %d", len(report.Sections))
	}
}

// TestRecommender_GenerateReport_WithSecondaryPatterns exercises the secondary-patterns
// branch in generatePatternsSection (len(DetectedPatterns) > 1).
func TestRecommender_GenerateReport_WithSecondaryPatterns(t *testing.T) {
	a := DefaultAnalyzer()
	r := NewRecommender(a)
	g := makeGraph()

	// Build a graph that triggers multiple detected patterns:
	// 4 services with StatefulSets (StatefulSet detector) AND Deckhouse resources
	for _, name := range []string{"svc1", "svc2", "svc3", "svc4"} {
		res := addResource(g, "apps", "v1", "Deployment", name, "default", name)
		addGroup(g, name, res)
	}
	addResource(g, "deckhouse.io", "v1", "ModuleConfig", "mc", "default", "svc1")

	report := r.GenerateReport(g)
	if report == nil {
		t.Fatal("GenerateReport returned nil")
	}
	// Verify patterns section exists (index 1)
	if len(report.Sections) < 2 {
		t.Fatal("report should have at least 2 sections")
	}
}

// TestRecommender_GenerateReport_AllPracticesCompliant exercises the
// totalIssues == 0 branch in generateBestPracticesSection (recommender.go:152-158),
// which returns a "Status: All passed" item instead of a violation summary.
func TestRecommender_GenerateReport_AllPracticesCompliant(t *testing.T) {
	a := NewAnalyzer()
	// No checkers → BestPractices will be empty → totalIssues == 0
	r := NewRecommender(a)
	g := makeGraph()
	res := addResource(g, "apps", "v1", "Deployment", "app", "default", "app")
	addGroup(g, "app", res)

	report := r.GenerateReport(g)
	if report == nil {
		t.Fatal("GenerateReport returned nil")
	}

	// Best practices section is index 2
	if len(report.Sections) < 3 {
		t.Fatal("report should have at least 3 sections")
	}
	bpSection := report.Sections[2]
	found := false
	for _, item := range bpSection.Items {
		if item.Title == "Status" && strings.Contains(item.Content, "All best practices checks passed") {
			found = true
		}
	}
	if !found {
		t.Error("best practices section should show 'all passed' when no checkers are run")
	}
}

// TestRecommender_GenerateReport_CriticalViolation exercises the critical-severity
// branch in generateBestPracticesSection: the summary counts critical violations
// (recommender.go:160-162) and the level is set to "error" (recommender.go:196-198).
func TestRecommender_GenerateReport_CriticalViolation(t *testing.T) {
	a := NewAnalyzer()
	// Add security checker which can produce critical (BP-SEC-003) violations.
	a.AddChecker(NewSecurityChecker())
	r := NewRecommender(a)
	g := makeGraph()

	// Privileged container → BP-SEC-003 (SeverityCritical, not compliant).
	pr := addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{
			"name": "main",
			"securityContext": map[string]interface{}{
				"runAsNonRoot":           true,
				"readOnlyRootFilesystem": true,
				"privileged":             true,
			},
		},
	})
	addGroup(g, "app", pr)

	report := r.GenerateReport(g)
	if report == nil {
		t.Fatal("GenerateReport returned nil")
	}

	// Best practices section is index 2
	if len(report.Sections) < 3 {
		t.Fatal("report should have at least 3 sections")
	}
	bpSection := report.Sections[2]

	// The summary item should mention "Critical"
	foundCriticalSummary := false
	for _, item := range bpSection.Items {
		if item.Title == "Summary" && strings.Contains(item.Content, "Critical") {
			foundCriticalSummary = true
		}
	}
	if !foundCriticalSummary {
		t.Error("best practices section summary should mention 'Critical' when critical violations exist")
	}

	// One of the items should have level "error" (critical severity → error level)
	foundErrorLevel := false
	for _, item := range bpSection.Items {
		if item.Level == "error" {
			foundErrorLevel = true
		}
	}
	if !foundErrorLevel {
		t.Error("best practices section should have an item with level 'error' for critical severity")
	}
}

// TestRecommender_GenerateReport_ManyAffectedResources exercises the i >= 5 truncation
// branch in generateBestPracticesSection (recommender.go:184-186): when more than 5
// resources are affected, the list is truncated with "... and N more".
func TestRecommender_GenerateReport_ManyAffectedResources(t *testing.T) {
	a := NewAnalyzer()
	// Use SecurityChecker: workloads with nil containers get flagged for
	// both runAsNonRoot (SEC-001) and readOnlyRootFS (SEC-002).
	a.AddChecker(NewSecurityChecker())
	r := NewRecommender(a)
	g := makeGraph()

	// Add 7 deployments without containers — each will appear in SEC-001 and SEC-002.
	for i := 0; i < 7; i++ {
		name := strings.Repeat("x", i+1) // unique names: x, xx, xxx ...
		addResource(g, "apps", "v1", "Deployment", name, "default", name)
	}

	report := r.GenerateReport(g)
	if report == nil {
		t.Fatal("GenerateReport returned nil")
	}

	// Best practices section is index 2
	if len(report.Sections) < 3 {
		t.Fatal("report should have at least 3 sections")
	}
	bpSection := report.Sections[2]

	// One of the detail items should contain the truncation marker.
	foundTruncation := false
	for _, item := range bpSection.Items {
		if strings.Contains(item.Content, "... and") && strings.Contains(item.Content, "more") {
			foundTruncation = true
		}
	}
	if !foundTruncation {
		t.Error("best practices section should truncate affected resources list when > 5 resources")
	}
}

// TestRecommender_GenerateReport_StrategyWithAlternatives exercises the
// len(alternatives) > 0 branch in generateStrategySection (recommender.go:249-258):
// when the analyzer recommends StrategySeparate, an alternative of StrategyUmbrella
// is suggested, producing an "Alternatives" item in the strategy section.
func TestRecommender_GenerateReport_StrategyWithAlternatives(t *testing.T) {
	a := NewAnalyzer()
	// Use MicroservicesDetector: 8 services → many services → StrategySeparate.
	a.AddDetector(NewMicroservicesDetector())
	r := NewRecommender(a)
	g := makeGraph()

	// 8 service groups each with a Deployment — triggers StrategySeparate recommendation
	// (TotalServices=8 > 5 → StrategySeparate in recommendStrategy).
	for i := 0; i < 8; i++ {
		name := strings.Repeat("s", i+1)
		res := addResource(g, "apps", "v1", "Deployment", name, "default", name)
		addGroup(g, name, res)
	}

	report := r.GenerateReport(g)
	if report == nil {
		t.Fatal("GenerateReport returned nil")
	}

	// Strategy section is index 3
	if len(report.Sections) < 4 {
		t.Fatal("report should have at least 4 sections")
	}
	stratSection := report.Sections[3]

	// The section should contain an "Alternatives" item.
	foundAlternatives := false
	for _, item := range stratSection.Items {
		if item.Title == "Alternatives" {
			foundAlternatives = true
		}
	}
	if !foundAlternatives {
		t.Error("strategy section should have 'Alternatives' item when separate strategy is recommended")
	}
}

// TestRecommender_ActionItemsSection_Empty exercises the len(reportItems) == 0
// branch in generateActionItemsSection (recommender.go:348-356): when all
// recommendations have empty ImplementationSteps and no auto-fixable violations
// exist, the "No action items" status item is produced.
// NOTE: generateActionItemsSection is called directly (package-internal) with
// a crafted AnalysisResult to reach this otherwise hard-to-reach branch.
func TestRecommender_ActionItemsSection_Empty(t *testing.T) {
	r := NewRecommender(NewAnalyzer())

	// Craft an AnalysisResult with one recommendation that has no ImplementationSteps
	// and no auto-fixable best practices violations — this produces 0 action items.
	result := &AnalysisResult{
		Recommendations: []Recommendation{
			{
				Priority:            1,
				Title:               "Chart Strategy",
				ImplementationSteps: []string{}, // empty — no steps → no action items
			},
		},
		BestPractices: []BestPractice{
			{Compliant: true, AutoFixable: false}, // compliant → skipped
		},
	}

	section := r.generateActionItemsSection(result)

	found := false
	for _, item := range section.Items {
		if item.Title == "Status" && strings.Contains(item.Content, "No action items") {
			found = true
		}
	}
	if !found {
		t.Error("generateActionItemsSection should return 'No action items' status when no items are generated")
	}
}

// TestRecommender_ActionItemsSection_MoreThan10 exercises the i >= 10 truncation
// branch in generateActionItemsSection (recommender.go:319-321): when more than 10
// items fall in the same priority group, the list is truncated with "... and N more".
// NOTE: generateActionItemsSection is called directly (package-internal) to craft
// a scenario with > 10 steps in a single priority group.
func TestRecommender_ActionItemsSection_MoreThan10(t *testing.T) {
	r := NewRecommender(NewAnalyzer())

	// Build 12 implementation steps in a single priority-1 recommendation.
	steps := make([]string, 12)
	for i := range steps {
		steps[i] = "Step number " + strings.Repeat("x", i+1)
	}

	result := &AnalysisResult{
		Recommendations: []Recommendation{
			{
				Priority:            1,
				Title:               "Many Steps Recommendation",
				Impact:              "High impact",
				ImplementationSteps: steps,
			},
		},
		BestPractices: []BestPractice{},
	}

	section := r.generateActionItemsSection(result)

	// The generated content should contain the truncation marker.
	foundTruncation := false
	for _, item := range section.Items {
		if strings.Contains(item.Content, "... and") && strings.Contains(item.Content, "more") {
			foundTruncation = true
		}
	}
	if !foundTruncation {
		t.Error("generateActionItemsSection should truncate items list when > 10 items in a priority group")
	}
}

func TestRecommender_ComplexityLevel(t *testing.T) {
	r := NewRecommender(NewAnalyzer())
	if r.complexityLevel(10) != "Low" {
		t.Error("10 should be Low")
	}
	if r.complexityLevel(50) != "Medium" {
		t.Error("50 should be Medium")
	}
	if r.complexityLevel(80) != "High" {
		t.Error("80 should be High")
	}
}

// TestRecommender_ComplexityLevelSeverity covers all three branches of complexityLevelSeverity.
func TestRecommender_ComplexityLevelSeverity(t *testing.T) {
	r := NewRecommender(NewAnalyzer())
	if r.complexityLevelSeverity(10) != "success" {
		t.Error("score < 30 should be 'success'")
	}
	if r.complexityLevelSeverity(45) != "info" {
		t.Error("30 <= score < 60 should be 'info'")
	}
	if r.complexityLevelSeverity(70) != "warning" {
		t.Error("score >= 60 should be 'warning'")
	}
}

func TestRecommender_CouplingLevel(t *testing.T) {
	r := NewRecommender(NewAnalyzer())
	if !strings.Contains(r.couplingLevel(10), "Low") {
		t.Error("10 should be Low")
	}
	if r.couplingLevel(30) != "Medium" {
		t.Error("30 should be Medium")
	}
	if !strings.Contains(r.couplingLevel(60), "High") {
		t.Error("60 should be High")
	}
}

// TestRecommender_CouplingLevelSeverity covers all three branches of couplingLevelSeverity.
func TestRecommender_CouplingLevelSeverity(t *testing.T) {
	r := NewRecommender(NewAnalyzer())
	if r.couplingLevelSeverity(10) != "success" {
		t.Error("score < 20 should be 'success'")
	}
	if r.couplingLevelSeverity(35) != "info" {
		t.Error("20 <= score < 50 should be 'info'")
	}
	if r.couplingLevelSeverity(60) != "warning" {
		t.Error("score >= 50 should be 'warning'")
	}
}

func TestRecommender_EstimateEffort(t *testing.T) {
	r := NewRecommender(NewAnalyzer())
	if !strings.Contains(r.estimateEffort("Generate charts with dhg"), "automated") {
		t.Error("dhg should be Low (automated)")
	}
	if r.estimateEffort("Review configuration") != "Low" {
		t.Error("review should be Low")
	}
	if r.estimateEffort("Add monitoring") != "Medium" {
		t.Error("add should be Medium")
	}
	if r.estimateEffort("Refactor the entire system") != "High" {
		t.Error("refactor should be High")
	}
}

// TestRecommender_EstimateEffort_DefaultMedium covers the default "Medium" return
// when the step text matches none of the keyword patterns.
func TestRecommender_EstimateEffort_DefaultMedium(t *testing.T) {
	r := NewRecommender(NewAnalyzer())
	effort := r.estimateEffort("Set up the pipeline")
	if effort != "Medium" {
		t.Errorf("default effort = %q; want Medium", effort)
	}
}

func TestRecommender_SeverityToPriority(t *testing.T) {
	r := NewRecommender(NewAnalyzer())
	if r.severityToPriority(SeverityCritical) != 1 {
		t.Error("critical should be priority 1")
	}
	if r.severityToPriority(SeverityWarning) != 2 {
		t.Error("warning should be priority 2")
	}
	if r.severityToPriority(SeverityInfo) != 3 {
		t.Error("info should be priority 3")
	}
}

// TestRecommender_SeverityToPriority_Error covers the SeverityError branch → 1.
func TestRecommender_SeverityToPriority_Error(t *testing.T) {
	r := NewRecommender(NewAnalyzer())
	if r.severityToPriority(SeverityError) != 1 {
		t.Error("error severity should be priority 1")
	}
}

func TestRecommender_ExplainPattern(t *testing.T) {
	r := NewRecommender(NewAnalyzer())
	exp := r.explainPattern(PatternMonolith, AnalysisMetrics{})
	if !strings.Contains(exp, "monolithic") {
		t.Error("monolith explanation should mention 'monolithic'")
	}
	exp = r.explainPattern("unknown", AnalysisMetrics{})
	if !strings.Contains(exp, "Custom") {
		t.Error("unknown pattern should return custom explanation")
	}
}

// TestRecommender_ExplainPattern_AllKnown exercises all known pattern explanations.
func TestRecommender_ExplainPattern_AllKnown(t *testing.T) {
	r := NewRecommender(NewAnalyzer())
	knownPatterns := []ArchitecturePattern{
		PatternMicroservices, PatternMonolith, PatternStateful,
		PatternStateless, PatternDeckhouse,
	}
	for _, p := range knownPatterns {
		exp := r.explainPattern(p, AnalysisMetrics{})
		if exp == "" {
			t.Errorf("explanation for %s should not be empty", p)
		}
	}
}

func TestRecommender_GetAlternativeStrategies(t *testing.T) {
	r := NewRecommender(NewAnalyzer())

	alts := r.getAlternativeStrategies(&AnalysisResult{
		RecommendedStrategy: StrategyUniversal,
		Metrics:             AnalysisMetrics{TotalServices: 5},
	})
	if len(alts) != 1 || alts[0].Strategy != StrategyUmbrella {
		t.Error("universal with >3 services should suggest umbrella")
	}

	alts = r.getAlternativeStrategies(&AnalysisResult{
		RecommendedStrategy: StrategySeparate,
	})
	if len(alts) != 1 || alts[0].Strategy != StrategyUmbrella {
		t.Error("separate should suggest umbrella")
	}
}

// TestRecommender_GetAlternativeStrategies_UmbrellaHighCoupling covers the
// StrategyUmbrella + CouplingScore > 70 → suggest Universal branch.
func TestRecommender_GetAlternativeStrategies_UmbrellaHighCoupling(t *testing.T) {
	r := NewRecommender(NewAnalyzer())
	alts := r.getAlternativeStrategies(&AnalysisResult{
		RecommendedStrategy: StrategyUmbrella,
		Metrics:             AnalysisMetrics{CouplingScore: 75},
	})
	if len(alts) != 1 || alts[0].Strategy != StrategyUniversal {
		t.Errorf("umbrella+high coupling should suggest universal; got %v", alts)
	}
}

// TestRecommender_GetAlternativeStrategies_UmbrellaLowCoupling covers the
// StrategyUmbrella + CouplingScore <= 70 → no alternatives.
func TestRecommender_GetAlternativeStrategies_UmbrellaLowCoupling(t *testing.T) {
	r := NewRecommender(NewAnalyzer())
	alts := r.getAlternativeStrategies(&AnalysisResult{
		RecommendedStrategy: StrategyUmbrella,
		Metrics:             AnalysisMetrics{CouplingScore: 50},
	})
	if len(alts) != 0 {
		t.Errorf("umbrella+low coupling should have no alternatives; got %v", alts)
	}
}

// TestRecommender_GetAlternativeStrategies_UniversalFewServices covers
// StrategyUniversal + TotalServices <= 3 → no alternatives.
func TestRecommender_GetAlternativeStrategies_UniversalFewServices(t *testing.T) {
	r := NewRecommender(NewAnalyzer())
	alts := r.getAlternativeStrategies(&AnalysisResult{
		RecommendedStrategy: StrategyUniversal,
		Metrics:             AnalysisMetrics{TotalServices: 2},
	})
	if len(alts) != 0 {
		t.Errorf("universal+few services should have no alternatives; got %v", alts)
	}
}

// TestRecommender_GenerateReport_AutoFixablePractice exercises the AutoFixable branch
// in generateActionItemsSection (auto-fixable best practices).
func TestRecommender_GenerateReport_AutoFixablePractice(t *testing.T) {
	a := NewAnalyzer()
	// Create a checker that returns an auto-fixable practice
	a.AddChecker(NewSecurityChecker())
	r := NewRecommender(a)
	g := makeGraph()

	// Deployment with no containers → SecurityChecker will flag non-root and readonly
	addResource(g, "apps", "v1", "Deployment", "app", "default", "app")
	res := addResource(g, "apps", "v1", "Deployment", "app2", "default", "app2")
	addGroup(g, "app", res)

	report := r.GenerateReport(g)
	if report == nil {
		t.Fatal("GenerateReport returned nil")
	}
	// We just verify it doesn't panic and returns sections
	if len(report.Sections) != 5 {
		t.Errorf("expected 5 sections, got %d", len(report.Sections))
	}
}

// ── Strategy helper functions ────────────────────────────────────────────────

// TestGetStrategySteps_AllStrategies verifies step lists for all defined strategies.
func TestGetStrategySteps_AllStrategies(t *testing.T) {
	definedStrategies := []ChartStrategy{
		StrategyUniversal, StrategySeparate, StrategyUmbrella, StrategyLibrary,
	}
	for _, s := range definedStrategies {
		steps := getStrategySteps(s)
		if len(steps) == 0 {
			t.Errorf("strategy %s should have steps defined", s)
		}
	}
}

// ── Constants ────────────────────────────────────────────────────────────────

func TestArchitecturePattern_Constants(t *testing.T) {
	tests := map[ArchitecturePattern]string{
		PatternMicroservices: "microservices",
		PatternMonolith:      "monolith",
		PatternStateful:      "stateful",
		PatternStateless:     "stateless",
		PatternSidecar:       "sidecar",
		PatternDaemonSet:     "daemonset",
		PatternJob:           "job",
		PatternOperator:      "operator",
		PatternDeckhouse:     "deckhouse",
	}
	for c, w := range tests {
		if string(c) != w {
			t.Errorf("%q != %q", c, w)
		}
	}
}

func TestChartStrategy_Constants(t *testing.T) {
	tests := map[ChartStrategy]string{
		StrategyUniversal: "universal",
		StrategySeparate:  "separate",
		StrategyLibrary:   "library",
		StrategyUmbrella:  "umbrella",
		StrategyHybrid:    "hybrid",
	}
	for c, w := range tests {
		if string(c) != w {
			t.Errorf("%q != %q", c, w)
		}
	}
}

func TestSeverity_Constants(t *testing.T) {
	tests := map[Severity]string{
		SeverityInfo:     "info",
		SeverityWarning:  "warning",
		SeverityError:    "error",
		SeverityCritical: "critical",
	}
	for c, w := range tests {
		if string(c) != w {
			t.Errorf("%q != %q", c, w)
		}
	}
}

// ── JobDetector ───────────────────────────────────────────────────────────────

func TestJobDetector_Name(t *testing.T) {
	d := NewJobDetector()
	if d.Name() != "job" {
		t.Errorf("Name() = %q; want job", d.Name())
	}
}

func TestJobDetector_WithJob(t *testing.T) {
	d := NewJobDetector()
	g := makeGraph()
	addResource(g, "batch", "v1", "Job", "myjob", "default", "batch")

	patterns := d.Detect(g)
	found := false
	for _, p := range patterns {
		if p == PatternJob {
			found = true
		}
	}
	if !found {
		t.Error("should detect job pattern when Job resource exists")
	}
}

func TestJobDetector_WithCronJob(t *testing.T) {
	d := NewJobDetector()
	g := makeGraph()
	addResource(g, "batch", "v1", "CronJob", "mycronjob", "default", "batch")

	patterns := d.Detect(g)
	found := false
	for _, p := range patterns {
		if p == PatternJob {
			found = true
		}
	}
	if !found {
		t.Error("should detect job pattern when CronJob resource exists")
	}
}

func TestJobDetector_NoJobResources(t *testing.T) {
	d := NewJobDetector()
	g := makeGraph()
	addResource(g, "apps", "v1", "Deployment", "app", "default", "app")

	patterns := d.Detect(g)
	for _, p := range patterns {
		if p == PatternJob {
			t.Error("should not detect job pattern when no Job/CronJob resources exist")
		}
	}
}

func TestJobDetector_EmptyGraph(t *testing.T) {
	d := NewJobDetector()
	g := makeGraph()

	patterns := d.Detect(g)
	if len(patterns) != 0 {
		t.Errorf("empty graph should yield no patterns; got %v", patterns)
	}
}

// ── OperatorDetector ──────────────────────────────────────────────────────────

func TestOperatorDetector_Name(t *testing.T) {
	d := NewOperatorDetector()
	if d.Name() != "operator" {
		t.Errorf("Name() = %q; want operator", d.Name())
	}
}

func TestOperatorDetector_WithCRDAndControllerName(t *testing.T) {
	d := NewOperatorDetector()
	g := makeGraph()
	addResource(g, "apiextensions.k8s.io", "v1", "CustomResourceDefinition", "foos.example.com", "", "operator")
	addResource(g, "apps", "v1", "Deployment", "foo-controller", "default", "operator")

	patterns := d.Detect(g)
	found := false
	for _, p := range patterns {
		if p == PatternOperator {
			found = true
		}
	}
	if !found {
		t.Error("should detect operator pattern with CRD + controller Deployment")
	}
}

func TestOperatorDetector_WithCRDAndOperatorName(t *testing.T) {
	d := NewOperatorDetector()
	g := makeGraph()
	addResource(g, "apiextensions.k8s.io", "v1", "CustomResourceDefinition", "bars.example.com", "", "operator")
	addResource(g, "apps", "v1", "Deployment", "my-operator", "default", "operator")

	patterns := d.Detect(g)
	found := false
	for _, p := range patterns {
		if p == PatternOperator {
			found = true
		}
	}
	if !found {
		t.Error("should detect operator pattern with CRD + operator Deployment")
	}
}

func TestOperatorDetector_WithCRDAndControlPlaneLabel(t *testing.T) {
	d := NewOperatorDetector()
	g := makeGraph()
	addResource(g, "apiextensions.k8s.io", "v1", "CustomResourceDefinition", "things.example.com", "", "operator")

	// Create a Deployment with a control-plane label
	obj := &unstructured.Unstructured{}
	obj.SetKind("Deployment")
	obj.SetName("manager")
	obj.SetNamespace("default")
	obj.SetAPIVersion("apps/v1")
	obj.SetLabels(map[string]string{"control-plane": "controller-manager"})
	pr := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		},
		ServiceName: "operator",
		Values:      make(map[string]interface{}),
	}
	g.AddResource(pr)

	patterns := d.Detect(g)
	found := false
	for _, p := range patterns {
		if p == PatternOperator {
			found = true
		}
	}
	if !found {
		t.Error("should detect operator pattern with CRD + control-plane labeled Deployment")
	}
}

func TestOperatorDetector_CRDOnlyNoController(t *testing.T) {
	d := NewOperatorDetector()
	g := makeGraph()
	addResource(g, "apiextensions.k8s.io", "v1", "CustomResourceDefinition", "foos.example.com", "", "operator")
	addResource(g, "apps", "v1", "Deployment", "myapp", "default", "app")

	patterns := d.Detect(g)
	for _, p := range patterns {
		if p == PatternOperator {
			t.Error("should not detect operator pattern with CRD but no controller Deployment")
		}
	}
}

func TestOperatorDetector_ControllerWithoutCRD(t *testing.T) {
	d := NewOperatorDetector()
	g := makeGraph()
	addResource(g, "apps", "v1", "Deployment", "foo-controller", "default", "app")

	patterns := d.Detect(g)
	for _, p := range patterns {
		if p == PatternOperator {
			t.Error("should not detect operator pattern with controller but no CRD")
		}
	}
}

func TestOperatorDetector_EmptyGraph(t *testing.T) {
	d := NewOperatorDetector()
	g := makeGraph()

	patterns := d.Detect(g)
	if len(patterns) != 0 {
		t.Errorf("empty graph should yield no patterns; got %v", patterns)
	}
}

// ── InitContainerChecker ──────────────────────────────────────────────────────

func TestInitContainerChecker_Name(t *testing.T) {
	c := NewInitContainerChecker()
	if c.Name() != "init-containers" {
		t.Errorf("Name() = %q; want init-containers", c.Name())
	}
	if c.Category() != "Patterns" {
		t.Errorf("Category() = %q; want Patterns", c.Category())
	}
}

func TestInitContainerChecker_WithInitContainers(t *testing.T) {
	c := NewInitContainerChecker()
	g := makeGraph()

	pr := addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{"name": "main"},
	})
	pr.Values["initContainers"] = []map[string]interface{}{
		{"name": "init"},
	}
	addGroup(g, "app", pr)

	practices := c.Check(g)
	found := false
	for _, p := range practices {
		if p.ID == "BP-PAT-001" && p.Compliant {
			found = true
		}
	}
	if !found {
		t.Error("should report BP-PAT-001 (compliant) when init containers are present")
	}
}

func TestInitContainerChecker_NoInitContainers(t *testing.T) {
	c := NewInitContainerChecker()
	g := makeGraph()

	pr := addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{"name": "main"},
	})
	addGroup(g, "app", pr)

	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-PAT-001" {
			t.Error("should not report BP-PAT-001 when no init containers present")
		}
	}
}

func TestInitContainerChecker_NonWorkloadSkipped(t *testing.T) {
	c := NewInitContainerChecker()
	g := makeGraph()

	// Job should be skipped
	obj := &unstructured.Unstructured{}
	obj.SetKind("Job")
	obj.SetName("myjob")
	obj.SetNamespace("default")
	obj.SetAPIVersion("batch/v1")
	pr := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"},
		},
		ServiceName: "batch",
		Values: map[string]interface{}{
			"initContainers": []map[string]interface{}{{"name": "init"}},
		},
	}
	g.AddResource(pr)

	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-PAT-001" {
			t.Error("Job is not a tracked workload kind; BP-PAT-001 should not be reported")
		}
	}
}

func TestInitContainerChecker_StatefulSetAndDaemonSet(t *testing.T) {
	c := NewInitContainerChecker()
	g := makeGraph()

	ss := addWorkloadWithContainers(g, "StatefulSet", "db", "db", nil)
	ss.Values["initContainers"] = []map[string]interface{}{{"name": "init"}}

	ds := addWorkloadWithContainers(g, "DaemonSet", "agent", "agent", nil)
	ds.Values["initContainers"] = []map[string]interface{}{{"name": "init"}}

	practices := c.Check(g)
	count := 0
	for _, p := range practices {
		if p.ID == "BP-PAT-001" {
			count++
		}
	}
	// One report covering two affected resources
	if count == 0 {
		t.Error("should report BP-PAT-001 for StatefulSet and DaemonSet with init containers")
	}
}

// ── QoSClassChecker ───────────────────────────────────────────────────────────

func TestQoSClassChecker_Name(t *testing.T) {
	c := NewQoSClassChecker()
	if c.Name() != "qos-class" {
		t.Errorf("Name() = %q; want qos-class", c.Name())
	}
	if c.Category() != "Resource Management" {
		t.Errorf("Category() = %q; want Resource Management", c.Category())
	}
}

func TestQoSClassChecker_BestEffort_NoContainers(t *testing.T) {
	c := NewQoSClassChecker()
	g := makeGraph()
	addResource(g, "apps", "v1", "Deployment", "app", "default", "app")

	practices := c.Check(g)
	found := false
	for _, p := range practices {
		if p.ID == "BP-QOS-001" && !p.Compliant {
			found = true
		}
	}
	if !found {
		t.Error("should report BP-QOS-001 for workload with no containers (BestEffort)")
	}
}

func TestQoSClassChecker_BestEffort_EmptyContainerList(t *testing.T) {
	c := NewQoSClassChecker()
	g := makeGraph()
	addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{})

	practices := c.Check(g)
	found := false
	for _, p := range practices {
		if p.ID == "BP-QOS-001" {
			found = true
		}
	}
	if !found {
		t.Error("should report BP-QOS-001 for workload with empty container list")
	}
}

func TestQoSClassChecker_BestEffort_ContainersNoResources(t *testing.T) {
	c := NewQoSClassChecker()
	g := makeGraph()
	addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{"name": "main"},
	})

	practices := c.Check(g)
	found := false
	for _, p := range practices {
		if p.ID == "BP-QOS-001" && !p.Compliant {
			found = true
		}
	}
	if !found {
		t.Error("should report BP-QOS-001 for containers with no resource requests/limits")
	}
}

func TestQoSClassChecker_Guaranteed(t *testing.T) {
	c := NewQoSClassChecker()
	g := makeGraph()
	addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{
			"name": "main",
			"resources": map[string]interface{}{
				"limits":   map[string]interface{}{"cpu": "500m", "memory": "128Mi"},
				"requests": map[string]interface{}{"cpu": "500m", "memory": "128Mi"},
			},
		},
	})

	practices := c.Check(g)
	found := false
	for _, p := range practices {
		if p.ID == "BP-QOS-002" && p.Compliant {
			found = true
		}
	}
	if !found {
		t.Error("should report BP-QOS-002 for workload with equal limits and requests (Guaranteed)")
	}
}

func TestQoSClassChecker_Burstable_NotReported(t *testing.T) {
	c := NewQoSClassChecker()
	g := makeGraph()
	// Burstable: only requests set
	addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{
			"name": "main",
			"resources": map[string]interface{}{
				"requests": map[string]interface{}{"cpu": "100m", "memory": "64Mi"},
			},
		},
	})

	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-QOS-001" {
			t.Error("burstable workload should not be reported as BestEffort")
		}
		if p.ID == "BP-QOS-002" {
			t.Error("burstable workload should not be reported as Guaranteed")
		}
	}
}

func TestQoSClassChecker_ContainersWrongType(t *testing.T) {
	c := NewQoSClassChecker()
	g := makeGraph()

	obj := &unstructured.Unstructured{}
	obj.SetKind("Deployment")
	obj.SetName("app")
	obj.SetNamespace("default")
	obj.SetAPIVersion("apps/v1")
	pr := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		},
		ServiceName: "app",
		Values:      map[string]interface{}{"containers": "wrong-type"},
	}
	g.AddResource(pr)

	// Wrong type containers should be skipped without panicking
	practices := c.Check(g)
	_ = practices
}

func TestQoSClassChecker_GuaranteedMissingMemory(t *testing.T) {
	c := NewQoSClassChecker()
	g := makeGraph()
	// limits and requests both set but missing memory in limits → not guaranteed
	addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{
			"name": "main",
			"resources": map[string]interface{}{
				"limits":   map[string]interface{}{"cpu": "500m"},
				"requests": map[string]interface{}{"cpu": "500m", "memory": "128Mi"},
			},
		},
	})

	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-QOS-002" {
			t.Error("should not report Guaranteed QoS when memory is missing from limits")
		}
	}
}

// ── StatefulSetPatternChecker ─────────────────────────────────────────────────

func TestStatefulSetPatternChecker_Name(t *testing.T) {
	c := NewStatefulSetPatternChecker()
	if c.Name() != "statefulset-patterns" {
		t.Errorf("Name() = %q; want statefulset-patterns", c.Name())
	}
	if c.Category() != "Patterns" {
		t.Errorf("Category() = %q; want Patterns", c.Category())
	}
}

func TestStatefulSetPatternChecker_MissingAll(t *testing.T) {
	c := NewStatefulSetPatternChecker()
	g := makeGraph()
	addResource(g, "apps", "v1", "StatefulSet", "db", "default", "db")

	practices := c.Check(g)
	found := false
	for _, p := range practices {
		if p.ID == "BP-SS-001" && !p.Compliant {
			found = true
		}
	}
	if !found {
		t.Error("should report BP-SS-001 for StatefulSet missing serviceName/podManagementPolicy/updateStrategy")
	}
}

func TestStatefulSetPatternChecker_FullyConfigured(t *testing.T) {
	c := NewStatefulSetPatternChecker()
	g := makeGraph()

	pr := addResource(g, "apps", "v1", "StatefulSet", "db", "default", "db")
	pr.Values["serviceName"] = "db-headless"
	pr.Values["podManagementPolicy"] = "Parallel"
	pr.Values["updateStrategy"] = map[string]interface{}{"type": "RollingUpdate"}

	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-SS-001" {
			t.Error("should not report BP-SS-001 for fully configured StatefulSet")
		}
	}
}

func TestStatefulSetPatternChecker_MissingServiceNameOnly(t *testing.T) {
	c := NewStatefulSetPatternChecker()
	g := makeGraph()

	pr := addResource(g, "apps", "v1", "StatefulSet", "db", "default", "db")
	// serviceName set to empty string — counts as missing
	pr.Values["serviceName"] = ""
	pr.Values["podManagementPolicy"] = "OrderedReady"
	pr.Values["updateStrategy"] = map[string]interface{}{"type": "RollingUpdate"}

	practices := c.Check(g)
	found := false
	for _, p := range practices {
		if p.ID == "BP-SS-001" {
			found = true
		}
	}
	if !found {
		t.Error("should report BP-SS-001 when serviceName is empty string")
	}
}

func TestStatefulSetPatternChecker_NonStatefulSetSkipped(t *testing.T) {
	c := NewStatefulSetPatternChecker()
	g := makeGraph()
	addResource(g, "apps", "v1", "Deployment", "app", "default", "app")

	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-SS-001" {
			t.Error("Deployment should not be checked by StatefulSetPatternChecker")
		}
	}
}

// ── DaemonSetPatternChecker ───────────────────────────────────────────────────

func TestDaemonSetPatternChecker_Name(t *testing.T) {
	c := NewDaemonSetPatternChecker()
	if c.Name() != "daemonset-patterns" {
		t.Errorf("Name() = %q; want daemonset-patterns", c.Name())
	}
	if c.Category() != "Patterns" {
		t.Errorf("Category() = %q; want Patterns", c.Category())
	}
}

func TestDaemonSetPatternChecker_MissingAll(t *testing.T) {
	c := NewDaemonSetPatternChecker()
	g := makeGraph()
	addResource(g, "apps", "v1", "DaemonSet", "agent", "default", "agent")

	practices := c.Check(g)
	found := false
	for _, p := range practices {
		if p.ID == "BP-DS-001" && !p.Compliant {
			found = true
		}
	}
	if !found {
		t.Error("should report BP-DS-001 for DaemonSet missing tolerations/updateStrategy/limits")
	}
}

func TestDaemonSetPatternChecker_FullyConfigured(t *testing.T) {
	c := NewDaemonSetPatternChecker()
	g := makeGraph()

	pr := addWorkloadWithContainers(g, "DaemonSet", "agent", "agent", []map[string]interface{}{
		{
			"name": "agent",
			"resources": map[string]interface{}{
				"limits": map[string]interface{}{"cpu": "200m", "memory": "64Mi"},
			},
		},
	})
	pr.Values["tolerations"] = []interface{}{map[string]interface{}{"operator": "Exists"}}
	pr.Values["updateStrategy"] = map[string]interface{}{"type": "RollingUpdate"}

	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-DS-001" {
			t.Error("should not report BP-DS-001 for fully configured DaemonSet")
		}
	}
}

func TestDaemonSetPatternChecker_ContainersNoLimits(t *testing.T) {
	c := NewDaemonSetPatternChecker()
	g := makeGraph()

	pr := addWorkloadWithContainers(g, "DaemonSet", "agent", "agent", []map[string]interface{}{
		{"name": "agent"},
	})
	pr.Values["tolerations"] = []interface{}{map[string]interface{}{"operator": "Exists"}}
	pr.Values["updateStrategy"] = map[string]interface{}{"type": "RollingUpdate"}

	practices := c.Check(g)
	found := false
	for _, p := range practices {
		if p.ID == "BP-DS-001" {
			found = true
		}
	}
	if !found {
		t.Error("should report BP-DS-001 when DaemonSet containers have no limits")
	}
}

func TestDaemonSetPatternChecker_ContainersWrongType(t *testing.T) {
	c := NewDaemonSetPatternChecker()
	g := makeGraph()

	obj := &unstructured.Unstructured{}
	obj.SetKind("DaemonSet")
	obj.SetName("agent")
	obj.SetNamespace("default")
	obj.SetAPIVersion("apps/v1")
	pr := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"},
		},
		ServiceName: "agent",
		Values: map[string]interface{}{
			"containers":     "wrong-type",
			"tolerations":    []interface{}{},
			"updateStrategy": map[string]interface{}{"type": "RollingUpdate"},
		},
	}
	g.AddResource(pr)

	// Wrong-type containers: type assertion fails silently — neither nil-branch nor
	// typed-branch executes, so "resource limits not set" is NOT added to missingItems.
	// tolerations and updateStrategy are both set, so no other items missing either.
	// Result: no BP-DS-001 reported. Should not panic.
	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-DS-001" {
			t.Error("wrong-type containers with tolerations+updateStrategy set should not trigger BP-DS-001")
		}
	}
}

func TestDaemonSetPatternChecker_NonDaemonSetSkipped(t *testing.T) {
	c := NewDaemonSetPatternChecker()
	g := makeGraph()
	addResource(g, "apps", "v1", "Deployment", "app", "default", "app")

	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-DS-001" {
			t.Error("Deployment should not be checked by DaemonSetPatternChecker")
		}
	}
}

// ── GracefulShutdownChecker ───────────────────────────────────────────────────

func TestGracefulShutdownChecker_Name(t *testing.T) {
	c := NewGracefulShutdownChecker()
	if c.Name() != "graceful-shutdown" {
		t.Errorf("Name() = %q; want graceful-shutdown", c.Name())
	}
	if c.Category() != "Reliability" {
		t.Errorf("Category() = %q; want Reliability", c.Category())
	}
}

func TestGracefulShutdownChecker_NeitherConfigured(t *testing.T) {
	c := NewGracefulShutdownChecker()
	g := makeGraph()
	addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{"name": "main"},
	})

	practices := c.Check(g)
	found := false
	for _, p := range practices {
		if p.ID == "BP-GS-001" && !p.Compliant {
			found = true
		}
	}
	if !found {
		t.Error("should report BP-GS-001 when neither preStop nor terminationGracePeriodSeconds is configured")
	}
}

func TestGracefulShutdownChecker_WithTerminationGracePeriod(t *testing.T) {
	c := NewGracefulShutdownChecker()
	g := makeGraph()

	pr := addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{"name": "main"},
	})
	pr.Values["terminationGracePeriodSeconds"] = int64(30)

	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-GS-001" {
			t.Error("should not report BP-GS-001 when terminationGracePeriodSeconds is set")
		}
	}
}

func TestGracefulShutdownChecker_WithPreStopHook(t *testing.T) {
	c := NewGracefulShutdownChecker()
	g := makeGraph()

	addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{
			"name": "main",
			"lifecycle": map[string]interface{}{
				"preStop": map[string]interface{}{
					"exec": map[string]interface{}{"command": []string{"/bin/sh", "-c", "sleep 5"}},
				},
			},
		},
	})

	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-GS-001" {
			t.Error("should not report BP-GS-001 when preStop hook is configured")
		}
	}
}

func TestGracefulShutdownChecker_NoContainers(t *testing.T) {
	c := NewGracefulShutdownChecker()
	g := makeGraph()
	addResource(g, "apps", "v1", "Deployment", "app", "default", "app")

	practices := c.Check(g)
	found := false
	for _, p := range practices {
		if p.ID == "BP-GS-001" {
			found = true
		}
	}
	if !found {
		t.Error("should report BP-GS-001 for workload with no containers key")
	}
}

func TestGracefulShutdownChecker_ContainersWrongType(t *testing.T) {
	c := NewGracefulShutdownChecker()
	g := makeGraph()

	obj := &unstructured.Unstructured{}
	obj.SetKind("Deployment")
	obj.SetName("app")
	obj.SetNamespace("default")
	obj.SetAPIVersion("apps/v1")
	pr := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		},
		ServiceName: "app",
		Values:      map[string]interface{}{"containers": "wrong-type"},
	}
	g.AddResource(pr)

	// Should report missing graceful shutdown (containers parse fails, no preStop found)
	practices := c.Check(g)
	found := false
	for _, p := range practices {
		if p.ID == "BP-GS-001" {
			found = true
		}
	}
	if !found {
		t.Error("should report BP-GS-001 when containers has wrong type and no terminationGracePeriodSeconds")
	}
}

func TestGracefulShutdownChecker_NonWorkloadSkipped(t *testing.T) {
	c := NewGracefulShutdownChecker()
	g := makeGraph()
	addResource(g, "batch", "v1", "Job", "myjob", "default", "batch")

	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-GS-001" {
			t.Error("Job should not be checked by GracefulShutdownChecker")
		}
	}
}

func TestGracefulShutdownChecker_LifecycleNoPreStop(t *testing.T) {
	c := NewGracefulShutdownChecker()
	g := makeGraph()

	// Container with lifecycle but no preStop key
	addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{
			"name": "main",
			"lifecycle": map[string]interface{}{
				"postStart": map[string]interface{}{"exec": map[string]interface{}{"command": []string{"echo"}}},
			},
		},
	})

	practices := c.Check(g)
	found := false
	for _, p := range practices {
		if p.ID == "BP-GS-001" {
			found = true
		}
	}
	if !found {
		t.Error("should report BP-GS-001 when lifecycle has no preStop key")
	}
}

// ── PodSecurityStandardsChecker ───────────────────────────────────────────────

func TestPodSecurityStandardsChecker_Name(t *testing.T) {
	c := NewPodSecurityStandardsChecker()
	if c.Name() != "pod-security-standards" {
		t.Errorf("Name() = %q; want pod-security-standards", c.Name())
	}
	if c.Category() != "Security" {
		t.Errorf("Category() = %q; want Security", c.Category())
	}
}

func TestPodSecurityStandardsChecker_PrivilegedContainer(t *testing.T) {
	c := NewPodSecurityStandardsChecker()
	g := makeGraph()

	addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{
			"name": "main",
			"securityContext": map[string]interface{}{
				"privileged": true,
			},
		},
	})

	practices := c.Check(g)
	found := false
	for _, p := range practices {
		if p.ID == "BP-PSS-001" && !p.Compliant {
			found = true
		}
	}
	if !found {
		t.Error("should report BP-PSS-001 for privileged container (privileged PSS level)")
	}
}

func TestPodSecurityStandardsChecker_HostNetwork(t *testing.T) {
	c := NewPodSecurityStandardsChecker()
	g := makeGraph()

	pr := addWorkloadWithContainers(g, "Deployment", "app", "app", nil)
	pr.Values["hostNetwork"] = true

	practices := c.Check(g)
	found := false
	for _, p := range practices {
		if p.ID == "BP-PSS-001" && !p.Compliant {
			found = true
		}
	}
	if !found {
		t.Error("should report BP-PSS-001 for hostNetwork:true (privileged PSS level)")
	}
}

func TestPodSecurityStandardsChecker_HostPID(t *testing.T) {
	c := NewPodSecurityStandardsChecker()
	g := makeGraph()

	pr := addWorkloadWithContainers(g, "Deployment", "app", "app", nil)
	pr.Values["hostPID"] = true

	practices := c.Check(g)
	found := false
	for _, p := range practices {
		if p.ID == "BP-PSS-001" {
			found = true
		}
	}
	if !found {
		t.Error("should report BP-PSS-001 for hostPID:true")
	}
}

func TestPodSecurityStandardsChecker_HostIPC(t *testing.T) {
	c := NewPodSecurityStandardsChecker()
	g := makeGraph()

	pr := addWorkloadWithContainers(g, "Deployment", "app", "app", nil)
	pr.Values["hostIPC"] = true

	practices := c.Check(g)
	found := false
	for _, p := range practices {
		if p.ID == "BP-PSS-001" {
			found = true
		}
	}
	if !found {
		t.Error("should report BP-PSS-001 for hostIPC:true")
	}
}

func TestPodSecurityStandardsChecker_RestrictedCompliant(t *testing.T) {
	c := NewPodSecurityStandardsChecker()
	g := makeGraph()

	addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{
			"name": "main",
			"securityContext": map[string]interface{}{
				"runAsNonRoot": true,
				"capabilities": map[string]interface{}{
					"drop": []interface{}{"ALL"},
				},
				"seccompProfile": map[string]interface{}{"type": "RuntimeDefault"},
			},
		},
	})

	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-PSS-001" {
			t.Error("should not report BP-PSS-001 for restricted-compliant workload")
		}
	}
}

func TestPodSecurityStandardsChecker_BaselineLevel_NoSecCtx(t *testing.T) {
	c := NewPodSecurityStandardsChecker()
	g := makeGraph()

	// Container with no securityContext — baseline level, not privileged
	addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{"name": "main"},
	})

	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-PSS-001" {
			t.Error("baseline-level workload (no securityContext) should not be reported as privileged")
		}
	}
}

func TestPodSecurityStandardsChecker_NoContainersList(t *testing.T) {
	c := NewPodSecurityStandardsChecker()
	g := makeGraph()

	// Workload with no containers key — treated as baseline
	addResource(g, "apps", "v1", "Deployment", "app", "default", "app")

	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-PSS-001" {
			t.Error("workload with no containers key should be treated as baseline, not privileged")
		}
	}
}

func TestPodSecurityStandardsChecker_ContainersWrongType(t *testing.T) {
	c := NewPodSecurityStandardsChecker()
	g := makeGraph()

	obj := &unstructured.Unstructured{}
	obj.SetKind("Deployment")
	obj.SetName("app")
	obj.SetNamespace("default")
	obj.SetAPIVersion("apps/v1")
	pr := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		},
		ServiceName: "app",
		Values:      map[string]interface{}{"containers": "wrong-type"},
	}
	g.AddResource(pr)

	// Wrong type containers → classifyPSSLevel returns baseline → no BP-PSS-001
	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-PSS-001" {
			t.Error("wrong-type containers should not trigger BP-PSS-001")
		}
	}
}

func TestPodSecurityStandardsChecker_RunAsNonRootFalse(t *testing.T) {
	c := NewPodSecurityStandardsChecker()
	g := makeGraph()

	addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{
			"name": "main",
			"securityContext": map[string]interface{}{
				"runAsNonRoot": false,
				// drop ALL and seccompProfile present — but runAsNonRoot is false
				"capabilities": map[string]interface{}{
					"drop": []interface{}{"ALL"},
				},
				"seccompProfile": map[string]interface{}{"type": "RuntimeDefault"},
			},
		},
	})

	// runAsNonRoot:false → baseline level → not privileged → no BP-PSS-001
	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-PSS-001" {
			t.Error("runAsNonRoot:false makes it baseline, not privileged — should not report BP-PSS-001")
		}
	}
}

func TestPodSecurityStandardsChecker_DropCapabilitiesNotAll(t *testing.T) {
	c := NewPodSecurityStandardsChecker()
	g := makeGraph()

	addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{
			"name": "main",
			"securityContext": map[string]interface{}{
				"runAsNonRoot": true,
				"capabilities": map[string]interface{}{
					"drop": []interface{}{"NET_ADMIN"}, // not ALL
				},
				"seccompProfile": map[string]interface{}{"type": "RuntimeDefault"},
			},
		},
	})

	// Does not drop ALL → baseline → no BP-PSS-001
	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-PSS-001" {
			t.Error("not dropping ALL capabilities makes it baseline, not privileged")
		}
	}
}

func TestPodSecurityStandardsChecker_NoSeccompProfile(t *testing.T) {
	c := NewPodSecurityStandardsChecker()
	g := makeGraph()

	addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{
			"name": "main",
			"securityContext": map[string]interface{}{
				"runAsNonRoot": true,
				"capabilities": map[string]interface{}{
					"drop": []interface{}{"ALL"},
				},
				// seccompProfile missing → not restricted
			},
		},
	})

	// No seccompProfile → baseline → no BP-PSS-001
	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-PSS-001" {
			t.Error("missing seccompProfile makes it baseline, not privileged")
		}
	}
}

func TestPodSecurityStandardsChecker_NilCapabilities(t *testing.T) {
	c := NewPodSecurityStandardsChecker()
	g := makeGraph()

	addWorkloadWithContainers(g, "Deployment", "app", "app", []map[string]interface{}{
		{
			"name": "main",
			"securityContext": map[string]interface{}{
				"runAsNonRoot": true,
				"capabilities": nil,
				"seccompProfile": map[string]interface{}{"type": "RuntimeDefault"},
			},
		},
	})

	// nil capabilities → baseline → no BP-PSS-001
	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-PSS-001" {
			t.Error("nil capabilities makes it baseline, not privileged")
		}
	}
}

func TestPodSecurityStandardsChecker_NonWorkloadSkipped(t *testing.T) {
	c := NewPodSecurityStandardsChecker()
	g := makeGraph()
	addResource(g, "batch", "v1", "Job", "myjob", "default", "batch")

	practices := c.Check(g)
	for _, p := range practices {
		if p.ID == "BP-PSS-001" {
			t.Error("Job should not be checked by PodSecurityStandardsChecker")
		}
	}
}
