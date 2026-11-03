package synth

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseKnownKeys(t *testing.T) {
	got, err := ParseKnownKeys("platform.txt", strings.NewReader(`# keys the platform provides
DB_URL
 DB_LOGIN   # trailing comment
vault:DB_PASSWORD
vault:API_TOKEN

platform-env:KAFKA_HOST
DB_URL
`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"platform.txt": {"DB_LOGIN", "DB_URL"},
		"vault":        {"API_TOKEN", "DB_PASSWORD"},
		"platform-env": {"KAFKA_HOST"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	for _, bad := range []string{"vault:\n", ":KEY\n", "A B\n"} {
		if _, err := ParseKnownKeys("f", strings.NewReader(bad)); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}

func TestLoadKnownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known.txt")
	if err := os.WriteFile(path, []byte("A\ncm:B\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadKnownKeys(path)
	if err != nil || !reflect.DeepEqual(got, map[string][]string{"known.txt": {"A"}, "cm": {"B"}}) {
		t.Errorf("got %v err %v", got, err)
	}
	if _, err := LoadKnownKeys(path + ".none"); err == nil {
		t.Error("a missing file must fail")
	}
}

func TestMergeKnownKeys(t *testing.T) {
	got := MergeKnownKeys(map[string][]string{"a": {"X", "Y"}}, nil, map[string][]string{"a": {"Y", "Z"}, "b": {"Q"}})
	want := map[string][]string{"a": {"X", "Y", "Z"}, "b": {"Q"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

const knownKeysYAML = `spring:
  application:
    name: orders
  datasource:
    url: ${DB_URL}
    username: ${DB_LOGIN}
    password: ${DB_PASSWORD}
  kafka:
    bootstrap-servers: ${KAFKA_HOST}:${KAFKA_PORT}
`

// Placeholders that known keys cover are not emitted as empty variables and
// are not "required"; the others stay required.
func TestKnownKeysCoverPlaceholders(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":                            pom,
		"src/main/resources/application.yml": knownKeysYAML,
	})
	var notes Notes
	s, err := ReadSource(dir, &notes)
	if err != nil {
		t.Fatal(err)
	}
	s.KnownKeys = map[string][]string{
		"platform-env": {"DB_LOGIN", "DB_URL", "KAFKA_HOST", "KAFKA_PORT", "UNUSED"},
		"vault":        {"DB_PASSWORD"},
	}
	var run Notes
	app := s.App("orders:1", "", &run)
	names := envNames(app)
	for _, covered := range []string{"DB_LOGIN", "DB_URL", "KAFKA_HOST", "KAFKA_PORT", "DB_PASSWORD"} {
		for _, n := range names {
			if n == covered {
				t.Errorf("%s is known but was emitted as a variable", covered)
			}
		}
	}
	all := strings.Join(run.Items(), "\n")
	for _, want := range []string{
		"4 variables come from platform-env (DB_LOGIN, DB_URL, KAFKA_HOST, KAFKA_PORT)",
		"1 variable comes from vault (DB_PASSWORD)",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("notes lack %q:\n%s", want, all)
		}
	}
	if strings.Contains(all, "empty and required") {
		t.Errorf("everything is covered, nothing may be required:\n%s", all)
	}
}

func TestKnownKeysLeaveTheRestRequired(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":                            pom,
		"src/main/resources/application.yml": knownKeysYAML,
	})
	s, err := ReadSource(dir, &Notes{})
	if err != nil {
		t.Fatal(err)
	}
	s.KnownKeys = map[string][]string{"platform-env": {"DB_URL", "DB_LOGIN"}}
	var run Notes
	app := s.App("orders:1", "", &run)
	hasAll(t, envNames(app), "DB_PASSWORD", "KAFKA_HOST", "KAFKA_PORT")
	all := strings.Join(run.Items(), "\n")
	if !strings.Contains(all, "2 variables come from platform-env (DB_LOGIN, DB_URL)") ||
		!strings.Contains(all, "3 environment variables are empty and required (DB_PASSWORD, KAFKA_HOST, KAFKA_PORT)") {
		t.Errorf("notes:\n%s", all)
	}
}

// Without known keys nothing changes.
func TestNoKnownKeysKeepsOutput(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":                            pom,
		"src/main/resources/application.yml": knownKeysYAML,
	})
	var notes Notes
	app, err := FromSource(dir, "orders:1", &notes)
	if err != nil {
		t.Fatal(err)
	}
	hasAll(t, envNames(app), "DB_URL", "DB_LOGIN", "DB_PASSWORD", "KAFKA_HOST", "KAFKA_PORT")
	if strings.Contains(strings.Join(notes.Items(), "\n"), "come from") {
		t.Errorf("notes: %v", notes.Items())
	}
}

// A known key wins over the environment template; with several sources the
// first by name covers the variable.
func TestKnownKeysWinOverTemplateAndPickSourceByName(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":                            pom,
		"src/main/resources/application.yml": "a: ${ONE}\nb: ${TWO}\n",
		"env.properties":                     "ONE=from-template\nTWO=other\n",
	})
	prof := &Profile{EnvTemplateFiles: []string{"env.properties"}}
	s, err := ReadSourceWith(dir, &Notes{}, prof)
	if err != nil {
		t.Fatal(err)
	}
	s.KnownKeys = map[string][]string{"zeta": {"ONE"}, "alpha": {"ONE"}}
	var run Notes
	app := s.App("x:1", "", &run)
	if _, ok := envValue(app, "ONE"); ok {
		t.Error("ONE is known, no variable expected")
	}
	if v, ok := envValue(app, "TWO"); !ok || v != "other" {
		t.Errorf("TWO = %q %v, want the template value", v, ok)
	}
	all := strings.Join(run.Items(), "\n")
	if !strings.Contains(all, "1 variable comes from alpha (ONE)") || strings.Contains(all, "zeta") {
		t.Errorf("notes:\n%s", all)
	}
}

func TestEnvFromConfigMaps(t *testing.T) {
	prof := &Profile{
		Vars: map[string]string{"prefix": "acme-"},
		EnvFrom: []ProfileEnvFrom{
			{SecretRef: "{{ .Vars.prefix }}secrets"},
			{ConfigMapRef: "{{ .Vars.prefix }}system"},
			{ConfigMapRef: "{{ .Vars.prefix }}{{ .Name }}-config"},
			{ConfigMapRef: "acme-system"},
		},
	}
	got, err := prof.ConfigMapNames("orders")
	if err != nil || !reflect.DeepEqual(got, []string{"acme-system", "acme-orders-config"}) {
		t.Errorf("got %v err %v (config maps only, in order, no repeats)", got, err)
	}
	var none *Profile
	if got, err := none.ConfigMapNames("orders"); err != nil || got != nil {
		t.Errorf("nil profile: %v %v", got, err)
	}
}

// A name that envFrom gets must be a name of the API: a path in it would
// make dhg ask the cluster for another object.
func TestEnvFromNamesAreValidNames(t *testing.T) {
	prof := &Profile{
		Vars:    map[string]string{"prefix": "common/../../secrets/db-secret"},
		EnvFrom: []ProfileEnvFrom{{ConfigMapRef: "{{ .Vars.prefix }}"}},
	}
	if _, err := prof.ConfigMapNames("orders"); err == nil || !strings.Contains(err.Error(), "envFrom[0]") {
		t.Errorf("ConfigMapNames: err = %v, want a refusal of the rendered name", err)
	}
	if err := ApplyProfile(&App{Name: "orders"}, prof); err == nil || !strings.Contains(err.Error(), "envFrom[0]") {
		t.Errorf("ApplyProfile: err = %v, want a refusal of the rendered name", err)
	}
	if _, err := ParseProfile([]byte("envFrom:\n  - configMapRef: a/../b\n"), "p"); err == nil {
		t.Error("ParseProfile accepted a name with a path")
	}
	if _, err := ParseProfile([]byte("envFrom:\n  - secretRef: Not_Valid\n"), "p"); err == nil {
		t.Error("ParseProfile accepted an invalid Secret name")
	}
	if _, err := ParseProfile([]byte("envFrom:\n  - configMapRef: \"{{ .Vars.prefix }}cfg\"\nvars: {prefix: acme-}\n"), "p"); err != nil {
		t.Errorf("a template name must pass: %v", err)
	}
}
