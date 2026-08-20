package cli

import (
	"fmt"
	"slices"
	"text/tabwriter"

	"github.com/somaz94/helm-chart-kit/internal/chart"
	"github.com/somaz94/helm-chart-kit/internal/scaffold"
	"github.com/spf13/cobra"
)

func newSyncCmd() *cobra.Command {
	var opts struct {
		chartDir string
		check    bool
		write    bool
		all      bool
	}

	cmd := &cobra.Command{
		Use:   "sync [resource...]",
		Short: "Compare a chart's templates against what hck generates now",
		Long: `Report which of a chart's generated templates differ from what this hck
would write for them today.

What it cannot tell you is why. A template you edited and a template hck
improved in a later version look identical from here — both are simply not
what hck generates now. That is why the default is a report and why --write
takes names: the answer to "should this file change?" belongs to whoever
edited it.

    hck sync                  # what differs
    hck sync --check          # the same, and non-zero if anything does
    hck sync --write ingress  # take hck's version of these
    hck sync --write --all    # take hck's version of everything

--write overwrites. Read the diff first; a chart under version control makes
that easy and one without it makes this unrecoverable.`,
		Args: cobra.ArbitraryArgs,
		Example: `  hck sync
  hck sync --check
  hck sync --write deployment service
  hck sync --write --all --chart ./charts/payments-api`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.check && opts.write {
				return fmt.Errorf("--check reports and --write changes; pick one")
			}
			names := splitList(args)
			if opts.write && len(names) == 0 && !opts.all {
				return fmt.Errorf("--write overwrites, so name the resources to take, or pass --all")
			}
			if opts.all && len(names) > 0 {
				return fmt.Errorf("--all takes every drifted resource; naming some as well says two different things")
			}

			dir, err := chart.Find(opts.chartDir)
			if err != nil {
				return err
			}
			c, err := chart.Load(dir)
			if err != nil {
				return err
			}
			drifts, err := scaffold.DriftOfChart(c)
			if err != nil {
				return err
			}
			if len(drifts) == 0 {
				return fmt.Errorf("%s carries no resource this catalog knows, so there is nothing to compare", c.Meta.Name)
			}

			byName := map[string]scaffold.Drift{}
			for _, d := range drifts {
				byName[d.Resource] = d
			}
			for _, name := range names {
				if _, ok := byName[name]; !ok {
					return fmt.Errorf("%s does not carry %s", c.Meta.Name, name)
				}
			}

			out := cmd.OutOrStdout()
			p := newPainter(out)

			if opts.write {
				wrote := 0
				for _, d := range drifts {
					if !opts.all && !slices.Contains(names, d.Resource) {
						continue
					}
					if d.State == scaffold.Current {
						continue
					}
					if d.State == scaffold.Unreadable {
						return d.Err
					}
					if err := scaffold.WriteTemplate(c, d); err != nil {
						return err
					}
					fprintf(out, "  %s  %s\n", p.yellow("~"), d.Path)
					wrote++
				}
				if wrote == 0 {
					fprintf(out, "%s nothing to take — every named template is already what hck generates\n", p.dim("·"))
					return nil
				}
				fprintf(out, "\n%s %s\n", p.bold("updated"), c.Meta.Name)
				fprintf(out, "\nNext:\n  hck check --chart %s\n", shellQuote(c.Dir))
				return nil
			}

			fprintf(out, "%s %s\n\n", p.bold("sync"), c.Meta.Name)
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			for _, d := range drifts {
				mark, note := p.green("="), p.dim("current")
				switch d.State {
				case scaffold.Edited:
					mark, note = p.yellow("~"), p.yellow("differs from what hck generates")
				case scaffold.Unreadable:
					mark, note = p.red("!"), p.red(d.Err.Error())
				}
				fmt.Fprintf(w, "  %s %s\t%s\n", mark, d.Path, note)
			}
			_ = w.Flush()

			if !scaffold.AnyDrifted(drifts) {
				fprintf(out, "\n  %s every template is what hck generates\n", p.green("ok"))
				return nil
			}
			fprintf(out, "\n  %s\n", p.dim("hck cannot tell a local edit from an hck template that moved on."))
			fprintf(out, "  %s\n", p.dim("Read the difference before taking hck's version:"))
			fprintf(out, "    hck sync --write <resource>\n")
			if opts.check {
				return fmt.Errorf("chart differs from what hck generates")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.chartDir, "chart", ".", "chart directory; parent directories are searched for Chart.yaml")
	cmd.Flags().BoolVar(&opts.check, "check", false, "exit non-zero when anything differs")
	cmd.Flags().BoolVar(&opts.write, "write", false, "overwrite the named templates with what hck generates")
	cmd.Flags().BoolVar(&opts.all, "all", false, "with --write, take every drifted template")
	return cmd
}
