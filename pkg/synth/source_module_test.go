package synth

import (
	"path/filepath"
	"strings"
	"testing"
)

const aggregatorPom = `<project>
  <artifactId>shop-parent</artifactId>
  <version>1.0.0</version>
  <packaging>pom</packaging>
  <modules>
    <module>database</module>
    <module>service</module>
    <module>bundle</module>
  </modules>
</project>`

func TestReadSourceMavenMultiModule(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":            aggregatorPom,
		"database/pom.xml":   "<project><artifactId>db</artifactId></project>",
		"service/pom.xml":    pom,
		"service/Dockerfile": "FROM eclipse-temurin:21-jre\nEXPOSE 8080\n",
		"service/src/main/resources/application.yml": "server:\n  port: 8081\n",
		"bundle/module-deploy.xml":                   "<x/>",
	})
	var notes Notes
	s, err := ReadSource(dir, &notes)
	if err != nil {
		t.Fatal(err)
	}
	if s.Dir != filepath.Join(dir, "service") || s.Project == nil || s.Dockerfile == nil || s.Name() != "orders-service" {
		t.Errorf("source = %+v", s)
	}
	if all := strings.Join(notes.Items(), "\n"); !strings.Contains(all, "using module service") {
		t.Errorf("notes lack the chosen module:\n%s", all)
	}
	app := s.App("shop:1", "", &notes)
	if len(app.Ports) == 0 || app.Ports[len(app.Ports)-1].Port != 8081 {
		t.Errorf("ports = %+v", app.Ports)
	}
}

func TestReadSourceMultiModuleSeveralApps(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":            strings.Replace(aggregatorPom, "<module>bundle</module>", "<module>worker</module>", 1),
		"service/pom.xml":    pom,
		"worker/Dockerfile":  "FROM alpine\nEXPOSE 9000\n",
		"database/pom.xml":   "<project><artifactId>db</artifactId></project>",
		"service/Dockerfile": "FROM eclipse-temurin:21-jre\n",
	})
	_, err := ReadSource(dir, &Notes{})
	if err == nil {
		t.Fatal("several application modules must fail")
	}
	for _, want := range []string{"service", "worker", "-f"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "database") {
		t.Errorf("a library module is not a candidate: %v", err)
	}
}

func TestReadSourceGradleMultiProject(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"settings.gradle.kts":                       "rootProject.name = \"shop\"\ninclude(\":libs:common\", \":api\")\n",
		"libs/common/build.gradle.kts":              "plugins { `java-library` }",
		"api/build.gradle.kts":                      "plugins { id(\"org.springframework.boot\") version \"3.3.0\" }",
		"api/src/main/resources/application.yml":    "server:\n  port: 8085\n",
		"build.gradle.kts":                          "plugins { java }",
		"libs/common/src/main/resources/readme.txt": "x",
	})
	var notes Notes
	s, err := ReadSource(dir, &notes)
	if err != nil {
		t.Fatal(err)
	}
	if s.Dir != filepath.Join(dir, "api") || s.Project == nil || s.Project.Framework != Spring {
		t.Errorf("source = %+v", s)
	}
}

func TestReadSourceMultiModuleWithoutApp(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":          aggregatorPom,
		"database/pom.xml": "<project><artifactId>db</artifactId></project>",
	})
	_, err := ReadSource(dir, &Notes{})
	if err == nil || !strings.Contains(err.Error(), "neither a Dockerfile") {
		t.Errorf("err = %v, want the usual 'neither a Dockerfile' error", err)
	}
}

func TestReadSourceRootDockerfileWinsOverModules(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":            aggregatorPom,
		"Dockerfile":         "FROM alpine\nEXPOSE 80\n",
		"service/pom.xml":    pom,
		"service/Dockerfile": "FROM x\n",
	})
	var notes Notes
	s, err := ReadSource(dir, &notes)
	if err != nil || s.Dir != dir {
		t.Errorf("source = %+v err = %v", s, err)
	}
}

const springParentPom = `<project>
  <parent><groupId>org.springframework.boot</groupId><artifactId>spring-boot-starter-parent</artifactId><version>3.3.4</version></parent>
  <artifactId>proj</artifactId>
  <version>1.0.0</version>
  <packaging>pom</packaging>
  <modules>
    <module>common</module>
    <module>app</module>
  </modules>
</project>`

const libraryModulePom = `<project>
  <artifactId>common</artifactId>
  <dependencies>
    <dependency><groupId>org.springframework.boot</groupId><artifactId>spring-boot-starter-data-jpa</artifactId></dependency>
  </dependencies>
</project>`

const appModulePom = `<project>
  <artifactId>app</artifactId>
  <dependencies>
    <dependency><groupId>org.springframework.boot</groupId><artifactId>spring-boot-starter-web</artifactId></dependency>
  </dependencies>
  <build><plugins><plugin><artifactId>spring-boot-maven-plugin</artifactId></plugin></plugins></build>
</project>`

// A library module with a starter (spring-boot-starter-data-jpa) is not an
// application: only an executable-app signal counts.
func TestReadSourceLibraryModuleWithStarterIsNotApp(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":        springParentPom,
		"common/pom.xml": libraryModulePom,
		"app/pom.xml":    appModulePom,
	})
	var notes Notes
	s, err := ReadSource(dir, &notes)
	if err != nil {
		t.Fatal(err)
	}
	if s.Dir != filepath.Join(dir, "app") {
		t.Errorf("Dir = %s, want the app module", s.Dir)
	}
}

// Several real application modules under a root that is a project itself:
// the old behaviour (the root project) stays, with a note instead of an error.
func TestReadSourceSeveralAppsKeepRootProjectWithNote(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":        springParentPom,
		"common/pom.xml": strings.Replace(appModulePom, "<artifactId>app</artifactId>", "<artifactId>common</artifactId>", 1),
		"app/pom.xml":    appModulePom,
	})
	var notes Notes
	s, err := ReadSource(dir, &notes)
	if err != nil {
		t.Fatalf("a root project must not fail: %v", err)
	}
	if s.Dir != dir || s.Project == nil {
		t.Errorf("source = %+v, want the root project", s)
	}
	all := strings.Join(notes.Items(), "\n")
	for _, want := range []string{"common", "app", "-f"} {
		if !strings.Contains(all, want) {
			t.Errorf("notes lack %q:\n%s", want, all)
		}
	}
}

func TestReadSourceModuleWithApplicationYml(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pom.xml":                                springParentPom,
		"common/pom.xml":                         libraryModulePom,
		"app/pom.xml":                            libraryModulePom,
		"app/src/main/resources/application.yml": "server:\n  port: 8082\n",
	})
	s, err := ReadSource(dir, &Notes{})
	if err != nil || s.Dir != filepath.Join(dir, "app") {
		t.Errorf("source = %+v err = %v", s, err)
	}
}

func TestReadSourceGradleLibraryWithStarterIsNotApp(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"settings.gradle":     "include 'common', 'api'\n",
		"build.gradle":        "plugins { id 'java' }",
		"common/build.gradle": "dependencies { implementation 'org.springframework.boot:spring-boot-starter-data-jpa:3.3.0' }",
		"api/build.gradle":    "plugins { id 'org.springframework.boot' version '3.3.0' }",
	})
	s, err := ReadSource(dir, &Notes{})
	if err != nil || s.Dir != filepath.Join(dir, "api") {
		t.Errorf("source = %+v err = %v", s, err)
	}
}
