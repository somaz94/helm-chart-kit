package schema

import (
	"encoding/json"
	"strings"
	"testing"
)

func build(t *testing.T, opts Options, frags ...Fragment) (map[string]any, []byte, Result) {
	t.Helper()
	out, res, err := Build(frags, opts)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	return doc, out, res
}

func TestBuildEnvelope(t *testing.T) {
	doc, out, _ := build(t, Options{Title: "demo values"},
		Fragment{Resource: "chart", Body: `{"nameOverride": {"type": "string"}}`})

	if got := doc["$schema"]; got != draft {
		t.Errorf("$schema = %v, want %v", got, draft)
	}
	if got := doc["title"]; got != "demo values" {
		t.Errorf("title = %v", got)
	}
	if got := doc["type"]; got != "object" {
		t.Errorf("type = %v", got)
	}
	if _, ok := doc["additionalProperties"]; ok {
		t.Error("additionalProperties is set without Strict, which would reject keys the chart accepts")
	}
	if !strings.HasSuffix(string(out), "}\n") {
		t.Error("output does not end with a newline")
	}
}

func TestBuildStrictClosesOnlyTheTopLevel(t *testing.T) {
	doc, _, _ := build(t, Options{Strict: true},
		Fragment{Resource: "svc", Body: `{"service": {"type": "object", "properties": {"port": {"type": "integer"}}}}`})

	if got, ok := doc["additionalProperties"].(bool); !ok || got {
		t.Errorf("additionalProperties = %v, want false", doc["additionalProperties"])
	}
	svc := doc["properties"].(map[string]any)["service"].(map[string]any)
	if _, ok := svc["additionalProperties"]; ok {
		t.Error("Strict closed a nested object; it must only close the top level")
	}
}

// The first fragment to claim a key keeps it, matching how the values merge
// resolves a key two resources both contribute.
func TestBuildFirstFragmentWinsAKey(t *testing.T) {
	doc, _, res := build(t, Options{},
		Fragment{Resource: "statefulset", Body: `{"persistence": {"type": "object", "description": "first"}}`},
		Fragment{Resource: "pvc", Body: `{"persistence": {"type": "object", "description": "second"}}`})

	got := doc["properties"].(map[string]any)["persistence"].(map[string]any)["description"]
	if got != "first" {
		t.Errorf("description = %v, want the first fragment's", got)
	}
	if len(res.Added) != 1 || res.Added[0] != "persistence" {
		t.Errorf("Added = %v", res.Added)
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != "persistence" {
		t.Errorf("Skipped = %v", res.Skipped)
	}
}

// Property order is the order the fragments contributed, not sorted: the
// schema then reads in the same order as the values.yaml it describes.
func TestBuildKeepsContributionOrder(t *testing.T) {
	_, out, _ := build(t, Options{},
		Fragment{Resource: "a", Body: `{"zebra": {"type": "string"}, "apple": {"type": "string"}}`},
		Fragment{Resource: "b", Body: `{"mango": {"type": "string"}}`})

	zebra := strings.Index(string(out), `"zebra"`)
	apple := strings.Index(string(out), `"apple"`)
	mango := strings.Index(string(out), `"mango"`)
	if !(zebra < apple && apple < mango) {
		t.Errorf("properties were reordered: zebra=%d apple=%d mango=%d", zebra, apple, mango)
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	frags := []Fragment{
		{Resource: "a", Body: `{"one": {"type": "string"}, "two": {"type": "integer"}}`},
		{Resource: "b", Body: `{"three": {"type": "object", "properties": {"x": {"type": "boolean"}}}}`},
	}
	first, _, err := Build(frags, Options{Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		again, _, err := Build(frags, Options{Title: "t"})
		if err != nil {
			t.Fatal(err)
		}
		if string(first) != string(again) {
			t.Fatal("Build is not byte-stable, so --check would report spurious drift")
		}
	}
}

func TestBuildEmpty(t *testing.T) {
	out, res, err := Build(nil, Options{Title: "empty"})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("empty build is not valid JSON: %v\n%s", err, out)
	}
	if len(res.Added) != 0 {
		t.Errorf("Added = %v", res.Added)
	}
}

func TestBuildRejectsBadFragments(t *testing.T) {
	for name, body := range map[string]string{
		"not json":      `{"broken`,
		"not an object": `["service"]`,
		"bad value":     `{"service": }`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Build([]Fragment{{Resource: "x", Body: body}}, Options{}); err == nil {
				t.Error("want an error")
			} else if !strings.Contains(err.Error(), "x") {
				t.Errorf("error does not name the fragment: %v", err)
			}
		})
	}
}

func TestQuoteEscapes(t *testing.T) {
	doc, _, _ := build(t, Options{Title: `a "quoted" — title`},
		Fragment{Resource: "a", Body: `{"k": {"type": "string"}}`})
	if got := doc["title"]; got != `a "quoted" — title` {
		t.Errorf("title = %q", got)
	}
}
