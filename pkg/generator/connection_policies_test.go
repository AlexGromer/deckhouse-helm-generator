package generator

import (
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"strings"
	"testing"
)

func TestApplyConnectionPolicies(t *testing.T) {
	chart := &types.GeneratedChart{
		Name:      "test-chart",
		Templates: make(map[string]string),
	}
	fc := FeatureContext{}

	res, err := applyConnectionPolicies(chart, fc)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	policy, ok := res.Templates["templates/connection-policy.yaml"]
	if !ok {
		t.Fatalf("Expected connection-policy.yaml to be generated")
	}

	if !strings.Contains(policy, "kind: NetworkPolicy") {
		t.Errorf("Expected NetworkPolicy, got: %s", policy)
	}
}
