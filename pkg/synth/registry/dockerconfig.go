package registry

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// helperTimeout bounds one run of a credential helper; a variable for tests.
var helperTimeout = 30 * time.Second

// identityTokenUser is the user name a credential helper returns for an
// identity (OAuth2 refresh) token instead of a password.
const identityTokenUser = "<token>"

// DockerCredentials looks up registry credentials in a docker config.json:
// its "auths" entries and the credential helpers named by "credHelpers" and
// "credsStore" (docker/docker-credential-helpers protocol). Lookups are
// cached per host; problems that left a host without credentials are
// collected for Warnings. A nil *DockerCredentials has no credentials.
type DockerCredentials struct {
	path string
	cfg  dockerConfig

	mu       sync.Mutex
	cache    map[string]credential
	warnings []string
}

type dockerConfig struct {
	Auths map[string]struct {
		Auth     string `json:"auth"`
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"auths"`
	CredsStore  string            `json:"credsStore"`
	CredHelpers map[string]string `json:"credHelpers"`
}

type credential struct {
	user, password string
	ok             bool
}

// DockerConfigCredentials reads $DOCKER_CONFIG/config.json or
// ~/.docker/config.json. It returns nil when there is no such file; a file
// that is not valid JSON gives no credentials and a warning.
func DockerConfigCredentials() *DockerCredentials {
	dir := os.Getenv("DOCKER_CONFIG")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		dir = filepath.Join(home, ".docker")
	}
	path := filepath.Join(dir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	d := &DockerCredentials{path: path, cache: map[string]credential{}}
	if json.Unmarshal(data, &d.cfg) != nil {
		// The parse error is not reported: it may quote the file's content.
		d.cfg = dockerConfig{}
		d.warnings = append(d.warnings, fmt.Sprintf("%s is not valid JSON; no registry credentials were read from it", path))
	}
	return d
}

// Lookup returns the credentials for a registry host ("docker.io" for
// Docker Hub). It has the signature of Credentials.
//
// Order: credHelpers[host], then auths[host], then credsStore. Docker CLI
// (docker/cli config/configfile/file.go, GetAuthConfig and
// getConfiguredCredentialStore) picks credHelpers[host] if the key exists,
// else credsStore if set, else the auths entries, and with a helper chosen
// it does not use the auths secret. dhg follows that, except that a non-empty
// auths entry is used before credsStore: Docker itself leaves only empty
// placeholders in auths when it has a store, so such an entry was written by
// another tool or by hand, and using it saves running the helper. As in
// Docker, an empty credHelpers value selects the auths entries over
// credsStore, and a failing per-registry helper is final. Keys are matched
// as host, https://host, http://host and, for Docker Hub, its index URL and
// API host (Docker matches the exact server address only).
func (d *DockerCredentials) Lookup(host string) (user, password string, ok bool) {
	if d == nil {
		return "", "", false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	c, cached := d.cache[host]
	if !cached {
		c = d.lookup(host)
		d.cache[host] = c
	}
	return c.user, c.password, c.ok
}

// Warnings returns why credentials were not found: a config.json that is not
// JSON, helpers that are missing, failed or had nothing for a host. Helper
// warnings appear as Lookup runs, one per host. They hold no secrets.
func (d *DockerCredentials) Warnings() []string {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.warnings...)
}

func (d *DockerCredentials) lookup(host string) credential {
	keys := []string{host, "https://" + host, "http://" + host}
	if host == dockerHub {
		keys = append(keys, dockerHubIndexAuth, dockerHubAPI)
	}
	for _, k := range keys {
		if helper, found := d.cfg.CredHelpers[k]; found {
			if helper == "" {
				return d.fromAuths(keys)
			}
			return d.runHelper(helper, host)
		}
	}
	if c := d.fromAuths(keys); c.ok || d.cfg.CredsStore == "" {
		return c
	}
	return d.runHelper(d.cfg.CredsStore, host)
}

func (d *DockerCredentials) fromAuths(keys []string) credential {
	for _, k := range keys {
		a, ok := d.cfg.Auths[k]
		if !ok {
			continue
		}
		if a.Username != "" {
			return credential{a.Username, a.Password, true}
		}
		raw, err := base64.StdEncoding.DecodeString(a.Auth)
		if err != nil {
			continue
		}
		if user, pass, ok := strings.Cut(string(raw), ":"); ok {
			return credential{user, pass, true}
		}
	}
	return credential{}
}

// runHelper runs `docker-credential-<name> get` with the server URL on stdin,
// as Docker does: the registry host, or https://index.docker.io/v1/ for
// Docker Hub. A failure is recorded as a warning and gives no credentials.
func (d *DockerCredentials) runHelper(name, host string) credential {
	program := "docker-credential-" + name
	c, err := getFromHelper(program, host)
	if err != nil {
		d.warnings = append(d.warnings, fmt.Sprintf("no registry credentials for %s: %s: %v", host, program, err))
	}
	return c
}

func getFromHelper(program, host string) (credential, error) {
	if strings.ContainsAny(program, `/\`) {
		return credential{}, errors.New("invalid helper name")
	}
	path, err := exec.LookPath(program)
	if err != nil {
		return credential{}, errors.New("not found in PATH")
	}
	serverURL := host
	if host == dockerHub {
		serverURL = dockerHubIndexAuth
	}
	ctx, cancel := context.WithTimeout(context.Background(), helperTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "get") //nolint:gosec // the helper is named by the user's docker config.json
	cmd.Stdin = strings.NewReader(serverURL)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return credential{}, fmt.Errorf("timed out after %s", helperTimeout)
		}
		// Helpers report errors on stdout; stderr is not read.
		msg := strings.TrimSpace(stdout.String())
		if strings.Contains(msg, "credentials not found") {
			return credential{}, errors.New("no credentials stored")
		}
		if line, _, _ := strings.Cut(msg, "\n"); line != "" && len(line) <= 200 {
			return credential{}, fmt.Errorf("%v: %s", err, line)
		}
		return credential{}, err
	}
	var out struct {
		Username string `json:"Username"`
		Secret   string `json:"Secret"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		// The error is not wrapped: it may quote the secret.
		return credential{}, errors.New("printed invalid JSON")
	}
	switch {
	case out.Username == identityTokenUser:
		return credential{}, errors.New("identity tokens are not supported")
	case out.Secret == "":
		return credential{}, errors.New("returned no secret")
	}
	return credential{out.Username, out.Secret, true}, nil
}
