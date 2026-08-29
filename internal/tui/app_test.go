package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/knu/tcrit/internal/document"
	gitpkg "github.com/knu/tcrit/internal/git"
	"github.com/knu/tcrit/internal/review"
)

// setupSession points XDG state at a temp dir and opens a doc session there.
func setupSession(t *testing.T, docPath string) *review.Session {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	sess, err := review.OpenDocSession("", docPath)
	if err != nil {
		t.Fatalf("opening session: %v", err)
	}
	return sess
}

func setupAppWithDoc(t *testing.T, content string) AppModel {
	t.Helper()
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	app := NewApp(testFile, AppConfig{Session: setupSession(t, testFile), Author: "Tester"})
	app.tabs = []FileTab{{path: testFile}}
	app.activeTab = 0
	updated, _ := app.Update(docRenderedMsg{})
	a := updated.(AppModel)
	a.contentViewport.SetHeight(5)
	return a
}

func pressKey(app AppModel, code rune) AppModel {
	updated, _ := app.Update(tea.KeyPressMsg{Code: code})
	switch v := updated.(type) {
	case AppModel:
		return v
	case *AppModel:
		return *v
	}
	panic("unexpected model type")
}

func TestNavigationHome(t *testing.T) {
	lines := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	app := setupAppWithDoc(t, lines)
	app.tabs[0].cursorLine = 8

	app = pressKey(app, tea.KeyHome)

	if app.tabs[0].cursorLine != 1 {
		t.Errorf("Home: expected cursorLine 1, got %d", app.tabs[0].cursorLine)
	}
}

func TestNavigationEnd(t *testing.T) {
	lines := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	app := setupAppWithDoc(t, lines)

	app = pressKey(app, tea.KeyEnd)

	tab := app.tabs[0]
	if tab.cursorLine != tab.doc.LineCount() {
		t.Errorf("End: expected cursorLine %d, got %d", tab.doc.LineCount(), tab.cursorLine)
	}
}

func TestNavigationPgDown(t *testing.T) {
	lines := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	app := setupAppWithDoc(t, lines)
	app.tabs[0].cursorLine = 1

	app = pressKey(app, tea.KeyPgDown)

	tab := app.tabs[0]
	expected := 1 + app.contentViewport.Height()/2
	if tab.cursorLine != expected {
		t.Errorf("PgDown: expected cursorLine %d, got %d", expected, tab.cursorLine)
	}
}

func TestNavigationPgUp(t *testing.T) {
	lines := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	app := setupAppWithDoc(t, lines)
	app.tabs[0].cursorLine = 3

	app = pressKey(app, tea.KeyPgUp)

	tab := app.tabs[0]
	if tab.cursorLine != 1 {
		t.Errorf("PgUp: expected cursorLine 1 (clamped), got %d", tab.cursorLine)
	}
}

func TestNavigationPgDownClampsAtBottom(t *testing.T) {
	lines := "line1\nline2\nline3\n"
	app := setupAppWithDoc(t, lines)
	app.tabs[0].cursorLine = 3

	app = pressKey(app, tea.KeyPgDown)

	tab := app.tabs[0]
	if tab.cursorLine != tab.doc.LineCount() {
		t.Errorf("PgDown clamp: expected cursorLine %d, got %d", tab.doc.LineCount(), tab.cursorLine)
	}
}

func TestNewApp(t *testing.T) {
	app := NewApp("test.md", AppConfig{})
	if app.filePath != "test.md" {
		t.Errorf("expected filePath 'test.md', got %s", app.filePath)
	}
}

func newCommentNavigationTestApp() AppModel {
	app := NewApp("first.go", AppConfig{})
	lines := []string{"one", "two", "three", "four"}
	tab := func(path string, comments ...review.Comment) FileTab {
		return FileTab{
			path:       path,
			cursorLine: 1,
			doc: &document.Document{
				Path: path, Content: strings.Join(lines, "\n"), Lines: lines,
			},
			state: &fileReview{Comments: comments},
		}
	}
	comment := func(id string, line int) review.Comment {
		return review.Comment{ID: id, StartLine: line, EndLine: line, Body: id}
	}
	app.tabs = []FileTab{
		tab("first.go", comment("first-a", 2), comment("first-b", 2), comment("first-c", 4)),
		tab("empty.go"),
		tab("last.go", comment("last-a", 1), comment("last-b", 3)),
	}
	app.multiFile = true
	app.contentViewport.SetWidth(80)
	app.contentViewport.SetHeight(20)
	return app
}

func TestCommentNavigationCrossesFiles(t *testing.T) {
	app := newCommentNavigationTestApp()
	app.tabs[0].cursorLine = 4
	app.tabs[0].cursorOnAnnotation = true

	app = pressKey(app, ']')

	if app.activeTab != 2 || app.tab().cursorLine != 1 || !app.tab().cursorOnAnnotation {
		t.Fatalf("next comment = tab %d, line %d; want tab 2, line 1", app.activeTab, app.tab().cursorLine)
	}

	app = pressKey(app, '[')

	if app.activeTab != 0 || app.tab().cursorLine != 4 || !app.tab().cursorOnAnnotation {
		t.Fatalf("previous comment = tab %d, line %d; want tab 0, line 4", app.activeTab, app.tab().cursorLine)
	}
}

func TestCommentNavigationWrapsReviewAndVisitsSameLineThreads(t *testing.T) {
	app := newCommentNavigationTestApp()
	app.tabs[0].cursorLine = 2

	app = pressKey(app, ']')
	if app.activeTab != 0 || app.tab().cursorAnnoIdx != 0 {
		t.Fatalf("first comment = tab %d, annotation %d; want tab 0, annotation 0", app.activeTab, app.tab().cursorAnnoIdx)
	}

	app = pressKey(app, ']')
	if app.activeTab != 0 || app.tab().cursorAnnoIdx != 1 {
		t.Fatalf("second comment = tab %d, annotation %d; want tab 0, annotation 1", app.activeTab, app.tab().cursorAnnoIdx)
	}

	app.activeTab = 2
	app.tabs[2].cursorLine = 3
	app.tabs[2].cursorOnAnnotation = true
	app.tabs[2].cursorAnnoIdx = 0
	app = pressKey(app, ']')
	if app.activeTab != 0 || app.tab().cursorLine != 2 || app.tab().cursorAnnoIdx != 0 {
		t.Fatalf("wrapped next = tab %d, line %d, annotation %d", app.activeTab, app.tab().cursorLine, app.tab().cursorAnnoIdx)
	}

	app = pressKey(app, '[')
	if app.activeTab != 2 || app.tab().cursorLine != 3 {
		t.Fatalf("wrapped previous = tab %d, line %d; want tab 2, line 3", app.activeTab, app.tab().cursorLine)
	}
}

func newChangeNavigationTestApp() AppModel {
	app := newCommentNavigationTestApp()
	app.tabs[0].changeChunks = []changeChunk{
		{startLine: 2, endLine: 2},
		{startLine: 4, endLine: 4},
	}
	app.tabs[2].changeChunks = []changeChunk{
		{startLine: 1, endLine: 1},
		{startLine: 3, endLine: 3},
	}
	return app
}

func TestChangeNavigationCrossesFilesWithoutWrapping(t *testing.T) {
	app := newChangeNavigationTestApp()
	app.tabs[0].cursorLine = 4

	app = pressKey(app, 'n')
	if app.activeTab != 2 || app.tab().cursorLine != 1 {
		t.Fatalf("next change = tab %d, line %d; want tab 2, line 1", app.activeTab, app.tab().cursorLine)
	}

	app = pressKey(app, 'N')
	if app.activeTab != 0 || app.tab().cursorLine != 4 {
		t.Fatalf("previous change = tab %d, line %d; want tab 0, line 4", app.activeTab, app.tab().cursorLine)
	}

	app.activeTab = 2
	app.tabs[2].cursorLine = 3
	app = pressKey(app, 'n')
	if app.activeTab != 2 || app.tab().cursorLine != 3 {
		t.Fatalf("next change wrapped to tab %d, line %d", app.activeTab, app.tab().cursorLine)
	}

	app.activeTab = 0
	app.tabs[0].cursorLine = 2
	app = pressKey(app, 'N')
	if app.activeTab != 0 || app.tab().cursorLine != 2 {
		t.Fatalf("previous change wrapped to tab %d, line %d", app.activeTab, app.tab().cursorLine)
	}
}

func TestAngleBracketsMoveToFileBoundaries(t *testing.T) {
	app := newChangeNavigationTestApp()
	app.tabs[0].cursorLine = 2
	app.tabs[0].cursorOnAnnotation = true

	app = pressKey(app, '>')
	if app.tab().cursorLine != 4 || app.tab().cursorOnAnnotation {
		t.Fatalf("> moved to line %d, annotation=%t; want line 4", app.tab().cursorLine, app.tab().cursorOnAnnotation)
	}

	app = pressKey(app, '<')
	if app.tab().cursorLine != 1 || app.tab().cursorOnAnnotation {
		t.Fatalf("< moved to line %d, annotation=%t; want line 1", app.tab().cursorLine, app.tab().cursorOnAnnotation)
	}

	if !strings.Contains(app.renderFooter(), "</>") {
		t.Errorf("footer does not advertise angle-bracket navigation: %q", app.renderFooter())
	}
}

func TestDocRenderedMsg_LoadsExistingComments(t *testing.T) {
	// Create a temp directory and test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	if err := os.WriteFile(testFile, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Save review comments for that file in the session
	sess := setupSession(t, testFile)
	comment := review.Comment{
		ID:        "test-comment-1",
		StartLine: 1,
		EndLine:   1,
		Anchor:    "package main",
		Body:      "This is a test comment",
		CreatedAt: review.Now(),
	}
	sess.SetFileComments(testFile, "", []review.Comment{comment})
	if err := sess.Save(); err != nil {
		t.Fatalf("failed to save review state: %v", err)
	}

	// Create an AppModel with a tab for the test file
	app := NewApp(testFile, AppConfig{Session: sess, Author: "Tester"})
	app.tabs = []FileTab{
		{path: testFile},
	}
	app.activeTab = 0

	// Process the docRenderedMsg
	updatedModel, _ := app.Update(docRenderedMsg{})
	updatedApp := updatedModel.(AppModel)

	// Assert the tab's state contains the previously saved comment
	tab := updatedApp.tabs[0]
	if tab.state == nil {
		t.Fatal("expected tab.state to be non-nil after docRenderedMsg")
	}
	if len(tab.state.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(tab.state.Comments))
	}
	if tab.state.Comments[0].ID != "test-comment-1" {
		t.Errorf("expected comment ID 'test-comment-1', got %s", tab.state.Comments[0].ID)
	}
	if tab.state.Comments[0].Body != "This is a test comment" {
		t.Errorf("expected comment body 'This is a test comment', got %s", tab.state.Comments[0].Body)
	}
}

func newScrollTestApp(path string, lines []string, isMarkdown bool, width, height int) AppModel {
	app := NewApp(path, AppConfig{})
	app.tabs[0].doc = &document.Document{
		Path:    path,
		Content: strings.Join(lines, "\n"),
		Lines:   lines,
	}
	app.tabs[0].isMarkdown = isMarkdown
	if !isMarkdown {
		// Stand in for highlightCode output: one rendered line per doc line.
		app.tabs[0].chromaLines = lines
	}
	app.contentViewport.SetWidth(width)
	app.contentViewport.SetHeight(height)
	return app
}

func TestExtraLinesPerDocLine_ChromaLinesNotWrapped(t *testing.T) {
	longLine := strings.Repeat("x", 500)
	lines := []string{"short", longLine, "short"}
	app := newScrollTestApp("test.go", lines, false, 80, 24)

	counts := app.extraLinesPerDocLine()
	if len(counts) != 0 {
		t.Errorf("expected no extra lines for chroma-highlighted content, got %v", counts)
	}
}

func TestExtraLinesPerDocLine_MarkdownWraps(t *testing.T) {
	longLine := strings.Repeat("word ", 100) // 500 chars, wraps in markdown
	lines := []string{"short", longLine, "short"}
	app := newScrollTestApp("test.md", lines, true, 80, 24)

	counts := app.extraLinesPerDocLine()
	if counts[2] == 0 {
		t.Error("expected extra lines for wrapped markdown line, got none")
	}
	if counts[1] != 0 || counts[3] != 0 {
		t.Errorf("expected no extra lines for short lines, got %v", counts)
	}
}

func TestDeletedMarkdownLinesWrap(t *testing.T) {
	longLine := strings.Repeat("word ", 100)
	app := newScrollTestApp("test.md", []string{"current"}, true, 80, 24)
	app.tabs[0].deletedAfter = map[int][]gitpkg.DeletedLine{
		0: {{OldLineNum: 1, Content: longLine}},
	}

	lines := deletedDisplayLines(longLine, "", nil, true, app.contentViewport.Width()-8)
	if len(lines) < 2 {
		t.Fatalf("expected deleted Markdown line to wrap, got %d display line", len(lines))
	}
	if got := app.extraLinesPerDocLine()[1]; got != len(lines) {
		t.Errorf("expected %d extra lines for wrapped deletion, got %d", len(lines), got)
	}
}

func TestInlineDiffDisplayLinesUseDistinctBackgrounds(t *testing.T) {
	initAdaptiveStyles(true)
	segments := []gitpkg.InlineSegment{
		{Content: "common ", Changed: false},
		{Content: "added", Changed: true},
	}

	rendered := strings.Join(inlineDiffDisplayLines(segments, true, 80, diffCommonTextBg, diffAddedTextBg), "\n")
	commonBackground := bgToAnsi(diffCommonTextBg.GetBackground())
	changedBackground := bgToAnsi(diffAddedTextBg.GetBackground())
	if commonBackground == changedBackground {
		t.Fatal("expected common and changed text to use distinct backgrounds")
	}
	if commonBackground == bgToAnsi(diffChangedLineBg.GetBackground()) ||
		commonBackground == bgToAnsi(diffDeletedLineBg.GetBackground()) {
		t.Fatal("expected replacement-line common text to use a neutral background")
	}
	if !strings.Contains(rendered, commonBackground) || !strings.Contains(rendered, changedBackground) {
		t.Errorf("rendered line does not contain both backgrounds: %q", rendered)
	}
}

func TestScrollToChunk_SourceWithLongLines(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = strings.Repeat("x", 500) // longer than the viewport width
	}
	app := newScrollTestApp("test.go", lines, false, 80, 10)
	app.tabs[0].changedLines = map[int]bool{30: true}
	app.rebuildContent()

	app.scrollToChunk(changeChunk{startLine: 30, endLine: 30})

	// Chunk start (line 30) minus chunkScrollPadding should sit at the top:
	// rendered offset = 25 since each doc line renders as exactly one line.
	want := 30 - chunkScrollPadding - 1
	if got := app.contentViewport.YOffset(); got != want {
		t.Errorf("expected YOffset %d, got %d", want, got)
	}
}

func newFinishTestApp(t *testing.T, comments []review.Comment, serving bool) (AppModel, chan FinishEvent) {
	t.Helper()
	t.Chdir(t.TempDir())
	finishCh := make(chan FinishEvent, 4)
	app := NewApp("test.go", AppConfig{
		Session:  setupSession(t, "test.go"),
		Author:   "Tester",
		Serving:  serving,
		FinishCh: finishCh,
	})
	app.tabs[0].state = &fileReview{Comments: comments}
	return app, finishCh
}

func pressKeyCmd(app AppModel, code rune) (AppModel, tea.Cmd) {
	updated, cmd := app.Update(tea.KeyPressMsg{Code: code})
	switch v := updated.(type) {
	case AppModel:
		return v, cmd
	case *AppModel:
		return *v, cmd
	}
	panic("unexpected model type")
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func testComment() review.Comment {
	return review.Comment{ID: "c_test01", StartLine: 1, EndLine: 1, Body: "please fix", CreatedAt: review.Now()}
}

func takeEvent(t *testing.T, ch chan FinishEvent) FinishEvent {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	default:
		t.Fatal("expected a finish event")
		return FinishEvent{}
	}
}

func TestQuit_OpensFinishModal(t *testing.T) {
	app, _ := newFinishTestApp(t, []review.Comment{testComment()}, false)

	app, cmd := pressKeyCmd(app, 'q')

	if isQuit(cmd) {
		t.Fatal("expected quit to be intercepted by the finish modal")
	}
	if app.modal != finishModal {
		t.Fatalf("expected finishModal, got %v", app.modal)
	}
}

func TestFinishModal_ApproveQuitsAndEmitsApproved(t *testing.T) {
	app, ch := newFinishTestApp(t, nil, false)
	app, _ = pressKeyCmd(app, 'q')

	app, cmd := pressKeyCmd(app, 'y')

	if !isQuit(cmd) {
		t.Fatal("expected quit after approving")
	}
	if ev := takeEvent(t, ch); !ev.Approved {
		t.Error("expected approved finish event")
	}
	_ = app
}

func TestFinishModal_ResolvedCommentsStillApprove(t *testing.T) {
	resolved := testComment()
	resolved.Resolved = true
	app, ch := newFinishTestApp(t, []review.Comment{resolved}, false)
	app, _ = pressKeyCmd(app, 'q')

	_, cmd := pressKeyCmd(app, 'y')

	if !isQuit(cmd) {
		t.Fatal("expected quit: all comments resolved means approved")
	}
	if ev := takeEvent(t, ch); !ev.Approved {
		t.Error("expected approved finish event when all comments are resolved")
	}
}

func TestFinishModal_UnresolvedQuitsWhenNotServing(t *testing.T) {
	app, ch := newFinishTestApp(t, []review.Comment{testComment()}, false)
	app.newFeedback = true
	app, _ = pressKeyCmd(app, 'q')

	app, cmd := pressKeyCmd(app, 'y')

	if !isQuit(cmd) {
		t.Fatal("expected quit in inline mode")
	}
	if ev := takeEvent(t, ch); ev.Approved {
		t.Error("expected unapproved finish event")
	}
	if app.waiting {
		t.Error("inline mode should not enter the waiting state")
	}
}

func TestFinishModal_UnresolvedWaitsWhenServing(t *testing.T) {
	app, ch := newFinishTestApp(t, []review.Comment{testComment()}, true)
	app.newFeedback = true
	app, _ = pressKeyCmd(app, 'q')

	app, cmd := pressKeyCmd(app, 'y')

	if isQuit(cmd) {
		t.Fatal("expected the serving TUI to keep running")
	}
	if !app.waiting {
		t.Error("expected waiting state after unresolved finish")
	}
	if ev := takeEvent(t, ch); ev.Approved {
		t.Error("expected unapproved finish event")
	}
}

func TestFinishModal_EscReturnsToReview(t *testing.T) {
	app, ch := newFinishTestApp(t, []review.Comment{testComment()}, false)
	app, _ = pressKeyCmd(app, 'q')

	app, cmd := pressKeyCmd(app, tea.KeyEscape)

	if isQuit(cmd) {
		t.Fatal("expected esc to stay in the review")
	}
	if app.modal != noModal {
		t.Fatalf("expected modal dismissed, got %v", app.modal)
	}
	select {
	case <-ch:
		t.Error("expected no finish event on cancel")
	default:
	}
}

func TestFinishModal_NoNewFeedbackResolvesAllAndApproves(t *testing.T) {
	comment := testComment()
	app, ch := newFinishTestApp(t, []review.Comment{comment}, true)
	app.session.CJ.ReviewComments = []review.Comment{{
		ID: "r_test01", Body: "overall feedback", CreatedAt: review.Now(),
	}}
	app, _ = pressKeyCmd(app, 'q')
	app.width = 80
	app.height = 24

	rendered := app.renderWithModal(strings.Repeat(" ", 80))
	if !strings.Contains(rendered, "Resolve all & Approve?") {
		t.Fatalf("expected resolve-all prompt, got:\n%s", rendered)
	}

	app, cmd := pressKeyCmd(app, 'y')

	if !isQuit(cmd) {
		t.Fatal("expected approval to quit even while serving")
	}
	if ev := takeEvent(t, ch); !ev.Approved {
		t.Error("expected approved finish event")
	}
	if !app.tabs[0].state.Comments[0].Resolved {
		t.Error("expected file comment resolved")
	}
	if !app.session.CJ.ReviewComments[0].Resolved {
		t.Error("expected review comment resolved")
	}
	if got := app.tabs[0].state.Comments[0].ResolvedRound; got != 1 {
		t.Errorf("resolved round = %d, want 1", got)
	}
}

func TestResolveKey_TogglesSelectedComment(t *testing.T) {
	app, _ := newFinishTestApp(t, []review.Comment{testComment()}, false)
	app.focused = commentPane
	app.updateCommentSidebar()

	app = pressKey(app, 'r')

	comment := app.tabs[0].state.Comments[0]
	if !comment.Resolved || comment.ResolvedRound != 1 {
		t.Fatalf("expected resolved comment in round 1, got %+v", comment)
	}

	app = pressKey(app, 'r')

	comment = app.tabs[0].state.Comments[0]
	if comment.Resolved || comment.ResolvedRound != 0 {
		t.Fatalf("expected unresolved comment with cleared round, got %+v", comment)
	}
}

func TestEditModalDeletesOwnCurrentRoundParent(t *testing.T) {
	comment := testComment()
	comment.Author = "Tester"
	comment.ReviewRound = 1
	app, _ := newFinishTestApp(t, []review.Comment{comment}, false)
	app.modal = editModal
	app.editingID = comment.ID
	app.modalFocus = 3

	app = pressKey(app, tea.KeyEnter)

	if len(app.tabs[0].state.Comments) != 0 {
		t.Fatalf("expected parent comment deleted, got %+v", app.tabs[0].state.Comments)
	}
}

func TestEditModalDeletesOnlyOwnCurrentRoundReply(t *testing.T) {
	comment := testComment()
	comment.Author = "Tester"
	comment.ReviewRound = 1
	comment.Replies = []review.Reply{
		{ID: "rp_own", Author: "Tester", Body: "mine", ReviewRound: 1},
		{ID: "rp_old", Author: "Tester", Body: "old", ReviewRound: 0},
		{ID: "rp_other", Author: "AI", Body: "other", ReviewRound: 1},
	}
	app, _ := newFinishTestApp(t, []review.Comment{comment}, false)
	app.modal = editModal
	app.editingID = comment.ID
	app.editingReplyID = "rp_own"
	targets := app.modalDeleteTargets()
	if len(targets) != 1 || targets[0].replyID != "rp_own" {
		t.Fatalf("delete targets = %+v, want only rp_own", targets)
	}
	app.modalFocus = 3

	app = pressKey(app, tea.KeyEnter)

	comments := app.tabs[0].state.Comments
	if len(comments) != 1 {
		t.Fatalf("expected parent preserved, got %+v", comments)
	}
	if len(comments[0].Replies) != 2 || comments[0].Replies[0].ID != "rp_old" || comments[0].Replies[1].ID != "rp_other" {
		t.Fatalf("remaining replies = %+v, want old and other", comments[0].Replies)
	}
}

func TestEnterAddsThenEditsOwnCurrentRoundReply(t *testing.T) {
	comment := testComment()
	comment.Author = "Tester"
	comment.ReviewRound = 1
	comment.Resolved = true
	comment.ResolvedRound = 1
	comment.Replies = []review.Reply{{
		ID: "rp_ai", Author: "AI", Body: "addressed", ReviewRound: 1,
	}}
	app, _ := newFinishTestApp(t, []review.Comment{comment}, false)
	app.focused = commentPane
	app.updateCommentSidebar()

	app = pressKey(app, tea.KeyEnter)
	if app.modal != replyModal || app.editingID != comment.ID {
		t.Fatalf("enter opened modal %v for %q; want reply modal for %q", app.modal, app.editingID, comment.ID)
	}

	app.modalTextarea.SetValue("follow-up")
	app.modalSubmit()
	replies := app.tabs[0].state.Comments[0].Replies
	if len(replies) != 2 {
		t.Fatalf("replies = %+v, want added follow-up", replies)
	}
	added := replies[1]
	if added.Author != "Tester" || added.ReviewRound != 1 || added.Body != "follow-up" {
		t.Fatalf("added reply = %+v", added)
	}
	if app.tabs[0].state.Comments[0].Resolved || app.tabs[0].state.Comments[0].ResolvedRound != 0 {
		t.Error("adding a reply should reopen the thread")
	}

	app = pressKey(app, tea.KeyEnter)
	if app.modal != editModal || app.editingReplyID != added.ID {
		t.Fatalf("enter opened modal %v for reply %q; want edit modal for %q", app.modal, app.editingReplyID, added.ID)
	}
	if got := app.modalTextarea.Value(); got != "follow-up" {
		t.Errorf("edit reply body = %q, want follow-up", got)
	}
	if targets := app.modalDeleteTargets(); len(targets) != 1 || targets[0].replyID != added.ID {
		t.Fatalf("delete targets = %+v, want added reply", targets)
	}
}

func TestEnterEditsOnlyOwnUnrepliedCurrentRoundParent(t *testing.T) {
	tests := []struct {
		name      string
		author    string
		round     int
		wantModal modalType
	}{
		{name: "own current", author: "Tester", round: 1, wantModal: editModal},
		{name: "another author", author: "AI", round: 1, wantModal: replyModal},
		{name: "previous round", author: "Tester", round: 0, wantModal: replyModal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comment := testComment()
			comment.Author = tt.author
			comment.ReviewRound = tt.round
			app, _ := newFinishTestApp(t, []review.Comment{comment}, false)

			app.openCommentThread(comment.ID)

			if app.modal != tt.wantModal {
				t.Errorf("modal = %v, want %v", app.modal, tt.wantModal)
			}
		})
	}
}

func TestEditModalHidesDeleteForIneligibleParent(t *testing.T) {
	tests := []struct {
		name    string
		author  string
		round   int
		replies []review.Reply
	}{
		{name: "another author", author: "AI", round: 1},
		{name: "previous round", author: "Tester", round: 0},
		{name: "thread with reply", author: "Tester", round: 1, replies: []review.Reply{{ID: "rp_1", Author: "AI"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comment := testComment()
			comment.Author = tt.author
			comment.ReviewRound = tt.round
			comment.Replies = tt.replies
			app, _ := newFinishTestApp(t, []review.Comment{comment}, false)
			app.modal = editModal
			app.editingID = comment.ID

			if targets := app.modalDeleteTargets(); len(targets) != 0 {
				t.Fatalf("delete targets = %+v, want none", targets)
			}
		})
	}
}

func TestFinishModal_QuitWithoutFinishing(t *testing.T) {
	app, ch := newFinishTestApp(t, []review.Comment{testComment()}, true)
	app, _ = pressKeyCmd(app, 'q')

	_, cmd := pressKeyCmd(app, 'q')

	if !isQuit(cmd) {
		t.Fatal("expected q in the modal to quit without finishing")
	}
	select {
	case <-ch:
		t.Error("expected no finish event when quitting without finishing")
	default:
	}
}

func TestRoundStart_ReloadsCommentsAndAdvancesRound(t *testing.T) {
	app, _ := newFinishTestApp(t, []review.Comment{testComment()}, true)
	if err := os.WriteFile("test.go", []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app.persist()
	// Load the document so the next round has previous content to carry
	// comments forward from; docRenderedMsg reloads state from the session.
	updated0, _ := app.Update(docRenderedMsg{})
	app = updated0.(AppModel)
	app.waiting = true

	// Simulate the agent replying via the CLI while the TUI waits.
	other, err := review.OpenDocSession("", "test.go")
	if err != nil {
		t.Fatal(err)
	}
	if err := other.AppendReply("c_test01", "fixed it", "AI", "", true, ""); err != nil {
		t.Fatalf("reply: %v", err)
	}
	if err := other.Save(); err != nil {
		t.Fatal(err)
	}

	updated, _ := app.Update(RoundStartMsg{})
	app = updated.(AppModel)

	if app.waiting {
		t.Error("round start should leave the waiting state")
	}
	if app.session.CJ.ReviewRound != 2 {
		t.Errorf("expected round 2, got %d", app.session.CJ.ReviewRound)
	}
	comments := app.tabs[0].state.Comments
	if len(comments) != 1 || len(comments[0].Replies) != 1 {
		t.Fatalf("expected reloaded comment with reply, got %+v", comments)
	}
	if !comments[0].Resolved {
		t.Error("expected reloaded comment to be resolved")
	}
	if !comments[0].CarriedForward || comments[0].ID == "c_test01" {
		t.Errorf("expected a re-minted carried-forward comment, got %+v", comments[0])
	}
}
