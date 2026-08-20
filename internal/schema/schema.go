// Package schema assembles a chart's values.schema.json from the per-resource
// fragments that internal/render carries.
//
// Unlike values.yaml, values.schema.json is a generated artifact: nothing in
// it is hand-written, so it is rebuilt whole rather than appended to. What it
// does share with the values merge is the first-writer-wins rule — "image" is
// contributed by every workload, and the chart gets one description of it.
//
// The schema is deliberately permissive. Helm validates the coalesced values
// against it on every render, so a schema that is merely incomplete does not
// document a chart, it breaks one: a key left undeclared, or typed more
// narrowly than the template actually accepts, turns a working values file
// into a failed release. Objects therefore stay open unless the Kubernetes API
// itself closes them, and a scalar whose default is empty is typed as the
// union it really accepts rather than the type of its placeholder.
package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// draft is the JSON Schema dialect. draft-07 rather than 2020-12: Helm has
// validated values against a schema since 3.0, and the older releases still in
// use only understand this one.
const draft = "http://json-schema.org/draft-07/schema#"

// Fragment is the values.schema.json contribution of a single resource.
type Fragment struct {
	// Resource is the catalog name the fragment came from, used in reports.
	Resource string
	// Body is a JSON object mapping a top-level values key to its schema.
	Body string
}

// Options tunes the assembled document.
type Options struct {
	// Title is the schema title, normally "<chart> values".
	Title string
	// Strict closes the top level, so an undeclared top-level key is an
	// error rather than a passthrough. Nested objects stay open: the point
	// is to catch a misspelled key, not to model the Kubernetes API.
	Strict bool
}

// Result reports what the build did, per top-level key.
type Result struct {
	// Added lists keys that reached the document.
	Added []string
	// Skipped lists keys a later fragment re-declared and so did not own.
	Skipped []string
}

// property is one top-level key and its schema, kept as raw bytes so the
// ordering written into the fragment survives into the output.
type property struct {
	key string
	raw json.RawMessage
}

// Build assembles the fragments into a values.schema.json document.
func Build(frags []Fragment, opts Options) ([]byte, Result, error) {
	var res Result

	seen := map[string]bool{}
	var props []property
	for _, f := range frags {
		parsed, err := parseOrdered([]byte(f.Body))
		if err != nil {
			return nil, res, fmt.Errorf("fragment %q: %w", f.Resource, err)
		}
		for _, p := range parsed {
			if seen[p.key] {
				res.Skipped = append(res.Skipped, p.key)
				continue
			}
			seen[p.key] = true
			res.Added = append(res.Added, p.key)
			props = append(props, p)
		}
	}

	return emit(props, opts), res, nil
}

// parseOrdered reads a JSON object into key/value pairs in document order.
// encoding/json unmarshals an object into a map, which loses that order, so
// the token stream is walked by hand.
func parseOrdered(src []byte) ([]property, error) {
	dec := json.NewDecoder(bytes.NewReader(src))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("parse schema fragment: %w", err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("schema fragment must be a JSON object, found %v", tok)
	}

	var out []property
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("parse schema fragment: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("schema fragment has a non-string key %v", keyTok)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("parse schema for %q: %w", key, err)
		}
		out = append(out, property{key: key, raw: raw})
	}
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("parse schema fragment: %w", err)
	}
	return out, nil
}

// emit writes the document. The envelope is written by hand rather than
// marshalled from a struct so that "properties" keeps the order the resources
// contributed it in, which is the order values.yaml documents them in too.
func emit(props []property, opts Options) []byte {
	var b bytes.Buffer
	b.WriteString("{\n")
	fmt.Fprintf(&b, "  %s: %s,\n", quote("$schema"), quote(draft))
	if opts.Title != "" {
		fmt.Fprintf(&b, "  %s: %s,\n", quote("title"), quote(opts.Title))
	}
	fmt.Fprintf(&b, "  %s: %s,\n", quote("type"), quote("object"))
	if opts.Strict {
		fmt.Fprintf(&b, "  %s: false,\n", quote("additionalProperties"))
	}
	b.WriteString("  \"properties\": {")

	for i, p := range props {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("\n    ")
		b.WriteString(quote(p.key))
		b.WriteString(": ")
		var indented bytes.Buffer
		// The value sits two levels in, so its continuation lines carry four
		// spaces of prefix on top of the usual two-space step.
		if err := json.Indent(&indented, p.raw, "    ", "  "); err != nil {
			// parseOrdered already decoded it, so this cannot fail; fall
			// back to the compact form rather than dropping the property.
			b.Write(p.raw)
			continue
		}
		b.Write(indented.Bytes())
	}

	if len(props) > 0 {
		b.WriteString("\n  ")
	}
	b.WriteString("}\n}\n")
	return b.Bytes()
}

// quote renders a JSON string. It goes through encoding/json so that a
// description carrying a quote or an em dash is escaped the same way the rest
// of the document is.
func quote(s string) string {
	out, err := json.Marshal(s)
	if err != nil { // a string never fails to marshal
		return `""`
	}
	return string(out)
}
