package cli

import (
	"io"
	"slices"
	"strings"

	"github.com/somaz94/helm-chart-kit/internal/scaffold"
)

// printPlan renders a plan as the list of files it touches plus the values
// keys it appends. Skips are printed too — knowing a resource was already
// there is the answer to "why did nothing happen".
func printPlan(w io.Writer, p *scaffold.Plan, dryRun bool) {
	c := newPainter(w)

	verb := "wrote"
	if dryRun {
		verb = "would write"
	}

	files := make([]scaffold.File, len(p.Files))
	copy(files, p.Files)
	slices.SortFunc(files, func(a, b scaffold.File) int { return strings.Compare(a.Path, b.Path) })

	for _, f := range files {
		switch f.Action {
		case scaffold.Create:
			fprintf(w, "  %s  %s\n", c.green("+"), f.Path)
		case scaffold.Update:
			fprintf(w, "  %s  %s\n", c.yellow("~"), f.Path)
		case scaffold.Skip:
			fprintf(w, "  %s  %s %s\n", c.dim("."), c.dim(f.Path), c.dim("("+f.Reason+")"))
		case scaffold.Delete:
			fprintf(w, "  %s  %s\n", c.red("-"), f.Path)
		}
	}

	if len(p.ValuesAdded) > 0 {
		fprintf(w, "\n  values.yaml %s: %s\n", verb, strings.Join(p.ValuesAdded, ", "))
	}
	if len(p.ValuesSkipped) > 0 {
		fprintf(w, "  values.yaml kept as-is: %s\n", c.dim(strings.Join(p.ValuesSkipped, ", ")))
	}
	// Named rather than removed: values.yaml is never rewritten, so these keys
	// are still in the file and deleting them stays somebody's decision.
	if len(p.ValuesOrphaned) > 0 {
		fprintf(w, "\n  values.yaml still declares (now unused): %s\n", strings.Join(dedupe(p.ValuesOrphaned), ", "))
	}

	if len(p.Notes) > 0 {
		fprintf(w, "\n")
		for _, n := range dedupe(p.Notes) {
			fprintf(w, "  %s %s\n", c.yellow("note:"), n)
		}
	}
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
