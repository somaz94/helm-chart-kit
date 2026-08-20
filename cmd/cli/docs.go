package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/somaz94/helm-chart-kit/internal/chart"
	"github.com/somaz94/helm-chart-kit/internal/docs"
	"github.com/somaz94/helm-chart-kit/internal/scaffold"
	"github.com/spf13/cobra"
)

func newDocsCmd() *cobra.Command {
	var opts struct {
		chartDir string
		write    bool
		check    bool
	}

	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Generate the values table for the chart's README",
		Long: `Turn values.yaml into a Markdown table and print it, write it into the
chart's README, or check that the README is current.

Descriptions come from the file itself: a comment opening with "-- " documents
the key below it. Types and allowed values come from the schema, because
values.yaml cannot express them — a key defaulting to "" says nothing about
what it will accept.

--write replaces the block between the two marker comments, creating the
README and the markers when they are absent. Everything outside them is left
exactly as it was.`,
		Args: cobra.NoArgs,
		Example: `  hck docs
  hck docs --write
  hck docs --check`,
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
			values, err := c.Values()
			if err != nil {
				return err
			}
			if len(bytes.TrimSpace(values)) == 0 {
				return fmt.Errorf("%s has no values.yaml to document", c.Meta.Name)
			}

			// Prefer the schema the chart ships; fall back to one built from
			// the resources it carries, so a chart that never opted into
			// values.schema.json still gets types and allowed values.
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

			out := cmd.OutOrStdout()
			p := newPainter(out)

			switch {
			case opts.check:
				current, err := os.ReadFile(c.ReadmePath())
				if err != nil {
					return fmt.Errorf("%s has no README.md — run: hck docs --write", c.Meta.Name)
				}
				want, err := docs.Replace(current, table)
				if err != nil {
					return err
				}
				if !bytes.Equal(current, want) {
					return fmt.Errorf("README.md values table is out of date — run: hck docs --write")
				}
				fprintf(out, "  %s README.md values table is up to date\n", p.green("ok"))
				return nil

			case opts.write:
				current, err := os.ReadFile(c.ReadmePath())
				if err != nil {
					current = []byte(docs.Skeleton(c.Meta.Name, c.Meta.Description))
				}
				next, err := docs.Replace(current, table)
				if err != nil {
					return err
				}
				if err := os.WriteFile(c.ReadmePath(), next, 0o644); err != nil {
					return fmt.Errorf("write README.md: %w", err)
				}
				fprintf(out, "%s %s\n\n", p.bold("wrote"), c.ReadmePath())
				fprintf(out, "  %d value(s) documented\n", strings.Count(table, "\n")-2)
				return nil

			default:
				_, err := io.WriteString(out, table)
				return err
			}
		},
	}

	cmd.Flags().StringVar(&opts.chartDir, "chart", ".", "chart directory; parent directories are searched for Chart.yaml")
	cmd.Flags().BoolVar(&opts.write, "write", false, "write the table into the chart's README.md")
	cmd.Flags().BoolVar(&opts.check, "check", false, "fail when the README's table differs from what would be generated")
	return cmd
}
