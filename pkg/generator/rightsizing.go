package generator

import (
	"fmt"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// OverprovisionThreshold is the default limit/request ratio above which a
// container is reported as overcommitted (exclusive: the ratio must be
// strictly greater).
const OverprovisionThreshold = 3.0

// RightSizingIssue identifies a category of right-sizing finding.
type RightSizingIssue string

const (
	// RightSizingIssueOverprovisioned: the limit is far above the request, so
	// the scheduler reserves much less than the container may use and the node
	// can be overcommitted (CPU throttling, OOM kills under pressure).
	RightSizingIssueOverprovisioned RightSizingIssue = "overcommitted"
	// RightSizingIssueLimitBelowRequest: a limit lower than the request is
	// rejected by the API server.
	RightSizingIssueLimitBelowRequest RightSizingIssue = "limit-below-request"
	// RightSizingIssueNoLimits: no memory limit, so the container can use all
	// of the node's memory.
	RightSizingIssueNoLimits RightSizingIssue = "no-memory-limit"
	// RightSizingIssueNoRequests: no CPU and memory requests, so the scheduler
	// places the pod blindly and it is evicted first (BestEffort QoS).
	RightSizingIssueNoRequests RightSizingIssue = "no-requests"
)

// rightSizingIssueOrder fixes the order issues are reported in.
var rightSizingIssueOrder = []RightSizingIssue{
	RightSizingIssueLimitBelowRequest,
	RightSizingIssueNoRequests,
	RightSizingIssueNoLimits,
	RightSizingIssueOverprovisioned,
}

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
	// Analyzed is the number of workloads inspected.
	Analyzed     int
	Workloads    []RightSizingWorkload
	TotalIssues  int
	IssuesByType map[RightSizingIssue]int
}

// RightSizingOptions configures the right-sizing analysis.
type RightSizingOptions struct {
	// OverprovisionThreshold is the limit/request ratio above which a
	// container is reported as overcommitted; <= 1 uses the default (3).
	OverprovisionThreshold float64
	// IncludeBatchWorkloads also analyses Jobs and CronJobs.
	IncludeBatchWorkloads bool
}

// isBatchKind returns true for batch workload kinds (Job, CronJob).
func isBatchKind(kind string) bool {
	return kind == "Job" || kind == "CronJob"
}

// analyzeContainerRightSizing inspects a single container's resources and
// returns the detected issues with human-readable details.
func analyzeContainerRightSizing(c map[string]interface{}, overprovThreshold float64) ([]RightSizingIssue, []string) {
	var issues []RightSizingIssue
	var details []string
	add := func(issue RightSizingIssue, detail string) {
		if !hasIssue(issues, issue) {
			issues = append(issues, issue)
		}
		details = append(details, detail)
	}

	name, _ := c["name"].(string)
	if name == "" {
		name = "container"
	}
	requests := containerQuantities(c, "requests")
	limits := containerQuantities(c, "limits")

	if requests["cpu"] == "" && requests["memory"] == "" {
		// Kubernetes defaults requests to limits when only limits are set.
		if limits["cpu"] == "" && limits["memory"] == "" {
			add(RightSizingIssueNoRequests, fmt.Sprintf("%s: no CPU or memory requests (BestEffort QoS)", name))
		}
	}
	if limits["memory"] == "" {
		add(RightSizingIssueNoLimits, fmt.Sprintf("%s: no memory limit", name))
	}

	for _, res := range []struct {
		name  string
		isCPU bool
	}{{"cpu", true}, {"memory", false}} {
		reqStr, limStr := requests[res.name], limits[res.name]
		if reqStr == "" || limStr == "" {
			continue
		}
		req, errReq := parseResourceQuantity(reqStr, res.isCPU)
		lim, errLim := parseResourceQuantity(limStr, res.isCPU)
		if errReq != nil || errLim != nil || req <= 0 {
			continue
		}
		label := "CPU"
		if !res.isCPU {
			label = "memory"
		}
		ratio := float64(lim) / float64(req)
		switch {
		case lim < req:
			add(RightSizingIssueLimitBelowRequest,
				fmt.Sprintf("%s: %s limit %s is below the request %s (rejected by the API server)", name, label, limStr, reqStr))
		case ratio > overprovThreshold:
			add(RightSizingIssueOverprovisioned,
				fmt.Sprintf("%s: %s limit %s is %.1fx the request %s (threshold %.1fx); raise the request or lower the limit",
					name, label, limStr, ratio, reqStr, overprovThreshold))
		}
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

// containerQuantities returns resources.<field> of a container as strings.
// Numeric quantities (e.g. `cpu: 2` in YAML) are converted to strings.
func containerQuantities(c map[string]interface{}, field string) map[string]string {
	resources, _ := c["resources"].(map[string]interface{})
	raw, _ := resources[field].(map[string]interface{})
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		switch q := v.(type) {
		case string:
			out[k] = q
		case int, int32, int64, float64:
			out[k] = fmt.Sprint(q)
		}
	}
	return out
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
	threshold := opts.OverprovisionThreshold
	if threshold <= 1 {
		threshold = OverprovisionThreshold
	}

	for _, r := range opsSortedResources(graph) {
		kind := r.Original.GVK.Kind
		if !isWorkloadKind(kind) || (isBatchKind(kind) && !opts.IncludeBatchWorkloads) {
			continue
		}
		obj := r.Original.Object
		containers, ok := extractContainersFromObj(obj)
		if !ok || len(containers) == 0 {
			continue
		}
		report.Analyzed++

		wl := RightSizingWorkload{Name: obj.GetName(), Namespace: obj.GetNamespace(), Kind: kind}
		for _, cRaw := range containers {
			c, ok := cRaw.(map[string]interface{})
			if !ok {
				continue
			}
			issues, details := analyzeContainerRightSizing(c, threshold)
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

// Markdown renders the right-sizing findings as a Markdown section.
func (r *RightSizingReport) Markdown() string {
	var b strings.Builder
	b.WriteString("## Requests and limits\n\n")
	if r == nil || r.Analyzed == 0 {
		b.WriteString("No workloads analysed.\n")
		return b.String()
	}
	if len(r.Workloads) == 0 {
		fmt.Fprintf(&b, "No findings for %d workload(s).\n", r.Analyzed)
		return b.String()
	}
	var summary []string
	for _, issue := range rightSizingIssueOrder {
		if n := r.IssuesByType[issue]; n > 0 {
			summary = append(summary, fmt.Sprintf("%s: %d", issue, n))
		}
	}
	fmt.Fprintf(&b, "%d of %d workload(s) have findings (%s).\n\n", len(r.Workloads), r.Analyzed, strings.Join(summary, ", "))
	b.WriteString("| Workload | Finding |\n")
	b.WriteString("|---|---|\n")
	for _, w := range r.Workloads {
		for _, d := range w.Details {
			fmt.Fprintf(&b, "| %s/%s | %s |\n", w.Kind, w.Name, d)
		}
	}
	return b.String()
}
