package generator

import (
	"reflect"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

func TestDetectLanguageFromImage(t *testing.T) {
	cases := map[string]string{
		"eclipse-temurin:21-jre": "java",
		"openjdk:17":             "java",
		"registry.example.com/team/spring-orders:1.2": "java",
		"amazoncorretto:21":                           "java",
		"public.ecr.aws/amazoncorretto/corretto:21":   "java",
		"python:3.12-slim":                            "python",
		"ghcr.io/acme/fastapi-app@sha256:abc":         "python",
		"node:20-alpine":                              "nodejs",
		"mcr.microsoft.com/dotnet/aspnet:8.0":         "dotnet",
		"quay.io/prometheus/node-exporter:v1.8.0":     "",
		"golang:1.22":                                 "",
		"nginx:1.27":                                  "",
		"localhost:5000/python-worker:latest":         "python",
		"":                                            "",
	}
	for image, want := range cases {
		if got := detectLanguageFromImage(image); got != want {
			t.Errorf("detectLanguageFromImage(%q) = %q, want %q", image, got, want)
		}
	}
}

func TestOTelFeature(t *testing.T) {
	out := applyObsFeatures(t, []string{"otel"}, nil)

	inst := out.Templates["templates/otel-instrumentation.yaml"]
	for _, want := range []string{
		"{{- if .Values.otel.enabled }}",
		"apiVersion: opentelemetry.io/v1alpha1",
		"kind: Instrumentation",
		`  name: {{ include "app.fullname" . }}`,
		`    {{- include "app.labels" . | nindent 4 }}`,
		"  {{- toYaml .Values.otel.instrumentation | nindent 2 }}",
	} {
		if !strings.Contains(inst, want) {
			t.Errorf("Instrumentation template misses %q:\n%s", want, inst)
		}
	}

	dep := out.Templates[obsDeploymentPath]
	if !strings.Contains(dep, `instrumentation.opentelemetry.io/inject-java: {{ include "app.fullname" $ | quote }}`) {
		t.Errorf("Java Deployment not annotated:\n%s", dep)
	}
	if out.Templates[obsStatefulSetPath] != obsStatefulSetTemplate {
		t.Error("postgres StatefulSet has no detectable language and must stay unchanged")
	}

	want := map[string]interface{}{
		"enabled": true,
		"instrumentation": map[string]interface{}{
			"exporter":    map[string]interface{}{"endpoint": "http://otel-collector:4317"},
			"propagators": []interface{}{"tracecontext", "baggage"},
			"sampler":     map[string]interface{}{"type": "parentbased_traceidratio", "argument": "1"},
		},
	}
	if got := valuesOf(t, out)["otel"]; !reflect.DeepEqual(got, want) {
		t.Errorf("values.otel = %v, want %v", got, want)
	}
}

func TestOTelFeature_ForcedLanguageAndSampling(t *testing.T) {
	out := applyObsFeatures(t, []string{"otel"}, map[string]map[string]string{"otel": {
		"language": "python", "sampling-ratio": "0.25", "endpoint": "", "propagators": "b3",
	}})
	for _, path := range []string{obsDeploymentPath, obsStatefulSetPath} {
		if !strings.Contains(out.Templates[path], "instrumentation.opentelemetry.io/inject-python") {
			t.Errorf("%s not annotated with the forced language", path)
		}
	}
	inst := valuesOf(t, out)["otel"].(map[string]interface{})["instrumentation"].(map[string]interface{})
	if _, ok := inst["exporter"]; ok {
		t.Error("empty endpoint must leave the exporter to the Operator default")
	}
	if inst["sampler"].(map[string]interface{})["argument"] != "0.25" || !reflect.DeepEqual(inst["propagators"], []interface{}{"b3"}) {
		t.Errorf("instrumentation values = %v", inst)
	}
}

func TestOTelFeature_InvalidParams(t *testing.T) {
	for _, opts := range []map[string]string{
		{"language": "go"},
		{"sampler": "sometimes"},
		{"sampling-ratio": "2"},
		{"sampling-ratio": "abc"},
	} {
		_, err := ApplyFeatures([]*types.GeneratedChart{obsTestChart()}, []string{"otel"},
			map[string]map[string]string{"otel": opts}, obsTestGraph())
		if err == nil {
			t.Errorf("options %v must be rejected", opts)
		}
	}
}
