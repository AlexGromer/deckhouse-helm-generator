package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// VaultAgentOptions configures Vault Agent injector annotation generation.
type VaultAgentOptions struct {
	VaultAddress       string
	VaultRole          string
	AuthPath           string
	PrePopulate        bool
	ExitOnRetryFailure bool
}

// VaultAgentSecret describes a secret to be injected via Vault Agent.
type VaultAgentSecret struct {
	Name      string
	Namespace string
	VaultPath string
	// Template is an optional Vault Agent template for the secret.
	// If empty, no agent-inject-template annotation is generated.
	Template string
}

// GenerateVaultAgentAnnotations generates a map of Vault Agent injector annotations
// for the given secrets and options.
func GenerateVaultAgentAnnotations(secrets []VaultAgentSecret, opts VaultAgentOptions) map[string]string {
	annotations := make(map[string]string)

	// Core injection annotation.
	annotations["vault.hashicorp.com/agent-inject"] = "true"

	// Role annotation.
	if opts.VaultRole != "" {
		annotations["vault.hashicorp.com/role"] = opts.VaultRole
	}

	// Auth path annotation.
	if opts.AuthPath != "" {
		annotations["vault.hashicorp.com/auth-path"] = opts.AuthPath
	}

	// Pre-populate annotation.
	if opts.PrePopulate {
		annotations["vault.hashicorp.com/agent-pre-populate"] = "true"
	}

	// Exit on retry failure annotation.
	if opts.ExitOnRetryFailure {
		annotations["vault.hashicorp.com/agent-exit-on-retry-failure"] = "true"
	}

	// Per-secret annotations.
	for _, s := range secrets {
		if s.VaultPath != "" {
			annotations[fmt.Sprintf("vault.hashicorp.com/agent-inject-secret-%s", s.Name)] = s.VaultPath
		}
		if s.Template != "" {
			annotations[fmt.Sprintf("vault.hashicorp.com/agent-inject-template-%s", s.Name)] = s.Template
		}
	}

	return annotations
}

// InjectVaultAgentAnnotations injects Vault Agent annotations into workload templates
// in the chart that reference secrets from the graph. Returns the updated chart
// (copy-on-write) and the count of workloads that received annotations.
//
// Returns (nil, 0) if chart is nil.
func InjectVaultAgentAnnotations(chart *types.GeneratedChart, graph *types.ResourceGraph, opts VaultAgentOptions) (*types.GeneratedChart, int) {
	if chart == nil {
		return nil, 0
	}

	result := copyChartTemplates(chart)
	count := 0

	// Collect Vault secrets from the graph (Secrets referenced by workloads).
	var secrets []VaultAgentSecret
	if graph != nil {
		for _, r := range graph.Resources {
			if r.Original.GVK.Kind != "Secret" {
				continue
			}
			obj := r.Original.Object
			secrets = append(secrets, VaultAgentSecret{
				Name:      obj.GetName(),
				Namespace: obj.GetNamespace(),
				VaultPath: fmt.Sprintf("secret/data/%s/%s", obj.GetNamespace(), obj.GetName()),
			})
		}
	}

	annotations := GenerateVaultAgentAnnotations(secrets, opts)

	for path, content := range result.Templates {
		if !isWorkloadTemplate(content) {
			continue
		}
		updated := injectVaultAnnotationsIntoTemplate(content, annotations)
		if updated != content {
			result.Templates[path] = updated
			count++
		}
	}

	return result, count
}

// injectVaultAnnotationsIntoTemplate injects Vault Agent annotations into a template's
// spec.template.metadata.annotations section (for Deployments/StatefulSets/etc).
func injectVaultAnnotationsIntoTemplate(content string, annotations map[string]string) string {
	if len(annotations) == 0 {
		return content
	}

	// Build annotation block.
	var sb strings.Builder
	for k, v := range annotations {
		sb.WriteString(fmt.Sprintf("        %s: %q\n", k, v))
	}
	annotationBlock := sb.String()

	// Look for "annotations: {}" pattern (empty annotations).
	oldAnnotations := "      annotations: {}"
	newAnnotations := "      annotations:\n" + annotationBlock

	if strings.Contains(content, oldAnnotations) {
		return strings.Replace(content, oldAnnotations, newAnnotations, 1)
	}

	// Look for existing "annotations:" block under spec.template.metadata.
	if strings.Contains(content, "      annotations:") {
		return strings.Replace(content,
			"      annotations:",
			"      annotations:\n"+annotationBlock+"      # end vault annotations",
			1)
	}

	return content
}
