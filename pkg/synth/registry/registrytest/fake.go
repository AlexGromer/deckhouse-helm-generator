// Package registrytest provides an in-memory registry speaking the parts of
// the Docker Registry HTTP API v2 dhg uses, with optional bearer-token
// authentication. It serves multi-platform images as an OCI index.
package registrytest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// Server is a fake registry.
type Server struct {
	*httptest.Server
	// Token, when set, is required as a bearer token; it is issued by /token
	// to clients presenting User/Password (or anyone when User is empty).
	Token    string
	User     string
	Password string

	mu    sync.Mutex
	blobs map[string][]byte // digest → content
	tags  map[string]string // "repo:tag" → digest of the manifest or index
	types map[string]string // digest → media type
}

// New starts a fake registry.
func New() *Server {
	s := &Server{blobs: map[string][]byte{}, tags: map[string]string{}, types: map[string]string{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	return s
}

// Host returns host:port to use as the registry in image references.
func (s *Server) Host() string {
	return strings.TrimPrefix(s.URL, "http://")
}

// Push stores an image. config is the image configuration (its "config"
// object holds User, ExposedPorts, …). With more than one platform the tag
// points to an OCI index.
func (s *Server) Push(repo, tag string, config map[string]interface{}, platforms ...string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(platforms) == 0 {
		platforms = []string{"linux/amd64"}
	}
	var entries []interface{}
	var single string
	for _, p := range platforms {
		parts := strings.Split(p, "/")
		cfg := map[string]interface{}{"os": parts[0], "architecture": parts[1]}
		for k, v := range config {
			cfg[k] = v
		}
		cfgDigest := s.put(mustJSON(cfg), "application/vnd.oci.image.config.v1+json")
		manifest := mustJSON(map[string]interface{}{
			"schemaVersion": 2,
			"mediaType":     "application/vnd.oci.image.manifest.v1+json",
			"config":        map[string]interface{}{"mediaType": "application/vnd.oci.image.config.v1+json", "digest": cfgDigest, "size": 1},
			"layers":        []interface{}{},
		})
		single = s.put(manifest, "application/vnd.oci.image.manifest.v1+json")
		platform := map[string]interface{}{"os": parts[0], "architecture": parts[1]}
		if len(parts) > 2 {
			platform["variant"] = parts[2]
		}
		entries = append(entries, map[string]interface{}{
			"mediaType": "application/vnd.oci.image.manifest.v1+json", "digest": single, "size": len(manifest), "platform": platform,
		})
	}
	if len(platforms) == 1 {
		s.tags[repo+":"+tag] = single
		return single
	}
	index := mustJSON(map[string]interface{}{
		"schemaVersion": 2, "mediaType": "application/vnd.oci.image.index.v1+json", "manifests": entries,
	})
	d := s.put(index, "application/vnd.oci.image.index.v1+json")
	s.tags[repo+":"+tag] = d
	return d
}

func (s *Server) put(data []byte, mediaType string) string {
	sum := sha256.Sum256(data)
	d := "sha256:" + hex.EncodeToString(sum[:])
	s.blobs[d] = data
	s.types[d] = mediaType
	return d
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/token" {
		if s.User != "" {
			if u, p, ok := r.BasicAuth(); !ok || u != s.User || p != s.Password {
				http.Error(w, "bad credentials", http.StatusUnauthorized)
				return
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"token": s.Token})
		return
	}
	if s.Token != "" && r.Header.Get("Authorization") != "Bearer "+s.Token {
		w.Header().Set("WWW-Authenticate", `Bearer realm="`+s.URL+`/token",service="fake",scope="`+scopeOf(r.URL.Path)+`"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v2/")
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := strings.Index(path, "/manifests/"); i >= 0 {
		repo, ref := path[:i], path[i+len("/manifests/"):]
		d := ref
		if !strings.HasPrefix(ref, "sha256:") {
			d = s.tags[repo+":"+ref]
		}
		data, ok := s.blobs[d]
		if !ok {
			http.Error(w, "manifest unknown", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", s.types[d])
		_, _ = w.Write(data)
		return
	}
	if i := strings.Index(path, "/blobs/"); i >= 0 {
		data, ok := s.blobs[path[i+len("/blobs/"):]]
		if !ok {
			http.Error(w, "blob unknown", http.StatusNotFound)
			return
		}
		_, _ = w.Write(data)
		return
	}
	http.NotFound(w, r)
}

func scopeOf(path string) string {
	repo := strings.TrimPrefix(path, "/v2/")
	if i := strings.Index(repo, "/manifests/"); i >= 0 {
		repo = repo[:i]
	} else if i := strings.Index(repo, "/blobs/"); i >= 0 {
		repo = repo[:i]
	}
	return "repository:" + repo + ":pull"
}

func mustJSON(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
