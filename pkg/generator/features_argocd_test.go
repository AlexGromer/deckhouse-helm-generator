package generator

import (
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/testutil"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestApplyArgoCDFeature(t *testing.T) {
	t.Run("Empty Repo URL (Disabled)", func(t *testing.T) {
		chart := &types.GeneratedChart{
			Name:       "test-chart",
			ValuesYAML: "",
			Templates:  make(map[string]string),
		}

		params := map[string]string{
			"project":         "default",
			"repo-url":        "",
			"target-revision": "HEAD",
			"dest-server":     "https://kubernetes.default.svc",
			"dest-namespace":  "",
			"auto-sync":       "true",
			"self-heal":       "true",
			"prune":           "true",
		}

		var notes []string
		fc := FeatureContext{
			Params: params,
			Notes:  &notes,
		}

		chart, err := applyArgoCDFeature(chart, fc)
		testutil.AssertNoError(t, err)

		// Check Note
		if len(notes) != 1 || !strings.Contains(notes[0], "argocd: repo-url is empty") {
			t.Errorf("Expected note about empty repo URL")
		}

		// Check Values
		if !strings.Contains(chart.ValuesYAML, "argocd:") {
			t.Errorf("Expected argocd section in values.yaml")
		}
		if !strings.Contains(chart.ValuesYAML, "enabled: false") {
			t.Errorf("Expected argocd to be disabled")
		}

		// Check Templates
		template := chart.Templates["argocd-application.yaml"]
		if !strings.Contains(template, "apiVersion: argoproj.io/v1alpha1") {
			t.Errorf("Expected application template to be generated")
		}
	})

	t.Run("Explicit Repo URL (Enabled)", func(t *testing.T) {
		chart := &types.GeneratedChart{
			Name:       "test-chart",
			ValuesYAML: "",
			Templates:  make(map[string]string),
		}

		params := map[string]string{
			"project":         "my-proj",
			"repo-url":        "https://github.com/example/repo",
			"target-revision": "main",
			"dest-server":     "https://kubernetes.default.svc",
			"dest-namespace":  "prod",
			"auto-sync":       "true",
			"self-heal":       "false",
			"prune":           "true",
		}

		var notes []string
		fc := FeatureContext{
			Params: params,
			Notes:  &notes,
		}

		chart, err := applyArgoCDFeature(chart, fc)
		testutil.AssertNoError(t, err)

		// Check Values
		if !strings.Contains(chart.ValuesYAML, "enabled: true") {
			t.Errorf("Expected argocd to be enabled")
		}
		if !strings.Contains(chart.ValuesYAML, "repoURL: https://github.com/example/repo") {
			t.Errorf("Expected repoURL in values")
		}
		if !strings.Contains(chart.ValuesYAML, "selfHeal: false") {
			t.Errorf("Expected selfHeal: false")
		}
	})

	t.Run("No Auto Sync", func(t *testing.T) {
		chart := &types.GeneratedChart{
			Name:       "test-chart",
			ValuesYAML: "",
			Templates:  make(map[string]string),
		}

		params := map[string]string{
			"project":         "default",
			"repo-url":        "https://github.com/example/repo",
			"target-revision": "main",
			"dest-server":     "https://kubernetes.default.svc",
			"dest-namespace":  "prod",
			"auto-sync":       "false",
			"self-heal":       "true",
			"prune":           "true",
		}

		var notes []string
		fc := FeatureContext{
			Params: params,
			Notes:  &notes,
		}

		chart, err := applyArgoCDFeature(chart, fc)
		testutil.AssertNoError(t, err)

		// Check Values
		if strings.Contains(chart.ValuesYAML, "syncPolicy:") {
			t.Errorf("Expected syncPolicy to be omitted when auto-sync is false")
		}
	})
}
