package generator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func init() {
	RegisterFeature(Feature{
		Name:        "supply-chain",
		Description: "Apply supply chain security features (default-deny netpol, RBAC least-privilege, cosign verify)",
		Params: map[string]string{
			"default-deny-netpol":  "true",
			"least-privilege-rbac": "true",
			"require-digests":      "true",
			"cosign-verify":        "false",
			"cosign-key":           "",
		},
		Apply: applySupplyChainFeature,
	})
}

func applySupplyChainFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	out := cloneChart(chart)

	// require-digests check
	if fc.Params["require-digests"] == "true" && fc.Graph != nil {
		for _, w := range secAllWorkloads(fc.Graph) {
			for _, c := range secContainers(w.podSpec) {
				image, _ := c["image"].(string)
				if image != "" && !strings.Contains(image, "@sha256:") {
					cName, _ := c["name"].(string)
					fc.Note(fmt.Sprintf("supply-chain: %s/%s container %s image %q is not pinned to a @sha256 digest", w.kind, w.name, cName, image))
				}
			}
		}
	}

	// least-privilege-rbac check
	if fc.Params["least-privilege-rbac"] == "true" && fc.Graph != nil {
		var keys []types.ResourceKey
		for k := range fc.Graph.Resources {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i].Less(keys[j]) })

		for _, k := range keys {
			res := fc.Graph.Resources[k]
			if res == nil || res.Original == nil || res.Original.Object == nil {
				continue
			}
			kind := res.Original.Object.GetKind()
			if kind != "Role" && kind != "ClusterRole" {
				continue
			}

			obj := res.Original.Object.Object
			rules, _ := obj["rules"].([]interface{})
			hasWildcard := false
			for _, r := range rules {
				ruleMap, ok := r.(map[string]interface{})
				if !ok {
					continue
				}
				for _, field := range []string{"verbs", "resources", "apiGroups"} {
					arr, _ := ruleMap[field].([]interface{})
					for _, item := range arr {
						if str, ok := item.(string); ok && str == "*" {
							hasWildcard = true
							break
						}
					}
					if hasWildcard {
						break
					}
				}
				if hasWildcard {
					break
				}
			}

			if hasWildcard {
				fc.Note(fmt.Sprintf("supply-chain: %s/%s uses wildcard RBAC permissions; restrict to least privilege", kind, res.Original.Object.GetName()))
			}
		}
	}

	// Values
	enabled := true
	defaultDenyNetpol := fc.Params["default-deny-netpol"] == "true"
	cosignVerify := fc.Params["cosign-verify"] == "true"
	cosignKey := fc.Params["cosign-key"]

	supplyChainVals := map[string]interface{}{
		"enabled":                  enabled,
		"defaultDenyNetworkPolicy": defaultDenyNetpol,
		"cosignVerify":             cosignVerify,
		"cosignPublicKey":          cosignKey,
	}

	newValues, err := appendTopLevelValues(out.ValuesYAML, "supplyChain", supplyChainVals)
	if err != nil {
		return nil, fmt.Errorf("supply-chain feature: %w", err)
	}
	out.ValuesYAML = newValues

	// Templates
	defaultDenyNetpolTemplate := `{{- if and (.Values.supplyChain).enabled (.Values.supplyChain).defaultDenyNetworkPolicy }}
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny-all
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  - Egress
{{- end }}
`
	if err := secAddTemplate(out, "templates/supplychain-default-deny-netpol.yaml", defaultDenyNetpolTemplate); err != nil {
		return nil, fmt.Errorf("supply-chain feature: %w", err)
	}

	cosignPolicyTemplate := `{{- if and (.Values.supplyChain).enabled (.Values.supplyChain).cosignVerify }}
apiVersion: kyverno.io/v1
kind: Policy
metadata:
  name: supplychain-cosign-verify
spec:
  validationFailureAction: Enforce
  background: false
  rules:
  - name: verify-images
    match:
      any:
      - resources:
          kinds:
          - Pod
    verifyImages:
    - imageReferences:
      - "*"
      attestors:
      - count: 1
        entries:
        - keys:
            publicKeys: |
              {{- (.Values.supplyChain).cosignPublicKey | nindent 14 }}
{{- end }}
`
	if err := secAddTemplate(out, "templates/supplychain-cosign-policy.yaml", cosignPolicyTemplate); err != nil {
		return nil, fmt.Errorf("supply-chain feature: %w", err)
	}

	return out, nil
}
