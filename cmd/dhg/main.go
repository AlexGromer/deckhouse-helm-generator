package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

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
	rootCmd.AddCommand(newValidateCmd())
	rootCmd.AddCommand(newDiffCmd())
	rootCmd.AddCommand(newVersionCmd())

	return rootCmd
}

func newGenerateCmd() *cobra.Command {
	var (
		paths           []string
		outputDir       string
		chartName       string
		chartVersion    string
		appVersion      string
		mode            string
		source          string
		namespace       string
		namespaces      []string
		labelSelector   string
		includeKinds    []string
		excludeKinds    []string
		recursive       bool
		kubeConfig      string
		kubeContext     string
		includeTests    bool
		includeREADME   bool
		includeSchema   bool
		verbose         bool
		envValues       bool
		deckhouseModule    bool
		dryRun             bool
		airgapRegistry     string
		namespaceResources bool
		multiTenant        bool
		featureFlags       bool
		cloudProvider      string
		cloudInternal      bool
		detectIngress      bool
		monorepo           bool
		spot               bool
		spotGracePeriod    int
		kustomize          bool
		autoDeps           bool
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
				paths:           paths,
				outputDir:       outputDir,
				chartName:       chartName,
				chartVersion:    chartVersion,
				appVersion:      appVersion,
				mode:            mode,
				source:          source,
				namespace:       namespace,
				namespaces:      namespaces,
				labelSelector:   labelSelector,
				includeKinds:    includeKinds,
				excludeKinds:    excludeKinds,
				recursive:       recursive,
				kubeConfig:      kubeConfig,
				kubeContext:     kubeContext,
				includeTests:    includeTests,
				includeREADME:   includeREADME,
				includeSchema:   includeSchema,
				verbose:         verbose,
				envValues:       envValues,
				deckhouseModule:    deckhouseModule,
				dryRun:             dryRun,
				airgapRegistry:     airgapRegistry,
				namespaceResources: namespaceResources,
				multiTenant:        multiTenant,
				featureFlags:       featureFlags,
				cloudProvider:      cloudProvider,
				cloudInternal:      cloudInternal,
				detectIngress:      detectIngress,
				monorepo:           monorepo,
				spot:               spot,
				spotGracePeriod:    spotGracePeriod,
				kustomize:          kustomize,
				autoDeps:           autoDeps,
			})
		},
	}

	cmd.Flags().StringSliceVarP(&paths, "file", "f", []string{}, "Path(s) to YAML files or directories")
	cmd.Flags().StringVarP(&outputDir, "output", "o", "./chart", "Output directory for the chart")
	cmd.Flags().StringVar(&chartName, "chart-name", "", "Name of the chart (required)")
	cmd.Flags().StringVar(&chartVersion, "chart-version", "0.1.0", "Chart version")
	cmd.Flags().StringVar(&appVersion, "app-version", "1.0.0", "Application version")
	cmd.Flags().StringVar(&mode, "mode", "universal", "Output mode: universal, separate, library, umbrella")
	cmd.Flags().StringVarP(&source, "source", "s", "file", "Source type: file (default). cluster and gitops are not yet implemented.")
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
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print generated chart to stdout without writing to disk")
	cmd.Flags().StringVar(&airgapRegistry, "airgap-registry", "", "Generate air-gapped artifacts (images.txt, values-airgap.yaml, mirror-images.sh) targeting this registry")
	cmd.Flags().BoolVar(&namespaceResources, "namespace-resources", false, "Generate namespace governance resources (ResourceQuota, LimitRange, NetworkPolicy)")
	cmd.Flags().BoolVar(&multiTenant, "multi-tenant", false, "Generate multi-tenant chart overlay with per-tenant isolation")
	cmd.Flags().BoolVar(&featureFlags, "feature-flags", false, "Inject feature flags (monitoring, ingress, autoscaling, security, storage, rbac)")
	cmd.Flags().StringVar(&cloudProvider, "cloud-provider", "", "Cloud provider for Service annotations (aws, gcp, azure)")
	cmd.Flags().BoolVar(&cloudInternal, "cloud-internal", false, "Use internal load balancer for cloud annotations")
	cmd.Flags().BoolVar(&detectIngress, "detect-ingress", false, "Auto-detect ingress controller and generate controller-specific annotations")
	cmd.Flags().BoolVar(&monorepo, "monorepo", false, "Generate monorepo layout with Makefile, .helmignore, and ct.yaml")
	cmd.Flags().BoolVar(&spot, "spot", false, "Inject spot/preemptible instance tolerations and PDB")
	cmd.Flags().IntVar(&spotGracePeriod, "spot-grace-period", 15, "Grace period in seconds for spot instance preStop hook")
	cmd.Flags().BoolVar(&kustomize, "kustomize", false, "Generate Kustomize layout with base and dev/staging/prod overlays")
	cmd.Flags().BoolVar(&autoDeps, "auto-deps", false, "Auto-detect infrastructure dependencies (PostgreSQL, Redis, etc.)")

	_ = cmd.MarkFlagRequired("chart-name")

	return cmd
}

type generateOptions struct {
	paths           []string
	outputDir       string
	chartName       string
	chartVersion    string
	appVersion      string
	mode            string
	source          string
	namespace       string
	namespaces      []string
	labelSelector   string
	includeKinds    []string
	excludeKinds    []string
	recursive       bool
	kubeConfig      string
	kubeContext     string
	includeTests    bool
	includeREADME   bool
	includeSchema   bool
	verbose         bool
	envValues       bool
	deckhouseModule    bool
	dryRun             bool
	airgapRegistry     string
	namespaceResources bool
	multiTenant        bool
	featureFlags       bool
	cloudProvider      string
	cloudInternal      bool
	detectIngress      bool
	monorepo           bool
	spot               bool
	spotGracePeriod    int
	kustomize          bool
	autoDeps           bool
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

drain:
	for {
		select {
		case resource, ok := <-resourceChan:
			if !ok {
				resourceChan = nil
				if errChan == nil {
					break drain
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
					break drain
				}
				continue
			}
			extractErrors = append(extractErrors, err)
			fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
		case <-ctx.Done():
			return ctx.Err()
		}
	}

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

	// Apply air-gapped artifacts if requested
	if opts.airgapRegistry != "" {
		if opts.verbose {
			fmt.Printf("\n[4c/5] Generating air-gapped artifacts for registry: %s\n", opts.airgapRegistry)
		}
		for _, chart := range charts {
			refs := generator.ExtractImageReferences(chart)

			// Add images.txt
			imageList := generator.GenerateImageList(refs)
			chart.ExternalFiles = append(chart.ExternalFiles, types.ExternalFileInfo{
				Path: "images.txt", Content: imageList,
			})

			// Add mirror-images.sh
			mirrorScript := generator.GenerateMirrorScript(refs, opts.airgapRegistry)
			chart.ExternalFiles = append(chart.ExternalFiles, types.ExternalFileInfo{
				Path: "mirror-images.sh", Content: mirrorScript,
			})

			// Add values-airgap.yaml
			airgapValues := generator.GenerateAirgapValues(refs, opts.airgapRegistry)
			airgapYAML, _ := yaml.Marshal(airgapValues)
			chart.ExternalFiles = append(chart.ExternalFiles, types.ExternalFileInfo{
				Path: "values-airgap.yaml", Content: string(airgapYAML),
			})
		}
	}

	// Apply namespace resources if requested
	if opts.namespaceResources {
		if opts.verbose {
			fmt.Printf("\n[4d/5] Generating namespace governance resources...\n")
		}
		groupingResult, _ := generator.GroupResources(graph)
		nsOpts := generator.NamespaceOpts{
			ResourceQuota: true,
			LimitRange:    true,
			NetworkPolicy: true,
		}
		nsTemplates := generator.GenerateNamespaceResources(groupingResult.Groups, nsOpts)
		for _, chart := range charts {
			for path, content := range nsTemplates {
				chart.Templates[path] = content
			}
		}

		// Also generate auto-NetworkPolicies from service analysis
		autoNP := generator.GenerateAutoNetworkPolicies(graph, groupingResult.Groups)
		for _, chart := range charts {
			for path, content := range autoNP {
				chart.Templates[path] = content
			}
		}
	}

	// Apply multi-tenant overlay if requested
	if opts.multiTenant {
		if opts.verbose {
			fmt.Printf("\n[4e/5] Applying multi-tenant overlay...\n")
		}
		for i, chart := range charts {
			charts[i] = generator.GenerateMultiTenantOverlay(chart, 2) // default 2 tenants
		}
	}

	// Apply feature flags if requested
	if opts.featureFlags {
		if opts.verbose {
			fmt.Printf("\n[4f/5] Injecting feature flags...\n")
		}
		config := generator.DefaultFeatureFlagConfig()
		for i, chart := range charts {
			charts[i] = generator.InjectFeatureFlags(chart, config)
		}
	}

	// Apply cloud annotations if requested
	if opts.cloudProvider != "" {
		if opts.verbose {
			fmt.Printf("\n[4g/5] Injecting cloud annotations for %s...\n", opts.cloudProvider)
		}
		cloudConfig := generator.CloudAnnotationConfig{
			Provider: generator.CloudProvider(opts.cloudProvider),
			Internal: opts.cloudInternal,
		}
		if !opts.cloudInternal {
			cloudConfig.Scheme = "internet-facing"
		} else {
			cloudConfig.Scheme = "internal"
		}
		for i, chart := range charts {
			charts[i] = generator.InjectCloudAnnotations(chart, cloudConfig)
		}
	}

	// Auto-detect ingress controller and inject annotations if requested
	if opts.detectIngress {
		if opts.verbose {
			fmt.Printf("\n[4h/5] Detecting ingress controller...\n")
		}
		controller := generator.DetectIngressController(processedResources)
		if opts.verbose {
			fmt.Printf("  Detected controller: %s\n", controller)
		}
		if controller != generator.ControllerUnknown {
			features := []generator.IngressFeature{
				generator.IngressSSLRedirect,
			}
			for i, chart := range charts {
				charts[i] = generator.InjectIngressAnnotations(chart, controller, features)
			}
		}
	}

	// Apply spot instance configuration if requested
	if opts.spot {
		if opts.verbose {
			fmt.Printf("\n[4i/5] Injecting spot/preemptible instance configuration...\n")
		}
		spotConfig := generator.SpotConfig{
			Provider:    generator.SpotAWS,
			GracePeriod: opts.spotGracePeriod,
			Enabled:     true,
		}
		if opts.cloudProvider == "gcp" {
			spotConfig.Provider = generator.SpotGCP
		} else if opts.cloudProvider == "azure" {
			spotConfig.Provider = generator.SpotAzure
		}
		for i, chart := range charts {
			charts[i] = generator.InjectSpotConfig(chart, spotConfig)
		}
	}

	// Auto-detect dependencies if requested
	if opts.autoDeps {
		if opts.verbose {
			fmt.Printf("\n[4j/5] Auto-detecting infrastructure dependencies...\n")
		}
		detected := generator.DetectCommonDependencies(processedResources)
		if opts.verbose {
			fmt.Printf("  Detected %d dependencies\n", len(detected))
		}
		for i, chart := range charts {
			charts[i] = generator.InjectDependencies(chart, detected)
		}
	}

	// Dry-run: print to stdout instead of writing to disk
	if opts.dryRun {
		for _, chart := range charts {
			fmt.Printf("---\n# Chart: %s\n", chart.Name)
			fmt.Printf("# Chart.yaml\n%s\n", chart.ChartYAML)
			fmt.Printf("---\n# values.yaml\n%s\n", chart.ValuesYAML)

			// Print templates sorted
			templatePaths := make([]string, 0, len(chart.Templates))
			for path := range chart.Templates {
				templatePaths = append(templatePaths, path)
			}
			sort.Strings(templatePaths)
			for _, path := range templatePaths {
				fmt.Printf("---\n# %s\n%s\n", path, chart.Templates[path])
			}

			if chart.Helpers != "" {
				fmt.Printf("---\n# templates/_helpers.tpl\n%s\n", chart.Helpers)
			}
		}
		return nil
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

	// Generate monorepo layout if requested
	if opts.monorepo {
		if opts.verbose {
			fmt.Printf("\n[5c/5] Generating monorepo layout...\n")
		}
		layout, err := generator.GenerateMonorepoLayout(charts, opts.chartName)
		if err != nil {
			return fmt.Errorf("monorepo layout generation failed: %w", err)
		}
		// Write Makefile
		makefilePath := filepath.Join(opts.outputDir, "Makefile")
		if err := os.WriteFile(makefilePath, []byte(layout.Makefile), 0644); err != nil {
			return fmt.Errorf("failed to write Makefile: %w", err)
		}
		// Write .helmignore
		helmignorePath := filepath.Join(opts.outputDir, ".helmignore")
		if err := os.WriteFile(helmignorePath, []byte(layout.HelmIgnore), 0644); err != nil {
			return fmt.Errorf("failed to write .helmignore: %w", err)
		}
		// Write ct.yaml
		ctConfigPath := filepath.Join(opts.outputDir, "ct.yaml")
		if err := os.WriteFile(ctConfigPath, []byte(layout.CTConfig), 0644); err != nil {
			return fmt.Errorf("failed to write ct.yaml: %w", err)
		}
		if opts.verbose {
			fmt.Printf("  Written: Makefile, .helmignore, ct.yaml\n")
		}
	}

	// Generate Kustomize layout if requested
	if opts.kustomize {
		if opts.verbose {
			fmt.Printf("\n[5d/5] Generating Kustomize layout...\n")
		}
		for _, chart := range charts {
			kustomizeOutput, err := generator.GenerateKustomizeLayout(chart)
			if err != nil {
				if opts.verbose {
					fmt.Fprintf(os.Stderr, "  Warning: Kustomize generation skipped for %s: %v\n", chart.Name, err)
				}
				continue
			}
			kustomizeDir := filepath.Join(opts.outputDir, chart.Name, "kustomize")
			// Write base
			baseDir := filepath.Join(kustomizeDir, "base")
			if err := os.MkdirAll(baseDir, 0755); err != nil {
				return fmt.Errorf("failed to create base dir: %w", err)
			}
			if err := os.WriteFile(filepath.Join(baseDir, "kustomization.yaml"), []byte(kustomizeOutput.Base.Kustomization), 0644); err != nil {
				return fmt.Errorf("failed to write base kustomization: %w", err)
			}
			// Write overlays
			for envName, overlay := range kustomizeOutput.Overlays {
				overlayDir := filepath.Join(kustomizeDir, "overlays", envName)
				if err := os.MkdirAll(overlayDir, 0755); err != nil {
					return fmt.Errorf("failed to create overlay dir: %w", err)
				}
				if err := os.WriteFile(filepath.Join(overlayDir, "kustomization.yaml"), []byte(overlay.Kustomization), 0644); err != nil {
					return fmt.Errorf("failed to write overlay kustomization: %w", err)
				}
				for _, patch := range overlay.Patches {
					if err := os.WriteFile(filepath.Join(overlayDir, patch.Target), []byte(patch.Patch), 0644); err != nil {
						return fmt.Errorf("failed to write patch %s: %w", patch.Target, err)
					}
				}
			}
			if opts.verbose {
				fmt.Printf("  Written: kustomize layout for %s\n", chart.Name)
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

	_ = cmd.MarkFlagRequired("file")

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

drainExtract:
	for {
		select {
		case resource, ok := <-resourceChan:
			if !ok {
				resourceChan = nil
				if errChan == nil {
					break drainExtract
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
					break drainExtract
				}
				continue
			}
			fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
		case <-ctx.Done():
			return ctx.Err()
		}
	}

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

func newValidateCmd() *cobra.Command {
	var (
		paths   []string
		verbose bool
	)

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate Helm chart structure and templates",
		Long: `Validate Helm chart for common issues:
  - Chart.yaml presence and required fields
  - values.yaml syntax
  - Template syntax (Go template parsing)
  - Required files presence`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValidate(cmd.Context(), validateOptions{
				paths:   paths,
				verbose: verbose,
			})
		},
	}

	cmd.Flags().StringSliceVarP(&paths, "file", "f", []string{"."}, "Path(s) to chart directories to validate")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")

	return cmd
}

type validateOptions struct {
	paths   []string
	verbose bool
}

func runValidate(_ context.Context, opts validateOptions) error {
	totalErrors := 0
	totalWarnings := 0

	for _, chartPath := range opts.paths {
		fmt.Printf("Validating chart at: %s\n", chartPath)

		// Check Chart.yaml
		chartYAMLPath := filepath.Join(chartPath, "Chart.yaml")
		if _, err := os.Stat(chartYAMLPath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "  ERROR: Chart.yaml not found at %s\n", chartYAMLPath)
			totalErrors++
		} else {
			data, err := os.ReadFile(chartYAMLPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  ERROR: Cannot read Chart.yaml: %v\n", err)
				totalErrors++
			} else {
				if opts.verbose {
					fmt.Printf("  OK: Chart.yaml found (%d bytes)\n", len(data))
				}
				// Check required fields
				content := string(data)
				requiredFields := []string{"apiVersion:", "name:", "version:"}
				for _, field := range requiredFields {
					if !strings.Contains(content, field) {
						fmt.Fprintf(os.Stderr, "  ERROR: Chart.yaml missing required field: %s\n", strings.TrimSuffix(field, ":"))
						totalErrors++
					}
				}
			}
		}

		// Check values.yaml
		valuesPath := filepath.Join(chartPath, "values.yaml")
		if _, err := os.Stat(valuesPath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "  WARNING: values.yaml not found\n")
			totalWarnings++
		} else {
			data, err := os.ReadFile(valuesPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  ERROR: Cannot read values.yaml: %v\n", err)
				totalErrors++
			} else {
				// Try to parse YAML
				var values map[string]interface{}
				if err := yaml.Unmarshal(data, &values); err != nil {
					fmt.Fprintf(os.Stderr, "  ERROR: Invalid YAML in values.yaml: %v\n", err)
					totalErrors++
				} else if opts.verbose {
					fmt.Printf("  OK: values.yaml valid (%d bytes)\n", len(data))
				}
			}
		}

		// Check templates directory
		templatesDir := filepath.Join(chartPath, "templates")
		if _, err := os.Stat(templatesDir); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "  WARNING: templates/ directory not found\n")
			totalWarnings++
		} else {
			// Parse templates for syntax
			entries, err := os.ReadDir(templatesDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  ERROR: Cannot read templates directory: %v\n", err)
				totalErrors++
			} else {
				templateCount := 0
				for _, entry := range entries {
					if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
						continue
					}
					templateCount++
					tmplPath := filepath.Join(templatesDir, entry.Name())
					data, err := os.ReadFile(tmplPath)
					if err != nil {
						fmt.Fprintf(os.Stderr, "  ERROR: Cannot read template %s: %v\n", entry.Name(), err)
						totalErrors++
						continue
					}
					// Basic Go template syntax check (check balanced {{ }})
					content := string(data)
					opens := strings.Count(content, "{{")
					closes := strings.Count(content, "}}")
					if opens != closes {
						fmt.Fprintf(os.Stderr, "  ERROR: Unbalanced template delimiters in %s ({{ count: %d, }} count: %d)\n", entry.Name(), opens, closes)
						totalErrors++
					} else if opts.verbose {
						fmt.Printf("  OK: %s (%d template expressions)\n", entry.Name(), opens)
					}
				}
				if opts.verbose {
					fmt.Printf("  Templates: %d files checked\n", templateCount)
				}
			}
		}
	}

	// Summary
	fmt.Printf("\nValidation complete: %d error(s), %d warning(s)\n", totalErrors, totalWarnings)
	if totalErrors > 0 {
		return fmt.Errorf("validation failed with %d error(s)", totalErrors)
	}
	return nil
}

func newDiffCmd() *cobra.Command {
	var (
		color bool
	)

	cmd := &cobra.Command{
		Use:   "diff <dir1> <dir2>",
		Short: "Show differences between two chart directories",
		Long: `Compare two Helm chart directories and show differences.
Useful for comparing generated charts before and after changes.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiff(cmd.Context(), diffOptions{
				dir1:  args[0],
				dir2:  args[1],
				color: color,
			})
		},
	}

	cmd.Flags().BoolVar(&color, "color", true, "Enable colored output")

	return cmd
}

type diffOptions struct {
	dir1  string
	dir2  string
	color bool
}

func runDiff(_ context.Context, opts diffOptions) error {
	// Validate directories exist
	for _, dir := range []string{opts.dir1, opts.dir2} {
		info, err := os.Stat(dir)
		if err != nil {
			return fmt.Errorf("cannot access %s: %w", dir, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", dir)
		}
	}

	// Collect all files from both directories
	files1, err := collectFiles(opts.dir1)
	if err != nil {
		return fmt.Errorf("failed to scan %s: %w", opts.dir1, err)
	}
	files2, err := collectFiles(opts.dir2)
	if err != nil {
		return fmt.Errorf("failed to scan %s: %w", opts.dir2, err)
	}

	// Build a union of all relative paths
	allFiles := make(map[string]bool)
	for f := range files1 {
		allFiles[f] = true
	}
	for f := range files2 {
		allFiles[f] = true
	}

	// Sort for deterministic output
	sortedFiles := make([]string, 0, len(allFiles))
	for f := range allFiles {
		sortedFiles = append(sortedFiles, f)
	}
	sort.Strings(sortedFiles)

	hasDiff := false
	for _, relPath := range sortedFiles {
		content1, in1 := files1[relPath]
		content2, in2 := files2[relPath]

		if !in1 {
			hasDiff = true
			printDiffHeader(opts.dir1, opts.dir2, relPath, "added", opts.color)
			printLines(content2, "+", opts.color)
			continue
		}

		if !in2 {
			hasDiff = true
			printDiffHeader(opts.dir1, opts.dir2, relPath, "removed", opts.color)
			printLines(content1, "-", opts.color)
			continue
		}

		if content1 != content2 {
			hasDiff = true
			printDiffHeader(opts.dir1, opts.dir2, relPath, "modified", opts.color)
			printUnifiedDiff(content1, content2, opts.color)
		}
	}

	if !hasDiff {
		fmt.Println("No differences found.")
	}

	return nil
}

// collectFiles walks a directory and returns map of relative_path -> content
func collectFiles(dir string) (map[string]string, error) {
	files := make(map[string]string)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[relPath] = string(data)
		return nil
	})
	return files, err
}

func printDiffHeader(dir1, dir2, relPath, status string, color bool) {
	if color {
		fmt.Printf("\033[1m--- %s/%s\033[0m\n", dir1, relPath)
		fmt.Printf("\033[1m+++ %s/%s\033[0m\n", dir2, relPath)
		fmt.Printf("\033[36m@@ %s @@\033[0m\n", status)
	} else {
		fmt.Printf("--- %s/%s\n", dir1, relPath)
		fmt.Printf("+++ %s/%s\n", dir2, relPath)
		fmt.Printf("@@ %s @@\n", status)
	}
}

func printLines(content, prefix string, color bool) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if color && prefix == "+" {
			fmt.Printf("\033[32m%s%s\033[0m\n", prefix, line)
		} else if color && prefix == "-" {
			fmt.Printf("\033[31m%s%s\033[0m\n", prefix, line)
		} else {
			fmt.Printf("%s%s\n", prefix, line)
		}
	}
}

func printUnifiedDiff(content1, content2 string, color bool) {
	lines1 := strings.Split(content1, "\n")
	lines2 := strings.Split(content2, "\n")

	// Simple line-by-line comparison
	maxLines := len(lines1)
	if len(lines2) > maxLines {
		maxLines = len(lines2)
	}

	for i := 0; i < maxLines; i++ {
		var l1, l2 string
		if i < len(lines1) {
			l1 = lines1[i]
		}
		if i < len(lines2) {
			l2 = lines2[i]
		}

		if l1 == l2 {
			fmt.Printf(" %s\n", l1)
		} else {
			if i < len(lines1) {
				if color {
					fmt.Printf("\033[31m-%s\033[0m\n", l1)
				} else {
					fmt.Printf("-%s\n", l1)
				}
			}
			if i < len(lines2) {
				if color {
					fmt.Printf("\033[32m+%s\033[0m\n", l2)
				} else {
					fmt.Printf("+%s\n", l2)
				}
			}
		}
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "dhg version %s (built: %s)\n", version, buildTime)
		},
	}
}
