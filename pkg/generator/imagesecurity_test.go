package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// 2.6.6: Image Security — AnalyzeImageSecurity
// ============================================================

func TestImageSecurity_DetectsLatestTag(t *testing.T) {
	chart := &types.GeneratedChart{
		Templates: map[string]string{
			"templates/app-deployment.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  template:
    spec:
      containers:
        - name: app
          image: myapp:latest
`,
		},
	}

	findings := AnalyzeImageSecurity(chart)

	hasLatest := false
	for _, f := range findings {
		if f.Type == FindingLatestTag {
			hasLatest = true
			break
		}
	}
	if !hasLatest {
		t.Error("expected finding for :latest tag")
	}
}

func TestImageSecurity_DetectsUntaggedImage(t *testing.T) {
	chart := &types.GeneratedChart{
		Templates: map[string]string{
			"templates/app-deployment.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  template:
    spec:
      containers:
        - name: app
          image: nginx
`,
		},
	}

	findings := AnalyzeImageSecurity(chart)

	hasUntagged := false
	for _, f := range findings {
		if f.Type == FindingUntagged {
			hasUntagged = true
			break
		}
	}
	if !hasUntagged {
		t.Error("expected finding for untagged image (implicit :latest)")
	}
}

func TestImageSecurity_DetectsMissingPullPolicy(t *testing.T) {
	chart := &types.GeneratedChart{
		Templates: map[string]string{
			"templates/app-deployment.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  template:
    spec:
      containers:
        - name: app
          image: myapp:v1.0
`,
		},
	}

	findings := AnalyzeImageSecurity(chart)

	hasNoPullPolicy := false
	for _, f := range findings {
		if f.Type == FindingNoPullPolicy {
			hasNoPullPolicy = true
			break
		}
	}
	if !hasNoPullPolicy {
		t.Error("expected finding for missing imagePullPolicy")
	}
}

func TestImageSecurity_NoFindingsForTaggedWithPolicy(t *testing.T) {
	chart := &types.GeneratedChart{
		Templates: map[string]string{
			"templates/app-deployment.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  template:
    spec:
      containers:
        - name: app
          image: myapp:v1.0.0
          imagePullPolicy: IfNotPresent
`,
		},
	}

	findings := AnalyzeImageSecurity(chart)

	if len(findings) != 0 {
		t.Errorf("expected 0 findings for properly tagged image with pull policy, got %d: %+v", len(findings), findings)
	}
}

func TestImageSecurity_NilChart(t *testing.T) {
	findings := AnalyzeImageSecurity(nil)
	if findings != nil {
		t.Errorf("expected nil for nil chart, got %v", findings)
	}
}

// ============================================================
// 2.6.6: Image Security — InjectImageDefaults
// ============================================================

func TestImageSecurity_InjectsPullPolicy(t *testing.T) {
	chart := &types.GeneratedChart{
		Templates: map[string]string{
			"templates/app-deployment.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  template:
    spec:
      containers:
        - name: app
          image: myapp:latest
`,
		},
	}

	result := InjectImageDefaults(chart)

	content := result.Templates["templates/app-deployment.yaml"]
	if !strings.Contains(content, "imagePullPolicy: Always") {
		t.Error("expected imagePullPolicy: Always for :latest image")
	}
}

func TestImageSecurity_InjectsPullPolicyIfNotPresent(t *testing.T) {
	chart := &types.GeneratedChart{
		Templates: map[string]string{
			"templates/app-deployment.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  template:
    spec:
      containers:
        - name: app
          image: myapp:v2.1.0
`,
		},
	}

	result := InjectImageDefaults(chart)

	content := result.Templates["templates/app-deployment.yaml"]
	if !strings.Contains(content, "imagePullPolicy: IfNotPresent") {
		t.Error("expected imagePullPolicy: IfNotPresent for tagged image")
	}
}

func TestImageSecurity_InjectsImagePullSecrets_PrivateRegistry(t *testing.T) {
	chart := &types.GeneratedChart{
		Templates: map[string]string{
			"templates/app-deployment.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  template:
    spec:
      containers:
        - name: app
          image: registry.example.com/myapp:v1.0
`,
		},
	}

	result := InjectImageDefaults(chart)

	content := result.Templates["templates/app-deployment.yaml"]
	if !strings.Contains(content, "imagePullSecrets") {
		t.Error("expected imagePullSecrets for private registry image")
	}
	if !strings.Contains(content, "registry-secret") {
		t.Error("expected registry-secret in imagePullSecrets")
	}
}

func TestImageSecurity_NoSecretsForDockerHub(t *testing.T) {
	chart := &types.GeneratedChart{
		Templates: map[string]string{
			"templates/app-deployment.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  template:
    spec:
      containers:
        - name: app
          image: docker.io/library/nginx:1.25
          imagePullPolicy: IfNotPresent
`,
		},
	}

	result := InjectImageDefaults(chart)

	content := result.Templates["templates/app-deployment.yaml"]
	if strings.Contains(content, "imagePullSecrets") {
		t.Error("should not add imagePullSecrets for docker.io images")
	}
}

func TestImageSecurity_NilChartInject(t *testing.T) {
	result := InjectImageDefaults(nil)
	if result != nil {
		t.Error("expected nil for nil chart")
	}
}

func TestImageSecurity_CopyOnWrite(t *testing.T) {
	original := &types.GeneratedChart{
		Name: "test",
		Templates: map[string]string{
			"templates/app-deployment.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  template:
    spec:
      containers:
        - name: app
          image: myapp:latest
`,
		},
	}

	result := InjectImageDefaults(original)

	// Verify original is not modified
	if strings.Contains(original.Templates["templates/app-deployment.yaml"], "imagePullPolicy") {
		t.Error("original chart must not be modified (copy-on-write violation)")
	}

	// Verify result is modified
	if !strings.Contains(result.Templates["templates/app-deployment.yaml"], "imagePullPolicy") {
		t.Error("result chart must contain injected imagePullPolicy")
	}
}

func TestImageSecurity_SkipsNonWorkloads(t *testing.T) {
	chart := &types.GeneratedChart{
		Templates: map[string]string{
			"templates/configmap.yaml": `apiVersion: v1
kind: ConfigMap
metadata:
  name: myconfig
data:
  key: value
`,
		},
	}

	findings := AnalyzeImageSecurity(chart)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for ConfigMap, got %d", len(findings))
	}
}

// ============================================================
// Private registry detection
// ============================================================

func TestIsPrivateRegistry(t *testing.T) {
	tests := []struct {
		image   string
		private bool
	}{
		{"nginx", false},
		{"nginx:1.25", false},
		{"docker.io/library/nginx:1.25", false},
		{"index.docker.io/library/nginx:1.25", false},
		{"registry.example.com/myapp:v1", true},
		{"gcr.io/my-project/myapp:v1", true},
		{"ghcr.io/owner/repo:v1", true},
		{"my-registry:5000/myapp:v1", true},
	}

	for _, tt := range tests {
		got := isPrivateRegistry(tt.image)
		if got != tt.private {
			t.Errorf("isPrivateRegistry(%q) = %v, want %v", tt.image, got, tt.private)
		}
	}
}
