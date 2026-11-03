package generator

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"sigs.k8s.io/yaml"

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

	// Group first: a file:// library path is relative to a service chart, so
	// the name of the library may come from its Chart.yaml.
	groupResult, err := GroupResources(graph)
	if err != nil {
		return nil, fmt.Errorf("failed to group resources: %w", err)
	}
	groupDir := opts.ChartName
	if len(groupResult.Groups) > 0 {
		groupDir = groupResult.Groups[0].Name
	}
	target, err := resolveLibrary(opts, groupDir)
	if err != nil {
		return nil, err
	}
	libName := target.Name

	charts := make([]*types.GeneratedChart, 0)

	// Always generate the library chart with the shared helpers. With
	// --library-ref it carries the registry chart's name, so publishing it
	// provides the helpers the service charts include.
	// The library does not depend on the input: it is published once and
	// shared. The names of the objects of a chart (global.namePrefix) are
	// written into each service chart instead.
	libChart := g.generateLibraryChart(libName, libraryChartVersion(opts, target.Version), opts)
	charts = append(charts, libChart)

	for _, group := range groupResult.Groups {
		if group.Name == libChart.Name {
			return nil, fmt.Errorf("the chart name %q of --library-ref is also the name of a service chart; the library would overwrite it, name the library chart differently", libChart.Name)
		}
	}
	// The shared chart is a plain chart with helpers of its own: it needs no
	// library. It is installed first.
	plan, err := (&SeparateGenerator{}).planShared(graph, groupResult.Groups, opts, libChart.Name)
	if err != nil {
		return nil, err
	}
	if plan.chart != nil {
		charts = append(charts, plan.chart)
	}
	for _, group := range plan.groups {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		wrapper, err := g.generateWrapperChart(group, libChart, target, opts, plan.inChart)
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
// target.Repository is "" when the library is vendored into the chart,
// otherwise the repository (OCI registry, file:// path or http(s) chart
// repository) the library is pulled from, at target.Version.
func (g *LibraryGenerator) generateWrapperChart(group *ServiceGroup, library *types.GeneratedChart, target libraryDependency, opts Options, inChart map[string][]string) (*types.GeneratedChart, error) {
	repository := target.Repository
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
		dep.Version = target.Version
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
// exact version of the dependency (so the published chart satisfies it),
// otherwise the chart version.
func libraryChartVersion(opts Options, dependencyVersion string) string {
	if opts.LibraryRef != "" && exactVersion.MatchString(dependencyVersion) {
		return strings.TrimPrefix(dependencyVersion, "v")
	}
	return opts.ChartVersion
}

// defaultLibraryName is the name of the library chart that is vendored into
// the service charts.
const defaultLibraryName = "library"

// libraryDependency is the library as the service charts depend on it.
type libraryDependency struct {
	// Name is the name of the library chart.
	Name string
	// Repository is "" when the library is vendored into the service charts.
	Repository string
	// Version is the version or range of the dependency.
	Version string
}

// resolveLibrary decides which library the service charts use. groupDir is the
// name of the service chart directory a relative file:// path is resolved
// from (the library is looked up there, as Helm will).
func resolveLibrary(opts Options, groupDir string) (libraryDependency, error) {
	if err := ValidateLibraryOptions(opts.LibraryRef, opts.LibraryName, opts.LibraryVersion); err != nil {
		return libraryDependency{}, err
	}
	if opts.LibraryRef == "" {
		return libraryDependency{Name: defaultLibraryName}, nil
	}
	src, err := ParseLibrarySource(opts.LibraryRef, opts.LibraryName)
	if err != nil {
		return libraryDependency{}, err
	}
	dep := libraryDependency{Name: src.Name, Repository: src.Repository, Version: opts.LibraryVersion}
	if src.Scheme != "file" {
		return dep, nil
	}

	// file://: the library may be there already; its Chart.yaml is the truth
	// Helm checks the dependency against.
	dir := src.Path
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(opts.OutputDir, groupDir, dir)
	}
	meta, found, err := readChartMeta(dir)
	if err != nil {
		return libraryDependency{}, fmt.Errorf("--library-ref %q: %w", opts.LibraryRef, err)
	}
	if found {
		if opts.LibraryName != "" && opts.LibraryName != meta.Name {
			return libraryDependency{}, fmt.Errorf("--library-ref %q: the chart at that path is named %q, --library-name says %q", opts.LibraryRef, meta.Name, opts.LibraryName)
		}
		if !libraryChartName.MatchString(meta.Name) {
			return libraryDependency{}, fmt.Errorf("--library-ref %q: %q (name in %s) is not a valid chart name (lowercase letters, digits, '.', '_' and '-')", opts.LibraryRef, meta.Name, filepath.Join(dir, "Chart.yaml"))
		}
		dep.Name = meta.Name
		if dep.Version == "" {
			dep.Version = meta.Version
		}
	}
	if dep.Name == "" {
		return libraryDependency{}, fmt.Errorf("--library-ref %q: the chart name cannot be taken from the path, there is no Chart.yaml there yet; name it with --library-name", opts.LibraryRef)
	}
	if !found {
		// Nothing there yet: the library this run writes is the one the
		// dependency must find, and it goes to <out>/<name>.
		written, werr := filepath.Abs(filepath.Join(opts.OutputDir, dep.Name))
		want, derr := filepath.Abs(dir)
		if werr != nil || derr != nil || written != want {
			return libraryDependency{}, fmt.Errorf("--library-ref %q: no Chart.yaml at %s, and the library this run writes goes to %s (next to the service charts), where helm would not find it; use file://../%s, or create the library chart at that path first", opts.LibraryRef, dir, written, dep.Name)
		}
	}
	if dep.Version == "" {
		dep.Version = opts.ChartVersion
	}
	return dep, nil
}

// readChartMeta reads name and version from <dir>/Chart.yaml; found is false
// when there is no such file.
func readChartMeta(dir string) (meta struct{ Name, Version string }, found bool, err error) {
	file := filepath.Join(dir, "Chart.yaml")
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return meta, false, nil
	}
	if err != nil {
		return meta, false, fmt.Errorf("reading %s: %w", file, err)
	}
	var doc struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return meta, false, fmt.Errorf("%s is not a valid Chart.yaml: %w", file, err)
	}
	if doc.Name == "" {
		return meta, false, fmt.Errorf("%s has no name", file)
	}
	meta.Name, meta.Version = doc.Name, doc.Version
	return meta, true, nil
}

// ValidateLibraryOptions checks --library-ref, --library-name and
// --library-version against each other, without looking at the file system.
func ValidateLibraryOptions(ref, name, version string) error {
	if ref == "" {
		if version != "" {
			return fmt.Errorf("--library-version requires --library-ref")
		}
		if name != "" {
			return fmt.Errorf("--library-name requires --library-ref")
		}
		return nil
	}
	src, err := ParseLibrarySource(ref, name)
	if err != nil {
		return err
	}
	if version == "" && src.Scheme != "file" {
		return fmt.Errorf("--library-version is required with --library-ref %s", ref)
	}
	return nil
}

// LibrarySource is a parsed --library-ref.
type LibrarySource struct {
	// Scheme is oci, file, http or https.
	Scheme string
	// Name is the library chart name; empty for a file:// path whose name has
	// to come from the Chart.yaml there.
	Name string
	// Repository is what is written to the dependency's repository field.
	Repository string
	// Path is the directory of a file:// reference, as given.
	Path string
}

// ParseLibrarySource parses a library chart reference:
//
//   - oci://<registry>/<path>/<chart>: the chart is the last element;
//   - file://<path>: a chart directory, relative to the service chart or
//     absolute; the chart is the last element, unless the directory has a
//     Chart.yaml (see resolveLibrary) or name is given;
//   - https://<repository> and http://<repository>: a chart repository, which
//     does not say which chart to take: name is required.
func ParseLibrarySource(ref, name string) (LibrarySource, error) {
	const fileScheme = "file://"
	switch {
	case strings.HasPrefix(ref, "oci://"):
		chart, repository, err := ParseLibraryRef(ref)
		if err != nil {
			return LibrarySource{}, err
		}
		if name != "" && name != chart {
			return LibrarySource{}, fmt.Errorf("--library-name %q: an oci:// reference names its chart (%q); --library-name applies to file:// and http(s):// references", name, chart)
		}
		return LibrarySource{Scheme: "oci", Name: chart, Repository: repository}, nil

	case strings.HasPrefix(ref, fileScheme):
		dir := strings.TrimPrefix(ref, fileScheme)
		if dir == "" {
			return LibrarySource{}, fmt.Errorf("--library-ref %q: the path is missing (file://<path to the library chart>)", ref)
		}
		chart := name
		if chart == "" {
			if last := path.Base(path.Clean(dir)); libraryChartName.MatchString(last) {
				chart = last
			}
		} else if !libraryChartName.MatchString(chart) {
			return LibrarySource{}, fmt.Errorf("--library-name %q is not a valid chart name (lowercase letters, digits, '.', '_' and '-', starting and ending with a letter or digit)", chart)
		}
		return LibrarySource{Scheme: "file", Name: chart, Repository: ref, Path: dir}, nil

	case strings.HasPrefix(ref, "https://"), strings.HasPrefix(ref, "http://"):
		u, err := url.Parse(ref)
		if err != nil {
			return LibrarySource{}, fmt.Errorf("--library-ref %q: %w", ref, err)
		}
		switch {
		case u.Host == "":
			return LibrarySource{}, fmt.Errorf("--library-ref %q: the host is missing", ref)
		case u.RawQuery != "" || strings.Contains(ref, "?"):
			return LibrarySource{}, fmt.Errorf("--library-ref %q: a query does not belong in a chart repository URL", ref)
		case u.Fragment != "" || strings.Contains(ref, "#"):
			return LibrarySource{}, fmt.Errorf("--library-ref %q: a fragment does not belong in a chart repository URL", ref)
		}
		if name == "" {
			return LibrarySource{}, fmt.Errorf("--library-name is required with the %s:// repository --library-ref %q: a repository does not say which of its charts is the library", u.Scheme, ref)
		}
		if !libraryChartName.MatchString(name) {
			return LibrarySource{}, fmt.Errorf("--library-name %q is not a valid chart name (lowercase letters, digits, '.', '_' and '-', starting and ending with a letter or digit)", name)
		}
		return LibrarySource{Scheme: u.Scheme, Name: name, Repository: ref}, nil
	}
	return LibrarySource{}, fmt.Errorf("--library-ref %q: supported are oci://<registry>/<path>/<chart>, file://<path>, https://<repository> and http://<repository>", ref)
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
