package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
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
