package operationpolicy

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/yaml"
)

type Policy struct {
	Name string
	Spec PolicySpec
}

type PolicySpec struct {
	Match Match `json:"match,omitempty"`

	AllowedRepos            []string           `json:"allowedRepos,omitempty"`
	RequiredResources       *RequiredResources `json:"requiredResources,omitempty"`
	DisallowedImageTags     []string           `json:"disallowedImageTags,omitempty"`
	RequiredProbes          []string           `json:"requiredProbes,omitempty"`
	RequiredLabels          *RequiredLabels    `json:"requiredLabels,omitempty"`
	MaxRevisionHistoryLimit *int32             `json:"maxRevisionHistoryLimit,omitempty"`
	ImagePullPolicy         *string            `json:"imagePullPolicy,omitempty"`
	PriorityClassNames      []string           `json:"priorityClassNames,omitempty"`
	IngressClassNames       []string           `json:"ingressClassNames,omitempty"`
	StorageClassNames       []string           `json:"storageClassNames,omitempty"`
	ReplicaLimits           *ReplicaLimits     `json:"replicaLimits,omitempty"`

	// Ignored fields mapped to avoid struct problems (no checking required for now)
	DisallowedTolerations     []interface{} `json:"disallowedTolerations,omitempty"`
	RequiredAnnotations       interface{}   `json:"requiredAnnotations,omitempty"`
	CheckHostNetworkDNSPolicy *bool         `json:"checkHostNetworkDNSPolicy,omitempty"`
	CheckContainerDuplicates  *bool         `json:"checkContainerDuplicates,omitempty"`
	GpuResourceRestriction    interface{}   `json:"gpuResourceRestriction,omitempty"`
}

type Match struct {
	LabelSelector     map[string]string `json:"labelSelector,omitempty"`
	NamespaceSelector map[string]string `json:"namespaceSelector,omitempty"`
}

type RequiredResources struct {
	Limits   []string `json:"limits,omitempty"`
	Requests []string `json:"requests,omitempty"`
}

type RequiredLabels struct {
	WatchKinds []string    `json:"watchKinds,omitempty"`
	Labels     []LabelRule `json:"labels,omitempty"`
}

type LabelRule struct {
	Key          string `json:"key"`
	AllowedRegex string `json:"allowedRegex,omitempty"`
}

type ReplicaLimits struct {
	MinReplicas *int32 `json:"minReplicas,omitempty"`
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`
}

type Finding struct {
	ObjectName string
	PolicyName string
	Rule       string
	Message    string
}

func Load(paths []string) ([]Policy, error) {
	var policies []Policy

	for _, p := range paths {
		content, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}

		docs := bytes.Split(content, []byte("\n---"))
		for _, doc := range docs {
			doc = bytes.TrimSpace(doc)
			if len(doc) == 0 {
				continue
			}

			var obj map[string]interface{}
			if err := yaml.Unmarshal(doc, &obj); err != nil {
				return nil, fmt.Errorf("error parsing %s: %w", p, err)
			}

			if kind, _ := obj["kind"].(string); kind != "OperationPolicy" {
				continue
			}

			name := ""
			if md, ok := obj["metadata"].(map[string]interface{}); ok {
				if n, ok := md["name"].(string); ok {
					name = n
				}
			}

			var pol struct {
				Spec PolicySpec `json:"spec"`
			}
			if err := yaml.Unmarshal(doc, &pol); err != nil {
				return nil, fmt.Errorf("error parsing %s: %w", p, err)
			}
			policies = append(policies, Policy{
				Name: name,
				Spec: pol.Spec,
			})
		}
	}
	return policies, nil
}

func Check(p Policy, obj *unstructured.Unstructured) []Finding {
	var findings []Finding

	objLabels := obj.GetLabels()
	if objLabels == nil {
		objLabels = make(map[string]string)
	}

	if len(p.Spec.Match.LabelSelector) > 0 {
		sel := labels.Set(p.Spec.Match.LabelSelector).AsSelector()
		if !sel.Matches(labels.Set(objLabels)) {
			return nil
		}
	}

	objectName := fmt.Sprintf("%s/%s", obj.GetKind(), obj.GetName())

	add := func(rule, msg string) {
		findings = append(findings, Finding{
			ObjectName: objectName,
			PolicyName: p.Name,
			Rule:       rule,
			Message:    msg,
		})
	}

	if len(p.Spec.Match.NamespaceSelector) > 0 {
		add("namespaceSelector", "namespace labels are unknown to dhg")
	}

	kind := obj.GetKind()

	// allowedRepos
	if len(p.Spec.AllowedRepos) > 0 {
		for _, cont := range getContainers(obj) {
			img, _, _ := unstructured.NestedString(cont.(map[string]interface{}), "image")
			if img != "" {
				allowed := false
				for _, r := range p.Spec.AllowedRepos {
					if strings.HasPrefix(img, r) {
						allowed = true
						break
					}
				}
				if !allowed {
					add("allowedRepos", fmt.Sprintf("container %s image %s is not in allowedRepos", getContainerName(cont), img))
				}
			}
		}
	}

	// requiredResources
	if p.Spec.RequiredResources != nil {
		for _, cont := range getContainers(obj) {
			cname := getContainerName(cont)

			for _, r := range p.Spec.RequiredResources.Limits {
				val, _, _ := unstructured.NestedString(cont.(map[string]interface{}), "resources", "limits", r)
				if val == "" {
					add("requiredResources", fmt.Sprintf("container %s missing limits.%s", cname, r))
				}
			}
			for _, r := range p.Spec.RequiredResources.Requests {
				val, _, _ := unstructured.NestedString(cont.(map[string]interface{}), "resources", "requests", r)
				if val == "" {
					add("requiredResources", fmt.Sprintf("container %s missing requests.%s", cname, r))
				}
			}
		}
	}

	// disallowedImageTags
	if len(p.Spec.DisallowedImageTags) > 0 {
		for _, cont := range getContainers(obj) {
			img, _, _ := unstructured.NestedString(cont.(map[string]interface{}), "image")
			if img != "" {
				parts := strings.Split(img, ":")
				if len(parts) > 1 {
					tag := parts[len(parts)-1]
					for _, dt := range p.Spec.DisallowedImageTags {
						if tag == dt {
							add("disallowedImageTags", fmt.Sprintf("container %s uses disallowed tag %s", getContainerName(cont), tag))
						}
					}
				} else {
					for _, dt := range p.Spec.DisallowedImageTags {
						if dt == "latest" {
							add("disallowedImageTags", fmt.Sprintf("container %s uses implicit latest tag", getContainerName(cont)))
						}
					}
				}
			}
		}
	}

	// requiredProbes
	if len(p.Spec.RequiredProbes) > 0 {
		for _, cont := range getStandardContainers(obj) { // only non-init
			cname := getContainerName(cont)
			for _, probe := range p.Spec.RequiredProbes {
				_, found, _ := unstructured.NestedFieldNoCopy(cont.(map[string]interface{}), probe)
				if !found {
					add("requiredProbes", fmt.Sprintf("container %s has no %s", cname, probe))
				}
			}
		}
	}

	// requiredLabels
	if p.Spec.RequiredLabels != nil && len(p.Spec.RequiredLabels.Labels) > 0 {
		watchMatch := len(p.Spec.RequiredLabels.WatchKinds) == 0
		for _, wk := range p.Spec.RequiredLabels.WatchKinds {
			if wk == kind {
				watchMatch = true
				break
			}
		}

		if watchMatch {
			for _, rl := range p.Spec.RequiredLabels.Labels {
				val, ok := objLabels[rl.Key]
				if !ok {
					add("requiredLabels", fmt.Sprintf("missing required label %s", rl.Key))
				} else if rl.AllowedRegex != "" {
					matched, err := regexp.MatchString(rl.AllowedRegex, val)
					if err == nil && !matched {
						add("requiredLabels", fmt.Sprintf("label %s value %s does not match regex %s", rl.Key, val, rl.AllowedRegex))
					}
				}
			}
		}
	}

	// maxRevisionHistoryLimit
	if p.Spec.MaxRevisionHistoryLimit != nil && kind == "Deployment" {
		rhl, found, _ := unstructured.NestedInt64(obj.Object, "spec", "revisionHistoryLimit")
		if !found {
			// default is 10
			if 10 > *p.Spec.MaxRevisionHistoryLimit {
				add("maxRevisionHistoryLimit", fmt.Sprintf("default revisionHistoryLimit (10) > max (%d)", *p.Spec.MaxRevisionHistoryLimit))
			}
		} else if int32(rhl) > *p.Spec.MaxRevisionHistoryLimit {
			add("maxRevisionHistoryLimit", fmt.Sprintf("revisionHistoryLimit (%d) > max (%d)", rhl, *p.Spec.MaxRevisionHistoryLimit))
		}
	}

	// imagePullPolicy
	if p.Spec.ImagePullPolicy != nil {
		for _, cont := range getContainers(obj) {
			ipp, _, _ := unstructured.NestedString(cont.(map[string]interface{}), "imagePullPolicy")
			if ipp != "" && ipp != *p.Spec.ImagePullPolicy {
				add("imagePullPolicy", fmt.Sprintf("container %s has imagePullPolicy %s, required %s", getContainerName(cont), ipp, *p.Spec.ImagePullPolicy))
			} else if ipp == "" {
				add("imagePullPolicy", fmt.Sprintf("container %s missing imagePullPolicy, required %s", getContainerName(cont), *p.Spec.ImagePullPolicy))
			}
		}
	}

	// priorityClassNames
	if len(p.Spec.PriorityClassNames) > 0 {
		if hasPodTemplate(obj) {
			pcn, _, _ := unstructured.NestedString(obj.Object, "spec", "template", "spec", "priorityClassName")
			if pcn != "" {
				allowed := false
				for _, a := range p.Spec.PriorityClassNames {
					if a == pcn {
						allowed = true
					}
				}
				if !allowed {
					add("priorityClassNames", fmt.Sprintf("priorityClassName %s not allowed", pcn))
				}
			} else {
				add("priorityClassNames", "priorityClassName is required but missing")
			}
		} else if kind == "Pod" {
			pcn, _, _ := unstructured.NestedString(obj.Object, "spec", "priorityClassName")
			if pcn != "" {
				allowed := false
				for _, a := range p.Spec.PriorityClassNames {
					if a == pcn {
						allowed = true
					}
				}
				if !allowed {
					add("priorityClassNames", fmt.Sprintf("priorityClassName %s not allowed", pcn))
				}
			} else {
				add("priorityClassNames", "priorityClassName is required but missing")
			}
		}
	}

	// ingressClassNames
	if len(p.Spec.IngressClassNames) > 0 && kind == "Ingress" {
		icn, _, _ := unstructured.NestedString(obj.Object, "spec", "ingressClassName")
		if icn != "" {
			allowed := false
			for _, a := range p.Spec.IngressClassNames {
				if a == icn {
					allowed = true
				}
			}
			if !allowed {
				add("ingressClassNames", fmt.Sprintf("ingressClassName %s not allowed", icn))
			}
		} else {
			// also check annotations kubernetes.io/ingress.class
			anns := obj.GetAnnotations()
			if anns != nil {
				icn = anns["kubernetes.io/ingress.class"]
				if icn != "" {
					allowed := false
					for _, a := range p.Spec.IngressClassNames {
						if a == icn {
							allowed = true
						}
					}
					if !allowed {
						add("ingressClassNames", fmt.Sprintf("ingress class annotation %s not allowed", icn))
					}
				}
			}
			if icn == "" {
				add("ingressClassNames", "ingressClassName is required but missing")
			}
		}
	}

	// storageClassNames
	if len(p.Spec.StorageClassNames) > 0 {
		if kind == "PersistentVolumeClaim" {
			scn, _, _ := unstructured.NestedString(obj.Object, "spec", "storageClassName")
			if scn != "" {
				allowed := false
				for _, a := range p.Spec.StorageClassNames {
					if a == scn {
						allowed = true
					}
				}
				if !allowed {
					add("storageClassNames", fmt.Sprintf("storageClassName %s not allowed", scn))
				}
			}
		} else if kind == "StatefulSet" {
			vcts, found, _ := unstructured.NestedSlice(obj.Object, "spec", "volumeClaimTemplates")
			if found {
				for _, vctIface := range vcts {
					vct, ok := vctIface.(map[string]interface{})
					if !ok {
						continue
					}
					scn, _, _ := unstructured.NestedString(vct, "spec", "storageClassName")
					if scn != "" {
						allowed := false
						for _, a := range p.Spec.StorageClassNames {
							if a == scn {
								allowed = true
							}
						}
						if !allowed {
							add("storageClassNames", fmt.Sprintf("storageClassName %s in volumeClaimTemplates not allowed", scn))
						}
					}
				}
			}
		}
	}

	// replicaLimits
	if p.Spec.ReplicaLimits != nil && hasReplicas(kind) {
		reps, found, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas")
		if !found {
			reps = 1 // default is 1 for Deployments/StatefulSets
		}

		if p.Spec.ReplicaLimits.MinReplicas != nil && int32(reps) < *p.Spec.ReplicaLimits.MinReplicas {
			add("replicaLimits", fmt.Sprintf("replicas (%d) < min (%d)", reps, *p.Spec.ReplicaLimits.MinReplicas))
		}
		if p.Spec.ReplicaLimits.MaxReplicas != nil && int32(reps) > *p.Spec.ReplicaLimits.MaxReplicas {
			add("replicaLimits", fmt.Sprintf("replicas (%d) > max (%d)", reps, *p.Spec.ReplicaLimits.MaxReplicas))
		}
	}

	if p.Spec.DisallowedTolerations != nil || p.Spec.RequiredAnnotations != nil || p.Spec.CheckHostNetworkDNSPolicy != nil || p.Spec.CheckContainerDuplicates != nil || p.Spec.GpuResourceRestriction != nil {
		add("unsupported", "not checked by dhg")
	}

	return findings
}

func RenderMarkdownSection(findings []Finding) string {
	if len(findings) == 0 {
		return "No findings."
	}

	var buf strings.Builder
	buf.WriteString("| Object | Policy | Rule | Finding |\n")
	buf.WriteString("|---|---|---|---|\n")

	for _, f := range findings {
		if f.Rule == "unsupported" {
			// Skip unsupported, or add note
			continue
		}
		fmt.Fprintf(&buf, "| %s | %s | %s | %s |\n", f.ObjectName, f.PolicyName, f.Rule, f.Message)
	}

	// Add ignored
	hasUnsupp := false
	for _, f := range findings {
		if f.Rule == "unsupported" {
			hasUnsupp = true
			break
		}
	}
	if hasUnsupp {
		buf.WriteString("\n*Some policy rules are not checked by dhg.*\n")
	}

	return buf.String()
}

// Helpers

func hasPodTemplate(obj *unstructured.Unstructured) bool {
	kind := obj.GetKind()
	return kind == "Deployment" || kind == "DaemonSet" || kind == "StatefulSet" || kind == "Job" || kind == "CronJob"
}

func hasReplicas(kind string) bool {
	return kind == "Deployment" || kind == "StatefulSet" || kind == "ReplicaSet"
}

func getContainers(obj *unstructured.Unstructured) []interface{} {
	var c []interface{}
	c = append(c, getStandardContainers(obj)...)
	c = append(c, getInitContainers(obj)...)
	return c
}

func getStandardContainers(obj *unstructured.Unstructured) []interface{} {
	var spec map[string]interface{}

	kind := obj.GetKind()
	if kind == "Pod" {
		spec, _, _ = unstructured.NestedMap(obj.Object, "spec")
	} else if kind == "CronJob" {
		spec, _, _ = unstructured.NestedMap(obj.Object, "spec", "jobTemplate", "spec", "template", "spec")
	} else if hasPodTemplate(obj) {
		spec, _, _ = unstructured.NestedMap(obj.Object, "spec", "template", "spec")
	}

	if spec != nil {
		if c, found, _ := unstructured.NestedSlice(spec, "containers"); found {
			return c
		}
	}
	return nil
}

func getInitContainers(obj *unstructured.Unstructured) []interface{} {
	var spec map[string]interface{}

	kind := obj.GetKind()
	if kind == "Pod" {
		spec, _, _ = unstructured.NestedMap(obj.Object, "spec")
	} else if kind == "CronJob" {
		spec, _, _ = unstructured.NestedMap(obj.Object, "spec", "jobTemplate", "spec", "template", "spec")
	} else if hasPodTemplate(obj) {
		spec, _, _ = unstructured.NestedMap(obj.Object, "spec", "template", "spec")
	}

	if spec != nil {
		if c, found, _ := unstructured.NestedSlice(spec, "initContainers"); found {
			return c
		}
	}
	return nil
}

func getContainerName(c interface{}) string {
	cm, ok := c.(map[string]interface{})
	if !ok {
		return ""
	}
	n, _, _ := unstructured.NestedString(cm, "name")
	return n
}
