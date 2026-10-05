package processor

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

func collisionResource(kind, namespace, name, service, valuesKey string) *types.ProcessedResource {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	obj.SetAPIVersion("v1")
	obj.SetKind(kind)
	obj.SetNamespace(namespace)
	obj.SetName(name)
	return &types.ProcessedResource{
		Original:     &types.ExtractedResource{Object: obj, GVK: obj.GroupVersionKind()},
		ServiceName:  service,
		TemplatePath: "templates/" + service + "-" + strings.ToLower(kind) + ".yaml",
		ValuesPath:   "services." + service + "." + valuesKey,
		TemplateContent: "{{- $svc := .Values.services." + service + " -}}\n" +
			"metadata:\n  name: " + name + "\n  namespace: {{ $.Release.Namespace }}\n" +
			"other: {{ .Values.services." + service + "Extra.x }}\n",
	}
}

func TestResolveCollisions_SameServiceMovesToOwnService(t *testing.T) {
	api := collisionResource("Deployment", "default", "shop-api", "shop", "deployment")
	worker := collisionResource("Deployment", "default", "shop-worker", "shop", "deployment")

	notes := ResolveCollisions([]*types.ProcessedResource{api, worker})

	if api.ServiceName != "shop" || api.TemplatePath != "templates/shop-deployment.yaml" {
		t.Errorf("first resource must keep its paths, got %s %s", api.ServiceName, api.TemplatePath)
	}
	if worker.ServiceName != "shopWorker" || worker.ValuesPath != "services.shopWorker.deployment" ||
		worker.TemplatePath != "templates/shop-deployment-worker.yaml" {
		t.Errorf("worker moved to %s %s %s", worker.ServiceName, worker.ValuesPath, worker.TemplatePath)
	}
	if !strings.Contains(worker.TemplateContent, "$svc := .Values.services.shopWorker -}}") {
		t.Errorf("template still reads the old service:\n%s", worker.TemplateContent)
	}
	if !strings.Contains(worker.TemplateContent, ".Values.services.shopExtra.x") {
		t.Errorf("a longer service name sharing the prefix must not be rewritten:\n%s", worker.TemplateContent)
	}
	if !strings.Contains(worker.TemplateContent, "namespace: {{ $.Release.Namespace }}") {
		t.Errorf("distinct names need no literal namespace:\n%s", worker.TemplateContent)
	}
	if len(notes) != 1 {
		t.Errorf("notes = %v", notes)
	}
}

func TestResolveCollisions_SameNameInTwoNamespaces(t *testing.T) {
	a := collisionResource("ConfigMap", "team-a", "settings", "settings", "configMaps.settings")
	b := collisionResource("ConfigMap", "team-b", "settings", "settings", "configMaps.settings")

	ResolveCollisions([]*types.ProcessedResource{a, b})

	for _, r := range []*types.ProcessedResource{a, b} {
		want := `namespace: "` + r.Original.Object.GetNamespace() + `"`
		if !strings.Contains(r.TemplateContent, want) {
			t.Errorf("%s: template lacks %s:\n%s", r.Original.ResourceKey(), want, r.TemplateContent)
		}
	}
	if a.TemplatePath == b.TemplatePath || a.ValuesPath == b.ValuesPath {
		t.Errorf("paths still collide: %s %s / %s %s", a.TemplatePath, a.ValuesPath, b.TemplatePath, b.ValuesPath)
	}
	if b.ServiceName != "settingsTeamB" {
		t.Errorf("second ConfigMap moved to %q, want settingsTeamB", b.ServiceName)
	}
}

func TestResolveCollisions_NoCollisionNoChange(t *testing.T) {
	a := collisionResource("Deployment", "default", "a", "a", "deployment")
	b := collisionResource("Deployment", "default", "b", "b", "deployment")
	before := [4]string{b.ServiceName, b.TemplatePath, b.ValuesPath, b.TemplateContent}
	if notes := ResolveCollisions([]*types.ProcessedResource{a, b}); len(notes) != 0 {
		t.Errorf("unexpected notes %v", notes)
	}
	if after := [4]string{b.ServiceName, b.TemplatePath, b.ValuesPath, b.TemplateContent}; after != before {
		t.Errorf("resource changed without a collision")
	}
}
