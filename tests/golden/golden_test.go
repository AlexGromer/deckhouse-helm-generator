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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"sigs.k8s.io/yaml"
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
	// Post-processing flags of `dhg generate`.
	{"env-values", []string{"--env-values", "--include-schema"}},
	{"airgap", []string{"--airgap-registry", "registry.example.com"}},
	{"namespace-resources", []string{"--namespace-resources"}},
	{"multi-tenant", []string{"--multi-tenant"}},
	{"feature-flags", []string{"--feature-flags"}},
	{"cloud-aws", []string{"--cloud-provider", "aws"}},
	{"detect-ingress", []string{"--detect-ingress"}},
	{"spot", []string{"--spot", "--cloud-provider", "gcp"}},
	{"auto-deps", []string{"--auto-deps"}},
	{"kustomize", []string{"--kustomize"}},
	{"monorepo", []string{"--monorepo"}},
	{"post-renderer", []string{"--post-renderer"}},
	{"separate-post", []string{"--mode", "separate", "--env-values", "--namespace-resources", "--feature-flags", "--spot"}},
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
				if complete {
					checkReleaseIntegrity(t, helm, input, out)
				}
			})
		}
	}
}

// checkReleaseIntegrity renders every top-level chart and checks references
// and selectors against the input manifests.
func checkReleaseIntegrity(t *testing.T, helm, input, out string) {
	t.Helper()
	var rendered []object
	for _, chart := range findCharts(t, out) {
		data, err := os.ReadFile(filepath.Join(chart, "Chart.yaml"))
		if err != nil || bytes.Contains(data, []byte("type: library")) {
			continue
		}
		stream, err := run(helm, "template", "golden", chart)
		if err != nil {
			return // reported by checkCharts
		}
		rendered = append(rendered, parseStream(stream)...)
	}
	var inputs []object
	_ = filepath.Walk(input, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && (filepath.Ext(path) == ".yaml" || filepath.Ext(path) == ".yml") {
			data, _ := os.ReadFile(path)
			inputs = append(inputs, parseStream(string(data))...)
		}
		return nil
	})
	for _, p := range checkIntegrity(inputs, rendered) {
		t.Errorf("integrity: %s", p)
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
		for _, problem := range checkRendered(out) {
			t.Errorf("helm template %s: %s", rel, problem)
		}

		// A generated post-renderer must run end to end when a kustomize
		// implementation is available.
		if script := filepath.Join(chart, "post-renderer", "kustomize.sh"); fileExists(script) && hasKustomize() {
			for _, env := range []string{"dev", "staging", "prod"} {
				out, err := run(helm, "template", "golden", chart, "--post-renderer", script, "--post-renderer-args", env)
				if err != nil {
					t.Errorf("helm template %s with post-renderer %s failed:\n%s", rel, env, out)
				} else if countObjects(out) != n {
					t.Errorf("post-renderer %s changed the object count: %d, want %d", env, countObjects(out), n)
				}
			}
		}

		// Generated helm-unittest suites must pass when the plugin is installed.
		if fileExists(filepath.Join(chart, "tests")) && hasUnittestPlugin(helm) {
			if out, err := run(helm, "unittest", chart); err != nil {
				t.Errorf("helm unittest %s failed:\n%s", rel, out)
			}
		}

		// Every values overlay shipped with the chart must render too.
		overlays, _ := filepath.Glob(filepath.Join(chart, "values-*.yaml"))
		for _, overlay := range overlays {
			if out, err := run(helm, "template", "golden", chart, "-f", overlay); err != nil {
				t.Errorf("helm template %s -f %s failed:\n%s", rel, filepath.Base(overlay), out)
			}
		}
	}
	return rendered, complete
}

// checkRendered applies checks the Kubernetes API server would enforce but
// `helm template` does not: every document is strict YAML (no duplicate keys,
// which parsers resolve differently), and workload selectors match their pod
// template labels.
func checkRendered(stream string) []string {
	var problems []string
	for _, doc := range strings.Split(stream, "\n---") {
		source := ""
		if i := strings.Index(doc, "# Source: "); i >= 0 {
			source = strings.SplitN(doc[i+len("# Source: "):], "\n", 2)[0]
		}
		var obj map[string]interface{}
		if err := yaml.Unmarshal([]byte(doc), &obj); err != nil {
			problems = append(problems, fmt.Sprintf("%s: invalid YAML: %v", source, err))
			continue
		}
		if obj == nil {
			continue
		}
		if msg := selectorMismatch(obj); msg != "" {
			problems = append(problems, fmt.Sprintf("%s: %s", source, msg))
		}
	}
	return problems
}

// selectorMismatch reports a workload whose spec.selector.matchLabels are not
// all present in its pod template labels (rejected by the API server).
func selectorMismatch(obj map[string]interface{}) string {
	switch obj["kind"] {
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet":
	default:
		return ""
	}
	selector := nested(obj, "spec", "selector", "matchLabels")
	labels := nested(obj, "spec", "template", "metadata", "labels")
	for k, v := range selector {
		if fmt.Sprint(labels[k]) != fmt.Sprint(v) {
			return fmt.Sprintf("selector %s=%v does not match pod template label %s=%v", k, v, k, labels[k])
		}
	}
	return ""
}

func nested(obj map[string]interface{}, path ...string) map[string]interface{} {
	cur := obj
	for _, p := range path {
		next, ok := cur[p].(map[string]interface{})
		if !ok {
			return nil
		}
		cur = next
	}
	return cur
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

var unittestPlugin struct {
	once sync.Once
	ok   bool
}

func hasUnittestPlugin(helm string) bool {
	unittestPlugin.once.Do(func() {
		out, err := run(helm, "plugin", "list")
		unittestPlugin.ok = err == nil && strings.Contains(out, "unittest")
	})
	return unittestPlugin.ok
}

func hasKustomize() bool {
	for _, bin := range []string{"kustomize", "kubectl"} {
		if _, err := exec.LookPath(bin); err == nil {
			return true
		}
	}
	return false
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

// featureInputs are representative inputs every feature is exercised on:
// a web app, a stateful database, batch jobs, RBAC, monitoring CRDs.
var featureInputs = []string{
	"examples/01-simple-web",
	"examples/02-statefulset-db",
	"examples/03-batch-processing",
	"examples/04-rbac-setup",
	"examples/05-full-stack",
	"examples/11-monitoring-stack",
	"fixtures/full-stack",
}

func featureNames(t *testing.T) []string {
	t.Helper()
	out := runOK(t, dhgBin, "features", "--names")
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	return names
}

// TestFeaturesPassHelm enables every registered feature (with its default
// parameters) on representative inputs, alone and all together.
func TestFeaturesPassHelm(t *testing.T) {
	helm := requireHelm(t)
	in := inputs(t)
	names := featureNames(t)
	if len(names) == 0 {
		t.Skip("no features registered")
	}

	run := func(t *testing.T, input string, args ...string) {
		out := t.TempDir()
		args = append([]string{"generate", "-f", input, "-o", out, "--chart-name", "app"}, args...)
		runOK(t, dhgBin, args...)
		rendered, complete := checkCharts(t, helm, out)
		if want := countInputObjects(t, input); complete && rendered < want {
			t.Errorf("rendered %d objects, want at least %d (one per input manifest)", rendered, want)
		}
	}

	for _, name := range names {
		for _, key := range featureInputs {
			input, ok := in[key]
			if !ok {
				t.Fatalf("feature input %s not found", key)
			}
			t.Run(name+"/"+key, func(t *testing.T) {
				t.Parallel()
				run(t, input, "--with", name)
			})
		}
	}

	all := strings.Join(names, ",")
	for _, key := range sortedKeys(in) {
		input := in[key]
		for _, mode := range []string{"universal", "separate", "umbrella"} {
			t.Run("all/"+mode+"/"+key, func(t *testing.T) {
				t.Parallel()
				run(t, input, "--mode", mode, "--with", all)
			})
		}
	}
}
