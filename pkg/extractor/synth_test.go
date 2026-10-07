package extractor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/synth/registry/registrytest"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

// TestImageExtractorCredentialHelpers runs docker credential helpers named
// by a docker config.json against a registry requiring a token.
func TestImageExtractorCredentialHelpers(t *testing.T) {
	private := registrytest.New()
	defer private.Close()
	private.Token, private.User, private.Password = "tok", "robot", "pw"
	private.Push("team/api", "1", map[string]interface{}{})
	public := registrytest.New() // tokens for anyone
	defer public.Close()
	public.Token = "tok"
	public.Push("team/api", "1", map[string]interface{}{})

	bin := t.TempDir()
	for name, body := range map[string]string{
		"robot": `cat >/dev/null; echo '{"Username":"robot","Secret":"pw"}'`,
		"empty": `echo "credentials not found in native keychain"; exit 1`,
	} {
		if err := os.WriteFile(filepath.Join(bin, "docker-credential-"+name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil { //nolint:gosec // an executable test helper
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	dir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dir)
	extract := func(config, host string) (*ImageExtractor, error) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
		e := NewImageExtractor()
		_, err := drain(e.Extract(context.Background(), Options{Images: []string{host + "/team/api:1"}, InsecureRegistry: true}))
		return e, err
	}
	hasNote := func(e *ImageExtractor, s string) bool {
		for _, n := range e.Notes() {
			if strings.Contains(n, s) {
				return true
			}
		}
		return false
	}

	// A per-registry helper wins over credsStore.
	e, err := extract(`{"credHelpers":{"`+private.Host()+`":"robot"},"credsStore":"absent"}`, private.Host())
	if err != nil || hasNote(e, "no registry credentials") {
		t.Errorf("credHelpers: %v, notes %v", err, e.Notes())
	}
	// A missing helper leaves the registry without credentials; the error says why.
	if _, err := extract(`{"credsStore":"absent"}`, private.Host()); err == nil || !strings.Contains(err.Error(), "docker-credential-absent: not found in PATH") {
		t.Errorf("missing helper: %v", err)
	}
	// Anonymous access succeeds; the report says the helper had nothing.
	e, err = extract(`{"credsStore":"empty"}`, public.Host())
	if err != nil || !hasNote(e, "no registry credentials for "+public.Host()+": docker-credential-empty: no credentials stored") {
		t.Errorf("anonymous: %v, notes %v", err, e.Notes())
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

func TestSourceExtractorImageConfigAndProfiles(t *testing.T) {
	fake := registrytest.New()
	defer fake.Close()
	fake.Push("team/orders", "2", map[string]interface{}{"config": map[string]interface{}{
		"User":         "10001",
		"ExposedPorts": map[string]interface{}{"8080/tcp": map[string]interface{}{}},
	}})
	t.Setenv("DOCKER_CONFIG", t.TempDir())

	project := t.TempDir()
	for name, content := range map[string]string{
		"pom.xml": `<project><artifactId>orders</artifactId><version>2</version><dependencies>` +
			`<dependency><groupId>org.springframework.boot</groupId><artifactId>spring-boot-starter-actuator</artifactId></dependency></dependencies></project>`,
		"src/main/resources/application.properties":      "spring.kafka.bootstrap-servers=kafka:9092\n",
		"src/main/resources/application-prod.properties": "spring.kafka.bootstrap-servers=kafka-prod:9092\n",
	} {
		path := filepath.Join(project, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	s := NewSourceExtractor()
	if err := s.Validate(context.Background(), Options{Paths: []string{project}, ImageConfig: true}); err == nil || !strings.Contains(err.Error(), "--image") {
		t.Errorf("--image-config without --image: %v", err)
	}
	opts := Options{Paths: []string{project}, Images: []string{fake.Host() + "/team/orders:2"}, ImageConfig: true, InsecureRegistry: true}
	rs, err := drain(s.Extract(context.Background(), opts))
	if err != nil {
		t.Fatal(err)
	}
	var dep *types.ExtractedResource
	for _, r := range rs {
		if r.Object.GetKind() == "Deployment" {
			dep = r
		}
	}
	if uid, _, _ := unstructured.NestedInt64(dep.Object.Object, "spec", "template", "spec", "securityContext", "runAsUser"); uid != 10001 {
		t.Errorf("runAsUser from the image config = %d", uid)
	}
	if inputs := strings.Join(s.Inputs(), "\n"); !strings.Contains(inputs, "(Spring Boot)") || !strings.Contains(inputs, "manifest sha256:") {
		t.Errorf("inputs = %s", inputs)
	}
	variants := s.Variants()
	if len(variants) != 1 || len(variants["prod"]) != len(rs) {
		t.Fatalf("variants = %v", variants)
	}
	for _, r := range variants["prod"] {
		if r.Object.GetKind() == "ConfigMap" {
			if v, _, _ := unstructured.NestedString(r.Object.Object, "data", "SPRING_KAFKA_BOOTSTRAP_SERVERS"); v != "kafka-prod:9092" {
				t.Errorf("prod kafka = %q", v)
			}
		}
	}
	if !strings.Contains(strings.Join(s.Notes(), "\n"), "configuration profiles prod") {
		t.Error("profiles not reported")
	}

	missing := opts
	missing.Images = []string{fake.Host() + "/team/none:1"}
	if _, err := drain(NewSourceExtractor().Extract(context.Background(), missing)); err == nil {
		t.Error("a missing image must fail")
	}
	bad := opts
	bad.Images = []string{"Bad Ref"}
	if _, err := drain(NewSourceExtractor().Extract(context.Background(), bad)); err == nil {
		t.Error("an invalid reference must fail")
	}
}
