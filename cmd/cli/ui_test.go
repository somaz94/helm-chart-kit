package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// A non-terminal writer must never get escapes, or piped output and CI logs
// fill up with them.
func TestColoredIsOffForNonTerminals(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	if colored(&bytes.Buffer{}) {
		t.Error("a bytes.Buffer is not a terminal")
	}
	if colored(os.Stdout) && os.Getenv("CI") != "" {
		t.Error("CI stdout should not be treated as a terminal")
	}
}

func TestColoredHonoursNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if colored(os.Stdout) {
		t.Error("NO_COLOR was ignored")
	}
}

func TestPainterOn(t *testing.T) {
	p := painter{on: true}
	for name, got := range map[string]string{
		"green":  p.green("x"),
		"yellow": p.yellow("x"),
		"red":    p.red("x"),
		"dim":    p.dim("x"),
		"bold":   p.bold("x"),
	} {
		if !strings.HasPrefix(got, "\033[") || !strings.HasSuffix(got, "\033[0m") {
			t.Errorf("%s did not wrap: %q", name, got)
		}
	}
}

func TestPainterOff(t *testing.T) {
	p := painter{on: false}
	if p.red("x") != "x" || p.bold("x") != "x" {
		t.Error("colour was emitted with painting off")
	}
}

func TestDedupe(t *testing.T) {
	got := dedupe([]string{"a", "b", "a", "c", "b"})
	if strings.Join(got, "|") != "a|b|c" {
		t.Fatalf("got %v", got)
	}
}
