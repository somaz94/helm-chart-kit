package scaffold

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/somaz94/helm-chart-kit/internal/catalog"
	"github.com/somaz94/helm-chart-kit/internal/chart"
	"github.com/somaz94/helm-chart-kit/internal/render"
)

// A chart hck just wrote is, by definition, what hck writes.
func TestDriftOfChartIsCleanOnAFreshChart(t *testing.T) {
	c := newChart(t, "web")
	drifts, err := DriftOfChart(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) == 0 {
		t.Fatal("a web chart compared to nothing")
	}
	for _, d := range drifts {
		if d.State != Current {
			t.Errorf("%s is %s, want %s", d.Path, d.State, Current)
		}
	}
	if AnyDrifted(drifts) {
		t.Error("AnyDrifted said yes about a chart hck just generated")
	}
}

func TestDriftOfSeesAnEdit(t *testing.T) {
	c := newChart(t, "web")
	hpa, _ := catalog.LookupResource("hpa")

	d, err := DriftOf(c, hpa)
	if err != nil {
		t.Fatal(err)
	}
	if d.State != Current || !slices.Equal(d.Have, d.Want) {
		t.Fatalf("a fresh template is %s", d.State)
	}

	appendTo(t, c.TemplatePath(hpa.File), "\n# a local edit\n")
	d, err = DriftOf(c, hpa)
	if err != nil {
		t.Fatal(err)
	}
	if d.State != Edited {
		t.Errorf("State = %s, want %s", d.State, Edited)
	}
	if !strings.Contains(string(d.Have), "a local edit") {
		t.Error("Have does not carry the edit")
	}
	if strings.Contains(string(d.Want), "a local edit") {
		t.Error("Want carries the edit, so it is not what hck generates")
	}
}

// A file that is gone and a file that cannot be read are both distinct from
// one that was edited: reporting either as edited would invite
// "hck sync --write" to overwrite something nobody has seen.
func TestDriftOfSeparatesGoneFromUnreadable(t *testing.T) {
	c := newChart(t, "web")
	hpa, _ := catalog.LookupResource("hpa")

	if err := os.Remove(c.TemplatePath(hpa.File)); err != nil {
		t.Fatal(err)
	}
	d, err := DriftOf(c, hpa)
	if err != nil {
		t.Fatal(err)
	}
	if d.State != Missing || d.Err == nil {
		t.Errorf("State = %s, Err = %v, want %s", d.State, d.Err, Missing)
	}
	if !AnyDrifted([]Drift{d}) {
		t.Error("a missing template is not drift")
	}

	pdb, _ := catalog.LookupResource("pdb")
	if err := os.Chmod(c.TemplatePath(pdb.File), 0o000); err != nil {
		t.Skip("cannot make a file unreadable here")
	}
	t.Cleanup(func() { _ = os.Chmod(c.TemplatePath(pdb.File), 0o644) })
	d, err = DriftOf(c, pdb)
	if err != nil {
		t.Fatal(err)
	}
	if d.State != Unreadable || d.Err == nil {
		t.Errorf("State = %s, Err = %v, want %s", d.State, d.Err, Unreadable)
	}
}

func TestWriteTemplateTakesHcksVersion(t *testing.T) {
	c := newChart(t, "web")
	hpa, _ := catalog.LookupResource("hpa")
	appendTo(t, c.TemplatePath(hpa.File), "\n# a local edit\n")

	d, err := DriftOf(c, hpa)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteTemplate(c, d); err != nil {
		t.Fatal(err)
	}
	after, err := DriftOf(c, hpa)
	if err != nil {
		t.Fatal(err)
	}
	if after.State != Current {
		t.Errorf("State = %s after a write, want %s", after.State, Current)
	}
}

// Taking hck's version of a template the chart does not have would add a
// resource, and adding one is "hck add" — where the values it needs are
// appended too.
func TestWriteTemplateRefusesAFileThatIsNotThere(t *testing.T) {
	c := newChart(t, "web")
	hpa, _ := catalog.LookupResource("hpa")
	d, err := DriftOf(c, hpa)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(c.TemplatePath(hpa.File)); err != nil {
		t.Fatal(err)
	}
	if err := WriteTemplate(c, d); err == nil {
		t.Fatal("want an error")
	} else if !strings.Contains(err.Error(), "hck add") {
		t.Errorf("got %v, want a pointer to hck add", err)
	}
}

func TestPlanRemoveDeletesTheTemplateAndNamesTheKeys(t *testing.T) {
	c := newChart(t, "web")
	plan, err := PlanRemove(c, []string{"hpa"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 1 || plan.Files[0].Action != Delete ||
		plan.Files[0].Path != "templates/hpa.yaml" {
		t.Fatalf("got %+v", plan.Files)
	}
	if !slices.Equal(plan.ValuesOrphaned, []string{"autoscaling"}) {
		t.Errorf("ValuesOrphaned = %v, want [autoscaling]", plan.ValuesOrphaned)
	}
	if !plan.Changed() {
		t.Error("a plan that deletes a file says it changes nothing")
	}

	if err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(c.Dir, "templates", "hpa.yaml")); err == nil {
		t.Error("the template is still there")
	}
	// values.yaml is never rewritten, which is why the keys were named.
	raw, err := c.Values()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "autoscaling:") {
		t.Error("values.yaml lost a key to a removal")
	}
	// And applying the same deletion twice is not an error.
	if err := Apply(plan); err != nil {
		t.Errorf("re-applying a delete: %v", err)
	}
}

// A key two resources declare is not orphaned while the other one is still in
// the chart: "persistence" belongs to the StatefulSet as much as to the PVC.
func TestPlanRemoveLeavesASharedKeyAlone(t *testing.T) {
	c := newChart(t, "stateful", "pvc")
	plan, err := PlanRemove(c, []string{"pvc"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.ValuesOrphaned) != 0 {
		t.Errorf("ValuesOrphaned = %v, want none — the StatefulSet still declares persistence", plan.ValuesOrphaned)
	}
}

func TestPlanRemoveRefusals(t *testing.T) {
	t.Run("not in the chart", func(t *testing.T) {
		c := newChart(t, "worker")
		if _, err := PlanRemove(c, []string{"ingress"}, false); err == nil {
			t.Fatal("want an error")
		}
	})
	t.Run("unknown resource", func(t *testing.T) {
		c := newChart(t, "worker")
		if _, err := PlanRemove(c, []string{"nonsense"}, false); err == nil {
			t.Fatal("want an error")
		}
	})
	t.Run("still required", func(t *testing.T) {
		c := newChart(t, "web")
		_, err := PlanRemove(c, []string{"service"}, false)
		if err == nil || !strings.Contains(err.Error(), "required by") {
			t.Fatalf("got %v", err)
		}
		// --force is the way through, and removing the requirer as well is
		// the way that does not need one.
		if _, err := PlanRemove(c, []string{"service"}, true); err != nil {
			t.Errorf("--force: %v", err)
		}
		if _, err := PlanRemove(c, []string{"service", "ingress", "tests"}, false); err != nil {
			t.Errorf("removing them together: %v", err)
		}
	})
	t.Run("edited", func(t *testing.T) {
		c := newChart(t, "web")
		appendTo(t, c.TemplatePath("hpa.yaml"), "\n# a local edit\n")
		_, err := PlanRemove(c, []string{"hpa"}, false)
		if err == nil || !strings.Contains(err.Error(), "edited") {
			t.Fatalf("got %v", err)
		}
		if _, err := PlanRemove(c, []string{"hpa"}, true); err != nil {
			t.Errorf("--force: %v", err)
		}
	})
}

// A chart with a schema is where a leftover key actually bites, so that is
// where the plan has to say so.
func TestPlanRemoveWarnsAboutTheSchema(t *testing.T) {
	parent := t.TempDir()
	plan, err := PlanNew(NewOptions{
		Parent: parent, Name: "demo", Description: "d",
		Version: "0.1.0", AppVersion: "1.0.0", Preset: "web", Schema: true,
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

	rm, err := PlanRemove(c, []string{"hpa"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rm.Notes) != 1 || !strings.Contains(rm.Notes[0], SchemaFile) {
		t.Fatalf("Notes = %v", rm.Notes)
	}

	// Without a schema there is nothing to warn about.
	plain := newChart(t, "web")
	rm, err = PlanRemove(plain, []string{"hpa"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rm.Notes) != 0 {
		t.Errorf("Notes = %v, want none for a chart with no schema", rm.Notes)
	}
}

func appendTo(t *testing.T, path, text string) {
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

// The skeleton is compared by default: a file added to templates/chart/ is
// picked up by "hck sync" without anybody doing anything. That default is the
// right one and it is also the dangerous one — a file the chart's author is
// meant to maintain would be reported as drift forever, and --write would
// overwrite what they put in it.
//
// So the set is pinned. Adding or removing a skeleton file fails here, and
// clearing the failure means deciding which of the two it is.
func TestTheSkeletonSetIsADecision(t *testing.T) {
	want := []string{
		"Chart.yaml",
		"ci/install-values.yaml",
		"helmignore",
		"templates/NOTES.txt",
		"templates/_helpers.tpl",
		"values.yaml",
	}
	got, err := render.ChartFiles()
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("the chart skeleton changed.\n got %v\nwant %v\n\n"+
			"Decide which the new file is: one hck owns, which needs nothing "+
			"and is compared automatically, or one the author maintains, which "+
			"belongs in skeletonNotOwned with the reason. Then update this list.", got, want)
	}
}

// Chart.yaml and values.yaml are the two the author owns, and both exclusions
// are load-bearing: regenerating either would report drift on every chart that
// ever grew, and --write would delete the parts hck never wrote.
func TestTheAuthorsFilesAreNotCompared(t *testing.T) {
	c := newChart(t, "web")
	drifts, err := skeletonDrift(c)
	if err != nil {
		t.Fatal(err)
	}
	compared := map[string]bool{}
	for _, d := range drifts {
		if !d.Skeleton {
			t.Errorf("%s came out of skeletonDrift without the skeleton mark", d.Path)
		}
		compared[d.Path] = true
	}
	for name, reason := range skeletonNotOwned {
		if reason == "" {
			t.Errorf("%s is excused from the comparison with no reason given", name)
		}
		if compared[outputName(name)] {
			t.Errorf("%s is both excused and compared", name)
		}
	}
	for _, name := range []string{"Chart.yaml", "values.yaml"} {
		if _, excused := skeletonNotOwned[name]; !excused {
			t.Errorf("%s is compared, so hck sync --write would overwrite what the author put in it", name)
		}
	}
	// And what is left is actually compared, or the report is a stub.
	for _, want := range []string{".helmignore", "templates/_helpers.tpl", "templates/NOTES.txt", "ci/install-values.yaml"} {
		if !compared[want] {
			t.Errorf("%s is not compared by hck sync", want)
		}
	}
}

// The whole chart, skeleton included, is what hck writes the moment it writes
// it. This is what makes a later difference mean something.
func TestSkeletonIsCleanOnAFreshChart(t *testing.T) {
	drifts, err := DriftOfChart(newChart(t, "stateful"))
	if err != nil {
		t.Fatal(err)
	}
	var skeleton int
	for _, d := range drifts {
		if d.State != Current {
			t.Errorf("%s is %s on a chart hck just wrote", d.Path, d.State)
		}
		if d.Skeleton {
			skeleton++
		}
	}
	if skeleton == 0 {
		t.Fatal("DriftOfChart compared no skeleton file")
	}
}

// A skeleton file carries no values, so putting a missing one back is a
// repair. A missing resource template is a resource the chart does not have.
func TestWriteTemplatePutsAMissingSkeletonFileBack(t *testing.T) {
	c := newChart(t, "web")
	helpers := filepath.Join(c.Dir, "templates", "_helpers.tpl")
	if err := os.Remove(helpers); err != nil {
		t.Fatal(err)
	}
	drifts, err := skeletonDrift(c)
	if err != nil {
		t.Fatal(err)
	}
	var gone Drift
	for _, d := range drifts {
		if d.Path == "templates/_helpers.tpl" {
			gone = d
		}
	}
	if gone.State != Missing {
		t.Fatalf("State = %s, want %s", gone.State, Missing)
	}
	if err := WriteTemplate(c, gone); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(helpers); err != nil {
		t.Fatal("the helper set was not put back")
	}
}
