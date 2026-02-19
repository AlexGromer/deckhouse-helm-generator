package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	sigsyaml "sigs.k8s.io/yaml"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/generator"
)

// ============================================================
// Task 2.4: Pipeline Test — Deckhouse Module
// Tests Deckhouse-specific CRDs (ModuleConfig, IngressNginxController,
// NodeGroup) processed by the generic processor, Deckhouse module
// structure, Helm hooks, and mixed CRD+vanilla resources.
// ============================================================

// ============================================================
// Subtask 1: ModuleConfig extraction
// ============================================================

func TestDeckhouse_ModuleConfigExtraction(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	// ModuleConfig CRD — Deckhouse-specific resource for module configuration
	h.WriteInputFile("module-config.yaml", `
apiVersion: deckhouse.io/v1alpha1
kind: ModuleConfig
metadata:
  name: deckhouse-web
  labels:
    app: deckhouse-web
spec:
  enabled: true
  version: 1
  settings:
    auth:
      externalAuthentication:
        authURL: https://dex.example.com/dex/auth
        authSignInURL: https://dex.example.com/dex/sign_in
    https:
      mode: CustomCertificate
      customCertificate:
        secretName: tls-web
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "deckhouse-module",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// Verify resource was processed
	if len(output.Resources) == 0 {
		t.Fatal("Expected at least 1 processed resource")
	}

	// Find the ModuleConfig resource
	found := false
	for _, res := range output.Resources {
		if res.Original.Object.GetKind() == "ModuleConfig" {
			found = true

			// Values should contain spec
			spec, ok := res.Values["spec"]
			if !ok {
				t.Fatal("Expected spec in ModuleConfig values")
			}

			specMap, ok := spec.(map[string]interface{})
			if !ok {
				t.Fatal("Expected spec to be a map")
			}

			// Verify settings are preserved
			settings, ok := specMap["settings"].(map[string]interface{})
			if !ok {
				t.Fatal("Expected settings in spec")
			}

			auth, ok := settings["auth"].(map[string]interface{})
			if !ok {
				t.Fatal("Expected auth in settings")
			}

			extAuth, ok := auth["externalAuthentication"].(map[string]interface{})
			if !ok {
				t.Fatal("Expected externalAuthentication in auth")
			}

			if extAuth["authURL"] != "https://dex.example.com/dex/auth" {
				t.Errorf("Expected authURL 'https://dex.example.com/dex/auth', got %v", extAuth["authURL"])
			}

			// Verify enabled flag
			if res.Values["enabled"] != true {
				t.Error("Expected enabled=true in values")
			}

			// Verify template path uses lowercase kind
			if !strings.Contains(res.TemplatePath, "moduleconfig") {
				t.Errorf("Expected template path to contain 'moduleconfig', got %s", res.TemplatePath)
			}

			// Verify values path uses camelCase kind
			if !strings.Contains(res.ValuesPath, "moduleConfig") {
				t.Errorf("Expected values path to contain 'moduleConfig', got %s", res.ValuesPath)
			}

			break
		}
	}

	if !found {
		t.Fatal("ModuleConfig resource not found in processed resources")
	}

	// Verify chart was generated
	if len(output.Charts) == 0 {
		t.Fatal("Expected at least 1 generated chart")
	}

	// Verify template exists in chart
	chart := output.Charts[0]
	foundTemplate := false
	for path := range chart.Templates {
		if strings.Contains(path, "moduleconfig") {
			foundTemplate = true
			break
		}
	}
	if !foundTemplate {
		t.Errorf("Expected moduleconfig template in chart, templates: %v", templateNames(chart.Templates))
	}

	// Verify values.yaml contains moduleConfig section
	if !strings.Contains(chart.ValuesYAML, "moduleConfig") {
		t.Error("Expected 'moduleConfig' in values.yaml")
	}
}

// ============================================================
// Subtask 2: IngressNginxController extraction
// ============================================================

func TestDeckhouse_IngressNginxControllerExtraction(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	h.WriteInputFile("ingress-nginx.yaml", `
apiVersion: deckhouse.io/v1
kind: IngressNginxController
metadata:
  name: main
  labels:
    app: ingress-nginx
spec:
  ingressClass: nginx
  inlet: LoadBalancer
  loadBalancer:
    annotations:
      service.beta.kubernetes.io/aws-load-balancer-type: nlb
  resourcesRequests:
    mode: VPA
    vpa:
      mode: Auto
      cpu:
        min: 50m
      memory:
        min: 64Mi
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "deckhouse-module",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	if len(output.Resources) == 0 {
		t.Fatal("Expected at least 1 processed resource")
	}

	found := false
	for _, res := range output.Resources {
		if res.Original.Object.GetKind() == "IngressNginxController" {
			found = true

			spec, ok := res.Values["spec"]
			if !ok {
				t.Fatal("Expected spec in IngressNginxController values")
			}

			specMap, ok := spec.(map[string]interface{})
			if !ok {
				t.Fatal("Expected spec to be a map")
			}

			// Verify nginx-specific settings preserved
			if specMap["ingressClass"] != "nginx" {
				t.Errorf("Expected ingressClass 'nginx', got %v", specMap["ingressClass"])
			}
			if specMap["inlet"] != "LoadBalancer" {
				t.Errorf("Expected inlet 'LoadBalancer', got %v", specMap["inlet"])
			}

			// Verify nested loadBalancer settings
			lb, ok := specMap["loadBalancer"].(map[string]interface{})
			if !ok {
				t.Fatal("Expected loadBalancer in spec")
			}
			lbAnnotations, ok := lb["annotations"].(map[string]interface{})
			if !ok {
				t.Fatal("Expected annotations in loadBalancer")
			}
			if lbAnnotations["service.beta.kubernetes.io/aws-load-balancer-type"] != "nlb" {
				t.Error("Expected AWS NLB annotation in loadBalancer")
			}

			// Verify values path uses camelCase
			if !strings.Contains(res.ValuesPath, "ingressNginxController") {
				t.Errorf("Expected values path to contain 'ingressNginxController', got %s", res.ValuesPath)
			}

			break
		}
	}

	if !found {
		t.Fatal("IngressNginxController resource not found in processed resources")
	}
}

// ============================================================
// Subtask 3: NodeGroup extraction (cloud-specific)
// ============================================================

func TestDeckhouse_NodeGroupExtraction(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	h.WriteInputFile("nodegroup.yaml", `
apiVersion: deckhouse.io/v1
kind: NodeGroup
metadata:
  name: worker
  labels:
    app: worker-nodes
spec:
  nodeType: CloudEphemeral
  cloudInstances:
    maxPerZone: 5
    minPerZone: 2
    classReference:
      kind: AWSInstanceClass
      name: worker
  disruptions:
    approvalMode: Automatic
  kubelet:
    maxPods: 150
    containerLogMaxSize: 20Mi
  nodeTemplate:
    labels:
      node-role.kubernetes.io/worker: ""
    taints:
      - key: dedicated
        value: worker
        effect: NoSchedule
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "deckhouse-module",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	if len(output.Resources) == 0 {
		t.Fatal("Expected at least 1 processed resource")
	}

	found := false
	for _, res := range output.Resources {
		if res.Original.Object.GetKind() == "NodeGroup" {
			found = true

			spec, ok := res.Values["spec"]
			if !ok {
				t.Fatal("Expected spec in NodeGroup values")
			}

			specMap, ok := spec.(map[string]interface{})
			if !ok {
				t.Fatal("Expected spec to be a map")
			}

			// Verify cloud-specific settings
			if specMap["nodeType"] != "CloudEphemeral" {
				t.Errorf("Expected nodeType 'CloudEphemeral', got %v", specMap["nodeType"])
			}

			cloudInstances, ok := specMap["cloudInstances"].(map[string]interface{})
			if !ok {
				t.Fatal("Expected cloudInstances in spec")
			}

			// Check numeric values (may come as int64 or float64 from YAML)
			maxPerZone := toInt(cloudInstances["maxPerZone"])
			if maxPerZone != 5 {
				t.Errorf("Expected maxPerZone 5, got %v", cloudInstances["maxPerZone"])
			}

			minPerZone := toInt(cloudInstances["minPerZone"])
			if minPerZone != 2 {
				t.Errorf("Expected minPerZone 2, got %v", cloudInstances["minPerZone"])
			}

			// Verify classReference
			classRef, ok := cloudInstances["classReference"].(map[string]interface{})
			if !ok {
				t.Fatal("Expected classReference in cloudInstances")
			}
			if classRef["kind"] != "AWSInstanceClass" {
				t.Errorf("Expected classReference kind 'AWSInstanceClass', got %v", classRef["kind"])
			}

			// Verify kubelet settings
			kubelet, ok := specMap["kubelet"].(map[string]interface{})
			if !ok {
				t.Fatal("Expected kubelet in spec")
			}
			kubeletMaxPods := toInt(kubelet["maxPods"])
			if kubeletMaxPods != 150 {
				t.Errorf("Expected kubelet maxPods 150, got %v", kubelet["maxPods"])
			}

			// Verify template path
			if !strings.Contains(res.TemplatePath, "nodegroup") {
				t.Errorf("Expected template path to contain 'nodegroup', got %s", res.TemplatePath)
			}

			break
		}
	}

	if !found {
		t.Fatal("NodeGroup resource not found in processed resources")
	}
}

// ============================================================
// Subtask 4: Deckhouse module structure validation
// ============================================================

func TestDeckhouse_ModuleStructure(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	// Input: a set of Deckhouse CRDs that form a module
	h.WriteInputFile("module-config.yaml", `
apiVersion: deckhouse.io/v1alpha1
kind: ModuleConfig
metadata:
  name: ingress-nginx
  labels:
    app: ingress-nginx
spec:
  enabled: true
  version: 1
  settings:
    defaultControllerVersion: "1.6"
`)

	h.WriteInputFile("controller.yaml", `
apiVersion: deckhouse.io/v1
kind: IngressNginxController
metadata:
  name: main
  labels:
    app: ingress-nginx
spec:
  ingressClass: nginx
  inlet: HostPort
  hostPort:
    httpPort: 80
    httpsPort: 443
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "ingress-nginx-module",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// Verify chart structure
	if len(output.Charts) == 0 {
		t.Fatal("Expected at least 1 generated chart")
	}

	chart := output.Charts[0]

	// Verify Chart.yaml
	if chart.ChartYAML == "" {
		t.Fatal("Expected non-empty Chart.yaml")
	}
	if !strings.Contains(chart.ChartYAML, "ingress-nginx-module") {
		t.Error("Expected chart name 'ingress-nginx-module' in Chart.yaml")
	}

	// Verify values.yaml has proper structure
	if chart.ValuesYAML == "" {
		t.Fatal("Expected non-empty values.yaml")
	}

	var valuesMap map[string]interface{}
	if err := sigsyaml.Unmarshal([]byte(chart.ValuesYAML), &valuesMap); err != nil {
		t.Fatalf("values.yaml is not valid YAML: %v", err)
	}

	// Check global section exists
	if _, ok := valuesMap["global"]; !ok {
		t.Error("Expected 'global' section in values.yaml")
	}

	// Check services section exists
	services, ok := valuesMap["services"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected 'services' section in values.yaml")
	}

	// At least one service group should exist
	if len(services) == 0 {
		t.Error("Expected at least one service group in values")
	}

	// Verify helpers exist
	if chart.Helpers == "" {
		t.Fatal("Expected non-empty _helpers.tpl")
	}
	if !strings.Contains(chart.Helpers, "ingress-nginx-module.fullname") {
		t.Error("Expected fullname helper for chart")
	}
	if !strings.Contains(chart.Helpers, "ingress-nginx-module.labels") {
		t.Error("Expected labels helper for chart")
	}

	// Verify templates exist for both CRDs
	if len(chart.Templates) < 2 {
		t.Errorf("Expected at least 2 templates (ModuleConfig + IngressNginxController), got %d", len(chart.Templates))
	}

	// Verify chart was written to disk correctly
	chartDir := filepath.Join(output.OutputDir, chart.Name)
	ValidateChartStructure(t, chartDir)
	ValidateTemplates(t, chartDir)

	// Verify values.yaml on disk
	valuesPath := filepath.Join(chartDir, "values.yaml")
	valuesData, err := os.ReadFile(valuesPath)
	if err != nil {
		t.Fatalf("Cannot read values.yaml: %v", err)
	}
	ValidateValues(t, string(valuesData))
}

// ============================================================
// Subtask 5: Helm hooks for Deckhouse
// ============================================================

func TestDeckhouse_HelmHooks(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	// A Deckhouse resource with Helm hook annotations (e.g., pre-install migration)
	h.WriteInputFile("migration-job.yaml", `
apiVersion: batch/v1
kind: Job
metadata:
  name: db-migration
  labels:
    app: myapp
  annotations:
    helm.sh/hook: pre-install,pre-upgrade
    helm.sh/hook-weight: "-5"
    helm.sh/hook-delete-policy: before-hook-creation
spec:
  template:
    spec:
      containers:
        - name: migration
          image: myapp/migration:1.0
          command: ["./migrate"]
      restartPolicy: Never
  backoffLimit: 1
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "deckhouse-app",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	if len(output.Resources) == 0 {
		t.Fatal("Expected at least 1 processed resource")
	}

	// Find the Job resource
	found := false
	for _, res := range output.Resources {
		if res.Original.Object.GetKind() == "Job" {
			found = true

			// Verify the template content includes helm hook annotations
			tpl := res.TemplateContent
			if tpl == "" {
				t.Fatal("Expected non-empty template content for Job")
			}

			// Helm hook annotations should be present in the generated template
			if !strings.Contains(tpl, "helm.sh/hook") {
				t.Error("Expected 'helm.sh/hook' annotation in template")
			}
			if !strings.Contains(tpl, "pre-install") {
				t.Error("Expected 'pre-install' hook type in template")
			}
			if !strings.Contains(tpl, "pre-upgrade") {
				t.Error("Expected 'pre-upgrade' hook type in template")
			}
			if !strings.Contains(tpl, "helm.sh/hook-weight") {
				t.Error("Expected 'helm.sh/hook-weight' annotation in template")
			}
			if !strings.Contains(tpl, "helm.sh/hook-delete-policy") {
				t.Error("Expected 'helm.sh/hook-delete-policy' annotation in template")
			}

			break
		}
	}

	if !found {
		t.Fatal("Job resource not found in processed resources")
	}
}

// ============================================================
// Subtask 6: Mixed Deckhouse + vanilla K8s
// ============================================================

func TestDeckhouse_MixedDeckhouseAndVanillaK8s(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	// ModuleConfig (Deckhouse CRD)
	h.WriteInputFile("module-config.yaml", `
apiVersion: deckhouse.io/v1alpha1
kind: ModuleConfig
metadata:
  name: mymodule
  labels:
    app: myapp
spec:
  enabled: true
  version: 1
  settings:
    replicas: 3
`)

	// Standard Deployment
	h.WriteInputFile("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
  labels:
    app: myapp
spec:
  replicas: 3
  selector:
    matchLabels:
      app: myapp
  template:
    metadata:
      labels:
        app: myapp
    spec:
      containers:
        - name: myapp
          image: myapp:1.0
          ports:
            - containerPort: 8080
`)

	// Standard Service
	h.WriteInputFile("service.yaml", `
apiVersion: v1
kind: Service
metadata:
  name: myapp
  labels:
    app: myapp
spec:
  type: ClusterIP
  selector:
    app: myapp
  ports:
    - port: 80
      targetPort: 8080
      protocol: TCP
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "mixed-chart",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// Expect 3 processed resources (ModuleConfig + Deployment + Service)
	if len(output.Resources) < 3 {
		t.Fatalf("Expected at least 3 processed resources, got %d", len(output.Resources))
	}

	// Track which resource types we found
	foundKinds := make(map[string]bool)
	for _, res := range output.Resources {
		kind := res.Original.Object.GetKind()
		foundKinds[kind] = true
	}

	// Verify all three resource types are present
	expectedKinds := []string{"ModuleConfig", "Deployment", "Service"}
	for _, kind := range expectedKinds {
		if !foundKinds[kind] {
			t.Errorf("Expected %s in processed resources, found kinds: %v", kind, foundKinds)
		}
	}

	// Verify chart was generated
	if len(output.Charts) == 0 {
		t.Fatal("Expected at least 1 generated chart")
	}

	chart := output.Charts[0]

	// Verify we have templates for all resource types
	// At minimum: deployment + service + moduleconfig + _helpers.tpl
	if len(chart.Templates) < 3 {
		t.Errorf("Expected at least 3 templates, got %d: %v",
			len(chart.Templates), templateNames(chart.Templates))
	}

	// Check that templates contain both CRD and vanilla K8s resources
	hasModuleConfig := false
	hasDeployment := false
	hasService := false
	for path, content := range chart.Templates {
		if strings.Contains(path, "moduleconfig") {
			hasModuleConfig = true
			if !strings.Contains(content, "deckhouse.io") {
				t.Error("ModuleConfig template should reference deckhouse.io API")
			}
		}
		if strings.Contains(path, "deployment") {
			hasDeployment = true
			if !strings.Contains(content, "apps/v1") {
				t.Error("Deployment template should reference apps/v1 API")
			}
		}
		if strings.Contains(path, "service") && !strings.Contains(path, "serviceaccount") {
			hasService = true
		}
	}

	if !hasModuleConfig {
		t.Error("Expected moduleconfig template in chart")
	}
	if !hasDeployment {
		t.Error("Expected deployment template in chart")
	}
	if !hasService {
		t.Error("Expected service template in chart")
	}

	// Verify values.yaml is valid and contains sections for all resources
	var valuesMap map[string]interface{}
	if err := sigsyaml.Unmarshal([]byte(chart.ValuesYAML), &valuesMap); err != nil {
		t.Fatalf("values.yaml is not valid YAML: %v", err)
	}

	services, ok := valuesMap["services"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected 'services' section in values.yaml")
	}

	// The 'myapp' service group should exist (shared label app=myapp)
	myapp, ok := services["myapp"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected 'myapp' service in values, got services: %v", mapKeys(services))
	}

	// Should have deployment values
	if _, ok := myapp["deployment"]; !ok {
		t.Error("Expected 'deployment' in myapp service values")
	}

	// Should have service values
	if _, ok := myapp["service"]; !ok {
		t.Error("Expected 'service' in myapp service values")
	}

	// Verify chart structure on disk
	chartDir := filepath.Join(output.OutputDir, chart.Name)
	ValidateChartStructure(t, chartDir)
}

// ============================================================
// v0.4.0 — Monitoring Stack
// ============================================================

func TestPipelineDeckhouse_MonitoringStack(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	h.WriteInputFile("servicemonitor.yaml", `
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: myapp-metrics
  namespace: monitoring
  labels:
    app: myapp
spec:
  endpoints:
    - port: metrics
      path: /metrics
      interval: 30s
  selector:
    matchLabels:
      app: myapp
`)

	h.WriteInputFile("prometheusrule.yaml", `
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: myapp-alerts
  namespace: monitoring
  labels:
    app: myapp
spec:
  groups:
    - name: myapp.rules
      rules:
        - alert: HighErrorRate
          expr: rate(http_errors_total[5m]) > 0.5
          for: 5m
          labels:
            severity: critical
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "monitoring-test",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// Verify both resources processed
	foundKinds := make(map[string]bool)
	for _, res := range output.Resources {
		foundKinds[res.Original.Object.GetKind()] = true
	}

	if !foundKinds["ServiceMonitor"] {
		t.Error("Expected ServiceMonitor in processed resources")
	}
	if !foundKinds["PrometheusRule"] {
		t.Error("Expected PrometheusRule in processed resources")
	}

	// Verify chart has templates
	if len(output.Charts) == 0 {
		t.Fatal("Expected at least 1 chart")
	}
	chart := output.Charts[0]

	hasSM := false
	hasPR := false
	for path := range chart.Templates {
		if strings.Contains(path, "servicemonitor") {
			hasSM = true
		}
		if strings.Contains(path, "prometheusrule") {
			hasPR = true
		}
	}
	if !hasSM {
		t.Error("Expected servicemonitor template")
	}
	if !hasPR {
		t.Error("Expected prometheusrule template")
	}
}

// ============================================================
// v0.4.0 — Gateway API
// ============================================================

func TestPipelineDeckhouse_GatewayAPI(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	h.WriteInputFile("gateway.yaml", `
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: my-gateway
  namespace: default
  labels:
    app: gateway-app
spec:
  gatewayClassName: nginx
  listeners:
    - name: http
      port: 80
      protocol: HTTP
`)

	h.WriteInputFile("httproute.yaml", `
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: myapp-route
  namespace: default
  labels:
    app: gateway-app
spec:
  parentRefs:
    - name: my-gateway
  hostnames:
    - app.example.com
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: myapp-svc
          port: 80
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "gateway-test",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	foundKinds := make(map[string]bool)
	for _, res := range output.Resources {
		foundKinds[res.Original.Object.GetKind()] = true
	}

	if !foundKinds["Gateway"] {
		t.Error("Expected Gateway in processed resources")
	}
	if !foundKinds["HTTPRoute"] {
		t.Error("Expected HTTPRoute in processed resources")
	}

	if len(output.Charts) == 0 {
		t.Fatal("Expected at least 1 chart")
	}
	chart := output.Charts[0]

	hasGW := false
	hasHR := false
	for path := range chart.Templates {
		if strings.Contains(path, "gateway") {
			hasGW = true
		}
		if strings.Contains(path, "httproute") {
			hasHR = true
		}
	}
	if !hasGW {
		t.Error("Expected gateway template")
	}
	if !hasHR {
		t.Error("Expected httproute template")
	}
}

// ============================================================
// v0.4.0 — KEDA
// ============================================================

func TestPipelineDeckhouse_KEDA(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	h.WriteInputFile("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
  namespace: default
  labels:
    app: myapp
spec:
  replicas: 1
  selector:
    matchLabels:
      app: myapp
  template:
    metadata:
      labels:
        app: myapp
    spec:
      containers:
        - name: myapp
          image: myapp:v1
`)

	h.WriteInputFile("scaledobject.yaml", `
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: myapp-scaler
  namespace: default
  labels:
    app: myapp
spec:
  scaleTargetRef:
    name: myapp
    kind: Deployment
  minReplicaCount: 1
  maxReplicaCount: 10
  triggers:
    - type: prometheus
      metadata:
        serverAddress: http://prometheus:9090
        query: sum(rate(http_requests_total[2m]))
        threshold: "100"
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "keda-test",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	foundKinds := make(map[string]bool)
	for _, res := range output.Resources {
		foundKinds[res.Original.Object.GetKind()] = true
	}

	if !foundKinds["ScaledObject"] {
		t.Error("Expected ScaledObject in processed resources")
	}
	if !foundKinds["Deployment"] {
		t.Error("Expected Deployment in processed resources")
	}

	// Verify ScaledObject has dependency to Deployment
	for _, res := range output.Resources {
		if res.Original.Object.GetKind() == "ScaledObject" {
			if len(res.Dependencies) == 0 {
				t.Error("Expected ScaledObject to have dependency on Deployment")
			}
			break
		}
	}
}

// ============================================================
// v0.4.0 — Regression: v0.3.0 modes not broken
// ============================================================

func TestPipelineDeckhouse_Regression(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	// Standard v0.3.0 resources that must still work
	h.WriteInputFile("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
  labels:
    app: myapp
spec:
  replicas: 2
  selector:
    matchLabels:
      app: myapp
  template:
    metadata:
      labels:
        app: myapp
    spec:
      containers:
        - name: myapp
          image: myapp:1.0
          ports:
            - containerPort: 8080
`)

	h.WriteInputFile("service.yaml", `
apiVersion: v1
kind: Service
metadata:
  name: myapp
  labels:
    app: myapp
spec:
  type: ClusterIP
  selector:
    app: myapp
  ports:
    - port: 80
      targetPort: 8080
`)

	h.WriteInputFile("configmap.yaml", `
apiVersion: v1
kind: ConfigMap
metadata:
  name: myapp-config
  labels:
    app: myapp
data:
  APP_ENV: production
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "regression-test",
	})
	if err != nil {
		t.Fatalf("Regression pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// Verify all 3 resources processed
	if len(output.Resources) < 3 {
		t.Fatalf("Expected at least 3 resources, got %d", len(output.Resources))
	}

	// Verify chart generated
	if len(output.Charts) == 0 {
		t.Fatal("Expected at least 1 chart")
	}

	chart := output.Charts[0]

	// Validate structure
	chartDir := filepath.Join(output.OutputDir, chart.Name)
	ValidateChartStructure(t, chartDir)

	// Validate values are valid YAML
	var valuesMap map[string]interface{}
	if err := sigsyaml.Unmarshal([]byte(chart.ValuesYAML), &valuesMap); err != nil {
		t.Fatalf("values.yaml is not valid YAML: %v", err)
	}

	// Verify services section
	if _, ok := valuesMap["services"]; !ok {
		t.Error("Expected 'services' section in values.yaml")
	}

	// Verify global section
	if _, ok := valuesMap["global"]; !ok {
		t.Error("Expected 'global' section in values.yaml")
	}
}

// ============================================================
// v0.4.0 — All 18 processors coverage
// ============================================================

func TestPipelineDeckhouse_AllProcessors(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	h.WriteInputFile("moduleconfig.yaml", `
apiVersion: deckhouse.io/v1alpha1
kind: ModuleConfig
metadata:
  name: test-module
  labels:
    app: test-module
spec:
  enabled: true
  version: 1
`)

	h.WriteInputFile("ingressnginxcontroller.yaml", `
apiVersion: deckhouse.io/v1
kind: IngressNginxController
metadata:
  name: main
  labels:
    app: main
spec:
  ingressClass: nginx
  controllerVersion: "1.1"
  inlet: HostPort
`)

	h.WriteInputFile("clusterauthorizationrule.yaml", `
apiVersion: deckhouse.io/v1
kind: ClusterAuthorizationRule
metadata:
  name: admin-rule
  labels:
    app: admin
spec:
  subjects:
    - kind: User
      name: admin@example.com
  accessLevel: Admin
`)

	h.WriteInputFile("nodegroup.yaml", `
apiVersion: deckhouse.io/v1
kind: NodeGroup
metadata:
  name: worker
  labels:
    app: worker
spec:
  nodeType: CloudEphemeral
  cloudInstances:
    classReference:
      kind: OpenStackInstanceClass
      name: worker
    minPerZone: 1
    maxPerZone: 3
`)

	h.WriteInputFile("dexauthenticator.yaml", `
apiVersion: deckhouse.io/v1
kind: DexAuthenticator
metadata:
  name: myapp-auth
  namespace: default
  labels:
    app: myapp
spec:
  applicationDomain: myapp.example.com
  sendAuthorizationHeader: true
  applicationIngressCertificateSecretName: myapp-tls
  applicationIngressClassName: nginx
`)

	h.WriteInputFile("user.yaml", `
apiVersion: deckhouse.io/v1
kind: User
metadata:
  name: admin-user
  labels:
    app: admin
spec:
  email: admin@example.com
  password: "$2a$10$abc"
  groups:
    - admins
`)

	h.WriteInputFile("group.yaml", `
apiVersion: deckhouse.io/v1
kind: Group
metadata:
  name: admins
  labels:
    app: admin
spec:
  members:
    - kind: User
      name: admin-user
`)

	h.WriteInputFile("servicemonitor.yaml", `
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: myapp-monitor
  namespace: default
  labels:
    app: myapp
spec:
  selector:
    matchLabels:
      app: myapp
  endpoints:
    - port: metrics
      interval: 30s
`)

	h.WriteInputFile("podmonitor.yaml", `
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  name: myapp-pods
  namespace: default
  labels:
    app: myapp
spec:
  selector:
    matchLabels:
      app: myapp
  podMetricsEndpoints:
    - port: metrics
      interval: 30s
`)

	h.WriteInputFile("prometheusrule.yaml", `
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: myapp-alerts
  namespace: default
  labels:
    app: myapp
spec:
  groups:
    - name: myapp
      rules:
        - alert: MyAppDown
          expr: up{job="myapp"} == 0
          for: 5m
`)

	h.WriteInputFile("grafanadashboard.yaml", `
apiVersion: v1
kind: ConfigMap
metadata:
  name: myapp-dashboard
  namespace: default
  labels:
    app: myapp
    grafana_dashboard: "1"
data:
  dashboard.json: |
    {"title": "MyApp Dashboard"}
`)

	h.WriteInputFile("httproute.yaml", `
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: myapp-route
  namespace: default
  labels:
    app: myapp
spec:
  parentRefs:
    - name: main-gateway
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: myapp
          port: 80
`)

	h.WriteInputFile("gateway.yaml", `
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: main-gateway
  namespace: default
  labels:
    app: main
spec:
  gatewayClassName: nginx
  listeners:
    - name: http
      port: 80
      protocol: HTTP
`)

	h.WriteInputFile("scaledobject.yaml", `
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: myapp-scaler
  namespace: default
  labels:
    app: myapp
spec:
  scaleTargetRef:
    name: myapp
  triggers:
    - type: prometheus
      metadata:
        serverAddress: http://prometheus:9090
        metricName: http_requests_total
        threshold: "100"
`)

	h.WriteInputFile("triggerauthentication.yaml", `
apiVersion: keda.sh/v1alpha1
kind: TriggerAuthentication
metadata:
  name: myapp-trigger-auth
  namespace: default
  labels:
    app: myapp
spec:
  secretTargetRef:
    - parameter: password
      name: myapp-secret
      key: password
`)

	h.WriteInputFile("certificate.yaml", `
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: myapp-tls
  namespace: default
  labels:
    app: myapp
spec:
  secretName: myapp-tls
  issuerRef:
    name: letsencrypt
    kind: ClusterIssuer
  dnsNames:
    - myapp.example.com
`)

	h.WriteInputFile("clusterissuer.yaml", `
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt
  labels:
    app: letsencrypt
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: admin@example.com
    privateKeySecretRef:
      name: letsencrypt-key
    solvers:
      - http01:
          ingress:
            class: nginx
`)

	h.WriteInputFile("rollout.yaml", `
apiVersion: argoproj.io/v1alpha1
kind: Rollout
metadata:
  name: myapp-rollout
  namespace: default
  labels:
    app: myapp
spec:
  replicas: 2
  selector:
    matchLabels:
      app: myapp
  template:
    metadata:
      labels:
        app: myapp
    spec:
      containers:
        - name: myapp
          image: myapp:latest
  strategy:
    canary:
      steps:
        - setWeight: 20
        - pause: {}
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "all-processors-test",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// Collect all processed resource kinds
	processedKinds := make(map[string]int)
	for _, res := range output.Resources {
		kind := res.Original.Object.GetKind()
		processedKinds[kind]++
	}

	// Also collect template paths to detect GrafanaDashboard
	grafanaDashboardProcessed := false
	for _, chart := range output.Charts {
		for path := range chart.Templates {
			if strings.Contains(path, "grafana-dashboard") {
				grafanaDashboardProcessed = true
			}
		}
	}

	expectedKinds := []string{
		"ModuleConfig",
		"IngressNginxController",
		"ClusterAuthorizationRule",
		"NodeGroup",
		"DexAuthenticator",
		"User",
		"Group",
		"ServiceMonitor",
		"PodMonitor",
		"PrometheusRule",
		"HTTPRoute",
		"Gateway",
		"ScaledObject",
		"TriggerAuthentication",
		"Certificate",
		"ClusterIssuer",
		"Rollout",
	}

	for _, kind := range expectedKinds {
		if processedKinds[kind] == 0 {
			t.Errorf("Expected kind %q to be processed, but it was not found in output.Resources", kind)
		}
	}

	// GrafanaDashboard is a ConfigMap with grafana_dashboard label;
	// verify it produced a grafana-dashboard template in the chart.
	if !grafanaDashboardProcessed {
		t.Error("Expected GrafanaDashboard (ConfigMap with grafana_dashboard label) to produce a grafana-dashboard template")
	}

	// Total ConfigMaps should be at least 1 (the dashboard ConfigMap)
	if processedKinds["ConfigMap"] == 0 {
		t.Error("Expected at least one ConfigMap (the GrafanaDashboard) to be processed")
	}
}

// ============================================================
// v0.4.0 — Module scaffold via generator package
// ============================================================

func TestPipelineDeckhouse_ModuleScaffold(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	h.WriteInputFile("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
  labels:
    app: myapp
spec:
  replicas: 1
  selector:
    matchLabels:
      app: myapp
  template:
    metadata:
      labels:
        app: myapp
    spec:
      containers:
        - name: myapp
          image: myapp:latest
          ports:
            - containerPort: 8080
`)

	h.WriteInputFile("service.yaml", `
apiVersion: v1
kind: Service
metadata:
  name: myapp
  labels:
    app: myapp
spec:
  type: ClusterIP
  selector:
    app: myapp
  ports:
    - port: 80
      targetPort: 8080
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "scaffold-test",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	if len(output.Charts) == 0 {
		t.Fatal("Expected at least 1 chart from pipeline")
	}

	chart := output.Charts[0]

	// Apply Deckhouse module scaffold
	result := generator.GenerateDeckhouseModule(chart, nil)

	// Verify ChartYAML contains helm_lib dependency
	if !strings.Contains(result.ChartYAML, "helm_lib") {
		t.Error("Expected result.ChartYAML to contain 'helm_lib' dependency")
	}

	// Verify ExternalFiles contains the required paths
	requiredPaths := []string{
		"openapi/config-values.yaml",
		"openapi/values.yaml",
		"images/README.md",
		"hooks/README.md",
	}

	externalPathSet := make(map[string]bool)
	for _, ef := range result.ExternalFiles {
		externalPathSet[ef.Path] = true
	}

	for _, path := range requiredPaths {
		if !externalPathSet[path] {
			t.Errorf("Expected ExternalFiles to contain path %q", path)
		}
	}

	// Verify templates contain helm_lib comment
	helmLibFoundInTemplates := false
	for _, content := range result.Templates {
		if strings.Contains(content, "helm_lib") {
			helmLibFoundInTemplates = true
			break
		}
	}
	if !helmLibFoundInTemplates {
		t.Error("Expected at least one template to contain 'helm_lib' comment")
	}
}

// ============================================================
// v0.4.0 — Helm lint validation
// ============================================================

func TestPipelineDeckhouse_HelmLint(t *testing.T) {
	helmPath, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm not installed, skipping lint test")
	}

	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	h.WriteInputFile("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
  labels:
    app: myapp
spec:
  replicas: 1
  selector:
    matchLabels:
      app: myapp
  template:
    metadata:
      labels:
        app: myapp
    spec:
      containers:
        - name: myapp
          image: myapp:latest
          ports:
            - containerPort: 8080
`)

	h.WriteInputFile("service.yaml", `
apiVersion: v1
kind: Service
metadata:
  name: myapp
  labels:
    app: myapp
spec:
  type: ClusterIP
  selector:
    app: myapp
  ports:
    - port: 80
      targetPort: 8080
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "helm-lint-test",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	if len(output.Charts) == 0 {
		t.Fatal("Expected at least 1 chart from pipeline")
	}

	chart := output.Charts[0]
	chartDir := filepath.Join(output.OutputDir, chart.Name)

	// Run helm lint against the generated chart directory
	cmd := exec.Command(helmPath, "lint", chartDir)
	lintOutput, lintErr := cmd.CombinedOutput()
	if lintErr != nil {
		t.Fatalf("helm lint failed for chart %q:\n%s\nerror: %v", chartDir, string(lintOutput), lintErr)
	}
}

// ============================================================
// Helper functions
// ============================================================

// templateNames extracts template path names from a map.
func templateNames(templates map[string]string) []string {
	names := make([]string, 0, len(templates))
	for path := range templates {
		names = append(names, path)
	}
	return names
}

// mapKeys extracts keys from a map.
func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// toInt converts numeric types to int for comparison.
func toInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}
