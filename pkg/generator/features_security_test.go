package generator

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/analyzer"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/analyzer/detector"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor/k8s"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor/value"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// secTestManifests is a small application: a Deployment consuming three
// Secrets (one of them shipped as a manifest), a CronJob calling S3 and a
// Service.
const secTestManifests = `
apiVersion: v1
kind: Secret
metadata:
  name: api-keys
  namespace: shop
  labels:
    app.kubernetes.io/name: shop-api
type: Opaque
data:
  stripe-key: c2tfdGVzdA==
---
apiVersion: v1
kind: Secret
metadata:
  name: builder-token
  namespace: shop
  labels:
    app.kubernetes.io/name: shop-api
type: kubernetes.io/service-account-token
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: shop-api
  namespace: shop
  labels:
    app.kubernetes.io/name: shop-api
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: shop-api
  template:
    metadata:
      labels:
        app.kubernetes.io/name: shop-api
      annotations:
        prometheus.io/scrape: "true"
    spec:
      imagePullSecrets:
        - name: registry-creds
      containers:
        - name: api
          image: ghcr.io/acme/shop-api:1.2.3
          env:
            - name: PAYMENTS_URL
              value: https://api.stripe.com/v1
            - name: CACHE_HOST
              value: redis:6379
            - name: SMTP_HOST
              value: smtp.example.org
            - name: SMTP_PORT
              value: "587"
            - name: DB_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: db-credentials
                  key: password
          envFrom:
            - secretRef:
                name: api-keys
          volumeMounts:
            - name: tls
              mountPath: /tls
      volumes:
        - name: tls
          secret:
            secretName: api-tls
---
apiVersion: batch/v1
kind: CronJob
metadata:
  name: shop-report
  namespace: shop
  labels:
    app.kubernetes.io/name: shop-report
spec:
  schedule: "0 * * * *"
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: OnFailure
          containers:
            - name: report
              image: acme/report:1.0
              env:
                - name: S3_ENDPOINT
                  value: https://s3.eu-west-1.amazonaws.com
                - name: API_URL
                  value: http://shop-api.shop:8080
                - name: DB_PASSWORD
                  valueFrom:
                    secretKeyRef:
                      name: db-credentials
                      key: password
---
apiVersion: v1
kind: Service
metadata:
  name: shop-api
  namespace: shop
  labels:
    app.kubernetes.io/name: shop-api
spec:
  selector:
    app.kubernetes.io/name: shop-api
  ports:
    - port: 8080
`

// secTestCharts runs the real dhg pipeline (processors, analyzer, generator)
// over manifests and returns the generated charts and the resource graph.
func secTestCharts(t *testing.T, mode types.OutputMode, manifests string) ([]*types.GeneratedChart, *types.ResourceGraph) {
	t.Helper()
	ctx := context.Background()

	registry := processor.NewRegistry()
	k8s.RegisterAll(registry)
	files := value.NewExternalFileManager()
	all := map[types.ResourceKey]*types.ExtractedResource{}
	var extracted []*types.ExtractedResource
	for _, doc := range strings.Split(manifests, "\n---\n") {
		if strings.TrimSpace(doc) == "" {
			continue
		}
		obj := &unstructured.Unstructured{}
		if err := yaml.Unmarshal([]byte(doc), &obj.Object); err != nil {
			t.Fatalf("parsing manifest: %v", err)
		}
		r := &types.ExtractedResource{Object: obj, Source: types.SourceFile, GVK: obj.GroupVersionKind()}
		extracted = append(extracted, r)
		all[r.ResourceKey()] = r
	}

	var processed []*types.ProcessedResource
	for _, r := range extracted {
		res, err := registry.Process(processor.Context{
			Ctx: ctx, ChartName: "app", OutputMode: mode, Namespace: r.Object.GetNamespace(),
			AllResources: all, ExternalFileManager: files, ValueProcessor: value.DefaultProcessor(),
		}, r.Object)
		if err != nil {
			t.Fatalf("processing %s: %v", r.ResourceKey(), err)
		}
		processed = append(processed, &types.ProcessedResource{
			Original: r, ServiceName: res.ServiceName, TemplatePath: res.TemplatePath,
			TemplateContent: res.TemplateContent, ValuesPath: res.ValuesPath, Values: res.Values,
			Dependencies: res.Dependencies,
		})
	}

	a := analyzer.NewDefaultAnalyzer()
	detector.RegisterAll(a)
	graph, err := a.Analyze(ctx, processed)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	gen, err := DefaultRegistry().Get(mode)
	if err != nil {
		t.Fatal(err)
	}
	charts, err := gen.Generate(ctx, graph, Options{
		OutputDir: t.TempDir(), ChartName: "app", ChartVersion: "0.1.0", AppVersion: "1.0.0",
		Mode: mode, ExternalFileManager: files,
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return charts, graph
}

func secApply(t *testing.T, charts []*types.GeneratedChart, graph *types.ResourceGraph, feature string, opts map[string]string) []*types.GeneratedChart {
	t.Helper()
	var options map[string]map[string]string
	if opts != nil {
		options = map[string]map[string]string{feature: opts}
	}
	out, err := ApplyFeatures(charts, []string{feature}, options, graph)
	if err != nil {
		t.Fatalf("ApplyFeatures(%s): %v", feature, err)
	}
	return out
}

func secChartByName(t *testing.T, charts []*types.GeneratedChart, name string) *types.GeneratedChart {
	t.Helper()
	for _, c := range charts {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("chart %q not generated", name)
	return nil
}

// secValues parses the chart's values.yaml and returns a top-level key.
func secValues(t *testing.T, chart *types.GeneratedChart, key string) map[string]interface{} {
	t.Helper()
	var values map[string]interface{}
	if err := yaml.Unmarshal([]byte(chart.ValuesYAML), &values); err != nil {
		t.Fatalf("values.yaml of %s does not parse: %v", chart.Name, err)
	}
	block, _ := values[key].(map[string]interface{})
	return block
}

func TestSecSecretRefs(t *testing.T) {
	podSpec := map[string]interface{}{
		"initContainers": []interface{}{map[string]interface{}{
			"env": []interface{}{map[string]interface{}{
				"name": "A", "valueFrom": map[string]interface{}{"secretKeyRef": map[string]interface{}{"name": "s1", "key": "k2"}},
			}},
		}},
		"containers": []interface{}{map[string]interface{}{
			"env": []interface{}{
				map[string]interface{}{"name": "B", "valueFrom": map[string]interface{}{"secretKeyRef": map[string]interface{}{"name": "s1", "key": "k1"}}},
				map[string]interface{}{"name": "C", "valueFrom": map[string]interface{}{"configMapKeyRef": map[string]interface{}{"name": "cm", "key": "x"}}},
			},
			"envFrom": []interface{}{map[string]interface{}{"secretRef": map[string]interface{}{"name": "s2"}}},
		}},
		"volumes": []interface{}{
			map[string]interface{}{"name": "v1", "secret": map[string]interface{}{"secretName": "s3", "items": []interface{}{map[string]interface{}{"key": "tls.crt"}}}},
			map[string]interface{}{"name": "v2", "projected": map[string]interface{}{"sources": []interface{}{
				map[string]interface{}{"secret": map[string]interface{}{"name": "s4"}},
			}}},
		},
	}
	got := secSecretRefs(podSpec)
	want := []secSecretRef{
		{Name: "s1", Keys: []string{"k1", "k2"}},
		{Name: "s2", Whole: true},
		{Name: "s3", Keys: []string{"tls.crt"}},
		{Name: "s4", Whole: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("secSecretRefs =\n%+v\nwant\n%+v", got, want)
	}
}

func TestSecChartHelpers(t *testing.T) {
	charts, _ := secTestCharts(t, types.OutputModeSeparate, secTestManifests)
	h := newSecChartHelpers(secChartByName(t, charts, "shop-api"))
	if h.fullnameTpl != "shop-api.fullname" || h.labelsTpl != "shop-api.labels" {
		t.Errorf("helpers = %+v", h)
	}
	none := newSecChartHelpers(&types.GeneratedChart{Name: "x", Templates: map[string]string{}})
	if none.fullname() != "$.Release.Name" || strings.Contains(none.labels(4), "include") {
		t.Errorf("fallback helpers = %q / %q", none.fullname(), none.labels(4))
	}
}

func TestSecurityFeaturesRegistered(t *testing.T) {
	for _, name := range []string{"policies", "external-secrets", "vault-agent", "istio-egress"} {
		if _, ok := LookupFeature(name); !ok {
			t.Errorf("feature %q not registered", name)
		}
	}
}

func TestFeaturePolicies(t *testing.T) {
	charts, graph := secTestCharts(t, types.OutputModeUniversal, secTestManifests)
	out := secApply(t, charts, graph, "policies", nil)[0]

	tpl, ok := out.Templates["templates/admission-policies.yaml"]
	if !ok {
		t.Fatal("kyverno policies template not added")
	}
	for _, want := range []string{
		"apiVersion: kyverno.io/v1\nkind: Policy\n",
		`{{ printf "%s-%s" (include "app.fullname" $) "disallow-privileged" | trunc 63 | trimSuffix "-" }}`,
		"{{- if $ap.rules.requireProbes }}",
		`=(privileged): "false"`,
	} {
		if !strings.Contains(tpl, want) {
			t.Errorf("policies template misses %q", want)
		}
	}
	if strings.Contains(tpl, "restrict-registries") {
		t.Error("restrict-registries must only be generated when registries are configured")
	}
	if got := strings.Count(tpl, "kind: Policy\n"); got != len(defaultPolicyRules) {
		t.Errorf("%d policies generated, want %d", got, len(defaultPolicyRules))
	}
	ap := secValues(t, out, "admissionPolicies")
	if ap["enabled"] != true || ap["validationFailureAction"] != "Audit" {
		t.Errorf("admissionPolicies values = %v", ap)
	}
	if len(out.ExternalFiles) != len(charts[0].ExternalFiles) {
		t.Error("kyverno engine must not add files outside templates/")
	}

	// conftest engine with a registry allow-list and a subset of rules.
	out = secApply(t, charts, graph, "policies", map[string]string{
		"engine": "conftest", "rules": "disallow-privileged", "registries": "ghcr.io/acme",
	})[0]
	if _, ok := out.Templates["templates/admission-policies.yaml"]; ok {
		t.Error("conftest engine must not add Kyverno templates")
	}
	var paths []string
	for _, f := range out.ExternalFiles[len(charts[0].ExternalFiles):] {
		paths = append(paths, f.Path)
		if !strings.Contains(f.Content, "package main\n\nimport rego.v1\n") {
			t.Errorf("%s is not an OPA v1 policy:\n%s", f.Path, f.Content)
		}
	}
	sort.Strings(paths)
	want := []string{"policy/dhg_workloads.rego", "policy/disallow_privileged.rego", "policy/restrict_registries.rego"}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("conftest files = %v, want %v", paths, want)
	}

	// Charts without workloads (umbrella parent) are left alone.
	umbrella, ugraph := secTestCharts(t, types.OutputModeUmbrella, secTestManifests)
	uout := secApply(t, umbrella, ugraph, "policies", nil)
	if uout[0] != umbrella[0] {
		t.Error("umbrella parent chart without workloads must be unchanged")
	}

	for _, bad := range []map[string]string{{"engine": "gatekeeper"}, {"rules": "nope"}, {"action": "deny"}} {
		if _, err := ApplyFeatures(charts, []string{"policies"}, map[string]map[string]string{"policies": bad}, graph); err == nil {
			t.Errorf("expected error for %v", bad)
		}
	}
}

func TestFeatureExternalSecrets(t *testing.T) {
	charts, graph := secTestCharts(t, types.OutputModeUniversal, secTestManifests)
	out := secApply(t, charts, graph, "external-secrets", map[string]string{"key-prefix": "shop/"})[0]

	tpl := out.Templates["templates/external-secrets.yaml"]
	for _, want := range []string{"apiVersion: external-secrets.io/v1\nkind: ExternalSecret\n", "dataFrom:", "property: {{ . | quote }}"} {
		if !strings.Contains(tpl, want) {
			t.Errorf("external-secrets template misses %q", want)
		}
	}
	es := secValues(t, out, "externalSecrets")
	store, _ := es["secretStoreRef"].(map[string]interface{})
	if es["enabled"] != true || store["name"] != "secret-store" || store["kind"] != "ClusterSecretStore" {
		t.Errorf("externalSecrets values = %v", es)
	}
	secrets, _ := es["secrets"].(map[string]interface{})
	got := map[string]interface{}{}
	for name, v := range secrets {
		got[name] = v
	}
	want := map[string]interface{}{
		// From the Secret manifest: its keys are known.
		"api-keys": map[string]interface{}{"remoteKey": "shop/api-keys", "keys": []interface{}{"stripe-key"}},
		// Mounted whole and not shipped: all properties.
		"api-tls": map[string]interface{}{"remoteKey": "shop/api-tls", "keys": []interface{}{}},
		// Referenced key by key.
		"db-credentials": map[string]interface{}{"remoteKey": "shop/db-credentials", "keys": []interface{}{"password"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("externalSecrets.secrets =\n%v\nwant\n%v", got, want)
	}
	// The chart's own api-keys Secret (same name as the ExternalSecret's
	// target) gives way to the ExternalSecret; other Secrets are untouched.
	guarded := 0
	for path, content := range out.Templates {
		if !strings.Contains(content, "kind: Secret\n") {
			continue
		}
		if !strings.Contains(content, "  name: api-keys\n") {
			if strings.Contains(content, "externalSecrets") {
				t.Errorf("%s must not be guarded:\n%s", path, content)
			}
			continue
		}
		if !strings.HasPrefix(content, `{{- if not (and .Values.externalSecrets .Values.externalSecrets.enabled (hasKey (.Values.externalSecrets.secrets | default dict) "api-keys")) }}`) ||
			!strings.HasSuffix(content, "\n{{- end }}\n") {
			t.Errorf("%s is not guarded by externalSecrets:\n%s", path, content)
		}
		guarded++
	}
	if guarded != 1 {
		t.Errorf("guarded %d Secret templates, want 1", guarded)
	}

	// Separate mode: db-credentials is used by both charts but owned by one.
	sep, sgraph := secTestCharts(t, types.OutputModeSeparate, secTestManifests)
	sout := secApply(t, sep, sgraph, "external-secrets", nil)
	owners := 0
	for _, c := range sout {
		s, _ := secValues(t, c, "externalSecrets")["secrets"].(map[string]interface{})
		if _, ok := s["db-credentials"]; ok {
			owners++
		}
	}
	if owners != 1 {
		t.Errorf("db-credentials declared by %d charts, want exactly 1", owners)
	}

	// No secrets: chart unchanged.
	plain, pgraph := secTestCharts(t, types.OutputModeUniversal, `
apiVersion: v1
kind: ConfigMap
metadata:
  name: cfg
data:
  a: b
`)
	if pout := secApply(t, plain, pgraph, "external-secrets", nil); pout[0] != plain[0] {
		t.Error("chart without secrets must be unchanged")
	}
}

func TestFeatureVaultAgent(t *testing.T) {
	charts, graph := secTestCharts(t, types.OutputModeSeparate, secTestManifests)
	out := secApply(t, charts, graph, "vault-agent", map[string]string{"role": "shop"})

	api := secChartByName(t, out, "shop-api")
	deploy := api.Templates["templates/shopApi-deployment.yaml"]
	wantHook := `{{- with include "shop-api.vaultAgent.podAnnotations" (dict "root" $ "podAnnotations" .podAnnotations "secrets" (list "api-keys" "api-tls" "db-credentials")) | fromYaml }}`
	if !strings.Contains(deploy, wantHook) {
		t.Errorf("deployment pod annotations not wired:\n%s", deploy)
	}
	if strings.Contains(deploy, vaultAgentPodAnnotationsHook) {
		t.Error("original pod-annotations hook still present")
	}
	helper := api.Templates["templates/_vault-agent.tpl"]
	if !strings.Contains(helper, `{{- define "shop-api.vaultAgent.podAnnotations" -}}`) ||
		!strings.Contains(helper, `"vault.hashicorp.com/agent-inject-secret-%s"`) {
		t.Errorf("helper template:\n%s", helper)
	}
	va := secValues(t, api, "vaultAgent")
	if va["enabled"] != true || va["role"] != "shop" || va["secretPathPrefix"] != "secret/data" {
		t.Errorf("vaultAgent values = %v", va)
	}

	// The CronJob template has no pod-annotations hook: chart unchanged.
	report := secChartByName(t, charts, "shop-report")
	if secChartByName(t, out, "shop-report") != report {
		t.Error("chart without injectable workloads must be unchanged")
	}

	if _, err := ApplyFeatures(charts, []string{"vault-agent"}, map[string]map[string]string{"vault-agent": {"kv-version": "3"}}, graph); err == nil {
		t.Error("expected error for kv-version 3")
	}
}

func TestFeatureIstioEgress(t *testing.T) {
	charts, graph := secTestCharts(t, types.OutputModeUniversal, secTestManifests)
	out := secApply(t, charts, graph, "istio-egress", map[string]string{"hosts": "*.googleapis.com"})[0]

	tpl := out.Templates["templates/istio-egress.yaml"]
	if !strings.Contains(tpl, "apiVersion: networking.istio.io/v1\nkind: ServiceEntry\n") || !strings.Contains(tpl, "location: MESH_EXTERNAL") {
		t.Errorf("istio-egress template:\n%s", tpl)
	}
	eg := secValues(t, out, "istioEgress")
	entries, _ := eg["serviceEntries"].([]interface{})
	got := map[string]interface{}{}
	for _, e := range entries {
		m := e.(map[string]interface{})
		got[m["host"].(string)] = m["ports"]
	}
	tls443 := []interface{}{map[string]interface{}{"name": "tls-443", "number": float64(443), "protocol": "TLS"}}
	want := map[string]interface{}{
		"*.googleapis.com":           tls443,
		"api.stripe.com":             tls443,
		"s3.eu-west-1.amazonaws.com": tls443,
		"smtp.example.org":           []interface{}{map[string]interface{}{"name": "tcp-587", "number": float64(587), "protocol": "TCP"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("serviceEntries =\n%v\nwant\n%v", got, want)
	}
	if !reflect.DeepEqual(eg["exportTo"], []interface{}{"."}) {
		t.Errorf("exportTo = %v", eg["exportTo"])
	}

	if _, err := ApplyFeatures(charts, []string{"istio-egress"}, map[string]map[string]string{"istio-egress": {"hosts": "bad host"}}, graph); err == nil {
		t.Error("expected error for an invalid host")
	}
}

// TestSecurityFeaturesCompose applies every feature of this file together in
// every mode: they must not collide on templates or values keys.
func TestSecurityFeaturesCompose(t *testing.T) {
	names := []string{"policies", "external-secrets", "vault-agent", "istio-egress"}
	for _, mode := range []types.OutputMode{types.OutputModeUniversal, types.OutputModeSeparate, types.OutputModeUmbrella} {
		charts, graph := secTestCharts(t, mode, secTestManifests)
		out, err := ApplyFeatures(charts, names, nil, graph)
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		for _, c := range out {
			var values map[string]interface{}
			if err := yaml.Unmarshal([]byte(c.ValuesYAML), &values); err != nil {
				t.Errorf("%s/%s: values.yaml does not parse: %v", mode, c.Name, err)
			}
		}
	}
}
