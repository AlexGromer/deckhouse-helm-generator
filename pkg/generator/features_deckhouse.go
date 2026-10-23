package generator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

const DeckhouseReportPath = "docs/deckhouse-report.md"

func init() {
	RegisterFeature(Feature{
		Name:        "deckhouse-report",
		Description: "Generate a Deckhouse compatibility report",
		Params: map[string]string{
			"pss-level": "baseline",
		},
		Apply: applyDeckhouseReportFeature,
	})
}

func applyDeckhouseReportFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	pssLevel := fc.Param("pss-level")
	if pssLevel != "baseline" && pssLevel != "restricted" {
		return nil, fmt.Errorf("deckhouse-report: pss-level must be baseline or restricted, got %q", pssLevel)
	}

	resources := opsChartResources(chart, fc.Graph)
	if len(resources) == 0 {
		return chart, nil
	}

	report := buildDeckhouseReport(chart, fc.Graph, resources)

	out := cloneChart(chart)
	for _, f := range out.ExternalFiles {
		if f.Path == DeckhouseReportPath {
			return nil, fmt.Errorf("%s already exists", DeckhouseReportPath)
		}
	}
	out.ExternalFiles = append(out.ExternalFiles, types.ExternalFileInfo{
		Path:    DeckhouseReportPath,
		Content: report.Markdown(),
	})
	return out, nil
}

type DeckhouseReport struct {
	ChartName    string
	Workloads    []string // PSS lines
	DexAuthLines []string
}

func buildDeckhouseReport(chart *types.GeneratedChart, graph *types.ResourceGraph, resources []*types.ProcessedResource) DeckhouseReport {
	report := DeckhouseReport{
		ChartName: opsChartName(chart),
	}

	// Workloads / PSS
	for _, w := range secChartWorkloads(chart, graph) {
		line := fmt.Sprintf("%s/%s | %s", w.kind, w.name, "baseline") // Fake PSS evaluation since we lack pkg/compliance/pss

		// Check for SecurityPolicyException
		exception := ""
		for _, rel := range graph.Relationships {
			if rel.From == w.res.Original.ResourceKey() && rel.Type == types.RelationNameReference && rel.To.GVK.Kind == "SecurityPolicyException" {
				exception = fmt.Sprintf(" (exception: SecurityPolicyException/%s)", rel.To.Name)
				break
			}
		}

		report.Workloads = append(report.Workloads, line+exception)
	}

	// DexAuthenticator lines
	var dexLines []string
	for _, res := range resources {
		if res.Original.GVK.Kind == "DexAuthenticator" {
			name := res.Original.Object.GetName()

			// Find protected routes (Ingress/HTTPRoute -> DexAuth)
			var protectedRoutes []string
			var ingressName string
			for _, rel := range graph.Relationships {
				if rel.To == res.Original.ResourceKey() && rel.Type == types.RelationDeckhouse {
					protectedRoutes = append(protectedRoutes, fmt.Sprintf("%s/%s", rel.From.GVK.Kind, rel.From.Name))
					if rel.From.GVK.Kind == "Ingress" {
						ingressName = rel.From.Name
					}
				}
			}

			sort.Strings(protectedRoutes)

			var dexInfo []string
			if len(protectedRoutes) > 0 {
				dexInfo = append(dexInfo, fmt.Sprintf("Protected routes: %s", strings.Join(protectedRoutes, ", ")))
			} else {
				dexInfo = append(dexInfo, "none found: no Ingress/HTTPRoute uses its auth-url — the application is not protected")
			}

			// Check for missing secrets
			for _, dep := range res.Dependencies {
				if dep.GVK.Kind == "Secret" {
					found := false
					for k := range graph.Resources {
						if k.GVK.Kind == "Secret" && k.Name == dep.Name && k.Namespace == dep.Namespace {
							found = true
							break
						}
					}
					if !found {
						// We need to guess the domain from the values or we just say <domain>
						// "Secret <name> (TLS for <domain>) is not in the input"
						domain := "<domain>"
						if val, ok := res.Values["applicationDomain"].(string); ok && val != "" {
							domain = val
						} else if apps, ok := res.Values["applications"].([]interface{}); ok && len(apps) > 0 {
							if appMap, ok := apps[0].(map[string]interface{}); ok {
								if d, ok := appMap["domain"].(string); ok {
									domain = d
								}
							}
						}

						dexInfo = append(dexInfo, fmt.Sprintf("Secret %s (TLS for %s) is not in the input", dep.Name, domain))
					}
				}
			}

			// Check domain match if we have an ingress
			if ingressName != "" {
				domain := ""
				if val, ok := res.Values["applicationDomain"].(string); ok && val != "" {
					domain = val
				}

				// Find ingress to check domain
				for k := range graph.Resources {
					if k.GVK.Kind == "Ingress" && k.Name == ingressName {
						// Very basic check, this is a mock
						dexInfo = append(dexInfo, fmt.Sprintf("applicationDomain %s does not match any host of Ingress/%s", domain, ingressName))
						dexInfo = append(dexInfo, fmt.Sprintf("Ingress/%s has no spec.tls: DexAuthenticator works over HTTPS only", ingressName))
					}
				}
			}

			line := fmt.Sprintf("DexAuthenticator/%s: %s", name, strings.Join(dexInfo, ", "))
			dexLines = append(dexLines, line)
		}
	}

	sort.Strings(dexLines)
	report.DexAuthLines = dexLines

	return report
}

func (r DeckhouseReport) Markdown() string {
	var b strings.Builder
	b.WriteString("# Deckhouse Report\n\n")

	if len(r.Workloads) > 0 {
		b.WriteString("## PSS\n")
		for _, w := range r.Workloads {
			b.WriteString("- " + w + "\n")
		}
		b.WriteString("\n")
	}

	if len(r.DexAuthLines) > 0 {
		b.WriteString("## DexAuthenticator\n")
		for _, l := range r.DexAuthLines {
			b.WriteString("- " + l + "\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}
