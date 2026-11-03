package extractor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// configMapKeys reads the key names of the named ConfigMaps in namespace.
// It issues one GET per ConfigMap and nothing else: it never lists, never
// touches a Secret and keeps no value, only the keys of data and
// binaryData. A ConfigMap that is not found, or that the user may not read,
// is a note, not an error; any other failure is an error.
func configMapKeys(ctx context.Context, c *clusterClient, namespace string, names []string) (map[string][]string, []string, error) {
	keys := map[string][]string{}
	var notes []string
	seen := map[string]bool{}
	// A redirect is not followed: it could lead to any object, a Secret too.
	noRedirect := *c
	hc := *c.httpClient
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	noRedirect.httpClient = &hc
	c = &noRedirect
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		if errs := validation.IsDNS1123Subdomain(name); len(errs) > 0 {
			notes = append(notes, fmt.Sprintf("--known-keys-from-cluster: %q is not a valid ConfigMap name (%s); it is not read", name, errs[0]))
			continue
		}
		body, err := c.doGet(ctx, fmt.Sprintf("/api/v1/namespaces/%s/configmaps/%s", url.PathEscape(namespace), url.PathEscape(name)))
		var status *httpStatusError
		switch {
		case errors.As(err, &status) && status.Code == http.StatusNotFound:
			notes = append(notes, fmt.Sprintf("--known-keys-from-cluster: ConfigMap %s is not found in namespace %s; the variables it would provide stay required", name, namespace))
			continue
		case errors.As(err, &status) && status.Code == http.StatusForbidden:
			notes = append(notes, fmt.Sprintf("--known-keys-from-cluster: not allowed to read ConfigMap %s in namespace %s; the variables it would provide stay required", name, namespace))
			continue
		case err != nil:
			return nil, nil, fmt.Errorf("--known-keys-from-cluster: ConfigMap %s: %w", name, err)
		}
		var cm struct {
			Kind       string                     `json:"kind"`
			Data       map[string]json.RawMessage `json:"data"`
			BinaryData map[string]json.RawMessage `json:"binaryData"`
		}
		if err := json.Unmarshal(body, &cm); err != nil {
			return nil, nil, fmt.Errorf("--known-keys-from-cluster: ConfigMap %s: %w", name, err)
		}
		if cm.Kind != "ConfigMap" {
			notes = append(notes, fmt.Sprintf("--known-keys-from-cluster: the answer for %s in namespace %s has kind %q, not ConfigMap; its keys are not used", name, namespace, cm.Kind))
			continue
		}
		var list []string
		for k := range cm.Data {
			list = append(list, k)
		}
		for k := range cm.BinaryData {
			list = append(list, k)
		}
		sort.Strings(list)
		keys[name] = list
	}
	return keys, notes, nil
}

// knownKeysNamespace is the namespace whose ConfigMaps are read: --namespace,
// else the namespace of the kubeconfig context, else "default".
func knownKeysNamespace(opts Options) string {
	if opts.Namespace != "" {
		return opts.Namespace
	}
	path := opts.KubeConfig
	if path == "" {
		path = defaultKubeconfigPath()
	}
	if kc, err := parseKubeconfig(path); err == nil {
		if ctx := kc.resolveContext(opts.KubeContext); ctx != nil && strings.TrimSpace(ctx.Context.Namespace) != "" {
			return ctx.Context.Namespace
		}
	}
	return "default"
}

// kubeClient returns the injected client or one built from the kubeconfig.
func (e *SourceExtractor) kubeClient(opts Options) (*clusterClient, error) {
	if e.kube != nil {
		return e.kube, nil
	}
	path := opts.KubeConfig
	if path == "" {
		path = defaultKubeconfigPath()
	}
	return newClusterClient(path, opts.KubeContext)
}
