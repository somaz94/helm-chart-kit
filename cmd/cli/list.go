package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/somaz94/helm-chart-kit/internal/catalog"
	"github.com/somaz94/helm-chart-kit/internal/check"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "list [presets|resources|rules]",
		Short:     "List the presets, resources and check rules hck knows",
		Args:      cobra.OnlyValidArgs,
		ValidArgs: []string{"presets", "resources", "rules"},
		Example: `  hck list
  hck list presets
  hck list resources
  hck list rules`,
		RunE: func(cmd *cobra.Command, args []string) error {
			what := "all"
			if len(args) == 1 {
				what = args[0]
			}
			out := cmd.OutOrStdout()
			p := newPainter(out)

			if what == "all" || what == "presets" {
				fprintf(out, "%s\n", p.bold("PRESETS"))
				w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				for _, ps := range catalog.Presets() {
					fmt.Fprintf(w, "  %s\t%s\n", ps.Name, ps.Summary)
					fmt.Fprintf(w, "  \t%s\n", p.dim(strings.Join(ps.Resources, " ")))
				}
				_ = w.Flush()
			}

			if what == "all" {
				fprintf(out, "\n")
			}

			if what == "all" || what == "resources" {
				fprintf(out, "%s\n", p.bold("RESOURCES"))
				// Grouped rather than alphabetical: the names are Kubernetes
				// kinds, so a flat list answers "what exists" and leaves
				// "which of these do I want" to somebody who already knows.
				// The blank line before each group header ends the
				// tabwriter's current cell block, so every group sizes its
				// own columns. That is the readable outcome and not an
				// accident: @workload would otherwise carry a column wide
				// enough for gateway.networking.k8s.io/v1beta1.
				w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				for _, g := range catalog.Groups() {
					fmt.Fprintf(w, "\n  %s\t\t%s\n", p.bold("@"+string(g.Name)), p.dim(g.Summary))
					for _, r := range catalog.ResourcesInGroup(g.Name) {
						mark := ""
						if r.Optional {
							mark = p.yellow(" [crd]")
						}
						// The platform marker comes after [crd] because it is
						// the harder constraint: a CRD can be installed, a
						// GKE-only kind cannot be made to exist on EKS.
						if r.Platform != "" {
							mark += p.yellow(" [" + r.Platform + "]")
						}
						fmt.Fprintf(w, "    %s\t%s\t%s%s\n", r.Name, p.dim(r.APIVersion), r.Summary, mark)
					}
				}
				_ = w.Flush()
				fprintf(out, "\n  %s needs a CRD or feature the cluster may not have\n", p.yellow("[crd]"))
				fprintf(out, "  %s adds the whole group: hck add @observability\n", p.bold("@name"))
				fprintf(out, "  %s exists on that platform only, and is never pulled in by a group\n",
					p.yellow("[platform]"))
			}

			if what == "all" {
				fprintf(out, "\n")
			}

			if what == "all" || what == "rules" {
				fprintf(out, "%s\n", p.bold("CHECK RULES"))
				w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				for _, r := range check.Rules() {
					sev := p.yellow(string(r.Severity))
					if r.Severity == check.Error {
						sev = p.red(string(r.Severity))
					}
					fmt.Fprintf(w, "  %s\t%s\t%s\n", r.ID, sev, r.Summary)
				}
				_ = w.Flush()
				fprintf(out, "\n  Turn one off or change its severity in the chart's %s:\n", p.dim(check.ConfigFile))
				fprintf(out, "  %s\n", p.dim("rules:"))
				fprintf(out, "  %s\n", p.dim(`  "`+check.WildcardRule+`": off    # every rule that can be`))
				fprintf(out, "  %s\n", p.dim("  HCK025: off"))
				fprintf(out, "\n  Or for one run, without editing the chart: %s\n", p.dim("hck check --off HCK025"))
			}
			return nil
		},
	}
}

// groupArgs is the group names as completion candidates, "@" included, so
// that tab-completion offers them beside the resource names.
func groupArgs() []string {
	names := catalog.GroupNames()
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, "@"+n)
	}
	return out
}
