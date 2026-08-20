// Package chart locates and inspects a Helm chart directory on disk.
package chart

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// ErrNotFound is returned when no Chart.yaml is found.
var ErrNotFound = errors.New("no Chart.yaml found")

// Metadata is the subset of Chart.yaml this tool reads.
type Metadata struct {
	APIVersion  string            `yaml:"apiVersion"`
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Type        string            `yaml:"type"`
	Version     string            `yaml:"version"`
	AppVersion  string            `yaml:"appVersion"`
	Annotations map[string]string `yaml:"annotations"`
}

// Chart is a chart directory that exists on disk.
type Chart struct {
	// Dir is the chart root — the directory holding Chart.yaml.
	Dir  string
	Meta Metadata
}

// Find walks up from start looking for a directory with a Chart.yaml, so the
// commands work from anywhere inside a chart the way git does.
func Find(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "Chart.yaml")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNotFound
		}
		dir = parent
	}
}

// Load reads the chart rooted at dir.
func Load(dir string) (*Chart, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "Chart.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read Chart.yaml: %w", err)
	}
	var meta Metadata
	if err := yaml.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("parse Chart.yaml: %w", err)
	}
	if meta.Name == "" {
		return nil, fmt.Errorf("%s/Chart.yaml has no name", dir)
	}
	return &Chart{Dir: dir, Meta: meta}, nil
}

// ValuesPath is the chart's values.yaml.
func (c *Chart) ValuesPath() string { return filepath.Join(c.Dir, "values.yaml") }

// TemplatePath resolves a path relative to the chart's templates directory.
func (c *Chart) TemplatePath(rel string) string {
	return filepath.Join(c.Dir, "templates", filepath.FromSlash(rel))
}

// HasTemplate reports whether a template file already exists.
func (c *Chart) HasTemplate(rel string) bool {
	_, err := os.Stat(c.TemplatePath(rel))
	return err == nil
}

// TemplateFiles lists the chart's template files, as slash-separated paths
// relative to templates/. Partials and NOTES.txt are excluded: neither emits a
// Kubernetes object, so neither counts as a resource the chart carries.
func (c *Chart) TemplateFiles() ([]string, error) {
	root := filepath.Join(c.Dir, "templates")
	var out []string
	err := filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if e.IsDir() {
			return nil
		}
		name := e.Name()
		if name == "NOTES.txt" || filepath.Ext(name) == ".tpl" {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// Values reads values.yaml. A chart with no values.yaml yields no bytes and
// no error — an empty one is legal.
func (c *Chart) Values() ([]byte, error) {
	raw, err := os.ReadFile(c.ValuesPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read values.yaml: %w", err)
	}
	return raw, nil
}
