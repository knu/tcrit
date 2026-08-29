package cli

import (
	"github.com/spf13/cobra"

	"github.com/knu/tcrit/internal/config"
	"github.com/knu/tcrit/internal/review"
)

var tuiPlan string

// tuiCmd is the internal command run inside the tmux split pane: it owns
// the review session, serves the session socket, and stays across rounds.
var tuiCmd = &cobra.Command{
	Use:    "_tui [file]",
	Short:  "Run the internal review TUI server",
	Hidden: true,
	Args:   cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadCurrent()
		if err != nil {
			return err
		}

		var mode *reviewMode
		var sess *review.Session
		if tuiPlan != "" {
			sess, err = review.OpenPlanSession(tuiPlan)
			if err != nil {
				return err
			}
			mode = &reviewMode{
				docPath:  review.PlanCurrentPath(tuiPlan),
				planSlug: tuiPlan,
			}
		} else {
			mode, err = resolveReviewMode(args, cfg)
			if err != nil {
				return err
			}
			sess, err = openReviewSession(cfg, mode)
			if err != nil {
				return err
			}
		}
		_, err = runTUISession(cfg, sess, mode, true)
		return err
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
	tuiCmd.Flags().StringVar(&reviewBase, "base", "", "base ref to diff against in code mode")
	tuiCmd.Flags().StringVar(&tuiPlan, "plan", "", "plan slug to review")
}
