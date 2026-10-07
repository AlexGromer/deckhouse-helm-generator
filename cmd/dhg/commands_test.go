package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/generator"
)

const (
	exampleStatefulSet = "../../examples/02-statefulset-db"
	exampleFullStack   = "../../examples/05-full-stack"
	fixtureCollisions  = "../../tests/integration/fixtures/collisions"
)

func TestGenerateCmd_KustomizeWritesBuildableLayout(t *testing.T) {
	out := t.TempDir()
	if _, err := executeCmd(t, "generate", "-f", exampleFullStack, "-o", out, "--chart-name", "app", "--kustomize"); err != nil {
		t.Fatalf("generate: %v", err)
	}
	layout := filepath.Join(out, "app", "kustomize")
	base, err := os.ReadFile(filepath.Join(layout, "base", "kustomization.yaml"))
	if err != nil {
		t.Fatalf("base kustomization: %v", err)
	}
	var k struct {
		Resources []string `json:"resources"`
	}
	if err := yaml.Unmarshal(base, &k); err != nil || len(k.Resources) == 0 {
		t.Fatalf("base kustomization lists no resources: %v\n%s", err, base)
	}
	for _, r := range k.Resources {
		if _, err := os.Stat(filepath.Join(layout, "base", r)); err != nil {
			t.Errorf("base lists %s, which was not written", r)
		}
	}
	for _, env := range []string{"dev", "staging", "prod"} {
		data, err := os.ReadFile(filepath.Join(layout, "overlays", env, "kustomization.yaml"))
		if err != nil {
			t.Fatalf("%s overlay: %v", env, err)
		}
		if !strings.Contains(string(data), "path: /spec/replicas") {
			t.Errorf("%s overlay patches no replicas:\n%s", env, data)
		}
	}
}

func TestGenerateCmd_PostRendererAndFeatures(t *testing.T) {
	out := t.TempDir()
	_, err := executeCmd(t, "generate", "-f", exampleStatefulSet, "-o", out, "--chart-name", "db",
		"--post-renderer", "--with", "velero-backup", "--feature-opt", "velero-backup.schedule=15 3 * * *", "-v")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	for _, f := range []string{"post-renderer/kustomize.sh", "post-renderer/overlays/prod/kustomization.yaml", "templates/velero-schedule.yaml"} {
		if _, err := os.Stat(filepath.Join(out, "db", f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	values, err := os.ReadFile(filepath.Join(out, "db", "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(values), "15 3 * * *") {
		t.Errorf("feature option not applied to values:\n%s", values)
	}
}

func TestGenerateCmd_FeatureErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"option without feature", []string{"--feature-opt", "velero-backup.schedule=1 1 * * *"}, "requires the feature"},
		{"unknown feature", []string{"--with", "no-such-feature"}, "no-such-feature"},
		{"malformed option", []string{"--with", "velero-backup", "--feature-opt", "nodot"}, "nodot"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"generate", "-f", exampleStatefulSet, "-o", t.TempDir(), "--chart-name", "db"}, tt.args...)
			_, err := executeCmd(t, args...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestGenerateCmd_CollisionsKeepEveryObject(t *testing.T) {
	out := t.TempDir()
	if _, err := executeCmd(t, "generate", "-f", fixtureCollisions, "-o", out, "--chart-name", "app"); err != nil {
		t.Fatalf("generate: %v", err)
	}
	templates, err := filepath.Glob(filepath.Join(out, "app", "templates", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) != 4 {
		t.Errorf("want one template per input object (4), got %d: %v", len(templates), templates)
	}
}

func TestFeaturesCmd(t *testing.T) {
	names, err := executeCmd(t, "features", "--names")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(names, "velero-backup\n") || strings.Contains(names, "--feature-opt") {
		t.Errorf("--names output:\n%s", names)
	}
	full, err := executeCmd(t, "features")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(full, `--feature-opt velero-backup.schedule="0 2 * * *"`) {
		t.Errorf("features output lacks documented options:\n%s", full)
	}
}

func TestAnalyzeAndGraphCmds(t *testing.T) {
	report := filepath.Join(t.TempDir(), "report.md")
	if _, err := executeCmd(t, "analyze", "-f", exampleFullStack, "--output-format", "markdown", "-o", report); err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if data, err := os.ReadFile(report); err != nil || len(data) == 0 {
		t.Errorf("analyze wrote no report: %v", err)
	}

	graph := filepath.Join(t.TempDir(), "graph.mmd")
	if _, err := executeCmd(t, "graph", "-f", exampleFullStack, "--format", "mermaid", "-o", graph); err != nil {
		t.Fatalf("graph: %v", err)
	}
	if data, err := os.ReadFile(graph); err != nil || !strings.Contains(string(data), "graph") {
		t.Errorf("graph output: %v\n%s", err, data)
	}
}

func TestFixAndMigrateCmds(t *testing.T) {
	fixed := t.TempDir()
	if _, err := executeCmd(t, "fix", "-f", exampleFullStack, "-o", fixed); err != nil {
		t.Fatalf("fix: %v", err)
	}
	if entries, _ := os.ReadDir(fixed); len(entries) == 0 {
		t.Error("fix wrote nothing")
	}

	chart := t.TempDir()
	if _, err := executeCmd(t, "generate", "-f", exampleFullStack, "-o", chart, "--chart-name", "app"); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := executeCmd(t, "migrate", "--from", filepath.Join(chart, "app"), "-f", exampleFullStack, "--chart-name", "app"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func TestGenerateCmd_TestsHooksReadmeInEveryMode(t *testing.T) {
	for _, mode := range []string{"universal", "separate", "library", "umbrella"} {
		t.Run(mode, func(t *testing.T) {
			out := t.TempDir()
			_, err := executeCmd(t, "generate", "-f", exampleFullStack, "-o", out, "--chart-name", "app",
				"--mode", mode, "--include-tests", "--hooks", "--include-readme", "--include-schema")
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			var tests, readmes, schemas int
			_ = filepath.WalkDir(out, func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				switch {
				case strings.Contains(path, string(filepath.Separator)+"tests"+string(filepath.Separator)):
					tests++
				case filepath.Base(path) == "README.md":
					readmes++
				case filepath.Base(path) == "values.schema.json":
					schemas++
				}
				return nil
			})
			if tests == 0 || readmes == 0 || schemas == 0 {
				t.Errorf("tests=%d readmes=%d schemas=%d, want each > 0", tests, readmes, schemas)
			}
		})
	}
}

// Every registered feature generates in every mode. The golden suite renders
// the results with Helm; this keeps the feature code paths in unit coverage.
func TestGenerateCmd_EveryFeatureEveryMode(t *testing.T) {
	inputs := []string{exampleFullStack, exampleStatefulSet, "../../examples/11-monitoring-stack"}
	for _, f := range generator.Features() {
		for _, mode := range []string{"universal", "separate", "library", "umbrella"} {
			t.Run(f.Name+"/"+mode, func(t *testing.T) {
				for _, input := range inputs {
					out := t.TempDir()
					if _, err := executeCmd(t, "generate", "-f", input, "-o", out, "--chart-name", "app", "--mode", mode, "--with", f.Name); err != nil {
						t.Errorf("%s: %v", input, err)
					}
				}
			})
		}
	}
}

func TestGenerateCmd_SynthesizedSources(t *testing.T) {
	compose := "../../tests/golden/testdata/synth/compose/docker-compose.yml"
	project := "../../tests/golden/testdata/synth/orders-service"

	out := t.TempDir()
	if _, err := executeCmd(t, "generate", "-s", "compose", "-f", compose, "-o", out, "--chart-name", "shop"); err != nil {
		t.Fatalf("compose: %v", err)
	}
	report, err := os.ReadFile(filepath.Join(out, "SYNTHESIS.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"compose file", "| Deployment | api |", "PVC api-uploads", "depends_on is not enforced"} {
		if !strings.Contains(string(report), want) {
			t.Errorf("SYNTHESIS.md lacks %q:\n%s", want, report)
		}
	}

	out = t.TempDir()
	if _, err := executeCmd(t, "generate", "-s", "source", "-f", project, "--image", "orders:2", "-o", out, "--chart-name", "orders"); err != nil {
		t.Fatalf("source: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "SYNTHESIS.md")); err != nil {
		t.Error("SYNTHESIS.md not written for source")
	}

	dry := t.TempDir()
	if _, err := executeCmd(t, "generate", "-s", "source", "-f", project, "-o", dry, "--chart-name", "orders", "--dry-run"); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dry, "SYNTHESIS.md")); err == nil {
		t.Error("--dry-run must not write SYNTHESIS.md")
	}

	if _, err := executeCmd(t, "generate", "-s", "image", "-o", t.TempDir(), "--chart-name", "x"); err == nil || !strings.Contains(err.Error(), "--image") {
		t.Errorf("image without --image: %v", err)
	}
	if _, err := executeCmd(t, "generate", "-s", "nope", "-o", t.TempDir(), "--chart-name", "x"); err == nil || !strings.Contains(err.Error(), "compose") {
		t.Errorf("unknown source: %v", err)
	}
}
