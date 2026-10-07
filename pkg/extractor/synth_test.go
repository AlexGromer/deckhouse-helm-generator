package extractor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/synth/registry/registrytest"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// drain collects the resources of an extraction and its first error.
func drain(res <-chan *types.ExtractedResource, errs <-chan error) ([]*types.ExtractedResource, error) {
	var out []*types.ExtractedResource
	var first error
	for res != nil || errs != nil {
		select {
		case r, ok := <-res:
			if !ok {
				res = nil
				continue
			}
			out = append(out, r)
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if first == nil {
				first = err
			}
		}
	}
	return out, first
}

func kinds(rs []*types.ExtractedResource) string {
	var k []string
	for _, r := range rs {
		k = append(k, r.Object.GetKind()+"/"+r.Object.GetName())
	}
	return strings.Join(k, " ")
}

func TestDefaultRegistryHasSynthesizingSources(t *testing.T) {
	r := DefaultRegistry()
	for _, s := range []types.Source{types.SourceImage, types.SourceCompose, types.SourceCode} {
		e, ok := r.Get(s)
		if !ok {
			t.Fatalf("source %s not registered", s)
		}
		if _, ok := e.(Reporter); !ok {
			t.Errorf("source %s does not report notes", s)
		}
	}
}

func TestImageExtractor(t *testing.T) {
	fake := registrytest.New()
	defer fake.Close()
	fake.Push("team/api", "1", map[string]interface{}{"config": map[string]interface{}{
		"ExposedPorts": map[string]interface{}{"8080/tcp": map[string]interface{}{}},
	}})
	t.Setenv("DOCKER_CONFIG", t.TempDir())

	e := NewImageExtractor()
	if e.Source() != types.SourceImage {
		t.Error("source")
	}
	if err := e.Validate(context.Background(), Options{}); err == nil {
		t.Error("an image is required")
	}
	if err := e.Validate(context.Background(), Options{Images: []string{"Bad Ref"}}); err == nil {
		t.Error("invalid reference accepted")
	}
	opts := Options{Images: []string{fake.Host() + "/team/api:1"}, InsecureRegistry: true}
	if err := e.Validate(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	rs, err := drain(e.Extract(context.Background(), opts))
	if err != nil {
		t.Fatal(err)
	}
	if kinds(rs) != "Deployment/api Service/api" || rs[0].Source != types.SourceImage {
		t.Errorf("resources = %s", kinds(rs))
	}
	if len(e.Inputs()) != 1 || !strings.Contains(e.Inputs()[0], "linux/amd64") || len(e.Notes()) == 0 {
		t.Errorf("inputs = %v notes = %v", e.Inputs(), e.Notes())
	}

	missing := Options{Images: []string{fake.Host() + "/team/none:1"}, InsecureRegistry: true}
	if _, err := drain(NewImageExtractor().Extract(context.Background(), missing)); err == nil {
		t.Error("a missing image must fail")
	}
	filtered := opts
	filtered.ExcludeKinds = []string{"Service"}
	if rs, _ := drain(NewImageExtractor().Extract(context.Background(), filtered)); kinds(rs) != "Deployment/api" {
		t.Errorf("kind filter: %s", kinds(rs))
	}
}

func TestComposeAndSourceExtractors(t *testing.T) {
	dir := t.TempDir()
	compose := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(compose, []byte("services:\n  web:\n    image: web:${TAG}\n    ports: [\"8080\"]\n  web_:\n    image: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewComposeExtractor()
	c.env = func(k string) (string, bool) { return "2", k == "TAG" }
	if c.Source() != types.SourceCompose || c.Validate(context.Background(), Options{}) == nil ||
		c.Validate(context.Background(), Options{Paths: []string{dir}}) == nil {
		t.Error("compose validation")
	}
	// "web" and "web_" both become "web": the extraction must refuse.
	if _, err := drain(c.Extract(context.Background(), Options{Paths: []string{compose}})); err == nil || !strings.Contains(err.Error(), "rename one") {
		t.Errorf("name clash: %v", err)
	}
	if err := os.WriteFile(compose, []byte("services:\n  web:\n    image: web:${TAG}\n    ports: [\"8080\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c = NewComposeExtractor()
	c.env = func(k string) (string, bool) { return "2", k == "TAG" }
	rs, err := drain(c.Extract(context.Background(), Options{Paths: []string{compose}}))
	if err != nil || kinds(rs) != "Deployment/web Service/web" {
		t.Errorf("compose: %s %v", kinds(rs), err)
	}
	if _, err := drain(NewComposeExtractor().Extract(context.Background(), Options{Paths: []string{filepath.Join(dir, "none.yml")}})); err == nil {
		t.Error("missing compose file must fail")
	}

	project := filepath.Join(dir, "svc")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "Dockerfile"), []byte("FROM x\nEXPOSE 9000\nUSER 1000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewSourceExtractor()
	if s.Source() != types.SourceCode || s.Validate(context.Background(), Options{}) == nil ||
		s.Validate(context.Background(), Options{Paths: []string{compose}}) == nil ||
		s.Validate(context.Background(), Options{Paths: []string{project}, Images: []string{"a", "b"}}) == nil {
		t.Error("source validation")
	}
	opts := Options{Paths: []string{project}, Images: []string{"svc:1"}}
	if err := s.Validate(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	rs, err = drain(s.Extract(context.Background(), opts))
	if err != nil || kinds(rs) != "Deployment/svc Service/svc" {
		t.Errorf("source: %s %v", kinds(rs), err)
	}
	if _, err := drain(NewSourceExtractor().Extract(context.Background(), Options{Paths: []string{dir}})); err == nil {
		t.Error("a directory without Dockerfile or Spring Boot must fail")
	}
}
