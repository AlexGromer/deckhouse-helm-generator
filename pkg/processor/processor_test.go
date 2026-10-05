package processor

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func makeObj(kind, name, namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetKind(kind)
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetAPIVersion("v1")
	return obj
}

// stubProcessor implements Processor for testing.
type stubProcessor struct {
	BaseProcessor
	processFunc func(Context, *unstructured.Unstructured) (*Result, error)
}

func (s *stubProcessor) Process(ctx Context, obj *unstructured.Unstructured) (*Result, error) {
	if s.processFunc != nil {
		return s.processFunc(ctx, obj)
	}
	return &Result{Processed: true, ServiceName: "stub"}, nil
}

func newStub(name string, priority int, gvks ...schema.GroupVersionKind) *stubProcessor {
	return &stubProcessor{
		BaseProcessor: NewBaseProcessor(name, priority, gvks...),
	}
}

// ── BaseProcessor ────────────────────────────────────────────────────────────

func TestBaseProcessor_Name(t *testing.T) {
	bp := NewBaseProcessor("test-proc", 10)
	if bp.Name() != "test-proc" {
		t.Errorf("Name() = %q; want %q", bp.Name(), "test-proc")
	}
}

func TestBaseProcessor_Priority(t *testing.T) {
	bp := NewBaseProcessor("p", 42)
	if bp.Priority() != 42 {
		t.Errorf("Priority() = %d; want 42", bp.Priority())
	}
}

func TestBaseProcessor_Supports(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	bp := NewBaseProcessor("p", 1, gvk)
	if len(bp.Supports()) != 1 || bp.Supports()[0] != gvk {
		t.Errorf("Supports() = %v; want [%v]", bp.Supports(), gvk)
	}
}

func TestBaseProcessor_Supports_Empty(t *testing.T) {
	bp := NewBaseProcessor("empty", 0)
	if len(bp.Supports()) != 0 {
		t.Error("expected empty Supports slice")
	}
}

// ── ServiceNameFromLabels ────────────────────────────────────────────────────

func TestServiceNameFromLabels_AppKubernetesName(t *testing.T) {
	obj := makeObj("Deployment", "d", "default")
	obj.SetLabels(map[string]string{"app.kubernetes.io/name": "nginx"})
	if got := ServiceNameFromLabels(obj); got != "nginx" {
		t.Errorf("got %q; want nginx", got)
	}
}

func TestServiceNameFromLabels_FallbackApp(t *testing.T) {
	obj := makeObj("Deployment", "d", "default")
	obj.SetLabels(map[string]string{"app": "redis"})
	if got := ServiceNameFromLabels(obj); got != "redis" {
		t.Errorf("got %q; want redis", got)
	}
}

func TestServiceNameFromLabels_NoLabels(t *testing.T) {
	obj := makeObj("Deployment", "d", "default")
	if got := ServiceNameFromLabels(obj); got != "" {
		t.Errorf("got %q; want empty", got)
	}
}

func TestServiceNameFromLabels_EmptyLabel(t *testing.T) {
	obj := makeObj("Deployment", "d", "default")
	obj.SetLabels(map[string]string{"app.kubernetes.io/name": "", "app": "fallback"})
	if got := ServiceNameFromLabels(obj); got != "fallback" {
		t.Errorf("got %q; want fallback", got)
	}
}

// ── ServiceNameFromResource ──────────────────────────────────────────────────

func TestServiceNameFromResource_WithLabel(t *testing.T) {
	obj := makeObj("Service", "svc", "default")
	obj.SetLabels(map[string]string{"app.kubernetes.io/name": "from-label"})
	if got := ServiceNameFromResource(obj); got != "from-label" {
		t.Errorf("got %q; want from-label", got)
	}
}

func TestServiceNameFromResource_FallbackToName(t *testing.T) {
	obj := makeObj("ConfigMap", "my-config", "default")
	if got := ServiceNameFromResource(obj); got != "my-config" {
		t.Errorf("got %q; want my-config", got)
	}
}

// ── SanitizeServiceName ──────────────────────────────────────────────────────

func TestSanitizeServiceName(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"test-module", "testModule"},
		{"my.app", "myApp"},
		{"already", "already"},
		{"UpperCase", "upperCase"},
		{"a-b-c", "aBC"},
		{"", ""},
		{"---", "---"}, // all separators → original returned
	}
	for _, tc := range tests {
		if got := SanitizeServiceName(tc.input); got != tc.want {
			t.Errorf("SanitizeServiceName(%q) = %q; want %q", tc.input, got, tc.want)
		}
	}
}

// ── ValuesPathForKind ────────────────────────────────────────────────────────

func TestValuesPathForKind_WithService(t *testing.T) {
	got := ValuesPathForKind("Deployment", "web")
	if got != "services.web.deployment" {
		t.Errorf("got %q; want services.web.deployment", got)
	}
}

func TestValuesPathForKind_WithoutService(t *testing.T) {
	got := ValuesPathForKind("Deployment", "")
	if got != "deployment" {
		t.Errorf("got %q; want deployment", got)
	}
}

func TestValuesPathForKind_UnmappedKind(t *testing.T) {
	got := ValuesPathForKind("CustomResource", "svc")
	if got != "services.svc.customResource" {
		t.Errorf("got %q; want services.svc.customResource", got)
	}
}

// ── TemplatePathForResource ──────────────────────────────────────────────────

func TestTemplatePathForResource(t *testing.T) {
	got := TemplatePathForResource("Deployment", "web-app", "default")
	if got != "templates/deployment-web-app.yaml" {
		t.Errorf("got %q; want templates/deployment-web-app.yaml", got)
	}
}

func TestTemplatePathForResource_HPA(t *testing.T) {
	got := TemplatePathForResource("HorizontalPodAutoscaler", "web-hpa", "default")
	if got != "templates/hpa-web-hpa.yaml" {
		t.Errorf("got %q; want templates/hpa-web-hpa.yaml", got)
	}
}

// ── kindToValuesKey ──────────────────────────────────────────────────────────

func TestKindToValuesKey_Known(t *testing.T) {
	tests := map[string]string{
		"Deployment":              "deployment",
		"StatefulSet":             "statefulSet",
		"Service":                 "service",
		"HorizontalPodAutoscaler": "hpa",
		"PodDisruptionBudget":     "pdb",
	}
	for kind, want := range tests {
		if got := kindToValuesKey(kind); got != want {
			t.Errorf("kindToValuesKey(%q) = %q; want %q", kind, got, want)
		}
	}
}

func TestKindToValuesKey_Unknown(t *testing.T) {
	got := kindToValuesKey("MyCustomKind")
	if got != "myCustomKind" {
		t.Errorf("got %q; want myCustomKind", got)
	}
}

func TestKindToValuesKey_Empty(t *testing.T) {
	got := kindToValuesKey("")
	if got != "" {
		t.Errorf("got %q; want empty", got)
	}
}

// ── kindToFileName ───────────────────────────────────────────────────────────

func TestKindToFileName_Known(t *testing.T) {
	if got := kindToFileName("PersistentVolumeClaim"); got != "pvc" {
		t.Errorf("got %q; want pvc", got)
	}
}

func TestKindToFileName_Unknown(t *testing.T) {
	if got := kindToFileName("MyCustomKind"); got != "mycustomkind" {
		t.Errorf("got %q; want mycustomkind", got)
	}
}

// ── stringToLower ────────────────────────────────────────────────────────────

func TestStringToLower(t *testing.T) {
	tests := []struct{ in, want string }{
		{"ABC", "abc"},
		{"abc", "abc"},
		{"MixedCase", "mixedcase"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := stringToLower(tc.in); got != tc.want {
			t.Errorf("stringToLower(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

// ── escapeTemplateString ─────────────────────────────────────────────────────

func TestEscapeTemplateString(t *testing.T) {
	tests := []struct{ in, want string }{
		{"hello", "hello"},
		{"a\\b", "a\\\\b"},
		{`a"b`, `a\"b`},
		{"a\nb", "a\\nb"},
		{"a\tb", "a\\tb"},
		{"{{value}}", `{{"{{"}}` + "value" + `{{"}}"}}`},
		{"no braces", "no braces"},
	}
	for _, tc := range tests {
		if got := escapeTemplateString(tc.in); got != tc.want {
			t.Errorf("escapeTemplateString(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

// ── Registry ─────────────────────────────────────────────────────────────────





func TestRegistry_GetProcessors(t *testing.T) {
	r := NewRegistry()
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}

	r.Register(newStub("a", 1, gvk))
	r.Register(newStub("b", 2, gvk))

	procs := r.GetProcessors(gvk)
	if len(procs) != 2 {
		t.Fatalf("got %d processors; want 2", len(procs))
	}
	// Should be sorted by priority desc
	if procs[0].Name() != "b" {
		t.Errorf("first processor should be 'b' (higher priority), got %q", procs[0].Name())
	}
}

func TestRegistry_GetProcessors_NotFound(t *testing.T) {
	r := NewRegistry()
	procs := r.GetProcessors(schema.GroupVersionKind{Kind: "Missing"})
	if procs != nil {
		t.Error("expected nil for missing GVK")
	}
}



// ── Registry.Process ─────────────────────────────────────────────────────────

func TestRegistry_Process_MatchingProcessor(t *testing.T) {
	r := NewRegistry()
	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}
	s := newStub("svc-proc", 10, gvk)
	s.processFunc = func(ctx Context, obj *unstructured.Unstructured) (*Result, error) {
		return &Result{Processed: true, ServiceName: "custom"}, nil
	}
	r.Register(s)

	obj := makeObj("Service", "my-svc", "default")
	result, err := r.Process(Context{Ctx: context.Background()}, obj)
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}
	if result.ServiceName != "custom" {
		t.Errorf("ServiceName = %q; want custom", result.ServiceName)
	}
}

func TestRegistry_Process_GenericFallback(t *testing.T) {
	r := NewRegistry()

	obj := makeObj("ConfigMap", "app-config", "default")
	obj.SetLabels(map[string]string{"app": "myapp"})

	result, err := r.Process(Context{Ctx: context.Background(), ChartName: "test"}, obj)
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}
	if !result.Processed {
		t.Error("generic processor should set Processed=true")
	}
	if result.TemplatePath == "" {
		t.Error("TemplatePath should not be empty")
	}
}

func TestRegistry_Process_ProcessorReturnsNotProcessed(t *testing.T) {
	r := NewRegistry()
	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	s := newStub("skip", 10, gvk)
	s.processFunc = func(ctx Context, obj *unstructured.Unstructured) (*Result, error) {
		return &Result{Processed: false}, nil // Didn't handle it
	}
	r.Register(s)

	obj := makeObj("ConfigMap", "cfg", "default")
	result, err := r.Process(Context{Ctx: context.Background(), ChartName: "test"}, obj)
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}
	// Should fall through to generic
	if !result.Processed {
		t.Error("generic fallback should process")
	}
}

// ── generateGenericTemplate ──────────────────────────────────────────────────

func TestGenerateGenericTemplate_HasEnabledCheck(t *testing.T) {
	obj := makeObj("ConfigMap", "cfg", "default")
	tpl, vals := generateGenericTemplate(Context{ChartName: "myapp"}, obj, "cfg")

	if !strings.Contains(tpl, "{{- if") {
		t.Error("template should contain enabled check")
	}
	if !strings.Contains(tpl, "{{- end }}") {
		t.Error("template should end with end block")
	}
	if vals["enabled"] != true {
		t.Error("values should contain enabled=true")
	}
}

func TestGenerateGenericTemplate_IncludesLabels(t *testing.T) {
	obj := makeObj("ConfigMap", "cfg", "default")
	obj.SetLabels(map[string]string{"app": "test"})
	tpl, _ := generateGenericTemplate(Context{ChartName: "chart"}, obj, "cfg")

	if !strings.Contains(tpl, `app: "test"`) {
		t.Error("template should include original labels")
	}
}

func TestGenerateGenericTemplate_IncludesAnnotations(t *testing.T) {
	obj := makeObj("ConfigMap", "cfg", "default")
	obj.SetAnnotations(map[string]string{"note": "val"})
	tpl, _ := generateGenericTemplate(Context{ChartName: "chart"}, obj, "cfg")

	if !strings.Contains(tpl, "annotations:") {
		t.Error("template should include annotations section")
	}
}

func TestGenerateGenericTemplate_WithSpec(t *testing.T) {
	obj := makeObj("Deployment", "app", "default")
	obj.Object["spec"] = map[string]interface{}{"replicas": int64(3)}
	tpl, vals := generateGenericTemplate(Context{ChartName: "chart"}, obj, "app")

	if !strings.Contains(tpl, "spec:") {
		t.Error("template should include spec section")
	}
	if vals["spec"] == nil {
		t.Error("values should include spec")
	}
}

func TestGenerateGenericTemplate_KeepsTopLevelFieldsAndDropsStatus(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion":  "example.com/v1",
		"kind":        "Widget",
		"metadata":    map[string]interface{}{"name": "w", "namespace": "default"},
		"spec":        map[string]interface{}{"size": int64(3)},
		"data":        map[string]interface{}{"greeting": "hello"},
		"rules":       []interface{}{map[string]interface{}{"match": "*"}},
		"provisioner": "ebs.csi.aws.com",
		"enabled":     false,
		"x-extra":     "v",
		"status":      map[string]interface{}{"ready": true},
	}}
	tpl, vals := generateGenericTemplate(Context{ChartName: "app"}, obj, "widget")

	for _, want := range []string{
		"data:\n  {{- toYaml .Values.services.widget.widget.data | nindent 2 }}",
		"rules:\n  {{- toYaml .Values.services.widget.widget.rules | nindent 2 }}",
		"provisioner:\n  {{- toYaml .Values.services.widget.widget.provisioner | nindent 2 }}",
		"spec:\n  {{- toYaml .Values.services.widget.widget.spec | nindent 2 }}",
		"enabled:\n  {{- toYaml .Values.services.widget.widget.enabledField | nindent 2 }}",
		"x-extra:\n  {{- toYaml (index .Values.services.widget.widget \"x-extra\") | nindent 2 }}",
		`{{- if ne (toString .Values.services.widget.widget.enabled) "false" }}`,
	} {
		if !strings.Contains(tpl, want) {
			t.Errorf("template missing %q:\n%s", want, tpl)
		}
	}
	if strings.Contains(tpl, "status") {
		t.Errorf("status must not be templated:\n%s", tpl)
	}
	if _, ok := vals["status"]; ok {
		t.Error("status must not be copied into values")
	}
	if vals["enabled"] != true || vals["enabledField"] != false || vals["provisioner"] != "ebs.csi.aws.com" {
		t.Errorf("unexpected values: %v", vals)
	}
	if vals["data"] == nil || vals["rules"] == nil || vals["x-extra"] != "v" {
		t.Errorf("unexpected values: %v", vals)
	}
	// Values are copies, not aliases of the input object.
	vals["data"].(map[string]interface{})["greeting"] = "changed"
	if obj.Object["data"].(map[string]interface{})["greeting"] != "hello" {
		t.Error("values must not alias the input object")
	}
}

func TestGenerateGenericTemplate_CRDKeepsNameAndStaticBody(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata":   map[string]interface{}{"name": "widgets.example.com"},
		"spec": map[string]interface{}{
			"group": "example.com",
			"versions": []interface{}{map[string]interface{}{
				"name":        "v1",
				"description": "uses {{ braces }}",
			}},
		},
	}}
	tpl, vals := generateGenericTemplate(Context{ChartName: "app"}, obj, "widgets")

	if !strings.Contains(tpl, "  name: widgets.example.com\n") {
		t.Errorf("CRD name must be kept verbatim:\n%s", tpl)
	}
	if strings.Contains(tpl, "fullname") {
		t.Errorf("CRD name must not get the release prefix:\n%s", tpl)
	}
	if !strings.Contains(tpl, "spec:\n  group: example.com\n") {
		t.Errorf("CRD body should be rendered verbatim:\n%s", tpl)
	}
	if !strings.Contains(tpl, `uses {{"{{"}} braces {{"}}"}}`) {
		t.Errorf("template delimiters in the CRD body must be escaped:\n%s", tpl)
	}
	if len(vals) != 1 || vals["enabled"] != true {
		t.Errorf("CRD values should only hold the enabled switch, got %v", vals)
	}
}

func TestGenerateGenericTemplate_SortsAndQuotesLabels(t *testing.T) {
	obj := makeObj("Widget", "w", "")
	obj.SetLabels(map[string]string{"b": "true", "a": "1.0", "app.kubernetes.io/name": "w"})
	tpl, _ := generateGenericTemplate(Context{ChartName: "app"}, obj, "w")
	if !strings.Contains(tpl, "    a: \"1.0\"\n    b: \"true\"\n") {
		t.Errorf("labels should be sorted and quoted:\n%s", tpl)
	}
	if strings.Contains(tpl, "app.kubernetes.io/name") {
		t.Errorf("labels set by the chart helper must not be duplicated:\n%s", tpl)
	}
}

func TestEscapeTemplateDelimiters(t *testing.T) {
	got := escapeTemplateDelimiters(`a {{ b }} "c"`)
	want := `a {{"{{"}} b {{"}}"}} "c"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRegistry_PriorityOrdering(t *testing.T) {
	r := NewRegistry()
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}

	r.Register(newStub("low", 1, gvk))
	r.Register(newStub("high", 100, gvk))
	r.Register(newStub("mid", 50, gvk))

	ps := r.GetProcessors(gvk)
	if len(ps) != 3 || ps[0].Name() != "high" || ps[1].Name() != "mid" || ps[2].Name() != "low" {
		t.Errorf("expected processors by descending priority, got %v", ps)
	}
	if got := r.GetProcessors(schema.GroupVersionKind{Kind: "Missing"}); got != nil {
		t.Errorf("expected nil for a missing GVK, got %v", got)
	}
}
