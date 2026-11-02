package generator

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/helm"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// LibraryGenerator generates a Helm library chart holding the shared helpers
// (names, labels, selector labels, image, ...) plus one application chart per
// service group whose templates use those shared helpers instead of carrying
// their own _helpers.tpl — the pattern of common library charts such as
// bitnami/common.
type LibraryGenerator struct {
	BaseGenerator
}

// NewLibraryGenerator creates a new LibraryGenerator.
func NewLibraryGenerator() *LibraryGenerator {
	return &LibraryGenerator{
		BaseGenerator: NewBaseGenerator(types.OutputModeLibrary),
	}
}

// Generate creates a library chart and wrapper charts from the resource graph.
func (g *LibraryGenerator) Generate(ctx context.Context, graph *types.ResourceGraph, opts Options) ([]*types.GeneratedChart, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	libName, repository, err := libraryTarget(opts)
	if err != nil {
		return nil, err
	}

	charts := make([]*types.GeneratedChart, 0)

	// Always generate the library chart with the shared helpers. With
	// --library-ref it carries the registry chart's name, so publishing it
	// provides the helpers the service charts include.
	// The library does not depend on the input: it is published once and
	// shared. The names of the objects of a chart (global.namePrefix) are
	// written into each service chart instead.
	inChart := inChartNames(graph)
	libChart := g.generateLibraryChart(libName, libraryChartVersion(opts), opts)
	charts = append(charts, libChart)

	// Group resources and generate wrapper charts.
	groupResult, err := GroupResources(graph)
	if err != nil {
		return nil, fmt.Errorf("failed to group resources: %w", err)
	}

	for _, group := range groupResult.Groups {
		if group.Name == libChart.Name {
			return nil, fmt.Errorf("the chart name %q of --library-ref is also the name of a service chart; the library would overwrite it, name the library chart differently", libChart.Name)
		}
	}
	for _, group := range groupResult.Groups {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		wrapper, err := g.generateWrapperChart(group, libChart, repository, opts, inChart)
		if err != nil {
			return nil, fmt.Errorf("failed to generate chart for group %s: %w", group.Name, err)
		}
		charts = append(charts, wrapper)
	}

	return charts, nil
}

// generateLibraryChart creates the library chart with the shared helpers.
func (g *LibraryGenerator) generateLibraryChart(chartName, version string, opts Options) *types.GeneratedChart {
	chartMeta := helm.ChartMetadata{
		Name:        chartName,
		Version:     version,
		AppVersion:  opts.AppVersion,
		Description: "Library chart with helpers shared by the service charts",
		APIVersion:  "v2",
		Type:        "library",
		Keywords:    []string{"kubernetes", "library", "deckhouse"},
	}

	return &types.GeneratedChart{
		Name:       chartName,
		Path:       opts.OutputDir,
		ChartYAML:  helm.GenerateChartYAML(chartMeta),
		ValuesYAML: "# Library charts do not have values.yaml\n# Values are provided by wrapper charts\n",
		Templates:  map[string]string{},
		Helpers:    helm.GenerateLibraryHelpers(chartName),
	}
}

// generateWrapperChart creates the application chart for a service group.
// It is a separate-mode chart whose helper references point at the library.
// repository is "" when the library is vendored into the chart, otherwise the
// OCI repository the library is pulled from.
func (g *LibraryGenerator) generateWrapperChart(group *ServiceGroup, library *types.GeneratedChart, repository string, opts Options, inChart map[string][]string) (*types.GeneratedChart, error) {
	libraryName := library.Name
	sep := &SeparateGenerator{}
	chart, err := sep.generateChartForGroup(group, opts, opts.ChartName, inChart)
	if err != nil {
		return nil, err
	}

	dep := helm.Dependency{
		Name:    libraryName,
		Version: opts.ChartVersion,
		// The library is vendored into the chart (below), so lint and
		// template work without helm dependency build.
		Repository: "file://charts/" + libraryName,
	}
	if repository != "" {
		dep.Version = opts.LibraryVersion
		dep.Repository = repository
	}
	chart.ChartYAML = helm.GenerateChartYAML(helm.ChartMetadata{
		Name:         group.Name,
		Version:      opts.ChartVersion,
		AppVersion:   opts.AppVersion,
		Description:  fmt.Sprintf("Helm chart for %s (helpers from the %s chart)", group.Name, libraryName),
		APIVersion:   "v2",
		Type:         "application",
		Keywords:     []string{"kubernetes", "deckhouse"},
		Dependencies: []helm.Dependency{dep},
	})
	for path, content := range chart.Templates {
		chart.Templates[path] = rewriteHelperReferences(content, group.Name, libraryName)
	}
	chart.Notes = rewriteHelperReferences(chart.Notes, group.Name, libraryName)
	// The library's ref helper reads the object names from the chart that
	// includes it; the file is not rewritten (the rewrite would turn its define into
	// one of the library) and is not a manifest, so it is not a template entry.
	chart.ExternalFiles = append(chart.ExternalFiles, types.ExternalFileInfo{
		Path: inChartTemplatePath, Content: helm.GenerateInChartTemplate(group.Name, inChart),
	})
	chart.Helpers = ""
	if repository == "" {
		chart.ExternalFiles = append(chart.ExternalFiles, vendorLibrary(library)...)
	}
	return chart, nil
}

// inChartTemplatePath is the template of a service chart that defines the
// names of its objects for the helpers of the library.
const inChartTemplatePath = "templates/_dhg-inchart.tpl"

// exactVersion matches a plain semver, not a range.
var exactVersion = regexp.MustCompile(`^v?\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

// libraryChartVersion is the version of the library chart that is written: an
// exact --library-version (so the published chart satisfies the dependency),
// otherwise the chart version.
func libraryChartVersion(opts Options) string {
	if opts.LibraryRef != "" && exactVersion.MatchString(opts.LibraryVersion) {
		return strings.TrimPrefix(opts.LibraryVersion, "v")
	}
	return opts.ChartVersion
}

// defaultLibraryName is the name of the library chart that is vendored into
// the service charts.
const defaultLibraryName = "library"

// libraryTarget returns the library chart name and the repository the service
// charts pull it from; the repository is "" when the library is vendored.
func libraryTarget(opts Options) (name, repository string, err error) {
	if opts.LibraryRef == "" {
		if opts.LibraryVersion != "" {
			return "", "", fmt.Errorf("--library-version requires --library-ref")
		}
		return defaultLibraryName, "", nil
	}
	name, repository, err = ParseLibraryRef(opts.LibraryRef)
	if err != nil {
		return "", "", err
	}
	if opts.LibraryVersion == "" {
		return "", "", fmt.Errorf("--library-version is required with --library-ref")
	}
	return name, repository, nil
}

// libraryChartName is what an OCI repository name element may be.
var libraryChartName = regexp.MustCompile(`^[a-z0-9]+([._-][a-z0-9]+)*$`)

// ParseLibraryRef splits a library chart reference such as
// oci://registry.example.com/platform/platform-lib into the chart name (the
// last path element) and the repository Helm expects in a dependency for OCI
// registries (the reference without the chart name).
func ParseLibraryRef(ref string) (name, repository string, err error) {
	const scheme = "oci://"
	if !strings.HasPrefix(ref, scheme) {
		return "", "", fmt.Errorf("--library-ref %q: only oci:// references are supported (oci://<registry>/<path>/<chart>)", ref)
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(ref, scheme), "/"), "/")
	for _, p := range parts {
		if p == "" {
			return "", "", fmt.Errorf("--library-ref %q: empty path element", ref)
		}
	}
	if len(parts) < 2 {
		return "", "", fmt.Errorf("--library-ref %q: the chart name is missing (oci://<registry>/<path>/<chart>)", ref)
	}
	name = parts[len(parts)-1]
	if strings.ContainsAny(name, ":@") {
		return "", "", fmt.Errorf("--library-ref %q: a tag or digest does not belong in the reference, use --library-version", ref)
	}
	if strings.ToLower(name) != name && libraryChartName.MatchString(strings.ToLower(name)) {
		return "", "", fmt.Errorf("--library-ref %q: the chart name %q must be lowercase (OCI repository names are)", ref, name)
	}
	if !libraryChartName.MatchString(name) {
		return "", "", fmt.Errorf("--library-ref %q: %q is not a valid chart name (lowercase letters, digits, '.', '_' and '-', starting and ending with a letter or digit)", ref, name)
	}
	return name, scheme + strings.Join(parts[:len(parts)-1], "/"), nil
}

// vendorLibrary returns the files of the library chart under charts/<name> of
// an application chart, the way helm dependency build would unpack it.
func vendorLibrary(library *types.GeneratedChart) []types.ExternalFileInfo {
	prefix := "charts/" + library.Name + "/"
	return []types.ExternalFileInfo{
		{Path: prefix + "Chart.yaml", Content: library.ChartYAML},
		{Path: prefix + "values.yaml", Content: library.ValuesYAML},
		{Path: prefix + "templates/_helpers.tpl", Content: library.Helpers},
	}
}
