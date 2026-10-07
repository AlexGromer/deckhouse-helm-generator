package synth

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	yaml "sigs.k8s.io/yaml/goyaml.v3"
)

// maxConfigFile bounds bind-mounted files turned into ConfigMap entries.
const maxConfigFile = 1 << 20

// unsupportedServiceKeys are compose keys with no counterpart in the
// generated manifests; they are reported, not silently dropped.
var unsupportedServiceKeys = []string{
	"networks", "network_mode", "restart", "profiles", "extends", "secrets", "configs",
	"cap_add", "cap_drop", "privileged", "pid", "ipc", "links", "external_links",
	"extra_hosts", "dns", "dns_search", "logging", "ulimits", "sysctls", "devices",
	"hostname", "domainname", "labels", "stop_grace_period", "stop_signal", "init",
	"tmpfs", "shm_size", "security_opt", "userns_mode", "platform",
}

// FromCompose describes the services of docker-compose files. Later files
// override the keys of the same service in earlier ones. env looks up
// variables for ${VAR} interpolation; a .env file next to the first file
// provides defaults.
func FromCompose(paths []string, env func(string) (string, bool), notes *Notes) ([]App, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no compose file given")
	}
	dir := filepath.Dir(paths[0])
	dotenv, _ := readEnvFile(filepath.Join(dir, ".env"))
	lookup := func(name string) (string, bool) {
		if v, ok := env(name); ok {
			return v, true
		}
		v, ok := dotenv[name]
		return v, ok
	}

	services := map[string]map[string]interface{}{}
	var order []string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		// YAML 1.2, as Docker Compose v2 reads it: a service called "off"
		// or "yes" stays a string instead of becoming a boolean.
		var doc map[string]interface{}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		doc = interpolate(doc, lookup, notes).(map[string]interface{})
		for _, key := range []string{"secrets", "configs", "networks"} {
			if _, ok := doc[key]; ok {
				notes.Addf("%s: top-level %q is not carried over", filepath.Base(path), key)
			}
		}
		svcs, _ := doc["services"].(map[string]interface{})
		if len(svcs) == 0 {
			return nil, fmt.Errorf("%s: no services", path)
		}
		names := make([]string, 0, len(svcs))
		for n := range svcs {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			svc, _ := svcs[n].(map[string]interface{})
			if services[n] == nil {
				services[n] = map[string]interface{}{}
				order = append(order, n)
			}
			for k, v := range svc {
				services[n][k] = v
			}
		}
	}

	apps := make([]App, 0, len(order))
	for _, name := range order {
		app, err := composeService(name, services[name], dir, notes)
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}
	return apps, nil
}

func composeService(service string, s map[string]interface{}, dir string, notes *Notes) (App, error) {
	app := App{Name: DNSName(service)}
	if app.Name != service {
		notes.Addf("%s: renamed to %q (DNS-1123); other services reach it under that name", service, app.Name)
	}

	app.Image = str(s["image"])
	if app.Image == "" {
		if _, build := s["build"]; build {
			app.Image = app.Name + ":latest"
			notes.Addf("%s: built from source, not pulled; image set to %q, set the pushed image in values", service, app.Image)
		} else {
			return App{}, fmt.Errorf("compose service %q has neither image nor build", service)
		}
	}

	for _, p := range list(s["ports"]) {
		if err := composePort(&app, p, true, notes); err != nil {
			return App{}, fmt.Errorf("service %q: %w", service, err)
		}
	}
	for _, p := range list(s["expose"]) {
		if err := composePort(&app, p, false, notes); err != nil {
			return App{}, fmt.Errorf("service %q: %w", service, err)
		}
	}

	for _, f := range list(s["env_file"]) {
		path := str(f)
		required := true
		if m, ok := f.(map[string]interface{}); ok {
			path = str(m["path"])
			if r, ok := m["required"].(bool); ok {
				required = r
			}
		}
		vars, err := readEnvFile(filepath.Join(dir, path))
		if err != nil {
			if required {
				return App{}, fmt.Errorf("service %q: env_file: %w", service, err)
			}
			continue
		}
		for _, k := range sortedKeys(vars) {
			app.SetEnv(k, vars[k])
		}
	}
	switch env := s["environment"].(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if env[k] == nil {
				notes.Addf("%s: %s takes its value from the host environment in compose; set it in values", service, k)
			}
			app.SetEnv(k, scalar(env[k]))
		}
	case []interface{}:
		for _, e := range env {
			k, v, hasValue := strings.Cut(str(e), "=")
			if !hasValue {
				notes.Addf("%s: %s takes its value from the host environment in compose; set it in values", service, k)
			}
			app.SetEnv(k, v)
		}
	}
	if len(app.SecretEnv) > 0 {
		names := make([]string, len(app.SecretEnv))
		for i, e := range app.SecretEnv {
			names[i] = e.Name
		}
		notes.Addf("%s: %s went to Secret %s-env with the values from compose; replace them before committing the chart", service, strings.Join(names, ", "), app.Name)
	}

	app.Command = words(s["entrypoint"])
	app.Args = words(s["command"])
	app.WorkingDir = str(s["working_dir"])
	if u := str(s["user"]); u != "" {
		applyUser(&app, u, "compose service", notes)
	}

	for i, v := range list(s["volumes"]) {
		if err := composeVolume(&app, i, v, dir, notes); err != nil {
			return App{}, fmt.Errorf("service %q: %w", service, err)
		}
	}

	if hc, ok := s["healthcheck"].(map[string]interface{}); ok && hc["disable"] != true {
		var test []string
		switch t := hc["test"].(type) {
		case string:
			test = []string{"CMD-SHELL", t}
		case []interface{}:
			for _, x := range t {
				test = append(test, str(x))
			}
		}
		retries, _ := strconv.ParseInt(scalar(hc["retries"]), 10, 64)
		p := healthcheckProbe(test, duration(hc["interval"]), duration(hc["timeout"]), duration(hc["start_period"]), retries)
		if p != nil {
			app.Liveness, app.Readiness = p, copyProbe(p)
		}
	}
	if app.Liveness == nil && len(app.Ports) > 0 {
		notes.Addf("%s: no healthcheck in compose, so no probes; add liveness and readiness probes", service)
	}

	if deploy, ok := s["deploy"].(map[string]interface{}); ok {
		if r, err := strconv.ParseInt(scalar(deploy["replicas"]), 10, 64); err == nil {
			app.Replicas = &r
		}
		if res, ok := deploy["resources"].(map[string]interface{}); ok {
			app.Resources = &Resources{
				Limits:   composeResources(res["limits"]),
				Requests: composeResources(res["reservations"]),
			}
		}
	}
	if app.Resources == nil {
		notes.Addf("%s: no deploy.resources in compose; set requests and limits in values", service)
	}

	if deps := list(s["depends_on"]); len(deps) > 0 || s["depends_on"] != nil {
		notes.Addf("%s: depends_on is not enforced by Kubernetes; the app must retry until its dependencies are ready", service)
	}
	var dropped []string
	for _, k := range unsupportedServiceKeys {
		if _, ok := s[k]; ok {
			dropped = append(dropped, k)
		}
	}
	if len(dropped) > 0 {
		notes.Addf("%s: not carried over: %s", service, strings.Join(dropped, ", "))
	}
	return app, nil
}

var portSpec = regexp.MustCompile(`^(?:(?:\[[^\]]*\]|[^:]*):)??(?:(\d+(?:-\d+)?):)?(\d+(?:-\d+)?)(?:/(tcp|udp|sctp))?$`)

// composePort adds a "ports" or "expose" entry: the container (target) port
// is exposed; host ports only matter on a single docker host.
func composePort(app *App, p interface{}, published bool, notes *Notes) error {
	var target, proto, host string
	switch v := p.(type) {
	case map[string]interface{}:
		target, proto, host = scalar(v["target"]), str(v["protocol"]), scalar(v["published"])
	default:
		spec := scalar(v)
		m := portSpec.FindStringSubmatch(spec)
		if m == nil {
			return fmt.Errorf("invalid port %q", spec)
		}
		host, target, proto = m[1], m[2], m[3]
	}
	from, to, isRange := strings.Cut(target, "-")
	first, err := strconv.ParseInt(from, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid port %q", target)
	}
	last := first
	if isRange {
		if last, err = strconv.ParseInt(to, 10, 64); err != nil || last < first || last-first > 100 {
			return fmt.Errorf("invalid port range %q", target)
		}
	}
	for n := first; n <= last; n++ {
		if n < 1 || n > 65535 {
			return fmt.Errorf("port %d out of range", n)
		}
		app.AddPort(n, proto)
	}
	if published && host != "" && host != target {
		notes.Addf("%s: host port %s maps to %s in compose; inside the cluster the Service exposes %s", app.Name, host, target, target)
	}
	return nil
}

// composeVolume adds a "volumes" entry: named volumes become PVCs, small
// bind-mounted files ConfigMap entries, anything else an emptyDir.
func composeVolume(app *App, index int, v interface{}, dir string, notes *Notes) error {
	var kind, source, target string
	readOnly := false
	switch e := v.(type) {
	case map[string]interface{}:
		kind, source, target = str(e["type"]), str(e["source"]), str(e["target"])
		readOnly, _ = e["read_only"].(bool)
	default:
		parts := strings.Split(str(v), ":")
		switch len(parts) {
		case 1:
			target = parts[0]
		default:
			source, target = parts[0], parts[1]
			if len(parts) > 2 {
				readOnly = strings.Contains(parts[2], "ro")
			}
		}
		switch {
		case source == "":
			kind = "volume"
		case strings.HasPrefix(source, "/") || strings.HasPrefix(source, ".") || strings.HasPrefix(source, "~"):
			kind = "bind"
		default:
			kind = "volume"
		}
	}
	if target == "" {
		return fmt.Errorf("volume %v has no target", v)
	}
	name := fmt.Sprintf("volume-%d", index+1)

	switch kind {
	case "volume":
		if source == "" {
			app.Volumes = append(app.Volumes, Volume{Name: name, MountPath: target, Kind: VolumeEmptyDir, ReadOnly: readOnly})
			notes.Addf("%s: anonymous volume %s is an emptyDir (lost on restart)", app.Name, target)
			return nil
		}
		app.Volumes = append(app.Volumes, Volume{Name: source, MountPath: target, Kind: VolumePVC, ReadOnly: readOnly})
		notes.Addf("%s: named volume %q is PVC %s-%s of 1Gi ReadWriteOnce; set its size and storage class in values", app.Name, source, app.Name, DNSName(source))
	case "bind":
		path := source
		if !filepath.IsAbs(path) && !strings.HasPrefix(path, "~") {
			path = filepath.Join(dir, path)
		}
		info, err := os.Stat(path)
		if err == nil && info.Mode().IsRegular() && info.Size() <= maxConfigFile {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			app.Volumes = append(app.Volumes, Volume{
				Name: name, MountPath: target, Kind: VolumeConfigFile, FileName: filepath.Base(path), Content: string(data),
			})
			return nil
		}
		app.Volumes = append(app.Volumes, Volume{Name: name, MountPath: target, Kind: VolumeEmptyDir, ReadOnly: readOnly})
		notes.Addf("%s: bind mount %s → %s cannot be shipped in a chart (directory, missing or over 1 MiB); it is an emptyDir, provide the content another way", app.Name, source, target)
	default:
		app.Volumes = append(app.Volumes, Volume{Name: name, MountPath: target, Kind: VolumeEmptyDir, ReadOnly: readOnly})
		if kind != "tmpfs" {
			notes.Addf("%s: %s volume %s is an emptyDir", app.Name, kind, target)
		}
	}
	return nil
}

var memoryUnit = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*([bkmgt]?)(?:i?b)?$`)

// composeResources converts deploy.resources limits/reservations: cpus as
// is, memory from compose units (1024-based) to Kubernetes binary units.
func composeResources(v interface{}) map[string]string {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	out := map[string]string{}
	if cpus := scalar(m["cpus"]); cpus != "" {
		out["cpu"] = cpus
	}
	if mem := strings.ToLower(scalar(m["memory"])); mem != "" {
		if u := memoryUnit.FindStringSubmatch(mem); u != nil {
			suffix := map[string]string{"": "", "b": "", "k": "Ki", "m": "Mi", "g": "Gi", "t": "Ti"}[u[2]]
			out["memory"] = u[1] + suffix
		} else {
			out["memory"] = mem
		}
	}
	return out
}

var interpolation = regexp.MustCompile(`\$\$|\$\{([A-Za-z_][A-Za-z0-9_]*)(?:(:?[-?])([^}]*))?\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// interpolate substitutes ${VAR}, ${VAR:-default}, ${VAR-default},
// ${VAR:?err}, ${VAR?err} and $VAR in every string of v; $$ is a literal $.
func interpolate(v interface{}, lookup func(string) (string, bool), notes *Notes) interface{} {
	switch t := v.(type) {
	case string:
		return interpolation.ReplaceAllStringFunc(t, func(match string) string {
			if match == "$$" {
				return "$"
			}
			m := interpolation.FindStringSubmatch(match)
			name, op, arg := m[1], m[2], m[3]
			if name == "" {
				name = m[4]
			}
			val, ok := lookup(name)
			switch op {
			case ":-":
				if !ok || val == "" {
					return arg
				}
			case "-":
				if !ok {
					return arg
				}
			case ":?", "?":
				if !ok || (op == ":?" && val == "") {
					notes.Addf("variable %s is required (%s) but not set; used an empty value", name, arg)
				}
			default:
				if !ok {
					notes.Addf("variable %s is not set; used an empty value", name)
				}
			}
			return val
		})
	case map[string]interface{}:
		for k, x := range t {
			t[k] = interpolate(x, lookup, notes)
		}
		return t
	case []interface{}:
		for i, x := range t {
			t[i] = interpolate(x, lookup, notes)
		}
		return t
	}
	return v
}

// readEnvFile reads KEY=VALUE lines; # starts a comment line, surrounding
// quotes are removed.
func readEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	vars := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		vars[strings.TrimSpace(k)] = v
	}
	return vars, scanner.Err()
}

// duration parses compose durations ("30s", "1m30s"); bare numbers are seconds.
func duration(v interface{}) int64 {
	s := scalar(v)
	if s == "" {
		return 0
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(n * float64(time.Second))
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return int64(d)
}

// words splits a compose command: a list as is, a string like a shell would
// (whitespace-separated, single and double quotes group words).
func words(v interface{}) []string {
	switch t := v.(type) {
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, x := range t {
			out = append(out, scalar(x))
		}
		return out
	case string:
		var out []string
		var cur strings.Builder
		var quote rune
		inWord := false
		for _, r := range t {
			switch {
			case quote != 0 && r == quote:
				quote = 0
			case quote != 0:
				cur.WriteRune(r)
			case r == '"' || r == '\'':
				quote, inWord = r, true
			case r == ' ' || r == '\t' || r == '\n':
				if inWord {
					out = append(out, cur.String())
					cur.Reset()
					inWord = false
				}
			default:
				cur.WriteRune(r)
				inWord = true
			}
		}
		if inWord {
			out = append(out, cur.String())
		}
		return out
	}
	return nil
}

func list(v interface{}) []interface{} {
	switch t := v.(type) {
	case []interface{}:
		return t
	case nil:
		return nil
	case map[string]interface{}:
		// depends_on long syntax and similar maps: the keys.
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]interface{}, len(keys))
		for i, k := range keys {
			out[i] = k
		}
		return out
	default:
		return []interface{}{t}
	}
}

func str(v interface{}) string {
	s, _ := v.(string)
	return s
}

// scalar renders a YAML scalar as compose would read it.
func scalar(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprint(t)
	}
}
