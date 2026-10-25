package generator

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/compliance/pss"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

func init() {
	RegisterFeature(Feature{
		Name:        "deckhouse",
		Description: "generates ## Deckhouse Integration section in SYNTHESIS.md (list it last in --with)",
		Params:      map[string]string{"pss-profile": "restricted"},
		Apply:       applyDeckhouseFeature,
	})
	RegisterFeature(Feature{
		Name:        "pss-restricted",
		Description: "sets global.securityContextDefaults for restricted Pod Security Standards compliance",
		Params:      map[string]string{},
		Apply:       applyPSSRestrictedFeature,
	})
}

func applyPSSRestrictedFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	hasWorkloads := false
	workloads := opsChartResources(chart, fc.Graph)
	for _, w := range workloads {
		if _, _, _, ok := pss.PodSpec(w.Original.Object); ok {
			hasWorkloads = true
			break
		}
	}
	if !hasWorkloads {
		return chart, nil
	}

	podSC := map[string]interface{}{
		"seccompProfile": map[string]interface{}{
			"type": "RuntimeDefault",
		},
	}
	containerSC := map[string]interface{}{
		"allowPrivilegeEscalation": false,
		"capabilities": map[string]interface{}{
			"drop": []interface{}{"ALL"},
		},
	}
	defaults := map[string]interface{}{
		"pod":       podSC,
		"container": containerSC,
	}

	newValuesYAML, err := setGlobalValue(chart.ValuesYAML, "securityContextDefaults", defaults)
	if err != nil {
		if strings.Contains(err.Error(), "already has global.securityContextDefaults") {
			return nil, fmt.Errorf("pss-restricted: global.securityContextDefaults is already set")
		}
		return nil, err
	}

	out := cloneChart(chart)
	out.ValuesYAML = newValuesYAML
	return out, nil
}

func applyDeckhouseFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	profileStr := fc.Param("pss-profile")
	targetProfile, err := pss.ParseProfile(profileStr)
	if err != nil {
		return nil, fmt.Errorf("deckhouse: pss-profile must be baseline or restricted, got %q", profileStr)
	}

	report, err := buildDeckhouseReport(chart, fc, targetProfile)
	if err != nil {
		return nil, err
	}

	out := cloneChart(chart)

	// Check if SYNTHESIS.md exists, if so append, else create
	synthesisPath := "SYNTHESIS.md"
	existing := ""
	for _, ext := range out.ExternalFiles {
		if ext.Path == synthesisPath {
			existing = ext.Content
			break
		}
	}

	markdown := report.Markdown()

	if existing != "" {
		if !strings.HasSuffix(existing, "\n") {
			existing += "\n\n"
		} else {
			existing += "\n"
		}
		existing += markdown
	} else {
		existing = markdown
	}

	// Update or add SYNTHESIS.md
	found := false
	for i, ext := range out.ExternalFiles {
		if ext.Path == synthesisPath {
			out.ExternalFiles[i].Content = existing
			found = true
			break
		}
	}
	if !found {
		out.ExternalFiles = append(out.ExternalFiles, types.ExternalFileInfo{
			Path:    synthesisPath,
			Content: existing,
		})
	}

	return out, nil
}

type DeckhouseReport struct {
	TargetProfile pss.Profile
	Workloads     []WorkloadPSS
	NSLabels      []string
	Monitoring    []string
	Platform      []PlatformObject
}

type WorkloadPSS struct {
	Kind     string
	Name     string
	Profile  pss.Profile
	Findings []pss.Finding
}

type PlatformObject struct {
	Kind string
	Name string
	Desc string
}

func buildDeckhouseReport(chart *types.GeneratedChart, fc FeatureContext, target pss.Profile) (DeckhouseReport, error) {
	rep := DeckhouseReport{TargetProfile: target}

	// 1. Pod Security Standards
	minProfile := pss.ProfileRestricted
	hasWorkloads := false

	workloads := opsChartResources(chart, fc.Graph)
	for _, w := range workloads {
		_, _, _, ok := pss.PodSpec(w.Original.Object)
		if !ok {
			continue
		}
		hasWorkloads = true

		res := pss.EvaluateWorkload(w.Original.Object, target)

		// Update minProfile correctly: Privileged < Baseline < Restricted
		if res.Profile == pss.ProfilePrivileged {
			minProfile = pss.ProfilePrivileged
		} else if res.Profile == pss.ProfileBaseline && minProfile == pss.ProfileRestricted {
			minProfile = pss.ProfileBaseline
		}

		rep.Workloads = append(rep.Workloads, WorkloadPSS{
			Kind:     w.Original.Object.GetKind(),
			Name:     w.Original.Object.GetName(),
			Profile:  res.Profile,
			Findings: res.Findings,
		})
	}

	sort.Slice(rep.Workloads, func(i, j int) bool {
		if rep.Workloads[i].Kind != rep.Workloads[j].Kind {
			return rep.Workloads[i].Kind < rep.Workloads[j].Kind
		}
		return rep.Workloads[i].Name < rep.Workloads[j].Name
	})

	if hasWorkloads {
		rep.NSLabels = append(rep.NSLabels, fmt.Sprintf("security.deckhouse.io/pod-policy: %s", minProfile))
	}

	// 2. Observability (Prometheus)
	hasServiceMonitor := false
	hasPodMonitor := false
	hasPrometheusRule := false
	hasScrapeConfig := false
	hasProbe := false

	// Gather objects from chart templates and graph
	var monitoringObjs []*unstructured.Unstructured

	// Add graph objects
	if fc.Graph != nil {
		for _, r := range fc.Graph.Resources {
			if r != nil && r.Original != nil && r.Original.Object != nil {
				kind := r.Original.GVK.Kind
				if kind == "ServiceMonitor" || kind == "PodMonitor" || kind == "PrometheusRule" || kind == "ScrapeConfig" || kind == "Probe" {
					monitoringObjs = append(monitoringObjs, r.Original.Object)
				}
			}
		}
	}

	// Add templates
	var values map[string]interface{}
	_ = yaml.Unmarshal([]byte(chart.ValuesYAML), &values)

	for _, content := range chart.Templates {
		// Try to parse just to see what kind it is
		// This is a simplification; a full implementation would parse all resources
		if strings.Contains(content, "kind: ServiceMonitor") {
			hasServiceMonitor = true
		}
		if strings.Contains(content, "kind: PodMonitor") {
			hasPodMonitor = true
		}
		if strings.Contains(content, "kind: PrometheusRule") {
			hasPrometheusRule = true

			// Check rule labels from values
			ruleLabels := getNestedMapStringFromValues(values, "prometheusRules", "labels")
			if ruleLabels == nil || ruleLabels["prometheus"] != "main" || ruleLabels["component"] != "rules" {
				rep.Monitoring = append(rep.Monitoring, "PrometheusRule (from templates) is missing 'prometheus: main' or 'component: rules'")
			}
		}
		if strings.Contains(content, "kind: ScrapeConfig") {
			hasScrapeConfig = true
		}
		if strings.Contains(content, "kind: Probe") {
			hasProbe = true
		}
	}

	for _, obj := range monitoringObjs {
		kind := obj.GetKind()
		name := obj.GetName()
		labels := obj.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}

		if kind == "ServiceMonitor" {
			hasServiceMonitor = true
			if labels["prometheus"] != "main" {
				rep.Monitoring = append(rep.Monitoring, fmt.Sprintf("ServiceMonitor/%s is missing 'prometheus: main'", name))
			}
		}
		if kind == "PodMonitor" {
			hasPodMonitor = true
			if labels["prometheus"] != "main" {
				rep.Monitoring = append(rep.Monitoring, fmt.Sprintf("PodMonitor/%s is missing 'prometheus: main'", name))
			}
		}
		if kind == "PrometheusRule" {
			hasPrometheusRule = true
			if labels["prometheus"] != "main" || labels["component"] != "rules" {
				rep.Monitoring = append(rep.Monitoring, fmt.Sprintf("PrometheusRule/%s is missing 'prometheus: main' or 'component: rules'", name))
			}
		}
		if kind == "ScrapeConfig" {
			hasScrapeConfig = true
			if labels["prometheus"] != "main" {
				rep.Monitoring = append(rep.Monitoring, fmt.Sprintf("ScrapeConfig/%s is missing 'prometheus: main'", name))
			}
		}
		if kind == "Probe" {
			hasProbe = true
			if labels["prometheus"] != "main" {
				rep.Monitoring = append(rep.Monitoring, fmt.Sprintf("Probe/%s is missing 'prometheus: main'", name))
			}
		}
	}

	if hasServiceMonitor || hasPodMonitor {
		rep.NSLabels = append(rep.NSLabels, "prometheus.deckhouse.io/monitor-watcher-enabled: \"true\"")
	}
	if hasPrometheusRule {
		rep.NSLabels = append(rep.NSLabels, "prometheus.deckhouse.io/rules-watcher-enabled: \"true\"")
	}
	if hasScrapeConfig {
		rep.NSLabels = append(rep.NSLabels, "prometheus.deckhouse.io/scrape-configs-watcher-enabled: \"true\"")
	}
	if hasProbe {
		rep.NSLabels = append(rep.NSLabels, "prometheus.deckhouse.io/probe-watcher-enabled: \"true\"")
	}

	// 3. Platform Objects & Grafana ConfigMap
	if fc.Graph != nil {
		for _, r := range fc.Graph.Resources {
			if r == nil || r.Original == nil || r.Original.Object == nil {
				continue
			}
			obj := r.Original.Object
			kind := obj.GetKind()
			name := obj.GetName()

			if kind == "ConfigMap" {
				labels := obj.GetLabels()
				if labels != nil && labels["grafana_dashboard"] == "1" {
					rep.Platform = append(rep.Platform, PlatformObject{
						Kind: "ConfigMap",
						Name: name,
						Desc: "Grafana sidecar convention (kube-prometheus-stack); Deckhouse uses ObservabilityDashboard (module observability) or legacy GrafanaDashboardDefinition",
					})
				}
			}

			if strings.HasPrefix(obj.GroupVersionKind().Group, "deckhouse.io") {
				rep.Platform = append(rep.Platform, PlatformObject{
					Kind: kind,
					Name: name,
					Desc: "Deckhouse Custom Resource",
				})
			}
		}
	}

	sort.Strings(rep.NSLabels)
	sort.Strings(rep.Monitoring)
	sort.Slice(rep.Platform, func(i, j int) bool {
		if rep.Platform[i].Kind != rep.Platform[j].Kind {
			return rep.Platform[i].Kind < rep.Platform[j].Kind
		}
		return rep.Platform[i].Name < rep.Platform[j].Name
	})

	return rep, nil
}

func getNestedMapStringFromValues(values map[string]interface{}, fields ...string) map[string]string {
	if values == nil {
		return nil
	}

	var current interface{} = values
	for i, f := range fields {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current, ok = m[f]
		if !ok {
			return nil
		}

		if i == len(fields)-1 {
			if mapIntf, ok := current.(map[string]interface{}); ok {
				result := make(map[string]string)
				for k, v := range mapIntf {
					result[k] = fmt.Sprintf("%v", v)
				}
				return result
			}
		}
	}
	return nil
}

func (r DeckhouseReport) Markdown() string {
	tmpl := `## Deckhouse Integration

### Observability (prometheus / operator-prometheus)
{{ if or .NSLabels .Monitoring }}
**Namespace labels required:**
{{ range .NSLabels }}
- {{ . }}
{{- end }}

**Prometheus objects missing labels:**
{{ range .Monitoring }}
- {{ . }}
{{- end }}
{{ else }}
No findings.
{{ end }}

### Pod Security Standards ({{ .TargetProfile }})
{{ if .Workloads }}
| Workload | Profile | Findings |
|---|---|---|
{{- range .Workloads }}
| {{ .Kind }}/{{ .Name }} | {{ .Profile }} | {{ if .Findings }}{{ range $i, $f := .Findings }}{{ if $i }}; {{ end }}{{ $f.Control }}: {{ $f.Field }}{{ end }}{{ else }}None{{ end }} |
{{- end }}

*Use --with security-hardening and dhg fix to automatically resolve these issues.*
{{ else }}
No findings.
{{ end }}

### Deckhouse Custom Resources
{{ if .Platform }}
| Kind/Name | Description |
|---|---|
{{- range .Platform }}
| {{ .Kind }}/{{ .Name }} | {{ .Desc }} |
{{- end }}
{{ else }}
No findings.
{{ end }}
`
	var buf bytes.Buffer
	t := template.Must(template.New("deckhouse").Parse(tmpl))
	_ = t.Execute(&buf, r)
	return buf.String()
}
