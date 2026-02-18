// Package integration provides a test framework for end-to-end integration tests
// of the Deckhouse Helm Generator pipeline.
package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sigsyaml "sigs.k8s.io/yaml"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/analyzer"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/analyzer/detector"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/extractor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/generator"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor/k8s"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// TestHarness manages test lifecycle including temporary directories and cleanup.
type TestHarness struct {
	T         *testing.T
	TempDir   string
	OutputDir string
	InputDir  string
}

// NewTestHarness creates a new TestHarness associated with the given test.
func NewTestHarness(t *testing.T) *TestHarness {
	t.Helper()
	return &TestHarness{
		T: t,
	}
}

// Setup creates the temporary directory structure required for a test run.
// It creates TempDir, OutputDir (under TempDir), and InputDir (under TempDir).
func (h *TestHarness) Setup() {
	h.T.Helper()

	tmpDir, err := os.MkdirTemp("", "dhg-integration-*")
	if err != nil {
		h.T.Fatalf("failed to create temp dir: %v", err)
	}

	h.TempDir = tmpDir
	h.OutputDir = filepath.Join(tmpDir, "output")
	h.InputDir = filepath.Join(tmpDir, "input")

	if err := os.MkdirAll(h.OutputDir, 0755); err != nil {
		h.T.Fatalf("failed to create output dir: %v", err)
	}
	if err := os.MkdirAll(h.InputDir, 0755); err != nil {
		h.T.Fatalf("failed to create input dir: %v", err)
	}
}

// Cleanup removes all temporary directories created by Setup.
// Call this via t.Cleanup or defer after Setup.
func (h *TestHarness) Cleanup() {
	if h.TempDir != "" {
		if err := os.RemoveAll(h.TempDir); err != nil {
			h.T.Logf("warning: failed to remove temp dir %s: %v", h.TempDir, err)
		}
	}
}

// WriteInputFile writes the given YAML content to a file in InputDir.
// The filename parameter should include the .yaml or .yml extension.
func (h *TestHarness) WriteInputFile(filename, content string) {
	h.T.Helper()

	path := filepath.Join(h.InputDir, filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		h.T.Fatalf("failed to create parent dir for %s: %v", filename, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		h.T.Fatalf("failed to write input file %s: %v", filename, err)
	}
}

// PipelineOptions configures pipeline execution parameters.
type PipelineOptions struct {
	// ChartName is the name used for the generated Helm chart.
	ChartName string

	// ChartVersion is the semantic version of the chart (e.g., "1.0.0").
	ChartVersion string

	// AppVersion is the application version embedded in the chart metadata.
	AppVersion string

	// Namespace is the default Kubernetes namespace for resources.
	Namespace string

	// Mode is the output mode (universal, separate, library, umbrella).
	// Defaults to universal if empty.
	Mode types.OutputMode
}

// ChartOutput holds the result of a full pipeline execution.
type ChartOutput struct {
	// Charts is the list of generated Helm charts.
	Charts []*types.GeneratedChart

	// Graph is the resource relationship graph built during analysis.
	Graph *types.ResourceGraph

	// Resources is the list of processed resources after the processor stage.
	Resources []*types.ProcessedResource

	// OutputDir is the directory where charts were written to disk.
	OutputDir string
}

// ExecutePipeline runs the full Extract → Process → Analyze → Generate pipeline
// against the YAML files found in inputDir, using the provided options.
//
// It returns a ChartOutput containing all intermediate and final results,
// or an error if any stage fails.
func ExecutePipeline(inputDir string, opts PipelineOptions) (*ChartOutput, error) {
	ctx := context.Background()

	// ── Stage 1: Extract ──────────────────────────────────────────────────────

	fileExtractor := extractor.NewFileExtractor()

	extractOpts := extractor.Options{
		Paths:     []string{inputDir},
		Recursive: true,
	}

	resourceCh, errCh := fileExtractor.Extract(ctx, extractOpts)

	var extractedResources []*types.ExtractedResource
	var extractErrors []error

	// Drain both channels. Errors are buffered and reported together so that
	// partial results from other files are still collected.
	for {
		select {
		case res, ok := <-resourceCh:
			if !ok {
				resourceCh = nil
			} else {
				extractedResources = append(extractedResources, res)
			}
		case err, ok := <-errCh:
			if !ok {
				errCh = nil
			} else {
				extractErrors = append(extractErrors, err)
			}
		}

		if resourceCh == nil && errCh == nil {
			break
		}
	}

	if len(extractErrors) > 0 {
		msgs := make([]string, len(extractErrors))
		for i, e := range extractErrors {
			msgs[i] = e.Error()
		}
		return nil, fmt.Errorf("extraction errors: %s", strings.Join(msgs, "; "))
	}

	// ── Stage 2: Process ──────────────────────────────────────────────────────

	registry := processor.NewRegistry()
	k8s.RegisterAll(registry)

	chartName := opts.ChartName
	if chartName == "" {
		chartName = "chart"
	}

	procCtx := processor.Context{
		Ctx:        ctx,
		ChartName:  chartName,
		OutputMode: types.OutputModeUniversal,
	}

	var processedResources []*types.ProcessedResource

	for _, extracted := range extractedResources {
		result, err := registry.Process(procCtx, extracted.Object)
		if err != nil {
			return nil, fmt.Errorf("processing resource %s: %w",
				extracted.ResourceKey().String(), err)
		}
		if result == nil {
			continue
		}

		processed := &types.ProcessedResource{
			Original:        extracted,
			ServiceName:     result.ServiceName,
			TemplatePath:    result.TemplatePath,
			TemplateContent: result.TemplateContent,
			ValuesPath:      result.ValuesPath,
			Values:          result.Values,
			Dependencies:    result.Dependencies,
		}
		processedResources = append(processedResources, processed)
	}

	// ── Stage 3: Analyze ──────────────────────────────────────────────────────

	defaultAnalyzer := analyzer.NewDefaultAnalyzer()
	detector.RegisterAll(defaultAnalyzer)

	graph, err := defaultAnalyzer.Analyze(ctx, processedResources)
	if err != nil {
		return nil, fmt.Errorf("analyzing resources: %w", err)
	}

	// ── Stage 4: Generate ─────────────────────────────────────────────────────

	chartVersion := opts.ChartVersion
	if chartVersion == "" {
		chartVersion = "0.1.0"
	}

	appVersion := opts.AppVersion
	if appVersion == "" {
		appVersion = "latest"
	}

	outputDir, err := os.MkdirTemp("", "dhg-output-*")
	if err != nil {
		return nil, fmt.Errorf("creating output dir: %w", err)
	}

	genOpts := generator.Options{
		OutputDir:    outputDir,
		ChartName:    chartName,
		ChartVersion: chartVersion,
		AppVersion:   appVersion,
		Mode:         types.OutputModeUniversal,
		Namespace:    opts.Namespace,
	}

	gen := generator.NewUniversalGenerator()

	charts, err := gen.Generate(ctx, graph, genOpts)
	if err != nil {
		return nil, fmt.Errorf("generating charts: %w", err)
	}

	// Write each chart to disk.
	for _, chart := range charts {
		if err := generator.WriteChart(chart, outputDir); err != nil {
			return nil, fmt.Errorf("writing chart %s: %w", chart.Name, err)
		}
	}

	return &ChartOutput{
		Charts:    charts,
		Graph:     graph,
		Resources: processedResources,
		OutputDir: outputDir,
	}, nil
}

// ValidateChartStructure checks that a Helm chart directory contains the
// mandatory files: Chart.yaml, values.yaml, and at least one file inside
// templates/.
func ValidateChartStructure(t *testing.T, chartDir string) {
	t.Helper()

	required := []string{
		"Chart.yaml",
		"values.yaml",
	}

	for _, name := range required {
		path := filepath.Join(chartDir, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("required file missing in chart dir %s: %s (%v)", chartDir, name, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("required file is empty in chart dir %s: %s", chartDir, name)
		}
	}

	templatesDir := filepath.Join(chartDir, "templates")
	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		t.Errorf("templates/ directory missing or unreadable in %s: %v", chartDir, err)
		return
	}
	if len(entries) == 0 {
		t.Errorf("templates/ directory is empty in %s", chartDir)
	}
}

// ValidateValues checks that the given string is valid YAML. It is intended
// for validating the content of a generated values.yaml.
func ValidateValues(t *testing.T, valuesContent string) {
	t.Helper()

	if strings.TrimSpace(valuesContent) == "" {
		t.Error("values content is empty")
		return
	}

	var out interface{}
	if err := sigsyaml.Unmarshal([]byte(valuesContent), &out); err != nil {
		t.Errorf("values content is not valid YAML: %v", err)
	}
}

// ValidateTemplates checks that all files inside chartDir/templates/ exist and
// are non-empty. The templates/ directory itself must also be present.
func ValidateTemplates(t *testing.T, chartDir string) {
	t.Helper()

	templatesDir := filepath.Join(chartDir, "templates")
	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		t.Errorf("cannot read templates/ dir in %s: %v", chartDir, err)
		return
	}
	if len(entries) == 0 {
		t.Errorf("templates/ dir is empty in %s", chartDir)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(templatesDir, entry.Name())
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("cannot stat template file %s: %v", path, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("template file is empty: %s", path)
		}
	}
}

// CompareGeneratedChart compares two GeneratedChart values and returns a list
// of human-readable difference descriptions. An empty slice means the charts
// are equivalent.
func CompareGeneratedChart(actual, expected *types.GeneratedChart) []string {
	var diffs []string

	if actual.Name != expected.Name {
		diffs = append(diffs, fmt.Sprintf("Name: got %q, want %q",
			actual.Name, expected.Name))
	}

	if actual.ChartYAML != expected.ChartYAML {
		diffs = append(diffs, fmt.Sprintf("ChartYAML differs:\n--- expected ---\n%s\n--- actual ---\n%s",
			expected.ChartYAML, actual.ChartYAML))
	}

	if actual.ValuesYAML != expected.ValuesYAML {
		diffs = append(diffs, fmt.Sprintf("ValuesYAML differs:\n--- expected ---\n%s\n--- actual ---\n%s",
			expected.ValuesYAML, actual.ValuesYAML))
	}

	if actual.Helpers != expected.Helpers {
		diffs = append(diffs, fmt.Sprintf("Helpers content differs (len: got %d, want %d)",
			len(actual.Helpers), len(expected.Helpers)))
	}

	if actual.Notes != expected.Notes {
		diffs = append(diffs, fmt.Sprintf("Notes content differs (len: got %d, want %d)",
			len(actual.Notes), len(expected.Notes)))
	}

	// Compare template counts.
	if len(actual.Templates) != len(expected.Templates) {
		diffs = append(diffs, fmt.Sprintf("Templates count: got %d, want %d",
			len(actual.Templates), len(expected.Templates)))
	}

	// Compare individual templates that exist in expected.
	for path, expectedContent := range expected.Templates {
		actualContent, ok := actual.Templates[path]
		if !ok {
			diffs = append(diffs, fmt.Sprintf("template %q missing from actual chart", path))
			continue
		}
		if actualContent != expectedContent {
			diffs = append(diffs, fmt.Sprintf("template %q content differs:\n--- expected ---\n%s\n--- actual ---\n%s",
				path, expectedContent, actualContent))
		}
	}

	// Report templates present in actual but not in expected.
	for path := range actual.Templates {
		if _, ok := expected.Templates[path]; !ok {
			diffs = append(diffs, fmt.Sprintf("unexpected template %q in actual chart", path))
		}
	}

	return diffs
}

// ExecutePipelineWithMode runs the full pipeline with a specific output mode.
func ExecutePipelineWithMode(inputDir string, opts PipelineOptions) (*ChartOutput, error) {
	ctx := context.Background()

	// ── Stage 1: Extract ──
	fileExtractor := extractor.NewFileExtractor()
	extractOpts := extractor.Options{
		Paths:     []string{inputDir},
		Recursive: true,
	}

	resourceCh, errCh := fileExtractor.Extract(ctx, extractOpts)

	var extractedResources []*types.ExtractedResource
	var extractErrors []error

	for {
		select {
		case res, ok := <-resourceCh:
			if !ok {
				resourceCh = nil
			} else {
				extractedResources = append(extractedResources, res)
			}
		case err, ok := <-errCh:
			if !ok {
				errCh = nil
			} else {
				extractErrors = append(extractErrors, err)
			}
		}
		if resourceCh == nil && errCh == nil {
			break
		}
	}

	if len(extractErrors) > 0 {
		msgs := make([]string, len(extractErrors))
		for i, e := range extractErrors {
			msgs[i] = e.Error()
		}
		return nil, fmt.Errorf("extraction errors: %s", strings.Join(msgs, "; "))
	}

	// ── Stage 2: Process ──
	registry := processor.NewRegistry()
	k8s.RegisterAll(registry)

	chartName := opts.ChartName
	if chartName == "" {
		chartName = "chart"
	}

	mode := opts.Mode
	if mode == "" {
		mode = types.OutputModeUniversal
	}

	procCtx := processor.Context{
		Ctx:        ctx,
		ChartName:  chartName,
		OutputMode: mode,
	}

	var processedResources []*types.ProcessedResource
	for _, extracted := range extractedResources {
		result, err := registry.Process(procCtx, extracted.Object)
		if err != nil {
			return nil, fmt.Errorf("processing resource %s: %w",
				extracted.ResourceKey().String(), err)
		}
		if result == nil {
			continue
		}
		processed := &types.ProcessedResource{
			Original:        extracted,
			ServiceName:     result.ServiceName,
			TemplatePath:    result.TemplatePath,
			TemplateContent: result.TemplateContent,
			ValuesPath:      result.ValuesPath,
			Values:          result.Values,
			Dependencies:    result.Dependencies,
		}
		processedResources = append(processedResources, processed)
	}

	// ── Stage 3: Analyze ──
	defaultAnalyzer := analyzer.NewDefaultAnalyzer()
	detector.RegisterAll(defaultAnalyzer)

	graph, err := defaultAnalyzer.Analyze(ctx, processedResources)
	if err != nil {
		return nil, fmt.Errorf("analyzing resources: %w", err)
	}

	// ── Stage 4: Generate ──
	chartVersion := opts.ChartVersion
	if chartVersion == "" {
		chartVersion = "0.1.0"
	}
	appVersion := opts.AppVersion
	if appVersion == "" {
		appVersion = "latest"
	}

	outputDir, err := os.MkdirTemp("", "dhg-output-*")
	if err != nil {
		return nil, fmt.Errorf("creating output dir: %w", err)
	}

	genOpts := generator.Options{
		OutputDir:    outputDir,
		ChartName:    chartName,
		ChartVersion: chartVersion,
		AppVersion:   appVersion,
		Mode:         mode,
		Namespace:    opts.Namespace,
	}

	// Select generator based on mode.
	genRegistry := generator.DefaultRegistry()
	gen, err := genRegistry.Get(mode)
	if err != nil {
		return nil, fmt.Errorf("getting generator for mode %s: %w", mode, err)
	}

	charts, err := gen.Generate(ctx, graph, genOpts)
	if err != nil {
		return nil, fmt.Errorf("generating charts: %w", err)
	}

	// Write each chart to disk.
	for _, chart := range charts {
		if err := generator.WriteChart(chart, outputDir); err != nil {
			return nil, fmt.Errorf("writing chart %s: %w", chart.Name, err)
		}
	}

	return &ChartOutput{
		Charts:    charts,
		Graph:     graph,
		Resources: processedResources,
		OutputDir: outputDir,
	}, nil
}
