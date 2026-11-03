package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/generator"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestValidateSharedChart(t *testing.T) {
	for _, tc := range []struct {
		mode, shared string
		ok           bool
	}{
		{"separate", "platform", true},
		{"library", "platform", true},
		{"separate", "", true},
		{"universal", "", true},
		{"universal", "platform", false},
		{"umbrella", "platform", false},
		{"separate", "Platform", false},
		{"separate", "a/b", false},
	} {
		err := validateSharedChart(generateOptions{mode: tc.mode, sharedChart: tc.shared})
		if (err == nil) != tc.ok {
			t.Errorf("mode %q shared %q: err = %v, want ok=%v", tc.mode, tc.shared, err, tc.ok)
		}
	}
	err := validateSharedChart(generateOptions{mode: "umbrella", sharedChart: "platform"})
	if err == nil || !strings.Contains(err.Error(), "separate") {
		t.Errorf("the error names the modes that support the flag: %v", err)
	}
}

func TestValidateSharedOverrides(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts generateOptions
		want string // "" = valid
	}{
		{"keep and exclude", generateOptions{mode: "separate", sharedChart: "platform", sharedKeep: []string{"ConfigMap/a"}, sharedExclude: []string{"secret/b"}}, ""},
		{"keep without chart", generateOptions{mode: "separate", sharedKeep: []string{"ConfigMap/a"}}, "--shared-chart"},
		{"exclude without chart", generateOptions{mode: "library", sharedExclude: []string{"ConfigMap/a"}}, "--shared-chart"},
		{"bad entry", generateOptions{mode: "separate", sharedChart: "platform", sharedKeep: []string{"a"}}, "Kind/name"},
		{"both", generateOptions{mode: "separate", sharedChart: "platform", sharedKeep: []string{"ConfigMap/a"}, sharedExclude: []string{"configmap/a"}}, "both"},
	} {
		err := validateSharedChart(tc.opts)
		if (tc.want == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%s: err = %v, want %q", tc.name, err, tc.want)
		}
	}
}

func TestSharedOverridesFromConfigFile(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "dhg.yaml")
	content := "chart-name: app\nmode: separate\nshared-chart: platform\nshared-keep:\n  - ConfigMap/common\n  - Secret/tls\nshared-exclude: [PersistentVolumeClaim/data]\n"
	if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := newGenerateCmd()
	if err := applyConfigFile(cmd, cfg); err != nil {
		t.Fatal(err)
	}
	if got, _ := cmd.Flags().GetStringSlice("shared-keep"); strings.Join(got, ",") != "ConfigMap/common,Secret/tls" {
		t.Errorf("shared-keep = %v", got)
	}
	if got, _ := cmd.Flags().GetStringSlice("shared-exclude"); strings.Join(got, ",") != "PersistentVolumeClaim/data" {
		t.Errorf("shared-exclude = %v", got)
	}
}

func TestLibraryHint(t *testing.T) {
	for ref, want := range map[string]string{
		"oci://r.example.com/p/lib":         "publish the library chart to oci://r.example.com/p/lib (version 1.0.0)",
		"file://../library":                 "was written to out/lib; run \"helm dependency build\"",
		"https://charts.example.com/stable": "chart repository https://charts.example.com/stable (version 1.0.0)",
	} {
		if got := libraryHint(ref, "1.0.0", "out/lib"); !strings.Contains(got, want) {
			t.Errorf("libraryHint(%q) = %q, want it to contain %q", ref, got, want)
		}
	}
}

func TestSharedChartFromConfigFile(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "dhg.yaml")
	if err := os.WriteFile(cfg, []byte("chart-name: app\nmode: separate\nshared-chart: platform\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := newGenerateCmd()
	if err := applyConfigFile(cmd, cfg); err != nil {
		t.Fatal(err)
	}
	if got := cmd.Flags().Lookup("shared-chart").Value.String(); got != "platform" {
		t.Errorf("shared-chart = %q, want platform", got)
	}
}

func sharedMoved(kind, ns, name string, usedBy ...string) generator.SharedObject {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{"kind": kind, "metadata": map[string]interface{}{"name": name, "namespace": ns}}}
	return generator.SharedObject{
		Resource: &types.ProcessedResource{Original: &types.ExtractedResource{Object: obj, GVK: schema.GroupVersionKind{Version: "v1", Kind: kind}}},
		UsedBy:   usedBy,
	}
}

func TestSharedSummary(t *testing.T) {
	moved := []generator.SharedObject{
		sharedMoved("ConfigMap", "prod", "common", "api", "web"),
		sharedMoved("Secret", "prod", "spare"),
	}
	lines := sharedSummary("platform", moved, false)
	if len(lines) != 1 || lines[0] != "2 shared objects moved to chart platform" {
		t.Errorf("lines = %q", lines)
	}
	verbose := sharedSummary("platform", moved, true)
	if len(verbose) != 3 {
		t.Fatalf("verbose lines = %q", verbose)
	}
	for i, want := range []string{"ConfigMap prod/common (used by api, web)", "Secret prod/spare (not referenced by any resource)"} {
		if !strings.Contains(verbose[i+1], want) {
			t.Errorf("line %d = %q, want %q", i+1, verbose[i+1], want)
		}
	}
	if got := sharedSummary("platform", moved[:1], false); got[0] != "1 shared objects moved to chart platform" {
		t.Errorf("singular = %q", got)
	}
	none := sharedSummary("platform", nil, false)
	if len(none) != 1 || !strings.Contains(none[0], "no shared objects") || !strings.Contains(none[0], "platform") {
		t.Errorf("nothing moved = %q", none)
	}
}

func TestInstallHint_SharedChartFirst(t *testing.T) {
	mk := func(names ...string) []*types.GeneratedChart {
		var cs []*types.GeneratedChart
		for _, n := range names {
			cs = append(cs, &types.GeneratedChart{Name: n, ChartYAML: "name: " + n + "\ntype: application\n"})
		}
		return cs
	}
	var sb strings.Builder
	printInstallHint(&sb, "/out", mk("api", "web", "platform"), "app", "prod", "platform")
	got := sb.String()
	iPlatform := strings.Index(got, "helm install platform /out/platform")
	iAPI := strings.Index(got, "helm install api /out/api")
	iWeb := strings.Index(got, "helm install web /out/web")
	if iPlatform < 0 || iAPI < 0 || iWeb < 0 || iPlatform > iAPI || iPlatform > iWeb {
		t.Errorf("the shared chart must be installed first:\n%s", got)
	}
	if !strings.Contains(got, "platform first") {
		t.Errorf("no install order remark:\n%s", got)
	}
}

func TestInstallHint_SharedChartAbsent(t *testing.T) {
	var sb strings.Builder
	cs := []*types.GeneratedChart{{Name: "web", ChartYAML: "name: web\ntype: application\n"}}
	printInstallHint(&sb, "/out", cs, "app", "prod", "platform")
	if strings.Contains(sb.String(), "first") {
		t.Errorf("nothing was shared, no order remark expected:\n%s", sb.String())
	}
}

func TestLibraryChartDir(t *testing.T) {
	charts := []*types.GeneratedChart{
		{Name: "web", ChartYAML: "name: web\ntype: application\n"},
		{Name: "common-lib", ChartYAML: "name: common-lib\ntype: library\n"},
	}
	if got := libraryChartDir("out", charts); got != filepath.Join("out", "common-lib") {
		t.Errorf("libraryChartDir = %q", got)
	}
	if got := libraryChartDir("out", charts[:1]); got != "" {
		t.Errorf("no library: %q", got)
	}
}
