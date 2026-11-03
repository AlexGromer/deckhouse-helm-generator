// Package redact replaces credentials that sit in plain ConfigMaps and
// container env values with a placeholder before the objects become chart
// values. Secrets are not handled here: they have their own strategy
// (--cluster-secrets).
//
// A value is a credential by the name of its key (package secretname), or by
// the name of a JVM system property inside an env value
// (-Djavax.net.ssl.trustStorePassword=...). Reference names (secretName,
// secretKeyRef.name, existingSecret, ...) are never touched: only the values
// of ConfigMap data keys and of env entries change.
package redact

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/secretname"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// Placeholder replaces a credential value; it tells the reader to supply the
// real value (--set) or to move it to a Secret.
const Placeholder = "<SET-ME>"

// maxListed is how many redacted values Note lists without verbose.
const maxListed = 20

// jvmCredential matches a JVM system property whose last name segment ends in
// password, secret or token: -Djavax.net.ssl.trustStorePassword=changeit.
// Group 1 is the property with its separator, the value follows.
var jvmCredential = regexp.MustCompile(`(?i)(-D[^\s=:]*(?:password|secret|token)[=:])(\S*)`)

// reference is a value that only names another variable ($(VAR), ${VAR}).
var reference = regexp.MustCompile(`^\$[({][A-Za-z_][A-Za-z0-9_]*[)}]$`)

// podSpecPaths are where a workload keeps its pod spec.
var podSpecPaths = [][]string{
	{"spec", "template", "spec"},
	{"spec", "jobTemplate", "spec", "template", "spec"},
}

// containerLists are the pod spec fields that hold containers with env.
var containerLists = []string{"initContainers", "containers", "ephemeralContainers"}

// Credentials redacts, in place, credentials in the ConfigMaps and the pod
// specs among resources, and returns what it changed as sorted
// "<Kind>/<name> <path>" entries.
func Credentials(resources []*types.ExtractedResource) []string {
	var items []string
	for _, r := range resources {
		if r == nil || r.Object == nil {
			continue
		}
		obj := r.Object
		kind, name := obj.GetKind(), obj.GetName()
		var paths []string
		switch kind {
		case "Secret":
			continue
		case "ConfigMap":
			paths = configMap(obj)
		default:
			paths = workload(obj, kind)
		}
		for _, p := range paths {
			items = append(items, fmt.Sprintf("%s/%s %s", kind, name, p))
		}
	}
	sort.Strings(items)
	return items
}

// configMap redacts the sensitive keys of data and binaryData.
func configMap(obj *unstructured.Unstructured) []string {
	var paths []string
	for _, field := range []string{"data", "binaryData"} {
		m, ok := nested(obj, field)
		if !ok {
			continue
		}
		for key, v := range m {
			s, ok := v.(string)
			if !ok || s == "" || s == Placeholder || !secretname.Sensitive(key) || referenceKey(key) {
				continue
			}
			m[key] = Placeholder
			paths = append(paths, field+"."+key)
		}
	}
	return paths
}

// referenceKey reports whether a key of a ConfigMap names another object
// (existingSecret, secretRef, secretKeyRef) rather than holding a credential.
func referenceKey(key string) bool {
	k := strings.ToLower(strings.NewReplacer("_", "", "-", "", ".", "").Replace(key))
	return strings.HasPrefix(k, "existing") || strings.HasSuffix(k, "ref") || strings.HasSuffix(k, "name")
}

// nested returns the map at path inside obj without copying it, so that
// changes land in the object.
func nested(obj *unstructured.Unstructured, path ...string) (map[string]interface{}, bool) {
	v, found, err := unstructured.NestedFieldNoCopy(obj.Object, path...)
	if err != nil || !found {
		return nil, false
	}
	m, ok := v.(map[string]interface{})
	return m, ok
}

// workload redacts the env of every container of the pod spec of obj.
func workload(obj *unstructured.Unstructured, kind string) []string {
	var paths []string
	specs := podSpecPaths
	if kind == "Pod" {
		specs = [][]string{{"spec"}}
	}
	for _, specPath := range specs {
		spec, ok := nested(obj, specPath...)
		if !ok {
			continue
		}
		prefix := strings.Join(specPath, ".")
		for _, list := range containerLists {
			containers, ok := spec[list].([]interface{})
			if !ok {
				continue
			}
			for _, c := range containers {
				container, ok := c.(map[string]interface{})
				if !ok {
					continue
				}
				cname, _ := container["name"].(string)
				env, ok := container["env"].([]interface{})
				if !ok {
					continue
				}
				for _, e := range env {
					entry, ok := e.(map[string]interface{})
					if !ok {
						continue
					}
					ename, _ := entry["name"].(string)
					value, ok := entry["value"].(string)
					if !ok || ename == "" {
						continue
					}
					redacted, changed := envValue(ename, value)
					if !changed {
						continue
					}
					entry["value"] = redacted
					paths = append(paths, fmt.Sprintf("%s.%s[%s].env[%s]", prefix, list, cname, ename))
				}
			}
		}
	}
	return paths
}

// envValue returns the redacted value of env variable name and whether it
// changed: the whole value for a sensitive name, otherwise only the values of
// credential JVM properties inside it.
func envValue(name, value string) (string, bool) {
	if value == "" || value == Placeholder {
		return value, false
	}
	if secretname.Sensitive(name) && !referenceKey(name) {
		if reference.MatchString(value) {
			return value, false
		}
		return Placeholder, true
	}
	changed := false
	out := jvmCredential.ReplaceAllStringFunc(value, func(m string) string {
		parts := jvmCredential.FindStringSubmatch(m)
		if parts[2] == "" || parts[2] == Placeholder || strings.HasPrefix(parts[2], "$") {
			return m
		}
		changed = true
		return parts[1] + Placeholder
	})
	return out, changed
}

// Note is the one message of a run about redacted credentials: the first 20
// entries (all when verbose) and how to supply the values. It is empty when
// nothing was redacted.
func Note(items []string, verbose bool) string {
	if len(items) == 0 {
		return ""
	}
	listed := items
	if !verbose && len(items) > maxListed {
		listed = items[:maxListed]
	}
	list := strings.Join(listed, ", ")
	if len(listed) < len(items) {
		list += fmt.Sprintf(", +%d more (use -v to list all)", len(items)-len(listed))
	}
	return fmt.Sprintf("%d credential values replaced with %s (--redact-credentials): %s; supply the values with --set or move them to a Secret (--redact-credentials=false keeps them as they are)",
		len(items), Placeholder, list)
}
