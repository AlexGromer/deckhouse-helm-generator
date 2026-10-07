package registry

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeHelper is a docker credential helper keyed by the server URL it reads
// on stdin; it appends each server URL to $DHG_HELPER_CALLS.
const fakeHelper = `url=$(cat)
echo "$url" >> "$DHG_HELPER_CALLS"
case "$url" in
https://index.docker.io/v1/) echo '{"ServerURL":"https://index.docker.io/v1/","Username":"hub","Secret":"hub-pw"}' ;;
per.example) echo '{"ServerURL":"per.example","Username":"per","Secret":"per-pw"}' ;;
store.example) echo '{"ServerURL":"store.example","Username":"store","Secret":"store-pw"}' ;;
token.example) echo '{"ServerURL":"token.example","Username":"<token>","Secret":"refresh-secret"}' ;;
badjson.example) echo '{"Username":"u","Secret":"json-secret"' ;;
nosecret.example) echo '{"Username":"u"}' ;;
fail.example) echo boom; exit 3 ;;
long.example) printf '%0300d\n' 0; exit 1 ;;
*) echo "credentials not found in native keychain"; exit 1 ;;
esac
`

// installHelpers writes docker-credential-<name> scripts into a directory
// prepended to PATH, and a config.json into $DOCKER_CONFIG. It returns the
// file the fake helper logs its calls to.
func installHelpers(t *testing.T, config string, helpers map[string]string) string {
	t.Helper()
	bin := t.TempDir()
	for name, body := range helpers {
		if err := os.WriteFile(filepath.Join(bin, "docker-credential-"+name), []byte("#!/bin/sh\n"+body), 0o755); err != nil { //nolint:gosec // an executable test helper
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	calls := filepath.Join(bin, "calls")
	t.Setenv("DHG_HELPER_CALLS", calls)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_CONFIG", dir)
	return calls
}

func helperCalls(t *testing.T, calls string) []string {
	t.Helper()
	data, err := os.ReadFile(calls)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(data))
}

func TestDockerCredentialHelpers(t *testing.T) {
	ab := base64.StdEncoding.EncodeToString([]byte("a:b"))
	calls := installHelpers(t, `{
  "auths": {"auths.example": {"auth": "`+ab+`"}, "fileonly.example": {"auth": "`+ab+`"}, "placeholder.example": {}},
  "credHelpers": {
    "per.example": "fake", "token.example": "fake", "badjson.example": "fake", "nosecret.example": "fake",
    "fail.example": "fake", "long.example": "fake", "notfound.example": "fake",
    "absent.example": "absent", "evil.example": "../evil", "slow.example": "slow",
    "fileonly.example": "", "nofile.example": ""
  },
  "credsStore": "fake"
}`, map[string]string{"fake": fakeHelper, "slow": "exec sleep 5\n"})
	old := helperTimeout
	helperTimeout = 200 * time.Millisecond
	t.Cleanup(func() { helperTimeout = old })

	d := DockerConfigCredentials()
	cases := []struct{ host, user, password string }{
		{"per.example", "per", "per-pw"},       // credHelpers
		{"auths.example", "a", "b"},            // auths before credsStore
		{"store.example", "store", "store-pw"}, // credsStore
		{"placeholder.example", "", ""},        // empty auths entry → credsStore, which has none
		{"docker.io", "hub", "hub-pw"},         // credsStore with Docker Hub's server URL
		{"fileonly.example", "a", "b"},         // empty credHelpers value → auths only
		{"nofile.example", "", ""},             // … even when auths has nothing
		{"notfound.example", "", ""},           // "credentials not found"
		{"token.example", "", ""},              // identity token
		{"badjson.example", "", ""},            // invalid JSON
		{"nosecret.example", "", ""},           // no secret
		{"fail.example", "", ""},               // exit 3
		{"long.example", "", ""},               // exit 1, message too long to quote
		{"absent.example", "", ""},             // helper not in PATH
		{"evil.example", "", ""},               // helper name with a path
		{"slow.example", "", ""},               // timeout
		{"https://per.example", "", ""},        // not a credHelpers key → credsStore
	}
	for round := 0; round < 2; round++ { // the second round is served from the cache
		for _, c := range cases {
			u, p, ok := d.Lookup(c.host)
			if u != c.user || p != c.password || ok != (c.user != "") {
				t.Errorf("round %d, %s: %q %q %v, want %q %q", round, c.host, u, p, ok, c.user, c.password)
			}
		}
	}

	// Each host runs the helper once; auths and empty credHelpers values
	// never run it.
	want := "per.example store.example placeholder.example https://index.docker.io/v1/ notfound.example token.example badjson.example nosecret.example fail.example long.example https://per.example"
	if got := strings.Join(helperCalls(t, calls), " "); got != want {
		t.Errorf("helper calls:\n got %s\nwant %s", got, want)
	}

	w := strings.Join(d.Warnings(), "\n")
	for _, s := range []string{
		"no registry credentials for placeholder.example: docker-credential-fake: no credentials stored",
		"notfound.example: docker-credential-fake: no credentials stored",
		"token.example: docker-credential-fake: identity tokens are not supported",
		"badjson.example: docker-credential-fake: printed invalid JSON",
		"nosecret.example: docker-credential-fake: returned no secret",
		"fail.example: docker-credential-fake: exit status 3: boom",
		"long.example: docker-credential-fake: exit status 1\n",
		"absent.example: docker-credential-absent: not found in PATH",
		"evil.example: docker-credential-../evil: invalid helper name",
		"slow.example: docker-credential-slow: timed out after 200ms",
	} {
		if !strings.Contains(w, s) {
			t.Errorf("warnings lack %q:\n%s", s, w)
		}
	}
	if n := len(d.Warnings()); n != 11 {
		t.Errorf("%d warnings, want 11 (one per host):\n%s", n, w)
	}
	for _, secret := range []string{"-pw", "refresh-secret", "json-secret", "0000"} {
		if strings.Contains(w, secret) {
			t.Errorf("warnings quote helper output %q:\n%s", secret, w)
		}
	}
}

func TestDockerCredentialsHubHelperAndHome(t *testing.T) {
	// A per-registry helper for Docker Hub is keyed by its index URL, as
	// docker login writes it.
	calls := installHelpers(t, `{"credHelpers":{"https://index.docker.io/v1/":"fake"}}`, map[string]string{"fake": fakeHelper})
	if u, _, ok := DockerConfigCredentials().Lookup("docker.io"); !ok || u != "hub" {
		t.Errorf("docker.io: %q %v", u, ok)
	}
	if got := helperCalls(t, calls); len(got) != 1 || got[0] != dockerHubIndexAuth {
		t.Errorf("helper calls = %v", got)
	}

	// Without DOCKER_CONFIG the file is ~/.docker/config.json.
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".docker"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".docker", "config.json"), []byte(`{"auths":{"h.example":{"username":"u","password":"p"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_CONFIG", "")
	t.Setenv("HOME", home)
	if u, p, ok := DockerConfigCredentials().Lookup("h.example"); !ok || u != "u" || p != "p" {
		t.Errorf("home config: %q %q %v", u, p, ok)
	}
	t.Setenv("HOME", "")
	if DockerConfigCredentials() != nil {
		t.Error("no home directory gives no credentials")
	}

	var none *DockerCredentials
	if _, _, ok := none.Lookup("h.example"); ok || none.Warnings() != nil {
		t.Error("nil DockerCredentials has no credentials")
	}
}
