package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/knu/tcrit/internal/config"
	"github.com/knu/tcrit/internal/review"
)

var planName string

var planCmd = &cobra.Command{
	Use:   "plan [--name <slug>] [file]",
	Short: "Create or continue a versioned plan review",
	Long: `Create or continue a plan review.  The plan content (from the file
argument or piped stdin) is saved as a new immutable version under the
plan's storage directory, and a review of the latest version opens,
blocking like ` + "`tcrit review`" + `.

Without --name, the slug is derived from the plan's first heading.
Re-running with the same slug saves a new version and starts the next
review round.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPlan(args)
	},
}

func runPlan(args []string) error {
	review.WarnLegacyState()

	cfg, err := config.LoadCurrent()
	if err != nil {
		return err
	}

	content, sourceFile, err := readPlanContent(args)
	if err != nil {
		return err
	}

	slug := review.Slugify(planName)
	if slug == "" {
		slug = review.ResolveSlug(content)
		fmt.Fprintf(os.Stderr, "No --name provided, derived slug: %s\n", slug)
	}

	ver, err := review.SavePlanVersion(slug, content)
	if err != nil {
		return err
	}
	if !cfg.Quiet {
		fmt.Fprintf(os.Stderr, "Plan '%s' saved as v%03d (%d bytes)\n", slug, ver, len(content))
	}

	sess, err := review.OpenPlanSession(slug)
	if err != nil {
		return err
	}
	cliArgs := []string{"plan", "--name", slug}
	if sourceFile != "" {
		cliArgs = append(cliArgs, sourceFile)
	}
	sess.CJ.CliArgs = cliArgs
	if err := sess.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "tcrit: warning: could not save session: %v\n", err)
	}

	mode := &reviewMode{
		docPath:  review.PlanCurrentPath(slug),
		planSlug: slug,
		planFile: sourceFile,
	}
	return runReviewFlow(cfg, sess, mode)
}

// readPlanContent reads the plan from the file argument or piped stdin.
func readPlanContent(args []string) (content []byte, sourceFile string, err error) {
	if len(args) == 1 {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return nil, "", fmt.Errorf("reading plan: %w", err)
		}
		if len(data) == 0 {
			return nil, "", fmt.Errorf("plan file %s is empty", args[0])
		}
		return data, args[0], nil
	}

	if term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, "", fmt.Errorf("usage: tcrit plan [--name <slug>] <file>  (or pipe the plan on stdin)")
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, "", fmt.Errorf("reading stdin: %w", err)
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("no plan content on stdin")
	}
	return data, "", nil
}

func init() {
	rootCmd.AddCommand(planCmd)
	planCmd.Flags().StringVar(&planName, "name", "", "plan slug (derived from the first heading when omitted)")
}
