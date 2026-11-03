package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/extractor"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/synth"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Work with platform profiles (--synth-profile)",
	}
	cmd.AddCommand(newProfileInferCmd())
	return cmd
}

type profileInferOptions struct {
	source      string
	paths       []string
	kubeConfig  string
	kubeContext string
	namespace   string
	selector    string
	minShare    float64
	output      string
	valuesOut   string
}

func newProfileInferCmd() *cobra.Command {
	var opts profileInferOptions
	cmd := &cobra.Command{
		Use:   "infer",
		Short: "Infer a platform profile from the Deployments of a stand",
		Long: `Infer a platform profile (see docs/SYNTH_PROFILE.md) from the Deployments of a
running stand or from manifests: the name prefix, init containers, envFrom,
volumes, labels, probes, resources, rollout strategy, Service type and extra
ports, and the container and pod fields that at least --min-share of the
Deployments have in common. What is particular to one service (its name) becomes
a template; addresses of the environment (IPv4, host names, jdbc/http URLs)
become variables, with the stand's values in the file given by --values-out.
Addresses are recognised when they are the whole string or follow "name=";
inside a command only IPv4 addresses and URLs are, as separate words. The
credentials of a URL (user:password@) are dropped and noted, never written.

The profile is the most frequent shape of the Deployments read, not a
specification: review it before use.`,
		Example: `  dhg profile infer -n shop -o profile.yaml --values-out values-shop.yaml
  dhg profile infer -s file -f ./manifests -o profile.yaml`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProfileInfer(cmd.Context(), cmd, opts)
		},
	}
	cmd.Flags().StringVarP(&opts.source, "source", "s", "cluster", "Where the Deployments come from: cluster (live cluster via kubeconfig) or file")
	cmd.Flags().StringSliceVarP(&opts.paths, "file", "f", nil, "Path(s) to YAML files or directories (--source file)")
	cmd.Flags().StringVar(&opts.kubeConfig, "kubeconfig", "", "Path to kubeconfig file")
	cmd.Flags().StringVar(&opts.kubeContext, "context", "", "Kubeconfig context to use")
	cmd.Flags().StringVarP(&opts.namespace, "namespace", "n", "", "Read this namespace only")
	cmd.Flags().StringVarP(&opts.selector, "selector", "l", "", "Label selector filter")
	cmd.Flags().Float64Var(&opts.minShare, "min-share", synth.DefaultInferShare, "Share of the Deployments (above 0, up to 1) that must agree on a property for it to enter the profile")
	cmd.Flags().StringVarP(&opts.output, "output", "o", "", "Write the profile to this file instead of stdout")
	cmd.Flags().StringVar(&opts.valuesOut, "values-out", "", "Write the stand's values of the profile's variables to this file (e.g. values-<namespace>.yaml); without it they stay in the profile's vars")
	return cmd
}

func runProfileInfer(ctx context.Context, cmd *cobra.Command, opts profileInferOptions) error {
	if !(opts.minShare > 0 && opts.minShare <= 1) {
		return fmt.Errorf("--min-share is %v, want a number above 0 and up to 1", opts.minShare)
	}
	var source types.Source
	described := opts.source
	switch opts.source {
	case "cluster":
		source = types.SourceCluster
	case "file":
		source = types.SourceFile
		if len(opts.paths) == 0 {
			return fmt.Errorf("at least one path is required for file source (-f flag)")
		}
	default:
		return fmt.Errorf("invalid source: %s (must be cluster or file)", opts.source)
	}
	if opts.namespace != "" {
		described += ", namespace " + opts.namespace
	}
	if opts.selector != "" {
		described += ", selector " + opts.selector
	}

	extracted, _, err := extractResources(ctx, pipelineOptions{
		source: source,
		extract: extractor.Options{
			Paths:          opts.paths,
			Namespace:      opts.namespace,
			LabelSelector:  opts.selector,
			IncludeKinds:   []string{"Deployment", "Service"},
			Recursive:      true,
			KubeConfig:     opts.kubeConfig,
			KubeContext:    opts.kubeContext,
			ClusterSecrets: "skip",
		},
	})
	if err != nil {
		return err
	}
	objs := make([]*unstructured.Unstructured, 0, len(extracted))
	for _, r := range extracted {
		if r.Object != nil {
			objs = append(objs, r.Object)
		}
	}
	res, err := synth.InferProfile(objs, synth.InferOptions{
		MinShare:   opts.minShare,
		Source:     described,
		KeepValues: opts.valuesOut == "",
	})
	if err != nil {
		return err
	}
	for _, n := range res.Notes {
		fmt.Fprintf(cmd.ErrOrStderr(), "Note: %s\n", n)
	}

	if opts.output == "" {
		_, err = cmd.OutOrStdout().Write(res.Profile)
		return err
	}
	if err := os.WriteFile(opts.output, res.Profile, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Profile inferred from %d Deployments: %s\n", res.Deployments, opts.output)
	switch {
	case res.Values != nil:
		if err := os.WriteFile(opts.valuesOut, res.Values, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Values of the profile's variables: %s\n", opts.valuesOut)
	case opts.valuesOut != "":
		fmt.Fprintf(cmd.ErrOrStderr(), "Note: no environment-specific values found; %s is not written\n", opts.valuesOut)
	}
	return nil
}
