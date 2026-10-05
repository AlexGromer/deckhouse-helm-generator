package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// readExample concatenates the manifests of an example directory.
func readExample(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join("..", "..", "examples", name)
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if len(files) == 0 {
		files, err = filepath.Glob(filepath.Join(dir, "input", "*.yaml"))
	}
	if err != nil || len(files) == 0 {
		t.Fatalf("example %s: %v", name, err)
	}
	var docs []string
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		docs = append(docs, strings.TrimPrefix(strings.TrimSpace(string(data)), "---\n"))
	}
	return strings.Join(docs, "\n---\n")
}

// Every registered feature, with its default options, applies in every mode
// to charts generated from the examples and leaves them valid. The golden
// suite renders the same combinations with Helm.
func TestEveryFeatureEveryMode(t *testing.T) {
	examples := map[string]string{}
	for _, name := range []string{"05-full-stack", "02-statefulset-db", "11-monitoring-stack", "12-gateway-api", "10-deckhouse-module"} {
		examples[name] = readExample(t, name)
	}
	modes := []types.OutputMode{types.OutputModeUniversal, types.OutputModeSeparate, types.OutputModeLibrary, types.OutputModeUmbrella}
	for _, mode := range modes {
		for name, manifests := range examples {
			charts, graph := generateChartsFromManifests(t, manifests, mode)
			for _, f := range Features() {
				out, err := ApplyFeatures(charts, []string{f.Name}, nil, graph)
				if err != nil {
					t.Errorf("%s/%s/%s: %v", f.Name, mode, name, err)
					continue
				}
				for _, chart := range out {
					if err := ValidateChart(chart); err != nil {
						t.Errorf("%s/%s/%s: chart %s invalid: %v", f.Name, mode, name, chart.Name, err)
					}
				}
			}
		}
	}
}

// readFixture concatenates the manifests of tests/integration/fixtures/<name>.
func readFixture(t *testing.T, name string) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "tests", "integration", "fixtures", name, "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("fixture %s: %v", name, err)
	}
	var docs []string
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		docs = append(docs, string(data))
	}
	return strings.Join(docs, "\n---\n")
}

func TestGenerators_TestsHooksReadmeSchema(t *testing.T) {
	manifests := readExample(t, "05-full-stack")
	for _, mode := range []types.OutputMode{types.OutputModeUniversal, types.OutputModeSeparate, types.OutputModeLibrary, types.OutputModeUmbrella} {
		charts, _ := generateChartsWithOptions(t, manifests, Options{
			Mode: mode, IncludeTests: true, IncludeHooks: true, IncludeREADME: true, IncludeSchema: true,
		})
		var tests, readmes, schemas int
		for _, c := range charts {
			for path := range c.Templates {
				if strings.HasPrefix(path, "tests/") {
					tests++
				}
			}
			for _, f := range c.ExternalFiles {
				if f.Path == "README.md" {
					readmes++
				}
			}
			if c.ValuesSchema != "" {
				schemas++
			}
		}
		if tests == 0 || readmes == 0 || schemas == 0 {
			t.Errorf("%s: tests=%d readmes=%d schemas=%d", mode, tests, readmes, schemas)
		}
	}
}

// Two Deployments of one service group keep separate values in group charts.
func TestSeparate_CollidingServicesKeepOwnValues(t *testing.T) {
	charts, _ := generateChartsFromManifests(t, readFixture(t, "collisions"), types.OutputModeSeparate)
	found := false
	for _, c := range charts {
		for path, tpl := range c.Templates {
			if strings.Contains(path, "deployment-worker") {
				found = true
				if !strings.Contains(tpl, "$svc := .Values.shopWorker") {
					t.Errorf("worker template does not read its own values:\n%s", tpl)
				}
				if !strings.Contains(c.ValuesYAML, "shopWorker:") {
					t.Errorf("values lack the worker's key:\n%s", c.ValuesYAML)
				}
			}
		}
	}
	if !found {
		t.Fatal("worker Deployment template not generated")
	}
}

func TestValidateChart_ChartsWithoutTemplates(t *testing.T) {
	base := func() *types.GeneratedChart {
		return &types.GeneratedChart{Name: "c", ChartYAML: "apiVersion: v2\nname: c\nversion: 0.1.0\n", ValuesYAML: "{}\n", Templates: map[string]string{}}
	}
	library := base()
	library.ChartYAML += "type: library\n"
	library.Helpers = `{{- define "c.name" -}}c{{- end -}}`
	umbrella := base()
	umbrella.ChartYAML += "dependencies:\n  - name: a\n    version: 0.1.0\n"
	crdsOnly := base()
	crdsOnly.ExternalFiles = []types.ExternalFileInfo{{Path: "crds/widgets.yaml", Content: "kind: CustomResourceDefinition\n"}}
	for name, c := range map[string]*types.GeneratedChart{"library": library, "umbrella": umbrella, "crds only": crdsOnly} {
		if err := ValidateChart(c); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	empty := base()
	empty.ValuesYAML = ""
	if err := ValidateChart(empty); err == nil || !strings.Contains(err.Error(), "values.yaml is empty") {
		t.Errorf("empty values: %v", err)
	}
}
