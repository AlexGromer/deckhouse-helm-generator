package integration

import (
	"os"
	"strings"
	"testing"
)

// ============================================================
// Task 4.6 Subtask 3: Pipeline with NetworkPolicy
// ============================================================

func TestPipeline_DeploymentWithNetworkPolicy(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	h.WriteInputFile("deployment.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: db-app
  namespace: default
  labels:
    app.kubernetes.io/name: db-app
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: db-app
  template:
    metadata:
      labels:
        app.kubernetes.io/name: db-app
    spec:
      containers:
        - name: postgres
          image: postgres:15
          ports:
            - containerPort: 5432
`)

	h.WriteInputFile("service.yaml", `apiVersion: v1
kind: Service
metadata:
  name: db-app
  namespace: default
  labels:
    app.kubernetes.io/name: db-app
spec:
  type: ClusterIP
  selector:
    app.kubernetes.io/name: db-app
  ports:
    - name: postgres
      port: 5432
      targetPort: 5432
`)

	h.WriteInputFile("networkpolicy.yaml", `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: db-netpol
  namespace: default
  labels:
    app.kubernetes.io/name: db-app
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: db-app
  policyTypes:
    - Ingress
  ingress:
    - from:
        - podSelector:
            matchLabels:
              role: backend
      ports:
        - protocol: TCP
          port: 5432
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "db-app",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// Expect at least 3 resources: Deployment + Service + NetworkPolicy
	if len(output.Resources) < 3 {
		t.Fatalf("Expected at least 3 resources, got %d", len(output.Resources))
	}

	foundNetPol := false
	for _, res := range output.Resources {
		if res.Original.Object.GetKind() == "NetworkPolicy" {
			foundNetPol = true

			if res.TemplateContent == "" {
				t.Error("Expected non-empty NetworkPolicy template")
			}
			if !strings.Contains(res.TemplateContent, "NetworkPolicy") {
				t.Error("Expected NetworkPolicy kind in template")
			}
			if !strings.Contains(res.TemplateContent, "podSelector") {
				t.Error("Expected podSelector in NetworkPolicy template")
			}
			if !strings.Contains(res.TemplateContent, "ingress") {
				t.Error("Expected ingress in NetworkPolicy template")
			}

			// Values should contain policy config
			if res.Values["podSelector"] == nil {
				t.Error("Expected podSelector in NetworkPolicy values")
			}
			if res.Values["policyTypes"] == nil {
				t.Error("Expected policyTypes in NetworkPolicy values")
			}
			if res.Values["ingress"] == nil {
				t.Error("Expected ingress rules in NetworkPolicy values")
			}

			break
		}
	}
	if !foundNetPol {
		t.Error("NetworkPolicy resource not found in pipeline output")
	}
}

// ============================================================
// Task 4.6 Subtask 6 (part): Complex with NetworkPolicy
// ============================================================

func TestPipeline_CompleteSecurityStack(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	h.WriteInputFile("deployment.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: secure-app
  namespace: default
  labels:
    app.kubernetes.io/name: secure-app
spec:
  replicas: 3
  selector:
    matchLabels:
      app.kubernetes.io/name: secure-app
  template:
    metadata:
      labels:
        app.kubernetes.io/name: secure-app
    spec:
      containers:
        - name: app
          image: secure-app:1.0
          ports:
            - containerPort: 8080
          resources:
            requests:
              cpu: "200m"
              memory: "256Mi"
`)

	h.WriteInputFile("hpa.yaml", `apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: secure-app-hpa
  namespace: default
  labels:
    app.kubernetes.io/name: secure-app
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: secure-app
  minReplicas: 3
  maxReplicas: 15
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 75
`)

	h.WriteInputFile("pdb.yaml", `apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: secure-app-pdb
  namespace: default
  labels:
    app.kubernetes.io/name: secure-app
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app.kubernetes.io/name: secure-app
`)

	h.WriteInputFile("networkpolicy.yaml", `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: secure-app-netpol
  namespace: default
  labels:
    app.kubernetes.io/name: secure-app
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: secure-app
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - podSelector:
            matchLabels:
              role: frontend
      ports:
        - protocol: TCP
          port: 8080
  egress:
    - to:
        - podSelector:
            matchLabels:
              role: database
      ports:
        - protocol: TCP
          port: 5432
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "secure-stack",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// Expect 4 resources: Deployment + HPA + PDB + NetworkPolicy
	if len(output.Resources) < 4 {
		t.Fatalf("Expected at least 4 resources, got %d", len(output.Resources))
	}

	kinds := make(map[string]bool)
	for _, res := range output.Resources {
		kinds[res.Original.Object.GetKind()] = true
	}

	for _, expected := range []string{"Deployment", "HorizontalPodAutoscaler", "PodDisruptionBudget", "NetworkPolicy"} {
		if !kinds[expected] {
			t.Errorf("Expected %s in pipeline output", expected)
		}
	}
}
