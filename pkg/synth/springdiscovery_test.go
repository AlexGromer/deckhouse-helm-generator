package synth

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const codegenFixture = "testdata/orders-codegen"

func envNames(app App) []string {
	var out []string
	for _, e := range app.Env {
		out = append(out, e.Name)
	}
	for _, e := range app.SecretEnv {
		out = append(out, e.Name)
	}
	return out
}

func hasAll(t *testing.T, got []string, want ...string) {
	t.Helper()
	set := map[string]bool{}
	for _, g := range got {
		set[g] = true
	}
	for _, w := range want {
		if !set[w] {
			t.Errorf("%q missing in %v", w, got)
		}
	}
}

// A profile included by the default configuration lives in a generated
// resource directory that the pom adds with build-helper; the include inside
// it is followed too, and the placeholders of the whole chain become
// variables of the app.
func TestSpringIncludeChainAcrossResourceDirs(t *testing.T) {
	var notes Notes
	app, err := FromSource(codegenFixture, "orders:1", &notes)
	if err != nil {
		t.Fatal(err)
	}
	hasAll(t, envNames(app), "DB_URL", "KAFKA_HOST", "KAFKA_PORT", "KEYCLOAK_CLIENT_SECRET", "LOGGING_LEVEL")

	p, err := DetectProject(codegenFixture, &Notes{})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Properties["spring.kafka.bootstrap-servers"]; got != "${KAFKA_HOST}:${KAFKA_PORT}" {
		t.Errorf("bootstrap-servers = %q: the nested include was not followed", got)
	}
	want := []string{
		filepath.Join(codegenFixture, "src/main/resources"),
		filepath.Join(codegenFixture, "codegen/generated-resources/app"),
	}
	if !reflect.DeepEqual(p.ResourceDirs, want) {
		t.Errorf("ResourceDirs = %v, want %v (main resources first, no repeats)", p.ResourceDirs, want)
	}
}

// Without build-helper the conventional generated directories are found by
// glob: <module>/*/generated-resources and target/generated-resources.
func TestSpringGeneratedResourcesFoundByGlob(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":                            pom,
		"src/main/resources/application.yml": "spring:\n  profiles:\n    include: shared\n",
		"codegen/generated-resources/app/application-shared.yml":     "demo:\n  url: ${SHARED_URL}\n",
		"target/generated-resources/xx/application-other.properties": "demo.other=${OTHER}\n",
		"deep/a/b/c/d/generated-resources/application-x.yml":         "x: ${NOT_FOUND}\n",
	})
	p, err := DetectProject(dir, &Notes{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Properties["demo.url"] != "${SHARED_URL}" {
		t.Errorf("properties = %v", p.Properties)
	}
	if _, ok := p.Profiles["other"]; !ok {
		t.Errorf("profiles = %v: target/generated-resources was not read", p.ProfileNames())
	}
	if _, ok := p.Profiles["x"]; ok {
		t.Error("a directory deeper than the module's children was read")
	}
}

// <build><resources>, ${project.basedir}, ${basedir} and <properties>.
func TestSpringPomResourceDirectories(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml": `<project>
  <parent><groupId>org.springframework.boot</groupId><artifactId>spring-boot-starter-parent</artifactId><version>3.3.4</version></parent>
  <artifactId>svc</artifactId>
  <properties><cfg.dir>src/main/config</cfg.dir></properties>
  <build>
    <resources>
      <resource><directory>${cfg.dir}</directory></resource>
      <resource><directory>${project.basedir}/extra</directory></resource>
      <resource><directory>${basedir}/more</directory></resource>
      <resource><directory>${unknown.dir}/skipped</directory></resource>
    </resources>
  </build>
</project>`,
		"src/main/resources/application.yml": "a: 1\n",
		"src/main/config/application.yml":    "b: ${B}\n",
		"extra/application.properties":       "c=${C}\n",
		"more/application-dev.yml":           "d: ${D}\n",
	})
	p, err := DetectProject(dir, &Notes{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Properties["a"] != "1" || p.Properties["b"] != "${B}" || p.Properties["c"] != "${C}" {
		t.Errorf("properties = %v", p.Properties)
	}
	if p.Profiles["dev"]["d"] != "${D}" {
		t.Errorf("profiles = %v", p.Profiles)
	}
	for _, d := range p.ResourceDirs {
		if strings.Contains(d, "${") {
			t.Errorf("unresolved directory %q kept", d)
		}
	}
}

// Only the profiles the application activates by default are followed: dev
// and test are not activated on the app's behalf.
func TestSpringActiveAndIncludeOnlyDefaults(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml": pom,
		"src/main/resources/application.yml": `spring:
  profiles:
    active: cloud
    include: base
`,
		"src/main/resources/application-cloud.yml": "cloud.key: ${CLOUD}\n",
		"src/main/resources/application-base.yml":  "base.key: ${BASE}\n",
		"src/main/resources/application-dev.yml":   "dev.key: ${DEV}\n",
		"src/main/resources/application-test.yml":  "test.key: ${TEST}\n",
	})
	var notes Notes
	app, err := FromSource(dir, "x:1", &notes)
	if err != nil {
		t.Fatal(err)
	}
	names := envNames(app)
	hasAll(t, names, "CLOUD", "BASE")
	for _, n := range names {
		if n == "DEV" || n == "TEST" {
			t.Errorf("variable %s comes from a profile the app does not activate by default", n)
		}
	}
}

// A profile that includes another gets its properties in its own variant,
// not in the default configuration.
func TestSpringProfileIncludesInsideProfile(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":                                  pom,
		"src/main/resources/application.yml":       "a: 1\n",
		"src/main/resources/application-dev.yml":   "spring.profiles.include: kafka\nd: 1\n",
		"src/main/resources/application-kafka.yml": "spring.kafka.bootstrap-servers: ${K}\n",
	})
	p, err := DetectProject(dir, &Notes{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.Properties["spring.kafka.bootstrap-servers"]; ok {
		t.Error("kafka leaked into the default configuration")
	}
	if got := p.properties("dev")["spring.kafka.bootstrap-servers"]; got != "${K}" {
		t.Errorf("dev lacks the properties of its included profile: %q", got)
	}
}

// spring.config.import: optional:, classpath:, file:, nested and cyclic.
func TestSpringConfigImportChain(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml": pom,
		"src/main/resources/application.yml": `spring:
  config:
    import: "optional:classpath:imports/a.yml, optional:classpath:imports/absent.yml"
`,
		"src/main/resources/imports/a.yml": `spring:
  config:
    import: file:./extra/b.properties
a.key: ${A}
`,
		"extra/b.properties": "spring.config.import=classpath:application.yml\nb.key=${B}\n",
	})
	var notes Notes
	p, err := DetectProject(dir, &notes)
	if err != nil {
		t.Fatal(err)
	}
	if p.Properties["a.key"] != "${A}" || p.Properties["b.key"] != "${B}" {
		t.Errorf("properties = %v", p.Properties)
	}
	if all := strings.Join(notes.Items(), "\n"); strings.Contains(all, "absent.yml") {
		t.Errorf("an optional import that is absent must not be reported:\n%s", all)
	}
}

func TestSpringConfigImportMissingAndRuntime(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml": pom,
		"src/main/resources/application.yml": `spring:
  config:
    import: "classpath:gone.yml,configserver:http://cfg:8888"
`,
	})
	var notes Notes
	if _, err := DetectProject(dir, &notes); err != nil {
		t.Fatal(err)
	}
	all := strings.Join(notes.Items(), "\n")
	for _, want := range []string{"gone.yml", "configserver:http://cfg:8888"} {
		if !strings.Contains(all, want) {
			t.Errorf("notes lack %q:\n%s", want, all)
		}
	}
}

// An import that sits in a profile document or file belongs to that profile.
func TestSpringConfigImportInProfileFile(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":                               pom,
		"src/main/resources/application.yml":    "a: 1\n",
		"src/main/resources/application-ci.yml": "spring.config.import: classpath:ci-extra.yml\n",
		"src/main/resources/ci-extra.yml":       "ci.key: ${CI}\n",
	})
	p, err := DetectProject(dir, &Notes{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.Properties["ci.key"]; ok {
		t.Error("the import of a profile file leaked into the default configuration")
	}
	if p.Profiles["ci"]["ci.key"] != "${CI}" {
		t.Errorf("profiles = %v", p.Profiles)
	}
}

// A directory the build generates and the checkout lacks gets a note.
func TestSpringMissingGeneratedDirectoryNote(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml": `<project>
  <parent><groupId>org.springframework.boot</groupId><artifactId>spring-boot-starter-parent</artifactId><version>3.3.4</version></parent>
  <artifactId>svc</artifactId>
  <build><plugins>
    <plugin><artifactId>build-helper-maven-plugin</artifactId>
      <executions><execution><goals><goal>add-resource</goal></goals>
        <configuration><resources><resource><directory>${project.build.directory}/generated-resources/cfg</directory></resource></resources></configuration>
      </execution></executions>
    </plugin>
    <plugin><artifactId>config-codegen-maven-plugin</artifactId>
      <configuration><outputDirectory>${project.basedir}/gen/generated-resources/app</outputDirectory></configuration>
    </plugin>
    <plugin><artifactId>build-helper-maven-plugin</artifactId>
      <executions><execution><goals><goal>add-resource</goal></goals>
        <configuration><resources><resource><directory>src/missing-but-plain</directory></resource></resources></configuration>
      </execution></executions>
    </plugin>
  </plugins></build>
</project>`,
		"src/main/resources/application.yml": "a: 1\n",
	})
	var notes Notes
	if _, err := DetectProject(dir, &notes); err != nil {
		t.Fatal(err)
	}
	all := strings.Join(notes.Items(), "\n")
	for _, want := range []string{
		"target/generated-resources/cfg is generated by the build; run mvn generate-resources (or the build) before dhg to include its configuration",
		"gen/generated-resources/app is generated by the build",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("notes lack %q:\n%s", want, all)
		}
	}
	if strings.Contains(all, "missing-but-plain") {
		t.Errorf("a missing directory that is not generated must stay silent:\n%s", all)
	}

	// Once the directory exists there is nothing to note.
	dir2 := writeFiles(t, map[string]string{
		"pom.xml": `<project>
  <parent><groupId>org.springframework.boot</groupId><artifactId>spring-boot-starter-parent</artifactId><version>3.3.4</version></parent>
  <artifactId>svc</artifactId>
  <build><plugins><plugin><artifactId>config-codegen-maven-plugin</artifactId>
    <configuration><outputDirectory>${project.basedir}/gen/generated-resources/app</outputDirectory></configuration>
  </plugin></plugins></build>
</project>`,
		"gen/generated-resources/app/application.yml": "g: 1\n",
	})
	var notes2 Notes
	p, err := DetectProject(dir2, &notes2)
	if err != nil {
		t.Fatal(err)
	}
	if p.Properties["g"] != "1" || strings.Contains(strings.Join(notes2.Items(), "\n"), "generated by the build") {
		t.Errorf("props = %v notes = %v", p.Properties, notes2.Items())
	}
}

func TestSpringDiscoveryIsDeterministic(t *testing.T) {
	first, err := DetectProject(codegenFixture, &Notes{})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		again, err := DetectProject(codegenFixture, &Notes{})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(first.Properties, again.Properties) || !reflect.DeepEqual(first.ResourceDirs, again.ResourceDirs) ||
			!reflect.DeepEqual(first.Profiles, again.Profiles) {
			t.Fatalf("run %d differs", i)
		}
	}
}

// Quarkus and Micronaut read the same resource directories.
func TestQuarkusReadsPomResourceDirectories(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":                     strings.Replace(quarkusPom, "</project>", "<build><resources><resource><directory>conf</directory></resource></resources></build></project>", 1),
		"conf/application.properties": "quarkus.http.port=9090\n",
		"gen/generated-resources/application-x.properties": "x=1\n",
	})
	p, err := DetectProject(dir, &Notes{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Properties["quarkus.http.port"] != "9090" {
		t.Errorf("properties = %v", p.Properties)
	}
	if _, ok := p.Profiles["x"]; !ok {
		t.Errorf("profiles = %v", p.ProfileNames())
	}
}

// Configuration of a project must not make dhg read files outside it:
// spring.config.import with "../", an absolute file:, a symlink, and resource
// directories of the pom that leave the module.
func TestSpringConfigStaysInsideModule(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"app/pom.xml": strings.Replace(pom, "</project>", `<build><resources>
  <resource><directory>../outside-res</directory></resource>
  <resource><directory>${project.basedir}/../outside-res</directory></resource>
  <resource><directory>OUTSIDE_ABS</directory></resource>
</resources></build></project>`, 1),
		"app/src/main/resources/application.yml": "spring:\n  config:\n    import:\n" +
			"      - file:../outside.yml\n      - classpath:../../../../outside.yml\n      - ../../../../outside.yml\n" +
			"      - file:OUTSIDE_FILE\n      - link.yml\n      - inside.yml\n",
		"app/src/main/resources/inside.yml": "demo:\n  in: ${INSIDE_VAR}\n",
		"outside.yml":                       "demo:\n  leak: ${LEAK_URL}\n",
		"outside-res/application.yml":       "demo:\n  leak2: ${LEAK_RES}\n",
		"abs/application.yml":               "demo:\n  leak3: ${LEAK_ABS}\n",
	})
	abs := filepath.Join(root, "abs")
	outFile := filepath.Join(root, "outside.yml")
	for _, f := range []string{"app/pom.xml", "app/src/main/resources/application.yml"} {
		data, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatal(err)
		}
		s := strings.ReplaceAll(strings.ReplaceAll(string(data), "OUTSIDE_ABS", abs), "OUTSIDE_FILE", outFile)
		if err := os.WriteFile(filepath.Join(root, f), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outFile, filepath.Join(root, "app/src/main/resources/link.yml")); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	var notes Notes
	app, err := FromSource(filepath.Join(root, "app"), "app:1", &notes)
	if err != nil {
		t.Fatal(err)
	}
	names := strings.Join(envNames(app), " ")
	for _, leak := range []string{"LEAK_URL", "LEAK_RES", "LEAK_ABS"} {
		if strings.Contains(names, leak) {
			t.Errorf("%s was read from outside the module: %v", leak, envNames(app))
		}
	}
	hasAll(t, envNames(app), "INSIDE_VAR")
	text := strings.Join(notes.Items(), "\n")
	if strings.Count(text, "outside the module, not read") < 5 {
		t.Errorf("want a note per location outside the module (5 imports, 2 directories):\n%s", text)
	}
	if strings.Contains(text, "was not found") {
		t.Errorf("a location outside the module is also reported as not found:\n%s", text)
	}
}
