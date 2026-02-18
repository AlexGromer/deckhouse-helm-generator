package integration

import (
	"os"
	"strings"
	"testing"
)

// ============================================================
// Task 5.1-5.4: RBAC Chain Integration Test
// ============================================================

func TestPipeline_RBACChain(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	h.WriteInputFile("serviceaccount.yaml", `apiVersion: v1
kind: ServiceAccount
metadata:
  name: app-sa
  namespace: default
  labels:
    app.kubernetes.io/name: myapp
`)

	h.WriteInputFile("role.yaml", `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: app-role
  namespace: default
  labels:
    app.kubernetes.io/name: myapp
rules:
  - apiGroups: [""]
    resources: ["pods", "services"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["apps"]
    resources: ["deployments"]
    verbs: ["get", "list"]
`)

	h.WriteInputFile("rolebinding.yaml", `apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: app-rolebinding
  namespace: default
  labels:
    app.kubernetes.io/name: myapp
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: app-role
subjects:
  - kind: ServiceAccount
    name: app-sa
    namespace: default
`)

	h.WriteInputFile("clusterrole.yaml", `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: app-clusterrole
  labels:
    app.kubernetes.io/name: myapp
rules:
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list"]
`)

	h.WriteInputFile("clusterrolebinding.yaml", `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: app-clusterrolebinding
  labels:
    app.kubernetes.io/name: myapp
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: app-clusterrole
subjects:
  - kind: ServiceAccount
    name: app-sa
    namespace: default
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "rbac-app",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// Expect 5 resources: SA + Role + RoleBinding + ClusterRole + ClusterRoleBinding
	if len(output.Resources) < 5 {
		t.Fatalf("Expected at least 5 resources, got %d", len(output.Resources))
	}

	kinds := make(map[string]bool)
	for _, res := range output.Resources {
		kinds[res.Original.Object.GetKind()] = true
	}

	for _, expected := range []string{"ServiceAccount", "Role", "RoleBinding", "ClusterRole", "ClusterRoleBinding"} {
		if !kinds[expected] {
			t.Errorf("Expected %s in pipeline output", expected)
		}
	}

	// Verify RBAC templates contain expected content
	for _, res := range output.Resources {
		kind := res.Original.Object.GetKind()
		switch kind {
		case "Role":
			if !strings.Contains(res.TemplateContent, "rules") {
				t.Error("Role template should contain rules")
			}
		case "ClusterRole":
			if !strings.Contains(res.TemplateContent, "rules") {
				t.Error("ClusterRole template should contain rules")
			}
		case "RoleBinding":
			if !strings.Contains(res.TemplateContent, "roleRef") {
				t.Error("RoleBinding template should contain roleRef")
			}
			if !strings.Contains(res.TemplateContent, "subjects") {
				t.Error("RoleBinding template should contain subjects")
			}
		case "ClusterRoleBinding":
			if !strings.Contains(res.TemplateContent, "roleRef") {
				t.Error("ClusterRoleBinding template should contain roleRef")
			}
			if !strings.Contains(res.TemplateContent, "subjects") {
				t.Error("ClusterRoleBinding template should contain subjects")
			}
		}
	}
}

func TestPipeline_RBACWithDeployment(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	h.WriteInputFile("deployment.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: webapp
  namespace: default
  labels:
    app.kubernetes.io/name: webapp
spec:
  replicas: 2
  selector:
    matchLabels:
      app.kubernetes.io/name: webapp
  template:
    metadata:
      labels:
        app.kubernetes.io/name: webapp
    spec:
      serviceAccountName: webapp-sa
      containers:
        - name: web
          image: webapp:1.0
          ports:
            - containerPort: 8080
`)

	h.WriteInputFile("serviceaccount.yaml", `apiVersion: v1
kind: ServiceAccount
metadata:
  name: webapp-sa
  namespace: default
  labels:
    app.kubernetes.io/name: webapp
`)

	h.WriteInputFile("role.yaml", `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: webapp-role
  namespace: default
  labels:
    app.kubernetes.io/name: webapp
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list"]
`)

	h.WriteInputFile("rolebinding.yaml", `apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: webapp-rolebinding
  namespace: default
  labels:
    app.kubernetes.io/name: webapp
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: webapp-role
subjects:
  - kind: ServiceAccount
    name: webapp-sa
    namespace: default
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "webapp",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// Expect 4 resources: Deployment + SA + Role + RoleBinding
	if len(output.Resources) < 4 {
		t.Fatalf("Expected at least 4 resources, got %d", len(output.Resources))
	}

	kinds := make(map[string]bool)
	for _, res := range output.Resources {
		kinds[res.Original.Object.GetKind()] = true
	}

	for _, expected := range []string{"Deployment", "ServiceAccount", "Role", "RoleBinding"} {
		if !kinds[expected] {
			t.Errorf("Expected %s in pipeline output", expected)
		}
	}
}
