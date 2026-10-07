package processor

import (
	"fmt"
	"path"
	"regexp"
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
	for _, r := range resources {
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

// moveToOwnService renames r's service to the first free candidate and
// rewrites its template path, values path and template references.
func moveToOwnService(r *types.ProcessedResource, templates, values, services map[string]bool) string {
	old := r.ServiceName
	prefix := "services." + old + "."
	if old == "" || (r.ValuesPath != "" && !strings.HasPrefix(r.ValuesPath, prefix)) {
		return ""
	}
	obj := r.Original.Object
	// "shop-worker" in service "shop" becomes "shopWorker", not "shopShopWorker".
	candidates := []string{strings.TrimPrefix(obj.GetName(), old+"-"), obj.GetNamespace(), strings.ToLower(obj.GetKind())}
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
