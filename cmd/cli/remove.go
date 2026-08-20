package cli

import (
	"github.com/somaz94/helm-chart-kit/internal/chart"
	"github.com/somaz94/helm-chart-kit/internal/scaffold"
	"github.com/spf13/cobra"
)

func newRemoveCmd() *cobra.Command {
	var opts struct {
		chartDir string
		dryRun   bool
		force    bool
	}

	cmd := &cobra.Command{
		Use:     "remove <resource>...",
		Aliases: []string{"rm"},
		Short:   "Remove resources from a chart",
		Long: `Delete the template files for one or more resources.

Templates only. values.yaml is never rewritten — the keys the resources
introduced stay exactly where they are, and the plan names them so that
deleting them is a decision rather than a side effect.

Two removals are refused unless you pass --force: one another resource in the
chart still requires, and one whose file has been edited. The second is the
one worth reading twice — a template that differs from what hck generates is
somebody's work, and this is not the command that should be able to delete it
by accident.`,
		Args: cobra.MinimumNArgs(1),
		Example: `  hck remove ingress
  hck remove hpa pdb --dry-run
  hck remove service --force
  hck rm servicemonitor --chart ./charts/payments-api`,
		// Only what the chart has: completing a name that is not there offers
		// the user an error, which is not what a completion is for.
		ValidArgsFunction: func(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			c, err := chartFromFlag(cmd)
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			resources, err := scaffold.ChartResources(c)
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			out := make([]string, 0, len(resources))
			for _, r := range resources {
				out = append(out, r.Name)
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := chart.Find(opts.chartDir)
			if err != nil {
				return err
			}
			c, err := chart.Load(dir)
			if err != nil {
				return err
			}

			plan, err := scaffold.PlanRemove(c, splitList(args), opts.force)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			p := newPainter(out)
			if opts.dryRun {
				fprintf(out, "%s %s (dry run)\n\n", p.bold("plan"), c.Meta.Name)
				printPlan(out, plan, true)
				return nil
			}

			if err := scaffold.Apply(plan); err != nil {
				return err
			}
			fprintf(out, "%s %s\n\n", p.bold("updated"), c.Meta.Name)
			printPlan(out, plan, false)
			fprintf(out, "\nNext:\n  hck check --chart %s\n", shellQuote(c.Dir))
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.chartDir, "chart", ".", "chart directory; parent directories are searched for Chart.yaml")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "print what would be deleted and exit")
	cmd.Flags().BoolVar(&opts.force, "force", false, "delete an edited template, and one another resource still requires")
	return cmd
}
