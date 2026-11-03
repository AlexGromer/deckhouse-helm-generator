package synth

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Known keys are names of variables that the platform provides to the
// container by other means (ConfigMaps and Secrets the profile's envFrom
// names, a vault). A ${NAME} placeholder with such a name needs no variable
// of the chart. Only names are known, never values.

var (
	knownKeyName    = regexp.MustCompile(`^[-._a-zA-Z0-9]+$`)
	knownSourceName = regexp.MustCompile(`^[^\s:]+$`)
)

// LoadKnownKeys reads a known keys file; see ParseKnownKeys. Keys without a
// source belong to the source named after the file.
func LoadKnownKeys(path string) (map[string][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	keys, err := ParseKnownKeys(filepath.Base(path), f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return keys, nil
}

// ParseKnownKeys reads lines "KEY" or "source:KEY"; "#" starts a comment
// (a whole line, or after the key). A key without a source belongs to
// defaultSource. The result maps each source to its sorted, distinct keys.
func ParseKnownKeys(defaultSource string, r io.Reader) (map[string][]string, error) {
	out := map[string][]string{}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		source, key := defaultSource, line
		if i := strings.Index(line, ":"); i >= 0 {
			source, key = strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:])
			if !knownSourceName.MatchString(source) {
				return nil, fmt.Errorf("line %d: %q has no source before the colon", n, line)
			}
		}
		if !knownKeyName.MatchString(key) {
			return nil, fmt.Errorf("line %d: %q is not a key name (use KEY or source:KEY)", n, line)
		}
		out[source] = append(out[source], key)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return MergeKnownKeys(out), nil
}

// MergeKnownKeys unions known keys maps; each source's keys come out sorted
// and distinct. The result is never nil.
func MergeKnownKeys(maps ...map[string][]string) map[string][]string {
	sets := map[string]map[string]bool{}
	for _, m := range maps {
		for source, keys := range m {
			if sets[source] == nil {
				sets[source] = map[string]bool{}
			}
			for _, k := range keys {
				sets[source][k] = true
			}
		}
	}
	out := make(map[string][]string, len(sets))
	for source, set := range sets {
		keys := make([]string, 0, len(set))
		for k := range set {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out[source] = keys
	}
	return out
}

// knownIndex maps each key to the source that provides it; with several the
// first source by name.
func knownIndex(known map[string][]string) map[string]string {
	sources := make([]string, 0, len(known))
	for s := range known {
		sources = append(sources, s)
	}
	sort.Strings(sources)
	idx := map[string]string{}
	for _, s := range sources {
		for _, k := range known[s] {
			if _, ok := idx[k]; !ok {
				idx[k] = s
			}
		}
	}
	return idx
}

// ConfigMapNames returns the ConfigMaps the profile's envFrom names for the
// app, rendered, in order without repeats. A nil profile has none. Secrets
// are not listed: dhg never reads them.
func (p *Profile) ConfigMapNames(app string) ([]string, error) {
	if p == nil {
		return nil, nil
	}
	data := profileData{Name: app, Vars: p.Vars}
	if data.Vars == nil {
		data.Vars = map[string]string{}
	}
	var names []string
	seen := map[string]bool{}
	for i, e := range p.EnvFrom {
		if e.ConfigMapRef == "" {
			continue
		}
		name, err := render(fmt.Sprintf("envFrom[%d]", i), e.ConfigMapRef, data)
		if err != nil {
			return nil, err
		}
		if err := checkRefName(fmt.Sprintf("envFrom[%d]", i), name); err != nil {
			return nil, err
		}
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names, nil
}
