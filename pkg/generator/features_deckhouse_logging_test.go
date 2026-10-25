package generator

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/testutil"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestDeckhouseLoggingFeature(t *testing.T) {
	// Create graph with deployment
	graph := types.NewResourceGraph()

	deploy := makeTestResource("apps/v1", "Deployment", "nginx", "default", map[string]interface{}{
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"app.kubernetes.io/name": "nginx",
				},
			},
		},
	})
	deploy.TemplatePath = "templates/deployment-nginx.yaml" // make it part of chart
	graph.AddResource(deploy)

	chart := &types.GeneratedChart{
		Templates: map[string]string{
			"templates/deployment-nginx.yaml": "test",
		},
		Helpers: "{{- define \"test.labels\" -}}\napp: test\n{{- end -}}",
	}

	opts := map[string]string{
		"destination": "loki-storage,loki-archive",
		"multiline":   "MultilineJSON",
	}

	fc := FeatureContext{Graph: graph, Params: opts}

	out, err := applyDeckhouseLoggingFeature(chart, fc)
	testutil.AssertNoError(t, err)

	tpl, ok := out.Templates["templates/pod-logging-config.yaml"]
	if !ok {
		t.Fatal("Expected templates/pod-logging-config.yaml to be generated")
	}

	testutil.AssertContains(t, tpl, "kind: PodLoggingConfig", "should be PodLoggingConfig")
	testutil.AssertContains(t, tpl, "type: {{ . }}", "multilineParser")

	valYaml := out.ValuesYAML
	testutil.AssertContains(t, valYaml, "logging:", "should add logging values")
	testutil.AssertContains(t, valYaml, "destinations:", "destinations values")
	testutil.AssertContains(t, valYaml, "loki-storage", "first dest")
	testutil.AssertContains(t, valYaml, "loki-archive", "second dest")
	testutil.AssertContains(t, valYaml, "MultilineJSON", "multiline type should be included")
	testutil.AssertContains(t, valYaml, "nginx:", "nginx workload key")
}

func TestDeckhouseLoggingFeature_EmptyDestinations(t *testing.T) {
	graph := types.NewResourceGraph()

	deploy := makeTestResource("apps/v1", "Deployment", "nginx", "default", map[string]interface{}{
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"app.kubernetes.io/name": "nginx",
				},
			},
		},
	})
	deploy.TemplatePath = "templates/deployment-nginx.yaml" // make it part of chart
	graph.AddResource(deploy)

	chart := &types.GeneratedChart{
		Templates: map[string]string{
			"templates/deployment-nginx.yaml": "test",
		},
	}

	opts := map[string]string{} // no destination

	fc := FeatureContext{Graph: graph, Params: opts}
	out, err := applyDeckhouseLoggingFeature(chart, fc)
	testutil.AssertNoError(t, err)

	if !strings.Contains(out.Notes, "deckhouse-logging: set logging.destinations") {
		t.Error("Expected NOTES.txt to complain about empty destinations")
	}
}

func TestDeckhouseLoggingFeature_InvalidDestination(t *testing.T) {
	chart := &types.GeneratedChart{}
	opts := map[string]string{
		"destination": "Loki_Storage", // invalid DNS subdomain
	}

	fc := FeatureContext{Graph: types.NewResourceGraph(), Params: opts}
	_, err := applyDeckhouseLoggingFeature(chart, fc)

	if err == nil {
		t.Fatal("Expected error for invalid destination")
	}
	testutil.AssertContains(t, err.Error(), "invalid", "error message")
}

func TestDeckhouseLoggingFeature_InvalidMultiline(t *testing.T) {
	chart := &types.GeneratedChart{}
	opts := map[string]string{
		"multiline": "Custom", // unsupported
	}

	fc := FeatureContext{Graph: types.NewResourceGraph(), Params: opts}
	_, err := applyDeckhouseLoggingFeature(chart, fc)

	if err == nil {
		t.Fatal("Expected error for invalid multiline")
	}
	testutil.AssertContains(t, err.Error(), "unsupported multiline parser", "error message")
}

func TestDeckhouseLoggingFeature_CoveredByExisting(t *testing.T) {
	graph := types.NewResourceGraph()

	deploy := makeTestResource("apps/v1", "Deployment", "nginx", "default", map[string]interface{}{
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"app": "nginx",
				},
			},
		},
	})
	deploy.TemplatePath = "templates/deployment-nginx.yaml"
	graph.AddResource(deploy)

	plc := makeTestResource("deckhouse.io/v1alpha1", "PodLoggingConfig", "nginx-logs", "default", nil)
	graph.AddResource(plc)

	// Add relationship
	graph.AddRelationship(types.Relationship{
		From: plc.Original.ResourceKey(),
		To:   deploy.Original.ResourceKey(),
		Type: types.RelationLabelSelector,
	})

	chart := &types.GeneratedChart{
		Templates: map[string]string{
			"templates/deployment-nginx.yaml": "test",
		},
	}

	opts := map[string]string{
		"destination": "loki",
	}

	fc := FeatureContext{Graph: graph, Params: opts}
	out, err := applyDeckhouseLoggingFeature(chart, fc)
	testutil.AssertNoError(t, err)

	valYaml := out.ValuesYAML
	if strings.Contains(valYaml, "nginx:") {
		t.Error("Nginx should be skipped because it is already covered")
	}
}

func makeTestResource(apiVersion, kind, name, namespace string, obj map[string]interface{}) *types.ProcessedResource {
	if obj == nil {
		obj = map[string]interface{}{}
	}
	obj["apiVersion"] = apiVersion
	obj["kind"] = kind
	if obj["metadata"] == nil {
		obj["metadata"] = map[string]interface{}{}
	}
	meta := obj["metadata"].(map[string]interface{})
	meta["name"] = name
	meta["namespace"] = namespace

	ext := &types.ExtractedResource{
		Object: &unstructured.Unstructured{Object: obj},
	}
	ext.GVK = ext.ResourceKey().GVK
	ext.GVK.Kind = kind

	return &types.ProcessedResource{
		Original:     ext,
		TemplatePath: "templates/" + name + ".yaml",
	}
}
