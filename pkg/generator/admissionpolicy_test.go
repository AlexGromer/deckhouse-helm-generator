package generator

import (
	"strings"
	"testing"
)

func TestAdmission_KyvernoPolicies(t *testing.T) {
	policies := GenerateAdmissionPolicies("kyverno")

	expectedFiles := []string{
		"require-labels.yaml",
		"deny-privileged.yaml",
		"require-resources.yaml",
		"require-probes.yaml",
	}
	for _, f := range expectedFiles {
		content, ok := policies[f]
		if !ok {
			t.Errorf("missing kyverno policy file: %s", f)
			continue
		}
		if !strings.Contains(content, "apiVersion: kyverno.io/v1") {
			t.Errorf("%s: missing kyverno apiVersion", f)
		}
		if !strings.Contains(content, "kind: ClusterPolicy") {
			t.Errorf("%s: missing ClusterPolicy kind", f)
		}
		if !strings.Contains(content, "validationFailureAction: audit") {
			t.Errorf("%s: validationFailureAction must be 'audit'", f)
		}
	}

	if len(policies) != 4 {
		t.Errorf("expected 4 kyverno policies, got %d", len(policies))
	}
}

func TestAdmission_KyvernoPolicyContent(t *testing.T) {
	policies := GenerateAdmissionPolicies("kyverno")

	if !strings.Contains(policies["deny-privileged.yaml"], "privileged") {
		t.Error("deny-privileged policy must reference 'privileged'")
	}
	if !strings.Contains(policies["require-labels.yaml"], "app.kubernetes.io/name") {
		t.Error("require-labels policy must reference standard k8s labels")
	}
	if !strings.Contains(policies["require-resources.yaml"], "resources") {
		t.Error("require-resources policy must reference resource requirements")
	}
	if !strings.Contains(policies["require-probes.yaml"], "readinessProbe") {
		t.Error("require-probes policy must reference readinessProbe")
	}
}

func TestAdmission_OPAPolicies(t *testing.T) {
	policies := GenerateAdmissionPolicies("opa")

	expectedFiles := []string{
		"require-labels.yaml",
		"deny-privileged.yaml",
		"require-resources.yaml",
		"require-probes.yaml",
	}
	for _, f := range expectedFiles {
		content, ok := policies[f]
		if !ok {
			t.Errorf("missing OPA policy file: %s", f)
			continue
		}
		if !strings.Contains(content, "kind: ConstraintTemplate") {
			t.Errorf("%s: missing ConstraintTemplate", f)
		}
		if !strings.Contains(content, "templates.gatekeeper.sh") {
			t.Errorf("%s: missing gatekeeper apiVersion", f)
		}
		if !strings.Contains(content, "enforcementAction: dryrun") {
			t.Errorf("%s: enforcementAction must be 'dryrun'", f)
		}
		// Each OPA file must have both ConstraintTemplate and Constraint
		if !strings.Contains(content, "---") {
			t.Errorf("%s: must contain document separator for Constraint", f)
		}
	}

	if len(policies) != 4 {
		t.Errorf("expected 4 OPA policies, got %d", len(policies))
	}
}

func TestAdmission_OPARegoContent(t *testing.T) {
	policies := GenerateAdmissionPolicies("opa")

	if !strings.Contains(policies["deny-privileged.yaml"], "privileged == true") {
		t.Error("OPA deny-privileged must check privileged == true in rego")
	}
	if !strings.Contains(policies["require-labels.yaml"], "missing") {
		t.Error("OPA require-labels must check for missing labels")
	}
}

func TestAdmission_UnknownEngine(t *testing.T) {
	policies := GenerateAdmissionPolicies("unknown")
	if len(policies) != 0 {
		t.Error("unknown engine should return empty map")
	}
}
