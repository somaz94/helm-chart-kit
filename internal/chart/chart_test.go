package chart

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeChart(t *testing.T, meta string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "templates", "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

const validMeta = `apiVersion: v2
name: demo
description: a demo chart
version: 0.1.0
appVersion: "1.0.0"
annotations:
  helm-chart-kit/preset: "web"
`

func TestLoad(t *testing.T) {
	c, err := Load(writeChart(t, validMeta))
	if err != nil {
		t.Fatal(err)
	}
	if c.Meta.Name != "demo" || c.Meta.Version != "0.1.0" {
		t.Fatalf("unexpected metadata: %+v", c.Meta)
	}
	if c.Meta.Annotations["helm-chart-kit/preset"] != "web" {
		t.Error("preset annotation not read")
	}
}

func TestLoadRejectsBadCharts(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil {
		t.Error("want an error when Chart.yaml is absent")
	}
	if _, err := Load(writeChart(t, "name: [\n")); err == nil {
		t.Error("want an error for malformed YAML")
	}
	if _, err := Load(writeChart(t, "apiVersion: v2\nversion: 0.1.0\n")); err == nil {
		t.Error("want an error when name is missing")
	}
}

// Find walks up so the commands work from anywhere inside a chart.
func TestFindWalksUp(t *testing.T) {
	dir := writeChart(t, validMeta)
	deep := filepath.Join(dir, "templates", "tests")
	got, err := Find(deep)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(dir)
	gotResolved, _ := filepath.EvalSymlinks(got)
	if gotResolved != want {
		t.Fatalf("Find(%s) = %s, want %s", deep, gotResolved, want)
	}
}

func TestFindReportsNotFound(t *testing.T) {
	// t.TempDir() has no Chart.yaml anywhere above it inside the temp root,
	// but the walk ends at "/" regardless, so assert on the error only.
	_, err := Find(t.TempDir())
	if err == nil {
		t.Skip("a Chart.yaml exists somewhere above the temp dir on this machine")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestTemplateFilesExcludesPartialsAndNotes(t *testing.T) {
	dir := writeChart(t, validMeta)
	for _, f := range []string{
		"templates/deployment.yaml",
		"templates/_helpers.tpl",
		"templates/NOTES.txt",
		"templates/tests/test-connection.yaml",
	} {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(f)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.TemplateFiles()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"deployment.yaml", "tests/test-connection.yaml"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestHasTemplate(t *testing.T) {
	dir := writeChart(t, validMeta)
	if err := os.WriteFile(filepath.Join(dir, "templates", "tests", "test-connection.yaml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _ := Load(dir)
	if !c.HasTemplate("tests/test-connection.yaml") {
		t.Error("nested template not detected")
	}
	if c.HasTemplate("service.yaml") {
		t.Error("absent template reported as present")
	}
}

// A chart with no values.yaml is legal, so reading it is not an error.
func TestValuesMissingIsNotAnError(t *testing.T) {
	c, _ := Load(writeChart(t, validMeta))
	got, err := c.Values()
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("got %q, want nil", got)
	}
}

func TestValuesRead(t *testing.T) {
	dir := writeChart(t, validMeta)
	if err := os.WriteFile(filepath.Join(dir, "values.yaml"), []byte("a: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _ := Load(dir)
	got, err := c.Values()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "a: 1\n" {
		t.Fatalf("got %q", got)
	}
}

// A chart with no templates directory at all is unusual but legal, and must
// not make the walk error out.
func TestTemplateFilesWithoutTemplatesDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte(validMeta), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.TemplateFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want none", got)
	}
}

func TestValuesReportsReadFailures(t *testing.T) {
	dir := writeChart(t, validMeta)
	// A directory where values.yaml should be: the read fails with something
	// other than "not exist", which must not be swallowed.
	if err := os.Mkdir(filepath.Join(dir, "values.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	c, _ := Load(dir)
	if _, err := c.Values(); err == nil {
		t.Fatal("want an error, got nil")
	}
}

func TestTemplatePath(t *testing.T) {
	c := &Chart{Dir: "/tmp/demo"}
	if got := c.TemplatePath("tests/test-connection.yaml"); got != filepath.Join("/tmp/demo", "templates", "tests", "test-connection.yaml") {
		t.Fatalf("got %q", got)
	}
}
