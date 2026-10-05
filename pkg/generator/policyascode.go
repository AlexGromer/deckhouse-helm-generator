package generator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// Policy-as-code: one catalogue of workload rules rendered for two engines.
//
//   - kyverno:  namespaced kyverno.io/v1 Policy resources shipped as chart
//     templates. They apply to the release namespace only (never cluster-wide)
//     and are toggled per rule from values (admissionPolicies.*).
//   - conftest: Rego (OPA v1 syntax) files in the chart's policy/ directory,
//     for CI checks of the rendered chart:
//     helm template <chart> | conftest test --policy <chart>/policy -
//
// Gatekeeper is intentionally not supported: a ConstraintTemplate and its
// Constraint cannot be installed in one Helm release (the Constraint CRD only
// exists after Gatekeeper has processed the template).

// policyRule is one rule of the catalogue.
type policyRule struct {
	ID          string
	ValuesKey   string
	Title       string
	Category    string
	Severity    string
	Description string
	// Kyverno is the spec.rules list of the Kyverno Policy (indented by 4).
	Kyverno string
	// Rego is the body of the conftest policy (package and imports excluded).
	Rego string
}

// defaultPolicyRules are enabled by default (restrict-registries is enabled
// automatically when registries are configured).
var defaultPolicyRules = []string{
	"disallow-privileged",
	"disallow-host-namespaces",
	"disallow-host-path",
	"require-non-root",
	"require-resources",
	"require-probes",
	"disallow-latest-tag",
}

const kyvernoPodMatch = `      match:
        any:
          - resources:
              kinds:
                - Pod
`

var policyCatalogue = []policyRule{
	{
		ID:          "disallow-privileged",
		ValuesKey:   "disallowPrivileged",
		Title:       "Disallow Privileged Containers",
		Category:    "Pod Security Standards (Baseline)",
		Severity:    "high",
		Description: "Privileged containers have full access to the host. securityContext.privileged must be unset or false.",
		Kyverno: `    - name: privileged-containers
` + kyvernoPodMatch + `      validate:
        message: >-
          Privileged mode is disallowed. securityContext.privileged must be unset or false
          for all containers, init containers and ephemeral containers.
        pattern:
          spec:
            =(ephemeralContainers):
              - =(securityContext):
                  =(privileged): "false"
            =(initContainers):
              - =(securityContext):
                  =(privileged): "false"
            containers:
              - =(securityContext):
                  =(privileged): "false"
`,
		Rego: `deny contains msg if {
	some c in dhg_containers
	c.securityContext.privileged == true
	msg := sprintf("%s: container %q must not run privileged", [dhg_workload, c.name])
}
`,
	},
	{
		ID:          "disallow-host-namespaces",
		ValuesKey:   "disallowHostNamespaces",
		Title:       "Disallow Host Namespaces",
		Category:    "Pod Security Standards (Baseline)",
		Severity:    "high",
		Description: "Sharing the host network, PID or IPC namespace breaks workload isolation.",
		Kyverno: `    - name: host-namespaces
` + kyvernoPodMatch + `      validate:
        message: >-
          Sharing the host namespaces is disallowed. spec.hostNetwork, spec.hostIPC
          and spec.hostPID must be unset or false.
        pattern:
          spec:
            =(hostPID): "false"
            =(hostIPC): "false"
            =(hostNetwork): "false"
`,
		Rego: `deny contains msg if {
	some field in ["hostNetwork", "hostPID", "hostIPC"]
	dhg_pod_spec[field] == true
	msg := sprintf("%s: %s must not be enabled", [dhg_workload, field])
}
`,
	},
	{
		ID:          "disallow-host-path",
		ValuesKey:   "disallowHostPath",
		Title:       "Disallow hostPath Volumes",
		Category:    "Pod Security Standards (Baseline)",
		Severity:    "medium",
		Description: "hostPath volumes expose the node filesystem to the pod.",
		Kyverno: `    - name: host-path
` + kyvernoPodMatch + `      validate:
        message: hostPath volumes are forbidden. spec.volumes[*].hostPath must be unset.
        pattern:
          spec:
            =(volumes):
              - X(hostPath): "null"
`,
		Rego: `deny contains msg if {
	some v in dhg_pod_spec.volumes
	v.hostPath
	msg := sprintf("%s: volume %q must not use hostPath", [dhg_workload, v.name])
}
`,
	},
	{
		ID:          "require-non-root",
		ValuesKey:   "requireNonRoot",
		Title:       "Require runAsNonRoot",
		Category:    "Pod Security Standards (Restricted)",
		Severity:    "medium",
		Description: "Containers must run as a non-root user (runAsNonRoot: true on the pod or on every container).",
		Kyverno: `    - name: run-as-non-root
` + kyvernoPodMatch + `      validate:
        message: >-
          Running as root is not allowed. Either spec.securityContext.runAsNonRoot must be true,
          or securityContext.runAsNonRoot must be true on every container.
        anyPattern:
          - spec:
              securityContext:
                runAsNonRoot: true
              =(ephemeralContainers):
                - =(securityContext):
                    =(runAsNonRoot): true
              =(initContainers):
                - =(securityContext):
                    =(runAsNonRoot): true
              containers:
                - =(securityContext):
                    =(runAsNonRoot): true
          - spec:
              =(ephemeralContainers):
                - securityContext:
                    runAsNonRoot: true
              =(initContainers):
                - securityContext:
                    runAsNonRoot: true
              containers:
                - securityContext:
                    runAsNonRoot: true
`,
		Rego: `dhg_runs_as_non_root(c) if c.securityContext.runAsNonRoot == true

dhg_runs_as_non_root(c) if {
	dhg_pod_spec.securityContext.runAsNonRoot == true
	not c.securityContext.runAsNonRoot == false
}

deny contains msg if {
	some c in dhg_containers
	not dhg_runs_as_non_root(c)
	msg := sprintf("%s: container %q must run with runAsNonRoot: true", [dhg_workload, c.name])
}
`,
	},
	{
		ID:          "require-resources",
		ValuesKey:   "requireResources",
		Title:       "Require Requests and Memory Limits",
		Category:    "Best Practices",
		Severity:    "medium",
		Description: "Containers must declare CPU and memory requests and a memory limit.",
		Kyverno: `    - name: requests-and-memory-limit
` + kyvernoPodMatch + `      validate:
        message: CPU and memory requests and a memory limit are required for every container.
        pattern:
          spec:
            containers:
              - resources:
                  requests:
                    memory: "?*"
                    cpu: "?*"
                  limits:
                    memory: "?*"
`,
		Rego: `deny contains msg if {
	some c in dhg_pod_spec.containers
	some field in ["requests.cpu", "requests.memory", "limits.memory"]
	parts := split(field, ".")
	not c.resources[parts[0]][parts[1]]
	msg := sprintf("%s: container %q must set resources.%s", [dhg_workload, c.name, field])
}
`,
	},
	{
		ID:          "require-probes",
		ValuesKey:   "requireProbes",
		Title:       "Require Liveness and Readiness Probes",
		Category:    "Best Practices",
		Severity:    "medium",
		Description: "Long-running workloads (Deployments, StatefulSets, DaemonSets) must define liveness and readiness probes.",
		Kyverno: `    - name: liveness-and-readiness-probes
      match:
        any:
          - resources:
              kinds:
                - Deployment
                - StatefulSet
                - DaemonSet
      validate:
        message: Liveness and readiness probes are required for every container.
        pattern:
          spec:
            template:
              spec:
                containers:
                  - livenessProbe:
                      periodSeconds: ">0"
                    readinessProbe:
                      periodSeconds: ">0"
`,
		Rego: `deny contains msg if {
	input.kind in {"Deployment", "StatefulSet", "DaemonSet"}
	some c in dhg_pod_spec.containers
	some probe in ["livenessProbe", "readinessProbe"]
	not c[probe]
	msg := sprintf("%s: container %q must define a %s", [dhg_workload, c.name, probe])
}
`,
	},
	{
		ID:          "disallow-latest-tag",
		ValuesKey:   "disallowLatestTag",
		Title:       "Disallow Latest Tag",
		Category:    "Best Practices",
		Severity:    "medium",
		Description: "Images must be pinned to a tag other than latest (or a digest) so that rollouts are reproducible.",
		Kyverno: `    - name: require-image-tag
` + kyvernoPodMatch + `      validate:
        message: An image tag is required.
        pattern:
          spec:
            containers:
              - image: "*:*"
    - name: disallow-latest-tag
` + kyvernoPodMatch + `      validate:
        message: Using a mutable image tag such as 'latest' is not allowed.
        pattern:
          spec:
            containers:
              - image: "!*:latest"
`,
		Rego: `dhg_untagged(image) if {
	not contains(image, "@")
	parts := split(image, "/")
	not contains(parts[count(parts) - 1], ":")
}

deny contains msg if {
	some c in dhg_containers
	endswith(c.image, ":latest")
	msg := sprintf("%s: container %q must not use the latest tag (%s)", [dhg_workload, c.name, c.image])
}

deny contains msg if {
	some c in dhg_containers
	dhg_untagged(c.image)
	msg := sprintf("%s: container %q image %s must be pinned to a tag or digest", [dhg_workload, c.name, c.image])
}
`,
	},
	{
		ID:          "restrict-registries",
		ValuesKey:   "restrictRegistries",
		Title:       "Restrict Image Registries",
		Category:    "Supply Chain",
		Severity:    "medium",
		Description: "Images must come from the allowed registries (admissionPolicies.allowedRegistries).",
		// The Kyverno rule is built in the template from values; see kyvernoPolicyTemplate.
		Rego: `dhg_allowed_image(image) if {
	some registry in dhg_allowed_registries
	startswith(image, concat("", [trim_suffix(registry, "/"), "/"]))
}

deny contains msg if {
	some c in dhg_containers
	not dhg_allowed_image(c.image)
	msg := sprintf("%s: container %q image %s is not from an allowed registry %v", [dhg_workload, c.name, c.image, dhg_allowed_registries])
}
`,
	},
}

func lookupPolicyRule(id string) (policyRule, bool) {
	for _, r := range policyCatalogue {
		if r.ID == id {
			return r, true
		}
	}
	return policyRule{}, false
}

// policyRuleIDs returns all known rule IDs (for error messages).
func policyRuleIDs() []string {
	ids := make([]string, 0, len(policyCatalogue))
	for _, r := range policyCatalogue {
		ids = append(ids, r.ID)
	}
	return ids
}

// policyOptions are the parsed parameters of the policies feature.
type policyOptions struct {
	Kyverno    bool
	Conftest   bool
	Rules      []policyRule
	Action     string
	Registries []string
}

func parsePolicyOptions(fc FeatureContext) (policyOptions, error) {
	var opts policyOptions
	for _, e := range fc.ListParam("engine") {
		switch strings.ToLower(e) {
		case "kyverno":
			opts.Kyverno = true
		case "conftest", "opa", "rego":
			opts.Conftest = true
		default:
			return opts, fmt.Errorf("unknown policy engine %q (supported: kyverno, conftest)", e)
		}
	}
	if !opts.Kyverno && !opts.Conftest {
		return opts, fmt.Errorf("engine must list at least one of kyverno, conftest")
	}

	switch strings.ToLower(fc.Param("action")) {
	case "audit", "":
		opts.Action = "Audit"
	case "enforce":
		opts.Action = "Enforce"
	default:
		return opts, fmt.Errorf("action must be Audit or Enforce, got %q", fc.Param("action"))
	}

	opts.Registries = fc.ListParam("registries")
	seen := map[string]bool{}
	ids := fc.ListParam("rules")
	if len(opts.Registries) > 0 {
		ids = append(ids, "restrict-registries")
	}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		rule, ok := lookupPolicyRule(id)
		if !ok {
			return opts, fmt.Errorf("unknown policy rule %q (known: %s)", id, strings.Join(policyRuleIDs(), ", "))
		}
		opts.Rules = append(opts.Rules, rule)
	}
	if len(opts.Rules) == 0 {
		return opts, fmt.Errorf("no policy rules selected")
	}
	return opts, nil
}

func applyPoliciesFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	opts, err := parsePolicyOptions(fc)
	if err != nil {
		return nil, err
	}
	// Charts without workloads (e.g. an umbrella parent) have nothing to police.
	if !secHasWorkloadTemplate(chart) {
		return chart, nil
	}

	out := cloneChart(chart)
	if opts.Kyverno {
		rules := map[string]interface{}{}
		for _, r := range opts.Rules {
			rules[r.ValuesKey] = true
		}
		registries := make([]interface{}, 0, len(opts.Registries))
		for _, r := range opts.Registries {
			registries = append(registries, r)
		}
		if err := secAddValues(out, "admissionPolicies",
			"# Kyverno policies for this release's namespace (dhg feature: policies).\n"+
				"# validationFailureAction: Audit only reports violations, Enforce blocks them.\n",
			map[string]interface{}{
				"enabled":                 true,
				"validationFailureAction": opts.Action,
				"background":              true,
				"rules":                   rules,
				"allowedRegistries":       registries,
			}); err != nil {
			return nil, err
		}
		if err := secAddTemplate(out, "templates/admission-policies.yaml", kyvernoPolicyTemplate(newSecChartHelpers(chart), opts.Rules)); err != nil {
			return nil, err
		}
	}
	if opts.Conftest {
		files := conftestPolicyFiles(opts.Rules, opts.Registries)
		paths := make([]string, 0, len(files))
		for p := range files {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		existing := map[string]bool{}
		for _, f := range out.ExternalFiles {
			existing[f.Path] = true
		}
		for _, p := range paths {
			if existing[p] {
				return nil, fmt.Errorf("external file %s already exists", p)
			}
			out.ExternalFiles = append(out.ExternalFiles, types.ExternalFileInfo{Path: p, Content: files[p]})
		}
	}
	return out, nil
}

// kyvernoPolicyTemplate renders templates/admission-policies.yaml: one
// namespaced Policy per rule, each guarded by its values toggle.
func kyvernoPolicyTemplate(h secChartHelpers, rules []policyRule) string {
	var b strings.Builder
	b.WriteString("{{- /* Generated by dhg feature \"policies\" (engine kyverno). Requires Kyverno. */}}\n")
	b.WriteString("{{- $ap := .Values.admissionPolicies }}\n")
	b.WriteString("{{- if and $ap $ap.enabled }}\n")
	for _, r := range rules {
		guard := fmt.Sprintf("{{- if $ap.rules.%s }}\n", r.ValuesKey)
		if r.ID == "restrict-registries" {
			guard = fmt.Sprintf("{{- if and $ap.rules.%s $ap.allowedRegistries }}\n", r.ValuesKey)
		}
		b.WriteString(guard)
		b.WriteString("---\n")
		b.WriteString("apiVersion: kyverno.io/v1\n")
		b.WriteString("kind: Policy\n")
		b.WriteString("metadata:\n")
		fmt.Fprintf(&b, "  name: %s\n", h.name(fmt.Sprintf("%q", r.ID)))
		b.WriteString("  namespace: {{ $.Release.Namespace }}\n")
		b.WriteString(h.labels(4))
		b.WriteString("  annotations:\n")
		fmt.Fprintf(&b, "    policies.kyverno.io/title: %s\n", r.Title)
		fmt.Fprintf(&b, "    policies.kyverno.io/category: %s\n", r.Category)
		fmt.Fprintf(&b, "    policies.kyverno.io/severity: %s\n", r.Severity)
		b.WriteString("    policies.kyverno.io/subject: Pod\n")
		fmt.Fprintf(&b, "    policies.kyverno.io/description: %q\n", r.Description)
		b.WriteString("spec:\n")
		b.WriteString("  validationFailureAction: {{ $ap.validationFailureAction | default \"Audit\" }}\n")
		b.WriteString("  background: {{ ne (toString $ap.background) \"false\" }}\n")
		b.WriteString("  rules:\n")
		if r.ID == "restrict-registries" {
			b.WriteString(kyvernoRegistriesRule)
		} else {
			b.WriteString(r.Kyverno)
		}
		b.WriteString("{{- end }}\n")
	}
	b.WriteString("{{- end }}\n")
	return b.String()
}

// kyvernoRegistriesRule builds the image pattern ("reg1/* | reg2/*") from
// admissionPolicies.allowedRegistries at render time.
const kyvernoRegistriesRule = `    {{- $patterns := list }}
    {{- range $ap.allowedRegistries }}
    {{- $patterns = append $patterns (printf "%s/*" (trimSuffix "/" .)) }}
    {{- end }}
    {{- $image := join " | " $patterns }}
    - name: allowed-registries
` + kyvernoPodMatch + `      validate:
        message: {{ printf "Images must come from an allowed registry: %s" (join ", " $ap.allowedRegistries) | quote }}
        pattern:
          spec:
            =(ephemeralContainers):
              - image: {{ $image | quote }}
            =(initContainers):
              - image: {{ $image | quote }}
            containers:
              - image: {{ $image | quote }}
`

// conftestPolicyFiles returns the Rego files (path → content) for the rules.
func conftestPolicyFiles(rules []policyRule, registries []string) map[string]string {
	const header = "# Generated by dhg feature \"policies\" (engine conftest).\n" +
		"# Usage: helm template <chart> | conftest test --policy <chart>/policy -\n"
	files := map[string]string{
		"policy/dhg_workloads.rego": header + `package main

import rego.v1

# Helpers shared by the generated policies: the pod spec and containers of
# the object under test, whatever its workload kind.

dhg_pod_controllers := {"Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job"}

dhg_pod_spec := input.spec.template.spec if input.kind in dhg_pod_controllers

dhg_pod_spec := input.spec.jobTemplate.spec.template.spec if input.kind == "CronJob"

dhg_pod_spec := input.spec if input.kind == "Pod"

dhg_workload := sprintf("%s/%s", [input.kind, input.metadata.name])

dhg_containers contains c if some c in dhg_pod_spec.containers

dhg_containers contains c if some c in dhg_pod_spec.initContainers
`,
	}
	for _, r := range rules {
		body := r.Rego
		if r.ID == "restrict-registries" {
			if len(registries) == 0 {
				continue // nothing is allowed yet: the rule would reject every image
			}
			quoted := make([]string, 0, len(registries))
			for _, reg := range registries {
				quoted = append(quoted, fmt.Sprintf("%q", reg))
			}
			body = fmt.Sprintf("dhg_allowed_registries := [%s]\n\n", strings.Join(quoted, ", ")) + body
		}
		files["policy/"+strings.ReplaceAll(r.ID, "-", "_")+".rego"] = fmt.Sprintf(
			"%s# %s: %s\npackage main\n\nimport rego.v1\n\n%s", header, r.Title, r.Description, body)
	}
	return files
}
