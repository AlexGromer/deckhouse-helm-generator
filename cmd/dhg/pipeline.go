package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/analyzer"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/analyzer/detector"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/extractor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor/k8s"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor/value"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

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
}

// pipelineResult is the analyzed input of a command.
type pipelineResult struct {
	extracted     []*types.ExtractedResource
	processed     []*types.ProcessedResource
	graph         *types.ResourceGraph
	externalFiles *value.ExternalFileManager
}

func runPipeline(ctx context.Context, opts pipelineOptions) (*pipelineResult, error) {
	extracted, err := extractResources(ctx, opts)
	if err != nil {
		return nil, err
	}

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
		})
		if opts.verbose {
			fmt.Printf("  Processed: %s -> service: %s\n", r.ResourceKey(), result.ServiceName)
		}
	}

	for _, note := range processor.ResolveCollisions(processed) {
		fmt.Fprintf(os.Stderr, "Note: %s\n", note)
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

	return &pipelineResult{extracted: extracted, processed: processed, graph: graph, externalFiles: externalFiles}, nil
}

// extractResources runs the extractor for opts.source, drops duplicate
// objects and warns about deprecated APIs.
func extractResources(ctx context.Context, opts pipelineOptions) ([]*types.ExtractedResource, error) {
	if opts.verbose {
		fmt.Printf("\n[1/5] Extracting resources from %s...\n", opts.source)
	}
	ext, ok := extractor.DefaultRegistry().Get(opts.source)
	if !ok {
		return nil, fmt.Errorf("no extractor available for source type: %s", opts.source)
	}
	if err := ext.Validate(ctx, opts.extract); err != nil {
		return nil, fmt.Errorf("extractor validation failed: %w", err)
	}

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
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if len(resources) == 0 {
		return nil, fmt.Errorf("no resources extracted")
	}

	resources, duplicates := extractor.Deduplicate(resources)
	for _, d := range duplicates {
		fmt.Fprintf(os.Stderr, "Warning: %s is defined more than once; using the first definition, ignoring %s\n", d.ResourceKey(), d.SourcePath)
	}
	warnDeprecatedAPIs(resources)
	if opts.verbose {
		fmt.Printf("  Total extracted: %d resources\n", len(resources))
	}
	return resources, nil
}
