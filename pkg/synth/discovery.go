package synth

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// dockerfileCandidate is a Dockerfile found for the application.
type dockerfileCandidate struct {
	path string // relative to the build root, slash-separated
	file Dockerfile
	raw  string
}

// buildRoot returns the top of the Maven or Gradle build that dir belongs
// to: dir itself, or the nearest parent (at most maxModuleDepth up) that
// lists the way down to dir as a module.
func buildRoot(dir string) string {
	cur, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	for i := 0; i < maxModuleDepth; i++ {
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		rel, err := filepath.Rel(parent, cur)
		if err != nil || !listsModule(parent, filepath.ToSlash(rel)) {
			break
		}
		cur = parent
	}
	return cur
}

func listsModule(dir, rel string) bool {
	for _, m := range modules(dir) {
		if strings.Trim(filepath.ToSlash(filepath.Clean(m)), "/") == rel {
			return true
		}
	}
	return false
}

// localGlob expands a profile glob relative to base. A pattern that is
// absolute or leaves base is ignored: a profile reads project files only.
func localGlob(base, pattern string) []string {
	if pattern == "" || !filepath.IsLocal(filepath.FromSlash(pattern)) {
		return nil
	}
	matches, err := filepath.Glob(filepath.Join(base, filepath.FromSlash(pattern)))
	if err != nil {
		return nil
	}
	sort.Strings(matches)
	return matches
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// findDockerfiles lists the Dockerfiles of the application in dir, best
// first: dir/Dockerfile, dir/docker/Dockerfile*, root/docker/Dockerfile*
// (names like Dockerfile-service.txt), then the globs of the profile. In a
// group, a name that contains the module's name comes first.
func findDockerfiles(dir, root string, prof *Profile) []dockerfileCandidate {
	module := strings.ToLower(filepath.Base(dir))
	var groups [][]string
	if p := filepath.Join(dir, "Dockerfile"); isFile(p) {
		groups = append(groups, []string{p})
	}
	groups = append(groups, localGlob(dir, "docker/Dockerfile*"))
	if root != dir {
		groups = append(groups, localGlob(root, "docker/Dockerfile*"))
	}
	if prof != nil {
		var extra []string
		for _, g := range prof.DockerfileGlobs {
			extra = append(extra, localGlob(dir, g)...)
			if root != dir {
				extra = append(extra, localGlob(root, g)...)
			}
		}
		groups = append(groups, extra)
	}

	var out []dockerfileCandidate
	seen := map[string]bool{}
	for _, group := range groups {
		sort.SliceStable(group, func(i, j int) bool {
			ni := strings.Contains(strings.ToLower(filepath.Base(group[i])), module)
			nj := strings.Contains(strings.ToLower(filepath.Base(group[j])), module)
			return ni && !nj
		})
		for _, p := range group {
			abs, err := filepath.Abs(p)
			if err != nil || seen[abs] || !isFile(abs) {
				continue
			}
			seen[abs] = true
			data, err := os.ReadFile(abs)
			if err != nil {
				continue
			}
			rel, err := filepath.Rel(root, abs)
			if err != nil || strings.HasPrefix(rel, "..") {
				rel = abs
			}
			out = append(out, dockerfileCandidate{path: filepath.ToSlash(rel), file: ParseDockerfile(string(data)), raw: string(data)})
		}
	}
	return out
}

// mergeDockerfiles takes USER, EXPOSE, VOLUME and HEALTHCHECK from the best
// candidate that sets them. It returns the facts and notes (without the
// application's name) on facts that come from a lesser candidate or on which
// candidates disagree.
func mergeDockerfiles(cands []dockerfileCandidate) (*Dockerfile, []string) {
	if len(cands) == 0 {
		return nil, nil
	}
	if len(cands) == 1 {
		d := cands[0].file
		return &d, nil
	}
	merged := &Dockerfile{}
	var notes []string
	primary := cands[0].path

	type fact struct {
		name string
		set  func(Dockerfile) bool
		show func(Dockerfile) string
		take func(*Dockerfile, Dockerfile)
	}
	facts := []fact{
		{"USER", func(d Dockerfile) bool { return d.User != "" }, func(d Dockerfile) string { return d.User },
			func(m *Dockerfile, d Dockerfile) { m.User = d.User }},
		{"EXPOSE", func(d Dockerfile) bool { return len(d.Expose) > 0 || len(d.Unresolved) > 0 },
			func(d Dockerfile) string {
				return strings.Join(append(append([]string(nil), d.Expose...), d.Unresolved...), " ")
			},
			func(m *Dockerfile, d Dockerfile) { m.Expose, m.Unresolved = d.Expose, d.Unresolved }},
		{"VOLUME", func(d Dockerfile) bool { return len(d.Volumes) > 0 }, func(d Dockerfile) string { return strings.Join(d.Volumes, " ") },
			func(m *Dockerfile, d Dockerfile) { m.Volumes = d.Volumes }},
		{"HEALTHCHECK", func(d Dockerfile) bool { return len(d.Healthcheck) > 0 }, func(d Dockerfile) string { return strings.Join(d.Healthcheck, " ") },
			func(m *Dockerfile, d Dockerfile) {
				m.Healthcheck, m.Interval, m.Timeout, m.StartPeriod, m.Retries = d.Healthcheck, d.Interval, d.Timeout, d.StartPeriod, d.Retries
			}},
	}
	for _, f := range facts {
		var chosen *dockerfileCandidate
		for i := range cands {
			if f.set(cands[i].file) {
				chosen = &cands[i]
				break
			}
		}
		if chosen == nil {
			continue
		}
		f.take(merged, chosen.file)
		if chosen.path != primary {
			notes = append(notes, f.name+" "+f.show(chosen.file)+" is taken from "+chosen.path+" ("+primary+" sets none)")
		}
		for i := range cands {
			c := cands[i]
			if c.path != chosen.path && f.set(c.file) && f.show(c.file) != f.show(chosen.file) {
				notes = append(notes, c.path+" differs from "+chosen.path+" in "+f.name+" ("+f.show(c.file)+" against "+f.show(chosen.file)+"); the chart follows "+chosen.path)
			}
		}
	}
	return merged, notes
}

// envTemplate is the defaults of the environment variables found next to
// the sources: KEY=VALUE files under docker/ and deploy/.
type envTemplate struct {
	values map[string]string
	// files are the files that gave at least one value, for a note.
	files []string
}

// readEnvTemplates reads docker/*.properties and deploy/*.properties of dir
// and the build root, and the files of the profile. The first definition of
// a key wins; empty values and values with a placeholder give no default.
func readEnvTemplates(dir, root string, prof *Profile) envTemplate {
	var paths []string
	bases := []string{dir}
	if root != dir {
		bases = append(bases, root)
	}
	for _, base := range bases {
		paths = append(paths, localGlob(base, "docker/*.properties")...)
		paths = append(paths, localGlob(base, "deploy/*.properties")...)
		if prof != nil {
			for _, g := range prof.EnvTemplateFiles {
				paths = append(paths, localGlob(base, g)...)
			}
		}
	}
	tpl := envTemplate{values: map[string]string{}}
	seen := map[string]bool{}
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil || seen[abs] || !isFile(abs) {
			continue
		}
		seen[abs] = true
		data, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		props := map[string]string{}
		readProperties(string(data), props)
		used := false
		for k, v := range props {
			if v == "" || strings.Contains(v, "${") {
				continue
			}
			if _, dup := tpl.values[k]; !dup {
				tpl.values[k] = v
				used = true
			}
		}
		if used {
			rel, err := filepath.Rel(root, abs)
			if err != nil || strings.HasPrefix(rel, "..") {
				rel = abs
			}
			tpl.files = append(tpl.files, filepath.ToSlash(rel))
		}
	}
	return tpl
}

// fallbackFor returns the template values usable as placeholder defaults:
// never those of a credential.
func (t envTemplate) fallbackFor(rules *SecretRules) map[string]string {
	if len(t.values) == 0 {
		return nil
	}
	out := make(map[string]string, len(t.values))
	for k, v := range t.values {
		if !rules.Match(k) {
			out[k] = v
		}
	}
	return out
}

// migration is a database migration tool found in a module of the build.
type migration struct {
	tool     string // "liquibase" or "flyway"
	module   string // module directory relative to the build root
	evidence string // file that tells, relative to the module
}

var (
	liquibaseFile = regexp.MustCompile(`(?i)changelog.*\.(xml|ya?ml|json|sql)$`)
	flywayFile    = regexp.MustCompile(`^[VR]\d*(\.\d+)*__.+\.sql$`)
	liquibaseDep  = regexp.MustCompile(`liquibase-core|liquibase-maven-plugin|org\.liquibase|liquibase-gradle-plugin`)
	flywayDep     = regexp.MustCompile(`flyway-core|flyway-maven-plugin|org\.flywaydb|flyway-gradle-plugin`)
)

// maxScanFiles bounds the files looked at per module.
const maxScanFiles = 20000

// scanMigrations looks for Liquibase and Flyway in one module: changelog or
// migration files, or the library, plugin in its build file.
func scanMigrations(moduleDir string) []migration {
	found := map[string]string{}
	count := 0
	_ = filepath.WalkDir(moduleDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // best-effort scan: an unreadable entry is skipped
		}
		if d.IsDir() {
			switch d.Name() {
			case "target", "build", "node_modules", ".git", ".gradle":
				return filepath.SkipDir
			}
			return nil
		}
		if count++; count > maxScanFiles {
			return filepath.SkipAll
		}
		rel, _ := filepath.Rel(moduleDir, path)
		rel = filepath.ToSlash(rel)
		name := d.Name()
		switch {
		case name == "pom.xml" || name == "build.gradle" || name == "build.gradle.kts":
			if data, err := os.ReadFile(path); err == nil {
				if liquibaseDep.Match(data) && found["liquibase"] == "" {
					found["liquibase"] = rel
				}
				if flywayDep.Match(data) && found["flyway"] == "" {
					found["flyway"] = rel
				}
			}
		case liquibaseFile.MatchString(name) && strings.Contains(rel, "src/"):
			if found["liquibase"] == "" {
				found["liquibase"] = rel
			}
		case flywayFile.MatchString(name) && strings.Contains(rel, "db/migration"):
			if found["flyway"] == "" {
				found["flyway"] = rel
			}
		}
		return nil
	})
	var out []migration
	for _, tool := range []string{"liquibase", "flyway"} {
		if ev := found[tool]; ev != "" {
			out = append(out, migration{tool: tool, evidence: ev})
		}
	}
	return out
}

// allModules lists the module directories below dir, relative to it,
// descending into nested builds.
func allModules(dir string, depth int) []string {
	if depth >= maxModuleDepth {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, m := range modules(dir) {
		if m == "" || seen[m] || !filepath.IsLocal(filepath.FromSlash(m)) {
			continue
		}
		seen[m] = true
		sub := filepath.Join(dir, filepath.FromSlash(m))
		if info, err := os.Stat(sub); err != nil || !info.IsDir() {
			continue
		}
		out = append(out, m)
		for _, n := range allModules(sub, depth+1) {
			out = append(out, filepath.ToSlash(filepath.Join(m, n)))
		}
	}
	return out
}

// findMigrations looks for migration tools in the modules of the build
// other than the application's own; the application's own are reported
// separately (they run at start-up, no Job is needed).
func findMigrations(root, appDir string) (siblings, own []migration) {
	appAbs, _ := filepath.Abs(appDir)
	for _, m := range allModules(root, 0) {
		modDir := filepath.Join(root, filepath.FromSlash(m))
		abs, _ := filepath.Abs(modDir)
		// A parent of the application holds it as a module: skip it.
		if abs == appAbs || strings.HasPrefix(appAbs, abs+string(filepath.Separator)) {
			if abs == appAbs {
				for _, f := range scanMigrations(modDir) {
					f.module = m
					own = append(own, f)
				}
			}
			continue
		}
		for _, f := range scanMigrations(modDir) {
			f.module = m
			siblings = append(siblings, f)
		}
	}
	return siblings, own
}

// debugAgent matches a JDWP agent option and captures the address's port.
var debugAgent = regexp.MustCompile(`jdwp[=:]\S*?address=(?:[^,\s"':]*:)?(\d+)`)

// debugPorts returns the ports of JDWP agents set in the texts.
func debugPorts(texts []string) []int64 {
	var ports []int64
	seen := map[int64]bool{}
	for _, t := range texts {
		for _, m := range debugAgent.FindAllStringSubmatch(t, -1) {
			if n, err := strconv.ParseInt(m[1], 10, 64); err == nil && !seen[n] {
				seen[n] = true
				ports = append(ports, n)
			}
		}
	}
	sort.Slice(ports, func(i, j int) bool { return ports[i] < ports[j] })
	return ports
}
