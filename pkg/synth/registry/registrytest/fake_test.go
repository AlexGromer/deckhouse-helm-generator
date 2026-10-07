package registrytest

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func get(t *testing.T, url, auth string) (int, string, http.Header) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), resp.Header
}

func TestFake(t *testing.T) {
	s := New()
	defer s.Close()
	s.Token, s.User, s.Password = "t", "u", "p"
	single := s.Push("a/b", "1", map[string]interface{}{"config": map[string]interface{}{}})
	s.Push("a/b", "multi", map[string]interface{}{}, "linux/amd64", "linux/arm64/v8")

	code, _, hdr := get(t, s.URL+"/v2/a/b/manifests/1", "")
	if code != http.StatusUnauthorized || !strings.Contains(hdr.Get("WWW-Authenticate"), `scope="repository:a/b:pull"`) {
		t.Errorf("challenge: %d %v", code, hdr)
	}
	if code, _, _ := get(t, s.URL+"/token", ""); code != http.StatusUnauthorized {
		t.Errorf("token without credentials: %d", code)
	}
	req, _ := http.NewRequest(http.MethodGet, s.URL+"/token", nil)
	req.SetBasicAuth("u", "p")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("token: %d", resp.StatusCode)
	}

	for path, want := range map[string]int{
		"/v2/a/b/manifests/1":         http.StatusOK,
		"/v2/a/b/manifests/" + single: http.StatusOK,
		"/v2/a/b/manifests/multi":     http.StatusOK,
		"/v2/a/b/manifests/none":      http.StatusNotFound,
		"/v2/a/b/blobs/sha256:00":     http.StatusNotFound,
		"/v2/a/b/tags/list":           http.StatusNotFound,
	} {
		if code, _, _ := get(t, s.URL+path, "Bearer t"); code != want {
			t.Errorf("%s: %d, want %d", path, code, want)
		}
	}
	if _, body, hdr := get(t, s.URL+"/v2/a/b/manifests/multi", "Bearer t"); !strings.Contains(hdr.Get("Content-Type"), "index") || !strings.Contains(body, `"variant":"v8"`) {
		t.Errorf("multi-platform tag must be an index: %s", body)
	}
	if !strings.Contains(s.Host(), "127.0.0.1") {
		t.Errorf("host = %s", s.Host())
	}
}
