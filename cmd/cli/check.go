package cli

import (
	"errors"
	"fmt"

	"github.com/somaz94/helm-chart-kit/internal/chart"
	"github.com/somaz94/helm-chart-kit/internal/check"
	"github.com/spf13/cobra"
)

func newCheckCmd() *cobra.Command {
	var opts struct {
		chartDir    string
		valuesFiles []string
		strict      bool
		printOutput bool
		noRender    bool
	}

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Render a chart and apply the house rules",
		Long: `Render the chart with your own helm, then run the house rules over the
manifests that come out.

Rendering uses ci/install-values.yaml when the chart has one and no -f was
given, because a chart that requires an image tag cannot render on its
defaults — which is the point of requiring one.`,
		Args: cobra.NoArgs,
		Example: `  hck check
  hck check --chart ./charts/payments-api
  hck check -f values/prod.yaml --strict
  hck check --print`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := chart.Find(opts.chartDir)
			if err != nil {
				return err
			}
			c, err := chart.Load(dir)
			if err != nil {
				return err
			}

			rep, err := check.Run(c, check.Options{
				ValuesFiles: opts.valuesFiles,
				SkipRender:  opts.noRender,
			})
			if err != nil {
				if errors.Is(err, check.ErrNoHelm) {
					return fmt.Errorf("%w (or pass --no-render to run only the layout rules)", err)
				}
				return err
			}

			out := cmd.OutOrStdout()
			p := newPainter(out)

			if opts.printOutput && rep.Rendered != "" {
				fprintf(out, "%s\n", rep.Rendered)
			}

			fprintf(out, "%s %s\n\n", p.bold("check"), c.Meta.Name)
			for _, f := range rep.Findings {
				tag := p.yellow("warn ")
				if f.Severity == check.Error {
					tag = p.red("error")
				}
				fprintf(out, "  %s %s  %s\n", tag, p.dim(f.Rule), f.Where)
				fprintf(out, "        %s\n", f.Message)
			}

			errs, warns := rep.Errors(), rep.Warns()
			switch {
			case errs == 0 && warns == 0:
				fprintf(out, "  %s no findings\n", p.green("ok"))
				return nil
			case errs == 0 && !opts.strict:
				fprintf(out, "\n  %s %d warning(s), 0 errors\n", p.green("ok"), warns)
				return nil
			}
			fprintf(out, "\n  %d error(s), %d warning(s)\n", errs, warns)
			return fmt.Errorf("check failed")
		},
	}

	cmd.Flags().StringVar(&opts.chartDir, "chart", ".", "chart directory; parent directories are searched for Chart.yaml")
	cmd.Flags().StringSliceVarP(&opts.valuesFiles, "values", "f", nil, "values files passed to helm; repeatable")
	cmd.Flags().BoolVar(&opts.strict, "strict", false, "fail on warnings as well as errors")
	cmd.Flags().BoolVar(&opts.printOutput, "print", false, "print the rendered manifests")
	cmd.Flags().BoolVar(&opts.noRender, "no-render", false, "skip helm; run only the rules that read the chart directory")
	return cmd
}
