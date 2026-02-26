package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// Test Helpers
// ============================================================

func makeChart(name string, templates map[string]string) *types.GeneratedChart {
	return &types.GeneratedChart{
		Name:       name,
		ChartYAML:  "apiVersion: v2\nname: " + name + "\nversion: 0.1.0\n",
		ValuesYAML: "replicaCount: 1\n",
		Templates:  templates,
	}
}

// ============================================================
// Subtask 1: ExtractImageReferences — basic extraction
// ============================================================

func TestAirgap_ExtractImageReferences_SingleImage(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  template:
    spec:
      containers:
        - name: app
          image: nginx:1.21`,
	})

	refs := ExtractImageReferences(chart)
	if len(refs) != 1 {
		t.Fatalf("expected 1 image ref, got %d", len(refs))
	}
	if refs[0].Repository != "nginx" {
		t.Errorf("expected repository 'nginx', got '%s'", refs[0].Repository)
	}
	if refs[0].Tag != "1.21" {
		t.Errorf("expected tag '1.21', got '%s'", refs[0].Tag)
	}
}

func TestAirgap_ExtractImageReferences_MultipleImages(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": `apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
        - name: app
          image: nginx:1.21
        - name: sidecar
          image: envoyproxy/envoy:v1.28`,
		"templates/worker.yaml": `apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
        - name: worker
          image: redis:7.2`,
	})

	refs := ExtractImageReferences(chart)
	if len(refs) < 3 {
		t.Fatalf("expected at least 3 image refs, got %d", len(refs))
	}
}

func TestAirgap_ExtractImageReferences_NoImages(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/configmap.yaml": `apiVersion: v1
kind: ConfigMap
metadata:
  name: myconfig
data:
  key: value`,
	})

	refs := ExtractImageReferences(chart)
	if len(refs) != 0 {
		t.Errorf("expected 0 image refs for chart without images, got %d", len(refs))
	}
}

func TestAirgap_ExtractImageReferences_InitContainers(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": `apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      initContainers:
        - name: init-db
          image: busybox:1.36
      containers:
        - name: app
          image: myapp:latest`,
	})

	refs := ExtractImageReferences(chart)
	if len(refs) < 2 {
		t.Fatalf("expected at least 2 image refs (init + main container), got %d", len(refs))
	}

	// Verify busybox is found
	found := false
	for _, ref := range refs {
		if ref.Repository == "busybox" {
			found = true
			break
		}
	}
	if !found {
		t.Error("initContainer image 'busybox' not found in extracted refs")
	}
}

func TestAirgap_ExtractImageReferences_DigestRef(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": `apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
        - name: app
          image: nginx@sha256:abcdef1234567890abcdef1234567890`,
	})

	refs := ExtractImageReferences(chart)
	if len(refs) != 1 {
		t.Fatalf("expected 1 image ref, got %d", len(refs))
	}
	if refs[0].Digest == "" {
		t.Error("expected digest to be parsed from image@sha256:... reference")
	}
	if refs[0].Repository != "nginx" {
		t.Errorf("expected repository 'nginx', got '%s'", refs[0].Repository)
	}
}

func TestAirgap_ExtractImageReferences_FullRegistryPath(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": `apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
        - name: app
          image: registry.example.com/team/myapp:v2.1.0`,
	})

	refs := ExtractImageReferences(chart)
	if len(refs) != 1 {
		t.Fatalf("expected 1 image ref, got %d", len(refs))
	}
	if !strings.Contains(refs[0].FullRef, "registry.example.com") {
		t.Errorf("expected FullRef to contain registry.example.com, got '%s'", refs[0].FullRef)
	}
}

// ============================================================
// Subtask 2: GenerateImageList
// ============================================================

func TestAirgap_GenerateImageList_Sorted(t *testing.T) {
	refs := []ImageRef{
		{Repository: "redis", Tag: "7.2", FullRef: "redis:7.2"},
		{Repository: "nginx", Tag: "1.21", FullRef: "nginx:1.21"},
		{Repository: "alpine", Tag: "3.18", FullRef: "alpine:3.18"},
	}

	list := GenerateImageList(refs)
	lines := strings.Split(strings.TrimSpace(list), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	// Must be sorted alphabetically
	if lines[0] != "alpine:3.18" {
		t.Errorf("expected first line 'alpine:3.18', got '%s'", lines[0])
	}
	if lines[1] != "nginx:1.21" {
		t.Errorf("expected second line 'nginx:1.21', got '%s'", lines[1])
	}
	if lines[2] != "redis:7.2" {
		t.Errorf("expected third line 'redis:7.2', got '%s'", lines[2])
	}
}

func TestAirgap_GenerateImageList_Deduplicated(t *testing.T) {
	refs := []ImageRef{
		{Repository: "nginx", Tag: "1.21", FullRef: "nginx:1.21"},
		{Repository: "nginx", Tag: "1.21", FullRef: "nginx:1.21"},
		{Repository: "redis", Tag: "7.2", FullRef: "redis:7.2"},
	}

	list := GenerateImageList(refs)
	lines := strings.Split(strings.TrimSpace(list), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 unique lines after dedup, got %d: %v", len(lines), lines)
	}
}

func TestAirgap_GenerateImageList_EmptyRefs(t *testing.T) {
	list := GenerateImageList(nil)
	trimmed := strings.TrimSpace(list)
	if trimmed != "" {
		t.Errorf("expected empty output for nil refs, got '%s'", trimmed)
	}
}

// ============================================================
// Subtask 3: GenerateAirgapValues
// ============================================================

func TestAirgap_GenerateAirgapValues_RegistryOverride(t *testing.T) {
	refs := []ImageRef{
		{Repository: "nginx", Tag: "1.21", FullRef: "nginx:1.21"},
	}

	values := GenerateAirgapValues(refs, "registry.internal.com")

	global, ok := values["global"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'global' key in airgap values")
	}

	registry, ok := global["imageRegistry"]
	if !ok {
		t.Fatal("expected 'global.imageRegistry' in airgap values")
	}
	if registry != "registry.internal.com" {
		t.Errorf("expected registry 'registry.internal.com', got '%v'", registry)
	}
}

func TestAirgap_GenerateAirgapValues_ImagePullSecrets(t *testing.T) {
	refs := []ImageRef{
		{Repository: "nginx", Tag: "1.21", FullRef: "nginx:1.21"},
	}

	values := GenerateAirgapValues(refs, "registry.internal.com")

	global, ok := values["global"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'global' key in airgap values")
	}

	_, ok = global["imagePullSecrets"]
	if !ok {
		t.Error("expected 'global.imagePullSecrets' in airgap values")
	}
}

// ============================================================
// Subtask 4: GenerateMirrorScript
// ============================================================

func TestAirgap_GenerateMirrorScript_SkopeoCommands(t *testing.T) {
	refs := []ImageRef{
		{Repository: "nginx", Tag: "1.21", FullRef: "nginx:1.21"},
		{Repository: "redis", Tag: "7.2", FullRef: "redis:7.2"},
	}

	script := GenerateMirrorScript(refs, "registry.internal.com")

	if !strings.Contains(script, "skopeo copy") {
		t.Error("expected 'skopeo copy' commands in mirror script")
	}
	if !strings.Contains(script, "nginx:1.21") {
		t.Error("expected source image 'nginx:1.21' in mirror script")
	}
	if !strings.Contains(script, "registry.internal.com") {
		t.Error("expected target registry in mirror script")
	}
	if !strings.Contains(script, "redis:7.2") {
		t.Error("expected source image 'redis:7.2' in mirror script")
	}
}

func TestAirgap_GenerateMirrorScript_EmptyRefs(t *testing.T) {
	script := GenerateMirrorScript(nil, "registry.internal.com")

	// Should have a header (shebang) but no copy commands
	if !strings.Contains(script, "#!/") {
		t.Error("expected shebang in mirror script even with empty refs")
	}
	if strings.Contains(script, "skopeo copy") {
		t.Error("expected no 'skopeo copy' commands for empty refs")
	}
}

func TestAirgap_GenerateMirrorScript_DigestRef(t *testing.T) {
	refs := []ImageRef{
		{Repository: "nginx", Digest: "sha256:abcdef123456", FullRef: "nginx@sha256:abcdef123456"},
	}

	script := GenerateMirrorScript(refs, "registry.internal.com")

	if !strings.Contains(script, "nginx@sha256:abcdef123456") {
		t.Error("expected digest reference preserved in mirror script")
	}
}
