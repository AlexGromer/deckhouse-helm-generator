// Package generator provides interfaces and implementations for generating Helm charts.
package generator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/helm"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor/value"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// Options configures the generator behavior.
type Options struct {
	// OutputDir is the directory where charts will be generated.
	OutputDir string

	// ChartName is the name of the chart.
	ChartName string

	// ChartVersion is the chart version.
	ChartVersion string

	// AppVersion is the application version.
	AppVersion string

	// Mode is the output mode (universal, separate, library).
	Mode types.OutputMode

	// Namespace is the default namespace for resources.
	Namespace string

	// IncludeTests generates test templates.
	IncludeTests bool

	// IncludeREADME generates README.md.
	IncludeREADME bool

	// IncludeSchema generates values.schema.json.
	IncludeSchema bool

	// EnvValues enables generation of environment-specific values overrides
	// (values-dev.yaml, values-staging.yaml, values-prod.yaml).
	EnvValues bool

	// ExternalFileManager manages external files for the chart.
	ExternalFileManager *value.ExternalFileManager

	// DeckhouseModule enables Deckhouse module scaffold generation
	// (openapi/, images/, hooks/). The deckhouse_lib_helm dependency is added
	// only when a template or helper uses a helm_lib_* helper.
	DeckhouseModule bool

	// ValuesFlat generates values.yaml with inline dot-notation path comments
	// for easier --set reference (e.g., "# image.repository").
	ValuesFlat bool

	// IncludeHooks generates Helm lifecycle hook Job templates
	// (pre-upgrade, post-install, pre-delete).
	IncludeHooks bool

	// LibraryRef (library mode) is where the library chart comes from instead
	// of being vendored into the service charts, which then declare it as a
	// dependency on that repository, at LibraryVersion:
	// oci://registry.example.com/platform/platform-lib (an OCI registry),
	// file://../library (a chart directory, relative to the service chart or
	// absolute) or https://charts.example.com/stable (a chart repository,
	// needs LibraryName).
	LibraryRef string

	// LibraryVersion is the version or version range of the library dependency.
	// Required with LibraryRef except for file://, where the default is the
	// version in the Chart.yaml at that path, else the version of the library
	// chart this run writes.
	LibraryVersion string

	// LibraryName is the chart name of the library for a LibraryRef that does
	// not carry one: required for an http(s):// chart repository, optional
	// for file:// (the name in the Chart.yaml there wins, and must agree).
	LibraryName string

	// SharedChart (separate and library modes) is the name of a chart that
	// receives the objects several service charts reference, and those no
	// workload references (see SplitShared). Empty: every object stays in the
	// chart of its service group.
	SharedChart string

	// SharedKeep (with SharedChart) lists "Kind/name" objects that stay with
	// a service group even when the rule moves them to the shared chart. One
	// referenced by several groups stays with the first of them in sorted
	// order.
	SharedKeep []string

	// SharedExclude (with SharedChart) lists "Kind/name" objects that are
	// never moved: they stay where grouping put them.
	SharedExclude []string

	// OnSharedNote, when set, receives the observations about SharedKeep and
	// SharedExclude entries and about objects of the shared chart.
	OnSharedNote func(note string)

	// OnShared, when set, is called by a generator that honours SharedChart
	// with the shared chart's name and the objects that moved (none: no chart
	// is written).
	OnShared func(chart string, moved []SharedObject)
}

// Generator generates Helm charts from a resource graph.
type Generator interface {
	// Generate creates Helm chart(s) from the resource graph.
	Generate(ctx context.Context, graph *types.ResourceGraph, opts Options) ([]*types.GeneratedChart, error)

	// Mode returns the output mode this generator supports.
	Mode() types.OutputMode
}

// BaseGenerator provides common functionality for generators.
type BaseGenerator struct {
	mode types.OutputMode
}

// NewBaseGenerator creates a new base generator.
func NewBaseGenerator(mode types.OutputMode) BaseGenerator {
	return BaseGenerator{
		mode: mode,
	}
}

// Mode returns the generator mode.
func (g BaseGenerator) Mode() types.OutputMode {
	return g.mode
}

// WriteChart writes a generated chart to disk.
func WriteChart(chart *types.GeneratedChart, outputDir string) error {
	chartDir := filepath.Join(outputDir, chart.Name)

	// Create chart directory structure
	dirs := []string{
		chartDir,
		filepath.Join(chartDir, "templates"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Write Chart.yaml
	if err := os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(chart.ChartYAML), 0644); err != nil {
		return fmt.Errorf("failed to write Chart.yaml: %w", err)
	}

	// Write values.yaml
	if err := os.WriteFile(filepath.Join(chartDir, "values.yaml"), []byte(chart.ValuesYAML), 0644); err != nil {
		return fmt.Errorf("failed to write values.yaml: %w", err)
	}

	// Write templates in sorted order for deterministic output.
	templatePaths := make([]string, 0, len(chart.Templates))
	for path := range chart.Templates {
		templatePaths = append(templatePaths, path)
	}
	sort.Strings(templatePaths)
	for _, path := range templatePaths {
		content := chart.Templates[path]
		templatePath := filepath.Join(chartDir, path)
		if err := os.MkdirAll(filepath.Dir(templatePath), 0755); err != nil {
			return fmt.Errorf("failed to create template directory for %s: %w", path, err)
		}
		if err := os.WriteFile(templatePath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write template %s: %w", path, err)
		}
	}

	// Write _helpers.tpl
	if chart.Helpers != "" {
		helpersPath := filepath.Join(chartDir, "templates", "_helpers.tpl")
		if err := os.WriteFile(helpersPath, []byte(chart.Helpers), 0644); err != nil {
			return fmt.Errorf("failed to write _helpers.tpl: %w", err)
		}
	}

	// Write NOTES.txt
	if chart.Notes != "" {
		notesPath := filepath.Join(chartDir, "templates", "NOTES.txt")
		if err := os.WriteFile(notesPath, []byte(chart.Notes), 0644); err != nil {
			return fmt.Errorf("failed to write NOTES.txt: %w", err)
		}
	}

	// Write values.schema.json if present
	if chart.ValuesSchema != "" {
		schemaPath := filepath.Join(chartDir, "values.schema.json")
		if err := os.WriteFile(schemaPath, []byte(chart.ValuesSchema), 0644); err != nil {
			return fmt.Errorf("failed to write values.schema.json: %w", err)
		}
	}

	// Write .helmignore
	helmignorePath := filepath.Join(chartDir, ".helmignore")
	if err := os.WriteFile(helmignorePath, []byte(helm.GenerateHelmIgnore()), 0644); err != nil {
		return fmt.Errorf("failed to write .helmignore: %w", err)
	}

	// Write external files
	if len(chart.ExternalFiles) > 0 {
		absChartDir, err := filepath.Abs(chartDir)
		if err != nil {
			return fmt.Errorf("failed to resolve chart directory: %w", err)
		}
		for _, file := range chart.ExternalFiles {
			filePath := filepath.Join(chartDir, file.Path)
			absFilePath, err := filepath.Abs(filePath)
			if err != nil {
				return fmt.Errorf("failed to resolve external file path %s: %w", file.Path, err)
			}
			rel, err := filepath.Rel(absChartDir, absFilePath)
			if err != nil || strings.HasPrefix(rel, "..") {
				return fmt.Errorf("invalid external file path %q: outside chart directory", file.Path)
			}
			if err := os.MkdirAll(filepath.Dir(absFilePath), 0755); err != nil {
				return fmt.Errorf("failed to create directory for external file %s: %w", file.Path, err)
			}
			// Shell scripts (post-renderer, image mirroring) must be executable.
			mode := os.FileMode(0644)
			if strings.HasSuffix(file.Path, ".sh") {
				mode = 0755
			}
			if err := os.WriteFile(absFilePath, []byte(file.Content), mode); err != nil {
				return fmt.Errorf("failed to write external file %s: %w", file.Path, err)
			}
		}
	}

	return nil
}

// Registry manages generator registration.
type Registry struct {
	generators map[types.OutputMode]Generator
}

// NewRegistry creates a new generator registry.
func NewRegistry() *Registry {
	return &Registry{
		generators: make(map[types.OutputMode]Generator),
	}
}

// Register adds a generator to the registry.
func (r *Registry) Register(g Generator) {
	r.generators[g.Mode()] = g
}

// Get returns a generator by mode.
func (r *Registry) Get(mode types.OutputMode) (Generator, error) {
	g, ok := r.generators[mode]
	if !ok {
		return nil, fmt.Errorf("no generator registered for mode %s", mode)
	}
	return g, nil
}

// DefaultRegistry returns a registry with all default generators.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(NewUniversalGenerator())
	r.Register(NewSeparateGenerator())
	r.Register(NewLibraryGenerator())
	r.Register(NewUmbrellaGenerator())
	return r
}
