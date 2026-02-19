package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/analyzer"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/analyzer/detector"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/analyzer/pattern"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/extractor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/generator"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor/k8s"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor/value"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Fprintln(os.Stderr, "\nReceived interrupt signal, shutting down...")
		cancel()
	}()

	// Execute root command
	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "dhg",
		Short: "Deckhouse Helm Generator",
		Long: `Deckhouse Helm Generator (DHG) is a CLI tool for generating Helm charts
from Kubernetes resources with automatic relationship detection.

It supports extracting resources from:
  - YAML files
  - Live Kubernetes clusters
  - GitOps repositories`,
		Version: fmt.Sprintf("%s (built: %s)", version, buildTime),
	}

	rootCmd.AddCommand(newGenerateCmd())
	rootCmd.AddCommand(newAnalyzeCmd())
	rootCmd.AddCommand(newVersionCmd())

	return rootCmd
}

func newGenerateCmd() *cobra.Command {
	var (
		paths          []string
		outputDir      string
		chartName      string
		chartVersion   string
		appVersion     string
		mode           string
		source         string
		namespace      string
		namespaces     []string
		labelSelector  string
		includeKinds   []string
		excludeKinds   []string
		recursive      bool
		kubeConfig     string
		kubeContext    string
		includeTests   bool
		includeREADME  bool
		includeSchema  bool
		verbose        bool
		envValues      bool
		deckhouseModule bool
	)

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate Helm chart from Kubernetes resources",
		Long: `Generate Helm chart from Kubernetes resources.

Examples:
  # Generate from YAML files
  dhg generate -f ./manifests -o ./chart --chart-name myapp

  # Generate from live cluster
  dhg generate -s cluster -n production --kubeconfig ~/.kube/config

  # Generate with filtering
  dhg generate -f ./manifests --include-kinds Deployment,Service,Ingress`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGenerate(cmd.Context(), generateOptions{
				paths:         paths,
				outputDir:     outputDir,
				chartName:     chartName,
				chartVersion:  chartVersion,
				appVersion:    appVersion,
				mode:          mode,
				source:        source,
				namespace:     namespace,
				namespaces:    namespaces,
				labelSelector: labelSelector,
				includeKinds:  includeKinds,
				excludeKinds:  excludeKinds,
				recursive:     recursive,
				kubeConfig:    kubeConfig,
				kubeContext:   kubeContext,
				includeTests:  includeTests,
				includeREADME: includeREADME,
				includeSchema: includeSchema,
				verbose:       verbose,
				envValues:       envValues,
				deckhouseModule: deckhouseModule,
			})
		},
	}

	cmd.Flags().StringSliceVarP(&paths, "file", "f", []string{}, "Path(s) to YAML files or directories")
	cmd.Flags().StringVarP(&outputDir, "output", "o", "./chart", "Output directory for the chart")
	cmd.Flags().StringVar(&chartName, "chart-name", "", "Name of the chart (required)")
	cmd.Flags().StringVar(&chartVersion, "chart-version", "0.1.0", "Chart version")
	cmd.Flags().StringVar(&appVersion, "app-version", "1.0.0", "Application version")
	cmd.Flags().StringVar(&mode, "mode", "universal", "Output mode: universal, separate, library, umbrella")
	cmd.Flags().StringVarP(&source, "source", "s", "file", "Source type: file, cluster, gitops")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Filter by namespace")
	cmd.Flags().StringSliceVar(&namespaces, "namespaces", []string{}, "Filter by multiple namespaces")
	cmd.Flags().StringVarP(&labelSelector, "selector", "l", "", "Label selector filter")
	cmd.Flags().StringSliceVar(&includeKinds, "include-kinds", []string{}, "Include only these resource kinds")
	cmd.Flags().StringSliceVar(&excludeKinds, "exclude-kinds", []string{}, "Exclude these resource kinds")
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", true, "Recursively scan directories")
	cmd.Flags().StringVar(&kubeConfig, "kubeconfig", "", "Path to kubeconfig file")
	cmd.Flags().StringVar(&kubeContext, "context", "", "Kubeconfig context to use")
	cmd.Flags().BoolVar(&includeTests, "include-tests", false, "Generate test templates")
	cmd.Flags().BoolVar(&includeREADME, "include-readme", true, "Generate README.md")
	cmd.Flags().BoolVar(&includeSchema, "include-schema", false, "Generate values.schema.json")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")
	cmd.Flags().BoolVar(&envValues, "env-values", false, "Generate environment-specific values (dev/staging/prod)")
	cmd.Flags().BoolVar(&deckhouseModule, "deckhouse-module", false, "Generate Deckhouse module scaffold (helm_lib, openapi/, images/, hooks/)")

	cmd.MarkFlagRequired("chart-name")

	return cmd
}

type generateOptions struct {
	paths         []string
	outputDir     string
	chartName     string
	chartVersion  string
	appVersion    string
	mode          string
	source        string
	namespace     string
	namespaces    []string
	labelSelector string
	includeKinds  []string
	excludeKinds  []string
	recursive     bool
	kubeConfig    string
	kubeContext   string
	includeTests  bool
	includeREADME bool
	includeSchema bool
	verbose         bool
	envValues       bool
	deckhouseModule bool
}

func runGenerate(ctx context.Context, opts generateOptions) error {
	if opts.verbose {
		fmt.Printf("Starting chart generation...\n")
		fmt.Printf("Chart name: %s\n", opts.chartName)
		fmt.Printf("Output directory: %s\n", opts.outputDir)
		fmt.Printf("Mode: %s\n", opts.mode)
	}

	// Validate output mode
	var outputMode types.OutputMode
	switch opts.mode {
	case "universal":
		outputMode = types.OutputModeUniversal
	case "separate":
		outputMode = types.OutputModeSeparate
	case "library":
		outputMode = types.OutputModeLibrary
	case "umbrella":
		outputMode = types.OutputModeUmbrella
	default:
		return fmt.Errorf("invalid mode: %s (must be universal, separate, library, or umbrella)", opts.mode)
	}

	// Validate source
	var sourceType types.Source
	switch opts.source {
	case "file":
		sourceType = types.SourceFile
		if len(opts.paths) == 0 {
			return fmt.Errorf("at least one path is required for file source (-f flag)")
		}
	case "cluster":
		sourceType = types.SourceCluster
	case "gitops":
		sourceType = types.SourceGitOps
	default:
		return fmt.Errorf("invalid source: %s (must be file, cluster, or gitops)", opts.source)
	}

	// Step 1: Extract resources
	if opts.verbose {
		fmt.Printf("\n[1/5] Extracting resources from source...\n")
	}

	extractorRegistry := extractor.DefaultRegistry()
	ext, ok := extractorRegistry.Get(sourceType)
	if !ok {
		return fmt.Errorf("no extractor available for source type: %s", sourceType)
	}

	extractOpts := extractor.Options{
		Paths:         opts.paths,
		Namespace:     opts.namespace,
		Namespaces:    opts.namespaces,
		LabelSelector: opts.labelSelector,
		IncludeKinds:  opts.includeKinds,
		ExcludeKinds:  opts.excludeKinds,
		Recursive:     opts.recursive,
		KubeConfig:    opts.kubeConfig,
		KubeContext:   opts.kubeContext,
	}

	if err := ext.Validate(ctx, extractOpts); err != nil {
		return fmt.Errorf("extractor validation failed: %w", err)
	}

	resourceChan, errChan := ext.Extract(ctx, extractOpts)

	var extractedResources []*types.ExtractedResource
	extractErrors := make([]error, 0)

	for {
		select {
		case resource, ok := <-resourceChan:
			if !ok {
				resourceChan = nil
				if errChan == nil {
					goto done
				}
				continue
			}
			extractedResources = append(extractedResources, resource)
			if opts.verbose {
				fmt.Printf("  Extracted: %s\n", resource.ResourceKey().String())
			}
		case err, ok := <-errChan:
			if !ok {
				errChan = nil
				if resourceChan == nil {
					goto done
				}
				continue
			}
			extractErrors = append(extractErrors, err)
			fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
done:

	if len(extractedResources) == 0 {
		return fmt.Errorf("no resources extracted")
	}

	if opts.verbose {
		fmt.Printf("  Total extracted: %d resources\n", len(extractedResources))
		if len(extractErrors) > 0 {
			fmt.Printf("  Warnings: %d\n", len(extractErrors))
		}
	}

	// Step 2: Process resources
	if opts.verbose {
		fmt.Printf("\n[2/5] Processing resources...\n")
	}

	processorRegistry := processor.NewRegistry()
	k8s.RegisterAll(processorRegistry)

	// Initialize value processor and external file manager
	valueProcessor := value.DefaultProcessor()
	externalFileManager := value.NewExternalFileManager()

	var processedResources []*types.ProcessedResource
	allResourcesMap := make(map[types.ResourceKey]*types.ExtractedResource)
	for _, r := range extractedResources {
		allResourcesMap[r.ResourceKey()] = r
	}

	for _, extracted := range extractedResources {
		if err := ctx.Err(); err != nil {
			return err
		}
		procCtx := processor.Context{
			Ctx:                 ctx,
			ChartName:           opts.chartName,
			OutputMode:          outputMode,
			Namespace:           extracted.Object.GetNamespace(),
			AllResources:        allResourcesMap,
			ExternalFileManager: externalFileManager,
			ValueProcessor:      valueProcessor,
		}

		result, err := processorRegistry.Process(procCtx, extracted.Object)
		if err != nil {
			return fmt.Errorf("failed to process %s: %w", extracted.ResourceKey().String(), err)
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

		if opts.verbose {
			fmt.Printf("  Processed: %s -> service: %s\n", extracted.ResourceKey().String(), result.ServiceName)
		}
	}

	if opts.verbose {
		fmt.Printf("  Total processed: %d resources\n", len(processedResources))
	}

	// Step 3: Analyze relationships
	if opts.verbose {
		fmt.Printf("\n[3/5] Analyzing relationships...\n")
	}

	analyzer := analyzer.NewDefaultAnalyzer()
	detector.RegisterAll(analyzer)

	graph, err := analyzer.Analyze(ctx, processedResources)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	if opts.verbose {
		fmt.Printf("  Detected relationships: %d\n", len(graph.Relationships))
		fmt.Printf("  Service groups: %d\n", len(graph.Groups))
		for _, group := range graph.Groups {
			fmt.Printf("    - %s (%d resources)\n", group.Name, len(group.Resources))
		}
	}

	// Step 4: Generate chart
	if opts.verbose {
		fmt.Printf("\n[4/5] Generating Helm chart...\n")
	}

	generatorRegistry := generator.DefaultRegistry()
	gen, err := generatorRegistry.Get(outputMode)
	if err != nil {
		return fmt.Errorf("failed to get generator: %w", err)
	}

	genOpts := generator.Options{
		OutputDir:           opts.outputDir,
		ChartName:           opts.chartName,
		ChartVersion:        opts.chartVersion,
		AppVersion:          opts.appVersion,
		Mode:                outputMode,
		Namespace:           opts.namespace,
		IncludeTests:        opts.includeTests,
		IncludeREADME:       opts.includeREADME,
		IncludeSchema:       opts.includeSchema,
		ExternalFileManager: externalFileManager,
		EnvValues:           opts.envValues,
		DeckhouseModule:     opts.deckhouseModule,
	}

	charts, err := gen.Generate(ctx, graph, genOpts)
	if err != nil {
		return fmt.Errorf("chart generation failed: %w", err)
	}

	if len(charts) == 0 {
		return fmt.Errorf("no charts generated")
	}

	// Apply Deckhouse module scaffold if requested
	if opts.deckhouseModule {
		if opts.verbose {
			fmt.Printf("\n[4b/5] Applying Deckhouse module scaffold...\n")
		}
		for i, chart := range charts {
			charts[i] = generator.GenerateDeckhouseModule(chart, nil)
		}
	}

	// Step 5: Write charts to disk
	if opts.verbose {
		fmt.Printf("\n[5/5] Writing charts to disk...\n")
	}

	for _, chart := range charts {
		if err := generator.ValidateChart(chart); err != nil {
			return fmt.Errorf("chart validation failed for %s: %w", chart.Name, err)
		}

		if err := generator.WriteChart(chart, opts.outputDir); err != nil {
			return fmt.Errorf("failed to write chart %s: %w", chart.Name, err)
		}

		if opts.verbose {
			fmt.Printf("  Written chart: %s\n", chart.Name)
			fmt.Printf("    Templates: %d\n", len(chart.Templates))
		}
	}

	// Generate environment-specific values if requested
	if opts.envValues {
		if opts.verbose {
			fmt.Printf("\n[5b/5] Generating environment-specific values...\n")
		}
		envFiles := generator.GenerateEnvValues(nil)
		for _, chart := range charts {
			chartDir := filepath.Join(opts.outputDir, chart.Name)
			for filename, content := range envFiles {
				envPath := filepath.Join(chartDir, filename)
				if err := os.WriteFile(envPath, content, 0644); err != nil {
					return fmt.Errorf("failed to write %s: %w", filename, err)
				}
				if opts.verbose {
					fmt.Printf("  Written: %s/%s\n", chart.Name, filename)
				}
			}
		}
	}

	fmt.Printf("\n✓ Successfully generated %d chart(s) in %s\n", len(charts), opts.outputDir)
	fmt.Printf("\nTo install the chart, run:\n")
	fmt.Printf("  helm install my-release %s/%s\n", opts.outputDir, opts.chartName)

	return nil
}

func newAnalyzeCmd() *cobra.Command {
	var (
		paths         []string
		outputFormat  string
		outputFile    string
		summaryOnly   bool
		color         bool
		verbose       bool
		namespace     string
		namespaces    []string
		includeKinds  []string
		excludeKinds  []string
		recursive     bool
	)

	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Analyze resources and provide recommendations",
		Long: `Analyze Kubernetes resources for architecture patterns, best practices,
and provide recommendations for Helm chart organization.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAnalyze(cmd.Context(), analyzeOptions{
				paths:        paths,
				outputFormat: outputFormat,
				outputFile:   outputFile,
				summaryOnly:  summaryOnly,
				color:        color,
				verbose:      verbose,
				namespace:    namespace,
				namespaces:   namespaces,
				includeKinds: includeKinds,
				excludeKinds: excludeKinds,
				recursive:    recursive,
			})
		},
	}

	cmd.Flags().StringSliceVarP(&paths, "file", "f", []string{}, "Path(s) to YAML files or directories (required)")
	cmd.Flags().StringVar(&outputFormat, "output-format", "text", "Output format: text, json, markdown")
	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "Output file (default: stdout)")
	cmd.Flags().BoolVar(&summaryOnly, "summary", false, "Show only summary")
	cmd.Flags().BoolVar(&color, "color", true, "Enable colored output")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Filter by namespace")
	cmd.Flags().StringSliceVar(&namespaces, "namespaces", nil, "Filter by multiple namespaces")
	cmd.Flags().StringSliceVar(&includeKinds, "include-kinds", nil, "Include only these resource kinds")
	cmd.Flags().StringSliceVar(&excludeKinds, "exclude-kinds", nil, "Exclude these resource kinds")
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", true, "Recursively scan directories")

	cmd.MarkFlagRequired("file")

	return cmd
}

type analyzeOptions struct {
	paths        []string
	outputFormat string
	outputFile   string
	summaryOnly  bool
	color        bool
	verbose      bool
	namespace    string
	namespaces   []string
	includeKinds []string
	excludeKinds []string
	recursive    bool
}

func runAnalyze(ctx context.Context, opts analyzeOptions) error {
	// Step 1: Extract resources
	if opts.verbose {
		fmt.Printf("[1/4] Extracting resources...\n")
	}

	extractorRegistry := extractor.DefaultRegistry()
	ext, ok := extractorRegistry.Get(types.SourceFile)
	if !ok {
		return fmt.Errorf("file extractor not available")
	}

	extractOpts := extractor.Options{
		Paths:        opts.paths,
		Namespace:    opts.namespace,
		Namespaces:   opts.namespaces,
		IncludeKinds: opts.includeKinds,
		ExcludeKinds: opts.excludeKinds,
		Recursive:    opts.recursive,
	}

	if err := ext.Validate(ctx, extractOpts); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	resourceChan, errChan := ext.Extract(ctx, extractOpts)

	var extractedResources []*types.ExtractedResource

	for {
		select {
		case resource, ok := <-resourceChan:
			if !ok {
				resourceChan = nil
				if errChan == nil {
					goto extracted
				}
				continue
			}
			extractedResources = append(extractedResources, resource)
			if opts.verbose {
				fmt.Printf("  Extracted: %s\n", resource.ResourceKey().String())
			}
		case err, ok := <-errChan:
			if !ok {
				errChan = nil
				if resourceChan == nil {
					goto extracted
				}
				continue
			}
			fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
extracted:

	if len(extractedResources) == 0 {
		return fmt.Errorf("no resources extracted")
	}

	if opts.verbose {
		fmt.Printf("  Total: %d resources\n", len(extractedResources))
	}

	// Step 2: Process resources
	if opts.verbose {
		fmt.Printf("\n[2/4] Processing resources...\n")
	}

	processorRegistry := processor.NewRegistry()
	k8s.RegisterAll(processorRegistry)

	var processedResources []*types.ProcessedResource
	allResourcesMap := make(map[types.ResourceKey]*types.ExtractedResource)
	for _, r := range extractedResources {
		allResourcesMap[r.ResourceKey()] = r
	}

	for _, extracted := range extractedResources {
		procCtx := processor.Context{
			Ctx:          ctx,
			ChartName:    "analysis",
			OutputMode:   types.OutputModeUniversal,
			Namespace:    extracted.Object.GetNamespace(),
			AllResources: allResourcesMap,
		}

		result, err := processorRegistry.Process(procCtx, extracted.Object)
		if err != nil {
			if opts.verbose {
				fmt.Fprintf(os.Stderr, "  Warning: Failed to process %s: %v\n", extracted.ResourceKey(), err)
			}
			continue
		}

		processed := &types.ProcessedResource{
			Original:        extracted,
			ServiceName:     result.ServiceName,
			TemplatePath:    result.TemplatePath,
			TemplateContent: result.TemplateContent,
			ValuesPath:      result.ValuesPath,
			Values:          result.Values,
		}
		processedResources = append(processedResources, processed)

		if opts.verbose {
			fmt.Printf("  Processed: %s\n", extracted.ResourceKey().String())
		}
	}

	if opts.verbose {
		fmt.Printf("  Total: %d processed\n", len(processedResources))
	}

	// Step 3: Analyze relationships
	if opts.verbose {
		fmt.Printf("\n[3/4] Analyzing relationships...\n")
	}

	relationshipAnalyzer := analyzer.NewDefaultAnalyzer()
	detector.RegisterAll(relationshipAnalyzer)

	resourceGraph, err := relationshipAnalyzer.Analyze(ctx, processedResources)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	if opts.verbose {
		fmt.Printf("  Detected: %d relationships\n", len(resourceGraph.Relationships))
		fmt.Printf("  Grouped into: %d services\n", len(resourceGraph.Groups))
	}

	// Step 4: Pattern analysis
	if opts.verbose {
		fmt.Printf("\n[4/4] Analyzing patterns and best practices...\n")
	}

	patternAnalyzer := pattern.DefaultAnalyzer()
	recommender := pattern.NewRecommender(patternAnalyzer)
	report := recommender.GenerateReport(resourceGraph)

	// Output
	formatter := pattern.NewFormatter(opts.color)

	var output string

	if opts.summaryOnly {
		output = formatter.FormatSummary(report.AnalysisResult)
	} else {
		switch opts.outputFormat {
		case "text":
			output = formatter.FormatReport(report)
		case "json":
			var jsonErr error
			output, jsonErr = formatter.FormatJSON(report)
			if jsonErr != nil {
				return fmt.Errorf("failed to format JSON: %w", jsonErr)
			}
		case "markdown", "md":
			output = formatter.FormatMarkdown(report)
		default:
			return fmt.Errorf("invalid output format: %s (must be text, json, or markdown)", opts.outputFormat)
		}
	}

	// Write output
	if opts.outputFile != "" {
		if err := os.WriteFile(opts.outputFile, []byte(output), 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		fmt.Printf("Analysis report written to: %s\n", opts.outputFile)
	} else {
		fmt.Print(output)
	}

	return nil
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("dhg version %s (built: %s)\n", version, buildTime)
		},
	}
}
