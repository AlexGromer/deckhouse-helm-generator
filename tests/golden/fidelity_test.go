package golden

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// checkFidelity verifies that rendering the chart with its default values
// reproduces every input object: each field set in the input must appear in
// the rendered object with the same value. The chart may add fields (chart
// labels, defaults), but must not drop or change input data.
func checkFidelity(input, rendered []object) []string {
	byKey := map[string]object{}
	for _, o := range rendered {
		byKey[o.kind()+"/"+o.name()] = o
	}
	var problems []string
	for _, in := range input {
		key := in.kind() + "/" + in.name()
		if in.kind() == "CustomResourceDefinition" {
			continue // copied verbatim to crds/
		}
		out, ok := byKey[key]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: not rendered under its input name", key))
			continue
		}
		for _, d := range diffSubset("", map[string]interface{}(in), map[string]interface{}(out)) {
			problems = append(problems, fmt.Sprintf("%s: %s", key, d))
		}
	}
	return problems
}

// ignoredPaths are input fields a chart legitimately replaces.
var ignoredPaths = map[string]bool{
	"metadata.namespace": true, // the release namespace
	"status":             true,
}

// diffSubset lists the places where want (input) is not contained in got.
func diffSubset(path string, want, got interface{}) []string {
	if ignoredPaths[path] {
		return nil
	}
	switch w := want.(type) {
	case map[string]interface{}:
		g, ok := got.(map[string]interface{})
		if !ok {
			if len(w) == 0 && got == nil {
				return nil
			}
			return []string{fmt.Sprintf("%s: want an object, got %v", orRoot(path), short(got))}
		}
		keys := make([]string, 0, len(w))
		for k := range w {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var out []string
		for _, k := range keys {
			p := k
			if path != "" {
				p = path + "." + k
			}
			if ignoredPaths[p] {
				continue
			}
			if _, present := g[k]; !present {
				if isEmpty(w[k]) {
					continue // an empty input field carries no data
				}
				out = append(out, fmt.Sprintf("%s: missing (input %v)", p, short(w[k])))
				continue
			}
			out = append(out, diffSubset(p, w[k], g[k])...)
		}
		return out
	case []interface{}:
		g, ok := got.([]interface{})
		if !ok || len(g) != len(w) {
			if len(w) == 0 && got == nil {
				return nil
			}
			return []string{fmt.Sprintf("%s: want %d items, got %v", path, len(w), short(got))}
		}
		var out []string
		for i := range w {
			out = append(out, diffSubset(fmt.Sprintf("%s[%d]", path, i), w[i], g[i])...)
		}
		return out
	default:
		if scalar(want) != scalar(got) {
			return []string{fmt.Sprintf("%s: want %q, got %q", path, scalar(want), scalar(got))}
		}
		return nil
	}
}

func isEmpty(v interface{}) bool {
	switch x := v.(type) {
	case nil:
		return true
	case map[string]interface{}:
		return len(x) == 0
	case []interface{}:
		return len(x) == 0
	}
	return false
}

// scalar normalizes numbers so that 3, 3.0 and "3"-typed JSON numbers compare
// equal; strings are compared exactly.
func scalar(v interface{}) string {
	switch x := v.(type) {
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case nil:
		return "<nil>"
	}
	return fmt.Sprint(v)
}

func short(v interface{}) string {
	s := fmt.Sprint(v)
	if len(s) > 80 {
		s = s[:77] + "..."
	}
	return strings.ReplaceAll(s, "\n", `\n`)
}

func orRoot(p string) string {
	if p == "" {
		return "(root)"
	}
	return p
}
