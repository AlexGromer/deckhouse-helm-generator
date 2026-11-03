package generator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestFeatureVaultAgent_EnabledParam(t *testing.T) {
	charts, graph := secTestCharts(t, types.OutputModeSeparate, secTestManifests)

	off := secApply(t, charts, graph, "vault-agent", map[string]string{"role": "shop", "enabled": "false"})
	if va := secValues(t, secChartByName(t, off, "shop-api"), "vaultAgent"); va["enabled"] != false {
		t.Errorf("enabled=false: vaultAgent = %v", va)
	}
	on := secApply(t, charts, graph, "vault-agent", map[string]string{"role": "shop", "enabled": "true"})
	if va := secValues(t, secChartByName(t, on, "shop-api"), "vaultAgent"); va["enabled"] != true {
		t.Errorf("enabled=true: vaultAgent = %v", va)
	}
	// Default: on when the feature is requested.
	def := secApply(t, charts, graph, "vault-agent", map[string]string{"role": "shop"})
	if va := secValues(t, secChartByName(t, def, "shop-api"), "vaultAgent"); va["enabled"] != true {
		t.Errorf("default: vaultAgent = %v", va)
	}
	if _, err := ApplyFeatures(charts, []string{"vault-agent"}, map[string]map[string]string{"vault-agent": {"enabled": "maybe"}}, graph); err == nil {
		t.Error("enabled=maybe must be an error")
	}
}

func TestGenerateEnvValues_VaultAgentSwitchPerEnvironment(t *testing.T) {
	base := map[string]interface{}{
		"services": map[string]interface{}{"api": map[string]interface{}{
			"deployment": map[string]interface{}{"replicas": int64(1), "containers": map[string]interface{}{}},
		}},
		"vaultAgent": map[string]interface{}{"enabled": true, "role": "shop", "secretPathPrefix": "secret/data"},
	}
	files := GenerateEnvValues(base)
	want := map[string]bool{"values-dev.yaml": false, "values-staging.yaml": true, "values-prod.yaml": true}
	for file, enabled := range want {
		got := parseEnvYAML(t, files[file])
		va, ok := got["vaultAgent"].(map[string]interface{})
		if !ok {
			t.Fatalf("%s has no vaultAgent block:\n%s", file, files[file])
		}
		if va["enabled"] != enabled {
			t.Errorf("%s: vaultAgent.enabled = %v, want %v", file, va["enabled"], enabled)
		}
		if len(va) != 1 {
			t.Errorf("%s: only the switch is overridden, got %v", file, va)
		}
	}

	// A chart without vault-agent gets no vaultAgent block.
	delete(base, "vaultAgent")
	for file, data := range GenerateEnvValues(base) {
		if strings.Contains(string(data), "vaultAgent") {
			t.Errorf("%s mentions vaultAgent without the feature:\n%s", file, data)
		}
	}
}

// One chart: plain Kubernetes Secrets in base and dev, Vault Agent in prod.
func TestHelmTemplateVaultAgentPerEnvironment(t *testing.T) {
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
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: 2
  selector:
    matchLabels: {app: web}
  template:
    metadata:
      labels: {app: web}
    spec:
      containers:
      - name: app
        image: reg:5000/app:v1
        envFrom:
        - secretRef:
            name: web-creds
---
apiVersion: v1
kind: Secret
metadata:
  name: web-creds
stringData:
  TOKEN: change-me
`
	if err := os.WriteFile(filepath.Join(inputDir, "w.yaml"), []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")
	gen := exec.Command("go", "run", "../../cmd/dhg", "generate", "-f", inputDir, "--chart-name", "app", "--mode", "universal",
		"--with", "vault-agent", "--feature-opt", "vault-agent.role=web", "--feature-opt", "vault-agent.enabled=false",
		"--env-values", "-o", outDir)
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

	for name, args := range map[string][]string{
		"base": nil,
		"dev":  {"-f", filepath.Join(chart, "values-dev.yaml")},
	} {
		out := render(args...)
		if strings.Contains(out, "vault.hashicorp.com") {
			t.Errorf("%s: Vault annotations rendered while disabled:\n%s", name, out)
		}
		if !strings.Contains(out, "kind: Secret") {
			t.Errorf("%s: the Kubernetes Secret is missing", name)
		}
	}
	for _, env := range []string{"staging", "prod"} {
		out := render("-f", filepath.Join(chart, "values-"+env+".yaml"))
		for _, want := range []string{
			`vault.hashicorp.com/agent-inject: "true"`,
			`vault.hashicorp.com/role: web`,
			`vault.hashicorp.com/agent-inject-secret-web-creds: secret/data/web-creds`,
		} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: missing %q:\n%s", env, want, out)
			}
		}
	}
	// An explicit switch beats the overlay.
	if out := render("-f", filepath.Join(chart, "values-prod.yaml"), "--set", "vaultAgent.enabled=false"); strings.Contains(out, "vault.hashicorp.com") {
		t.Errorf("--set vaultAgent.enabled=false must switch Vault off:\n%s", out)
	}
}

// In an umbrella chart helm reads only the values files of the parent, and the
// vaultAgent block lives in the values of each subchart: the overlay of the
// parent switches <subchart>.vaultAgent.enabled.
func TestGenerateEnvValues_UmbrellaVaultAgentSwitchPerSubchart(t *testing.T) {
	sub := func(name string, withVault bool) *types.GeneratedChart {
		values := "enabled: true\ndeployment:\n  replicas: 1\n  containers: {}\n"
		if withVault {
			values += "vaultAgent:\n  enabled: true\n  role: shop\n  secretPathPrefix: secret/data\n"
		}
		return &types.GeneratedChart{Name: "app/charts/" + name, ValuesYAML: values}
	}
	parent := &types.GeneratedChart{Name: "app", ValuesYAML: "global:\n  namePrefix: \"\"\n" +
		"api:\n  enabled: true\n  deployment:\n    replicas: 1\n    containers: {}\n" +
		"db:\n  enabled: true\n  deployment:\n    replicas: 1\n    containers: {}\n"}
	charts := []*types.GeneratedChart{parent, sub("api", true), sub("db", false)}

	base := parseEnvYAML(t, []byte(parent.ValuesYAML))
	MirrorSubchartVaultAgent(base, parent.Name, charts)
	files := GenerateEnvValues(base)

	for file, enabled := range map[string]bool{"values-dev.yaml": false, "values-staging.yaml": true, "values-prod.yaml": true} {
		got := parseEnvYAML(t, files[file])
		api, _ := got["api"].(map[string]interface{})
		va, ok := api["vaultAgent"].(map[string]interface{})
		if !ok {
			t.Fatalf("%s has no api.vaultAgent block:\n%s", file, files[file])
		}
		if va["enabled"] != enabled || len(va) != 1 {
			t.Errorf("%s: api.vaultAgent = %v, want only enabled=%v", file, va, enabled)
		}
		if db, _ := got["db"].(map[string]interface{}); db["vaultAgent"] != nil {
			t.Errorf("%s: db has no Vault Agent, got %v", file, db["vaultAgent"])
		}
	}
	// The base values of the parent are not changed on disk: only the copy that
	// finds the slots carries the mirrored block.
	if strings.Contains(parent.ValuesYAML, "vaultAgent") {
		t.Errorf("parent values changed:\n%s", parent.ValuesYAML)
	}
}

// An umbrella chart: Vault Agent is off in dev and on in prod for the subchart
// that has it, whatever the value in the subchart is.
func TestHelmTemplateVaultAgentPerEnvironmentUmbrella(t *testing.T) {
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
	workload := func(name string) string {
		return `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ` + name + `
  labels: {app.kubernetes.io/name: ` + name + `}
spec:
  replicas: 2
  selector:
    matchLabels: {app: ` + name + `}
  template:
    metadata:
      labels: {app: ` + name + `}
    spec:
      containers:
      - name: app
        image: reg:5000/app:v1
        envFrom:
        - secretRef:
            name: ` + name + `-creds
---
apiVersion: v1
kind: Secret
metadata:
  name: ` + name + `-creds
  labels: {app.kubernetes.io/name: ` + name + `}
stringData:
  TOKEN: change-me
---
`
	}
	if err := os.WriteFile(filepath.Join(inputDir, "w.yaml"), []byte(workload("web")+workload("api")), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")
	gen := exec.Command("go", "run", "../../cmd/dhg", "generate", "-f", inputDir, "--chart-name", "app", "--mode", "umbrella",
		"--with", "vault-agent", "--feature-opt", "vault-agent.role=shop", "--feature-opt", "vault-agent.enabled=true",
		"--env-values", "-o", outDir)
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
	count := func(out string) int { return strings.Count(out, "vault.hashicorp.com/agent-inject: \"true\"") }

	if got := count(render()); got != 2 {
		t.Errorf("base: %d workloads with Vault Agent, want 2", got)
	}
	if got := count(render("-f", filepath.Join(chart, "values-dev.yaml"))); got != 0 {
		t.Errorf("dev: %d workloads with Vault Agent, want 0", got)
	}
	for _, env := range []string{"staging", "prod"} {
		if got := count(render("-f", filepath.Join(chart, "values-"+env+".yaml"))); got != 2 {
			t.Errorf("%s: %d workloads with Vault Agent, want 2", env, got)
		}
	}
	// An explicit switch of one subchart beats the overlay.
	if got := count(render("-f", filepath.Join(chart, "values-prod.yaml"), "--set", "web.vaultAgent.enabled=false")); got != 1 {
		t.Errorf("prod with web off: %d workloads with Vault Agent, want 1", got)
	}
}
