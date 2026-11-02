package synth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const ordersFixture = "testdata/orders"

func parentPomOf(modules ...string) string {
	var b strings.Builder
	b.WriteString("<project><artifactId>parent</artifactId><version>1</version><packaging>pom</packaging><modules>")
	for _, m := range modules {
		b.WriteString("<module>" + m + "</module>")
	}
	b.WriteString("</modules></project>")
	return b.String()
}

func readOrders(t *testing.T, dir string, prof *Profile) (*SourceProject, *Notes) {
	t.Helper()
	var notes Notes
	s, err := ReadSourceWith(dir, &notes, prof)
	if err != nil {
		t.Fatal(err)
	}
	return s, &notes
}

// The Dockerfile of a module may sit under docker/ at the build root with a
// name like Dockerfile-service.txt; the module's own Dockerfile does not
// say USER, EXPOSE or HEALTHCHECK.
func TestDockerfileDiscoveryFindsDockerDirectory(t *testing.T) {
	s, notes := readOrders(t, ordersFixture, nil)
	if filepath.Base(s.Dir) != "service" {
		t.Fatalf("dir = %s, want the service module", s.Dir)
	}
	if s.Dockerfile == nil || s.Dockerfile.User != "10001:10001" || len(s.Dockerfile.Expose) != 1 || s.Dockerfile.Expose[0] != "8080" {
		t.Fatalf("dockerfile = %+v: USER and EXPOSE must come from docker/Dockerfile-service.txt", s.Dockerfile)
	}
	app := s.App("orders:1", "", notes)
	if app.RunAsUser == nil || *app.RunAsUser != 10001 || *app.RunAsGroup != 10001 {
		t.Errorf("user = %v", app.RunAsUser)
	}
	all := strings.Join(notes.Items(), "\n")
	// The variant for another module disagrees on USER: reported, not used.
	for _, want := range []string{"Dockerfile-service.txt", "Dockerfile-worker.txt", "20002"} {
		if !strings.Contains(all, want) {
			t.Errorf("notes lack %q:\n%s", want, all)
		}
	}
}

func TestDockerfileDiscoveryFromTheModuleDirectory(t *testing.T) {
	// -f service: the build root is found through the parent pom.
	s, _ := readOrders(t, filepath.Join(ordersFixture, "service"), nil)
	if s.Dockerfile == nil || s.Dockerfile.User != "10001:10001" {
		t.Errorf("dockerfile = %+v", s.Dockerfile)
	}
}

func TestDockerfilePrefersTheModuleDockerfile(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":                            pom,
		"Dockerfile":                         "FROM x\nUSER 1000\nEXPOSE 9000\n",
		"docker/Dockerfile-other":            "FROM x\nUSER 2000\nEXPOSE 9001\n",
		"src/main/resources/application.yml": "server:\n  port: 9000\n",
	})
	s, notes := readOrders(t, dir, nil)
	if s.Dockerfile.User != "1000" || s.Dockerfile.Expose[0] != "9000" || len(s.Dockerfile.Expose) != 1 {
		t.Errorf("dockerfile = %+v: Dockerfile in the module wins", s.Dockerfile)
	}
	s.App("x:1", "", notes)
	if all := strings.Join(notes.Items(), "\n"); !strings.Contains(all, "Dockerfile-other") || !strings.Contains(all, "2000") {
		t.Errorf("the disagreeing variant is not reported:\n%s", all)
	}
}

func TestDockerfileGlobsFromProfile(t *testing.T) {
	files := map[string]string{
		"pom.xml":                            pom,
		"build/images/app.dockerfile":        "FROM x\nUSER 4242\n",
		"src/main/resources/application.yml": "server:\n  port: 8080\n",
	}
	dir := writeFiles(t, files)
	if s, _ := readOrders(t, dir, nil); s.Dockerfile != nil {
		t.Errorf("without the glob nothing is found: %+v", s.Dockerfile)
	}
	s, _ := readOrders(t, dir, &Profile{DockerfileGlobs: []string{"build/images/*.dockerfile"}})
	if s.Dockerfile == nil || s.Dockerfile.User != "4242" {
		t.Errorf("dockerfile = %+v", s.Dockerfile)
	}
}

func TestDockerfileGlobsStayInsideTheProject(t *testing.T) {
	dir := writeFiles(t, map[string]string{"pom.xml": pom})
	outside := filepath.Join(filepath.Dir(dir), "outside.dockerfile")
	if err := os.WriteFile(outside, []byte("FROM x\nUSER 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, _ := readOrders(t, dir, &Profile{DockerfileGlobs: []string{"../outside.dockerfile", outside}})
	if s.Dockerfile != nil {
		t.Errorf("a glob must not leave the project: %+v", s.Dockerfile)
	}
}

// *.properties under docker/ or deploy/ are defaults for ${KEY} without an
// inline default; credentials are never taken from them.
func TestEnvTemplateSuppliesPlaceholderDefaults(t *testing.T) {
	s, notes := readOrders(t, ordersFixture, nil)
	app := s.App("orders:1", "", notes)
	env, secret := envMap(app.Env), envMap(app.SecretEnv)
	want := map[string]string{
		"SERVICE_PORT": "8080", "KAFKA_HOST": "kafka.messaging", "KAFKA_PORT": "9092",
		"KEYCLOAK_URL": "http://keycloak:8080", "TOKEN_URL": "http://keycloak:8080/token", "LOGGING_LEVEL": "INFO",
		"DB_URL": "", "DB_LOGIN": "",
	}
	for k, v := range want {
		if got, ok := env[k]; !ok || got != v {
			t.Errorf("%s = %q, %v; want %q in the ConfigMap", k, got, ok, v)
		}
	}
	for _, k := range []string{"DB_PASSWORD", "KEYCLOAK_CLIENT_SECRET"} {
		if got, ok := secret[k]; !ok || got != "" {
			t.Errorf("%s = %q, %v: a credential from the template must not be copied", k, got, ok)
		}
	}
	if app.Ports[0].Port != 8080 {
		t.Errorf("ports = %+v: server.port = ${SERVICE_PORT} is 8080 by the template", app.Ports)
	}
	// SPRING_KAFKA_BOOTSTRAP_SERVERS is derived from placeholders the template resolves.
	if got := env["SPRING_KAFKA_BOOTSTRAP_SERVERS"]; got != "kafka.messaging:9092" {
		t.Errorf("bootstrap servers = %q", got)
	}
	all := strings.Join(notes.Items(), "\n")
	if strings.Contains(all, "example-only") {
		t.Errorf("a template value leaked into the notes:\n%s", all)
	}
	for _, want := range []string{"docker/env.properties", "required (DB_LOGIN, DB_PASSWORD, DB_URL, KEYCLOAK_CLIENT_SECRET)"} {
		if !strings.Contains(all, want) {
			t.Errorf("notes lack %q:\n%s", want, all)
		}
	}
}

func TestInlineDefaultBeatsTheEnvTemplate(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":                            pom,
		"deploy/app.properties":              "HOST=from-template\nOTHER=from-template\n",
		"src/main/resources/application.yml": "app:\n  a: ${HOST:inline}\n  b: ${OTHER}\n",
	})
	s, notes := readOrders(t, dir, nil)
	app := s.App("x:1", "", notes)
	env := envMap(app.Env)
	if _, ok := env["HOST"]; ok {
		t.Errorf("HOST has an inline default and needs no variable: %v", env)
	}
	if env["OTHER"] != "from-template" {
		t.Errorf("OTHER = %q", env["OTHER"])
	}
}

func TestEnvTemplateFilesFromProfile(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":                            pom,
		"ops/vars.txt":                       "HOST=ops-host\n",
		"src/main/resources/application.yml": "app:\n  a: ${HOST}\n",
	})
	s, notes := readOrders(t, dir, &Profile{EnvTemplateFiles: []string{"ops/vars.txt"}})
	if got := envMap(s.App("x:1", "", notes).Env)["HOST"]; got != "ops-host" {
		t.Errorf("HOST = %q", got)
	}
}

func TestEnvTemplateSecretPatternsFromProfile(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":                            pom,
		"docker/app.properties":              "CARD_PIN=1234\nDB_PASSWORD=plain\n",
		"src/main/resources/application.yml": "app:\n  a: ${CARD_PIN}\n  b: ${DB_PASSWORD}\n",
	})
	s, notes := readOrders(t, dir, &Profile{SecretKeyPatterns: []string{"_PIN$"}})
	app := s.App("x:1", "", notes)
	if envMap(app.SecretEnv)["CARD_PIN"] != "" || len(app.SecretEnv) != 1 {
		t.Errorf("secret = %+v", app.SecretEnv)
	}
	if envMap(app.Env)["DB_PASSWORD"] != "plain" {
		t.Errorf("with its own patterns the profile decides: env = %+v", app.Env)
	}
}

func TestMigrationsInSiblingModule(t *testing.T) {
	s, notes := readOrders(t, ordersFixture, nil)
	app := s.App("orders:1", "", notes)
	if len(app.Migrations) != 1 || app.Migrations[0] != "liquibase" {
		t.Errorf("migrations = %v", app.Migrations)
	}
	all := strings.Join(notes.Items(), "\n")
	if !strings.Contains(all, "Liquibase") || !strings.Contains(all, "database") {
		t.Errorf("notes lack the Liquibase module:\n%s", all)
	}
}

func TestMigrationsFlywayAndNone(t *testing.T) {
	flyway := writeFiles(t, map[string]string{
		"pom.xml":    parentPomOf("db", "app"),
		"db/pom.xml": "<project><artifactId>db</artifactId></project>",
		"db/src/main/resources/db/migration/V1__init.sql": "create table t (id int);",
		"app/pom.xml":    pom,
		"app/Dockerfile": "FROM x\n",
	})
	s, notes := readOrders(t, flyway, nil)
	app := s.App("x:1", "", notes)
	if len(app.Migrations) != 1 || app.Migrations[0] != "flyway" {
		t.Errorf("migrations = %v\n%s", app.Migrations, strings.Join(notes.Items(), "\n"))
	}

	plain := writeFiles(t, map[string]string{"pom.xml": pom, "src/main/resources/application.yml": "a: b\n"})
	s, notes = readOrders(t, plain, nil)
	if app := s.App("x:1", "", notes); len(app.Migrations) != 0 {
		t.Errorf("migrations = %v", app.Migrations)
	}
}

func TestDebugPortNote(t *testing.T) {
	s, notes := readOrders(t, ordersFixture, nil)
	s.App("orders:1", "", notes)
	all := strings.Join(notes.Items(), "\n")
	if !strings.Contains(all, "jdwp") || !strings.Contains(all, "5005") {
		t.Errorf("notes lack the debug port:\n%s", all)
	}

	dir := writeFiles(t, map[string]string{
		"Dockerfile": "FROM x\nENV JAVA_OPTS=\"-Xmx1g -agentlib:jdwp=transport=dt_socket,server=y,address=8000\"\nEXPOSE 8080\n",
	})
	s, notes = readOrders(t, dir, nil)
	s.App("x:1", "", notes)
	if all := strings.Join(notes.Items(), "\n"); !strings.Contains(all, "8000") {
		t.Errorf("JAVA_OPTS in a Dockerfile: no debug note:\n%s", all)
	}

	// An agent without an address is no listening port.
	dir = writeFiles(t, map[string]string{"Dockerfile": "FROM x\nENV JAVA_OPTS=-agentlib:jdwp=transport=dt_socket\nEXPOSE 8080\n"})
	s, notes = readOrders(t, dir, nil)
	s.App("x:1", "", notes)
	if all := strings.Join(notes.Items(), "\n"); strings.Contains(all, "jdwp") {
		t.Errorf("no address, no note:\n%s", all)
	}
}
