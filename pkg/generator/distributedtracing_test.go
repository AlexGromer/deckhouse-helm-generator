package generator

// ============================================================
// Test Plan: W3C Distributed Tracing Config Generator (Task 5.9.5)
// ============================================================
//
// | #  | Test Name                                              | Category | Input                                              | Expected Output                                                              |
// |----|--------------------------------------------------------|----------|----------------------------------------------------|------------------------------------------------------------------------------|
// |  1 | TestGenerateTracingConfig_JaegerBackend                | happy    | Backend="jaeger", CollectorEndpoint set            | Templates contain jaeger-related config and endpoint                         |
// |  2 | TestGenerateTracingConfig_TempoBackend                 | happy    | Backend="tempo", CollectorEndpoint set             | Templates contain tempo-related config and endpoint                          |
// |  3 | TestGenerateTracingConfig_CollectorEndpointInTemplate  | happy    | CollectorEndpoint="http://jaeger:14268/api/traces" | Templates contain the exact collector endpoint                               |
// |  4 | TestGenerateTracingConfig_SamplingRateInConfig         | happy    | SamplingRate=0.05                                  | Templates contain "0.05"                                                     |
// |  5 | TestGenerateTracingConfig_W3CPropagation               | happy    | ContextPropagation=["traceparent","tracestate"]    | Templates contain "traceparent" and "tracestate"                             |
// |  6 | TestGenerateTracingConfig_B3Propagation                | happy    | ContextPropagation=["b3"]                          | Templates contain "b3"                                                       |
// |  7 | TestGenerateTracingConfig_BothPropagators              | happy    | ContextPropagation=["traceparent","b3"]            | Templates contain both "traceparent" and "b3"                                |
// |  8 | TestGenerateTracingConfig_EmptyBackendDefaultsJaeger   | edge     | Backend=""                                         | Templates default to jaeger config (non-empty result)                        |
// |  9 | TestGenerateTracingConfig_NilContextPropagation        | edge     | ContextPropagation=nil                             | Does not panic, returns non-nil TracingResult                                |
// | 10 | TestGenerateTracingConfig_NOTESTxt                     | happy    | valid opts                                         | NOTESTxt is non-empty and mentions tracing                                   |

import (
	"strings"
	"testing"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// defaultTracingOpts returns a baseline TracingOptions for reuse in tests.
func defaultTracingOpts() TracingOptions {
	return TracingOptions{
		Backend:            "jaeger",
		CollectorEndpoint:  "http://jaeger-collector.tracing:14268/api/traces",
		SamplingRate:       1.0,
		ContextPropagation: []string{"traceparent", "tracestate"},
	}
}

// allTracingTemplateContent flattens all template values into one string for assertion convenience.
func allTracingTemplateContent(result *TracingResult) string {
	if result == nil {
		return ""
	}
	var sb strings.Builder
	for _, v := range result.Templates {
		sb.WriteString(v)
		sb.WriteString("\n")
	}
	return sb.String()
}

// ── Test 1: jaeger backend generates jaeger config ────────────────────────────

func TestGenerateTracingConfig_JaegerBackend(t *testing.T) {
	opts := TracingOptions{
		Backend:            "jaeger",
		CollectorEndpoint:  "http://jaeger-collector:14268/api/traces",
		SamplingRate:       1.0,
		ContextPropagation: []string{"traceparent"},
	}

	result := GenerateTracingConfig(opts)

	if result == nil {
		t.Fatal("expected non-nil TracingResult")
	}
	if len(result.Templates) == 0 {
		t.Fatal("expected at least one template in TracingResult")
	}
	content := allTracingTemplateContent(result)
	lc := strings.ToLower(content)
	if !strings.Contains(lc, "jaeger") {
		t.Errorf("TracingResult templates must reference 'jaeger' backend, got:\n%s", content)
	}
}

// ── Test 2: tempo backend generates tempo config ─────────────────────────────

func TestGenerateTracingConfig_TempoBackend(t *testing.T) {
	opts := TracingOptions{
		Backend:            "tempo",
		CollectorEndpoint:  "http://tempo.monitoring:4317",
		SamplingRate:       1.0,
		ContextPropagation: []string{"traceparent"},
	}

	result := GenerateTracingConfig(opts)

	if result == nil {
		t.Fatal("expected non-nil TracingResult")
	}
	if len(result.Templates) == 0 {
		t.Fatal("expected at least one template in TracingResult for tempo backend")
	}
	content := allTracingTemplateContent(result)
	lc := strings.ToLower(content)
	if !strings.Contains(lc, "tempo") {
		t.Errorf("TracingResult templates must reference 'tempo' backend, got:\n%s", content)
	}
}

// ── Test 3: collector endpoint propagated verbatim ───────────────────────────

func TestGenerateTracingConfig_CollectorEndpointInTemplate(t *testing.T) {
	endpoint := "http://jaeger-collector.tracing.svc.cluster.local:14268/api/traces"
	opts := TracingOptions{
		Backend:            "jaeger",
		CollectorEndpoint:  endpoint,
		SamplingRate:       1.0,
		ContextPropagation: []string{"traceparent"},
	}

	result := GenerateTracingConfig(opts)

	if result == nil {
		t.Fatal("expected non-nil TracingResult")
	}
	content := allTracingTemplateContent(result)
	if !strings.Contains(content, endpoint) {
		t.Errorf("TracingResult templates must contain the exact collector endpoint %q, got:\n%s", endpoint, content)
	}
}

// ── Test 4: sampling rate appears in generated config ────────────────────────

func TestGenerateTracingConfig_SamplingRateInConfig(t *testing.T) {
	opts := TracingOptions{
		Backend:            "jaeger",
		CollectorEndpoint:  "http://jaeger:14268/api/traces",
		SamplingRate:       0.05,
		ContextPropagation: []string{"traceparent"},
	}

	result := GenerateTracingConfig(opts)

	if result == nil {
		t.Fatal("expected non-nil TracingResult")
	}
	content := allTracingTemplateContent(result)
	if !strings.Contains(content, "0.05") {
		t.Errorf("TracingResult templates must contain sampling rate '0.05', got:\n%s", content)
	}
}

// ── Test 5: W3C context propagation headers ──────────────────────────────────

func TestGenerateTracingConfig_W3CPropagation(t *testing.T) {
	opts := TracingOptions{
		Backend:            "jaeger",
		CollectorEndpoint:  "http://jaeger:14268/api/traces",
		SamplingRate:       1.0,
		ContextPropagation: []string{"traceparent", "tracestate"},
	}

	result := GenerateTracingConfig(opts)

	if result == nil {
		t.Fatal("expected non-nil TracingResult")
	}
	content := allTracingTemplateContent(result)
	if !strings.Contains(content, "traceparent") {
		t.Errorf("TracingResult must contain W3C propagator 'traceparent', got:\n%s", content)
	}
	if !strings.Contains(content, "tracestate") {
		t.Errorf("TracingResult must contain W3C propagator 'tracestate', got:\n%s", content)
	}
}

// ── Test 6: B3 propagation header ────────────────────────────────────────────

func TestGenerateTracingConfig_B3Propagation(t *testing.T) {
	opts := TracingOptions{
		Backend:            "tempo",
		CollectorEndpoint:  "http://tempo:4317",
		SamplingRate:       1.0,
		ContextPropagation: []string{"b3"},
	}

	result := GenerateTracingConfig(opts)

	if result == nil {
		t.Fatal("expected non-nil TracingResult")
	}
	content := allTracingTemplateContent(result)
	if !strings.Contains(content, "b3") {
		t.Errorf("TracingResult must contain B3 propagator 'b3', got:\n%s", content)
	}
}

// ── Test 7: both W3C traceparent and B3 propagators present ──────────────────

func TestGenerateTracingConfig_BothPropagators(t *testing.T) {
	opts := TracingOptions{
		Backend:            "jaeger",
		CollectorEndpoint:  "http://jaeger:14268/api/traces",
		SamplingRate:       1.0,
		ContextPropagation: []string{"traceparent", "b3"},
	}

	result := GenerateTracingConfig(opts)

	if result == nil {
		t.Fatal("expected non-nil TracingResult")
	}
	content := allTracingTemplateContent(result)
	if !strings.Contains(content, "traceparent") {
		t.Errorf("TracingResult must contain 'traceparent', got:\n%s", content)
	}
	if !strings.Contains(content, "b3") {
		t.Errorf("TracingResult must contain 'b3', got:\n%s", content)
	}
}

// ── Test 8: empty backend defaults to jaeger ─────────────────────────────────

func TestGenerateTracingConfig_EmptyBackendDefaultsJaeger(t *testing.T) {
	opts := TracingOptions{
		Backend:            "",
		CollectorEndpoint:  "http://jaeger:14268/api/traces",
		SamplingRate:       1.0,
		ContextPropagation: []string{"traceparent"},
	}

	result := GenerateTracingConfig(opts)

	if result == nil {
		t.Fatal("expected non-nil TracingResult even with empty backend")
	}
	if len(result.Templates) == 0 {
		t.Error("expected at least one template when backend defaults to jaeger")
	}
	// The implementation must default to jaeger when backend is empty.
	content := allTracingTemplateContent(result)
	lc := strings.ToLower(content)
	if !strings.Contains(lc, "jaeger") {
		t.Errorf("empty backend should default to jaeger config, got:\n%s", content)
	}
}

// ── Test 9: nil ContextPropagation does not panic ────────────────────────────

func TestGenerateTracingConfig_NilContextPropagation(t *testing.T) {
	opts := TracingOptions{
		Backend:            "jaeger",
		CollectorEndpoint:  "http://jaeger:14268/api/traces",
		SamplingRate:       1.0,
		ContextPropagation: nil,
	}

	// Must not panic.
	result := GenerateTracingConfig(opts)

	if result == nil {
		t.Fatal("expected non-nil TracingResult even with nil ContextPropagation")
	}
}

// ── Test 10: NOTESTxt non-empty and mentions tracing ─────────────────────────

func TestGenerateTracingConfig_NOTESTxt(t *testing.T) {
	opts := defaultTracingOpts()

	result := GenerateTracingConfig(opts)

	if result == nil {
		t.Fatal("expected non-nil TracingResult")
	}
	if result.NOTESTxt == "" {
		t.Error("expected NOTESTxt to be non-empty")
	}
	lower := strings.ToLower(result.NOTESTxt)
	if !strings.Contains(lower, "trac") && !strings.Contains(lower, "jaeger") && !strings.Contains(lower, "tempo") {
		t.Errorf("NOTESTxt must mention tracing/jaeger/tempo context, got: %s", result.NOTESTxt)
	}
}
