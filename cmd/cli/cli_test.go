package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// run drives a fresh command tree and captures its output. NO_COLOR keeps the
// assertions readable by stripping ANSI escapes at the source.
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	t.Setenv("NO_COLOR", "1")
	var buf bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func mustRun(t *testing.T, args ...string) string {
	t.Helper()
	out, err := run(t, args...)
	if err != nil {
		t.Fatalf("hck %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func TestVersion(t *testing.T) {
	out := mustRun(t, "version")
	if !strings.HasPrefix(out, "hck ") {
		t.Fatalf("got %q", out)
	}
}

func TestList(t *testing.T) {
	all := mustRun(t, "list")
	for _, want := range []string{"PRESETS", "RESOURCES", "web", "deployment", "[crd]"} {
		if !strings.Contains(all, want) {
			t.Errorf("hck list output is missing %q", want)
		}
	}

	presets := mustRun(t, "list", "presets")
	if strings.Contains(presets, "RESOURCES") {
		t.Error("hck list presets printed the resources section")
	}

	resources := mustRun(t, "list", "resources")
	if strings.Contains(resources, "PRESETS") {
		t.Error("hck list resources printed the presets section")
	}

	if _, err := run(t, "list", "nonsense"); err == nil {
		t.Error("want an error for an unknown list target")
	}
}

func TestNewAndAddAndCheck(t *testing.T) {
	dir := t.TempDir()

	out := mustRun(t, "new", "demo", "--dir", dir, "--preset", "web")
	if !strings.Contains(out, "templates/deployment.yaml") {
		t.Errorf("new did not report the files it wrote:\n%s", out)
	}
	chartDir := filepath.Join(dir, "demo")
	if _, err := os.Stat(filepath.Join(chartDir, "Chart.yaml")); err != nil {
		t.Fatal("Chart.yaml was not written")
	}

	before, err := os.ReadFile(filepath.Join(chartDir, "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	out = mustRun(t, "add", "servicemonitor", "--chart", chartDir)
	if !strings.Contains(out, "metrics") {
		t.Errorf("add did not report the values key it appended:\n%s", out)
	}
	after, err := os.ReadFile(filepath.Join(chartDir, "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(after), string(before)) {
		t.Fatal("add rewrote the existing values.yaml")
	}

	out = mustRun(t, "add", "servicemonitor", "--chart", chartDir)
	if !strings.Contains(out, "nothing to do") {
		t.Errorf("a repeated add was not reported as a no-op:\n%s", out)
	}

	out = mustRun(t, "check", "--chart", chartDir, "--no-render")
	if !strings.Contains(out, "no findings") {
		t.Errorf("a freshly generated chart has layout findings:\n%s", out)
	}
}

// The whole point of --dry-run is that it touches nothing.
func TestNewDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	out := mustRun(t, "new", "demo", "--dir", dir, "--dry-run")
	if !strings.Contains(out, "dry run") {
		t.Errorf("output does not say it was a dry run:\n%s", out)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("--dry-run created %d entries", len(entries))
	}
}

func TestAddDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, "new", "demo", "--dir", dir)
	chartDir := filepath.Join(dir, "demo")

	before, _ := os.ReadFile(filepath.Join(chartDir, "values.yaml"))
	mustRun(t, "add", "pvc", "--chart", chartDir, "--dry-run")
	after, _ := os.ReadFile(filepath.Join(chartDir, "values.yaml"))

	if string(before) != string(after) {
		t.Error("--dry-run modified values.yaml")
	}
	if _, err := os.Stat(filepath.Join(chartDir, "templates", "pvc.yaml")); err == nil {
		t.Error("--dry-run wrote a template file")
	}
}

func TestNewWithExtraResources(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, "new", "demo", "--dir", dir, "--preset", "worker", "--with", "httproute,servicemonitor")
	for _, f := range []string{"httproute.yaml", "servicemonitor.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, "demo", "templates", f)); err != nil {
			t.Errorf("--with did not add %s", f)
		}
	}
}

func TestNewRejectsBadNames(t *testing.T) {
	dir := t.TempDir()
	if _, err := run(t, "new", "Demo", "--dir", dir); err == nil {
		t.Error("want an error for an uppercase chart name")
	}
	if _, err := run(t, "new", "demo", "--dir", dir, "--preset", "nope"); err == nil {
		t.Error("want an error for an unknown preset")
	}
}

func TestAddOutsideAChart(t *testing.T) {
	if _, err := run(t, "add", "pdb", "--chart", t.TempDir()); err == nil {
		t.Skip("a Chart.yaml exists somewhere above the temp dir on this machine")
	}
}

func TestCheckRendersTheGeneratedChart(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not on PATH")
	}
	dir := t.TempDir()
	for _, preset := range []string{"web", "worker", "cronjob", "stateful"} {
		t.Run(preset, func(t *testing.T) {
			mustRun(t, "new", preset, "--dir", dir, "--preset", preset)
			out := mustRun(t, "check", "--chart", filepath.Join(dir, preset))
			if !strings.Contains(out, "no findings") {
				t.Fatalf("a chart hck just generated does not pass its own check:\n%s", out)
			}
		})
	}
}

func TestCheckWithoutHelmSuggestsNoRender(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	dir := t.TempDir()
	mustRun(t, "new", "demo", "--dir", dir)
	_, err := run(t, "check", "--chart", filepath.Join(dir, "demo"))
	if err == nil || !strings.Contains(err.Error(), "--no-render") {
		t.Fatalf("got %v, want a hint about --no-render", err)
	}
}

func TestSplitList(t *testing.T) {
	got := splitList([]string{"a,b", " c ", "", "d,,e"})
	if strings.Join(got, "|") != "a|b|c|d|e" {
		t.Fatalf("got %v", got)
	}
	if splitList(nil) != nil {
		t.Error("nil input should yield nil")
	}
}
