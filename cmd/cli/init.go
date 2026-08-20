package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/somaz94/helm-chart-kit/internal/catalog"
	"github.com/somaz94/helm-chart-kit/internal/chart"
	"github.com/somaz94/helm-chart-kit/internal/docs"
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
}

// prompt puts a question with the hint shown in brackets and returns the raw
// line. An empty line means "take the default"; so does EOF, so a script that
// answers the first few questions does not have to answer the rest.
func (q *prompter) prompt(question, hint string) string {
	fprintf(q.out, "%s %s ", question, q.p.dim("["+hint+"]"))
	line, err := q.in.ReadString('\n')
	if err != nil && line == "" {
		// Nothing more is coming; close the line the prompt opened.
		fprintf(q.out, "\n")
	}
	return strings.TrimSpace(line)
}

// ask returns the answer, or def when the line was empty.
func (q *prompter) ask(question, def string) string {
	hint := def
	if hint == "" {
		hint = "none"
	}
	if s := q.prompt(question, hint); s != "" {
		return s
	}
	return def
}

// askYesNo shows which way Enter goes, with the default capitalised the way
// every other command-line tool does it.
func (q *prompter) askYesNo(question string, def bool) bool {
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	switch strings.ToLower(q.prompt(question, hint)) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return def
	}
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

With --defaults, or when stdin is not a terminal and has nothing to say, every
answer takes its default and nothing is asked.`,
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
				fprintf(out, "%s\n", p.bold("A few questions. Enter takes the default in brackets."))
				fprintf(out, "\n")

				a.name = q.ask("Chart name?", a.name)

				fprintf(out, "\n  %s\n", p.dim("presets:"))
				for _, ps := range catalog.Presets() {
					fprintf(out, "    %-9s %s\n", ps.Name, p.dim(ps.Summary))
				}
				a.preset = q.ask("Preset?", a.preset)

				fprintf(out, "\n  %s\n", p.dim("optional resources: "+strings.Join(addableNames(), ", ")))
				a.extra = splitList([]string{q.ask("Extra resources? (comma-separated)", "")})

				fprintf(out, "\n  %s\n", p.dim("platforms: "+strings.Join(catalog.PlatformNames(), ", ")))
				a.platforms = splitList([]string{q.ask("Platform overlays?", "")})

				fprintf(out, "\n  %s\n", p.dim("environments: "+strings.Join(catalog.EnvironmentNames(), ", ")))
				a.envs = splitList([]string{q.ask("Environment overlays?", "")})

				fprintf(out, "\n")
				a.schema = q.askYesNo("Write values.schema.json?", false)
				fprintf(out, "\n")
				a.readme = q.askYesNo("Write a values table into README.md?", false)
				fprintf(out, "\n")
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
			fprintf(out, "\nNext:\n  hck check --chart %s%s\n", plan.ChartDir, checkSuffix(a))
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
	b.WriteString("hck new " + a.name)
	if dir != "." {
		b.WriteString(" --dir " + dir)
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
		b.WriteString(" && hck docs --chart " + a.name + " --write")
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

// writeReadme generates the values table for a chart that has just been made.
func writeReadme(dir string) error {
	c, err := chart.Load(dir)
	if err != nil {
		return err
	}
	values, err := c.Values()
	if err != nil {
		return err
	}
	schema, err := c.Schema()
	if err != nil {
		return err
	}
	if schema == nil {
		if resources, err := scaffold.ChartResources(c); err == nil && len(resources) > 0 {
			schema, _, _ = scaffold.BuildSchema(scaffold.DataFor(c), resources, false)
		}
	}
	table, err := docs.Table(values, docs.Options{Schema: schema})
	if err != nil {
		return err
	}
	next, err := docs.Replace([]byte(docs.Skeleton(c.Meta.Name, c.Meta.Description)), table)
	if err != nil {
		return err
	}
	return os.WriteFile(c.ReadmePath(), next, 0o644)
}
