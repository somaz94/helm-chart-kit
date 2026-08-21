package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/somaz94/helm-chart-kit/internal/catalog"
	"github.com/somaz94/helm-chart-kit/internal/chart"
	"github.com/somaz94/helm-chart-kit/internal/check"
	"github.com/spf13/cobra"
)

func newCheckCmd() *cobra.Command {
	var opts struct {
		chartDir    string
		valuesFiles []string
		platforms   []string
		envs        []string
		strict      bool
		printOutput bool
		noRender    bool
		format      string
		off         []string
	}

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Render a chart and apply the house rules",
		Long: `Render the chart with your own helm, then run the house rules over the
manifests that come out.

Rendering uses ci/install-values.yaml when the chart has one and no -f was
given, because a chart that requires an image tag cannot render on its
defaults — which is the point of requiring one.

A chart can turn a rule off or change its severity in its own .hck.yaml:

    rules:
      HCK025: off      # this chart wants its CPU limits
      HCK023: error    # and will not ship without requests

"*" stands for every rule that can be configured, and a named ID beats it, so
a chart that wants the house rules mostly out of the way says so once:

    rules:
      "*": off
      HCK021: error    # except: an untagged image is still a defect

--off does the same thing for one run without editing the chart, and takes "*"
too. Either way the report still names what it did not look for.

A rule takes off, info, warn or error. An info is a prerequisite rather than a
defect — something true about the chart that is nobody's mistake, such as a
storage class the platform does not ship — so it is reported and --strict does
not fail on it. Raise one to warn when it is your job to satisfy.

Run "hck list rules" for the rule IDs.`,
		Args: cobra.NoArgs,
		Example: `  hck check
  hck check --chart ./charts/payments-api
  hck check -f values/prod.yaml --strict
  hck check --platform aws
  hck check --platform aws,gcp --strict
  hck check --platform aws --env prod --strict
  hck check --format json
  hck check --off HCK025,HCK011
  hck check --off '*'
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

			// An overlay that does not render is worse than no overlay: it
			// looks like configuration until the day it is used. Environments
			// go last — overlays apply left to right, so the size one asks for
			// wins over whatever a platform overlay set.
			overlays, err := overlayPaths(c, catalog.PlatformAxis, splitList(opts.platforms))
			if err != nil {
				return err
			}
			envs, err := overlayPaths(c, catalog.EnvironmentAxis, splitList(opts.envs))
			if err != nil {
				return err
			}
			overlays = append(overlays, envs...)

			if opts.format != formatText && opts.format != formatJSON {
				return fmt.Errorf("unknown --format %q; it takes %s or %s", opts.format, formatText, formatJSON)
			}

			// A chart's own .hck.yaml, read before the run so a typo in a rule
			// ID is an error rather than a rule that quietly kept reporting.
			cfg, err := check.LoadConfig(c.Dir)
			if err != nil {
				return err
			}
			// --off layers over whatever the chart said, for this run only. A
			// rule ID nobody has is an error here for the same reason it is in
			// the file: a misspelled --off that quietly kept reporting reads
			// exactly like the rule being right.
			cfg, err = cfg.TurnOff(splitList(opts.off))
			if err != nil {
				return err
			}

			rep, err := check.Run(c, check.Options{
				Config:       cfg,
				ValuesFiles:  opts.valuesFiles,
				OverlayFiles: overlays,
				SkipRender:   opts.noRender,
			})
			if err != nil {
				if errors.Is(err, check.ErrNoHelm) {
					return fmt.Errorf("%w (or pass --no-render to run only the layout rules)", err)
				}
				return err
			}

			out := cmd.OutOrStdout()
			p := newPainter(out)

			label := c.Meta.Name
			names := append(splitList(opts.platforms), splitList(opts.envs)...)
			if len(names) > 0 {
				label += " (" + strings.Join(names, " + ") + ")"
			}

			if opts.format == formatJSON {
				return printReportJSON(out, c.Meta.Name, names, rep, opts.strict)
			}

			if opts.printOutput && rep.Rendered != "" {
				fprintf(out, "%s\n", rep.Rendered)
			}
			fprintf(out, "%s %s\n\n", p.bold("check"), label)
			for _, f := range rep.Findings {
				tag := p.yellow("warn ")
				switch f.Severity {
				case check.Error:
					tag = p.red("error")
				case check.Info:
					tag = p.cyan("info ")
				}
				fprintf(out, "  %s %s  %s\n", tag, p.dim(f.Rule), f.Where)
				fprintf(out, "        %s\n", f.Message)
			}

			// What was not looked for belongs next to what was: a clean
			// report over a chart with half the rules off says less than it
			// looks like it does.
			if len(rep.Disabled) > 0 {
				fprintf(out, "  %s\n", p.dim("not checked: "+strings.Join(rep.Disabled, ", ")))
			}

			// Notes ride along in every tally and decide none of them: they
			// are what the chart needs from its cluster, not what is wrong
			// with the chart, so --strict does not see them. Counting them
			// separately is what keeps "no findings" honest — a run that
			// reported a prerequisite did not find nothing.
			errs, warns, infos := rep.Errors(), rep.Warns(), rep.Infos()
			notes := ""
			if infos > 0 {
				notes = fmt.Sprintf(", %d info(s)", infos)
			}
			switch {
			case errs == 0 && warns == 0 && infos == 0:
				fprintf(out, "  %s no findings\n", p.green("ok"))
				return nil
			case errs == 0 && warns == 0:
				fprintf(out, "\n  %s 0 warnings, 0 errors%s\n", p.green("ok"), notes)
				return nil
			case errs == 0 && !opts.strict:
				fprintf(out, "\n  %s %d warning(s), 0 errors%s\n", p.green("ok"), warns, notes)
				return nil
			}
			fprintf(out, "\n  %d error(s), %d warning(s)%s\n", errs, warns, notes)
			return fmt.Errorf("check failed")
		},
	}

	cmd.Flags().StringVar(&opts.chartDir, "chart", ".", "chart directory; parent directories are searched for Chart.yaml")
	cmd.Flags().StringSliceVarP(&opts.valuesFiles, "values", "f", nil, "values files passed to helm; repeatable")
	cmd.Flags().StringSliceVar(&opts.platforms, "platform", nil, "also apply these platform overlays: "+strings.Join(catalog.OverlayNames(catalog.PlatformAxis), ", "))
	cmd.Flags().StringSliceVar(&opts.envs, "env", nil, "also apply these environment overlays: "+strings.Join(catalog.OverlayNames(catalog.EnvironmentAxis), ", "))
	cmd.Flags().BoolVar(&opts.strict, "strict", false, "fail on warnings as well as errors; info findings never fail")
	cmd.Flags().BoolVar(&opts.printOutput, "print", false, "print the rendered manifests")
	cmd.Flags().BoolVar(&opts.noRender, "no-render", false, "skip helm; run only the rules that read the chart directory")
	cmd.Flags().StringVar(&opts.format, "format", formatText, "output format: "+formatText+" or "+formatJSON)
	cmd.Flags().StringSliceVar(&opts.off, "off", nil, `rules to turn off for this run; "*" turns off every rule that can be`)

	_ = cmd.RegisterFlagCompletionFunc("off",
		func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			ids := make([]string, 0, len(check.Rules()))
			for _, r := range check.Rules() {
				if !r.Locked {
					ids = append(ids, r.ID)
				}
			}
			return ids, cobra.ShellCompDirectiveNoFileComp
		})
	return cmd
}

// The two shapes a report comes out in. JSON exists so a CI step can act on a
// finding rather than grep for it; the text form stays the default because it
// is what a person reads.
const (
	formatText = "text"
	formatJSON = "json"
)

// jsonReport is the machine-readable shape of a run. The field names are part
// of the interface — a CI step that reads "ok" should keep working.
type jsonReport struct {
	Chart    string        `json:"chart"`
	Overlays []string      `json:"overlays,omitempty"`
	Findings []jsonFinding `json:"findings"`
	Errors   int           `json:"errors"`
	Warnings int           `json:"warnings"`
	// Infos counts the findings that report a prerequisite rather than a
	// defect. It is reported separately from Warnings and not folded into it,
	// because OK ignores it: a consumer summing the two would fail a chart
	// over something --strict deliberately lets pass.
	Infos int `json:"infos"`
	// Disabled names the rules the chart turned off, so a consumer can tell a
	// clean run from an unasked question.
	Disabled []string `json:"disabled,omitempty"`
	// OK is the verdict, and matches the exit status: --strict makes a warning
	// enough to fail.
	OK bool `json:"ok"`
}

type jsonFinding struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Where    string `json:"where"`
	Message  string `json:"message"`
}

// printReportJSON writes the report as JSON and returns the same failure the
// text path would. The rendered manifests are deliberately absent: --print is
// for reading, and a manifest stream inside a JSON string is neither.
func printReportJSON(out io.Writer, chartName string, overlays []string, rep *check.Report, strict bool) error {
	errs, warns := rep.Errors(), rep.Warns()
	doc := jsonReport{
		Chart:    chartName,
		Overlays: overlays,
		Findings: make([]jsonFinding, 0, len(rep.Findings)),
		Errors:   errs,
		Warnings: warns,
		Infos:    rep.Infos(),
		Disabled: rep.Disabled,
		OK:       errs == 0 && (!strict || warns == 0),
	}
	for _, f := range rep.Findings {
		doc.Findings = append(doc.Findings, jsonFinding{
			Rule:     f.Rule,
			Severity: string(f.Severity),
			Where:    f.Where,
			Message:  f.Message,
		})
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return err
	}
	if !doc.OK {
		return fmt.Errorf("check failed")
	}
	return nil
}

// overlayPaths resolves overlay names on one axis to the files inside a chart,
// refusing a name the catalog does not know and one the chart never had a file
// written for.
func overlayPaths(c *chart.Chart, axis catalog.Axis, names []string) ([]string, error) {
	out := make([]string, 0, len(names))
	for _, name := range names {
		o, ok := catalog.LookupOverlay(axis, name)
		if !ok {
			return nil, fmt.Errorf("unknown %s %q (known: %s)", axis, name, strings.Join(catalog.OverlayNames(axis), ", "))
		}
		path := filepath.Join(c.Dir, o.ValuesFile())
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("%s has no %s — run: hck %s add %s", c.Meta.Name, o.ValuesFile(), axis.Command(), o.Name)
		}
		out = append(out, path)
	}
	return out, nil
}
