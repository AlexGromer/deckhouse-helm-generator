package extractor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── Registry ─────────────────────────────────────────────────────────────────

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
}

func TestRegistry_Register_And_Get(t *testing.T) {
	r := NewRegistry()
	fe := NewFileExtractor()
	r.Register(fe)

	got, ok := r.Get(types.SourceFile)
	if !ok {
		t.Fatal("Get(SourceFile) returned false")
	}
	if got.Source() != types.SourceFile {
		t.Errorf("Source() = %q; want %q", got.Source(), types.SourceFile)
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get(types.SourceCluster)
	if ok {
		t.Error("Get(SourceCluster) should return false on empty registry")
	}
}

func TestDefaultRegistry(t *testing.T) {
	r := DefaultRegistry()
	fe, ok := r.Get(types.SourceFile)
	if !ok {
		t.Fatal("DefaultRegistry should include file extractor")
	}
	if fe.Source() != types.SourceFile {
		t.Error("file extractor Source() incorrect")
	}
}

// ── FileExtractor.Source ─────────────────────────────────────────────────────

func TestFileExtractor_Source(t *testing.T) {
	fe := NewFileExtractor()
	if fe.Source() != types.SourceFile {
		t.Errorf("Source() = %q; want %q", fe.Source(), types.SourceFile)
	}
}

// ── FileExtractor.Validate ───────────────────────────────────────────────────

func TestFileExtractor_Validate_NoPaths(t *testing.T) {
	fe := NewFileExtractor()
	err := fe.Validate(context.Background(), Options{})
	if err == nil {
		t.Error("Validate with no paths should fail")
	}
}

func TestFileExtractor_Validate_NonexistentPath(t *testing.T) {
	fe := NewFileExtractor()
	err := fe.Validate(context.Background(), Options{Paths: []string{"/nonexistent/path"}})
	if err == nil {
		t.Error("Validate with nonexistent path should fail")
	}
}

func TestFileExtractor_Validate_ValidDir(t *testing.T) {
	dir := t.TempDir()
	fe := NewFileExtractor()
	err := fe.Validate(context.Background(), Options{Paths: []string{dir}})
	if err != nil {
		t.Errorf("Validate with valid dir should pass: %v", err)
	}
}

func TestFileExtractor_Validate_NonYAMLFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "readme.txt")
	if err := os.WriteFile(f, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	fe := NewFileExtractor()
	err := fe.Validate(context.Background(), Options{Paths: []string{f}})
	if err == nil {
		t.Error("Validate with non-YAML file should fail")
	}
}

func TestFileExtractor_Validate_YAMLFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "deploy.yaml")
	if err := os.WriteFile(f, []byte("apiVersion: v1"), 0644); err != nil {
		t.Fatal(err)
	}

	fe := NewFileExtractor()
	err := fe.Validate(context.Background(), Options{Paths: []string{f}})
	if err != nil {
		t.Errorf("Validate with YAML file should pass: %v", err)
	}
}

// ── FileExtractor.Extract ────────────────────────────────────────────────────

func TestFileExtractor_Extract_SingleResource(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "deploy.yaml")
	if err := os.WriteFile(f, []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: default
spec:
  replicas: 1
`), 0644); err != nil {
		t.Fatal(err)
	}

	fe := NewFileExtractor()
	resCh, errCh := fe.Extract(context.Background(), Options{Paths: []string{f}})

	var resources []*types.ExtractedResource
	var errors []error
	for r := range resCh {
		resources = append(resources, r)
	}
	for e := range errCh {
		errors = append(errors, e)
	}

	if len(errors) > 0 {
		t.Fatalf("unexpected errors: %v", errors)
	}
	if len(resources) != 1 {
		t.Fatalf("got %d resources; want 1", len(resources))
	}
	if resources[0].Object.GetKind() != "Deployment" {
		t.Errorf("Kind = %q; want Deployment", resources[0].Object.GetKind())
	}
	if resources[0].Object.GetName() != "web" {
		t.Errorf("Name = %q; want web", resources[0].Object.GetName())
	}
	if resources[0].Source != types.SourceFile {
		t.Errorf("Source = %q; want file", resources[0].Source)
	}
}

func TestFileExtractor_Extract_MultiDoc(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "resources.yaml")
	if err := os.WriteFile(f, []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: cfg1
data: {}
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: cfg2
data: {}
`), 0644); err != nil {
		t.Fatal(err)
	}

	fe := NewFileExtractor()
	resCh, errCh := fe.Extract(context.Background(), Options{Paths: []string{f}})

	var resources []*types.ExtractedResource
	for r := range resCh {
		resources = append(resources, r)
	}
	for range errCh {
	}

	if len(resources) != 2 {
		t.Errorf("got %d resources; want 2", len(resources))
	}
}

func TestFileExtractor_Extract_SkipsEmptyDocs(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "sparse.yaml")
	if err := os.WriteFile(f, []byte(`---
---
apiVersion: v1
kind: Service
metadata:
  name: svc
---
---
`), 0644); err != nil {
		t.Fatal(err)
	}

	fe := NewFileExtractor()
	resCh, errCh := fe.Extract(context.Background(), Options{Paths: []string{f}})

	var resources []*types.ExtractedResource
	for r := range resCh {
		resources = append(resources, r)
	}
	for range errCh {
	}

	if len(resources) != 1 {
		t.Errorf("got %d resources; want 1 (skipping empty docs)", len(resources))
	}
}

func TestFileExtractor_Extract_SkipsCommentsOnly(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "comments.yaml")
	if err := os.WriteFile(f, []byte(`# Just a comment
# Another comment
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: real
data: {}
`), 0644); err != nil {
		t.Fatal(err)
	}

	fe := NewFileExtractor()
	resCh, errCh := fe.Extract(context.Background(), Options{Paths: []string{f}})

	var resources []*types.ExtractedResource
	for r := range resCh {
		resources = append(resources, r)
	}
	for range errCh {
	}

	if len(resources) != 1 {
		t.Errorf("got %d resources; want 1", len(resources))
	}
}

func TestFileExtractor_Extract_RecursiveDir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a\ndata: {}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.yaml"), []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: b\ndata: {}"), 0644); err != nil {
		t.Fatal(err)
	}

	fe := NewFileExtractor()
	resCh, errCh := fe.Extract(context.Background(), Options{Paths: []string{dir}, Recursive: true})

	var resources []*types.ExtractedResource
	for r := range resCh {
		resources = append(resources, r)
	}
	for range errCh {
	}

	if len(resources) != 2 {
		t.Errorf("recursive: got %d resources; want 2", len(resources))
	}
}

func TestFileExtractor_Extract_NonRecursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a\ndata: {}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.yaml"), []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: b\ndata: {}"), 0644); err != nil {
		t.Fatal(err)
	}

	fe := NewFileExtractor()
	resCh, errCh := fe.Extract(context.Background(), Options{Paths: []string{dir}, Recursive: false})

	var resources []*types.ExtractedResource
	for r := range resCh {
		resources = append(resources, r)
	}
	for range errCh {
	}

	if len(resources) != 1 {
		t.Errorf("non-recursive: got %d resources; want 1", len(resources))
	}
}

// ── Filters ──────────────────────────────────────────────────────────────────

func TestFileExtractor_Extract_IncludeKinds(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "mixed.yaml")
	if err := os.WriteFile(f, []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: cfg
data: {}
---
apiVersion: v1
kind: Service
metadata:
  name: svc
`), 0644); err != nil {
		t.Fatal(err)
	}

	fe := NewFileExtractor()
	resCh, errCh := fe.Extract(context.Background(), Options{
		Paths:        []string{f},
		IncludeKinds: []string{"Service"},
	})

	var resources []*types.ExtractedResource
	for r := range resCh {
		resources = append(resources, r)
	}
	for range errCh {
	}

	if len(resources) != 1 {
		t.Fatalf("IncludeKinds: got %d resources; want 1", len(resources))
	}
	if resources[0].Object.GetKind() != "Service" {
		t.Errorf("Kind = %q; want Service", resources[0].Object.GetKind())
	}
}

func TestFileExtractor_Extract_ExcludeKinds(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "mixed.yaml")
	if err := os.WriteFile(f, []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: cfg
data: {}
---
apiVersion: v1
kind: Service
metadata:
  name: svc
`), 0644); err != nil {
		t.Fatal(err)
	}

	fe := NewFileExtractor()
	resCh, errCh := fe.Extract(context.Background(), Options{
		Paths:        []string{f},
		ExcludeKinds: []string{"ConfigMap"},
	})

	var resources []*types.ExtractedResource
	for r := range resCh {
		resources = append(resources, r)
	}
	for range errCh {
	}

	if len(resources) != 1 {
		t.Fatalf("ExcludeKinds: got %d resources; want 1", len(resources))
	}
	if resources[0].Object.GetKind() != "Service" {
		t.Errorf("Kind = %q; want Service", resources[0].Object.GetKind())
	}
}

func TestFileExtractor_Extract_NamespaceFilter(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "ns.yaml")
	if err := os.WriteFile(f, []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: cfg1
  namespace: prod
data: {}
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: cfg2
  namespace: dev
data: {}
`), 0644); err != nil {
		t.Fatal(err)
	}

	fe := NewFileExtractor()
	resCh, errCh := fe.Extract(context.Background(), Options{
		Paths:     []string{f},
		Namespace: "prod",
	})

	var resources []*types.ExtractedResource
	for r := range resCh {
		resources = append(resources, r)
	}
	for range errCh {
	}

	if len(resources) != 1 {
		t.Fatalf("Namespace filter: got %d resources; want 1", len(resources))
	}
	if resources[0].Object.GetName() != "cfg1" {
		t.Errorf("Name = %q; want cfg1", resources[0].Object.GetName())
	}
}

func TestFileExtractor_Extract_CancelledContext(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "deploy.yaml")
	if err := os.WriteFile(f, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a\ndata: {}"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fe := NewFileExtractor()
	resCh, errCh := fe.Extract(ctx, Options{Paths: []string{f}})

	for range resCh {
	}
	for range errCh {
	}
	// With cancelled context, we expect 0 resources (or at most an error)
	// The exact behavior depends on goroutine scheduling, but we don't want a panic
}

// ── splitYAMLDocuments ───────────────────────────────────────────────────────

func TestSplitYAMLDocuments(t *testing.T) {
	input := []byte("doc1\n---\ndoc2\n---\ndoc3")
	docs := splitYAMLDocuments(input)
	if len(docs) != 3 {
		t.Errorf("got %d docs; want 3", len(docs))
	}
}

func TestSplitYAMLDocuments_Empty(t *testing.T) {
	docs := splitYAMLDocuments([]byte(""))
	if len(docs) != 0 {
		t.Errorf("got %d docs; want 0", len(docs))
	}
}

func TestSplitYAMLDocuments_SingleDoc(t *testing.T) {
	docs := splitYAMLDocuments([]byte("apiVersion: v1\nkind: Service"))
	if len(docs) != 1 {
		t.Errorf("got %d docs; want 1", len(docs))
	}
}

// ── isYAMLFile ───────────────────────────────────────────────────────────────

func TestIsYAMLFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"deploy.yaml", true},
		{"deploy.yml", true},
		{"deploy.YAML", true},
		{"deploy.YML", true},
		{"readme.txt", false},
		{"config.json", false},
		{"Makefile", false},
	}
	for _, tc := range tests {
		if got := isYAMLFile(tc.path); got != tc.want {
			t.Errorf("isYAMLFile(%q) = %v; want %v", tc.path, got, tc.want)
		}
	}
}

// ── isCommentOnly ────────────────────────────────────────────────────────────

func TestIsCommentOnly(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want bool
	}{
		{"comments", "# comment\n# another", true},
		{"empty lines", "\n  \n", true},
		{"mixed", "# comment\nkey: value", false},
		{"real yaml", "apiVersion: v1", false},
	}
	for _, tc := range tests {
		if got := isCommentOnly([]byte(tc.doc)); got != tc.want {
			t.Errorf("isCommentOnly(%q) = %v; want %v", tc.name, got, tc.want)
		}
	}
}

// ── ParseGVK ─────────────────────────────────────────────────────────────────

func TestParseGVK_CoreV1(t *testing.T) {
	data := []byte(`apiVersion: v1
kind: Service`)
	gvk, err := ParseGVK(data)
	if err != nil {
		t.Fatalf("ParseGVK() error: %v", err)
	}
	if gvk.Kind != "Service" || gvk.Version != "v1" || gvk.Group != "" {
		t.Errorf("GVK = %v; want /v1/Service", gvk)
	}
}

func TestParseGVK_AppsV1(t *testing.T) {
	data := []byte(`apiVersion: apps/v1
kind: Deployment`)
	gvk, err := ParseGVK(data)
	if err != nil {
		t.Fatalf("ParseGVK() error: %v", err)
	}
	if gvk.Group != "apps" || gvk.Version != "v1" || gvk.Kind != "Deployment" {
		t.Errorf("GVK = %v; want apps/v1/Deployment", gvk)
	}
}

func TestParseGVK_Invalid(t *testing.T) {
	_, err := ParseGVK([]byte("not: yaml: at: all: {{{"))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

// ── ClusterExtractor stub ────────────────────────────────────────────────────

func TestClusterExtractor_Source(t *testing.T) {
	ce := NewClusterExtractor()
	if ce.Source() != types.SourceCluster {
		t.Errorf("Source() = %q; want cluster", ce.Source())
	}
}

func TestClusterExtractor_Validate_NoKubeconfig(t *testing.T) {
	// Without a valid kubeconfig, Validate should return an error.
	ce := NewClusterExtractor()
	err := ce.Validate(context.Background(), Options{KubeConfig: "/nonexistent/kubeconfig"})
	if err == nil {
		t.Error("expected error when kubeconfig is missing")
	}
}

func TestClusterExtractor_Extract_NoKubeconfig(t *testing.T) {
	// Without a valid kubeconfig, Extract should produce an error.
	ce := NewClusterExtractor()
	resCh, errCh := ce.Extract(context.Background(), Options{KubeConfig: "/nonexistent/kubeconfig"})
	for range resCh {
	}
	var gotErr bool
	for e := range errCh {
		if e != nil {
			gotErr = true
		}
	}
	if !gotErr {
		t.Error("expected error from cluster extractor without kubeconfig")
	}
}

func TestClusterExtractor_ConfigValidation(t *testing.T) {
	ce := NewClusterExtractorWithConfig(ClusterExtractorConfig{
		Kubeconfig:     "~/.kube/config",
		Context:        "prod",
		Namespace:      "default",
		Selector:       "app=web",
		IncludeSecrets: false,
		SecretStrategy: "mask",
	})

	// Validate should fail because kubeconfig doesn't exist
	err := ce.Validate(context.Background(), Options{})
	if err == nil {
		t.Fatal("expected error for nonexistent kubeconfig")
	}

	// Extract should also fail for the same reason
	_, errCh := ce.Extract(context.Background(), Options{})
	var gotErr bool
	for e := range errCh {
		if e != nil {
			gotErr = true
		}
	}
	if !gotErr {
		t.Error("expected error from Extract with invalid kubeconfig")
	}
}

func TestClusterExtractor_InvalidSecretStrategy(t *testing.T) {
	ce := NewClusterExtractorWithConfig(ClusterExtractorConfig{
		SecretStrategy: "invalid-strategy",
	})
	err := ce.Validate(context.Background(), Options{})
	if err == nil {
		t.Error("expected error for invalid secret strategy")
	}
}

// ── GitOpsExtractor ─────────────────────────────────────────────────────────

func TestGitOpsExtractor_Source(t *testing.T) {
	ge := NewGitOpsExtractor()
	if ge.Source() != types.SourceGitOps {
		t.Errorf("Source() = %q; want gitops", ge.Source())
	}
}

func TestGitOpsExtractor_DefaultBranch(t *testing.T) {
	ge := NewGitOpsExtractor()
	if ge.Config().Branch != "main" {
		t.Errorf("default Branch = %q; want main", ge.Config().Branch)
	}
}

func TestGitOpsExtractor_WithConfig_DefaultBranch(t *testing.T) {
	ge := NewGitOpsExtractorWithConfig(GitOpsExtractorConfig{
		RepoURL: "https://github.com/org/repo.git",
	})
	if ge.Config().Branch != "main" {
		t.Errorf("Branch = %q; want main (default)", ge.Config().Branch)
	}
}

func TestGitOpsExtractor_Validate_NoURL(t *testing.T) {
	ge := NewGitOpsExtractor()
	err := ge.Validate(context.Background(), Options{})
	if err == nil {
		t.Fatal("expected error for empty repo URL")
	}
	if !strings.Contains(err.Error(), "repo URL is required") {
		t.Errorf("error = %q; want 'repo URL is required'", err.Error())
	}
}

func TestGitOpsExtractor_Validate_WithURL(t *testing.T) {
	ge := NewGitOpsExtractorWithConfig(GitOpsExtractorConfig{
		RepoURL: "https://github.com/org/repo.git",
	})
	err := ge.Validate(context.Background(), Options{})
	if err == nil {
		t.Fatal("expected 'not yet implemented' error")
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("error = %q; want 'not yet implemented'", err.Error())
	}
}

func TestGitOpsExtractor_Extract_NotImplemented(t *testing.T) {
	ge := NewGitOpsExtractor()
	_, errCh := ge.Extract(context.Background(), Options{})
	var gotErr bool
	for e := range errCh {
		if e != nil {
			gotErr = true
		}
	}
	if !gotErr {
		t.Error("expected error from gitops extractor")
	}
}

func TestGitOpsExtractor_Validate_NegativeDepth(t *testing.T) {
	ge := NewGitOpsExtractorWithConfig(GitOpsExtractorConfig{
		RepoURL: "https://github.com/org/repo.git",
		Depth:   -1,
	})
	err := ge.Validate(context.Background(), Options{})
	if err == nil {
		t.Fatal("expected error for negative depth")
	}
	if !strings.Contains(err.Error(), "non-negative") {
		t.Errorf("error = %q; want 'non-negative'", err.Error())
	}
}

// ── GitAuth ─────────────────────────────────────────────────────────────────

func TestGitAuth_Validate_Nil(t *testing.T) {
	var a *GitAuth
	if err := a.Validate(); err != nil {
		t.Errorf("nil auth should be valid: %v", err)
	}
}

func TestGitAuth_Validate_EmptyType(t *testing.T) {
	a := &GitAuth{}
	if err := a.Validate(); err != nil {
		t.Errorf("empty type (public repo) should be valid: %v", err)
	}
}

func TestGitAuth_Validate_TokenMissing(t *testing.T) {
	a := &GitAuth{Type: GitAuthTypeToken}
	if err := a.Validate(); err == nil {
		t.Error("expected error for token auth without token")
	}
}

func TestGitAuth_Validate_TokenOK(t *testing.T) {
	a := &GitAuth{
		Type:  GitAuthTypeToken,
		Token: &TokenAuth{Token: "ghp_test123"},
	}
	if err := a.Validate(); err != nil {
		t.Errorf("valid token auth failed: %v", err)
	}
}

func TestGitAuth_Validate_SSHKeyMissing(t *testing.T) {
	a := &GitAuth{Type: GitAuthTypeSSHKey}
	if err := a.Validate(); err == nil {
		t.Error("expected error for ssh-key auth without key")
	}
}

func TestGitAuth_Validate_SSHKeyNotFound(t *testing.T) {
	a := &GitAuth{
		Type:   GitAuthTypeSSHKey,
		SSHKey: &SSHKeyAuth{KeyPath: "/nonexistent/key"},
	}
	if err := a.Validate(); err == nil {
		t.Error("expected error for missing SSH key file")
	}
}

func TestGitAuth_Validate_CredHelperMissing(t *testing.T) {
	a := &GitAuth{Type: GitAuthTypeCredHelper}
	if err := a.Validate(); err == nil {
		t.Error("expected error for cred-helper auth without helper")
	}
}

func TestGitAuth_Validate_CredHelperOK(t *testing.T) {
	a := &GitAuth{
		Type:       GitAuthTypeCredHelper,
		CredHelper: &CredentialHelper{Helper: "store"},
	}
	if err := a.Validate(); err != nil {
		t.Errorf("valid cred-helper auth failed: %v", err)
	}
}

func TestGitAuth_Validate_UnknownType(t *testing.T) {
	a := &GitAuth{Type: "magic"}
	if err := a.Validate(); err == nil {
		t.Error("expected error for unknown auth type")
	}
}

// ── GitOpsExtractorConfig.Validate ──────────────────────────────────────────

func TestGitOpsExtractorConfig_Validate_Empty(t *testing.T) {
	c := &GitOpsExtractorConfig{}
	if err := c.Validate(); err == nil {
		t.Error("expected error for empty config")
	}
}

func TestGitOpsExtractorConfig_Validate_OK(t *testing.T) {
	c := &GitOpsExtractorConfig{RepoURL: "https://github.com/org/repo.git"}
	if err := c.Validate(); err != nil {
		t.Errorf("valid config failed: %v", err)
	}
}

func TestGitOpsExtractorConfig_Validate_BadAuth(t *testing.T) {
	c := &GitOpsExtractorConfig{
		RepoURL: "https://github.com/org/repo.git",
		Auth:    &GitAuth{Type: GitAuthTypeToken}, // missing token
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error for bad auth in config")
	}
}

// ── DiscoverYAMLFiles ───────────────────────────────────────────────────────

func TestDiscoverYAMLFiles_Basic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "b.yml"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("x"), 0644)

	files, err := DiscoverYAMLFiles(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("got %d files; want 2", len(files))
	}
}

func TestDiscoverYAMLFiles_ExcludesDirs(t *testing.T) {
	dir := t.TempDir()
	vendor := filepath.Join(dir, "vendor")
	os.MkdirAll(vendor, 0755)
	os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(vendor, "b.yaml"), []byte("x"), 0644)

	files, err := DiscoverYAMLFiles(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Errorf("got %d files; want 1 (vendor excluded)", len(files))
	}
}

func TestDiscoverYAMLFiles_CustomExcludes(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "mydir")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(sub, "b.yaml"), []byte("x"), 0644)

	files, err := DiscoverYAMLFiles(dir, []string{"mydir"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Errorf("got %d files; want 1 (mydir excluded)", len(files))
	}
}

func TestDiscoverYAMLFiles_NonexistentDir(t *testing.T) {
	_, err := DiscoverYAMLFiles("/nonexistent", nil)
	if err == nil {
		t.Error("expected error for nonexistent dir")
	}
}

func TestDiscoverYAMLFiles_FileNotDir(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	os.WriteFile(f, []byte("x"), 0644)
	_, err := DiscoverYAMLFiles(f, nil)
	if err == nil {
		t.Error("expected error for file (not dir)")
	}
}

// ── DefaultExcludeDirs ──────────────────────────────────────────────────────

func TestDefaultExcludeDirs(t *testing.T) {
	dirs := DefaultExcludeDirs()
	if len(dirs) == 0 {
		t.Error("expected non-empty default exclude dirs")
	}
	found := false
	for _, d := range dirs {
		if d == ".git" {
			found = true
		}
	}
	if !found {
		t.Error("expected .git in default exclude dirs")
	}
}

// ── DetectKustomization ─────────────────────────────────────────────────────

func TestDetectKustomization_Found(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "kustomization.yaml"), []byte("resources: []"), 0644)
	if !DetectKustomization(dir) {
		t.Error("should detect kustomization.yaml")
	}
}

func TestDetectKustomization_FoundYml(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "kustomization.yml"), []byte("resources: []"), 0644)
	if !DetectKustomization(dir) {
		t.Error("should detect kustomization.yml")
	}
}

func TestDetectKustomization_NotFound(t *testing.T) {
	dir := t.TempDir()
	if DetectKustomization(dir) {
		t.Error("should not detect kustomization in empty dir")
	}
}

// ── DetectGitOpsManifests ───────────────────────────────────────────────────

func TestDetectGitOpsManifests_ArgoCD(t *testing.T) {
	dir := t.TempDir()
	argoApp := `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: my-app
  namespace: argocd
`
	os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(argoApp), 0644)

	manifests, err := DetectGitOpsManifests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) != 1 {
		t.Fatalf("got %d manifests; want 1", len(manifests))
	}
	if manifests[0].Type != GitOpsManifestArgoApplication {
		t.Errorf("Type = %q; want argocd-application", manifests[0].Type)
	}
	if manifests[0].Name != "my-app" {
		t.Errorf("Name = %q; want my-app", manifests[0].Name)
	}
}

func TestDetectGitOpsManifests_FluxGitRepo(t *testing.T) {
	dir := t.TempDir()
	fluxRepo := `apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: my-repo
  namespace: flux-system
`
	os.WriteFile(filepath.Join(dir, "source.yaml"), []byte(fluxRepo), 0644)

	manifests, err := DetectGitOpsManifests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) != 1 {
		t.Fatalf("got %d manifests; want 1", len(manifests))
	}
	if manifests[0].Type != GitOpsManifestFluxGitRepository {
		t.Errorf("Type = %q; want flux-gitrepository", manifests[0].Type)
	}
}

func TestDetectGitOpsManifests_FluxKustomization(t *testing.T) {
	dir := t.TempDir()
	fluxKs := `apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: my-ks
  namespace: flux-system
`
	os.WriteFile(filepath.Join(dir, "ks.yaml"), []byte(fluxKs), 0644)

	manifests, err := DetectGitOpsManifests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) != 1 {
		t.Fatalf("got %d manifests; want 1", len(manifests))
	}
	if manifests[0].Type != GitOpsManifestFluxKustomization {
		t.Errorf("Type = %q; want flux-kustomization", manifests[0].Type)
	}
}

func TestDetectGitOpsManifests_NonGitOps(t *testing.T) {
	dir := t.TempDir()
	plainYAML := `apiVersion: v1
kind: ConfigMap
metadata:
  name: plain
`
	os.WriteFile(filepath.Join(dir, "cm.yaml"), []byte(plainYAML), 0644)

	manifests, err := DetectGitOpsManifests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) != 0 {
		t.Errorf("got %d manifests; want 0 for non-gitops", len(manifests))
	}
}

func TestDetectGitOpsManifests_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	manifests, err := DetectGitOpsManifests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) != 0 {
		t.Errorf("got %d manifests; want 0 for empty dir", len(manifests))
	}
}

func TestDetectGitOpsManifests_NonexistentDir(t *testing.T) {
	_, err := DetectGitOpsManifests("/nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent dir")
	}
}

func TestDetectGitOpsManifests_MultiDoc(t *testing.T) {
	dir := t.TempDir()
	multiDoc := `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: app1
---
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: repo1
`
	os.WriteFile(filepath.Join(dir, "multi.yaml"), []byte(multiDoc), 0644)

	manifests, err := DetectGitOpsManifests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) != 2 {
		t.Errorf("got %d manifests; want 2", len(manifests))
	}
}

// ── matchesKindFilters ───────────────────────────────────────────────────────

func TestMatchesKindFilters_NoFilters(t *testing.T) {
	fe := NewFileExtractor()
	if !fe.matchesKindFilters("Deployment", Options{}) {
		t.Error("no filters should match all")
	}
}

func TestMatchesKindFilters_IncludeMatch(t *testing.T) {
	fe := NewFileExtractor()
	if !fe.matchesKindFilters("Service", Options{IncludeKinds: []string{"service"}}) {
		t.Error("case-insensitive include should match")
	}
}

func TestMatchesKindFilters_IncludeNoMatch(t *testing.T) {
	fe := NewFileExtractor()
	if fe.matchesKindFilters("ConfigMap", Options{IncludeKinds: []string{"Service"}}) {
		t.Error("should not match non-included kind")
	}
}

func TestMatchesKindFilters_ExcludeMatch(t *testing.T) {
	fe := NewFileExtractor()
	if fe.matchesKindFilters("Secret", Options{ExcludeKinds: []string{"secret"}}) {
		t.Error("case-insensitive exclude should filter out")
	}
}

// ── matchesNamespaceFilters ──────────────────────────────────────────────────

func TestMatchesNamespaceFilters_NoFilters(t *testing.T) {
	fe := NewFileExtractor()
	if !fe.matchesNamespaceFilters("anything", Options{}) {
		t.Error("no filters should match all")
	}
}

func TestMatchesNamespaceFilters_SingleMatch(t *testing.T) {
	fe := NewFileExtractor()
	if !fe.matchesNamespaceFilters("prod", Options{Namespace: "prod"}) {
		t.Error("should match single namespace")
	}
}

func TestMatchesNamespaceFilters_SingleNoMatch(t *testing.T) {
	fe := NewFileExtractor()
	if fe.matchesNamespaceFilters("dev", Options{Namespace: "prod"}) {
		t.Error("should not match different namespace")
	}
}

func TestMatchesNamespaceFilters_ClusterScoped(t *testing.T) {
	fe := NewFileExtractor()
	// Cluster-scoped resources (empty namespace) should always be included
	if !fe.matchesNamespaceFilters("", Options{Namespace: "prod"}) {
		t.Error("cluster-scoped resources should be included")
	}
}

func TestMatchesNamespaceFilters_MultipleNamespaces(t *testing.T) {
	fe := NewFileExtractor()
	opts := Options{Namespaces: []string{"prod", "staging"}}
	if !fe.matchesNamespaceFilters("prod", opts) {
		t.Error("should match prod in multi-ns filter")
	}
	if !fe.matchesNamespaceFilters("staging", opts) {
		t.Error("should match staging in multi-ns filter")
	}
	if fe.matchesNamespaceFilters("dev", opts) {
		t.Error("should not match dev in multi-ns filter")
	}
}

func TestMatchesNamespaceFilters_ClusterScopedWithMultiNs(t *testing.T) {
	fe := NewFileExtractor()
	// Cluster-scoped resources should be included even with multi-namespace filter
	if !fe.matchesNamespaceFilters("", Options{Namespaces: []string{"prod", "staging"}}) {
		t.Error("cluster-scoped resources should be included with multi-ns filter")
	}
}

// ── Edge cases for coverage ──────────────────────────────────────────────────

func TestFileExtractor_Extract_NonexistentPath(t *testing.T) {
	fe := NewFileExtractor()
	resCh, errCh := fe.Extract(context.Background(), Options{
		Paths: []string{"/nonexistent/path/file.yaml"},
	})

	for range resCh {
	}
	var gotErr bool
	for e := range errCh {
		if e != nil {
			gotErr = true
		}
	}
	if !gotErr {
		t.Error("expected error for nonexistent path")
	}
}

func TestFileExtractor_Extract_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(f, []byte("key: [invalid yaml {{{\n"), 0644); err != nil {
		t.Fatal(err)
	}

	fe := NewFileExtractor()
	resCh, errCh := fe.Extract(context.Background(), Options{Paths: []string{f}})

	var resources []*types.ExtractedResource
	for r := range resCh {
		resources = append(resources, r)
	}
	var errors []error
	for e := range errCh {
		errors = append(errors, e)
	}

	if len(errors) == 0 {
		t.Error("expected parse error for invalid YAML")
	}
	if len(resources) != 0 {
		t.Errorf("got %d resources; want 0 from invalid YAML", len(resources))
	}
}

func TestFileExtractor_Extract_NoAPIVersionOrKind(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "noapi.yaml")
	if err := os.WriteFile(f, []byte("key: value\nfoo: bar\n"), 0644); err != nil {
		t.Fatal(err)
	}

	fe := NewFileExtractor()
	resCh, errCh := fe.Extract(context.Background(), Options{Paths: []string{f}})

	var resources []*types.ExtractedResource
	for r := range resCh {
		resources = append(resources, r)
	}
	for range errCh {
	}

	if len(resources) != 0 {
		t.Errorf("got %d resources; want 0 (no apiVersion/kind)", len(resources))
	}
}

func TestFileExtractor_Extract_NonYAMLFilesSkipped(t *testing.T) {
	dir := t.TempDir()
	// Create a non-YAML file in the directory
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	// Create a valid YAML file
	if err := os.WriteFile(filepath.Join(dir, "deploy.yaml"), []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: x\ndata: {}"), 0644); err != nil {
		t.Fatal(err)
	}

	fe := NewFileExtractor()
	resCh, errCh := fe.Extract(context.Background(), Options{
		Paths:     []string{dir},
		Recursive: true,
	})

	var resources []*types.ExtractedResource
	for r := range resCh {
		resources = append(resources, r)
	}
	for range errCh {
	}

	if len(resources) != 1 {
		t.Errorf("got %d resources; want 1 (non-YAML should be skipped)", len(resources))
	}
}

func TestFileExtractor_Extract_EmptyObject(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "empty.yaml")
	// YAML that parses to empty object
	if err := os.WriteFile(f, []byte("---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	fe := NewFileExtractor()
	resCh, errCh := fe.Extract(context.Background(), Options{Paths: []string{f}})

	var resources []*types.ExtractedResource
	for r := range resCh {
		resources = append(resources, r)
	}
	for range errCh {
	}

	if len(resources) != 0 {
		t.Errorf("got %d resources; want 0 for empty object", len(resources))
	}
}

func TestFileExtractor_Extract_MultiplePaths(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir1, "a.yaml"), []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a\ndata: {}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "b.yaml"), []byte("apiVersion: v1\nkind: Secret\nmetadata:\n  name: b\ntype: Opaque"), 0644); err != nil {
		t.Fatal(err)
	}

	fe := NewFileExtractor()
	resCh, errCh := fe.Extract(context.Background(), Options{
		Paths: []string{
			filepath.Join(dir1, "a.yaml"),
			filepath.Join(dir2, "b.yaml"),
		},
	})

	var resources []*types.ExtractedResource
	for r := range resCh {
		resources = append(resources, r)
	}
	for range errCh {
	}

	if len(resources) != 2 {
		t.Errorf("got %d resources; want 2 from multiple paths", len(resources))
	}
}

func TestParseGVK_EmptyAPIVersion(t *testing.T) {
	data := []byte("kind: Service")
	gvk, err := ParseGVK(data)
	if err != nil {
		t.Fatalf("ParseGVK() error: %v", err)
	}
	// Empty apiVersion should still parse (group and version will be empty)
	if gvk.Kind != "Service" {
		t.Errorf("Kind = %q; want Service", gvk.Kind)
	}
}

func TestParseGVK_CRD(t *testing.T) {
	data := []byte("apiVersion: deckhouse.io/v1alpha1\nkind: ModuleConfig")
	gvk, err := ParseGVK(data)
	if err != nil {
		t.Fatalf("ParseGVK() error: %v", err)
	}
	if gvk.Group != "deckhouse.io" || gvk.Version != "v1alpha1" || gvk.Kind != "ModuleConfig" {
		t.Errorf("GVK = %v; want deckhouse.io/v1alpha1/ModuleConfig", gvk)
	}
}

// ── Merger / ResourceDeduplicator ───────────────────────────────────────────

func makeResource(kind, name, ns string, source types.Source) *types.ExtractedResource {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("v1")
	obj.SetKind(kind)
	obj.SetName(name)
	obj.SetNamespace(ns)

	return &types.ExtractedResource{
		Object:     obj,
		Source:     source,
		SourcePath: "test",
		GVK:        schema.GroupVersionKind{Version: "v1", Kind: kind},
	}
}

func TestResourceDeduplicator_NoDuplicates(t *testing.T) {
	d := NewResourceDeduplicator()
	resources := []*types.ExtractedResource{
		makeResource("ConfigMap", "cfg1", "default", types.SourceFile),
		makeResource("ConfigMap", "cfg2", "default", types.SourceFile),
		makeResource("Service", "svc1", "default", types.SourceFile),
	}

	result, err := d.Deduplicate(resources)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 3 {
		t.Errorf("got %d resources; want 3", len(result))
	}
	if len(d.Conflicts) != 0 {
		t.Errorf("got %d conflicts; want 0", len(d.Conflicts))
	}
}

func TestResourceDeduplicator_DuplicateWarn(t *testing.T) {
	d := NewResourceDeduplicator()
	d.Strategy = ConflictStrategyWarn

	resources := []*types.ExtractedResource{
		makeResource("ConfigMap", "cfg1", "default", types.SourceFile),
		makeResource("ConfigMap", "cfg1", "default", types.SourceCluster),
	}

	result, err := d.Deduplicate(resources)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d resources; want 1", len(result))
	}
	// Cluster has higher priority (0) than file (1), so cluster wins
	if result[0].Source != types.SourceCluster {
		t.Errorf("Source = %q; want cluster (higher priority)", result[0].Source)
	}
	if len(d.Conflicts) != 1 {
		t.Errorf("got %d conflicts; want 1", len(d.Conflicts))
	}
}

func TestResourceDeduplicator_DuplicateError(t *testing.T) {
	d := NewResourceDeduplicator()
	d.Strategy = ConflictStrategyError

	resources := []*types.ExtractedResource{
		makeResource("ConfigMap", "cfg1", "default", types.SourceFile),
		makeResource("ConfigMap", "cfg1", "default", types.SourceCluster),
	}

	_, err := d.Deduplicate(resources)
	if err == nil {
		t.Fatal("expected error for duplicate with error strategy")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error = %q; want 'duplicate'", err.Error())
	}
}

func TestResourceDeduplicator_DuplicateMerge(t *testing.T) {
	d := NewResourceDeduplicator()
	d.Strategy = ConflictStrategyMerge

	resources := []*types.ExtractedResource{
		makeResource("ConfigMap", "cfg1", "default", types.SourceFile),
		makeResource("ConfigMap", "cfg1", "default", types.SourceCluster),
	}

	result, err := d.Deduplicate(resources)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d resources; want 1", len(result))
	}
	// Cluster has higher priority, so it should be kept
	if result[0].Source != types.SourceCluster {
		t.Errorf("Source = %q; want cluster", result[0].Source)
	}
}

func TestResourceDeduplicator_DifferentNamespaces(t *testing.T) {
	d := NewResourceDeduplicator()

	resources := []*types.ExtractedResource{
		makeResource("ConfigMap", "cfg1", "default", types.SourceFile),
		makeResource("ConfigMap", "cfg1", "prod", types.SourceFile),
	}

	result, err := d.Deduplicate(resources)
	if err != nil {
		t.Fatal(err)
	}
	// Different namespaces = different resources, no dedup
	if len(result) != 2 {
		t.Errorf("got %d resources; want 2 (different namespaces)", len(result))
	}
}

func TestResourceDeduplicator_NilResources(t *testing.T) {
	d := NewResourceDeduplicator()

	resources := []*types.ExtractedResource{
		nil,
		makeResource("ConfigMap", "cfg1", "default", types.SourceFile),
		nil,
	}

	result, err := d.Deduplicate(resources)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Errorf("got %d resources; want 1 (nil skipped)", len(result))
	}
}

func TestResourceDeduplicator_EmptyInput(t *testing.T) {
	d := NewResourceDeduplicator()

	result, err := d.Deduplicate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Errorf("got %d resources; want 0", len(result))
	}
}

func TestResourceDeduplicator_ThreeSourceConflict(t *testing.T) {
	d := NewResourceDeduplicator()
	d.Strategy = ConflictStrategyWarn

	resources := []*types.ExtractedResource{
		makeResource("ConfigMap", "cfg1", "default", types.SourceGitOps),
		makeResource("ConfigMap", "cfg1", "default", types.SourceFile),
		makeResource("ConfigMap", "cfg1", "default", types.SourceCluster),
	}

	result, err := d.Deduplicate(resources)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d resources; want 1", len(result))
	}
	// Cluster (priority 0) wins over file (1) and gitops (2)
	if result[0].Source != types.SourceCluster {
		t.Errorf("Source = %q; want cluster", result[0].Source)
	}
	if len(d.Conflicts) != 2 {
		t.Errorf("got %d conflicts; want 2", len(d.Conflicts))
	}
}

// ── SourcePriority ──────────────────────────────────────────────────────────

func TestDefaultSourcePriority(t *testing.T) {
	sp := DefaultSourcePriority()
	if sp.Priority(types.SourceCluster) >= sp.Priority(types.SourceFile) {
		t.Error("cluster should have higher priority (lower number) than file")
	}
	if sp.Priority(types.SourceFile) >= sp.Priority(types.SourceGitOps) {
		t.Error("file should have higher priority than gitops")
	}
}

func TestSourcePriority_Higher(t *testing.T) {
	sp := DefaultSourcePriority()
	if !sp.Higher(types.SourceCluster, types.SourceFile) {
		t.Error("cluster should be higher than file")
	}
	if sp.Higher(types.SourceGitOps, types.SourceCluster) {
		t.Error("gitops should not be higher than cluster")
	}
}

func TestSourcePriority_UnknownSource(t *testing.T) {
	sp := DefaultSourcePriority()
	if sp.Priority("unknown") != 999 {
		t.Errorf("unknown source priority = %d; want 999", sp.Priority("unknown"))
	}
}

func TestNewSourcePriority_Custom(t *testing.T) {
	sp := NewSourcePriority([]types.Source{types.SourceGitOps, types.SourceFile, types.SourceCluster})
	// gitops is first = highest priority
	if !sp.Higher(types.SourceGitOps, types.SourceCluster) {
		t.Error("custom: gitops should be higher than cluster")
	}
}

// ── ConflictStrategy validation ─────────────────────────────────────────────

func TestValidConflictStrategies(t *testing.T) {
	strategies := ValidConflictStrategies()
	if len(strategies) != 3 {
		t.Errorf("got %d strategies; want 3", len(strategies))
	}
}

func TestIsValidConflictStrategy(t *testing.T) {
	if !IsValidConflictStrategy("error") {
		t.Error("'error' should be valid")
	}
	if !IsValidConflictStrategy("warn") {
		t.Error("'warn' should be valid")
	}
	if !IsValidConflictStrategy("merge") {
		t.Error("'merge' should be valid")
	}
	if IsValidConflictStrategy("invalid") {
		t.Error("'invalid' should not be valid")
	}
}

// ── DefaultRegistry with GitOps ─────────────────────────────────────────────

func TestDefaultRegistry_IncludesGitOps(t *testing.T) {
	r := DefaultRegistry()
	_, ok := r.Get(types.SourceGitOps)
	if !ok {
		t.Error("DefaultRegistry should include gitops extractor")
	}
}
