package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/somaz94/helm-chart-kit/internal/catalog"
	"github.com/somaz94/helm-chart-kit/internal/chart"
	"github.com/somaz94/helm-chart-kit/internal/scaffold"
	"github.com/spf13/cobra"
)

func newEnvCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Manage the chart's environment values overlays",
		Long: `Environment overlays carry how hard the chart is being asked to work:
one replica and loose limits while someone develops against it, three replicas
and a disruption budget once it carries traffic.

Orthogonal to platform. A chart is installed somewhere and at some size, and
the two stack — environment last, so its replica count wins:

    helm install app . -f values-aws.yaml -f values-prod.yaml`,
	}
	cmd.AddCommand(newEnvListCmd(), newEnvAddCmd())
	return cmd
}

func newEnvListCmd() *cobra.Command {
	var chartDir string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the known environments, and which the chart already has",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p := newPainter(out)

			// Outside a chart this is still useful as a plain catalog listing.
			have := map[string]bool{}
			if dir, err := chart.Find(chartDir); err == nil {
				if c, err := chart.Load(dir); err == nil {
					for _, pf := range scaffold.ChartEnvironments(c) {
						have[pf.Name] = true
					}
				}
			}

			fprintf(out, "%s\n", p.bold("ENVIRONMENTS"))
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			for _, pf := range catalog.Environments() {
				mark := p.dim("-")
				if have[pf.Name] {
					mark = p.green("+")
				}
				fmt.Fprintf(w, "  %s %s\t%s\n", mark, pf.Name, pf.Summary)
			}
			_ = w.Flush()
			fprintf(out, "\n  %s already in this chart\n", p.green("+"))
			return nil
		},
	}
	cmd.Flags().StringVar(&chartDir, "chart", ".", "chart directory")
	return cmd
}

func newEnvAddCmd() *cobra.Command {
	var opts struct {
		chartDir string
		dryRun   bool
		force    bool
	}
	cmd := &cobra.Command{
		Use:   "add <environment>...",
		Short: "Write environment values overlays into an existing chart",
		Args:  cobra.MinimumNArgs(1),
		Example: `  hck env add aws
  hck env add gcp azure
  hck env add onprem --dry-run`,
		ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return catalog.EnvironmentNames(), cobra.ShellCompDirectiveNoFileComp
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
			resources, err := scaffold.ChartResources(c)
			if err != nil {
				return err
			}
			if len(resources) == 0 {
				return fmt.Errorf("%s carries no resource this catalog knows, so there is nothing to overlay", c.Meta.Name)
			}

			out := cmd.OutOrStdout()
			p := newPainter(out)
			data := scaffold.DataFor(c)

			wrote := 0
			for _, name := range splitList(args) {
				pf, ok := catalog.LookupEnvironment(name)
				if !ok {
					return fmt.Errorf("unknown environment %q (known: %s)", name, strings.Join(catalog.EnvironmentNames(), ", "))
				}
				dest := filepath.Join(c.Dir, pf.ValuesFile())
				if _, err := os.Stat(dest); err == nil && !opts.force {
					fprintf(out, "  %s %s (already exists; pass --force to rewrite)\n", p.dim("."), pf.ValuesFile())
					continue
				}
				overlay, ok, err := scaffold.BuildEnvironmentValues(data, resources, pf)
				if err != nil {
					return err
				}
				if !ok {
					fprintf(out, "  %s %s — nothing in this chart differs there\n", p.dim("."), pf.Name)
					continue
				}
				if opts.dryRun {
					fprintf(out, "  %s %s\n", p.green("+"), pf.ValuesFile())
					continue
				}
				if err := os.WriteFile(dest, overlay, 0o644); err != nil {
					return fmt.Errorf("write %s: %w", pf.ValuesFile(), err)
				}
				fprintf(out, "  %s %s\n", p.green("+"), pf.ValuesFile())
				wrote++
			}
			if wrote > 0 {
				fprintf(out, "\nNext:\n  hck check --chart %s --env %s\n", c.Dir, splitList(args)[0])
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.chartDir, "chart", ".", "chart directory; parent directories are searched for Chart.yaml")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "print what would be written and exit")
	cmd.Flags().BoolVar(&opts.force, "force", false, "rewrite an overlay that already exists")
	return cmd
}
