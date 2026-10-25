package generator

import (
	"fmt"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func init() {
	RegisterFeature(Feature{
		Name:        "argocd",
		Description: "generates an Argo CD Application referencing the chart (disables by default if repo-url is empty)",
		Params: map[string]string{
			"project":         "default",
			"repo-url":        "",
			"target-revision": "HEAD",
			"dest-server":     "https://kubernetes.default.svc",
			"dest-namespace":  "",
			"auto-sync":       "true",
			"self-heal":       "true",
			"prune":           "true",
		},
		Apply: applyArgoCDFeature,
	})
}

func applyArgoCDFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	enabled := true
	repoURL := fc.Param("repo-url")
	if repoURL == "" {
		enabled = false
		fc.Note("argocd: repo-url is empty; set argocd.source.repoURL in values.yaml")
	}

	destNs := fc.Param("dest-namespace")

	argocdValues := map[string]interface{}{
		"enabled": enabled,
		"project": fc.Param("project"),
		"source": map[string]interface{}{
			"repoURL":        repoURL,
			"targetRevision": fc.Param("target-revision"),
			"path":           ".",
		},
		"destination": map[string]interface{}{
			"server":    fc.Param("dest-server"),
			"namespace": destNs,
		},
		"syncPolicy": map[string]interface{}{
			"automated": map[string]interface{}{
				"selfHeal": fc.Param("self-heal") == "true",
				"prune":    fc.Param("prune") == "true",
			},
		},
	}

	if fc.Param("auto-sync") != "true" {
		delete(argocdValues, "syncPolicy")
	}

	valuesYAML, err := appendTopLevelValues(chart.ValuesYAML, "argocd", argocdValues)
	if err != nil {
		return chart, fmt.Errorf("failed to append argocd to values.yaml: %w", err)
	}
	chart.ValuesYAML = valuesYAML

	appTemplate := fmt.Sprintf(`{{- if .Values.argocd.enabled }}
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: %s
  namespace: argocd
  labels:
    {{- include "%s.labels" . | nindent 4 }}
spec:
  project: {{ .Values.argocd.project }}
  source:
    repoURL: {{ .Values.argocd.source.repoURL }}
    targetRevision: {{ .Values.argocd.source.targetRevision }}
    path: {{ .Values.argocd.source.path }}
  destination:
    server: {{ .Values.argocd.destination.server }}
    namespace: {{ .Values.argocd.destination.namespace | default .Release.Namespace }}
  {{- with .Values.argocd.syncPolicy }}
  syncPolicy:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
`, chart.Name, chart.Name)

	if chart.Templates == nil {
		chart.Templates = make(map[string]string)
	}
	chart.Templates["argocd-application.yaml"] = appTemplate

	return chart, nil
}
