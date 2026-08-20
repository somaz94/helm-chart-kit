package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// NewRootCmd builds the command tree.
//
// A constructor rather than a package-level variable: cobra binds flags to
// addresses, so a shared tree carries flag values from one run into the next.
// That is invisible in a binary that runs one command and exits, and it makes
// the commands untestable.
// Command groups. Twelve commands listed alphabetically tell a first-time
// reader nothing about where to start — "add" sorts above "init", and cobra's
// own "completion" carries the same weight as "new". Grouping puts the three
// commands most people ever need at the top and lets the rest recede.
const (
	groupStart  = "start"
	groupChart  = "chart"
	groupValues = "values"
	groupLook   = "look"
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "hck",
		Short: "Scaffold and extend Helm charts",
		Long: `hck (helm-chart-kit) scaffolds Helm charts and adds resources to existing ones.

Where "helm create" writes a fixed six-file chart and then leaves, hck keeps
working on a chart after it exists: "hck add" drops a new resource template in
and appends the values it needs, without rewriting a line of what is already
there.

New here? Run "hck init" — it asks a few questions, scaffolds the chart, and
prints the equivalent flags for next time. Most work is three commands:

    hck new <name>        create a chart
    hck add <resource>    add to one that exists
    hck check             render it and apply the house rules

Everything else is opt-in: a chart that never asks for a schema, a values
table or a platform overlay does not get one.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddGroup(
		&cobra.Group{ID: groupStart, Title: "Getting started:"},
		&cobra.Group{ID: groupChart, Title: "Working on a chart:"},
		&cobra.Group{ID: groupValues, Title: "Values, on top of the chart:"},
		&cobra.Group{ID: groupLook, Title: "Looking around:"},
	)

	for group, cmds := range map[string][]*cobra.Command{
		groupStart:  {newInitCmd(), newNewCmd()},
		groupChart:  {newAddCmd(), newRemoveCmd(), newSyncCmd(), newCheckCmd()},
		groupValues: {newSchemaCmd(), newDocsCmd(), newPlatformCmd(), newEnvCmd()},
		groupLook:   {newListCmd(), newVersionCmd()},
	} {
		for _, c := range cmds {
			c.GroupID = group
			root.AddCommand(c)
		}
	}
	return root
}

// Execute runs the root command.
func Execute() error {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return err
	}
	return nil
}
