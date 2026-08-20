// Package docs turns a chart's values.yaml into a Markdown table.
//
// The descriptions come from the file itself: a comment line opening with
// "-- " documents the key below it, the helm-docs convention the templates
// already follow. Reading them back out of the chart rather than out of the
// catalog means the table describes what the user actually has, keys they
// added by hand included.
//
// Types and allowed values come from the schema when one is supplied, because
// values.yaml cannot express them: a key defaulting to "" says nothing about
// whether it takes a string or a number, and nothing at all about the four
// values the API will accept.
package docs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Row is one documented value.
type Row struct {
	// Key is the dotted path, e.g. "image.repository".
	Key string
	// Type is the YAML kind, refined by the schema where it disagrees.
	Type string
	// Default is the value rendered for a Markdown cell.
	Default string
	// Description is the "-- " comment, or empty.
	Description string
}

// Options tunes the generated table.
type Options struct {
	// Schema is an assembled values.schema.json. Optional: without it the
	// table still lists every key, just with no allowed-value detail.
	Schema []byte
}

// Table renders values.yaml as a Markdown table.
func Table(values []byte, opts Options) (string, error) {
	rows, err := Rows(values, opts)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("| Key | Type | Default | Description |\n")
	b.WriteString("|---|---|---|---|\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n",
			r.Key, r.Type, r.Default, cell(r.Description))
	}
	return b.String(), nil
}

// Rows walks values.yaml in document order and returns one row per leaf.
func Rows(values []byte, opts Options) ([]Row, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(values, &doc); err != nil {
		return nil, fmt.Errorf("parse values: %w", err)
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("values must be a mapping at the top level")
	}

	types := map[string]schemaFacts{}
	if len(opts.Schema) > 0 {
		if err := collectSchema(opts.Schema, types); err != nil {
			return nil, err
		}
	}

	var rows []Row
	walk(root, nil, types, &rows)
	return rows, nil
}

// walk descends a mapping, emitting a row per leaf and recursing into any
// mapping that has children. An empty mapping is a leaf: "{}" is the value,
// and there is nothing below it to describe.
func walk(node *yaml.Node, path []string, facts map[string]schemaFacts, out *[]Row) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		k, v := node.Content[i], node.Content[i+1]
		here := append(append([]string{}, path...), k.Value)
		key := strings.Join(here, ".")

		if v.Kind == yaml.MappingNode && len(v.Content) > 0 {
			walk(v, here, facts, out)
			continue
		}

		row := Row{
			Key:         key,
			Type:        yamlType(v),
			Default:     renderDefault(v),
			Description: description(k, v),
		}
		if f, ok := facts[key]; ok {
			if f.typeName != "" {
				row.Type = f.typeName
			}
			if len(f.enum) > 0 {
				row.Description = strings.TrimSpace(sentence(row.Description) + fmt.Sprintf(" One of: %s.", quoteAll(f.enum)))
			}
		}
		*out = append(*out, row)
	}
}

// description pulls the documented text out of an entry's comments. Only a
// line opening with "-- " starts one; the banner comments that divide
// values.yaml into sections are not descriptions of the key that happens to
// follow them.
//
// A trailing comment lands on the value node rather than the key node — "a: 1
// # note" belongs to the 1 — so both are read, head comment first.
func description(key, value *yaml.Node) string {
	for _, c := range []string{key.HeadComment, value.HeadComment, key.LineComment, value.LineComment} {
		if d := fromComment(c); d != "" {
			return d
		}
	}
	return ""
}

func fromComment(c string) string {
	if c == "" {
		return ""
	}
	var parts []string
	started := false
	for _, raw := range strings.Split(c, "\n") {
		text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "#"))
		if !started {
			rest, ok := strings.CutPrefix(text, "-- ")
			if !ok {
				continue
			}
			started = true
			parts = append(parts, strings.TrimSpace(rest))
			continue
		}
		// A blank line ends the description; what follows is commentary
		// about the section, not about this key.
		if text == "" {
			break
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, " ")
}

// yamlType names the kind a value has in the file.
func yamlType(v *yaml.Node) string {
	switch v.Kind {
	case yaml.MappingNode:
		return "object"
	case yaml.SequenceNode:
		return "list"
	}
	switch v.Tag {
	case "!!int":
		return "int"
	case "!!float":
		return "float"
	case "!!bool":
		return "bool"
	case "!!null":
		return "null"
	default:
		return "string"
	}
}

// renderDefault formats a value for a table cell, on one line.
func renderDefault(v *yaml.Node) string {
	switch {
	case v.Kind == yaml.MappingNode && len(v.Content) == 0:
		return "`{}`"
	case v.Kind == yaml.SequenceNode && len(v.Content) == 0:
		return "`[]`"
	case v.Kind == yaml.MappingNode, v.Kind == yaml.SequenceNode:
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(v); err != nil {
			return "`?`"
		}
		_ = enc.Close()
		// Flatten: a table cell is one line, and a multi-line default is
		// better shown compactly than not at all.
		flat := strings.Join(strings.Fields(buf.String()), " ")
		return "`" + strings.ReplaceAll(flat, "|", "\\|") + "`"
	case v.Tag == "!!null":
		return "`null`"
	case v.Value == "":
		return `` + "`\"\"`"
	default:
		return "`" + strings.ReplaceAll(v.Value, "|", "\\|") + "`"
	}
}

// cell escapes a description for a Markdown table.
func cell(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

// sentence closes a description so an appended clause does not run into it.
// The comments are written as prose but not all of them end in a full stop.
func sentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.ContainsAny(s[len(s)-1:], ".!?:") {
		return s
	}
	return s + "."
}

func quoteAll(vs []string) string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, "`"+v+"`")
	}
	return strings.Join(out, ", ")
}

// schemaFacts is what the schema knows about one key that values.yaml cannot
// express on its own.
type schemaFacts struct {
	typeName string
	enum     []string
}

// collectSchema flattens a values.schema.json into dotted key paths.
func collectSchema(raw []byte, out map[string]schemaFacts) error {
	var doc struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("parse schema: %w", err)
	}
	return collectProps(doc.Properties, nil, out)
}

func collectProps(props map[string]json.RawMessage, path []string, out map[string]schemaFacts) error {
	// Sorted for determinism; the row order comes from values.yaml, not here.
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		var node struct {
			Type       json.RawMessage            `json:"type"`
			Enum       []any                      `json:"enum"`
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(props[name], &node); err != nil {
			return fmt.Errorf("parse schema for %q: %w", name, err)
		}
		here := append(append([]string{}, path...), name)
		key := strings.Join(here, ".")

		f := schemaFacts{typeName: typeNames(node.Type)}
		for _, e := range node.Enum {
			f.enum = append(f.enum, fmt.Sprint(e))
		}
		if f.typeName != "" || len(f.enum) > 0 {
			out[key] = f
		}
		if len(node.Properties) > 0 {
			if err := collectProps(node.Properties, here, out); err != nil {
				return err
			}
		}
	}
	return nil
}

// typeNames renders a schema "type", which is either a string or a list of
// them. A union is what a key whose default is empty really accepts, and
// hiding half of it is how a values table becomes wrong.
func typeNames(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		return normalizeType(one)
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return ""
	}
	out := make([]string, 0, len(many))
	for _, t := range many {
		out = append(out, normalizeType(t))
	}
	return strings.Join(out, "/")
}

// normalizeType maps JSON Schema's names onto the ones yamlType produces, so
// the Type column does not mix "boolean" with "bool" depending on whether the
// schema happened to describe the key.
func normalizeType(t string) string {
	switch t {
	case "boolean":
		return "bool"
	case "integer":
		return "int"
	case "number":
		return "float"
	case "array":
		return "list"
	default:
		return t
	}
}

// The generated table lives between these two markers, so everything a person
// wrote around it survives a regeneration. helm-docs uses a comparable pair;
// these are named for this tool so the two can coexist in one README.
const (
	StartMarker = "<!-- hck:values:start -->"
	EndMarker   = "<!-- hck:values:end -->"
)

// Replace swaps the block between the markers for a freshly generated table.
// A README with no markers gets them appended, under a heading, rather than
// being rewritten.
func Replace(readme []byte, table string) ([]byte, error) {
	block := StartMarker + "\n\n" + strings.TrimRight(table, "\n") + "\n\n" + EndMarker

	text := string(readme)
	start := strings.Index(text, StartMarker)
	end := strings.Index(text, EndMarker)

	switch {
	case start < 0 && end < 0:
		out := strings.TrimRight(text, "\n")
		if out != "" {
			out += "\n\n<br/>\n\n"
		}
		return []byte(out + "## Values\n\n" + block + "\n"), nil
	case start < 0 || end < 0:
		return nil, fmt.Errorf("README has %s without %s; the pair delimits the generated table", presentMarker(start >= 0), missingMarker(start >= 0))
	case end < start:
		return nil, fmt.Errorf("README has %s before %s", EndMarker, StartMarker)
	}
	return []byte(text[:start] + block + text[end+len(EndMarker):]), nil
}

func presentMarker(startPresent bool) string {
	if startPresent {
		return StartMarker
	}
	return EndMarker
}

func missingMarker(startPresent bool) string {
	if startPresent {
		return EndMarker
	}
	return StartMarker
}

// Skeleton is the README a chart gets when it has none and asks for a table.
// It stops before the Values heading: Replace appends that, along with the
// <br/> the documentation convention puts between heading sections.
func Skeleton(name, description string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", name)
	if description != "" {
		fmt.Fprintf(&b, "\n%s\n", description)
	}
	return b.String()
}
