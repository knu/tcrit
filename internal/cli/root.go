package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/knu/tcrit/internal/config"
	"github.com/knu/tcrit/internal/ipc"
	"github.com/knu/tcrit/internal/review"
)

var rootSession string

var rootCmd = &cobra.Command{
	Use:     "tcrit [file]",
	Version: versionString(),
	Short:   "Review code changes and documents from the terminal",
	Long: "TCrit is a terminal-based review tool for code changes and documents. " +
		"It provides an interactive TUI for humans and scriptable CLI commands for agents.\n\n" +
		"Run `tcrit` to review the current git changes, `tcrit <file>` to review a document, " +
		"or `tcrit --session <id>` to reconnect to a running review and start the next round.",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if rootSession != "" {
			if len(args) > 0 {
				return fmt.Errorf("--session cannot be combined with a file argument")
			}
			return reconnectSession(rootSession)
		}
		return runReview(args)
	},
}

// reconnectSession starts the next review round against a running session.
func reconnectSession(key string) error {
	if !review.ValidSessionKey(key) {
		return fmt.Errorf("invalid session ID %q", key)
	}
	cfg, err := config.LoadCurrent()
	if err != nil {
		return err
	}
	entry, err := review.ReadSessionEntry(key)
	if err != nil {
		return err
	}
	sess, err := review.OpenSessionFromEntry(*entry)
	if err != nil {
		return err
	}
	sock := review.SocketPathFor(key)
	if !ipc.Alive(sock) {
		return fmt.Errorf("review session %s is not running; start a new review with `tcrit`", key)
	}
	return runReviewCycle(cfg, sess, sock)
}

func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		return 1
	}
	return 0
}

func init() {
	rootCmd.Flags().StringVar(&rootSession, "session", "", "reconnect to a running review session by ID")
}
