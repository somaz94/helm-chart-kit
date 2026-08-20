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
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "hck",
		Short: "Scaffold and extend Helm charts",
		Long: `hck (helm-chart-kit) scaffolds Helm charts and adds resources to existing ones.

Where "helm create" writes a fixed six-file chart and then leaves, hck keeps
working on a chart after it exists: "hck add" drops a new resource template in
and appends the values it needs, without rewriting a line of what is already
there.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		newNewCmd(),
		newAddCmd(),
		newListCmd(),
		newCheckCmd(),
		newDocsCmd(),
		newSchemaCmd(),
		newVersionCmd(),
	)
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
