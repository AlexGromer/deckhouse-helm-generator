package generator

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestInChartNames(t *testing.T) {
	cm := makeProcessedResourceWithValues("ConfigMap", "cfg", "default", nil, nil, "kind: ConfigMap")
	cm2 := makeProcessedResourceWithValues("ConfigMap", "aaa", "default", nil, nil, "kind: ConfigMap")
	svc := makeProcessedResourceWithValues("Service", "web", "default", nil, nil, "kind: Service")
	// Not templates, or names the system fixes: not prefixed, so not listed.
	crd := makeProcessedResourceWithValues("CustomResourceDefinition", "widgets.example.com", "", nil, nil, "kind: CustomResourceDefinition")
	ng := makeProcessedResourceWithValues("NodeGroup", "worker", "", nil, nil, "kind: NodeGroup")
	noTemplate := makeProcessedResourceWithValues("Secret", "skipped", "default", nil, nil, "")

	graph := buildGraph([]*types.ProcessedResource{cm, cm2, svc, crd, ng, noTemplate}, nil)
	got := inChartNames(graph)
	want := map[string][]string{"ConfigMap": {"aaa", "cfg"}, "Service": {"web"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("inChartNames = %v, want %v", got, want)
	}
	if inChartNames(nil) != nil {
		t.Error("nil graph has no objects")
	}
}

// Every chart of a separate layout knows the objects of all charts: a
// reference crosses the chart boundary and must get the same prefix.
func TestSeparateGenerator_HelpersKnowObjectsOfAllCharts(t *testing.T) {
	front := makeProcessedResourceWithValues("Service", "frontend", "default", map[string]string{"app.kubernetes.io/name": "frontend"}, nil, "kind: Service")
	back := makeProcessedResourceWithValues("Service", "backend", "default", map[string]string{"app.kubernetes.io/name": "backend"}, nil, "kind: Service")
	front.ServiceName, back.ServiceName = "frontend", "backend"
	front.ValuesPath, back.ValuesPath = "services.frontend.service", "services.backend.service"
	front.TemplatePath, back.TemplatePath = "templates/frontend-service.yaml", "templates/backend-service.yaml"

	charts, err := NewSeparateGenerator().Generate(context.Background(), buildGraph([]*types.ProcessedResource{front, back}, nil), Options{ChartName: "app", ChartVersion: "0.1.0", AppVersion: "1.0"})
	if err != nil {
		t.Fatal(err)
	}
	if len(charts) < 2 {
		t.Fatalf("want a chart per service, got %d", len(charts))
	}
	for _, c := range charts {
		if !strings.Contains(c.Helpers, `Service: ["backend","frontend"]`) {
			t.Errorf("chart %s helpers do not list the objects of all charts:\n%s", c.Name, c.Helpers)
		}
		if !strings.Contains(c.ValuesYAML, "namePrefix") {
			t.Errorf("chart %s values lack global.namePrefix:\n%s", c.Name, c.ValuesYAML)
		}
	}
}
