package generator

import (
	"sort"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

const supplyChainTestManifests = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
  namespace: test-ns
spec:
  template:
    spec:
      containers:
      - name: app
        image: nginx:1.21
      - name: sidecar
        image: envoyproxy/envoy@sha256:1234567890abcdef
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: admin-role
  namespace: test-ns
rules:
- apiGroups: [""]
  resources: ["*"]
  verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: wild-verbs
rules:
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["*"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: good-role
  namespace: test-ns
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list"]
`

func TestSupplyChain(t *testing.T) {
	// Parse manifests
	docs := strings.Split(supplyChainTestManifests, "---")
	graph := types.NewResourceGraph()
	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}
		obj := &unstructured.Unstructured{}
		_ = yaml.Unmarshal([]byte(doc), &obj.Object)
		gvk := schema.FromAPIVersionAndKind(obj.GetAPIVersion(), obj.GetKind())
		in := types.ExtractedResource{
			Object:     obj,
			GVK:        gvk,
			SourcePath: "manifests.yaml",
		}
		key := types.ResourceKey{
			GVK:       gvk,
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		}
		graph.Resources[key] = &types.ProcessedResource{
			Original: &in,
		}
	}

	// Create initial chart
	chart := &types.GeneratedChart{
		Templates:  make(map[string]string),
		ValuesYAML: "",
	}

	t.Run("defaults (require-digests=true, least-privilege-rbac=true, default-deny-netpol=true, cosign-verify=false)", func(t *testing.T) {
		var notes []string
		fc := FeatureContext{
			Graph: graph,
			Params: map[string]string{
				"default-deny-netpol":  "true",
				"least-privilege-rbac": "true",
				"require-digests":      "true",
				"cosign-verify":        "false",
				"cosign-key":           "",
			},
			Notes: &notes,
		}

		out, err := applySupplyChainFeature(chart, fc)
		if err != nil {
			t.Fatalf("applySupplyChainFeature failed: %v", err)
		}

		// Check Notes
		sort.Strings(notes)
		expectedNotes := []string{
			`supply-chain: ClusterRole/wild-verbs uses wildcard RBAC permissions; restrict to least privilege`,
			`supply-chain: Deployment/frontend container app image "nginx:1.21" is not pinned to a @sha256 digest`,
			`supply-chain: Role/admin-role uses wildcard RBAC permissions; restrict to least privilege`,
		}

		if len(notes) != len(expectedNotes) {
			t.Errorf("Expected %d notes, got %d:\n%v", len(expectedNotes), len(notes), notes)
		} else {
			for i, exp := range expectedNotes {
				if notes[i] != exp {
					t.Errorf("Note %d mismatch:\nexp: %s\ngot: %s", i, exp, notes[i])
				}
			}
		}

		// Check Templates
		if _, ok := out.Templates["templates/supplychain-default-deny-netpol.yaml"]; !ok {
			t.Errorf("Expected templates/supplychain-default-deny-netpol.yaml to be generated")
		}
		if _, ok := out.Templates["templates/supplychain-cosign-policy.yaml"]; !ok {
			t.Errorf("Expected templates/supplychain-cosign-policy.yaml to be generated")
		}

		// Check Values
		if !strings.Contains(out.ValuesYAML, "supplyChain:") {
			t.Errorf("Expected supplyChain section in values.yaml, got:\n%s", out.ValuesYAML)
		}
		if !strings.Contains(out.ValuesYAML, "defaultDenyNetworkPolicy: true") {
			t.Errorf("Expected defaultDenyNetworkPolicy: true in values.yaml")
		}
		if !strings.Contains(out.ValuesYAML, "cosignVerify: false") {
			t.Errorf("Expected cosignVerify: false in values.yaml")
		}
	})

	t.Run("cosign enabled, rbac and digests disabled", func(t *testing.T) {
		var notes []string
		fc := FeatureContext{
			Graph: graph,
			Params: map[string]string{
				"default-deny-netpol":  "false",
				"least-privilege-rbac": "false",
				"require-digests":      "false",
				"cosign-verify":        "true",
				"cosign-key":           "MY_PUBLIC_KEY",
			},
			Notes: &notes,
		}

		out, err := applySupplyChainFeature(chart, fc)
		if err != nil {
			t.Fatalf("applySupplyChainFeature failed: %v", err)
		}

		if len(notes) != 0 {
			t.Errorf("Expected 0 notes, got: %v", notes)
		}

		// Check Values
		if !strings.Contains(out.ValuesYAML, "defaultDenyNetworkPolicy: false") {
			t.Errorf("Expected defaultDenyNetworkPolicy: false in values.yaml")
		}
		if !strings.Contains(out.ValuesYAML, "cosignVerify: true") {
			t.Errorf("Expected cosignVerify: true in values.yaml")
		}
		if !strings.Contains(out.ValuesYAML, "cosignPublicKey: MY_PUBLIC_KEY") {
			t.Errorf("Expected cosignPublicKey: MY_PUBLIC_KEY in values.yaml")
		}

		// The netpol template is rendered with defaultDenyNetworkPolicy: false in helm, but we just check if template is injected.
		if _, ok := out.Templates["templates/supplychain-default-deny-netpol.yaml"]; !ok {
			t.Errorf("Expected templates/supplychain-default-deny-netpol.yaml to be generated")
		}

		cosignTmpl := out.Templates["templates/supplychain-cosign-policy.yaml"]
		if !strings.Contains(cosignTmpl, "{{- (.Values.supplyChain).cosignPublicKey") {
			t.Errorf("Expected cosignPublicKey in cosign template")
		}
	})
}
