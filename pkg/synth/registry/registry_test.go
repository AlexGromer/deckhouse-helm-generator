package registry_test

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/synth/registry"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/synth/registry/registrytest"
)

func TestParseReference(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	tests := []struct {
		in                  string
		reg, repo, tag, dig string
	}{
		{"nginx", "docker.io", "library/nginx", "latest", ""},
		{"org/app:1.2", "docker.io", "org/app", "1.2", ""},
		{"registry.deckhouse.ru/team/app:1.2", "registry.deckhouse.ru", "team/app", "1.2", ""},
		{"localhost:5000/app", "localhost:5000", "app", "latest", ""},
		{"localhost:5000/app@" + digest, "localhost:5000", "app", "", digest},
		{"harbor.local:8443/p/a/b:v1@" + digest, "harbor.local:8443", "p/a/b", "v1", digest},
	}
	for _, tt := range tests {
		r, err := registry.ParseReference(tt.in)
		if err != nil {
			t.Errorf("%s: %v", tt.in, err)
			continue
		}
		if r.Registry != tt.reg || r.Repository != tt.repo || r.Tag != tt.tag || r.Digest != tt.dig {
			t.Errorf("%s: got %+v", tt.in, r)
		}
	}
	for _, bad := range []string{"", "Upper/Case", "app@sha256:short", "a b"} {
		if _, err := registry.ParseReference(bad); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
	r, _ := registry.ParseReference("nginx:1.27")
	if r.Name() != "nginx:1.27" || r.String() != "docker.io/library/nginx:1.27" {
		t.Errorf("Name/String = %s / %s", r.Name(), r.String())
	}
}

func TestClientConfig(t *testing.T) {
	fake := registrytest.New()
	defer fake.Close()
	fake.Token, fake.User, fake.Password = "t0k", "robot", "s3cret"
	fake.Push("team/api", "1.0", map[string]interface{}{
		"config": map[string]interface{}{
			"User":         "10001",
			"ExposedPorts": map[string]interface{}{"8080/tcp": map[string]interface{}{}},
			"Labels":       map[string]interface{}{"org.opencontainers.image.title": "orders-api"},
		},
	}, "linux/amd64", "linux/arm64/v8")

	ref, _ := registry.ParseReference(fake.Host() + "/team/api:1.0")
	creds := func(host string) (string, string, bool) { return "robot", "s3cret", host == fake.Host() }

	for _, platform := range []string{"linux/arm64", "linux/amd64"} {
		c := &registry.Client{PlainHTTP: true, Platform: platform, Credentials: creds}
		cfg, digest, err := c.Config(context.Background(), ref)
		if err != nil {
			t.Fatalf("%s: %v", platform, err)
		}
		if !strings.HasPrefix(platform, cfg.OS+"/"+cfg.Architecture) {
			t.Errorf("%s: got config for %s/%s", platform, cfg.OS, cfg.Architecture)
		}
		if cfg.Config.User != "10001" || len(cfg.Config.ExposedPorts) != 1 || !strings.HasPrefix(digest, "sha256:") {
			t.Errorf("%s: config = %+v digest %s", platform, cfg.Config, digest)
		}
	}

	c := &registry.Client{PlainHTTP: true, Platform: "windows/amd64", Credentials: creds}
	if _, _, err := c.Config(context.Background(), ref); err == nil || !strings.Contains(err.Error(), "linux/arm64/v8") {
		t.Errorf("unknown platform: %v", err)
	}
	c = &registry.Client{PlainHTTP: true}
	if _, _, err := c.Config(context.Background(), ref); err == nil {
		t.Error("expected token request without credentials to fail")
	}
	missing, _ := registry.ParseReference(fake.Host() + "/team/none:1")
	c = &registry.Client{PlainHTTP: true, Credentials: creds}
	if _, _, err := c.Config(context.Background(), missing); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("missing image: %v", err)
	}
}

func TestClientDigestReference(t *testing.T) {
	fake := registrytest.New()
	defer fake.Close()
	d := fake.Push("app", "1", map[string]interface{}{"config": map[string]interface{}{"User": "1"}})
	ref, err := registry.ParseReference(fake.Host() + "/app@" + d)
	if err != nil {
		t.Fatal(err)
	}
	c := &registry.Client{PlainHTTP: true}
	if _, got, err := c.Config(context.Background(), ref); err != nil || got != d {
		t.Errorf("digest ref: %s, %v", got, err)
	}
}

func TestDockerConfigCredentials(t *testing.T) {
	dir := t.TempDir()
	auth := base64.StdEncoding.EncodeToString([]byte("robot:s3cret"))
	cfg := `{"auths":{"registry.example.com":{"auth":"` + auth + `"},"https://index.docker.io/v1/":{"username":"hub","password":"pw"}},"credsStore":"desktop"}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_CONFIG", dir)
	creds, helpers := registry.DockerConfigCredentials()
	if creds == nil || !helpers {
		t.Fatalf("creds=%v helpers=%v", creds != nil, helpers)
	}
	if u, p, ok := creds("registry.example.com"); !ok || u != "robot" || p != "s3cret" {
		t.Errorf("registry.example.com: %s %s %v", u, p, ok)
	}
	if u, _, ok := creds("docker.io"); !ok || u != "hub" {
		t.Errorf("docker.io: %s %v", u, ok)
	}
	if _, _, ok := creds("other.example.com"); ok {
		t.Error("unexpected credentials for an unknown host")
	}
}
