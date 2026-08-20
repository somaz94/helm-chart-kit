package cli

import (
	"fmt"
	"strings"

	"github.com/somaz94/helm-chart-kit/internal/catalog"
	"github.com/somaz94/helm-chart-kit/internal/scaffold"
	"github.com/spf13/cobra"
)

func newNewCmd() *cobra.Command {
	var opts struct {
		preset      string
		dir         string
		description string
		version     string
		appVersion  string
		with        []string
		dryRun      bool
	}

	cmd := &cobra.Command{
		Use:   "new <chart-name>",
		Short: "Create a chart from a preset",
		Long: `Create a chart directory seeded with a preset's resources.

The preset decides which templates the chart starts with; --with adds more on
top. Every resource contributes its own documented section to values.yaml.`,
		Args: cobra.ExactArgs(1),
		Example: `  hck new payments-api
  hck new payments-api --preset worker
  hck new cache --preset stateful --with servicemonitor,pdb
  hck new gateway --preset web --with httproute --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			desc := opts.description
			if desc == "" {
				desc = fmt.Sprintf("A Helm chart for %s", name)
			}

			plan, err := scaffold.PlanNew(scaffold.NewOptions{
				Parent:      opts.dir,
				Name:        name,
				Description: desc,
				Version:     opts.version,
				AppVersion:  opts.appVersion,
				Preset:      opts.preset,
				Extra:       splitList(opts.with),
			})
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			c := newPainter(out)
			if opts.dryRun {
				fprintf(out, "%s %s (preset %s, dry run)\n\n", c.bold("plan"), plan.ChartDir, opts.preset)
				printPlan(out, plan, true)
				return nil
			}

			if err := scaffold.Apply(plan); err != nil {
				return err
			}
			fprintf(out, "%s %s (preset %s)\n\n", c.bold("created"), plan.ChartDir, opts.preset)
			printPlan(out, plan, false)
			fprintf(out, "\nNext:\n  hck check --chart %s\n", plan.ChartDir)
			return nil
		},
	}

	presets := strings.Join(catalog.PresetNames(), ", ")
	cmd.Flags().StringVarP(&opts.preset, "preset", "p", "web", "resource set to seed with: "+presets)
	cmd.Flags().StringVarP(&opts.dir, "dir", "d", ".", "parent directory to create the chart in")
	cmd.Flags().StringVar(&opts.description, "description", "", "Chart.yaml description")
	cmd.Flags().StringVar(&opts.version, "version", "0.1.0", "chart version")
	cmd.Flags().StringVar(&opts.appVersion, "app-version", "1.0.0", "version of the application the chart deploys")
	cmd.Flags().StringSliceVar(&opts.with, "with", nil, "extra resources on top of the preset")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "print what would be written and exit")

	_ = cmd.RegisterFlagCompletionFunc("preset",
		func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return catalog.PresetNames(), cobra.ShellCompDirectiveNoFileComp
		})
	_ = cmd.RegisterFlagCompletionFunc("with",
		func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return catalog.ResourceNames(), cobra.ShellCompDirectiveNoFileComp
		})

	return cmd
}

// splitList flattens comma-separated and repeated flag values, dropping empties.
func splitList(in []string) []string {
	var out []string
	for _, s := range in {
		for _, part := range strings.Split(s, ",") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}
