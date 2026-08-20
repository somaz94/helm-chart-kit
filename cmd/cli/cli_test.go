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

// runWith drives the command tree with something on stdin, for the prompts.
func runWith(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	t.Setenv("NO_COLOR", "1")
	var buf bytes.Buffer
	root := NewRootCmd()
	root.SetIn(strings.NewReader(stdin))
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func mustRunWith(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	out, err := runWith(t, stdin, args...)
	if err != nil {
		t.Fatalf("hck %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
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

// Every non-workload resource has to be addable to one chart at once, and the
// result has to render and pass its own check. A resource that only works in
// isolation is one nobody finds out about until they add the second one.
func TestAddEveryNonWorkloadResource(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not on PATH")
	}
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent, "--preset", "web", "--schema")
	dir := filepath.Join(parent, "demo")

	var add []string
	for _, r := range catalog.Resources() {
		if !r.Workload {
			add = append(add, r.Name)
		}
	}
	mustRun(t, append([]string{"add", "--chart", dir}, add...)...)

	// The schema has to have grown with values.yaml, or helm rejects the chart.
	mustRun(t, "schema", "--chart", dir, "--check")
	out := mustRun(t, "check", "--chart", dir, "--strict")
	if !strings.Contains(out, "no findings") {
		t.Fatalf("a chart carrying every resource does not pass its own check:\n%s", out)
	}
}

func TestDocsPrintsATableWithoutTouchingTheChart(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent)
	dir := filepath.Join(parent, "demo")

	out := mustRun(t, "docs", "--chart", dir)
	if !strings.HasPrefix(out, "| Key | Type | Default | Description |") {
		t.Fatalf("not a Markdown table:\n%s", out)
	}
	if !strings.Contains(out, "`replicaCount`") {
		t.Error("table does not document replicaCount")
	}
	if _, err := os.Stat(filepath.Join(dir, "README.md")); err == nil {
		t.Error("printing wrote a README")
	}
}

// Types and allowed values come from the schema. A chart that never opted into
// values.schema.json still gets them, because one is assembled on the fly.
func TestDocsUsesTheSchemaEvenWithoutTheFile(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent)
	dir := filepath.Join(parent, "demo")
	if _, err := os.Stat(filepath.Join(dir, "values.schema.json")); err == nil {
		t.Fatal("the chart has a schema, so this proves nothing")
	}
	out := mustRun(t, "docs", "--chart", dir)
	if !strings.Contains(out, "One of: `Always`, `IfNotPresent`, `Never`.") {
		t.Errorf("no allowed values in the table:\n%s", firstLines(out, 6))
	}
}

func TestDocsWriteAndCheck(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent, "--schema")
	dir := filepath.Join(parent, "demo")
	readme := filepath.Join(dir, "README.md")

	if _, err := run(t, "docs", "--chart", dir, "--check"); err == nil {
		t.Error("want an error when the chart has no README")
	}
	mustRun(t, "docs", "--chart", dir, "--write")
	mustRun(t, "docs", "--chart", dir, "--check")

	// Hand-written text around the block has to survive a regeneration, and
	// adding a resource has to make --check notice.
	raw, err := os.ReadFile(readme)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(readme, append(raw, []byte("\n<br/>\n\n## Notes\n\nKeep me.\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, "add", "servicemonitor", "--chart", dir)
	if _, err := run(t, "docs", "--chart", dir, "--check"); err == nil {
		t.Error("--check passed a README that predates the new values")
	}
	mustRun(t, "docs", "--chart", dir, "--write")
	mustRun(t, "docs", "--chart", dir, "--check")

	after, err := os.ReadFile(readme)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "Keep me.") {
		t.Error("regenerating ate the hand-written section")
	}
	if !strings.Contains(string(after), "metrics.serviceMonitor.enabled") {
		t.Error("the new values never reached the table")
	}
}

func TestDocsRejectsWriteAndCheckTogether(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent)
	if _, err := run(t, "docs", "--chart", filepath.Join(parent, "demo"), "--write", "--check"); err == nil {
		t.Error("want an error when both are passed")
	}
}

func TestDocsOutsideAChart(t *testing.T) {
	if _, err := run(t, "docs", "--chart", t.TempDir()); err == nil {
		t.Error("want an error when there is no Chart.yaml")
	}
}

// firstLines trims long output for a failure message.
func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

func TestPlatformList(t *testing.T) {
	out := mustRun(t, "platform", "list", "--chart", t.TempDir())
	for _, want := range []string{"PLATFORMS", "aws", "gcp", "azure", "onprem", "needs:"} {
		if !strings.Contains(out, want) {
			t.Errorf("listing is missing %q:\n%s", want, out)
		}
	}
}

// The listing marks what the chart already carries, so it doubles as status.
func TestPlatformListMarksWhatTheChartHas(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent, "--platform", "aws")
	dir := filepath.Join(parent, "demo")

	out := mustRun(t, "platform", "list", "--chart", dir)
	if !strings.Contains(out, "+ aws") {
		t.Errorf("aws is not marked as present:\n%s", out)
	}
	if strings.Contains(out, "+ gcp") {
		t.Errorf("gcp is marked as present but was never added:\n%s", out)
	}
}

func TestPlatformAdd(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent)
	dir := filepath.Join(parent, "demo")

	mustRun(t, "platform", "add", "aws", "gcp", "--chart", dir)
	for _, name := range []string{"values-aws.yaml", "values-gcp.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s was not written", name)
		}
	}
	// Adding again leaves the file alone unless --force.
	before, err := os.ReadFile(filepath.Join(dir, "values-aws.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "values-aws.yaml"), append(before, []byte("\n# hand-edited\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	out := mustRun(t, "platform", "add", "aws", "--chart", dir)
	if !strings.Contains(out, "already exists") {
		t.Errorf("got %q", out)
	}
	after, err := os.ReadFile(filepath.Join(dir, "values-aws.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "hand-edited") {
		t.Error("re-adding overwrote a hand-edited overlay")
	}
	mustRun(t, "platform", "add", "aws", "--chart", dir, "--force")
	after, err = os.ReadFile(filepath.Join(dir, "values-aws.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(after), "hand-edited") {
		t.Error("--force did not rewrite the overlay")
	}
}

func TestPlatformAddDryRun(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent)
	dir := filepath.Join(parent, "demo")

	out := mustRun(t, "platform", "add", "aws", "--chart", dir, "--dry-run")
	if !strings.Contains(out, "values-aws.yaml") {
		t.Errorf("got %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "values-aws.yaml")); err == nil {
		t.Error("--dry-run wrote the file")
	}
}

func TestPlatformAddRejectsUnknown(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent)
	if _, err := run(t, "platform", "add", "nope", "--chart", filepath.Join(parent, "demo")); err == nil {
		t.Error("want an error for an unknown platform")
	}
}

func TestPlatformAddOutsideAChart(t *testing.T) {
	if _, err := run(t, "platform", "add", "aws", "--chart", t.TempDir()); err == nil {
		t.Error("want an error when there is no Chart.yaml")
	}
}

// An overlay that does not render is worse than no overlay: it looks like
// configuration right up until the day someone installs with it.
func TestCheckAppliesThePlatformOverlay(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not on PATH")
	}
	for _, platform := range catalog.PlatformNames() {
		t.Run(platform, func(t *testing.T) {
			parent := t.TempDir()
			mustRun(t, "new", "demo", "-d", parent, "--preset", "web", "--platform", platform, "--schema")
			dir := filepath.Join(parent, "demo")

			out := mustRun(t, "check", "--chart", dir, "--platform", platform, "--strict")
			if !strings.Contains(out, "no findings") {
				t.Fatalf("chart does not pass its own check under %s:\n%s", platform, out)
			}
			// The report names the overlay, so a passing check is unambiguous.
			if !strings.Contains(out, "("+platform+")") {
				t.Errorf("report does not name the overlay:\n%s", out)
			}
		})
	}
}

// The overlay is additive: it must not suppress the ci/install-values.yaml
// fallback, or the chart stops rendering for want of an image tag.
func TestCheckPlatformKeepsTheCIValues(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not on PATH")
	}
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent, "--platform", "aws")
	dir := filepath.Join(parent, "demo")

	out := mustRun(t, "check", "--chart", dir, "--platform", "aws")
	if strings.Contains(out, "image.tag is required") {
		t.Fatalf("the overlay replaced the base values instead of layering on them:\n%s", out)
	}
}

func TestCheckPlatformNeedsTheOverlay(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent)
	_, err := run(t, "check", "--chart", filepath.Join(parent, "demo"), "--platform", "aws")
	if err == nil {
		t.Fatal("want an error when the overlay is absent")
	}
	if !strings.Contains(err.Error(), "hck platform add aws") {
		t.Errorf("error does not say how to fix it: %v", err)
	}
}

func TestCheckRejectsAnUnknownPlatform(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent)
	if _, err := run(t, "check", "--chart", filepath.Join(parent, "demo"), "--platform", "nope"); err == nil {
		t.Error("want an error for an unknown platform")
	}
}

func TestEnvList(t *testing.T) {
	out := mustRun(t, "env", "list", "--chart", t.TempDir())
	for _, want := range []string{"ENVIRONMENTS", "dev", "staging", "prod"} {
		if !strings.Contains(out, want) {
			t.Errorf("listing is missing %q:\n%s", want, out)
		}
	}
}

func TestEnvAdd(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent)
	dir := filepath.Join(parent, "demo")

	mustRun(t, "env", "add", "dev", "prod", "--chart", dir)
	for _, name := range []string{"values-dev.yaml", "values-prod.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s was not written", name)
		}
	}
	out := mustRun(t, "env", "list", "--chart", dir)
	if !strings.Contains(out, "+ dev") {
		t.Errorf("dev is not marked as present:\n%s", out)
	}
	if _, err := run(t, "env", "add", "nope", "--chart", dir); err == nil {
		t.Error("want an error for an unknown environment")
	}
}

// The two axes stack, and the environment goes last so its size wins.
func TestCheckStacksPlatformAndEnvironment(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not on PATH")
	}
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent, "--preset", "web", "--platform", "aws", "--env", "prod", "--schema")
	dir := filepath.Join(parent, "demo")

	mustRun(t, "check", "--chart", dir, "--platform", "aws", "--env", "prod", "--strict")
	both := mustRun(t, "check", "--chart", dir, "--platform", "aws", "--env", "prod", "--print")

	// The platform half survived...
	if !strings.Contains(both, "eks.amazonaws.com/role-arn") {
		t.Error("the platform overlay was lost when an environment was added")
	}
	// ...and so did the environment half.
	if !strings.Contains(both, "kind: PodDisruptionBudget") {
		t.Error("the prod overlay did not reach the render")
	}
}

// An overlay that changes nothing about the render is not an overlay. This is
// what a check passing on the base chart looks like, and it used to pass.
func TestCheckOverlayActuallyChangesTheRender(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not on PATH")
	}
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent, "--preset", "web", "--platform", "aws")
	dir := filepath.Join(parent, "demo")

	base := mustRun(t, "check", "--chart", dir, "--print")
	if strings.Contains(base, "eks.amazonaws.com/role-arn") {
		t.Fatal("the base already carries the annotation, so this proves nothing")
	}
	with := mustRun(t, "check", "--chart", dir, "--platform", "aws", "--print")
	if !strings.Contains(with, "eks.amazonaws.com/role-arn") {
		t.Error("--platform aws did not reach the render")
	}
}

func TestCheckEnvNeedsTheOverlay(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent)
	_, err := run(t, "check", "--chart", filepath.Join(parent, "demo"), "--env", "prod")
	if err == nil {
		t.Fatal("want an error when the overlay is absent")
	}
	if !strings.Contains(err.Error(), "hck env add prod") {
		t.Errorf("error does not say how to fix it: %v", err)
	}
}

func TestNewWritesBothAxes(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent, "--platform", "aws", "--env", "dev,prod")
	dir := filepath.Join(parent, "demo")
	for _, name := range []string{"values-aws.yaml", "values-dev.yaml", "values-prod.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s was not written", name)
		}
	}
	if _, err := run(t, "new", "other", "-d", parent, "--env", "nope"); err == nil {
		t.Error("want an error for an unknown environment")
	}
}

func TestInitAsksAndScaffolds(t *testing.T) {
	parent := t.TempDir()
	out := mustRunWith(t, "payments-api\nworker\npvc\naws\nprod\ny\nn\n", "init", "-d", parent)

	for _, want := range []string{"Chart name?", "Preset?", "Platform overlays?", "Environment overlays?", "[y/N]"} {
		if !strings.Contains(out, want) {
			t.Errorf("prompt %q was not asked:\n%s", want, out)
		}
	}
	dir := filepath.Join(parent, "payments-api")
	for _, name := range []string{"Chart.yaml", "values.yaml", "values.schema.json", "values-aws.yaml", "values-prod.yaml", "templates/pvc.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
			t.Errorf("%s was not written", name)
		}
	}
	// "n" to the README question means no README.
	if _, err := os.Stat(filepath.Join(dir, "README.md")); err == nil {
		t.Error("a README was written despite answering no")
	}
}

// The command init prints has to actually reproduce what init just made —
// that equivalence is the whole reason for printing it.
func TestInitPrintsAWorkingEquivalent(t *testing.T) {
	viaInit := t.TempDir()
	out := mustRunWith(t, "app\nstateful\npvc\ngcp\nprod\ny\nn\n", "init", "-d", viaInit)

	_, tail, ok := strings.Cut(out, "without the questions:")
	if !ok {
		t.Fatalf("init printed no equivalent command:\n%s", out)
	}
	line := strings.TrimSpace(strings.SplitN(strings.TrimSpace(tail), "\n", 2)[0])
	if !strings.HasPrefix(line, "hck new app ") {
		t.Fatalf("equivalent command looks wrong: %q", line)
	}

	// Run the printed flags through "new" and compare the two trees.
	viaNew := t.TempDir()
	args := strings.Fields(line)[1:] // drop "hck"
	for i := range args {
		if args[i] == viaInit {
			args[i] = viaNew
		}
	}
	mustRun(t, args...)

	compareTrees(t, filepath.Join(viaInit, "app"), filepath.Join(viaNew, "app"))
}

// compareTrees fails if the two chart directories differ in any file.
func compareTrees(t *testing.T, a, b string) {
	t.Helper()
	read := func(root string) map[string]string {
		out := map[string]string{}
		if err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			body, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			out[filepath.ToSlash(rel)] = string(body)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	left, right := read(a), read(b)
	for name, body := range left {
		other, ok := right[name]
		if !ok {
			t.Errorf("%s is only in the init tree", name)
			continue
		}
		if body != other {
			t.Errorf("%s differs between init and new", name)
		}
	}
	for name := range right {
		if _, ok := left[name]; !ok {
			t.Errorf("%s is only in the new tree", name)
		}
	}
}

func TestInitDefaults(t *testing.T) {
	parent := t.TempDir()
	out := mustRun(t, "init", "quick", "-d", parent, "--defaults")
	if strings.Contains(out, "Chart name?") {
		t.Error("--defaults asked a question")
	}
	if _, err := os.Stat(filepath.Join(parent, "quick", "Chart.yaml")); err != nil {
		t.Errorf("no chart was written: %v", err)
	}
	if !strings.Contains(out, "hck new quick") {
		t.Errorf("no equivalent command printed:\n%s", out)
	}
}

// EOF partway through means "take the rest of the defaults", so a short
// heredoc does not have to answer every question.
func TestInitStopsAskingAtEOF(t *testing.T) {
	parent := t.TempDir()
	mustRunWith(t, "half-answered\n", "init", "-d", parent)
	if _, err := os.Stat(filepath.Join(parent, "half-answered", "Chart.yaml")); err != nil {
		t.Errorf("no chart was written: %v", err)
	}
}

func TestInitWritesTheReadmeWhenAsked(t *testing.T) {
	parent := t.TempDir()
	mustRunWith(t, "documented\n\n\n\n\nn\ny\n", "init", "-d", parent)
	body, err := os.ReadFile(filepath.Join(parent, "documented", "README.md"))
	if err != nil {
		t.Fatalf("no README: %v", err)
	}
	if !strings.Contains(string(body), "| Key | Type | Default | Description |") {
		t.Error("README carries no values table")
	}
	if !strings.Contains(string(body), "One of: `Always`, `IfNotPresent`, `Never`.") {
		t.Error("the table has no allowed values, so the schema was not consulted")
	}
}

func TestInitRejectsABadName(t *testing.T) {
	if _, err := runWith(t, "Not A Chart Name\n", "init", "-d", t.TempDir()); err == nil {
		t.Error("want an error for a name Helm would refuse")
	}
}

// Every declared overlay has to change the rendered output of a chart that
// carries every resource. An overlay that renders identically to the base is
// either wired up wrong or says nothing — and the version of this feature that
// forgot to pass OverlayFiles to helm passed every other test in this file.
func TestEveryOverlayChangesTheRender(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not on PATH")
	}
	parent := t.TempDir()
	mustRun(t, "new", "probe", "-d", parent, "--preset", "web", "--schema")
	dir := filepath.Join(parent, "probe")

	var extra []string
	for _, r := range catalog.Resources() {
		if !r.Workload {
			extra = append(extra, r.Name)
		}
	}
	mustRun(t, append([]string{"add", "--chart", dir}, extra...)...)

	base := mustRun(t, "check", "--chart", dir, "--print")

	for _, p := range catalog.Platforms() {
		t.Run("platform/"+p.Name, func(t *testing.T) {
			mustRun(t, "platform", "add", p.Name, "--chart", dir, "--force")
			got := mustRun(t, "check", "--chart", dir, "--platform", p.Name, "--print")
			if got == base {
				t.Errorf("the %s overlay renders identically to the base", p.Name)
			}
		})
	}
	for _, e := range catalog.Environments() {
		t.Run("env/"+e.Name, func(t *testing.T) {
			mustRun(t, "env", "add", e.Name, "--chart", dir, "--force")
			got := mustRun(t, "check", "--chart", dir, "--env", e.Name, "--print")
			if got == base {
				t.Errorf("the %s overlay renders identically to the base", e.Name)
			}
		})
	}
}

// Platform and environment overlays both become -f arguments, so any key both
// axes set is resolved by argument order. That is not a decision anybody made,
// and it produced a chart whose NetworkPolicy existed or not depending on
// which file came last. The axes must not overlap at all.
func TestOverlayOrderDoesNotChangeTheRender(t *testing.T) {
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is not on PATH")
	}
	parent := t.TempDir()
	mustRun(t, "new", "ov", "-d", parent, "--preset", "web")
	dir := filepath.Join(parent, "ov")

	var extra []string
	for _, r := range catalog.Resources() {
		if !r.Workload {
			extra = append(extra, r.Name)
		}
	}
	mustRun(t, append([]string{"add", "--chart", dir}, extra...)...)
	for _, p := range catalog.Platforms() {
		mustRun(t, "platform", "add", p.Name, "--chart", dir, "--force")
	}
	for _, e := range catalog.Environments() {
		mustRun(t, "env", "add", e.Name, "--chart", dir, "--force")
	}

	render := func(files ...string) string {
		t.Helper()
		args := []string{"template", "t", dir, "-f", filepath.Join(dir, "ci", "install-values.yaml")}
		for _, f := range files {
			args = append(args, "-f", filepath.Join(dir, f))
		}
		out, err := exec.Command(helm, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("helm template: %v\n%s", err, out)
		}
		return string(out)
	}

	for _, p := range catalog.Platforms() {
		for _, e := range catalog.Environments() {
			t.Run(p.Name+"+"+e.Name, func(t *testing.T) {
				if render(p.ValuesFile(), e.ValuesFile()) != render(e.ValuesFile(), p.ValuesFile()) {
					t.Error("the two overlays disagree about a key, so the render depends on -f order")
				}
			})
		}
	}
}

// Every optional resource defaults to enabled: false, so a chart carrying all
// of them renders none of them — and a check over that chart reports "no
// findings" while proving nothing. Turn them all on and render for real.
//
// The dashboard body deliberately contains Grafana's own {{pod}} legend
// syntax, which is what broke the first version of that template.
func TestEveryResourceRendersWhenEnabled(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not on PATH")
	}
	parent := t.TempDir()
	mustRun(t, "new", "all", "-d", parent, "--preset", "web", "--schema")
	dir := filepath.Join(parent, "all")

	var extra []string
	for _, r := range catalog.Resources() {
		if !r.Workload {
			extra = append(extra, r.Name)
		}
	}
	mustRun(t, append([]string{"add", "--chart", dir}, extra...)...)

	values, err := filepath.Abs(filepath.Join("testdata", "enable-all.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	out := mustRun(t, "check", "--chart", dir, "-f", values, "--print")

	// Turning on every scaler at once is itself a finding — HPA and KEDA both
	// driving the replica count is what HCK031 is for — so that one is
	// expected. Anything else means a resource renders something the house
	// rules object to.
	if !strings.Contains(out, "HCK031") {
		t.Error("HPA and a KEDA ScaledObject are both enabled; HCK031 should have fired")
	}
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "HCK0") || strings.Contains(line, "HCK031") {
			continue
		}
		t.Errorf("unexpected finding: %s", strings.TrimSpace(line))
	}

	// Every resource the catalog knows has to appear in the output. This is
	// the assertion the previous suite was missing entirely.
	for _, r := range catalog.Resources() {
		if r.Workload && r.Name != "deployment" {
			continue // one workload per chart
		}
		t.Run(r.Name, func(t *testing.T) {
			if !strings.Contains(out, "# Source: all/templates/"+r.File) {
				t.Errorf("%s rendered nothing", r.Name)
			}
		})
	}
}

func TestEnvAddDryRun(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent)
	dir := filepath.Join(parent, "demo")

	out := mustRun(t, "env", "add", "prod", "--chart", dir, "--dry-run")
	if !strings.Contains(out, "values-prod.yaml") {
		t.Errorf("got %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "values-prod.yaml")); err == nil {
		t.Error("--dry-run wrote the file")
	}
}

func TestEnvAddOutsideAChart(t *testing.T) {
	if _, err := run(t, "env", "add", "prod", "--chart", t.TempDir()); err == nil {
		t.Error("want an error when there is no Chart.yaml")
	}
}

func TestCheckRejectsAnUnknownEnv(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent)
	if _, err := run(t, "check", "--chart", filepath.Join(parent, "demo"), "--env", "nope"); err == nil {
		t.Error("want an error for an unknown environment")
	}
}

// The platform side has this; the environment side did not.
func TestCheckAppliesTheEnvOverlay(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not on PATH")
	}
	for _, env := range catalog.EnvironmentNames() {
		t.Run(env, func(t *testing.T) {
			parent := t.TempDir()
			mustRun(t, "new", "demo", "-d", parent, "--preset", "web", "--env", env, "--schema")
			dir := filepath.Join(parent, "demo")

			out := mustRun(t, "check", "--chart", dir, "--env", env, "--strict")
			if !strings.Contains(out, "no findings") {
				t.Fatalf("chart does not pass its own check at %s:\n%s", env, out)
			}
			if !strings.Contains(out, "("+env+")") {
				t.Errorf("report does not name the overlay:\n%s", out)
			}
		})
	}
}

// Naming nothing but separators is not naming a platform.
func TestOverlayAddRejectsAnEmptyList(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent)
	dir := filepath.Join(parent, "demo")
	for _, axis := range []string{"platform", "env"} {
		t.Run(axis, func(t *testing.T) {
			if _, err := run(t, axis, "add", ",", "--chart", dir); err == nil {
				t.Error("want an error; the command exited 0 having done nothing")
			}
		})
	}
}
