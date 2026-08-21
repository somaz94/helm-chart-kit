package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/somaz94/helm-chart-kit/internal/catalog"
	"github.com/somaz94/helm-chart-kit/internal/check"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:       "list [presets|resources|rules]",
		Short:     "List the presets, resources and check rules hck knows",
		Args:      cobra.OnlyValidArgs,
		ValidArgs: []string{"presets", "resources", "rules"},
		Long: `List what hck knows: the presets, the resource catalog and the check rules.

--format json emits the same content as a document whose field names are part
of the interface. The default output is a table for people, and its columns,
indentation and markers are not: a script reading them breaks the next time a
column is added.`,
		Example: `  hck list
  hck list presets
  hck list resources
  hck list rules
  hck list resources --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			what := "all"
			if len(args) == 1 {
				what = args[0]
			}
			out := cmd.OutOrStdout()
			if format == formatJSON {
				return printListJSON(out, what)
			}
			if format != formatText {
				return fmt.Errorf("unknown format %q (known: %s, %s)", format, formatText, formatJSON)
			}
			p := newPainter(out)

			if what == "all" || what == "presets" {
				fprintf(out, "%s\n", p.bold("PRESETS"))
				w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				for _, ps := range catalog.Presets() {
					fmt.Fprintf(w, "  %s\t%s\n", ps.Name, ps.Summary)
					fmt.Fprintf(w, "  \t%s\n", p.dim(strings.Join(ps.Resources, " ")))
				}
				_ = w.Flush()
			}

			if what == "all" {
				fprintf(out, "\n")
			}

			if what == "all" || what == "resources" {
				fprintf(out, "%s\n", p.bold("RESOURCES"))
				// Grouped rather than alphabetical: the names are Kubernetes
				// kinds, so a flat list answers "what exists" and leaves
				// "which of these do I want" to somebody who already knows.
				// The blank line before each group header ends the
				// tabwriter's current cell block, so every group sizes its
				// own columns. That is the readable outcome and not an
				// accident: @workload would otherwise carry a column wide
				// enough for gateway.networking.k8s.io/v1beta1.
				w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				for _, g := range catalog.Groups() {
					fmt.Fprintf(w, "\n  %s\t\t%s\n", p.bold("@"+string(g.Name)), p.dim(g.Summary))
					for _, r := range catalog.ResourcesInGroup(g.Name) {
						mark := ""
						if r.Optional {
							mark = p.yellow(" [crd]")
						}
						// The platform marker comes after [crd] because it is
						// the harder constraint: a CRD can be installed, a
						// GKE-only kind cannot be made to exist on EKS.
						if r.Platform != "" {
							mark += p.yellow(" [" + r.Platform + "]")
						}
						fmt.Fprintf(w, "    %s\t%s\t%s%s\n", r.Name, p.dim(r.APIVersion), r.Summary, mark)
					}
				}
				_ = w.Flush()
				fprintf(out, "\n  %s needs a CRD or feature the cluster may not have\n", p.yellow("[crd]"))
				fprintf(out, "  %s adds the whole group: hck add @observability\n", p.bold("@name"))
				fprintf(out, "  %s exists on that platform only, and is never pulled in by a group\n",
					p.yellow("[platform]"))
			}

			if what == "all" {
				fprintf(out, "\n")
			}

			if what == "all" || what == "rules" {
				fprintf(out, "%s\n", p.bold("CHECK RULES"))
				w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				for _, r := range check.Rules() {
					sev := p.yellow(string(r.Severity))
					if r.Severity == check.Error {
						sev = p.red(string(r.Severity))
					}
					fmt.Fprintf(w, "  %s\t%s\t%s\n", r.ID, sev, r.Summary)
				}
				_ = w.Flush()
				fprintf(out, "\n  Turn one off or change its severity in the chart's %s:\n", p.dim(check.ConfigFile))
				fprintf(out, "  %s\n", p.dim("rules:"))
				fprintf(out, "  %s\n", p.dim(`  "`+check.WildcardRule+`": off    # every rule that can be`))
				fprintf(out, "  %s\n", p.dim("  HCK025: off"))
				fprintf(out, "\n  Or for one run, without editing the chart: %s\n", p.dim("hck check --off HCK025"))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", formatText, "output format: text or json")
	return cmd
}

// jsonListing is the machine-readable shape of "hck list". The field names are
// part of the interface — a CI step reading "resources" should keep working,
// which is the whole reason this exists: the table above is for people, and
// changing its indentation once already broke a workflow parsing it with awk.
type jsonListing struct {
	Groups    []jsonGroup    `json:"groups,omitempty"`
	Presets   []jsonPreset   `json:"presets,omitempty"`
	Resources []jsonResource `json:"resources,omitempty"`
	Rules     []jsonRule     `json:"rules,omitempty"`
}

type jsonGroup struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

type jsonPreset struct {
	Name      string   `json:"name"`
	Summary   string   `json:"summary"`
	Resources []string `json:"resources"`
	// What the preset answers beyond its resource list, and what "hck init"
	// reads. Empty and false are the ordinary case.
	Platform    string `json:"platform,omitempty"`
	Environment string `json:"environment,omitempty"`
	Schema      bool   `json:"schema"`
	Docs        bool   `json:"docs"`
}

type jsonResource struct {
	Name       string   `json:"name"`
	Group      string   `json:"group"`
	File       string   `json:"file"`
	APIVersion string   `json:"apiVersion"`
	Summary    string   `json:"summary"`
	ValuesKeys []string `json:"valuesKeys"`
	Requires   []string `json:"requires,omitempty"`
	// Platform is the one cloud this resource exists on, and "" for the ones
	// that work anywhere. A group never expands to a resource that has one.
	Platform string `json:"platform,omitempty"`
	Optional bool   `json:"optional"`
	Workload bool   `json:"workload"`
}

type jsonRule struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	// Locked rules cannot be turned off, by .hck.yaml or by --off.
	Locked bool `json:"locked"`
}

// printListJSON writes the listing as JSON. "all" carries every section;
// naming one carries that section and leaves the others out entirely rather
// than emitting them empty, so a consumer can tell "none" from "not asked".
func printListJSON(out io.Writer, what string) error {
	var doc jsonListing
	if what == "all" || what == "resources" {
		doc.Groups = make([]jsonGroup, 0, len(catalog.Groups()))
		for _, g := range catalog.Groups() {
			doc.Groups = append(doc.Groups, jsonGroup{Name: string(g.Name), Summary: g.Summary})
		}
		doc.Resources = make([]jsonResource, 0, len(catalog.Resources()))
		for _, r := range catalog.Resources() {
			doc.Resources = append(doc.Resources, jsonResource{
				Name: r.Name, Group: string(r.Group), File: r.File,
				APIVersion: r.APIVersion, Summary: r.Summary,
				ValuesKeys: r.ValuesKeys, Requires: r.Requires,
				Platform: r.Platform, Optional: r.Optional, Workload: r.Workload,
			})
		}
	}
	if what == "all" || what == "presets" {
		doc.Presets = make([]jsonPreset, 0, len(catalog.Presets()))
		for _, ps := range catalog.Presets() {
			doc.Presets = append(doc.Presets, jsonPreset{
				Name: ps.Name, Summary: ps.Summary, Resources: ps.Resources,
				Platform: ps.Platform, Environment: ps.Environment,
				Schema: ps.Schema, Docs: ps.Docs,
			})
		}
	}
	if what == "all" || what == "rules" {
		doc.Rules = make([]jsonRule, 0, len(check.Rules()))
		for _, r := range check.Rules() {
			doc.Rules = append(doc.Rules, jsonRule{
				ID: r.ID, Severity: string(r.Severity), Summary: r.Summary, Locked: r.Locked,
			})
		}
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// groupArgs is the group names as completion candidates, "@" included, so
// that tab-completion offers them beside the resource names.
func groupArgs() []string {
	names := catalog.GroupNames()
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, "@"+n)
	}
	return out
}
