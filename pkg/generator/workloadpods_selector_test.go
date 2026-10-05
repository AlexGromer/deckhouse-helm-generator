package generator

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

func TestLabelSelectorMatches(t *testing.T) {
	labels := map[string]string{"app": "shop", "tier": "api"}
	expr := func(key, op string, values ...interface{}) map[string]interface{} {
		return map[string]interface{}{"matchExpressions": []interface{}{
			map[string]interface{}{"key": key, "operator": op, "values": values},
		}}
	}
	tests := []struct {
		name     string
		selector map[string]interface{}
		want     bool
	}{
		{"empty selector selects nothing here", map[string]interface{}{}, false},
		{"matchLabels hit", map[string]interface{}{"matchLabels": map[string]interface{}{"app": "shop"}}, true},
		{"matchLabels miss", map[string]interface{}{"matchLabels": map[string]interface{}{"app": "cart"}}, false},
		{"In hit", expr("tier", "In", "api", "web"), true},
		{"In miss", expr("tier", "In", "worker"), false},
		{"In absent key", expr("zone", "In", "a"), false},
		{"NotIn hit", expr("tier", "NotIn", "worker"), true},
		{"NotIn miss", expr("tier", "NotIn", "api"), false},
		{"Exists hit", expr("app", "Exists"), true},
		{"Exists miss", expr("zone", "Exists"), false},
		{"DoesNotExist hit", expr("zone", "DoesNotExist"), true},
		{"DoesNotExist miss", expr("app", "DoesNotExist"), false},
		{"unknown operator", expr("app", "Gt", "1"), false},
	}
	for _, tt := range tests {
		if got := labelSelectorMatches(tt.selector, labels); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestSuffixedName(t *testing.T) {
	for in, want := range map[string]string{
		"web":                          "web-pdb",
		`"web.example"`:                `"web.example-pdb"`,
		`{{ include "a.fullname" $ }}`: `{{ include "a.fullname" $ }}-pdb`,
	} {
		if got := suffixedName(in, "-pdb"); got != want {
			t.Errorf("suffixedName(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestTemplateScope(t *testing.T) {
	values := map[string]interface{}{"services": map[string]interface{}{"web": map[string]interface{}{
		"deployment": map[string]interface{}{"replicas": 2},
	}}}
	prefix := []string{"{{- $svc := .Values.services.web -}}", "{{- if $svc.enabled }}", "{{- with $svc.deployment }}"}
	if scope, ok := templateScope(prefix, values); !ok || scope["replicas"] != 2 {
		t.Errorf("scope = %v, %v", scope, ok)
	}
	for name, p := range map[string][]string{
		"no with":          {"{{- $svc := .Values.services.web -}}"},
		"unknown variable": {"{{- with $other.deployment }}"},
		"missing path":     {"{{- $svc := .Values.services.api -}}", "{{- with $svc.deployment }}"},
		"through a scalar": {"{{- $svc := .Values.services.web.deployment.replicas -}}", "{{- with $svc.x }}"},
	} {
		if _, ok := templateScope(p, values); ok {
			t.Errorf("%s: resolved, want failure", name)
		}
	}
	if splitPath(".") != nil || len(splitPath(".a.b")) != 2 {
		t.Error("splitPath")
	}
}

func TestWorkloadPodSelector(t *testing.T) {
	res := func(kind string, obj map[string]interface{}, values map[string]interface{}) *types.ProcessedResource {
		u := &unstructured.Unstructured{Object: obj}
		u.SetKind(kind)
		u.SetName("web")
		return &types.ProcessedResource{Original: &types.ExtractedResource{Object: u, GVK: u.GroupVersionKind()}, Values: values}
	}
	labels := map[string]interface{}{"app": "web"}
	tests := []struct {
		name string
		r    *types.ProcessedResource
		want interface{}
	}{
		{"nil", nil, nil},
		{"values selector", res("Deployment", map[string]interface{}{}, map[string]interface{}{"selector": map[string]interface{}{"matchLabels": labels}}), "app"},
		{"input selector", res("StatefulSet", map[string]interface{}{"spec": map[string]interface{}{"selector": map[string]interface{}{"matchLabels": labels}}}, nil), "app"},
		{"cronjob pod labels", res("CronJob", map[string]interface{}{"spec": map[string]interface{}{"jobTemplate": map[string]interface{}{"spec": map[string]interface{}{"template": map[string]interface{}{"metadata": map[string]interface{}{"labels": labels}}}}}}, nil), "app"},
		{"podLabels values", res("Job", map[string]interface{}{}, map[string]interface{}{"podLabels": labels}), "app"},
		{"processor default", res("DaemonSet", map[string]interface{}{}, nil), "app"},
		{"job without labels", res("Job", map[string]interface{}{}, nil), nil},
	}
	for _, tt := range tests {
		sel := workloadPodSelector(tt.r)
		if tt.want == nil {
			if sel != nil {
				t.Errorf("%s: selector %v, want none", tt.name, sel)
			}
			continue
		}
		ml, _ := sel["matchLabels"].(map[string]interface{})
		if ml["app"] != "web" {
			t.Errorf("%s: selector %v", tt.name, sel)
		}
	}
}

func TestSelectorYAMLEscapesDelimiters(t *testing.T) {
	lines := selectorYAML(map[string]interface{}{"matchLabels": map[string]interface{}{"tpl": "{{x}}"}}, 4)
	if len(lines) != 2 || lines[0] != "    matchLabels:" || lines[1] != `      tpl: '{{"{{"}}x{{"}}"}}'` {
		t.Errorf("selectorYAML = %q", lines)
	}
}
