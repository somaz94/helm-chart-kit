package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/somaz94/helm-chart-kit/internal/catalog"
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

// "hck new --with <workload>" used to build a chart carrying two of them,
// which hck refuses to do through "hck add" and documents as impossible.
func TestNewRefusesASecondWorkload(t *testing.T) {
	_, err := run(t, "new", "demo", "-d", t.TempDir(), "--preset", "web", "--with", "daemonset")
	if err == nil {
		t.Fatal("want an error for two workloads in one chart")
	}
	if !strings.Contains(err.Error(), "primary workload") {
		t.Errorf("unexpected error: %v", err)
	}
}

// And the rendered form of that chart is reported by check, so one built
// before the refusal existed — or by hand — does not stay invisible.
func TestCheckFlagsTwoWorkloads(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not on PATH")
	}
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent, "--preset", "web")
	dir := filepath.Join(parent, "demo")
	// --force is the documented escape hatch, and it must still produce the
	// chart it promises — the finding belongs to check, not to add.
	mustRun(t, "add", "daemonset", "--chart", dir, "--force")

	out := mustRun(t, "check", "--chart", dir)
	if !strings.Contains(out, "HCK030") {
		t.Errorf("check did not flag two workloads:\n%s", out)
	}
	if _, err := run(t, "check", "--chart", dir, "--strict"); err == nil {
		t.Error("--strict passed a chart with two workloads")
	}
}

func TestCheckRendersTheGeneratedChart(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not on PATH")
	}
	dir := t.TempDir()
	for _, preset := range catalog.PresetNames() {
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

// schemaChart scaffolds a chart carrying a values.schema.json and returns its
// directory.
func schemaChart(t *testing.T, extra ...string) string {
	t.Helper()
	parent := t.TempDir()
	args := append([]string{"new", "demo", "-d", parent, "--schema"}, extra...)
	mustRun(t, args...)
	return filepath.Join(parent, "demo")
}

func TestSchemaPrintsToStdoutByDefault(t *testing.T) {
	dir := schemaChart(t)
	out := mustRun(t, "schema", "--chart", dir)

	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("printed schema is not valid JSON: %v\n%s", err, out)
	}
	if doc["title"] != "demo values" {
		t.Errorf("title = %v", doc["title"])
	}
	// Printing must not touch the chart.
	before, err := os.ReadFile(filepath.Join(dir, "values.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	mustRun(t, "schema", "--chart", dir)
	after, err := os.ReadFile(filepath.Join(dir, "values.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("printing rewrote the chart's schema")
	}
}

func TestSchemaWrite(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent)
	dir := filepath.Join(parent, "demo")
	path := filepath.Join(dir, "values.schema.json")

	if _, err := os.Stat(path); err == nil {
		t.Fatal("hck new wrote a schema without --schema")
	}
	out := mustRun(t, "schema", "--chart", dir, "--write")
	if !strings.Contains(out, "wrote") {
		t.Errorf("got %q", out)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("--write produced no file: %v", err)
	}
	mustRun(t, "schema", "--chart", dir, "--check")
}

func TestSchemaCheckReportsDrift(t *testing.T) {
	dir := schemaChart(t)
	mustRun(t, "schema", "--chart", dir, "--check")

	// Adding a resource without regenerating is exactly the drift --check is
	// for, so simulate it by truncating the schema instead.
	path := filepath.Join(dir, "values.schema.json")
	if err := os.WriteFile(path, []byte(`{"type": "object"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, "schema", "--chart", dir, "--check")
	if err == nil {
		t.Fatalf("want an error for a stale schema, got %q", out)
	}
	if !strings.Contains(err.Error(), "out of date") {
		t.Errorf("error = %v", err)
	}
}

func TestSchemaCheckNeedsASchema(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent)
	_, err := run(t, "schema", "--chart", filepath.Join(parent, "demo"), "--check")
	if err == nil {
		t.Fatal("want an error when the chart has no schema")
	}
	if !strings.Contains(err.Error(), "has no values.schema.json") {
		t.Errorf("error = %v", err)
	}
}

func TestSchemaCheckFollowsTheExistingStrictness(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent, "--schema-strict")
	dir := filepath.Join(parent, "demo")

	// Without an explicit flag, --check has to notice the file is strict.
	mustRun(t, "schema", "--chart", dir, "--check")

	// Asked for the permissive build explicitly, it must report the difference.
	if _, err := run(t, "schema", "--chart", dir, "--check", "--strict=false"); err == nil {
		t.Error("want an error when the requested strictness differs")
	}
}

// The command a --check failure prints has to fix that failure. It used to
// omit --strict, so following the advice rebuilt the file exactly as it was
// and a CI job pinned to --check --strict stayed red forever.
func TestSchemaCheckHintConverges(t *testing.T) {
	dir := schemaChart(t) // permissive on disk

	_, err := run(t, "schema", "--chart", dir, "--check", "--strict")
	if err == nil {
		t.Fatal("want an error comparing a permissive file against a strict build")
	}
	hint := extractHint(t, err.Error())

	// Run exactly what was printed, then the same check has to pass.
	args := append(strings.Fields(hint)[1:], "--chart", dir) // drop the leading "hck"
	mustRun(t, args...)
	mustRun(t, "schema", "--chart", dir, "--check", "--strict")
}

func TestSchemaCheckHintConvergesTheOtherWay(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent, "--schema-strict")
	dir := filepath.Join(parent, "demo")

	_, err := run(t, "schema", "--chart", dir, "--check", "--strict=false")
	if err == nil {
		t.Fatal("want an error comparing a strict file against a permissive build")
	}
	args := append(strings.Fields(extractHint(t, err.Error()))[1:], "--chart", dir)
	mustRun(t, args...)
	mustRun(t, "schema", "--chart", dir, "--check", "--strict=false")
}

// extractHint pulls the "run: <command>" tail out of an error message.
func extractHint(t *testing.T, msg string) string {
	t.Helper()
	_, hint, ok := strings.Cut(msg, "run: ")
	if !ok {
		t.Fatalf("error carries no remediation: %s", msg)
	}
	return strings.TrimSpace(hint)
}

// An unreadable schema must not be mistaken for a permissive one and rewritten.
func TestSchemaWriteSurfacesAnUnreadableFile(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent, "--schema-strict")
	dir := filepath.Join(parent, "demo")
	path := filepath.Join(dir, "values.schema.json")

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "schema", "--chart", dir, "--write"); err == nil {
		t.Error("--write silently rebuilt an unreadable schema instead of reporting it")
	}
}

func TestSchemaStrictClosesTheTopLevel(t *testing.T) {
	dir := schemaChart(t)
	out := mustRun(t, "schema", "--chart", dir, "--strict")
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if got, ok := doc["additionalProperties"].(bool); !ok || got {
		t.Errorf("additionalProperties = %v, want false", doc["additionalProperties"])
	}
}

// The write report names the keys a second resource did not get to describe,
// because the description a user goes looking for came from the other one.
func TestSchemaWriteReportsKeysOwnedByAnotherResource(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent, "--preset", "stateful", "--with", "pvc")
	dir := filepath.Join(parent, "demo")

	out := mustRun(t, "schema", "--chart", dir, "--write")
	// The StatefulSet and the PVC both define persistence; only one wins.
	if !strings.Contains(out, "persistence") {
		t.Errorf("the report does not mention the contested key:\n%s", out)
	}
	if !strings.Contains(out, "key(s) from") {
		t.Errorf("the report does not count the keys it wrote:\n%s", out)
	}
}

// With no contested key there is nothing to report, and the line is omitted.
func TestSchemaWriteOmitsTheOwnershipLineWhenEmpty(t *testing.T) {
	dir := schemaChart(t)
	out := mustRun(t, "schema", "--chart", dir, "--write")
	if strings.Contains(out, "described once") {
		t.Errorf("the ownership line appeared with nothing to say:\n%s", out)
	}
}

func TestSchemaRejectsWriteAndCheckTogether(t *testing.T) {
	dir := schemaChart(t)
	if _, err := run(t, "schema", "--chart", dir, "--write", "--check"); err == nil {
		t.Error("want an error when both are passed")
	}
}

func TestSchemaOutsideAChart(t *testing.T) {
	if _, err := run(t, "schema", "--chart", t.TempDir()); err == nil {
		t.Error("want an error when there is no Chart.yaml")
	}
}

func TestNewSchemaFlagShowsInThePlan(t *testing.T) {
	out := mustRun(t, "new", "demo", "-d", t.TempDir(), "--schema", "--dry-run")
	if !strings.Contains(out, "values.schema.json") {
		t.Errorf("dry run does not mention the schema:\n%s", out)
	}
	plain := mustRun(t, "new", "demo", "-d", t.TempDir(), "--dry-run")
	if strings.Contains(plain, "values.schema.json") {
		t.Errorf("dry run mentions a schema without --schema:\n%s", plain)
	}
}

// The generated schema has to survive the thing that actually enforces it.
// Helm validates the coalesced values against values.schema.json on every
// render, so a schema that is wrong about a key does not annotate a chart, it
// stops it installing — and only helm itself can prove otherwise.
func TestHelmAcceptsTheGeneratedSchema(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not on PATH")
	}
	dir := t.TempDir()
	for _, preset := range catalog.PresetNames() {
		for _, strict := range []bool{false, true} {
			name := preset
			flag := "--schema"
			if strict {
				name += "-strict"
				flag = "--schema-strict"
			}
			t.Run(name, func(t *testing.T) {
				mustRun(t, "new", name, "--dir", dir, "--preset", preset, flag)
				chartDir := filepath.Join(dir, name)
				out := mustRun(t, "check", "--chart", chartDir, "--strict")
				if !strings.Contains(out, "no findings") {
					t.Fatalf("a chart hck just generated does not pass its own check:\n%s", out)
				}
			})
		}
	}
}

// A resource added after the fact contributes values keys, and helm rejects
// the chart if the schema does not grow with them.
func TestHelmAcceptsTheSchemaAfterAdd(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not on PATH")
	}
	dir := schemaChart(t)
	mustRun(t, "add", "servicemonitor", "prometheusrule", "job", "secret", "--chart", dir)
	mustRun(t, "schema", "--chart", dir, "--check")
	out := mustRun(t, "check", "--chart", dir)
	if !strings.Contains(out, "no findings") {
		t.Fatalf("the chart stopped passing its own check after hck add:\n%s", out)
	}
}

// The schema has to reject what it claims to reject, or it is decoration.
func TestHelmRejectsAValueTheSchemaForbids(t *testing.T) {
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is not on PATH")
	}
	dir := schemaChart(t)
	cmd := exec.Command(helm, "template", "rel", dir,
		"-f", filepath.Join(dir, "ci", "install-values.yaml"),
		"--set", "image.pullPolicy=Sometimes")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("helm accepted an invalid pullPolicy:\n%s", out)
	}
	if !strings.Contains(string(out), "pullPolicy") {
		t.Errorf("helm failed for some other reason:\n%s", out)
	}
}
