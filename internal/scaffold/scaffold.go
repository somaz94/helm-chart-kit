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
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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
	// Delete removes a file the chart carries.
	Delete Action = "delete"
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
	Files    []File
	// ValuesAdded and ValuesSkipped are top-level values.yaml keys.
	ValuesAdded   []string
	ValuesSkipped []string
	// ValuesOrphaned are keys a removal leaves behind. values.yaml is never
	// rewritten, so they stay in the file and the plan names them instead.
	ValuesOrphaned []string
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
	// Platforms names the platform overlays to write alongside values.yaml.
	Platforms []string
	// Environments names the environment overlays to write alongside them.
	Environments []string
	// Force waives the two refusals that stand between a request and a chart:
	// a second primary workload, and a target directory that is not empty.
	//
	// It is the escape hatch, not a second mode. A second workload still
	// renders a chart "hck check" reports HCK030 over, and a non-empty
	// directory is filled in rather than overwritten — every file already
	// there is skipped, values.yaml included, because values.yaml is never
	// rewritten.
	Force bool
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
	// --with can name a workload on top of the one the preset already brings.
	// "hck add" has always refused that; creating the same chart in one shot
	// used to be allowed, which is the more likely way to reach for it.
	if !opts.Force {
		if err := checkSingleWorkload(resources, nil); err != nil {
			return nil, fmt.Errorf("%w, or pass --force if you really mean it", err)
		}
	}

	dir, err := filepath.Abs(filepath.Join(opts.Parent, opts.Name))
	if err != nil {
		return nil, err
	}
	occupied := false
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		if !opts.Force {
			return nil, fmt.Errorf("%s already exists and is not empty; pass --force to fill in what is missing there", dir)
		}
		occupied = true
	}

	data := render.Data{
		ChartName:    opts.Name,
		Description:  opts.Description,
		Version:      opts.Version,
		AppVersion:   opts.AppVersion,
		Preset:       opts.Preset,
		Resources:    names(resources),
		WorkloadKind: workloadKind(resources),
	}

	plan := &Plan{ChartDir: dir}

	skeleton, err := render.ChartFiles()
	if err != nil {
		return nil, err
	}
	slices.Sort(skeleton)

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

	frags, err := planResources(plan, data, resources, resourceState{})
	if err != nil {
		return nil, err
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

	// Platforms first, then environments: both write into one file-name space
	// and the order they are generated in is the order they are meant to be
	// passed to helm.
	for _, axis := range []catalog.Axis{catalog.PlatformAxis, catalog.EnvironmentAxis} {
		requested := opts.Platforms
		if axis == catalog.EnvironmentAxis {
			requested = opts.Environments
		}
		for _, name := range requested {
			o, ok := catalog.LookupOverlay(axis, name)
			if !ok {
				return nil, fmt.Errorf("unknown %s %q (known: %s)", axis, name, strings.Join(catalog.OverlayNames(axis), ", "))
			}
			overlay, ok, err := BuildOverlayValues(data, resources, o)
			if err != nil {
				return nil, err
			}
			if !ok {
				plan.Notes = append(plan.Notes,
					noOverlayNote(o))
				continue
			}
			plan.Files = append(plan.Files, File{Path: o.ValuesFile(), Action: Create, Content: overlay})
			if len(o.Needs) > 0 {
				plan.Notes = append(plan.Notes,
					fmt.Sprintf("%s expects the cluster to have: %s", o.Name, strings.Join(o.Needs, ", ")))
			}
		}
	}

	if occupied {
		if kept := keepWhatIsThere(plan); kept > 0 {
			plan.Notes = append(plan.Notes, fmt.Sprintf(
				"%d file(s) were already there and were left alone; to extend a chart that exists, use: hck add", kept))
		}
	}

	return plan, nil
}

// keepWhatIsThere turns every write over a file that is already on disk into a
// Skip.
//
// This is what --force means on a directory that is not empty: fill in what is
// missing, and touch nothing that is there. Overwriting would take values.yaml
// with it, and values.yaml is never rewritten — a chart somebody has spent an
// afternoon on would come back as the defaults, with no way to tell what was
// lost.
func keepWhatIsThere(p *Plan) int {
	kept := 0
	for i, f := range p.Files {
		if f.Action != Create {
			continue
		}
		if _, err := os.Stat(filepath.Join(p.ChartDir, filepath.FromSlash(f.Path))); err != nil {
			continue
		}
		p.Files[i] = File{Path: f.Path, Action: Skip, Reason: "already there"}
		if f.Path == "values.yaml" {
			// The merge already ran and named every key it was going to
			// contribute. None of them are going anywhere now, and a plan
			// that still listed them would be describing a file it is not
			// writing.
			p.ValuesAdded, p.ValuesSkipped = nil, nil
		}
		kept++
	}
	return kept
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
			return nil, fmt.Errorf("%w, or pass --force if you really mean it", err)
		}
	}

	plan := &Plan{ChartDir: c.Dir}
	present := presentResources(existingTemplates)
	existing := resourcesFrom(present)

	// The finished chart, not just what is arriving: a scaler added to a chart
	// that already has a StatefulSet has to point at the StatefulSet, and the
	// arriving list does not mention it.
	after := union(existing, resources)
	data := DataFor(c)
	data.Resources = names(after)
	data.WorkloadKind = workloadKind(after)

	frags, err := planResources(plan, data, resources, resourceState{
		present: present,
		hasFile: c.HasTemplate,
		force:   force,
	})
	if err != nil {
		return nil, err
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

// resourceState is what the chart already looks like. The zero value describes
// a chart that does not exist yet, which is what lets one loop serve both
// "hck new" and "hck add".
type resourceState struct {
	// present holds the catalog names the chart already carries.
	present map[string]bool
	// hasFile reports whether templates/<file> is already there. Nil for a
	// chart being created, where nothing is.
	hasFile func(string) bool
	// force rewrites a template that exists rather than skipping it.
	force bool
}

// planResources appends one template file per resource to the plan, collects
// the values fragments they contribute, and records the advisories they carry.
//
// One loop rather than one per command: creating a chart and adding to one
// differ only in what is already on disk, and while this was written twice the
// two copies disagreed about which advisories to print.
func planResources(plan *Plan, data render.Data, resources []catalog.Resource, st resourceState) ([]values.Fragment, error) {
	incoming := names(resources)
	frags := make([]values.Fragment, 0, len(resources))
	for _, r := range resources {
		dest := filepath.ToSlash(filepath.Join("templates", r.File))
		exists := st.hasFile != nil && st.hasFile(r.File)
		if exists && !st.force {
			plan.Files = append(plan.Files, File{Path: dest, Action: Skip, Reason: "already exists"})
			continue
		}
		tmpl, err := render.ResourceTemplate(r.Name, data)
		if err != nil {
			return nil, err
		}
		action := Create
		if exists {
			action = Update
		}
		plan.Files = append(plan.Files, File{Path: dest, Action: action, Content: tmpl})

		frag, err := render.ResourceValues(r.Name, data)
		if err != nil {
			return nil, err
		}
		frags = append(frags, values.Fragment{Resource: r.Name, Body: string(frag)})

		// A preset satisfies its own resources' requirements, but --with can
		// name one it does not: "hck new x --preset cronjob --with
		// referencegrant" has no Service.
		for _, req := range r.Requires {
			if !st.present[req] && !slices.Contains(incoming, req) {
				plan.Notes = append(plan.Notes,
					fmt.Sprintf("%s expects %s, which this chart does not have — run: hck add %s", r.Name, req, req))
			}
		}
		if r.Optional {
			plan.Notes = append(plan.Notes, optionalNote(r))
		}
	}
	return frags, nil
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
		dest := filepath.Join(p.ChartDir, filepath.FromSlash(f.Path))
		switch f.Action {
		case Skip:
			continue
		case Delete:
			if err := os.Remove(dest); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("remove %s: %w", dest, err)
			}
			continue
		}
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

// checkSingleWorkload refuses a chart that would end up carrying more than one
// primary workload.
//
// The count is over the finished chart, not just over what is arriving: two
// workloads named in the same command are the same defect as one landing next
// to one already there, and both used to slip through — the old version
// returned early whenever the chart had none yet.
//
// They contend for image, resources and updateStrategy with incompatible
// shapes, and the first to be merged wins, so the chart renders and then does
// not apply. With a values.schema.json it is worse: the schema resolves the
// contested key by canonical order while values.yaml resolves it by merge
// order, the two pick different owners, and helm rejects values the workload
// actually in the chart accepts.
func checkSingleWorkload(adding []catalog.Resource, existingTemplates []string) error {
	present := presentResources(existingTemplates)
	var have []string
	for _, r := range catalog.Resources() {
		if r.Workload && present[r.Name] {
			have = append(have, r.Name)
		}
	}
	var incoming []string
	for _, r := range adding {
		if r.Workload && !present[r.Name] {
			incoming = append(incoming, r.Name)
		}
	}
	if len(have)+len(incoming) < 2 {
		return nil
	}
	if len(have) > 0 {
		return fmt.Errorf(
			"chart already has the %s workload; adding %s would give it two, and they contend for the same values keys with incompatible shapes. Split it into two charts",
			strings.Join(have, " and "), strings.Join(incoming, " and "))
	}
	return fmt.Errorf(
		"%s are both primary workloads and a chart carries one; they contend for the same values keys with incompatible shapes. Split them into two charts",
		strings.Join(incoming, " and "))
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

// workloadKindByName maps a catalog workload to the Kubernetes kind its
// template emits. Hand-written and then checked: TestWorkloadKindsMatchTheTemplates
// renders each one and compares, so an entry that drifts from its template
// fails rather than mis-aiming a scaler.
var workloadKindByName = map[string]string{
	"deployment":  "Deployment",
	"statefulset": "StatefulSet",
	"daemonset":   "DaemonSet",
	"cronjob":     "CronJob",
}

// workloadKind is the kind of the one primary workload a resource set carries,
// or "" when it carries none. A chart with two is refused before this is
// reached, so the first is the only one.
func workloadKind(rs []catalog.Resource) string {
	for _, r := range canonicalOrder(rs) {
		if r.Workload {
			return workloadKindByName[r.Name]
		}
	}
	return ""
}

// DataFor builds the template substitution context from a chart on disk.
//
// WorkloadKind is deliberately left empty here: it needs the chart's resource
// set, which this does not read. Every caller that renders a values fragment
// resolves it from the set it already has.
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
	out := slices.Clone(rs)
	slices.SortStableFunc(out, func(a, b catalog.Resource) int {
		if a.Workload != b.Workload {
			if a.Workload {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
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

// overlayHeader opens a generated overlay. It says what the file is for,
// because a values file with no context is indistinguishable from one someone
// meant to finish editing.
func overlayHeader(chartName, summary, install string, needs []string) string {
	var b strings.Builder
	b.WriteString("# =============================================================================\n")
	fmt.Fprintf(&b, "# %s — %s\n", chartName, summary)
	b.WriteString("#\n")
	b.WriteString("# An overlay, not a replacement. Helm reads values.yaml first and always,\n")
	b.WriteString("# so this file carries only what is different here:\n")
	b.WriteString("#\n")
	fmt.Fprintf(&b, "#   helm install %s . %s\n", chartName, install)
	b.WriteString("#\n")
	if len(needs) > 0 {
		b.WriteString("# Generated by hck. Expects the cluster to already have:\n")
		for _, n := range needs {
			fmt.Fprintf(&b, "#   - %s\n", n)
		}
	} else {
		b.WriteString("# Generated by hck.\n")
	}
	b.WriteString("# =============================================================================\n")
	return b.String()
}

// buildOverlay collects the fragments named for one suffix across a resource
// set. It returns false when nothing in the chart differs there, so a worker
// chart does not get a file consisting of a header.
func buildOverlay(data render.Data, resources []catalog.Resource, suffix, header string) ([]byte, bool, error) {
	var body strings.Builder
	for _, r := range canonicalOrder(resources) {
		frag, ok, err := render.ResourceOverlayValues(r.Name, suffix, data)
		if err != nil {
			return nil, false, fmt.Errorf("%s overlay for %s: %w", suffix, r.Name, err)
		}
		if !ok {
			continue
		}
		if body.Len() > 0 {
			body.WriteString("\n")
		}
		body.WriteString(strings.TrimRight(string(frag), "\n"))
		body.WriteString("\n")
	}
	if body.Len() == 0 {
		return nil, false, nil
	}
	return []byte(header + "\n" + body.String()), true, nil
}

// BuildOverlayValues assembles one overlay for a resource set.
//
// The environment install line puts the environment last on purpose: overlays
// are applied left to right, so the size the environment asks for wins over
// whatever a platform overlay happened to set.
func BuildOverlayValues(data render.Data, resources []catalog.Resource, o catalog.Overlay) ([]byte, bool, error) {
	install := "-f " + o.ValuesFile()
	if o.Axis == catalog.EnvironmentAxis {
		install = "[-f values-<platform>.yaml] " + install
	}
	return buildOverlay(data, resources, o.Name, overlayHeader(data.ChartName, o.Summary, install, o.Needs))
}

// ChartOverlays reports which of an axis's overlays a chart on disk carries.
// Best effort: an overlay that cannot be stat'd is reported as absent, which
// for a listing is the honest answer — "hck platform add" is where an
// unreadable file has to be an error, and it is one there.
func ChartOverlays(c *chart.Chart, axis catalog.Axis) []catalog.Overlay {
	var out []catalog.Overlay
	for _, o := range catalog.Overlays(axis) {
		if _, err := os.Stat(filepath.Join(c.Dir, o.ValuesFile())); err == nil {
			out = append(out, o)
		}
	}
	return out
}

// noOverlayNote explains an overlay that came out empty, which happens when
// nothing the chart carries has a fragment for that name.
//
// The preposition follows the axis: a chart differs "on aws" but "at prod".
// No preset reaches this today — every one carries a workload, and every
// workload differs on every platform — so it is covered by a test directly
// rather than through a generated chart.
func noOverlayNote(o catalog.Overlay) string {
	in := "on"
	if o.Axis == catalog.EnvironmentAxis {
		in = "at"
	}
	return fmt.Sprintf("nothing in this chart differs %s %s, so no %s was written", in, o.Name, o.ValuesFile())
}

// optionalNote explains what an optional resource is waiting on. Most are
// waiting on a CRD; a Grafana dashboard is a core ConfigMap waiting on a
// sidecar that reads it, and calling that a CRD sends the reader looking for
// the wrong thing.
func optionalNote(r catalog.Resource) string {
	if strings.Contains(r.APIVersion, "/") {
		return fmt.Sprintf("%s needs a CRD the cluster may not have (%s)", r.Name, r.APIVersion)
	}
	return fmt.Sprintf("%s needs something outside the cluster API to read it (%s)", r.Name, r.Summary)
}
