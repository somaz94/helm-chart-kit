// Package scaffold turns a request — create this chart, add these resources —
// into a plan of file writes, and applies it.
//
// Planning and applying are separate so that --dry-run shows exactly what a
// real run would do rather than an approximation of it, and so the interesting
// decisions stay testable without touching a disk.
package scaffold

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/somaz94/helm-chart-kit/internal/catalog"
	"github.com/somaz94/helm-chart-kit/internal/chart"
	"github.com/somaz94/helm-chart-kit/internal/render"
	"github.com/somaz94/helm-chart-kit/internal/schema"
	"github.com/somaz94/helm-chart-kit/internal/values"
)

// Action is what the plan does to one file.
type Action string

const (
	// Create writes a file that does not exist yet.
	Create Action = "create"
	// Update rewrites an existing file — only ever values.yaml, and only by
	// appending to it.
	Update Action = "update"
	// Skip leaves an existing file alone.
	Skip Action = "skip"
)

// File is one entry in a plan.
type File struct {
	// Path is relative to the chart directory.
	Path    string
	Action  Action
	Content []byte
	// Reason explains a Skip.
	Reason string
}

// Plan is the full set of changes a command would make.
type Plan struct {
	// ChartDir is the chart root, absolute.
	ChartDir string
	// NewChart is true when the plan creates the chart directory itself.
	NewChart bool
	Files    []File
	// ValuesAdded and ValuesSkipped are top-level values.yaml keys.
	ValuesAdded   []string
	ValuesSkipped []string
	// Notes are advisories worth printing: unmet requirements, resources
	// needing a CRD.
	Notes []string
}

// Changed reports whether applying the plan would write anything.
func (p *Plan) Changed() bool {
	for _, f := range p.Files {
		if f.Action != Skip {
			return true
		}
	}
	return false
}

// chartNameRE is the Helm chart name constraint: a lowercase DNS label,
// optionally dotted. Rejecting it here beats a `helm lint` failure later.
var chartNameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9._-]*[a-z0-9])?$`)

// ValidateName checks a chart name against Helm's own constraint.
func ValidateName(name string) error {
	switch {
	case name == "":
		return errors.New("chart name is empty")
	case len(name) > 63:
		return fmt.Errorf("chart name %q is longer than 63 characters", name)
	case !chartNameRE.MatchString(name):
		return fmt.Errorf("chart name %q must be lowercase alphanumeric, and may contain '-', '_' and '.'", name)
	}
	return nil
}

// NewOptions describes a chart to create.
type NewOptions struct {
	// Parent is the directory the chart directory is created in.
	Parent string
	// Name is the chart name and its directory name.
	Name string
	// Description goes into Chart.yaml.
	Description string
	// Version is the chart version.
	Version string
	// AppVersion is the deployed application's version.
	AppVersion string
	// Preset names the resource set to seed with.
	Preset string
	// Extra are resources added on top of the preset.
	Extra []string
	// Schema emits values.schema.json alongside values.yaml.
	Schema bool
	// SchemaStrict closes the top level of the generated schema.
	SchemaStrict bool
}

// PlanNew builds the plan for creating a chart.
func PlanNew(opts NewOptions) (*Plan, error) {
	if err := ValidateName(opts.Name); err != nil {
		return nil, err
	}
	preset, ok := catalog.LookupPreset(opts.Preset)
	if !ok {
		return nil, fmt.Errorf("unknown preset %q (known: %s)", opts.Preset, strings.Join(catalog.PresetNames(), ", "))
	}

	resources, err := resolve(append(append([]string{}, preset.Resources...), opts.Extra...))
	if err != nil {
		return nil, err
	}

	dir, err := filepath.Abs(filepath.Join(opts.Parent, opts.Name))
	if err != nil {
		return nil, err
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return nil, fmt.Errorf("%s already exists and is not empty", dir)
	}

	data := render.Data{
		ChartName:   opts.Name,
		Description: opts.Description,
		Version:     opts.Version,
		AppVersion:  opts.AppVersion,
		Preset:      opts.Preset,
		Resources:   names(resources),
	}

	plan := &Plan{ChartDir: dir, NewChart: true}

	skeleton, err := render.ChartFiles()
	if err != nil {
		return nil, err
	}
	sort.Strings(skeleton)

	var baseValues []byte
	for _, name := range skeleton {
		content, err := render.ChartFile(name, data)
		if err != nil {
			return nil, err
		}
		if name == "values.yaml" {
			baseValues = content
			continue // appended below, after the fragments are merged in
		}
		plan.Files = append(plan.Files, File{Path: outputName(name), Action: Create, Content: content})
	}

	frags := make([]values.Fragment, 0, len(resources))
	for _, r := range resources {
		tmpl, err := render.ResourceTemplate(r.Name, data)
		if err != nil {
			return nil, err
		}
		plan.Files = append(plan.Files, File{
			Path:    filepath.ToSlash(filepath.Join("templates", r.File)),
			Action:  Create,
			Content: tmpl,
		})
		frag, err := render.ResourceValues(r.Name, data)
		if err != nil {
			return nil, err
		}
		frags = append(frags, values.Fragment{Resource: r.Name, Body: string(frag)})
		if r.Optional {
			plan.Notes = append(plan.Notes, fmt.Sprintf("%s needs a CRD the cluster may not have (%s)", r.Name, r.APIVersion))
		}
	}

	merged, res, err := values.Merge(baseValues, frags)
	if err != nil {
		return nil, err
	}
	plan.ValuesAdded, plan.ValuesSkipped = res.Added, res.Skipped
	plan.Files = append(plan.Files, File{Path: "values.yaml", Action: Create, Content: merged})

	if opts.Schema {
		doc, _, err := BuildSchema(data, resources, opts.SchemaStrict)
		if err != nil {
			return nil, err
		}
		plan.Files = append(plan.Files, File{Path: SchemaFile, Action: Create, Content: doc})
	}

	return plan, nil
}

// PlanAdd builds the plan for adding resources to an existing chart.
func PlanAdd(c *chart.Chart, requested []string, force bool) (*Plan, error) {
	resources, err := resolve(requested)
	if err != nil {
		return nil, err
	}

	existingTemplates, err := c.TemplateFiles()
	if err != nil {
		return nil, err
	}
	if !force {
		if err := checkSingleWorkload(resources, existingTemplates); err != nil {
			return nil, err
		}
	}

	data := DataFor(c)
	data.Resources = names(resources)

	plan := &Plan{ChartDir: c.Dir}
	present := presentResources(existingTemplates)
	existing := resourcesFrom(present)

	frags := make([]values.Fragment, 0, len(resources))
	for _, r := range resources {
		if c.HasTemplate(r.File) && !force {
			plan.Files = append(plan.Files, File{
				Path:   filepath.ToSlash(filepath.Join("templates", r.File)),
				Action: Skip,
				Reason: "already exists",
			})
			continue
		}
		tmpl, err := render.ResourceTemplate(r.Name, data)
		if err != nil {
			return nil, err
		}
		action := Create
		if c.HasTemplate(r.File) {
			action = Update
		}
		plan.Files = append(plan.Files, File{
			Path:    filepath.ToSlash(filepath.Join("templates", r.File)),
			Action:  action,
			Content: tmpl,
		})
		frag, err := render.ResourceValues(r.Name, data)
		if err != nil {
			return nil, err
		}
		frags = append(frags, values.Fragment{Resource: r.Name, Body: string(frag)})

		for _, req := range r.Requires {
			if !present[req] && !contains(names(resources), req) {
				plan.Notes = append(plan.Notes,
					fmt.Sprintf("%s expects %s, which this chart does not have — run: hck add %s", r.Name, req, req))
			}
		}
		if r.Optional {
			plan.Notes = append(plan.Notes, fmt.Sprintf("%s needs a CRD the cluster may not have (%s)", r.Name, r.APIVersion))
		}
	}

	current, err := c.Values()
	if err != nil {
		return nil, err
	}
	merged, res, err := values.Merge(current, frags)
	if err != nil {
		return nil, err
	}
	plan.ValuesAdded, plan.ValuesSkipped = res.Added, res.Skipped
	if res.Changed() {
		plan.Files = append(plan.Files, File{Path: "values.yaml", Action: Update, Content: merged})
	} else {
		plan.Files = append(plan.Files, File{Path: "values.yaml", Action: Skip, Reason: "no new keys"})
	}

	// A chart that already declares a schema has to keep declaring every key
	// its values.yaml carries — Helm validates the two against each other on
	// every render, so leaving the schema behind would break the chart.
	//
	// One read answers both questions asked of the file, and surfaces an
	// unreadable one instead of quietly rebuilding it permissive.
	currentSchema, err := c.Schema()
	if err != nil {
		return nil, err
	}
	if currentSchema != nil {
		after := union(existing, resources)
		doc, _, err := BuildSchema(data, after, SchemaIsStrictBytes(currentSchema))
		if err != nil {
			return nil, err
		}
		if bytes.Equal(currentSchema, doc) {
			plan.Files = append(plan.Files, File{Path: SchemaFile, Action: Skip, Reason: "already up to date"})
		} else {
			plan.Files = append(plan.Files, File{Path: SchemaFile, Action: Update, Content: doc})
		}
	}

	return plan, nil
}

// SchemaIsStrictBytes reports whether a schema document closes its top level,
// so that regenerating it preserves the choice the author made.
//
// It takes bytes rather than a chart because the caller has to read the file
// anyway to compare against it, and reading it twice invites the two reads to
// disagree. Absent or unparseable input is reported as not strict — the
// permissive guess, which is the safe one — but an unreadable file is a real
// error and belongs to whoever did the read, not here.
func SchemaIsStrictBytes(raw []byte) bool {
	var doc struct {
		AdditionalProperties *bool `json:"additionalProperties"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return false
	}
	return doc.AdditionalProperties != nil && !*doc.AdditionalProperties
}

// Apply writes the plan to disk.
func Apply(p *Plan) error {
	for _, f := range p.Files {
		if f.Action == Skip {
			continue
		}
		dest := filepath.Join(p.ChartDir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(dest), err)
		}
		if err := os.WriteFile(dest, f.Content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", dest, err)
		}
	}
	return nil
}

// resolve maps names to catalog resources, dropping duplicates and keeping
// the order they were asked for.
func resolve(requested []string) ([]catalog.Resource, error) {
	seen := map[string]bool{}
	out := make([]catalog.Resource, 0, len(requested))
	for _, name := range requested {
		if seen[name] {
			continue
		}
		r, ok := catalog.LookupResource(name)
		if !ok {
			return nil, fmt.Errorf("unknown resource %q (see: hck list resources)", name)
		}
		seen[name] = true
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, errors.New("no resources requested")
	}
	return out, nil
}

// checkSingleWorkload refuses a second primary workload in one chart.
func checkSingleWorkload(adding []catalog.Resource, existingTemplates []string) error {
	present := presentResources(existingTemplates)
	var have []string
	for _, r := range catalog.Resources() {
		if r.Workload && present[r.Name] {
			have = append(have, r.Name)
		}
	}
	if len(have) == 0 {
		return nil
	}
	for _, r := range adding {
		if r.Workload {
			return fmt.Errorf(
				"chart already has the %s workload; adding %s would give it two, and they contend for the same values keys with incompatible shapes. Split it into two charts, or pass --force if you really mean it",
				strings.Join(have, " and "), r.Name)
		}
	}
	return nil
}

// presentResources maps catalog names to whether the chart carries their file.
func presentResources(templateFiles []string) map[string]bool {
	byFile := map[string]string{}
	for _, r := range catalog.Resources() {
		byFile[r.File] = r.Name
	}
	out := map[string]bool{}
	for _, f := range templateFiles {
		if name, ok := byFile[f]; ok {
			out[name] = true
		}
	}
	return out
}

// outputName maps an embedded skeleton file to its name in the chart. The
// leading dot is dropped in the embedded tree because go:embed skips dotfiles
// unless the pattern opts back in, and an "all:" pattern would also drag in
// editor droppings.
func outputName(name string) string {
	if name == "helmignore" {
		return ".helmignore"
	}
	return name
}

func names(rs []catalog.Resource) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Name)
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// DataFor builds the template substitution context from a chart on disk.
func DataFor(c *chart.Chart) render.Data {
	return render.Data{
		ChartName:   c.Meta.Name,
		Description: c.Meta.Description,
		Version:     c.Meta.Version,
		AppVersion:  c.Meta.AppVersion,
		Preset:      c.Meta.Annotations["helm-chart-kit/preset"],
	}
}

// SchemaFile is the generated schema's name inside a chart.
const SchemaFile = "values.schema.json"

// canonicalOrder fixes the order resources contribute to values.yaml and to
// values.schema.json: the workload first, then the rest by name.
//
// Determinism is the point — "hck schema --check" compares bytes, so a chart
// read back off disk has to reassemble in the same order it was written in.
// Workload-first is what makes the two agree on a key two resources both
// define: "persistence" belongs to the StatefulSet when the chart has one,
// and to the PVC otherwise, which is exactly what the values merge decides.
func canonicalOrder(rs []catalog.Resource) []catalog.Resource {
	out := make([]catalog.Resource, len(rs))
	copy(out, rs)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Workload != out[j].Workload {
			return out[i].Workload
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// ChartResources reports which catalog resources a chart on disk carries.
func ChartResources(c *chart.Chart) ([]catalog.Resource, error) {
	files, err := c.TemplateFiles()
	if err != nil {
		return nil, err
	}
	return canonicalOrder(resourcesFrom(presentResources(files))), nil
}

// union merges two resource lists, dropping duplicates. "hck add configmap"
// against a chart that already has one names it in both, and a schema built
// from the doubled list renders the same fragment twice and reports phantom
// skipped keys.
func union(a, b []catalog.Resource) []catalog.Resource {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]catalog.Resource, 0, len(a)+len(b))
	for _, r := range append(append([]catalog.Resource{}, a...), b...) {
		if seen[r.Name] {
			continue
		}
		seen[r.Name] = true
		out = append(out, r)
	}
	return out
}

// resourcesFrom maps a presence set to catalog entries, ordered by name.
func resourcesFrom(present map[string]bool) []catalog.Resource {
	out := make([]catalog.Resource, 0, len(present))
	for _, r := range catalog.Resources() {
		if present[r.Name] {
			out = append(out, r)
		}
	}
	return out
}

// BuildSchema assembles values.schema.json for a resource set.
func BuildSchema(data render.Data, resources []catalog.Resource, strict bool) ([]byte, schema.Result, error) {
	var empty schema.Result

	base, err := render.BaseSchema(data)
	if err != nil {
		return nil, empty, err
	}
	frags := make([]schema.Fragment, 0, len(resources)+1)
	frags = append(frags, schema.Fragment{Resource: "chart", Body: string(base)})
	for _, r := range canonicalOrder(resources) {
		body, err := render.ResourceSchema(r.Name, data)
		if err != nil {
			return nil, empty, fmt.Errorf("schema fragment for %s: %w", r.Name, err)
		}
		frags = append(frags, schema.Fragment{Resource: r.Name, Body: string(body)})
	}
	return schema.Build(frags, schema.Options{Title: data.ChartName + " values", Strict: strict})
}
