package values

import (
	"strings"
	"testing"
)

func TestTopLevelKeys(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
		err  bool
	}{
		{name: "empty", src: "", want: nil},
		{name: "comments only", src: "# nothing here\n", want: nil},
		{name: "ordered", src: "b: 1\na: 2\nc: 3\n", want: []string{"b", "a", "c"}},
		{name: "nested keys are not top level", src: "a:\n  b: 1\n  c: 2\n", want: []string{"a"}},
		{name: "list at root", src: "- a\n- b\n", err: true},
		{name: "malformed", src: "a: [\n", err: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := TopLevelKeys([]byte(tc.src))
			if tc.err {
				if err == nil {
					t.Fatal("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// The whole reason this package exists: what is already in values.yaml must
// come back out unchanged, byte for byte.
func TestMergePreservesExistingBytes(t *testing.T) {
	src := `# =============================================================================
# Banner that a YAML round-trip would eat
# =============================================================================

# -- how many
replicaCount: 1

image:
  repository: nginx   # trailing comment
  tag: "1.0"


# stray blank lines above are deliberate
service:
  port: 80
`
	out, res, err := Merge([]byte(src), []Fragment{{
		Resource: "hpa",
		Body:     "# -- scaling\nautoscaling:\n  enabled: false\n",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(out), src) {
		t.Fatalf("existing content was rewritten.\n--- got ---\n%s", out)
	}
	if len(res.Added) != 1 || res.Added[0] != "autoscaling" {
		t.Fatalf("added = %v, want [autoscaling]", res.Added)
	}
	if !strings.Contains(string(out), "# -- scaling") {
		t.Error("the comment documenting the key did not travel with it")
	}
}

func TestMergeSkipsExistingKeys(t *testing.T) {
	src := "image:\n  repository: nginx\n  tag: \"1.0\"\n"
	out, res, err := Merge([]byte(src), []Fragment{{
		Resource: "deployment",
		Body:     "image:\n  repository: OVERWRITTEN\n  tag: \"9.9\"\nreplicaCount: 3\n",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "OVERWRITTEN") {
		t.Fatal("an existing key was overwritten")
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != "image" {
		t.Fatalf("skipped = %v, want [image]", res.Skipped)
	}
	if len(res.Added) != 1 || res.Added[0] != "replicaCount" {
		t.Fatalf("added = %v, want [replicaCount]", res.Added)
	}
}

// Two resources both contributing "image" must not append it twice, or the
// resulting values.yaml has a duplicate key and stops parsing.
func TestMergeDeduplicatesAcrossFragments(t *testing.T) {
	out, res, err := Merge(nil, []Fragment{
		{Resource: "deployment", Body: "image:\n  tag: \"a\"\n"},
		{Resource: "job", Body: "image:\n  tag: \"b\"\njob:\n  enabled: false\n"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(out), "\nimage:"); n > 1 {
		t.Fatalf("image appears %d times:\n%s", n, out)
	}
	if got := strings.Join(res.Added, ","); got != "image,job" {
		t.Fatalf("added = %q, want \"image,job\"", got)
	}
	if _, err := TopLevelKeys(out); err != nil {
		t.Fatalf("merged document does not parse: %v", err)
	}
}

func TestMergeIntoEmptyDocument(t *testing.T) {
	out, res, err := Merge(nil, []Fragment{{Resource: "pdb", Body: "podDisruptionBudget:\n  enabled: false\n"}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed() {
		t.Error("Changed() = false after adding a key")
	}
	if got := string(out); got != "podDisruptionBudget:\n  enabled: false\n" {
		t.Fatalf("got %q", got)
	}
}

func TestMergeNoFragmentsIsIdentity(t *testing.T) {
	src := []byte("a: 1\n")
	out, res, err := Merge(src, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(src) || res.Changed() {
		t.Fatalf("identity merge changed something: %q", out)
	}
}

func TestMergeRejectsBadInput(t *testing.T) {
	if _, _, err := Merge([]byte("- a\n"), nil); err == nil {
		t.Error("want error for a list-rooted values document")
	}
	if _, _, err := Merge(nil, []Fragment{{Resource: "x", Body: "- a\n"}}); err == nil {
		t.Error("want error for a list-rooted fragment")
	}
}

// A key's leading comment block must not be swallowed by the key above it.
func TestSplitBlocksAssignsCommentsToTheKeyBelow(t *testing.T) {
	blocks, err := splitBlocks("a: 1\n\n# doc for b\n# more doc\nb: 2\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
	if strings.Contains(blocks[0].text, "doc for b") {
		t.Error("comment landed on the preceding key")
	}
	if !strings.Contains(blocks[1].text, "# doc for b") {
		t.Error("comment did not travel with its key")
	}
}

func TestKindName(t *testing.T) {
	// The names appear in user-facing errors, so a wrong one is a wrong error.
	for _, tc := range []struct{ src, want string }{
		{"- a\n", "a list"},
		{"just a string\n", "a scalar"},
	} {
		_, err := TopLevelKeys([]byte(tc.src))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("TopLevelKeys(%q) error = %v, want it to mention %q", tc.src, err, tc.want)
		}
	}
}
