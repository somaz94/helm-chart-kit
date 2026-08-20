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

func newPlatformCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "platform",
		Short: "Manage the chart's platform values overlays",
		Long: `Platform overlays carry the values that differ between one target and
another — the IAM annotation on a ServiceAccount, the ingress class, the
storage class — and nothing else.

They are overlays, not replacements: Helm reads values.yaml first and always,
so an overlay only has to say what is different.

    helm install app . -f values-aws.yaml`,
	}
	cmd.AddCommand(newPlatformListCmd(), newPlatformAddCmd())
	return cmd
}

func newPlatformListCmd() *cobra.Command {
	var chartDir string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the known platforms, and which the chart already has",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p := newPainter(out)

			// Outside a chart this is still useful as a plain catalog listing.
			have := map[string]bool{}
			if dir, err := chart.Find(chartDir); err == nil {
				if c, err := chart.Load(dir); err == nil {
					for _, pf := range scaffold.ChartPlatforms(c) {
						have[pf.Name] = true
					}
				}
			}

			fprintf(out, "%s\n", p.bold("PLATFORMS"))
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			for _, pf := range catalog.Platforms() {
				mark := p.dim("-")
				if have[pf.Name] {
					mark = p.green("+")
				}
				fmt.Fprintf(w, "  %s %s\t%s\n", mark, pf.Name, pf.Summary)
				fmt.Fprintf(w, "  \t%s\n", p.dim("needs: "+strings.Join(pf.Needs, ", ")))
			}
			_ = w.Flush()
			fprintf(out, "\n  %s already in this chart\n", p.green("+"))
			return nil
		},
	}
	cmd.Flags().StringVar(&chartDir, "chart", ".", "chart directory")
	return cmd
}

func newPlatformAddCmd() *cobra.Command {
	var opts struct {
		chartDir string
		dryRun   bool
		force    bool
	}
	cmd := &cobra.Command{
		Use:   "add <platform>...",
		Short: "Write platform values overlays into an existing chart",
		Args:  cobra.MinimumNArgs(1),
		Example: `  hck platform add aws
  hck platform add gcp azure
  hck platform add onprem --dry-run`,
		ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return catalog.PlatformNames(), cobra.ShellCompDirectiveNoFileComp
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
				pf, ok := catalog.LookupPlatform(name)
				if !ok {
					return fmt.Errorf("unknown platform %q (known: %s)", name, strings.Join(catalog.PlatformNames(), ", "))
				}
				dest := filepath.Join(c.Dir, pf.ValuesFile())
				if _, err := os.Stat(dest); err == nil && !opts.force {
					fprintf(out, "  %s %s (already exists; pass --force to rewrite)\n", p.dim("."), pf.ValuesFile())
					continue
				}
				overlay, ok, err := scaffold.BuildPlatformValues(data, resources, pf)
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
				fprintf(out, "        %s\n", p.dim("needs: "+strings.Join(pf.Needs, ", ")))
				wrote++
			}
			if wrote > 0 {
				fprintf(out, "\nNext:\n  hck check --chart %s --platform %s\n", c.Dir, splitList(args)[0])
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.chartDir, "chart", ".", "chart directory; parent directories are searched for Chart.yaml")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "print what would be written and exit")
	cmd.Flags().BoolVar(&opts.force, "force", false, "rewrite an overlay that already exists")
	return cmd
}
