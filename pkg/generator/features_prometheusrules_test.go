package generator

import (
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"strings"
	"testing"
)

func TestMapParam(t *testing.T) {
	cases := []struct {
		input string
		want  map[string]string
		err   string
	}{
		{"", map[string]string{}, ""},
		{"a=b", map[string]string{"a": "b"}, ""},
		{"a=b;c=d", map[string]string{"a": "b", "c": "d"}, ""},
		{"a=b; a=c", nil, "duplicate key"},
		{"a", nil, "not key=value"},
		{"a=; b=c", map[string]string{"a": "", "b": "c"}, ""},
		{"!bad=v", nil, "invalid"},
		{"a=bad v", nil, "invalid"},
		{"app.kubernetes.io/part-of=shop", map[string]string{"app.kubernetes.io/part-of": "shop"}, ""},
	}
	for _, c := range cases {
		fc := FeatureContext{Params: map[string]string{"p": c.input}}
		got, err := fc.MapParam("p")
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("MapParam(%q) error = %v, want %q", c.input, err, c.err)
			}
		} else {
			if err != nil {
				t.Errorf("MapParam(%q) unexpected error: %v", c.input, err)
			} else {
				if len(got) != len(c.want) {
					t.Errorf("MapParam(%q) = %v, want %v", c.input, got, c.want)
				}
				for k, v := range c.want {
					if got[k] != v {
						t.Errorf("MapParam(%q)[%q] = %q, want %q", c.input, k, got[k], v)
					}
				}
			}
		}
	}
}

func TestPrometheusRulesLabels(t *testing.T) {
	chart := &types.GeneratedChart{
		Name:      "app",
		ChartYAML: "name: app\n",
		Templates: map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: {{ include \"app.fullname\" $ }}-web\n",
		},
		Helpers: "{{- define \"app.fullname\" -}}app{{- end -}}\n",
	}

	cases := []struct {
		params map[string]string
		want   []string
	}{
		{
			params: map[string]string{"labels": "a=b;c=d"},
			want: []string{
				"  labels:\n",
				"    a: b\n",
				"    c: d\n",
			},
		},
		{
			params: map[string]string{"labels": "prometheus=custom;component=rules"},
			want: []string{
				"  labels:\n",
				"    component: rules\n",
				"    prometheus: custom\n",
			},
		},
		{
			params: map[string]string{"prometheus": "default-prom"},
			want: []string{
				"  labels:\n",
				"    component: rules\n",
				"    prometheus: default-prom\n",
			},
		},
		{
			params: map[string]string{"labels": "x=y", "prometheus": "old"},
			want: []string{
				"  labels:\n",
				"    prometheus: old\n",
				"    x: \"y\"\n",
			},
		},
	}

	for _, c := range cases {
		out, err := ApplyFeatures([]*types.GeneratedChart{chart}, []string{"prometheus-rules"}, map[string]map[string]string{"prometheus-rules": c.params}, nil)
		if err != nil {
			t.Fatal(err)
		}

		values := out[0].ValuesYAML
		for _, w := range c.want {
			if !strings.Contains(values, w) {
				t.Errorf("labels block missing %q in values:\n%s", w, values)
			}
		}
	}
}
