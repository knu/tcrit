package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/knu/tcrit/internal/config"
	"github.com/knu/tcrit/internal/git"
	"github.com/knu/tcrit/internal/ipc"
	"github.com/knu/tcrit/internal/review"
)

var reviewCode bool
var reviewBase string

// The following function variables allow tests to replace shell interactions
// without actually shelling out.
var runCommand = func(cmd *exec.Cmd) error {
	return cmd.Run()
}

var lookPath = exec.LookPath

var commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
	return cmd.Output()
}

var parentProcessID = os.Getppid

var inspectProcess = inspectParentProcess

var resolveExec = func() (string, error) {
	return resolveExecutable()
}

var reviewCmd = &cobra.Command{
	Use:   "review [file]",
	Short: "Review git changes (default) or a single document",
	Long: `Open a review and block until the human finishes it.

With no file argument, reviews the current git changes (multi-file mode).
With a file argument, reviews that document.

Inside Herdr the TUI opens in a dedicated tab; inside tmux it opens in a
split pane. This command blocks until the reviewer approves or finishes
with comments, printing the resulting agent prompt on stdout and
"approved: true|false" on stderr. Outside a multiplexer the TUI runs in
the current terminal.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runReview(args)
	},
}

// reviewMode describes what is being reviewed.
type reviewMode struct {
	docPath  string // non-empty for single-document and plan modes
	ref      string // diff base for code mode
	files    []git.FileChange
	planSlug string // non-empty for plan mode
	planFile string // original plan path ("" when read from stdin)
}

func (m *reviewMode) code() bool { return m.docPath == "" }

func (m *reviewMode) plan() bool { return m.planSlug != "" }

func (m *reviewMode) promptMode() string {
	if m.code() {
		return "diff"
	}
	return "files"
}

func (m *reviewMode) internalMode() string {
	switch {
	case m.plan():
		return "plan"
	case m.code():
		return "git"
	default:
		return "files"
	}
}

func runReview(args []string) error {
	review.WarnLegacyState()

	cfg, err := config.LoadCurrent()
	if err != nil {
		return err
	}

	mode, err := resolveReviewMode(args, cfg)
	if err != nil {
		return err
	}

	sess, err := openReviewSession(cfg, mode)
	if err != nil {
		return err
	}

	return runReviewFlow(cfg, sess, mode)
}

// runReviewFlow connects to a live session, opens the TUI in the caller's
// terminal multiplexer, or runs it inline, then handles the finish result.
func runReviewFlow(cfg *config.Config, sess *review.Session, mode *reviewMode) error {
	sock := review.SocketPathFor(sess.Key)
	if ipc.Alive(sock) {
		return runReviewCycle(cfg, sess, sock)
	}

	if multiplexer := findMultiplexerContext(); multiplexer != nil {
		launch, err := multiplexer.launchReview(mode)
		if err != nil {
			return err
		}
		defer launch.restoreFocus()
		if err := ipc.WaitAlive(sock, 15*time.Second); err != nil {
			launch.close()
			return err
		}
		return runReviewCycle(cfg, sess, sock)
	}

	if term.IsTerminal(int(os.Stdin.Fd())) {
		payload, err := runTUISession(cfg, sess, mode, false)
		if err != nil {
			return err
		}
		if payload == nil {
			fmt.Fprintln(os.Stderr, "approved: false")
			return fmt.Errorf("review ended without finishing")
		}
		printFinish(payload)
		if payload.Approved {
			cleanupOnApprove(cfg, sess)
		}
		return nil
	}

	return fmt.Errorf("no Herdr or tmux session and no terminal to open the TUI in; run inside Herdr or tmux, or have the reviewer run `tcrit%s` in a terminal", reviewArgSuffix(mode))
}

func reviewArgSuffix(mode *reviewMode) string {
	if mode.code() {
		return ""
	}
	return " " + mode.docPath
}

// resolveReviewMode classifies the arguments and, for code mode, detects
// the changed files up front so failures surface before any TUI spawns.
func resolveReviewMode(args []string, cfg *config.Config) (*reviewMode, error) {
	if len(args) == 1 && !reviewCode {
		filePath := args[0]
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", filePath)
		}
		return &reviewMode{docPath: filePath}, nil
	}

	if !git.IsGitRepo() {
		return nil, fmt.Errorf("code review requires a git repository (pass a file argument to review a document)")
	}

	base := reviewBase
	if base == "" {
		base = cfg.BaseBranch
	}

	var ref string
	var files []git.FileChange
	var err error
	if base != "" {
		ref = base
		files, err = git.ChangedFilesFrom(ref)
		if err != nil {
			return nil, fmt.Errorf("detecting changed files from %s: %w", ref, err)
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("no changed files found relative to %s", ref)
		}
	} else {
		ref = "HEAD"
		files, err = git.ChangedFiles()
		if err != nil {
			return nil, fmt.Errorf("detecting changed files: %w", err)
		}
		if len(files) == 0 {
			ref, files, err = fallbackRef()
			if err != nil {
				return nil, err
			}
		}
	}
	return &reviewMode{ref: ref, files: files}, nil
}

func openReviewSession(cfg *config.Config, mode *reviewMode) (*review.Session, error) {
	if !mode.code() {
		sess, err := review.OpenDocSession(cfg.Output, mode.docPath)
		if err != nil {
			return nil, fmt.Errorf("loading review state: %w", err)
		}
		return sess, nil
	}

	sess, err := review.OpenCodeSession(cfg.Output)
	if err != nil {
		return nil, fmt.Errorf("loading review state: %w", err)
	}
	sess.CJ.BaseRef = mode.ref
	for _, f := range mode.files {
		sess.SetFileComments(f.Path, f.Status.String(), sess.FileComments(f.Path))
	}
	if err := sess.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "tcrit: warning: could not save session: %v\n", err)
	}
	return sess, nil
}

// runReviewCycle blocks on the session socket until the reviewer finishes,
// then prints the agent-facing result and applies approval cleanup.
func runReviewCycle(cfg *config.Config, sess *review.Session, sock string) error {
	payload, err := ipc.ReviewCycle(sock)
	if err != nil {
		fmt.Fprintln(os.Stderr, "approved: false")
		return err
	}
	printFinish(payload)
	if payload.Approved {
		cleanupOnApprove(cfg, sess)
	}
	return nil
}

// printFinish writes the agent contract: the rendered prompt on stdout and
// the approval verdict on stderr.
func printFinish(payload *ipc.FinishPayload) {
	if payload.Approved {
		fmt.Fprintln(os.Stderr, "approved: true")
	} else {
		fmt.Fprintln(os.Stderr, "approved: false")
	}
	if payload.Prompt != "" {
		fmt.Println(payload.Prompt)
	}
}

func cleanupOnApprove(cfg *config.Config, sess *review.Session) {
	if !cfg.CleanupOnApprove {
		return
	}
	if err := sess.Clear(); err != nil {
		fmt.Fprintf(os.Stderr, "tcrit: warning: could not clean up review: %v\n", err)
	}
}

// spawnTUIPane opens the TUI in a tmux split pane running `tcrit _tui`.
func spawnTUIPane(mode *reviewMode, tmux tmuxContext) error {
	tmuxBin, err := lookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux binary not found on PATH: %w", err)
	}
	tuiCmd, err := buildTUICommand(mode)
	if err != nil {
		return err
	}

	splitCmd := tmuxCommand(tmuxBin, tmux, splitWindowArgs(true, tuiCmd, tmux.pane)...)
	if err := runCommand(splitCmd); err != nil {
		// Retry without -p — percentage sizing fails when the parent pane
		// size isn't available (e.g. invoked from an agent subprocess).
		splitCmd = tmuxCommand(tmuxBin, tmux, splitWindowArgs(false, tuiCmd, tmux.pane)...)
		if err := runCommand(splitCmd); err != nil {
			return fmt.Errorf("failed to open tmux pane: %w", err)
		}
	}
	fmt.Fprintln(os.Stderr, "Opened review in tmux pane")
	return nil
}

func buildTUICommand(mode *reviewMode) (string, error) {
	tcritBin, err := resolveExec()
	if err != nil {
		return "", fmt.Errorf("resolving tcrit binary path: %w", err)
	}

	// Multiplexer panes inherit their host's environment, not necessarily the
	// caller's, so pass through the variables that locate review state.
	envPrefix := "env TCRIT_DETACHED=1"
	for _, name := range []string{"XDG_STATE_HOME", "XDG_CONFIG_HOME"} {
		if val := os.Getenv(name); val != "" {
			envPrefix += " " + name + "=" + shellEscape(val)
		}
	}

	switch {
	case mode.plan():
		return fmt.Sprintf("%s %s _tui --plan %s",
			envPrefix, shellEscape(tcritBin), shellEscape(mode.planSlug)), nil
	case mode.code():
		return fmt.Sprintf("%s %s _tui --base %s",
			envPrefix, shellEscape(tcritBin), shellEscape(mode.ref)), nil
	default:
		absPath, err := filepath.Abs(mode.docPath)
		if err != nil {
			return "", fmt.Errorf("resolving absolute path: %w", err)
		}
		return fmt.Sprintf("%s %s _tui %s",
			envPrefix, shellEscape(tcritBin), shellEscape(absPath)), nil
	}
}

func fallbackRef() (string, []git.FileChange, error) {
	// Try common alternatives in order
	alternatives := []struct {
		label string
		ref   string
	}{
		{"last commit (HEAD~1)", "HEAD~1"},
		{"base branch (main)", "main"},
	}

	for _, alt := range alternatives {
		files, err := git.ChangedFilesFrom(alt.ref)
		if err != nil {
			continue
		}
		if len(files) > 0 {
			fmt.Fprintf(os.Stderr, "No unstaged changes found. Using %s.\n", alt.label)
			return alt.ref, files, nil
		}
	}

	return "", nil, fmt.Errorf("no changed files found")
}

// resolveExecutable returns the absolute path to the currently running binary.
func resolveExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// splitWindowArgs builds the tmux split-window arguments, targeting the
// invoking pane via TMUX_PANE and pinning the pane's working directory to
// the caller's so both sides derive the same session key.
func splitWindowArgs(withSize bool, tuiCmd, pane string) []string {
	args := []string{"split-window", "-h"}
	if pane != "" {
		args = append(args, "-t", pane)
	}
	if cwd, err := os.Getwd(); err == nil {
		args = append(args, "-c", cwd)
	}
	if withSize {
		args = append(args, "-p", "70")
	}
	return append(args, tuiCmd)
}

type tmuxContext struct {
	session string
	pane    string
}

type tmuxDetector struct{}

func (tmuxDetector) environmentPresent() bool {
	return os.Getenv("TMUX") != "" || os.Getenv("TMUX_PANE") != ""
}

func (tmuxDetector) environmentContext() reviewMultiplexerContext {
	ctx := tmuxContext{session: os.Getenv("TMUX"), pane: os.Getenv("TMUX_PANE")}
	if !ctx.active() {
		return nil
	}
	return ctx
}

func (tmuxDetector) processContexts() map[int]reviewMultiplexerContext {
	contexts := make(map[int]reviewMultiplexerContext)
	for pid, ctx := range tmuxProcessContexts() {
		contexts[pid] = ctx
	}
	return contexts
}

func (c tmuxContext) launchReview(mode *reviewMode) (reviewMultiplexerLaunch, error) {
	if err := spawnTUIPane(mode, c); err != nil {
		return nil, err
	}
	return tmuxLaunch{}, nil
}

func (tmuxContext) restoreFocus() {}

type tmuxLaunch struct{}

func (tmuxLaunch) close()        {}
func (tmuxLaunch) restoreFocus() {}

func (c tmuxContext) active() bool {
	return c.session != "" || c.pane != ""
}

func tmuxProcessContexts() map[int]tmuxContext {
	tmuxBin, err := lookPath("tmux")
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	panes := tmuxPIDContexts(ctx, tmuxBin, "list-panes", "-a", "-F", "#{pane_pid}\t#{socket_path}\t#{pid}\t#{session_id}\t#{pane_id}")
	clients := tmuxPIDContexts(ctx, tmuxBin, "list-clients", "-F", "#{client_pid}\t#{socket_path}\t#{pid}\t#{session_id}")
	if panes == nil {
		panes = make(map[int]tmuxContext)
	}
	for pid, ctx := range clients {
		if _, exists := panes[pid]; !exists {
			panes[pid] = ctx
		}
	}
	return panes
}

func tmuxPIDContexts(ctx context.Context, tmuxBin string, args ...string) map[int]tmuxContext {
	out, err := commandOutput(exec.CommandContext(ctx, tmuxBin, args...))
	if err != nil {
		return nil
	}
	contexts := make(map[int]tmuxContext)
	for line := range strings.Lines(string(out)) {
		fields := strings.Split(strings.TrimSuffix(line, "\n"), "\t")
		if len(fields) != 4 && len(fields) != 5 {
			continue
		}
		processID, err := strconv.Atoi(fields[0])
		if err != nil || processID < 2 {
			continue
		}
		socket, serverPID, sessionID := fields[1], fields[2], strings.TrimPrefix(fields[3], "$")
		if socket == "" || serverPID == "" || sessionID == "" {
			continue
		}
		ctx := tmuxContext{session: strings.Join([]string{socket, serverPID, sessionID}, ",")}
		if len(fields) == 5 {
			ctx.pane = fields[4]
		}
		if _, exists := contexts[processID]; !exists {
			contexts[processID] = ctx
		}
	}
	return contexts
}

func tmuxCommand(tmuxBin string, tmux tmuxContext, args ...string) *exec.Cmd {
	cmd := exec.Command(tmuxBin, args...)
	if os.Getenv("TMUX") == "" && tmux.session != "" {
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "TMUX=") {
				cmd.Env = append(cmd.Env, entry)
			}
		}
		cmd.Env = append(cmd.Env, "TMUX="+tmux.session)
	}
	return cmd
}

// shellEscape escapes a string for safe embedding in a POSIX shell command.
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func init() {
	rootCmd.AddCommand(reviewCmd)
	reviewCmd.Flags().BoolVar(&reviewCode, "code", false, "review code changes (default when no file argument is given)")
	reviewCmd.Flags().StringVar(&reviewBase, "base", "", "base ref to diff against in code mode")
	reviewCmd.Flags().StringVar(&reviewBase, "base-branch", "", "alias for --base")
	reviewCmd.Flags().MarkHidden("base-branch")

	// Deprecated no-ops: blocking on a tmux split pane is now the default.
	var deprecatedDetach, deprecatedWait bool
	reviewCmd.Flags().BoolVar(&deprecatedDetach, "detach", false, "deprecated: no-op")
	reviewCmd.Flags().BoolVar(&deprecatedWait, "wait", false, "deprecated: no-op")
	reviewCmd.Flags().MarkHidden("detach")
	reviewCmd.Flags().MarkHidden("wait")
}
