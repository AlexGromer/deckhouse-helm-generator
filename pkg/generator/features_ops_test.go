package generator

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/analyzer"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/analyzer/detector"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor/k8s"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor/value"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// opsTestManifests is a small application: a web Deployment reading a
// ConfigMap and exposed through an Ingress without TLS, and a database
// StatefulSet with a volumeClaimTemplate.
const opsTestManifests = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: web-config
  namespace: prod
  labels:
    app.kubernetes.io/name: web
data:
  mode: production
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: prod
  labels:
    app.kubernetes.io/name: web
spec:
  replicas: 3
  selector:
    matchLabels:
      app.kubernetes.io/name: web
  template:
    metadata:
      labels:
        app.kubernetes.io/name: web
    spec:
      containers:
        - name: web
          image: nginx:1.25
          envFrom:
            - configMapRef:
                name: web-config
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
            limits:
              cpu: "1"
              memory: 256Mi
---
apiVersion: v1
kind: Service
metadata:
  name: web
  namespace: prod
  labels:
    app.kubernetes.io/name: web
spec:
  selector:
    app.kubernetes.io/name: web
  ports:
    - name: http
      port: 80
      targetPort: 8080
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web
  namespace: prod
  labels:
    app.kubernetes.io/name: web
spec:
  ingressClassName: nginx
  rules:
    - host: web.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: web
                port:
                  number: 80
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: db
  namespace: prod
  labels:
    app.kubernetes.io/name: db
spec:
  serviceName: db
  replicas: 2
  selector:
    matchLabels:
      app.kubernetes.io/name: db
  template:
    metadata:
      labels:
        app.kubernetes.io/name: db
    spec:
      containers:
        - name: postgres
          image: postgres:16
          resources:
            requests:
              cpu: 500m
              memory: 1Gi
  volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        accessModes: ["ReadWriteOnce"]
        resources:
          requests:
            storage: 10Gi
`

// generateOpsCharts runs the real processing pipeline (processors, analyzer,
// generator) on opsTestManifests in the given mode.
func generateOpsCharts(t *testing.T, mode types.OutputMode) ([]*types.GeneratedChart, *types.ResourceGraph) {
	t.Helper()
	return generateChartsFromManifests(t, opsTestManifests, mode)
}

// generateChartsFromManifests runs the processing, analysis and generation
// steps of `dhg generate` on a YAML stream.
func generateChartsFromManifests(t *testing.T, manifests string, mode types.OutputMode) ([]*types.GeneratedChart, *types.ResourceGraph) {
	t.Helper()
	return generateChartsWithOptions(t, manifests, Options{Mode: mode})
}

// generateChartsWithOptions is generateChartsFromManifests with generator
// options; chart name, versions and the file manager are filled in.
func generateChartsWithOptions(t *testing.T, manifests string, opts Options) ([]*types.GeneratedChart, *types.ResourceGraph) {
	t.Helper()
	ctx := context.Background()
	mode := opts.Mode

	var extracted []*types.ExtractedResource
	all := map[types.ResourceKey]*types.ExtractedResource{}
	for _, doc := range strings.Split(manifests, "\n---\n") {
		if strings.TrimSpace(doc) == "" {
			continue
		}
		obj := &unstructured.Unstructured{}
		if err := yaml.Unmarshal([]byte(doc), &obj.Object); err != nil {
			t.Fatalf("parsing manifest: %v", err)
		}
		if obj.GetKind() == "" {
			continue // comments only
		}
		r := &types.ExtractedResource{Object: obj, Source: types.SourceFile, GVK: obj.GroupVersionKind()}
		extracted = append(extracted, r)
		all[r.ResourceKey()] = r
	}

	registry := processor.NewRegistry()
	k8s.RegisterAll(registry)
	files := value.NewExternalFileManager()
	var processed []*types.ProcessedResource
	for _, r := range extracted {
		result, err := registry.Process(processor.Context{
			Ctx:                 ctx,
			ChartName:           "app",
			OutputMode:          mode,
			Namespace:           r.Object.GetNamespace(),
			AllResources:        all,
			ExternalFileManager: files,
			ValueProcessor:      value.DefaultProcessor(),
		}, r.Object)
		if err != nil {
			t.Fatalf("processing %s: %v", r.ResourceKey(), err)
		}
		processed = append(processed, &types.ProcessedResource{
			Original:        r,
			ServiceName:     result.ServiceName,
			TemplatePath:    result.TemplatePath,
			TemplateContent: result.TemplateContent,
			ValuesPath:      result.ValuesPath,
			Values:          result.Values,
			Dependencies:    result.Dependencies,
		})
	}

	processor.ResolveCollisions(processed)

	a := analyzer.NewDefaultAnalyzer()
	detector.RegisterAll(a)
	graph, err := a.Analyze(ctx, processed)
	if err != nil {
		t.Fatal(err)
	}
	gen, err := DefaultRegistry().Get(mode)
	if err != nil {
		t.Fatal(err)
	}
	opts.ChartName, opts.ChartVersion, opts.AppVersion = "app", "0.1.0", "1.0.0"
	opts.ExternalFileManager = files
	charts, err := gen.Generate(ctx, graph, opts)
	if err != nil {
		t.Fatal(err)
	}
	return charts, graph
}

var opsModes = []types.OutputMode{types.OutputModeUniversal, types.OutputModeSeparate, types.OutputModeUmbrella}

// applyOps applies the named features with the given options.
func applyOps(t *testing.T, mode types.OutputMode, names []string, options map[string]map[string]string) []*types.GeneratedChart {
	t.Helper()
	charts, graph := generateOpsCharts(t, mode)
	out, err := ApplyFeatures(charts, names, options, graph)
	if err != nil {
		t.Fatalf("ApplyFeatures(%v) in %s mode: %v", names, mode, err)
	}
	return out
}

// templatesOfKind returns the chart's template paths whose top-level kind is kind.
func templatesOfKind(chart *types.GeneratedChart, kind string) []string {
	var out []string
	for _, p := range opsSortedTemplatePaths(chart) {
		if opsTemplateKind(chart.Templates[p]) == kind {
			out = append(out, p)
		}
	}
	return out
}

func externalFile(chart *types.GeneratedChart, path string) (string, bool) {
	for _, f := range chart.ExternalFiles {
		if f.Path == path {
			return f.Content, true
		}
	}
	return "", false
}

func TestOpsFeaturesRegistered(t *testing.T) {
	for _, name := range []string{"anti-affinity", "config-checksums", "ingress-tls", "velero-backup", "resource-report"} {
		if _, ok := LookupFeature(name); !ok {
			t.Errorf("feature %q is not registered", name)
		}
	}
}

func TestAntiAffinityFeature(t *testing.T) {
	for _, mode := range opsModes {
		t.Run(string(mode), func(t *testing.T) {
			charts := applyOps(t, mode, []string{"anti-affinity"}, map[string]map[string]string{
				"anti-affinity": {"mode": "required", "zone-spread": "true"},
			})
			workloads := 0
			for _, chart := range charts {
				paths := append(templatesOfKind(chart, "Deployment"), templatesOfKind(chart, "StatefulSet")...)
				for _, p := range paths {
					workloads++
					tpl := chart.Templates[p]
					if !strings.Contains(tpl, "podAntiAffinity:") || !strings.Contains(tpl, "topologySpreadConstraints:") {
						t.Errorf("%s/%s: anti-affinity not injected:\n%s", chart.Name, p, tpl)
					}
					// The selector is copied from spec.selector and re-indented
					// for each place it is used.
					for _, n := range []string{"16", "18", "12"} {
						want := `{{- toYaml .selector | nindent ` + n + ` }}`
						if !strings.Contains(tpl, want) {
							t.Errorf("%s/%s: missing %q", chart.Name, p, want)
						}
					}
					if strings.Index(tpl, "podAntiAffinity:") > strings.Index(tpl, "      containers:") {
						t.Errorf("%s/%s: affinity must be part of the pod spec, before containers", chart.Name, p)
					}
				}
				if len(paths) > 0 {
					aa, _ := valuesOf(t, chart)["antiAffinity"].(map[string]interface{})
					if aa["enabled"] != true || aa["mode"] != "required" || aa["topologyKey"] != defaultAntiAffinityTopologyKey {
						t.Errorf("%s: unexpected antiAffinity values: %v", chart.Name, aa)
					}
					if zs, _ := aa["zoneSpread"].(map[string]interface{}); zs["enabled"] != true {
						t.Errorf("%s: zoneSpread not enabled: %v", chart.Name, aa)
					}
				} else if _, ok := valuesOf(t, chart)["antiAffinity"]; ok {
					t.Errorf("%s: values added to a chart without workloads", chart.Name)
				}
			}
			if workloads != 2 {
				t.Errorf("expected 2 workload templates, got %d", workloads)
			}
		})
	}

	charts, graph := generateOpsCharts(t, types.OutputModeUniversal)
	if _, err := ApplyFeatures(charts, []string{"anti-affinity"}, map[string]map[string]string{"anti-affinity": {"mode": "always"}}, graph); err == nil {
		t.Error("expected an error for an invalid mode")
	}
}

func TestConfigChecksumsFeature(t *testing.T) {
	for _, mode := range opsModes {
		t.Run(string(mode), func(t *testing.T) {
			charts := applyOps(t, mode, []string{"config-checksums"}, nil)
			found := false
			for _, chart := range charts {
				for _, p := range templatesOfKind(chart, "Deployment") {
					tpl := chart.Templates[p]
					cm := templatesOfKind(chart, "ConfigMap")
					if len(cm) != 1 {
						t.Fatalf("%s: expected one ConfigMap template, got %v", chart.Name, cm)
					}
					want := `checksum/configmap-web-config: {{ include (print $.Template.BasePath "/` +
						strings.TrimPrefix(cm[0], "templates/") + `") $ | sha256sum | quote }}`
					if !strings.Contains(tpl, want) {
						t.Errorf("%s/%s: missing %q in:\n%s", chart.Name, p, want, tpl)
					}
					if !strings.Contains(tpl, "{{- with .podAnnotations }}") {
						t.Errorf("%s/%s: the workload's own podAnnotations must still render", chart.Name, p)
					}
					found = true
					if cc, _ := valuesOf(t, chart)["configChecksums"].(map[string]interface{}); cc["enabled"] != true {
						t.Errorf("%s: configChecksums.enabled not set", chart.Name)
					}
				}
				for _, p := range templatesOfKind(chart, "StatefulSet") {
					if strings.Contains(chart.Templates[p], "checksum/") {
						t.Errorf("%s/%s: StatefulSet without config references must not change", chart.Name, p)
					}
				}
			}
			if !found {
				t.Error("no Deployment template found")
			}
		})
	}
}

func TestIngressTLSFeature(t *testing.T) {
	for _, mode := range opsModes {
		t.Run(string(mode), func(t *testing.T) {
			charts := applyOps(t, mode, []string{"ingress-tls"}, map[string]map[string]string{
				"ingress-tls": {"issuer": "internal-ca", "issuer-kind": "Issuer"},
			})
			found := false
			for _, chart := range charts {
				for _, p := range templatesOfKind(chart, "Ingress") {
					found = true
					tpl := chart.Templates[p]
					name := ingressNameRe.FindStringSubmatch(tpl)[1]
					for _, want := range []string{
						"secretName: " + name + "-tls",
						`"cert-manager.io/issuer"`,
						"{{- if $dhgAutoTLS }}",
						"{{- if .tls }}", // the template's own tls block is kept
					} {
						if !strings.Contains(tpl, want) {
							t.Errorf("%s/%s: missing %q in:\n%s", chart.Name, p, want, tpl)
						}
					}
					tls, _ := valuesOf(t, chart)["ingressTLS"].(map[string]interface{})
					if tls["issuer"] != "internal-ca" || tls["issuerKind"] != "Issuer" || tls["enabled"] != true {
						t.Errorf("%s: unexpected ingressTLS values %v", chart.Name, tls)
					}
				}
			}
			if !found {
				t.Error("no Ingress template found")
			}
		})
	}

	charts, graph := generateOpsCharts(t, types.OutputModeUniversal)
	if _, err := ApplyFeatures(charts, []string{"ingress-tls"}, map[string]map[string]string{"ingress-tls": {"issuer-kind": "Vault"}}, graph); err == nil {
		t.Error("expected an error for an invalid issuer kind")
	}
}

func TestVeleroBackupFeature(t *testing.T) {
	for _, mode := range opsModes {
		t.Run(string(mode), func(t *testing.T) {
			charts := applyOps(t, mode, []string{"velero-backup"}, map[string]map[string]string{
				"velero-backup": {"schedule": "30 1 * * *", "volume-snapshot-locations": "aws-default, aws-dr"},
			})
			withSchedule := 0
			for _, chart := range charts {
				tpl, ok := chart.Templates[VeleroScheduleTemplatePath]
				hasDB := len(templatesOfKind(chart, "StatefulSet")) > 0
				if ok != hasDB {
					t.Errorf("%s: velero schedule present=%v, but chart has persistent storage=%v", chart.Name, ok, hasDB)
				}
				if !ok {
					continue
				}
				withSchedule++
				prefix := opsHelperPrefix(chart)
				for _, want := range []string{"apiVersion: velero.io/v1", "kind: Schedule", `include "` + prefix + `.selectorLabels" .`} {
					if !strings.Contains(tpl, want) {
						t.Errorf("%s: missing %q in:\n%s", chart.Name, want, tpl)
					}
				}
				v, _ := valuesOf(t, chart)["veleroBackup"].(map[string]interface{})
				if v["schedule"] != "30 1 * * *" || v["namespace"] != defaultVeleroNamespace || v["snapshotVolumes"] != true {
					t.Errorf("%s: unexpected veleroBackup values %v", chart.Name, v)
				}
				if locs, _ := v["volumeSnapshotLocations"].([]interface{}); len(locs) != 2 || locs[1] != "aws-dr" {
					t.Errorf("%s: volumeSnapshotLocations = %v", chart.Name, v["volumeSnapshotLocations"])
				}
				// The StatefulSet's selector picks up its pods and the PVCs of
				// its volumeClaimTemplates, which lack the chart's labels.
				if sels, _ := v["labelSelectors"].([]interface{}); len(sels) == 0 {
					t.Errorf("%s: no labelSelectors for the release's workloads: %v", chart.Name, v)
				} else if _, ok := sels[0].(map[string]interface{})["matchLabels"]; !ok {
					t.Errorf("%s: labelSelectors[0] = %v, want a workload selector", chart.Name, sels[0])
				}
			}
			if withSchedule != 1 {
				t.Errorf("expected exactly one chart with a velero schedule, got %d", withSchedule)
			}
		})
	}

	charts, graph := generateOpsCharts(t, types.OutputModeUniversal)
	if _, err := ApplyFeatures(charts, []string{"velero-backup"}, map[string]map[string]string{"velero-backup": {"ttl": "30 days"}}, graph); err == nil {
		t.Error("expected an error for an invalid ttl")
	}
}

func TestResourceReportFeature(t *testing.T) {
	charts := applyOps(t, types.OutputModeUniversal, []string{"resource-report"}, map[string]map[string]string{
		"resource-report": {"provider": "gcp"},
	})
	report, ok := externalFile(charts[0], ResourceReportPath)
	if !ok {
		t.Fatalf("%s not written", ResourceReportPath)
	}
	for _, want := range []string{
		"# Resource report: app",
		"GCP (us-central1)",
		"| Deployment/web | 3 | 100m | 128Mi |",
		"| StatefulSet/db | 2 | 500m | 1024Mi |",
		"StatefulSet/db volumeClaimTemplate data | 20.0 GiB",
		"web: CPU limit 1 is 10.0x the request 100m",
		"postgres: no memory limit",
		"## Persistent volumes",
		"| INFO | db/data | missing-storage-class: volumeClaimTemplate data of StatefulSet db has no storageClassName",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report misses %q:\n%s", want, report)
		}
	}
	if strings.Contains(charts[0].Notes, "Resource report") {
		t.Error("the report must not go into NOTES.txt")
	}

	// In separate mode every chart reports only its own workloads.
	for _, chart := range applyOps(t, types.OutputModeSeparate, []string{"resource-report"}, nil) {
		report, ok := externalFile(chart, ResourceReportPath)
		if !ok {
			t.Errorf("%s: no report", chart.Name)
			continue
		}
		hasWeb := strings.Contains(report, "Deployment/web")
		hasDB := strings.Contains(report, "StatefulSet/db")
		if hasWeb == hasDB {
			t.Errorf("%s: expected the report of exactly one workload:\n%s", chart.Name, report)
		}
	}

	charts, graph := generateOpsCharts(t, types.OutputModeUniversal)
	for _, opts := range []map[string]string{{"provider": "oracle"}, {"unit": "yearly"}, {"overcommit-ratio": "0.5"}} {
		if _, err := ApplyFeatures(charts, []string{"resource-report"}, map[string]map[string]string{"resource-report": opts}, graph); err == nil {
			t.Errorf("expected an error for %v", opts)
		}
	}
}

// TestOpsFeaturesTogether applies every feature of this file in one run: the
// features must compose (distinct template paths and values keys) and keep
// values.yaml valid.
func TestOpsFeaturesTogether(t *testing.T) {
	names := []string{"anti-affinity", "config-checksums", "ingress-tls", "velero-backup", "resource-report"}
	for _, mode := range opsModes {
		t.Run(string(mode), func(t *testing.T) {
			before, _ := generateOpsCharts(t, mode)
			after := applyOps(t, mode, names, nil)
			if len(before) != len(after) {
				t.Fatalf("chart count changed: %d -> %d", len(before), len(after))
			}
			beforeByName := map[string]*types.GeneratedChart{}
			for _, c := range before {
				beforeByName[c.Name] = c
			}
			keys := map[string]bool{}
			for _, chart := range after {
				for p := range beforeByName[chart.Name].Templates {
					if _, ok := chart.Templates[p]; !ok {
						t.Errorf("%s: template %s disappeared", chart.Name, p)
					}
				}
				for k := range valuesOf(t, chart) {
					keys[k] = true
				}
			}
			for _, k := range []string{"antiAffinity", "configChecksums", "ingressTLS", "veleroBackup"} {
				if !keys[k] {
					t.Errorf("values key %s missing", k)
				}
			}
		})
	}
}
