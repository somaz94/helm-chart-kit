package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/somaz94/helm-chart-kit/internal/chart"
)

func TestValidateName(t *testing.T) {
	ok := []string{"demo", "payments-api", "a", "a.b", "x_y", "app1"}
	bad := []string{"", "Demo", "-demo", "demo-", "de mo", "démo", strings.Repeat("a", 64)}
	for _, n := range ok {
		if err := ValidateName(n); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", n, err)
		}
	}
	for _, n := range bad {
		if err := ValidateName(n); err == nil {
			t.Errorf("ValidateName(%q) = nil, want an error", n)
		}
	}
}

func newChart(t *testing.T, preset string, extra ...string) *chart.Chart {
	t.Helper()
	parent := t.TempDir()
	plan, err := PlanNew(NewOptions{
		Parent: parent, Name: "demo", Description: "d",
		Version: "0.1.0", AppVersion: "1.0.0", Preset: preset, Extra: extra,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	c, err := chart.Load(plan.ChartDir)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPlanNewWritesTheExpectedTree(t *testing.T) {
	c := newChart(t, "web")
	for _, f := range []string{
		"Chart.yaml", "values.yaml", ".helmignore", "ci/install-values.yaml",
		"templates/_helpers.tpl", "templates/NOTES.txt",
		"templates/deployment.yaml", "templates/service.yaml",
		"templates/tests/test-connection.yaml",
	} {
		if _, err := os.Stat(filepath.Join(c.Dir, filepath.FromSlash(f))); err != nil {
			t.Errorf("%s was not written", f)
		}
	}
	if c.Meta.Annotations["helm-chart-kit/preset"] != "web" {
		t.Error("preset was not recorded in Chart.yaml")
	}
}

func TestPlanNewRejectsBadInput(t *testing.T) {
	base := NewOptions{Parent: t.TempDir(), Name: "demo", Version: "0.1.0", AppVersion: "1", Preset: "web"}

	bad := base
	bad.Name = "Demo"
	if _, err := PlanNew(bad); err == nil {
		t.Error("want an error for an invalid chart name")
	}

	bad = base
	bad.Preset = "nope"
	if _, err := PlanNew(bad); err == nil {
		t.Error("want an error for an unknown preset")
	}

	bad = base
	bad.Extra = []string{"nope"}
	if _, err := PlanNew(bad); err == nil {
		t.Error("want an error for an unknown resource")
	}
}

func TestPlanNewRefusesANonEmptyDirectory(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := PlanNew(NewOptions{Parent: parent, Name: "demo", Version: "0.1.0", AppVersion: "1", Preset: "web"})
	if err == nil {
		t.Fatal("want an error, got nil — an existing chart would have been clobbered")
	}
	if !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPlanAddAppendsWithoutTouchingWhatIsThere(t *testing.T) {
	c := newChart(t, "web")
	before, err := os.ReadFile(c.ValuesPath())
	if err != nil {
		t.Fatal(err)
	}

	plan, err := PlanAdd(c, []string{"servicemonitor"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(plan); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(c.ValuesPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(after), string(before)) {
		t.Fatal("values.yaml was rewritten rather than appended to")
	}
	if len(plan.ValuesAdded) != 1 || plan.ValuesAdded[0] != "metrics" {
		t.Fatalf("ValuesAdded = %v, want [metrics]", plan.ValuesAdded)
	}
	if !c.HasTemplate("servicemonitor.yaml") {
		t.Error("servicemonitor.yaml was not written")
	}
	// The resource needs a CRD, which is worth saying out loud.
	if len(plan.Notes) == 0 {
		t.Error("no note about the CRD requirement")
	}
}

func TestPlanAddIsIdempotent(t *testing.T) {
	c := newChart(t, "web")
	first, err := PlanAdd(c, []string{"pdb"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(first); err != nil {
		t.Fatal(err)
	}
	values1, _ := os.ReadFile(c.ValuesPath())

	second, err := PlanAdd(c, []string{"pdb"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed() {
		t.Fatal("a repeated add reported changes")
	}
	if err := Apply(second); err != nil {
		t.Fatal(err)
	}
	values2, _ := os.ReadFile(c.ValuesPath())
	if string(values1) != string(values2) {
		t.Fatal("a repeated add modified values.yaml")
	}
}

// pdb is already in the web preset, so this exercises the skip path on a
// resource the chart was created with.
func TestPlanAddSkipsPresetResources(t *testing.T) {
	c := newChart(t, "web")
	plan, err := PlanAdd(c, []string{"pdb"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Changed() {
		t.Error("adding a resource the preset already included reported changes")
	}
	var skipped bool
	for _, f := range plan.Files {
		if f.Path == "templates/pdb.yaml" && f.Action == Skip {
			skipped = true
		}
	}
	if !skipped {
		t.Error("templates/pdb.yaml was not reported as skipped")
	}
}

func TestPlanAddRefusesASecondWorkload(t *testing.T) {
	c := newChart(t, "web") // has deployment
	_, err := PlanAdd(c, []string{"statefulset"}, false)
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	if !strings.Contains(err.Error(), "two") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := PlanAdd(c, []string{"statefulset"}, true); err != nil {
		t.Fatalf("--force should allow it: %v", err)
	}
}

func TestPlanAddReportsUnmetRequirements(t *testing.T) {
	// worker has no service, and ingress needs one.
	c := newChart(t, "worker")
	plan, err := PlanAdd(c, []string{"ingress"}, false)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, n := range plan.Notes {
		if strings.Contains(n, "hck add service") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no note about the missing service requirement: %v", plan.Notes)
	}
}

func TestPlanAddRejectsUnknownResource(t *testing.T) {
	c := newChart(t, "web")
	if _, err := PlanAdd(c, []string{"nope"}, false); err == nil {
		t.Error("want an error for an unknown resource")
	}
	if _, err := PlanAdd(c, nil, false); err == nil {
		t.Error("want an error when nothing was requested")
	}
}

// A requirement satisfied by another resource in the same command must not be
// reported as missing.
func TestPlanAddDoesNotWarnAboutSelfSatisfiedRequirements(t *testing.T) {
	c := newChart(t, "worker")
	plan, err := PlanAdd(c, []string{"service", "ingress"}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range plan.Notes {
		if strings.Contains(n, "hck add service") {
			t.Fatalf("warned about a requirement being added in the same run: %v", plan.Notes)
		}
	}
}

func TestOutputNameMapsHelmignore(t *testing.T) {
	if got := outputName("helmignore"); got != ".helmignore" {
		t.Fatalf("got %q, want .helmignore", got)
	}
	if got := outputName("Chart.yaml"); got != "Chart.yaml" {
		t.Fatalf("got %q, want Chart.yaml", got)
	}
}

func TestPlanChanged(t *testing.T) {
	if (&Plan{}).Changed() {
		t.Error("an empty plan reported changes")
	}
	if (&Plan{Files: []File{{Action: Skip}}}).Changed() {
		t.Error("a skip-only plan reported changes")
	}
	if !(&Plan{Files: []File{{Action: Skip}, {Action: Create}}}).Changed() {
		t.Error("a plan with a create reported no changes")
	}
}

func TestApplyReportsWriteFailures(t *testing.T) {
	dir := t.TempDir()
	// A file where the plan expects a directory: MkdirAll then fails.
	if err := os.WriteFile(filepath.Join(dir, "templates"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Apply(&Plan{ChartDir: dir, Files: []File{
		{Path: "templates/service.yaml", Action: Create, Content: []byte("x")},
	}})
	if err == nil {
		t.Fatal("want an error, got nil")
	}
}

func TestApplySkipsSkips(t *testing.T) {
	dir := t.TempDir()
	if err := Apply(&Plan{ChartDir: dir, Files: []File{
		{Path: "values.yaml", Action: Skip, Reason: "no new keys"},
	}}); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatal("a skip-only plan wrote something")
	}
}
