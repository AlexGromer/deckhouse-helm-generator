package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// captureStdout returns what fn prints to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	os.Stdout = old
	_ = w.Close()
	return <-done
}

func TestValidate_PositionalPath(t *testing.T) {
	chart := writeChart(t, map[string]string{"cm.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: x\n"})

	run := func(args ...string) error {
		cmd := newValidateCmd()
		cmd.SetArgs(args)
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		return cmd.Execute()
	}

	if err := run(chart); err != nil {
		t.Errorf("positional chart dir must be validated, got %v", err)
	}
	if err := run(t.TempDir()); err == nil {
		t.Error("an empty dir given positionally must fail (no Chart.yaml); the cwd was validated instead")
	}
	if err := run(chart, chart); err != nil {
		t.Errorf("several positional dirs: %v", err)
	}
	if err := run("-f", chart, chart); err == nil || !strings.Contains(err.Error(), "positional") {
		t.Errorf("both -f and positional must be an error, got %v", err)
	}
}

func TestValidate_VersionGatedBranchIsWarning(t *testing.T) {
	ingress := `{{- if semverCompare ">=1.19-0" .Capabilities.KubeVersion.GitVersion -}}
apiVersion: networking.k8s.io/v1
{{- else if semverCompare ">=1.14-0" .Capabilities.KubeVersion.GitVersion -}}
apiVersion: networking.k8s.io/v1beta1
{{- else -}}
apiVersion: extensions/v1beta1
{{- end }}
kind: Ingress
metadata:
  name: x
`
	chart := writeChart(t, map[string]string{"ingress.yaml": ingress})
	if err := runValidate(context.Background(), validateOptions{paths: []string{chart}, kubeVersions: "1.27-1.32"}); err != nil {
		t.Errorf("removed API in a version-gated branch must be a warning, got %v", err)
	}

	// The same API behind a condition that is not version-based stays an error.
	plain := writeChart(t, map[string]string{"ingress.yaml": "{{- if .Values.ingress.enabled }}\napiVersion: extensions/v1beta1\nkind: Ingress\n{{- end }}\n"})
	if err := runValidate(context.Background(), validateOptions{paths: []string{plain}, kubeVersions: "1.27-1.32"}); err == nil {
		t.Error("removed API behind a non-version condition must stay an error")
	}
}

func TestAPIVersionGated(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"apiVersion: v1\nkind: X\n", false},
		{"{{ if .Values.a }}\napiVersion: v1\n{{ end }}\n", false},
		{"{{ if .Capabilities.APIVersions.Has \"x/v1\" }}\napiVersion: x/v1\n{{ end }}\n", true},
		{"{{ if .Capabilities.KubeVersion.Major }}\n{{ end }}\napiVersion: v1\nkind: X\n", false},
		{"{{ if semverCompare \">=1.1\" .Capabilities.KubeVersion.Version }}a{{ else }}\napiVersion: v1\n{{ end }}\n", true},
	}
	for _, c := range cases {
		if got := apiVersionGated(c.text); got != c.want {
			t.Errorf("apiVersionGated(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestDiffCmd_ExitCode(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir1, "values.yaml"), []byte("a: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "values.yaml"), []byte("a: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := diffOptions{dir1: dir1, dir2: dir2}
	if err := runDiff(context.Background(), opts); err != nil {
		t.Errorf("differences without --exit-code must not fail, got %v", err)
	}
	opts.exitCode = true
	err := runDiff(context.Background(), opts)
	if err == nil {
		t.Fatal("--exit-code with differences must fail")
	}
	var ec exitCodeError
	if !errors.As(err, &ec) || ec.code != 1 {
		t.Errorf("want exitCodeError{1}, got %#v", err)
	}

	if err := os.WriteFile(filepath.Join(dir2, "values.yaml"), []byte("a: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runDiff(context.Background(), opts); err != nil {
		t.Errorf("--exit-code on identical dirs must succeed, got %v", err)
	}
}

// With --exit-code a real error exits 2 (like diff(1)), so CI can tell it
// from "differences" (1); without the flag errors stay plain errors.
func TestDiffCmd_ExitCodeRealErrorIsTwo(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	other := t.TempDir()

	err := runDiff(context.Background(), diffOptions{dir1: missing, dir2: other, exitCode: true})
	var ec exitCodeError
	if !errors.As(err, &ec) || ec.code != 2 || !strings.Contains(ec.Error(), "cannot access") {
		t.Errorf("want exitCodeError{2, cannot access...}, got %#v", err)
	}

	err = runDiff(context.Background(), diffOptions{dir1: missing, dir2: other})
	if err == nil || errors.As(err, &ec) {
		t.Errorf("without --exit-code want a plain error, got %#v", err)
	}
}

// Differences are not an error to print: stderr stays empty, the exit code
// carries the result.
func TestDiffCmd_DifferencesPrintNoError(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir1, "values.yaml"), []byte("a: 1\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir2, "values.yaml"), []byte("a: 2\n"), 0o644)

	root := newRootCmd()
	errBuf := &strings.Builder{}
	root.SetErr(errBuf)
	root.SetOut(io.Discard)
	root.SetArgs([]string{"diff", "--exit-code", dir1, dir2})
	var err error
	captureStdout(t, func() { err = root.Execute() })
	var ec exitCodeError
	if !errors.As(err, &ec) || ec.code != 1 {
		t.Fatalf("want exitCodeError{1}, got %#v", err)
	}
	if errBuf.Len() != 0 {
		t.Errorf("differences must not print an error, got %q", errBuf.String())
	}

	// A real error is still reported on stderr.
	errBuf.Reset()
	root = newRootCmd()
	root.SetErr(errBuf)
	root.SetArgs([]string{"diff", "--exit-code", filepath.Join(dir1, "nope"), dir2})
	err = root.Execute()
	if !errors.As(err, &ec) || ec.code != 2 {
		t.Fatalf("want exitCodeError{2}, got %#v", err)
	}
	if !strings.Contains(errBuf.String(), "cannot access") {
		t.Errorf("real error must be printed, got %q", errBuf.String())
	}
}

// Usage errors (arguments, flags, --color) are printed, and with --exit-code
// they are "trouble" (2), never "differences" (1).
func TestDiffCmd_UsageErrorsArePrinted(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name     string
		args     []string
		wantCode int // 0: any non-exitCodeError error
		wantMsg  string
	}{
		{"one arg", []string{"diff", dir}, 0, "accepts 2 arg(s)"},
		{"one arg exit-code", []string{"diff", "--exit-code", dir}, 2, "accepts 2 arg(s)"},
		{"unknown flag exit-code", []string{"diff", "--exit-code", "--nosuch", dir, dir}, 2, "unknown flag"},
		{"bad color exit-code", []string{"diff", "--exit-code", "--color=bogus", dir, dir}, 2, "invalid --color"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newRootCmd()
			errBuf := &strings.Builder{}
			root.SetErr(errBuf)
			root.SetOut(io.Discard)
			root.SetArgs(tc.args)
			err := root.Execute()
			if err == nil {
				t.Fatal("want an error")
			}
			var ec exitCodeError
			isEC := errors.As(err, &ec)
			if tc.wantCode != 0 && (!isEC || ec.code != tc.wantCode) {
				t.Errorf("want exitCodeError{%d}, got %#v", tc.wantCode, err)
			}
			if tc.wantCode == 0 && isEC {
				t.Errorf("without --exit-code want a plain error, got %#v", err)
			}
			if !strings.Contains(errBuf.String(), tc.wantMsg) {
				t.Errorf("error must be printed with %q, got %q", tc.wantMsg, errBuf.String())
			}
		})
	}
}

func TestDiffCmd_ColorAuto(t *testing.T) {
	cmd := newDiffCmd()
	if f := cmd.Flags().Lookup("color"); f == nil || f.DefValue != "auto" {
		t.Fatalf("--color must default to auto, got %+v", f)
	}
	for in, want := range map[string]bool{"always": true, "never": false, "true": true, "false": false} {
		got, err := resolveColor(in, false)
		if err != nil || got != want {
			t.Errorf("resolveColor(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if got, _ := resolveColor("auto", false); got {
		t.Error("auto on a non-terminal must disable color")
	}
	if got, _ := resolveColor("auto", true); !got {
		t.Error("auto on a terminal must enable color")
	}
	if _, err := resolveColor("bogus", false); err == nil {
		t.Error("unknown --color value must be an error")
	}
}

func TestDiffCmd_NoNewlineMarkerOnlyWhenMissing(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir1, "f.yaml"), []byte("a: 1\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir2, "f.yaml"), []byte("a: 2\n"), 0o644)
	out := captureStdout(t, func() {
		_ = runDiff(context.Background(), diffOptions{dir1: dir1, dir2: dir2})
	})
	if strings.Contains(out, "No newline") {
		t.Errorf("files end with a newline, no marker expected:\n%s", out)
	}
	if !strings.Contains(out, "-a: 1") || !strings.Contains(out, "+a: 2") {
		t.Errorf("diff body missing:\n%s", out)
	}
}

func TestInstallHint(t *testing.T) {
	mk := func(names ...string) []*types.GeneratedChart {
		var cs []*types.GeneratedChart
		for _, n := range names {
			cs = append(cs, &types.GeneratedChart{Name: n, ChartYAML: "name: " + n + "\ntype: application\n"})
		}
		return cs
	}

	var sb strings.Builder
	printInstallHint(&sb, "/out", mk("web"), "app", "prod", "")
	if want := "helm install my-release /out/web --namespace prod\n"; !strings.Contains(sb.String(), want) {
		t.Errorf("missing %q in:\n%s", want, sb.String())
	}

	// Separate mode: <out>/<chartName> does not exist, list the real charts.
	sb.Reset()
	printInstallHint(&sb, "/out", mk("a", "b", "c", "d", "e"), "app", "", "")
	got := sb.String()
	// One release name per chart: the same name twice fails in one namespace.
	for _, want := range []string{"helm install a /out/a ", "helm install b /out/b ", "helm install c /out/c ", "--namespace <namespace>", "2 more"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "my-release") || strings.Contains(got, "/out/app") || strings.Contains(got, "/out/d ") {
		t.Errorf("unexpected path in:\n%s", got)
	}

	// The library chart and umbrella subcharts are not installable on their own.
	sb.Reset()
	cs := mk("app", "app/charts/x")
	cs = append(cs, &types.GeneratedChart{Name: "library", ChartYAML: "name: library\ntype: library\n"})
	printInstallHint(&sb, "/out", cs, "app", "", "")
	got = sb.String()
	if !strings.Contains(got, "/out/app --namespace") || strings.Contains(got, "library") || strings.Contains(got, "charts/x") {
		t.Errorf("unexpected hint:\n%s", got)
	}
}
