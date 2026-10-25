package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/analyzer"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/extractor"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/generator"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

type graphOptions struct {
	paths        []string
	format       string
	outputFile   string
	namespace    string
	includeKinds []string
	excludeKinds []string
	recursive    bool
	level        string
	groupBy      string
	pbc          []string
	pbcLabel     string
	product      string
	connections  bool
}

func newGraphCmd() *cobra.Command {
	var opts graphOptions
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Render the resource dependency graph",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGraph(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringSliceVarP(&opts.paths, "file", "f", nil, "Path(s) to YAML files or directories")
	cmd.Flags().StringVar(&opts.format, "format", "dot", "Output format: dot, mermaid, json, or report")
	cmd.Flags().StringVarP(&opts.outputFile, "output", "o", "", "Write the graph to a file instead of stdout")
	cmd.Flags().StringVarP(&opts.namespace, "namespace", "n", "", "Filter by namespace")
	cmd.Flags().StringSliceVar(&opts.includeKinds, "include-kinds", nil, "Include only these resource kinds")
	cmd.Flags().StringSliceVar(&opts.excludeKinds, "exclude-kinds", nil, "Exclude these resource kinds")
	cmd.Flags().BoolVarP(&opts.recursive, "recursive", "r", true, "Recursively scan directories")
	cmd.Flags().StringVar(&opts.level, "level", "resource", "Graph level: resource, service, pbc")
	cmd.Flags().StringVar(&opts.groupBy, "group-by", "", "Group by: \"\" or pbc")
	cmd.Flags().StringSliceVar(&opts.pbc, "pbc", nil, "Explicit PBC mappings (<pbc>=<svc1>,<svc2>)")
	cmd.Flags().StringVar(&opts.pbcLabel, "pbc-label", "app.kubernetes.io/part-of", "Label for PBC discovery")
	cmd.Flags().StringVar(&opts.product, "product", "", "Product name for JSON/report")
	cmd.Flags().BoolVar(&opts.connections, "connections", true, "Include connections in graph")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func runGraph(ctx context.Context, opts graphOptions) error {
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

	explicitMap, err := generator.ParsePBCMapping(opts.pbc)
	if err != nil {
		return err
	}
	topology := generator.ResolveTopology(graph, explicitMap)

	var out string
	switch opts.format {
	case "dot":
		switch opts.level {
		case "service":
			out = analyzer.RenderServiceGraphDOT(graph)
		case "pbc":
			out = analyzer.RenderPBCGraphDOT(graph, topology)
		default:
			out = analyzer.GenerateDOTGraph(graph)
		}
	case "mermaid":
		switch opts.level {
		case "service":
			out = analyzer.RenderServiceGraphMermaid(graph)
		case "pbc":
			out = analyzer.RenderPBCGraphMermaid(graph, topology)
		default:
			out = analyzer.GenerateMermaidGraph(graph)
		}
	case "json":
		out = analyzer.FormatGraphJSON(graph, topology)
	case "report":
		out = analyzer.GeneratePBCReport(graph, topology)
	default:
		return fmt.Errorf("invalid --format %q (dot, mermaid, json, report)", opts.format)
	}

	if opts.outputFile != "" {
		if err := os.WriteFile(opts.outputFile, []byte(out), 0o644); err != nil {
			return err
		}
	} else {
		fmt.Print(out)
	}

	return nil
}
