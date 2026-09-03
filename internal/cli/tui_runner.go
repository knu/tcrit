package cli

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/knu/tcrit/internal/config"
	"github.com/knu/tcrit/internal/ipc"
	"github.com/knu/tcrit/internal/prompt"
	"github.com/knu/tcrit/internal/review"
	"github.com/knu/tcrit/internal/tui"
)

// runTUISession runs the review TUI in this process.  When serving, it
// listens on the session socket so blocking agent clients receive finish
// payloads and can start new rounds; otherwise it returns the last finish
// payload for the caller to print (inline mode).
func runTUISession(cfg *config.Config, sess *review.Session, mode *reviewMode, serving bool) (*ipc.FinishPayload, error) {
	finishCh := make(chan tui.FinishEvent, 4)
	appCfg := tui.AppConfig{
		Session:  sess,
		Author:   cfg.Author,
		Serving:  serving,
		FinishCh: finishCh,
	}

	var model tui.AppModel
	if mode.code() {
		model = tui.NewCodeReviewApp(mode.files, mode.ref, appCfg)
	} else {
		model = tui.NewApp(mode.docPath, appCfg)
	}
	p := tea.NewProgram(model)

	srv := &tuiServer{cfg: cfg, sess: sess, mode: mode, program: p}

	if serving {
		sock := review.SocketPathFor(sess.Key)
		if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
			return nil, fmt.Errorf("creating sessions dir: %w", err)
		}
		ln, err := ipc.Listen(sock)
		if err != nil {
			return nil, err
		}
		defer func() {
			ln.Close()
			os.Remove(sock)
		}()

		sess.Meta.PID = os.Getpid()
		sess.Meta.SocketPath = sock
		if err := sess.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "tcrit: warning: could not register session: %v\n", err)
		}

		go srv.acceptLoop(ln)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range finishCh {
			srv.handleFinish(ev.Approved)
		}
	}()

	_, runErr := p.Run()
	close(finishCh)
	<-done
	srv.closeWaiters()
	if runErr != nil {
		return nil, fmt.Errorf("TUI error: %w", runErr)
	}
	return srv.lastPayload, nil
}

// tuiServer coordinates agent connections with the TUI program.
type tuiServer struct {
	cfg     *config.Config
	sess    *review.Session
	mode    *reviewMode
	program *tea.Program

	mu          sync.Mutex
	waiters     []net.Conn
	finishCount int
	lastPayload *ipc.FinishPayload
}

func (s *tuiServer) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *tuiServer) handleConn(conn net.Conn) {
	req, err := ipc.ReadRequest(bufio.NewReader(conn))
	if err != nil || req.Type != "review-cycle" {
		conn.Close()
		return
	}
	focusCurrentHerdrTab()
	s.mu.Lock()
	startRound := s.finishCount > 0
	s.waiters = append(s.waiters, conn)
	s.mu.Unlock()
	// The first review cycle reviews the session as loaded; later cycles
	// mean the agent finished a round of fixes, so reload and advance.
	if startRound {
		s.program.Send(tui.RoundStartMsg{})
	}
}

// handleFinish builds the finish payload and delivers it to every blocked
// client.  It runs on the runner goroutine after the TUI persisted the
// session, so reading sess.CJ here does not race with the model.
func (s *tuiServer) handleFinish(approved bool) {
	payload := buildFinishPayload(s.cfg, s.sess, s.mode, approved)

	s.mu.Lock()
	s.finishCount++
	waiters := s.waiters
	s.waiters = nil
	s.lastPayload = &payload
	s.mu.Unlock()

	for _, conn := range waiters {
		_ = ipc.WriteMessage(conn, payload)
		conn.Close()
	}
}

func (s *tuiServer) closeWaiters() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, conn := range s.waiters {
		conn.Close()
	}
	s.waiters = nil
}

// buildFinishPayload assembles the agent-facing finish result, rendering
// the prompt through the template chain.
func buildFinishPayload(cfg *config.Config, sess *review.Session, mode *reviewMode, approved bool) ipc.FinishPayload {
	unresolved := sess.CJ.ListComments(true)
	all := sess.CJ.ListComments(false)

	unresolvedJSON := ""
	if len(unresolved) > 0 {
		if data, err := review.EncodeCommentsJSON(unresolved); err == nil {
			unresolvedJSON = string(data)
		}
	}
	allJSON := ""
	if data, err := review.EncodeCommentsJSON(all); err == nil {
		allJSON = string(data)
	}

	seen := map[string]bool{}
	var filesWithComments []string
	for _, c := range unresolved {
		if c.Path != nil && !seen[*c.Path] {
			seen[*c.Path] = true
			filesWithComments = append(filesWithComments, *c.Path)
		}
	}

	nextCmd := ""
	if !approved {
		nextCmd = nextRoundCommand(sess, mode)
	}

	ctx := prompt.Context{
		ReviewPath:        sess.Path(),
		SessionKey:        sess.Key,
		Mode:              mode.promptMode(),
		InternalMode:      mode.internalMode(),
		PlanSlug:          mode.planSlug,
		UnresolvedCount:   len(unresolved),
		TotalCount:        len(all),
		FilesWithComments: filesWithComments,
		UnresolvedJSON:    unresolvedJSON,
		CommentsJSON:      allJSON,
		Approved:          approved,
		NextRoundCmd:      nextCmd,
	}

	comments := unresolved
	if approved {
		comments = nil
	}
	return ipc.FinishPayload{
		Type:        "finish",
		Approved:    approved,
		Prompt:      prompt.RenderFinish(cfg.Prompts, cfg.ProjectRoot, ctx),
		Comments:    comments,
		NextCommand: nextCmd,
	}
}

// nextRoundCommand builds the command the agent runs to start the next
// round.  Plan sessions reconnect through `tcrit plan` so the revised plan
// content is versioned; the original file path is recovered from the
// session's recorded cli_args when this process was spawned without it.
func nextRoundCommand(sess *review.Session, mode *reviewMode) string {
	if !mode.plan() {
		return "tcrit --session " + sess.Key
	}
	planFile := mode.planFile
	if planFile == "" && len(sess.CJ.CliArgs) >= 4 && sess.CJ.CliArgs[0] == "plan" {
		planFile = sess.CJ.CliArgs[3]
	}
	cmd := "tcrit plan --name " + mode.planSlug
	if planFile != "" {
		cmd += " " + planFile
	}
	return cmd
}
