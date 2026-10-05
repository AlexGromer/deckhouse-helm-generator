package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

func TestParseFeatureOptions(t *testing.T) {
	got, err := ParseFeatureOptions([]string{"vault-agent.role=app", "vault-agent.path=a=b", "otel.lang=go"})
	if err != nil {
		t.Fatal(err)
	}
	if got["vault-agent"]["role"] != "app" || got["vault-agent"]["path"] != "a=b" || got["otel"]["lang"] != "go" {
		t.Errorf("unexpected parse result: %v", got)
	}

	for _, bad := range []string{"novalue", "nofeature=1", ".key=1", "feature.=1"} {
		if _, err := ParseFeatureOptions([]string{bad}); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func withTestFeature(t *testing.T, f Feature) {
	t.Helper()
	RegisterFeature(f)
	t.Cleanup(func() { delete(featureRegistry, f.Name) })
}

func TestApplyFeatures(t *testing.T) {
	var gotParams map[string]string
	withTestFeature(t, Feature{
		Name:        "test-feature",
		Description: "test",
		Params:      map[string]string{"color": "red", "size": "1"},
		Apply: func(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
			gotParams = fc.Params
			out := cloneChart(chart)
			out.Templates["templates/extra.yaml"] = "x"
			return out, nil
		},
	})

	app := &types.GeneratedChart{Name: "app", ChartYAML: "apiVersion: v2\nname: app\ntype: application\n", Templates: map[string]string{}}
	lib := &types.GeneratedChart{Name: "library", ChartYAML: "apiVersion: v2\nname: library\ntype: library\n", Templates: map[string]string{}}

	charts, err := ApplyFeatures([]*types.GeneratedChart{app, lib}, []string{"test-feature"},
		map[string]map[string]string{"test-feature": {"color": "blue"}}, types.NewResourceGraph())
	if err != nil {
		t.Fatal(err)
	}
	if gotParams["color"] != "blue" || gotParams["size"] != "1" {
		t.Errorf("params not merged with defaults: %v", gotParams)
	}
	if _, ok := charts[0].Templates["templates/extra.yaml"]; !ok {
		t.Error("feature not applied to application chart")
	}
	if len(app.Templates) != 0 {
		t.Error("feature mutated its input chart")
	}
	if len(charts[1].Templates) != 0 {
		t.Error("feature must not be applied to library charts")
	}

	cases := []struct {
		names   []string
		options map[string]map[string]string
		want    string
	}{
		{[]string{"missing"}, nil, "unknown feature"},
		{[]string{"test-feature"}, map[string]map[string]string{"test-feature": {"typo": "1"}}, "no parameter"},
		{nil, map[string]map[string]string{"test-feature": {"color": "x"}}, "not enabled"},
	}
	for _, c := range cases {
		_, err := ApplyFeatures([]*types.GeneratedChart{app}, c.names, c.options, types.NewResourceGraph())
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("ApplyFeatures(%v, %v) error = %v, want %q", c.names, c.options, err, c.want)
		}
	}
}

func TestFeatureContextParams(t *testing.T) {
	fc := FeatureContext{Params: map[string]string{"b": "yes", "n": "42", "l": "a, b,,c"}}
	if !fc.BoolParam("b") || fc.BoolParam("missing") {
		t.Error("BoolParam")
	}
	if fc.IntParam("n") != 42 || fc.IntParam("l") != 0 {
		t.Error("IntParam")
	}
	if got := fc.ListParam("l"); strings.Join(got, "|") != "a|b|c" {
		t.Errorf("ListParam = %v", got)
	}
}

func TestRegisteredFeaturesAreComplete(t *testing.T) {
	for _, f := range Features() {
		if f.Name != strings.ToLower(f.Name) || strings.ContainsAny(f.Name, " _.") {
			t.Errorf("feature name %q must be lower-kebab-case", f.Name)
		}
	}
}
