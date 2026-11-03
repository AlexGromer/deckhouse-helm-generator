package golden

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// policyOf is the helm.sh/resource-policy annotation of a rendered object.
func policyOf(o object) string {
	return str(get(map[string]interface{}(o), "metadata", "annotations", "helm.sh/resource-policy"))
}

// Everything the shared chart renders carries helm.sh/resource-policy: keep
// (helm uninstall of the release leaves it), existing annotations stay, and
// the data of its ConfigMaps is in values, so it can be edited with
// helm upgrade --set.
func TestSharedChart_ResourcePolicyKeep(t *testing.T) {
	helm := requireHelm(t)
	for _, mode := range []string{"separate", "library"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			in := writeSharedInput(t, dir)
			// An annotation the object has already.
			app := filepath.Join(in, "app.yaml")
			data, err := os.ReadFile(app)
			if err != nil {
				t.Fatal(err)
			}
			patched := strings.Replace(string(data), "kind: ConfigMap\nmetadata:\n  name: common\n", "kind: ConfigMap\nmetadata:\n  name: common\n  annotations:\n    team: platform\n", 1)
			if patched == string(data) {
				t.Fatal("the fixture did not change")
			}
			if err := os.WriteFile(app, []byte(patched), 0o644); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(dir, "out")
			runOK(t, dhgBin, "generate", "-f", in, "-o", out, "--chart-name", "app", "--mode", mode, "--shared-chart", "platform")
			checkCharts(t, helm, out)
			checkReleaseIntegrity(t, helm, in, out, true)

			platform := renderChart(t, helm, out, "platform")
			sameIDs(t, "platform", platform, "ConfigMap/common", "ServiceAccount/runner", "Secret/spare")
			for _, o := range platform {
				if got := policyOf(o); got != "keep" {
					t.Errorf("%s/%s: helm.sh/resource-policy = %q, want keep", o.kind(), o.name(), got)
				}
			}
			common := findObject(platform, "ConfigMap", "common")
			if got := str(get(map[string]interface{}(common), "metadata", "annotations", "team")); got != "platform" {
				t.Errorf("the annotation of the input is lost: %v", get(map[string]interface{}(common), "metadata", "annotations"))
			}
			for _, svc := range []string{"web", "api"} {
				for _, o := range renderChart(t, helm, out, svc) {
					if got := policyOf(o); got != "" {
						t.Errorf("%s: %s/%s carries helm.sh/resource-policy = %q", svc, o.kind(), o.name(), got)
					}
				}
			}

			// The data of the shared ConfigMap is a value of the shared chart.
			edited := renderChart(t, helm, out, "platform", "--set", "configMaps.common.data.LOG_LEVEL=debug")
			cm := findObject(edited, "ConfigMap", "common")
			if got := str(get(map[string]interface{}(cm), "data", "LOG_LEVEL")); got != "debug" {
				t.Errorf("LOG_LEVEL = %q after --set configMaps.common.data.LOG_LEVEL=debug, want debug", got)
			}
			if got := policyOf(cm); got != "keep" {
				t.Errorf("--set of the data dropped the policy: %q", got)
			}
		})
	}
}

// --shared-exclude keeps an object out of the shared chart, --shared-keep
// keeps it with a service chart; both from the config file too. Every
// reference still resolves.
func TestSharedChart_KeepAndExclude(t *testing.T) {
	helm := requireHelm(t)
	for _, viaConfig := range []bool{false, true} {
		name := "flags"
		if viaConfig {
			name = "config-file"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			in := writeSharedInput(t, dir)
			out := filepath.Join(dir, "out")
			args := []string{"generate", "-f", in, "-o", out, "--chart-name", "app", "--mode", "separate", "--shared-chart", "platform"}
			if viaConfig {
				cfg := filepath.Join(dir, "dhg.yaml")
				if err := os.WriteFile(cfg, []byte("shared-keep:\n  - serviceaccount/runner\n  - ConfigMap/ghost\nshared-exclude: [ConfigMap/common]\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--config", cfg)
			} else {
				args = append(args, "--shared-keep", "serviceaccount/runner,ConfigMap/ghost", "--shared-exclude", "ConfigMap/common")
			}
			log := runOK(t, dhgBin, args...)
			for _, want := range []string{
				"Note: --shared-keep: no object ConfigMap/ghost in the input, ignored",
				"Note: --shared-keep: ServiceAccount/runner stays in chart api, the first (in sorted order) of the charts that reference it (api, web)",
				"1 shared objects moved to chart platform",
			} {
				if !strings.Contains(log, want) {
					t.Errorf("output lacks %q:\n%s", want, log)
				}
			}
			checkCharts(t, helm, out)
			checkReleaseIntegrity(t, helm, in, out, true)

			// Only the Secret nothing references is left for the shared chart.
			sameIDs(t, "platform", renderChart(t, helm, out, "platform"), "Secret/spare")
			holders := map[string][]string{}
			for _, chart := range []string{"api", "web"} {
				for _, o := range renderChart(t, helm, out, chart) {
					holders[o.kind()+"/"+o.name()] = append(holders[o.kind()+"/"+o.name()], chart)
				}
			}
			if got := holders["ServiceAccount/runner"]; len(got) != 1 || got[0] != "api" {
				t.Errorf("ServiceAccount/runner is in %v, want [api]", got)
			}
			if got := holders["ConfigMap/common"]; len(got) != 1 {
				t.Errorf("ConfigMap/common is in %v, want one service chart", got)
			}
			// Kept and excluded objects are not shared: no policy.
			for _, o := range renderChart(t, helm, out, "api") {
				if policyOf(o) != "" {
					t.Errorf("api: %s/%s carries helm.sh/resource-policy", o.kind(), o.name())
				}
			}
		})
	}
}

func TestSharedChart_OverridesNeedTheChart(t *testing.T) {
	dir := t.TempDir()
	in := writeSharedInput(t, dir)
	for _, flag := range []string{"--shared-keep", "--shared-exclude"} {
		out, err := run(dhgBin, "generate", "-f", in, "-o", filepath.Join(dir, "o"+flag), "--chart-name", "app", "--mode", "separate", flag, "ConfigMap/common")
		if err == nil || !strings.Contains(out, "--shared-chart") {
			t.Errorf("%s without --shared-chart: err = %v, output:\n%s", flag, err, out)
		}
	}
	out, err := run(dhgBin, "generate", "-f", in, "-o", filepath.Join(dir, "both"), "--chart-name", "app", "--mode", "separate", "--shared-chart", "platform",
		"--shared-keep", "ConfigMap/common", "--shared-exclude", "configmap/common")
	if err == nil || !strings.Contains(out, "both --shared-keep and --shared-exclude") {
		t.Errorf("both lists: err = %v, output:\n%s", err, out)
	}
}
