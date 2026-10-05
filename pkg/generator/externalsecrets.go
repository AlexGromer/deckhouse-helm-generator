package generator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// External Secrets Operator integration (feature "external-secrets").
//
// For every Secret that the chart's workloads consume (env secretKeyRef,
// envFrom, secret and projected volumes) and every Secret manifest rendered
// by the chart, an ExternalSecret is generated that makes ESO create that
// Secret (under the name the workloads reference) from an existing
// SecretStore or ClusterSecretStore. Provisioning the store itself needs
// backend credentials, so it is referenced, not generated.

// esoSecret is one ExternalSecret to generate.
type esoSecret struct {
	// Name is the Kubernetes Secret name workloads reference.
	Name string
	// Keys lists the keys to fetch; empty means "all properties" (dataFrom).
	Keys []string
	// Type is the Secret type when it is not Opaque.
	Type string
}

// esoOwnedSecrets returns the Secrets the chart must provide via ESO.
//
// When several charts (separate/umbrella mode) use the same Secret, exactly
// one of them owns its ExternalSecret: the chart of the input Secret manifest
// if there is one, otherwise the chart of the first workload (in resource key
// order) that references it. This avoids two releases fighting over the same
// object.
func esoOwnedSecrets(chart *types.GeneratedChart, graph *types.ResourceGraph) []esoSecret {
	if graph == nil {
		return nil
	}

	type info struct {
		owner    string // template path of the owning resource
		keys     map[string]bool
		whole    bool
		known    bool // keys are exhaustive (taken from a Secret manifest)
		typ      string
		skip     bool
		manifest bool
	}
	secrets := map[string]*info{}
	get := func(name string) *info {
		if secrets[name] == nil {
			secrets[name] = &info{keys: map[string]bool{}}
		}
		return secrets[name]
	}

	// Secret manifests from the input.
	keys := make([]types.ResourceKey, 0, len(graph.Resources))
	for k := range graph.Resources {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	for _, k := range keys {
		r := graph.Resources[k]
		if r == nil || r.Original == nil || r.Original.Object == nil || r.Original.Object.GetKind() != "Secret" {
			continue
		}
		obj := r.Original.Object.Object
		s := get(r.Original.Object.GetName())
		s.manifest = true
		s.owner = r.TemplatePath
		s.known = true
		typ, _ := obj["type"].(string)
		switch typ {
		case "kubernetes.io/service-account-token", "kubernetes.io/dockerconfigjson", "kubernetes.io/dockercfg":
			// Token secrets are populated by Kubernetes; registry credentials
			// are provisioned with the cluster, not per application.
			s.skip = true
		case "", "Opaque":
		default:
			s.typ = typ
		}
		for _, field := range []string{"data", "stringData"} {
			if m, ok := obj[field].(map[string]interface{}); ok {
				for key := range m {
					s.keys[key] = true
				}
			}
		}
	}

	// Secrets referenced by workloads.
	for _, w := range secAllWorkloads(graph) {
		for _, ref := range secSecretRefs(w.podSpec) {
			s := get(ref.Name)
			if s.owner == "" {
				s.owner = w.res.TemplatePath
			}
			if s.known {
				continue
			}
			if ref.Whole {
				s.whole = true
			}
			for _, key := range ref.Keys {
				s.keys[key] = true
			}
		}
	}

	var out []esoSecret
	for name, s := range secrets {
		if s.skip || s.owner == "" {
			continue
		}
		if _, ours := chart.Templates[s.owner]; !ours {
			continue
		}
		e := esoSecret{Name: name, Type: s.typ}
		if !s.whole || s.known {
			for key := range s.keys {
				e.Keys = append(e.Keys, key)
			}
			sort.Strings(e.Keys)
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func applyExternalSecretsFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	kind := fc.Param("store-kind")
	if kind != "SecretStore" && kind != "ClusterSecretStore" {
		return nil, fmt.Errorf("store-kind must be SecretStore or ClusterSecretStore, got %q", kind)
	}
	if fc.Param("store") == "" {
		return nil, fmt.Errorf("store must name an existing %s", kind)
	}
	policy := fc.Param("creation-policy")
	switch policy {
	case "Owner", "Orphan", "Merge", "None":
	default:
		return nil, fmt.Errorf("creation-policy must be Owner, Orphan, Merge or None, got %q", policy)
	}
	apiVersion := fc.Param("api-version")
	if !strings.HasPrefix(apiVersion, "external-secrets.io/") {
		return nil, fmt.Errorf("api-version must be in the external-secrets.io group, got %q", apiVersion)
	}

	secrets := esoOwnedSecrets(chart, fc.Graph)
	if len(secrets) == 0 {
		return chart, nil
	}

	values := map[string]interface{}{}
	for _, s := range secrets {
		entry := map[string]interface{}{
			"remoteKey": fc.Param("key-prefix") + s.Name,
		}
		keys := make([]interface{}, 0, len(s.Keys))
		for _, k := range s.Keys {
			keys = append(keys, k)
		}
		entry["keys"] = keys
		if s.Type != "" {
			entry["type"] = s.Type
		}
		values[s.Name] = entry
	}

	out := cloneChart(chart)
	if err := secAddValues(out, "externalSecrets",
		"# External Secrets Operator (dhg feature: external-secrets).\n"+
			"# Each entry creates the Secret of that name from secretStoreRef; remoteKey is the\n"+
			"# path in the backend, keys the properties to fetch (empty: all properties).\n",
		map[string]interface{}{
			"enabled":         true,
			"refreshInterval": fc.Param("refresh"),
			"creationPolicy":  policy,
			"secretStoreRef": map[string]interface{}{
				"name": fc.Param("store"),
				"kind": kind,
			},
			"secrets": values,
		}); err != nil {
		return nil, err
	}
	if err := secAddTemplate(out, "templates/external-secrets.yaml", externalSecretsTemplate(newSecChartHelpers(chart), apiVersion)); err != nil {
		return nil, err
	}
	return out, nil
}

func externalSecretsTemplate(h secChartHelpers, apiVersion string) string {
	return `{{- /* Generated by dhg feature "external-secrets". Requires External Secrets Operator. */}}
{{- $es := .Values.externalSecrets }}
{{- if and $es $es.enabled }}
{{- range $name, $secret := $es.secrets }}
---
apiVersion: ` + apiVersion + `
kind: ExternalSecret
metadata:
  name: {{ $name }}
  namespace: {{ $.Release.Namespace }}
` + h.labels(4) + `spec:
  refreshInterval: {{ $es.refreshInterval | default "1h" | quote }}
  secretStoreRef:
    name: {{ $es.secretStoreRef.name }}
    kind: {{ $es.secretStoreRef.kind }}
  target:
    name: {{ $name }}
    creationPolicy: {{ $es.creationPolicy | default "Owner" }}
    {{- with $secret.type }}
    template:
      type: {{ . }}
    {{- end }}
  {{- if $secret.keys }}
  data:
    {{- range $secret.keys }}
    - secretKey: {{ . | quote }}
      remoteRef:
        key: {{ $secret.remoteKey | quote }}
        property: {{ . | quote }}
    {{- end }}
  {{- else }}
  dataFrom:
    - extract:
        key: {{ $secret.remoteKey | quote }}
  {{- end }}
{{- end }}
{{- end }}
`
}
