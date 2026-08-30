package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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

func clickMouse(app AppModel, x, y int) AppModel {
	updated, _ := app.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	updated, _ = updated.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	switch v := updated.(type) {
	case AppModel:
		return v
	case *AppModel:
		return *v
	}
	panic("unexpected model type")
}

func pressMouse(app AppModel, x, y int) AppModel {
	updated, _ := app.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	switch v := updated.(type) {
	case AppModel:
		return v
	case *AppModel:
		return *v
	}
	panic("unexpected model type")
}

func moveMouse(app AppModel, x, y int) AppModel {
	updated, _ := app.Update(tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft})
	switch v := updated.(type) {
	case AppModel:
		return v
	case *AppModel:
		return *v
	}
	panic("unexpected model type")
}

func releaseMouse(app AppModel, x, y int) AppModel {
	updated, _ := app.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	switch v := updated.(type) {
	case AppModel:
		return v
	case *AppModel:
		return *v
	}
	panic("unexpected model type")
}

func renderedLineY(t *testing.T, app AppModel, needle string) int {
	t.Helper()
	for y, line := range strings.Split(ansi.Strip(app.View().Content), "\n") {
		if strings.Contains(line, needle) {
			return y
		}
	}
	t.Fatalf("rendered line %q not found", needle)
	return 0
}

func contentScreenPoint(app AppModel, x, contentY int) (int, int) {
	left, top, _, _ := app.contentBounds()
	return left + x, top + contentY - app.contentViewport.YOffset()
}

func wheelMouse(app AppModel, x, y int, button tea.MouseButton) AppModel {
	updated, _ := app.Update(tea.MouseWheelMsg{X: x, Y: y, Button: button})
	switch v := updated.(type) {
	case AppModel:
		return v
	case *AppModel:
		return *v
	}
	panic("unexpected model type")
}

func TestMouseClickSelectsFileTab(t *testing.T) {
	app := newCommentNavigationTestApp()
	app.width = 120

	labels := app.tabLabels()
	firstWidth := lipgloss.Width(app.renderTab(labels, 0, true))
	app = clickMouse(app, firstWidth+1, app.headerHeight())

	if app.activeTab != 1 {
		t.Fatalf("active tab = %d, want 1", app.activeTab)
	}
}

func TestMouseClickOutsideTabBarDoesNotSelectFileTab(t *testing.T) {
	app := newCommentNavigationTestApp()
	app.width = 120

	app = clickMouse(app, 20, app.headerHeight()+app.tabBarHeight())

	if app.activeTab != 0 {
		t.Fatalf("active tab = %d, want 0", app.activeTab)
	}
}

func TestMouseClickSelectsVisibleOverflowTab(t *testing.T) {
	app := newCommentNavigationTestApp()
	fourth := app.tabs[1]
	fourth.path = "fourth.go"
	fifth := app.tabs[1]
	fifth.path = "fifth.go"
	app.tabs = append(app.tabs, fourth, fifth)
	app.width = 50
	app.activeTab = 2

	labels := app.tabLabels()
	for i := range labels {
		labels[i].width = lipgloss.Width(app.renderTab(labels, i, i == 0))
	}
	start, end := app.visibleTabWindow(labels)
	target := start
	if target == app.activeTab {
		target = end - 1
	}
	if target == app.activeTab {
		t.Fatal("test setup did not expose another tab")
	}
	x := 0
	if start > 0 {
		x = lipgloss.Width(inactiveTabStyle.Render("↤ 1 more"))
	}
	for i := start; i < target; i++ {
		x += labels[i].width
	}

	app = clickMouse(app, x+1, app.headerHeight())

	if app.activeTab != target {
		t.Fatalf("active tab = %d, want %d", app.activeTab, target)
	}
}

func TestMouseClickSelectsTabBehindOverflowIndicator(t *testing.T) {
	newApp := func() AppModel {
		app := newCommentNavigationTestApp()
		template := app.tabs[1]
		paths := []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go", "g.go", "h.go"}
		app.tabs = make([]FileTab, len(paths))
		for i, path := range paths {
			app.tabs[i] = template
			app.tabs[i].path = path
		}
		app.width = 44
		app.activeTab = 4
		return app
	}

	tests := []struct {
		name  string
		right bool
	}{
		{name: "left"},
		{name: "right", right: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newApp()
			labels := app.tabLabels()
			for i := range labels {
				labels[i].rendered = app.renderTab(labels, i, i == 0)
				labels[i].width = lipgloss.Width(labels[i].rendered)
			}
			start, end := app.visibleTabWindow(labels)
			if start == 0 || end == len(labels) {
				t.Fatalf("visible window = [%d,%d), want overflow on both sides", start, end)
			}

			x := 1
			want := start - 1
			if tt.right {
				x = lipgloss.Width(app.renderTabOverflowIndicator("↤ "+strconv.Itoa(start)+" more", true))
				for i := start; i < end; i++ {
					x += labels[i].width
				}
				x++
				want = end
			}

			app = clickMouse(app, x, 1)
			if app.activeTab != want {
				t.Fatalf("active tab = %d, want adjacent hidden tab %d", app.activeTab, want)
			}
		})
	}
}

func TestMouseWheelScrollsCodePane(t *testing.T) {
	app := setupAppWithDoc(t, strings.Repeat("line\n", 20))
	app.width = 100
	app.contentViewport.SetWidth(80)

	x, y := contentScreenPoint(app, 10, 1)
	app = wheelMouse(app, x, y, tea.MouseWheelDown)

	if got := app.contentViewport.YOffset(); got != app.contentViewport.MouseWheelDelta {
		t.Fatalf("viewport offset = %d, want %d", got, app.contentViewport.MouseWheelDelta)
	}
}

func TestMouseWheelOutsideCodePaneDoesNotScroll(t *testing.T) {
	app := setupAppWithDoc(t, strings.Repeat("line\n", 20))
	app.width = 100
	app.contentViewport.SetWidth(75)

	_, top, right, _ := app.contentBounds()
	app = wheelMouse(app, right+1, top+1, tea.MouseWheelDown)

	if got := app.contentViewport.YOffset(); got != 0 {
		t.Fatalf("viewport offset = %d, want 0", got)
	}
}

func TestMouseClickFocusesCodeLine(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\nthird\n")
	app.width = 100
	app.contentViewport.SetWidth(75)
	app.tabs[0].cursorLine = 1

	x, y := contentScreenPoint(app, 10, 1)
	app = clickMouse(app, x, y)

	if app.focused != contentPane || app.tab().cursorLine != 2 || app.tab().cursorOnAnnotation {
		t.Fatalf("focus = %v, line = %d, annotation = %t; want content line 2",
			app.focused, app.tab().cursorLine, app.tab().cursorOnAnnotation)
	}
}

func TestMouseClickCodeGutterOpensLineComment(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\nthird\n")
	app.width = 100
	app.contentViewport.SetWidth(75)

	x, y := contentScreenPoint(app, 0, 1)
	app = clickMouse(app, x, y)

	if app.modal != commentModal || app.tab().cursorLine != 2 || app.tab().selecting {
		t.Fatalf("modal = %v, line = %d, selecting = %t; want comment for line 2",
			app.modal, app.tab().cursorLine, app.tab().selecting)
	}
}

func TestMouseClickRenderedCodeLineUsesItsDisplayedPosition(t *testing.T) {
	app := setupAppWithDoc(t, "first unique\nsecond unique\nthird unique\n")
	app.multiFile = true
	app.detached = true
	app.width = 60
	app.height = 20
	app.recalculateLayout()
	app.rebuildContent()
	left, _, _, _ := app.contentBounds()
	y := renderedLineY(t, app, "second unique")

	app = clickMouse(app, left, y)

	if app.tab().cursorLine != 2 {
		t.Fatalf("line = %d, want displayed line 2 at screen row %d", app.tab().cursorLine, y)
	}
}

func TestMouseClickWrappedContinuationUsesOriginalLine(t *testing.T) {
	longLine := "wrapped-start " + strings.Repeat("word ", 20) + "continuation-tail"
	app := setupAppWithDoc(t, longLine+"\nnext unique\n")
	app.multiFile = true
	app.detached = true
	app.width = 60
	app.height = 24
	app.recalculateLayout()
	app.rebuildContent()
	left, _, _, _ := app.contentBounds()
	y := renderedLineY(t, app, "continuation-tail")

	app = clickMouse(app, left, y)

	if app.tab().cursorLine != 1 {
		t.Fatalf("line = %d, want wrapped source line 1 at screen row %d", app.tab().cursorLine, y)
	}
}

func TestMouseClickTabIndentedWrappedRowsUsesOriginalLine(t *testing.T) {
	tests := []struct {
		name    string
		content string
		minRows int
	}{
		{
			name:    "two physical rows",
			content: "\t\ttab-start " + strings.Repeat("word ", 10) + "two-row-tail",
			minRows: 2,
		},
		{
			name:    "three physical rows",
			content: "\t\ttab-start " + strings.Repeat("word ", 20) + "three-row-tail",
			minRows: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := setupAppWithDoc(t, tt.content+"\nnext unique\n")
			app.multiFile = true
			app.detached = true
			app.width = 60
			app.height = 24
			app.recalculateLayout()
			app.rebuildContent()
			left, _, _, _ := app.contentBounds()
			firstY := renderedLineY(t, app, "tab-start")
			nextY := renderedLineY(t, app, "next unique")
			if rows := nextY - firstY; rows < tt.minRows {
				t.Fatalf("rendered rows = %d, want at least %d", rows, tt.minRows)
			}

			for y := firstY; y < nextY; y++ {
				clicked := clickMouse(app, left+gutterWidth, y)
				if clicked.tab().cursorLine != 1 {
					t.Fatalf("screen row %d selected line %d, want wrapped source line 1", y, clicked.tab().cursorLine)
				}
			}
		})
	}
}

func TestMouseClickWrappedDeletedRowsUsesFollowingLine(t *testing.T) {
	app := setupAppWithDoc(t, "current unique\nnext unique\n")
	app.multiFile = true
	app.detached = true
	app.width = 60
	app.height = 24
	app.tabs[0].isMarkdown = false
	app.tabs[0].chromaLines = []string{"current unique", "next unique"}
	app.tabs[0].deletedAfter = map[int][]gitpkg.DeletedLine{
		0: {{OldLineNum: 1, Content: "deleted-start " + strings.Repeat("word ", 20)}},
	}
	app.recalculateLayout()
	app.rebuildContent()
	left, _, _, _ := app.contentBounds()
	firstY := renderedLineY(t, app, "deleted-start")
	currentY := renderedLineY(t, app, "current unique")
	if currentY-firstY < 2 {
		t.Fatalf("deleted line occupies %d rows, want wrapped rows", currentY-firstY)
	}

	for y := firstY; y < currentY; y++ {
		clicked := clickMouse(app, left+gutterWidth, y)
		if clicked.tab().cursorLine != 1 {
			t.Fatalf("deleted screen row %d selected line %d, want following source line 1",
				y, clicked.tab().cursorLine)
		}
	}
}

func TestMouseClickWrappedMarkdownTableRowsUsesOriginalLine(t *testing.T) {
	header := "| very-long-header-cell " + strings.Repeat("word ", 12) + "| value |"
	app := setupAppWithDoc(t, header+"\n| --- | --- |\n| body | value |\n")
	app.multiFile = true
	app.detached = true
	app.width = 60
	app.height = 24
	app.recalculateLayout()
	app.rebuildContent()
	left, _, _, _ := app.contentBounds()
	firstY := renderedLineY(t, app, "very-long-header-cell")
	separatorY := renderedLineY(t, app, "2 │")
	if separatorY-firstY < 2 {
		t.Fatalf("table header occupies %d rows, want wrapped rows", separatorY-firstY)
	}

	for y := firstY; y < separatorY; y++ {
		clicked := clickMouse(app, left+gutterWidth, y)
		if clicked.tab().cursorLine != 1 {
			t.Fatalf("table screen row %d selected line %d, want source line 1",
				y, clicked.tab().cursorLine)
		}
	}
}

func TestMouseDragRenderedCodeLinesUsesDisplayedPositions(t *testing.T) {
	app := setupAppWithDoc(t, "first unique\nsecond unique\nthird unique\nfourth unique\n")
	app.multiFile = true
	app.detached = true
	app.width = 60
	app.height = 22
	app.recalculateLayout()
	app.rebuildContent()
	left, _, _, _ := app.contentBounds()
	startY := renderedLineY(t, app, "second unique")
	endY := renderedLineY(t, app, "third unique")

	app = pressMouse(app, left, startY)
	app = moveMouse(app, left, endY)
	app = releaseMouse(app, left, endY)

	start, end := app.selectionRange()
	if start != 2 || end != 3 {
		t.Fatalf("selection = %d-%d, want displayed lines 2-3", start, end)
	}
}

func TestMouseClickCodeTextDoesNotOpenLineComment(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\nthird\n")
	app.width = 100
	app.contentViewport.SetWidth(75)

	x, y := contentScreenPoint(app, gutterWidth, 1)
	app = clickMouse(app, x, y)

	if app.modal != noModal {
		t.Fatalf("modal = %v, want no modal", app.modal)
	}
}

func TestMouseDragCodeGutterSelectsLinesAndOpensComment(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\nthird\nfourth\nfifth\n")
	app.width = 100
	app.contentViewport.SetWidth(75)

	startX, startY := contentScreenPoint(app, 0, 1)
	endX, endY := contentScreenPoint(app, 0, 3)
	app = pressMouse(app, startX, startY)
	app = moveMouse(app, endX, endY)
	if !app.mouseSelecting || !app.tab().selecting || app.tab().selectAnchor != 2 || app.tab().cursorLine != 4 {
		t.Fatalf("drag = %t, selecting = %t, range = %d-%d; want 2-4",
			app.mouseSelecting, app.tab().selecting, app.tab().selectAnchor, app.tab().cursorLine)
	}

	app = releaseMouse(app, endX, endY)
	if app.mouseSelecting || app.modal != commentModal || !app.tab().selecting {
		t.Fatalf("drag = %t, modal = %v, selecting = %t; want selection comment",
			app.mouseSelecting, app.modal, app.tab().selecting)
	}
}

func TestMouseDragCodeGutterScrollsAtBottomEdge(t *testing.T) {
	app := setupAppWithDoc(t, strings.Repeat("line\n", 10))
	app.width = 100
	app.contentViewport.SetWidth(75)
	app.contentViewport.SetHeight(3)
	app.contentViewport.SetYOffset(1)

	startX, startY := contentScreenPoint(app, 0, 1)
	_, _, _, bottom := app.contentBounds()
	app = pressMouse(app, startX, startY)
	app = moveMouse(app, startX, bottom-1)

	if got := app.contentViewport.YOffset(); got != 2 {
		t.Fatalf("viewport offset = %d, want 2", got)
	}
}

func TestMouseClickFocusesScrolledCodeLine(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\nthird\nfourth\n")
	app.width = 100
	app.contentViewport.SetWidth(75)
	app.contentViewport.SetHeight(2)
	app.contentViewport.SetYOffset(1)

	x, y := contentScreenPoint(app, 10, 1)
	app = clickMouse(app, x, y)

	if app.tab().cursorLine != 2 {
		t.Fatalf("line = %d, want first visible line 2", app.tab().cursorLine)
	}
}

func TestMouseClickDeletedLineFocusesFollowingCodeLine(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\nthird\n")
	app.width = 100
	app.contentViewport.SetWidth(75)
	app.tabs[0].deletedAfter = map[int][]gitpkg.DeletedLine{
		1: {{OldLineNum: 2, Content: "deleted"}},
	}
	app.rebuildContent()

	x, y := contentScreenPoint(app, 10, 1)
	app = clickMouse(app, x, y)

	if app.tab().cursorLine != 2 || app.tab().cursorOnAnnotation {
		t.Fatalf("line = %d, annotation = %t; want following line 2",
			app.tab().cursorLine, app.tab().cursorOnAnnotation)
	}
}

func TestMouseClickFocusesInlineComment(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.width = 100
	app.contentViewport.SetWidth(75)
	app.tabs[0].state.Comments = []review.Comment{{
		ID: "c_inline", StartLine: 1, EndLine: 1, Body: "comment",
	}}
	app.updateCommentSidebar()
	app.rebuildContent()

	target := app.contentLayout.lineRanges[1]
	x, y := contentScreenPoint(app, 10, target.start+1)
	app = clickMouse(app, x, y)

	if app.focused != contentPane || !app.tab().cursorOnAnnotation || app.tab().cursorAnnoIdx != 0 {
		t.Fatalf("focus = %v, annotation = %t/%d; want first inline comment",
			app.focused, app.tab().cursorOnAnnotation, app.tab().cursorAnnoIdx)
	}
}

func TestMouseClickOpensFocusedInlineComment(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.width = 100
	app.contentViewport.SetWidth(75)
	app.tabs[0].state.Comments = []review.Comment{{
		ID: "c_inline", StartLine: 1, EndLine: 1, Body: "comment",
	}}
	app.updateCommentSidebar()
	app.rebuildContent()

	target := app.contentLayout.lineRanges[1]
	x, y := contentScreenPoint(app, 10, target.start+1)
	app = clickMouse(app, x, y)
	if app.modal != noModal {
		t.Fatalf("first click opened modal %v, want focus only", app.modal)
	}
	app = clickMouse(app, x, y)

	if app.modal != replyModal || app.editingID != "c_inline" {
		t.Fatalf("second click opened modal %v for %q, want reply modal for c_inline", app.modal, app.editingID)
	}
}

func TestMouseClickFocusesSidebar(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.width = 100
	app.contentViewport.SetWidth(75)

	left, top, _, _ := app.commentBounds()
	app = clickMouse(app, left+1, top+1)

	if app.focused != commentPane {
		t.Fatalf("focus = %v, want comment pane", app.focused)
	}
}

func TestMouseClickSelectsSidebarComment(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\nthird\n")
	app.width = 100
	app.contentViewport.SetWidth(75)
	app.tabs[0].state.Comments = []review.Comment{
		{ID: "c_first", StartLine: 1, EndLine: 1, Body: "first comment"},
		{ID: "c_second", StartLine: 3, EndLine: 3, Body: "second comment"},
	}
	app.updateCommentSidebar()
	app.rebuildContent()

	left, top, _, _ := app.commentBounds()
	app = clickMouse(app, left+1, top+4)

	if app.focused != commentPane || app.tab().sidebarCursor != 1 || app.tab().cursorLine != 3 {
		t.Fatalf("focus = %v, sidebar = %d, line = %d; want second comment at line 3",
			app.focused, app.tab().sidebarCursor, app.tab().cursorLine)
	}
}

func TestMouseClickOpensFocusedSidebarComment(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.width = 100
	app.contentViewport.SetWidth(75)
	app.tabs[0].state.Comments = []review.Comment{{
		ID: "c_sidebar", StartLine: 1, EndLine: 1, Body: "comment",
	}}
	app.updateCommentSidebar()
	app.rebuildContent()

	left, top, _, _ := app.commentBounds()
	app = clickMouse(app, left+1, top+1)
	if app.modal != noModal {
		t.Fatalf("first click opened modal %v, want focus only", app.modal)
	}
	app = clickMouse(app, left+1, top+1)

	if app.modal != replyModal || app.editingID != "c_sidebar" {
		t.Fatalf("second click opened modal %v for %q, want reply modal for c_sidebar", app.modal, app.editingID)
	}
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

}

func TestFooterKeepsOnlyNonstandardNavigationHints(t *testing.T) {
	app := newChangeNavigationTestApp()
	footer := app.renderFooter()

	for _, omitted := range []string{"j/k", "shift+↑↓", "</>"} {
		if strings.Contains(footer, omitted) {
			t.Errorf("footer contains fallback navigation hint %q: %q", omitted, footer)
		}
	}
	for _, retained := range []string{"[/]", "?"} {
		if !strings.Contains(footer, retained) {
			t.Errorf("footer does not contain %q: %q", retained, footer)
		}
	}

	app.tabs[0].selecting = true
	if footer := app.renderFooter(); strings.Contains(footer, "j/k") || !strings.Contains(footer, "?") {
		t.Errorf("selection footer = %q, want help without j/k", footer)
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
		"↑/↓,j/k", "PgUp/PgDn", "Home/End,g/G,</>", "tab/shift+tab", "ctrl+s", "y/n/esc", "Backspace",
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

func TestRenderedContentLayoutTracksWrappedChromaLines(t *testing.T) {
	longLine := strings.Repeat("x", 500)
	lines := []string{"short", longLine, "short"}
	app := newScrollTestApp("test.go", lines, false, 80, 24)
	app.tabs[0].chromaLines[1] = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(longLine)
	app.tabs[0].changedLines = map[int]bool{2: true}

	app.rebuildContent()
	if r := app.contentLayout.lineRanges[2]; r.end-r.start <= 1 {
		t.Error("expected multiple rendered rows for wrapped Chroma-highlighted line")
	}
	for _, line := range []int{1, 3} {
		if r := app.contentLayout.lineRanges[line]; r.end-r.start != 1 {
			t.Errorf("line %d occupies %d rows, want 1", line, r.end-r.start)
		}
	}
	if got := strings.Count(app.contentViewport.View(), "x"); got != len(longLine) {
		t.Errorf("rendered %d of %d highlighted characters", got, len(longLine))
	}
}

func TestRenderedContentLayoutTracksWrappedMarkdown(t *testing.T) {
	longLine := strings.Repeat("word ", 100) // 500 chars, wraps in markdown
	lines := []string{"short", longLine, "short"}
	app := newScrollTestApp("test.md", lines, true, 80, 24)

	app.rebuildContent()
	if r := app.contentLayout.lineRanges[2]; r.end-r.start <= 1 {
		t.Error("expected multiple rendered rows for wrapped Markdown line")
	}
	for _, line := range []int{1, 3} {
		if r := app.contentLayout.lineRanges[line]; r.end-r.start != 1 {
			t.Errorf("line %d occupies %d rows, want 1", line, r.end-r.start)
		}
	}
}

func TestHighlightMarkdownNestedBoldLinkPreservesText(t *testing.T) {
	line := "- **[Crit](https://crit.md/)-compatible agent workflow** — review commands"
	want := "- Crit-compatible agent workflow — review commands"
	if got := ansi.Strip(highlightMarkdown(line)); got != want {
		t.Fatalf("highlighted text = %q, want %q", got, want)
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
	app.rebuildContent()
	r := app.contentLayout.lineRanges[1]
	if got := r.end - r.start; got != len(lines)+1 {
		t.Errorf("line range has %d rows, want %d deletion and source rows", got, len(lines)+1)
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

func TestInlineDiffMarkdownRestoresSegmentBackgrounds(t *testing.T) {
	initAdaptiveStyles(true)
	segments := []gitpkg.InlineSegment{
		{Content: "- **common** `code` tail ", Changed: false},
		{Content: "**changed** `code` tail", Changed: true},
	}
	rendered := strings.Join(inlineDiffDisplayLines(
		segments, true, 200, diffCommonTextBg, diffAddedTextBg), "\n")
	commonBackground := bgToAnsi(diffCommonTextBg.GetBackground())
	changedBackground := bgToAnsi(diffAddedTextBg.GetBackground())
	if !strings.Contains(rendered, "\x1b[m"+commonBackground) {
		t.Fatalf("common background was not restored after Markdown style reset: %q", rendered)
	}
	if !strings.Contains(rendered, "\x1b[m"+changedBackground) {
		t.Fatalf("changed background was not restored after Markdown style reset: %q", rendered)
	}
}

func TestInlineBackgroundResumesAfterInlineCode(t *testing.T) {
	initAdaptiveStyles(true)
	base := bgToAnsi(diffChangedLineBg.GetBackground())
	rendered := inlineBackground(diffChangedLineBg, "before "+mdCodeStyle.Render("code")+" after")
	beforeAfter := rendered[:strings.Index(rendered, "after")]
	reset := strings.LastIndex(beforeAfter, "\x1b[m")
	if reset < 0 {
		t.Fatalf("inline code has no background reset: %q", rendered)
	}
	if resumed := strings.LastIndex(beforeAfter, base); resumed < reset {
		t.Fatalf("line background was not resumed after inline code: %q", rendered)
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

	// Chunk start minus padding should use the row recorded by rendering.
	want := app.contentLayout.lineRanges[30-chunkScrollPadding].start
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

	app, _ := newFinishTestApp(t, []review.Comment{resolved, unresolved}, false)
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

func TestFileCommentShortcutCreatesSidebarOnlyComment(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.contentViewport.SetWidth(80)
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
	if targets := app.commentTargets(0); len(targets) != 0 {
		t.Fatalf("file comment became an inline navigation target: %+v", targets)
	}
	if got := app.contentViewport.View(); strings.Contains(got, comment.Body) {
		t.Fatalf("file comment rendered inline: %q", got)
	}
}

func TestSuggestionButtonInsertsSelectedCodeAndPersistsComment(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\nthird\n")
	app.tabs[0].selecting = true
	app.tabs[0].selectAnchor = 1
	app.tabs[0].cursorLine = 2
	app.modal = commentModal
	app.modalTextarea.SetValue("Use clearer names.")

	updated, _ := app.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	app = *updated.(*AppModel)

	wantBody := "Use clearer names.\n\n```suggestion\nfirst\nsecond\n```"
	if got := app.modalTextarea.Value(); got != wantBody {
		t.Fatalf("suggestion body = %q, want %q", got, wantBody)
	}
	if app.modalFocus != 0 || !app.modalTextarea.Focused() {
		t.Fatalf("suggestion left focus at %d, focused=%t; want textarea", app.modalFocus, app.modalTextarea.Focused())
	}

	app.modalSubmit()
	comments := app.session.FileComments(app.tab().path)
	if len(comments) != 1 {
		t.Fatalf("persisted comments = %+v, want one", comments)
	}
	comment := comments[0]
	if comment.StartLine != 1 || comment.EndLine != 2 || comment.Anchor != "first\nsecond" || comment.Body != wantBody {
		t.Fatalf("persisted suggestion = %+v", comment)
	}
}

func TestSuggestionButtonIsInCommentModal(t *testing.T) {
	app := setupAppWithDoc(t, "first\n")
	app.width = 80
	app.height = 24
	app.modal = commentModal

	background := lipgloss.NewStyle().Width(app.width).Height(app.height).Render("")
	rendered := app.renderWithModal(background)
	if !strings.Contains(rendered, "Suggest") || !strings.Contains(rendered, "ctrl+y") {
		t.Fatalf("comment modal does not contain Suggest button: %q", rendered)
	}
	if strings.Index(rendered, "Suggest") < strings.Index(rendered, "Cancel") {
		t.Fatalf("Suggest button should follow Cancel: %q", rendered)
	}
}

func TestSuggestionIsAvailableWhenReplyingToLineComment(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\nthird\n")
	comment := review.Comment{
		ID: "c_agent", StartLine: 1, EndLine: 2, Scope: "line",
		Body: "agent reply", Author: "Codex", ReviewRound: 1,
	}
	app.tabs[0].state.Comments = []review.Comment{comment}
	app.openCommentThread(comment.ID)
	if app.modal != replyModal || !app.canSuggest() {
		t.Fatalf("line reply modal = %v, canSuggest=%t", app.modal, app.canSuggest())
	}

	updated, _ := app.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	app = *updated.(*AppModel)
	want := "```suggestion\nfirst\nsecond\n```"
	if got := app.modalTextarea.Value(); got != want {
		t.Fatalf("reply suggestion = %q, want %q", got, want)
	}

	app.width = 80
	app.height = 24
	background := lipgloss.NewStyle().Width(app.width).Height(app.height).Render("")
	if rendered := app.renderWithModal(background); !strings.Contains(rendered, "Suggest") {
		t.Fatalf("reply modal does not contain Suggest button: %q", rendered)
	}
}

func TestSuggestionIsUnavailableWhenReplyingToFileComment(t *testing.T) {
	app := setupAppWithDoc(t, "first\n")
	comment := review.Comment{ID: "c_file", Scope: "file", Body: "whole file", Author: "Codex"}
	app.tabs[0].state.Comments = []review.Comment{comment}
	app.openCommentThread(comment.ID)

	if app.modal != replyModal || app.canSuggest() {
		t.Fatalf("file reply modal = %v, canSuggest=%t", app.modal, app.canSuggest())
	}
}

func TestCommentSidebarSortsFileCommentsBeforeLineComments(t *testing.T) {
	line := testComment()
	line.ID = "c_line"
	file := review.Comment{ID: "c_file", Scope: "file", Body: "whole file"}
	app, _ := newFinishTestApp(t, []review.Comment{line, file}, false)
	app.commentViewport.SetWidth(40)
	app.commentViewport.SetHeight(20)

	app.updateCommentSidebar()

	items := app.tabs[0].sidebarItems
	if len(items) != 2 || items[0].id != file.ID || items[1].id != line.ID {
		t.Fatalf("sidebar items = %+v, want file comment before line comment", items)
	}
}

func TestSidebarNavigationToFileCommentKeepsLineCursor(t *testing.T) {
	line := testComment()
	line.ID = "c_line"
	line.StartLine = 3
	line.EndLine = 3
	file := review.Comment{ID: "c_file", Scope: "file", Body: "whole file"}
	app := setupAppWithDoc(t, "one\ntwo\nthree\n")
	app.tabs[0].state.Comments = []review.Comment{file, line}
	app.focused = commentPane
	app.tabs[0].cursorLine = 3
	app.tabs[0].sidebarCursor = 1
	app.updateCommentSidebar()

	app = pressKey(app, 'k')

	if app.tabs[0].sidebarCursor != 0 {
		t.Fatalf("sidebar cursor = %d, want file comment", app.tabs[0].sidebarCursor)
	}
	if app.tabs[0].cursorLine != 3 {
		t.Fatalf("line cursor = %d, want it unchanged", app.tabs[0].cursorLine)
	}
}

func TestRenderAnnotationBoxCollapsesResolvedThread(t *testing.T) {
	app, _ := newFinishTestApp(t, nil, false)
	ann := newAnnotation(review.Comment{
		ID: "c_resolved", StartLine: 1, EndLine: 1,
		Body: "please fix", Resolved: true,
		Replies: []review.Reply{{Author: "AI", Body: "fixed"}},
	})

	box := app.renderAnnotationBox(ann, 40, false)
	if !strings.Contains(box, "resolved") {
		t.Fatalf("resolved annotation box = %q", box)
	}
	if strings.Contains(box, "please fix") || strings.Contains(box, "fixed") {
		t.Fatalf("resolved annotation body remained visible: %q", box)
	}
	if got := strings.Count(box, "\n"); got != 3 {
		t.Fatalf("resolved annotation box height = %d lines, want 3", got)
	}
}

func TestEditModalDeletesOwnCurrentRoundParent(t *testing.T) {
	comment := testComment()
	comment.Author = "Tester"
	comment.ReviewRound = 1
	app, _ := newFinishTestApp(t, []review.Comment{comment}, false)
	app.modal = editModal
	app.editingID = comment.ID
	app.modalFocus = app.modalDeleteStartFocus()

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
	app.modalFocus = app.modalDeleteStartFocus()

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
	app.focused = contentPane
	app.tabs[0].doc = &document.Document{Path: "test.go", Content: "line", Lines: []string{"line"}}
	app.tabs[0].cursorLine = comment.EndAt()
	app.tabs[0].cursorOnAnnotation = true
	app.tabs[0].cursorAnnoIdx = 0

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

	app.startNextRound()
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
