package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	gitpkg "github.com/knu/tcrit/internal/git"
	"github.com/knu/tcrit/internal/review"
)

func TestDriftedFocusDoesNotExpandUntilEnter(t *testing.T) {
	for _, hidden := range []bool{false, true} {
		app := setupAppWithDoc(t, strings.Repeat("source\n", 50))
		app.width, app.height = 100, 30
		app.hideComments = hidden
		app.tab().cursorLine = 1
		app.recalculateLayout()
		app.tab().state.Comments = []review.Comment{
			{ID: "lost", StartLine: 200, EndLine: 200, Body: "lost body", Anchor: "old source"},
			{ID: "drifted", StartLine: 40, EndLine: 40, Body: "drifted body", Drifted: true},
		}
		app.rebuildContent()
		app.updateCommentSidebar()
		height := len(app.contentLayout.rows)
		app = pressKey(app, ']')
		if app.selectedCommentID() != "drifted" || app.modal != noModal || len(app.contentLayout.rows) != height {
			t.Fatal("navigation expanded a folded thread")
		}
		if view := ansi.Strip(app.contentViewport.View()); !strings.Contains(view, "> 💬") || strings.Contains(view, "drifted body") {
			t.Fatalf("focused marker = %q", view)
		}
		app = pressKey(app, tea.KeyEnter)
		if view := ansi.Strip(app.contentViewport.View()); !strings.Contains(view, "drifted body") || app.modal != noModal {
			t.Fatalf("Enter did not expand inline: %q", view)
		}
		app = pressKey(app, tea.KeyEnter)
		if view := ansi.Strip(app.contentViewport.View()); strings.Contains(view, "drifted body") || !strings.Contains(view, "> 💬") || len(app.contentLayout.rows) != height {
			t.Fatalf("Enter did not collapse inline: %q", view)
		}
		app = pressKey(app, '[')
		if app.selectedCommentID() != "lost" || !strings.Contains(ansi.Strip(app.contentViewport.View()), "> 💬") || app.contentViewport.YOffset() != 0 {
			t.Fatal("unplaced marker did not gain focus and scroll into view")
		}
		app = pressKey(app, tea.KeyEnter)
		if !strings.Contains(ansi.Strip(app.contentViewport.View()), "old source") {
			t.Fatal("unplaced thread did not expand")
		}
		app = pressKey(app, 'e')
		if app.modal == noModal || app.editingID != "lost" {
			t.Fatal("editor is no longer keyboard-accessible")
		}
	}
}

func TestDriftedMarkersOpenAndCycleThreads(t *testing.T) {
	app := setupAppWithDoc(t, strings.Repeat("long source ", 30)+"\nsecond\n")
	app.width, app.height = 100, 35
	app.recalculateLayout()
	comments := []review.Comment{
		{ID: "normal", StartLine: 1, EndLine: 1, Body: "normal body"},
		{ID: "first", StartLine: 1, EndLine: 1, Body: "first body", Drifted: true, Quote: "original quote", Anchor: "whole anchor"},
		{ID: "second", StartLine: 1, EndLine: 1, Body: "second body", Drifted: true, Anchor: "original anchor"},
	}
	app.tab().state.Comments = append([]review.Comment(nil), comments...)
	app.rebuildContent()
	app.updateCommentSidebar()
	view := ansi.Strip(app.contentViewport.View())
	if !strings.Contains(view, "normal body") || !strings.Contains(view, "💬 2") || strings.Contains(view, "first body") || strings.Contains(view, "second body") {
		t.Fatalf("initial view = %q", view)
	}
	if len(app.contentLayout.markers) != 1 {
		t.Fatalf("markers = %+v", app.contentLayout.markers)
	}
	marker := app.contentLayout.markers[0]
	if marker.rect.top == 0 || app.contentLayout.rows[marker.rect.top].annotation {
		t.Fatal("marker must be on the last wrapped source row, not an annotation")
	}
	for _, row := range strings.Split(app.contentViewport.View(), "\n") {
		if lipgloss.Width(row) > app.contentViewport.Width() {
			t.Fatalf("row exceeds viewport: %q", row)
		}
	}
	clickMarker := func() {
		marker := app.contentLayout.markers[0]
		app.contentViewport.SetYOffset(0)
		left, top, _, _ := app.contentBounds()
		app = clickMouse(app, left+marker.rect.left, top+marker.rect.top)
	}
	clickMarker()
	if app.selectedCommentID() != "first" {
		t.Fatalf("selected = %q", app.selectedCommentID())
	}
	view = ansi.Strip(app.contentViewport.View())
	if !strings.Contains(view, "Position could not be tracked.") || !strings.Contains(view, "original quote") || strings.Contains(view, "whole anchor") {
		t.Fatalf("opened thread = %q", view)
	}
	clickMarker()
	if app.selectedCommentID() != "second" || !strings.Contains(ansi.Strip(app.contentViewport.View()), "original anchor") {
		t.Fatal("second click did not open the next thread with its anchor")
	}
	app = pressKey(app, '[')
	if app.selectedCommentID() != "first" {
		t.Fatal("previous-thread navigation skipped a folded comment")
	}
	app = pressKey(app, ']')
	if app.selectedCommentID() != "second" {
		t.Fatal("next-thread navigation skipped a folded comment")
	}
	if !reflect.DeepEqual(comments, app.tab().state.Comments) {
		t.Fatal("display changed persisted comment data")
	}
}

func TestUnplacedCommentsStayReachable(t *testing.T) {
	for _, kind := range []string{"past end", "missing old side", "partial diff", "missing file", "binary"} {
		t.Run(kind, func(t *testing.T) {
			app := setupAppWithDoc(t, "first\nsecond\n")
			app.width, app.height = 100, 35
			app.recalculateLayout()
			c := review.Comment{ID: "lost", StartLine: 2, EndLine: 2, Body: "lost body", Anchor: "original source"}
			switch kind {
			case "past end":
				c.StartLine, c.EndLine = 500, 500
			case "missing old side":
				c.Side = "old"
			case "partial diff":
				app.tab().doc.Known = map[int]bool{1: true}
			case "missing file":
				app.tab().doc = nil
				app.tab().outsideChanges = true
			case "binary":
				app.tab().isBinary = true
			}
			app.tab().state.Comments = []review.Comment{{ID: "file", Scope: "file", Body: "file body"}, c}
			app.rebuildContent()
			app.updateCommentSidebar()
			if len(app.contentLayout.markers) != 1 || app.contentLayout.markers[0].rect.top != 0 {
				t.Fatalf("missing top marker: %+v", app.contentLayout.markers)
			}
			app.tab().cursorLine = 0
			app = pressKey(app, ']')
			app = pressKey(app, ']')
			app = pressKey(app, tea.KeyEnter)
			if app.selectedCommentID() != c.ID || !strings.Contains(ansi.Strip(app.contentViewport.View()), c.Anchor) {
				t.Fatalf("thread not reachable: selected %q, view %q", app.selectedCommentID(), ansi.Strip(app.contentViewport.View()))
			}
			if app.tab().state.Comments[1].Drifted || app.tab().state.Comments[1].Scope != "" {
				t.Fatal("fallback changed stored drift or scope")
			}
			app = pressKey(app, tea.KeyEnter)
			app = pressKey(app, 'H')
			app.selectMarker([]string{c.ID})
			if !app.hideComments || app.modal != noModal || !strings.Contains(ansi.Strip(app.contentViewport.View()), c.Anchor) {
				t.Fatal("unplaced comment cannot be opened while comments are hidden")
			}
		})
	}
}

func TestDriftedDeletedMarkerAndHiddenMode(t *testing.T) {
	app := setupAppWithDoc(t, "new\n")
	app.width, app.height = 100, 35
	app.recalculateLayout()
	app.tab().deletedAfter = map[int][]gitpkg.DeletedLine{0: {{OldLineNum: 1, Content: "deleted"}}}
	app.tab().state.Comments = []review.Comment{{ID: "old", StartLine: 1, EndLine: 1, Side: "old", Drifted: true, Body: "body", Anchor: "old anchor"}}
	app = pressKey(app, 'H')
	marker := app.contentLayout.markers[0]
	if row := app.contentLayout.rows[marker.rect.top]; row.side != "old" || row.line != 1 {
		t.Fatalf("marker attached to wrong row: %+v", row)
	}
	left, top, _, _ := app.contentBounds()
	app = clickMouse(app, left+marker.rect.left, top+marker.rect.top)
	if !app.hideComments || app.modal != noModal || !app.tab().expandedDrifted["old"] {
		t.Fatal("hidden marker did not open its thread")
	}
	if view := ansi.Strip(app.View().Content); !strings.Contains(view, "old anchor") {
		t.Fatalf("modal lacks original context: %q", view)
	}
	app.releaseThreadFocus()
	app.tab().state.Comments[0].Resolved = true
	app.rebuildContent()
	if len(app.contentLayout.markers) != 0 {
		t.Fatal("resolved drifted marker should follow the resolved visibility setting")
	}
	app = pressKey(app, 'h')
	if len(app.contentLayout.markers) != 1 {
		t.Fatal("showing resolved comments did not restore the marker")
	}
}
