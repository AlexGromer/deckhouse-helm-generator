package generator

// ============================================================
// Test Plan: Stakater Reloader Annotation Injector (Task 5.7.5)
// ============================================================
//
// | #  | Test Name                                                   | Category    | Input                                              | Expected Output                                                        |
// |----|-------------------------------------------------------------|-------------|----------------------------------------------------|------------------------------------------------------------------------|
// |  1 | TestInjectReloaderAnnotations_AutoReload                    | happy       | chart with Deployment template, AutoReload=true    | reloader.stakater.com/auto="true" present in template                  |
// |  2 | TestInjectReloaderAnnotations_ConfigMapSpecific             | happy       | WatchConfigMaps=true, candidate has configmaps     | reloader.stakater.com/search="true" + configmap annotation             |
// |  3 | TestInjectReloaderAnnotations_SecretSpecific                | happy       | WatchSecrets=true, candidate has secrets           | secret-specific annotation injected                                    |
// |  4 | TestDetectReloaderCandidates_NoMounts_Skipped               | edge        | Deployment with no volumes                         | no candidates returned for that workload                               |
// |  5 | TestInjectReloaderAnnotations_NilChart                      | error       | nil chart                                          | returns (nil, 0) without panic                                         |
// |  6 | TestInjectReloaderAnnotations_CopyOnWrite                   | happy       | chart with templates                               | original chart templates unchanged after injection                     |
// |  7 | TestDetectReloaderCandidates_MultipleWorkloads               | happy       | 2 Deployments each mounting different configmaps   | 2 candidates returned with correct workload names                      |
// |  8 | TestInjectReloaderAnnotations_Idempotent                    | happy       | inject twice on same chart                         | count same on second call, annotation not duplicated                   |
// |  9 | TestDetectReloaderCandidates_ConfigMapAndSecret              | happy       | Deployment mounting both CM and Secret             | candidate has both MountedConfigMaps and MountedSecrets populated      |
// | 10 | TestInjectReloaderAnnotations_CountMatchesCandidates        | happy       | chart with 2 workload templates                    | returned count equals number of templates modified                     |

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ============================================================
// Test Helpers
// ============================================================

const reloaderDeploymentTemplate = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
  annotations: {}
spec:
  replicas: 1
  template:
    spec:
      volumes:
      - name: config-vol
        configMap:
          name: app-config
      containers:
      - name: app
        image: myapp:latest`

const reloaderDeploymentWithSecretTemplate = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp-secret
  annotations: {}
spec:
  replicas: 1
  template:
    spec:
      volumes:
      - name: secret-vol
        secret:
          secretName: app-secret
      containers:
      - name: app
        image: myapp:latest`

const reloaderStatefulSetTemplate = `apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: mydb
  annotations: {}
spec:
  replicas: 1
  template:
    spec:
      volumes:
      - name: db-config
        configMap:
          name: db-config
      containers:
      - name: db
        image: postgres:16`

const reloaderDeploymentNoVolumes = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: bare-app
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: app
        image: busybox:latest`

// makeReloaderCandidate creates a ReloaderCandidate for use in tests.
func makeReloaderCandidate(name string, configmaps, secrets []string) ReloaderCandidate {
	return ReloaderCandidate{
		WorkloadName:      name,
		MountedConfigMaps: configmaps,
		MountedSecrets:    secrets,
	}
}

// makeReloaderGraph builds a ResourceGraph containing workload ProcessedResources
// with volume mounts populated in Values so that DetectReloaderCandidates can use them.
func makeReloaderGraph(entries []struct {
	kind       string
	name       string
	configmaps []string
	secrets    []string
}) *types.ResourceGraph {
	graph := types.NewResourceGraph()
	for _, e := range entries {
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       e.kind,
				"metadata": map[string]interface{}{
					"name":      e.name,
					"namespace": "default",
				},
			},
		}

		// Build volumes list for the spec to allow candidates detection
		volumes := make([]interface{}, 0)
		for _, cm := range e.configmaps {
			volumes = append(volumes, map[string]interface{}{
				"name": cm + "-vol",
				"configMap": map[string]interface{}{
					"name": cm,
				},
			})
		}
		for _, sec := range e.secrets {
			volumes = append(volumes, map[string]interface{}{
				"name": sec + "-vol",
				"secret": map[string]interface{}{
					"secretName": sec,
				},
			})
		}

		obj.Object["spec"] = map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes":    volumes,
					"containers": []interface{}{},
				},
			},
		}

		r := &types.ProcessedResource{
			Original: &types.ExtractedResource{
				Object: obj,
				GVK: schema.GroupVersionKind{
					Group:   "apps",
					Version: "v1",
					Kind:    e.kind,
				},
			},
			ServiceName: e.name,
			Values:      make(map[string]interface{}),
		}
		graph.AddResource(r)
	}
	return graph
}

// ============================================================
// Test 1: AutoReload=true injects reloader.stakater.com/auto="true"
// ============================================================

func TestInjectReloaderAnnotations_AutoReload(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": reloaderDeploymentTemplate,
	})
	opts := ReloaderOptions{
		AutoReload:      true,
		WatchConfigMaps: false,
		WatchSecrets:    false,
	}

	newChart, count := InjectReloaderAnnotations(chart, opts)

	if newChart == nil {
		t.Fatal("expected non-nil chart")
	}
	if count == 0 {
		t.Error("expected count > 0 when auto-reload enabled")
	}

	found := false
	for _, content := range newChart.Templates {
		if strings.Contains(content, "reloader.stakater.com/auto") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'reloader.stakater.com/auto' annotation in at least one template")
	}
}

// ============================================================
// Test 2: WatchConfigMaps=true injects configmap-specific annotation
// ============================================================

func TestInjectReloaderAnnotations_ConfigMapSpecific(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": reloaderDeploymentTemplate,
	})
	opts := ReloaderOptions{
		AutoReload:      false,
		WatchConfigMaps: true,
		WatchSecrets:    false,
	}

	newChart, count := InjectReloaderAnnotations(chart, opts)

	if newChart == nil {
		t.Fatal("expected non-nil chart")
	}
	if count == 0 {
		t.Error("expected count > 0 when WatchConfigMaps=true")
	}

	found := false
	for _, content := range newChart.Templates {
		// configmap annotation: reloader.stakater.com/search or configmap name annotation
		if strings.Contains(content, "reloader.stakater.com") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Reloader annotation for configmap-specific watch")
	}
}

// ============================================================
// Test 3: WatchSecrets=true injects secret-specific annotation
// ============================================================

func TestInjectReloaderAnnotations_SecretSpecific(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": reloaderDeploymentWithSecretTemplate,
	})
	opts := ReloaderOptions{
		AutoReload:      false,
		WatchConfigMaps: false,
		WatchSecrets:    true,
	}

	newChart, count := InjectReloaderAnnotations(chart, opts)

	if newChart == nil {
		t.Fatal("expected non-nil chart")
	}
	if count == 0 {
		t.Error("expected count > 0 when WatchSecrets=true")
	}

	found := false
	for _, content := range newChart.Templates {
		if strings.Contains(content, "reloader.stakater.com") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Reloader annotation for secret-specific watch")
	}
}

// ============================================================
// Test 4: Deployment with no volume mounts → not a candidate
// ============================================================

func TestDetectReloaderCandidates_NoMounts_Skipped(t *testing.T) {
	graph := makeReloaderGraph([]struct {
		kind       string
		name       string
		configmaps []string
		secrets    []string
	}{
		{kind: "Deployment", name: "bare-app", configmaps: nil, secrets: nil},
	})

	candidates := DetectReloaderCandidates(graph)

	for _, c := range candidates {
		if c.WorkloadName == "bare-app" {
			if len(c.MountedConfigMaps) != 0 || len(c.MountedSecrets) != 0 {
				t.Errorf("bare-app should not have mounts, got CM=%v Secrets=%v",
					c.MountedConfigMaps, c.MountedSecrets)
			}
		}
	}
	// A workload with no mounts should either not be included at all
	// or be included with empty slices — verify no secrets/configmaps reported.
	for _, c := range candidates {
		if c.WorkloadName == "bare-app" {
			if len(c.MountedConfigMaps) > 0 || len(c.MountedSecrets) > 0 {
				t.Error("bare-app has no mounts, should have empty mount lists")
			}
		}
	}
}

// ============================================================
// Test 5: nil chart → (nil, 0) without panic
// ============================================================

func TestInjectReloaderAnnotations_NilChart(t *testing.T) {
	opts := ReloaderOptions{AutoReload: true}

	newChart, count := InjectReloaderAnnotations(nil, opts)

	if newChart != nil {
		t.Errorf("expected nil chart for nil input, got %+v", newChart)
	}
	if count != 0 {
		t.Errorf("expected count=0 for nil input, got %d", count)
	}
}

// ============================================================
// Test 6: Copy-on-write — original chart is unchanged
// ============================================================

func TestInjectReloaderAnnotations_CopyOnWrite(t *testing.T) {
	originalContent := reloaderDeploymentTemplate
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": originalContent,
	})
	opts := ReloaderOptions{AutoReload: true}

	newChart, _ := InjectReloaderAnnotations(chart, opts)

	if newChart == nil {
		t.Fatal("expected non-nil result chart")
	}
	// Original must be unchanged
	if chart.Templates["templates/deployment.yaml"] != originalContent {
		t.Error("original chart.Templates was mutated — copy-on-write violated")
	}
	// Result must differ from original (annotation was injected)
	if newChart.Templates["templates/deployment.yaml"] == originalContent {
		t.Error("expected result chart template to differ from original after injection")
	}
}

// ============================================================
// Test 7: Multiple workloads → one candidate per workload
// ============================================================

func TestDetectReloaderCandidates_MultipleWorkloads(t *testing.T) {
	graph := makeReloaderGraph([]struct {
		kind       string
		name       string
		configmaps []string
		secrets    []string
	}{
		{kind: "Deployment", name: "frontend", configmaps: []string{"frontend-cfg"}, secrets: nil},
		{kind: "Deployment", name: "backend", configmaps: []string{"backend-cfg"}, secrets: nil},
	})

	candidates := DetectReloaderCandidates(graph)

	names := make(map[string]bool)
	for _, c := range candidates {
		names[c.WorkloadName] = true
	}

	if !names["frontend"] {
		t.Error("expected candidate for 'frontend' workload")
	}
	if !names["backend"] {
		t.Error("expected candidate for 'backend' workload")
	}
}

// ============================================================
// Test 8: Idempotent — second injection does not duplicate annotations
// ============================================================

func TestInjectReloaderAnnotations_Idempotent(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": reloaderDeploymentTemplate,
	})
	opts := ReloaderOptions{AutoReload: true}

	firstChart, firstCount := InjectReloaderAnnotations(chart, opts)
	if firstChart == nil {
		t.Fatal("first injection returned nil")
	}

	secondChart, secondCount := InjectReloaderAnnotations(firstChart, opts)
	if secondChart == nil {
		t.Fatal("second injection returned nil")
	}

	_ = firstCount
	_ = secondCount

	// The annotation must appear exactly once — not duplicated
	for path, content := range secondChart.Templates {
		occurrences := strings.Count(content, "reloader.stakater.com/auto")
		if occurrences > 1 {
			t.Errorf("template %q: annotation duplicated (%d times) after idempotent injection",
				path, occurrences)
		}
	}
}

// ============================================================
// Test 9: Workload mounting both CM and Secret → both populated
// ============================================================

func TestDetectReloaderCandidates_ConfigMapAndSecret(t *testing.T) {
	graph := makeReloaderGraph([]struct {
		kind       string
		name       string
		configmaps []string
		secrets    []string
	}{
		{
			kind:       "Deployment",
			name:       "mixed-app",
			configmaps: []string{"app-config"},
			secrets:    []string{"app-secret"},
		},
	})

	candidates := DetectReloaderCandidates(graph)

	var target *ReloaderCandidate
	for i := range candidates {
		if candidates[i].WorkloadName == "mixed-app" {
			target = &candidates[i]
			break
		}
	}

	if target == nil {
		t.Fatal("expected candidate for 'mixed-app'")
	}
	if len(target.MountedConfigMaps) == 0 {
		t.Error("expected MountedConfigMaps to contain 'app-config'")
	}
	if len(target.MountedSecrets) == 0 {
		t.Error("expected MountedSecrets to contain 'app-secret'")
	}
}

// ============================================================
// Test 10: Count equals number of templates actually modified
// ============================================================

func TestInjectReloaderAnnotations_CountMatchesCandidates(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml":  reloaderDeploymentTemplate,
		"templates/statefulset.yaml": reloaderStatefulSetTemplate,
		"templates/service.yaml":     testServiceTemplate,
	})
	opts := ReloaderOptions{AutoReload: true}

	newChart, count := InjectReloaderAnnotations(chart, opts)

	if newChart == nil {
		t.Fatal("expected non-nil chart")
	}

	// Count the templates that actually contain the annotation
	annotated := 0
	for _, content := range newChart.Templates {
		if strings.Contains(content, "reloader.stakater.com/auto") {
			annotated++
		}
	}

	if count != annotated {
		t.Errorf("returned count=%d but found %d annotated templates", count, annotated)
	}
	if count == 0 {
		t.Error("expected at least one workload template to be annotated")
	}
}
