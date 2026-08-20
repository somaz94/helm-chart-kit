package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/somaz94/helm-chart-kit/internal/catalog"
	"github.com/somaz94/helm-chart-kit/internal/chart"
	"github.com/somaz94/helm-chart-kit/internal/scaffold"
	"github.com/spf13/cobra"
)

// overlayAxis is everything "hck platform" and "hck env" disagree about, which
// is the prose in their help. Listing, lookup and building are one mechanism
// keyed on catalog.Axis, and this type carries no behaviour of its own.
//
// The two commands were once a copy of each other, and the copy immediately
// drifted: every example under "hck env add --help" named a platform and
// errored out. Keeping the examples next to the axis they belong to is what
// makes that visible.
type overlayAxis struct {
	axis     catalog.Axis
	heading  string // "PLATFORMS" or "ENVIRONMENTS"
	long     string
	examples string
}

// use is the subcommand word — "platform", "env".
func (a overlayAxis) use() string { return a.axis.Command() }

// noun is the word messages use — "platform", "environment".
func (a overlayAxis) noun() string { return string(a.axis) }

func (a overlayAxis) names() []string { return catalog.OverlayNames(a.axis) }

var platformAxis = overlayAxis{
	axis:    catalog.PlatformAxis,
	heading: "PLATFORMS",
	long: `Platform overlays carry the values that differ between one target and
another — the IAM annotation on a ServiceAccount, the ingress class, the
storage class — and nothing else.

They are overlays, not replacements: Helm reads values.yaml first and always,
so an overlay only has to say what is different.

    helm install app . -f values-aws.yaml`,
	examples: `  hck platform add aws
  hck platform add gcp azure
  hck platform add onprem --dry-run`,
}

var envAxis = overlayAxis{
	axis:    catalog.EnvironmentAxis,
	heading: "ENVIRONMENTS",
	long: `Environment overlays carry how hard the chart is being asked to work:
one replica and loose limits while someone develops against it, three replicas
and a disruption budget once it carries traffic.

Orthogonal to platform. A chart is installed somewhere and at some size, and
the two stack — environment last, so its replica count wins:

    helm install app . -f values-aws.yaml -f values-prod.yaml`,
	examples: `  hck env add prod
  hck env add dev staging
  hck env add prod --dry-run`,
}

func newPlatformCmd() *cobra.Command { return newOverlayCmd(platformAxis) }
func newEnvCmd() *cobra.Command      { return newOverlayCmd(envAxis) }

func newOverlayCmd(a overlayAxis) *cobra.Command {
	cmd := &cobra.Command{
		Use:   a.use(),
		Short: fmt.Sprintf("Manage the chart's %s values overlays", a.noun()),
		Long:  a.long,
	}
	cmd.AddCommand(newOverlayListCmd(a), newOverlayAddCmd(a))
	return cmd
}

func newOverlayListCmd(a overlayAxis) *cobra.Command {
	var chartDir string
	cmd := &cobra.Command{
		Use:   "list",
		Short: fmt.Sprintf("List the known %ss, and which the chart already has", a.noun()),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p := newPainter(out)

			// Outside a chart this is still useful as a plain catalog listing.
			have := map[string]bool{}
			if dir, err := chart.Find(chartDir); err == nil {
				if c, err := chart.Load(dir); err == nil {
					for _, o := range scaffold.ChartOverlays(c, a.axis) {
						have[o.Name] = true
					}
				}
			}

			fprintf(out, "%s\n", p.bold(a.heading))
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			for _, o := range catalog.Overlays(a.axis) {
				mark := p.dim("-")
				if have[o.Name] {
					mark = p.green("+")
				}
				fmt.Fprintf(w, "  %s %s\t%s\n", mark, o.Name, o.Summary)
				if len(o.Needs) > 0 {
					fmt.Fprintf(w, "  \t%s\n", p.dim("needs: "+strings.Join(o.Needs, ", ")))
				}
			}
			_ = w.Flush()
			fprintf(out, "\n  %s already in this chart\n", p.green("+"))
			return nil
		},
	}
	cmd.Flags().StringVar(&chartDir, "chart", ".", "chart directory")
	return cmd
}

func newOverlayAddCmd(a overlayAxis) *cobra.Command {
	var opts struct {
		chartDir string
		dryRun   bool
		force    bool
	}
	cmd := &cobra.Command{
		Use:     fmt.Sprintf("add <%s>...", a.noun()),
		Short:   fmt.Sprintf("Write %s values overlays into an existing chart", a.noun()),
		Args:    cobra.MinimumNArgs(1),
		Example: a.examples,
		ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return a.names(), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			names := splitList(args)
			if len(names) == 0 {
				return fmt.Errorf("name at least one %s (known: %s)", a.noun(), strings.Join(a.names(), ", "))
			}

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
			for _, name := range names {
				o, ok := catalog.LookupOverlay(a.axis, name)
				if !ok {
					return fmt.Errorf("unknown %s %q (known: %s)", a.noun(), name, strings.Join(a.names(), ", "))
				}
				dest := filepath.Join(c.Dir, o.ValuesFile())
				switch _, err := os.Stat(dest); {
				case err == nil && !opts.force:
					fprintf(out, "  %s %s (already exists; pass --force to rewrite)\n", p.dim("."), o.ValuesFile())
					continue
				case err != nil && !errors.Is(err, fs.ErrNotExist):
					return fmt.Errorf("stat %s: %w", dest, err)
				}
				overlay, ok, err := scaffold.BuildOverlayValues(data, resources, o)
				if err != nil {
					return err
				}
				if !ok {
					fprintf(out, "  %s %s — nothing in this chart differs there\n", p.dim("."), o.Name)
					continue
				}
				if opts.dryRun {
					fprintf(out, "  %s %s\n", p.green("+"), o.ValuesFile())
					continue
				}
				if err := os.WriteFile(dest, overlay, 0o644); err != nil {
					return fmt.Errorf("write %s: %w", o.ValuesFile(), err)
				}
				fprintf(out, "  %s %s\n", p.green("+"), o.ValuesFile())
				if len(o.Needs) > 0 {
					fprintf(out, "        %s\n", p.dim("needs: "+strings.Join(o.Needs, ", ")))
				}
				wrote++
			}
			if wrote > 0 {
				fprintf(out, "\nNext:\n  hck check --chart %s --%s %s\n", shellQuote(c.Dir), a.use(), names[0])
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.chartDir, "chart", ".", "chart directory; parent directories are searched for Chart.yaml")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "print what would be written and exit")
	cmd.Flags().BoolVar(&opts.force, "force", false, "rewrite an overlay that already exists")
	return cmd
}
