package docs

import (
	"strings"
	"testing"
)

const sample = `# =============================================================================
# Section banner that documents nothing in particular
# =============================================================================

# -- Number of replicas
replicaCount: 1

# -- A description that runs
# onto a second line
image:
  repository: nginx
  # -- Pull policy
  pullPolicy: IfNotPresent

# -- Empty map
podAnnotations: {}

# -- Empty list
tolerations: []

# -- Explicit null
memoryTarget: null

# -- Empty string
priorityClassName: ""

flags:
  - a
  - b
`

func rowsByKey(t *testing.T, values string, opts Options) map[string]Row {
	t.Helper()
	rows, err := Rows([]byte(values), opts)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Row{}
	for _, r := range rows {
		out[r.Key] = r
	}
	return out
}

func TestRowsWalkLeaves(t *testing.T) {
	got := rowsByKey(t, sample, Options{})
	for key, want := range map[string]Row{
		"replicaCount":      {Type: "int", Default: "`1`", Description: "Number of replicas"},
		"image.repository":  {Type: "string", Default: "`nginx`"},
		"image.pullPolicy":  {Type: "string", Default: "`IfNotPresent`", Description: "Pull policy"},
		"podAnnotations":    {Type: "object", Default: "`{}`", Description: "Empty map"},
		"tolerations":       {Type: "list", Default: "`[]`", Description: "Empty list"},
		"memoryTarget":      {Type: "null", Default: "`null`", Description: "Explicit null"},
		"priorityClassName": {Type: "string", Default: `` + "`\"\"`", Description: "Empty string"},
	} {
		r, ok := got[key]
		if !ok {
			t.Errorf("%s is missing", key)
			continue
		}
		if r.Type != want.Type {
			t.Errorf("%s type = %q, want %q", key, r.Type, want.Type)
		}
		if r.Default != want.Default {
			t.Errorf("%s default = %q, want %q", key, r.Default, want.Default)
		}
		if r.Description != want.Description {
			t.Errorf("%s description = %q, want %q", key, r.Description, want.Description)
		}
	}
	// A mapping with children is not itself a row.
	if _, ok := got["image"]; ok {
		t.Error("image was emitted as a leaf even though it has children")
	}
}

// A banner comment sits above the first key but documents the section, not
// that key. Only a "-- " line starts a description.
func TestRowsIgnoreBannerComments(t *testing.T) {
	got := rowsByKey(t, sample, Options{})
	if d := got["replicaCount"].Description; d != "Number of replicas" {
		t.Errorf("banner leaked into the description: %q", d)
	}
}

func TestRowsJoinContinuationLines(t *testing.T) {
	got := rowsByKey(t, sample, Options{})
	if d := got["image.repository"].Description; d != "" {
		t.Errorf("the multi-line comment attached to the wrong key: %q", d)
	}
}

func TestRowsRenderNonEmptyCollectionsOnOneLine(t *testing.T) {
	got := rowsByKey(t, sample, Options{})
	d := got["flags"].Default
	if strings.Contains(d, "\n") {
		t.Errorf("default spans lines, which breaks the table row: %q", d)
	}
	if !strings.Contains(d, "a") || !strings.Contains(d, "b") {
		t.Errorf("default lost its contents: %q", d)
	}
}

func TestRowsUseTheSchemaForTypesAndEnums(t *testing.T) {
	schema := []byte(`{
	  "properties": {
	    "replicaCount": {"type": "integer"},
	    "image": {"type": "object", "properties": {
	      "pullPolicy": {"type": "string", "enum": ["Always", "IfNotPresent", "Never"]}
	    }},
	    "priorityClassName": {"type": ["string", "integer", "null"]}
	  }
	}`)
	got := rowsByKey(t, sample, Options{Schema: schema})

	// JSON Schema's names are mapped onto the ones the rest of the column uses.
	if got["replicaCount"].Type != "int" {
		t.Errorf("type = %q, want int", got["replicaCount"].Type)
	}
	if got["priorityClassName"].Type != "string/int/null" {
		t.Errorf("union type = %q", got["priorityClassName"].Type)
	}
	d := got["image.pullPolicy"].Description
	if !strings.Contains(d, "One of: `Always`, `IfNotPresent`, `Never`.") {
		t.Errorf("enum missing: %q", d)
	}
	// The comment did not end in a full stop, so one is added before the
	// appended clause rather than the two running together.
	if !strings.Contains(d, "Pull policy. One of:") {
		t.Errorf("clause ran into the description: %q", d)
	}
}

func TestTableEscapesPipes(t *testing.T) {
	out, err := Table([]byte("# -- A | B\nkey: \"x|y\"\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "| `key`") {
			continue
		}
		if strings.Count(line, "|")-strings.Count(line, `\|`) != 5 {
			t.Errorf("row has unescaped pipes, so the columns shift: %s", line)
		}
	}
}

func TestRowsRejectABadDocument(t *testing.T) {
	if _, err := Rows([]byte("- a\n- b\n"), Options{}); err == nil {
		t.Error("want an error for a top-level list")
	}
	if _, err := Rows([]byte("key: [unclosed\n"), Options{}); err == nil {
		t.Error("want an error for malformed YAML")
	}
	if _, err := Rows([]byte("key: 1\n"), Options{Schema: []byte("{not json")}); err == nil {
		t.Error("want an error for a malformed schema")
	}
}

func TestRowsEmptyDocument(t *testing.T) {
	rows, err := Rows(nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows", len(rows))
	}
}

func TestReplaceCreatesTheBlockWhenAbsent(t *testing.T) {
	got, err := Replace([]byte("# chart\n\nIntro.\n"), "TABLE\n")
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	for _, want := range []string{"# chart", "Intro.", "## Values", StartMarker, "TABLE", EndMarker} {
		if !strings.Contains(s, want) {
			t.Errorf("output is missing %q:\n%s", want, s)
		}
	}
}

func TestReplaceKeepsEverythingOutsideTheMarkers(t *testing.T) {
	before := "# chart\n\nIntro.\n\n" + StartMarker + "\n\nOLD\n\n" + EndMarker + "\n\n## Tail\n\nKeep me.\n"
	got, err := Replace([]byte(before), "NEW\n")
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if strings.Contains(s, "OLD") {
		t.Error("the old table survived")
	}
	for _, want := range []string{"Intro.", "NEW", "## Tail", "Keep me."} {
		if !strings.Contains(s, want) {
			t.Errorf("output is missing %q:\n%s", want, s)
		}
	}
}

func TestReplaceRejectsAHalfPair(t *testing.T) {
	for name, readme := range map[string]string{
		"start only": "x\n" + StartMarker + "\n",
		"end only":   "x\n" + EndMarker + "\n",
		"reversed":   EndMarker + "\nx\n" + StartMarker + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Replace([]byte(readme), "T\n"); err == nil {
				t.Error("want an error")
			}
		})
	}
}

// Regenerating twice must not keep growing the file.
func TestReplaceIsIdempotent(t *testing.T) {
	once, err := Replace([]byte(Skeleton("demo", "a demo")), "TABLE\n")
	if err != nil {
		t.Fatal(err)
	}
	twice, err := Replace(once, "TABLE\n")
	if err != nil {
		t.Fatal(err)
	}
	if string(once) != string(twice) {
		t.Errorf("second pass differs:\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
	}
}

func TestSkeleton(t *testing.T) {
	if got := Skeleton("demo", ""); strings.Contains(got, "\n\n\n") {
		t.Errorf("blank description left a gap: %q", got)
	}
	if got := Skeleton("demo", "a demo"); !strings.Contains(got, "a demo") {
		t.Errorf("description dropped: %q", got)
	}
}

func TestYamlTypeCoversEveryScalar(t *testing.T) {
	got := rowsByKey(t, `
s: text
i: 1
f: 1.5
b: true
n: null
q: "7"
`, Options{})
	for key, want := range map[string]string{
		"s": "string", "i": "int", "f": "float", "b": "bool", "n": "null", "q": "string",
	} {
		if got[key].Type != want {
			t.Errorf("%s type = %q, want %q", key, got[key].Type, want)
		}
	}
}

// A blank line closes a description: what follows is commentary about the
// section, not more of this key's sentence.
func TestFromCommentStopsAtABlankLine(t *testing.T) {
	got := rowsByKey(t, `# -- The description
# continues here
#
# But this paragraph is about something else
key: 1
`, Options{})
	if d := got["key"].Description; d != "The description continues here" {
		t.Errorf("description = %q", d)
	}
}

// A trailing comment documents the key on the same line when there is no head
// comment to take precedence.
func TestDescriptionFallsBackToTheLineComment(t *testing.T) {
	got := rowsByKey(t, "key: 1 # -- Inline description\n", Options{})
	if d := got["key"].Description; d != "Inline description" {
		t.Errorf("description = %q", d)
	}
}

func TestFromCommentIgnoresCommentsWithNoMarker(t *testing.T) {
	got := rowsByKey(t, "# just a note\nkey: 1\n", Options{})
	if d := got["key"].Description; d != "" {
		t.Errorf("description = %q, want empty", d)
	}
}

func TestSentence(t *testing.T) {
	for in, want := range map[string]string{
		"":               "",
		"No punctuation": "No punctuation.",
		"Already done.":  "Already done.",
		"A question?":    "A question?",
		"Emphatic!":      "Emphatic!",
		"Introducing:":   "Introducing:",
		"  padded  ":     "padded.",
	} {
		if got := sentence(in); got != want {
			t.Errorf("sentence(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeType(t *testing.T) {
	for in, want := range map[string]string{
		"boolean": "bool",
		"integer": "int",
		"number":  "float",
		"array":   "list",
		"string":  "string",
		"object":  "object",
		"null":    "null",
	} {
		if got := normalizeType(in); got != want {
			t.Errorf("normalizeType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTypeNamesHandlesAMissingOrOddType(t *testing.T) {
	for name, raw := range map[string]string{
		"absent": ``,
		"number": `3`,
		"object": `{"nested": true}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := typeNames([]byte(raw)); got != "" {
				t.Errorf("got %q, want empty", got)
			}
		})
	}
}

// A schema node describing a key values.yaml does not have is simply unused,
// not an error: the two are allowed to differ, and the table follows the file.
func TestSchemaKeysAbsentFromValuesAreIgnored(t *testing.T) {
	rows, err := Rows([]byte("key: 1\n"), Options{
		Schema: []byte(`{"properties": {"key": {"type": "integer"}, "ghost": {"type": "string"}}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Key != "key" {
		t.Fatalf("got %+v", rows)
	}
}
