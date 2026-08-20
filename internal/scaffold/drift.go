package scaffold

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/somaz94/helm-chart-kit/internal/catalog"
	"github.com/somaz94/helm-chart-kit/internal/chart"
	"github.com/somaz94/helm-chart-kit/internal/render"
)

// State is how a chart's copy of a generated template compares to what hck
// would write for it today.
type State string

const (
	// Current means the file is byte for byte what hck generates.
	Current State = "current"
	// Edited means it is not. Whether that is a local change or an hck
	// template that moved on is not something this can tell — both look
	// identical from here, and saying so is more useful than guessing.
	Edited State = "edited"
	// Missing means hck wrote the file and it is no longer there.
	Missing State = "missing"
	// Unreadable means the file is there and could not be read. Reported
	// rather than treated as edited: a permissions problem is not a decision
	// anybody made, and calling it drift would invite --write to overwrite a
	// file nobody has seen.
	Unreadable State = "unreadable"
)

// Drift is one file's comparison.
type Drift struct {
	// Resource names the thing "hck sync" refers to: a catalog resource name,
	// or the file's own path for a skeleton file, which is what the report
	// prints either way.
	Resource string
	// Path is the file inside the chart, slash-separated.
	Path  string
	State State
	// Want is what hck generates now. It is what "hck sync --write" writes and
	// what "hck remove" compares against, so both read one rendering.
	Want []byte
	// Have is what the chart carries, or nil when it could not be read.
	Have []byte
	// Err explains an Unreadable.
	Err error
	// Skeleton marks a chart-skeleton file rather than a catalog resource.
	// The difference matters to --write: a skeleton file carries no values, so
	// putting a missing one back is a repair. A missing resource template is a
	// resource the chart does not have, and adding one is "hck add", where the
	// values it needs are appended too.
	Skeleton bool
}

// AnyDrifted reports whether any resource's file is not what hck writes today.
func AnyDrifted(ds []Drift) bool {
	for _, d := range ds {
		if d.State != Current {
			return true
		}
	}
	return false
}

// DriftOf compares one resource's file against what hck generates for it.
func DriftOf(c *chart.Chart, r catalog.Resource) (Drift, error) {
	data := DataFor(c)
	want, err := render.ResourceTemplate(r.Name, data)
	if err != nil {
		return Drift{}, err
	}
	d := Drift{Resource: r.Name, Path: filepath.ToSlash(filepath.Join("templates", r.File)), Want: want}

	compare(&d, c.TemplatePath(r.File))
	return d, nil
}

// compare fills in a Drift's state by reading the file it describes.
func compare(d *Drift, path string) {
	have, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		d.State, d.Err = Missing, err
	case err != nil:
		d.State, d.Err = Unreadable, err
	case bytes.Equal(have, d.Want):
		d.State, d.Have = Current, have
	default:
		d.State, d.Have = Edited, have
	}
}

// skeletonNotOwned are the chart-skeleton files hck writes once and does not
// own afterwards, with the reason. Everything else in the skeleton is compared
// by "hck sync", and TestSkeletonIsEitherComparedOrExcused makes sure a file
// added to the embedded tree cannot quietly escape both.
//
// The distinction is not a detail. Regenerating either of these would report
// drift on every chart that ever grew, and --write would delete the parts hck
// never wrote.
var skeletonNotOwned = map[string]string{
	"Chart.yaml":  "the author maintains it: dependencies, maintainers, keywords, and a version that moves on every change",
	"values.yaml": "it is appended to and never rewritten, so the skeleton is only ever the first few lines of it",
}

// skeletonDrift compares the chart-skeleton files hck owns outright.
//
// These are the ones with no way to notice: _helpers.tpl is what every
// resource template calls into, and .helmignore decides what a package
// sweeps up. Neither is a resource, so neither was ever compared, and an
// hck that changed one left every existing chart quietly behind.
func skeletonDrift(c *chart.Chart) ([]Drift, error) {
	names, err := render.ChartFiles()
	if err != nil {
		return nil, err
	}
	// By the name the file takes in the chart, not the one it has in the
	// embedded tree, so the report reads in the order someone would look.
	slices.SortFunc(names, func(a, b string) int {
		return strings.Compare(outputName(a), outputName(b))
	})

	data := DataFor(c)
	out := make([]Drift, 0, len(names))
	for _, name := range names {
		if _, excused := skeletonNotOwned[name]; excused {
			continue
		}
		want, err := render.ChartFile(name, data)
		if err != nil {
			return nil, err
		}
		path := outputName(name)
		d := Drift{Resource: path, Path: path, Want: want, Skeleton: true}
		compare(&d, filepath.Join(c.Dir, filepath.FromSlash(path)))
		out = append(out, d)
	}
	return out, nil
}

// DriftOfChart compares every catalog resource the chart carries.
//
// It answers one question — "would hck write this file differently today?" —
// and deliberately does not answer why. A template hck improved and a template
// somebody edited by hand are the same bytes from here.
func DriftOfChart(c *chart.Chart) ([]Drift, error) {
	resources, err := ChartResources(c)
	if err != nil {
		return nil, err
	}
	out := make([]Drift, 0, len(resources))
	for _, r := range resources {
		d, err := DriftOf(c, r)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	// Resources first, then the skeleton: the resources are what someone came
	// to look at, and the skeleton is what they did not know to.
	skel, err := skeletonDrift(c)
	if err != nil {
		return nil, err
	}
	return append(out, skel...), nil
}

// PlanRemove builds the plan for taking resources out of a chart.
//
// It removes template files and nothing else. values.yaml is not rewritten —
// that is the invariant the whole tool is built on — so the keys the removed
// resources introduced stay where they are, and the plan says which they were
// so that deleting them is a decision rather than a side effect.
func PlanRemove(c *chart.Chart, requested []string, force bool) (*Plan, error) {
	resources, err := resolve(requested)
	if err != nil {
		return nil, err
	}

	files, err := c.TemplateFiles()
	if err != nil {
		return nil, err
	}
	present := presentResources(files)

	plan := &Plan{ChartDir: c.Dir}
	going := map[string]bool{}
	for _, r := range resources {
		if !present[r.Name] {
			return nil, fmt.Errorf("%s does not carry %s (see: hck check --chart %s)", c.Meta.Name, r.Name, c.Dir)
		}
		going[r.Name] = true
	}

	for _, r := range resources {
		// A resource another one still needs is the one deletion that turns a
		// working chart into a broken one, and it is invisible until helm runs.
		if !force {
			if needed := neededBy(r.Name, present, going); len(needed) > 0 {
				return nil, fmt.Errorf("%s is still required by %s; remove them together, or pass --force to leave the chart referring to something it does not have",
					r.Name, strings.Join(needed, ", "))
			}
		}
		// Deleting somebody's work because a name was mistyped is not a thing
		// this should be able to do.
		d, err := DriftOf(c, r)
		if err != nil {
			return nil, err
		}
		if d.State == Edited && !force {
			return nil, fmt.Errorf("%s differs from what hck generates — it has been edited. Pass --force to delete it anyway", d.Path)
		}
		plan.Files = append(plan.Files, File{Path: d.Path, Action: Delete})
		plan.ValuesOrphaned = append(plan.ValuesOrphaned, orphanedKeys(r, present, going)...)
	}

	if c.HasSchema() && len(plan.ValuesOrphaned) > 0 {
		plan.Notes = append(plan.Notes, fmt.Sprintf(
			"values.yaml still declares %s, and %s still describes those keys. The next \"hck add\" regenerates the schema without them, which a strict schema then rejects — delete them from values.yaml before then",
			strings.Join(plan.ValuesOrphaned, ", "), SchemaFile))
	}
	return plan, nil
}

// neededBy lists the resources still in the chart that require name, ignoring
// the ones going away in the same command.
func neededBy(name string, present, going map[string]bool) []string {
	var out []string
	for _, r := range catalog.Resources() {
		if !present[r.Name] || going[r.Name] {
			continue
		}
		for _, req := range r.Requires {
			if req == name {
				out = append(out, r.Name)
			}
		}
	}
	return out
}

// orphanedKeys lists the values keys a removal leaves behind — the ones this
// resource introduced that nothing left in the chart also declares. A key two
// resources share, such as "persistence" on a StatefulSet and a PVC, is not
// orphaned while the other one is still there.
func orphanedKeys(r catalog.Resource, present, going map[string]bool) []string {
	claimed := map[string]bool{}
	for _, other := range catalog.Resources() {
		if other.Name == r.Name || !present[other.Name] || going[other.Name] {
			continue
		}
		for _, k := range other.ValuesKeys {
			claimed[k] = true
		}
	}
	var out []string
	for _, k := range r.ValuesKeys {
		if !claimed[k] {
			out = append(out, k)
		}
	}
	return out
}

// WriteTemplate replaces one file with what hck generates for it now.
//
// A missing skeleton file is put back — it carries no values, so restoring
// _helpers.tpl or .helmignore is a repair. A missing resource template is not:
// writing it would give the chart a resource whose values.yaml keys are not
// there, and adding a resource is "hck add".
func WriteTemplate(c *chart.Chart, d Drift) error {
	dest := filepath.Join(c.Dir, filepath.FromSlash(d.Path))
	// Checked against the disk rather than against d.State, which was read
	// before this call and can be older than it looks.
	if !d.Skeleton {
		if _, err := os.Stat(dest); errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%s is not in the chart — run: hck add %s", d.Path, d.Resource)
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(dest), err)
	}
	if err := os.WriteFile(dest, d.Want, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", d.Path, err)
	}
	return nil
}
