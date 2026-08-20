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

// HasResource reports whether the embedded set carries templates for a name.
// It backs the test that keeps the catalog and the template tree in step.
func HasResource(resource string) bool {
	for _, f := range []string{"template.yaml.tmpl", "values.yaml.tmpl"} {
		if _, err := files.Open(path.Join("templates/resources", resource, f)); err != nil {
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
