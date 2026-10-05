package generator

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

func kustomizeObj(kind, namespace, name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	obj.SetAPIVersion("v1")
	if kind == "Deployment" || kind == "StatefulSet" {
		obj.SetAPIVersion("apps/v1")
	}
	obj.SetKind(kind)
	obj.SetName(name)
	obj.SetNamespace(namespace)
	return obj
}

func TestKustomize_BaseHoldsOneManifestPerObject(t *testing.T) {
	deploy := kustomizeObj("Deployment", "", "web")
	_ = unstructured.SetNestedField(deploy.Object, int64(5), "spec", "replicas")
	_ = unstructured.SetNestedField(deploy.Object, map[string]interface{}{"readyReplicas": int64(5)}, "status")

	out, err := GenerateKustomizeLayout([]*unstructured.Unstructured{
		deploy,
		kustomizeObj("Service", "", "web"),
		kustomizeObj("ConfigMap", "", "web-config"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n" +
		"  - configmap-web-config.yaml\n  - deployment-web.yaml\n  - service-web.yaml\n"
	if out.Base.Kustomization != want {
		t.Errorf("base kustomization:\n%s\nwant:\n%s", out.Base.Kustomization, want)
	}

	var manifest map[string]interface{}
	if err := yaml.Unmarshal([]byte(out.Base.Resources["deployment-web.yaml"]), &manifest); err != nil {
		t.Fatalf("base manifest is not YAML: %v", err)
	}
	if got, _, _ := unstructured.NestedFieldNoCopy(manifest, "spec", "replicas"); got != float64(5) {
		t.Errorf("spec.replicas = %v, want the input value 5", got)
	}
	if _, found := manifest["status"]; found {
		t.Error("status must not be written to the base")
	}
	if _, found := deploy.Object["status"]; !found {
		t.Error("input object must not be mutated")
	}
}

func TestKustomize_SameNameInTwoNamespaces(t *testing.T) {
	out, err := GenerateKustomizeLayout([]*unstructured.Unstructured{
		kustomizeObj("ConfigMap", "a", "settings"),
		kustomizeObj("ConfigMap", "b", "settings"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Base.Resources) != 2 {
		t.Errorf("expected 2 base resources, got %v", out.Base.Resources)
	}
}

func TestKustomize_OverlaysPatchEachWorkloadByIdentity(t *testing.T) {
	out, err := GenerateKustomizeLayout([]*unstructured.Unstructured{
		kustomizeObj("Deployment", "shop", "web"),
		kustomizeObj("StatefulSet", "", "db"),
		kustomizeObj("Service", "shop", "web"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for env, replicas := range map[string]string{"dev": "1", "staging": "2", "prod": "3"} {
		overlay, ok := out.Overlays[env]
		if !ok {
			t.Fatalf("missing overlay %q", env)
		}
		if overlay.Path != "overlays/"+env {
			t.Errorf("overlay path = %q", overlay.Path)
		}
		k := overlay.Kustomization
		for _, want := range []string{
			"  - ../../base\n",
			"      kind: Deployment\n      name: web\n      namespace: shop\n",
			"      kind: StatefulSet\n      name: db\n    patch:",
			"value: " + replicas + "\n",
		} {
			if !strings.Contains(k, want) {
				t.Errorf("%s overlay lacks %q:\n%s", env, want, k)
			}
		}
		if strings.Contains(k, "kind: Service") {
			t.Errorf("%s overlay must not patch a Service:\n%s", env, k)
		}
	}
}

func TestKustomize_NoWorkloadsMeansNoPatches(t *testing.T) {
	out, err := GenerateKustomizeLayout([]*unstructured.Unstructured{kustomizeObj("ConfigMap", "", "c")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out.Overlays["prod"].Kustomization, "patches:") {
		t.Errorf("unexpected patches:\n%s", out.Overlays["prod"].Kustomization)
	}
}

func TestKustomize_NoObjects_ReturnsError(t *testing.T) {
	if _, err := GenerateKustomizeLayout(nil); err == nil {
		t.Fatal("expected error for no objects")
	}
}

func TestKustomize_RejectsUnsafeResourceNames(t *testing.T) {
	for _, name := range []string{"deploy\nment", "deploy:ment", "../../etc/passwd"} {
		out, err := GenerateKustomizeLayout([]*unstructured.Unstructured{kustomizeObj("ConfigMap", "", name)})
		if err == nil {
			t.Errorf("expected error for name %q", name)
		}
		if out != nil {
			t.Errorf("expected nil output on error for %q", name)
		}
	}
}
