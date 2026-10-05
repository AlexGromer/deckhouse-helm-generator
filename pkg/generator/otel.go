package generator

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// OpenTelemetry auto-instrumentation (`dhg generate --with otel`).
//
// The feature adds one OpenTelemetry Operator `Instrumentation` resource per
// chart (exporter, propagators and sampler come from `.Values.otel.instrumentation`)
// and annotates the pod template of every Deployment/StatefulSet/DaemonSet
// whose language is known with `instrumentation.opentelemetry.io/inject-<lang>`
// pointing at that Instrumentation. The language is detected from the first
// container image, or forced for all workloads with the `language` parameter.

// otelLanguages are the languages the Operator can auto-instrument without
// extra per-workload settings (Go additionally needs the target executable
// path and a privileged sidecar, so it is not offered).
var otelLanguages = map[string]bool{"java": true, "nodejs": true, "python": true, "dotnet": true}

var otelSamplers = map[string]bool{
	"always_on": true, "always_off": true, "traceidratio": true,
	"parentbased_always_on": true, "parentbased_always_off": true, "parentbased_traceidratio": true,
	"jaeger_remote": true, "parentbased_jaeger_remote": true, "xray": true,
}

// languageImageTokens maps image name tokens to an instrumentation language.
var languageImageTokens = map[string]string{
	"java": "java", "openjdk": "java", "jdk": "java", "jre": "java", "temurin": "java",
	"corretto": "java", "amazoncorretto": "java", "spring": "java", "tomcat": "java", "wildfly": "java", "jboss": "java",
	"python": "python", "django": "python", "flask": "python", "fastapi": "python",
	"gunicorn": "python", "uvicorn": "python",
	"node": "nodejs", "nodejs": "nodejs", "express": "nodejs", "nestjs": "nodejs",
	"dotnet": "dotnet", "aspnet": "dotnet", "aspnetcore": "dotnet",
}

// detectLanguageFromImage infers the instrumentation language from a
// container image reference by looking at the tokens of its repository name
// (registry and tag are ignored). It returns "" when unsure.
func detectLanguageFromImage(image string) string {
	repo := strings.ToLower(image)
	if i := strings.Index(repo, "@"); i >= 0 {
		repo = repo[:i]
	}
	if i := strings.LastIndex(repo, ":"); i > strings.LastIndex(repo, "/") {
		repo = repo[:i]
	}
	if parts := strings.Split(repo, "/"); len(parts) > 1 && strings.ContainsAny(parts[0], ".:") {
		repo = strings.Join(parts[1:], "/") // drop the registry host
	}
	tokens := strings.FieldsFunc(repo, func(r rune) bool { return r == '/' || r == '-' || r == '_' || r == '.' })
	for _, t := range tokens {
		if t == "exporter" { // e.g. node-exporter is not a Node.js application
			return ""
		}
	}
	for _, t := range tokens {
		if lang, ok := languageImageTokens[t]; ok {
			return lang
		}
	}
	return ""
}

// firstContainerImage returns the image of the first container of a workload.
func firstContainerImage(obj map[string]interface{}) string {
	containers := asList(nestedMap(obj, "spec", "template", "spec")["containers"])
	if len(containers) == 0 {
		return ""
	}
	c, _ := containers[0].(map[string]interface{})
	image, _ := c["image"].(string)
	return image
}

const otelInstrumentationTemplate = `{{- if .Values.otel.enabled }}
apiVersion: opentelemetry.io/v1alpha1
kind: Instrumentation
metadata:
  name: {{ include "%[1]s.fullname" . }}
  namespace: {{ .Release.Namespace }}
%[2]sspec:
  {{- toYaml .Values.otel.instrumentation | nindent 2 }}
{{- end }}
`

func applyOTelFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	forced := fc.Param("language")
	if forced != "" && !otelLanguages[forced] {
		return nil, fmt.Errorf("language must be one of java, nodejs, python, dotnet (or empty to detect); got %q", forced)
	}
	sampler := fc.Param("sampler")
	if !otelSamplers[sampler] {
		return nil, fmt.Errorf("unknown sampler %q", sampler)
	}
	ratio, err := strconv.ParseFloat(fc.Param("sampling-ratio"), 64)
	if err != nil || ratio < 0 || ratio > 1 {
		return nil, fmt.Errorf("sampling-ratio must be a number between 0 and 1; got %q", fc.Param("sampling-ratio"))
	}

	workloads := resourceTemplates(chart, func(k string) bool { return podWorkloadKinds[k] })
	prefix := chartHelperPrefix(chart)
	if len(workloads) == 0 || prefix == "" {
		return chart, nil
	}

	out := cloneChart(chart)
	for _, rt := range workloads {
		if strings.Contains(chart.Templates[rt.path], "instrumentation.opentelemetry.io/") {
			continue
		}
		lang := forced
		if lang == "" {
			if r := graphResourceFor(fc.Graph, rt); r != nil {
				lang = detectLanguageFromImage(firstContainerImage(r.Original.Object.Object))
			}
		}
		if lang == "" {
			continue
		}
		ok := rt.injectAnnotations(rt.podTemplateMetadata(), "$.Values.otel.enabled", func(indent int) []string {
			return annotationLines("$.Values.otel.enabled", indent, [][2]string{{
				"instrumentation.opentelemetry.io/inject-" + lang,
				fmt.Sprintf(`{{ include "%s.fullname" $ | quote }}`, prefix),
			}})
		})
		if ok {
			out.Templates[rt.path] = rt.render()
		}
	}

	labels := ""
	if chartHasHelper(chart, prefix+".labels") {
		labels = fmt.Sprintf("  labels:\n    {{- include %q . | nindent 4 }}\n", prefix+".labels")
	}
	content := fmt.Sprintf(otelInstrumentationTemplate, prefix, labels)
	if err := addTemplate(out, "templates/otel-instrumentation.yaml", content); err != nil {
		return nil, err
	}

	instrumentation := map[string]interface{}{
		"sampler": map[string]interface{}{
			"type":     sampler,
			"argument": strconv.FormatFloat(ratio, 'f', -1, 64),
		},
	}
	if endpoint := fc.Param("endpoint"); endpoint != "" {
		instrumentation["exporter"] = map[string]interface{}{"endpoint": endpoint}
	}
	if propagators := fc.ListParam("propagators"); len(propagators) > 0 {
		instrumentation["propagators"] = propagators
	}
	err = addFeatureValues(out, "otel", map[string]interface{}{
		"enabled":         true,
		"instrumentation": instrumentation,
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
