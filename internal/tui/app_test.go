package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevindutra/crit/internal/document"
	"github.com/kevindutra/crit/internal/review"
)

func setupAppWithDoc(t *testing.T, content string) AppModel {
	t.Helper()
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	app := NewApp(testFile)
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
	app := NewApp("test.md")
	if app.filePath != "test.md" {
		t.Errorf("expected filePath 'test.md', got %s", app.filePath)
	}
}

func TestDocRenderedMsg_LoadsExistingComments(t *testing.T) {
	// Create a temp directory and test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	if err := os.WriteFile(testFile, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Save a review state with comments for that file
	comment := review.Comment{
		ID:             "test-comment-1",
		Line:           1,
		ContentSnippet: "package main",
		Body:           "This is a test comment",
		CreatedAt:      time.Now(),
	}
	state := &review.ReviewState{
		File:     testFile,
		Comments: []review.Comment{comment},
	}
	if err := review.Save(state); err != nil {
		t.Fatalf("failed to save review state: %v", err)
	}

	// Create an AppModel with a tab for the test file
	app := NewApp(testFile)
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
	app := NewApp(path)
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

func newApproveTestApp(t *testing.T, comments []review.Comment) AppModel {
	t.Helper()
	t.Chdir(t.TempDir())
	app := NewApp("test.go")
	app.tabs[0].state = &review.ReviewState{File: "test.go", Comments: comments}
	return app
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
	return review.Comment{ID: "c1", Line: 1, Body: "please fix", CreatedAt: time.Now()}
}

func TestQuitWithRemainingComments_PromptsApproval(t *testing.T) {
	app := newApproveTestApp(t, []review.Comment{testComment()})

	app, cmd := pressKeyCmd(app, 'q')

	if isQuit(cmd) {
		t.Fatal("expected quit to be intercepted by approval prompt")
	}
	if app.modal != approveModal {
		t.Fatalf("expected approveModal, got %v", app.modal)
	}
}

func TestApproveModal_ApproveClearsComments(t *testing.T) {
	app := newApproveTestApp(t, []review.Comment{testComment()})
	app, _ = pressKeyCmd(app, 'q')

	app, cmd := pressKeyCmd(app, 'y')

	if !isQuit(cmd) {
		t.Fatal("expected quit after approving")
	}
	if len(app.tabs[0].state.Comments) != 0 {
		t.Errorf("expected comments cleared, got %d", len(app.tabs[0].state.Comments))
	}
	state, err := review.Load("test.go")
	if err != nil {
		t.Fatalf("loading saved state: %v", err)
	}
	if len(state.Comments) != 0 {
		t.Errorf("expected persisted comments cleared, got %d", len(state.Comments))
	}
}

func TestApproveModal_DeclineKeepsComments(t *testing.T) {
	app := newApproveTestApp(t, []review.Comment{testComment()})
	app, _ = pressKeyCmd(app, 'q')

	app, cmd := pressKeyCmd(app, 'n')

	if !isQuit(cmd) {
		t.Fatal("expected quit after declining")
	}
	if len(app.tabs[0].state.Comments) != 1 {
		t.Errorf("expected comments kept, got %d", len(app.tabs[0].state.Comments))
	}
}

func TestApproveModal_EscReturnsToReview(t *testing.T) {
	app := newApproveTestApp(t, []review.Comment{testComment()})
	app, _ = pressKeyCmd(app, 'q')

	app, cmd := pressKeyCmd(app, tea.KeyEscape)

	if isQuit(cmd) {
		t.Fatal("expected esc to stay in the review")
	}
	if app.modal != noModal {
		t.Fatalf("expected modal dismissed, got %v", app.modal)
	}
	if len(app.tabs[0].state.Comments) != 1 {
		t.Errorf("expected comments kept, got %d", len(app.tabs[0].state.Comments))
	}
}

func TestQuitWithNewFeedback_NoPrompt(t *testing.T) {
	app := newApproveTestApp(t, []review.Comment{testComment()})
	app.newFeedback = true

	app, cmd := pressKeyCmd(app, 'q')

	if !isQuit(cmd) {
		t.Fatal("expected direct quit when new feedback was given")
	}
	if app.modal != noModal {
		t.Fatalf("expected no modal, got %v", app.modal)
	}
}

func TestQuitWithNoComments_NoPrompt(t *testing.T) {
	app := newApproveTestApp(t, nil)

	app, cmd := pressKeyCmd(app, 'q')

	if !isQuit(cmd) {
		t.Fatal("expected direct quit with no comments")
	}
	if app.modal != noModal {
		t.Fatalf("expected no modal, got %v", app.modal)
	}
}
