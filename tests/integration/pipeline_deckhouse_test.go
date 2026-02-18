package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	sigsyaml "sigs.k8s.io/yaml"
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
