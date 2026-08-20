package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/somaz94/helm-chart-kit/internal/render"
	"github.com/somaz94/helm-chart-kit/internal/values"
	"gopkg.in/yaml.v3"
)

func TestLookup(t *testing.T) {
	if _, ok := LookupResource("deployment"); !ok {
		t.Error("deployment not found")
	}
	if _, ok := LookupResource("nope"); ok {
		t.Error("unknown resource reported as found")
	}
	if _, ok := LookupPreset("web"); !ok {
		t.Error("web preset not found")
	}
	if _, ok := LookupPreset("nope"); ok {
		t.Error("unknown preset reported as found")
	}
}

func TestResourcesAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	files := map[string]string{}
	for _, r := range Resources() {
		if seen[r.Name] {
			t.Errorf("%s is listed twice", r.Name)
		}
		seen[r.Name] = true
		if r.File == "" || r.Summary == "" || r.APIVersion == "" {
			t.Errorf("%s has an empty File, Summary or APIVersion", r.Name)
		}
		if prev, ok := files[r.File]; ok {
			t.Errorf("%s and %s both emit %s, so one would overwrite the other", prev, r.Name, r.File)
		}
		files[r.File] = r.Name
		if len(r.ValuesKeys) == 0 {
			t.Errorf("%s declares no values keys", r.Name)
		}
		for _, req := range r.Requires {
			if _, ok := LookupResource(req); !ok {
				t.Errorf("%s requires unknown resource %q", r.Name, req)
			}
		}
	}
}

func TestPresetsReferenceKnownResources(t *testing.T) {
	for _, p := range Presets() {
		if len(p.Resources) == 0 {
			t.Errorf("preset %s is empty", p.Name)
		}
		seen := map[string]bool{}
		workloads := 0
		for _, name := range p.Resources {
			r, ok := LookupResource(name)
			if !ok {
				t.Errorf("preset %s references unknown resource %q", p.Name, name)
				continue
			}
			if seen[name] {
				t.Errorf("preset %s lists %s twice", p.Name, name)
			}
			seen[name] = true
			if r.Workload {
				workloads++
			}
		}
		// Two workloads in one preset would contend for the same values keys
		// with incompatible shapes — the exact thing "hck add" refuses.
		if workloads != 1 {
			t.Errorf("preset %s has %d workloads, want exactly 1", p.Name, workloads)
		}
	}
}

// A resource's requirements have to be satisfiable inside a preset that uses
// it, otherwise every chart from that preset starts out broken.
func TestPresetsSatisfyTheirRequirements(t *testing.T) {
	for _, p := range Presets() {
		in := map[string]bool{}
		for _, name := range p.Resources {
			in[name] = true
		}
		for _, name := range p.Resources {
			r, ok := LookupResource(name)
			if !ok {
				continue
			}
			for _, req := range r.Requires {
				if !in[req] {
					t.Errorf("preset %s includes %s but not its requirement %s", p.Name, name, req)
				}
			}
		}
	}
}

func TestNameListsAreSorted(t *testing.T) {
	for _, list := range [][]string{ResourceNames(), PresetNames()} {
		for i := 1; i < len(list); i++ {
			if list[i-1] > list[i] {
				t.Errorf("not sorted: %q before %q", list[i-1], list[i])
			}
		}
	}
}

// TestEveryResourceHasTemplates walks catalog -> templates. The render package
// walks templates -> catalog. A resource added to only one of the two fails
// one of the pair.
func TestEveryResourceHasTemplates(t *testing.T) {
	for _, r := range Resources() {
		if !render.HasResource(r.Name) {
			t.Errorf("catalog lists %q but internal/render/templates/resources/%s has no templates", r.Name, r.Name)
		}
	}
}

// TestEveryWorkloadHasAPreset closes the gap that made "hck new --preset web
// --with daemonset" look necessary in the first place.
//
// A chart carries one primary workload, so --with cannot add a second on top
// of whatever the preset already brings. A workload no preset offers is
// therefore not reachable at all — which is what happened to the DaemonSet:
// the only way to scaffold one was the --with route, and that route was a bug.
func TestEveryWorkloadHasAPreset(t *testing.T) {
	offered := map[string]bool{}
	for _, p := range Presets() {
		for _, name := range p.Resources {
			if r, ok := LookupResource(name); ok && r.Workload {
				offered[name] = true
			}
		}
	}
	for _, r := range Resources() {
		if r.Workload && !offered[r.Name] {
			t.Errorf("%s is a primary workload but no preset includes it, so no chart can be scaffolded with it", r.Name)
		}
	}
}

// testData is the minimum context the fragments need to render.
func testData() render.Data {
	return render.Data{
		ChartName:   "demo",
		Description: "a demo chart",
		Version:     "0.1.0",
		AppVersion:  "1.0.0",
		Preset:      "web",
	}
}

// TestValuesKeysMatchTheTemplates closes the triangle between the three places
// a resource's values keys are written down: this catalog, the values.yaml
// fragment that actually contributes them, and the schema fragment that
// describes them. Declaring a key in one or two of the three is the failure
// this catches — most consequentially a key present in values.yaml but absent
// from the schema, which Helm rejects the chart for on every render once the
// chart has a values.schema.json.
func TestValuesKeysMatchTheTemplates(t *testing.T) {
	for _, r := range Resources() {
		t.Run(r.Name, func(t *testing.T) {
			raw, err := render.ResourceValues(r.Name, testData())
			if err != nil {
				t.Fatal(err)
			}
			fromValues, err := values.TopLevelKeys(raw)
			if err != nil {
				t.Fatal(err)
			}

			rawSchema, err := render.ResourceSchema(r.Name, testData())
			if err != nil {
				t.Fatal(err)
			}
			fromSchema, err := schemaKeys(rawSchema)
			if err != nil {
				t.Fatal(err)
			}

			if !slices.Equal(r.ValuesKeys, fromValues) {
				t.Errorf("catalog ValuesKeys and values.yaml.tmpl disagree\n  catalog: %v\n  template: %v", r.ValuesKeys, fromValues)
			}
			if !slices.Equal(fromValues, fromSchema) {
				t.Errorf("values.yaml.tmpl and schema.json.tmpl disagree\n  values: %v\n  schema: %v", fromValues, fromSchema)
			}
		})
	}
}

// schemaKeys returns a schema fragment's top-level keys in document order.
func schemaKeys(src []byte) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(src))
	if _, err := dec.Token(); err != nil { // opening brace
		return nil, err
	}
	var out []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("non-string key %v", tok)
		}
		var discard json.RawMessage
		if err := dec.Decode(&discard); err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	return out, nil
}

// TestPlatformOverlaysMatchTheCatalog walks the overlay tree back to the
// platform list, the way TestEveryResourceHasTemplates walks the catalog to
// the templates. A fragment named values-<x>.yaml.tmpl for an <x> nobody
// declared renders for no one and would sit there unnoticed.
func TestPlatformOverlaysMatchTheCatalog(t *testing.T) {
	found, err := render.OverlaySuffixes()
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, o := range AllOverlays() {
		known[o.Name] = true
	}
	for _, name := range found {
		if !known[name] {
			t.Errorf("templates carry values-%s.yaml.tmpl but no overlay %q is declared", name, name)
		}
	}
	// And every declared overlay has to actually differ somewhere, or it is
	// a name that produces an empty file.
	for _, o := range AllOverlays() {
		if !slices.Contains(found, o.Name) {
			t.Errorf("%s %q is declared but no resource has a values-%s.yaml.tmpl", o.Axis, o.Name, o.Name)
		}
	}
}

// Both axes share one file-name space inside a chart, so a name used twice
// would have one overlay silently overwrite the other. Walking every overlay
// at once rather than one axis against the other is what keeps this honest
// when a third axis arrives.
func TestPlatformAndEnvironmentNamesDoNotCollide(t *testing.T) {
	seen := map[string]string{}
	for _, o := range AllOverlays() {
		who := string(o.Axis) + " " + o.Name
		if prev, ok := seen[o.ValuesFile()]; ok {
			t.Errorf("%s and %s both write %s", who, prev, o.ValuesFile())
		}
		seen[o.ValuesFile()] = who
	}
}

// An axis names itself one way in prose and another on the command line: the
// noun is "environment", the subcommand is "env". Messages that mix the two
// send the reader to a command that does not exist.
func TestAxisCommand(t *testing.T) {
	if got := PlatformAxis.Command(); got != "platform" {
		t.Errorf("PlatformAxis.Command() = %q, want %q", got, "platform")
	}
	if got := EnvironmentAxis.Command(); got != "env" {
		t.Errorf("EnvironmentAxis.Command() = %q, want %q", got, "env")
	}
}

// A lookup is scoped to one axis, so a real overlay named on the wrong one is
// reported as unknown rather than quietly written.
func TestLookupOverlayIsScopedToItsAxis(t *testing.T) {
	if _, ok := LookupOverlay(PlatformAxis, "prod"); ok {
		t.Error("prod found on the platform axis")
	}
	if _, ok := LookupOverlay(EnvironmentAxis, "aws"); ok {
		t.Error("aws found on the environment axis")
	}
	if len(AllOverlays()) != len(Overlays(PlatformAxis))+len(Overlays(EnvironmentAxis)) {
		t.Error("AllOverlays and the per-axis lists disagree")
	}
}

func TestEnvironmentMetadata(t *testing.T) {
	for _, e := range Overlays(EnvironmentAxis) {
		if e.Summary == "" {
			t.Errorf("%s has an empty Summary", e.Name)
		}
		if want := "values-" + e.Name + ".yaml"; e.ValuesFile() != want {
			t.Errorf("ValuesFile = %q, want %q", e.ValuesFile(), want)
		}
		if _, ok := LookupOverlay(EnvironmentAxis, e.Name); !ok {
			t.Errorf("%s is listed but not found", e.Name)
		}
	}
	if _, ok := LookupOverlay(EnvironmentAxis, "nope"); ok {
		t.Error("unknown environment reported as found")
	}
	if len(OverlayNames(EnvironmentAxis)) != len(Overlays(EnvironmentAxis)) {
		t.Error("OverlayNames and Overlays disagree on the environment axis")
	}
}

func TestPlatformMetadata(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range Overlays(PlatformAxis) {
		if p.Summary == "" || len(p.Needs) == 0 {
			t.Errorf("%s has an empty Summary or Needs", p.Name)
		}
		if seen[p.Name] {
			t.Errorf("%s is declared twice", p.Name)
		}
		seen[p.Name] = true
		if want := "values-" + p.Name + ".yaml"; p.ValuesFile() != want {
			t.Errorf("ValuesFile = %q, want %q", p.ValuesFile(), want)
		}
	}
	if _, ok := LookupOverlay(PlatformAxis, "nope"); ok {
		t.Error("unknown platform reported as found")
	}
	if len(OverlayNames(PlatformAxis)) != len(Overlays(PlatformAxis)) {
		t.Error("OverlayNames and Overlays disagree on the platform axis")
	}
}

// An overlay is only worth writing when it says something values.yaml does
// not. A fragment that repeats a default is noise in a file whose whole
// purpose is to show the difference.
func TestPlatformOverlaysDifferFromTheBase(t *testing.T) {
	for _, r := range Resources() {
		for _, p := range overlaySuffixes() {
			if !render.HasOverlayValues(r.Name, p) {
				continue
			}
			t.Run(r.Name+"/"+p, func(t *testing.T) {
				base, err := render.ResourceValues(r.Name, testData())
				if err != nil {
					t.Fatal(err)
				}
				over, ok, err := render.ResourceOverlayValues(r.Name, p, testData())
				if err != nil || !ok {
					t.Fatalf("overlay did not render: %v", err)
				}
				var baseDoc, overDoc map[string]any
				if err := yaml.Unmarshal(base, &baseDoc); err != nil {
					t.Fatal(err)
				}
				if err := yaml.Unmarshal(over, &overDoc); err != nil {
					t.Fatalf("overlay is not valid YAML: %v", err)
				}
				if len(overDoc) == 0 {
					t.Fatal("overlay declares nothing")
				}
				for k, v := range overDoc {
					if reflect.DeepEqual(baseDoc[k], v) {
						t.Errorf("%s repeats the base value, so it says nothing", k)
					}
				}
			})
		}
	}
}

// Every key an overlay sets has to be one the resource actually owns, or it
// lands in values.yaml describing nothing and helm ignores it.
func TestPlatformOverlayKeysBelongToTheResource(t *testing.T) {
	for _, r := range Resources() {
		for _, p := range overlaySuffixes() {
			if !render.HasOverlayValues(r.Name, p) {
				continue
			}
			t.Run(r.Name+"/"+p, func(t *testing.T) {
				over, _, err := render.ResourceOverlayValues(r.Name, p, testData())
				if err != nil {
					t.Fatal(err)
				}
				keys, err := values.TopLevelKeys(over)
				if err != nil {
					t.Fatal(err)
				}
				for _, k := range keys {
					if !slices.Contains(r.ValuesKeys, k) {
						t.Errorf("overlay sets %q, which %s does not contribute to values.yaml", k, r.Name)
					}
				}
			})
		}
	}
}

func TestLookupPlatformKnown(t *testing.T) {
	for _, name := range OverlayNames(PlatformAxis) {
		if _, ok := LookupOverlay(PlatformAxis, name); !ok {
			t.Errorf("%s is listed but not found", name)
		}
	}
}

// overlaySuffixes is every axis an overlay can be named for.
func overlaySuffixes() []string {
	out := append([]string{}, OverlayNames(PlatformAxis)...)
	return append(out, OverlayNames(EnvironmentAxis)...)
}

// A platform overlay describes wiring; an environment overlay decides what is
// on. Both end up as -f arguments, so a key both axes set is resolved by
// argument order rather than by intent — "aws says no NetworkPolicy, prod says
// yes" rendered differently depending on which came last.
func TestPlatformOverlaysDoNotToggle(t *testing.T) {
	for _, r := range Resources() {
		for _, p := range Overlays(PlatformAxis) {
			if !render.HasOverlayValues(r.Name, p.Name) {
				continue
			}
			t.Run(r.Name+"/"+p.Name, func(t *testing.T) {
				raw, _, err := render.ResourceOverlayValues(r.Name, p.Name, testData())
				if err != nil {
					t.Fatal(err)
				}
				var doc map[string]any
				if err := yaml.Unmarshal(raw, &doc); err != nil {
					t.Fatal(err)
				}
				for _, path := range enabledPaths(doc, nil) {
					t.Errorf("platform overlay sets %s; enabling belongs to the environment axis, and a prerequisite belongs in Needs", path)
				}
			})
		}
	}
}

// enabledPaths finds every "enabled" key at any depth.
func enabledPaths(m map[string]any, prefix []string) []string {
	var out []string
	for k, v := range m {
		here := append(append([]string{}, prefix...), k)
		if k == "enabled" {
			out = append(out, strings.Join(here, "."))
		}
		if child, ok := v.(map[string]any); ok {
			out = append(out, enabledPaths(child, here)...)
		}
	}
	return out
}

// Both READMEs claim a number of generatable resources, in a table whose whole
// point is the comparison. A number that quietly falls behind the catalog is
// the one kind of documentation error nobody reads carefully enough to catch,
// so it is checked rather than trusted.
func TestTheReadmeResourceCountIsTrue(t *testing.T) {
	want := len(Resources())
	for _, readme := range []string{"../../README.md", "../../README-ko.md"} {
		raw, err := os.ReadFile(readme)
		if err != nil {
			t.Fatal(err)
		}
		// The row reads "| 6, fixed | 32, composable |" in English and
		// "| 6개 고정 | 32개, 조합 가능 |" in Korean; the count is the first
		// number after the "6" that opens the helm create column.
		m := readmeCountRE.FindSubmatch(raw)
		if m == nil {
			t.Fatalf("%s no longer states a resource count in the comparison table", readme)
		}
		got, err := strconv.Atoi(string(m[1]))
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s says hck emits %d resources, the catalog has %d", readme, got, want)
		}
	}
}

var readmeCountRE = regexp.MustCompile(`\| 6(?:, fixed|개 고정) \| (\d+)`)
