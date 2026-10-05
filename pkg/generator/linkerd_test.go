package generator

import (
	"reflect"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

func TestLinkerdFeature(t *testing.T) {
	out := applyObsFeatures(t, []string{"linkerd"}, nil)

	for _, path := range []string{obsDeploymentPath, obsStatefulSetPath} {
		content := out.Templates[path]
		podMeta := content[strings.Index(content, "    metadata:"):strings.Index(content, "    spec:")]
		for _, want := range []string{
			"      {{- if or (and $.Values.linkerd.enabled $.Values.linkerd.podAnnotations) (.podAnnotations) }}",
			"      annotations:",
			"        {{- toYaml $.Values.linkerd.podAnnotations | nindent 8 }}",
			"        {{- with .podAnnotations }}",
		} {
			if !strings.Contains(podMeta, want) {
				t.Errorf("%s: pod metadata misses %q:\n%s", path, want, podMeta)
			}
		}
		if strings.Count(podMeta, "annotations:") != 1 {
			t.Errorf("%s: duplicate annotations key:\n%s", path, podMeta)
		}
	}
	if out.Templates[obsCronJobPath] != obsCronJobTemplate {
		t.Error("CronJob pods must not be meshed (the proxy keeps them from completing)")
	}
	want := map[string]interface{}{"enabled": true, "podAnnotations": map[string]interface{}{"linkerd.io/inject": "enabled"}}
	if got := valuesOf(t, out)["linkerd"]; !reflect.DeepEqual(got, want) {
		t.Errorf("values.linkerd = %v, want %v", got, want)
	}
}

func TestLinkerdFeature_InjectParam(t *testing.T) {
	out := applyObsFeatures(t, []string{"linkerd"}, map[string]map[string]string{"linkerd": {"inject": "ingress"}})
	if !strings.Contains(out.ValuesYAML, "linkerd.io/inject: ingress") {
		t.Errorf("inject=ingress not reflected in values:\n%s", out.ValuesYAML)
	}

	_, err := ApplyFeatures([]*types.GeneratedChart{obsTestChart()}, []string{"linkerd"},
		map[string]map[string]string{"linkerd": {"inject": "yes"}}, obsTestGraph())
	if err == nil {
		t.Error("invalid inject mode must be rejected")
	}
}
