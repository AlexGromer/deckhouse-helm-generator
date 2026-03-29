package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// OTELOptions configures OpenTelemetry instrumentation generation.
type OTELOptions struct {
	ExporterEndpoint string
	ExporterProtocol string
	Propagators      []string
	SamplingRate     float64
	Namespace        string
}

// OTELResult holds detected language and generated Instrumentation YAML.
type OTELResult struct {
	// DetectedLanguages maps workload name → detected language.
	DetectedLanguages map[string]string
	// Instrumentations maps workload name → Instrumentation YAML.
	Instrumentations map[string]string
	// NOTESTxt is an optional human-readable summary.
	NOTESTxt string
}

// GenerateOTELInstrumentation detects languages from workload images and generates
// OpenTelemetry Instrumentation CRs for the given graph.
func GenerateOTELInstrumentation(graph *types.ResourceGraph, opts OTELOptions) *OTELResult {
	result := &OTELResult{
		DetectedLanguages: make(map[string]string),
		Instrumentations:  make(map[string]string),
	}

	if graph == nil {
		return result
	}

	for _, r := range graph.Resources {
		kind := r.Original.GVK.Kind
		if kind != "Deployment" && kind != "StatefulSet" && kind != "DaemonSet" {
			continue
		}
		name := r.Original.Object.GetName()
		image := extractImageFromResource(r)
		lang := detectLanguageFromImage(image)
		if lang == "" || lang == "unknown" {
			continue
		}
		result.DetectedLanguages[name] = lang
		result.Instrumentations[name] = generateInstrumentationYAML(name, lang, opts)
	}

	return result
}

// detectLanguageFromImage infers a language from an image name.
func detectLanguageFromImage(image string) string {
	lower := strings.ToLower(image)
	switch {
	case strings.Contains(lower, "java") || strings.Contains(lower, "openjdk") ||
		strings.Contains(lower, "spring") || strings.Contains(lower, "corretto"):
		return "java"
	case strings.Contains(lower, "python") || strings.Contains(lower, "django") ||
		strings.Contains(lower, "flask") || strings.Contains(lower, "fastapi"):
		return "python"
	case strings.Contains(lower, "node") || strings.Contains(lower, "npm") ||
		strings.Contains(lower, "express"):
		return "nodejs"
	case strings.Contains(lower, "golang") || strings.Contains(lower, "/go:") ||
		strings.Contains(lower, "go-"):
		return "go"
	case strings.Contains(lower, "dotnet") || strings.Contains(lower, "aspnet") ||
		strings.Contains(lower, "aspnetcore"):
		return "dotnet"
	default:
		return ""
	}
}

// extractImageFromResource extracts the container image from a processed resource.
func extractImageFromResource(r *types.ProcessedResource) string {
	obj := r.Original.Object.Object
	spec, ok := obj["spec"].(map[string]interface{})
	if !ok {
		return ""
	}
	template, ok := spec["template"].(map[string]interface{})
	if !ok {
		return ""
	}
	tSpec, ok := template["spec"].(map[string]interface{})
	if !ok {
		return ""
	}
	containers, ok := tSpec["containers"].([]interface{})
	if !ok || len(containers) == 0 {
		return ""
	}
	c, ok := containers[0].(map[string]interface{})
	if !ok {
		return ""
	}
	image, _ := c["image"].(string)
	return image
}

func generateInstrumentationYAML(name, lang string, opts OTELOptions) string {
	var sb strings.Builder
	sb.WriteString("apiVersion: opentelemetry.io/v1alpha1\n")
	sb.WriteString("kind: Instrumentation\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: %s-instrumentation\n", name))
	sb.WriteString("spec:\n")
	sb.WriteString(fmt.Sprintf("  # language: %s\n", lang))
	if opts.ExporterEndpoint != "" {
		sb.WriteString("  exporter:\n")
		sb.WriteString(fmt.Sprintf("    endpoint: %s\n", opts.ExporterEndpoint))
	}
	if len(opts.Propagators) > 0 {
		sb.WriteString("  propagators:\n")
		for _, p := range opts.Propagators {
			sb.WriteString(fmt.Sprintf("    - %s\n", p))
		}
	}
	if opts.SamplingRate > 0 {
		sb.WriteString("  sampler:\n")
		sb.WriteString("    type: parentbased_traceidratio\n")
		sb.WriteString(fmt.Sprintf("    argument: \"%g\"\n", opts.SamplingRate))
	}
	return sb.String()
}

// InjectOTELInstrumentation injects Instrumentation CRs into a chart's templates.
// Returns (nil, 0) if chart is nil.
func InjectOTELInstrumentation(chart *types.GeneratedChart, result *OTELResult) (*types.GeneratedChart, int) {
	if chart == nil {
		return nil, 0
	}

	newChart := copyChartTemplates(chart)
	count := 0

	if result == nil {
		return newChart, 0
	}

	for workload, yaml := range result.Instrumentations {
		path := fmt.Sprintf("templates/otel-%s.yaml", strings.ToLower(workload))
		if _, exists := newChart.Templates[path]; exists {
			continue
		}
		newChart.Templates[path] = yaml
		count++
	}

	return newChart, count
}
