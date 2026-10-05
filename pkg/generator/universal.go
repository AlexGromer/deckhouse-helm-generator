package generator

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/helm"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// UniversalGenerator generates a single chart with all services in values.yaml.
type UniversalGenerator struct {
	BaseGenerator
}

// NewUniversalGenerator creates a new universal generator.
func NewUniversalGenerator() *UniversalGenerator {
	return &UniversalGenerator{
		BaseGenerator: NewBaseGenerator(types.OutputModeUniversal),
	}
}

// Generate creates a universal chart.
func (g *UniversalGenerator) Generate(ctx context.Context, graph *types.ResourceGraph, opts Options) ([]*types.GeneratedChart, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Build chart metadata
	chartMeta := helm.ChartMetadata{
		Name:        opts.ChartName,
		Version:     opts.ChartVersion,
		AppVersion:  opts.AppVersion,
		Description: fmt.Sprintf("Helm chart for %s", opts.ChartName),
		APIVersion:  "v2",
		Type:        "application",
		Keywords:    []string{"kubernetes", "deckhouse"},
	}

	// Build values
	valuesBuilder := helm.NewValuesBuilder()

	// Add global values
	valuesBuilder.SetGlobal("imageRegistry", "")
	valuesBuilder.SetGlobal("imagePullSecrets", []interface{}{})

	// Place every resource's values exactly where its template reads them
	// (processor.Result.ValuesPath); resources without a ValuesPath fall back
	// to the per-group layout.
	for _, group := range graph.Groups {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		var unplaced []*types.ProcessedResource
		for _, resource := range group.Resources {
			if !placeValuesByPath(valuesBuilder, resource) {
				unplaced = append(unplaced, resource)
			}
		}
		if len(unplaced) > 0 {
			config := g.buildServiceConfig(&types.ResourceGroup{Name: group.Name, Resources: unplaced})
			for key, value := range config {
				valuesBuilder.SetValue("services."+group.Name+"."+key, value)
			}
		}
	}

	serviceNames := make([]string, 0)
	if services, ok := valuesBuilder.GetValue("services"); ok {
		if m, ok := services.(map[string]interface{}); ok {
			for name := range m {
				serviceNames = append(serviceNames, name)
			}
		}
	}

	// Sort service names for consistent output
	sort.Strings(serviceNames)

	// Build templates map
	templates := make(map[string]string)
	for _, group := range graph.Groups {
		for _, resource := range group.Resources {
			if resource.TemplatePath != "" && resource.TemplateContent != "" {
				templates[resource.TemplatePath] = resource.TemplateContent
			}
		}
	}

	// Generate Chart.yaml
	chartYAML := helm.GenerateChartYAML(chartMeta)

	// Generate values.yaml
	var valuesYAML string
	var err error
	if opts.ValuesFlat {
		valuesYAML, err = valuesBuilder.BuildFlat()
	} else {
		valuesYAML, err = valuesBuilder.Build()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to build values.yaml: %w", err)
	}

	// Generate _helpers.tpl
	helpers := helm.GenerateHelpers(opts.ChartName)

	// Collect external files from ExternalFileManager
	externalFiles := make([]types.ExternalFileInfo, 0)
	if opts.ExternalFileManager != nil {
		files := opts.ExternalFileManager.GetFiles()
		for _, file := range files {
			externalFiles = append(externalFiles, types.ExternalFileInfo{
				Path:    file.Path,
				Content: file.Content,
			})
		}

		// Add helper functions for external files if any exist
		if len(files) > 0 {
			fileHelpers := opts.ExternalFileManager.GenerateHelmHelper(opts.ChartName)
			helpers = helpers + "\n" + fileHelpers
		}
	}

	// Generate NOTES.txt
	notes := helm.GenerateNOTES(opts.ChartName, serviceNames, helm.NOTESContext{})

	// Generate values.schema.json if requested
	var valuesSchema string
	if opts.IncludeSchema {
		valuesSchema = helm.GenerateValuesSchema(serviceNames)
	}

	chart := &types.GeneratedChart{
		Name:          opts.ChartName,
		Path:          opts.OutputDir,
		ChartYAML:     chartYAML,
		ValuesYAML:    valuesYAML,
		Templates:     templates,
		Helpers:       helpers,
		Notes:         notes,
		ValuesSchema:  valuesSchema,
		ExternalFiles: externalFiles,
	}

	// Generate helm-unittest test files if requested
	if opts.IncludeTests {
		testFiles := GenerateHelmTests(chart)
		for path, content := range testFiles {
			chart.Templates[path] = content
		}
	}

	// Generate Helm lifecycle hook Job templates if requested
	if opts.IncludeHooks {
		hookTemplates := GenerateHelmHooks(chart)
		for path, content := range hookTemplates {
			chart.Templates[path] = content
		}
	}

	return []*types.GeneratedChart{chart}, nil
}

// placeValuesByPath stores resource.Values at resource.ValuesPath and enables
// the owning service. It returns false when the resource has no ValuesPath.
func placeValuesByPath(b *helm.ValuesBuilder, resource *types.ProcessedResource) bool {
	if resource.ValuesPath == "" {
		return false
	}
	values := resource.Values
	if values == nil {
		values = map[string]interface{}{}
	}
	b.SetValue(resource.ValuesPath, values)
	if parts := strings.SplitN(resource.ValuesPath, ".", 3); len(parts) == 3 && parts[0] == "services" {
		if _, ok := b.GetValue("services." + parts[1] + ".enabled"); !ok {
			b.SetValue("services."+parts[1]+".enabled", true)
		}
	}
	return true
}

// buildServiceConfig builds the configuration for a service from its resource group.
func (g *UniversalGenerator) buildServiceConfig(group *types.ResourceGroup) map[string]interface{} {
	config := make(map[string]interface{})
	config["enabled"] = true

	// Organize resources by kind
	resourcesByKind := make(map[string][]*types.ProcessedResource)
	for _, resource := range group.Resources {
		kind := resource.Original.GVK.Kind
		resourcesByKind[kind] = append(resourcesByKind[kind], resource)
	}

	// Merge values from all resources
	for kind, resources := range resourcesByKind {
		// Always use nested structure for ConfigMaps and Secrets
		if kind == "ConfigMap" || kind == "Secret" {
			kindMap := make(map[string]interface{})
			for _, resource := range resources {
				resourceName := sanitizeName(resource.Original.Object.GetName())
				kindMap[resourceName] = resource.Values
			}
			kindKey := pluralizeKind(kind)
			config[kindKey] = kindMap
		} else if len(resources) == 1 {
			// Single resource: nest under kind key to match template references
			// (e.g., $svc.deployment, $svc.service, $svc.statefulSet)
			resource := resources[0]
			kindKey := kindToValuesKey(kind)
			config[kindKey] = resource.Values
		} else {
			// Multiple resources of the same kind
			// Create a map with resource names as keys
			kindMap := make(map[string]interface{})
			for _, resource := range resources {
				resourceName := resource.Original.Object.GetName()
				kindMap[resourceName] = resource.Values
			}

			// Use a pluralized kind name as the key
			kindKey := pluralizeKind(kind)
			config[kindKey] = kindMap
		}
	}

	return config
}

// kindToValuesKey converts a GVK Kind name to the values.yaml key used by templates.
// Templates reference values as $svc.deployment, $svc.service, $svc.statefulSet, etc.
func kindToValuesKey(kind string) string {
	switch kind {
	case "PersistentVolumeClaim":
		return "pvc"
	default:
		if len(kind) == 0 {
			return "resource"
		}
		// lowerCamelCase: Deployment → deployment, StatefulSet → statefulSet
		return strings.ToLower(kind[:1]) + kind[1:]
	}
}

// sanitizeName converts a Kubernetes resource name to a valid Go/YAML key (camelCase).
// Similar to processor.SanitizeServiceName but also handles _ as separator and
// always lowercases the first character.
func sanitizeName(name string) string {
	result := make([]byte, 0, len(name))
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			result = append(result, byte(c))
		} else if c == '-' || c == '_' || c == '.' {
			// Always use '_' as a camelCase marker for the second pass.
			result = append(result, '_')
		}
	}
	// Handle camelCase conversion
	final := make([]byte, 0, len(result))
	capitalize := false
	for i, c := range result {
		if c == '_' {
			capitalize = true
			continue
		}
		if capitalize && c >= 'a' && c <= 'z' {
			final = append(final, c-32)
			capitalize = false
		} else if i == 0 && c >= 'A' && c <= 'Z' {
			// Lowercase first character
			final = append(final, c+32)
		} else {
			final = append(final, c)
		}
	}
	if len(final) == 0 {
		return "config"
	}
	return string(final)
}

// pluralizeKind returns a pluralized version of the kind name.
func pluralizeKind(kind string) string {
	// Simple pluralization rules
	switch kind {
	case "Ingress":
		return "ingresses"
	case "Service":
		return "services"
	case "ConfigMap":
		return "configMaps"
	case "Secret":
		return "secrets"
	case "ServiceAccount":
		return "serviceAccounts"
	case "Deployment":
		return "deployments"
	case "StatefulSet":
		return "statefulSets"
	case "DaemonSet":
		return "daemonSets"
	case "PersistentVolumeClaim":
		return "persistentVolumeClaims"
	case "Role":
		return "roles"
	case "RoleBinding":
		return "roleBindings"
	case "ClusterRole":
		return "clusterRoles"
	case "ClusterRoleBinding":
		return "clusterRoleBindings"
	default:
		// Default: add 's'
		return kind + "s"
	}
}

// ValidateChart performs basic validation on a generated chart.
func ValidateChart(chart *types.GeneratedChart) error {
	if chart.Name == "" {
		return fmt.Errorf("chart name is empty")
	}
	if chart.ChartYAML == "" {
		return fmt.Errorf("Chart.yaml is empty")
	}
	if chart.ValuesYAML == "" {
		return fmt.Errorf("values.yaml is empty")
	}
	// An umbrella parent chart legitimately has no templates of its own (its
	// content is the set of subcharts declared as dependencies), nor does a
	// library chart (its content is _helpers.tpl).
	isLibrary := strings.Contains(chart.ChartYAML, "\ntype: library") && chart.Helpers != ""
	hasDeps := strings.Contains(chart.ChartYAML, "\ndependencies:")
	if len(chart.Templates) == 0 && !isLibrary && !hasDeps {
		return fmt.Errorf("no templates generated")
	}
	return nil
}
