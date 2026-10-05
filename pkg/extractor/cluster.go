package extractor

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	sigYAML "sigs.k8s.io/yaml"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// SecretStrategy defines how Kubernetes Secrets are handled during extraction.
type SecretStrategy string

const (
	// SecretStrategyMask replaces secret data values with "REDACTED".
	SecretStrategyMask SecretStrategy = "mask"

	// SecretStrategyInclude includes secret data as-is (use with caution).
	SecretStrategyInclude SecretStrategy = "include"

	// SecretStrategyExternalSecret converts secrets to ExternalSecret references.
	SecretStrategyExternalSecret SecretStrategy = "external-secret"
)

// ValidSecretStrategies returns all valid secret strategy values.
func ValidSecretStrategies() []SecretStrategy {
	return []SecretStrategy{SecretStrategyMask, SecretStrategyInclude, SecretStrategyExternalSecret}
}

// IsValidSecretStrategy checks if a string is a valid secret strategy.
func IsValidSecretStrategy(s string) bool {
	for _, v := range ValidSecretStrategies() {
		if string(v) == s {
			return true
		}
	}
	return false
}

// PaginationConfig holds pagination options for list requests.
type PaginationConfig struct {
	// Limit is the maximum number of resources per list request.
	// Default: 500.
	Limit int64

	// ContinueToken is used for resuming a paginated list.
	ContinueToken string
}

// DefaultPaginationLimit is the default number of resources per page.
const DefaultPaginationLimit int64 = 500

// NewPaginationConfig returns a PaginationConfig with default values.
func NewPaginationConfig() PaginationConfig {
	return PaginationConfig{Limit: DefaultPaginationLimit}
}

// ClusterExtractorConfig holds configuration for extracting resources from a live cluster.
type ClusterExtractorConfig struct {
	// Kubeconfig is the path to the kubeconfig file.
	Kubeconfig string

	// Context is the kubeconfig context to use.
	Context string

	// Namespace limits extraction to a specific namespace.
	Namespace string

	// Selector is a label selector to filter resources.
	Selector string

	// ExcludeNamespaces lists namespaces to skip during extraction.
	ExcludeNamespaces []string

	// IncludeSecrets controls whether Secret resources are extracted.
	IncludeSecrets bool

	// SecretStrategy defines how secrets are handled: "mask", "include", or "external-secret".
	SecretStrategy string

	// Pagination configures list request pagination.
	Pagination PaginationConfig
}

// Validate checks if the ClusterExtractorConfig is valid.
func (c *ClusterExtractorConfig) Validate() error {
	// Validate secret strategy if set
	if c.SecretStrategy != "" && !IsValidSecretStrategy(c.SecretStrategy) {
		return fmt.Errorf("invalid secret strategy %q; valid values: %v",
			c.SecretStrategy, ValidSecretStrategies())
	}

	// Validate pagination limit
	if c.Pagination.Limit < 0 {
		return fmt.Errorf("pagination limit must be non-negative, got %d", c.Pagination.Limit)
	}

	return nil
}

func unstructuredNestedMap(obj map[string]interface{}, fields ...string) (map[string]interface{}, bool, error) {
	val, found := obj[fields[0]]
	if !found {
		return nil, false, nil
	}
	m, ok := val.(map[string]interface{})
	if !ok {
		return nil, false, fmt.Errorf("field %q is not a map", fields[0])
	}
	return m, true, nil
}

// setNestedField sets a field in an unstructured object.
func setNestedField(obj map[string]interface{}, value interface{}, fields ...string) {
	obj[fields[0]] = value
}

// ── ClusterExtractor ───────────────────────────────────────────────────────

// ClusterExtractor extracts Kubernetes resources from a live cluster.
type ClusterExtractor struct {
	config ClusterExtractorConfig
	client *clusterClient
}

// NewClusterExtractor creates a new cluster extractor with default config.
func NewClusterExtractor() *ClusterExtractor {
	return &ClusterExtractor{
		config: ClusterExtractorConfig{
			SecretStrategy: string(SecretStrategyMask),
			Pagination:     NewPaginationConfig(),
		},
	}
}

// NewClusterExtractorWithConfig creates a new cluster extractor with the given config.
func NewClusterExtractorWithConfig(cfg ClusterExtractorConfig) *ClusterExtractor {
	// Apply defaults
	if cfg.SecretStrategy == "" {
		cfg.SecretStrategy = string(SecretStrategyMask)
	}
	if cfg.Pagination.Limit == 0 {
		cfg.Pagination = NewPaginationConfig()
	}
	return &ClusterExtractor{config: cfg}
}

// Config returns a copy of the extractor's configuration.
func (e *ClusterExtractor) Config() ClusterExtractorConfig {
	return e.config
}

// Source returns the source type.
func (e *ClusterExtractor) Source() types.Source {
	return types.SourceCluster
}

// SetClient sets a pre-built clusterClient (used for testing).
func (e *ClusterExtractor) SetClient(c *clusterClient) {
	e.client = c
}

// Validate checks if the cluster connection is valid.
func (e *ClusterExtractor) Validate(ctx context.Context, opts Options) error {
	switch opts.ClusterSecrets {
	case "", "skip", "mask", "include":
	default:
		return fmt.Errorf("invalid cluster secrets mode %q (skip, mask, include)", opts.ClusterSecrets)
	}
	if err := e.config.Validate(); err != nil {
		return fmt.Errorf("invalid cluster config: %w", err)
	}

	client, err := e.getClient(opts)
	if err != nil {
		return fmt.Errorf("cluster validation failed: %w", err)
	}

	// Try to discover core API resources to verify connectivity.
	_, err = client.discoverCoreResources(ctx)
	if err != nil {
		return fmt.Errorf("cannot connect to cluster: %w", err)
	}

	return nil
}

// Extract extracts resources from a Kubernetes cluster.
func (e *ClusterExtractor) Extract(ctx context.Context, opts Options) (<-chan *types.ExtractedResource, <-chan error) {
	resources := make(chan *types.ExtractedResource, 100)
	errors := make(chan error, 10)

	go func() {
		defer close(resources)
		defer close(errors)

		client, err := e.getClient(opts)
		if err != nil {
			errors <- fmt.Errorf("cannot create cluster client: %w", err)
			return
		}

		// Discover available API resources.
		apiResources, err := client.discoverResources(ctx)
		if err != nil {
			errors <- fmt.Errorf("cannot discover API resources: %w", err)
			return
		}

		namespace := e.effectiveNamespace(opts)

		for _, ar := range apiResources {
			if ctx.Err() != nil {
				return
			}

			// Skip secrets unless explicitly included.
			if ar.Kind == "Secret" && !e.includeSecrets(opts) {
				continue
			}

			if skipClusterKinds[ar.Kind] || !matchesKinds(ar.Kind, opts) {
				continue
			}

			err := client.listResources(ctx, ar, namespace, e.effectiveSelector(opts), e.config.Pagination.Limit, func(obj *unstructured.Unstructured) {
				// Apply namespace exclusion filter.
				if e.isExcludedNamespace(obj.GetNamespace()) {
					return
				}
				if !prepareClusterObject(obj) {
					return
				}

				// Apply secret strategy.
				if obj.GetKind() == "Secret" {
					e.applySecretStrategy(obj, opts)
				}

				resource := &types.ExtractedResource{
					Object:     obj,
					Source:     types.SourceCluster,
					SourcePath: client.server,
					GVK:        obj.GroupVersionKind(),
				}

				select {
				case resources <- resource:
				case <-ctx.Done():
				}
			})
			if err != nil {
				errors <- fmt.Errorf("error listing %s: %w", ar.Kind, err)
			}
		}
	}()

	return resources, errors
}

func (e *ClusterExtractor) getClient(opts Options) (*clusterClient, error) {
	if e.client != nil {
		return e.client, nil
	}

	kubeconfigPath := e.config.Kubeconfig
	if kubeconfigPath == "" {
		kubeconfigPath = opts.KubeConfig
	}
	if kubeconfigPath == "" {
		kubeconfigPath = defaultKubeconfigPath()
	}

	kubeContext := e.config.Context
	if kubeContext == "" {
		kubeContext = opts.KubeContext
	}

	client, err := newClusterClient(kubeconfigPath, kubeContext)
	if err != nil {
		return nil, err
	}

	e.client = client
	return client, nil
}

func (e *ClusterExtractor) effectiveNamespace(opts Options) string {
	if e.config.Namespace != "" {
		return e.config.Namespace
	}
	return opts.Namespace
}

func (e *ClusterExtractor) effectiveSelector(opts Options) string {
	if e.config.Selector != "" {
		return e.config.Selector
	}
	return opts.LabelSelector
}

// isExcludedNamespace reports namespaces skipped when extracting a whole
// cluster: configured exclusions plus system namespaces (kube-*, and d8-*
// managed by Deckhouse), unless that namespace was requested explicitly.
// Entries ending in "*" are prefixes.
func (e *ClusterExtractor) isExcludedNamespace(ns string) bool {
	if ns == "" || ns == e.config.Namespace {
		return false
	}
	for _, excluded := range append(append([]string{}, defaultExcludedNamespaces...), e.config.ExcludeNamespaces...) {
		if strings.HasSuffix(excluded, "*") && strings.HasPrefix(ns, strings.TrimSuffix(excluded, "*")) {
			return true
		}
		if ns == excluded {
			return true
		}
	}
	return false
}

var defaultExcludedNamespaces = []string{"kube-*", "d8-*"}

// skipClusterKinds are objects that exist in every cluster but are created
// and owned by the control plane, never by a chart.
var skipClusterKinds = map[string]bool{
	"Event": true, "Endpoints": true, "EndpointSlice": true, "Lease": true,
	"ControllerRevision": true, "Node": true, "Namespace": true,
	"ComponentStatus": true, "CertificateSigningRequest": true,
	"PodMetrics": true, "NodeMetrics": true, "APIService": true,
	"Pod": true, "ReplicaSet": true,
}

// matchesKinds applies --include-kinds / --exclude-kinds.
func matchesKinds(kind string, opts Options) bool {
	if len(opts.IncludeKinds) > 0 {
		found := false
		for _, k := range opts.IncludeKinds {
			if strings.EqualFold(k, kind) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for _, k := range opts.ExcludeKinds {
		if strings.EqualFold(k, kind) {
			return false
		}
	}
	return true
}

// prepareClusterObject turns a live object into a manifest a chart can
// install: objects owned by a controller (pods of a Deployment, Jobs of a
// CronJob, ...) and objects every namespace gets automatically are skipped;
// fields the API server sets are removed. It reports false to skip obj.
func prepareClusterObject(obj *unstructured.Unstructured) bool {
	if len(obj.GetOwnerReferences()) > 0 {
		return false
	}
	switch {
	case obj.GetKind() == "ConfigMap" && obj.GetName() == "kube-root-ca.crt":
		return false
	case obj.GetKind() == "ServiceAccount" && obj.GetName() == "default":
		return false
	case obj.GetKind() == "Secret":
		if t, _, _ := unstructured.NestedString(obj.Object, "type"); t == "kubernetes.io/service-account-token" {
			return false
		}
	}

	for _, field := range []string{"uid", "resourceVersion", "generation", "creationTimestamp", "managedFields", "selfLink", "deletionTimestamp", "deletionGracePeriodSeconds"} {
		unstructured.RemoveNestedField(obj.Object, "metadata", field)
	}
	unstructured.RemoveNestedField(obj.Object, "status")
	if annotations := obj.GetAnnotations(); annotations != nil {
		for _, key := range []string{"kubectl.kubernetes.io/last-applied-configuration", "deployment.kubernetes.io/revision", "meta.helm.sh/release-name", "meta.helm.sh/release-namespace"} {
			delete(annotations, key)
		}
		if len(annotations) == 0 {
			obj.SetAnnotations(nil)
		} else {
			obj.SetAnnotations(annotations)
		}
	}
	if obj.GetKind() == "Service" {
		// Cluster IPs are allocated by the cluster; pinning them makes the
		// chart fail to install ("provided IP is already allocated").
		if ip, _, _ := unstructured.NestedString(obj.Object, "spec", "clusterIP"); ip != "None" {
			unstructured.RemoveNestedField(obj.Object, "spec", "clusterIP")
			unstructured.RemoveNestedField(obj.Object, "spec", "clusterIPs")
		}
	}
	return true
}

// includeSecrets reports whether Secrets are extracted at all.
func (e *ClusterExtractor) includeSecrets(opts Options) bool {
	switch opts.ClusterSecrets {
	case "mask", "include":
		return true
	}
	return e.config.IncludeSecrets
}

func (e *ClusterExtractor) applySecretStrategy(obj *unstructured.Unstructured, opts Options) {
	strategy := e.config.SecretStrategy
	switch opts.ClusterSecrets {
	case "mask":
		strategy = string(SecretStrategyMask)
	case "include":
		strategy = string(SecretStrategyInclude)
	}
	if strategy == "" {
		strategy = string(SecretStrategyMask)
	}

	switch SecretStrategy(strategy) {
	case SecretStrategyInclude:
		// Keep as-is.
	case SecretStrategyExternalSecret:
		maskFields(obj, "EXTERNAL_SECRET_REF")
	default: // mask
		maskFields(obj, "REDACTED")
	}
}

func maskFields(obj *unstructured.Unstructured, placeholder string) {
	if data, ok, _ := unstructuredNestedMap(obj.Object, "data"); ok {
		masked := make(map[string]interface{}, len(data))
		for k := range data {
			masked[k] = placeholder
		}
		setNestedField(obj.Object, masked, "data")
	}
	if sd, ok, _ := unstructuredNestedMap(obj.Object, "stringData"); ok {
		masked := make(map[string]interface{}, len(sd))
		for k := range sd {
			masked[k] = placeholder
		}
		setNestedField(obj.Object, masked, "stringData")
	}
}

// ── clusterClient — lightweight K8s REST client ────────────────────────────

// clusterClient provides HTTP access to a Kubernetes API server.
type clusterClient struct {
	httpClient *http.Client
	server     string
	headers    http.Header
}

// newClusterClient creates a clusterClient from a kubeconfig file and optional context name.
func newClusterClient(kubeconfigPath, contextName string) (*clusterClient, error) {
	kc, err := parseKubeconfig(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("cannot load kubeconfig %s: %w", kubeconfigPath, err)
	}

	kctx := kc.resolveContext(contextName)
	if kctx == nil {
		if contextName == "" {
			return nil, fmt.Errorf("no current-context set in kubeconfig and no --context specified")
		}
		return nil, fmt.Errorf("context %q not found in kubeconfig", contextName)
	}

	cluster := kc.findCluster(kctx.Context.Cluster)
	if cluster == nil {
		return nil, fmt.Errorf("cluster %q not found in kubeconfig", kctx.Context.Cluster)
	}

	user := kc.findUser(kctx.Context.User)

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// Configure CA certificate.
	if cluster.Cluster.CertificateAuthorityData != "" {
		caData, err := base64.StdEncoding.DecodeString(cluster.Cluster.CertificateAuthorityData)
		if err != nil {
			return nil, fmt.Errorf("cannot decode CA data: %w", err)
		}
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(caData)
		tlsConfig.RootCAs = pool
	} else if cluster.Cluster.CertificateAuthority != "" {
		caData, err := os.ReadFile(cluster.Cluster.CertificateAuthority)
		if err != nil {
			return nil, fmt.Errorf("cannot read CA file %s: %w", cluster.Cluster.CertificateAuthority, err)
		}
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(caData)
		tlsConfig.RootCAs = pool
	}

	if cluster.Cluster.InsecureSkipTLSVerify {
		tlsConfig.InsecureSkipVerify = true //nolint:gosec // user-requested via kubeconfig
	}

	// Configure client certificate auth.
	if user != nil {
		if user.User.ClientCertificateData != "" && user.User.ClientKeyData != "" {
			certData, err := base64.StdEncoding.DecodeString(user.User.ClientCertificateData)
			if err != nil {
				return nil, fmt.Errorf("cannot decode client cert: %w", err)
			}
			keyData, err := base64.StdEncoding.DecodeString(user.User.ClientKeyData)
			if err != nil {
				return nil, fmt.Errorf("cannot decode client key: %w", err)
			}
			cert, err := tls.X509KeyPair(certData, keyData)
			if err != nil {
				return nil, fmt.Errorf("cannot create client certificate: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		} else if user.User.ClientCertificate != "" && user.User.ClientKey != "" {
			cert, err := tls.LoadX509KeyPair(user.User.ClientCertificate, user.User.ClientKey)
			if err != nil {
				return nil, fmt.Errorf("cannot load client certificate: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	cc := &clusterClient{
		httpClient: httpClient,
		server:     strings.TrimRight(cluster.Cluster.Server, "/"),
		headers:    make(http.Header),
	}

	// Set bearer token.
	if user != nil && user.User.Token != "" {
		cc.headers.Set("Authorization", "Bearer "+user.User.Token)
	}

	return cc, nil
}

// newClusterClientFromHTTP creates a clusterClient from an existing http.Client and server URL.
// Used in tests with httptest.Server.
func newClusterClientFromHTTP(httpClient *http.Client, serverURL string) *clusterClient {
	return &clusterClient{
		httpClient: httpClient,
		server:     strings.TrimRight(serverURL, "/"),
		headers:    make(http.Header),
	}
}

// apiResource represents a discovered Kubernetes API resource.
type apiResource struct {
	Group      string
	Version    string
	Kind       string
	Name       string // plural name for URL construction
	Namespaced bool
}

// discoverResources queries /api/v1 and /apis to find available resource types.
func (c *clusterClient) discoverResources(ctx context.Context) ([]apiResource, error) {
	var resources []apiResource

	// Discover core/v1 resources.
	coreResources, err := c.discoverCoreResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot discover core resources: %w", err)
	}
	resources = append(resources, coreResources...)

	// Discover API group resources.
	groupResources, err := c.discoverGroupResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot discover group resources: %w", err)
	}
	resources = append(resources, groupResources...)

	return resources, nil
}

func (c *clusterClient) discoverCoreResources(ctx context.Context) ([]apiResource, error) {
	body, err := c.doGet(ctx, "/api/v1")
	if err != nil {
		return nil, err
	}

	var resourceList k8sResourceList
	if err := json.Unmarshal(body, &resourceList); err != nil {
		return nil, fmt.Errorf("cannot parse /api/v1 response: %w", err)
	}

	var resources []apiResource
	for _, r := range resourceList.Resources {
		// Skip subresources (contain '/').
		if strings.Contains(r.Name, "/") {
			continue
		}
		// Skip resources that don't support list.
		if !containsVerb(r.Verbs, "list") {
			continue
		}
		resources = append(resources, apiResource{
			Group:      "",
			Version:    "v1",
			Kind:       r.Kind,
			Name:       r.Name,
			Namespaced: r.Namespaced,
		})
	}

	return resources, nil
}

func (c *clusterClient) discoverGroupResources(ctx context.Context) ([]apiResource, error) {
	body, err := c.doGet(ctx, "/apis")
	if err != nil {
		return nil, err
	}

	var groupList k8sGroupList
	if err := json.Unmarshal(body, &groupList); err != nil {
		return nil, fmt.Errorf("cannot parse /apis response: %w", err)
	}

	var resources []apiResource
	for _, group := range groupList.Groups {
		if len(group.Versions) == 0 {
			continue
		}
		// Use preferred version.
		version := group.PreferredVersion.GroupVersion
		if version == "" {
			version = group.Versions[0].GroupVersion
		}

		// Discover resources for this group version.
		path := "/apis/" + version
		grBody, err := c.doGet(ctx, path)
		if err != nil {
			continue // Skip unavailable groups.
		}

		var rl k8sResourceList
		if err := json.Unmarshal(grBody, &rl); err != nil {
			continue
		}

		parts := strings.SplitN(version, "/", 2)
		groupName := ""
		versionName := version
		if len(parts) == 2 {
			groupName = parts[0]
			versionName = parts[1]
		}

		for _, r := range rl.Resources {
			if strings.Contains(r.Name, "/") {
				continue
			}
			if !containsVerb(r.Verbs, "list") {
				continue
			}
			resources = append(resources, apiResource{
				Group:      groupName,
				Version:    versionName,
				Kind:       r.Kind,
				Name:       r.Name,
				Namespaced: r.Namespaced,
			})
		}
	}

	return resources, nil
}

// listResources lists all resources of a given type, handling pagination via continue tokens.
func (c *clusterClient) listResources(ctx context.Context, ar apiResource, namespace, selector string, limit int64, fn func(*unstructured.Unstructured)) error {
	if limit <= 0 {
		limit = DefaultPaginationLimit
	}

	continueToken := ""
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		path := buildListPath(ar, namespace)
		query := buildListQuery(selector, continueToken, limit)
		if query != "" {
			path = path + "?" + query
		}

		body, err := c.doGet(ctx, path)
		if err != nil {
			return fmt.Errorf("list %s failed: %w", ar.Name, err)
		}

		var listObj map[string]interface{}
		if err := json.Unmarshal(body, &listObj); err != nil {
			return fmt.Errorf("cannot parse list response for %s: %w", ar.Name, err)
		}

		// Extract items from the list.
		itemsRaw, ok := listObj["items"]
		if !ok {
			break
		}
		items, ok := itemsRaw.([]interface{})
		if !ok {
			break
		}

		for _, itemRaw := range items {
			itemMap, ok := itemRaw.(map[string]interface{})
			if !ok {
				continue
			}

			obj := &unstructured.Unstructured{Object: itemMap}
			// Ensure GVK is set (list items often omit apiVersion/kind).
			if obj.GetAPIVersion() == "" {
				if ar.Group == "" {
					obj.SetAPIVersion(ar.Version)
				} else {
					obj.SetAPIVersion(ar.Group + "/" + ar.Version)
				}
			}
			if obj.GetKind() == "" {
				obj.SetKind(ar.Kind)
			}

			fn(obj)
		}

		// Check for continue token (pagination).
		metadata, _ := listObj["metadata"].(map[string]interface{})
		cont, _ := metadata["continue"].(string)
		if cont == "" {
			break
		}
		continueToken = cont
	}

	return nil
}

func (c *clusterClient) doGet(ctx context.Context, path string) ([]byte, error) {
	url := c.server + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot create request for %s: %w", path, err)
	}

	req.Header.Set("Accept", "application/json")
	for k, vals := range c.headers {
		for _, v := range vals {
			req.Header.Set(k, v)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to %s failed: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cannot read response from %s: %w", path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, path, truncateStr(string(body), 200))
	}

	return body, nil
}

// ── K8s API response types ─────────────────────────────────────────────────

type k8sResourceList struct {
	Resources []k8sResourceEntry `json:"resources"`
}

type k8sResourceEntry struct {
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	Namespaced bool     `json:"namespaced"`
	Verbs      []string `json:"verbs"`
}

type k8sGroupList struct {
	Groups []k8sGroup `json:"groups"`
}

type k8sGroup struct {
	Name             string        `json:"name"`
	Versions         []k8sGroupVer `json:"versions"`
	PreferredVersion k8sGroupVer   `json:"preferredVersion"`
}

type k8sGroupVer struct {
	GroupVersion string `json:"groupVersion"`
	Version      string `json:"version"`
}

// ── Kubeconfig parsing ─────────────────────────────────────────────────────

type kubeconfigFile struct {
	APIVersion     string              `json:"apiVersion"`
	Kind           string              `json:"kind"`
	CurrentContext string              `json:"current-context"`
	Clusters       []kubeconfigCluster `json:"clusters"`
	Contexts       []kubeconfigContext `json:"contexts"`
	Users          []kubeconfigUser    `json:"users"`
}

type kubeconfigCluster struct {
	Name    string                  `json:"name"`
	Cluster kubeconfigClusterDetail `json:"cluster"`
}

type kubeconfigClusterDetail struct {
	Server                   string `json:"server"`
	CertificateAuthority     string `json:"certificate-authority,omitempty"`
	CertificateAuthorityData string `json:"certificate-authority-data,omitempty"`
	InsecureSkipTLSVerify    bool   `json:"insecure-skip-tls-verify,omitempty"`
}

type kubeconfigContext struct {
	Name    string                  `json:"name"`
	Context kubeconfigContextDetail `json:"context"`
}

type kubeconfigContextDetail struct {
	Cluster   string `json:"cluster"`
	User      string `json:"user"`
	Namespace string `json:"namespace,omitempty"`
}

type kubeconfigUser struct {
	Name string               `json:"name"`
	User kubeconfigUserDetail `json:"user"`
}

type kubeconfigUserDetail struct {
	Token                 string `json:"token,omitempty"`
	ClientCertificate     string `json:"client-certificate,omitempty"`
	ClientCertificateData string `json:"client-certificate-data,omitempty"`
	ClientKey             string `json:"client-key,omitempty"`
	ClientKeyData         string `json:"client-key-data,omitempty"`
}

func parseKubeconfig(path string) (*kubeconfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var kc kubeconfigFile
	if err := sigYAML.Unmarshal(data, &kc); err != nil {
		return nil, fmt.Errorf("cannot parse kubeconfig: %w", err)
	}

	return &kc, nil
}

func (kc *kubeconfigFile) resolveContext(name string) *kubeconfigContext {
	if name == "" {
		name = kc.CurrentContext
	}
	for i := range kc.Contexts {
		if kc.Contexts[i].Name == name {
			return &kc.Contexts[i]
		}
	}
	return nil
}

func (kc *kubeconfigFile) findCluster(name string) *kubeconfigCluster {
	for i := range kc.Clusters {
		if kc.Clusters[i].Name == name {
			return &kc.Clusters[i]
		}
	}
	return nil
}

func (kc *kubeconfigFile) findUser(name string) *kubeconfigUser {
	for i := range kc.Users {
		if kc.Users[i].Name == name {
			return &kc.Users[i]
		}
	}
	return nil
}

func defaultKubeconfigPath() string {
	if env := os.Getenv("KUBECONFIG"); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("/root", ".kube", "config")
	}
	return filepath.Join(home, ".kube", "config")
}

// ── URL construction helpers ───────────────────────────────────────────────

func buildListPath(ar apiResource, namespace string) string {
	if ar.Group == "" {
		// Core API.
		if ar.Namespaced && namespace != "" {
			return fmt.Sprintf("/api/%s/namespaces/%s/%s", ar.Version, namespace, ar.Name)
		}
		return fmt.Sprintf("/api/%s/%s", ar.Version, ar.Name)
	}
	// Named API group.
	if ar.Namespaced && namespace != "" {
		return fmt.Sprintf("/apis/%s/%s/namespaces/%s/%s", ar.Group, ar.Version, namespace, ar.Name)
	}
	return fmt.Sprintf("/apis/%s/%s/%s", ar.Group, ar.Version, ar.Name)
}

func buildListQuery(selector, continueToken string, limit int64) string {
	var params []string

	params = append(params, fmt.Sprintf("limit=%d", limit))

	if selector != "" {
		params = append(params, "labelSelector="+selector)
	}

	if continueToken != "" {
		params = append(params, "continue="+continueToken)
	}

	return strings.Join(params, "&")
}

// ── General helpers ────────────────────────────────────────────────────────

func containsVerb(verbs []string, verb string) bool {
	for _, v := range verbs {
		if v == verb {
			return true
		}
	}
	return false
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
