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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/analyzer/pattern"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/extractor"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/generator"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/synth"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
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
		// Runtime errors are not usage errors: print the error, not the help.
		SilenceUsage: true,
	}

	rootCmd.AddCommand(newGenerateCmd())
	rootCmd.AddCommand(newAnalyzeCmd())
	rootCmd.AddCommand(newValidateCmd())
	rootCmd.AddCommand(newDiffCmd())
	rootCmd.AddCommand(newMigrateCmd())
	rootCmd.AddCommand(newFixCmd())
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newFeaturesCmd())
	rootCmd.AddCommand(newGraphCmd())

	return rootCmd
}

func newGenerateCmd() *cobra.Command {
	var (
		paths              []string
		outputDir          string
		chartName          string
		chartVersion       string
		appVersion         string
		mode               string
		source             string
		namespace          string
		namespaces         []string
		labelSelector      string
		includeKinds       []string
		excludeKinds       []string
		recursive          bool
		kubeConfig         string
		kubeContext        string
		clusterSecrets     string
		gitPath            string
		gitRepo            string
		gitBranch          string
		sshKey             string
		images             []string
		platform           string
		insecureRegistry   bool
		includeTests       bool
		includeREADME      bool
		includeSchema      bool
		verbose            bool
		envValues          bool
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
		postRenderer       bool
		autoDeps           bool
		tenantCount        int
		templateDir        string
		templateStrategy   string
		plugins            []string
		configPath         string
		includeHooks       bool
		valuesFlat         bool
		withFeatures       []string
		featureOpts        []string
	)

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate Helm chart from Kubernetes resources",
		Long: `Generate Helm chart from Kubernetes resources.

Examples:
  # Generate from YAML files
  dhg generate -f ./manifests -o ./chart --chart-name myapp

  # Generate from a live cluster (system namespaces kube-*/d8-* are skipped)
  dhg generate -s cluster -n production --kubeconfig ~/.kube/config --chart-name myapp

  # Generate from a Git repository
  dhg generate -s gitops --git-repo https://github.com/org/manifests --git-path apps/web --chart-name web

  # Generate with filtering
  dhg generate -f ./manifests --include-kinds Deployment,Service,Ingress`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGenerate(cmd.Context(), generateOptions{
				paths:              paths,
				outputDir:          outputDir,
				chartName:          chartName,
				chartVersion:       chartVersion,
				appVersion:         appVersion,
				mode:               mode,
				source:             source,
				namespace:          namespace,
				namespaces:         namespaces,
				labelSelector:      labelSelector,
				includeKinds:       includeKinds,
				excludeKinds:       excludeKinds,
				recursive:          recursive,
				kubeConfig:         kubeConfig,
				kubeContext:        kubeContext,
				clusterSecrets:     clusterSecrets,
				gitPath:            gitPath,
				gitRepo:            gitRepo,
				gitBranch:          gitBranch,
				sshKey:             sshKey,
				images:             images,
				platform:           platform,
				insecureRegistry:   insecureRegistry,
				includeTests:       includeTests,
				includeREADME:      includeREADME,
				includeSchema:      includeSchema,
				verbose:            verbose,
				envValues:          envValues,
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
				postRenderer:       postRenderer,
				autoDeps:           autoDeps,
				tenantCount:        tenantCount,
				templateDir:        templateDir,
				templateStrategy:   templateStrategy,
				plugins:            plugins,
				includeHooks:       includeHooks,
				valuesFlat:         valuesFlat,
				withFeatures:       withFeatures,
				featureOpts:        featureOpts,
			})
		},
	}

	cmd.Flags().StringSliceVarP(&paths, "file", "f", []string{}, "Path(s) to YAML files or directories")
	cmd.Flags().StringVarP(&outputDir, "output", "o", "./chart", "Output directory for the chart")
	cmd.Flags().StringVar(&chartName, "chart-name", "", "Name of the chart (required; may come from the config file)")
	cmd.Flags().StringVar(&chartVersion, "chart-version", "0.1.0", "Chart version")
	cmd.Flags().StringVar(&appVersion, "app-version", "1.0.0", "Application version")
	cmd.Flags().StringVar(&mode, "mode", "universal", "Output mode: universal, separate, library, umbrella")
	cmd.Flags().StringVarP(&source, "source", "s", "file", "Source type: file, cluster (live cluster via kubeconfig), gitops (shallow git clone), "+
		"or synthesized from: image (registry image config), compose (docker-compose files), source (project directory: Dockerfile, Spring Boot)")
	cmd.Flags().StringSliceVar(&images, "image", nil, "Image reference: the images to read for --source image, the application image for --source source")
	cmd.Flags().StringVar(&platform, "platform", "linux/amd64", "Platform of multi-platform images for --source image (os/arch[/variant])")
	cmd.Flags().BoolVar(&insecureRegistry, "insecure-registry", false, "Talk plain HTTP to the registry (--source image)")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Filter by namespace")
	cmd.Flags().StringSliceVar(&namespaces, "namespaces", []string{}, "Filter by multiple namespaces")
	cmd.Flags().StringVarP(&labelSelector, "selector", "l", "", "Label selector filter")
	cmd.Flags().StringSliceVar(&includeKinds, "include-kinds", []string{}, "Include only these resource kinds")
	cmd.Flags().StringSliceVar(&excludeKinds, "exclude-kinds", []string{}, "Exclude these resource kinds")
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", true, "Recursively scan directories")
	cmd.Flags().StringVar(&kubeConfig, "kubeconfig", "", "Path to kubeconfig file")
	cmd.Flags().StringVar(&kubeContext, "context", "", "Kubeconfig context to use")
	cmd.Flags().StringVar(&clusterSecrets, "cluster-secrets", "skip", "Secrets in cluster extraction: skip, mask (values replaced with REDACTED) or include")
	cmd.Flags().StringVar(&gitRepo, "git-repo", "", "Git repository URL for gitops extraction (any URL git clone accepts)")
	cmd.Flags().StringVar(&gitBranch, "git-branch", "", "Git branch or tag for gitops extraction (default: the remote's default branch)")
	cmd.Flags().StringVar(&gitPath, "git-path", "", "Directory inside the repository to read manifests from (default: repository root)")
	cmd.Flags().StringVar(&sshKey, "ssh-key", "", "SSH private key for gitops extraction over ssh")
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
	cmd.Flags().IntVar(&spotGracePeriod, "spot-grace-period", 15, "terminationGracePeriodSeconds for pods on spot nodes (spot.terminationGracePeriodSeconds)")
	cmd.Flags().BoolVar(&kustomize, "kustomize", false, "Generate Kustomize layout with base and dev/staging/prod overlays")
	cmd.Flags().BoolVar(&postRenderer, "post-renderer", false, "Generate a Helm post-renderer (post-renderer/kustomize.sh) applying per-environment Kustomize overlays")
	cmd.Flags().BoolVar(&autoDeps, "auto-deps", false, "Auto-detect infrastructure dependencies (PostgreSQL, Redis, etc.)")
	cmd.Flags().IntVar(&tenantCount, "tenant-count", 2, "Number of tenant examples to scaffold")
	cmd.Flags().StringVar(&templateDir, "template-dir", "", "Directory of template overrides merged into every chart (<dir>/x.yaml → templates/x.yaml; _helpers.tpl and NOTES.txt allowed)")
	cmd.Flags().StringVar(&templateStrategy, "template-strategy", "override", "How --template-dir files are merged: override, append, prepend")
	cmd.Flags().StringArrayVar(&plugins, "plugin", nil, "External processor <apiVersion>/<Kind>[,...]=<executable> (repeatable), e.g. example.com/v1/Widget=./bin/widget")
	cmd.Flags().StringVar(&configPath, "config", "", "Config file whose keys are flag names (default: .dhg.yaml in the working directory, if present)")
	cmd.Flags().BoolVar(&includeHooks, "hooks", false, "Generate Helm lifecycle hook Job templates (pre-upgrade, post-install, pre-delete)")
	cmd.Flags().BoolVar(&valuesFlat, "values-flat", false, "Add inline dot-notation path comments to values.yaml for --set reference")
	cmd.Flags().StringSliceVar(&withFeatures, "with", nil, "Enable optional features, applied in order (list them with: dhg features)")
	cmd.Flags().StringArrayVar(&featureOpts, "feature-opt", nil, "Feature parameter as <feature>.<key>=<value> (repeatable)")

	// chart-name may come from the config file, so it is checked after loading it.
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if err := applyConfigFile(cmd, configPath); err != nil {
			return err
		}
		if chartName == "" {
			return fmt.Errorf("--chart-name is required (flag or config file)")
		}
		return nil
	}

	return cmd
}

type generateOptions struct {
	paths              []string
	outputDir          string
	chartName          string
	chartVersion       string
	appVersion         string
	mode               string
	source             string
	namespace          string
	namespaces         []string
	labelSelector      string
	includeKinds       []string
	excludeKinds       []string
	recursive          bool
	kubeConfig         string
	kubeContext        string
	clusterSecrets     string
	gitPath            string
	gitRepo            string
	gitBranch          string
	sshKey             string
	images             []string
	platform           string
	insecureRegistry   bool
	includeTests       bool
	includeREADME      bool
	includeSchema      bool
	verbose            bool
	envValues          bool
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
	postRenderer       bool
	autoDeps           bool
	tenantCount        int
	templateDir        string
	templateStrategy   string
	plugins            []string
	includeHooks       bool
	valuesFlat         bool
	withFeatures       []string
	featureOpts        []string
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
	case "image":
		sourceType = types.SourceImage
	case "compose":
		sourceType = types.SourceCompose
	case "source":
		sourceType = types.SourceCode
	default:
		return fmt.Errorf("invalid source: %s (must be file, cluster, gitops, image, compose or source)", opts.source)
	}

	// Validate mutually exclusive flags
	if opts.monorepo && opts.kustomize {
		return fmt.Errorf("--monorepo and --kustomize are mutually exclusive")
	}

	// Validate template override strategy
	switch opts.templateStrategy {
	case "", "override", "append", "prepend":
	default:
		return fmt.Errorf("unknown template strategy: %q (must be override, append or prepend)", opts.templateStrategy)
	}

	// Validate cloud provider
	if opts.cloudProvider != "" {
		switch opts.cloudProvider {
		case "aws", "gcp", "azure":
			// valid
		default:
			return fmt.Errorf("unknown cloud provider: %q (must be aws, gcp, or azure)", opts.cloudProvider)
		}
	}

	extractOpts := extractor.Options{
		Paths:            opts.paths,
		Namespace:        opts.namespace,
		Namespaces:       opts.namespaces,
		LabelSelector:    opts.labelSelector,
		IncludeKinds:     opts.includeKinds,
		ExcludeKinds:     opts.excludeKinds,
		Recursive:        opts.recursive,
		KubeConfig:       opts.kubeConfig,
		KubeContext:      opts.kubeContext,
		ClusterSecrets:   opts.clusterSecrets,
		GitURL:           opts.gitRepo,
		GitBranch:        opts.gitBranch,
		GitPath:          opts.gitPath,
		Images:           opts.images,
		Platform:         opts.platform,
		InsecureRegistry: opts.insecureRegistry,
	}
	if opts.sshKey != "" {
		extractOpts.GitAuth = &extractor.GitAuthOptions{SSHKeyPath: opts.sshKey}
	}

	pipeline, err := runPipeline(ctx, pipelineOptions{
		source:     sourceType,
		extract:    extractOpts,
		chartName:  opts.chartName,
		outputMode: outputMode,
		plugins:    opts.plugins,
		verbose:    opts.verbose,
	})
	if err != nil {
		return err
	}
	graph := pipeline.graph
	processedResources := pipeline.processed
	externalFileManager := pipeline.externalFiles

	if opts.verbose {
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
		IncludeHooks:        opts.includeHooks,
		ValuesFlat:          opts.valuesFlat,
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
			mirrorScript, err := generator.GenerateMirrorScript(refs, opts.airgapRegistry)
			if err != nil {
				return fmt.Errorf("generating mirror script: %w", err)
			}
			chart.ExternalFiles = append(chart.ExternalFiles, types.ExternalFileInfo{
				Path: "mirror-images.sh", Content: mirrorScript,
			})

			// Add values-airgap.yaml
			airgapValues := generator.GenerateAirgapValues(opts.airgapRegistry)
			airgapYAML, _ := yaml.Marshal(airgapValues)
			chart.ExternalFiles = append(chart.ExternalFiles, types.ExternalFileInfo{
				Path: "values-airgap.yaml", Content: string(airgapYAML),
			})
		}
	}

	// Pre-compute resource grouping (reused by namespace resources and env values).
	var groupingResult *generator.GroupingResult
	if opts.namespaceResources || opts.envValues {
		var err error
		groupingResult, err = generator.GroupResources(graph)
		if err != nil {
			return fmt.Errorf("resource grouping: %w", err)
		}
	}

	// Apply namespace resources if requested
	if opts.namespaceResources {
		if opts.verbose {
			fmt.Printf("\n[4d/5] Generating namespace governance resources...\n")
		}
		nsOpts := generator.NamespaceOpts{
			ResourceQuota: true,
			LimitRange:    true,
			NetworkPolicy: true,
		}
		for i, chart := range charts {
			updated, err := generator.ApplyNamespaceResources(chart, graph, groupingResult.Groups, nsOpts)
			if err != nil {
				return fmt.Errorf("namespace resources for %s: %w", chart.Name, err)
			}
			charts[i] = updated
		}
	}

	// Apply multi-tenant overlay if requested
	if opts.multiTenant {
		if opts.verbose {
			fmt.Printf("\n[4e/5] Applying multi-tenant overlay...\n")
		}
		for i, chart := range charts {
			charts[i] = generator.GenerateMultiTenantOverlay(chart, opts.tenantCount)
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
			GracePeriod: opts.spotGracePeriod,
			Enabled:     true,
		}
		switch opts.cloudProvider {
		case "aws", "":
			spotConfig.Provider = generator.SpotAWS
		case "gcp":
			spotConfig.Provider = generator.SpotGCP
		case "azure":
			spotConfig.Provider = generator.SpotAzure
		}
		for i, chart := range charts {
			updated, err := generator.InjectSpotConfig(chart, spotConfig)
			if err != nil {
				return fmt.Errorf("spot configuration for %s: %w", chart.Name, err)
			}
			charts[i] = updated
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

	// Helm post-renderer layout (post-renderer/kustomize.sh + overlays).
	if opts.postRenderer {
		if opts.verbose {
			fmt.Println("\nGenerating Helm post-renderer layout (post-renderer/)...")
		}
		for i, chart := range charts {
			charts[i] = generator.InjectPostRenderer(chart, []string{"dev", "staging", "prod"})
		}
	}

	// Apply optional features (--with)
	if len(opts.withFeatures) > 0 {
		if opts.verbose {
			fmt.Printf("\n[4k/5] Applying features: %s\n", strings.Join(opts.withFeatures, ", "))
		}
		featureOptions, err := generator.ParseFeatureOptions(opts.featureOpts)
		if err != nil {
			return err
		}
		charts, err = generator.ApplyFeatures(charts, opts.withFeatures, featureOptions, graph)
		if err != nil {
			return err
		}
	} else if len(opts.featureOpts) > 0 {
		return fmt.Errorf("--feature-opt requires the feature to be enabled with --with")
	}

	// Apply template overrides (--template-dir)
	if opts.templateDir != "" {
		overrides, err := generator.LoadTemplateOverrides(opts.templateDir)
		if err != nil {
			return err
		}
		for i, chart := range charts {
			if !strings.Contains(chart.ChartYAML, "\ntype: library") {
				charts[i] = generator.ApplyTemplateOverrides(chart, overrides, opts.templateStrategy)
			}
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
		if pipeline.synthesis != nil {
			for _, n := range pipeline.synthesis.Notes() {
				fmt.Fprintf(os.Stderr, "Note: %s\n", n)
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

		// Build a name→group index for workload-aware profile selection.
		var groupsByName map[string]*generator.ServiceGroup
		if groupingResult != nil && len(groupingResult.Groups) > 0 {
			groupsByName = make(map[string]*generator.ServiceGroup, len(groupingResult.Groups))
			for _, g := range groupingResult.Groups {
				groupsByName[g.Name] = g
			}
		}

		for _, chart := range charts {
			var envFiles map[string][]byte

			if group, ok := groupsByName[chart.Name]; ok {
				// Workload-aware path: detect workload type and parse base values.
				workloadType := generator.DetectWorkloadType(group)
				var baseValues map[string]interface{}
				if chart.ValuesYAML != "" {
					_ = yaml.Unmarshal([]byte(chart.ValuesYAML), &baseValues)
				}
				envFiles = generator.GenerateEnvValuesForWorkload(baseValues, workloadType)
				if opts.verbose {
					fmt.Printf("  Chart %s: workload=%s (workload-aware profiles)\n", chart.Name, workloadType)
				}
			} else {
				// Fallback: no matching group — use static profiles.
				envFiles = generator.GenerateEnvValues(nil)
				if opts.verbose {
					fmt.Printf("  Chart %s: using default profiles (no group match)\n", chart.Name)
				}
			}

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
			objects := chartObjects(chart, processedResources)
			kustomizeOutput, err := generator.GenerateKustomizeLayout(objects)
			if err != nil {
				if opts.verbose {
					fmt.Fprintf(os.Stderr, "  Warning: Kustomize generation skipped for %s: %v\n", chart.Name, err)
				}
				continue
			}
			kustomizeDir := filepath.Join(opts.outputDir, chart.Name, "kustomize")
			dirs := []*generator.KustomizeDir{kustomizeOutput.Base}
			for _, overlay := range kustomizeOutput.Overlays {
				dirs = append(dirs, overlay)
			}
			for _, dir := range dirs {
				if err := writeKustomizeDir(filepath.Join(kustomizeDir, dir.Path), dir); err != nil {
					return err
				}
			}
			if opts.verbose {
				fmt.Printf("  Written: kustomize layout for %s\n", chart.Name)
			}
		}
	}

	if pipeline.synthesis != nil {
		if err := writeSynthesisReport(opts.outputDir, pipeline); err != nil {
			return err
		}
	}

	fmt.Printf("\n✓ Successfully generated %d chart(s) in %s\n", len(charts), opts.outputDir)
	fmt.Printf("\nTo install the chart, run:\n")
	fmt.Printf("  helm install my-release %s/%s\n", opts.outputDir, opts.chartName)

	return nil
}

func newAnalyzeCmd() *cobra.Command {
	var (
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
	pipeline, err := runPipeline(ctx, pipelineOptions{
		source: types.SourceFile,
		extract: extractor.Options{
			Paths:        opts.paths,
			Namespace:    opts.namespace,
			Namespaces:   opts.namespaces,
			IncludeKinds: opts.includeKinds,
			ExcludeKinds: opts.excludeKinds,
			Recursive:    opts.recursive,
		},
		chartName:  "analysis",
		outputMode: types.OutputModeUniversal,
		lenient:    true,
		verbose:    opts.verbose,
	})
	if err != nil {
		return err
	}
	resourceGraph := pipeline.graph

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

func newMigrateCmd() *cobra.Command {
	var (
		fromDir      string
		sourceFiles  []string
		chartName    string
		chartVersion string
		appVersion   string
		mode         string
		verbose      bool
	)

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Detect drift and generate migration plan between existing and new chart",
		Long: `Compare an existing Helm chart directory with a newly generated chart
from source manifests. Produces a drift report and step-by-step migration plan.

Examples:
  # Compare existing chart with newly generated one
  dhg migrate --from ./chart --source ./manifests --chart-name myapp

  # Verbose output with custom chart version
  dhg migrate --from ./chart -f ./manifests --chart-name myapp --chart-version 0.2.0 -v`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMigrate(cmd.Context(), migrateOptions{
				fromDir:      fromDir,
				sourceFiles:  sourceFiles,
				chartName:    chartName,
				chartVersion: chartVersion,
				appVersion:   appVersion,
				mode:         mode,
				verbose:      verbose,
			})
		},
	}

	cmd.Flags().StringVar(&fromDir, "from", "", "Path to existing chart directory (required)")
	cmd.Flags().StringSliceVarP(&sourceFiles, "source", "f", []string{}, "Path(s) to source manifest files/directories (required)")
	cmd.Flags().StringVar(&chartName, "chart-name", "", "Name of the chart (required)")
	cmd.Flags().StringVar(&chartVersion, "chart-version", "0.1.0", "Chart version for generated chart")
	cmd.Flags().StringVar(&appVersion, "app-version", "1.0.0", "Application version for generated chart")
	cmd.Flags().StringVar(&mode, "mode", "universal", "Output mode: universal, separate, library, umbrella")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")

	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("chart-name")

	return cmd
}

type migrateOptions struct {
	fromDir      string
	sourceFiles  []string
	chartName    string
	chartVersion string
	appVersion   string
	mode         string
	verbose      bool
}

func runMigrate(ctx context.Context, opts migrateOptions) error {
	// Validate existing chart directory
	info, err := os.Stat(opts.fromDir)
	if err != nil {
		return fmt.Errorf("cannot access existing chart: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", opts.fromDir)
	}

	// Load existing chart from disk
	existingChart, err := loadChartFromDir(opts.fromDir)
	if err != nil {
		return fmt.Errorf("failed to load existing chart: %w", err)
	}

	if opts.verbose {
		fmt.Printf("Loaded existing chart %q from %s\n", existingChart.Name, opts.fromDir)
		fmt.Printf("  Templates: %d\n", len(existingChart.Templates))
	}

	if len(opts.sourceFiles) == 0 {
		return fmt.Errorf("no source files provided: use --source/-f to specify manifest paths")
	}

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
		return fmt.Errorf("invalid mode: %s", opts.mode)
	}

	pipeline, err := runPipeline(ctx, pipelineOptions{
		source:     types.SourceFile,
		extract:    extractor.Options{Paths: opts.sourceFiles, Recursive: true},
		chartName:  opts.chartName,
		outputMode: outputMode,
	})
	if err != nil {
		return err
	}
	graph := pipeline.graph

	// Generate new chart
	generatorRegistry := generator.DefaultRegistry()
	gen, err := generatorRegistry.Get(outputMode)
	if err != nil {
		return fmt.Errorf("failed to get generator: %w", err)
	}

	genOpts := generator.Options{
		ChartName:    opts.chartName,
		ChartVersion: opts.chartVersion,
		AppVersion:   opts.appVersion,
		Mode:         outputMode,
	}

	charts, err := gen.Generate(ctx, graph, genOpts)
	if err != nil {
		return fmt.Errorf("chart generation failed: %w", err)
	}

	if len(charts) == 0 {
		return fmt.Errorf("no charts generated")
	}

	newChart := charts[0]

	// Detect drift
	drift := generator.DetectDrift(existingChart, newChart)

	if !drift.HasDrift() {
		fmt.Println("No drift detected. Charts are identical.")
		return nil
	}

	// Print drift summary
	fmt.Printf("Drift detected: %d changes\n\n", drift.TotalItems())
	if len(drift.Templates) > 0 {
		fmt.Printf("Templates (%d):\n", len(drift.Templates))
		for _, item := range drift.Templates {
			fmt.Printf("  [%s] %s — %s\n", item.Category, item.Path, item.Detail)
		}
		fmt.Println()
	}
	if len(drift.Values) > 0 {
		fmt.Printf("Values (%d):\n", len(drift.Values))
		for _, item := range drift.Values {
			fmt.Printf("  [%s] %s — %s\n", item.Category, item.Path, item.Detail)
		}
		fmt.Println()
	}
	if len(drift.Helpers) > 0 {
		fmt.Printf("Helpers (%d):\n", len(drift.Helpers))
		for _, item := range drift.Helpers {
			fmt.Printf("  [%s] %s — %s\n", item.Category, item.Path, item.Detail)
		}
		fmt.Println()
	}

	// Print migration plan
	plan := generator.GenerateMigrationPlan(drift)
	fmt.Println(plan)

	// Print values migration template if there are value changes
	if len(drift.Values) > 0 {
		migration := generator.GenerateValuesMigration(existingChart.ValuesYAML, newChart.ValuesYAML)
		if !strings.Contains(migration, "No value migrations needed") {
			fmt.Println("## Values Migration Template (_migrate.tpl)")
			fmt.Println(migration)
		}
	}

	return nil
}

// loadChartFromDir reads a Helm chart directory into a GeneratedChart struct.
func loadChartFromDir(dir string) (*types.GeneratedChart, error) {
	chart := &types.GeneratedChart{
		Templates: make(map[string]string),
	}

	// Read Chart.yaml
	chartYAML, err := os.ReadFile(filepath.Join(dir, "Chart.yaml"))
	if err != nil {
		return nil, fmt.Errorf("failed to read Chart.yaml: %w", err)
	}
	chart.ChartYAML = string(chartYAML)

	// Extract name from Chart.yaml
	var chartMeta struct {
		Name string `json:"name"`
	}
	if err := yaml.Unmarshal(chartYAML, &chartMeta); err == nil {
		chart.Name = chartMeta.Name
	}
	chart.Path = dir

	// Read values.yaml (optional)
	if data, err := os.ReadFile(filepath.Join(dir, "values.yaml")); err == nil {
		chart.ValuesYAML = string(data)
	}

	// Read _helpers.tpl (optional)
	if data, err := os.ReadFile(filepath.Join(dir, "templates", "_helpers.tpl")); err == nil {
		chart.Helpers = string(data)
	}

	// Read NOTES.txt (optional)
	if data, err := os.ReadFile(filepath.Join(dir, "templates", "NOTES.txt")); err == nil {
		chart.Notes = string(data)
	}

	// Read templates
	templatesDir := filepath.Join(dir, "templates")
	if info, err := os.Stat(templatesDir); err == nil && info.IsDir() {
		err := filepath.Walk(templatesDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			relPath, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			// Skip _helpers.tpl and NOTES.txt (already loaded)
			base := filepath.Base(path)
			if base == "_helpers.tpl" || base == "NOTES.txt" {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			chart.Templates[relPath] = string(data)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to read templates: %w", err)
		}
	}

	return chart, nil
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

func newFixCmd() *cobra.Command {
	var (
		paths        []string
		outputDir    string
		chartName    string
		workloadType string
		verbose      bool
		recursive    bool
	)

	cmd := &cobra.Command{
		Use:   "fix",
		Short: "Auto-fix Kubernetes manifests with security best practices",
		Long: `Auto-fix Kubernetes manifests by injecting:
  - SecurityContext (runAsNonRoot, readOnlyRootFilesystem, etc.)
  - Resource requests/limits
  - Health probes (liveness, readiness, startup)
  - PodDisruptionBudgets
  - PSS restricted compliance
  - Graceful shutdown hooks

Examples:
  dhg fix -f ./manifests -o ./fixed
  dhg fix -f ./manifests -o ./fixed --workload-type worker`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFix(cmd.Context(), fixOptions{
				paths:        paths,
				outputDir:    outputDir,
				chartName:    chartName,
				workloadType: workloadType,
				verbose:      verbose,
				recursive:    recursive,
			})
		},
	}

	cmd.Flags().StringSliceVarP(&paths, "file", "f", []string{}, "Path(s) to YAML files or directories")
	cmd.Flags().StringVarP(&outputDir, "output", "o", "./fixed", "Output directory for fixed manifests")
	cmd.Flags().StringVar(&chartName, "chart-name", "fixed-chart", "Name of the output chart")
	cmd.Flags().StringVar(&workloadType, "workload-type", "web", "Workload type for resource profiles: web, worker, database, batch, cache")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", true, "Recursively scan directories")

	_ = cmd.MarkFlagRequired("file")

	return cmd
}

type fixOptions struct {
	paths        []string
	outputDir    string
	chartName    string
	workloadType string
	verbose      bool
	recursive    bool
}

func runFix(ctx context.Context, opts fixOptions) error {
	if opts.verbose {
		fmt.Printf("Auto-fix: reading manifests from %v\n", opts.paths)
	}

	pipeline, err := runPipeline(ctx, pipelineOptions{
		source:     types.SourceFile,
		extract:    extractor.Options{Paths: opts.paths, Recursive: opts.recursive},
		chartName:  opts.chartName,
		outputMode: types.OutputModeUniversal,
	})
	if err != nil {
		return err
	}
	graph := pipeline.graph

	// Step 4: Generate chart
	generatorRegistry := generator.DefaultRegistry()
	gen, err := generatorRegistry.Get(types.OutputModeUniversal)
	if err != nil {
		return fmt.Errorf("failed to get generator: %w", err)
	}

	genOpts := generator.Options{
		OutputDir:    opts.outputDir,
		ChartName:    opts.chartName,
		ChartVersion: "0.1.0",
		AppVersion:   "1.0.0",
		Mode:         types.OutputModeUniversal,
	}

	charts, err := gen.Generate(ctx, graph, genOpts)
	if err != nil {
		return fmt.Errorf("chart generation failed: %w", err)
	}

	if len(charts) == 0 {
		return fmt.Errorf("no charts generated")
	}

	// Step 5: Apply all fixes
	wt := generator.WorkloadType(opts.workloadType)
	for i, chart := range charts {
		fixed, fixResult := generator.ApplyAllFixes(chart, wt)
		charts[i] = fixed

		if opts.verbose {
			fmt.Printf("Chart %q fixes applied:\n", chart.Name)
			fmt.Printf("  SecurityContext: %d\n", fixResult.SecurityContextInjected)
			fmt.Printf("  Resources:      %d\n", fixResult.ResourcesInjected)
			fmt.Printf("  HealthProbes:   %d\n", fixResult.HealthProbesInjected)
			fmt.Printf("  PDBs:           %d\n", fixResult.PDBsGenerated)
			fmt.Printf("  PSS Restricted: %d\n", fixResult.PSSRestrictedApplied)
			fmt.Printf("  GracefulShutdown: %d\n", fixResult.GracefulShutdownAdded)
		} else {
			total := fixResult.SecurityContextInjected + fixResult.ResourcesInjected +
				fixResult.HealthProbesInjected + fixResult.PDBsGenerated +
				fixResult.PSSRestrictedApplied + fixResult.GracefulShutdownAdded
			fmt.Printf("Applied %d fixes to chart %q\n", total, chart.Name)
		}
	}

	// Step 6: Write output
	for _, chart := range charts {
		chartDir := filepath.Join(opts.outputDir, chart.Name)
		if err := os.MkdirAll(chartDir, 0o755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}

		if chart.ChartYAML != "" {
			if err := os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(chart.ChartYAML), 0o644); err != nil {
				return fmt.Errorf("writing Chart.yaml: %w", err)
			}
		}

		if chart.ValuesYAML != "" {
			if err := os.WriteFile(filepath.Join(chartDir, "values.yaml"), []byte(chart.ValuesYAML), 0o644); err != nil {
				return fmt.Errorf("writing values.yaml: %w", err)
			}
		}

		if chart.Helpers != "" {
			tplDir := filepath.Join(chartDir, "templates")
			if err := os.MkdirAll(tplDir, 0o755); err != nil {
				return fmt.Errorf("creating templates directory: %w", err)
			}
			if err := os.WriteFile(filepath.Join(tplDir, "_helpers.tpl"), []byte(chart.Helpers), 0o644); err != nil {
				return fmt.Errorf("writing _helpers.tpl: %w", err)
			}
		}

		for tplPath, content := range chart.Templates {
			fullPath := filepath.Join(chartDir, tplPath)
			dir := filepath.Dir(fullPath)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("creating directory %s: %w", dir, err)
			}
			if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
				return fmt.Errorf("writing template %s: %w", tplPath, err)
			}
		}
	}

	fmt.Printf("Fixed charts written to %s\n", opts.outputDir)
	return nil
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

func newFeaturesCmd() *cobra.Command {
	var namesOnly bool
	cmd := &cobra.Command{
		Use:   "features",
		Short: "List optional features available to `generate --with`",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			for _, f := range generator.Features() {
				if namesOnly {
					fmt.Fprintln(out, f.Name)
					continue
				}
				fmt.Fprintf(out, "%s\n    %s\n", f.Name, f.Description)
				keys := make([]string, 0, len(f.Params))
				for k := range f.Params {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					fmt.Fprintf(out, "    --feature-opt %s.%s=%q\n", f.Name, k, f.Params[k])
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&namesOnly, "names", false, "Print feature names only")
	return cmd
}

// chartObjects returns the input objects rendered by the chart's templates.
func chartObjects(chart *types.GeneratedChart, processed []*types.ProcessedResource) []*unstructured.Unstructured {
	var objects []*unstructured.Unstructured
	for _, pr := range processed {
		if pr.Original == nil || pr.Original.Object == nil {
			continue
		}
		if _, ok := chart.Templates[pr.TemplatePath]; ok {
			objects = append(objects, pr.Original.Object)
		}
	}
	return objects
}

// writeKustomizeDir writes a kustomization.yaml and its resources to dir.
func writeKustomizeDir(dir string, kd *generator.KustomizeDir) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", dir, err)
	}
	files := map[string]string{"kustomization.yaml": kd.Kustomization}
	for name, content := range kd.Resources {
		files[name] = content
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", filepath.Join(dir, name), err)
		}
	}
	return nil
}

// writeSynthesisReport prints the notes of a synthesizing source and writes
// them to SYNTHESIS.md in the output directory.
func writeSynthesisReport(outputDir string, p *pipelineResult) error {
	notes := p.synthesis.Notes()
	for _, n := range notes {
		fmt.Fprintf(os.Stderr, "Note: %s\n", n)
	}
	objects := make([]*unstructured.Unstructured, 0, len(p.extracted))
	for _, r := range p.extracted {
		objects = append(objects, r.Object)
	}
	report := synth.Report(p.synthesis.Inputs(), objects, notes)
	path := filepath.Join(outputDir, "SYNTHESIS.md")
	if err := os.WriteFile(path, []byte(report), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	fmt.Printf("  Written: %s (review before deploying)\n", path)
	return nil
}
