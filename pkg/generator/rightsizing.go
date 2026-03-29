package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// OverprovisionThreshold is the limit/request ratio above which a container is
// considered over-provisioned (exclusive — ratio must be strictly greater).
const OverprovisionThreshold = 3.0

// UnderprovisionThreshold is the limit/request ratio below which a container is
// considered under-provisioned (exclusive — ratio must be strictly less).
const UnderprovisionThreshold = 1.2

// RightSizingIssue identifies a category of right-sizing finding.
type RightSizingIssue string

const (
	RightSizingIssueOverprovisioned  RightSizingIssue = "overprovisioned"
	RightSizingIssueUnderprovisioned RightSizingIssue = "underprovisioned"
	RightSizingIssueNoLimits        RightSizingIssue = "no-limits"
	RightSizingIssueNoRequests      RightSizingIssue = "no-requests"
)

// RightSizingWorkload holds per-workload right-sizing findings.
type RightSizingWorkload struct {
	Name      string
	Namespace string
	Kind      string
	Issues    []RightSizingIssue
	Details   []string
}

// RightSizingReport summarises the right-sizing analysis across all workloads.
type RightSizingReport struct {
	Workloads    []RightSizingWorkload
	TotalIssues  int
	IssuesByType map[RightSizingIssue]int
}

// RightSizingOptions configures the right-sizing analysis.
type RightSizingOptions struct {
	WorkloadType            WorkloadType
	OverprovisionThreshold  float64
	UnderprovisionThreshold float64
	IncludeBatchWorkloads   bool
}

// isBatchKind returns true for batch workload kinds (Job, CronJob).
func isBatchKind(kind string) bool {
	return kind == "Job" || kind == "CronJob"
}

// analyzeContainerRightSizing inspects a single container's resources and returns
// detected issues.
func analyzeContainerRightSizing(c map[string]interface{}, overprovThreshold, underprovThreshold float64) ([]RightSizingIssue, []string) {
	var issues []RightSizingIssue
	var details []string

	// Read requests and limits maps.
	requests, _, _ := unstructuredStringMap(c, "resources", "requests")
	limits, _, _ := unstructuredStringMap(c, "resources", "limits")

	cpuReqStr := requests["cpu"]
	memReqStr := requests["memory"]
	cpuLimStr := limits["cpu"]
	memLimStr := limits["memory"]

	hasRequests := cpuReqStr != "" || memReqStr != ""
	hasLimits := cpuLimStr != "" || memLimStr != ""

	if !hasRequests {
		issues = append(issues, RightSizingIssueNoRequests)
		details = append(details, "container has no resource requests")
		// Cannot compute ratios without requests.
		if !hasLimits {
			issues = append(issues, RightSizingIssueNoLimits)
			details = append(details, "container has no resource limits")
		}
		return issues, details
	}

	if !hasLimits {
		issues = append(issues, RightSizingIssueNoLimits)
		details = append(details, "container has no resource limits")
		return issues, details
	}

	// Check CPU ratio.
	if cpuReqStr != "" && cpuLimStr != "" {
		reqMillis, errReq := parseResourceQuantity(cpuReqStr, true)
		limMillis, errLim := parseResourceQuantity(cpuLimStr, true)
		if errReq == nil && errLim == nil && reqMillis > 0 {
			ratio := float64(limMillis) / float64(reqMillis)
			if ratio > overprovThreshold {
				issues = append(issues, RightSizingIssueOverprovisioned)
				details = append(details, fmt.Sprintf("CPU limit/request ratio %.2f > %.1f", ratio, overprovThreshold))
			} else if ratio < underprovThreshold {
				issues = append(issues, RightSizingIssueUnderprovisioned)
				details = append(details, fmt.Sprintf("CPU limit/request ratio %.2f < %.1f", ratio, underprovThreshold))
			}
		}
		// If reqMillis == 0 → ratio undefined, skip ratio check.
	}

	// Check memory ratio.
	if memReqStr != "" && memLimStr != "" {
		reqMiB, errReq := parseResourceQuantity(memReqStr, false)
		limMiB, errLim := parseResourceQuantity(memLimStr, false)
		if errReq == nil && errLim == nil && reqMiB > 0 {
			ratio := float64(limMiB) / float64(reqMiB)
			if ratio > overprovThreshold {
				// Avoid duplicate overprovisioned issue if CPU already added it.
				if !hasIssue(issues, RightSizingIssueOverprovisioned) {
					issues = append(issues, RightSizingIssueOverprovisioned)
				}
				details = append(details, fmt.Sprintf("memory limit/request ratio %.2f > %.1f", ratio, overprovThreshold))
			} else if ratio < underprovThreshold {
				if !hasIssue(issues, RightSizingIssueUnderprovisioned) {
					issues = append(issues, RightSizingIssueUnderprovisioned)
				}
				details = append(details, fmt.Sprintf("memory limit/request ratio %.2f < %.1f", ratio, underprovThreshold))
			}
		}
		// If reqMiB == 0 → skip; if parse error → skip silently.
	}

	return issues, details
}

// hasIssue returns true if the issue is already in the slice.
func hasIssue(issues []RightSizingIssue, target RightSizingIssue) bool {
	for _, i := range issues {
		if i == target {
			return true
		}
	}
	return false
}

// unstructuredStringMap is a helper to get a map[string]string from an unstructured map path.
func unstructuredStringMap(obj map[string]interface{}, fields ...string) (map[string]string, bool, error) {
	cur := obj
	for i, f := range fields {
		v, ok := cur[f]
		if !ok {
			return nil, false, nil
		}
		if i == len(fields)-1 {
			// Last field — convert to map[string]string.
			switch m := v.(type) {
			case map[string]string:
				return m, true, nil
			case map[string]interface{}:
				result := make(map[string]string, len(m))
				for k, val := range m {
					if s, ok := val.(string); ok {
						result[k] = s
					}
				}
				return result, true, nil
			}
			return nil, false, nil
		}
		next, ok := v.(map[string]interface{})
		if !ok {
			return nil, false, nil
		}
		cur = next
	}
	return nil, false, nil
}

// AnalyzeRightSizing inspects all workloads in the graph and returns a RightSizingReport.
func AnalyzeRightSizing(graph *types.ResourceGraph, opts RightSizingOptions) *RightSizingReport {
	report := &RightSizingReport{
		Workloads:    []RightSizingWorkload{},
		IssuesByType: make(map[RightSizingIssue]int),
	}

	if graph == nil {
		return report
	}

	for _, r := range graph.Resources {
		kind := r.Original.GVK.Kind
		if !isWorkloadKind(kind) {
			continue
		}

		// Skip batch workloads if not included.
		if isBatchKind(kind) && !opts.IncludeBatchWorkloads {
			continue
		}

		obj := r.Original.Object
		containers, ok := extractContainersFromObj(obj)
		if !ok || len(containers) == 0 {
			continue
		}

		wl := RightSizingWorkload{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
			Kind:      kind,
		}

		for _, cRaw := range containers {
			c, ok := cRaw.(map[string]interface{})
			if !ok {
				continue
			}
			issues, details := analyzeContainerRightSizing(c, opts.OverprovisionThreshold, opts.UnderprovisionThreshold)
			for _, iss := range issues {
				if !hasIssue(wl.Issues, iss) {
					wl.Issues = append(wl.Issues, iss)
				}
			}
			wl.Details = append(wl.Details, details...)
		}

		if len(wl.Issues) > 0 {
			report.Workloads = append(report.Workloads, wl)
			for _, iss := range wl.Issues {
				report.IssuesByType[iss]++
				report.TotalIssues++
			}
		}
	}

	return report
}

// GenerateRightSizingNotes returns a human-readable summary of the right-sizing report.
// The returned string includes workload names and issue descriptions.
func GenerateRightSizingNotes(report *RightSizingReport) string {
	if report == nil || len(report.Workloads) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Right-Sizing Analysis\n\n")
	sb.WriteString(fmt.Sprintf("Total issues found: %d\n\n", report.TotalIssues))

	if len(report.IssuesByType) > 0 {
		sb.WriteString("Issue summary:\n")
		for issType, count := range report.IssuesByType {
			sb.WriteString(fmt.Sprintf("  - %s: %d\n", issType, count))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Workload details:\n")
	for _, wl := range report.Workloads {
		sb.WriteString(fmt.Sprintf("- %s/%s (%s):\n", wl.Namespace, wl.Name, wl.Kind))
		for _, iss := range wl.Issues {
			sb.WriteString(fmt.Sprintf("    [%s]\n", iss))
		}
		for _, d := range wl.Details {
			sb.WriteString(fmt.Sprintf("    %s\n", d))
		}
	}

	return sb.String()
}

// InjectRightSizingNotes injects a right-sizing analysis section into the chart's NOTES.txt.
// Idempotent: if the section already exists, the chart is returned unchanged.
// Returns a copy of the chart and a boolean indicating whether injection occurred.
func InjectRightSizingNotes(chart *types.GeneratedChart, report *RightSizingReport) (*types.GeneratedChart, bool) {
	if chart == nil {
		return nil, false
	}
	if report == nil {
		result := copyChartTemplates(chart)
		return result, false
	}

	const marker = "Right-Sizing"
	if strings.Contains(chart.Notes, marker) {
		result := copyChartTemplates(chart)
		return result, false
	}

	notes := GenerateRightSizingNotes(report)
	if notes == "" {
		result := copyChartTemplates(chart)
		return result, false
	}

	result := copyChartTemplates(chart)

	var sb strings.Builder
	if result.Notes != "" {
		sb.WriteString(result.Notes)
		if !strings.HasSuffix(result.Notes, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString(notes)

	result.Notes = sb.String()
	return result, true
}
