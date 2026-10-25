package synth

import (
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/synth/registry"
)

const quarkusPom = `<project>
  <artifactId>payments</artifactId>
  <version>1.0.0</version>
  <dependencies>
    <dependency><groupId>io.quarkus</groupId><artifactId>quarkus-rest</artifactId></dependency>
    <dependency><groupId>io.quarkus</groupId><artifactId>quarkus-smallrye-health</artifactId></dependency>
    <dependency><groupId>io.quarkus</groupId><artifactId>quarkus-spring-boot-properties</artifactId></dependency>
  </dependencies>
</project>`

func envValue(app App, name string) (string, bool) {
	for _, e := range append(append([]EnvVar(nil), app.Env...), app.SecretEnv...) {
		if e.Name == name {
			return e.Value, true
		}
	}
	return "", false
}

func TestQuarkus(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml": quarkusPom,
		"src/main/resources/application.properties": "quarkus.http.port=8081\n" +
			"quarkus.http.root-path=/api\n" +
			"quarkus.datasource.jdbc.url=jdbc:postgresql://db:5432/pay\n" +
			"quarkus.datasource.password=secret\n" +
			"%prod.kafka.bootstrap.servers=kafka:9092\n" +
			"%dev.quarkus.http.port=9999\n" +
			"%staging,qa.quarkus.datasource.jdbc.url=jdbc:postgresql://staging-db/pay\n" +
			"quarkus.oidc.auth-server-url=https://sso/realms/bank\n",
		// application.yaml (ordinal 255) wins over application.properties (250).
		"src/main/resources/application.yaml": "quarkus:\n  application:\n    name: payments-api\n  datasource:\n    username: pay\n",
	})
	var notes Notes
	s, err := ReadSource(dir, &notes)
	if err != nil {
		t.Fatal(err)
	}
	if s.Project.Framework != Quarkus || !s.Project.Health {
		t.Fatalf("project = %+v", s.Project)
	}
	if strings.Join(s.Profiles(), ",") != "qa,staging" {
		t.Errorf("profiles = %v (prod is the container default; dev and test are skipped)", s.Profiles())
	}
	app := s.App("pay:1", "", &notes)
	if app.Name != "payments-api" || app.Ports[0].Port != 8081 || app.Ports[0].Name != "http" {
		t.Errorf("name = %s ports = %+v", app.Name, app.Ports)
	}
	// Relative non-application root "q" nests under the HTTP root path.
	if app.Liveness.HTTPPath != "/api/q/health/live" || app.Readiness.HTTPPath != "/api/q/health/ready" || app.Liveness.Port != 8081 {
		t.Errorf("probes = %+v %+v", app.Liveness, app.Readiness)
	}
	for name, want := range map[string]string{
		"QUARKUS_DATASOURCE_JDBC_URL":  "jdbc:postgresql://db:5432/pay",
		"QUARKUS_DATASOURCE_USERNAME":  "pay",
		"QUARKUS_DATASOURCE_PASSWORD":  "",
		"KAFKA_BOOTSTRAP_SERVERS":      "kafka:9092",
		"QUARKUS_OIDC_AUTH_SERVER_URL": "https://sso/realms/bank",
	} {
		if v, ok := envValue(app, name); !ok || v != want {
			t.Errorf("%s = %q (%v), want %q", name, v, ok, want)
		}
	}
	all := strings.Join(notes.Items(), "\n")
	for _, want := range []string{"%dev and %test", "QUARKUS_DATASOURCE_PASSWORD is empty", "OAuth2 issuer"} {
		if !strings.Contains(all, want) {
			t.Errorf("notes lack %q:\n%s", want, all)
		}
	}

	staging := s.App("pay:1", "staging", &Notes{})
	if v, _ := envValue(staging, "QUARKUS_DATASOURCE_JDBC_URL"); v != "jdbc:postgresql://staging-db/pay" {
		t.Errorf("staging url = %q", v)
	}
	if v, _ := envValue(staging, "QUARKUS_PROFILE"); v != "staging" {
		t.Errorf("QUARKUS_PROFILE = %q", v)
	}
}

func TestQuarkusPaths(t *testing.T) {
	for _, tt := range []struct {
		name, props, live string
		port              int64
	}{
		{"defaults", "", "/q/health/live", 8080},
		{"absolute non-application root", "quarkus.http.root-path=/api\nquarkus.http.non-application-root-path=/ops\n", "/ops/health/live", 8080},
		{"custom health paths", "quarkus.smallrye-health.root-path=hc\nquarkus.smallrye-health.liveness-path=/alive\n", "/alive", 8080},
		{"management interface", "quarkus.management.enabled=true\nquarkus.http.root-path=/api\n", "/q/health/live", 9000},
		{"management port and root", "quarkus.management.enabled=true\nquarkus.management.port=9100\nquarkus.management.root-path=mgmt\n", "/mgmt/health/live", 9100},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeFiles(t, map[string]string{"pom.xml": quarkusPom, "src/main/resources/application.properties": tt.props})
			app, err := FromSource(dir, "x:1", &Notes{})
			if err != nil {
				t.Fatal(err)
			}
			if app.Liveness.HTTPPath != tt.live || app.Liveness.Port != tt.port {
				t.Errorf("liveness = %+v, want %s on %d", app.Liveness, tt.live, tt.port)
			}
		})
	}
}

func TestQuarkusWithoutHealth(t *testing.T) {
	pomNoHealth := strings.Replace(quarkusPom, "quarkus-smallrye-health", "quarkus-arc", 1)
	var notes Notes
	app, err := FromSource(writeFiles(t, map[string]string{"pom.xml": pomNoHealth}), "x:1", &notes)
	if err != nil || app.Liveness != nil {
		t.Fatalf("app = %+v err = %v", app, err)
	}
	if !strings.Contains(strings.Join(notes.Items(), "\n"), "without quarkus-smallrye-health") {
		t.Error("missing health extension not reported")
	}
}

const micronautGradle = `plugins { id("io.micronaut.application") version "4.4.0" }
version = "0.3.0"
dependencies {
    implementation("io.micronaut:micronaut-management")
}
`

func TestMicronaut(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"build.gradle.kts":    micronautGradle,
		"settings.gradle.kts": `rootProject.name = "ledger"`,
		"src/main/resources/application.yml": "micronaut:\n  server:\n    port: 8090\n    context-path: /ledger\n" +
			"  security:\n    oauth2:\n      clients:\n        keycloak:\n          client-secret: s3cr3t\n          openid:\n            issuer: https://sso/realms/bank\n" +
			"datasources:\n  default:\n    url: jdbc:postgresql://db/ledger\n    password: x\nendpoints:\n  all:\n    path: /mgmt\n",
		"src/main/resources/application.properties": "micronaut.server.port=8091\nkafka.bootstrap.servers=kafka:9092\n",
		"src/main/resources/application-k8s.yml":    "datasources:\n  default:\n    url: jdbc:postgresql://pg.data/ledger\n",
	})
	var notes Notes
	s, err := ReadSource(dir, &notes)
	if err != nil {
		t.Fatal(err)
	}
	app := s.App("", "", &notes)
	if s.Project.Framework != Micronaut || app.Name != "ledger" || app.Image != "ledger:0.3.0" {
		t.Errorf("framework = %s name = %s image = %s", s.Project.Framework, app.Name, app.Image)
	}
	if app.Ports[0].Port != 8091 || app.Liveness.HTTPPath != "/ledger/mgmt/health/liveness" || app.Readiness.HTTPPath != "/ledger/mgmt/health/readiness" {
		t.Errorf("ports = %+v liveness = %+v", app.Ports, app.Liveness)
	}
	for name, want := range map[string]string{
		"DATASOURCES_DEFAULT_URL":                                  "jdbc:postgresql://db/ledger",
		"DATASOURCES_DEFAULT_PASSWORD":                             "",
		"KAFKA_BOOTSTRAP_SERVERS":                                  "kafka:9092",
		"MICRONAUT_SECURITY_OAUTH2_CLIENTS_KEYCLOAK_OPENID_ISSUER": "https://sso/realms/bank",
		"MICRONAUT_SECURITY_OAUTH2_CLIENTS_KEYCLOAK_CLIENT_SECRET": "",
	} {
		if v, ok := envValue(app, name); !ok || v != want {
			t.Errorf("%s = %q (%v), want %q", name, v, ok, want)
		}
	}
	if !strings.Contains(strings.Join(notes.Items(), "\n"), `micronaut.server.port is "8090" in application.yml and "8091" in application.properties`) {
		t.Errorf("conflict between yml and properties not reported:\n%s", strings.Join(notes.Items(), "\n"))
	}
	k8s := s.App("", "k8s", &Notes{})
	if v, _ := envValue(k8s, "DATASOURCES_DEFAULT_URL"); v != "jdbc:postgresql://pg.data/ledger" {
		t.Errorf("k8s environment url = %q", v)
	}
	if v, _ := envValue(k8s, "MICRONAUT_ENVIRONMENTS"); v != "k8s" {
		t.Errorf("MICRONAUT_ENVIRONMENTS = %q", v)
	}
}

func TestMicronautManagementPortAndDisabled(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"build.gradle": micronautGradle,
		"src/main/resources/application.properties": "endpoints.all.port=8085\n",
	})
	var notes Notes
	app, err := FromSource(dir, "x:1", &notes)
	if err != nil || app.Liveness.Port != 8085 || app.Liveness.HTTPPath != "/health/liveness" || len(app.Ports) != 2 {
		t.Fatalf("app = %+v err = %v", app, err)
	}
	if !strings.Contains(strings.Join(notes.Items(), "\n"), "listen on port 8085") {
		t.Error("separate management port not reported")
	}

	off := writeFiles(t, map[string]string{"build.gradle": micronautGradle, "src/main/resources/application.properties": "endpoints.health.enabled=false\n"})
	notes = Notes{}
	if app, _ := FromSource(off, "x:1", &notes); app.Liveness != nil || !strings.Contains(strings.Join(notes.Items(), "\n"), "health endpoint is disabled") {
		t.Errorf("disabled health: %+v %v", app.Liveness, notes.Items())
	}

	plain := writeFiles(t, map[string]string{"build.gradle": strings.Replace(micronautGradle, "micronaut-management", "micronaut-http-server", 1)})
	notes = Notes{}
	if app, _ := FromSource(plain, "x:1", &notes); app.Liveness != nil || !strings.Contains(strings.Join(notes.Items(), "\n"), "without micronaut-management") {
		t.Errorf("no management: %+v", app.Liveness)
	}
}

func TestSpringProfiles(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml": pom,
		"src/main/resources/application.yml": "spring:\n  datasource:\n    url: jdbc:postgresql://localhost/orders\n" +
			"---\nspring:\n  config:\n    activate:\n      on-profile: prod\n  datasource:\n    url: jdbc:postgresql://pg-prod/orders\n" +
			"---\nspring:\n  profiles: legacy,old\nserver:\n  port: 9000\n" +
			"---\nspring:\n  config:\n    activate:\n      on-profile: \"prod & eu\"\nserver:\n  port: 7000\n",
		"src/main/resources/application.properties":         "server.port=8080\n#---\nspring.config.activate.on-profile=test\nserver.port=1234\n",
		"src/main/resources/application-staging.properties": "spring.kafka.bootstrap-servers=kafka-staging:9092\n",
	})
	var notes Notes
	s, err := ReadSource(dir, &notes)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(s.Profiles(), ",") != "legacy,old,prod,staging,test" {
		t.Errorf("profiles = %v", s.Profiles())
	}
	if !strings.Contains(strings.Join(notes.Items(), "\n"), `profile expression "prod & eu"`) {
		t.Error("profile expression not reported")
	}
	base := s.App("o:1", "", &Notes{})
	if v, _ := envValue(base, "SPRING_DATASOURCE_URL"); v != "jdbc:postgresql://localhost/orders" || base.Ports[0].Port != 8080 {
		t.Errorf("base: url %q ports %+v", v, base.Ports)
	}
	prod := s.App("o:1", "prod", &Notes{})
	if v, _ := envValue(prod, "SPRING_DATASOURCE_URL"); v != "jdbc:postgresql://pg-prod/orders" {
		t.Errorf("prod url = %q", v)
	}
	if v, _ := envValue(prod, "SPRING_PROFILES_ACTIVE"); v != "prod" {
		t.Errorf("SPRING_PROFILES_ACTIVE = %q", v)
	}
	if legacy := s.App("o:1", "legacy", &Notes{}); legacy.Ports[0].Port != 9000 {
		t.Errorf("legacy port = %+v", legacy.Ports)
	}
	if staging := s.App("o:1", "staging", &Notes{}); staging.Env == nil {
		t.Error("staging env missing")
	} else if v, _ := envValue(staging, "SPRING_KAFKA_BOOTSTRAP_SERVERS"); v != "kafka-staging:9092" {
		t.Errorf("staging kafka = %q", v)
	}
}

func TestRandomPortAndImageConfig(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":    pom,
		"Dockerfile": "FROM eclipse-temurin:21\nEXPOSE 8080\n",
		"src/main/resources/application.properties": "server.port=0\n",
	})
	s, err := ReadSource(dir, &Notes{})
	if err != nil {
		t.Fatal(err)
	}
	// The base image's user and an extra port only show in the image config.
	s.ImageConfig = &registry.ImageConfig{Config: registry.ContainerConfig{
		User: "65532", ExposedPorts: map[string]struct{}{"8080/tcp": {}, "9404/tcp": {}},
	}}
	var notes Notes
	app := s.App("o:1", "", &notes)
	all := strings.Join(notes.Items(), "\n")
	if !strings.Contains(all, "server.port = 0 is not a fixed port") || !strings.Contains(all, "the chart follows the image") {
		t.Errorf("notes:\n%s", all)
	}
	if app.RunAsUser == nil || *app.RunAsUser != 65532 || len(app.Ports) != 2 {
		t.Errorf("user = %v ports = %+v", app.RunAsUser, app.Ports)
	}
}

func TestEnvNameAndResolvePath(t *testing.T) {
	if envName("spring.kafka.bootstrap-servers") != "SPRING_KAFKA_BOOTSTRAP_SERVERS" || envName("a.b2.C") != "A_B2_C" {
		t.Error("envName")
	}
	if resolvePath("/api", "q") != "/api/q" || resolvePath("/api", "/q/") != "/q" {
		t.Error("resolvePath")
	}
}
