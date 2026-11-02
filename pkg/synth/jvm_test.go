package synth

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestJVMFrameworksGetStartupProbe(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":                            pom,
		"src/main/resources/application.yml": "server:\n  port: 8081\n",
	})
	var notes Notes
	app, err := FromSource(dir, "orders:1", &notes)
	if err != nil {
		t.Fatal(err)
	}
	if app.Startup == nil || app.Liveness == nil {
		t.Fatalf("startup = %+v liveness = %+v", app.Startup, app.Liveness)
	}
	if app.Startup.HTTPPath != app.Liveness.HTTPPath || app.Startup.Port != 8081 ||
		app.Startup.PeriodSeconds != 10 || app.Startup.FailureThreshold != 30 {
		t.Errorf("startup probe = %+v, want the liveness path on 8081, period 10s, 30 failures (5 min)", app.Startup)
	}

	var container map[string]interface{}
	for _, o := range Manifests(app) {
		if o.GetKind() == "Deployment" {
			cs, _, _ := unstructured.NestedSlice(o.Object, "spec", "template", "spec", "containers")
			container = cs[0].(map[string]interface{})
		}
	}
	sp, ok := container["startupProbe"].(map[string]interface{})
	if !ok {
		t.Fatalf("container has no startupProbe: %v", container)
	}
	if sp["failureThreshold"] != int64(30) || sp["periodSeconds"] != int64(10) {
		t.Errorf("startupProbe = %v", sp)
	}
}

func TestQuarkusMicronautStartupProbe(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"quarkus": {"pom.xml": quarkusPom},
		"micronaut": {
			"build.gradle.kts": `plugins { id("io.micronaut.application") }
dependencies { implementation("io.micronaut:micronaut-management") }`,
		},
	} {
		var notes Notes
		app, err := FromSource(writeFiles(t, files), "x:1", &notes)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if app.Startup == nil || app.Startup.FailureThreshold != 30 || app.Startup.HTTPPath != app.Liveness.HTTPPath {
			t.Errorf("%s: startup = %+v", name, app.Startup)
		}
	}
}

func TestNoStartupProbeWithoutHealth(t *testing.T) {
	dir := writeFiles(t, map[string]string{"pom.xml": strings.Replace(pom, "spring-boot-starter-actuator", "spring-boot-starter-data", 1)})
	var notes Notes
	app, err := FromSource(dir, "x:1", &notes)
	if err != nil {
		t.Fatal(err)
	}
	if app.Startup != nil {
		t.Errorf("startup probe without a health endpoint: %+v", app.Startup)
	}
}

// A value with a placeholder that only the runtime resolves must not be
// copied half resolved: its placeholders become the variables, listed empty
// in one note.
func TestUnresolvedPlaceholdersAreNotCopiedPartially(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml": pom,
		"src/main/resources/application.yml": `
spring:
  datasource:
    url: jdbc:postgresql://${DB_HOST}:${DB_PORT:5432}/${DB_NAME}
    username: ${DB_LOGIN:orders}
  kafka:
    bootstrap-servers: ${KAFKA_HOST}:{KAFKA_PORT}
`,
	})
	var notes Notes
	app, err := FromSource(dir, "x:1", &notes)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"SPRING_DATASOURCE_URL", "SPRING_KAFKA_BOOTSTRAP_SERVERS"} {
		if v, ok := envValue(app, name); ok {
			t.Errorf("%s = %q: a partially resolved value must not be emitted", name, v)
		}
	}
	for _, name := range []string{"DB_HOST", "DB_NAME", "KAFKA_HOST"} {
		if v, ok := envValue(app, name); !ok || v != "" {
			t.Errorf("%s = %q, %v: the placeholder must be an empty variable", name, v, ok)
		}
	}
	if v, _ := envValue(app, "SPRING_DATASOURCE_USERNAME"); v != "orders" {
		t.Errorf("a fully resolved value must be kept, got %q", v)
	}
	var aggregated []string
	for _, n := range notes.Items() {
		if strings.Contains(n, "required") {
			aggregated = append(aggregated, n)
		}
	}
	if len(aggregated) != 1 || !strings.Contains(aggregated[0], "3 environment variables") ||
		!strings.Contains(aggregated[0], "DB_HOST, DB_NAME, KAFKA_HOST") {
		t.Errorf("want one aggregated note, got %q\nall notes:\n%s", aggregated, strings.Join(notes.Items(), "\n"))
	}
}

// Placeholders the chart does not map to a derived variable are not dropped.
func TestUnmappedPlaceholdersAreVariables(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml": pom,
		"src/main/resources/application.yml": `
app:
  one: ${ONE_URL}
  two: http://${TWO_HOST}/x
  three: ${THREE:ok}
spring:
  datasource:
    url: jdbc:postgresql://${DB_HOST}/db
`,
	})
	var notes Notes
	app, err := FromSource(dir, "x:1", &notes)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ONE_URL", "TWO_HOST", "DB_HOST"} {
		if v, ok := envValue(app, name); !ok || v != "" {
			t.Errorf("%s = %q, %v", name, v, ok)
		}
	}
	all := strings.Join(notes.Items(), "\n")
	if !strings.Contains(all, "3 environment variables are empty and required (DB_HOST, ONE_URL, TWO_HOST)") {
		t.Errorf("notes lack the required placeholders:\n%s", all)
	}
}
