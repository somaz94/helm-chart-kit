package catalog

import (
	"slices"
	"testing"

	"github.com/somaz94/helm-chart-kit/internal/render"
	"github.com/somaz94/helm-chart-kit/internal/values"
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

// testData is the minimum context the values fragments need to render.
func testData() render.Data {
	return render.Data{
		ChartName:   "demo",
		Description: "a demo chart",
		Version:     "0.1.0",
		AppVersion:  "1.0.0",
		Preset:      "web",
	}
}

// TestValuesKeysMatchTheTemplates compares what the catalog says a resource
// contributes against what its values fragment actually declares.
//
// Nothing at runtime reads ValuesKeys — the merge splits the fragment itself —
// so the field had drifted: four workloads under-declared their keys and the
// CronJob claimed an "affinity" its template never had. A field nothing checks
// is a field that stops being true.
func TestValuesKeysMatchTheTemplates(t *testing.T) {
	for _, r := range Resources() {
		t.Run(r.Name, func(t *testing.T) {
			raw, err := render.ResourceValues(r.Name, testData())
			if err != nil {
				t.Fatal(err)
			}
			declared, err := values.TopLevelKeys(raw)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(r.ValuesKeys, declared) {
				t.Errorf("catalog ValuesKeys and values.yaml.tmpl disagree\n  catalog: %v\n  template: %v", r.ValuesKeys, declared)
			}
		})
	}
}
