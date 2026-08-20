package cli

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// colored reports whether to emit ANSI escapes: only to a real terminal, and
// never when NO_COLOR is set.
func colored(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

type painter struct{ on bool }

func newPainter(w io.Writer) painter { return painter{on: colored(w)} }

func (p painter) wrap(code, s string) string {
	if !p.on {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}

func (p painter) green(s string) string  { return p.wrap("32", s) }
func (p painter) yellow(s string) string { return p.wrap("33", s) }
func (p painter) red(s string) string    { return p.wrap("31", s) }
func (p painter) dim(s string) string    { return p.wrap("2", s) }
func (p painter) bold(s string) string   { return p.wrap("1", s) }

func fprintf(w io.Writer, format string, a ...any) {
	fmt.Fprintf(w, format, a...)
}
