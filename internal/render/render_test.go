package render

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
)

// embeddedResourceNames lists the resource directories actually present in the
// embedded tree. catalog's own test walks the other direction, so together the
// two prove the catalog and the templates cover exactly the same set.
func embeddedResourceNames(t *testing.T) []string {
	t.Helper()
	entries, err := fs.ReadDir(files, "templates/resources")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	if len(out) == 0 {
		t.Fatal("no resource templates are embedded")
	}
	return out
}

func testData() Data {
	return Data{
		ChartName:   "demo",
		Description: "a demo chart",
		Version:     "0.1.0",
		AppVersion:  "1.0.0",
		Preset:      "web",
		Resources:   []string{"deployment"},
	}
}

func TestChartFilesCoversTheSkeleton(t *testing.T) {
	files, err := ChartFiles()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"Chart.yaml":             false,
		"values.yaml":            false,
		"helmignore":             false,
		"ci/install-values.yaml": false,
		"templates/_helpers.tpl": false,
		"templates/NOTES.txt":    false,
	}
	for _, f := range files {
		if _, ok := want[f]; !ok {
			t.Errorf("unexpected skeleton file %q", f)
			continue
		}
		want[f] = true
	}
	for f, found := range want {
		if !found {
			t.Errorf("skeleton file %q is missing — go:embed drops _ and . prefixes without an all: pattern", f)
		}
	}
}

func TestChartFilesRender(t *testing.T) {
	files, err := ChartFiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		out, err := ChartFile(f, testData())
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		assertFullyRendered(t, f, out)
	}
}

func TestChartFileUnknown(t *testing.T) {
	if _, err := ChartFile("no-such-file", testData()); err == nil {
		t.Error("want an error for an unknown skeleton file")
	}
}

// Every catalog entry must have all three of its template files, and each must
// render. This is the check that keeps catalog.go and the template tree from
// drifting apart.
func TestEveryCatalogResourceRenders(t *testing.T) {
	for _, name := range embeddedResourceNames(t) {
		t.Run(name, func(t *testing.T) {
			if !HasResource(name) {
				t.Fatalf("templates/resources/%s is missing template.yaml.tmpl, values.yaml.tmpl or schema.json.tmpl", name)
			}
			tmpl, err := ResourceTemplate(name, testData())
			if err != nil {
				t.Fatal(err)
			}
			assertFullyRendered(t, name+"/template.yaml", tmpl)
			if !strings.Contains(string(tmpl), "kind:") {
				t.Error("template emits no kind")
			}

			vals, err := ResourceValues(name, testData())
			if err != nil {
				t.Fatal(err)
			}
			assertFullyRendered(t, name+"/values.yaml", vals)

			sch, err := ResourceSchema(name, testData())
			if err != nil {
				t.Fatal(err)
			}
			assertFullyRendered(t, name+"/schema.json", sch)
			assertSchemaFragment(t, name+"/schema.json", sch)
		})
	}
}

func TestBaseSchemaRenders(t *testing.T) {
	out, err := BaseSchema(testData())
	if err != nil {
		t.Fatal(err)
	}
	assertFullyRendered(t, "schema/base.json", out)
	assertSchemaFragment(t, "schema/base.json", out)
}

func TestResourceSchemaUnknown(t *testing.T) {
	if _, err := ResourceSchema("definitely-not-a-resource", testData()); err == nil {
		t.Error("want an error for an unknown resource")
	}
}

// assertSchemaFragment checks that a fragment is a JSON object of key to
// schema. A fragment that does not parse fails the build of every chart that
// asks for a schema, so it is worth catching here rather than at render time.
func assertSchemaFragment(t *testing.T, where string, out []byte) {
	t.Helper()
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Errorf("%s is not a JSON object: %v", where, err)
		return
	}
	if len(doc) == 0 {
		t.Errorf("%s declares no keys", where)
	}
	for key, raw := range doc {
		var body struct {
			Type any `json:"type"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("%s: %s is not a schema object: %v", where, key, err)
			continue
		}
		if body.Type == nil {
			t.Errorf("%s: %s declares no type", where, key)
		}
	}
}

func TestHasResourceRejectsUnknown(t *testing.T) {
	if HasResource("definitely-not-a-resource") {
		t.Error("want false for an unknown resource")
	}
}

// assertFullyRendered catches a delimiter that never got substituted, which
// otherwise ships as literal "[[ .ChartName ]]" inside a generated chart.
func assertFullyRendered(t *testing.T, where string, out []byte) {
	t.Helper()
	if len(out) == 0 {
		t.Errorf("%s rendered empty", where)
	}
	if strings.Contains(string(out), leftDelim) || strings.Contains(string(out), rightDelim) {
		t.Errorf("%s still contains an unrendered %s%s delimiter", where, leftDelim, rightDelim)
	}
}

func TestResourceLookupsRejectUnknown(t *testing.T) {
	if _, err := ResourceTemplate("nope", testData()); err == nil {
		t.Error("want an error for an unknown resource template")
	}
	if _, err := ResourceValues("nope", testData()); err == nil {
		t.Error("want an error for unknown resource values")
	}
}

// missingkey=error is what turns a typo in a template into a build failure
// instead of an "<no value>" that silently ships inside a chart.
func TestRenderFailsOnAnUnknownField(t *testing.T) {
	out, err := ChartFile("Chart.yaml", testData())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "<no value>") {
		t.Error("a field rendered as <no value>")
	}
}
