package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ============================================================
// Task 4.6 Subtask 1: Pipeline with HPA
// ============================================================

func TestPipeline_DeploymentWithHPA(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	h.WriteInputFile("deployment.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-server
  namespace: default
  labels:
    app.kubernetes.io/name: api-server
spec:
  replicas: 2
  selector:
    matchLabels:
      app.kubernetes.io/name: api-server
  template:
    metadata:
      labels:
        app.kubernetes.io/name: api-server
    spec:
      containers:
        - name: api
          image: api-server:2.0
          ports:
            - containerPort: 8080
          resources:
            requests:
              cpu: "200m"
              memory: "256Mi"
            limits:
              cpu: "1"
              memory: "512Mi"
`)

	h.WriteInputFile("hpa.yaml", `apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: api-server-hpa
  namespace: default
  labels:
    app.kubernetes.io/name: api-server
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: api-server
  minReplicas: 2
  maxReplicas: 10
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "api-app",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// Expect at least 2 resources (Deployment + HPA)
	if len(output.Resources) < 2 {
		t.Fatalf("Expected at least 2 resources, got %d", len(output.Resources))
	}

	// Find HPA resource
	foundHPA := false
	for _, res := range output.Resources {
		if res.Original.Object.GetKind() == "HorizontalPodAutoscaler" {
			foundHPA = true

			// Template should reference the HPA
			if res.TemplateContent == "" {
				t.Error("Expected non-empty HPA template")
			}
			if !strings.Contains(res.TemplateContent, "HorizontalPodAutoscaler") {
				t.Error("Expected HPA kind in template")
			}
			if !strings.Contains(res.TemplateContent, "scaleTargetRef") {
				t.Error("Expected scaleTargetRef in HPA template")
			}

			// Values should contain HPA config
			if res.Values["minReplicas"] == nil {
				t.Error("Expected minReplicas in HPA values")
			}
			if res.Values["maxReplicas"] == nil {
				t.Error("Expected maxReplicas in HPA values")
			}

			break
		}
	}
	if !foundHPA {
		t.Error("HPA resource not found in pipeline output")
	}
}

// ============================================================
// Task 4.6 Subtask 2: Pipeline with PDB
// ============================================================

func TestPipeline_DeploymentWithPDB(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	h.WriteInputFile("deployment.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
  namespace: default
  labels:
    app.kubernetes.io/name: frontend
spec:
  replicas: 3
  selector:
    matchLabels:
      app.kubernetes.io/name: frontend
  template:
    metadata:
      labels:
        app.kubernetes.io/name: frontend
    spec:
      containers:
        - name: web
          image: frontend:1.5
          ports:
            - containerPort: 3000
`)

	h.WriteInputFile("pdb.yaml", `apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: frontend-pdb
  namespace: default
  labels:
    app.kubernetes.io/name: frontend
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app.kubernetes.io/name: frontend
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "frontend-app",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	if len(output.Resources) < 2 {
		t.Fatalf("Expected at least 2 resources, got %d", len(output.Resources))
	}

	foundPDB := false
	for _, res := range output.Resources {
		if res.Original.Object.GetKind() == "PodDisruptionBudget" {
			foundPDB = true

			if res.TemplateContent == "" {
				t.Error("Expected non-empty PDB template")
			}
			if !strings.Contains(res.TemplateContent, "PodDisruptionBudget") {
				t.Error("Expected PDB kind in template")
			}

			// PDB should have selector matching Deployment
			if res.Values["selector"] == nil {
				t.Error("Expected selector in PDB values")
			}
			if res.Values["minAvailable"] == nil {
				t.Error("Expected minAvailable in PDB values")
			}

			break
		}
	}
	if !foundPDB {
		t.Error("PDB resource not found in pipeline output")
	}
}

// ============================================================
// Task 4.6 Subtask 6: Complex scenario — all together
// ============================================================

func TestPipeline_DeploymentWithHPAAndPDB(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	h.WriteInputFile("deployment.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: backend
  namespace: default
  labels:
    app.kubernetes.io/name: backend
spec:
  replicas: 3
  selector:
    matchLabels:
      app.kubernetes.io/name: backend
  template:
    metadata:
      labels:
        app.kubernetes.io/name: backend
    spec:
      containers:
        - name: api
          image: backend:3.0
          ports:
            - containerPort: 8080
          resources:
            requests:
              cpu: "500m"
              memory: "512Mi"
`)

	h.WriteInputFile("hpa.yaml", `apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: backend-hpa
  namespace: default
  labels:
    app.kubernetes.io/name: backend
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: backend
  minReplicas: 3
  maxReplicas: 20
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 60
`)

	h.WriteInputFile("pdb.yaml", `apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: backend-pdb
  namespace: default
  labels:
    app.kubernetes.io/name: backend
spec:
  maxUnavailable: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: backend
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "backend-app",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// Expect 3 resources: Deployment + HPA + PDB
	if len(output.Resources) < 3 {
		t.Fatalf("Expected at least 3 resources, got %d", len(output.Resources))
	}

	kinds := make(map[string]bool)
	for _, res := range output.Resources {
		kinds[res.Original.Object.GetKind()] = true
	}

	for _, expected := range []string{"Deployment", "HorizontalPodAutoscaler", "PodDisruptionBudget"} {
		if !kinds[expected] {
			t.Errorf("Expected %s in pipeline output", expected)
		}
	}

	// Verify chart structure
	if len(output.Charts) == 0 {
		t.Fatal("Expected at least one generated chart")
	}
	chartDir := output.OutputDir
	entries, _ := os.ReadDir(chartDir)
	for _, e := range entries {
		if e.IsDir() {
			ValidateChartStructure(t, filepath.Join(chartDir, e.Name()))
			break
		}
	}
}
