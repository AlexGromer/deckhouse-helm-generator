package generator

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// Operations and reliability features: scheduling spread, config rollouts,
// TLS, backups and an analysis report. See `dhg features`.
func init() {
	RegisterFeature(Feature{
		Name: "anti-affinity",
		Description: "Spread Deployment/StatefulSet replicas across nodes with podAntiAffinity " +
			"(values: antiAffinity.*; skipped for workloads that already set affinity)",
		Params: map[string]string{
			"mode":         string(AffinityModePreferred),
			"topology-key": defaultAntiAffinityTopologyKey,
			"zone-spread":  "false",
		},
		Apply: applyAntiAffinityFeature,
	})
	RegisterFeature(Feature{
		Name: "config-checksums",
		Description: "Roll workloads when their ConfigMaps/Secrets change via checksum/* pod annotations " +
			"(values: configChecksums.enabled)",
		Params: map[string]string{},
		Apply:  applyConfigChecksumsFeature,
	})
	RegisterFeature(Feature{
		Name: "ingress-tls",
		Description: "Enable TLS on Ingresses without a tls section: a cert-manager issuer annotation and " +
			"a per-Ingress certificate secret (values: ingressTLS.*)",
		Params: map[string]string{
			"issuer":      "letsencrypt-prod",
			"issuer-kind": "ClusterIssuer",
		},
		Apply: applyIngressTLSFeature,
	})
	RegisterFeature(Feature{
		Name: "velero-backup",
		Description: "Add a Velero Schedule (velero.io/v1) backing up the release to charts with " +
			"persistent volumes (values: veleroBackup.*)",
		Params: map[string]string{
			"schedule":                     defaultVeleroSchedule,
			"ttl":                          defaultVeleroTTL,
			"namespace":                    defaultVeleroNamespace,
			"storage-location":             "",
			"volume-snapshot-locations":    "",
			"snapshot-volumes":             "true",
			"default-volumes-to-fs-backup": "false",
		},
		Apply: applyVeleroBackupFeature,
	})
	RegisterFeature(Feature{
		Name: "resource-report",
		Description: "Write docs/resource-report.md: estimated cost, request/limit right-sizing findings and " +
			"persistent volume findings for the chart's workloads",
		Params: map[string]string{
			"provider":         string(CloudProviderAWS),
			"region":           "",
			"unit":             string(CostUnitMonthly),
			"overcommit-ratio": strconv.FormatFloat(OverprovisionThreshold, 'f', -1, 64),
			"include-batch":    "true",
		},
		Apply: applyResourceReportFeature,
	})
}

// ── anti-affinity ───────────────────────────────────────────────────────────

func applyAntiAffinityFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	opts := AntiAffinityOptions{
		Mode:        AffinityMode(fc.Param("mode")),
		TopologyKey: fc.Param("topology-key"),
		ZoneSpread:  fc.BoolParam("zone-spread"),
	}
	out, _, err := InjectAntiAffinity(chart, opts)
	return out, err
}

// ── config-checksums ────────────────────────────────────────────────────────

func applyConfigChecksumsFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	out, _, err := InjectConfigChecksums(chart, fc.Graph)
	return out, err
}

// ── ingress-tls ─────────────────────────────────────────────────────────────

func applyIngressTLSFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	out, _, err := InjectIngressTLS(chart, IngressTLSOptions{
		Issuer:     fc.Param("issuer"),
		IssuerKind: fc.Param("issuer-kind"),
	})
	return out, err
}

// ── velero-backup ───────────────────────────────────────────────────────────

func applyVeleroBackupFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	opts := VeleroBackupOptions{
		Namespace:                fc.Param("namespace"),
		Schedule:                 fc.Param("schedule"),
		TTL:                      fc.Param("ttl"),
		StorageLocation:          fc.Param("storage-location"),
		VolumeSnapshotLocations:  fc.ListParam("volume-snapshot-locations"),
		SnapshotVolumes:          fc.BoolParam("snapshot-volumes"),
		DefaultVolumesToFsBackup: fc.BoolParam("default-volumes-to-fs-backup"),
	}
	if err := opts.normalize(); err != nil {
		return nil, err
	}
	resources := opsChartResources(chart, fc.Graph)
	if !opsHasPersistentStorage(resources) {
		return chart, nil
	}
	opts.LabelSelectors = veleroLabelSelectors(resources)
	out, _, err := InjectVeleroBackup(chart, opts)
	return out, err
}

// veleroLabelSelectors returns, without duplicates, the pod selector of each
// workload (its pods and their PVCs) and the labels of each PVC.
func veleroLabelSelectors(resources []*types.ProcessedResource) []map[string]interface{} {
	var out []map[string]interface{}
	seen := map[string]bool{}
	add := func(sel map[string]interface{}) {
		if len(sel) == 0 {
			return
		}
		key, _ := json.Marshal(sel)
		if !seen[string(key)] {
			seen[string(key)] = true
			out = append(out, sel)
		}
	}
	for _, r := range resources {
		obj := r.Original.Object.Object
		switch r.Original.GVK.Kind {
		case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet":
			sel, _, _ := unstructured.NestedMap(obj, "spec", "selector")
			add(sel)
		case "PersistentVolumeClaim":
			if labels, _, _ := unstructured.NestedMap(obj, "metadata", "labels"); len(labels) > 0 {
				add(map[string]interface{}{"matchLabels": labels})
			}
		}
	}
	return out
}

// ── resource-report ─────────────────────────────────────────────────────────

func applyResourceReportFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	provider := CloudProvider(fc.Param("provider"))
	if _, ok := cloudPrices[provider]; !ok {
		return nil, fmt.Errorf("unsupported provider %q (want aws, gcp or azure)", provider)
	}
	unit := CostUnit(fc.Param("unit"))
	if unit != CostUnitMonthly && unit != CostUnitHourly {
		return nil, fmt.Errorf("unsupported unit %q (want monthly or hourly)", unit)
	}
	ratio, err := strconv.ParseFloat(fc.Param("overcommit-ratio"), 64)
	if err != nil || ratio <= 1 {
		return nil, fmt.Errorf("overcommit-ratio must be a number greater than 1, got %q", fc.Param("overcommit-ratio"))
	}

	resources := opsChartResources(chart, fc.Graph)
	if !opsHasWorkloadOrVolume(resources) {
		return chart, nil
	}
	sub := opsSubGraph(fc.Graph, resources)

	report := ResourceReport{
		ChartName: opsChartName(chart),
		Cost: GenerateCostEstimate(sub, CostEstimateOptions{
			Provider:       provider,
			Region:         fc.Param("region"),
			Unit:           unit,
			IncludeStorage: true,
		}),
		RightSizing: AnalyzeRightSizing(sub, RightSizingOptions{
			OverprovisionThreshold: ratio,
			IncludeBatchWorkloads:  fc.BoolParam("include-batch"),
		}),
		Volumes: AnalyzePVBestPractices(sub, DefaultPVAnalysisOptions()),
	}

	out := cloneChart(chart)
	for _, f := range out.ExternalFiles {
		if f.Path == ResourceReportPath {
			return nil, fmt.Errorf("%s already exists", ResourceReportPath)
		}
	}
	out.ExternalFiles = append(out.ExternalFiles, types.ExternalFileInfo{
		Path:    ResourceReportPath,
		Content: report.Markdown(),
	})
	return out, nil
}

// ResourceReportPath is where the resource-report feature writes its report,
// relative to the chart directory.
const ResourceReportPath = "docs/resource-report.md"

// ResourceReport is the operations report written by the resource-report
// feature.
type ResourceReport struct {
	ChartName   string
	Cost        *CostEstimateReport
	RightSizing *RightSizingReport
	Volumes     *PVAnalysisReport
}

// Markdown renders the whole report.
func (r ResourceReport) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Resource report: %s\n\n", r.ChartName)
	b.WriteString("Generated by dhg from the input manifests (the chart's default values). " +
		"Regenerate after changing requests, limits, replicas or volumes.\n\n")
	b.WriteString(r.Cost.Markdown())
	b.WriteString("\n")
	b.WriteString(r.RightSizing.Markdown())
	b.WriteString("\n")
	b.WriteString(r.Volumes.Markdown())
	return b.String()
}

// ── shared helpers ──────────────────────────────────────────────────────────

// opsChartResources returns the graph resources rendered by chart, i.e. those
// whose template is part of the chart, sorted by resource key. This works the
// same in every output mode: a universal chart holds every resource, a
// separate or umbrella subchart only those of its service group.
func opsChartResources(chart *types.GeneratedChart, graph *types.ResourceGraph) []*types.ProcessedResource {
	if chart == nil || graph == nil {
		return nil
	}
	var out []*types.ProcessedResource
	for _, r := range graph.Resources {
		if r == nil || r.Original == nil || r.Original.Object == nil || r.TemplatePath == "" {
			continue
		}
		if _, ok := chart.Templates[r.TemplatePath]; ok {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Original.ResourceKey().String() < out[j].Original.ResourceKey().String()
	})
	return out
}

// opsSubGraph returns a graph holding only the given resources plus every
// relationship of the full graph (lookups of resources outside the subset
// simply miss).
func opsSubGraph(graph *types.ResourceGraph, resources []*types.ProcessedResource) *types.ResourceGraph {
	sub := types.NewResourceGraph()
	for _, r := range resources {
		sub.AddResource(r)
	}
	if graph != nil {
		for _, rel := range graph.Relationships {
			sub.AddRelationship(rel)
		}
	}
	return sub
}

func opsHasWorkloadOrVolume(resources []*types.ProcessedResource) bool {
	for _, r := range resources {
		if kind := r.Original.GVK.Kind; isWorkloadKind(kind) || kind == "PersistentVolumeClaim" {
			return true
		}
	}
	return false
}

// opsHasPersistentStorage reports whether the resources include a
// PersistentVolumeClaim or a StatefulSet with volumeClaimTemplates.
func opsHasPersistentStorage(resources []*types.ProcessedResource) bool {
	for _, r := range resources {
		switch r.Original.GVK.Kind {
		case "PersistentVolumeClaim":
			return true
		case "StatefulSet":
			if hasVolumeClaimTemplates(r) {
				return true
			}
		}
	}
	return false
}

var (
	opsHelperDefineRe  = regexp.MustCompile(`define "([^"]+)\.fullname"`)
	opsHelperIncludeRe = regexp.MustCompile(`include "([^"]+)\.fullname"`)
)

// opsHelperPrefix returns the name prefix of the chart's template helpers
// ("<prefix>.fullname", "<prefix>.labels", ...). Charts generated in separate
// and umbrella mode name their helpers after themselves, not after
// --chart-name, so the prefix is read from the chart rather than assumed.
func opsHelperPrefix(chart *types.GeneratedChart) string {
	if m := opsHelperDefineRe.FindStringSubmatch(chart.Helpers); m != nil {
		return m[1]
	}
	for _, p := range opsSortedTemplatePaths(chart) {
		if m := opsHelperIncludeRe.FindStringSubmatch(chart.Templates[p]); m != nil {
			return m[1]
		}
	}
	return opsChartName(chart)
}

// opsChartName returns the name declared in Chart.yaml, falling back to
// the last path element of chart.Name.
func opsChartName(chart *types.GeneratedChart) string {
	var meta struct {
		Name string `json:"name"`
	}
	if err := yaml.Unmarshal([]byte(chart.ChartYAML), &meta); err == nil && meta.Name != "" {
		return meta.Name
	}
	name := chart.Name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return name
}

// opsSortedTemplatePaths returns the chart's template paths in sorted order.
func opsSortedTemplatePaths(chart *types.GeneratedChart) []string {
	paths := make([]string, 0, len(chart.Templates))
	for p := range chart.Templates {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// opsTemplateKind returns the kind declared at the top level of a template
// ("kind: X" at column 0), or "" if there is none.
func opsTemplateKind(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "kind: ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "kind: "))
		}
	}
	return ""
}

// opsIndentWidth returns the number of leading spaces of line.
func opsIndentWidth(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

// opsValidateDuration checks a Go duration string (as used by Velero's ttl).
func opsValidateDuration(s string) error {
	if _, err := time.ParseDuration(s); err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	return nil
}
