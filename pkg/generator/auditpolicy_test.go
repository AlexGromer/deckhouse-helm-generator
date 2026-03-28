package generator

import (
	"strings"
	"testing"
)

func TestAuditPolicy_ContainsSecretRule(t *testing.T) {
	policy := GenerateAuditPolicy()

	if !strings.Contains(policy, "level: RequestResponse") {
		t.Error("audit policy must contain RequestResponse level for secrets")
	}
	if !strings.Contains(policy, `resources: ["secrets"]`) {
		t.Error("audit policy must reference secrets resource")
	}
}

func TestAuditPolicy_ContainsRBACRule(t *testing.T) {
	policy := GenerateAuditPolicy()

	if !strings.Contains(policy, "rbac.authorization.k8s.io") {
		t.Error("audit policy must contain RBAC API group")
	}
	if !strings.Contains(policy, "clusterroles") {
		t.Error("audit policy must reference clusterroles")
	}
	if !strings.Contains(policy, "rolebindings") {
		t.Error("audit policy must reference rolebindings")
	}
}

func TestAuditPolicy_ContainsPodExecRule(t *testing.T) {
	policy := GenerateAuditPolicy()

	if !strings.Contains(policy, `pods/exec`) {
		t.Error("audit policy must reference pods/exec")
	}
	if !strings.Contains(policy, `pods/attach`) {
		t.Error("audit policy must reference pods/attach")
	}
}

func TestAuditPolicy_DefaultMetadata(t *testing.T) {
	policy := GenerateAuditPolicy()

	if !strings.Contains(policy, "level: Metadata") {
		t.Error("audit policy must have Metadata as default level")
	}
}

func TestAuditPolicy_ValidYAMLStructure(t *testing.T) {
	policy := GenerateAuditPolicy()

	if !strings.Contains(policy, "apiVersion: audit.k8s.io/v1") {
		t.Error("audit policy must have correct apiVersion")
	}
	if !strings.Contains(policy, "kind: Policy") {
		t.Error("audit policy must have kind: Policy")
	}
	if !strings.Contains(policy, "cluster admin must apply manually") {
		t.Error("audit policy must contain manual-apply warning comment")
	}
}
