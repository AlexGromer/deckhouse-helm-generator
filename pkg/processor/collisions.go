package processor

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// releaseNamespaceLine is the metadata.namespace line of namespaced templates.
var releaseNamespaceLine = regexp.MustCompile(`(?m)^  namespace: \{\{ \$\.Release\.Namespace \}\}$`)

// ResolveCollisions makes every processed resource render as its own object
// with its own values. It returns a note for each change it made.
//
// Two cases would otherwise lose objects silently:
//   - the same kind and name in several namespaces: charts render namespaced
//     objects into the release namespace, where they would replace each
//     other, so these keep their input namespace;
//   - two resources with the same template file or values path (e.g. two
//     Deployments labelled with the same app): the later ones move to a
//     service of their own, named after the object.
//
// Collisions are resolved in namespace, kind, name order, so which resource
// keeps the shared service and how the others are named do not depend on the
// order the input came in.
func ResolveCollisions(resources []*types.ProcessedResource) []string {
	var notes []string

	namespaces := map[string]map[string]bool{}
	for _, r := range resources {
		if id := kindName(r); id != "" {
			if namespaces[id] == nil {
				namespaces[id] = map[string]bool{}
			}
			namespaces[id][r.Original.Object.GetNamespace()] = true
		}
	}
	for _, r := range resources {
		ns := r.Original.Object.GetNamespace()
		if id := kindName(r); id != "" && ns != "" && len(namespaces[id]) > 1 &&
			releaseNamespaceLine.MatchString(r.TemplateContent) {
			r.TemplateContent = releaseNamespaceLine.ReplaceAllString(r.TemplateContent, "  namespace: "+strconv.Quote(ns))
			notes = append(notes, fmt.Sprintf("%s exists in several namespaces; it keeps namespace %q", r.Original.ResourceKey(), ns))
		}
	}

	templates := map[string]bool{}
	values := map[string]bool{}
	services := map[string]bool{}
	for _, r := range resources {
		services[r.ServiceName] = true
	}
	for _, r := range sortedForCollisions(resources) {
		if r.TemplatePath == "" {
			continue
		}
		if templates[r.TemplatePath] || (r.ValuesPath != "" && values[r.ValuesPath]) {
			if note := moveToOwnService(r, templates, values, services); note != "" {
				notes = append(notes, note)
			}
		}
		templates[r.TemplatePath] = true
		if r.ValuesPath != "" {
			values[r.ValuesPath] = true
		}
	}
	return notes
}

func kindName(r *types.ProcessedResource) string {
	if r == nil || r.Original == nil || r.Original.Object == nil {
		return ""
	}
	return r.Original.Object.GetKind() + "/" + r.Original.Object.GetName()
}

// sortedForCollisions returns resources ordered so that stable objects come
// before run-id copies (a one-off Job such as job-runner-550001-2 never takes
// the shared service from worker-1), then by namespace, kind and name (stable,
// so equal keys keep their input order). The input slice is not changed.
func sortedForCollisions(resources []*types.ProcessedResource) []*types.ProcessedResource {
	sorted := append([]*types.ProcessedResource(nil), resources...)
	key := func(r *types.ProcessedResource) [4]string {
		if r == nil || r.Original == nil || r.Original.Object == nil {
			return [4]string{}
		}
		o := r.Original.Object
		runCopy := "0"
		if runIDTail(strings.Split(o.GetName(), "-")) > 0 {
			runCopy = "1"
		}
		return [4]string{runCopy, o.GetNamespace(), o.GetKind(), o.GetName()}
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := key(sorted[i]), key(sorted[j])
		for k := range a {
			if a[k] != b[k] {
				return a[k] < b[k]
			}
		}
		return false
	})
	return sorted
}

// readableSuffix turns an object name or label value into a short suffix for
// a service name: a trailing run id ("-240101", "-550001-2": numeric tokens
// of which one has 5+ digits, or a timestamp) is dropped, and so are the words
// of the drop list (namespace, service), which the service name already
// carries. Short ordinals ("worker-1") tell objects apart and stay. It
// returns "" when nothing is left.
func readableSuffix(name string, drop ...string) string {
	tokens := strings.Split(name, "-")
	tokens = tokens[:len(tokens)-runIDTail(tokens)]
	for _, d := range drop {
		tokens = removeTokens(tokens, strings.Split(d, "-"))
	}
	return strings.Join(tokens, "-")
}

// runIDMinDigits is the length of a numeric token that looks like a run id
// (a CI build number, a Job suffix, a timestamp) rather than an ordinal.
const runIDMinDigits = 5

// runIDTail returns how many trailing tokens are a run id: the numeric tail,
// when one of its tokens has runIDMinDigits or more digits ("2-240101" is one
// tail); else 0.
func runIDTail(tokens []string) int {
	n, long := 0, false
	for i := len(tokens) - 1; i >= 0 && isNumeric(tokens[i]); i-- {
		n++
		if len(tokens[i]) >= runIDMinDigits {
			long = true
		}
	}
	if !long {
		return 0
	}
	return n
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// removeTokens removes the first run of drop's tokens from tokens.
func removeTokens(tokens, drop []string) []string {
	if len(drop) == 0 || drop[0] == "" {
		return tokens
	}
	for i := 0; i+len(drop) <= len(tokens); i++ {
		match := true
		for j := range drop {
			if tokens[i+j] != drop[j] {
				match = false
				break
			}
		}
		if match {
			return append(append([]string(nil), tokens[:i]...), tokens[i+len(drop):]...)
		}
	}
	return tokens
}

// moveToOwnService renames r's service to the first free candidate and
// rewrites its template path, values path and template references. The
// candidates are readable: the object name without namespace, service and run
// ids, then its app label, namespace and kind; a counter is the last resort.
func moveToOwnService(r *types.ProcessedResource, templates, values, services map[string]bool) string {
	old := r.ServiceName
	prefix := "services." + old + "."
	if old == "" || (r.ValuesPath != "" && !strings.HasPrefix(r.ValuesPath, prefix)) {
		return ""
	}
	obj := r.Original.Object
	labels := obj.GetLabels()
	group := labels["app"]
	if group == "" {
		group = labels["app.kubernetes.io/name"]
	}
	// "shop-worker" in service "shop" becomes "shopWorker", not "shopShopWorker".
	candidates := []string{
		readableSuffix(obj.GetName(), obj.GetNamespace(), old),
		readableSuffix(group, obj.GetNamespace(), old),
		obj.GetNamespace(),
		strings.ToLower(obj.GetKind()),
	}
	for i := 2; i < 100; i++ {
		candidates = append(candidates, strconv.Itoa(i))
	}
	for _, suffix := range candidates {
		if suffix == "" || suffix == old {
			continue
		}
		name := SanitizeServiceName(old + "-" + suffix)
		valuesPath := ""
		if r.ValuesPath != "" {
			valuesPath = "services." + name + "." + strings.TrimPrefix(r.ValuesPath, prefix)
		}
		dir, file := path.Split(r.TemplatePath)
		templatePath := dir + strings.TrimSuffix(file, ".yaml") + "-" + suffix + ".yaml"
		if services[name] || templates[templatePath] || (valuesPath != "" && values[valuesPath]) {
			continue
		}
		ref := regexp.MustCompile(`\.Values\.services\.` + regexp.QuoteMeta(old) + `\b`)
		r.TemplateContent = ref.ReplaceAllString(r.TemplateContent, ".Values.services."+name)
		r.ServiceName = name
		r.ValuesPath = valuesPath
		r.TemplatePath = templatePath
		services[name] = true
		return fmt.Sprintf("%s shares its template or values with another resource of service %q; it moves to service %q", r.Original.ResourceKey(), old, name)
	}
	return ""
}
