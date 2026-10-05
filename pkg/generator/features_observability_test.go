package generator

import (
	"reflect"
	"strings"
	"testing"
	"text/template"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// The fixture mirrors what the universal generator emits (see
// examples/05-full-stack): a Deployment with the conditional podAnnotations
// block, its Service, a StatefulSet and a CronJob.

const obsDeploymentTemplate = `{{- $svc := .Values.services.web -}}
{{- if $svc.enabled }}
{{- with $svc.deployment }}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "app.fullname" $ }}-web
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "app.labels" $ | nindent 4 }}
    app.kubernetes.io/component: web
spec:
  {{- if not .autoscaling }}
  replicas: {{ .replicas | default 1 }}
  {{- end }}
  selector:
    matchLabels:
      {{- include "app.selectorLabels" $ | nindent 6 }}
      app.kubernetes.io/component: web
  template:
    metadata:
      {{- with .podAnnotations }}
      annotations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      labels:
        {{- include "app.labels" $ | nindent 8 }}
        app.kubernetes.io/component: web
    spec:
      containers:
        {{- range .containers }}
        - name: {{ .name }}
          image: "{{ .image.repository }}:{{ .image.tag }}"
        {{- end }}
{{- end }}
{{- end }}
`

const obsServiceTemplate = `{{- $svc := .Values.services.web -}}
{{- if $svc.enabled }}
{{- with $svc.service }}
apiVersion: v1
kind: Service
metadata:
  name: {{ include "app.fullname" $ }}-web
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "app.labels" $ | nindent 4 }}
    app.kubernetes.io/component: web
  {{- with .annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  type: {{ .type | default "ClusterIP" }}
  ports:
    {{- range .ports }}
    - name: {{ .name | default "http" }}
      port: {{ .port }}
    {{- end }}
  selector:
    {{- include "app.selectorLabels" $ | nindent 4 }}
    app.kubernetes.io/component: web
{{- end }}
{{- end }}
`

const obsStatefulSetTemplate = `{{- $svc := .Values.services.db -}}
{{- if $svc.enabled }}
{{- with $svc.statefulSet }}
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: {{ include "app.fullname" $ }}-db
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "app.labels" $ | nindent 4 }}
    app.kubernetes.io/component: db
spec:
  serviceName: {{ .serviceName }}
  replicas: {{ .replicas | default 1 }}
  selector:
    matchLabels:
      {{- include "app.selectorLabels" $ | nindent 6 }}
      app.kubernetes.io/component: db
  template:
    metadata:
      {{- with .podAnnotations }}
      annotations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      labels:
        {{- include "app.labels" $ | nindent 8 }}
        app.kubernetes.io/component: db
    spec:
      containers:
        {{- range .containers }}
        - name: {{ .name }}
          image: "{{ .image.repository }}:{{ .image.tag }}"
        {{- end }}
{{- end }}
{{- end }}
`

const obsCronJobTemplate = `{{- $svc := .Values.services.backup -}}
{{- if $svc.enabled }}
{{- with $svc.cronJob }}
apiVersion: batch/v1
kind: CronJob
metadata:
  name: {{ include "app.fullname" $ }}-backup
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "app.labels" $ | nindent 4 }}
    app.kubernetes.io/component: backup
spec:
  schedule: {{ .schedule | quote }}
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: OnFailure
          containers:
            - name: backup
              image: busybox
{{- end }}
{{- end }}
`

const obsHelpers = `{{- define "app.fullname" -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- define "app.selectorLabels" -}}
app.kubernetes.io/name: app
{{- end }}
{{- define "app.labels" -}}
{{ include "app.selectorLabels" . }}
{{- end }}
`

const (
	obsDeploymentPath  = "templates/web-deployment.yaml"
	obsServicePath     = "templates/web-service.yaml"
	obsStatefulSetPath = "templates/db-statefulset.yaml"
	obsCronJobPath     = "templates/backup-cronjob.yaml"
)

func obsTestChart() *types.GeneratedChart {
	return &types.GeneratedChart{
		Name:       "app",
		ChartYAML:  "apiVersion: v2\nname: app\nversion: 0.1.0\ntype: application\n",
		ValuesYAML: "# Values\nglobal: {}\nservices:\n  web:\n    enabled: true\n",
		Helpers:    obsHelpers,
		Templates: map[string]string{
			obsDeploymentPath:  obsDeploymentTemplate,
			obsServicePath:     obsServiceTemplate,
			obsStatefulSetPath: obsStatefulSetTemplate,
			obsCronJobPath:     obsCronJobTemplate,
		},
	}
}

// obsWorkload builds a processed input resource generated into templatePath.
func obsWorkload(kind, name, templatePath string, podSpec map[string]interface{}) *types.ProcessedResource {
	group := "apps"
	if kind == "CronJob" || kind == "Job" {
		group = "batch"
	}
	obj := map[string]interface{}{
		"apiVersion": group + "/v1",
		"kind":       kind,
		"metadata":   map[string]interface{}{"name": name, "namespace": "default"},
		"spec":       map[string]interface{}{"template": map[string]interface{}{"spec": podSpec}},
	}
	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: &unstructured.Unstructured{Object: obj},
			GVK:    schema.GroupVersionKind{Group: group, Version: "v1", Kind: kind},
		},
		ServiceName:  name,
		TemplatePath: templatePath,
	}
}

func container(image string, extra map[string]interface{}) map[string]interface{} {
	c := map[string]interface{}{"name": "main", "image": image}
	for k, v := range extra {
		c[k] = v
	}
	return c
}

// obsTestGraph: web (Java image, uses a ConfigMap), db (postgres, no config
// references), backup (CronJob).
func obsTestGraph() *types.ResourceGraph {
	graph := types.NewResourceGraph()
	graph.AddResource(obsWorkload("Deployment", "web", obsDeploymentPath, map[string]interface{}{
		"containers": []interface{}{container("eclipse-temurin:21-jre", map[string]interface{}{
			"envFrom": []interface{}{map[string]interface{}{"configMapRef": map[string]interface{}{"name": "web-config"}}},
		})},
	}))
	graph.AddResource(obsWorkload("StatefulSet", "db", obsStatefulSetPath, map[string]interface{}{
		"containers": []interface{}{container("postgres:16", nil)},
	}))
	graph.AddResource(obsWorkload("CronJob", "backup", obsCronJobPath, map[string]interface{}{
		"containers": []interface{}{container("busybox", nil)},
	}))
	return graph
}

// applyObsFeatures applies features to the fixture chart and checks the
// generic contract: input not mutated, every template is a valid Go template.
func applyObsFeatures(t *testing.T, names []string, opts map[string]map[string]string) *types.GeneratedChart {
	t.Helper()
	chart := obsTestChart()
	charts, err := ApplyFeatures([]*types.GeneratedChart{chart}, names, opts, obsTestGraph())
	if err != nil {
		t.Fatalf("ApplyFeatures(%v): %v", names, err)
	}
	if !reflect.DeepEqual(chart, obsTestChart()) {
		t.Fatalf("ApplyFeatures(%v) mutated its input chart", names)
	}
	out := charts[0]
	for path, content := range out.Templates {
		assertTemplateParses(t, path, content)
	}
	var values map[string]interface{}
	if err := yaml.Unmarshal([]byte(out.ValuesYAML), &values); err != nil {
		t.Fatalf("values.yaml is not valid YAML: %v\n%s", err, out.ValuesYAML)
	}
	return out
}

// assertTemplateParses checks a template with Helm's delimiters and the
// functions generated templates use (balanced blocks, valid actions).
func assertTemplateParses(t *testing.T, name, content string) {
	t.Helper()
	stub := func(...interface{}) interface{} { return nil }
	funcs := template.FuncMap{}
	for _, f := range []string{"include", "toYaml", "nindent", "indent", "quote", "default", "list",
		"join", "trimSuffix", "trunc", "tpl", "printf"} {
		funcs[f] = stub
	}
	if _, err := template.New(name).Funcs(funcs).Parse(content); err != nil {
		t.Errorf("template %s does not parse: %v\n%s", name, err, content)
	}
}

// valuesOf returns the parsed values.yaml of a chart.
func valuesOf(t *testing.T, chart *types.GeneratedChart) map[string]interface{} {
	t.Helper()
	var values map[string]interface{}
	if err := yaml.Unmarshal([]byte(chart.ValuesYAML), &values); err != nil {
		t.Fatal(err)
	}
	return values
}

func TestParseResourceTemplate(t *testing.T) {
	rt, ok := parseResourceTemplate(obsDeploymentPath, obsDeploymentTemplate)
	if !ok {
		t.Fatal("generated Deployment template not recognised")
	}
	if rt.kind != "Deployment" || rt.name != `{{ include "app.fullname" $ }}-web` {
		t.Errorf("kind=%q name=%q", rt.kind, rt.name)
	}
	if len(rt.prefix) != 3 || len(rt.suffix) != 2 {
		t.Errorf("prefix=%v suffix=%v", rt.prefix, rt.suffix)
	}
	if rt.render() != obsDeploymentTemplate {
		t.Errorf("render() does not round-trip:\n%s", rt.render())
	}
	if got := rt.selector(); len(got) != 4 || got[0] != "  selector:" {
		t.Errorf("selector() = %q", got)
	}
	if got := rt.labels(); len(got) != 3 || got[0] != "  labels:" {
		t.Errorf("labels() = %q", got)
	}

	for name, content := range map[string]string{
		"multi-document":   "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a\n---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: b\n",
		"unclosed wrapper": "{{- if .Values.x }}\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a\n",
		"no name":          "apiVersion: v1\nkind: ConfigMap\ndata: {}\n",
		"yaml before doc":  "foo: bar\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a\n",
	} {
		if _, ok := parseResourceTemplate("t.yaml", content); ok {
			t.Errorf("%s: template should not be recognised", name)
		}
	}
}

func TestReindent(t *testing.T) {
	got := reindent([]string{"    {{- include \"x\" $ | nindent 4 }}", "    a: b", ""}, 2)
	want := []string{"      {{- include \"x\" $ | nindent 6 }}", "      a: b", ""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reindent = %q, want %q", got, want)
	}
}

func TestInjectAnnotations(t *testing.T) {
	entries := func(cond, line string) func(int) []string {
		return func(indent int) []string {
			return annotationLines(cond, indent, [][2]string{{line, `"x"`}})
		}
	}
	cases := []struct {
		name, metadata string
		want           []string
		ok             bool
	}{
		{
			name:     "no annotations",
			metadata: "metadata:\n  name: a\nspec: {}",
			want: []string{"metadata:", "  name: a", "  {{- if or ($.c1) }}", "  annotations:",
				"    {{- if $.c1 }}", `    k1: "x"`, "    {{- end }}", "  {{- end }}", "spec: {}"},
			ok: true,
		},
		{
			name:     "generator with-block",
			metadata: "metadata:\n  {{- with .podAnnotations }}\n  annotations:\n    {{- toYaml . | nindent 4 }}\n  {{- end }}\n  name: a",
			want: []string{"metadata:", "  {{- if or ($.c1) (.podAnnotations) }}", "  annotations:",
				"    {{- if $.c1 }}", `    k1: "x"`, "    {{- end }}",
				"    {{- with .podAnnotations }}", "    {{- toYaml . | nindent 4 }}", "    {{- end }}",
				"  {{- end }}", "  name: a"},
			ok: true,
		},
		{
			name:     "unconditional",
			metadata: "metadata:\n  annotations:\n    a: b\n  name: a",
			want:     []string{"metadata:", "  annotations:", "    {{- if $.c1 }}", `    k1: "x"`, "    {{- end }}", "    a: b", "  name: a"},
			ok:       true,
		},
		{
			name:     "empty map",
			metadata: "metadata:\n  annotations: {}\n  name: a",
			want:     []string{"metadata:", "  annotations:", "    {{- if $.c1 }}", `    k1: "x"`, "    {{- end }}", "  name: a"},
			ok:       true,
		},
		{
			name:     "unknown conditional",
			metadata: "metadata:\n  {{- if .x }}\n  annotations:\n    a: b\n  {{- end }}",
			ok:       false,
		},
	}
	for _, c := range cases {
		rt := &resourceTemplate{body: strings.Split(c.metadata, "\n")}
		ok := rt.injectAnnotations(0, "$.c1", entries("$.c1", "k1"))
		if ok != c.ok {
			t.Errorf("%s: ok=%v, want %v", c.name, ok, c.ok)
			continue
		}
		if ok && !reflect.DeepEqual(rt.body, c.want) {
			t.Errorf("%s:\ngot  %q\nwant %q", c.name, rt.body, c.want)
		}
	}

	// A second feature extends the guard instead of adding a second key.
	rt := &resourceTemplate{body: strings.Split("metadata:\n  {{- with .podAnnotations }}\n  annotations:\n    {{- toYaml . | nindent 4 }}\n  {{- end }}", "\n")}
	rt.injectAnnotations(0, "$.c1", entries("$.c1", "k1"))
	rt.injectAnnotations(0, "$.c2", entries("$.c2", "k2"))
	joined := strings.Join(rt.body, "\n")
	if strings.Count(joined, "annotations:") != 1 || !strings.Contains(joined, "{{- if or ($.c2) ($.c1) (.podAnnotations) }}") {
		t.Errorf("guard not extended:\n%s", joined)
	}
	assertTemplateParses(t, "composed", joined)
}

func TestNameExpressions(t *testing.T) {
	templates := []*resourceTemplate{
		{name: `{{ include "app.fullname" $ }}-web`},
		{name: "literal-name"},
		{name: `{{ .Values.weird }}`},
		{name: `{{ include "app.fullname" $ }}-web`},
	}
	got := nameExpressions(templates)
	want := []string{`(printf "%s-web" (include "app.fullname" $))`, `"literal-name"`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("nameExpressions = %q, want %q", got, want)
	}
}

func TestObservabilityFeaturesRegistered(t *testing.T) {
	for _, name := range []string{"reloader", "linkerd", "istio", "otel", "prometheus-rules", "argo-rollouts"} {
		if _, ok := LookupFeature(name); !ok {
			t.Errorf("feature %q is not registered", name)
		}
	}
}

// TestObservabilityFeaturesCompose applies every feature of this file to the
// same chart, in both orders.
func TestObservabilityFeaturesCompose(t *testing.T) {
	names := []string{"argo-rollouts", "istio", "linkerd", "otel", "prometheus-rules", "reloader"}
	reversed := make([]string, len(names))
	for i, n := range names {
		reversed[len(names)-1-i] = n
	}
	for _, order := range [][]string{names, reversed} {
		out := applyObsFeatures(t, order, nil)
		values := valuesOf(t, out)
		for _, key := range []string{"argoRollouts", "istio", "linkerd", "otel", "prometheusRules", "reloader"} {
			if _, ok := values[key]; !ok {
				t.Errorf("order %v: values.%s missing", order, key)
			}
		}
		dep := out.Templates[obsDeploymentPath]
		podMeta := dep[strings.Index(dep, "    metadata:"):strings.Index(dep, "    spec:")]
		if strings.Count(podMeta, "annotations:") != 1 {
			t.Errorf("order %v: pod template must have exactly one annotations key:\n%s", order, podMeta)
		}
		for _, want := range []string{"linkerd.podAnnotations", "instrumentation.opentelemetry.io/inject-java", ".podAnnotations"} {
			if !strings.Contains(podMeta, want) {
				t.Errorf("order %v: pod annotations miss %q:\n%s", order, want, podMeta)
			}
		}
		for _, path := range []string{"templates/istio-web-service.yaml", "templates/argo-rollout-web-deployment.yaml",
			"templates/otel-instrumentation.yaml", "templates/prometheus-rules.yaml"} {
			if _, ok := out.Templates[path]; !ok {
				t.Errorf("order %v: %s missing", order, path)
			}
		}
	}
}
