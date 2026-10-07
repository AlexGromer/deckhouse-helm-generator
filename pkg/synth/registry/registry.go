// Package registry reads an image's configuration from a registry speaking
// the Docker Registry HTTP API v2 / OCI Distribution protocol: Docker Hub,
// Harbor, GitLab, Nexus, the Deckhouse registry. Only the manifest and the
// config blob are fetched, never the layers.
package registry

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// maxDocument bounds the manifest and config documents read from a registry.
const maxDocument = 4 << 20

const (
	mediaOCIIndex      = "application/vnd.oci.image.index.v1+json"
	mediaOCIManifest   = "application/vnd.oci.image.manifest.v1+json"
	mediaDockerList    = "application/vnd.docker.distribution.manifest.list.v2+json"
	mediaDockerV2      = "application/vnd.docker.distribution.manifest.v2+json"
	dockerHub          = "docker.io"
	dockerHubAPI       = "registry-1.docker.io"
	dockerHubIndexAuth = "https://index.docker.io/v1/"
)

// Reference is a parsed image reference.
type Reference struct {
	Registry   string // host[:port], "docker.io" for Docker Hub
	Repository string // e.g. "library/nginx"
	Tag        string
	Digest     string // "sha256:…"
}

// ParseReference parses an image reference with Docker's rules: a first
// path component containing "." or ":" or equal to "localhost" is a
// registry; otherwise the registry is Docker Hub, where single-component
// names live under "library/". The tag defaults to "latest" unless a digest
// is given.
func ParseReference(ref string) (Reference, error) {
	if ref == "" || strings.ContainsAny(ref, " \t\n") {
		return Reference{}, fmt.Errorf("invalid image reference %q", ref)
	}
	var r Reference
	rest := ref
	if i := strings.Index(rest, "@"); i >= 0 {
		r.Digest = rest[i+1:]
		rest = rest[:i]
		if !strings.HasPrefix(r.Digest, "sha256:") || len(r.Digest) != len("sha256:")+64 {
			return Reference{}, fmt.Errorf("invalid digest in image reference %q", ref)
		}
	}
	// A tag is the part after the last ":" that is not part of a host:port.
	if i := strings.LastIndex(rest, ":"); i > strings.LastIndex(rest, "/") {
		r.Tag = rest[i+1:]
		rest = rest[:i]
	}
	first, remainder, hasSlash := strings.Cut(rest, "/")
	if hasSlash && (strings.ContainsAny(first, ".:") || first == "localhost") {
		r.Registry = first
		r.Repository = remainder
	} else {
		r.Registry = dockerHub
		r.Repository = rest
		if !hasSlash {
			r.Repository = "library/" + rest
		}
	}
	if r.Repository == "" || strings.ToLower(r.Repository) != r.Repository {
		return Reference{}, fmt.Errorf("invalid repository in image reference %q", ref)
	}
	if r.Tag == "" && r.Digest == "" {
		r.Tag = "latest"
	}
	return r, nil
}

// String returns the reference in canonical form.
func (r Reference) String() string {
	s := r.Registry + "/" + r.Repository
	if r.Tag != "" {
		s += ":" + r.Tag
	}
	if r.Digest != "" {
		s += "@" + r.Digest
	}
	return s
}

// Name returns the reference as users write it: Docker Hub images without
// the registry, official ones without "library/".
func (r Reference) Name() string {
	s := r.Registry + "/" + r.Repository
	if r.Registry == dockerHub {
		s = strings.TrimPrefix(r.Repository, "library/")
	}
	if r.Tag != "" {
		s += ":" + r.Tag
	}
	if r.Digest != "" {
		s += "@" + r.Digest
	}
	return s
}

func (r Reference) apiHost() string {
	if r.Registry == dockerHub {
		return dockerHubAPI
	}
	return r.Registry
}

// ImageConfig is the part of the OCI image configuration dhg uses.
type ImageConfig struct {
	Architecture string          `json:"architecture"`
	OS           string          `json:"os"`
	Config       ContainerConfig `json:"config"`
}

// ContainerConfig is the execution configuration of an image.
type ContainerConfig struct {
	User         string              `json:"User"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts"`
	Env          []string            `json:"Env"`
	Entrypoint   []string            `json:"Entrypoint"`
	Cmd          []string            `json:"Cmd"`
	Volumes      map[string]struct{} `json:"Volumes"`
	WorkingDir   string              `json:"WorkingDir"`
	Labels       map[string]string   `json:"Labels"`
	StopSignal   string              `json:"StopSignal"`
	// Healthcheck is a Docker extension of the OCI format.
	Healthcheck *Healthcheck `json:"Healthcheck"`
}

// Healthcheck is Docker's HEALTHCHECK; durations are in nanoseconds.
type Healthcheck struct {
	Test        []string `json:"Test"`
	Interval    int64    `json:"Interval"`
	Timeout     int64    `json:"Timeout"`
	StartPeriod int64    `json:"StartPeriod"`
	Retries     int64    `json:"Retries"`
}

// Credentials returns the user name and password for a registry host.
type Credentials func(host string) (user, password string, ok bool)

// Client fetches image configurations.
type Client struct {
	HTTP *http.Client
	// PlainHTTP talks HTTP instead of HTTPS (local registries).
	PlainHTTP bool
	// Platform selects the manifest of a multi-platform image: os/arch[/variant].
	Platform string
	// Credentials are used when the registry asks for authentication.
	Credentials Credentials

	// tokens caches bearer tokens by "<registry> <scope>": a token is only
	// ever sent back to the registry that issued it.
	tokens map[string]string
}

// Config returns the configuration of the image ref points to and the digest
// of the manifest it came from.
func (c *Client) Config(ctx context.Context, ref Reference) (*ImageConfig, string, error) {
	reference := ref.Digest
	if reference == "" {
		reference = ref.Tag
	}
	body, mediaType, digest, err := c.manifest(ctx, ref, reference)
	if err != nil {
		return nil, "", err
	}

	if mediaType == mediaOCIIndex || mediaType == mediaDockerList {
		var index struct {
			Manifests []struct {
				Digest   string `json:"digest"`
				Platform struct {
					OS           string `json:"os"`
					Architecture string `json:"architecture"`
					Variant      string `json:"variant"`
				} `json:"platform"`
			} `json:"manifests"`
		}
		if err := json.Unmarshal(body, &index); err != nil {
			return nil, "", fmt.Errorf("%s: invalid image index: %w", ref, err)
		}
		want := c.platform()
		var match string
		var available []string
		for _, m := range index.Manifests {
			p := m.Platform.OS + "/" + m.Platform.Architecture
			if m.Platform.Variant != "" {
				p += "/" + m.Platform.Variant
			}
			available = append(available, p)
			if match == "" && (p == want || (strings.Count(want, "/") == 1 && strings.HasPrefix(p, want+"/"))) {
				match = m.Digest
			}
		}
		if match == "" {
			return nil, "", fmt.Errorf("%s: no manifest for platform %s (available: %s)", ref, want, strings.Join(available, ", "))
		}
		body, mediaType, digest, err = c.manifest(ctx, ref, match)
		if err != nil {
			return nil, "", err
		}
	}
	if mediaType != mediaOCIManifest && mediaType != mediaDockerV2 {
		return nil, "", fmt.Errorf("%s: unsupported manifest type %q", ref, mediaType)
	}

	var manifest struct {
		Config struct {
			Digest string `json:"digest"`
		} `json:"config"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil || manifest.Config.Digest == "" {
		return nil, "", fmt.Errorf("%s: manifest has no config", ref)
	}
	blob, err := c.get(ctx, ref, "/blobs/"+manifest.Config.Digest, "")
	if err != nil {
		return nil, "", err
	}
	data, err := readLimited(blob.Body)
	blob.Body.Close()
	if err != nil {
		return nil, "", fmt.Errorf("%s: reading config: %w", ref, err)
	}
	if err := verify(data, manifest.Config.Digest); err != nil {
		return nil, "", fmt.Errorf("%s: config blob: %w", ref, err)
	}
	var cfg ImageConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, "", fmt.Errorf("%s: invalid image config: %w", ref, err)
	}
	return &cfg, digest, nil
}

func (c *Client) platform() string {
	if c.Platform == "" {
		return "linux/amd64"
	}
	return c.Platform
}

// manifest fetches a manifest or index and returns its body, media type and
// digest. A digest reference is verified against the body.
func (c *Client) manifest(ctx context.Context, ref Reference, reference string) ([]byte, string, string, error) {
	accept := strings.Join([]string{mediaOCIIndex, mediaDockerList, mediaOCIManifest, mediaDockerV2}, ", ")
	resp, err := c.get(ctx, ref, "/manifests/"+reference, accept)
	if err != nil {
		return nil, "", "", err
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body)
	if err != nil {
		return nil, "", "", fmt.Errorf("%s: reading manifest: %w", ref, err)
	}
	digest := "sha256:" + hexSum(body)
	if strings.HasPrefix(reference, "sha256:") {
		if err := verify(body, reference); err != nil {
			return nil, "", "", fmt.Errorf("%s: manifest: %w", ref, err)
		}
	}
	// Some registries and proxies send a generic Content-Type; the document
	// then names its own media type.
	mediaType, _, _ := strings.Cut(resp.Header.Get("Content-Type"), ";")
	mediaType = strings.TrimSpace(mediaType)
	switch mediaType {
	case mediaOCIIndex, mediaDockerList, mediaOCIManifest, mediaDockerV2:
	default:
		var probe struct {
			MediaType string `json:"mediaType"`
		}
		_ = json.Unmarshal(body, &probe)
		mediaType = probe.MediaType
	}
	return body, strings.TrimSpace(mediaType), digest, nil
}

// get issues a GET against /v2/<repo><path>, authenticating on a 401.
func (c *Client) get(ctx context.Context, ref Reference, path, accept string) (*http.Response, error) {
	scheme := "https"
	if c.PlainHTTP {
		scheme = "http"
	}
	u := scheme + "://" + ref.apiHost() + "/v2/" + ref.Repository + path
	scope := "repository:" + ref.Repository + ":pull"

	do := func(auth string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		return c.httpClient().Do(req)
	}

	resp, err := do(c.tokens[ref.Registry+" "+scope])
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ref, err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		challenge := resp.Header.Get("WWW-Authenticate")
		resp.Body.Close()
		auth, err := c.authorize(ctx, ref, challenge, scope)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", ref, err)
		}
		if resp, err = do(auth); err != nil {
			return nil, fmt.Errorf("%s: %w", ref, err)
		}
	}
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, fmt.Errorf("%s: GET %s: %s %s", ref, path, resp.Status, strings.TrimSpace(string(msg)))
	}
	return resp, nil
}

var challengeParam = regexp.MustCompile(`(\w+)="([^"]*)"`)

// authorize answers a WWW-Authenticate challenge with an Authorization value.
func (c *Client) authorize(ctx context.Context, ref Reference, challenge, scope string) (string, error) {
	user, password, haveCreds := "", "", false
	if c.Credentials != nil {
		user, password, haveCreds = c.Credentials(ref.Registry)
	}
	scheme, params, _ := strings.Cut(challenge, " ")
	switch strings.ToLower(scheme) {
	case "basic":
		if !haveCreds {
			return "", errors.New("registry requires credentials (none found in docker config.json)")
		}
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password)), nil
	case "bearer":
	default:
		return "", fmt.Errorf("unsupported authentication challenge %q", challenge)
	}

	p := map[string]string{}
	for _, m := range challengeParam.FindAllStringSubmatch(params, -1) {
		p[m[1]] = m[2]
	}
	if p["realm"] == "" {
		return "", fmt.Errorf("bearer challenge without realm: %q", challenge)
	}
	realm, err := url.Parse(p["realm"])
	if err != nil {
		return "", fmt.Errorf("invalid token realm: %w", err)
	}
	q := realm.Query()
	if p["service"] != "" {
		q.Set("service", p["service"])
	}
	if p["scope"] != "" {
		scope = p["scope"]
	}
	q.Set("scope", scope)
	realm.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realm.String(), nil)
	if err != nil {
		return "", err
	}
	if haveCreds {
		req.SetBasicAuth(user, password)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request: %s", resp.Status)
	}
	body, err := readLimited(resp.Body)
	if err != nil {
		return "", err
	}
	var tok struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("invalid token response: %w", err)
	}
	token := tok.Token
	if token == "" {
		token = tok.AccessToken
	}
	if token == "" {
		return "", errors.New("token response has no token")
	}
	if c.tokens == nil {
		c.tokens = map[string]string{}
	}
	c.tokens[ref.Registry+" "+scope] = "Bearer " + token
	return "Bearer " + token, nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func readLimited(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxDocument+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxDocument {
		return nil, fmt.Errorf("document larger than %d bytes", maxDocument)
	}
	return data, nil
}

func hexSum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func verify(data []byte, digest string) error {
	if !strings.HasPrefix(digest, "sha256:") {
		return fmt.Errorf("unsupported digest %q", digest)
	}
	if got := "sha256:" + hexSum(data); got != digest {
		return fmt.Errorf("digest mismatch: got %s, want %s", got, digest)
	}
	return nil
}
