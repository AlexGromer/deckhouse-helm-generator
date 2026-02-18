package generator

import (
	"sort"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// Test Helpers
// ============================================================

// makeProcessedResource creates a ProcessedResource for testing.
func makeProcessedResource(kind, name, namespace string, labels map[string]string) *types.ProcessedResource {
	metadata := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
	}
	if labels != nil {
		labelMap := make(map[string]interface{}, len(labels))
		for k, v := range labels {
			labelMap[k] = v
		}
		metadata["labels"] = labelMap
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": gvkForKind(kind).GroupVersion().String(),
			"kind":       kind,
			"metadata":   metadata,
		},
	}

	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    gvkForKind(kind),
		},
		ServiceName: "",
		Values:      make(map[string]interface{}),
	}
}

// gvkForKind returns a schema.GroupVersionKind for common K8s kinds.
func gvkForKind(kind string) schema.GroupVersionKind {
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet":
		return schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kind}
	case "CronJob", "Job":
		return schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: kind}
	case "Ingress", "NetworkPolicy":
		return schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: kind}
	case "HorizontalPodAutoscaler":
		return schema.GroupVersionKind{Group: "autoscaling", Version: "v2", Kind: kind}
	case "PodDisruptionBudget":
		return schema.GroupVersionKind{Group: "policy", Version: "v1", Kind: kind}
	case "Role", "ClusterRole":
		return schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: kind}
	case "RoleBinding", "ClusterRoleBinding":
		return schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: kind}
	case "PersistentVolumeClaim":
		return schema.GroupVersionKind{Group: "", Version: "v1", Kind: kind}
	default:
		return schema.GroupVersionKind{Group: "", Version: "v1", Kind: kind}
	}
}

// buildGraph creates a ResourceGraph from resources and relationships.
func buildGraph(resources []*types.ProcessedResource, relationships []types.Relationship) *types.ResourceGraph {
	graph := types.NewResourceGraph()
	for _, r := range resources {
		graph.AddResource(r)
	}
	for _, rel := range relationships {
		graph.AddRelationship(rel)
	}
	return graph
}

// resourceKey returns a ResourceKey for a ProcessedResource.
func resourceKey(r *types.ProcessedResource) types.ResourceKey {
	return r.Original.ResourceKey()
}

// findGroup finds a group by name in the result.
func findGroup(result *GroupingResult, name string) *ServiceGroup {
	for _, g := range result.Groups {
		if g.Name == name {
			return g
		}
	}
	return nil
}

// sortedGroupNames returns sorted group names from result.
func sortedGroupNames(result *GroupingResult) []string {
	names := make([]string, len(result.Groups))
	for i, g := range result.Groups {
		names[i] = g.Name
	}
	sort.Strings(names)
	return names
}

// ============================================================
// Subtask 1: Group by app.kubernetes.io/name label (single app)
// ============================================================

func TestGroupResources_ByLabel_SingleApp(t *testing.T) {
	// Input: 3 resources (Deployment+Service+ConfigMap) all with label app.kubernetes.io/name: myapp
	// Expected: 1 group named "myapp" with 3 resources
	deploy := makeProcessedResource("Deployment", "myapp-deploy", "default",
		map[string]string{"app.kubernetes.io/name": "myapp"})
	svc := makeProcessedResource("Service", "myapp-svc", "default",
		map[string]string{"app.kubernetes.io/name": "myapp"})
	cm := makeProcessedResource("ConfigMap", "myapp-config", "default",
		map[string]string{"app.kubernetes.io/name": "myapp"})

	graph := buildGraph([]*types.ProcessedResource{deploy, svc, cm}, nil)

	result, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	if len(result.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result.Groups))
	}

	group := result.Groups[0]
	if group.Name != "myapp" {
		t.Errorf("expected group name 'myapp', got '%s'", group.Name)
	}
	if len(group.Resources) != 3 {
		t.Errorf("expected 3 resources in group, got %d", len(group.Resources))
	}
}

// ============================================================
// Subtask 2: Group by app.kubernetes.io/name label (multiple apps)
// ============================================================

func TestGroupResources_ByLabel_MultipleApps(t *testing.T) {
	// Input: 6 resources, 2 apps
	// "frontend": Deployment+Service+Ingress
	// "backend": Deployment+Service+ConfigMap
	// Expected: 2 groups, 3 resources each
	frontDeploy := makeProcessedResource("Deployment", "frontend-deploy", "default",
		map[string]string{"app.kubernetes.io/name": "frontend"})
	frontSvc := makeProcessedResource("Service", "frontend-svc", "default",
		map[string]string{"app.kubernetes.io/name": "frontend"})
	frontIngress := makeProcessedResource("Ingress", "frontend-ingress", "default",
		map[string]string{"app.kubernetes.io/name": "frontend"})

	backDeploy := makeProcessedResource("Deployment", "backend-deploy", "default",
		map[string]string{"app.kubernetes.io/name": "backend"})
	backSvc := makeProcessedResource("Service", "backend-svc", "default",
		map[string]string{"app.kubernetes.io/name": "backend"})
	backCM := makeProcessedResource("ConfigMap", "backend-config", "default",
		map[string]string{"app.kubernetes.io/name": "backend"})

	graph := buildGraph([]*types.ProcessedResource{
		frontDeploy, frontSvc, frontIngress,
		backDeploy, backSvc, backCM,
	}, nil)

	result, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	if len(result.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(result.Groups))
	}

	names := sortedGroupNames(result)
	if names[0] != "backend" || names[1] != "frontend" {
		t.Errorf("expected groups 'backend' and 'frontend', got %v", names)
	}

	frontend := findGroup(result, "frontend")
	if frontend == nil {
		t.Fatal("frontend group not found")
	}
	if len(frontend.Resources) != 3 {
		t.Errorf("expected 3 resources in frontend, got %d", len(frontend.Resources))
	}

	backend := findGroup(result, "backend")
	if backend == nil {
		t.Fatal("backend group not found")
	}
	if len(backend.Resources) != 3 {
		t.Errorf("expected 3 resources in backend, got %d", len(backend.Resources))
	}
}

func TestGroupResources_ByLabel_AppWithSingleResource(t *testing.T) {
	// Input: 1 Deployment with label app.kubernetes.io/name: worker
	// Expected: 1 group "worker" with 1 resource
	deploy := makeProcessedResource("Deployment", "worker-deploy", "default",
		map[string]string{"app.kubernetes.io/name": "worker"})

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	result, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	if len(result.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result.Groups))
	}
	if result.Groups[0].Name != "worker" {
		t.Errorf("expected group name 'worker', got '%s'", result.Groups[0].Name)
	}
	if len(result.Groups[0].Resources) != 1 {
		t.Errorf("expected 1 resource, got %d", len(result.Groups[0].Resources))
	}
}

// ============================================================
// Subtask 3: Group by namespace when labels absent
// ============================================================

func TestGroupResources_ByNamespace_NoLabels(t *testing.T) {
	// Input: 4 resources across 2 namespaces (ns-a: Deployment+Service, ns-b: ConfigMap+Secret), no standard labels
	// Expected: 2 groups named "ns-a" and "ns-b"
	deployA := makeProcessedResource("Deployment", "app-deploy", "ns-a", nil)
	svcA := makeProcessedResource("Service", "app-svc", "ns-a", nil)
	cmB := makeProcessedResource("ConfigMap", "config", "ns-b", nil)
	secretB := makeProcessedResource("Secret", "creds", "ns-b", nil)

	graph := buildGraph([]*types.ProcessedResource{deployA, svcA, cmB, secretB}, nil)

	result, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	if len(result.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(result.Groups))
	}

	names := sortedGroupNames(result)
	if names[0] != "ns-a" || names[1] != "ns-b" {
		t.Errorf("expected groups 'ns-a' and 'ns-b', got %v", names)
	}

	nsA := findGroup(result, "ns-a")
	if nsA == nil {
		t.Fatal("ns-a group not found")
	}
	if len(nsA.Resources) != 2 {
		t.Errorf("expected 2 resources in ns-a, got %d", len(nsA.Resources))
	}

	nsB := findGroup(result, "ns-b")
	if nsB == nil {
		t.Fatal("ns-b group not found")
	}
	if len(nsB.Resources) != 2 {
		t.Errorf("expected 2 resources in ns-b, got %d", len(nsB.Resources))
	}
}

func TestGroupResources_ByNamespace_DefaultNamespace(t *testing.T) {
	// Input: 2 resources in "default" namespace, no labels
	// Expected: 1 group named "default"
	deploy := makeProcessedResource("Deployment", "app", "default", nil)
	svc := makeProcessedResource("Service", "app-svc", "default", nil)

	graph := buildGraph([]*types.ProcessedResource{deploy, svc}, nil)

	result, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	if len(result.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result.Groups))
	}
	if result.Groups[0].Name != "default" {
		t.Errorf("expected group name 'default', got '%s'", result.Groups[0].Name)
	}
	if len(result.Groups[0].Resources) != 2 {
		t.Errorf("expected 2 resources, got %d", len(result.Groups[0].Resources))
	}
}

// ============================================================
// Subtask 4: Connected components from relationship graph
// ============================================================

func TestGroupResources_ByRelationship_ConnectedComponents(t *testing.T) {
	// Input: 5 resources with relationships forming 2 connected components:
	//   Component 1: Deployment-A → Service-A → Ingress-A (label selector + name reference)
	//   Component 2: Deployment-B → ConfigMap-B (volume mount)
	// Expected: 2 groups, first with 3 resources, second with 2
	deployA := makeProcessedResource("Deployment", "app-a", "default", nil)
	svcA := makeProcessedResource("Service", "svc-a", "default", nil)
	ingressA := makeProcessedResource("Ingress", "ingress-a", "default", nil)
	deployB := makeProcessedResource("Deployment", "app-b", "default", nil)
	cmB := makeProcessedResource("ConfigMap", "config-b", "default", nil)

	relationships := []types.Relationship{
		{From: resourceKey(svcA), To: resourceKey(deployA), Type: types.RelationLabelSelector},
		{From: resourceKey(ingressA), To: resourceKey(svcA), Type: types.RelationNameReference},
		{From: resourceKey(deployB), To: resourceKey(cmB), Type: types.RelationVolumeMount},
	}

	graph := buildGraph([]*types.ProcessedResource{deployA, svcA, ingressA, deployB, cmB}, relationships)

	result, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	if len(result.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(result.Groups))
	}

	// Find which group has 3 resources and which has 2
	var group3, group2 *ServiceGroup
	for _, g := range result.Groups {
		if len(g.Resources) == 3 {
			group3 = g
		} else if len(g.Resources) == 2 {
			group2 = g
		}
	}

	if group3 == nil {
		t.Error("expected a group with 3 resources (connected component 1)")
	}
	if group2 == nil {
		t.Error("expected a group with 2 resources (connected component 2)")
	}
}

func TestGroupResources_ByRelationship_ChainedDependencies(t *testing.T) {
	// Input: 4 resources: Ingress → Service → Deployment → Secret (chain)
	// Expected: 1 group with all 4 resources (transitive closure)
	ingress := makeProcessedResource("Ingress", "app-ingress", "default", nil)
	svc := makeProcessedResource("Service", "app-svc", "default", nil)
	deploy := makeProcessedResource("Deployment", "app-deploy", "default", nil)
	secret := makeProcessedResource("Secret", "app-secret", "default", nil)

	relationships := []types.Relationship{
		{From: resourceKey(ingress), To: resourceKey(svc), Type: types.RelationNameReference},
		{From: resourceKey(svc), To: resourceKey(deploy), Type: types.RelationLabelSelector},
		{From: resourceKey(deploy), To: resourceKey(secret), Type: types.RelationVolumeMount},
	}

	graph := buildGraph([]*types.ProcessedResource{ingress, svc, deploy, secret}, relationships)

	result, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	if len(result.Groups) != 1 {
		t.Fatalf("expected 1 group (transitive closure), got %d", len(result.Groups))
	}
	if len(result.Groups[0].Resources) != 4 {
		t.Errorf("expected 4 resources in group, got %d", len(result.Groups[0].Resources))
	}
}

// ============================================================
// Subtask 5: Strategy priority (label > relationship > namespace > individual)
// ============================================================

func TestGroupResources_StrategyPriority_LabelOverNamespace(t *testing.T) {
	// Input: 3 resources in same namespace but different app labels
	// Expected: Grouped by label, NOT by namespace
	deployFront := makeProcessedResource("Deployment", "frontend", "production",
		map[string]string{"app.kubernetes.io/name": "frontend"})
	deployBack := makeProcessedResource("Deployment", "backend", "production",
		map[string]string{"app.kubernetes.io/name": "backend"})
	svcFront := makeProcessedResource("Service", "frontend-svc", "production",
		map[string]string{"app.kubernetes.io/name": "frontend"})

	graph := buildGraph([]*types.ProcessedResource{deployFront, deployBack, svcFront}, nil)

	result, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	if len(result.Groups) != 2 {
		t.Fatalf("expected 2 groups (by label, not namespace), got %d", len(result.Groups))
	}

	names := sortedGroupNames(result)
	if names[0] != "backend" || names[1] != "frontend" {
		t.Errorf("expected groups 'backend' and 'frontend', got %v", names)
	}

	frontend := findGroup(result, "frontend")
	if len(frontend.Resources) != 2 {
		t.Errorf("expected 2 resources in frontend, got %d", len(frontend.Resources))
	}

	backend := findGroup(result, "backend")
	if len(backend.Resources) != 1 {
		t.Errorf("expected 1 resource in backend, got %d", len(backend.Resources))
	}
}

func TestGroupResources_StrategyPriority_RelationshipOverNamespace(t *testing.T) {
	// Input: 2 resources in different namespaces but connected by relationship
	// Expected: 1 group (relationship wins)
	deploy := makeProcessedResource("Deployment", "app", "ns-a", nil)
	cm := makeProcessedResource("ConfigMap", "app-config", "ns-b", nil)

	relationships := []types.Relationship{
		{From: resourceKey(deploy), To: resourceKey(cm), Type: types.RelationVolumeMount},
	}

	graph := buildGraph([]*types.ProcessedResource{deploy, cm}, relationships)

	result, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	if len(result.Groups) != 1 {
		t.Fatalf("expected 1 group (relationship over namespace), got %d", len(result.Groups))
	}
	if len(result.Groups[0].Resources) != 2 {
		t.Errorf("expected 2 resources in group, got %d", len(result.Groups[0].Resources))
	}
}

// ============================================================
// Subtask 6: Edge cases
// ============================================================

func TestGroupResources_Edge_SingleResource(t *testing.T) {
	// Input: 1 Deployment, no labels, no relationships
	// Expected: 1 group with 1 resource
	deploy := makeProcessedResource("Deployment", "lonely-app", "default", nil)

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	result, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	if len(result.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result.Groups))
	}
	if len(result.Groups[0].Resources) != 1 {
		t.Errorf("expected 1 resource, got %d", len(result.Groups[0].Resources))
	}
}

func TestGroupResources_Edge_EmptyInput(t *testing.T) {
	// Input: empty resource list
	// Expected: 0 groups, no error
	graph := buildGraph(nil, nil)

	result, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	if len(result.Groups) != 0 {
		t.Errorf("expected 0 groups, got %d", len(result.Groups))
	}
}

func TestGroupResources_Edge_OrphanResources(t *testing.T) {
	// Input: 3 resources: 2 with label "myapp", 1 orphan (no labels, no relationships)
	// Expected: 2 groups: "myapp" (2 resources), orphan in its own group
	deploy := makeProcessedResource("Deployment", "myapp-deploy", "default",
		map[string]string{"app.kubernetes.io/name": "myapp"})
	svc := makeProcessedResource("Service", "myapp-svc", "default",
		map[string]string{"app.kubernetes.io/name": "myapp"})
	orphan := makeProcessedResource("ConfigMap", "orphan-config", "other-ns", nil)

	graph := buildGraph([]*types.ProcessedResource{deploy, svc, orphan}, nil)

	result, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	if len(result.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(result.Groups))
	}

	myapp := findGroup(result, "myapp")
	if myapp == nil {
		t.Fatal("myapp group not found")
	}
	if len(myapp.Resources) != 2 {
		t.Errorf("expected 2 resources in myapp, got %d", len(myapp.Resources))
	}
}

func TestGroupResources_Edge_ConflictingLabels(t *testing.T) {
	// Input: Resource with app.kubernetes.io/name: A and app: B
	// Expected: Grouped by app.kubernetes.io/name (standard label priority)
	deploy := makeProcessedResource("Deployment", "dual-label", "default",
		map[string]string{
			"app.kubernetes.io/name": "standard-app",
			"app":                    "legacy-app",
		})

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	result, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	if len(result.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result.Groups))
	}
	if result.Groups[0].Name != "standard-app" {
		t.Errorf("expected group name 'standard-app' (app.kubernetes.io/name priority), got '%s'", result.Groups[0].Name)
	}
}

// ============================================================
// Subtask 7: Group naming
// ============================================================

func TestGroupResources_GroupNaming_FromLabel(t *testing.T) {
	// Expected: Group name = value of app.kubernetes.io/name label
	deploy := makeProcessedResource("Deployment", "web-server", "default",
		map[string]string{"app.kubernetes.io/name": "my-web-app"})

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	result, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	if len(result.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result.Groups))
	}
	if result.Groups[0].Name != "my-web-app" {
		t.Errorf("expected group name 'my-web-app', got '%s'", result.Groups[0].Name)
	}
}

func TestGroupResources_GroupNaming_FromNamespace(t *testing.T) {
	// Expected: Group name = namespace when no labels
	deploy := makeProcessedResource("Deployment", "app", "monitoring", nil)

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	result, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	if len(result.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result.Groups))
	}
	if result.Groups[0].Name != "monitoring" {
		t.Errorf("expected group name 'monitoring', got '%s'", result.Groups[0].Name)
	}
}

func TestGroupResources_GroupNaming_FromResourceName(t *testing.T) {
	// Expected: For orphans with empty namespace, group name = first resource's name
	deploy := makeProcessedResource("Deployment", "standalone-worker", "", nil)

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	result, err := GroupResources(graph)
	if err != nil {
		t.Fatalf("GroupResources returned error: %v", err)
	}

	if len(result.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result.Groups))
	}
	if result.Groups[0].Name != "standalone-worker" {
		t.Errorf("expected group name 'standalone-worker', got '%s'", result.Groups[0].Name)
	}
}
