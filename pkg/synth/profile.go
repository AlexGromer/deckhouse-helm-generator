package synth

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// Profile is a platform profile: what an organisation's platform adds to
// every chart that runs on it (init containers, shared ConfigMaps and
// Secrets, labels, probes, resources, database Jobs) and where a project of
// that organisation keeps facts the common conventions do not cover. It is
// read from a YAML file given with --synth-profile; see docs/SYNTH_PROFILE.md.
type Profile struct {
	// Vars are variables for the templates of the profile (.Vars.<name>).
	Vars map[string]string `json:"vars"`
	// EnvFrom are added to the container's envFrom before the app's own
	// ConfigMap and Secret.
	EnvFrom []ProfileEnvFrom `json:"envFrom"`
	// InitContainers are added to the pod; string fields are templates.
	InitContainers []map[string]interface{} `json:"initContainers"`
	// PodLabels and PodAnnotations are added to the pod template,
	// DeploymentLabels to the Deployment itself.
	PodLabels        map[string]string `json:"podLabels"`
	PodAnnotations   map[string]string `json:"podAnnotations"`
	DeploymentLabels map[string]string `json:"deploymentLabels"`
	// Defaults are what the platform runs with when the sources do not say.
	Defaults ProfileDefaults `json:"defaults"`
	// Jobs are added as Helm hooks.
	Jobs []ProfileJob `json:"jobs"`
	// SecretKeyPatterns replace the built-in rule that sends an environment
	// variable to the Secret (regular expressions, case-insensitive).
	SecretKeyPatterns []string `json:"secretKeyPatterns"`
	// DockerfileGlobs and EnvTemplateFiles are searched in addition to the
	// conventional locations (paths relative to the project or build root).
	DockerfileGlobs  []string `json:"dockerfileGlobs"`
	EnvTemplateFiles []string `json:"envTemplateFiles"`
	// Volumes are added to the pod, VolumeMounts to the application container.
	Volumes      []map[string]interface{} `json:"volumes"`
	VolumeMounts []map[string]interface{} `json:"volumeMounts"`
	// Container and Pod are merged into the application container and the
	// pod spec: fields the profile has no dedicated key for (imagePullPolicy,
	// securityContext, lifecycle, imagePullSecrets, nodeSelector, ...).
	// Maps merge key by key, the profile winning; lists are replaced.
	Container map[string]interface{} `json:"container"`
	Pod       map[string]interface{} `json:"pod"`
}

// ProfileEnvFrom is one envFrom entry; exactly one reference is set and its
// value is a template for the ConfigMap or Secret name.
type ProfileEnvFrom struct {
	ConfigMapRef string `json:"configMapRef"`
	SecretRef    string `json:"secretRef"`
}

// ProfileDefaults are the platform's defaults.
type ProfileDefaults struct {
	// Resources apply when the sources give none.
	Resources *ProfileResources `json:"resources"`
	// Port applies when no port was found.
	Port int64 `json:"port"`
	// Probes replace the probes derived from the sources.
	Probes ProfileProbes `json:"probes"`
}

// ProfileResources are container requests and limits.
type ProfileResources struct {
	Requests map[string]interface{} `json:"requests"`
	Limits   map[string]interface{} `json:"limits"`
}

// ProfileProbes are the three container probes.
type ProfileProbes struct {
	Startup   *ProfileProbe `json:"startup"`
	Liveness  *ProfileProbe `json:"liveness"`
	Readiness *ProfileProbe `json:"readiness"`
}

// ProfileProbe is a probe: TCP on the port, HTTP GET of Path, or Exec.
type ProfileProbe struct {
	TCP  bool     `json:"tcp"`
	Path string   `json:"path"`
	Exec []string `json:"exec"`
	// Port is the probe's port; the application's first port when 0.
	Port int64 `json:"port"`

	PeriodSeconds       int64 `json:"periodSeconds"`
	TimeoutSeconds      int64 `json:"timeoutSeconds"`
	FailureThreshold    int64 `json:"failureThreshold"`
	SuccessThreshold    int64 `json:"successThreshold"`
	InitialDelaySeconds int64 `json:"initialDelaySeconds"`
}

// ProfileJob is a Job added as a Helm hook.
type ProfileJob struct {
	// When is liquibase or flyway (the project has such migrations in a
	// sibling module) or always.
	When string `json:"when"`
	// Name is a template for the Job's name.
	Name string `json:"name"`
	// Manifest is the Job: a mapping, or a string holding Job YAML. Every
	// string in it is a template.
	Manifest interface{} `json:"manifest"`
}

// Job is an extra object a profile adds, rendered as a Helm hook.
type Job struct {
	Name   string
	Object map[string]interface{}
}

// EnvFromRef is a reference to a ConfigMap or Secret whose keys all become
// environment variables of the container.
type EnvFromRef struct {
	Secret bool // a Secret, else a ConfigMap
	Name   string
}

var profileFuncs = template.FuncMap{
	"default": func(def, v string) string {
		if v == "" {
			return def
		}
		return v
	},
	"quote":      strconv.Quote,
	"lower":      strings.ToLower,
	"upper":      strings.ToUpper,
	"trim":       strings.TrimSpace,
	"trimPrefix": func(prefix, s string) string { return strings.TrimPrefix(s, prefix) },
	"trimSuffix": func(suffix, s string) string { return strings.TrimSuffix(s, suffix) },
	"replace":    func(old, new, s string) string { return strings.ReplaceAll(s, old, new) },
}

// profileData is what templates see: .Name is the application's name, .Vars
// the variables of the profile.
type profileData struct {
	Name string
	Vars map[string]string
}

// LoadProfile reads a platform profile from a YAML file. Unknown keys are
// errors, so a typo does not pass silently.
func LoadProfile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("synth profile: %w", err)
	}
	return ParseProfile(data, path)
}

// ParseProfile parses and validates a profile; source names it in errors.
func ParseProfile(data []byte, source string) (*Profile, error) {
	var p Profile
	if err := yaml.UnmarshalStrict(data, &p); err != nil {
		return nil, fmt.Errorf("synth profile %s: %w", source, err)
	}
	if err := p.validate(); err != nil {
		return nil, fmt.Errorf("synth profile %s: %w", source, err)
	}
	return &p, nil
}

func (p *Profile) validate() error {
	if _, err := NewSecretRules(p.SecretKeyPatterns); err != nil {
		return fmt.Errorf("secretKeyPatterns: %w", err)
	}
	for _, field := range []struct {
		name  string
		globs []string
	}{{"dockerfileGlobs", p.DockerfileGlobs}, {"envTemplateFiles", p.EnvTemplateFiles}} {
		for _, g := range field.globs {
			if g == "" || !filepath.IsLocal(filepath.FromSlash(g)) {
				return fmt.Errorf("%s: %q must be a relative path inside the project", field.name, g)
			}
		}
	}
	for i, e := range p.EnvFrom {
		if (e.ConfigMapRef == "") == (e.SecretRef == "") {
			return fmt.Errorf("envFrom[%d]: set exactly one of configMapRef and secretRef", i)
		}
	}
	for i, c := range p.InitContainers {
		if s, _ := c["name"].(string); s == "" {
			return fmt.Errorf("initContainers[%d]: name is required", i)
		}
		if s, _ := c["image"].(string); s == "" {
			return fmt.Errorf("initContainers[%d]: image is required", i)
		}
	}
	for i, v := range p.Volumes {
		if s, _ := v["name"].(string); s == "" {
			return fmt.Errorf("volumes[%d]: name is required", i)
		}
	}
	for i, m := range p.VolumeMounts {
		if s, _ := m["name"].(string); s == "" {
			return fmt.Errorf("volumeMounts[%d]: name is required", i)
		}
		if s, _ := m["mountPath"].(string); s == "" {
			return fmt.Errorf("volumeMounts[%d]: mountPath is required", i)
		}
	}
	for key, dedicated := range containerOwned {
		if _, ok := p.Container[key]; ok {
			return fmt.Errorf("container.%s: set by %s", key, dedicated)
		}
	}
	for key, dedicated := range podOwned {
		if _, ok := p.Pod[key]; ok {
			return fmt.Errorf("pod.%s: set by %s", key, dedicated)
		}
	}
	if p.Defaults.Port < 0 || p.Defaults.Port > 65535 {
		return fmt.Errorf("defaults.port: %d is not a port", p.Defaults.Port)
	}
	for name, probe := range map[string]*ProfileProbe{
		"startup": p.Defaults.Probes.Startup, "liveness": p.Defaults.Probes.Liveness, "readiness": p.Defaults.Probes.Readiness,
	} {
		if probe == nil {
			continue
		}
		handlers := 0
		for _, set := range []bool{probe.TCP, probe.Path != "", len(probe.Exec) > 0} {
			if set {
				handlers++
			}
		}
		if handlers != 1 {
			return fmt.Errorf("defaults.probes.%s: set exactly one of tcp, path, exec", name)
		}
	}
	for i, j := range p.Jobs {
		switch j.When {
		case "liquibase", "flyway", "always":
		default:
			return fmt.Errorf("jobs[%d]: when is %q, want liquibase, flyway or always", i, j.When)
		}
		switch j.Manifest.(type) {
		case map[string]interface{}, string:
		default:
			return fmt.Errorf("jobs[%d]: manifest is required (a mapping or a string with Job YAML)", i)
		}
	}
	return p.checkTemplates()
}

// containerOwned and podOwned are fields dhg builds itself or that have a
// dedicated key; the patch keys container and pod must not set them.
var containerOwned = map[string]string{
	"name": "the application", "image": "--image", "ports": "the sources or defaults.port",
	"envFrom": "envFrom", "volumeMounts": "volumeMounts", "resources": "defaults.resources",
	"livenessProbe": "defaults.probes", "readinessProbe": "defaults.probes", "startupProbe": "defaults.probes",
}

var podOwned = map[string]string{
	"containers": "the application", "initContainers": "initContainers", "volumes": "volumes",
}

// checkTemplates parses every string that is a template, so a syntax error
// shows at load time, with its location.
func (p *Profile) checkTemplates() error {
	check := func(path string, v interface{}) error {
		var err error
		walkStrings(path, v, func(at, s string) {
			if err == nil && strings.Contains(s, "{{") {
				if _, e := template.New(at).Funcs(profileFuncs).Option("missingkey=error").Parse(s); e != nil {
					err = fmt.Errorf("%s: %w", at, e)
				}
			}
		})
		return err
	}
	for k, v := range p.PodLabels {
		if err := check("podLabels."+k, v); err != nil {
			return err
		}
	}
	for k, v := range p.PodAnnotations {
		if err := check("podAnnotations."+k, v); err != nil {
			return err
		}
	}
	for k, v := range p.DeploymentLabels {
		if err := check("deploymentLabels."+k, v); err != nil {
			return err
		}
	}
	for i, e := range p.EnvFrom {
		if err := check(fmt.Sprintf("envFrom[%d]", i), e.ConfigMapRef+e.SecretRef); err != nil {
			return err
		}
	}
	for i, c := range p.InitContainers {
		if err := check(fmt.Sprintf("initContainers[%d]", i), c); err != nil {
			return err
		}
	}
	for i, v := range p.Volumes {
		if err := check(fmt.Sprintf("volumes[%d]", i), v); err != nil {
			return err
		}
	}
	for i, m := range p.VolumeMounts {
		if err := check(fmt.Sprintf("volumeMounts[%d]", i), m); err != nil {
			return err
		}
	}
	for name, probe := range map[string]*ProfileProbe{
		"startup": p.Defaults.Probes.Startup, "liveness": p.Defaults.Probes.Liveness, "readiness": p.Defaults.Probes.Readiness,
	} {
		if probe != nil {
			if err := check("defaults.probes."+name+".path", probe.Path); err != nil {
				return err
			}
		}
	}
	if err := check("container", p.Container); err != nil {
		return err
	}
	if err := check("pod", p.Pod); err != nil {
		return err
	}
	for i, j := range p.Jobs {
		if err := check(fmt.Sprintf("jobs[%d].name", i), j.Name); err != nil {
			return err
		}
		if err := check(fmt.Sprintf("jobs[%d].manifest", i), j.Manifest); err != nil {
			return err
		}
	}
	return nil
}

// walkStrings calls fn for every string in v with its path.
func walkStrings(path string, v interface{}, fn func(path, s string)) {
	switch t := v.(type) {
	case string:
		fn(path, t)
	case map[string]interface{}:
		for k, x := range t {
			walkStrings(path+"."+k, x, fn)
		}
	case []interface{}:
		for i, x := range t {
			walkStrings(fmt.Sprintf("%s[%d]", path, i), x, fn)
		}
	}
}

// render executes the template s with data.
func render(path, s string, data profileData) (string, error) {
	if !strings.Contains(s, "{{") {
		return s, nil
	}
	tpl, err := template.New(path).Funcs(profileFuncs).Option("missingkey=error").Parse(s)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	var b strings.Builder
	if err := tpl.Execute(&b, data); err != nil {
		return "", fmt.Errorf("%s: %s", path, templateReason(err, data))
	}
	return b.String(), nil
}

var missingEntry = regexp.MustCompile(`map has no entry for key "([^"]+)"`)

// templateReason words a template error: the usual one is a variable the
// profile does not define.
func templateReason(err error, data profileData) string {
	if m := missingEntry.FindStringSubmatch(err.Error()); m != nil {
		return fmt.Sprintf("variable %q is not set (vars.%s)", m[1], m[1])
	}
	return err.Error()
}

// renderValue renders every string in v and returns a copy; whole numbers
// become int64, as the objects are deep-copied later.
func renderValue(path string, v interface{}, data profileData) (interface{}, error) {
	switch t := v.(type) {
	case string:
		return render(path, t, data)
	case float64:
		if t == math.Trunc(t) && math.Abs(t) < 1<<53 {
			return int64(t), nil
		}
		return t, nil
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, x := range t {
			r, err := renderValue(path+"."+k, x, data)
			if err != nil {
				return nil, err
			}
			out[k] = r
		}
		return out, nil
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, x := range t {
			r, err := renderValue(fmt.Sprintf("%s[%d]", path, i), x, data)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	}
	return v, nil
}

func renderMap(path string, m map[string]interface{}, data profileData) (map[string]interface{}, error) {
	r, err := renderValue(path, m, data)
	if err != nil {
		return nil, err
	}
	return r.(map[string]interface{}), nil
}

func renderStrings(path string, in map[string]string, data profileData) (map[string]string, error) {
	out := make(map[string]string, len(in))
	for k, v := range in {
		r, err := render(path+"."+k, v, data)
		if err != nil {
			return nil, err
		}
		out[k] = r
	}
	return out, nil
}

// ApplyProfile adds what the platform profile says to app: envFrom, init
// containers, labels, annotations, volumes, mounts and Jobs; fills in
// resources and port the sources did not give; replaces the probes the
// profile defines; and re-sorts the environment by the profile's secret
// patterns. Call it before Manifests. A nil profile changes nothing.
func ApplyProfile(app *App, prof *Profile) error {
	if prof == nil {
		return nil
	}
	data := profileData{Name: app.Name, Vars: prof.Vars}
	if data.Vars == nil {
		data.Vars = map[string]string{}
	}

	if len(prof.SecretKeyPatterns) > 0 {
		rules, err := NewSecretRules(prof.SecretKeyPatterns)
		if err != nil {
			return err
		}
		all := append(append([]EnvVar(nil), app.Env...), app.SecretEnv...)
		app.secrets, app.Env, app.SecretEnv = rules, nil, nil
		for _, e := range all {
			app.SetEnv(e.Name, e.Value)
		}
	}

	for i, e := range prof.EnvFrom {
		path := fmt.Sprintf("envFrom[%d]", i)
		name, err := render(path, e.ConfigMapRef+e.SecretRef, data)
		if err != nil {
			return err
		}
		app.EnvFromExtra = append(app.EnvFromExtra, EnvFromRef{Secret: e.SecretRef != "", Name: name})
	}
	for i, c := range prof.InitContainers {
		r, err := renderMap(fmt.Sprintf("initContainers[%d]", i), c, data)
		if err != nil {
			return err
		}
		app.InitContainers = append(app.InitContainers, r)
	}
	for field, target := range map[string]struct {
		in  map[string]string
		out *map[string]string
	}{
		"podLabels":        {prof.PodLabels, &app.PodLabels},
		"podAnnotations":   {prof.PodAnnotations, &app.PodAnnotations},
		"deploymentLabels": {prof.DeploymentLabels, &app.DeploymentLabels},
	} {
		if len(target.in) == 0 {
			continue
		}
		r, err := renderStrings(field, target.in, data)
		if err != nil {
			return err
		}
		if *target.out == nil {
			*target.out = map[string]string{}
		}
		for k, v := range r {
			(*target.out)[k] = v
		}
	}
	for i, v := range prof.Volumes {
		r, err := renderMap(fmt.Sprintf("volumes[%d]", i), v, data)
		if err != nil {
			return err
		}
		app.ExtraVolumes = append(app.ExtraVolumes, r)
	}
	for i, m := range prof.VolumeMounts {
		r, err := renderMap(fmt.Sprintf("volumeMounts[%d]", i), m, data)
		if err != nil {
			return err
		}
		app.ExtraMounts = append(app.ExtraMounts, r)
	}

	if len(prof.Container) > 0 {
		r, err := renderMap("container", prof.Container, data)
		if err != nil {
			return err
		}
		app.ContainerPatch = mergePatch(app.ContainerPatch, r)
	}
	if len(prof.Pod) > 0 {
		r, err := renderMap("pod", prof.Pod, data)
		if err != nil {
			return err
		}
		app.PodPatch = mergePatch(app.PodPatch, r)
	}

	if err := applyDefaults(app, prof.Defaults, data); err != nil {
		return err
	}
	return applyJobs(app, prof.Jobs, data)
}

func applyDefaults(app *App, d ProfileDefaults, data profileData) error {
	if d.Resources != nil && app.Resources == nil {
		app.Resources = &Resources{Requests: quantities(d.Resources.Requests), Limits: quantities(d.Resources.Limits)}
	}
	if d.Port > 0 && len(app.Ports) == 0 {
		app.AddNamedPort("http", d.Port, "TCP")
	}
	for name, spec := range map[string]struct {
		probe  *ProfileProbe
		target **Probe
	}{
		"startup":   {d.Probes.Startup, &app.Startup},
		"liveness":  {d.Probes.Liveness, &app.Liveness},
		"readiness": {d.Probes.Readiness, &app.Readiness},
	} {
		if spec.probe == nil {
			continue
		}
		p := Probe{
			Port: spec.probe.Port, TCP: spec.probe.TCP, Exec: spec.probe.Exec,
			PeriodSeconds: spec.probe.PeriodSeconds, TimeoutSeconds: spec.probe.TimeoutSeconds,
			FailureThreshold: spec.probe.FailureThreshold, SuccessThreshold: spec.probe.SuccessThreshold,
			InitialDelaySeconds: spec.probe.InitialDelaySeconds,
		}
		if len(p.Exec) == 0 {
			if p.Port == 0 {
				if len(app.Ports) == 0 {
					return fmt.Errorf("defaults.probes.%s: %s has no port to probe; set defaults.port or the probe's port", name, app.Name)
				}
				p.Port = app.Ports[0].Port
			}
			if spec.probe.Path != "" {
				path, err := render("defaults.probes."+name+".path", spec.probe.Path, data)
				if err != nil {
					return err
				}
				p.HTTPPath = path
			}
		}
		*spec.target = &p
	}
	return nil
}

func quantities(m map[string]interface{}) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = fmt.Sprint(v)
	}
	return out
}

var hookAnnotation = "helm.sh/hook"

func applyJobs(app *App, jobs []ProfileJob, data profileData) error {
	for i, j := range jobs {
		if j.When != "always" && !contains(app.Migrations, j.When) {
			continue
		}
		path := fmt.Sprintf("jobs[%d]", i)
		var obj map[string]interface{}
		switch m := j.Manifest.(type) {
		case string:
			text, err := render(path+".manifest", m, data)
			if err != nil {
				return err
			}
			if err := yaml.Unmarshal([]byte(text), &obj); err != nil {
				return fmt.Errorf("%s.manifest: %w", path, err)
			}
		case map[string]interface{}:
			r, err := renderMap(path+".manifest", m, data)
			if err != nil {
				return err
			}
			obj = r
		}
		if obj == nil {
			return fmt.Errorf("%s.manifest: empty", path)
		}
		name, err := render(path+".name", j.Name, data)
		if err != nil {
			return err
		}
		if name == "" {
			name, _, _ = unstructured.NestedString(obj, "metadata", "name")
		}
		if name == "" {
			return fmt.Errorf("%s: the Job needs a name (name or manifest.metadata.name)", path)
		}
		name = DNSName(name)
		if _, ok := obj["apiVersion"]; !ok {
			obj["apiVersion"] = "batch/v1"
		}
		if _, ok := obj["kind"]; !ok {
			obj["kind"] = "Job"
		}
		meta, _ := obj["metadata"].(map[string]interface{})
		if meta == nil {
			meta = map[string]interface{}{}
		}
		meta["name"] = name
		labels, _ := meta["labels"].(map[string]interface{})
		if labels == nil {
			labels = map[string]interface{}{}
		}
		if _, ok := labels[nameLabel]; !ok {
			labels[nameLabel] = DNSName(app.Name)
		}
		meta["labels"] = labels
		ann, _ := meta["annotations"].(map[string]interface{})
		if ann == nil {
			ann = map[string]interface{}{}
		}
		if _, ok := ann[hookAnnotation]; !ok {
			ann[hookAnnotation] = "pre-install,pre-upgrade"
			if _, ok := ann["helm.sh/hook-delete-policy"]; !ok {
				ann["helm.sh/hook-delete-policy"] = "before-hook-creation"
			}
		}
		meta["annotations"] = ann
		obj["metadata"] = meta
		app.Jobs = append(app.Jobs, Job{Name: name, Object: obj})
	}
	return nil
}

// mergePatch merges src into dst (which may be nil) and returns it: maps
// merge key by key, anything else is replaced.
func mergePatch(dst, src map[string]interface{}) map[string]interface{} {
	if dst == nil {
		dst = map[string]interface{}{}
	}
	for k, v := range src {
		if sm, ok := v.(map[string]interface{}); ok {
			if dm, ok := dst[k].(map[string]interface{}); ok {
				dst[k] = mergePatch(dm, sm)
				continue
			}
		}
		dst[k] = v
	}
	return dst
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
