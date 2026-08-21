package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/somaz94/helm-chart-kit/internal/catalog"
	"github.com/somaz94/helm-chart-kit/internal/check"
	"gopkg.in/yaml.v3"
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

// writeFile overwrites a file inside a chart, for the tests that need a chart
// in a state hck itself would never generate.
func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
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
// A second workload is built and reported, not refused. Guarded so that one
// renders at a time it is the Deployment-to-Rollout swap; both rendering at
// once is HCK030's finding, over the render rather than the resource names.
func TestNewBuildsASecondWorkloadAndSaysSo(t *testing.T) {
	dir := t.TempDir()
	out, err := run(t, "new", "demo", "-d", dir, "--preset", "web", "--with", "daemonset")
	if err != nil {
		t.Fatalf("want a chart, got %v", err)
	}
	if !strings.Contains(out, "HCK030") {
		t.Errorf("the note does not name the rule that answers it:\n%s", out)
	}
	for _, f := range []string{"templates/deployment.yaml", "templates/daemonset.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, "demo", f)); err != nil {
			t.Errorf("%s was not written: %v", f, err)
		}
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
	for _, platform := range catalog.OverlayNames(catalog.PlatformAxis) {
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
	// "n" to "Keep those?" is what opens the four the preset had answered.
	out := mustRunWith(t, "payments-api\nworker\npvc\nn\naws\nprod\ny\nn\n", "init", "-d", parent)

	for _, want := range []string{"Chart name?", "Preset?", "Keep those?", "Platform overlays?", "Environment overlays?", "[y/N]"} {
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

// The short path: a name, a preset, nothing extra, and Enter. A preset that
// carries a platform and asks for a schema produces both without being asked
// about either, which is the whole reason the fields exist.
func TestInitTakesWhatThePresetDecided(t *testing.T) {
	parent := t.TempDir()
	out := mustRunWith(t, "house\nbase\n\n\n", "init", "-d", parent)

	if !strings.Contains(out, "base also decided:") {
		t.Errorf("init did not show what the preset settled:\n%s", out)
	}
	// The four it settled are shown, not merely applied.
	for _, want := range []string{"platform overlays: onprem", "values.schema.json: yes", "values table in README.md: yes"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q is not in the summary:\n%s", want, out)
		}
	}
	// And it never puts the questions it answered.
	for _, never := range []string{"Platform overlays?", "Write values.schema.json?"} {
		if strings.Contains(out, never) {
			t.Errorf("%q was asked anyway:\n%s", never, out)
		}
	}
	dir := filepath.Join(parent, "house")
	for _, name := range []string{"values-onprem.yaml", "values.schema.json", "README.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s was not written", name)
		}
	}
	// The printed command has to reproduce it, overlay and schema included.
	if !strings.Contains(out, "--preset base --platform onprem --schema") {
		t.Errorf("the equivalent command does not carry what the preset decided:\n%s", out)
	}
}

func TestInitWritesTheReadmeWhenAsked(t *testing.T) {
	parent := t.TempDir()
	mustRunWith(t, "documented\n\n\nn\n\n\nn\ny\n", "init", "-d", parent)
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

	for _, p := range catalog.Overlays(catalog.PlatformAxis) {
		t.Run("platform/"+p.Name, func(t *testing.T) {
			mustRun(t, "platform", "add", p.Name, "--chart", dir, "--force")
			got := mustRun(t, "check", "--chart", dir, "--platform", p.Name, "--print")
			if got == base {
				t.Errorf("the %s overlay renders identically to the base", p.Name)
			}
		})
	}
	for _, e := range catalog.Overlays(catalog.EnvironmentAxis) {
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
	for _, p := range catalog.Overlays(catalog.PlatformAxis) {
		mustRun(t, "platform", "add", p.Name, "--chart", dir, "--force")
	}
	for _, e := range catalog.Overlays(catalog.EnvironmentAxis) {
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

	for _, p := range catalog.Overlays(catalog.PlatformAxis) {
		for _, e := range catalog.Overlays(catalog.EnvironmentAxis) {
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

	// Turning everything on at once necessarily creates pairs that want a
	// decision, and each of these is the rule working rather than a defect in
	// a template: HPA and KEDA both driving the replica count (HCK031), the
	// cert-manager/GKE and ServiceMonitor/PodMonitoring pairs (HCK037), and a
	// SecretStore beside an ExternalSecret still reading the default
	// ClusterSecretStore (HCK039). Anything else means a resource renders
	// something the house rules object to.
	expected := []string{"HCK031", "HCK037", "HCK039"}
	for _, id := range expected {
		if !strings.Contains(out, id) {
			t.Errorf("everything is enabled; %s should have fired", id)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "HCK0") {
			continue
		}
		if slices.ContainsFunc(expected, func(id string) bool { return strings.Contains(line, id) }) {
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
	for _, env := range catalog.OverlayNames(catalog.EnvironmentAxis) {
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

// Twelve commands listed alphabetically tell a first-time reader nothing about
// where to start. Every command has to sit in a group, or it falls into
// cobra's "Additional Commands" bucket next to `completion` and is effectively
// hidden.
func TestEveryCommandIsGrouped(t *testing.T) {
	root := NewRootCmd()

	groups := map[string]bool{}
	for _, g := range root.Groups() {
		groups[g.ID] = true
	}
	if len(groups) == 0 {
		t.Fatal("no command groups are declared")
	}

	for _, c := range root.Commands() {
		// cobra adds these itself and groups them for us.
		if c.Name() == "completion" || c.Name() == "help" {
			continue
		}
		t.Run(c.Name(), func(t *testing.T) {
			if c.GroupID == "" {
				t.Error("has no group, so it lands under Additional Commands")
			} else if !groups[c.GroupID] {
				t.Errorf("group %q is not declared on the root", c.GroupID)
			}
		})
	}
}

// The first thing anyone reads has to say where to start.
func TestRootHelpPointsAtTheWayIn(t *testing.T) {
	out := mustRun(t, "--help")
	for _, want := range []string{
		"hck init",         // the way in
		"Getting started:", // the groups render
		"Working on a chart:",
		"opt-in", // nothing below is mandatory
	} {
		if !strings.Contains(out, want) {
			t.Errorf("root help is missing %q:\n%s", want, out)
		}
	}
	// The three-command path has to be in there verbatim.
	for _, want := range []string{"hck new <name>", "hck add <resource>", "hck check"} {
		if !strings.Contains(out, want) {
			t.Errorf("root help does not show %q", want)
		}
	}
}

// "hck list rules" is what a chart's .hck.yaml is written against, so every
// rule the checker runs has to appear in it.
func TestListRules(t *testing.T) {
	out := mustRun(t, "list", "rules")
	for _, r := range check.Rules() {
		if !strings.Contains(out, r.ID) {
			t.Errorf("hck list rules does not mention %s", r.ID)
		}
	}
	if !strings.Contains(out, check.ConfigFile) {
		t.Errorf("hck list rules does not say where to configure them:\n%s", out)
	}
	if strings.Contains(out, "PRESETS") {
		t.Error("hck list rules printed the presets section")
	}
	if !strings.Contains(mustRun(t, "list"), "CHECK RULES") {
		t.Error("hck list left the rules out")
	}
}

// A chart turns a rule off in its own .hck.yaml, and the report says so — a
// clean check over a chart with half the rules off would otherwise read as a
// chart with nothing wrong with it.
func TestCheckHonoursTheChartConfig(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, "new", "demo", "--dir", dir)
	chartDir := filepath.Join(dir, "demo")
	// A chart with no description trips HCK013, without needing helm.
	writeFile(t, filepath.Join(chartDir, "Chart.yaml"), "apiVersion: v2\nname: demo\n")

	out := mustRun(t, "check", "--chart", chartDir, "--no-render")
	if !strings.Contains(out, "HCK013") {
		t.Fatalf("HCK013 did not fire:\n%s", out)
	}

	writeFile(t, filepath.Join(chartDir, check.ConfigFile), "rules:\n  HCK013: off\n")
	out = mustRun(t, "check", "--chart", chartDir, "--no-render")
	// The ID still appears in the "not checked" line, so look for a finding line.
	if strings.Contains(out, "warn  HCK013") {
		t.Errorf("HCK013 is off but reported anyway:\n%s", out)
	}
	if !strings.Contains(out, "no findings") {
		t.Errorf("turning off the only finding did not leave a clean report:\n%s", out)
	}
	if !strings.Contains(out, "not checked: HCK013") {
		t.Errorf("the report does not say what it skipped:\n%s", out)
	}

	// Raised to an error, the same finding fails the check.
	writeFile(t, filepath.Join(chartDir, check.ConfigFile), "rules:\n  HCK013: error\n")
	if _, err := run(t, "check", "--chart", chartDir, "--no-render"); err == nil {
		t.Error("a rule raised to error did not fail the check")
	}

	// And a rule ID nobody has is an error rather than a silent no-op.
	writeFile(t, filepath.Join(chartDir, check.ConfigFile), "rules:\n  HCK999: off\n")
	_, err := run(t, "check", "--chart", chartDir, "--no-render")
	if err == nil || !strings.Contains(err.Error(), "HCK999") {
		t.Errorf("got %v, want a complaint about HCK999", err)
	}
}

// The JSON form exists so a CI step can act on a finding rather than grep for
// one, which means the verdict has to match the exit status.
func TestCheckJSONFormat(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, "new", "demo", "--dir", dir)
	chartDir := filepath.Join(dir, "demo")
	writeFile(t, filepath.Join(chartDir, "Chart.yaml"), "apiVersion: v1\nname: demo\n")
	writeFile(t, filepath.Join(chartDir, check.ConfigFile), "rules:\n  HCK013: off\n")

	out := mustRun(t, "check", "--chart", chartDir, "--no-render", "--format", "json")
	var doc struct {
		Chart    string `json:"chart"`
		Findings []struct {
			Rule     string `json:"rule"`
			Severity string `json:"severity"`
			Where    string `json:"where"`
			Message  string `json:"message"`
		} `json:"findings"`
		Errors   int      `json:"errors"`
		Warnings int      `json:"warnings"`
		Disabled []string `json:"disabled"`
		OK       bool     `json:"ok"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if doc.Chart != "demo" || !doc.OK || doc.Errors != 0 {
		t.Errorf("got %+v", doc)
	}
	if len(doc.Findings) != doc.Warnings || doc.Warnings == 0 {
		t.Errorf("findings and counts disagree: %+v", doc)
	}
	if doc.Findings[0].Rule != "HCK012" || doc.Findings[0].Message == "" {
		t.Errorf("got %+v", doc.Findings[0])
	}
	if len(doc.Disabled) != 1 || doc.Disabled[0] != "HCK013" {
		t.Errorf("disabled = %v", doc.Disabled)
	}

	// --strict turns those warnings into a failure, and the document says so
	// rather than only the exit status.
	out, err := run(t, "check", "--chart", chartDir, "--no-render", "--format", "json", "--strict")
	if err == nil {
		t.Error("--strict passed a chart with warnings")
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if doc.OK {
		t.Error("ok is true on a run that failed")
	}

	if _, err := run(t, "check", "--chart", chartDir, "--no-render", "--format", "xml"); err == nil {
		t.Error("an unknown --format was accepted")
	}
}

// remove deletes templates and nothing else: the values keys stay, and the
// plan names them so that deleting them is a decision.
func TestRemove(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, "new", "demo", "--dir", dir, "--preset", "web")
	chartDir := filepath.Join(dir, "demo")

	out := mustRun(t, "remove", "hpa", "--chart", chartDir, "--dry-run")
	if !strings.Contains(out, "templates/hpa.yaml") || !strings.Contains(out, "autoscaling") {
		t.Fatalf("the dry run did not say what it would do:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(chartDir, "templates", "hpa.yaml")); err != nil {
		t.Fatal("--dry-run deleted the file anyway")
	}

	out = mustRun(t, "rm", "hpa", "--chart", chartDir)
	if !strings.Contains(out, "still declares") {
		t.Errorf("the removal did not name the orphaned keys:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(chartDir, "templates", "hpa.yaml")); err == nil {
		t.Error("the template is still there")
	}
	values, err := os.ReadFile(filepath.Join(chartDir, "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(values), "autoscaling:") {
		t.Error("values.yaml was rewritten by a removal")
	}

	// Removing it twice is an error, not a silent success.
	if _, err := run(t, "remove", "hpa", "--chart", chartDir); err == nil {
		t.Error("removing a resource the chart does not have was accepted")
	}
}

func TestRemoveRefusesWhatWouldBreakOrLoseSomething(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, "new", "demo", "--dir", dir, "--preset", "web")
	chartDir := filepath.Join(dir, "demo")

	// The Ingress and the test hook both point at the Service.
	_, err := run(t, "remove", "service", "--chart", chartDir)
	if err == nil || !strings.Contains(err.Error(), "required by") {
		t.Errorf("got %v, want a complaint about what still needs it", err)
	}
	mustRun(t, "remove", "service", "ingress", "tests", "--chart", chartDir)

	// An edited template is somebody's work, and --force is the only way past.
	appendToFile(t, filepath.Join(chartDir, "templates", "pdb.yaml"), "\n# a local edit\n")
	_, err = run(t, "remove", "pdb", "--chart", chartDir)
	if err == nil || !strings.Contains(err.Error(), "edited") {
		t.Errorf("got %v, want a complaint about the edit", err)
	}
	mustRun(t, "remove", "pdb", "--chart", chartDir, "--force")
	if _, err := os.Stat(filepath.Join(chartDir, "templates", "pdb.yaml")); err == nil {
		t.Error("--force did not delete it")
	}
}

// sync reports what differs, and --write is the only thing that changes a file.
func TestSync(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, "new", "demo", "--dir", dir, "--preset", "web")
	chartDir := filepath.Join(dir, "demo")

	out := mustRun(t, "sync", "--chart", chartDir)
	if !strings.Contains(out, "every generated file is what hck writes") {
		t.Fatalf("a chart hck just wrote already differs from hck:\n%s", out)
	}
	// The skeleton is compared too. It was not, once, and an hck that changed
	// _helpers.tpl or .helmignore left every existing chart quietly behind.
	for _, want := range []string{"templates/_helpers.tpl", ".helmignore", "ci/install-values.yaml"} {
		if !strings.Contains(out, want) {
			t.Errorf("sync does not compare %s:\n%s", want, out)
		}
	}
	// Anchored to the path column: "ci/install-values.yaml" ends in
	// "values.yaml" and is a file hck does own.
	if authorsFileRE.MatchString(out) {
		t.Errorf("sync compares a file the author maintains:\n%s", out)
	}
	mustRun(t, "sync", "--chart", chartDir, "--check")

	appendToFile(t, filepath.Join(chartDir, "templates", "hpa.yaml"), "\n# a local edit\n")
	out = mustRun(t, "sync", "--chart", chartDir)
	if !strings.Contains(out, "templates/hpa.yaml") || !strings.Contains(out, "differs") {
		t.Fatalf("the edit was not reported:\n%s", out)
	}
	if _, err := run(t, "sync", "--chart", chartDir, "--check"); err == nil {
		t.Error("--check passed a chart that differs")
	}

	// The report on its own never touches a file.
	body, err := os.ReadFile(filepath.Join(chartDir, "templates", "hpa.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "a local edit") {
		t.Fatal("the report overwrote the file")
	}

	mustRun(t, "sync", "--chart", chartDir, "--write", "hpa")
	body, err = os.ReadFile(filepath.Join(chartDir, "templates", "hpa.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "a local edit") {
		t.Error("--write did not take hck's version")
	}
	mustRun(t, "sync", "--chart", chartDir, "--check")
}

func TestSyncWriteAll(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, "new", "demo", "--dir", dir, "--preset", "worker")
	chartDir := filepath.Join(dir, "demo")
	for _, f := range []string{"deployment.yaml", "configmap.yaml"} {
		appendToFile(t, filepath.Join(chartDir, "templates", f), "\n# a local edit\n")
	}

	mustRun(t, "sync", "--chart", chartDir, "--write", "--all")
	mustRun(t, "sync", "--chart", chartDir, "--check")

	// Nothing left to take says so rather than claiming to have written.
	out := mustRun(t, "sync", "--chart", chartDir, "--write", "--all")
	if !strings.Contains(out, "nothing to take") {
		t.Errorf("got:\n%s", out)
	}
}

// A skeleton file hck owns drifts the same way a resource template does, and
// a missing one is put back rather than reported forever.
func TestSyncCoversTheChartSkeleton(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, "new", "demo", "--dir", dir, "--preset", "worker")
	chartDir := filepath.Join(dir, "demo")

	appendToFile(t, filepath.Join(chartDir, "templates", "_helpers.tpl"), "\n{{/* an older helper set */}}\n")
	if err := os.Remove(filepath.Join(chartDir, "ci", "install-values.yaml")); err != nil {
		t.Fatal(err)
	}

	out := mustRun(t, "sync", "--chart", chartDir)
	if !strings.Contains(out, "no longer here") {
		t.Errorf("a deleted skeleton file was not reported:\n%s", out)
	}
	if _, err := run(t, "sync", "--chart", chartDir, "--check"); err == nil {
		t.Error("--check passed a chart whose skeleton differs")
	}

	mustRun(t, "sync", "--chart", chartDir, "--write", "--all")
	mustRun(t, "sync", "--chart", chartDir, "--check")
	if _, err := os.Stat(filepath.Join(chartDir, "ci", "install-values.yaml")); err != nil {
		t.Error("the missing skeleton file was not put back")
	}
	body, err := os.ReadFile(filepath.Join(chartDir, "templates", "_helpers.tpl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "an older helper set") {
		t.Error("--write did not take hck's version of the helper set")
	}

	// Naming a skeleton file directly works too — the report prints its path,
	// so that is what the reader has to be able to type.
	appendToFile(t, filepath.Join(chartDir, ".helmignore"), "\n# mine\n")
	mustRun(t, "sync", "--chart", chartDir, "--write", ".helmignore")
	mustRun(t, "sync", "--chart", chartDir, "--check")
}

// --write overwrites, so the ways of asking for it that mean two things at
// once are refused rather than guessed at.
func TestSyncRefusesAmbiguousFlags(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, "new", "demo", "--dir", dir, "--preset", "worker")
	chartDir := filepath.Join(dir, "demo")

	for _, args := range [][]string{
		{"sync", "--chart", chartDir, "--check", "--write", "deployment"},
		{"sync", "--chart", chartDir, "--write"},
		{"sync", "--chart", chartDir, "--write", "--all", "deployment"},
		{"sync", "--chart", chartDir, "nonsense"},
	} {
		if _, err := run(t, args...); err == nil {
			t.Errorf("hck %s was accepted", strings.Join(args[1:], " "))
		}
	}
}

func appendToFile(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

// authorsFileRE matches a report line whose path is exactly Chart.yaml or
// values.yaml — the two files hck writes once and does not own afterwards.
var authorsFileRE = regexp.MustCompile(`(?m)^\s+[=~!] (Chart\.yaml|values\.yaml)\s`)

// Completion is only useful if it offers what this chart actually has: a name
// the chart does not carry completes to an error, which is not what a
// completion is for.
func TestCompletionFollowsTheChart(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, "new", "demo", "--dir", dir, "--preset", "worker")
	chartDir := filepath.Join(dir, "demo")

	// cobra drives completion through the hidden __complete command, which is
	// the same path a shell takes.
	out := mustRun(t, "__complete", "remove", "--chart", chartDir, "")
	for _, want := range []string{"deployment", "configmap", "serviceaccount"} {
		if !strings.Contains(out, want) {
			t.Errorf("remove does not complete %q, which the chart has:\n%s", want, out)
		}
	}
	for _, notThere := range []string{"ingress", "statefulset", "grafanadashboard"} {
		if completionOffers(out, notThere) {
			t.Errorf("remove completes %q, which the chart does not have:\n%s", notThere, out)
		}
	}

	out = mustRun(t, "__complete", "sync", "--chart", chartDir, "")
	for _, want := range []string{"deployment", "templates/_helpers.tpl", ".helmignore"} {
		if !completionOffers(out, want) {
			t.Errorf("sync does not complete %q:\n%s", want, out)
		}
	}
	for _, notThere := range []string{"Chart.yaml", "values.yaml"} {
		if completionOffers(out, notThere) {
			t.Errorf("sync completes %q, a file the author maintains:\n%s", notThere, out)
		}
	}

	// --off offers the rules that can be turned off, and not the one that
	// cannot: completing HCK001 would offer a flag value the command refuses.
	out = mustRun(t, "__complete", "check", "--off", "")
	if !completionOffers(out, "HCK025") {
		t.Errorf("--off does not complete a rule ID:\n%s", out)
	}
	if completionOffers(out, "HCK001") {
		t.Errorf("--off completes the locked rule:\n%s", out)
	}

	// Pointed somewhere that is not a chart, both offer nothing rather than
	// an error a shell would print in the middle of a command line.
	for _, cmd := range []string{"remove", "sync"} {
		out, err := run(t, "__complete", cmd, "--chart", t.TempDir(), "")
		if err != nil {
			t.Errorf("%s completion outside a chart: %v", cmd, err)
		}
		if completionOffers(out, "deployment") {
			t.Errorf("%s completed a resource outside a chart:\n%s", cmd, out)
		}
	}
}

// completionOffers reports whether a candidate is a whole line of cobra's
// completion output. Substring matching would count "ci/install-values.yaml"
// as an offer of "values.yaml".
func completionOffers(out, candidate string) bool {
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == candidate {
			return true
		}
	}
	return false
}

// --force is the escape hatch on the two things "hck new" refuses: a second
// workload, and a directory that is not empty.
func TestNewForce(t *testing.T) {
	parent := t.TempDir()
	mustRun(t, "new", "demo", "-d", parent, "--preset", "web", "--with", "daemonset", "--force")
	if _, err := os.Stat(filepath.Join(parent, "demo", "templates", "daemonset.yaml")); err != nil {
		t.Errorf("--force did not write the second workload: %v", err)
	}

	// Forcing over the chart that is now there fills in what is missing and
	// says so, rather than writing anything back over it.
	before := readFile(t, filepath.Join(parent, "demo", "values.yaml"))
	out := mustRun(t, "new", "demo", "-d", parent, "--preset", "web", "--force")
	if !strings.Contains(out, "already there") {
		t.Errorf("the plan does not say it left the existing files alone:\n%s", out)
	}
	if got := readFile(t, filepath.Join(parent, "demo", "values.yaml")); got != before {
		t.Error("values.yaml was rewritten by hck new --force")
	}
}

// A rule turned off from the command line is turned off exactly as far as one
// turned off in the chart's own file — and is still named in the report,
// because a clean run has to be distinguishable from an unasked question.
func TestCheckOffFlag(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, "new", "demo", "--dir", dir)
	chartDir := filepath.Join(dir, "demo")
	writeFile(t, filepath.Join(chartDir, "Chart.yaml"), "apiVersion: v2\nname: demo\n")

	out := mustRun(t, "check", "--chart", chartDir, "--no-render")
	if !strings.Contains(out, "warn  HCK013") {
		t.Fatalf("HCK013 did not fire:\n%s", out)
	}

	out = mustRun(t, "check", "--chart", chartDir, "--no-render", "--off", "HCK013")
	if strings.Contains(out, "warn  HCK013") {
		t.Errorf("--off HCK013 did not turn it off:\n%s", out)
	}
	if !strings.Contains(out, "not checked: HCK013") {
		t.Errorf("the report does not say what --off skipped:\n%s", out)
	}

	// The wildcard reaches every rule that can be configured, and no others.
	out = mustRun(t, "check", "--chart", chartDir, "--no-render", "--off", "*")
	if !strings.Contains(out, "no findings") {
		t.Errorf(`--off "*" left findings:\n%s`, out)
	}
	if strings.Contains(out, "HCK001") {
		t.Errorf(`--off "*" reached the locked rule:\n%s`, out)
	}

	// The same refusals the file gets: a rule nobody has, and the one rule
	// that cannot be configured at all.
	for _, id := range []string{"HCK999", "HCK001"} {
		if _, err := run(t, "check", "--chart", chartDir, "--no-render", "--off", id); err == nil {
			t.Errorf("--off %s was accepted", id)
		}
	}
}

// readFile is the read half of writeFile, for the tests that care that a file
// did not move.
func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// Every preset has to survive its own resources being switched on.
//
// TestCheckRendersTheGeneratedChart renders each preset on its defaults, and
// every optional resource defaults to off — so a preset that carries a
// VirtualService and a ScaledObject passes that test while proving nothing
// about either. This turns on everything the preset actually brought.
//
// The fixture is filtered to the keys the chart's own values.yaml declares.
// Setting a key a chart never declared is a different test: it is what a user
// typo looks like, not what a preset does.
func TestEveryPresetRendersWithItsResourcesOn(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not on PATH")
	}
	fixture := map[string]any{}
	raw, err := os.ReadFile(filepath.Join("testdata", "enable-all.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}

	parent := t.TempDir()
	for _, preset := range catalog.PresetNames() {
		t.Run(preset, func(t *testing.T) {
			mustRun(t, "new", preset, "--dir", parent, "--preset", preset)
			dir := filepath.Join(parent, preset)

			declared := map[string]any{}
			if err := yaml.Unmarshal([]byte(readFile(t, filepath.Join(dir, "values.yaml"))), &declared); err != nil {
				t.Fatal(err)
			}
			on := map[string]any{}
			for key, value := range fixture {
				if _, ok := declared[key]; ok {
					on[key] = value
				}
			}
			// The fixture wires its Certificate to an Issuer named "all",
			// which is a chart in another test. Here there is no Issuer at
			// all, so the Certificate keeps its default ClusterIssuer.
			delete(on, "certificate")
			if _, ok := declared["certificate"]; ok {
				on["certificate"] = map[string]any{"enabled": true}
			}

			values := filepath.Join(t.TempDir(), "on.yaml")
			out, err := yaml.Marshal(on)
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, values, string(out))

			report := mustRun(t, "check", "--chart", dir, "-f", values)
			if !strings.Contains(report, "no findings") {
				t.Fatalf("preset %s does not pass its own check with its resources on:\n%s", preset, report)
			}
		})
	}
}

// The resource listing is grouped, and the group headers carry the "@" that
// makes them usable: reading "@observability" in the listing is the whole way
// somebody learns that "hck add @observability" is a thing.
func TestListResourcesIsGrouped(t *testing.T) {
	out := mustRun(t, "list", "resources")
	for _, g := range catalog.Groups() {
		if !strings.Contains(out, "@"+string(g.Name)) {
			t.Errorf("group %q has no header in the listing:\n%s", g.Name, out)
		}
		if !strings.Contains(out, g.Summary) {
			t.Errorf("group %q has no summary in the listing", g.Name)
		}
	}
	// Every resource still appears — grouping must not drop any.
	for _, r := range catalog.Resources() {
		if !strings.Contains(out, r.Name) {
			t.Errorf("resource %q is not in the listing", r.Name)
		}
	}
	if !strings.Contains(out, "hck add @observability") {
		t.Errorf("the listing does not say what an @name is for:\n%s", out)
	}
}

// Tab completion offers the groups beside the resources, "@" included.
func TestGroupArgsCarryThePrefix(t *testing.T) {
	got := groupArgs()
	if len(got) != len(catalog.GroupNames()) {
		t.Fatalf("groupArgs has %d, there are %d groups", len(got), len(catalog.GroupNames()))
	}
	for i, name := range catalog.GroupNames() {
		if got[i] != "@"+name {
			t.Errorf("position %d is %q, want %q", i, got[i], "@"+name)
		}
	}
}

// And the group reaches the chart through the CLI, not only through resolve.
func TestAddAcceptsAGroup(t *testing.T) {
	dir := t.TempDir()
	if _, err := run(t, "new", "demo", "-d", dir, "--preset", "minimal"); err != nil {
		t.Fatal(err)
	}
	chartDir := filepath.Join(dir, "demo")
	if _, err := run(t, "add", "@observability", "--chart", chartDir); err != nil {
		t.Fatalf("hck add @observability: %v", err)
	}
	for _, r := range catalog.ResourcesInGroup(catalog.ObservabilityGroup) {
		_, err := os.Stat(filepath.Join(chartDir, "templates", r.File))
		if r.Platform != "" {
			// A group never drags in a resource that exists on one platform
			// only, so this one has to be absent.
			if err == nil {
				t.Errorf("%s is %s only and the group pulled it in", r.Name, r.Platform)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s was not written: %v", r.File, err)
		}
	}
}

// The JSON listing is the interface a script reads. The table is not: its
// indentation changed once and took a workflow parsing it with awk down with
// it, which is why this exists at all.
func TestListJSON(t *testing.T) {
	var doc struct {
		Groups []struct {
			Name, Summary string
		} `json:"groups"`
		Presets []struct {
			Name, Summary, Platform string
			Resources               []string
			Schema, Docs            bool
		} `json:"presets"`
		Resources []struct {
			Name, Group, File, APIVersion, Platform string
			ValuesKeys                              []string
			Optional, Workload                      bool
		} `json:"resources"`
		Rules []struct {
			ID, Severity, Summary string
			Locked                bool
		} `json:"rules"`
	}
	mustDecode := func(t *testing.T, args ...string) {
		t.Helper()
		doc.Groups, doc.Presets, doc.Resources, doc.Rules = nil, nil, nil, nil
		out := mustRun(t, append(args, "--format", "json")...)
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("not JSON: %v\n%s", err, out)
		}
	}

	// Naming a section leaves the others out entirely, so a consumer can tell
	// "none" from "not asked".
	mustDecode(t, "list", "presets")
	if len(doc.Presets) != len(catalog.Presets()) {
		t.Errorf("presets: got %d want %d", len(doc.Presets), len(catalog.Presets()))
	}
	if doc.Resources != nil || doc.Rules != nil {
		t.Error("hck list presets carried resources or rules")
	}

	mustDecode(t, "list", "resources")
	if len(doc.Resources) != len(catalog.Resources()) {
		t.Errorf("resources: got %d want %d", len(doc.Resources), len(catalog.Resources()))
	}
	if len(doc.Groups) != len(catalog.Groups()) {
		t.Errorf("groups: got %d want %d", len(doc.Groups), len(catalog.Groups()))
	}
	if doc.Presets != nil {
		t.Error("hck list resources carried presets")
	}
	// The fields the CI steps and any consumer actually read.
	var gke int
	for _, r := range doc.Resources {
		if r.Name == "" || r.Group == "" || r.File == "" || r.APIVersion == "" {
			t.Errorf("resource %+v has an empty field", r)
		}
		if r.Platform == "gcp" {
			gke++
		}
	}
	if gke == 0 {
		t.Error("no resource reports a platform; the field is not being emitted")
	}

	mustDecode(t, "list", "rules")
	if len(doc.Rules) != len(check.Rules()) {
		t.Errorf("rules: got %d want %d", len(doc.Rules), len(check.Rules()))
	}

	// And "all" carries every section at once.
	mustDecode(t, "list")
	if len(doc.Presets) == 0 || len(doc.Resources) == 0 || len(doc.Rules) == 0 || len(doc.Groups) == 0 {
		t.Errorf("hck list --format json left a section out: %d presets, %d resources, %d rules, %d groups",
			len(doc.Presets), len(doc.Resources), len(doc.Rules), len(doc.Groups))
	}
}

func TestListRejectsAnUnknownFormat(t *testing.T) {
	if _, err := run(t, "list", "--format", "yaml"); err == nil {
		t.Fatal("want an error for an unknown format")
	}
}

// hck sync answers "which files were compared, and how did each come out".
// An exit status cannot carry that, and the text report carries it in a
// column, so this is the form a CI step should read.
func TestSyncJSON(t *testing.T) {
	dir := t.TempDir()
	if _, err := run(t, "new", "demo", "-d", dir, "--preset", "minimal"); err != nil {
		t.Fatal(err)
	}
	chartDir := filepath.Join(dir, "demo")

	decode := func(t *testing.T, args ...string) (doc struct {
		Chart string `json:"chart"`
		Files []struct {
			Resource, Path, State, Error string
			Skeleton                     bool
		} `json:"files"`
		OK bool `json:"ok"`
	}) {
		t.Helper()
		out, err := run(t, args...)
		if err != nil && out == "" {
			t.Fatalf("%v", err)
		}
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("not JSON: %v\n%s", err, out)
		}
		return doc
	}

	// A chart hck just wrote is by definition what hck writes.
	doc := decode(t, "sync", "--chart", chartDir, "--format", "json")
	if !doc.OK {
		t.Errorf("a freshly generated chart reported drift: %+v", doc.Files)
	}
	if doc.Chart != "demo" {
		t.Errorf("chart is %q", doc.Chart)
	}
	if len(doc.Files) == 0 {
		t.Fatal("nothing was compared")
	}
	// The two files the author owns are absent rather than reported current.
	for _, f := range doc.Files {
		if f.Path == "Chart.yaml" || f.Path == "values.yaml" {
			t.Errorf("%s was compared", f.Path)
		}
		if f.State != "current" {
			t.Errorf("%s is %q on a fresh chart", f.Path, f.State)
		}
	}

	// Edit one and delete another, and each comes back as its own state.
	helpers := filepath.Join(chartDir, "templates", "_helpers.tpl")
	body, err := os.ReadFile(helpers)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(helpers, append(body, []byte("\n{{/* mine */}}\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(chartDir, "ci", "install-values.yaml")); err != nil {
		t.Fatal(err)
	}
	doc = decode(t, "sync", "--chart", chartDir, "--format", "json")
	if doc.OK {
		t.Error("a chart with drift reported ok")
	}
	states := map[string]string{}
	for _, f := range doc.Files {
		states[f.Path] = f.State
	}
	if states["templates/_helpers.tpl"] != "edited" {
		t.Errorf("_helpers.tpl is %q, want edited", states["templates/_helpers.tpl"])
	}
	if states["ci/install-values.yaml"] != "missing" {
		t.Errorf("ci/install-values.yaml is %q, want missing", states["ci/install-values.yaml"])
	}

	// --check still fails, and still prints the document.
	if _, err := run(t, "sync", "--chart", chartDir, "--format", "json", "--check"); err == nil {
		t.Error("--check passed a chart with drift")
	}
}
