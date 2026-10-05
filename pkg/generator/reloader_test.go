package generator

import (
	"reflect"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

func TestConfigReferences(t *testing.T) {
	obj := map[string]interface{}{"spec": map[string]interface{}{"template": map[string]interface{}{"spec": map[string]interface{}{
		"volumes": []interface{}{
			map[string]interface{}{"name": "a", "configMap": map[string]interface{}{"name": "cm-volume"}},
			map[string]interface{}{"name": "b", "secret": map[string]interface{}{"secretName": "secret-volume"}},
			map[string]interface{}{"name": "c", "projected": map[string]interface{}{"sources": []interface{}{
				map[string]interface{}{"configMap": map[string]interface{}{"name": "cm-projected"}},
				map[string]interface{}{"secret": map[string]interface{}{"name": "secret-projected"}},
			}}},
			map[string]interface{}{"name": "d", "emptyDir": map[string]interface{}{}},
		},
		"initContainers": []interface{}{map[string]interface{}{
			"envFrom": []interface{}{map[string]interface{}{"secretRef": map[string]interface{}{"name": "secret-envfrom"}}},
		}},
		"containers": []interface{}{map[string]interface{}{
			"env": []interface{}{
				map[string]interface{}{"name": "A", "valueFrom": map[string]interface{}{"configMapKeyRef": map[string]interface{}{"name": "cm-env", "key": "a"}}},
				map[string]interface{}{"name": "B", "valueFrom": map[string]interface{}{"secretKeyRef": map[string]interface{}{"name": "secret-env", "key": "b"}}},
				map[string]interface{}{"name": "C", "value": "plain"},
			},
			"envFrom": []interface{}{map[string]interface{}{"configMapRef": map[string]interface{}{"name": "cm-volume"}}},
		}},
	}}}}

	cms, secrets := configReferences(obj)
	if want := []string{"cm-env", "cm-projected", "cm-volume"}; !reflect.DeepEqual(cms, want) {
		t.Errorf("configMaps = %v, want %v", cms, want)
	}
	if want := []string{"secret-env", "secret-envfrom", "secret-projected", "secret-volume"}; !reflect.DeepEqual(secrets, want) {
		t.Errorf("secrets = %v, want %v", secrets, want)
	}
}

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
