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
	if len(a.detectors) != 3 {
		t.Errorf("DefaultAnalyzer detectors = %d; want 3", len(a.detectors))
	}
	if len(a.checkers) != 3 {
		t.Errorf("DefaultAnalyzer checkers = %d; want 3", len(a.checkers))
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

// ── Strategy helper functions ────────────────────────────────────────────────

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
