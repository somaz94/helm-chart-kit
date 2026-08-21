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
		preset       string
		dir          string
		description  string
		version      string
		appVersion   string
		with         []string
		dryRun       bool
		force        bool
		schema       bool
		schemaStrict bool
		platforms    []string
		envs         []string
	}

	cmd := &cobra.Command{
		Use:   "new <chart-name>",
		Short: "Create a chart from a preset",
		Long: `Create a chart directory seeded with a preset's resources.

The preset decides which templates the chart starts with; --with adds more on
top. Every resource contributes its own documented section to values.yaml.

A target directory that is not empty is refused by default and waived by
--force, which fills in what is missing and leaves every file already there
alone, values.yaml included — use "hck add" to extend a chart that exists.

A second primary workload is allowed and noted. Two workload templates guarded
so that one renders at a time is an ordinary shape; two rendering at once is
not, and "hck check" reports HCK030 over the render, where the question can be
answered.`,
		Args: cobra.ExactArgs(1),
		Example: `  hck new payments-api
  hck new payments-api --preset worker
  hck new cache --preset stateful --with servicemonitor,pdb
  hck new gateway --preset web --with httproute --dry-run
  hck new payments-api --schema
  hck new payments-api --platform aws
  hck new payments-api --platform aws,gcp --schema
  hck new payments-api --platform aws --env dev,prod
  hck new blue-green --preset web --with daemonset`,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			desc := opts.description
			if desc == "" {
				desc = fmt.Sprintf("A Helm chart for %s", name)
			}

			plan, err := scaffold.PlanNew(scaffold.NewOptions{
				Parent:       opts.dir,
				Name:         name,
				Description:  desc,
				Version:      opts.version,
				AppVersion:   opts.appVersion,
				Preset:       opts.preset,
				Extra:        splitList(opts.with),
				Schema:       opts.schema || opts.schemaStrict,
				SchemaStrict: opts.schemaStrict,
				Platforms:    splitList(opts.platforms),
				Environments: splitList(opts.envs),
				Force:        opts.force,
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
	cmd.Flags().StringSliceVar(&opts.with, "with", nil, "extra resources on top of the preset, or @group for all of one")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "print what would be written and exit")
	cmd.Flags().BoolVar(&opts.force, "force", false, "write into a directory that is not empty (files already there are left alone)")
	cmd.Flags().BoolVar(&opts.schema, "schema", false, "also write values.schema.json")
	cmd.Flags().BoolVar(&opts.schemaStrict, "schema-strict", false, "write values.schema.json and reject undeclared top-level keys")
	cmd.Flags().StringSliceVar(&opts.platforms, "platform", nil, "platform values overlays to write: "+strings.Join(catalog.OverlayNames(catalog.PlatformAxis), ", "))
	cmd.Flags().StringSliceVar(&opts.envs, "env", nil, "environment values overlays to write: "+strings.Join(catalog.OverlayNames(catalog.EnvironmentAxis), ", "))

	_ = cmd.RegisterFlagCompletionFunc("preset",
		func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return catalog.PresetNames(), cobra.ShellCompDirectiveNoFileComp
		})
	_ = cmd.RegisterFlagCompletionFunc("env",
		func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return catalog.OverlayNames(catalog.EnvironmentAxis), cobra.ShellCompDirectiveNoFileComp
		})
	_ = cmd.RegisterFlagCompletionFunc("platform",
		func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return catalog.OverlayNames(catalog.PlatformAxis), cobra.ShellCompDirectiveNoFileComp
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
