// Package values merges resource-contributed values.yaml fragments into a
// chart's existing values.yaml.
//
// The merge is append-only and works on text, not on a re-encoded document.
// That is deliberate. Round-tripping YAML through a decoder and an encoder
// preserves keys and comments but not the blank lines and section banners that
// make a 400-line values.yaml readable, so a tool that "just adds a key" would
// silently reformat the whole file. Here the original bytes are never touched:
// absent keys are appended, present keys are reported and left alone.
package values

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Fragment is the values.yaml contribution of a single resource template.
type Fragment struct {
	// Resource is the catalog name the fragment came from, used in reports.
	Resource string
	// Body is raw YAML text — comments and all — defining one or more
	// top-level keys.
	Body string
}

// Result reports what a merge did, per top-level key.
type Result struct {
	// Added lists keys appended to the document.
	Added []string
	// Skipped lists keys that were already present and so left untouched.
	Skipped []string
}

// Changed reports whether the merge produced a different document.
func (r Result) Changed() bool { return len(r.Added) > 0 }

// block is one top-level key together with the comment lines above it.
type block struct {
	key  string
	text string
}

// TopLevelKeys returns the top-level mapping keys of a YAML document, in
// document order. An empty or comment-only document has no keys and is not an
// error — a freshly scaffolded values.yaml is exactly that.
func TopLevelKeys(src []byte) ([]string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, fmt.Errorf("parse values: %w", err)
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("values must be a mapping at the top level, found %s", kindName(root.Kind))
	}
	keys := make([]string, 0, len(root.Content)/2)
	for i := 0; i+1 < len(root.Content); i += 2 {
		keys = append(keys, root.Content[i].Value)
	}
	return keys, nil
}

// Merge appends every fragment block whose key is absent from src and returns
// the new document. src is copied verbatim; nothing in it is rewritten.
func Merge(src []byte, frags []Fragment) ([]byte, Result, error) {
	var res Result

	existing, err := TopLevelKeys(src)
	if err != nil {
		return nil, res, err
	}
	seen := make(map[string]bool, len(existing))
	for _, k := range existing {
		seen[k] = true
	}

	var appended []string
	for _, f := range frags {
		blocks, err := splitBlocks(f.Body)
		if err != nil {
			return nil, res, fmt.Errorf("fragment %q: %w", f.Resource, err)
		}
		for _, b := range blocks {
			if seen[b.key] {
				res.Skipped = append(res.Skipped, b.key)
				continue
			}
			// Mark before appending so two resources contributing the same
			// key — "image" belongs to every workload — add it only once.
			seen[b.key] = true
			res.Added = append(res.Added, b.key)
			appended = append(appended, b.text)
		}
	}

	if len(appended) == 0 {
		return src, res, nil
	}

	var out bytes.Buffer
	if len(bytes.TrimSpace(src)) > 0 {
		out.Write(bytes.TrimRight(src, "\n"))
		out.WriteString("\n")
	}
	for _, text := range appended {
		if out.Len() > 0 {
			out.WriteString("\n")
		}
		out.WriteString(strings.TrimRight(text, "\n"))
		out.WriteString("\n")
	}
	return out.Bytes(), res, nil
}

// splitBlocks cuts a fragment into one text block per top-level key. A key's
// block starts at the first of the contiguous comment and blank lines above
// it, so the documentation written for a key travels with it.
func splitBlocks(body string) ([]block, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		return nil, fmt.Errorf("parse fragment: %w", err)
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("fragment must be a mapping at the top level, found %s", kindName(root.Kind))
	}

	lines := strings.Split(body, "\n")

	// starts[i] is the 0-based line where key i's block begins.
	type entry struct {
		key   string
		start int
	}
	entries := make([]entry, 0, len(root.Content)/2)
	for i := 0; i+1 < len(root.Content); i += 2 {
		k := root.Content[i]
		start := k.Line - 1 // yaml.Node lines are 1-based
		for start > 0 {
			prev := strings.TrimSpace(lines[start-1])
			if prev == "" || strings.HasPrefix(prev, "#") {
				start--
				continue
			}
			break
		}
		entries = append(entries, entry{key: k.Value, start: start})
	}

	// A later key's block must not reach back past an earlier one.
	for i := 1; i < len(entries); i++ {
		if entries[i].start <= entries[i-1].start {
			entries[i].start = entries[i-1].start + 1
		}
	}

	out := make([]block, 0, len(entries))
	for i, e := range entries {
		end := len(lines)
		if i+1 < len(entries) {
			end = entries[i+1].start
		}
		out = append(out, block{
			key:  e.key,
			text: strings.Join(lines[e.start:end], "\n"),
		})
	}
	return out, nil
}

func kindName(k yaml.Kind) string {
	switch k {
	case yaml.SequenceNode:
		return "a list"
	case yaml.ScalarNode:
		return "a scalar"
	case yaml.AliasNode:
		return "an alias"
	case yaml.MappingNode:
		return "a mapping"
	default:
		return "an unknown node"
	}
}
