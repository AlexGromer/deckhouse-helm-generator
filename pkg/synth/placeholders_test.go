package synth

import (
	"strings"
	"testing"
)

func envMap(vars []EnvVar) map[string]string {
	m := map[string]string{}
	for _, e := range vars {
		m[e.Name] = e.Value
	}
	return m
}

// Every placeholder without a default becomes a variable with an empty
// value, a Secret key when its name says so, and one aggregated note lists
// them. The variable derived from a property that needs them is not emitted
// (an empty SPRING_DATASOURCE_URL would override the property).
func TestRemainingPlaceholdersBecomeEmptyVariables(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml": pom,
		"src/main/resources/application.yml": `
app:
  one: ${ONE_URL}
  two: http://${TWO_HOST}/x
  three: ${THREE:ok}
  key: ${SERVICE_API_KEY}
spring:
  datasource:
    url: jdbc:postgresql://${DB_HOST}:${DB_PORT:5432}/orders
    username: ${DB_LOGIN:orders}
    password: ${DB_PASSWORD}
`,
	})
	var notes Notes
	app, err := FromSource(dir, "x:1", &notes)
	if err != nil {
		t.Fatal(err)
	}
	env, secret := envMap(app.Env), envMap(app.SecretEnv)
	for _, name := range []string{"ONE_URL", "TWO_HOST", "DB_HOST"} {
		if v, ok := env[name]; !ok || v != "" {
			t.Errorf("%s = %q, %v: want an empty variable in the ConfigMap", name, v, ok)
		}
	}
	for _, name := range []string{"SERVICE_API_KEY", "DB_PASSWORD"} {
		if v, ok := secret[name]; !ok || v != "" {
			t.Errorf("%s = %q, %v: want an empty Secret key", name, v, ok)
		}
	}
	for _, name := range []string{"THREE", "DB_PORT", "DB_LOGIN"} {
		if _, ok := env[name]; ok {
			t.Errorf("%s has a default in the configuration and needs no variable", name)
		}
	}
	for _, name := range []string{"SPRING_DATASOURCE_URL", "SPRING_DATASOURCE_PASSWORD"} {
		if _, ok := env[name]; ok {
			t.Errorf("%s depends on placeholders and must not be emitted half resolved", name)
		}
		if _, ok := secret[name]; ok {
			t.Errorf("%s depends on placeholders and must not be emitted", name)
		}
	}
	if env["SPRING_DATASOURCE_USERNAME"] != "orders" {
		t.Errorf("a fully resolved value must be kept: %v", env)
	}
	var aggregated []string
	for _, n := range notes.Items() {
		if strings.Contains(n, "required") {
			aggregated = append(aggregated, n)
		}
	}
	if len(aggregated) != 1 {
		t.Fatalf("want one aggregated note, got %q", aggregated)
	}
	for _, want := range []string{"5 environment variables", "DB_HOST", "DB_PASSWORD", "ONE_URL", "SERVICE_API_KEY", "TWO_HOST"} {
		if !strings.Contains(aggregated[0], want) {
			t.Errorf("note lacks %q: %s", want, aggregated[0])
		}
	}
}

func TestPlaceholderTypoIsReported(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":                            pom,
		"src/main/resources/application.yml": "spring:\n  kafka:\n    bootstrap-servers: ${KAFKA_HOST}:{KAFKA_PORT}\n",
	})
	var notes Notes
	app, err := FromSource(dir, "x:1", &notes)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := envMap(app.Env)["SPRING_KAFKA_BOOTSTRAP_SERVERS"]; ok {
		t.Error("a value with a typo must not be copied")
	}
	all := strings.Join(notes.Items(), "\n")
	if !strings.Contains(all, "{KAFKA_PORT}") || !strings.Contains(all, "without $") {
		t.Errorf("the typo is not reported:\n%s", all)
	}
}

func TestResolvePlaceholdersWithFallback(t *testing.T) {
	out, missing := ResolvePlaceholdersWith("${A}:${B:2}:${C}", map[string]string{"A": "1", "B": "ignored"})
	if out != "1:2:" || missing != "C" {
		t.Errorf("out = %q missing = %q", out, missing)
	}
	if got := Placeholders("${A}/${B:x}/${C}"); strings.Join(got, ",") != "A,C" {
		t.Errorf("Placeholders = %v", got)
	}
}
