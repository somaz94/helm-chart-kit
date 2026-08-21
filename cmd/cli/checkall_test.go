package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/somaz94/helm-chart-kit/internal/chart"
	"github.com/somaz94/helm-chart-kit/internal/check"
)

// loadChart builds a chart and hands back the loaded value, so the enumeration
// is read off a real directory rather than a hand-made struct: what the chart
// carries is what is on disk, and that is the whole input to this code.
func loadChart(t *testing.T, dir, name string, args ...string) *chart.Chart {
	t.Helper()
	mustRun(t, append([]string{"new", name, "--dir", dir}, args...)...)
	c, err := chart.Load(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// The set is the product of both axes, each offering what the chart carries
// and none. Order is part of it: platform-major with the environment last
// within each, which is the order helm reads them in and the order a reader
// scanning the report expects.
func TestCombinationsForIsTheProductOfBothAxes(t *testing.T) {
	dir := t.TempDir()
	c := loadChart(t, dir, "ov", "--preset", "web", "--platform", "aws")
	mustRun(t, "platform", "add", "gcp", "--chart", c.Dir)
	mustRun(t, "env", "add", "prod", "--chart", c.Dir)
	mustRun(t, "env", "add", "dev", "--chart", c.Dir)

	var got []string
	for _, combo := range combinationsFor(c) {
		got = append(got, combo.label())
	}
	want := []string{
		"base", "dev", "prod",
		"aws", "aws + dev", "aws + prod",
		"gcp", "gcp + dev", "gcp + prod",
	}
	if strings.Join(got, ", ") != strings.Join(want, ", ") {
		t.Errorf("combinations =\n  %v\nwant\n  %v", got, want)
	}
}

// A chart with no overlays is one combination, and it is the one hck check has
// always run. --all has to be a superset of the old behaviour, never a
// different question asked of a chart that never opted into overlays.
func TestCombinationsForAChartWithNoOverlaysIsJustTheBase(t *testing.T) {
	dir := t.TempDir()
	c := loadChart(t, dir, "plain", "--preset", "minimal")

	combos := combinationsFor(c)
	if len(combos) != 1 {
		t.Fatalf("got %d combinations, want 1", len(combos))
	}
	if combos[0].label() != "base" {
		t.Errorf("label = %q, want %q", combos[0].label(), "base")
	}
	if files := combos[0].files(c.Dir); len(files) != 0 {
		t.Errorf("the base combination passes files: %v", files)
	}
}

// Never two overlays from one axis: a chart is installed on one platform at
// one size, and "aws + gcp" is not something to check.
func TestNoCombinationTakesTwoFromOneAxis(t *testing.T) {
	dir := t.TempDir()
	c := loadChart(t, dir, "ov", "--preset", "web", "--platform", "aws")
	mustRun(t, "platform", "add", "gcp", "--chart", c.Dir)
	mustRun(t, "env", "add", "prod", "--chart", c.Dir)

	for _, combo := range combinationsFor(c) {
		seen := map[string]bool{}
		for _, o := range combo.overlays {
			if seen[string(o.Axis)] {
				t.Errorf("%q takes two overlays from the %s axis", combo.label(), o.Axis)
			}
			seen[string(o.Axis)] = true
		}
		if len(combo.overlays) > 2 {
			t.Errorf("%q has %d overlays; there are only two axes", combo.label(), len(combo.overlays))
		}
	}
}

// The files come out in the order helm has to read them: platform first, so
// the environment's size wins over whatever the platform set.
func TestCombinationFilesPutTheEnvironmentLast(t *testing.T) {
	dir := t.TempDir()
	c := loadChart(t, dir, "ov", "--preset", "web", "--platform", "aws")
	mustRun(t, "env", "add", "prod", "--chart", c.Dir)

	for _, combo := range combinationsFor(c) {
		if combo.label() != "aws + prod" {
			continue
		}
		files := combo.files(c.Dir)
		if len(files) != 2 ||
			filepath.Base(files[0]) != "values-aws.yaml" ||
			filepath.Base(files[1]) != "values-prod.yaml" {
			t.Fatalf("files = %v, want values-aws.yaml then values-prod.yaml", files)
		}
		return
	}
	t.Fatal("no aws + prod combination")
}

// A clean combination says nothing beyond ok, and every non-zero count is
// named — a tally that folded infos into warnings would report a passing
// prerequisite as a complaint.
func TestTally(t *testing.T) {
	for _, tc := range []struct {
		name string
		rep  *check.Report
		want string
	}{
		{"clean", &check.Report{}, ""},
		{"one of each", &check.Report{Findings: []check.Finding{
			{Severity: check.Error}, {Severity: check.Warn}, {Severity: check.Info},
		}}, "1 error(s), 1 warning(s), 1 info(s)"},
		{"only a note", &check.Report{Findings: []check.Finding{
			{Severity: check.Info},
		}}, "1 info(s)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tally(tc.rep); got != tc.want {
				t.Errorf("tally = %q, want %q", got, tc.want)
			}
		})
	}
}

// --all and a hand-written loop over --platform/--env must reach the same
// verdict for the same combination, or one of the two is lying.
func TestPassesMatchesTheSingleRunVerdict(t *testing.T) {
	warned := &check.Report{Findings: []check.Finding{{Severity: check.Warn}}}
	noted := &check.Report{Findings: []check.Finding{{Severity: check.Info}}}
	broken := &check.Report{Findings: []check.Finding{{Severity: check.Error}}}

	for _, tc := range []struct {
		name   string
		rep    *check.Report
		strict bool
		want   bool
	}{
		{"a warning passes", warned, false, true},
		{"a warning fails --strict", warned, true, false},
		{"a note passes", noted, false, true},
		{"a note passes --strict too", noted, true, true},
		{"an error fails", broken, false, false},
		{"an error fails --strict", broken, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := passes(tc.rep, tc.strict); got != tc.want {
				t.Errorf("passes = %v, want %v", got, tc.want)
			}
		})
	}
}
