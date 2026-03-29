package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// OperatorScope defines the CRD scope for an operator.
type OperatorScope string

const (
	OperatorScopeNamespaced OperatorScope = "Namespaced"
	OperatorScopeCluster    OperatorScope = "Cluster"
)

// OperatorScaffoldOptions configures operator scaffold generation.
type OperatorScaffoldOptions struct {
	OperatorName          string
	Namespace             string
	CRDGroup              string
	CRDKind               string
	CRDScope              OperatorScope
	SpecFields            []OperatorSpecField
	EnableLeaderElection  bool
	Image                 string
}

// OperatorSpecField represents a CRD spec field definition.
type OperatorSpecField struct {
	Name        string
	Type        string
	Description string
	Required    bool
}

// OperatorScaffoldResult holds generated operator scaffold files.
type OperatorScaffoldResult struct {
	Files map[string]string
}

// ValidateOperatorScaffoldOptions validates operator scaffold options and returns errors.
func ValidateOperatorScaffoldOptions(opts OperatorScaffoldOptions) []error {
	var errs []error
	if opts.OperatorName == "" {
		errs = append(errs, fmt.Errorf("OperatorName is required"))
	}
	if opts.CRDKind == "" {
		errs = append(errs, fmt.Errorf("CRDKind is required"))
	}
	if opts.CRDScope != "" && opts.CRDScope != OperatorScopeNamespaced && opts.CRDScope != OperatorScopeCluster {
		errs = append(errs, fmt.Errorf("CRDScope must be Namespaced or Cluster, got %q", opts.CRDScope))
	}
	return errs
}

// GenerateOperatorScaffold generates operator scaffold files.
func GenerateOperatorScaffold(opts OperatorScaffoldOptions) (*OperatorScaffoldResult, error) {
	if opts.OperatorName == "" {
		return nil, fmt.Errorf("OperatorName is required")
	}
	if errs := ValidateOperatorScaffoldOptions(opts); len(errs) > 0 {
		return nil, errs[0]
	}

	name := opts.OperatorName
	kind := opts.CRDKind

	files := map[string]string{
		"templates/crd-" + strings.ToLower(kind) + ".yaml":   GenerateCRDTemplate(opts),
		"templates/" + name + "-controller-deployment.yaml":   GenerateControllerDeploymentTemplate(opts),
		"templates/" + name + "-clusterrole.yaml":             GenerateClusterRoleTemplate(opts),
	}

	return &OperatorScaffoldResult{Files: files}, nil
}

// CRDSpecField represents a CRD spec field (alias for OperatorSpecField).
type CRDSpecField = OperatorSpecField

// GenerateCRDTemplate generates a CRD YAML template from the given opts.
func GenerateCRDTemplate(opts OperatorScaffoldOptions) string {
	scope := string(opts.CRDScope)
	if scope == "" {
		scope = string(OperatorScopeNamespaced)
	}
	var specProps strings.Builder
	for _, f := range opts.SpecFields {
		specProps.WriteString(fmt.Sprintf("              %s:\n                type: %s\n", f.Name, f.Type))
	}
	kind := opts.CRDKind
	group := opts.CRDGroup
	return fmt.Sprintf(`apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: %ss.%s
spec:
  group: %s
  names:
    kind: %s
    plural: %ss
    singular: %s
  scope: %s
  versions:
  - name: v1alpha1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
%s`, strings.ToLower(kind), group, group, kind, strings.ToLower(kind), strings.ToLower(kind), scope, specProps.String())
}

// GenerateControllerDeploymentTemplate generates a controller Deployment template.
func GenerateControllerDeploymentTemplate(opts OperatorScaffoldOptions) string {
	name := opts.OperatorName
	image := opts.Image
	if image == "" {
		image = name + ":latest"
	}
	leaderElectArg := ""
	if opts.EnableLeaderElection {
		leaderElectArg = "\n        - --leader-elect"
	}
	return fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s-controller
  namespace: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: %s-controller
  template:
    metadata:
      labels:
        app: %s-controller
    spec:
      containers:
      - name: manager
        image: %s
        args:%s
`, name, opts.Namespace, name, name, image, leaderElectArg)
}

// GenerateClusterRoleTemplate generates a ClusterRole template.
func GenerateClusterRoleTemplate(opts OperatorScaffoldOptions) string {
	name := opts.OperatorName
	group := opts.CRDGroup
	kind := opts.CRDKind
	return fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: %s-role
rules:
- apiGroups: [%q]
  resources: [%ss]
  verbs: [get, list, watch, create, update, patch, delete]
- apiGroups: [%q]
  resources: [%ss/status]
  verbs: [get, update, patch]
`, name, group, strings.ToLower(kind), group, strings.ToLower(kind))
}

// InjectOperatorScaffold injects operator scaffold files into a chart's templates.
// opts is accepted for future extensibility but currently unused.
func InjectOperatorScaffold(chart *types.GeneratedChart, result *OperatorScaffoldResult, opts OperatorScaffoldOptions) (*types.GeneratedChart, int, error) {
	return InjectScaffold(chart, result)
}

// InjectScaffold injects operator scaffold files into a chart's templates.
func InjectScaffold(chart *types.GeneratedChart, result *OperatorScaffoldResult) (*types.GeneratedChart, int, error) {
	if chart == nil {
		return nil, 0, fmt.Errorf("chart is nil")
	}
	if result == nil {
		return nil, 0, fmt.Errorf("scaffold result is nil")
	}

	out := copyChartTemplates(chart)
	count := 0
	for path, content := range result.Files {
		out.Templates[path] = content
		count++
	}
	return out, count, nil
}
