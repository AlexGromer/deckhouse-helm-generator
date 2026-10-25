package generator

import (
	"fmt"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func init() {
	RegisterFeature(Feature{
		Name:        "gateway-api",
		Description: "Gateway API: HTTPRoute (and ListenerSet/GRPCRoute) for every Ingress, Ingress switchable off via values",
		Params: map[string]string{
			"class-name":        "alb",
			"create-gateway":    "true",
			"gateway-name":      "",
			"gateway-namespace": "",
			"listener-set":      "false",
			"keep-ingress":      "true",
		},
		Apply: applyGatewayAPIFeature,
	})
}

func applyGatewayAPIFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	facts, allNotes := CollectIngressFacts(fc.Graph)
	var notesString string
	for _, n := range allNotes {
		notesString += fmt.Sprintf("gateway-api: %s\n", n)
	}
	if notesString != "" {
		chart.Notes += "\n" + notesString
	}

	ingresses := resourceTemplates(chart, isKind("Ingress"))
	if len(ingresses) == 0 {
		return chart, nil
	}

	hasRelevantIngress := false
	for _, f := range facts {
		if f.InChart(chart) {
			hasRelevantIngress = true
			break
		}
	}

	if !hasRelevantIngress {
		return chart, nil
	}

	return finishApplyGatewayAPIFeature(chart, fc, facts)
}
