package cli

import (
	"github.com/somaz94/helm-chart-kit/internal/catalog"
	"github.com/somaz94/helm-chart-kit/internal/chart"
	"github.com/somaz94/helm-chart-kit/internal/scaffold"
	"github.com/spf13/cobra"
)

func newAddCmd() *cobra.Command {
	var opts struct {
		chartDir string
		dryRun   bool
		force    bool
	}

	cmd := &cobra.Command{
		Use:   "add <resource>...",
		Short: "Add resources to an existing chart",
		Long: `Add one or more resource templates to a chart that already exists.

Files are never overwritten and values.yaml is never rewritten: a key already
present is reported and left exactly as it is, comments and ordering intact.
New keys are appended with the documentation that belongs to them.`,
		Args: cobra.MinimumNArgs(1),
		Example: `  hck add servicemonitor
  hck add pdb networkpolicy
  hck add httproute --dry-run
  hck add externalsecret --chart ./charts/payments-api`,
		ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return catalog.ResourceNames(), cobra.ShellCompDirectiveNoFileComp
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

			plan, err := scaffold.PlanAdd(c, splitList(args), opts.force)
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
			if !plan.Changed() {
				fprintf(out, "%s nothing to do — every requested resource is already in %s\n", p.dim("·"), c.Meta.Name)
				printPlan(out, plan, false)
				return nil
			}
			fprintf(out, "%s %s\n\n", p.bold("updated"), c.Meta.Name)
			printPlan(out, plan, false)
			fprintf(out, "\nNext:\n  hck check --chart %s\n", c.Dir)
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.chartDir, "chart", ".", "chart directory; parent directories are searched for Chart.yaml")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "print what would be written and exit")
	cmd.Flags().BoolVar(&opts.force, "force", false, "overwrite existing template files and allow a second workload")
	return cmd
}
