package generator

import (
	"context"
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestLibraryGenerator_WrapperDescription_NoStutter(t *testing.T) {
	deploy := makeProcessedResourceWithValues("Deployment", "app", "default",
		map[string]string{"app.kubernetes.io/name": "app"},
		map[string]interface{}{"replicaCount": 1}, "# deploy")
	charts, err := NewLibraryGenerator().Generate(context.Background(), buildGraph([]*types.ProcessedResource{deploy}, nil), Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	for _, c := range charts {
		if strings.Contains(c.ChartYAML, "library library") {
			t.Errorf("stuttering wording in Chart.yaml of %s:\n%s", c.Name, c.ChartYAML)
		}
	}
}
