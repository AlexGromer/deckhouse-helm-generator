// Package golden runs the real dhg binary over every example and fixture in
// the repository, in every output mode, and checks the produced charts with
// the real Helm CLI (helm lint + helm template).
//
// Unit tests assert on strings; this suite asserts that the output is a chart
// Helm actually accepts. It is skipped when helm is not installed, unless
// DHG_REQUIRE_HELM=1 is set (CI sets it so the suite can never skip silently).
package golden

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

var dhgBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "dhg-golden-")
	if err != nil {
		panic(err)
	}
	dhgBin = filepath.Join(dir, "dhg")
	build := exec.Command("go", "build", "-o", dhgBin, "./cmd/dhg")
	build.Dir = repoRoot()
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		panic("failed to build dhg: " + err.Error())
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func requireHelm(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("helm")
	if err != nil {
		if os.Getenv("DHG_REQUIRE_HELM") == "1" {
			t.Fatalf("helm is required (DHG_REQUIRE_HELM=1) but not found: %v", err)
		}
		t.Skip("helm not installed; set DHG_REQUIRE_HELM=1 to make this a failure")
	}
	return path
}

// inputs returns every manifest directory shipped with the repository.
func inputs(t *testing.T) map[string]string {
	t.Helper()
	root := repoRoot()
	result := map[string]string{}

	examples, err := filepath.Glob(filepath.Join(root, "examples", "*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range examples {
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			continue
		}
		in := dir
		if fi, err := os.Stat(filepath.Join(dir, "input")); err == nil && fi.IsDir() {
			in = filepath.Join(dir, "input")
		}
		result["examples/"+filepath.Base(dir)] = in
	}

	fixtures, err := filepath.Glob(filepath.Join(root, "tests", "integration", "fixtures", "*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range fixtures {
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			result["fixtures/"+filepath.Base(dir)] = dir
		}
	}
	return result
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// scenario is one dhg invocation whose output must be accepted by Helm.
type scenario struct {
	name string
	args []string
}

var scenarios = []scenario{
	{"universal", []string{"--mode", "universal"}},
	{"universal-full", []string{"--mode", "universal", "--include-schema", "--include-tests", "--hooks", "--values-flat"}},
	{"separate", []string{"--mode", "separate", "--include-schema", "--include-tests"}},
	{"library", []string{"--mode", "library", "--include-schema", "--include-tests"}},
	{"umbrella", []string{"--mode", "umbrella", "--include-schema", "--include-tests"}},
	{"deckhouse-module", []string{"--mode", "universal", "--deckhouse-module"}},
}

func TestGeneratedChartsPassHelm(t *testing.T) {
	helm := requireHelm(t)
	in := inputs(t)

	for _, name := range sortedKeys(in) {
		input := in[name]
		for _, sc := range scenarios {
			t.Run(name+"/"+sc.name, func(t *testing.T) {
				t.Parallel()
				out := t.TempDir()
				args := append([]string{"generate", "-f", input, "-o", out, "--chart-name", "app"}, sc.args...)
				runOK(t, dhgBin, args...)
				rendered, complete := checkCharts(t, helm, out)
				if want := countInputObjects(t, input); complete && rendered < want {
					t.Errorf("rendered %d objects, want at least %d (one per input manifest)", rendered, want)
				}
			})
		}
	}
}

// checkCharts lints and renders every chart under dir and returns the number
// of rendered objects; complete is false when some chart could not be
// rendered hermetically. Subcharts below a charts/ directory are validated
// through their parent.
func checkCharts(t *testing.T, helm, dir string) (rendered int, complete bool) {
	t.Helper()
	complete = true
	charts := findCharts(t, dir)
	if len(charts) == 0 {
		t.Fatalf("no Chart.yaml produced under %s", dir)
	}
	for _, chart := range charts {
		chartYAML, err := os.ReadFile(filepath.Join(chart, "Chart.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(dir, chart)
		if hasExternalDeps(chartYAML) {
			// Remote dependencies (e.g. Deckhouse lib-helm) cannot be fetched
			// in a hermetic test; lint without rendering.
			if out, err := run(helm, "lint", chart); err != nil && !strings.Contains(out, "missing in charts/") {
				t.Errorf("helm lint %s failed:\n%s", rel, out)
			}
			complete = false
			continue
		}
		if bytes.Contains(chartYAML, []byte("file://")) {
			// Local dependencies (library/umbrella modes) are vendored the
			// same way a user would: helm dependency build.
			if out, err := run(helm, "dependency", "build", chart); err != nil {
				t.Errorf("helm dependency build %s failed:\n%s", rel, out)
				continue
			}
		}
		if out, err := run(helm, "lint", "--strict", chart); err != nil {
			t.Errorf("helm lint %s failed:\n%s", rel, out)
		}
		if bytes.Contains(chartYAML, []byte("type: library")) {
			continue // library charts are not installable and cannot be templated
		}
		out, err := run(helm, "template", "golden", chart)
		if err != nil {
			t.Errorf("helm template %s failed:\n%s", rel, out)
			continue
		}
		n := countObjects(out)
		if n == 0 {
			t.Errorf("helm template %s rendered no objects", rel)
		}
		rendered += n
	}
	return rendered, complete
}

// countObjects counts top-level Kubernetes objects in a YAML stream.
func countObjects(yamlStream string) int {
	n := 0
	for _, line := range strings.Split(yamlStream, "\n") {
		if strings.HasPrefix(line, "kind: ") {
			n++
		}
	}
	return n
}

// countInputObjects counts the manifests dhg is given as input.
func countInputObjects(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if ext := filepath.Ext(path); ext != ".yaml" && ext != ".yml" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		n += countObjects(string(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// hasExternalDeps reports whether Chart.yaml declares a dependency fetched
// from a remote repository (as opposed to file:// or a bundled subchart).
func hasExternalDeps(chartYAML []byte) bool {
	for _, line := range strings.Split(string(chartYAML), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "repository:") && strings.Contains(line, "://") && !strings.Contains(line, "file://") {
			return true
		}
	}
	return false
}

func findCharts(t *testing.T, dir string) []string {
	t.Helper()
	var charts []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && info.Name() == "charts" {
			return filepath.SkipDir
		}
		if !info.IsDir() && info.Name() == "Chart.yaml" {
			charts = append(charts, filepath.Dir(path))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(charts)
	return charts
}

func run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	return buf.String(), err
}

func runOK(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := run(name, args...)
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return out
}
