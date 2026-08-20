package cli

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/somaz94/helm-chart-kit/internal/catalog"
	"github.com/somaz94/helm-chart-kit/internal/chart"
	"github.com/somaz94/helm-chart-kit/internal/scaffold"
	"github.com/spf13/cobra"
)

func newSchemaCmd() *cobra.Command {
	var opts struct {
		chartDir string
		write    bool
		check    bool
		strict   bool
	}

	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Generate the chart's values.schema.json",
		Long: `Assemble values.schema.json from the resources the chart carries and print
it, write it, or check that the file on disk is current.

Helm validates the coalesced values against this file on every render, so the
schema is deliberately permissive: objects stay open, and a scalar whose
default is empty is typed as the union it really accepts. A schema that is
merely incomplete does not document a chart, it breaks one.

--strict closes the top level, so a misspelled top-level key is an error
instead of a value that silently does nothing. Nested objects stay open even
then; the point is to catch a typo, not to model the Kubernetes API.`,
		Args: cobra.NoArgs,
		Example: `  hck schema
  hck schema --write
  hck schema --write --strict
  hck schema --check
  hck schema --chart ./charts/payments-api --write`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.write && opts.check {
				return fmt.Errorf("--write and --check do the opposite things; pass one")
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
				return fmt.Errorf("%s carries no resource this catalog knows, so there is nothing to describe", c.Meta.Name)
			}

			// Read the file once. Both questions asked of it — is it current,
			// and was it written strict — are answered from the same bytes,
			// and an unreadable one is reported rather than rebuilt permissive.
			current, err := c.Schema()
			if err != nil {
				return err
			}

			// Without an explicit --strict, a schema already on disk keeps
			// whatever it was generated with. Otherwise --check would compare
			// a strict file against a permissive rebuild and always fail.
			strict := opts.strict
			if !cmd.Flags().Changed("strict") {
				strict = scaffold.SchemaIsStrictBytes(current)
			}

			doc, res, err := scaffold.BuildSchema(scaffold.DataFor(c), resources, strict)
			if err != nil {
				return err
			}

			// The command offered for a --check failure has to ask for the
			// strictness --check just compared against. Without the flag,
			// running it rebuilds the file the way it already is, --check
			// fails again, and a CI job pinned to --check --strict never goes
			// green no matter how many times the advice is followed.
			fix := "hck schema --write"
			if cmd.Flags().Changed("strict") {
				fix = fmt.Sprintf("hck schema --write --strict=%t", strict)
			}

			out := cmd.OutOrStdout()
			p := newPainter(out)

			switch {
			case opts.check:
				if current == nil {
					return fmt.Errorf("%s has no %s — run: %s", c.Meta.Name, scaffold.SchemaFile, fix)
				}
				if !bytes.Equal(current, doc) {
					return fmt.Errorf("%s is out of date — run: %s", scaffold.SchemaFile, fix)
				}
				fprintf(out, "  %s %s is up to date\n", p.green("ok"), scaffold.SchemaFile)
				return nil

			case opts.write:
				if err := os.WriteFile(c.SchemaPath(), doc, 0o644); err != nil {
					return fmt.Errorf("write %s: %w", scaffold.SchemaFile, err)
				}
				fprintf(out, "%s %s\n\n", p.bold("wrote"), c.SchemaPath())
				fprintf(out, "  %d key(s) from %d resource(s): %s\n", len(res.Added), len(resources), p.dim(joinNames(resources)))
				// A key two resources both define is described once, by the
				// first to claim it — the same resolution values.yaml made.
				// Worth saying, because the description a user goes looking
				// for may have come from the other one.
				if len(res.Skipped) > 0 {
					fprintf(out, "  %s %s\n", p.dim("described once, by the resource that owns it:"), p.dim(strings.Join(res.Skipped, " ")))
				}
				fprintf(out, "\nNext:\n  hck check --chart %s\n", c.Dir)
				return nil

			default:
				_, err := out.Write(doc)
				return err
			}
		},
	}

	cmd.Flags().StringVar(&opts.chartDir, "chart", ".", "chart directory; parent directories are searched for Chart.yaml")
	cmd.Flags().BoolVar(&opts.write, "write", false, "write values.schema.json into the chart")
	cmd.Flags().BoolVar(&opts.check, "check", false, "fail when the file on disk differs from what would be generated")
	cmd.Flags().BoolVar(&opts.strict, "strict", false, "reject undeclared top-level keys; defaults to whatever the existing schema uses")
	return cmd
}

// joinNames renders a resource list for a one-line report.
func joinNames(rs []catalog.Resource) string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Name)
	}
	return strings.Join(out, " ")
}
