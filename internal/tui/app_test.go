package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/knu/tcrit/internal/document"
	gitpkg "github.com/knu/tcrit/internal/git"
	"github.com/knu/tcrit/internal/review"
)

func TestRenderHeaderUsesTCritBrand(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*AppModel)
	}{
		{name: "document"},
		{name: "selection", setup: func(app *AppModel) { app.tabs[0].selecting = true }},
		{name: "loading", setup: func(app *AppModel) { app.tabs[0].doc = nil }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := setupAppWithDoc(t, "first\nsecond\n")
			app.width = 120
			if tt.setup != nil {
				tt.setup(&app)
			}

			header := ansi.Strip(app.renderHeader())
			if !strings.Contains(header, " TCrit:") {
				t.Errorf("header = %q, want TCrit branding", header)
			}
			if strings.Contains(header, " Crit:") {
				t.Errorf("header = %q, contains legacy Crit branding", header)
			}
		})
	}
}

func TestRenderHeaderShowsCodeReviewScope(t *testing.T) {
	tests := []struct {
		name    string
		baseRef string
		staged  bool
		want    string
	}{
		{name: "working tree", baseRef: "HEAD", want: "[Working tree]"},
		{name: "staged", baseRef: "HEAD", staged: true, want: "[Staged]"},
		{name: "base ref", baseRef: "main", want: "[Base: main]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := setupAppWithDoc(t, "first\nsecond\n")
			app.width = 120
			app.filePath = ""
			app.multiFile = true
			app.baseRef = tt.baseRef
			app.staged = tt.staged

			header := ansi.Strip(app.renderHeader())
			if !strings.Contains(header, tt.want) {
				t.Errorf("header = %q, want scope %q", header, tt.want)
			}
		})
	}
}

func TestRenderHeaderStaysOnOneLineWithLongPath(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*AppModel)
		want  string
	}{
		{name: "document", want: "0 comments  L0/3"},
		{name: "selection", setup: func(app *AppModel) { app.tabs[0].selecting = true }, want: "VISUAL  L0-0"},
		{name: "loading", setup: func(app *AppModel) { app.tabs[0].doc = nil }, want: "0 comments"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := setupAppWithDoc(t, "first\nsecond\n")
			app.width = 40
			app.filePath = strings.Repeat("long/", 20) + "file.md"
			if tt.setup != nil {
				tt.setup(&app)
			}

			header := app.renderHeader()
			plainHeader := ansi.Strip(header)
			if got := lipgloss.Height(header); got != 1 {
				t.Errorf("header height = %d, want 1: %q", got, plainHeader)
			}
			if !strings.Contains(plainHeader, "…") {
				t.Errorf("header = %q, want truncated path", plainHeader)
			}
			if !strings.Contains(plainHeader, tt.want) {
				t.Errorf("header = %q, want status %q", plainHeader, tt.want)
			}
		})
	}
}

func TestViewEnablesMouseHoverReporting(t *testing.T) {
	app := setupAppWithDoc(t, "line\n")
	app.width = 80
	app.height = 20

	if got := app.View().MouseMode; got != tea.MouseModeAllMotion {
		t.Fatalf("mouse mode = %v, want all-motion reporting", got)
	}
}

func TestKeyboardCommentsDeletedLineRange(t *testing.T) {
	app := setupAppWithDoc(t, "first\nfourth\n")
	app.width = 100
	app.contentViewport.SetWidth(75)
	app.commentViewport.SetWidth(25)
	app.commentViewport.SetHeight(10)
	app.tabs[0].deletedAfter = map[int][]gitpkg.DeletedLine{
		1: {
			{OldLineNum: 2, Content: "second"},
			{OldLineNum: 3, Content: "third"},
		},
	}
	app.tabs[0].cursorLine = 1
	app.rebuildContent()

	app = pressKey(app, 'j')
	if app.tab().cursorLine != 2 || app.tab().cursorSide != "old" {
		t.Fatalf("cursor = %d/%s, want deleted line 2", app.tab().cursorLine, app.tab().cursorSide)
	}
	app = pressKey(app, 'v')
	app = pressKey(app, 'j')
	app = pressKey(app, tea.KeyEnter)
	if app.modal != commentModal || app.canSuggest() {
		t.Fatalf("modal = %v, can suggest = %t; want deleted-line comment without Suggest", app.modal, app.canSuggest())
	}
	app.modalTextarea.SetValue("restore this logic")
	app.modalSubmit()

	if len(app.tab().state.Comments) != 1 {
		t.Fatalf("comments = %+v, want one", app.tab().state.Comments)
	}
	c := app.tab().state.Comments[0]
	if c.Side != "old" || c.StartLine != 2 || c.EndLine != 3 || c.Anchor != "second\nthird" {
		t.Fatalf("comment = %+v, want old lines 2-3 with deleted anchor", c)
	}
}

func TestLineCommentCritJSONCompatibility(t *testing.T) {
	tests := []struct {
		name    string
		side    string
		start   int
		end     int
		anchor  string
		deleted map[int][]gitpkg.DeletedLine
		changed map[int]bool
	}{
		{name: "added line", start: 1, end: 1, anchor: "first", changed: map[int]bool{1: true}},
		{name: "added range", start: 1, end: 2, anchor: "first\nsecond", changed: map[int]bool{1: true, 2: true}},
		{name: "deleted line", side: "old", start: 2, end: 2, anchor: "removed", deleted: map[int][]gitpkg.DeletedLine{
			1: {{OldLineNum: 2, Content: "removed"}},
		}},
		{name: "deleted range", side: "old", start: 2, end: 3, anchor: "removed\nalso removed", deleted: map[int][]gitpkg.DeletedLine{
			1: {
				{OldLineNum: 2, Content: "removed"},
				{OldLineNum: 3, Content: "also removed"},
			},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := setupAppWithDoc(t, "first\nsecond\n")
			app.tabs[0].deletedAfter = tt.deleted
			app.tabs[0].changedLines = tt.changed
			app.tabs[0].cursorLine = tt.end
			app.tabs[0].cursorSide = tt.side
			if tt.start != tt.end {
				app.tabs[0].selecting = true
				app.tabs[0].selectAnchor = tt.start
				app.tabs[0].selectSide = tt.side
			}
			app.openLineComment()
			app.modalTextarea.SetValue("review note")
			app.modalSubmit()

			data, err := json.Marshal(app.tab().state.Comments[0])
			if err != nil {
				t.Fatalf("marshaling comment: %v", err)
			}
			var wire map[string]any
			if err := json.Unmarshal(data, &wire); err != nil {
				t.Fatalf("unmarshaling comment: %v", err)
			}
			if got := int(wire["start_line"].(float64)); got != tt.start {
				t.Errorf("start_line = %d, want %d", got, tt.start)
			}
			if got := int(wire["end_line"].(float64)); got != tt.end {
				t.Errorf("end_line = %d, want %d", got, tt.end)
			}
			if got := wire["anchor"]; got != tt.anchor {
				t.Errorf("anchor = %q, want %q", got, tt.anchor)
			}
			gotSide, hasSide := wire["side"]
			if tt.side == "" && hasSide {
				t.Errorf("new-side JSON contains side = %q, want omitted", gotSide)
			}
			if tt.side == "old" && gotSide != "old" {
				t.Errorf("old-side JSON side = %q, want old", gotSide)
			}
		})
	}
}

func TestOldSideCommentRendersAfterDeletedLine(t *testing.T) {
	app := setupAppWithDoc(t, "first\nthird\n")
	app.width = 100
	app.contentViewport.SetWidth(75)
	app.contentViewport.SetHeight(12)
	app.commentViewport.SetWidth(25)
	app.commentViewport.SetHeight(10)
	app.tabs[0].deletedAfter = map[int][]gitpkg.DeletedLine{
		1: {{OldLineNum: 2, Content: "second"}},
	}
	app.tabs[0].state.Comments = []review.Comment{{
		ID: "c_old", Side: "old", StartLine: 2, EndLine: 2, Body: "deleted comment",
	}}
	app.updateCommentSidebar()
	app.rebuildContent()

	r := app.contentLayout.oldRanges[2]
	if r.end-r.start < 2 {
		t.Fatalf("deleted line occupies %d row(s), want inline annotation", r.end-r.start)
	}
	if got := ansi.Strip(app.contentViewport.View()); !strings.Contains(got, "deleted comment") {
		t.Fatalf("content = %q, want old-side annotation", got)
	}
	if got := ansi.Strip(app.commentViewport.View()); !strings.Contains(got, "L2 (deleted)") {
		t.Fatalf("sidebar = %q, want deleted-side label", got)
	}
}

func TestDeletedFileRendersSelectableLines(t *testing.T) {
	app := NewApp("deleted.go", AppConfig{})
	app.tabs[0].doc = &document.Document{Path: "deleted.go"}
	app.tabs[0].isDeleted = true
	app.tabs[0].deletedAfter = map[int][]gitpkg.DeletedLine{
		0: {
			{OldLineNum: 1, Content: "package deleted"},
			{OldLineNum: 2, Content: "var removed = true"},
		},
	}
	app.tabs[0].cursorLine = 1
	app.tabs[0].cursorSide = "old"
	app.contentViewport.SetWidth(75)
	app.contentViewport.SetHeight(10)
	app.rebuildContent()

	content := ansi.Strip(app.contentViewport.View())
	if !strings.Contains(content, "package deleted") || !strings.Contains(content, "var removed = true") {
		t.Fatalf("content = %q, want deleted file contents", content)
	}
	app = pressKey(app, 'j')
	if app.tab().cursorLine != 2 || app.tab().cursorSide != "old" {
		t.Fatalf("cursor = %d/%s, want deleted line 2", app.tab().cursorLine, app.tab().cursorSide)
	}
}

func TestReviewFrameFitsTerminalWithSidebar(t *testing.T) {
	for _, width := range []int{60, 120} {
		for _, hidden := range []bool{false, true} {
			t.Run(fmt.Sprintf("width=%d/hidden=%t", width, hidden), func(t *testing.T) {
				app := setupAppWithDoc(t, strings.Repeat("source\n", 30))
				app.width, app.height, app.multiFile = width, 30, true
				app.hideComments = hidden
				app.tab().state.Comments = []review.Comment{{ID: "file", Scope: "file", Body: "comment"}}
				app.recalculateLayout()
				app.updateCommentSidebar()
				app.rebuildContent()
				rows := strings.Split(ansi.Strip(app.View().Content), "\n")
				_, top, contentRight, bottom := app.contentBounds()
				for y := top; y < bottom; y++ {
					if lipgloss.Width(rows[y]) != width || ansi.Cut(rows[y], width-1, width) != "│" {
						t.Fatalf("row %d: width=%d, want %d with right border at last column; %q", y, lipgloss.Width(rows[y]), width, rows[y])
					}
				}
				left, _, right, _ := app.commentBounds()
				if left != contentRight || right != width-1 {
					t.Fatalf("sidebar bounds [%d,%d), want [%d,%d)", left, right, contentRight, width-1)
				}
				if !hidden && ansi.Cut(rows[top], left, left+1) != "│" {
					t.Fatal("sidebar mouse bounds should start at its rendered divider")
				}
			})
		}
	}
}

func TestHelpModalShowsAllShortcutGroupsAndCloses(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.width = 80
	app.height = 24

	app = pressKey(app, '?')
	if app.modal != helpModal {
		t.Fatalf("? opened modal %v, want help modal", app.modal)
	}

	background := lipgloss.NewStyle().Width(app.width).Height(app.height).Render("")
	rendered := app.renderWithModal(background)
	for _, want := range []string{
		"Keyboard Help", "General", "Navigation", "Code review", "Selection and dialogs",
		"↑/↓,j/k", "PgUp/PgDn", "Home/End,g/G,</>", "tab/S-tab", "ctrl+s", "ctrl+PgUp/PgDn", "y/n/esc", "alt+p",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("help modal does not contain %q", want)
		}
	}
	if width := lipgloss.Width(rendered); width > app.width {
		t.Errorf("help width = %d, terminal width = %d", width, app.width)
	}
	if height := lipgloss.Height(rendered); height > app.height {
		t.Errorf("help height = %d, terminal height = %d", height, app.height)
	}

	app = pressKey(app, tea.KeyEscape)
	if app.modal != noModal {
		t.Fatalf("esc left modal %v open", app.modal)
	}

	app = pressKey(app, '?')
	app = pressKey(app, '?')
	if app.modal != noModal {
		t.Fatalf("second ? left modal %v open", app.modal)
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

func TestResolveKey_TogglesSelectedComment(t *testing.T) {
	app, _ := newFinishTestApp(t, []review.Comment{testComment()})
	app.commentViewport.SetWidth(40)
	app.commentViewport.SetHeight(20)
	app.focused = commentPane
	app.updateCommentSidebar()

	app = pressKey(app, 'r')

	comment := app.tabs[0].state.Comments[0]
	if !comment.Resolved || comment.ResolvedRound != 1 {
		t.Fatalf("expected resolved comment in round 1, got %+v", comment)
	}
	if len(app.tabs[0].sidebarItems) != 0 {
		t.Fatalf("resolved comment remained in sidebar: %+v", app.tabs[0].sidebarItems)
	}
	if got := app.commentViewport.View(); !strings.Contains(got, "All comments resolved.") {
		t.Fatalf("resolved sidebar message = %q", got)
	}

	// The inline annotation remains available so the thread can be reopened.
	app.focused = contentPane
	app.tabs[0].doc = &document.Document{Path: "test.go", Content: "line", Lines: []string{"line"}}
	app.tabs[0].cursorLine = comment.EndAt()
	app.tabs[0].cursorOnAnnotation = true
	app.tabs[0].cursorAnnoIdx = 0
	app = pressKey(app, 'r')

	comment = app.tabs[0].state.Comments[0]
	if comment.Resolved || comment.ResolvedRound != 0 {
		t.Fatalf("expected unresolved comment with cleared round, got %+v", comment)
	}
}

func TestCommentSidebarHidesResolvedThreads(t *testing.T) {
	unresolved := testComment()
	unresolved.ID = "c_unresolved"
	unresolved.Body = "still open"
	resolved := testComment()
	resolved.ID = "c_resolved"
	resolved.Body = "already handled"
	resolved.Resolved = true

	app, _ := newFinishTestApp(t, []review.Comment{resolved, unresolved})
	app.commentViewport.SetWidth(40)
	app.commentViewport.SetHeight(20)
	app.updateCommentSidebar()

	items := app.tabs[0].sidebarItems
	if len(items) != 1 || items[0].id != unresolved.ID {
		t.Fatalf("sidebar items = %+v, want only %s", items, unresolved.ID)
	}
	rendered := app.commentViewport.View()
	if strings.Contains(rendered, resolved.Body) {
		t.Errorf("resolved thread visible in sidebar: %q", rendered)
	}
	if !strings.Contains(rendered, unresolved.Body) {
		t.Errorf("unresolved thread missing from sidebar: %q", rendered)
	}
	if got := unresolvedCommentCount(app.tabs[0].state.Comments); got != 1 {
		t.Errorf("unresolved comment count = %d, want 1", got)
	}
}

func TestFileCommentShortcutCreatesInlineFileComment(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.contentViewport.SetWidth(80)
	app.contentViewport.SetHeight(12)
	app.commentViewport.SetWidth(40)
	app.commentViewport.SetHeight(20)

	app = pressKey(app, 'f')
	if app.modal != fileCommentModal {
		t.Fatalf("f opened modal %v, want file comment modal", app.modal)
	}

	app.modalTextarea.SetValue("applies to the whole file")
	app.modalSubmit()

	comments := app.tabs[0].state.Comments
	if len(comments) != 1 {
		t.Fatalf("comments = %+v, want one file comment", comments)
	}
	comment := comments[0]
	if comment.Scope != "file" || comment.StartLine != 0 || comment.EndLine != 0 || comment.Anchor != "" {
		t.Fatalf("file comment has line metadata: %+v", comment)
	}
	if got := app.session.FileComments(app.tab().path); len(got) != 1 || got[0].Scope != "file" {
		t.Fatalf("persisted comments = %+v, want one file comment", got)
	}
	if got := app.commentViewport.View(); !strings.Contains(got, "File") || !strings.Contains(got, comment.Body) {
		t.Fatalf("sidebar = %q, want file comment", got)
	}
	if targets := app.commentTargets(0); len(targets) != 1 || targets[0].scope != "file" {
		t.Fatalf("navigation targets = %+v, want one file-scoped target", targets)
	}
	if got := app.contentViewport.View(); !strings.Contains(got, comment.Body) {
		t.Fatalf("file comment missing inline: %q", got)
	}
}

func TestFileCommentShortcutRepliesToExistingThread(t *testing.T) {
	for _, resolved := range []bool{false, true} {
		for _, ownReply := range []bool{false, true} {
			for _, hidden := range []bool{false, true} {
				t.Run(fmt.Sprintf("resolved=%t/ownReply=%t/hidden=%t", resolved, ownReply, hidden), func(t *testing.T) {
					app := setupAppWithDoc(t, "first\nsecond\n")
					thread := review.Comment{
						ID: "file-thread", Scope: "file", Body: "original", Author: app.author,
						ReviewRound: app.reviewRound(), Resolved: resolved,
					}
					if ownReply {
						thread.Replies = []review.Reply{{ID: "own-reply", Body: "previous reply", Author: app.author, ReviewRound: app.reviewRound()}}
					}
					app.tab().state.Comments = []review.Comment{
						{ID: "line", Scope: "line", StartLine: 1, EndLine: 1, Body: "line comment"},
						thread,
						{ID: "other-file-thread", Scope: "file", Body: "another thread"},
					}
					app.hideComments = hidden
					app.updateCommentSidebar()
					app = pressKey(app, 'f')
					if app.modal != replyModal || app.editingID != thread.ID || app.editingReplyID != "" {
						t.Fatalf("f opened modal %v for %q/%q, want a new reply to %q", app.modal, app.editingID, app.editingReplyID, thread.ID)
					}
					if app.modalTextarea.Value() != "" || !app.modalTextarea.Focused() || app.modalInitial != "" {
						t.Fatal("reply editor should be empty and focused")
					}
					app.modalTextarea.SetValue("new reply")
					app.modalSubmit()
					comments := app.session.FileComments(app.tab().path)
					if len(comments) != 3 {
						t.Fatalf("got %d threads, want 3", len(comments))
					}
					got := comments[1]
					if got.Body != thread.Body || got.Resolved || len(got.Replies) != len(thread.Replies)+1 || got.Replies[len(got.Replies)-1].Body != "new reply" {
						t.Fatalf("persisted thread = %+v, want original body and appended reply", got)
					}
					if ownReply && got.Replies[0].Body != "previous reply" {
						t.Fatal("existing reply was overwritten")
					}
				})
			}
		}
	}
}

func TestCommentSidebarSortsFileCommentsBeforeLineComments(t *testing.T) {
	line := testComment()
	line.ID = "c_line"
	file := review.Comment{ID: "c_file", Scope: "file", Body: "whole file"}
	app, _ := newFinishTestApp(t, []review.Comment{line, file})
	app.commentViewport.SetWidth(40)
	app.commentViewport.SetHeight(20)

	app.updateCommentSidebar()

	items := app.tabs[0].sidebarItems
	if len(items) != 2 || items[0].id != file.ID || items[1].id != line.ID {
		t.Fatalf("sidebar items = %+v, want file comment before line comment", items)
	}
}

func TestRoundStart_ReloadsCommentsAndAdvancesRound(t *testing.T) {
	app, _ := newFinishTestApp(t, []review.Comment{testComment()})
	if err := os.WriteFile("test.go", []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app.persist()
	// Load the document so the next round has previous content to carry
	// comments forward from; docRenderedMsg reloads state from the session.
	updated0, _ := app.Update(docRenderedMsg{})
	app = updated0.(AppModel)

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

	app = restartRound(t, app)

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
	if !comments[0].CarriedForward || comments[0].ID != "c_test01" {
		t.Errorf("expected a carried-forward comment with the same ID, got %+v", comments[0])
	}
}

func TestRoundStartRefreshesCodeReviewTabs(t *testing.T) {
	t.Chdir(t.TempDir())
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}

	runGit("init", "--quiet")
	if err := os.WriteFile("existing.go", []byte("package existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "existing.go")
	runGit("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--quiet", "-m", "initial")
	if err := os.WriteFile("existing.go", []byte("package existing\n\nvar changed = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := gitpkg.ChangedFiles()
	if err != nil {
		t.Fatal(err)
	}
	app := NewCodeReviewApp(files, "HEAD", AppConfig{})
	if len(app.tabs) != 1 {
		t.Fatalf("initial tabs = %d, want 1", len(app.tabs))
	}

	newFiles := []string{"go.mod", "internal/cli/process_darwin.go", "internal/cli/process_linux.go", "internal/cli/process_other.go"}
	for _, path := range newFiles {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("new file\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runGit("add", path)
	}

	app = restartRound(t, app)
	want := []string{"existing.go", "go.mod", "internal/cli/process_darwin.go", "internal/cli/process_linux.go", "internal/cli/process_other.go"}
	if len(app.tabs) != len(want) {
		t.Fatalf("refreshed tabs = %d, want %d", len(app.tabs), len(want))
	}
	for i, path := range want {
		if app.tabs[i].path != path {
			t.Errorf("tab %d = %q, want %q", i, app.tabs[i].path, path)
		}
	}
}

func TestStagedRoundUsesIndexDocument(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	runGitIn(t, dir, "init", "-q")
	runGitIn(t, dir, "config", "user.email", "test@example.com")
	runGitIn(t, dir, "config", "user.name", "Test")
	runGitIn(t, dir, "config", "commit.gpgsign", "false")
	path := filepath.Join(dir, "partial.go")
	if err := os.WriteFile(path, []byte("package base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitIn(t, dir, "add", "partial.go")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")
	if err := os.WriteFile(path, []byte("package staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitIn(t, dir, "add", "partial.go")
	if err := os.WriteFile(path, []byte("package unstaged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	files, err := gitpkg.ChangedFilesStaged()
	if err != nil {
		t.Fatal(err)
	}
	sess, err := review.OpenCodeSession("")
	if err != nil {
		t.Fatal(err)
	}
	app := NewCodeReviewApp(files, "HEAD", AppConfig{Session: sess, Staged: true})
	updated, _ := app.Update(docRenderedMsg{})
	app = updated.(AppModel)
	if got := app.tab().doc.Content; got != "package staged\n" {
		t.Fatalf("initial document = %q, want indexed content", got)
	}

	if err := os.WriteFile(path, []byte("package nextstaged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitIn(t, dir, "add", "partial.go")
	if err := os.WriteFile(path, []byte("package nextunstaged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app = restartRound(t, app)
	if got := app.tab().doc.Content; got != "package nextstaged\n" {
		t.Fatalf("next-round document = %q, want indexed content", got)
	}
}

func TestRevertedAdditionShowsPlaceholderAndKeepsComments(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	runGitIn(t, dir, "init", "-q")
	runGitIn(t, dir, "config", "user.email", "test@example.com")
	runGitIn(t, dir, "config", "user.name", "Test")
	runGitIn(t, dir, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitIn(t, dir, "add", "keep.txt")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")
	if err := os.WriteFile(filepath.Join(dir, "added.go"), []byte("package added\nvar x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitIn(t, dir, "add", "added.go")
	t.Chdir(dir)

	files, err := gitpkg.ChangedFilesFrom("HEAD")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := review.OpenCodeSession("")
	if err != nil {
		t.Fatal(err)
	}
	app := NewCodeReviewApp(files, "HEAD", AppConfig{Session: sess, Author: "Tester"})
	updated, _ := app.Update(docRenderedMsg{})
	app = updated.(AppModel)
	app.width, app.height = 100, 30
	app.recalculateLayout()
	for i := range app.tabs {
		if app.tabs[i].path == "added.go" {
			app.activeTab = i
			app.tabs[i].state.Comments = append(app.tabs[i].state.Comments, review.Comment{
				ID: "c_1", StartLine: 2, EndLine: 2, Body: "why one?", Anchor: "var x = 1", Author: "Tester",
			})
		}
	}
	app.persist()

	runGitIn(t, dir, "rm", "-q", "-f", "added.go")
	app = restartRound(t, app)

	if app.tab().path != "added.go" || !app.tab().outsideChanges {
		t.Fatalf("active tab = %q (outside changes: %t), want added.go kept outside the changes", app.tab().path, app.tab().outsideChanges)
	}
	content := ansi.Strip(app.contentViewport.View())
	if !strings.Contains(content, "no longer part of the changes") || strings.Contains(content, "var x = 1") {
		t.Fatalf("content = %q, want placeholder without stale content", content)
	}
	if got := ansi.Strip(app.commentViewport.View()); !strings.Contains(got, "why one?") {
		t.Fatalf("sidebar = %q, want the kept comment", got)
	}
	app.persist()
	fresh, err := review.OpenSessionAt(sess.Key, sess.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := fresh.CJ.Files["added.go"].Comments; len(got) != 1 || got[0].Body != "why one?" {
		t.Fatalf("persisted comments = %+v, want the kept comment", got)
	}
}

func TestCommentSidebarCollapsesResolvedFileComments(t *testing.T) {
	fileComment := review.Comment{ID: "c_file", Scope: "file", Body: "split this file", Author: "Reviewer", Resolved: true, CreatedAt: review.Now()}
	lineComment := testComment()

	app, _ := newFinishTestApp(t, []review.Comment{lineComment, fileComment})
	app.commentViewport.SetWidth(40)
	app.commentViewport.SetHeight(20)
	app.focused = contentPane
	app.updateCommentSidebar()

	items := app.tabs[0].sidebarItems
	if len(items) != 2 || items[0].id != fileComment.ID || !items[0].resolved {
		t.Fatalf("sidebar items = %+v, want the resolved file comment first", items)
	}
	rendered := ansi.Strip(app.commentViewport.View())
	if !strings.Contains(rendered, "☑︎ Resolved") || strings.Contains(rendered, fileComment.Body) {
		t.Fatalf("sidebar = %q, want a collapsed resolved header without the body", rendered)
	}

	app.tabs[0].sidebarCursor = 0
	app.focused = commentPane
	app = pressKey(app, 'r')

	if app.tabs[0].state.Comments[1].Resolved {
		t.Fatal("expected r to reopen the resolved file comment")
	}
	rendered = ansi.Strip(app.commentViewport.View())
	if !strings.Contains(rendered, fileComment.Body) {
		t.Fatalf("sidebar = %q, want the reopened body", rendered)
	}
}
