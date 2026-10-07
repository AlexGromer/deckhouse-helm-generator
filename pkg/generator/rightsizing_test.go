package generator

import (
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func analyzeOne(t *testing.T, kind string, cpuReq, cpuLim, memReq, memLim string, opts RightSizingOptions) *RightSizingReport {
	t.Helper()
	graph := makeTestGraphWithWorkload(kind, "web", "default", 1, cpuReq, cpuLim, memReq, memLim)
	report := AnalyzeRightSizing(graph, opts)
	if report == nil {
		t.Fatal("AnalyzeRightSizing returned nil")
	}
	return report
}

func issuesOf(report *RightSizingReport) string {
	var out []string
	for _, w := range report.Workloads {
		for _, i := range w.Issues {
			out = append(out, string(i))
		}
	}
	return strings.Join(out, ",")
}

func TestRightSizing_Findings(t *testing.T) {
	cases := []struct {
		name                           string
		cpuReq, cpuLim, memReq, memLim string
		want                           string
	}{
		{"balanced", "100m", "200m", "128Mi", "256Mi", ""},
		{"guaranteed QoS is fine", "500m", "500m", "1Gi", "1Gi", ""},
		{"cpu limit 5x request", "100m", "500m", "128Mi", "256Mi", "overcommitted"},
		{"memory limit 4x request", "100m", "200m", "128Mi", "512Mi", "overcommitted"},
		{"ratio exactly at threshold", "100m", "300m", "128Mi", "384Mi", ""},
		{"limit below request", "500m", "250m", "128Mi", "256Mi", "limit-below-request"},
		{"no memory limit", "100m", "200m", "128Mi", "", "no-memory-limit"},
		{"no cpu limit is fine", "100m", "", "128Mi", "256Mi", ""},
		{"nothing set", "", "", "", "", "no-requests,no-memory-limit"},
		{"limits only (requests default to limits)", "", "1", "", "1Gi", ""},
		{"zero request skips the ratio", "0", "500m", "128Mi", "256Mi", ""},
		{"malformed quantity skips the ratio", "100m", "200m", "128Mi", "lots", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			report := analyzeOne(t, "Deployment", c.cpuReq, c.cpuLim, c.memReq, c.memLim, RightSizingOptions{})
			if got := issuesOf(report); got != c.want {
				t.Errorf("issues = %q, want %q (details %v)", got, c.want, report.Workloads)
			}
			if report.Analyzed != 1 {
				t.Errorf("Analyzed = %d", report.Analyzed)
			}
		})
	}
}

func TestRightSizing_CustomThreshold(t *testing.T) {
	report := analyzeOne(t, "Deployment", "100m", "250m", "128Mi", "256Mi", RightSizingOptions{OverprovisionThreshold: 2})
	if got := issuesOf(report); got != "overcommitted" {
		t.Errorf("issues = %q", got)
	}
	if !strings.Contains(report.Workloads[0].Details[0], "CPU limit 250m is 2.5x the request 100m (threshold 2.0x)") {
		t.Errorf("details = %v", report.Workloads[0].Details)
	}
}

func TestRightSizing_BatchWorkloads(t *testing.T) {
	skipped := analyzeOne(t, "Job", "", "", "", "", RightSizingOptions{})
	if skipped.Analyzed != 0 || len(skipped.Workloads) != 0 {
		t.Errorf("Jobs must be skipped unless IncludeBatchWorkloads: %+v", skipped)
	}
	included := analyzeOne(t, "CronJob", "", "", "", "", RightSizingOptions{IncludeBatchWorkloads: true})
	if included.Analyzed != 1 || issuesOf(included) != "no-requests,no-memory-limit" {
		t.Errorf("CronJob not analysed: %+v", included)
	}
}

func TestRightSizing_EmptyGraph(t *testing.T) {
	for _, g := range []*types.ResourceGraph{nil, types.NewResourceGraph()} {
		report := AnalyzeRightSizing(g, RightSizingOptions{})
		if report.TotalIssues != 0 || len(report.Workloads) != 0 {
			t.Errorf("unexpected report %+v", report)
		}
		if !strings.Contains(report.Markdown(), "No workloads analysed.") {
			t.Errorf("Markdown:\n%s", report.Markdown())
		}
	}
}

func TestRightSizingReport_Markdown(t *testing.T) {
	report := analyzeOne(t, "Deployment", "100m", "500m", "128Mi", "", RightSizingOptions{})
	md := report.Markdown()
	for _, want := range []string{
		"## Requests and limits",
		"1 of 1 workload(s) have findings (no-memory-limit: 1, overcommitted: 1)",
		"| Deployment/web | app: no memory limit |",
		"| Deployment/web | app: CPU limit 500m is 5.0x the request 100m",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("Markdown misses %q:\n%s", want, md)
		}
	}

	clean := analyzeOne(t, "Deployment", "100m", "100m", "128Mi", "128Mi", RightSizingOptions{})
	if !strings.Contains(clean.Markdown(), "No findings for 1 workload(s).") {
		t.Errorf("Markdown:\n%s", clean.Markdown())
	}
}
