package k8s

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/testutil"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func jobWithState(state string) (processor.Context, *types.ExtractedResource) {
	obj := makeJobObj("migrate", "prod", map[string]interface{}{"app": "migrate"},
		makeBasicJobSpec("migrate", "migrate:1.0"))
	res := &types.ExtractedResource{
		Object:        obj,
		Source:        types.SourceCluster,
		GVK:           obj.GroupVersionKind(),
		FinishedState: state,
	}
	ctx := newTestProcessorContext()
	ctx.AllResources = map[types.ResourceKey]*types.ExtractedResource{res.ResourceKey(): res}
	return ctx, res
}

func hookValues(t *testing.T, values map[string]interface{}) map[string]interface{} {
	t.Helper()
	hook, ok := values["hook"].(map[string]interface{})
	if !ok {
		t.Fatalf("values[hook] = %#v; want a map", values["hook"])
	}
	return hook
}

func TestProcessJob_SucceededClusterJobGetsHookEnabled(t *testing.T) {
	ctx, res := jobWithState("succeeded")
	result, err := NewJobProcessor().Process(ctx, res.Object)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Values["enabled"])
	hook := hookValues(t, result.Values)
	testutil.AssertEqual(t, true, hook["enabled"])
	testutil.AssertEqual(t, "pre-install,pre-upgrade", hook["events"])
	testutil.AssertEqual(t, "before-hook-creation", hook["deletePolicy"])
	testutil.AssertEqual(t, "0", hook["weight"])
}

// A hook that fails aborts the install and every upgrade: a Job that already
// failed in the cluster must not become one by default.
func TestProcessJob_FailedClusterJobGetsHookDisabled(t *testing.T) {
	ctx, res := jobWithState("failed")
	result, err := NewJobProcessor().Process(ctx, res.Object)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Values["enabled"])
	testutil.AssertEqual(t, false, hookValues(t, result.Values)["enabled"])
}

// A pre-install hook runs before any object of the chart exists: a Job that
// reads a Secret, ConfigMap or ServiceAccount of the chart would hang. Such a
// Job defaults to post-install,post-upgrade.
func TestProcessJob_HookWithInChartDependenciesRunsAfterInstall(t *testing.T) {
	jobWithEnvFrom := func(secret string) (processor.Context, *types.ExtractedResource) {
		ctx, res := jobWithState("succeeded")
		containers, _, _ := unstructured.NestedSlice(res.Object.Object, "spec", "template", "spec", "containers")
		containers[0].(map[string]interface{})["envFrom"] = []interface{}{
			map[string]interface{}{"secretRef": map[string]interface{}{"name": secret}},
		}
		if err := unstructured.SetNestedSlice(res.Object.Object, containers, "spec", "template", "spec", "containers"); err != nil {
			t.Fatal(err)
		}
		return ctx, res
	}
	addSecret := func(ctx processor.Context, name string) {
		obj := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Secret",
			"metadata": map[string]interface{}{"name": name, "namespace": "prod"},
		}}
		r := &types.ExtractedResource{Object: obj, GVK: obj.GroupVersionKind()}
		ctx.AllResources[r.ResourceKey()] = r
	}

	t.Run("secret in the chart", func(t *testing.T) {
		ctx, res := jobWithEnvFrom("db-creds")
		addSecret(ctx, "db-creds")
		result, err := NewJobProcessor().Process(ctx, res.Object)
		testutil.AssertNoError(t, err)
		hook := hookValues(t, result.Values)
		testutil.AssertEqual(t, true, hook["enabled"])
		testutil.AssertEqual(t, "post-install,post-upgrade", hook["events"])
	})
	t.Run("secret outside the chart", func(t *testing.T) {
		ctx, res := jobWithEnvFrom("external-creds")
		result, err := NewJobProcessor().Process(ctx, res.Object)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, "pre-install,pre-upgrade", hookValues(t, result.Values)["events"])
	})
}

// A values file without the enabled key (written by hand, or made before the
// key existed) must still render the Job.
func TestProcessJob_TemplateRendersWhenEnabledKeyIsMissing(t *testing.T) {
	ctx, res := jobWithState("")
	result, err := NewJobProcessor().Process(ctx, res.Object)
	testutil.AssertNoError(t, err)
	testutil.AssertContains(t, result.TemplateContent, `{{- if (hasKey . "enabled" | ternary .enabled true) }}`)
}

func TestProcessJob_OtherJobsGetHookDisabled(t *testing.T) {
	// Not finished (empty state) and not from the cluster at all.
	ctx, res := jobWithState("")
	for name, c := range map[string]processor.Context{
		"cluster-running": ctx,
		"no-resources":    newTestProcessorContext(),
	} {
		t.Run(name, func(t *testing.T) {
			result, err := NewJobProcessor().Process(c, res.Object)
			testutil.AssertNoError(t, err)
			testutil.AssertEqual(t, true, result.Values["enabled"])
			hook := hookValues(t, result.Values)
			testutil.AssertEqual(t, false, hook["enabled"])
			testutil.AssertEqual(t, "pre-install,pre-upgrade", hook["events"])
			testutil.AssertEqual(t, "before-hook-creation", hook["deletePolicy"])
			testutil.AssertEqual(t, "0", hook["weight"])
		})
	}
}

func TestProcessJob_TemplateGuardsAndHookAnnotations(t *testing.T) {
	ctx, res := jobWithState("succeeded")
	result, err := NewJobProcessor().Process(ctx, res.Object)
	testutil.AssertNoError(t, err)
	tmpl := result.TemplateContent
	testutil.AssertContains(t, tmpl, `{{- if (hasKey . "enabled" | ternary .enabled true) }}`)
	for _, want := range []string{"helm.sh/hook", "helm.sh/hook-weight", "helm.sh/hook-delete-policy", "$hook.events", "$hook.weight", "$hook.deletePolicy"} {
		testutil.AssertContains(t, tmpl, want)
	}
	if strings.Count(tmpl, "helm.sh/hook:") != 1 {
		t.Errorf("helm.sh/hook rendered %d times; want once\n%s", strings.Count(tmpl, "helm.sh/hook:"), tmpl)
	}
}

func TestProcessJob_InputHookAnnotationsAreNotDuplicated(t *testing.T) {
	ctx, res := jobWithState("succeeded")
	res.Object.SetAnnotations(map[string]string{
		"helm.sh/hook":            "post-install",
		"helm.sh/hook-weight":     "5",
		"helm.sh/resource-policy": "keep",
		"team":                    "data",
	})
	result, err := NewJobProcessor().Process(ctx, res.Object)
	testutil.AssertNoError(t, err)
	tmpl := result.TemplateContent
	// The input's hook wins: one literal key each, none from the switch.
	if n := strings.Count(tmpl, "helm.sh/hook:"); n != 1 {
		t.Errorf("helm.sh/hook appears %d times; want 1\n%s", n, tmpl)
	}
	if n := strings.Count(tmpl, "helm.sh/hook-weight"); n != 1 {
		t.Errorf("helm.sh/hook-weight appears %d times; want 1\n%s", n, tmpl)
	}
	if strings.Contains(tmpl, "$hook.events") {
		t.Errorf("switch-driven hook must not be rendered next to the input's hook\n%s", tmpl)
	}
	testutil.AssertContains(t, tmpl, "post-install")
	testutil.AssertContains(t, tmpl, "helm.sh/resource-policy")
	testutil.AssertEqual(t, true, result.Values["enabled"])
	if _, set := result.Values["hook"]; set {
		t.Errorf("a Job with its own hook annotations gets no hook switch, got %v", result.Values["hook"])
	}
}

func TestHelmTemplateJobHookSwitches(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		if os.Getenv("DHG_REQUIRE_HELM") != "" {
			t.Fatalf("helm not found: %v", err)
		}
		t.Skip("helm not installed")
	}
	dir := t.TempDir()
	inputDir := filepath.Join(dir, "input")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	input := `
apiVersion: batch/v1
kind: Job
metadata:
  name: migrate
  annotations:
    team: data
spec:
  backoffLimit: 1
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: migrate
        image: reg:5000/migrate:v1
`
	if err := os.WriteFile(filepath.Join(inputDir, "j.yaml"), []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")
	gen := exec.Command("go", "run", "../../../cmd/dhg", "generate", "-f", inputDir, "--chart-name", "app", "--mode", "universal", "-o", outDir)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("dhg generate failed: %v\n%s", err, out)
	}
	helmPath := "helm"
	if p := os.Getenv("DHG_HELM_PATH"); p != "" {
		helmPath = p
	}
	chart := filepath.Join(outDir, "app")
	render := func(args ...string) string {
		t.Helper()
		out, err := exec.Command(helmPath, append([]string{"template", "r", chart}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("helm template %v failed: %v\n%s", args, err, out)
		}
		return string(out)
	}

	plain := render()
	testutil.AssertContains(t, plain, "kind: Job")
	if strings.Contains(plain, "helm.sh/hook") {
		t.Errorf("default render of a non-cluster Job must have no hook annotations:\n%s", plain)
	}
	testutil.AssertContains(t, plain, "team: data")

	hooked := render("--set", "services.migrate.job.hook.enabled=true")
	for _, want := range []string{
		`helm.sh/hook: "pre-install,pre-upgrade"`,
		`helm.sh/hook-weight: "0"`,
		`helm.sh/hook-delete-policy: "before-hook-creation"`,
		"team: data",
	} {
		testutil.AssertContains(t, hooked, want)
	}

	custom := render(
		"--set", "services.migrate.job.hook.enabled=true",
		"--set", "services.migrate.job.hook.events=post-install",
		"--set", "services.migrate.job.hook.weight=-3",
		"--set", "services.migrate.job.hook.deletePolicy=hook-succeeded",
	)
	for _, want := range []string{`helm.sh/hook: "post-install"`, `helm.sh/hook-weight: "-3"`, `helm.sh/hook-delete-policy: "hook-succeeded"`} {
		testutil.AssertContains(t, custom, want)
	}

	off := render("--set", "services.migrate.job.enabled=false")
	if strings.Contains(off, "kind: Job") {
		t.Errorf("job.enabled=false must render no Job:\n%s", off)
	}
}
