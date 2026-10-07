package synth

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Dockerfile holds the instructions of a Dockerfile's final stage that
// describe how the container runs.
type Dockerfile struct {
	Expose      []string // "8080", "53/udp"
	User        string
	Volumes     []string
	Healthcheck []string // Docker form: CMD …, CMD-SHELL …, NONE
	Interval    int64    // nanoseconds
	Timeout     int64
	StartPeriod int64
	Retries     int64
	// Unresolved lists EXPOSE arguments with variables that have no default.
	Unresolved []string
}

var dockerVar = regexp.MustCompile(`\$\{?([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}?`)

// ParseDockerfile reads the final stage of a Dockerfile. Line continuations
// and comments are handled; ARG and ENV defaults are substituted in EXPOSE.
func ParseDockerfile(content string) Dockerfile {
	var df Dockerfile
	vars := map[string]string{}
	for _, line := range logicalLines(content) {
		instr, rest, _ := strings.Cut(line, " ")
		rest = strings.TrimSpace(rest)
		switch strings.ToUpper(instr) {
		case "FROM":
			df = Dockerfile{}
			vars = map[string]string{}
		case "ARG":
			k, v, _ := strings.Cut(rest, "=")
			vars[k] = strings.Trim(v, `"'`)
		case "ENV":
			for k, v := range envPairs(rest) {
				vars[k] = v
			}
		case "EXPOSE":
			for _, p := range strings.Fields(rest) {
				resolved := dockerVar.ReplaceAllStringFunc(p, func(m string) string {
					sm := dockerVar.FindStringSubmatch(m)
					if v, ok := vars[sm[1]]; ok && v != "" {
						return v
					}
					return sm[2]
				})
				if resolved == "" || strings.Contains(resolved, "$") {
					df.Unresolved = append(df.Unresolved, p)
					continue
				}
				df.Expose = append(df.Expose, resolved)
			}
		case "USER":
			df.User = rest
		case "VOLUME":
			df.Volumes = append(df.Volumes, jsonOrFields(rest)...)
		case "HEALTHCHECK":
			df.Healthcheck, df.Interval, df.Timeout, df.StartPeriod, df.Retries = nil, 0, 0, 0, 0
			args := rest
			for strings.HasPrefix(args, "--") {
				opt, after, _ := strings.Cut(args, " ")
				args = strings.TrimSpace(after)
				k, v, _ := strings.Cut(strings.TrimPrefix(opt, "--"), "=")
				d, _ := time.ParseDuration(v)
				switch k {
				case "interval":
					df.Interval = int64(d)
				case "timeout":
					df.Timeout = int64(d)
				case "start-period":
					df.StartPeriod = int64(d)
				case "retries":
					df.Retries, _ = strconv.ParseInt(v, 10, 64)
				}
			}
			kind, cmd, _ := strings.Cut(args, " ")
			switch strings.ToUpper(kind) {
			case "NONE":
				df.Healthcheck = []string{"NONE"}
			case "CMD":
				cmd = strings.TrimSpace(cmd)
				var exec []string
				if json.Unmarshal([]byte(cmd), &exec) == nil {
					df.Healthcheck = append([]string{"CMD"}, exec...)
				} else {
					df.Healthcheck = []string{"CMD-SHELL", cmd}
				}
			}
		}
	}
	return df
}

// logicalLines joins continuation lines and drops comments and blanks.
func logicalLines(content string) []string {
	var out []string
	var cur strings.Builder
	for _, raw := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "#") || (line == "" && cur.Len() == 0) {
			continue
		}
		if strings.HasSuffix(line, "\\") {
			cur.WriteString(strings.TrimSuffix(line, "\\"))
			cur.WriteString(" ")
			continue
		}
		cur.WriteString(line)
		if s := strings.TrimSpace(cur.String()); s != "" {
			out = append(out, strings.Join(strings.Fields(s), " "))
		}
		cur.Reset()
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		out = append(out, s)
	}
	return out
}

// envPairs parses "ENV k=v k2=v2" and the legacy "ENV k v".
func envPairs(rest string) map[string]string {
	out := map[string]string{}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return out
	}
	if !strings.Contains(fields[0], "=") {
		k, v, _ := strings.Cut(rest, " ")
		out[k] = strings.TrimSpace(v)
		return out
	}
	for _, w := range words(rest) {
		if k, v, ok := strings.Cut(w, "="); ok {
			out[k] = v
		}
	}
	return out
}

func jsonOrFields(s string) []string {
	var list []string
	if json.Unmarshal([]byte(s), &list) == nil {
		return list
	}
	return strings.Fields(s)
}
