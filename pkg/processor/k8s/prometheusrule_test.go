package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Helper: create PrometheusRule unstructured object
// ============================================================

func makePrometheusRuleObj(name, namespace string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "monitoring.coreos.com/v1",
			"kind":       "PrometheusRule",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": spec,
		},
	}
}

// ============================================================
// Test 1: Processor name
// ============================================================

func TestPrometheusRuleProcessor_Name(t *testing.T) {
	proc := NewPrometheusRuleProcessor()
	testutil.AssertEqual(t, "prometheusrule", proc.Name(), "processor name")
}

// ============================================================
// Test 2: Supports GVK
// ============================================================

func TestPrometheusRuleProcessor_Supports(t *testing.T) {
	proc := NewPrometheusRuleProcessor()
	gvks := proc.Supports()

	if len(gvks) != 1 {
		t.Fatalf("Expected 1 supported GVK, got %d", len(gvks))
	}

	expected := schema.GroupVersionKind{
		Group:   "monitoring.coreos.com",
		Version: "v1",
		Kind:    "PrometheusRule",
	}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

// ============================================================
// Test 3: Single group extraction
// ============================================================

func TestPrometheusRuleProcessor_SingleGroup(t *testing.T) {
	proc := NewPrometheusRuleProcessor()
	ctx := newTestProcessorContext()

	obj := makePrometheusRuleObj("myapp-alerts", "monitoring", map[string]interface{}{
		"groups": []interface{}{
			map[string]interface{}{
				"name": "myapp.rules",
				"rules": []interface{}{
					map[string]interface{}{
						"alert": "HighErrorRate",
						"expr":  "rate(http_errors_total[5m]) > 0.5",
						"for":   "5m",
						"labels": map[string]interface{}{
							"severity": "critical",
						},
						"annotations": map[string]interface{}{
							"summary": "High error rate detected",
						},
					},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")

	groups, ok := result.Values["groups"].([]interface{})
	if !ok {
		t.Fatal("Expected groups slice in values")
	}
	if len(groups) != 1 {
		t.Fatalf("Expected 1 group, got %d", len(groups))
	}
}

// ============================================================
// Test 4: Multiple groups
// ============================================================

func TestPrometheusRuleProcessor_MultiGroup(t *testing.T) {
	proc := NewPrometheusRuleProcessor()
	ctx := newTestProcessorContext()

	obj := makePrometheusRuleObj("myapp-alerts", "monitoring", map[string]interface{}{
		"groups": []interface{}{
			map[string]interface{}{
				"name": "myapp.alerts",
				"rules": []interface{}{
					map[string]interface{}{
						"alert": "HighErrorRate",
						"expr":  "rate(http_errors_total[5m]) > 0.5",
					},
				},
			},
			map[string]interface{}{
				"name": "myapp.recording",
				"rules": []interface{}{
					map[string]interface{}{
						"record": "myapp:http_requests:rate5m",
						"expr":   "rate(http_requests_total[5m])",
					},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	groups, ok := result.Values["groups"].([]interface{})
	if !ok {
		t.Fatal("Expected groups slice in values")
	}
	if len(groups) != 2 {
		t.Fatalf("Expected 2 groups, got %d", len(groups))
	}
}

// ============================================================
// Test 5: Alert rule fields (alert, expr, for, labels, annotations)
// ============================================================

func TestPrometheusRuleProcessor_AlertRule_Fields(t *testing.T) {
	proc := NewPrometheusRuleProcessor()
	ctx := newTestProcessorContext()

	obj := makePrometheusRuleObj("myapp-alerts", "monitoring", map[string]interface{}{
		"groups": []interface{}{
			map[string]interface{}{
				"name": "myapp.rules",
				"rules": []interface{}{
					map[string]interface{}{
						"alert": "PodCrashLooping",
						"expr":  `kube_pod_container_status_restarts_total > 3`,
						"for":   "10m",
						"labels": map[string]interface{}{
							"severity": "warning",
							"team":     "platform",
						},
						"annotations": map[string]interface{}{
							"summary":     "Pod {{ $labels.pod }} is crash-looping",
							"description": "Pod has restarted more than 3 times",
						},
					},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	groups := result.Values["groups"].([]interface{})
	group := groups[0].(map[string]interface{})
	rules := group["rules"].([]interface{})
	rule := rules[0].(map[string]interface{})

	testutil.AssertEqual(t, "PodCrashLooping", rule["alert"], "alert name")
	testutil.AssertEqual(t, "kube_pod_container_status_restarts_total > 3", rule["expr"], "expr")
	testutil.AssertEqual(t, "10m", rule["for"], "for duration")

	labels := rule["labels"].(map[string]interface{})
	testutil.AssertEqual(t, "warning", labels["severity"], "severity label")

	annotations := rule["annotations"].(map[string]interface{})
	if _, ok := annotations["summary"]; !ok {
		t.Error("Expected summary annotation")
	}
}

// ============================================================
// Test 6: Recording rule
// ============================================================

func TestPrometheusRuleProcessor_RecordRule(t *testing.T) {
	proc := NewPrometheusRuleProcessor()
	ctx := newTestProcessorContext()

	obj := makePrometheusRuleObj("myapp-recording", "monitoring", map[string]interface{}{
		"groups": []interface{}{
			map[string]interface{}{
				"name": "myapp.recording",
				"rules": []interface{}{
					map[string]interface{}{
						"record": "myapp:http_requests:rate5m",
						"expr":   "rate(http_requests_total[5m])",
						"labels": map[string]interface{}{
							"source": "prometheus",
						},
					},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	groups := result.Values["groups"].([]interface{})
	group := groups[0].(map[string]interface{})
	rules := group["rules"].([]interface{})
	rule := rules[0].(map[string]interface{})

	testutil.AssertEqual(t, "myapp:http_requests:rate5m", rule["record"], "record name")
	if _, hasAlert := rule["alert"]; hasAlert {
		t.Error("Recording rule should not have 'alert' field")
	}
}

// ============================================================
// Test 7: Expression templating (threshold extraction)
// ============================================================

func TestPrometheusRuleProcessor_ExprTemplating(t *testing.T) {
	proc := NewPrometheusRuleProcessor()
	ctx := newTestProcessorContext()

	obj := makePrometheusRuleObj("myapp-alerts", "monitoring", map[string]interface{}{
		"groups": []interface{}{
			map[string]interface{}{
				"name": "myapp.rules",
				"rules": []interface{}{
					map[string]interface{}{
						"alert": "HighLatency",
						"expr":  "histogram_quantile(0.99, rate(http_duration_seconds_bucket[5m])) > 1.0",
						"for":   "5m",
					},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	// Verify the expression is preserved in values
	groups := result.Values["groups"].([]interface{})
	group := groups[0].(map[string]interface{})
	rules := group["rules"].([]interface{})
	rule := rules[0].(map[string]interface{})
	testutil.AssertEqual(t, "histogram_quantile(0.99, rate(http_duration_seconds_bucket[5m])) > 1.0", rule["expr"], "expr should be preserved")
}

// ============================================================
// Test 8: Template content
// ============================================================

func TestPrometheusRuleProcessor_Template(t *testing.T) {
	proc := NewPrometheusRuleProcessor()
	ctx := newTestProcessorContext()

	obj := makePrometheusRuleObj("myapp-alerts", "monitoring", map[string]interface{}{
		"groups": []interface{}{
			map[string]interface{}{
				"name": "myapp.rules",
				"rules": []interface{}{
					map[string]interface{}{
						"alert": "Test",
						"expr":  "up == 0",
					},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tpl := result.TemplateContent
	testutil.AssertContains(t, tpl, "apiVersion: monitoring.coreos.com/v1", "apiVersion")
	testutil.AssertContains(t, tpl, "kind: PrometheusRule", "kind")
	testutil.AssertContains(t, tpl, "$svc.enabled", "enabled check")
	if !strings.Contains(tpl, "groups") {
		t.Error("Template should reference groups")
	}
}
