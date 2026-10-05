package generator

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// AffinityMode controls whether anti-affinity rules are preferred or required.
type AffinityMode string

const (
	// AffinityModePreferred uses preferredDuringSchedulingIgnoredDuringExecution:
	// replicas are spread when possible but still schedule on a single node.
	AffinityModePreferred AffinityMode = "preferred"
	// AffinityModeRequired uses requiredDuringSchedulingIgnoredDuringExecution:
	// a replica stays Pending rather than share a topology domain.
	AffinityModeRequired AffinityMode = "required"
)

const (
	defaultAntiAffinityTopologyKey = "kubernetes.io/hostname"
	zoneTopologyKey                = "topology.kubernetes.io/zone"
	antiAffinityValuesKey          = "antiAffinity"
)

// AntiAffinityOptions configures the defaults written to values.yaml
// (antiAffinity.*). All of them can be changed at install time.
type AntiAffinityOptions struct {
	// Mode is "preferred" (default) or "required".
	Mode AffinityMode
	// TopologyKey is the node label replicas are spread across
	// (default kubernetes.io/hostname).
	TopologyKey string
	// ZoneSpread additionally adds a topologySpreadConstraint across
	// topology.kubernetes.io/zone.
	ZoneSpread bool
}

// AntiAffinityResult tracks the result of InjectAntiAffinity.
type AntiAffinityResult struct {
	// Injected lists the template paths that received the anti-affinity block.
	Injected []string
	// Skipped lists workload templates left unchanged (affinity already set,
	// or a template shape the injector does not recognise).
	Skipped []string
}

// InjectAntiAffinity adds a values-controlled podAntiAffinity (and an optional
// zone topologySpreadConstraint) to every Deployment and StatefulSet template
// of the chart. The label selector is copied from the workload's own
// spec.selector, so it matches exactly the workload's pods in every output
// mode. A workload whose values set `affinity` keeps it: the injected block
// renders only when `.affinity` is empty. Templates that already declare
// affinity or topology spread outside the generated values block are skipped.
//
// The chart is not modified; the values toggle `antiAffinity` is appended to
// values.yaml when at least one template was changed.
func InjectAntiAffinity(chart *types.GeneratedChart, opts AntiAffinityOptions) (*types.GeneratedChart, AntiAffinityResult, error) {
	var res AntiAffinityResult
	if chart == nil {
		return nil, res, nil
	}
	if opts.Mode == "" {
		opts.Mode = AffinityModePreferred
	}
	if opts.Mode != AffinityModePreferred && opts.Mode != AffinityModeRequired {
		return nil, res, fmt.Errorf("invalid anti-affinity mode %q (want preferred or required)", opts.Mode)
	}
	if opts.TopologyKey == "" {
		opts.TopologyKey = defaultAntiAffinityTopologyKey
	}

	out := cloneChart(chart)
	for _, path := range opsSortedTemplatePaths(chart) {
		content := chart.Templates[path]
		kind := opsTemplateKind(content)
		if kind != "Deployment" && kind != "StatefulSet" {
			continue
		}
		updated, ok := injectAntiAffinityBlock(content)
		if !ok {
			res.Skipped = append(res.Skipped, path)
			continue
		}
		out.Templates[path] = updated
		res.Injected = append(res.Injected, path)
	}
	if len(res.Injected) == 0 {
		return chart, res, nil
	}

	values, err := appendTopLevelValues(out.ValuesYAML, antiAffinityValuesKey, map[string]interface{}{
		"enabled":     true,
		"mode":        string(opts.Mode),
		"topologyKey": opts.TopologyKey,
		"weight":      100,
		"zoneSpread": map[string]interface{}{
			"enabled":           opts.ZoneSpread,
			"maxSkew":           1,
			"whenUnsatisfiable": "ScheduleAnyway",
		},
	})
	if err != nil {
		return nil, res, err
	}
	out.ValuesYAML = values
	return out, res, nil
}

// standardAffinityBlockRe matches the values-driven affinity block that the
// processors render; any other "affinity:" key means the template already
// carries hand-made affinity.
var standardAffinityBlockRe = regexp.MustCompile(`(?m)^ *\{\{- with \.affinity \}\}\n *affinity:\n *\{\{- toYaml \. \| nindent \d+ \}\}\n *\{\{- end \}\}\n`)

var nindentRe = regexp.MustCompile(`nindent (\d+)`)

// injectAntiAffinityBlock inserts the anti-affinity block in front of the pod
// spec's containers list. It returns false when the template already declares
// affinity/topology spread or its shape is not recognised.
func injectAntiAffinityBlock(content string) (string, bool) {
	rest := standardAffinityBlockRe.ReplaceAllString(content, "")
	if strings.Contains(rest, "affinity:") || strings.Contains(content, "topologySpreadConstraints") {
		return content, false
	}

	lines := strings.Split(content, "\n")
	selector, ok := extractSelectorMatchLabels(lines)
	if !ok {
		return content, false
	}
	idx := podContainersLine(lines)
	if idx < 0 {
		return content, false
	}
	ind := strings.Repeat(" ", opsIndentWidth(lines[idx]))
	n := opsIndentWidth(lines[idx])

	var b strings.Builder
	w := func(s string) { b.WriteString(ind + s + "\n") }
	w(`{{- $dhgAntiAffinity := $.Values.antiAffinity | default dict }}`)
	w(`{{- if and $dhgAntiAffinity.enabled (not .affinity) }}`)
	w(`affinity:`)
	w(`  podAntiAffinity:`)
	w(`    {{- if eq ($dhgAntiAffinity.mode | default "preferred") "required" }}`)
	w(`    requiredDuringSchedulingIgnoredDuringExecution:`)
	w(`      - topologyKey: {{ $dhgAntiAffinity.topologyKey | default "` + defaultAntiAffinityTopologyKey + `" | quote }}`)
	w(`        labelSelector:`)
	w(`          matchLabels:`)
	b.WriteString(reindentBlock(selector, n+12))
	w(`    {{- else }}`)
	w(`    preferredDuringSchedulingIgnoredDuringExecution:`)
	w(`      - weight: {{ $dhgAntiAffinity.weight | default 100 | int }}`)
	w(`        podAffinityTerm:`)
	w(`          topologyKey: {{ $dhgAntiAffinity.topologyKey | default "` + defaultAntiAffinityTopologyKey + `" | quote }}`)
	w(`          labelSelector:`)
	w(`            matchLabels:`)
	b.WriteString(reindentBlock(selector, n+14))
	w(`    {{- end }}`)
	w(`{{- end }}`)
	w(`{{- $dhgZoneSpread := $dhgAntiAffinity.zoneSpread | default dict }}`)
	w(`{{- if and $dhgAntiAffinity.enabled $dhgZoneSpread.enabled }}`)
	w(`topologySpreadConstraints:`)
	w(`  - maxSkew: {{ $dhgZoneSpread.maxSkew | default 1 | int }}`)
	w(`    topologyKey: ` + zoneTopologyKey)
	w(`    whenUnsatisfiable: {{ $dhgZoneSpread.whenUnsatisfiable | default "ScheduleAnyway" }}`)
	w(`    labelSelector:`)
	w(`      matchLabels:`)
	b.WriteString(reindentBlock(selector, n+8))
	w(`{{- end }}`)

	block := strings.TrimSuffix(b.String(), "\n")
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:idx]...)
	out = append(out, block)
	out = append(out, lines[idx:]...)
	return strings.Join(out, "\n"), true
}

// extractSelectorMatchLabels returns the lines under the workload's
// spec.selector.matchLabels (the first selector of the template).
func extractSelectorMatchLabels(lines []string) ([]string, bool) {
	for i := 0; i+1 < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "selector:" || strings.TrimSpace(lines[i+1]) != "matchLabels:" {
			continue
		}
		base := opsIndentWidth(lines[i+1])
		if base <= opsIndentWidth(lines[i]) {
			return nil, false
		}
		var body []string
		for j := i + 2; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == "" || opsIndentWidth(lines[j]) <= base {
				break
			}
			body = append(body, lines[j])
		}
		return body, len(body) > 0
	}
	return nil, false
}

// podContainersLine returns the index of the pod spec's "containers:" line
// (the first one after "template:"), or -1.
func podContainersLine(lines []string) int {
	inTemplate := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "template:" {
			inTemplate = true
		}
		if inTemplate && trimmed == "containers:" {
			return i
		}
	}
	return -1
}

// reindentBlock moves a block of lines to the given indentation, adjusting
// `nindent N` arguments by the same offset so included helpers line up.
func reindentBlock(block []string, indent int) string {
	if len(block) == 0 {
		return ""
	}
	base := opsIndentWidth(block[0])
	for _, l := range block[1:] {
		if s := opsIndentWidth(l); s < base {
			base = s
		}
	}
	delta := indent - base
	var b strings.Builder
	for _, l := range block {
		l = strings.Repeat(" ", indent) + l[base:]
		l = nindentRe.ReplaceAllStringFunc(l, func(m string) string {
			n, _ := strconv.Atoi(strings.TrimPrefix(m, "nindent "))
			return "nindent " + strconv.Itoa(n+delta)
		})
		b.WriteString(l + "\n")
	}
	return b.String()
}
