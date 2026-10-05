package generator

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func checksumResource(kind, name, templatePath string, spec map[string]interface{}) *types.ProcessedResource {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": gvkForKind(kind).GroupVersion().String(),
		"kind":       kind,
		"metadata":   map[string]interface{}{"name": name, "namespace": "prod"},
	}}
	if spec != nil {
		obj.Object["spec"] = spec
	}
	gvk := gvkForKind(kind)
	if kind == "Secret" || kind == "ConfigMap" {
		gvk = schema.GroupVersionKind{Version: "v1", Kind: kind}
	}
	return &types.ProcessedResource{
		Original:     &types.ExtractedResource{Object: obj, GVK: gvk},
		TemplatePath: templatePath,
	}
}

func podSpec(spec map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"template": map[string]interface{}{"spec": spec}}
}

func TestConfigReferences(t *testing.T) {
	w := checksumResource("Deployment", "web", "", podSpec(map[string]interface{}{
		"volumes": []interface{}{
			map[string]interface{}{"name": "a", "configMap": map[string]interface{}{"name": "cm-volume"}},
			map[string]interface{}{"name": "b", "secret": map[string]interface{}{"secretName": "secret-volume"}},
			map[string]interface{}{"name": "c", "projected": map[string]interface{}{"sources": []interface{}{
				map[string]interface{}{"configMap": map[string]interface{}{"name": "cm-projected"}},
				map[string]interface{}{"secret": map[string]interface{}{"name": "secret-projected"}},
			}}},
		},
		"initContainers": []interface{}{
			map[string]interface{}{"name": "init", "envFrom": []interface{}{
				map[string]interface{}{"secretRef": map[string]interface{}{"name": "secret-envfrom"}},
			}},
		},
		"containers": []interface{}{
			map[string]interface{}{"name": "app",
				"envFrom": []interface{}{map[string]interface{}{"configMapRef": map[string]interface{}{"name": "cm-envfrom"}}},
				"env": []interface{}{
					map[string]interface{}{"name": "A", "valueFrom": map[string]interface{}{"configMapKeyRef": map[string]interface{}{"name": "cm-key", "key": "k"}}},
					map[string]interface{}{"name": "B", "valueFrom": map[string]interface{}{"secretKeyRef": map[string]interface{}{"name": "secret-key", "key": "k"}}},
					map[string]interface{}{"name": "C", "valueFrom": map[string]interface{}{"configMapKeyRef": map[string]interface{}{"name": "cm-key", "key": "other"}}},
				}},
		},
	}))

	var got []string
	for _, ref := range configReferences(w) {
		if ref.Namespace != "prod" {
			t.Errorf("reference %v must use the workload namespace", ref)
		}
		got = append(got, ref.GVK.Kind+"/"+ref.Name)
	}
	want := "ConfigMap/cm-volume Secret/secret-volume ConfigMap/cm-projected Secret/secret-projected " +
		"Secret/secret-envfrom ConfigMap/cm-envfrom ConfigMap/cm-key Secret/secret-key"
	if strings.Join(got, " ") != want {
		t.Errorf("configReferences =\n%s\nwant\n%s", strings.Join(got, " "), want)
	}
}

func TestChecksumAnnotationKey(t *testing.T) {
	if got := checksumAnnotationKey("ConfigMap", "app-config"); got != "checksum/configmap-app-config" {
		t.Errorf("got %q", got)
	}
	long := checksumAnnotationKey("Secret", strings.Repeat("a", 54)+"-"+strings.Repeat("b", 20))
	if name := strings.TrimPrefix(long, "checksum/"); len(name) > 63 || strings.HasSuffix(name, "-") {
		t.Errorf("annotation name %q exceeds 63 characters or ends with '-'", name)
	}
}

const checksumDeploymentTemplate = `{{- with .Values.deployment }}
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    metadata:
      {{- with .podAnnotations }}
      annotations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
    spec:
      containers: []
{{- end }}
`

func TestInjectConfigChecksums(t *testing.T) {
	web := checksumResource("Deployment", "web", "templates/web-deployment.yaml", podSpec(map[string]interface{}{
		"containers": []interface{}{map[string]interface{}{"name": "app",
			"envFrom": []interface{}{
				map[string]interface{}{"configMapRef": map[string]interface{}{"name": "web-config"}},
				map[string]interface{}{"secretRef": map[string]interface{}{"name": "external-secret"}},
			}}},
	}))
	cm := checksumResource("ConfigMap", "web-config", "templates/web-configmap.yaml", nil)
	graph := types.NewResourceGraph()
	graph.AddResource(web)
	graph.AddResource(cm)

	chart := &types.GeneratedChart{
		Name:       "web",
		ChartYAML:  "apiVersion: v2\nname: web\n",
		ValuesYAML: "deployment: {}\n",
		Templates: map[string]string{
			"templates/web-deployment.yaml": checksumDeploymentTemplate,
			"templates/web-configmap.yaml":  "apiVersion: v1\nkind: ConfigMap\n",
		},
	}
	out, changed, err := InjectConfigChecksums(chart, graph)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 {
		t.Fatalf("changed = %v", changed)
	}
	tpl := out.Templates["templates/web-deployment.yaml"]
	want := `      {{- if or .podAnnotations $dhgConfigChecksums.enabled }}
      annotations:
        {{- if $dhgConfigChecksums.enabled }}
        checksum/configmap-web-config: {{ include (print $.Template.BasePath "/web-configmap.yaml") $ | sha256sum | quote }}
        {{- end }}
        {{- with .podAnnotations }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
      {{- end }}
`
	if !strings.Contains(tpl, want) {
		t.Errorf("template:\n%s\nwant block:\n%s", tpl, want)
	}
	if strings.Contains(tpl, "external-secret") {
		t.Error("objects that are not part of the chart must not be hashed")
	}
	if !strings.Contains(out.ValuesYAML, "configChecksums:\n  enabled: true\n") {
		t.Errorf("values:\n%s", out.ValuesYAML)
	}

	// Already injected: unchanged.
	again, changed, err := InjectConfigChecksums(out, graph)
	if err != nil || again != out || len(changed) != 0 {
		t.Errorf("second application changed the chart (err=%v, changed=%v)", err, changed)
	}

	// Customised annotations block: skipped rather than broken.
	custom := *chart
	custom.Templates = map[string]string{
		"templates/web-deployment.yaml": strings.Replace(checksumDeploymentTemplate, "{{- with .podAnnotations }}", "{{- with .customAnnotations }}", 1),
		"templates/web-configmap.yaml":  chart.Templates["templates/web-configmap.yaml"],
	}
	if got, changed, err := InjectConfigChecksums(&custom, graph); err != nil || got != &custom || len(changed) != 0 {
		t.Errorf("customised template: err=%v changed=%v", err, changed)
	}
}
