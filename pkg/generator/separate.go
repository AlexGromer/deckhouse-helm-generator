package generator

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/helm"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"

	"sigs.k8s.io/yaml"
)

// SeparateGenerator generates separate Helm charts for each service group.
type SeparateGenerator struct {
	BaseGenerator
}

// NewSeparateGenerator creates a new SeparateGenerator.
func NewSeparateGenerator() *SeparateGenerator {
	return &SeparateGenerator{
		BaseGenerator: NewBaseGenerator(types.OutputModeSeparate),
	}
}

// Generate creates one Helm chart per service group.
func (g *SeparateGenerator) Generate(ctx context.Context, graph *types.ResourceGraph, opts Options) ([]*types.GeneratedChart, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Group resources into logical services.
	groupResult, err := GroupResources(graph)
	if err != nil {
		return nil, fmt.Errorf("failed to group resources: %w", err)
	}

	if len(groupResult.Groups) == 0 {
		return []*types.GeneratedChart{}, nil
	}

	charts := make([]*types.GeneratedChart, 0, len(groupResult.Groups))

	for _, group := range groupResult.Groups {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		chart, err := g.generateChartForGroup(group, opts, opts.ChartName)
		if err != nil {
			return nil, fmt.Errorf("failed to generate chart for group %s: %w", group.Name, err)
		}
		charts = append(charts, chart)
	}

	return charts, nil
}

// generateChartForGroup creates a complete Helm chart for a single service group.
// sourceChartName is the chart name the processors rendered templates with;
// their helper references are rewritten to the group chart's own helpers.
func (g *SeparateGenerator) generateChartForGroup(group *ServiceGroup, opts Options, sourceChartName string) (*types.GeneratedChart, error) {
	chartName := group.Name

	// Build Chart.yaml.
	chartMeta := helm.ChartMetadata{
		Name:        chartName,
		Version:     opts.ChartVersion,
		AppVersion:  opts.AppVersion,
		Description: fmt.Sprintf("Helm chart for %s", chartName),
		APIVersion:  "v2",
		Type:        "application",
		Keywords:    []string{"kubernetes", "deckhouse"},
	}
	chartYAML := helm.GenerateChartYAML(chartMeta)

	// Build flat values (no service name nesting).
	values := g.buildFlatValues(group)
	valuesYAML, err := marshalFlatValues(chartName, values)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal values: %w", err)
	}

	// Collect templates and rewrite value paths for flat structure.
	// Processor-generated templates reference .Values.services.<serviceName>.<kind>
	// but in separate mode, values are flat: .Values.<kind>.
	templates := make(map[string]string)
	keys := serviceValuesKeys(group)
	for _, resource := range group.Resources {
		if isCRD(resource) {
			continue
		}
		if resource.TemplatePath != "" && resource.TemplateContent != "" {
			content := rewriteTemplateForSeparateMode(resource.TemplateContent, resource.ServiceName, keys[resource.ServiceName])
			content = rewriteHelperReferences(content, sourceChartName, chartName)
			templates[resource.TemplatePath] = content
		}
	}

	// Generate _helpers.tpl.
	helpers := helm.GenerateHelpers(chartName)

	// Generate NOTES.txt.
	notes := helm.GenerateNOTES(chartName, []string{chartName}, helm.NOTESContext{})

	var valuesSchema string
	if opts.IncludeSchema {
		valuesSchema = helm.InferValuesSchema(values)
	}

	chart := &types.GeneratedChart{
		Name:          chartName,
		Path:          opts.OutputDir,
		ChartYAML:     chartYAML,
		ValuesYAML:    valuesYAML,
		Templates:     templates,
		Helpers:       helpers,
		Notes:         notes,
		ValuesSchema:  valuesSchema,
		ExternalFiles: crdFiles(group.Resources),
	}
	if opts.IncludeREADME {
		chart.ExternalFiles = append(chart.ExternalFiles, types.ExternalFileInfo{
			Path: "README.md", Content: helm.GenerateREADME(chartMeta, values),
		})
	}
	addTestsAndHooks(chart, opts)
	return chart, nil
}

// buildFlatValues builds flat values for a service group.
// Unlike universal mode, values are NOT nested under a service name: the
// services.<svc>. prefix of each resource's ValuesPath is stripped, matching
// rewriteTemplateForSeparateMode.
func (g *SeparateGenerator) buildFlatValues(group *ServiceGroup) map[string]interface{} {
	b := helm.NewValuesBuilder()
	// Processor templates are guarded by `if $svc.enabled`; in separate mode
	// $svc is .Values itself, so the chart must be enabled by default.
	b.SetValue("enabled", true)
	// Templates reference $.Values.global.*; an umbrella parent overrides it.
	b.SetValue("global", map[string]interface{}{})

	keys := serviceValuesKeys(group)
	var unplaced []*types.ProcessedResource
	for _, resource := range group.Resources {
		if isCRD(resource) {
			continue
		}
		path := resource.ValuesPath
		if parts := strings.SplitN(path, ".", 3); len(parts) == 3 && parts[0] == "services" {
			path = parts[2]
			if key := keys[resource.ServiceName]; key != "" {
				b.SetValue(key+".enabled", true)
				path = key + "." + path
			}
		}
		if path == "" {
			unplaced = append(unplaced, resource)
			continue
		}
		values := resource.Values
		if values == nil {
			values = map[string]interface{}{}
		}
		b.SetValue(path, values)
	}
	values := b.BuildMap()

	// Organize resources without a ValuesPath by kind.
	resourcesByKind := make(map[string][]*types.ProcessedResource)
	for _, resource := range unplaced {
		kind := resource.Original.GVK.Kind
		resourcesByKind[kind] = append(resourcesByKind[kind], resource)
	}

	// Build values per kind.
	for kind, resources := range resourcesByKind {
		if kind == "ConfigMap" || kind == "Secret" {
			// Always use nested structure for ConfigMaps and Secrets.
			kindMap := make(map[string]interface{})
			for _, resource := range resources {
				resourceName := sanitizeName(resource.Original.Object.GetName())
				kindMap[resourceName] = resource.Values
			}
			values[pluralizeKind(kind)] = kindMap
		} else if len(resources) == 1 {
			// Single resource: nest under kind key.
			values[kindToValuesKey(kind)] = resources[0].Values
		} else {
			// Multiple resources of same kind.
			kindMap := make(map[string]interface{})
			for _, resource := range resources {
				resourceName := sanitizeName(resource.Original.Object.GetName())
				kindMap[resourceName] = resource.Values
			}
			values[pluralizeKind(kind)] = kindMap
		}
	}

	return values
}

// marshalFlatValues marshals flat values to YAML with a header comment.
func marshalFlatValues(chartName string, values map[string]interface{}) (string, error) {
	if len(values) == 0 {
		return fmt.Sprintf("# Default values for %s\n# Generated by Deckhouse Helm Generator\n", chartName), nil
	}

	yamlBytes, err := yaml.Marshal(values)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# Default values for %s\n", chartName)
	sb.WriteString("# Generated by Deckhouse Helm Generator\n\n")
	sb.Write(yamlBytes)

	return sb.String(), nil
}

// rewriteTemplateForSeparateMode rewrites template content to use flat value paths.
// Replaces patterns like:
//
//	.Values.services.<svc>.<path> -> .Values.<path>
//	$svc := .Values.services.<svc> -> $svc := .Values
//
// or, for a service kept under its own key, .Values.services.<svc> -> .Values.<key>.
func rewriteTemplateForSeparateMode(content, serviceName, key string) string {
	if serviceName == "" {
		return content
	}
	target := ".Values"
	if key != "" {
		target += "." + key
	}
	ref := regexp.MustCompile(`\.Values\.services\.` + regexp.QuoteMeta(serviceName) + `\b`)
	return ref.ReplaceAllString(content, target)
}

// serviceValuesKeys decides where each service of a group puts its values in
// the group chart: "" for the top level, or a key of its own when its paths
// would collide with those of a service already at the top level (e.g. two
// Deployments of one group both writing "deployment"). The group's main
// service (the one named like the group, else the one with most resources)
// is always at the top level, so single-service charts stay flat.
func serviceValuesKeys(group *ServiceGroup) map[string]string {
	paths := map[string][]string{}
	count := map[string]int{}
	for _, r := range group.Resources {
		if isCRD(r) || r.ServiceName == "" {
			continue
		}
		count[r.ServiceName]++
		if parts := strings.SplitN(r.ValuesPath, ".", 3); len(parts) == 3 && parts[0] == "services" {
			paths[r.ServiceName] = append(paths[r.ServiceName], parts[2])
		}
	}
	services := make([]string, 0, len(count))
	for svc := range count {
		services = append(services, svc)
	}
	main := processor.SanitizeServiceName(group.Name)
	sort.Slice(services, func(i, j int) bool {
		a, b := services[i], services[j]
		if (a == main) != (b == main) {
			return a == main
		}
		if count[a] != count[b] {
			return count[a] > count[b]
		}
		return a < b
	})

	keys := make(map[string]string, len(services))
	taken := []string{"enabled", "global"}
	for _, svc := range services {
		if !pathsCollide(paths[svc], taken) {
			taken = append(taken, paths[svc]...)
			continue
		}
		keys[svc] = svc
		taken = append(taken, svc)
	}
	return keys
}

// pathsCollide reports whether a dotted values path of a equals, contains or
// is contained in one of b.
func pathsCollide(a, b []string) bool {
	for _, p := range a {
		for _, q := range b {
			if p == q || strings.HasPrefix(p, q+".") || strings.HasPrefix(q, p+".") {
				return true
			}
		}
	}
	return false
}

// rewriteHelperReferences points include/template calls at the helpers of the
// chart the template ends up in: `include "app.labels"` → `include "frontend.labels"`.
func rewriteHelperReferences(content, from, to string) string {
	if from == "" || from == to {
		return content
	}
	for _, fn := range []string{"include", "template"} {
		content = strings.ReplaceAll(content, fn+` "`+from+`.`, fn+` "`+to+`.`)
	}
	return content
}
