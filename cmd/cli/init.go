package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/somaz94/helm-chart-kit/internal/catalog"
	"github.com/somaz94/helm-chart-kit/internal/chart"
	"github.com/somaz94/helm-chart-kit/internal/scaffold"
	"github.com/spf13/cobra"
)

// answers is everything init asks for, and the shape PlanNew needs.
type answers struct {
	name      string
	preset    string
	extra     []string
	platforms []string
	envs      []string
	schema    bool
	readme    bool
}

// prompter reads one answer at a time. Reading from cmd.InOrStdin rather than
// os.Stdin is what lets the whole flow be driven from a test, and from a
// heredoc in CI.
type prompter struct {
	in  *bufio.Reader
	out io.Writer
	p   painter
	// done latches once the input is exhausted, so the remaining questions
	// take their defaults without being printed.
	done bool
}

// prompt puts a question with the hint shown in brackets and returns the raw
// line. An empty line means "take the default".
//
// EOF means the same, and latches: once the input is exhausted nothing further
// is printed, so a script that answers the first two questions does not scroll
// five more prompts past the reader. A read error that is not EOF is a real
// failure and is returned — treating it as "take the default" would scaffold a
// chart called my-app out of a broken pipe.
func (q *prompter) prompt(question, hint string) (string, error) {
	if q.done {
		return "", nil
	}
	fprintf(q.out, "%s %s ", question, q.p.dim("["+hint+"]"))
	line, err := q.in.ReadString('\n')
	if err != nil {
		if !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read answer: %w", err)
		}
		q.done = true
		if line == "" {
			// Close the line this prompt opened; nothing will.
			fprintf(q.out, "\n")
		}
	}
	return strings.TrimSpace(line), nil
}

// ask returns the answer, or def when the line was empty.
func (q *prompter) ask(question, def string) (string, error) {
	hint := def
	if hint == "" {
		hint = "none"
	}
	s, err := q.prompt(question, hint)
	if err != nil {
		return "", err
	}
	if s != "" {
		return s, nil
	}
	return def, nil
}

// askYesNo shows which way Enter goes, with the default capitalised the way
// every other command-line tool does it.
func (q *prompter) askYesNo(question string, def bool) (bool, error) {
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	s, err := q.prompt(question, hint)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(s) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return def, nil
	}
}

// say prints only while there is still someone answering. After EOF the
// listings below each question are noise.
func (q *prompter) say(format string, a ...any) {
	if !q.done {
		fprintf(q.out, format, a...)
	}
}

// interview fills in the answers. Each question can fail, because a read
// error is not the same as an empty line, and a chart scaffolded from a
// broken pipe is worse than no chart.
func (q *prompter) interview(a *answers, p painter) error {
	var err error
	q.say("%s\n\n", p.bold("A few questions. Enter takes the default in brackets."))

	if a.name, err = q.ask("Chart name?", a.name); err != nil {
		return err
	}

	q.say("\n  %s\n", p.dim("presets:"))
	for _, ps := range catalog.Presets() {
		q.say("    %-9s %s\n", ps.Name, p.dim(ps.Summary))
	}
	if a.preset, err = q.ask("Preset?", a.preset); err != nil {
		return err
	}

	q.say("\n  %s\n", p.dim("optional resources: "+strings.Join(addableNames(), ", ")))
	list, err := q.ask("Extra resources? (comma-separated)", "")
	if err != nil {
		return err
	}
	a.extra = splitList([]string{list})

	q.say("\n  %s\n", p.dim("platforms: "+strings.Join(catalog.OverlayNames(catalog.PlatformAxis), ", ")))
	if list, err = q.ask("Platform overlays?", ""); err != nil {
		return err
	}
	a.platforms = splitList([]string{list})

	q.say("\n  %s\n", p.dim("environments: "+strings.Join(catalog.OverlayNames(catalog.EnvironmentAxis), ", ")))
	if list, err = q.ask("Environment overlays?", ""); err != nil {
		return err
	}
	a.envs = splitList([]string{list})

	q.say("\n")
	if a.schema, err = q.askYesNo("Write values.schema.json?", false); err != nil {
		return err
	}
	q.say("\n")
	if a.readme, err = q.askYesNo("Write a values table into README.md?", false); err != nil {
		return err
	}
	q.say("\n")
	return nil
}

func newInitCmd() *cobra.Command {
	var opts struct {
		dir      string
		defaults bool
	}

	cmd := &cobra.Command{
		Use:   "init [chart-name]",
		Short: "Create a chart by answering a few questions",
		Long: `Ask what the chart is for, then scaffold it.

Everything here is also a flag on "hck new"; init prints the equivalent
command when it is done, so the second chart can skip the questions.

--defaults asks nothing. Input that runs out partway through is fine too: the
remaining questions take their defaults and stop being printed.`,
		Args: cobra.MaximumNArgs(1),
		Example: `  hck init
  hck init payments-api
  hck init payments-api --defaults`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p := newPainter(out)
			q := &prompter{in: bufio.NewReader(cmd.InOrStdin()), out: out, p: p}

			a := answers{name: "my-app", preset: "web"}
			if len(args) == 1 {
				a.name = args[0]
			}

			if !opts.defaults {
				if err := q.interview(&a, p); err != nil {
					return err
				}
			}

			if err := scaffold.ValidateName(a.name); err != nil {
				return err
			}

			plan, err := scaffold.PlanNew(scaffold.NewOptions{
				Parent:       opts.dir,
				Name:         a.name,
				Description:  fmt.Sprintf("A Helm chart for %s", a.name),
				Version:      "0.1.0",
				AppVersion:   "1.0.0",
				Preset:       a.preset,
				Extra:        a.extra,
				Schema:       a.schema,
				Platforms:    a.platforms,
				Environments: a.envs,
			})
			if err != nil {
				return err
			}
			if err := scaffold.Apply(plan); err != nil {
				return err
			}

			fprintf(out, "%s %s (preset %s)\n\n", p.bold("created"), plan.ChartDir, a.preset)
			printPlan(out, plan, false)

			if a.readme {
				if err := writeReadme(plan.ChartDir); err != nil {
					return err
				}
				fprintf(out, "  %s README.md\n", p.green("+"))
			}

			fprintf(out, "\n%s\n  %s\n", p.dim("The same thing without the questions:"), equivalentCommand(a, opts.dir))
			fprintf(out, "\nNext:\n  hck check --chart %s%s\n", shellQuote(plan.ChartDir), checkSuffix(a))
			return nil
		},
	}

	cmd.Flags().StringVarP(&opts.dir, "dir", "d", ".", "parent directory to create the chart in")
	cmd.Flags().BoolVar(&opts.defaults, "defaults", false, "take every default and ask nothing")
	return cmd
}

// addableNames lists the resources worth naming at the prompt: the workloads
// are the preset's job, not an extra.
func addableNames() []string {
	var out []string
	for _, r := range catalog.Resources() {
		if !r.Workload {
			out = append(out, r.Name)
		}
	}
	return out
}

// equivalentCommand renders the "hck new" that would have produced the same
// chart. Printing it is the point of the command: the questions are for the
// first chart, the flags are for every one after it.
func equivalentCommand(a answers, dir string) string {
	var b strings.Builder
	b.WriteString("hck new " + shellQuote(a.name))
	if dir != "." {
		b.WriteString(" --dir " + shellQuote(dir))
	}
	if a.preset != "web" {
		b.WriteString(" --preset " + a.preset)
	}
	if len(a.extra) > 0 {
		b.WriteString(" --with " + strings.Join(a.extra, ","))
	}
	if len(a.platforms) > 0 {
		b.WriteString(" --platform " + strings.Join(a.platforms, ","))
	}
	if len(a.envs) > 0 {
		b.WriteString(" --env " + strings.Join(a.envs, ","))
	}
	if a.schema {
		b.WriteString(" --schema")
	}
	if a.readme {
		// The chart is at <dir>/<name>, not <name>: chaining "hck docs
		// --chart <name>" only worked when --dir was left at the default.
		b.WriteString(" && hck docs --chart " + shellQuote(filepath.Join(dir, a.name)) + " --write")
	}
	return b.String()
}

// checkSuffix carries the overlays into the suggested check, so the command
// offered actually exercises what was just written.
func checkSuffix(a answers) string {
	var b strings.Builder
	if len(a.platforms) > 0 {
		b.WriteString(" --platform " + strings.Join(a.platforms, ","))
	}
	if len(a.envs) > 0 {
		b.WriteString(" --env " + strings.Join(a.envs, ","))
	}
	return b.String()
}

// writeReadme generates the values table for a chart that has just been made,
// through the same path "hck docs --write" takes. The two were briefly
// separate copies and immediately drifted — different error wrapping, and only
// one of them read an existing README.
func writeReadme(dir string) error {
	c, err := chart.Load(dir)
	if err != nil {
		return err
	}
	return writeValuesTable(c)
}
