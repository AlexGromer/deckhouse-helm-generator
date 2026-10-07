package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// server serves a fixed manifest and config for repository "app", with
// authentication and body tweaks chosen by the test.
type server struct {
	manifest    []byte
	contentType string
	config      []byte
	challenge   string // WWW-Authenticate sent without a valid Authorization
	wantAuth    string
	token       string // body of /token
}

func (s *server) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = w.Write([]byte(s.token))
			return
		}
		if s.challenge != "" && (s.wantAuth == "" || r.Header.Get("Authorization") != s.wantAuth) {
			w.Header().Set("WWW-Authenticate", s.challenge)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case strings.Contains(r.URL.Path, "/manifests/"):
			if s.contentType != "" {
				w.Header().Set("Content-Type", s.contentType)
			}
			_, _ = w.Write(s.manifest)
		case strings.Contains(r.URL.Path, "/blobs/"):
			_, _ = w.Write(s.config)
		}
	})
}

func manifestFor(config []byte, mediaType string) []byte {
	m, _ := json.Marshal(map[string]interface{}{
		"mediaType": mediaType,
		"config":    map[string]interface{}{"digest": "sha256:" + hexSum(config)},
	})
	return m
}

func fetch(t *testing.T, s *server, creds Credentials) (*ImageConfig, error) {
	t.Helper()
	ts := httptest.NewServer(s.handler(t))
	defer ts.Close()
	s.challenge = strings.ReplaceAll(s.challenge, "REALM", ts.URL+"/token")
	ref, err := ParseReference(strings.TrimPrefix(ts.URL, "http://") + "/app:1")
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{PlainHTTP: true, Credentials: creds}
	cfg, _, err := c.Config(context.Background(), ref)
	return cfg, err
}

func TestClientDockerV2AndBasicAuth(t *testing.T) {
	config := []byte(`{"os":"linux","architecture":"amd64","config":{"User":"1"}}`)
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte("u:p"))
	creds := func(string) (string, string, bool) { return "u", "p", true }

	// Docker v2 manifest without a Content-Type: the media type comes from the body.
	s := &server{manifest: manifestFor(config, mediaDockerV2), config: config, challenge: `Basic realm="x"`, wantAuth: basic}
	cfg, err := fetch(t, s, creds)
	if err != nil || cfg.Config.User != "1" {
		t.Fatalf("basic auth: %v %v", cfg, err)
	}
	s = &server{manifest: manifestFor(config, mediaDockerV2), config: config, challenge: `Basic realm="x"`, wantAuth: basic}
	if _, err := fetch(t, s, nil); err == nil || !strings.Contains(err.Error(), "requires credentials") {
		t.Errorf("basic without credentials: %v", err)
	}
}

func TestClientErrors(t *testing.T) {
	config := []byte(`{"config":{}}`)
	good := manifestFor(config, mediaOCIManifest)
	tests := []struct {
		name string
		s    *server
		want string
	}{
		{"digest mismatch", &server{manifest: good, config: []byte(`{"config":{"User":"0"}}`), contentType: mediaOCIManifest}, "digest mismatch"},
		{"unsupported manifest", &server{manifest: []byte(`{"schemaVersion":1}`), contentType: "application/vnd.docker.distribution.manifest.v1+json"}, "unsupported manifest type"},
		{"no config", &server{manifest: []byte(`{}`), contentType: mediaOCIManifest}, "manifest has no config"},
		{"bad config", &server{manifest: manifestFor([]byte("nope"), mediaOCIManifest), config: []byte("nope"), contentType: mediaOCIManifest}, "invalid image config"},
		{"bad index", &server{manifest: []byte(`{"manifests":"x"}`), contentType: mediaOCIIndex}, "invalid image index"},
		{"too large", &server{manifest: []byte(strings.Repeat(" ", maxDocument+10)), contentType: mediaOCIManifest}, "larger than"},
		{"unknown challenge", &server{manifest: good, config: config, challenge: `Negotiate`}, "unsupported authentication"},
		{"no realm", &server{manifest: good, config: config, challenge: `Bearer service="x"`}, "without realm"},
		{"empty token", &server{manifest: good, config: config, challenge: `Bearer realm="REALM"`, token: `{}`}, "no token"},
		{"token not JSON", &server{manifest: good, config: config, challenge: `Bearer realm="REALM"`, token: `x`}, "invalid token response"},
		{"token rejected", &server{manifest: good, config: config, challenge: `Bearer realm="REALM"`, token: `{"access_token":"a"}`, wantAuth: "Bearer b"}, "401"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := fetch(t, tt.s, nil); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestVerifyAndPlatform(t *testing.T) {
	if verify([]byte("x"), "md5:abc") == nil {
		t.Error("only sha256 digests are accepted")
	}
	if (&Client{}).platform() != "linux/amd64" {
		t.Error("default platform")
	}
	if (&Client{}).httpClient() == nil {
		t.Error("default HTTP client")
	}
}

func TestReferenceFormsAndDockerConfigEdges(t *testing.T) {
	r, err := ParseReference("nginx:1.27@sha256:" + strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(r.String(), "docker.io/library/nginx:1.27@sha256:") || !strings.HasPrefix(r.Name(), "nginx:1.27@sha256:") || r.apiHost() != dockerHubAPI {
		t.Errorf("String = %s Name = %s apiHost = %s", r.String(), r.Name(), r.apiHost())
	}

	dir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dir)
	if creds, _ := DockerConfigCredentials(); creds != nil {
		t.Error("no config.json gives no credentials")
	}
	write := func(s string) {
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("{")
	if creds, _ := DockerConfigCredentials(); creds != nil {
		t.Error("invalid config.json gives no credentials")
	}
	write(`{"auths":{"bad.example":{"auth":"!!"},"nocolon.example":{"auth":"` + base64.StdEncoding.EncodeToString([]byte("x")) + `"}}}`)
	creds, _ := DockerConfigCredentials()
	for _, host := range []string{"bad.example", "nocolon.example", "missing.example"} {
		if _, _, ok := creds(host); ok {
			t.Errorf("%s: unusable auth entries must be skipped", host)
		}
	}
}
