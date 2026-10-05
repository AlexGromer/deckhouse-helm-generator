package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/analyzer"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/extractor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

type graphOptions struct {
	paths        []string
	format       string
	outputFile   string
	namespace    string
	includeKinds []string
	excludeKinds []string
	recursive    bool
}

func newGraphCmd() *cobra.Command {
	var opts graphOptions
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Render the resource dependency graph (DOT or Mermaid)",
		Long: `Render the relationships dhg detects between input resources: label
selectors, name references, volume mounts, env references, service accounts.
The graph goes to stdout (or --output); a structure summary with the coupling
between service groups and any dependency cycles goes to stderr.

Examples:
  dhg graph -f ./manifests | dot -Tsvg > graph.svg
  dhg graph -f ./manifests --format mermaid > graph.mmd`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGraph(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringSliceVarP(&opts.paths, "file", "f", nil, "Path(s) to YAML files or directories")
	cmd.Flags().StringVar(&opts.format, "format", "dot", "Output format: dot or mermaid")
	cmd.Flags().StringVarP(&opts.outputFile, "output", "o", "", "Write the graph to a file instead of stdout")
	cmd.Flags().StringVarP(&opts.namespace, "namespace", "n", "", "Filter by namespace")
	cmd.Flags().StringSliceVar(&opts.includeKinds, "include-kinds", nil, "Include only these resource kinds")
	cmd.Flags().StringSliceVar(&opts.excludeKinds, "exclude-kinds", nil, "Exclude these resource kinds")
	cmd.Flags().BoolVarP(&opts.recursive, "recursive", "r", true, "Recursively scan directories")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func runGraph(ctx context.Context, opts graphOptions) error {
	var render func(*types.ResourceGraph) string
	switch opts.format {
	case "dot":
		render = analyzer.GenerateDOTGraph
	case "mermaid":
		render = analyzer.GenerateMermaidGraph
	default:
		return fmt.Errorf("invalid --format %q (dot or mermaid)", opts.format)
	}

	pipeline, err := runPipeline(ctx, pipelineOptions{
		source: types.SourceFile,
		extract: extractor.Options{
			Paths:        opts.paths,
			Namespace:    opts.namespace,
			IncludeKinds: opts.includeKinds,
			ExcludeKinds: opts.excludeKinds,
			Recursive:    opts.recursive,
		},
		chartName:  "graph",
		outputMode: types.OutputModeUniversal,
		lenient:    true,
	})
	if err != nil {
		return err
	}
	graph := pipeline.graph

	out := render(graph)
	if opts.outputFile != "" {
		if err := os.WriteFile(opts.outputFile, []byte(out), 0o644); err != nil {
			return err
		}
	} else {
		fmt.Print(out)
	}

	decomposition := analyzer.AnalyzeDecomposition(graph)
	fmt.Fprintf(os.Stderr, "%d resources, %d relationships, %d service groups: %s\n",
		len(graph.Resources), len(graph.Relationships), len(graph.Groups), decomposition.Reason)
	if err := analyzer.DetectCircularDependencies(graph); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}
	return nil
}
