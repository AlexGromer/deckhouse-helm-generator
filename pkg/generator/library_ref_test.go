package generator

import (
	"context"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestParseLibraryRef(t *testing.T) {
	cases := []struct {
		ref      string
		wantName string
		wantRepo string
		wantErr  string
	}{
		{ref: "oci://registry.example.com/platform/platform-lib", wantName: "platform-lib", wantRepo: "oci://registry.example.com/platform"},
		{ref: "oci://registry.example.com/platform/platform-lib/", wantName: "platform-lib", wantRepo: "oci://registry.example.com/platform"},
		{ref: "oci://registry.example.com:5000/a/b/lib", wantName: "lib", wantRepo: "oci://registry.example.com:5000/a/b"},
		{ref: "oci://registry.example.com/lib", wantName: "lib", wantRepo: "oci://registry.example.com"},
		{ref: "https://charts.example.com/lib", wantErr: "oci://"},
		{ref: "registry.example.com/platform/lib", wantErr: "oci://"},
		{ref: "oci://registry.example.com", wantErr: "chart name"},
		{ref: "oci://registry.example.com/", wantErr: "chart name"},
		{ref: "oci://registry.example.com/platform/lib:1.2.3", wantErr: "--library-version"},
		{ref: "oci://registry.example.com/platform/lib@sha256:abc", wantErr: "--library-version"},
		{ref: "oci://registry.example.com/platform//lib", wantErr: "empty"},
		{ref: "oci://registry.example.com/platform/lib?x=1", wantErr: "chart name"},
		{ref: "oci://registry.example.com/platform/lib#frag", wantErr: "chart name"},
		{ref: "oci://registry.example.com/platform/Lib_X", wantErr: "lowercase"},
		{ref: "oci://registry.example.com/platform/-lib", wantErr: "chart name"},
		{ref: "oci://registry.example.com/platform/my.lib_2-x", wantName: "my.lib_2-x", wantRepo: "oci://registry.example.com/platform"},
	}
	for _, tc := range cases {
		name, repo, err := ParseLibraryRef(tc.ref)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("ParseLibraryRef(%q) error = %v, want one containing %q", tc.ref, err, tc.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseLibraryRef(%q) unexpected error: %v", tc.ref, err)
			continue
		}
		if name != tc.wantName || repo != tc.wantRepo {
			t.Errorf("ParseLibraryRef(%q) = (%q, %q), want (%q, %q)", tc.ref, name, repo, tc.wantName, tc.wantRepo)
		}
	}
}

func libraryRefGraph() *types.ResourceGraph {
	deploy := makeLibraryModeResource("Deployment", "frontend", "frontend", "deployment",
		map[string]interface{}{"replicas": int64(2)})
	return buildGraph([]*types.ProcessedResource{deploy}, nil)
}

type chartDeps struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Dependencies []struct {
		Name       string `json:"name"`
		Version    string `json:"version"`
		Repository string `json:"repository"`
	} `json:"dependencies"`
}

func parseChartDeps(t *testing.T, chartYAML string) chartDeps {
	t.Helper()
	var c chartDeps
	if err := yaml.Unmarshal([]byte(chartYAML), &c); err != nil {
		t.Fatalf("Chart.yaml is not valid YAML: %v\n%s", err, chartYAML)
	}
	return c
}

func TestLibraryGenerator_LibraryRef_OCIDependency(t *testing.T) {
	opts := libraryTestOptions()
	opts.LibraryRef = "oci://registry.example.com/platform/platform-lib"
	opts.LibraryVersion = "1.4.2"

	charts, err := NewLibraryGenerator().Generate(context.Background(), libraryRefGraph(), opts)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	wrapper := findChartByName(charts, "frontend")
	if wrapper == nil {
		t.Fatalf("wrapper chart not found, got %v", chartNamesOf(charts))
	}
	c := parseChartDeps(t, wrapper.ChartYAML)
	if len(c.Dependencies) != 1 {
		t.Fatalf("want one dependency, got %+v\n%s", c.Dependencies, wrapper.ChartYAML)
	}
	dep := c.Dependencies[0]
	if dep.Name != "platform-lib" || dep.Version != "1.4.2" || dep.Repository != "oci://registry.example.com/platform" {
		t.Errorf("dependency = %+v", dep)
	}
	if strings.Contains(wrapper.ChartYAML, "file://") {
		t.Errorf("registry mode must not point at a local library:\n%s", wrapper.ChartYAML)
	}

	// Nothing is vendored.
	for _, f := range wrapper.ExternalFiles {
		if strings.HasPrefix(f.Path, "charts/") {
			t.Errorf("library must not be vendored in registry mode, found %s", f.Path)
		}
	}

	// The top-level library chart is still written, named after the registry
	// chart, so publishing it provides the helpers the service charts include.
	lib := findChartByName(charts, "platform-lib")
	if lib == nil {
		t.Fatalf("library chart platform-lib not written, got %v", chartNamesOf(charts))
	}
	if lc := parseChartDeps(t, lib.ChartYAML); lc.Type != "library" {
		t.Errorf("library chart type = %q", lc.Type)
	}
	if !strings.Contains(lib.Helpers, `define "platform-lib.objname"`) {
		t.Errorf("library helpers must define platform-lib.* names:\n%s", lib.Helpers)
	}
	if findChartByName(charts, "library") != nil {
		t.Error("a chart named library must not be written when the registry chart has another name")
	}

	// Templates reference the library chart name.
	for path, content := range wrapper.Templates {
		if strings.Contains(content, `include "library.`) {
			t.Errorf("%s still includes library.*:\n%s", path, content)
		}
	}
	joined := wrapper.Templates["templates/frontend-deployment.yaml"]
	if !strings.Contains(joined, `include "platform-lib.`) {
		t.Errorf("template does not include platform-lib.*:\n%s", joined)
	}
}

func TestLibraryGenerator_LibraryRef_DefaultStaysVendored(t *testing.T) {
	charts, err := NewLibraryGenerator().Generate(context.Background(), libraryRefGraph(), libraryTestOptions())
	if err != nil {
		t.Fatal(err)
	}
	wrapper := findChartByName(charts, "frontend")
	if !strings.Contains(wrapper.ChartYAML, "repository: file://charts/library") {
		t.Errorf("without --library-ref the library stays vendored:\n%s", wrapper.ChartYAML)
	}
}

// The library is written next to the service charts: a service chart with the
// library's name would overwrite it.
func TestLibraryGenerator_LibraryRef_NameOfAServiceChart(t *testing.T) {
	deploy := makeLibraryModeResource("Deployment", "web", "web", "deployment", map[string]interface{}{"replicas": int64(1)})
	opts := libraryTestOptions()
	opts.LibraryRef = "oci://registry.example.com/platform/web"
	opts.LibraryVersion = "1.0.0"
	_, err := NewLibraryGenerator().Generate(context.Background(), buildGraph([]*types.ProcessedResource{deploy}, nil), opts)
	if err == nil || !strings.Contains(err.Error(), "--library-ref") || !strings.Contains(err.Error(), "web") {
		t.Errorf("error must name --library-ref and the chart, got %v", err)
	}
}

func TestLibraryGenerator_LibraryRef_Errors(t *testing.T) {
	cases := map[string]Options{
		"version missing": {ChartName: "src", ChartVersion: "0.1.0", LibraryRef: "oci://registry.example.com/p/lib"},
		"version alone":   {ChartName: "src", ChartVersion: "0.1.0", LibraryVersion: "1.0.0"},
		"bad ref":         {ChartName: "src", ChartVersion: "0.1.0", LibraryRef: "https://x/lib", LibraryVersion: "1.0.0"},
	}
	for name, opts := range cases {
		if _, err := NewLibraryGenerator().Generate(context.Background(), libraryRefGraph(), opts); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	_, err := NewLibraryGenerator().Generate(context.Background(), libraryRefGraph(),
		Options{ChartName: "src", ChartVersion: "0.1.0", LibraryRef: "oci://registry.example.com/p/lib"})
	if err == nil || !strings.Contains(err.Error(), "--library-version") {
		t.Errorf("error must name --library-version, got %v", err)
	}
}

// Version ranges are valid in dependencies; YAML must keep them strings.
func TestLibraryGenerator_LibraryRef_VersionRangesStayStrings(t *testing.T) {
	for _, v := range []string{"^1.2.0", "~1.2.0", ">=1.0.0 <2.0.0", "1.x", "*", "1.20", "1.2.*"} {
		opts := libraryTestOptions()
		opts.LibraryRef = "oci://registry.example.com/platform/platform-lib"
		opts.LibraryVersion = v
		charts, err := NewLibraryGenerator().Generate(context.Background(), libraryRefGraph(), opts)
		if err != nil {
			t.Fatalf("%q: %v", v, err)
		}
		c := parseChartDeps(t, findChartByName(charts, "frontend").ChartYAML)
		if len(c.Dependencies) != 1 || c.Dependencies[0].Version != v {
			t.Errorf("version %q came back as %+v", v, c.Dependencies)
		}
	}
}

// An exact --library-version is the version of the library chart that is
// written, so publishing it satisfies the dependency; a range cannot be one.
func TestLibraryGenerator_LibraryRef_LibraryChartVersion(t *testing.T) {
	for v, want := range map[string]string{"1.4.2": "1.4.2", "v1.4.2": "1.4.2", "1.5.0-rc.1": "1.5.0-rc.1", "^1.4.0": "0.1.0", ">=1.0.0 <2.0.0": "0.1.0", "1.x": "0.1.0"} {
		opts := libraryTestOptions()
		opts.LibraryRef = "oci://registry.example.com/platform/platform-lib"
		opts.LibraryVersion = v
		charts, err := NewLibraryGenerator().Generate(context.Background(), libraryRefGraph(), opts)
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			Version string `json:"version"`
		}
		if err := yaml.Unmarshal([]byte(findChartByName(charts, "platform-lib").ChartYAML), &got); err != nil {
			t.Fatal(err)
		}
		if got.Version != want {
			t.Errorf("--library-version %q: library chart version = %q, want %q", v, got.Version, want)
		}
	}
}

// With --library-ref the library is published once and shared: two different
// applications must produce the very same library, and the names of each
// application's objects must live in its own charts.
func TestLibraryGenerator_LibraryRef_LibraryIndependentOfInput(t *testing.T) {
	opts := libraryTestOptions()
	opts.LibraryRef = "oci://registry.example.com/platform/platform-lib"
	opts.LibraryVersion = "1.0.0"

	gen := func(resources ...*types.ProcessedResource) []*types.GeneratedChart {
		charts, err := NewLibraryGenerator().Generate(context.Background(), buildGraph(resources, nil), opts)
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		return charts
	}
	app1 := gen(makeLibraryModeResource("Deployment", "api", "api", "deployment", map[string]interface{}{"replicas": int64(1)}))
	app2 := gen(
		makeLibraryModeResource("Deployment", "worker", "worker", "deployment", map[string]interface{}{"replicas": int64(1)}),
		makeLibraryModeResource("ConfigMap", "worker-cfg", "worker", "configmap", map[string]interface{}{"data": map[string]interface{}{"a": "b"}}),
	)

	lib1, lib2 := findLibraryChart(app1), findLibraryChart(app2)
	if lib1 == nil || lib2 == nil {
		t.Fatal("library chart not found")
	}
	if lib1.Helpers != lib2.Helpers || lib1.ChartYAML != lib2.ChartYAML {
		t.Error("the library differs between two applications; it must not depend on the input")
	}
	if strings.Contains(lib1.Helpers, `define "platform-lib.inChart"`) || strings.Contains(lib1.Helpers, "worker-cfg") {
		t.Error("the library carries object names of an application")
	}

	var worker *types.GeneratedChart
	for _, c := range app2 {
		if c.Name == "worker" {
			worker = c
		}
	}
	if worker == nil {
		t.Fatal("worker chart not found")
	}
	in, _ := externalFile(worker, inChartTemplatePath)
	for _, want := range []string{`define "worker.inChart"`, `ConfigMap: ["worker-cfg"]`, `Deployment: ["worker"]`} {
		if !strings.Contains(in, want) {
			t.Errorf("%s lacks %q:\n%s", inChartTemplatePath, want, in)
		}
	}
}
