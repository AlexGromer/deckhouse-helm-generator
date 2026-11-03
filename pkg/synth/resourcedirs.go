package synth

import (
	"encoding/xml"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// pomModel is the part of a pom.xml that tells where the configuration is.
type pomModel struct {
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	Parent     struct {
		Version string `xml:"version"`
	} `xml:"parent"`
	Properties struct {
		Items []pomElement `xml:",any"`
	} `xml:"properties"`
	Build struct {
		Resources []pomResource `xml:"resources>resource"`
		Plugins   []pomPlugin   `xml:"plugins>plugin"`
	} `xml:"build"`
}

type pomElement struct {
	XMLName xml.Name
	Value   string `xml:",chardata"`
}

type pomResource struct {
	Directory string `xml:"directory"`
}

type pomConfig struct {
	Resources []pomResource `xml:"resources>resource"`
	Other     []pomElement  `xml:",any"`
}

type pomPlugin struct {
	ArtifactID    string    `xml:"artifactId"`
	Configuration pomConfig `xml:"configuration"`
	Executions    []struct {
		Goals         []string  `xml:"goals>goal"`
		Configuration pomConfig `xml:"configuration"`
	} `xml:"executions>execution"`
}

// configs returns the plugin's configuration and those of its executions.
func (p pomPlugin) configs() []pomConfig {
	out := []pomConfig{p.Configuration}
	for _, e := range p.Executions {
		out = append(out, e.Configuration)
	}
	return out
}

func (p pomPlugin) hasGoal(goal string) bool {
	for _, e := range p.Executions {
		for _, g := range e.Goals {
			if strings.TrimSpace(g) == goal {
				return true
			}
		}
	}
	return false
}

// Limits of the walk through generated directories: a module with more is
// not a project dhg should crawl.
const (
	maxGeneratedDepth = 3
	maxGeneratedDirs  = 200
)

// pomSentinel stands for ${project.basedir} until the path is joined.
const pomSentinel = "\x00"

var pomVar = regexp.MustCompile(`\$\{([^}]+)\}`)

// codegenPlugin matches the artifactId of a plugin that generates files.
var codegenPlugin = regexp.MustCompile(`(?i)codegen|generat`)

// outputDirKeys are the configuration elements that name where a
// code generator writes.
var outputDirKeys = map[string]bool{
	"outputDirectory": true, "outputDir": true, "output": true,
	"generatedResourcesDirectory": true, "resourcesOutputDirectory": true,
}

// resourceFinder collects the directories that hold a module's configuration.
type resourceFinder struct {
	dir   string
	props map[string]string
	seen  map[string]bool
	dirs  []string
	// generated are directories the build is to create that the checkout
	// lacks.
	generated []string
	missing   map[string]bool
	// outside are existing directories the configuration names that are not
	// under the module; they are not read.
	outside []string
}

// expand substitutes ${...} in v from the pom's properties and the Maven
// built-ins; ok is false when a reference stays unresolved.
func (f *resourceFinder) expand(v string) (string, bool) {
	for i := 0; i < 8 && strings.Contains(v, "${"); i++ {
		changed := false
		v = pomVar.ReplaceAllStringFunc(v, func(ref string) string {
			name := ref[2 : len(ref)-1]
			if val, ok := f.props[name]; ok {
				changed = true
				return val
			}
			return ref
		})
		if !changed {
			break
		}
	}
	return v, !strings.Contains(v, "${")
}

// path turns a pom directory value into a path under the module.
func (f *resourceFinder) path(v string) (string, bool) {
	v, ok := f.expand(strings.TrimSpace(v))
	if !ok || v == "" {
		return "", false
	}
	switch {
	case strings.HasPrefix(v, pomSentinel):
		return filepath.Join(f.dir, v[len(pomSentinel):]), true
	case filepath.IsAbs(v):
		return filepath.Clean(v), true
	}
	return filepath.Join(f.dir, v), true
}

// add records dir when it exists; a missing one that the build generates is
// reported in generated.
func (f *resourceFinder) add(dir string, generatedByBuild bool) {
	dir = filepath.Clean(dir)
	if f.seen[dir] {
		return
	}
	if isDir(dir) {
		f.seen[dir] = true
		if !withinDir(f.dir, dir) {
			f.outside = append(f.outside, f.rel(dir))
			return
		}
		f.dirs = append(f.dirs, dir)
		return
	}
	if generatedByBuild && isGeneratedPath(f.rel(dir)) && !f.missing[dir] {
		f.missing[dir] = true
		f.generated = append(f.generated, f.rel(dir))
	}
}

func (f *resourceFinder) rel(dir string) string {
	if r, err := filepath.Rel(f.dir, dir); err == nil {
		return filepath.ToSlash(r)
	}
	return filepath.ToSlash(dir)
}

// isGeneratedPath reports whether a build creates path: it sits in target
// or in a directory whose name says "generated".
func isGeneratedPath(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if seg == "target" || strings.Contains(strings.ToLower(seg), "generated") {
			return true
		}
	}
	return false
}

// withinDir reports whether path is root or under it, by its name and by
// where it leads once symlinks are resolved.
func withinDir(root, path string) bool {
	inside := func(root, path string) bool {
		rel, err := filepath.Rel(root, path)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil || !inside(absRoot, absPath) {
		return false
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return false
	}
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// Not there: nothing to read through a link either.
		return true
	}
	return inside(realRoot, realPath)
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// findResourceDirs returns the existing directories with a module's
// configuration, main resources first: src/main/resources, the pom's
// <build><resources>, the directories build-helper's add-resource and a
// code generator's output configuration name, and the generated directories
// found by glob. generated lists the directories the build is to create that
// are missing, outside the directories the pom names that are not under the
// module (they are not read). pom may be nil.
func findResourceDirs(dir string, pom *pomModel) (dirs, generated, outside []string) {
	f := &resourceFinder{dir: dir, seen: map[string]bool{}, missing: map[string]bool{}, props: map[string]string{
		"project.basedir": pomSentinel, "basedir": pomSentinel,
		"project.build.directory": pomSentinel + "/target", "build.directory": pomSentinel + "/target",
		"project.build.outputDirectory": pomSentinel + "/target/classes",
	}}
	f.add(filepath.Join(dir, "src", "main", "resources"), false)
	if pom != nil {
		f.props["project.artifactId"] = pom.ArtifactID
		for _, e := range pom.Properties.Items {
			f.props[e.XMLName.Local] = strings.TrimSpace(e.Value)
		}
		for _, r := range pom.Build.Resources {
			if p, ok := f.path(r.Directory); ok {
				f.add(p, false)
			}
		}
		for _, plugin := range pom.Build.Plugins {
			f.addPlugin(plugin)
		}
	}
	f.addGeneratedByGlob()
	sort.Strings(f.generated)
	sort.Strings(f.outside)
	return f.dirs, f.generated, f.outside
}

func (f *resourceFinder) addPlugin(plugin pomPlugin) {
	buildHelper := strings.Contains(plugin.ArtifactID, "build-helper") && plugin.hasGoal("add-resource")
	generator := codegenPlugin.MatchString(plugin.ArtifactID)
	for _, c := range plugin.configs() {
		if buildHelper {
			for _, r := range c.Resources {
				if p, ok := f.path(r.Directory); ok {
					f.add(p, true)
				}
			}
		}
		if !generator {
			continue
		}
		for _, e := range c.Other {
			if !outputDirKeys[e.XMLName.Local] {
				continue
			}
			// A generator's output is only configuration when it is
			// resources; source output is not read.
			if p, ok := f.path(e.Value); ok && strings.Contains(strings.ToLower(f.rel(p)), "resources") {
				f.add(p, true)
			}
		}
	}
}

// addGeneratedByGlob adds the directories under <module>/*/generated-resources
// (which includes target/generated-resources) that hold application
// configuration. The walk is bounded and in lexical order.
func (f *resourceFinder) addGeneratedByGlob() {
	entries, err := os.ReadDir(f.dir)
	if err != nil {
		return
	}
	budget := maxGeneratedDirs
	for _, e := range entries {
		root := filepath.Join(f.dir, e.Name(), "generated-resources")
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || !isDir(root) {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return fs.SkipDir
			}
			if !d.IsDir() {
				return nil
			}
			if budget--; budget < 0 {
				return fs.SkipAll
			}
			if depth := strings.Count(strings.TrimPrefix(path, root), string(filepath.Separator)); depth > maxGeneratedDepth {
				return fs.SkipDir
			}
			if hasApplicationConfig(path) {
				f.add(path, false)
			}
			return nil
		})
	}
}

var applicationFile = regexp.MustCompile(`^application(-[A-Za-z0-9_.-]+)?\.(yml|yaml|properties)$`)

// hasApplicationConfig reports whether dir directly holds an
// application*.{yml,yaml,properties} file.
func hasApplicationConfig(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && applicationFile.MatchString(e.Name()) {
			return true
		}
	}
	return false
}
