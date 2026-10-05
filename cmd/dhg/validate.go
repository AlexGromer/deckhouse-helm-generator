package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template/parse"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/generator"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

func newValidateCmd() *cobra.Command {
	var (
		paths        []string
		verbose      bool
		kubeVersions string
	)

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate Helm chart structure, templates and Kubernetes API compatibility",
		Long: `Validate Helm charts without a cluster:
  - Chart.yaml presence and required fields
  - values.yaml syntax
  - Go template syntax of every file under templates/ (Helm functions allowed)
  - Kubernetes API compatibility: APIs removed in, or not yet available in,
    any version of --kube-versions are errors; deprecated APIs are warnings

For rendering and schema validation against a real cluster version, also run
` + "`helm lint`" + ` and ` + "`helm template | kubeconform`" + `.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValidate(cmd.Context(), validateOptions{
				paths:        paths,
				verbose:      verbose,
				kubeVersions: kubeVersions,
			})
		},
	}

	cmd.Flags().StringSliceVarP(&paths, "file", "f", []string{"."}, "Path(s) to chart directories to validate")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")
	cmd.Flags().StringVar(&kubeVersions, "kube-versions", "1.27-1.32", "Kubernetes versions to check API compatibility against: a range (1.27-1.32) or a list (1.29,1.31); empty disables the check")

	return cmd
}

type validateOptions struct {
	paths        []string
	verbose      bool
	kubeVersions string
}

// validationReport collects findings for one validate run.
type validationReport struct {
	errors   int
	warnings int
	verbose  bool
}

func (r *validationReport) errorf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "  ERROR: "+format+"\n", args...)
	r.errors++
}

func (r *validationReport) warnf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "  WARNING: "+format+"\n", args...)
	r.warnings++
}

func (r *validationReport) okf(format string, args ...interface{}) {
	if r.verbose {
		fmt.Printf("  OK: "+format+"\n", args...)
	}
}

func runValidate(_ context.Context, opts validateOptions) error {
	versionOpts, err := parseKubeVersions(opts.kubeVersions)
	if err != nil {
		return err
	}

	report := &validationReport{verbose: opts.verbose}
	for _, chartPath := range opts.paths {
		fmt.Printf("Validating chart at: %s\n", chartPath)
		validateChartYAML(report, chartPath)
		validateValuesYAML(report, chartPath)
		templates := validateTemplates(report, chartPath)
		if versionOpts != nil && len(templates) > 0 {
			validateAPIs(report, chartPath, templates, *versionOpts)
		}
	}

	fmt.Printf("\nValidation complete: %d error(s), %d warning(s)\n", report.errors, report.warnings)
	if report.errors > 0 {
		return fmt.Errorf("validation failed with %d error(s)", report.errors)
	}
	return nil
}

// parseKubeVersions parses "1.27-1.32" or "1.29,1.31"; empty means no check.
func parseKubeVersions(spec string) (*generator.K8sVersionOptions, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	if lo, hi, ok := strings.Cut(spec, "-"); ok {
		return &generator.K8sVersionOptions{MinVersion: strings.TrimSpace(lo), MaxVersion: strings.TrimSpace(hi)}, nil
	}
	var versions []string
	for _, v := range strings.Split(spec, ",") {
		if v = strings.TrimSpace(v); v != "" {
			versions = append(versions, v)
		}
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("invalid --kube-versions %q", spec)
	}
	return &generator.K8sVersionOptions{TargetVersions: versions}, nil
}

func validateChartYAML(r *validationReport, chartPath string) {
	chartYAMLPath := filepath.Join(chartPath, "Chart.yaml")
	data, err := os.ReadFile(chartYAMLPath)
	if os.IsNotExist(err) {
		r.errorf("Chart.yaml not found at %s", chartYAMLPath)
		return
	}
	if err != nil {
		r.errorf("Cannot read Chart.yaml: %v", err)
		return
	}
	var chart map[string]interface{}
	if err := yaml.Unmarshal(data, &chart); err != nil {
		r.errorf("Invalid YAML in Chart.yaml: %v", err)
		return
	}
	for _, field := range []string{"apiVersion", "name", "version"} {
		if v, ok := chart[field]; !ok || fmt.Sprint(v) == "" {
			r.errorf("Chart.yaml missing required field: %s", field)
		}
	}
	r.okf("Chart.yaml found (%d bytes)", len(data))
}

func validateValuesYAML(r *validationReport, chartPath string) {
	valuesPath := filepath.Join(chartPath, "values.yaml")
	data, err := os.ReadFile(valuesPath)
	if os.IsNotExist(err) {
		r.warnf("values.yaml not found")
		return
	}
	if err != nil {
		r.errorf("Cannot read values.yaml: %v", err)
		return
	}
	var values map[string]interface{}
	if err := yaml.Unmarshal(data, &values); err != nil {
		r.errorf("Invalid YAML in values.yaml: %v", err)
		return
	}
	r.okf("values.yaml valid (%d bytes)", len(data))
}

// validateTemplates parses every template under templates/ and returns their
// contents keyed by chart-relative path.
func validateTemplates(r *validationReport, chartPath string) map[string]string {
	templatesDir := filepath.Join(chartPath, "templates")
	if _, err := os.Stat(templatesDir); os.IsNotExist(err) {
		r.warnf("templates/ directory not found")
		return nil
	}

	templates := map[string]string{}
	err := filepath.WalkDir(templatesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		ext := filepath.Ext(path)
		if d.IsDir() || (ext != ".yaml" && ext != ".yml" && ext != ".tpl" && ext != ".txt") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			r.errorf("Cannot read template %s: %v", path, err)
			return nil
		}
		rel, _ := filepath.Rel(chartPath, path)
		templates[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		r.errorf("Cannot read templates directory: %v", err)
		return nil
	}

	paths := make([]string, 0, len(templates))
	for p := range templates {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	// All templates of a chart share one namespace of defines, as in Helm.
	treeSet := map[string]*parse.Tree{}
	for _, p := range paths {
		if err := parseTemplate(p, templates[p], treeSet); err != nil {
			r.errorf("Template syntax error: %v", err)
			continue
		}
		r.okf("%s", p)
	}
	r.okf("Templates: %d files checked", len(templates))
	return templates
}

// parseTemplate checks Go template syntax. Function names are not checked:
// Helm adds sprig and its own functions at render time.
func parseTemplate(name, text string, treeSet map[string]*parse.Tree) error {
	t := parse.New(name)
	t.Mode = parse.SkipFuncCheck | parse.ParseComments
	_, err := t.Parse(text, "", "", treeSet)
	return err
}

func validateAPIs(r *validationReport, chartPath string, templates map[string]string, opts generator.K8sVersionOptions) {
	chart := &types.GeneratedChart{Name: filepath.Base(chartPath), Templates: templates}

	matrix := generator.ValidateK8sVersionMatrix(chart, opts)
	versions := make([]string, 0, len(matrix.Compatibility))
	for v := range matrix.Compatibility {
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool { return minorOf(versions[i]) < minorOf(versions[j]) })
	for _, v := range versions {
		c := matrix.Compatibility[v]
		for _, issue := range c.Issues {
			r.errorf("Kubernetes %s: %s", v, issue)
		}
		if c.Compatible {
			r.okf("Kubernetes %s: compatible", v)
		}
	}

	deprecations := generator.CheckDeprecatedAPIs(chart, generator.PlutoCheckOptions{})
	for _, d := range deprecations.Deprecations {
		r.warnf("%s: %s/%s is deprecated: %s", d.TemplatePath, d.APIVersion, d.Kind, d.Message)
	}
}

func minorOf(v string) int {
	var major, minor int
	_, _ = fmt.Sscanf(strings.TrimPrefix(v, "v"), "%d.%d", &major, &minor)
	return minor
}

// warnDeprecatedAPIs prints a warning for every input manifest that uses a
// deprecated or removed Kubernetes API, with the replacement to migrate to.
func warnDeprecatedAPIs(resources []*types.ExtractedResource) {
	for _, r := range resources {
		info := generator.GetMigrationInfo(r.Object.GetAPIVersion(), r.Object.GetKind())
		if info == nil {
			continue
		}
		replacement := "no direct replacement"
		if info.NewAPIVersion != "" {
			replacement = "use " + info.NewAPIVersion
		}
		msg := fmt.Sprintf("Warning: %s uses %s/%s, deprecated in 1.%s and removed in %s; %s",
			r.ResourceKey().String(), info.OldAPIVersion, info.OldKind,
			strings.TrimPrefix(info.DeprecatedIn, "1."), info.RemovedIn, replacement)
		if info.Notes != "" {
			msg += " (" + info.Notes + ")"
		}
		fmt.Fprintln(os.Stderr, msg)
	}
}
