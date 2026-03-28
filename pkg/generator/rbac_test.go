package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

func TestRBAC_GeneratesServiceAccount(t *testing.T) {
	chart := makeChartWithDeployment("myapp")

	result := GenerateRBACTemplates(chart)

	sa, ok := result["templates/myapp-serviceaccount.yaml"]
	if !ok {
		t.Fatal("expected ServiceAccount template to be generated")
	}

	if !strings.Contains(sa, "kind: ServiceAccount") {
		t.Error("ServiceAccount template missing kind")
	}
	if !strings.Contains(sa, "name: {{ include \"myapp.fullname\" . }}-sa") {
		t.Error("ServiceAccount template missing templated name")
	}
	if !strings.Contains(sa, "namespace: {{ .Release.Namespace }}") {
		t.Error("ServiceAccount template missing namespace")
	}
}

func TestRBAC_GeneratesRole(t *testing.T) {
	chart := makeChartWithDeployment("myapp")

	result := GenerateRBACTemplates(chart)

	role, ok := result["templates/myapp-role.yaml"]
	if !ok {
		t.Fatal("expected Role template to be generated")
	}

	if !strings.Contains(role, "kind: Role") {
		t.Error("Role template missing kind")
	}
	if !strings.Contains(role, "name: {{ include \"myapp.fullname\" . }}-role") {
		t.Error("Role template missing templated name")
	}
	// Least-privilege: only get, list, watch
	for _, verb := range []string{"get", "list", "watch"} {
		if !strings.Contains(role, verb) {
			t.Errorf("Role template missing least-privilege verb: %s", verb)
		}
	}
	// Should NOT have write verbs in the rules section
	for _, verb := range []string{"\"create\"", "\"delete\"", "\"update\"", "\"patch\""} {
		if strings.Contains(role, verb) {
			t.Errorf("Role template should not contain write verb by default: %s", verb)
		}
	}
}

func TestRBAC_GeneratesRoleBinding(t *testing.T) {
	chart := makeChartWithDeployment("myapp")

	result := GenerateRBACTemplates(chart)

	rb, ok := result["templates/myapp-rolebinding.yaml"]
	if !ok {
		t.Fatal("expected RoleBinding template to be generated")
	}

	if !strings.Contains(rb, "kind: RoleBinding") {
		t.Error("RoleBinding template missing kind")
	}
	if !strings.Contains(rb, "name: {{ include \"myapp.fullname\" . }}-rolebinding") {
		t.Error("RoleBinding template missing templated name")
	}
	// References the Role
	if !strings.Contains(rb, "kind: Role") {
		t.Error("RoleBinding should reference Role kind")
	}
	// References the ServiceAccount
	if !strings.Contains(rb, "kind: ServiceAccount") {
		t.Error("RoleBinding should reference ServiceAccount kind")
	}
}

func TestRBAC_AutomountFalse(t *testing.T) {
	chart := makeChartWithDeployment("myapp")

	result := GenerateRBACTemplates(chart)

	sa, ok := result["templates/myapp-serviceaccount.yaml"]
	if !ok {
		t.Fatal("expected ServiceAccount template")
	}

	if !strings.Contains(sa, "automountServiceAccountToken: false") {
		t.Error("ServiceAccount should set automountServiceAccountToken: false")
	}
}

func TestRBAC_EmptyChart(t *testing.T) {
	chart := &types.GeneratedChart{
		Name:       "empty",
		Templates:  map[string]string{},
		ValuesYAML: "{}",
	}

	result := GenerateRBACTemplates(chart)
	if len(result) != 0 {
		t.Errorf("expected no RBAC templates for chart without workloads, got %d", len(result))
	}
}

func TestRBAC_StatefulSet(t *testing.T) {
	chart := &types.GeneratedChart{
		Name: "db",
		Templates: map[string]string{
			"templates/statefulset.yaml": "apiVersion: apps/v1\nkind: StatefulSet\nmetadata:\n  name: db\n",
		},
		ValuesYAML: "{}",
	}

	result := GenerateRBACTemplates(chart)
	if len(result) == 0 {
		t.Fatal("expected RBAC templates for StatefulSet")
	}
	if _, ok := result["templates/db-serviceaccount.yaml"]; !ok {
		t.Error("expected ServiceAccount for StatefulSet workload")
	}
}

func TestRBAC_MultipleWorkloads(t *testing.T) {
	chart := &types.GeneratedChart{
		Name: "multi",
		Templates: map[string]string{
			"templates/deployment-web.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n",
			"templates/deployment-api.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: api\n",
		},
		ValuesYAML: "{}",
	}

	result := GenerateRBACTemplates(chart)
	// Should generate RBAC for the chart (not per-deployment)
	if _, ok := result["templates/multi-serviceaccount.yaml"]; !ok {
		t.Error("expected ServiceAccount for chart with multiple workloads")
	}
}

// makeChartWithDeployment creates a minimal chart containing a Deployment template.
func makeChartWithDeployment(name string) *types.GeneratedChart {
	return &types.GeneratedChart{
		Name: name,
		Templates: map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: " + name + "\n",
		},
		ValuesYAML: "{}",
	}
}
