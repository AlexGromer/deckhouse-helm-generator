package generator

import (
	"reflect"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

func TestDetectReloaderCandidates(t *testing.T) {
	candidates := DetectReloaderCandidates(obsTestGraph())
	if len(candidates) != 1 || candidates[0].Key.Name != "web" || !reflect.DeepEqual(candidates[0].ConfigMaps, []string{"web-config"}) {
		t.Errorf("candidates = %+v, want only web using web-config", candidates)
	}
	if DetectReloaderCandidates(nil) != nil {
		t.Error("nil graph must give no candidates")
	}

	// CronJobs are not handled by the feature even when they use ConfigMaps.
	graph := types.NewResourceGraph()
	graph.AddResource(obsWorkload("CronJob", "job", obsCronJobPath, map[string]interface{}{
		"volumes": []interface{}{map[string]interface{}{"name": "v", "configMap": map[string]interface{}{"name": "c"}}},
	}))
	if got := DetectReloaderCandidates(graph); len(got) != 0 {
		t.Errorf("CronJob must not be a candidate: %+v", got)
	}
}

func TestReloaderFeature(t *testing.T) {
	out := applyObsFeatures(t, []string{"reloader"}, nil)

	dep := out.Templates[obsDeploymentPath]
	workloadMeta := dep[strings.Index(dep, "\nmetadata:"):strings.Index(dep, "\nspec:")]
	for _, want := range []string{
		"  {{- if or ($.Values.reloader.enabled) }}",
		"  annotations:",
		`    reloader.stakater.com/auto: "true"`,
	} {
		if !strings.Contains(workloadMeta, want) {
			t.Errorf("workload metadata misses %q:\n%s", want, workloadMeta)
		}
	}
	if strings.Contains(dep[strings.Index(dep, "\nspec:"):], "reloader") {
		t.Error("Reloader reads workload annotations; the pod template must not be annotated")
	}
	if out.Templates[obsStatefulSetPath] != obsStatefulSetTemplate {
		t.Error("StatefulSet without ConfigMap/Secret references must not be annotated")
	}
	if out.Templates[obsCronJobPath] != obsCronJobTemplate {
		t.Error("CronJob must not be annotated")
	}
	values := valuesOf(t, out)
	if !reflect.DeepEqual(values["reloader"], map[string]interface{}{"enabled": true}) {
		t.Errorf("values.reloader = %v", values["reloader"])
	}
}

func TestReloaderFeature_AllWorkloads(t *testing.T) {
	out := applyObsFeatures(t, []string{"reloader"}, map[string]map[string]string{"reloader": {"all-workloads": "true"}})
	for _, path := range []string{obsDeploymentPath, obsStatefulSetPath} {
		if !strings.Contains(out.Templates[path], "reloader.stakater.com/auto") {
			t.Errorf("%s not annotated with all-workloads=true", path)
		}
	}
}

func TestReloaderFeature_NothingToDo(t *testing.T) {
	chart := obsTestChart()
	charts, err := ApplyFeatures([]*types.GeneratedChart{chart}, []string{"reloader"}, nil, types.NewResourceGraph())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(charts[0], obsTestChart()) {
		t.Error("chart without candidates must be returned unchanged (no values key either)")
	}
}
