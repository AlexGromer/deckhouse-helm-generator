package generator

func init() {
	RegisterFeature(Feature{
		Name:        "prometheus-rules",
		Description: "PrometheusRule with workload alerts (crash loops, OOM kills, restarts, not ready) and optional SLO burn-rate alerts",
		Params: map[string]string{
			"severity":            "warning",
			"runbook-url":         "",
			"slo-availability":    "",
			"slo-requests-metric": "http_requests_total",
			"slo-errors-selector": `code=~"5.."`,
			"prometheus":          "main",
			"labels":              "prometheus=main;component=rules",
		},
		Apply: applyPrometheusRulesFeature,
	})
}
