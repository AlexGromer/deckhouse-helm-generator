package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// generateDeploymentYAML returns a Deployment manifest with a unique name.
func generateDeploymentYAML(index int) string {
	return fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: app-%d
  namespace: bench
  labels:
    app.kubernetes.io/name: app-%d
spec:
  replicas: 2
  selector:
    matchLabels:
      app.kubernetes.io/name: app-%d
  template:
    metadata:
      labels:
        app.kubernetes.io/name: app-%d
    spec:
      containers:
        - name: main
          image: registry.example.com/app-%d:1.0
          ports:
            - containerPort: 8080
          resources:
            requests:
              cpu: "100m"
              memory: "128Mi"
            limits:
              cpu: "500m"
              memory: "512Mi"
`, index, index, index, index, index)
}

// generateServiceYAML returns a Service manifest with a unique name.
func generateServiceYAML(index int) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Service
metadata:
  name: app-%d
  namespace: bench
  labels:
    app.kubernetes.io/name: app-%d
spec:
  type: ClusterIP
  selector:
    app.kubernetes.io/name: app-%d
  ports:
    - name: http
      port: 8080
      targetPort: 8080
`, index, index, index)
}

// generateConfigMapYAML returns a ConfigMap manifest with a unique name.
func generateConfigMapYAML(index int) string {
	return fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: app-%d-config
  namespace: bench
  labels:
    app.kubernetes.io/name: app-%d
data:
  APP_ENV: production
  APP_PORT: "8080"
  LOG_LEVEL: info
`, index, index)
}

// generateSecretYAML returns a Secret manifest with a unique name.
func generateSecretYAML(index int) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: app-%d-secret
  namespace: bench
  labels:
    app.kubernetes.io/name: app-%d
type: Opaque
data:
  password: Y2hhbmdlbWU=
`, index, index)
}

// generateIngressYAML returns an Ingress manifest with a unique name.
func generateIngressYAML(index int) string {
	return fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: app-%d
  namespace: bench
  labels:
    app.kubernetes.io/name: app-%d
spec:
  ingressClassName: nginx
  rules:
    - host: app-%d.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: app-%d
                port:
                  number: 8080
`, index, index, index, index)
}

// setupBenchmarkDir creates a temp directory with n resource sets.
// Each set contains: Deployment, Service, ConfigMap, Secret, Ingress (5 resources per set).
// Total resources = n * 5.
func setupBenchmarkDir(b *testing.B, n int) string {
	b.Helper()

	dir, err := os.MkdirTemp("", "dhg-bench-*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}

	for i := 0; i < n; i++ {
		files := map[string]string{
			fmt.Sprintf("deployment-%d.yaml", i): generateDeploymentYAML(i),
			fmt.Sprintf("service-%d.yaml", i):    generateServiceYAML(i),
			fmt.Sprintf("configmap-%d.yaml", i):   generateConfigMapYAML(i),
			fmt.Sprintf("secret-%d.yaml", i):      generateSecretYAML(i),
			fmt.Sprintf("ingress-%d.yaml", i):     generateIngressYAML(i),
		}
		for name, content := range files {
			path := filepath.Join(dir, name)
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				b.Fatalf("failed to write %s: %v", name, err)
			}
		}
	}

	return dir
}

// BenchmarkPipeline_10Resources benchmarks the full pipeline with 10 resource sets (50 resources).
func BenchmarkPipeline_10Resources(b *testing.B) {
	dir := setupBenchmarkDir(b, 10)
	defer os.RemoveAll(dir)

	opts := PipelineOptions{
		ChartName:    "bench-10",
		ChartVersion: "1.0.0",
		Namespace:    "bench",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		output, err := ExecutePipeline(dir, opts)
		if err != nil {
			b.Fatalf("pipeline failed: %v", err)
		}
		os.RemoveAll(output.OutputDir)
	}
}

// BenchmarkPipeline_100Resources benchmarks the full pipeline with 100 resource sets (500 resources).
func BenchmarkPipeline_100Resources(b *testing.B) {
	dir := setupBenchmarkDir(b, 100)
	defer os.RemoveAll(dir)

	opts := PipelineOptions{
		ChartName:    "bench-100",
		ChartVersion: "1.0.0",
		Namespace:    "bench",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		output, err := ExecutePipeline(dir, opts)
		if err != nil {
			b.Fatalf("pipeline failed: %v", err)
		}
		os.RemoveAll(output.OutputDir)
	}
}

// BenchmarkPipeline_1000Resources benchmarks the full pipeline with 1000 resource sets (5000 resources).
func BenchmarkPipeline_1000Resources(b *testing.B) {
	dir := setupBenchmarkDir(b, 1000)
	defer os.RemoveAll(dir)

	opts := PipelineOptions{
		ChartName:    "bench-1000",
		ChartVersion: "1.0.0",
		Namespace:    "bench",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		output, err := ExecutePipeline(dir, opts)
		if err != nil {
			b.Fatalf("pipeline failed: %v", err)
		}
		os.RemoveAll(output.OutputDir)
	}
}

// BenchmarkProcessorOnly_1000Resources benchmarks only the processor stage
// (no file I/O, no analysis, no generation) with 1000 resource sets.
func BenchmarkProcessorOnly_1000Resources(b *testing.B) {
	dir := setupBenchmarkDir(b, 1000)
	defer os.RemoveAll(dir)

	// Pre-run extraction once to isolate processor performance.
	opts := PipelineOptions{
		ChartName:    "bench-proc",
		ChartVersion: "1.0.0",
		Namespace:    "bench",
	}

	// Warm up — just verify it works.
	output, err := ExecutePipeline(dir, opts)
	if err != nil {
		b.Fatalf("warmup pipeline failed: %v", err)
	}
	os.RemoveAll(output.OutputDir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		output, err := ExecutePipeline(dir, opts)
		if err != nil {
			b.Fatalf("pipeline failed: %v", err)
		}
		os.RemoveAll(output.OutputDir)
	}
}
