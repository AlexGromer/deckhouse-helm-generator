package synth

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDockerfile(t *testing.T) {
	df := ParseDockerfile(`
FROM maven:3 AS build
EXPOSE 9999
USER root

# final stage
FROM eclipse-temurin:21-jre
ARG HTTP_PORT=8080
ENV GRPC_PORT=9090 \
    LOG=info
EXPOSE $HTTP_PORT ${GRPC_PORT}/tcp ${METRICS_PORT} ${ADMIN_PORT:-8558}
USER 10001:10001
VOLUME ["/data", "/logs"]
HEALTHCHECK --interval=15s --timeout=3s --start-period=20s --retries=4 \
  CMD ["wget", "-qO-", "http://localhost:8080/health"]
`)
	if strings.Join(df.Expose, " ") != "8080 9090/tcp 8558" || len(df.Unresolved) != 1 {
		t.Errorf("expose = %v unresolved = %v", df.Expose, df.Unresolved)
	}
	if df.User != "10001:10001" || strings.Join(df.Volumes, " ") != "/data /logs" {
		t.Errorf("user = %q volumes = %v", df.User, df.Volumes)
	}
	if strings.Join(df.Healthcheck, " ") != "CMD wget -qO- http://localhost:8080/health" ||
		df.Interval != 15e9 || df.Timeout != 3e9 || df.StartPeriod != 20e9 || df.Retries != 4 {
		t.Errorf("healthcheck = %+v", df)
	}

	shell := ParseDockerfile("FROM x\nHEALTHCHECK CMD curl -f http://localhost/ || exit 1\nENV\n")
	if strings.Join(shell.Healthcheck, "|") != "CMD-SHELL|curl -f http://localhost/ || exit 1" {
		t.Errorf("shell healthcheck = %q", shell.Healthcheck)
	}
	if none := ParseDockerfile("FROM x\nHEALTHCHECK NONE\n"); healthcheckProbe(none.Healthcheck, 0, 0, 0, 0) != nil {
		t.Error("HEALTHCHECK NONE must give no probe")
	}
}

const pom = `<?xml version="1.0"?>
<project>
  <parent><groupId>org.springframework.boot</groupId><artifactId>spring-boot-starter-parent</artifactId><version>3.3.4</version></parent>
  <artifactId>orders-service</artifactId>
  <version>2.1.0</version>
  <dependencies>
    <dependency><groupId>org.springframework.boot</groupId><artifactId>spring-boot-starter-web</artifactId></dependency>
    <dependency><groupId>org.springframework.boot</groupId><artifactId>spring-boot-starter-actuator</artifactId></dependency>
  </dependencies>
</project>`

func TestFromSourceSpringMaven(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":    pom,
		"Dockerfile": "FROM eclipse-temurin:21-jre\nUSER 10001\nEXPOSE 8080\nCOPY target/app.jar /app.jar\n",
		"src/main/resources/application.yml": `
spring:
  application:
    name: orders
  datasource:
    url: jdbc:postgresql://${DB_HOST:postgres}:5432/orders
    username: orders
    password: ${DB_PASSWORD}
  kafka:
    bootstrap-servers: kafka:9092
  security:
    oauth2:
      resourceserver:
        jwt:
          issuer-uri: https://keycloak.example.com/realms/shop
server:
  port: ${PORT:8081}
  servlet:
    context-path: /api
---
spring:
  config:
    activate:
      on-profile: dev
server:
  port: 9999
`,
		"src/main/resources/application.properties": "management.endpoints.web.base-path=/manage\n",
	})
	var notes Notes
	app, err := FromSource(dir, "registry.example.com/orders:2.1.0", &notes)
	if err != nil {
		t.Fatal(err)
	}
	if app.Name != "orders" || app.Image != "registry.example.com/orders:2.1.0" {
		t.Errorf("name = %s image = %s", app.Name, app.Image)
	}
	var ports []int64
	for _, p := range app.Ports {
		ports = append(ports, p.Port)
	}
	if len(ports) != 2 || ports[0] != 8080 || ports[1] != 8081 {
		t.Errorf("ports = %v (Dockerfile 8080, server.port default 8081; the dev profile must be ignored)", ports)
	}
	if app.Liveness == nil || app.Liveness.HTTPPath != "/api/manage/health/liveness" || app.Liveness.Port != 8081 ||
		app.Readiness.HTTPPath != "/api/manage/health/readiness" {
		t.Errorf("probes = %+v / %+v", app.Liveness, app.Readiness)
	}
	if app.RunAsUser == nil || *app.RunAsUser != 10001 {
		t.Error("Dockerfile USER not applied")
	}
	env := map[string]string{}
	for _, e := range app.Env {
		env[e.Name] = e.Value
	}
	if env["SPRING_DATASOURCE_URL"] != "jdbc:postgresql://postgres:5432/orders" || env["SPRING_DATASOURCE_USERNAME"] != "orders" ||
		env["SPRING_KAFKA_BOOTSTRAP_SERVERS"] != "kafka:9092" || !strings.Contains(env["SPRING_SECURITY_OAUTH2_RESOURCESERVER_JWT_ISSUER_URI"], "realms/shop") {
		t.Errorf("env = %v", env)
	}
	if len(app.SecretEnv) != 1 || app.SecretEnv[0].Name != "SPRING_DATASOURCE_PASSWORD" || app.SecretEnv[0].Value != "" {
		t.Errorf("secret env = %v (the password must never be copied)", app.SecretEnv)
	}
	all := strings.Join(notes.Items(), "\n")
	for _, want := range []string{"SPRING_DATASOURCE_PASSWORD is empty", "Keycloak"} {
		if !strings.Contains(all, want) {
			t.Errorf("notes lack %q:\n%s", want, all)
		}
	}
}

func TestFromSourceGradleAndManagementPort(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"build.gradle.kts":                          "plugins { id(\"org.springframework.boot\") version \"3.3.4\" }\nversion = \"0.4.0\"\ndependencies { implementation(\"org.springframework.boot:spring-boot-starter-actuator\") }\n",
		"settings.gradle.kts":                       "rootProject.name = \"billing\"\n",
		"src/main/resources/application.properties": "server.servlet.context-path=/billing\nmanagement.server.port=8081\n",
	})
	var notes Notes
	app, err := FromSource(dir, "", &notes)
	if err != nil {
		t.Fatal(err)
	}
	if app.Name != "billing" || app.Image != "billing:0.4.0" {
		t.Errorf("name = %s image = %s", app.Name, app.Image)
	}
	// On a separate management port the context path does not apply.
	if app.Liveness.HTTPPath != "/actuator/health/liveness" || app.Liveness.Port != 8081 {
		t.Errorf("liveness = %+v", app.Liveness)
	}
	all := strings.Join(notes.Items(), "\n")
	for _, want := range []string{"no --image given", "no Dockerfile"} {
		if !strings.Contains(all, want) {
			t.Errorf("notes lack %q", want)
		}
	}
}

func TestFromSourceErrorsAndPlainDockerfile(t *testing.T) {
	var notes Notes
	if _, err := FromSource(t.TempDir(), "", &notes); err == nil {
		t.Error("an empty directory must fail")
	}
	dir := writeFiles(t, map[string]string{"Dockerfile": "FROM nginx\nEXPOSE 80\nUSER nginx\n"})
	app, err := FromSource(dir, "web:1", &notes)
	if err != nil || len(app.Ports) != 1 || app.RunAsUser != nil || app.Name != DNSName(filepath.Base(dir)) {
		t.Errorf("plain Dockerfile: %+v %v", app, err)
	}
	if !strings.Contains(strings.Join(notes.Items(), "\n"), "named user \"nginx\"") {
		t.Error("named user not reported")
	}
}

func TestSpringWithoutActuator(t *testing.T) {
	dir := writeFiles(t, map[string]string{"pom.xml": strings.Replace(pom, "spring-boot-starter-actuator", "spring-boot-starter-jdbc", 1)})
	var notes Notes
	app, err := FromSource(dir, "x:1", &notes)
	if err != nil || app.Liveness != nil || app.Name != "orders-service" || app.Ports[0].Port != 8080 {
		t.Errorf("app = %+v err = %v", app, err)
	}
	if !strings.Contains(strings.Join(notes.Items(), "\n"), "without spring-boot-starter-actuator") {
		t.Error("missing actuator not reported")
	}
}

func TestFromSourceDockerfileAndSpringDetails(t *testing.T) {
	parentOnly := strings.Replace(pom, "<version>2.1.0</version>", "", 1)
	dir := writeFiles(t, map[string]string{
		"pom.xml":    parentOnly,
		"Dockerfile": "FROM eclipse-temurin:21\nEXPOSE ${APP_PORT}\nVOLUME /cache\nHEALTHCHECK --interval=15s CMD [\"/health\"]\n",
		"src/main/resources/application.yml": "server:\n  servlet:\n    context-path: /\nmanagement:\n  endpoints:\n    web:\n      base-path: /\napp:\n  hosts: [a, b]\n" +
			"spring:\n  datasource:\n    url: jdbc:postgresql://${DB_HOST}/orders\n---\nspring:\n  config:\n    activate:\n      on-profile: dev\nserver:\n  port: 9999\n",
		"src/main/resources/application.properties": "# comment\n! also a comment\nno-separator\nspring.application.name=${NAME}\n",
	})
	var notes Notes
	app, err := FromSource(dir, "orders:1", &notes)
	if err != nil {
		t.Fatal(err)
	}
	if app.Ports[0].Port != 8080 {
		t.Errorf("the dev profile must not change the port: %+v", app.Ports)
	}
	// Spring's actuator probes win over the Dockerfile HEALTHCHECK; "/" paths collapse.
	if app.Liveness.HTTPPath != "/health/liveness" || len(app.Volumes) != 1 || app.Volumes[0].MountPath != "/cache" {
		t.Errorf("liveness = %+v volumes = %+v", app.Liveness, app.Volumes)
	}
	all := strings.Join(notes.Items(), "\n")
	for _, want := range []string{"EXPOSE ${APP_PORT}", "volume /cache", "needs DB_HOST at runtime"} {
		if !strings.Contains(all, want) {
			t.Errorf("notes lack %q:\n%s", want, all)
		}
	}

	spring, err := DetectProject(dir, &Notes{})
	if err != nil || spring.Version != "3.3.4" || spring.Name != "orders-service" || spring.Properties["app.hosts"] != "a,b" {
		t.Errorf("spring = %+v err = %v (parent version, unresolved name, flattened list)", spring, err)
	}
}

func TestFromSourceHealthcheckOnly(t *testing.T) {
	dir := writeFiles(t, map[string]string{"Dockerfile": "FROM alpine\nHEALTHCHECK CMD wget -q localhost\n"})
	var notes Notes
	app, err := FromSource(dir, "", &notes)
	if err != nil || app.Liveness == nil || app.Readiness == nil || !strings.HasSuffix(app.Image, ":latest") {
		t.Fatalf("app = %+v err = %v", app, err)
	}
	if !strings.Contains(strings.Join(notes.Items(), "\n"), "no port found") {
		t.Error("missing port not reported")
	}
}

func TestDetectProjectErrors(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"bad pom":  {"pom.xml": "<project>spring-boot<unclosed"},
		"bad yaml": {"pom.xml": pom, "src/main/resources/application.yaml": "server: [\n"},
	} {
		if _, err := DetectProject(writeFiles(t, files), &Notes{}); err == nil {
			t.Errorf("%s: expected an error", name)
		}
		if _, err := FromSource(writeFiles(t, files), "x:1", &Notes{}); err == nil {
			t.Errorf("%s: FromSource must fail too", name)
		}
	}
	dir := writeFiles(t, map[string]string{"build.gradle": "plugins { id 'org.springframework.boot' }\nversion = '${revision}'\n"})
	p, err := DetectProject(dir, &Notes{})
	if err != nil || p.Version != "" || p.Name != filepath.Base(dir) {
		t.Errorf("gradle without settings: %+v %v", p, err)
	}
}
