package integration

import (
	"os"
	"strings"
	"testing"

	sigsyaml "sigs.k8s.io/yaml"
)

// ============================================================
// Subtask 1: Multi-tier app (3 deployments, 3 services)
// ============================================================

func TestFullStack_MultiTierApp(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	// Frontend
	h.WriteInputFile("frontend-deployment.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
  namespace: default
  labels:
    app.kubernetes.io/name: frontend
spec:
  replicas: 2
  selector:
    matchLabels:
      app.kubernetes.io/name: frontend
  template:
    metadata:
      labels:
        app.kubernetes.io/name: frontend
    spec:
      containers:
        - name: frontend
          image: frontend:1.0
          ports:
            - containerPort: 3000
`)
	h.WriteInputFile("frontend-service.yaml", `apiVersion: v1
kind: Service
metadata:
  name: frontend
  namespace: default
  labels:
    app.kubernetes.io/name: frontend
spec:
  type: ClusterIP
  selector:
    app.kubernetes.io/name: frontend
  ports:
    - name: http
      port: 80
      targetPort: 3000
`)

	// Backend
	h.WriteInputFile("backend-deployment.yaml", `apiVersion: apps/v1
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
        - name: backend
          image: backend:2.0
          ports:
            - containerPort: 8080
`)
	h.WriteInputFile("backend-service.yaml", `apiVersion: v1
kind: Service
metadata:
  name: backend
  namespace: default
  labels:
    app.kubernetes.io/name: backend
spec:
  type: ClusterIP
  selector:
    app.kubernetes.io/name: backend
  ports:
    - name: http
      port: 80
      targetPort: 8080
`)

	// Database
	h.WriteInputFile("database-deployment.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: database
  namespace: default
  labels:
    app.kubernetes.io/name: database
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: database
  template:
    metadata:
      labels:
        app.kubernetes.io/name: database
    spec:
      containers:
        - name: postgres
          image: postgres:15
          ports:
            - containerPort: 5432
`)
	h.WriteInputFile("database-service.yaml", `apiVersion: v1
kind: Service
metadata:
  name: database
  namespace: default
  labels:
    app.kubernetes.io/name: database
spec:
  type: ClusterIP
  selector:
    app.kubernetes.io/name: database
  ports:
    - name: tcp
      port: 5432
      targetPort: 5432
`)

	opts := PipelineOptions{
		ChartName:    "fullstack",
		ChartVersion: "1.0.0",
		AppVersion:   "1.0.0",
	}

	output, err := ExecutePipeline(h.InputDir, opts)
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// ── 1 chart with 6 templates ──
	if len(output.Charts) != 1 {
		t.Fatalf("Expected 1 chart, got %d", len(output.Charts))
	}
	chart := output.Charts[0]

	// 6 resources: 3 Deployments + 3 Services
	if len(output.Resources) != 6 {
		t.Errorf("Expected 6 resources, got %d", len(output.Resources))
	}

	// 6 templates (3 deployment + 3 service)
	if len(chart.Templates) < 6 {
		t.Errorf("Expected at least 6 templates, got %d", len(chart.Templates))
		for path := range chart.Templates {
			t.Logf("  template: %s", path)
		}
	}

	// 3 groups (frontend, backend, database)
	if len(output.Graph.Groups) != 3 {
		t.Errorf("Expected 3 groups, got %d", len(output.Graph.Groups))
		for _, g := range output.Graph.Groups {
			t.Logf("  group: %s (%d resources)", g.Name, len(g.Resources))
		}
	}

	// Each group has 2 resources (Deployment + Service)
	for _, g := range output.Graph.Groups {
		if len(g.Resources) != 2 {
			t.Errorf("Group %q: expected 2 resources, got %d", g.Name, len(g.Resources))
		}
	}

	// 3 Service→Deployment relationships
	relCount := 0
	for _, rel := range output.Graph.Relationships {
		if rel.From.GVK.Kind == "Service" && rel.To.GVK.Kind == "Deployment" {
			relCount++
		}
	}
	if relCount != 3 {
		t.Errorf("Expected 3 Service→Deployment relationships, got %d", relCount)
	}
}

// ============================================================
// Subtask 2: Service dependencies
// ============================================================

func TestFullStack_ServiceDependencies(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	// Frontend depends on backend (via env)
	h.WriteInputFile("frontend.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
  namespace: default
  labels:
    app.kubernetes.io/name: frontend
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: frontend
  template:
    metadata:
      labels:
        app.kubernetes.io/name: frontend
    spec:
      containers:
        - name: app
          image: frontend:1.0
          env:
            - name: BACKEND_URL
              value: "http://backend:8080"
`)

	// Backend depends on database (via env)
	h.WriteInputFile("backend.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: backend
  namespace: default
  labels:
    app.kubernetes.io/name: backend
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: backend
  template:
    metadata:
      labels:
        app.kubernetes.io/name: backend
    spec:
      containers:
        - name: app
          image: backend:2.0
          env:
            - name: DB_HOST
              value: "database:5432"
`)

	// Database
	h.WriteInputFile("database.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: database
  namespace: default
  labels:
    app.kubernetes.io/name: database
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: database
  template:
    metadata:
      labels:
        app.kubernetes.io/name: database
    spec:
      containers:
        - name: postgres
          image: postgres:15
`)

	// Services for label-selector detection
	h.WriteInputFile("frontend-svc.yaml", `apiVersion: v1
kind: Service
metadata:
  name: frontend
  namespace: default
  labels:
    app.kubernetes.io/name: frontend
spec:
  selector:
    app.kubernetes.io/name: frontend
  ports:
    - port: 80
`)
	h.WriteInputFile("backend-svc.yaml", `apiVersion: v1
kind: Service
metadata:
  name: backend
  namespace: default
  labels:
    app.kubernetes.io/name: backend
spec:
  selector:
    app.kubernetes.io/name: backend
  ports:
    - port: 8080
`)
	h.WriteInputFile("database-svc.yaml", `apiVersion: v1
kind: Service
metadata:
  name: database
  namespace: default
  labels:
    app.kubernetes.io/name: database
spec:
  selector:
    app.kubernetes.io/name: database
  ports:
    - port: 5432
`)

	opts := PipelineOptions{ChartName: "deptest"}

	output, err := ExecutePipeline(h.InputDir, opts)
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// ── Graph has relationships ──
	if len(output.Graph.Relationships) < 3 {
		t.Errorf("Expected at least 3 relationships, got %d", len(output.Graph.Relationships))
	}

	// ── 3 groups detected ──
	if len(output.Graph.Groups) != 3 {
		t.Errorf("Expected 3 groups, got %d", len(output.Graph.Groups))
	}

	// ── Verify Service→Deployment relationships exist for each tier ──
	expectedPairs := map[string]bool{
		"frontend": false,
		"backend":  false,
		"database": false,
	}
	for _, rel := range output.Graph.Relationships {
		if rel.From.GVK.Kind == "Service" && rel.To.GVK.Kind == "Deployment" {
			expectedPairs[rel.To.Name] = true
		}
	}
	for name, found := range expectedPairs {
		if !found {
			t.Errorf("Missing Service→Deployment relationship for %q", name)
		}
	}

	t.Logf("Relationships: %d", len(output.Graph.Relationships))
	for _, rel := range output.Graph.Relationships {
		t.Logf("  %s → %s (%s)", rel.From.String(), rel.To.String(), rel.Type)
	}
}

// ============================================================
// Subtask 3: Shared ConfigMap
// ============================================================

func TestFullStack_SharedConfigMap(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	// Shared ConfigMap
	h.WriteInputFile("shared-config.yaml", `apiVersion: v1
kind: ConfigMap
metadata:
  name: shared
  namespace: default
  labels:
    app.kubernetes.io/name: shared
data:
  DB_HOST: database.default.svc
  LOG_LEVEL: info
`)

	// Deployment A references shared ConfigMap
	h.WriteInputFile("app1.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: app1
  namespace: default
  labels:
    app.kubernetes.io/name: app1
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: app1
  template:
    metadata:
      labels:
        app.kubernetes.io/name: app1
    spec:
      containers:
        - name: app
          image: app1:latest
          envFrom:
            - configMapRef:
                name: shared
`)

	// Deployment B also references shared ConfigMap
	h.WriteInputFile("app2.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: app2
  namespace: default
  labels:
    app.kubernetes.io/name: app2
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: app2
  template:
    metadata:
      labels:
        app.kubernetes.io/name: app2
    spec:
      containers:
        - name: app
          image: app2:latest
          envFrom:
            - configMapRef:
                name: shared
`)

	opts := PipelineOptions{ChartName: "sharedcm"}

	output, err := ExecutePipeline(h.InputDir, opts)
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// ── ConfigMap template exists ──
	hasConfigMap := false
	for path := range output.Charts[0].Templates {
		if strings.Contains(path, "configmap") {
			hasConfigMap = true
			break
		}
	}
	if !hasConfigMap {
		t.Error("No ConfigMap template found in chart")
	}

	// ── ConfigMap present in values ──
	var values map[string]interface{}
	if err := sigsyaml.Unmarshal([]byte(output.Charts[0].ValuesYAML), &values); err != nil {
		t.Fatalf("Failed to parse values: %v", err)
	}

	services, ok := values["services"].(map[string]interface{})
	if !ok {
		t.Fatal("Missing 'services' in values")
	}

	// The shared configmap should be part of some service group
	configMapFound := false
	for svcName, svcVal := range services {
		svc, ok := svcVal.(map[string]interface{})
		if !ok {
			continue
		}
		if _, hasCM := svc["configMaps"]; hasCM {
			configMapFound = true
			t.Logf("Found configMaps in service %q", svcName)
		}
	}
	if !configMapFound {
		t.Error("ConfigMap not found in any service values")
	}

	// ── At least 3 resources processed (2 Deployments + 1 ConfigMap) ──
	if len(output.Resources) < 3 {
		t.Errorf("Expected at least 3 resources, got %d", len(output.Resources))
	}
}

// ============================================================
// Subtask 4: Ingress + Service + Deployment
// ============================================================

func TestFullStack_IngressServiceDeployment(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

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
      containers:
        - name: app
          image: webapp:1.0
          ports:
            - containerPort: 8080
`)

	h.WriteInputFile("service.yaml", `apiVersion: v1
kind: Service
metadata:
  name: webapp
  namespace: default
  labels:
    app.kubernetes.io/name: webapp
spec:
  selector:
    app.kubernetes.io/name: webapp
  ports:
    - name: http
      port: 80
      targetPort: 8080
`)

	h.WriteInputFile("ingress.yaml", `apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: webapp
  namespace: default
  labels:
    app.kubernetes.io/name: webapp
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
spec:
  ingressClassName: nginx
  rules:
    - host: webapp.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: webapp
                port:
                  number: 80
  tls:
    - hosts:
        - webapp.example.com
      secretName: webapp-tls
`)

	opts := PipelineOptions{ChartName: "ingresstest"}

	output, err := ExecutePipeline(h.InputDir, opts)
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// ── 3 resources (Deployment + Service + Ingress) ──
	if len(output.Resources) != 3 {
		t.Errorf("Expected 3 resources, got %d", len(output.Resources))
	}

	// ── Templates for all 3 ──
	chart := output.Charts[0]
	hasDeployment := false
	hasService := false
	hasIngress := false
	for path := range chart.Templates {
		if strings.Contains(path, "deployment") {
			hasDeployment = true
		}
		if strings.Contains(path, "service") {
			hasService = true
		}
		if strings.Contains(path, "ingress") {
			hasIngress = true
		}
	}
	if !hasDeployment {
		t.Error("Missing deployment template")
	}
	if !hasService {
		t.Error("Missing service template")
	}
	if !hasIngress {
		t.Error("Missing ingress template")
	}

	// ── Relationships: Ingress→Service, Service→Deployment ──
	ingressToSvc := false
	svcToDeployment := false
	for _, rel := range output.Graph.Relationships {
		if rel.From.GVK.Kind == "Ingress" && rel.To.GVK.Kind == "Service" {
			ingressToSvc = true
		}
		if rel.From.GVK.Kind == "Service" && rel.To.GVK.Kind == "Deployment" {
			svcToDeployment = true
		}
	}
	if !ingressToSvc {
		t.Error("Missing Ingress → Service relationship")
	}
	if !svcToDeployment {
		t.Error("Missing Service → Deployment relationship")
	}

	// ── Ingress values extracted ──
	var values map[string]interface{}
	if err := sigsyaml.Unmarshal([]byte(chart.ValuesYAML), &values); err != nil {
		t.Fatalf("Failed to parse values: %v", err)
	}

	// Check ingress rules are in values
	rulesVal, hasRules := findNestedKey(values, "rules")
	if !hasRules {
		t.Error("Ingress rules not found in values")
	} else if rules, ok := rulesVal.([]interface{}); ok {
		if len(rules) == 0 {
			t.Error("Ingress rules empty")
		}
	}
}

// ============================================================
// Subtask 5: StatefulSet with PVC
// ============================================================

func TestFullStack_StatefulSetWithPVC(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	h.WriteInputFile("statefulset.yaml", `apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: database
  namespace: default
  labels:
    app.kubernetes.io/name: database
spec:
  serviceName: database
  replicas: 3
  selector:
    matchLabels:
      app.kubernetes.io/name: database
  template:
    metadata:
      labels:
        app.kubernetes.io/name: database
    spec:
      containers:
        - name: postgres
          image: postgres:15
          ports:
            - containerPort: 5432
          volumeMounts:
            - name: data
              mountPath: /var/lib/postgresql/data
  volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        accessModes: ["ReadWriteOnce"]
        resources:
          requests:
            storage: 10Gi
`)

	h.WriteInputFile("headless-service.yaml", `apiVersion: v1
kind: Service
metadata:
  name: database
  namespace: default
  labels:
    app.kubernetes.io/name: database
spec:
  clusterIP: None
  selector:
    app.kubernetes.io/name: database
  ports:
    - port: 5432
`)

	opts := PipelineOptions{ChartName: "ststest"}

	output, err := ExecutePipeline(h.InputDir, opts)
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// ── 2 resources (StatefulSet + Service) ──
	if len(output.Resources) != 2 {
		t.Errorf("Expected 2 resources, got %d", len(output.Resources))
	}

	// ── StatefulSet template exists ──
	hasStatefulSet := false
	for path := range output.Charts[0].Templates {
		if strings.Contains(path, "statefulset") {
			hasStatefulSet = true
			break
		}
	}
	if !hasStatefulSet {
		t.Error("Missing StatefulSet template")
	}

	// ── Values contain volumeClaimTemplates ──
	var values map[string]interface{}
	if err := sigsyaml.Unmarshal([]byte(output.Charts[0].ValuesYAML), &values); err != nil {
		t.Fatalf("Failed to parse values: %v", err)
	}

	vctVal, hasVCT := findNestedKey(values, "volumeClaimTemplates")
	if !hasVCT {
		t.Error("volumeClaimTemplates not found in values")
	} else if vct, ok := vctVal.([]interface{}); ok {
		if len(vct) == 0 {
			t.Error("volumeClaimTemplates empty")
		}
	}

	// ── Service→StatefulSet relationship ──
	hasSvcToSts := false
	for _, rel := range output.Graph.Relationships {
		if rel.From.GVK.Kind == "Service" && rel.To.GVK.Kind == "StatefulSet" {
			hasSvcToSts = true
		}
	}
	if !hasSvcToSts {
		t.Error("Missing Service → StatefulSet relationship")
	}
}

// ============================================================
// Subtask 6: Job + ConfigMap (init job pattern)
// ============================================================

func TestFullStack_JobWithConfigMap(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	h.WriteInputFile("migration-config.yaml", `apiVersion: v1
kind: ConfigMap
metadata:
  name: migration
  namespace: default
  labels:
    app.kubernetes.io/name: migration
data:
  migrate.sql: |
    CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT);
    CREATE TABLE orders (id SERIAL PRIMARY KEY, user_id INT);
`)

	h.WriteInputFile("migration-job.yaml", `apiVersion: batch/v1
kind: Job
metadata:
  name: migration
  namespace: default
  labels:
    app.kubernetes.io/name: migration
spec:
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: migrate
          image: postgres:15
          command: ["psql", "-f", "/migrations/migrate.sql"]
          volumeMounts:
            - name: scripts
              mountPath: /migrations
      volumes:
        - name: scripts
          configMap:
            name: migration
  backoffLimit: 3
`)

	opts := PipelineOptions{ChartName: "jobtest"}

	output, err := ExecutePipeline(h.InputDir, opts)
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// ── 2 resources (Job + ConfigMap) ──
	if len(output.Resources) < 2 {
		t.Errorf("Expected at least 2 resources, got %d", len(output.Resources))
	}

	// ── Job template exists ──
	hasJob := false
	for path := range output.Charts[0].Templates {
		if strings.Contains(path, "job") {
			hasJob = true
			break
		}
	}
	if !hasJob {
		t.Error("Missing Job template")
	}

	// ── ConfigMap template exists ──
	hasConfigMap := false
	for path := range output.Charts[0].Templates {
		if strings.Contains(path, "configmap") {
			hasConfigMap = true
			break
		}
	}
	if !hasConfigMap {
		t.Error("Missing ConfigMap template")
	}

	// ── Both resources are processed ──
	kinds := make(map[string]bool)
	for _, r := range output.Resources {
		kinds[r.Original.GVK.Kind] = true
	}
	if !kinds["Job"] {
		t.Error("Job not processed")
	}
	if !kinds["ConfigMap"] {
		t.Error("ConfigMap not processed")
	}
}

// ============================================================
// Subtask 7: DaemonSet (logging agent pattern)
// ============================================================

func TestFullStack_DaemonSetLoggingAgent(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	h.WriteInputFile("fluentbit.yaml", `apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: fluentbit
  namespace: monitoring
  labels:
    app.kubernetes.io/name: fluentbit
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: fluentbit
  template:
    metadata:
      labels:
        app.kubernetes.io/name: fluentbit
    spec:
      tolerations:
        - key: node-role.kubernetes.io/master
          effect: NoSchedule
        - key: node-role.kubernetes.io/control-plane
          effect: NoSchedule
      nodeSelector:
        kubernetes.io/os: linux
      containers:
        - name: fluentbit
          image: fluent/fluent-bit:2.1
          volumeMounts:
            - name: varlog
              mountPath: /var/log
              readOnly: true
            - name: containers
              mountPath: /var/lib/docker/containers
              readOnly: true
      volumes:
        - name: varlog
          hostPath:
            path: /var/log
        - name: containers
          hostPath:
            path: /var/lib/docker/containers
`)

	opts := PipelineOptions{ChartName: "dstest"}

	output, err := ExecutePipeline(h.InputDir, opts)
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// ── 1 resource (DaemonSet) ──
	if len(output.Resources) != 1 {
		t.Errorf("Expected 1 resource, got %d", len(output.Resources))
	}

	// ── DaemonSet template exists ──
	hasDaemonSet := false
	for path := range output.Charts[0].Templates {
		if strings.Contains(path, "daemonset") {
			hasDaemonSet = true
			break
		}
	}
	if !hasDaemonSet {
		t.Error("Missing DaemonSet template")
	}

	// ── Values contain tolerations and nodeSelector ──
	var values map[string]interface{}
	if err := sigsyaml.Unmarshal([]byte(output.Charts[0].ValuesYAML), &values); err != nil {
		t.Fatalf("Failed to parse values: %v", err)
	}

	_, hasTolerations := findNestedKey(values, "tolerations")
	if !hasTolerations {
		t.Error("tolerations not found in values")
	}

	_, hasNodeSelector := findNestedKey(values, "nodeSelector")
	if !hasNodeSelector {
		t.Error("nodeSelector not found in values")
	}

	// ── Values contain volumes (hostPath) ──
	_, hasVolumes := findNestedKey(values, "volumes")
	if !hasVolumes {
		t.Error("volumes not found in values")
	}
}

// ============================================================
// Subtask 8: Complex values.yaml structure
// ============================================================

func TestFullStack_ComplexValuesStructure(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	// Frontend
	h.WriteInputFile("frontend.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
  namespace: default
  labels:
    app.kubernetes.io/name: frontend
spec:
  replicas: 2
  selector:
    matchLabels:
      app.kubernetes.io/name: frontend
  template:
    metadata:
      labels:
        app.kubernetes.io/name: frontend
    spec:
      containers:
        - name: nginx
          image: nginx:1.25
          ports:
            - containerPort: 80
          resources:
            limits:
              cpu: 200m
              memory: 128Mi
`)

	// Backend
	h.WriteInputFile("backend.yaml", `apiVersion: apps/v1
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
          image: api:2.0
          ports:
            - containerPort: 8080
          resources:
            limits:
              cpu: 500m
              memory: 512Mi
`)

	// Database (with different resource profile)
	h.WriteInputFile("database.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: database
  namespace: default
  labels:
    app.kubernetes.io/name: database
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: database
  template:
    metadata:
      labels:
        app.kubernetes.io/name: database
    spec:
      containers:
        - name: postgres
          image: postgres:15
          ports:
            - containerPort: 5432
          resources:
            limits:
              cpu: "1"
              memory: 1Gi
`)

	opts := PipelineOptions{
		ChartName:    "complex",
		ChartVersion: "1.0.0",
		AppVersion:   "1.0.0",
	}

	output, err := ExecutePipeline(h.InputDir, opts)
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// Parse values
	var values map[string]interface{}
	if err := sigsyaml.Unmarshal([]byte(output.Charts[0].ValuesYAML), &values); err != nil {
		t.Fatalf("Failed to parse values: %v", err)
	}

	// ── Has services section ──
	services, ok := values["services"].(map[string]interface{})
	if !ok {
		t.Fatal("Missing 'services' in values")
	}

	// ── 3 service entries: frontend, backend, database ──
	expectedServices := []string{"frontend", "backend", "database"}
	for _, name := range expectedServices {
		svc, ok := services[name].(map[string]interface{})
		if !ok {
			t.Errorf("Missing services.%s in values", name)
			continue
		}

		// Each service has enabled
		if _, ok := svc["enabled"]; !ok {
			t.Errorf("services.%s missing 'enabled' key", name)
		}

		// Each service has deployment nested values
		deployVal, ok := svc["deployment"].(map[string]interface{})
		if !ok {
			t.Errorf("services.%s missing 'deployment' key", name)
			continue
		}

		// Deployment has containers
		if _, ok := deployVal["containers"]; !ok {
			t.Errorf("services.%s.deployment missing 'containers'", name)
		}

		// Deployment has replicas
		if _, ok := deployVal["replicas"]; !ok {
			t.Errorf("services.%s.deployment missing 'replicas'", name)
		}
	}

	// ── Global section exists ──
	if _, ok := values["global"]; !ok {
		t.Error("Missing 'global' section in values")
	}

	// ── Values are valid YAML ──
	ValidateValues(t, output.Charts[0].ValuesYAML)

	t.Logf("Values structure:\n%s", output.Charts[0].ValuesYAML)
}
