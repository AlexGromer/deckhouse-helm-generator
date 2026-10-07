package generator

import (
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestParseResourceTemplate_RejectsUnknownShapes(t *testing.T) {
	for name, content := range map[string]string{
		"no apiVersion":         "kind: ConfigMap\n",
		"text before document":  "# comment\napiVersion: v1\nkind: ConfigMap\n",
		"two documents":         "apiVersion: v1\nkind: ConfigMap\n---\napiVersion: v1\nkind: Secret\n",
		"unclosed wrapper":      "{{- if .Values.a }}\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a\n",
		"end without an opener": "apiVersion: v1\nkind: ConfigMap\n{{- end }}\n{{- end }}\n",
	} {
		if _, ok := parseResourceTemplate("templates/x.yaml", content); ok {
			t.Errorf("%s: parsed, want rejection", name)
		}
	}
}

func TestParseResourceTemplate_WrappedDocument(t *testing.T) {
	content := "{{- $svc := .Values.services.web -}}\n{{- if $svc.enabled }}\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: web\n  labels:\n    app: web\ndata:\n  a: b\n{{- end }}\n\n"
	rt, ok := parseResourceTemplate("templates/web-configmap.yaml", content)
	if !ok {
		t.Fatal("generator template not recognised")
	}
	if rt.kind != "ConfigMap" || rt.name != "web" || len(rt.prefix) != 2 || len(rt.suffix) != 1 {
		t.Errorf("parsed kind=%q name=%q prefix=%v suffix=%v", rt.kind, rt.name, rt.prefix, rt.suffix)
	}
	if rt.render() != content[:len(content)-1] {
		t.Errorf("render does not round-trip:\n%q", rt.render())
	}
	if got := rt.child(-1, "x"); got != -1 {
		t.Errorf("child of missing parent = %d", got)
	}
	if got := rt.block(-1); got != nil {
		t.Errorf("block of missing line = %v", got)
	}
	if got := rt.child(rt.topLevel("metadata:"), "  missing:"); got != -1 {
		t.Errorf("missing child = %d", got)
	}
	if lbl := rt.labels(); len(lbl) != 2 {
		t.Errorf("labels block = %v", lbl)
	}
}

func TestBalancedControl(t *testing.T) {
	if !balancedControl([]string{"{{- if .a }}", "x: 1", "{{- end }}"}) {
		t.Error("balanced block rejected")
	}
	if balancedControl([]string{"{{- end }}", "{{- if .a }}"}) {
		t.Error("end before opener accepted")
	}
	if balancedControl([]string{"{{- with .a }}"}) {
		t.Error("unclosed opener accepted")
	}
}

func TestChartHelpers(t *testing.T) {
	if baseName("README.md") != "README.md" || baseName("templates/_x.tpl") != "_x.tpl" {
		t.Error("baseName")
	}
	chart := &types.GeneratedChart{
		Helpers:   `{{- define "web.fullname" -}}x{{- end -}}`,
		Templates: map[string]string{"templates/_extra.tpl": `{{- define "web.extra" -}}y{{- end -}}`},
	}
	if chartHelperPrefix(chart) != "web" {
		t.Errorf("prefix = %q", chartHelperPrefix(chart))
	}
	if chartHelperPrefix(&types.GeneratedChart{}) != "" {
		t.Error("prefix of a chart without helpers")
	}
	if !chartHasHelper(chart, "web.fullname") || !chartHasHelper(chart, "web.extra") || chartHasHelper(chart, "web.none") {
		t.Error("chartHasHelper")
	}
	rt := &resourceTemplate{path: "templates/a.yaml", kind: "ConfigMap"}
	if graphResourceFor(nil, rt) != nil || graphResourceFor(types.NewResourceGraph(), rt) != nil {
		t.Error("graphResourceFor without a match")
	}
}
