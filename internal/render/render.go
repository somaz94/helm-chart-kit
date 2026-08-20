// Package render turns the embedded template set into chart files.
//
// The templates emit Helm templates, which are themselves full of {{ }}, so
// this layer uses [[ ]] as its own delimiters. Helm's braces then pass through
// untouched and the templates stay readable as the Helm files they will
// become, instead of a thicket of escapes.
package render

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"text/template"
)

// all: is required, not decoration: a plain "embed templates" silently drops
// every path segment starting with "_" or "." — which is exactly
// templates/chart/templates/_helpers.tpl, the one file every other template
// calls into.
//
//go:embed all:templates
var files embed.FS

const (
	leftDelim  = "[["
	rightDelim = "]]"
)

// Data is the substitution context available to every template.
type Data struct {
	// ChartName is the chart directory and Chart.yaml name. It also prefixes
	// the named templates in _helpers.tpl.
	ChartName string
	// Description is the Chart.yaml description.
	Description string
	// Version is the chart version.
	Version string
	// AppVersion is the version of the application the chart deploys.
	AppVersion string
	// Preset is the preset the chart was created from, recorded in Chart.yaml
	// annotations so a later "hck add" can report what the chart started as.
	Preset string
	// Resources are the catalog names included in the chart.
	Resources []string
	// WorkloadKind is the Kubernetes kind of the chart's primary workload —
	// Deployment, StatefulSet, DaemonSet or CronJob — and "" for a chart that
	// carries none.
	//
	// A scaler has to name its target, and naming the wrong one is invisible:
	// the controller reports it only in its own status, and the chart renders,
	// installs and does nothing.
	WorkloadKind string
}

// ChartFile renders one file from the chart skeleton, e.g. "Chart.yaml" or
// "templates/_helpers.tpl".
func ChartFile(name string, d Data) ([]byte, error) {
	return renderPath(path.Join("templates/chart", name+".tmpl"), d)
}

// ChartFiles lists the chart-skeleton files, with the ".tmpl" suffix and the
// "templates/chart/" prefix stripped, so the result is the path each one takes
// inside the generated chart.
func ChartFiles() ([]string, error) {
	root := "templates/chart"
	var out []string
	err := fs.WalkDir(files, root, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			return nil
		}
		// all: also sweeps in whatever the filesystem left lying around.
		if !strings.HasSuffix(p, ".tmpl") {
			return nil
		}
		rel := strings.TrimPrefix(p, root+"/")
		out = append(out, strings.TrimSuffix(rel, ".tmpl"))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ResourceTemplate renders the Helm template for one catalog resource.
func ResourceTemplate(resource string, d Data) ([]byte, error) {
	return renderPath(path.Join("templates/resources", resource, "template.yaml.tmpl"), d)
}

// ResourceValues renders the values.yaml fragment for one catalog resource.
func ResourceValues(resource string, d Data) ([]byte, error) {
	return renderPath(path.Join("templates/resources", resource, "values.yaml.tmpl"), d)
}

// ResourceSchema renders the values.schema.json fragment for one catalog
// resource. The fragment is a JSON object mapping each top-level values key
// the resource owns to the schema for it; internal/schema assembles the
// fragments into the chart's values.schema.json.
func ResourceSchema(resource string, d Data) ([]byte, error) {
	return renderPath(path.Join("templates/resources", resource, "schema.json.tmpl"), d)
}

// ResourceOverlayValues renders one resource's values overlay for a suffix —
// a platform like "aws" or an environment like "prod". Most resources have
// none for a given suffix — a ConfigMap looks the same everywhere — and the
// absence is reported rather than treated as an error.
func ResourceOverlayValues(resource, suffix string, d Data) ([]byte, bool, error) {
	p := path.Join("templates/resources", resource, "values-"+suffix+".yaml.tmpl")
	if _, err := fs.Stat(files, p); err != nil {
		return nil, false, nil
	}
	out, err := renderPath(p, d)
	return out, err == nil, err
}

// HasOverlayValues reports whether a resource differs under a suffix. It backs
// the test that keeps the overlay tree and the catalog in step.
func HasOverlayValues(resource, suffix string) bool {
	_, err := fs.Stat(files, path.Join("templates/resources", resource, "values-"+suffix+".yaml.tmpl"))
	return err == nil
}

// OverlaySuffixes lists the suffixes present in the embedded tree, so a
// fragment named for a platform or environment nobody declared is caught.
func OverlaySuffixes() ([]string, error) {
	seen := map[string]bool{}
	err := fs.WalkDir(files, "templates/resources", func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		name := e.Name()
		rest, ok := strings.CutPrefix(name, "values-")
		if !ok {
			return nil
		}
		if suffix, ok := strings.CutSuffix(rest, ".yaml.tmpl"); ok {
			seen[suffix] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	slices.Sort(out)
	return out, nil
}

// BaseSchema renders the schema fragment for the keys every chart carries,
// independent of which resources it has.
func BaseSchema(d Data) ([]byte, error) {
	return renderPath("templates/schema/base.json.tmpl", d)
}

// HasResource reports whether the embedded set carries templates for a name.
// It backs the test that keeps the catalog and the template tree in step.
func HasResource(resource string) bool {
	for _, f := range []string{"template.yaml.tmpl", "values.yaml.tmpl", "schema.json.tmpl"} {
		if _, err := fs.Stat(files, path.Join("templates/resources", resource, f)); err != nil {
			return false
		}
	}
	return true
}

func renderPath(p string, d Data) ([]byte, error) {
	src, err := files.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("no embedded template at %s: %w", p, err)
	}
	t, err := template.New(path.Base(p)).
		Delims(leftDelim, rightDelim).
		Option("missingkey=error").
		Parse(string(src))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, d); err != nil {
		return nil, fmt.Errorf("render %s: %w", p, err)
	}
	return buf.Bytes(), nil
}
