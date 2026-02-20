package extractor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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

func TestClusterExtractor_Validate_NotImplemented(t *testing.T) {
	ce := NewClusterExtractor()
	err := ce.Validate(context.Background(), Options{})
	if err == nil {
		t.Error("expected 'not yet implemented' error")
	}
}

func TestClusterExtractor_Extract_NotImplemented(t *testing.T) {
	ce := NewClusterExtractor()
	_, errCh := ce.Extract(context.Background(), Options{})
	var gotErr bool
	for e := range errCh {
		if e != nil {
			gotErr = true
		}
	}
	if !gotErr {
		t.Error("expected error from cluster extractor")
	}
}

// ── GitOpsExtractor stub ─────────────────────────────────────────────────────

func TestGitOpsExtractor_Source(t *testing.T) {
	ge := NewGitOpsExtractor()
	if ge.Source() != types.SourceGitOps {
		t.Errorf("Source() = %q; want gitops", ge.Source())
	}
}

func TestGitOpsExtractor_Validate_NotImplemented(t *testing.T) {
	ge := NewGitOpsExtractor()
	err := ge.Validate(context.Background(), Options{})
	if err == nil {
		t.Error("expected 'not yet implemented' error")
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
