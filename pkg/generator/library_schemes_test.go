package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLibrarySource(t *testing.T) {
	cases := []struct {
		ref, name string
		want      LibrarySource
		wantErr   string
	}{
		{ref: "oci://registry.example.com/platform/platform-lib", want: LibrarySource{Scheme: "oci", Name: "platform-lib", Repository: "oci://registry.example.com/platform"}},
		{ref: "file://../library", want: LibrarySource{Scheme: "file", Name: "library", Repository: "file://../library", Path: "../library"}},
		{ref: "file://../library/", want: LibrarySource{Scheme: "file", Name: "library", Repository: "file://../library/", Path: "../library/"}},
		{ref: "file:///srv/charts/common-lib", want: LibrarySource{Scheme: "file", Name: "common-lib", Repository: "file:///srv/charts/common-lib", Path: "/srv/charts/common-lib"}},
		{ref: "file://libs/common", want: LibrarySource{Scheme: "file", Name: "common", Repository: "file://libs/common", Path: "libs/common"}},
		// The name of a path without a usable last element comes from Chart.yaml
		// or --library-name.
		{ref: "file://..", want: LibrarySource{Scheme: "file", Repository: "file://..", Path: ".."}},
		{ref: "file://../Common_Lib", want: LibrarySource{Scheme: "file", Repository: "file://../Common_Lib", Path: "../Common_Lib"}},
		{ref: "file://../x", name: "common", want: LibrarySource{Scheme: "file", Name: "common", Repository: "file://../x", Path: "../x"}},
		{ref: "https://charts.example.com/stable", name: "platform-lib", want: LibrarySource{Scheme: "https", Name: "platform-lib", Repository: "https://charts.example.com/stable"}},
		{ref: "http://charts.example.com:8080/", name: "platform-lib", want: LibrarySource{Scheme: "http", Name: "platform-lib", Repository: "http://charts.example.com:8080/"}},
		{ref: "file://", wantErr: "path is missing"},
		{ref: "https://charts.example.com/stable", wantErr: "--library-name"},
		{ref: "https://", name: "x", wantErr: "host"},
		{ref: "https://charts.example.com/stable?x=1", name: "lib", wantErr: "query"},
		{ref: "https://charts.example.com/stable#frag", name: "lib", wantErr: "fragment"},
		{ref: "https://charts.example.com/stable", name: "Bad_Name", wantErr: "not a valid chart name"},
		{ref: "oci://registry.example.com/platform/lib", name: "other", wantErr: "--library-name"},
		{ref: "ftp://example.com/lib", wantErr: "oci://"},
		{ref: "registry.example.com/lib", wantErr: "file://"},
	}
	for _, tc := range cases {
		got, err := ParseLibrarySource(tc.ref, tc.name)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("ParseLibrarySource(%q, %q) error = %v, want one containing %q", tc.ref, tc.name, err, tc.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseLibrarySource(%q, %q) unexpected error: %v", tc.ref, tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseLibrarySource(%q, %q) = %+v, want %+v", tc.ref, tc.name, got, tc.want)
		}
	}
}

func TestValidateLibraryOptions(t *testing.T) {
	cases := []struct {
		name               string
		ref, libName, vers string
		wantErr            string
	}{
		{name: "none"},
		{name: "oci", ref: "oci://r.example.com/p/lib", vers: "1.0.0"},
		{name: "oci without version", ref: "oci://r.example.com/p/lib", wantErr: "--library-version is required"},
		{name: "file without version", ref: "file://../library"},
		{name: "file with version", ref: "file://../library", vers: "^1.0.0"},
		{name: "https", ref: "https://c.example.com/s", libName: "lib", vers: "1.0.0"},
		{name: "https without version", ref: "https://c.example.com/s", libName: "lib", wantErr: "--library-version is required"},
		{name: "https without name", ref: "https://c.example.com/s", vers: "1.0.0", wantErr: "--library-name"},
		{name: "version alone", vers: "1.0.0", wantErr: "--library-version requires --library-ref"},
		{name: "name alone", libName: "lib", wantErr: "--library-name requires --library-ref"},
	}
	for _, tc := range cases {
		err := ValidateLibraryOptions(tc.ref, tc.libName, tc.vers)
		if tc.wantErr == "" && err != nil {
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		}
		if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
			t.Errorf("%s: error = %v, want one containing %q", tc.name, err, tc.wantErr)
		}
	}
}

func generateLibraryRefCharts(t *testing.T, opts Options) map[string]string {
	t.Helper()
	charts, err := NewLibraryGenerator().Generate(context.Background(), libraryRefGraph(), opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	out := map[string]string{}
	for _, c := range charts {
		out[c.Name] = c.ChartYAML
	}
	return out
}

func TestLibraryGenerator_FileRef_NewLibrary(t *testing.T) {
	opts := libraryTestOptions()
	opts.OutputDir = t.TempDir()
	opts.LibraryRef = "file://../library"

	charts, err := NewLibraryGenerator().Generate(context.Background(), libraryRefGraph(), opts)
	if err != nil {
		t.Fatal(err)
	}
	wrapper := findChartByName(charts, "frontend")
	if wrapper == nil {
		t.Fatalf("charts = %v", chartNamesOf(charts))
	}
	c := parseChartDeps(t, wrapper.ChartYAML)
	if len(c.Dependencies) != 1 {
		t.Fatalf("dependencies = %+v\n%s", c.Dependencies, wrapper.ChartYAML)
	}
	// The path does not exist yet: the name is its last element, the version
	// that of the library chart this run writes.
	if d := c.Dependencies[0]; d.Name != "library" || d.Version != "0.1.0" || d.Repository != "file://../library" {
		t.Errorf("dependency = %+v", d)
	}
	for _, f := range wrapper.ExternalFiles {
		if strings.HasPrefix(f.Path, "charts/") {
			t.Errorf("a file:// library must not be vendored, found %s", f.Path)
		}
	}
	lib := findChartByName(charts, "library")
	if lib == nil || !strings.Contains(lib.ChartYAML, "type: library") || !strings.Contains(lib.ChartYAML, "version: 0.1.0") {
		t.Errorf("library chart = %v", lib)
	}
}

// A library that exists already says its own name and version.
func TestLibraryGenerator_FileRef_ReadsChartYAML(t *testing.T) {
	out := t.TempDir()
	if err := os.MkdirAll(filepath.Join(out, "shared-lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "shared-lib", "Chart.yaml"),
		[]byte("apiVersion: v2\nname: common-lib\nversion: 2.3.1\ntype: library\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"file://../shared-lib", "file://" + filepath.Join(out, "shared-lib")} {
		opts := libraryTestOptions()
		opts.OutputDir = out
		opts.LibraryRef = ref
		charts, err := NewLibraryGenerator().Generate(context.Background(), libraryRefGraph(), opts)
		if err != nil {
			t.Fatalf("%s: %v", ref, err)
		}
		c := parseChartDeps(t, findChartByName(charts, "frontend").ChartYAML)
		if len(c.Dependencies) != 1 {
			t.Fatalf("%s: dependencies = %+v", ref, c.Dependencies)
		}
		if d := c.Dependencies[0]; d.Name != "common-lib" || d.Version != "2.3.1" || d.Repository != ref {
			t.Errorf("%s: dependency = %+v", ref, d)
		}
		lib := findChartByName(charts, "common-lib")
		if lib == nil || !strings.Contains(lib.ChartYAML, "version: 2.3.1") {
			t.Errorf("%s: library chart = %v", ref, lib)
		}
		tmpl := findChartByName(charts, "frontend").Templates["templates/frontend-deployment.yaml"]
		if !strings.Contains(tmpl, `include "common-lib.`) {
			t.Errorf("%s: template does not include common-lib.*:\n%s", ref, tmpl)
		}
	}
}

func TestLibraryGenerator_FileRef_ExplicitVersion(t *testing.T) {
	opts := libraryTestOptions()
	opts.OutputDir = t.TempDir()
	opts.LibraryRef = "file://../library"
	opts.LibraryVersion = "^1.2.0"
	charts, err := NewLibraryGenerator().Generate(context.Background(), libraryRefGraph(), opts)
	if err != nil {
		t.Fatal(err)
	}
	c := parseChartDeps(t, findChartByName(charts, "frontend").ChartYAML)
	if c.Dependencies[0].Version != "^1.2.0" {
		t.Errorf("dependency = %+v", c.Dependencies[0])
	}
	opts.LibraryVersion = "1.2.0"
	charts, err = NewLibraryGenerator().Generate(context.Background(), libraryRefGraph(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if lib := findChartByName(charts, "library"); !strings.Contains(lib.ChartYAML, "version: 1.2.0") {
		t.Errorf("an exact --library-version is the version of the library chart:\n%s", lib.ChartYAML)
	}
}

func TestLibraryGenerator_FileRef_LibraryName(t *testing.T) {
	opts := libraryTestOptions()
	opts.OutputDir = t.TempDir()
	opts.LibraryRef = "file://../common"
	opts.LibraryName = "common"
	charts := generateLibraryRefCharts(t, opts)
	if charts["common"] == "" {
		t.Fatalf("library chart common not written: %v", charts)
	}

	// A library that is there under another name is a mistake Helm reports
	// later and less clearly.
	opts.LibraryRef = "file://../x"
	if err := os.MkdirAll(filepath.Join(opts.OutputDir, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(opts.OutputDir, "x", "Chart.yaml"), []byte("apiVersion: v2\nname: other\nversion: 1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NewLibraryGenerator().Generate(context.Background(), libraryRefGraph(), opts)
	if err == nil || !strings.Contains(err.Error(), "other") || !strings.Contains(err.Error(), "--library-name") {
		t.Errorf("error = %v", err)
	}
}

func TestLibraryGenerator_FileRef_BrokenChartYAML(t *testing.T) {
	out := t.TempDir()
	if err := os.MkdirAll(filepath.Join(out, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "lib", "Chart.yaml"), []byte("name: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := libraryTestOptions()
	opts.OutputDir = out
	opts.LibraryRef = "file://../lib"
	if _, err := NewLibraryGenerator().Generate(context.Background(), libraryRefGraph(), opts); err == nil || !strings.Contains(err.Error(), "Chart.yaml") {
		t.Errorf("error = %v, want one naming Chart.yaml", err)
	}
}

func TestLibraryGenerator_FileRef_NameOfAServiceChart(t *testing.T) {
	opts := libraryTestOptions()
	opts.OutputDir = t.TempDir()
	opts.LibraryRef = "file://../frontend"
	if _, err := NewLibraryGenerator().Generate(context.Background(), libraryRefGraph(), opts); err == nil || !strings.Contains(err.Error(), "frontend") {
		t.Errorf("the library would overwrite the service chart, error = %v", err)
	}
}

func TestLibraryGenerator_HTTPSRepository(t *testing.T) {
	for _, repo := range []string{"https://charts.example.com/stable", "http://charts.example.com:8080/"} {
		opts := libraryTestOptions()
		opts.LibraryRef = repo
		opts.LibraryName = "platform-lib"
		opts.LibraryVersion = "~1.4.0"
		charts, err := NewLibraryGenerator().Generate(context.Background(), libraryRefGraph(), opts)
		if err != nil {
			t.Fatalf("%s: %v", repo, err)
		}
		wrapper := findChartByName(charts, "frontend")
		c := parseChartDeps(t, wrapper.ChartYAML)
		if len(c.Dependencies) != 1 {
			t.Fatalf("%s: dependencies = %+v", repo, c.Dependencies)
		}
		if d := c.Dependencies[0]; d.Name != "platform-lib" || d.Version != "~1.4.0" || d.Repository != repo {
			t.Errorf("%s: dependency = %+v", repo, d)
		}
		for _, f := range wrapper.ExternalFiles {
			if strings.HasPrefix(f.Path, "charts/") {
				t.Errorf("%s: library vendored: %s", repo, f.Path)
			}
		}
		if findChartByName(charts, "platform-lib") == nil {
			t.Errorf("%s: library chart platform-lib not written: %v", repo, chartNamesOf(charts))
		}
	}
}

func TestLibraryGenerator_SchemeErrors(t *testing.T) {
	cases := map[string]Options{
		"https without name":    {LibraryRef: "https://c.example.com/s", LibraryVersion: "1.0.0"},
		"https without version": {LibraryRef: "https://c.example.com/s", LibraryName: "lib"},
		"name without ref":      {LibraryName: "lib"},
		"name with oci":         {LibraryRef: "oci://r.example.com/p/lib", LibraryName: "other", LibraryVersion: "1.0.0"},
		"unknown scheme":        {LibraryRef: "ftp://r.example.com/lib", LibraryVersion: "1.0.0"},
	}
	for name, opts := range cases {
		opts.ChartName, opts.ChartVersion = "src", "0.1.0"
		if _, err := NewLibraryGenerator().Generate(context.Background(), libraryRefGraph(), opts); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

// A file:// path with no Chart.yaml must be the place this run writes the
// library to (<out>/<name>); anywhere else helm cannot build the dependency.
func TestLibraryGenerator_FileRef_NewLibraryElsewhere(t *testing.T) {
	out := t.TempDir()
	for _, ref := range []string{"file://../../extlib", "file://../sub/extlib", "file://" + filepath.Join(t.TempDir(), "extlib")} {
		opts := libraryTestOptions()
		opts.OutputDir = out
		opts.LibraryRef = ref
		_, err := NewLibraryGenerator().Generate(context.Background(), libraryRefGraph(), opts)
		if err == nil {
			t.Errorf("%s: want an error: the library this run writes would not be where the ref points", ref)
			continue
		}
		name := filepath.Base(strings.TrimPrefix(ref, "file://"))
		for _, want := range []string{"no Chart.yaml", "file://../" + name} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: error %q lacks %q", ref, err, want)
			}
		}
	}
	// The place the run writes to is fine, relative or absolute.
	for _, ref := range []string{"file://../library", "file://" + filepath.Join(out, "library")} {
		opts := libraryTestOptions()
		opts.OutputDir = out
		opts.LibraryRef = ref
		if _, err := NewLibraryGenerator().Generate(context.Background(), libraryRefGraph(), opts); err != nil {
			t.Errorf("%s: %v", ref, err)
		}
	}
	// A library that exists elsewhere is used as it is.
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "Chart.yaml"), []byte("apiVersion: v2\nname: extlib\nversion: 1.2.3\ntype: library\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := libraryTestOptions()
	opts.OutputDir = out
	opts.LibraryRef = "file://" + other
	if _, err := NewLibraryGenerator().Generate(context.Background(), libraryRefGraph(), opts); err != nil {
		t.Errorf("an existing library elsewhere: %v", err)
	}
}
