package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "tcrit [file]",
	Version: versionString(),
	Short:   "Review markdown documents from the terminal",
	Long:    "TCrit is a terminal-based review tool for documents. It provides an interactive TUI for humans and scriptable CLI commands for automation.\n\nRun `tcrit <file>` to start a review (shortcut for `tcrit review <file>`).",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return reviewCmd.RunE(cmd, args)
	},
}

func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		return 3
	}
	return 0
}

func exitWithError(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "tcrit: "+msg+"\n", args...)
}
