package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/somaz94/helm-chart-kit/internal/catalog"
	"github.com/somaz94/helm-chart-kit/internal/chart"
	"github.com/somaz94/helm-chart-kit/internal/check"
	"github.com/somaz94/helm-chart-kit/internal/scaffold"
)

// A chart that carries overlays is installed as one combination of them, and
// "hck check" renders exactly one — whichever the flags named. With no flags
// that is the combination nobody installs: a chart shipping values-aws.yaml
// and values-prod.yaml passes a check that applied neither, and the first
// thing that goes wrong under them goes wrong at install time.
//
// --all renders every combination the chart could be installed as. It is the
// same loop hck's own CI wrote by hand, twice, which is what said the loop
// belonged in hck.
//
// The set is a product of the two axes because the axes are orthogonal: a
// chart is installed somewhere and at some size. Overlay.Needs is about what
// a cluster has to provide, not about a pairing being impossible, so no
// combination is excluded. Each axis contributes "none" alongside what the
// chart carries — installing without an overlay is a real thing to do, and it
// is the case hck check has always covered. That is also why a chart with no
// overlays yields exactly one combination and --all changes nothing for it.
type combination struct {
	overlays []catalog.Overlay
}

// label is how the combination is named in a report. "base" rather than an
// empty string: a blank line item reads as a missing name, not as the absence
// of overlays, which is a real and separate thing to have checked.
func (c combination) label() string {
	if len(c.overlays) == 0 {
		return "base"
	}
	return strings.Join(c.names(), " + ")
}

func (c combination) names() []string {
	out := make([]string, 0, len(c.overlays))
	for _, o := range c.overlays {
		out = append(out, o.Name)
	}
	return out
}

// files are the -f paths for this combination, in the order helm has to read
// them. ChartOverlays only returns overlays whose file is on disk, so these
// exist by construction.
func (c combination) files(dir string) []string {
	out := make([]string, 0, len(c.overlays))
	for _, o := range c.overlays {
		out = append(out, filepath.Join(dir, o.ValuesFile()))
	}
	return out
}

// combinationsFor enumerates every way the chart could be installed.
//
// Platform-major, and environment last within each: overlays apply left to
// right and the size has to win over whatever the platform set, which is the
// same order a single run uses. The order is fixed so two runs over one chart
// read the same way.
func combinationsFor(c *chart.Chart) []combination {
	plats := axisChoices(scaffold.ChartOverlays(c, catalog.PlatformAxis))
	envs := axisChoices(scaffold.ChartOverlays(c, catalog.EnvironmentAxis))

	out := make([]combination, 0, len(plats)*len(envs))
	for _, p := range plats {
		for _, e := range envs {
			combo := make([]catalog.Overlay, 0, 2)
			combo = append(combo, p...)
			combo = append(combo, e...)
			out = append(out, combination{overlays: combo})
		}
	}
	return out
}

// axisChoices is what one axis offers: nothing, or any one of the overlays the
// chart carries. Never two from the same axis — a chart is installed on one
// platform at one size, and "aws + gcp" is not a thing to check.
func axisChoices(os []catalog.Overlay) [][]catalog.Overlay {
	out := make([][]catalog.Overlay, 0, len(os)+1)
	out = append(out, nil)
	for _, o := range os {
		out = append(out, []catalog.Overlay{o})
	}
	return out
}

// matrixOptions is the part of "hck check" that a whole-matrix run needs.
type matrixOptions struct {
	valuesFiles []string
	strict      bool
	noRender    bool
	format      string
}

// jsonMatrix is the machine-readable shape of "hck check --all".
//
// The top-level names are the ones a single run already uses — errors,
// warnings, infos, ok — so a CI step reading .ok or .errors keeps working
// across both shapes, and only a consumer that wants the breakdown has to
// know about .combinations.
type jsonMatrix struct {
	Chart        string       `json:"chart"`
	Combinations []jsonReport `json:"combinations"`
	Errors       int          `json:"errors"`
	Warnings     int          `json:"warnings"`
	Infos        int          `json:"infos"`
	// Failed counts combinations that did not pass, which is not the same as
	// the number of errors: one combination can carry several, and under
	// --strict a combination fails on warnings with no errors at all.
	Failed int  `json:"failed"`
	OK     bool `json:"ok"`
}

// runEveryCombination checks each combination in turn and reports them
// together. It keeps going after one fails: the point is which combinations
// are broken, and stopping at the first would answer a different question.
func runEveryCombination(out io.Writer, c *chart.Chart, cfg *check.Config, opts matrixOptions) error {
	combos := combinationsFor(c)
	p := newPainter(out)

	reports := make([]*check.Report, 0, len(combos))
	for _, combo := range combos {
		rep, err := check.Run(c, check.Options{
			Config:       cfg,
			ValuesFiles:  opts.valuesFiles,
			OverlayFiles: combo.files(c.Dir),
			SkipRender:   opts.noRender,
		})
		if err != nil {
			if errors.Is(err, check.ErrNoHelm) {
				return fmt.Errorf("%w (or pass --no-render to run only the layout rules)", err)
			}
			return fmt.Errorf("%s: %w", combo.label(), err)
		}
		reports = append(reports, rep)
	}

	if opts.format == formatJSON {
		return printMatrixJSON(out, c.Meta.Name, combos, reports, opts.strict)
	}

	width := 0
	for _, combo := range combos {
		width = max(width, len(combo.label()))
	}

	fprintf(out, "%s %s  %s\n\n", p.bold("check"), c.Meta.Name,
		p.dim(fmt.Sprintf("%d combination(s)", len(combos))))

	failed := 0
	for i, combo := range combos {
		rep := reports[i]
		// Padded before it is coloured: the escape codes are bytes that take
		// no width, so padding the coloured string misaligns every row that
		// has one.
		tag := p.green("ok  ")
		if !passes(rep, opts.strict) {
			tag = p.red("FAIL")
			failed++
		}
		// Trimmed rather than conditionally padded: a clean combination has
		// no tally, and the padding that lines the column up would otherwise
		// be trailing whitespace on most rows of a passing run.
		row := fmt.Sprintf("  %s  %-*s  %s", tag, width, combo.label(), p.dim(tally(rep)))
		fprintf(out, "%s\n", strings.TrimRight(row, " "))
		for _, f := range rep.Findings {
			fprintf(out, "          %s %s  %s\n", severityTag(p, f.Severity), p.dim(f.Rule), f.Where)
			fprintf(out, "                %s\n", f.Message)
		}
	}

	// Said once for the whole matrix rather than per combination: the rules a
	// chart turned off are the chart's, not any one combination's.
	if len(reports) > 0 && len(reports[0].Disabled) > 0 {
		fprintf(out, "\n  %s\n", p.dim("not checked: "+strings.Join(reports[0].Disabled, ", ")))
	}

	fprintf(out, "\n  %d combination(s), %d ok, %d failed\n", len(combos), len(combos)-failed, failed)
	if failed > 0 {
		return fmt.Errorf("check failed")
	}
	return nil
}

func printMatrixJSON(out io.Writer, chartName string, combos []combination, reports []*check.Report, strict bool) error {
	doc := jsonMatrix{
		Chart:        chartName,
		Combinations: make([]jsonReport, 0, len(combos)),
		OK:           true,
	}
	for i, combo := range combos {
		rep := reports[i]
		one := buildReportJSON(chartName, combo.names(), rep, strict)
		doc.Combinations = append(doc.Combinations, one)
		doc.Errors += one.Errors
		doc.Warnings += one.Warnings
		doc.Infos += one.Infos
		if !one.OK {
			doc.Failed++
			doc.OK = false
		}
	}
	if err := encodeJSON(out, doc); err != nil {
		return err
	}
	if !doc.OK {
		return fmt.Errorf("check failed")
	}
	return nil
}

// passes is the same verdict a single run reaches, so --all and a hand-written
// loop over --platform/--env cannot disagree about one combination.
func passes(rep *check.Report, strict bool) bool {
	return rep.Errors() == 0 && (!strict || rep.Warns() == 0)
}

// tally is the non-zero part of a report's counts, so a clean combination
// takes one line and says nothing more.
func tally(rep *check.Report) string {
	var parts []string
	if n := rep.Errors(); n > 0 {
		parts = append(parts, fmt.Sprintf("%d error(s)", n))
	}
	if n := rep.Warns(); n > 0 {
		parts = append(parts, fmt.Sprintf("%d warning(s)", n))
	}
	if n := rep.Infos(); n > 0 {
		parts = append(parts, fmt.Sprintf("%d info(s)", n))
	}
	return strings.Join(parts, ", ")
}

func severityTag(p painter, s check.Severity) string {
	switch s {
	case check.Error:
		return p.red("error")
	case check.Info:
		return p.cyan("info ")
	default:
		return p.yellow("warn ")
	}
}
