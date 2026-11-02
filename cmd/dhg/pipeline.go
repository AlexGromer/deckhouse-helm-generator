package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/analyzer"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/analyzer/detector"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/extractor"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor/k8s"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor/value"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// workloadKinds are the kinds with a pod template, whose references to
// ConfigMaps, Secrets, ServiceAccounts and PVCs must be satisfied at install.
var workloadKinds = map[string]bool{
	"Deployment": true, "StatefulSet": true, "DaemonSet": true,
	"Job": true, "CronJob": true, "Pod": true, "ReplicaSet": true,
}

// referencedKinds are the dependency kinds missingReferenceNotes reports.
var referencedKinds = map[string]bool{
	"ConfigMap": true, "Secret": true, "ServiceAccount": true, "PersistentVolumeClaim": true,
}

// missingReferenceNotes lists what workloads reference (envFrom, volumes,
// ServiceAccount, imagePullSecrets) but the chart does not contain. Objects
// every namespace gets on its own are not reported.
func missingReferenceNotes(processed []*types.ProcessedResource, all map[types.ResourceKey]*types.ExtractedResource, source types.Source, clusterSecrets string) []string {
	secretsSkipped := source == types.SourceCluster && clusterSecrets != "mask" && clusterSecrets != "include"
	var notes []string
	seen := map[string]bool{}
	for _, p := range processed {
		owner := p.Original.ResourceKey()
		if !workloadKinds[owner.GVK.Kind] {
			continue
		}
		for _, dep := range p.Dependencies {
			kind := dep.GVK.Kind
			if !referencedKinds[kind] {
				continue
			}
			if (kind == "ServiceAccount" && dep.Name == "default") || (kind == "ConfigMap" && dep.Name == "kube-root-ca.crt") {
				continue
			}
			if _, ok := all[dep]; ok {
				continue
			}
			// A definition written without a namespace applies to the workload's.
			unscoped := dep
			unscoped.Namespace = ""
			if _, ok := all[unscoped]; ok {
				continue
			}
			note := fmt.Sprintf("%s %s is referenced by %s %s but is not in the chart (create it before install", kind, dep.Name, owner.GVK.Kind, owner.Name)
			if kind == "Secret" && secretsSkipped {
				note += "; Secrets are skipped by --cluster-secrets skip"
			}
			note += ")"
			if !seen[note] {
				seen[note] = true
				notes = append(notes, note)
			}
		}
	}
	return notes
}

// printExtractWarning reports an error the extractor sent without failing:
// notes as "Note:", the aggregated permission errors as one summary line
// (with the per-kind detail under -v), anything else as "Warning:".
func printExtractWarning(w io.Writer, err error, verbose bool) {
	var note *extractor.NoteError
	var forbidden *extractor.ForbiddenKindsError
	switch {
	case errors.As(err, &note):
		fmt.Fprintf(w, "Note: %s\n", note.Msg)
	case errors.As(err, &forbidden):
		if verbose {
			fmt.Fprintf(w, "Warning: %v\n", forbidden)
			for _, d := range forbidden.Details {
				fmt.Fprintf(w, "  %s\n", d)
			}
		} else {
			fmt.Fprintf(w, "Warning: %v (use -v for details)\n", forbidden)
		}
	default:
		fmt.Fprintf(w, "Warning: %v\n", err)
	}
}

// pipelineOptions configures the extract → process → analyze stages shared
// by generate, analyze, graph, migrate and fix.
type pipelineOptions struct {
	source     types.Source
	extract    extractor.Options
	chartName  string
	outputMode types.OutputMode
	// plugins are --plugin specs (<apiVersion>/<Kind>=<executable>).
	plugins []string
	// lenient skips resources a processor fails on instead of failing.
	lenient bool
	verbose bool
	// quiet drops the notes of the process stage, for repeated runs.
	quiet bool
}

// pipelineResult is the analyzed input of a command.
type pipelineResult struct {
	extracted     []*types.ExtractedResource
	processed     []*types.ProcessedResource
	graph         *types.ResourceGraph
	externalFiles *value.ExternalFileManager
	// synthesis is set when the source built the manifests (image, compose,
	// source) instead of reading them.
	synthesis extractor.Reporter
	notes     []string
}

func runPipeline(ctx context.Context, opts pipelineOptions) (*pipelineResult, error) {
	extracted, synthesis, err := extractResources(ctx, opts)
	if err != nil {
		return nil, err
	}
	result, err := processResources(ctx, opts, extracted)
	if err != nil {
		return nil, err
	}
	result.synthesis = synthesis
	return result, nil
}

// processResources runs the process and analyze stages on extracted
// resources.
func processResources(ctx context.Context, opts pipelineOptions, extracted []*types.ExtractedResource) (*pipelineResult, error) {
	registry := processor.NewRegistry()
	k8s.RegisterAll(registry)
	for _, spec := range opts.plugins {
		path, gvks, err := processor.ParsePluginSpec(spec)
		if err != nil {
			return nil, err
		}
		registry.Register(processor.NewPluginProcessor(path, 30*time.Second, gvks...))
	}

	if opts.verbose {
		fmt.Printf("\n[2/5] Processing resources...\n")
	}
	externalFiles := value.NewExternalFileManager()
	valueProcessor := value.DefaultProcessor()
	all := make(map[types.ResourceKey]*types.ExtractedResource, len(extracted))
	for _, r := range extracted {
		all[r.ResourceKey()] = r
	}

	var allNotes []string
	seenNotes := map[string]bool{}

	processed := make([]*types.ProcessedResource, 0, len(extracted))
	for _, r := range extracted {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result, err := registry.Process(processor.Context{
			Ctx:                 ctx,
			ChartName:           opts.chartName,
			OutputMode:          opts.outputMode,
			Namespace:           r.Object.GetNamespace(),
			AllResources:        all,
			ExternalFileManager: externalFiles,
			ValueProcessor:      valueProcessor,
		}, r.Object)
		if err != nil {
			if opts.lenient {
				fmt.Fprintf(os.Stderr, "Warning: skipping %s: %v\n", r.ResourceKey(), err)
				continue
			}
			return nil, fmt.Errorf("failed to process %s: %w", r.ResourceKey(), err)
		}
		processed = append(processed, &types.ProcessedResource{
			Original:        r,
			ServiceName:     result.ServiceName,
			TemplatePath:    result.TemplatePath,
			TemplateContent: result.TemplateContent,
			ValuesPath:      result.ValuesPath,
			Values:          result.Values,
			Dependencies:    result.Dependencies,
			Notes:           result.Notes,
		})

		for _, note := range result.Notes {
			if !seenNotes[note] {
				seenNotes[note] = true
				allNotes = append(allNotes, note)
			}
		}

		if opts.verbose {
			fmt.Printf("  Processed: %s -> service: %s\n", r.ResourceKey(), result.ServiceName)
		}
	}

	for _, note := range missingReferenceNotes(processed, all, opts.source, opts.extract.ClusterSecrets) {
		if !seenNotes[note] {
			seenNotes[note] = true
			allNotes = append(allNotes, note)
		}
	}

	for _, note := range processor.ResolveCollisions(processed) {
		if !opts.quiet {
			fmt.Fprintf(os.Stderr, "Note: %s\n", note)
		}
	}

	if !opts.quiet {
		for _, note := range allNotes {
			fmt.Fprintf(os.Stderr, "Note: %s\n", note)
		}
	}

	if opts.verbose {
		fmt.Printf("\n[3/5] Analyzing relationships...\n")
	}
	a := analyzer.NewDefaultAnalyzer()
	detector.RegisterAll(a)
	graph, err := a.Analyze(ctx, processed)
	if err != nil {
		return nil, fmt.Errorf("analysis failed: %w", err)
	}
	if opts.verbose {
		fmt.Printf("  Detected relationships: %d\n  Service groups: %d\n", len(graph.Relationships), len(graph.Groups))
	}

	return &pipelineResult{extracted: extracted, processed: processed, graph: graph, externalFiles: externalFiles, notes: allNotes}, nil
}

// extractResources runs the extractor for opts.source, drops duplicate
// objects and warns about deprecated APIs. For sources that synthesize
// manifests it also returns their Reporter, and any error is fatal: a
// half-read image or compose file would give a misleading chart.
func extractResources(ctx context.Context, opts pipelineOptions) ([]*types.ExtractedResource, extractor.Reporter, error) {
	if opts.verbose {
		fmt.Printf("\n[1/5] Extracting resources from %s...\n", opts.source)
	}
	ext, ok := extractor.DefaultRegistry().Get(opts.source)
	if !ok {
		return nil, nil, fmt.Errorf("no extractor available for source type: %s", opts.source)
	}
	if err := ext.Validate(ctx, opts.extract); err != nil {
		return nil, nil, fmt.Errorf("extractor validation failed: %w", err)
	}

	reporter, synthesizes := ext.(extractor.Reporter)
	resCh, errCh := ext.Extract(ctx, opts.extract)
	var resources []*types.ExtractedResource
	for resCh != nil || errCh != nil {
		select {
		case r, ok := <-resCh:
			if !ok {
				resCh = nil
				continue
			}
			resources = append(resources, r)
			if opts.verbose {
				fmt.Printf("  Extracted: %s\n", r.ResourceKey())
			}
		case err, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}
			if synthesizes {
				return nil, nil, err
			}
			printExtractWarning(os.Stderr, err, opts.verbose)
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}
	if len(resources) == 0 {
		return nil, nil, fmt.Errorf("no resources extracted")
	}

	resources, duplicates := extractor.Deduplicate(resources)
	for _, d := range duplicates {
		fmt.Fprintf(os.Stderr, "Warning: %s is defined more than once; using the first definition, ignoring %s\n", d.ResourceKey(), d.SourcePath)
	}
	warnDeprecatedAPIs(resources)
	if opts.verbose {
		fmt.Printf("  Total extracted: %d resources\n", len(resources))
	}
	if !synthesizes {
		reporter = nil
	}
	return resources, reporter, nil
}
