package generator

import (
	"context"
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestGeneratedNOTESListsRolloutAndDriftCommands(t *testing.T) {
	labels := map[string]string{"app.kubernetes.io/name": "myapp"}
	build := func() *types.ResourceGraph {
		deploy := makeProcessedResourceWithValues("Deployment", "web", "default", labels, map[string]interface{}{"replicas": int64(1)}, "# deploy")
		cm := makeProcessedResourceWithValues("ConfigMap", "cfg", "default", labels, map[string]interface{}{}, "# cm")
		g := buildGraph([]*types.ProcessedResource{deploy, cm}, nil)
		g.Groups = []*types.ResourceGroup{{Name: "myapp", Resources: []*types.ProcessedResource{deploy, cm}}}
		return g
	}
	want := []string{
		`rollout status deploy/{{ include "test.objname" (list $ "web") }} --timeout=30m`,
		`helm get manifest {{ .Release.Name }} -n {{ .Release.Namespace }} | kubectl -n {{ .Release.Namespace }} diff -f -`,
	}

	t.Run("universal", func(t *testing.T) {
		charts, err := NewUniversalGenerator().Generate(context.Background(), build(), Options{ChartName: "test", ChartVersion: "1.0.0"})
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range want {
			if !strings.Contains(charts[0].Notes, w) {
				t.Errorf("NOTES lacks %q:\n%s", w, charts[0].Notes)
			}
		}
	})
	t.Run("separate", func(t *testing.T) {
		charts, err := NewSeparateGenerator().Generate(context.Background(), build(), Options{ChartName: "test", ChartVersion: "1.0.0"})
		if err != nil {
			t.Fatal(err)
		}
		if len(charts) != 1 {
			t.Fatalf("charts = %d", len(charts))
		}
		notes := strings.ReplaceAll(charts[0].Notes, `"`+charts[0].Name+`.`, `"test.`)
		for _, w := range want {
			if !strings.Contains(notes, w) {
				t.Errorf("NOTES lacks %q:\n%s", w, charts[0].Notes)
			}
		}
	})
	// An Argo Rollout is a workload: the drift check applies. Its rollout status
	// is `kubectl argo rollouts status`, so it is not listed with the Deployments.
	t.Run("rollout only", func(t *testing.T) {
		rollout := makeProcessedResourceWithValues("Rollout", "worker", "default", labels, map[string]interface{}{}, "# rollout")
		g := buildGraph([]*types.ProcessedResource{rollout}, nil)
		g.Groups = []*types.ResourceGroup{{Name: "myapp", Resources: []*types.ProcessedResource{rollout}}}
		charts, err := NewUniversalGenerator().Generate(context.Background(), g, Options{ChartName: "test", ChartVersion: "1.0.0"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(charts[0].Notes, want[1]) {
			t.Errorf("NOTES of a Rollout chart lack the drift check:\n%s", charts[0].Notes)
		}
		if strings.Contains(charts[0].Notes, "rollout status deploy/") {
			t.Errorf("NOTES list a Rollout as a Deployment:\n%s", charts[0].Notes)
		}
	})
	t.Run("no workloads", func(t *testing.T) {
		cm := makeProcessedResourceWithValues("ConfigMap", "cfg", "default", labels, map[string]interface{}{}, "# cm")
		g := buildGraph([]*types.ProcessedResource{cm}, nil)
		g.Groups = []*types.ResourceGroup{{Name: "myapp", Resources: []*types.ProcessedResource{cm}}}
		charts, err := NewUniversalGenerator().Generate(context.Background(), g, Options{ChartName: "test", ChartVersion: "1.0.0"})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(charts[0].Notes, "helm get manifest") {
			t.Errorf("a chart without workloads has no drift check:\n%s", charts[0].Notes)
		}
	})
}
