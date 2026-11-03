package synth

import (
	"path/filepath"
	"sort"
	"strings"
)

const (
	springOnProfile = "spring.config.activate.on-profile"
	springLegacy    = "spring.profiles"
	springInclude   = "spring.profiles.include"
	springActive    = "spring.profiles.active"
	springImport    = "spring.config.import"
)

// springLoader reads the Spring Boot configuration of one module: the
// application*.{yml,yaml,properties} files of its resource directories and
// what they pull in with spring.config.import and spring.profiles.include.
type springLoader struct {
	p     *Project
	dir   string
	dirs  []string
	notes *Notes
	// visited holds target and path of the files read, which ends cycles of
	// imports.
	visited map[string]bool
	// outside holds the import locations already noted as leaving the module.
	outside map[string]bool
	// includes holds the profiles each target ("" is the default
	// configuration) includes, active the profiles the default configuration
	// activates; both in the order found.
	includes map[string][]string
	active   []string
}

// loadSpring follows Spring Boot: application.properties overrides
// application.yml; a document with spring.config.activate.on-profile (or
// the legacy spring.profiles) belongs to that profile, as do the
// application-<profile>.* files.
//
// The profiles the default configuration activates (spring.profiles.active
// and spring.profiles.include, nested includes of the profiles too) are
// merged into the default properties; a profile that includes others gets
// their properties in its own. spring.config.import locations (optional:,
// classpath:, file:) are read into the document that imports them. Other
// profiles (dev, test) are never activated on the app's behalf.
func (p *Project) loadSpring(dir string, dirs []string, notes *Notes) error {
	l := &springLoader{p: p, dir: dir, dirs: dirs, notes: notes, visited: map[string]bool{}, outside: map[string]bool{}, includes: map[string][]string{}}
	for _, d := range dirs {
		for _, ext := range profileExts {
			if err := l.loadFile(filepath.Join(d, "application."+ext), ""); err != nil {
				return err
			}
		}
	}
	if err := p.loadProfileFiles(dirs, profileExts, func(path, profile string) error { return l.loadFile(path, profile) }); err != nil {
		return err
	}
	l.activate()
	return nil
}

// loadFile reads one file into target ("" is the default configuration, else
// a profile name) and follows its imports.
func (l *springLoader) loadFile(path, target string) error {
	if !isFile(path) {
		return nil
	}
	key := target + "\x00" + filepath.Clean(path)
	if l.visited[key] {
		return nil
	}
	l.visited[key] = true
	docs, err := readConfig(path)
	if err != nil {
		return err
	}
	for _, doc := range docs {
		on := doc[springOnProfile]
		if on == "" {
			on = doc[springLegacy]
		}
		delete(doc, springOnProfile)
		delete(doc, springLegacy)
		// Spring Boot rejects on-profile in a profile-specific file.
		if target != "" {
			on = ""
		}
		var targets []string
		switch {
		case on == "":
			targets = []string{target}
		case strings.ContainsAny(on, "&|!()"):
			l.notes.Addf("%s: profile expression %q in %s is not mapped to a values file", l.p.Name, on, filepath.Base(path))
			continue
		default:
			for _, name := range strings.Split(on, ",") {
				targets = append(targets, strings.TrimSpace(name))
			}
		}
		for _, t := range targets {
			if t == "" {
				mergeInto(l.p.Properties, doc)
			} else {
				l.p.addProfile(t, doc)
			}
			l.includes[t] = appendNames(l.includes[t], doc[springInclude])
			if t == "" {
				l.active = appendNames(l.active, doc[springActive])
			}
			if err := l.followImports(doc[springImport], path, t); err != nil {
				return err
			}
		}
	}
	return nil
}

// appendNames adds the profile names of a comma-separated value to list,
// skipping repeats and names only the build or the runtime can know.
func appendNames(list []string, value string) []string {
	for _, name := range strings.Split(value, ",") {
		name = strings.TrimSpace(name)
		if name == "" || strings.ContainsAny(name, "${}@") {
			continue
		}
		dup := false
		for _, have := range list {
			dup = dup || have == name
		}
		if !dup {
			list = append(list, name)
		}
	}
	return list
}

// followImports reads the spring.config.import locations of the file at
// from into target.
func (l *springLoader) followImports(value, from, target string) error {
	for _, loc := range strings.Split(value, ",") {
		loc = strings.TrimSpace(loc)
		if loc == "" {
			continue
		}
		path, ok := l.resolveImport(loc, from)
		if !ok {
			continue
		}
		if err := l.loadFile(path, target); err != nil {
			return err
		}
	}
	return nil
}

// resolveImport finds the file a spring.config.import location names. ok is
// false when there is nothing to read; a location that is neither optional
// nor found, or that only the runtime can resolve, is noted.
func (l *springLoader) resolveImport(loc, from string) (string, bool) {
	orig := loc
	optional := false
	for strings.HasPrefix(loc, "optional:") {
		optional = true
		loc = strings.TrimPrefix(loc, "optional:")
	}
	resolved, missing := ResolvePlaceholders(loc)
	if missing != "" {
		l.notes.Addf("%s: %s = %s needs %s at runtime; the imported file is not read", l.p.Name, springImport, orig, missing)
		return "", false
	}
	loc = resolved
	switch {
	case strings.HasPrefix(loc, "classpath:"):
		rel := strings.TrimPrefix(strings.TrimPrefix(loc, "classpath:"), "/")
		if rel == "" || strings.HasSuffix(rel, "/") {
			l.notes.Addf("%s: %s = %s names a directory; its files are not read, import them by name", l.p.Name, springImport, orig)
			return "", false
		}
		for _, d := range l.dirs {
			if path := filepath.Join(d, filepath.FromSlash(rel)); l.readable(path, orig) {
				return path, true
			}
		}
	case strings.HasPrefix(loc, "file:"):
		rel := strings.TrimPrefix(loc, "file:")
		if rel == "" || strings.HasSuffix(rel, "/") {
			l.notes.Addf("%s: %s = %s names a directory; its files are not read, import them by name", l.p.Name, springImport, orig)
			return "", false
		}
		path := filepath.FromSlash(rel)
		if !filepath.IsAbs(path) {
			path = filepath.Join(l.dir, path)
		}
		if l.readable(path, orig) {
			return path, true
		}
	case strings.Contains(loc, ":") && !strings.HasPrefix(loc, "/"):
		// configserver:, vault:, configtree:, aws-parameterstore: ...
		l.notes.Addf("%s: %s = %s is read at runtime; its properties are not in the chart", l.p.Name, springImport, orig)
		return "", false
	default:
		// No prefix: next to the importing file, else on the classpath.
		rel := filepath.FromSlash(loc)
		candidates := []string{filepath.Join(filepath.Dir(from), rel)}
		for _, d := range l.dirs {
			candidates = append(candidates, filepath.Join(d, rel))
		}
		for _, path := range candidates {
			if l.readable(path, orig) {
				return path, true
			}
		}
	}
	if !optional && !l.outside[orig] {
		l.notes.Addf("%s: %s = %s was not found in %s; its properties are not read", l.p.Name, springImport, orig, strings.Join(l.rels(), ", "))
	}
	return "", false
}

// readable reports whether path is a file dhg may read: one under the
// module. A file outside it is noted and not read, so the configuration of
// a project cannot make dhg read the host's files.
func (l *springLoader) readable(path, orig string) bool {
	if !isFile(path) {
		return false
	}
	if withinDir(l.dir, path) {
		return true
	}
	if !l.outside[orig] {
		l.outside[orig] = true
		l.notes.Addf("%s: %s = %s is outside the module, not read", l.p.Name, springImport, orig)
	}
	return false
}

func (l *springLoader) rels() []string {
	out := make([]string, len(l.dirs))
	for i, d := range l.dirs {
		if r, err := filepath.Rel(l.dir, d); err == nil {
			d = r
		}
		out[i] = filepath.ToSlash(d)
	}
	return out
}

// closure lists the profiles reached from names through includes, each
// before the profile that includes it.
func (l *springLoader) closure(names []string) []string {
	var out []string
	seen := map[string]bool{}
	var visit func(string)
	visit = func(n string) {
		if seen[n] {
			return
		}
		seen[n] = true
		for _, inc := range l.includes[n] {
			visit(inc)
		}
		out = append(out, n)
	}
	for _, n := range names {
		visit(n)
	}
	return out
}

// activate merges the profiles of the default configuration into its
// properties (included ones first, so the active ones win) and the includes
// of every profile into that profile.
func (l *springLoader) activate() {
	own := make(map[string]map[string]string, len(l.p.Profiles))
	for name, props := range l.p.Profiles {
		cp := make(map[string]string, len(props))
		mergeInto(cp, props)
		own[name] = cp
	}

	for _, name := range l.closure(append(append([]string(nil), l.includes[""]...), l.active...)) {
		mergeInto(l.p.Properties, own[name])
	}

	names := make([]string, 0, len(l.includes))
	for n := range l.includes {
		if n != "" && len(l.includes[n]) > 0 {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		merged := map[string]string{}
		for _, inc := range l.closure(l.includes[name]) {
			mergeInto(merged, own[inc])
		}
		mergeInto(merged, own[name])
		l.p.Profiles[name] = merged
	}
}
