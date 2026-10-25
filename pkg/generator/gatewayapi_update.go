package generator

import (
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// finishApplyGatewayAPIFeature implementation
func finishApplyGatewayAPIFeature(chart *types.GeneratedChart, fc FeatureContext, facts []IngressFacts) (*types.GeneratedChart, error) {
	opts, err := parseGatewayAPIOptions(fc)
	if err != nil {
		return nil, err
	}

	out := cloneChart(chart)

	ingresses := resourceTemplates(chart, isKind("Ingress"))

	for _, rt := range ingresses {
		// Find matching fact
		var fact *IngressFacts
		for _, f := range facts {
			if f.TemplatePath == rt.path {
				fact = &f
				break
			}
		}

		if fact == nil {
			continue
		}

		trans := translateIngress(*fact, opts, fc.Param("gateway-name"), fc.Param("gateway-namespace"))

		tmplStr := gatewayAPITemplate(rt, fact.Name, trans)

		path := featureTemplatePath("gateway-api", rt.path)
		if err := addTemplate(out, path, tmplStr); err != nil {
			return nil, err
		}

		if guarded, ok := guardIngressTemplate(out.Templates[rt.path]); ok {
			out.Templates[rt.path] = guarded
		}
	}

	values := map[string]interface{}{
		"enabled":        true,
		"disableIngress": !opts.KeepIngress,
		"listenerSet": map[string]interface{}{
			"enabled": opts.ListenerSet,
		},
	}

	if err := addFeatureValues(out, "gatewayApi", values); err != nil {
		return nil, err
	}

	// Add Gateway API v1.5 section to SYNTHESIS.md
	out.Notes += "\n## Gateway API v1.5\nGateway API resources were generated via `--with gateway-api`.\n"

	return out, nil
}
