package generator

import (
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// Subtask 1: Extract common image registry
// ============================================================

func TestGlobalValues_ExtractCommon_ImageRegistry(t *testing.T) {
	// Input: 3 services all using registry.example.com/ prefix
	// Expected: global.imageRegistry: registry.example.com
	groups := []*ServiceGroup{
		{
			Name: "frontend",
			Resources: []*types.ProcessedResource{
				makeProcessedResourceWithValues("Deployment", "frontend", "default",
					map[string]string{"app.kubernetes.io/name": "frontend"},
					map[string]interface{}{"image": map[string]interface{}{
						"registry":   "registry.example.com",
						"repository": "frontend",
						"tag":        "1.0",
					}}, ""),
			},
		},
		{
			Name: "backend",
			Resources: []*types.ProcessedResource{
				makeProcessedResourceWithValues("Deployment", "backend", "default",
					map[string]string{"app.kubernetes.io/name": "backend"},
					map[string]interface{}{"image": map[string]interface{}{
						"registry":   "registry.example.com",
						"repository": "backend",
						"tag":        "2.0",
					}}, ""),
			},
		},
		{
			Name: "worker",
			Resources: []*types.ProcessedResource{
				makeProcessedResourceWithValues("Deployment", "worker", "default",
					map[string]string{"app.kubernetes.io/name": "worker"},
					map[string]interface{}{"image": map[string]interface{}{
						"registry":   "registry.example.com",
						"repository": "worker",
						"tag":        "3.0",
					}}, ""),
			},
		},
	}

	globalValues := ExtractGlobalValues(groups)

	registry, ok := getNestedValue(globalValues, "imageRegistry")
	if !ok {
		t.Fatal("global.imageRegistry not found")
	}
	if registry != "registry.example.com" {
		t.Errorf("expected 'registry.example.com', got '%v'", registry)
	}
}

func TestGlobalValues_ExtractCommon_NoCommonRegistry(t *testing.T) {
	// Input: Services using different registries
	// Expected: No global.imageRegistry
	groups := []*ServiceGroup{
		{
			Name: "frontend",
			Resources: []*types.ProcessedResource{
				makeProcessedResourceWithValues("Deployment", "frontend", "default",
					map[string]string{"app.kubernetes.io/name": "frontend"},
					map[string]interface{}{"image": map[string]interface{}{
						"registry":   "registry-a.example.com",
						"repository": "frontend",
					}}, ""),
			},
		},
		{
			Name: "backend",
			Resources: []*types.ProcessedResource{
				makeProcessedResourceWithValues("Deployment", "backend", "default",
					map[string]string{"app.kubernetes.io/name": "backend"},
					map[string]interface{}{"image": map[string]interface{}{
						"registry":   "registry-b.example.com",
						"repository": "backend",
					}}, ""),
			},
		},
	}

	globalValues := ExtractGlobalValues(groups)

	_, ok := getNestedValue(globalValues, "imageRegistry")
	if ok {
		t.Error("global.imageRegistry should not be set when registries differ")
	}
}

// ============================================================
// Subtask 2: Extract common environment variables
// ============================================================

func TestGlobalValues_ExtractCommon_EnvVars(t *testing.T) {
	// Input: All services have env LOG_LEVEL=info and ENV=production
	// Expected: global.env.LOG_LEVEL: info, global.env.ENV: production
	groups := []*ServiceGroup{
		{
			Name: "frontend",
			Resources: []*types.ProcessedResource{
				makeProcessedResourceWithValues("Deployment", "frontend", "default",
					map[string]string{"app.kubernetes.io/name": "frontend"},
					map[string]interface{}{"env": map[string]interface{}{
						"LOG_LEVEL": "info",
						"ENV":       "production",
					}}, ""),
			},
		},
		{
			Name: "backend",
			Resources: []*types.ProcessedResource{
				makeProcessedResourceWithValues("Deployment", "backend", "default",
					map[string]string{"app.kubernetes.io/name": "backend"},
					map[string]interface{}{"env": map[string]interface{}{
						"LOG_LEVEL": "info",
						"ENV":       "production",
					}}, ""),
			},
		},
	}

	globalValues := ExtractGlobalValues(groups)

	envMap, ok := getNestedValue(globalValues, "env")
	if !ok {
		t.Fatal("global.env not found")
	}

	env, ok := envMap.(map[string]interface{})
	if !ok {
		t.Fatalf("global.env is not a map: %T", envMap)
	}

	if env["LOG_LEVEL"] != "info" {
		t.Errorf("expected LOG_LEVEL='info', got '%v'", env["LOG_LEVEL"])
	}
	if env["ENV"] != "production" {
		t.Errorf("expected ENV='production', got '%v'", env["ENV"])
	}
}

func TestGlobalValues_ExtractCommon_PartialOverlap(t *testing.T) {
	// Input: 2 of 3 services share LOG_LEVEL, only 1 has DEBUG=true
	// Expected: LOG_LEVEL in global (>=2 services), DEBUG stays local
	groups := []*ServiceGroup{
		{
			Name: "frontend",
			Resources: []*types.ProcessedResource{
				makeProcessedResourceWithValues("Deployment", "frontend", "default",
					map[string]string{"app.kubernetes.io/name": "frontend"},
					map[string]interface{}{"env": map[string]interface{}{
						"LOG_LEVEL": "info",
					}}, ""),
			},
		},
		{
			Name: "backend",
			Resources: []*types.ProcessedResource{
				makeProcessedResourceWithValues("Deployment", "backend", "default",
					map[string]string{"app.kubernetes.io/name": "backend"},
					map[string]interface{}{"env": map[string]interface{}{
						"LOG_LEVEL": "info",
						"DEBUG":     "true",
					}}, ""),
			},
		},
		{
			Name: "worker",
			Resources: []*types.ProcessedResource{
				makeProcessedResourceWithValues("Deployment", "worker", "default",
					map[string]string{"app.kubernetes.io/name": "worker"},
					map[string]interface{}{"env": map[string]interface{}{
						"LOG_LEVEL": "info",
					}}, ""),
			},
		},
	}

	globalValues := ExtractGlobalValues(groups)

	envMap, ok := getNestedValue(globalValues, "env")
	if !ok {
		t.Fatal("global.env not found")
	}

	env, ok := envMap.(map[string]interface{})
	if !ok {
		t.Fatalf("global.env is not a map: %T", envMap)
	}

	if env["LOG_LEVEL"] != "info" {
		t.Errorf("expected LOG_LEVEL='info' in global, got '%v'", env["LOG_LEVEL"])
	}

	if _, hasDebug := env["DEBUG"]; hasDebug {
		t.Error("DEBUG should NOT be in global (only in 1 service)")
	}
}

// ============================================================
// Subtask 3: Extract common labels
// ============================================================

func TestGlobalValues_ExtractCommon_Labels(t *testing.T) {
	// Input: All services have team: platform, environment: prod
	// Expected: global.labels.team: platform, global.labels.environment: prod
	groups := []*ServiceGroup{
		{
			Name: "frontend",
			Resources: []*types.ProcessedResource{
				makeProcessedResourceWithValues("Deployment", "frontend", "default",
					map[string]string{"app.kubernetes.io/name": "frontend"},
					map[string]interface{}{"commonLabels": map[string]interface{}{
						"team":        "platform",
						"environment": "prod",
					}}, ""),
			},
		},
		{
			Name: "backend",
			Resources: []*types.ProcessedResource{
				makeProcessedResourceWithValues("Deployment", "backend", "default",
					map[string]string{"app.kubernetes.io/name": "backend"},
					map[string]interface{}{"commonLabels": map[string]interface{}{
						"team":        "platform",
						"environment": "prod",
					}}, ""),
			},
		},
	}

	globalValues := ExtractGlobalValues(groups)

	labelsVal, ok := getNestedValue(globalValues, "labels")
	if !ok {
		t.Fatal("global.labels not found")
	}

	labels, ok := labelsVal.(map[string]interface{})
	if !ok {
		t.Fatalf("global.labels is not a map: %T", labelsVal)
	}

	if labels["team"] != "platform" {
		t.Errorf("expected team='platform', got '%v'", labels["team"])
	}
	if labels["environment"] != "prod" {
		t.Errorf("expected environment='prod', got '%v'", labels["environment"])
	}
}

// ============================================================
// Subtask 5: Parent values.yaml structure
// ============================================================

func TestGlobalValues_ParentValues_Structure(t *testing.T) {
	// Expected: Top-level global: section
	groups := []*ServiceGroup{
		{
			Name: "frontend",
			Resources: []*types.ProcessedResource{
				makeProcessedResourceWithValues("Deployment", "frontend", "default",
					map[string]string{"app.kubernetes.io/name": "frontend"},
					map[string]interface{}{"image": map[string]interface{}{
						"registry": "registry.example.com",
					}}, ""),
			},
		},
		{
			Name: "backend",
			Resources: []*types.ProcessedResource{
				makeProcessedResourceWithValues("Deployment", "backend", "default",
					map[string]string{"app.kubernetes.io/name": "backend"},
					map[string]interface{}{"image": map[string]interface{}{
						"registry": "registry.example.com",
					}}, ""),
			},
		},
	}

	globalValues := ExtractGlobalValues(groups)
	if globalValues == nil {
		t.Fatal("global values should not be nil")
	}
	if len(globalValues) == 0 {
		t.Fatal("global values should not be empty when common values exist")
	}
}

// ============================================================
// Subtask 6: Edge cases
// ============================================================

func TestGlobalValues_Edge_NoCommonValues(t *testing.T) {
	// Input: All services have completely different configurations
	// Expected: Empty global section
	groups := []*ServiceGroup{
		{
			Name: "frontend",
			Resources: []*types.ProcessedResource{
				makeProcessedResourceWithValues("Deployment", "frontend", "default",
					map[string]string{"app.kubernetes.io/name": "frontend"},
					map[string]interface{}{"uniqueKey": "a"}, ""),
			},
		},
		{
			Name: "backend",
			Resources: []*types.ProcessedResource{
				makeProcessedResourceWithValues("Deployment", "backend", "default",
					map[string]string{"app.kubernetes.io/name": "backend"},
					map[string]interface{}{"differentKey": "b"}, ""),
			},
		},
	}

	globalValues := ExtractGlobalValues(groups)
	if len(globalValues) != 0 {
		t.Errorf("expected empty global values, got %d entries: %+v", len(globalValues), globalValues)
	}
}

func TestGlobalValues_Edge_SingleService(t *testing.T) {
	// Input: 1 service
	// Expected: No global extraction (nothing to share)
	groups := []*ServiceGroup{
		{
			Name: "only-service",
			Resources: []*types.ProcessedResource{
				makeProcessedResourceWithValues("Deployment", "app", "default",
					map[string]string{"app.kubernetes.io/name": "only-service"},
					map[string]interface{}{"image": map[string]interface{}{
						"registry": "registry.example.com",
					}}, ""),
			},
		},
	}

	globalValues := ExtractGlobalValues(groups)
	if len(globalValues) != 0 {
		t.Errorf("expected empty global values for single service, got %d entries", len(globalValues))
	}
}

// ============================================================
// Helper function for tests
// ============================================================

func getNestedValue(m map[string]interface{}, key string) (interface{}, bool) {
	v, ok := m[key]
	return v, ok
}
