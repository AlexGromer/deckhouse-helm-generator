package operationpolicy

import (
	"os"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestLoad(t *testing.T) {
	content := `---
apiVersion: deckhouse.io/v1alpha1
kind: OperationPolicy
metadata:
  name: test-policy
spec:
  allowedRepos:
    - registry.example.com/shop/
  requiredProbes:
    - livenessProbe
    - readinessProbe
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: some-cm
---
garbage
`
	tmpfile, err := os.CreateTemp("", "policy-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	// 1. Should fail on garbage YAML
	_, err = Load([]string{tmpfile.Name()})
	if err == nil {
		t.Errorf("Expected error on invalid YAML")
	}

	// Fix the content
	content = `---
apiVersion: deckhouse.io/v1alpha1
kind: OperationPolicy
metadata:
  name: test-policy
spec:
  allowedRepos:
    - registry.example.com/shop/
  requiredProbes:
    - livenessProbe
    - readinessProbe
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: some-cm
`
	if err := os.WriteFile(tmpfile.Name(), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Should parse valid YAML and ignore ConfigMap
	policies, err := Load([]string{tmpfile.Name()})
	if err != nil {
		t.Fatal(err)
	}

	if len(policies) != 1 {
		t.Fatalf("Expected 1 policy, got %d", len(policies))
	}

	p := policies[0]
	if p.Name != "test-policy" {
		t.Errorf("Expected name test-policy, got %s", p.Name)
	}
	if len(p.Spec.AllowedRepos) != 1 || p.Spec.AllowedRepos[0] != "registry.example.com/shop/" {
		t.Errorf("Expected allowedRepos [registry.example.com/shop/], got %v", p.Spec.AllowedRepos)
	}
	if len(p.Spec.RequiredProbes) != 2 || p.Spec.RequiredProbes[0] != "livenessProbe" {
		t.Errorf("Expected requiredProbes [livenessProbe, readinessProbe], got %v", p.Spec.RequiredProbes)
	}
}

func TestCheck(t *testing.T) {
	ptrInt32 := func(i int32) *int32 { return &i }
	ptrString := func(s string) *string { return &s }

	p := Policy{
		Name: "test-policy",
		Spec: PolicySpec{
			Match: Match{
				LabelSelector: map[string]string{"app": "shop"},
			},
			AllowedRepos: []string{"registry.example.com/shop/"},
			RequiredResources: &RequiredResources{
				Limits: []string{"cpu", "memory"},
			},
			DisallowedImageTags: []string{"latest", "dev"},
			RequiredProbes:      []string{"livenessProbe"},
			RequiredLabels: &RequiredLabels{
				WatchKinds: []string{"Deployment"},
				Labels: []LabelRule{
					{Key: "owner", AllowedRegex: "^team-.*"},
				},
			},
			MaxRevisionHistoryLimit: ptrInt32(5),
			ImagePullPolicy:         ptrString("Always"),
			PriorityClassNames:      []string{"high-priority"},
			IngressClassNames:       []string{"nginx"},
			StorageClassNames:       []string{"fast"},
			ReplicaLimits: &ReplicaLimits{
				MinReplicas: ptrInt32(2),
				MaxReplicas: ptrInt32(10),
			},
		},
	}

	// Valid object that should have no findings
	validObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name": "orders",
				"labels": map[string]interface{}{
					"app":   "shop",
					"owner": "team-a",
				},
			},
			"spec": map[string]interface{}{
				"replicas":             int64(3),
				"revisionHistoryLimit": int64(3),
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"priorityClassName": "high-priority",
						"containers": []interface{}{
							map[string]interface{}{
								"name":            "app",
								"image":           "registry.example.com/shop/app:v1",
								"imagePullPolicy": "Always",
								"resources": map[string]interface{}{
									"limits": map[string]interface{}{
										"cpu":    "100m",
										"memory": "128Mi",
									},
								},
								"livenessProbe": map[string]interface{}{},
							},
						},
					},
				},
			},
		},
	}

	findings := Check(p, validObj)
	if len(findings) > 0 {
		t.Errorf("Expected 0 findings, got %d: %v", len(findings), findings)
	}

	// Invalid object violating many rules
	invalidObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name": "orders",
				"labels": map[string]interface{}{
					"app":   "shop",
					"owner": "guest",
				},
			},
			"spec": map[string]interface{}{
				"replicas":             int64(1), // < minReplicas (2)
				"revisionHistoryLimit": int64(6), // > maxRevisionHistoryLimit (5)
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"priorityClassName": "low-priority",
						"containers": []interface{}{
							map[string]interface{}{
								"name":            "app",
								"image":           "docker.io/nginx:latest", // disallowed repo and tag
								"imagePullPolicy": "IfNotPresent",           // required Always
								"resources": map[string]interface{}{
									"limits": map[string]interface{}{
										"cpu": "100m", // missing memory
									},
								},
								// missing livenessProbe
							},
						},
					},
				},
			},
		},
	}

	findings = Check(p, invalidObj)
	if len(findings) == 0 {
		t.Fatalf("Expected findings for invalid object, got 0")
	}

	hasRule := func(rule string) bool {
		for _, f := range findings {
			if f.Rule == rule {
				return true
			}
		}
		return false
	}

	expectedRules := []string{
		"allowedRepos",
		"requiredResources",
		"disallowedImageTags",
		"requiredProbes",
		"requiredLabels",
		"maxRevisionHistoryLimit",
		"imagePullPolicy",
		"priorityClassNames",
		"replicaLimits",
	}

	for _, rule := range expectedRules {
		if !hasRule(rule) {
			t.Errorf("Expected finding for rule %s, got none. Findings: %v", rule, findings)
		}
	}

	// Check LabelSelector mismatch (should skip)
	unmatchedObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name": "orders",
				"labels": map[string]interface{}{
					"app": "other",
				},
			},
		},
	}
	if len(Check(p, unmatchedObj)) > 0 {
		t.Errorf("Expected 0 findings for unmatched object")
	}

	// Check Ingress
	ingressObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "Ingress",
			"metadata": map[string]interface{}{
				"name": "web",
				"labels": map[string]interface{}{
					"app": "shop",
				},
			},
			"spec": map[string]interface{}{
				"ingressClassName": "haproxy",
			},
		},
	}
	findings = Check(p, ingressObj)
	if !hasRule("ingressClassNames") {
		t.Errorf("Expected ingressClassNames finding")
	}
}

func TestRenderMarkdownSection(t *testing.T) {
	findings := []Finding{
		{ObjectName: "Deployment/orders", PolicyName: "test", Rule: "requiredProbes", Message: "container app has no livenessProbe"},
		{ObjectName: "Deployment/orders", PolicyName: "test", Rule: "unsupported", Message: "not checked by dhg"},
	}
	md := RenderMarkdownSection(findings)
	if !strings.Contains(md, "| Deployment/orders | test | requiredProbes | container app has no livenessProbe |") {
		t.Errorf("Missing expected finding row in Markdown:\n%s", md)
	}
	if !strings.Contains(md, "*Some policy rules are not checked by dhg.*") {
		t.Errorf("Missing unsupported note in Markdown:\n%s", md)
	}
}
