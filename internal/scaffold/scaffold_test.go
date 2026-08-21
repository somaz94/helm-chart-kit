package scaffold

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/somaz94/helm-chart-kit/internal/catalog"
	"github.com/somaz94/helm-chart-kit/internal/chart"
	"github.com/somaz94/helm-chart-kit/internal/render"
	"github.com/somaz94/helm-chart-kit/internal/values"
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

// Adding a workload beside one already there is the Deployment-to-Rollout
// swap, and no longer needs --force. The note counts the finished chart, so it
// names the one already in the chart as well as the one arriving.
func TestPlanAddNotesASecondWorkload(t *testing.T) {
	c := newChart(t, "web") // has deployment
	plan, err := PlanAdd(c, []string{"statefulset"}, false)
	if err != nil {
		t.Fatalf("want a plan, got %v", err)
	}
	if !hasNote(plan, "deployment") || !hasNote(plan, "statefulset") {
		t.Errorf("the note does not name both workloads: %v", plan.Notes)
	}
	if !plansFile(plan, "templates/statefulset.yaml") {
		t.Errorf("statefulset was not written: %v", plan.Files)
	}
}

// Two workload templates in one chart is a shape, not a defect: guarded so
// that one renders at a time, it is how a Deployment is swapped for a Rollout.
// PlanNew says so and builds it; whether both actually render is HCK030's
// question, asked over the render rather than over the resource names.
func TestPlanNewNotesTwoWorkloadsRatherThanRefusing(t *testing.T) {
	for _, tc := range []struct{ preset, extra string }{
		{"web", "daemonset"},
		{"stateful", "cronjob"},
		{"cronjob", "deployment"},
	} {
		t.Run(tc.preset+"+"+tc.extra, func(t *testing.T) {
			plan, err := PlanNew(NewOptions{
				Parent: t.TempDir(), Name: "demo", Description: "d",
				Version: "0.1.0", AppVersion: "1.0.0", Preset: tc.preset,
				Extra: []string{tc.extra},
			})
			if err != nil {
				t.Fatalf("want a plan, got %v", err)
			}
			if !hasNote(plan, "workload templates") {
				t.Errorf("no note about the second workload: %v", plan.Notes)
			}
			if !hasNote(plan, "HCK030") {
				t.Errorf("the note does not name the rule that answers it: %v", plan.Notes)
			}
			// The second workload is written, not quietly dropped.
			if !plansFile(plan, "templates/"+tc.extra+".yaml") {
				t.Errorf("%s was not written: %v", tc.extra, plan.Files)
			}
		})
	}
}

// hasNote reports whether any note contains the substring.
func hasNote(p *Plan, want string) bool {
	for _, n := range p.Notes {
		if strings.Contains(n, want) {
			return true
		}
	}
	return false
}

// plansFile reports whether the plan writes that path.
func plansFile(p *Plan, path string) bool {
	for _, f := range p.Files {
		if f.Path == path && f.Action != Skip {
			return true
		}
	}
	return false
}

// A workload the preset already carries is not a second one.
func TestPlanNewAllowsRedundantWith(t *testing.T) {
	if _, err := PlanNew(NewOptions{
		Parent: t.TempDir(), Name: "demo", Description: "d",
		Version: "0.1.0", AppVersion: "1.0.0", Preset: "web",
		Extra: []string{"deployment", "pvc"},
	}); err != nil {
		t.Fatalf("naming the preset's own workload is not a second workload: %v", err)
	}
}

// Two arriving in one command reaches the same note as one landing beside one
// already there — the count is over the finished chart either way.
func TestPlanAddNotesTwoWorkloadsAtOnce(t *testing.T) {
	c := newChart(t, "cronjob")
	if err := os.Remove(c.TemplatePath("cronjob.yaml")); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanAdd(c, []string{"deployment", "daemonset"}, false)
	if err != nil {
		t.Fatalf("want a plan, got %v", err)
	}
	if !hasNote(plan, "workload templates") {
		t.Errorf("no note about the two workloads: %v", plan.Notes)
	}
}

// One workload is the ordinary case and says nothing at all.
func TestPlanAddSaysNothingAboutOneWorkload(t *testing.T) {
	c := newChart(t, "minimal")
	plan, err := PlanAdd(c, []string{"pdb"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if hasNote(plan, "workload templates") {
		t.Errorf("a chart with one workload was told about workloads: %v", plan.Notes)
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

// newSchemaChart scaffolds a chart that carries a values.schema.json.
func newSchemaChart(t *testing.T, preset string, strict bool, extra ...string) *chart.Chart {
	t.Helper()
	parent := t.TempDir()
	plan, err := PlanNew(NewOptions{
		Parent: parent, Name: "demo", Description: "d",
		Version: "0.1.0", AppVersion: "1.0.0", Preset: preset, Extra: extra,
		Schema: true, SchemaStrict: strict,
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

// schemaProps reads the top-level property names out of a chart's schema.
func schemaProps(t *testing.T, c *chart.Chart) map[string]bool {
	t.Helper()
	raw, err := c.Schema()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	out := map[string]bool{}
	for k := range doc.Properties {
		out[k] = true
	}
	return out
}

func TestPlanNewOmitsTheSchemaByDefault(t *testing.T) {
	c := newChart(t, "web")
	if c.HasSchema() {
		t.Error("a schema was written without --schema; Helm validates against it on every render, so it has to be opt-in")
	}
}

// Every key values.yaml declares has to be described, or Helm rejects the
// chart the moment it has a schema at all.
func TestPlanNewSchemaCoversEveryValuesKey(t *testing.T) {
	for _, preset := range catalog.PresetNames() {
		t.Run(preset, func(t *testing.T) {
			c := newSchemaChart(t, preset, false)
			raw, err := c.Values()
			if err != nil {
				t.Fatal(err)
			}
			keys, err := values.TopLevelKeys(raw)
			if err != nil {
				t.Fatal(err)
			}
			props := schemaProps(t, c)
			for _, k := range keys {
				if !props[k] {
					t.Errorf("values.yaml declares %q but the schema does not describe it", k)
				}
			}
		})
	}
}

// isStrict reads a chart's schema and reports whether it closes the top level.
func isStrict(t *testing.T, c *chart.Chart) bool {
	t.Helper()
	raw, err := c.Schema()
	if err != nil {
		t.Fatal(err)
	}
	return SchemaIsStrictBytes(raw)
}

func TestPlanNewSchemaStrictClosesTheTopLevel(t *testing.T) {
	if !isStrict(t, newSchemaChart(t, "web", true)) {
		t.Error("--schema-strict did not close the top level")
	}
	plain := newSchemaChart(t, "web", false)
	if !plain.HasSchema() {
		t.Error("--schema alone wrote no schema")
	}
	if isStrict(t, plain) {
		t.Error("a plain --schema chart reports as strict")
	}
}

func TestSchemaIsStrictBytes(t *testing.T) {
	for name, tc := range map[string]struct {
		raw  string
		want bool
	}{
		"closed":      {`{"additionalProperties": false}`, true},
		"open":        {`{"additionalProperties": true}`, false},
		"unset":       {`{"type": "object"}`, false},
		"absent file": {``, false},
		"unparseable": {`{not json`, false},
		"wrong type":  {`{"additionalProperties": {"type": "string"}}`, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := SchemaIsStrictBytes([]byte(tc.raw)); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPlanAddRegeneratesAnExistingSchema(t *testing.T) {
	c := newSchemaChart(t, "web", false)
	if schemaProps(t, c)["metrics"] {
		t.Fatal("the web preset already describes metrics")
	}

	plan, err := PlanAdd(c, []string{"servicemonitor"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	if !schemaProps(t, c)["metrics"] {
		t.Error("adding servicemonitor left values.schema.json behind, which breaks the chart")
	}

	var action Action
	for _, f := range plan.Files {
		if f.Path == SchemaFile {
			action = f.Action
		}
	}
	if action != Update {
		t.Errorf("schema action = %q, want %q", action, Update)
	}
}

func TestPlanAddLeavesAChartWithNoSchemaAlone(t *testing.T) {
	c := newChart(t, "web")
	plan, err := PlanAdd(c, []string{"servicemonitor"}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range plan.Files {
		if f.Path == SchemaFile {
			t.Fatal("hck add introduced a schema the chart never asked for")
		}
	}
}

func TestPlanAddKeepsTheSchemaStrict(t *testing.T) {
	c := newSchemaChart(t, "web", true)
	plan, err := PlanAdd(c, []string{"servicemonitor"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	if !isStrict(t, c) {
		t.Error("regenerating dropped additionalProperties: false")
	}
}

func TestPlanAddSkipsAnUpToDateSchema(t *testing.T) {
	c := newSchemaChart(t, "web", false)
	plan, err := PlanAdd(c, []string{"configmap"}, false) // already in the web preset
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range plan.Files {
		if f.Path == SchemaFile && f.Action != Skip {
			t.Errorf("schema action = %q, want %q when nothing changed", f.Action, Skip)
		}
	}
}

// A chart read back off disk has to reassemble byte-for-byte, or --check
// reports drift the moment it is written.
func TestBuildSchemaRoundTripsThroughDisk(t *testing.T) {
	for _, preset := range catalog.PresetNames() {
		for _, strict := range []bool{false, true} {
			name := preset
			if strict {
				name += "-strict"
			}
			t.Run(name, func(t *testing.T) {
				c := newSchemaChart(t, preset, strict)
				onDisk, err := c.Schema()
				if err != nil {
					t.Fatal(err)
				}
				resources, err := ChartResources(c)
				if err != nil {
					t.Fatal(err)
				}
				rebuilt, _, err := BuildSchema(DataFor(c), resources, SchemaIsStrictBytes(onDisk))
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(onDisk, rebuilt) {
					t.Error("rebuilding from the chart directory does not reproduce the written schema")
				}
			})
		}
	}
}

// A resource named on the command line that the chart already has must not be
// counted twice when the schema is rebuilt.
func TestUnionDropsDuplicates(t *testing.T) {
	a, _ := catalog.LookupResource("configmap")
	b, _ := catalog.LookupResource("service")
	got := union([]catalog.Resource{a, b}, []catalog.Resource{a})
	if len(got) != 2 {
		t.Fatalf("union produced %d entries, want 2: %v", len(got), names(got))
	}
	if got[0].Name != "configmap" || got[1].Name != "service" {
		t.Errorf("union reordered its inputs: %v", names(got))
	}
}

// The schema is rebuilt from the chart plus what is being added, so re-adding
// a resource the chart already carries must leave the document unchanged.
func TestPlanAddReAddingAResourceLeavesTheSchemaAlone(t *testing.T) {
	c := newSchemaChart(t, "web", false)
	before, err := c.Schema()
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanAdd(c, []string{"configmap"}, false) // already in the preset
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	after, err := c.Schema()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("re-adding a resource the chart already has rewrote the schema")
	}
}

// The StatefulSet owns "persistence" when the chart has one, and the PVC owns
// it otherwise — the same resolution the values merge makes.
func TestCanonicalOrderPutsTheWorkloadFirst(t *testing.T) {
	c := newSchemaChart(t, "stateful", false, "pvc")
	raw, err := c.Schema()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Properties struct {
			Persistence struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"persistence"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc.Properties.Persistence.Properties["mountPath"]; !ok {
		t.Error("persistence came from the PVC, but the StatefulSet contributes it to values.yaml first")
	}

	// The schema order is computed independently of the values merge, so the
	// claim that the two agree has to be checked on both sides, not inferred.
	vals, err := c.Values()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(vals), "mountPath:") {
		t.Error("values.yaml took persistence from the PVC while the schema took it from the StatefulSet")
	}
	if strings.Contains(string(vals), "existingClaim:") {
		t.Error("values.yaml took persistence from the PVC")
	}
}

func TestSchemaIsStrictHandlesAMissingOrBrokenFile(t *testing.T) {
	c := newChart(t, "web")
	if isStrict(t, c) {
		t.Error("a chart with no schema reported as strict")
	}
	if err := os.WriteFile(c.SchemaPath(), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if isStrict(t, c) {
		t.Error("an unparseable schema reported as strict; the permissive guess is the safe one")
	}
}

// An unreadable schema is a real error, not a reason to guess permissive and
// silently rewrite the author's strict file.
func TestPlanAddSurfacesAnUnreadableSchema(t *testing.T) {
	c := newSchemaChart(t, "web", true)
	if err := os.Remove(c.SchemaPath()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(c.SchemaPath(), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := PlanAdd(c, []string{"servicemonitor"}, false); err == nil {
		t.Error("PlanAdd swallowed an unreadable values.schema.json")
	}
}

func TestChartResourcesReadsTheTemplateDirectory(t *testing.T) {
	c := newChart(t, "worker")
	got, err := ChartResources(c)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, r := range got {
		names[r.Name] = true
	}
	for _, want := range []string{"deployment", "serviceaccount", "pdb", "networkpolicy", "configmap"} {
		if !names[want] {
			t.Errorf("worker chart is missing %q", want)
		}
	}
	if names["ingress"] {
		t.Error("worker chart reported an ingress it does not have")
	}
	if !got[0].Workload {
		t.Errorf("first resource is %q, want the workload", got[0].Name)
	}
}

func TestBuildOverlayValuesForAPlatform(t *testing.T) {
	c := newChart(t, "web")
	resources, err := ChartResources(c)
	if err != nil {
		t.Fatal(err)
	}
	aws, _ := catalog.LookupOverlay(catalog.PlatformAxis, "aws")

	out, ok, err := BuildOverlayValues(DataFor(c), resources, aws)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	body := string(out)
	for _, want := range []string{
		"# =====",                                // header
		"helm install demo . -f values-aws.yaml", // the command it is for
		"AWS Load Balancer Controller",           // what it expects
		"eks.amazonaws.com/role-arn",             // serviceaccount
		"alb.ingress.kubernetes.io/target-type",  // ingress
	} {
		if !strings.Contains(body, want) {
			t.Errorf("overlay is missing %q", want)
		}
	}
	// It has to parse, or helm rejects the -f.
	if _, err := values.TopLevelKeys(out); err != nil {
		t.Fatalf("overlay is not valid YAML: %v", err)
	}
}

// A chart whose resources do not differ on a platform gets no file at all,
// rather than one consisting of a header.
func TestBuildOverlayValuesEmptyWhenNothingDiffersOnAPlatform(t *testing.T) {
	aws, _ := catalog.LookupOverlay(catalog.PlatformAxis, "aws")
	cm, _ := catalog.LookupResource("configmap")
	_, ok, err := BuildOverlayValues(DataFor(newChart(t, "web")), []catalog.Resource{cm}, aws)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("wrote an overlay for a chart with nothing platform-specific in it")
	}
}

// The overlay only carries keys the chart's own resources contribute; a
// worker has no Ingress, so no ingress annotations.
func TestBuildOverlayValuesFollowsTheChartsResources(t *testing.T) {
	c := newChart(t, "worker")
	resources, err := ChartResources(c)
	if err != nil {
		t.Fatal(err)
	}
	aws, _ := catalog.LookupOverlay(catalog.PlatformAxis, "aws")
	out, ok, err := BuildOverlayValues(DataFor(c), resources, aws)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if strings.Contains(string(out), "alb.ingress") {
		t.Error("worker chart got ingress annotations for an Ingress it does not have")
	}
	if !strings.Contains(string(out), "eks.amazonaws.com/role-arn") {
		t.Error("worker chart lost the ServiceAccount annotation it should have")
	}
}

func TestPlanNewWritesPlatformOverlays(t *testing.T) {
	parent := t.TempDir()
	plan, err := PlanNew(NewOptions{
		Parent: parent, Name: "demo", Description: "d",
		Version: "0.1.0", AppVersion: "1.0.0", Preset: "web",
		Platforms: []string{"aws", "gcp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"values-aws.yaml", "values-gcp.yaml"} {
		if _, err := os.Stat(filepath.Join(plan.ChartDir, name)); err != nil {
			t.Errorf("%s was not written: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(plan.ChartDir, "values-azure.yaml")); err == nil {
		t.Error("wrote an overlay that was not asked for")
	}
	// The note tells the user what the platform expects to already exist.
	var noted bool
	for _, n := range plan.Notes {
		if strings.Contains(n, "AWS Load Balancer Controller") {
			noted = true
		}
	}
	if !noted {
		t.Errorf("plan does not report what aws expects: %v", plan.Notes)
	}
}

func TestPlanNewRejectsAnUnknownPlatform(t *testing.T) {
	_, err := PlanNew(NewOptions{
		Parent: t.TempDir(), Name: "demo", Description: "d",
		Version: "0.1.0", AppVersion: "1.0.0", Preset: "web",
		Platforms: []string{"nope"},
	})
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "unknown platform") {
		t.Errorf("error = %v", err)
	}
}

func TestChartOverlaysOnThePlatformAxis(t *testing.T) {
	c := newChart(t, "web")
	if got := ChartOverlays(c, catalog.PlatformAxis); len(got) != 0 {
		t.Errorf("a fresh chart reports %v", got)
	}
	aws, _ := catalog.LookupOverlay(catalog.PlatformAxis, "aws")
	if err := os.WriteFile(filepath.Join(c.Dir, aws.ValuesFile()), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := ChartOverlays(c, catalog.PlatformAxis)
	if len(got) != 1 || got[0].Name != "aws" {
		t.Errorf("got %v, want just aws", got)
	}
}

func TestBuildOverlayValuesForAnEnvironment(t *testing.T) {
	c := newChart(t, "web")
	resources, err := ChartResources(c)
	if err != nil {
		t.Fatal(err)
	}
	prod, _ := catalog.LookupOverlay(catalog.EnvironmentAxis, "prod")

	out, ok, err := BuildOverlayValues(DataFor(c), resources, prod)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	body := string(out)

	// The header has to carry the ordering hint. Overlays apply left to
	// right, so an environment passed before a platform loses to it — and
	// nothing else in the suite pins which way round the two go.
	if !strings.Contains(body, "[-f values-<platform>.yaml] -f values-prod.yaml") {
		t.Errorf("the install line does not put the environment last:\n%s", firstLines(body, 12))
	}
	// An environment is a size, not a prerequisite; it has nothing to demand
	// of the cluster the way a platform does.
	if strings.Contains(body, "Expects the cluster to already have") {
		t.Error("an environment overlay claimed a cluster prerequisite")
	}
	for _, want := range []string{"replicaCount: 3", "podDisruptionBudget", "autoscaling"} {
		if !strings.Contains(body, want) {
			t.Errorf("overlay is missing %q", want)
		}
	}
	if _, err := values.TopLevelKeys(out); err != nil {
		t.Fatalf("overlay is not valid YAML: %v", err)
	}
}

func TestBuildOverlayValuesEmptyWhenNothingDiffersAtAnEnvironment(t *testing.T) {
	dev, _ := catalog.LookupOverlay(catalog.EnvironmentAxis, "dev")
	sa, _ := catalog.LookupResource("serviceaccount")
	_, ok, err := BuildOverlayValues(DataFor(newChart(t, "web")), []catalog.Resource{sa}, dev)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("wrote an overlay for a chart with nothing size-dependent in it")
	}
}

func TestPlanNewWritesEnvironmentOverlays(t *testing.T) {
	parent := t.TempDir()
	plan, err := PlanNew(NewOptions{
		Parent: parent, Name: "demo", Description: "d",
		Version: "0.1.0", AppVersion: "1.0.0", Preset: "web",
		Environments: []string{"dev", "prod"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"values-dev.yaml", "values-prod.yaml"} {
		if _, err := os.Stat(filepath.Join(plan.ChartDir, name)); err != nil {
			t.Errorf("%s was not written: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(plan.ChartDir, "values-staging.yaml")); err == nil {
		t.Error("wrote an overlay that was not asked for")
	}

	if _, err := PlanNew(NewOptions{
		Parent: t.TempDir(), Name: "demo", Description: "d",
		Version: "0.1.0", AppVersion: "1.0.0", Preset: "web",
		Environments: []string{"nope"},
	}); err == nil {
		t.Error("want an error for an unknown environment")
	}
}

func TestChartOverlaysOnTheEnvironmentAxis(t *testing.T) {
	c := newChart(t, "web")
	if got := ChartOverlays(c, catalog.EnvironmentAxis); len(got) != 0 {
		t.Errorf("a fresh chart reports %v", got)
	}
	prod, _ := catalog.LookupOverlay(catalog.EnvironmentAxis, "prod")
	if err := os.WriteFile(filepath.Join(c.Dir, prod.ValuesFile()), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := ChartOverlays(c, catalog.EnvironmentAxis)
	if len(got) != 1 || got[0].Name != "prod" {
		t.Errorf("got %v, want just prod", got)
	}
}

// --with can name a workload the preset does not satisfy the requirements of.
// "hck add" has always said so; "hck new" used to say nothing.
func TestPlanNewReportsUnmetRequirements(t *testing.T) {
	plan, err := PlanNew(NewOptions{
		Parent: t.TempDir(), Name: "demo", Description: "d",
		Version: "0.1.0", AppVersion: "1.0.0", Preset: "cronjob",
		Extra: []string{"referencegrant"}, // Requires: service
	})
	if err != nil {
		t.Fatal(err)
	}
	var noted bool
	for _, n := range plan.Notes {
		if strings.Contains(n, "referencegrant expects service") {
			noted = true
		}
	}
	if !noted {
		t.Errorf("plan does not report the unmet requirement: %v", plan.Notes)
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

// The advisory for an empty overlay reads as a sentence on either axis: a
// chart differs "on aws" but "at prod".
func TestNoOverlayNoteFollowsTheAxis(t *testing.T) {
	aws, _ := catalog.LookupOverlay(catalog.PlatformAxis, "aws")
	if got, want := noOverlayNote(aws),
		"nothing in this chart differs on aws, so no values-aws.yaml was written"; got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	prod, _ := catalog.LookupOverlay(catalog.EnvironmentAxis, "prod")
	if got, want := noOverlayNote(prod),
		"nothing in this chart differs at prod, so no values-prod.yaml was written"; got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// The workload kind a scaler is pointed at comes from a hand-written map, and
// a map that drifts from the templates aims the scaler at a name nothing
// renders — the exact failure HCK033 exists to catch, arriving from inside hck
// instead of from the user. So the map is checked against what the templates
// actually emit.
func TestWorkloadKindsMatchTheTemplates(t *testing.T) {
	data := render.Data{ChartName: "demo", Description: "d", Version: "0.1.0", AppVersion: "1.0.0"}
	for _, r := range catalog.Resources() {
		_, mapped := workloadKindByName[r.Name]
		if !r.Workload {
			if mapped {
				t.Errorf("%s is not a workload but has a kind mapped for it", r.Name)
			}
			continue
		}
		if !mapped {
			t.Errorf("%s is a workload with no kind mapped for it, so a scaler added beside it aims at nothing", r.Name)
			continue
		}
		body, err := render.ResourceTemplate(r.Name, data)
		if err != nil {
			t.Fatal(err)
		}
		m := kindLineRE.FindSubmatch(body)
		if m == nil {
			t.Errorf("%s renders no kind line", r.Name)
			continue
		}
		if got, want := string(m[1]), workloadKindByName[r.Name]; got != want {
			t.Errorf("%s renders kind %q, the map says %q", r.Name, got, want)
		}
	}
}

var kindLineRE = regexp.MustCompile(`(?m)^kind: (\w+)`)

func TestWorkloadKindOfAResourceSet(t *testing.T) {
	for name, want := range map[string]string{
		"deployment":  "Deployment",
		"statefulset": "StatefulSet",
		"daemonset":   "DaemonSet",
		"cronjob":     "CronJob",
	} {
		r, ok := catalog.LookupResource(name)
		if !ok {
			t.Fatalf("no %s", name)
		}
		sa, _ := catalog.LookupResource("serviceaccount")
		if got := workloadKind([]catalog.Resource{sa, r}); got != want {
			t.Errorf("workloadKind with %s = %q, want %q", name, got, want)
		}
	}
	cm, _ := catalog.LookupResource("configmap")
	if got := workloadKind([]catalog.Resource{cm}); got != "" {
		t.Errorf("workloadKind with no workload = %q, want empty", got)
	}
}

// A scaler added beside a StatefulSet has to point at the StatefulSet. It did
// not: the values fragment said Deployment whatever the chart carried, so
// "hck add hpa" against a stateful chart shipped an HPA that never scaled.
func TestScalerDefaultsFollowTheChartsWorkload(t *testing.T) {
	for preset, want := range map[string]struct{ hpa, vpa, keda string }{
		"web":      {"Deployment", "Deployment", "Deployment"},
		"stateful": {"StatefulSet", "StatefulSet", "StatefulSet"},
		// An HPA and a KEDA ScaledObject cannot target a DaemonSet at all, so
		// they keep the schema's other option rather than emitting one it
		// rejects. HCK033 is what then says the HPA has nothing to scale.
		"daemon":  {"Deployment", "DaemonSet", "Deployment"},
		"cronjob": {"Deployment", "Deployment", "Deployment"},
	} {
		t.Run(preset, func(t *testing.T) {
			c := newChart(t, preset)
			plan, err := PlanAdd(c, []string{"hpa", "vpa", "scaledobject"}, false)
			if err != nil {
				t.Fatal(err)
			}
			if err := Apply(plan); err != nil {
				t.Fatal(err)
			}
			raw, err := c.Values()
			if err != nil {
				t.Fatal(err)
			}
			got := targetKindsIn(string(raw))
			if len(got) != 3 {
				t.Fatalf("got %d targetKind lines, want 3: %v", len(got), got)
			}
			for i, want := range []string{want.hpa, want.vpa, want.keda} {
				if got[i] != want {
					t.Errorf("targetKind %d = %q, want %q (all: %v)", i, got[i], want, got)
				}
			}
		})
	}
}

// targetKindsIn reads the targetKind lines in the order the merge appends
// them, which is canonical order: hpa, then scaledobject, then vpa — so the
// caller compares against that order, not the order it asked in.
func targetKindsIn(values string) []string {
	var out []string
	for _, m := range targetKindRE.FindAllStringSubmatch(values, -1) {
		out = append(out, m[1])
	}
	return out
}

var targetKindRE = regexp.MustCompile(`(?m)^\s+targetKind: (\w+)`)

// --force is the escape hatch on the single-workload rule, and it is the same
// escape hatch "hck add" has always had. The chart it produces is still one
// "hck check" reports HCK030 over — waiving the refusal is not the same as
// the combination being fine.
func TestPlanNewForceAllowsTwoWorkloads(t *testing.T) {
	plan, err := PlanNew(NewOptions{
		Parent: t.TempDir(), Name: "demo", Description: "d",
		Version: "0.1.0", AppVersion: "1.0.0", Preset: "web",
		Extra: []string{"daemonset"}, Force: true,
	})
	if err != nil {
		t.Fatalf("--force should allow it: %v", err)
	}
	for _, want := range []string{"templates/deployment.yaml", "templates/daemonset.yaml"} {
		if !planHas(plan, want, Create) {
			t.Errorf("plan does not create %s", want)
		}
	}
}

// Forcing into a directory that is not empty fills in what is missing and
// leaves everything else alone. Overwriting would take values.yaml with it,
// and values.yaml is never rewritten.
func TestPlanNewForceKeepsWhatIsAlreadyThere(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "demo")
	if err := os.MkdirAll(filepath.Join(dir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	mine := []byte("# mine\nimage:\n  tag: pinned\n")
	for path, content := range map[string][]byte{
		"values.yaml":               mine,
		"templates/deployment.yaml": []byte("# hand-written\n"),
		"README.md":                 []byte("# demo\n"),
	} {
		if err := os.WriteFile(filepath.Join(dir, path), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	plan, err := PlanNew(NewOptions{
		Parent: parent, Name: "demo", Description: "d",
		Version: "0.1.0", AppVersion: "1.0.0", Preset: "web", Force: true,
	})
	if err != nil {
		t.Fatalf("--force should allow a directory that is not empty: %v", err)
	}
	for _, path := range []string{"values.yaml", "templates/deployment.yaml"} {
		if !planHas(plan, path, Skip) {
			t.Errorf("%s is already there and the plan does not skip it", path)
		}
	}
	if !planHas(plan, "templates/service.yaml", Create) {
		t.Error("the plan does not fill in the template that was missing")
	}
	// The keys were named by a merge whose output is no longer being written.
	if len(plan.ValuesAdded) > 0 {
		t.Errorf("the plan claims to write values keys into a file it skips: %v", plan.ValuesAdded)
	}

	if err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(mine) {
		t.Errorf("values.yaml was rewritten:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "templates", "service.yaml")); err != nil {
		t.Errorf("the missing template was not written: %v", err)
	}
}

// planHas reports whether the plan does exactly this to exactly this file.
func planHas(p *Plan, path string, action Action) bool {
	for _, f := range p.Files {
		if f.Path == path {
			return f.Action == action
		}
	}
	return false
}

// "@name" stands for a group's members. This is the whole point of the
// grouping: somebody wanting a monitoring setup asks for the setup, not for
// three Kubernetes kinds they have to know the names of first.
func TestResolveExpandsAGroup(t *testing.T) {
	got, err := resolve([]string{"@observability"})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range got {
		names = append(names, r.Name)
	}
	want := neutral(catalog.ResourcesInGroup(catalog.ObservabilityGroup))
	if len(names) != len(want) {
		t.Fatalf("@observability resolved to %v, want %d resources", names, len(want))
	}
	for _, r := range want {
		if !containsName(names, r.Name) {
			t.Errorf("%s is in the group and not in the expansion: %v", r.Name, names)
		}
	}
}

// A group beside one of its own members resolves once. Without the dedup the
// plan would carry the same template twice and the values merge would be
// asked for the same keys twice.
func TestResolveDedupesAcrossAGroup(t *testing.T) {
	got, err := resolve([]string{"@exposure", "service", "@exposure"})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, r := range got {
		seen[r.Name]++
	}
	for name, n := range seen {
		if n != 1 {
			t.Errorf("%s resolved %d times", name, n)
		}
	}
	want := neutral(catalog.ResourcesInGroup(catalog.ExposureGroup))
	if len(got) != len(want) {
		t.Errorf("resolved %d resources, want the group's %d platform-neutral ones", len(got), len(want))
	}
}

// An unknown group is an error naming the groups, not an unknown-resource
// error naming 32 resources: the "@" says which question was being asked.
func TestResolveRejectsAnUnknownGroup(t *testing.T) {
	_, err := resolve([]string{"@monitoring"})
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown group") || !strings.Contains(err.Error(), "@observability") {
		t.Errorf("the error does not say what the groups are: %v", err)
	}
}

// containsName is slices.Contains without pulling the import in for one use.
func containsName(haystack []string, want string) bool {
	for _, s := range haystack {
		if s == want {
			return true
		}
	}
	return false
}

// A group never drags in a resource that only exists on one platform: "hck
// add @secrets" on an EKS chart must not put a GKE CRD in it.
func TestResolveLeavesPlatformResourcesOutOfAGroup(t *testing.T) {
	got, err := resolve([]string{"@secrets"})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range got {
		if r.Platform != "" {
			t.Errorf("@secrets expanded to %q, which is %s only", r.Name, r.Platform)
		}
	}
	// It is left out, not made unreachable: named explicitly it still goes in.
	named, err := resolve([]string{"managedcertificate"})
	if err != nil {
		t.Fatalf("naming it explicitly failed: %v", err)
	}
	if len(named) != 1 || named[0].Name != "managedcertificate" {
		t.Errorf("naming it explicitly resolved to %v", named)
	}
}

// And what a group left out is reported, because silently expanding to less
// than the listing shows reads as the group being smaller than it is.
func TestGroupSkipNote(t *testing.T) {
	note := groupSkipNote([]string{"@secrets"})
	if !strings.Contains(note, "managedcertificate") || !strings.Contains(note, "gcp") {
		t.Errorf("the note does not name what was left out: %q", note)
	}
	// Named explicitly, there is nothing to report.
	if note := groupSkipNote([]string{"@secrets", "managedcertificate"}); note != "" {
		t.Errorf("named explicitly and still reported as skipped: %q", note)
	}
	// A group with no platform resources says nothing at all.
	if note := groupSkipNote([]string{"@access"}); note != "" {
		t.Errorf("@access has no platform resources but reported: %q", note)
	}
	// And a plain resource name is not a group.
	if note := groupSkipNote([]string{"secret", "pdb"}); note != "" {
		t.Errorf("plain names reported a group skip: %q", note)
	}
}

// neutral drops the platform-specific members, which group expansion never
// reaches.
func neutral(rs []catalog.Resource) []catalog.Resource {
	var out []catalog.Resource
	for _, r := range rs {
		if r.Platform == "" {
			out = append(out, r)
		}
	}
	return out
}
