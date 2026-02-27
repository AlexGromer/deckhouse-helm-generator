package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/helm"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// knownDependency defines a single infrastructure dependency that can be
// detected from Kubernetes resource signals (env vars, images, ports).
type knownDependency struct {
	Name        string
	Version     string
	EnvPrefixes []string // env var prefixes (matched with strings.HasPrefix)
	EnvExact    []string // env var names requiring exact match
	EnvContains []string // env value substrings (e.g. "postgres" in DATABASE_URL)
	Images      []string // image name substrings (matched against the image field)
	Ports       []int64  // well-known container ports
}

// knownDependencies is the authoritative list of infrastructure dependencies
// that DetectCommonDependencies can recognise.
var knownDependencies = []knownDependency{
	{
		Name:        "postgresql",
		Version:     "12.x.x",
		EnvPrefixes: []string{"POSTGRES_", "PG_"},
		EnvExact:    []string{"PGHOST"},
		EnvContains: []string{"postgres"},
		Images:      []string{"postgres"},
		Ports:       []int64{5432},
	},
	{
		Name:        "mysql",
		Version:     "9.x.x",
		EnvPrefixes: []string{"MYSQL_"},
		Images:      []string{"mysql", "mariadb"},
		Ports:       []int64{3306},
	},
	{
		Name:        "redis",
		Version:     "18.x.x",
		EnvPrefixes: []string{"REDIS_"},
		Images:      []string{"redis"},
		Ports:       []int64{6379},
	},
	{
		Name:        "mongodb",
		Version:     "14.x.x",
		EnvPrefixes: []string{"MONGO_", "MONGODB_"},
		Images:      []string{"mongo"},
		Ports:       []int64{27017},
	},
	{
		Name:        "rabbitmq",
		Version:     "12.x.x",
		EnvPrefixes: []string{"RABBITMQ_"},
		EnvExact:    []string{"AMQP_URL"},
		Images:      []string{"rabbitmq"},
		Ports:       []int64{5672},
	},
	{
		Name:        "elasticsearch",
		Version:     "19.x.x",
		EnvPrefixes: []string{"ELASTIC_", "ELASTICSEARCH_"},
		Images:      []string{"elasticsearch", "elastic"},
		Ports:       []int64{9200},
	},
	{
		Name:        "kafka",
		Version:     "26.x.x",
		EnvPrefixes: []string{"KAFKA_"},
		Images:      []string{"kafka"},
		Ports:       []int64{9092},
	},
}

const bitnamiRepo = "https://charts.bitnami.com/bitnami"

// DetectCommonDependencies scans Kubernetes resources (Deployments, StatefulSets)
// for signals that indicate common infrastructure dependencies such as databases,
// caches, and message brokers. Signals are derived from container environment
// variables, images, and exposed ports.
//
// Returns a deduplicated slice of helm.Dependency (one per detected dependency).
// A nil or empty resource slice yields a non-nil empty slice.
func DetectCommonDependencies(resources []*types.ProcessedResource) []helm.Dependency {
	if len(resources) == 0 {
		return []helm.Dependency{}
	}

	detected := make(map[string]bool)

	for _, resource := range resources {
		if resource == nil || resource.Original == nil || resource.Original.Object == nil {
			continue
		}
		containers := extractContainers(resource)
		for _, container := range containers {
			envNames, envValues := extractEnvVars(container)
			image := extractImage(container)
			ports := extractPorts(container)

			for i := range knownDependencies {
				dep := &knownDependencies[i]
				if detected[dep.Name] {
					continue
				}
				if matchesDependency(dep, envNames, envValues, image, ports) {
					detected[dep.Name] = true
				}
			}
		}
	}

	result := make([]helm.Dependency, 0, len(detected))
	// Iterate knownDependencies to preserve a stable ordering.
	for i := range knownDependencies {
		dep := &knownDependencies[i]
		if detected[dep.Name] {
			result = append(result, helm.Dependency{
				Name:       dep.Name,
				Version:    dep.Version,
				Repository: bitnamiRepo,
				Condition:  dep.Name + ".enabled",
			})
		}
	}

	return result
}

// FilterExistingDependencies returns only those entries from detected whose Name
// does not appear in existing.
func FilterExistingDependencies(detected []helm.Dependency, existing []helm.Dependency) []helm.Dependency {
	existingNames := make(map[string]bool, len(existing))
	for _, e := range existing {
		existingNames[e.Name] = true
	}

	filtered := make([]helm.Dependency, 0, len(detected))
	for _, d := range detected {
		if !existingNames[d.Name] {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

// InjectDependencies appends a dependencies section to the chart's ChartYAML and
// adds condition values ("<name>.enabled: false") to ValuesYAML for each
// dependency. Returns nil if chart is nil.
func InjectDependencies(chart *types.GeneratedChart, deps []helm.Dependency) *types.GeneratedChart {
	if chart == nil {
		return nil
	}

	// Build the dependencies YAML block.
	var depYAML strings.Builder
	if len(deps) > 0 {
		depYAML.WriteString("\ndependencies:\n")
		for _, d := range deps {
			depYAML.WriteString(fmt.Sprintf("  - name: %s\n", d.Name))
			depYAML.WriteString(fmt.Sprintf("    version: %s\n", d.Version))
			depYAML.WriteString(fmt.Sprintf("    repository: %s\n", d.Repository))
			if d.Condition != "" {
				depYAML.WriteString(fmt.Sprintf("    condition: %s\n", d.Condition))
			}
		}
	}

	// Build the condition values block.
	var valuesBlock strings.Builder
	for _, d := range deps {
		valuesBlock.WriteString(fmt.Sprintf("\n%s:\n  enabled: false\n", d.Name))
	}

	return &types.GeneratedChart{
		Name:          chart.Name,
		Path:          chart.Path,
		ChartYAML:     chart.ChartYAML + depYAML.String(),
		ValuesYAML:    chart.ValuesYAML + valuesBlock.String(),
		Templates:     chart.Templates,
		Helpers:       chart.Helpers,
		Notes:         chart.Notes,
		ValuesSchema:  chart.ValuesSchema,
		ExternalFiles: chart.ExternalFiles,
	}
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// extractEnvVars returns the environment variable names and values from a
// container map. Names are upper-cased for consistent matching.
func extractEnvVars(container map[string]interface{}) (names []string, values []string) {
	rawEnv, ok := container["env"].([]interface{})
	if !ok {
		return nil, nil
	}

	for _, e := range rawEnv {
		envMap, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		if name, ok := envMap["name"].(string); ok {
			names = append(names, strings.ToUpper(name))
		}
		if value, ok := envMap["value"].(string); ok {
			values = append(values, strings.ToLower(value))
		}
	}
	return names, values
}

// extractImage returns the container image string (e.g. "postgres:15").
func extractImage(container map[string]interface{}) string {
	image, _ := container["image"].(string)
	return strings.ToLower(image)
}

// extractPorts returns the list of container ports as int64 values.
func extractPorts(container map[string]interface{}) []int64 {
	rawPorts, ok := container["ports"].([]interface{})
	if !ok {
		return nil
	}

	ports := make([]int64, 0, len(rawPorts))
	for _, p := range rawPorts {
		portMap, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if cp, ok := portMap["containerPort"].(int64); ok {
			ports = append(ports, cp)
		}
	}
	return ports
}

// matchesDependency checks whether any of the provided signals (env names,
// env values, image, ports) match a known dependency definition.
func matchesDependency(dep *knownDependency, envNames []string, envValues []string, image string, ports []int64) bool {
	// Check env var name prefixes.
	for _, envName := range envNames {
		for _, prefix := range dep.EnvPrefixes {
			if strings.HasPrefix(envName, prefix) {
				return true
			}
		}
		for _, exact := range dep.EnvExact {
			if envName == strings.ToUpper(exact) {
				return true
			}
		}
	}

	// Check env var values for known substrings (e.g. DATABASE_URL containing "postgres").
	for _, val := range envValues {
		for _, substr := range dep.EnvContains {
			if strings.Contains(val, substr) {
				return true
			}
		}
	}

	// Check image name.
	if image != "" {
		for _, imgPattern := range dep.Images {
			// Match the image name portion (before the colon tag separator and
			// after any registry prefix slash).
			imageName := image
			if idx := strings.LastIndex(imageName, "/"); idx >= 0 {
				imageName = imageName[idx+1:]
			}
			if idx := strings.Index(imageName, ":"); idx >= 0 {
				imageName = imageName[:idx]
			}
			if imageName == imgPattern {
				return true
			}
		}
	}

	// Check ports.
	for _, port := range ports {
		for _, knownPort := range dep.Ports {
			if port == knownPort {
				return true
			}
		}
	}

	return false
}
