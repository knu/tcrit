package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/knu/tcrit/internal/review"
)

func TestThreadInitialOffset(t *testing.T) {
	for _, tt := range []struct {
		name       string
		body       string
		replies    []review.Reply
		width      int
		height     int
		wantOffset int
	}{
		{name: "single long comment", body: strings.Repeat("parent\n", 20), width: 40, height: 10},
		{name: "two fit exactly", body: "parent", replies: []review.Reply{{Body: "latest"}}, width: 40, height: 5},
		{name: "two overflow by one", body: "parent", replies: []review.Reply{{Body: "latest"}}, width: 40, height: 4, wantOffset: 1},
		{name: "all three fit", body: "parent", replies: []review.Reply{{Body: "previous"}, {Body: "latest"}}, width: 40, height: 10},
		{name: "long predecessor tail", body: strings.Repeat("parent\n", 20), replies: []review.Reply{{Body: "latest"}}, width: 40, height: 10, wantOffset: 15},
		{name: "long latest starts at header", body: "parent", replies: []review.Reply{{Body: strings.Repeat("latest\n", 20)}}, width: 40, height: 10, wantOffset: 3},
		{name: "wrapped Japanese", body: strings.Repeat("日本語", 20), replies: []review.Reply{{Body: "latest"}}, width: 20, height: 5, wantOffset: 5},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := AppModel{}
			layout := app.layoutThread("Reviewer", tt.body, tt.replies, "", tt.width)
			if got, want := layout.initialOffset(tt.height), tt.wantOffset; got != want {
				t.Fatalf("offset = %d, want %d; lines = %q", got, want, layout.lines)
			}
			for _, line := range layout.lines {
				if ansi.StringWidth(line) > tt.width {
					t.Errorf("line exceeds width %d: %q", tt.width, line)
				}
			}
		})
	}
}

func TestThreadAuthorsKeepColorsAcrossRoles(t *testing.T) {
	app := AppModel{author: "Reviewer"}
	parent := app.commentAuthorStyle("Reviewer").Render("body")
	agent := app.commentAuthorStyle("Codex").Render("body")
	if parent == agent {
		t.Fatal("reviewer and agent share a color")
	}
	if want := lipgloss.NewStyle().Foreground(lipgloss.BrightWhite).Render("body"); parent != want {
		t.Fatal("the current reviewer's comments are not white")
	}
	if want := lipgloss.NewStyle().Foreground(lipgloss.Cyan).Render("body"); agent != want {
		t.Fatal("the first other author is not cyan")
	}
	app.commentAuthorStyle("Another author")
	if got := app.commentAuthorStyle("Reviewer").Render("body"); got != parent {
		t.Fatal("adding an author changed the reviewer's color")
	}
	layout := app.layoutThread("Reviewer", "body", []review.Reply{{Author: "Codex", Body: "body"}, {Author: "Reviewer", Body: "body"}}, "", 40)
	if layout.lines[1] != parent || layout.lines[layout.starts[1]+1] != agent || layout.lines[layout.starts[2]+1] != parent {
		t.Fatalf("parent/reply colors differ for the same author: %q", layout.lines)
	}
}

func TestThreadViewportKeepsHistoryAndResetsForNewReply(t *testing.T) {
	app := setupAppWithDoc(t, "source\n")
	app.width, app.height = 120, 40
	app.recalculateLayout()
	key := threadViewKey{path: app.tab().path, id: "thread"}
	body := "old first\n" + strings.Repeat("past\n", 20) + "old last"
	replies := []review.Reply{{ID: "r1", Author: "Codex", Body: "latest first\n" + strings.Repeat("new\n", 20) + "latest last"}}
	render := func() string { return ansi.Strip(app.renderThread(key, "Reviewer", body, replies, 40, true)) }
	first := render()
	if !strings.Contains(first, "latest first") || strings.Contains(first, "latest last") || strings.Contains(first, "old first") {
		t.Fatalf("initial viewport = %q", first)
	}
	height := lipgloss.Height(first)
	scroll := app.threadScrolls[key]
	scroll.manual, scroll.offset = true, 0
	app.threadScrolls[key] = scroll
	if got := render(); !strings.Contains(got, "old first") || lipgloss.Height(got) != height {
		t.Fatalf("history viewport = %q", got)
	}
	scroll.offset = scroll.maxOffset
	app.threadScrolls[key] = scroll
	if got := render(); !strings.Contains(got, "latest last") || lipgloss.Height(got) != height {
		t.Fatalf("bottom viewport = %q", got)
	}
	replies = append(replies, review.Reply{ID: "r2", Author: "Reviewer", Body: "newest short reply"})
	if got := render(); !strings.Contains(got, "newest short reply") || !strings.Contains(got, "latest last") || lipgloss.Height(got) != height {
		t.Fatalf("new reply viewport = %q", got)
	}
	rows := strings.Split(render(), "\n")
	if rows[len(rows)-2] != "newest short reply" {
		t.Fatalf("blank padding below the latest reply: %q", rows)
	}
}

func TestUnfocusedThreadShrinksToLatestComment(t *testing.T) {
	app := setupAppWithDoc(t, "source\n")
	app.width, app.height = 120, 40
	app.recalculateLayout()
	key := threadViewKey{id: "thread"}
	body := strings.Repeat("history\n", 30)
	replies := []review.Reply{{Author: "Bob", Body: "short reply"}}
	for _, focused := range []bool{false, true, false} {
		view := app.renderThread(key, "Reviewer", body, replies, 40, focused)
		if focused {
			if lipgloss.Height(view) != maxThreadLines+1 {
				t.Fatalf("focused thread did not expand: %q", view)
			}
		} else if got := ansi.Strip(view); got != "Bob: short reply" {
			t.Fatalf("unfocused thread = %q, want only the latest reply on one row", got)
		}
	}
	replies[0].Body = strings.Repeat("long reply\n", 30)
	if got := app.renderThread(key, "Reviewer", body, replies, 40, false); lipgloss.Height(got) > maxThreadLines+1 || !strings.Contains(got, "↓") {
		t.Fatalf("long unfocused reply = %q", got)
	}
}

func TestSidebarNavigationShowsExpandedThread(t *testing.T) {
	app := setupAppWithDoc(t, "source\n")
	app.width, app.height = 120, 24
	app.recalculateLayout()
	for i := range 8 {
		app.tabs[0].state.Comments = append(app.tabs[0].state.Comments, review.Comment{
			ID: string(rune('a' + i)), Scope: "file", Body: strings.Repeat("history\n", 20),
			Replies: []review.Reply{{Author: "Bob", Body: "short reply"}},
		})
	}
	app.focused = commentPane
	app.updateCommentSidebar()
	app = pressKey(app, 'G')
	if app.commentViewport.YOffset() == 0 {
		t.Fatal("selecting the final thread did not scroll it into view")
	}
	if got := ansi.Strip(app.commentViewport.View()); !strings.Contains(got, "> File") || !strings.Contains(got, "short reply") {
		t.Fatalf("selected thread is not fully visible: %q", got)
	}
	app = pressKey(app, 'g')
	if app.commentViewport.YOffset() != 0 {
		t.Fatal("returning to the first thread did not restore the top of the sidebar")
	}
}

func TestThreadScrollingUsesSelectedSurface(t *testing.T) {
	for _, sidebar := range []bool{false, true} {
		t.Run(map[bool]string{false: "inline", true: "sidebar"}[sidebar], func(t *testing.T) {
			app := setupAppWithDoc(t, "source\n")
			app.width, app.height = 120, 40
			app.recalculateLayout()
			app.tabs[0].state.Comments = []review.Comment{{ID: "thread", StartLine: 1, Body: strings.Repeat("history\n", 20),
				Replies: []review.Reply{{ID: "reply", Body: "latest"}}}}
			app.tabs[0].cursorOnAnnotation = true
			app.tabs[0].cursorLine = 1
			if sidebar {
				app.focused = commentPane
			}
			app.rebuildContent()
			app.updateCommentSidebar()
			key := threadViewKey{path: app.tab().path, id: "thread", sidebar: sidebar}
			otherKey := threadViewKey{path: app.tab().path, id: "thread", sidebar: !sidebar}
			before, other := app.threadScrolls[key], app.threadScrolls[otherKey]
			updated, _ := app.Update(tea.KeyPressMsg{Code: tea.KeyPgUp, Mod: tea.ModCtrl})
			app = *updated.(*AppModel)
			if got := app.threadScrolls[key].offset; got != max(0, before.offset-before.height) {
				t.Fatalf("page-up offset = %d, before = %+v", got, before)
			}
			if app.threadScrolls[otherKey].offset != other.offset {
				t.Fatal("scrolling changed the other surface")
			}
			left, top, _, _ := app.contentBounds()
			x, y := left+gutterWidth+2, top+2
			if sidebar {
				left, top, _, _ = app.commentBounds()
				x, y = left+2, top+2
			}
			before = app.threadScrolls[key]
			app = wheelMouse(app, x, y, tea.MouseWheelUp)
			if got := app.threadScrolls[key].offset; got != max(0, before.offset-3) {
				t.Fatalf("wheel offset = %d, before = %+v", got, before)
			}
		})
	}
}

func TestResolvedThreadsExpandOnFocus(t *testing.T) {
	for _, surface := range []string{"inline", "sidebar"} {
		for _, action := range []string{"keyboard", "click", "wheel"} {
			t.Run(surface+"/"+action, func(t *testing.T) {
				app := setupAppWithDoc(t, "source\n")
				app.width, app.height = 120, 40
				app.recalculateLayout()
				comment := review.Comment{ID: "resolved", StartLine: 1, EndLine: 1, Resolved: true,
					Body: strings.Repeat("history\n", 20), Replies: []review.Reply{{ID: "reply", Author: "Codex", Body: "latest reply"}}}
				if surface == "sidebar" {
					comment.Scope = "file"
				}
				app.tabs[0].state.Comments = []review.Comment{comment}
				app.tabs[0].cursorLine = 1
				app.rebuildContent()
				app.updateCommentSidebar()
				view := func() string {
					if surface == "sidebar" {
						return ansi.Strip(app.commentViewport.View())
					}
					return ansi.Strip(app.contentViewport.View())
				}
				if got := view(); strings.Contains(got, "latest reply") || !strings.Contains(got, "resolved") {
					t.Fatalf("unfocused resolved thread = %q", got)
				}
				left, top, _, _ := app.contentBounds()
				x, y, key := left+gutterWidth+2, top+2, 'j'
				if surface == "sidebar" {
					left, top, _, _ = app.commentBounds()
					x, y, key = left+2, top+1, 's'
				}
				switch action {
				case "keyboard":
					app = pressKey(app, key)
				case "click":
					app = clickMouse(app, x, y)
				case "wheel":
					app = wheelMouse(app, x, y, tea.MouseWheelUp)
				}
				if got := view(); !strings.Contains(got, "history") || !strings.Contains(got, "resolved") {
					t.Fatalf("focused resolved thread = %q", got)
				}
				if !app.tab().state.Comments[0].Resolved {
					t.Fatal("expanding the thread changed its resolution")
				}
				scroll := app.threadScrolls[threadViewKey{path: app.tab().path, id: comment.ID, sidebar: surface == "sidebar"}]
				if action == "wheel" && (!scroll.manual || scroll.offset >= scroll.maxOffset) {
					t.Fatalf("wheel did not scroll the expanded thread: %+v", scroll)
				}
				app = pressKey(app, 's')
				if got := view(); strings.Contains(got, "history") || strings.Contains(got, "latest reply") {
					t.Fatalf("resolved thread did not collapse after losing focus: %q", got)
				}
			})
		}
	}
}

func TestResolvedThreadFocusWithVerticalMovement(t *testing.T) {
	for _, direction := range []rune{'j', 'k'} {
		for _, selecting := range []bool{false, true} {
			app := setupAppWithDoc(t, "one\ntwo\nthree\n")
			app.tabs[0].state.Comments = []review.Comment{{ID: "resolved", StartLine: 2, EndLine: 2, Body: "resolved body", Resolved: true}}
			app.tab().cursorLine = 2
			if direction == 'k' {
				app.tab().cursorLine = 3
			}
			if selecting {
				app = pressKey(app, 'v')
			}
			app = pressKey(app, direction)
			if app.tab().cursorOnAnnotation == selecting {
				t.Fatalf("direction %c, selecting %t: annotation focus = %t", direction, selecting, app.tab().cursorOnAnnotation)
			}
			if !selecting {
				app = pressKey(app, 'v')
				if app.tab().cursorOnAnnotation {
					t.Fatal("entering visual mode retained annotation focus")
				}
			}
		}
	}
}

func TestReplyModalStartsAtLatestHeaderAndKeepsFullLines(t *testing.T) {
	app := setupAppWithDoc(t, "context\n")
	app.width, app.height = 120, 50
	app.recalculateLayout()
	app.tabs[0].state.Comments = []review.Comment{{ID: "thread", Author: "Reviewer", StartLine: 1,
		Body: strings.Repeat("past\n", 30), Replies: []review.Reply{{ID: "reply", Author: "Codex", Body: "latest first\n" + strings.Repeat("latest\n", 30) + "latest last"}}}}
	app.openCommentThread("thread")
	background := lipgloss.NewStyle().Width(app.width).Height(app.height).Render("")
	rendered, regions := app.renderWithModalLayout(background)
	plain := ansi.Strip(rendered)
	if !strings.Contains(plain, "Codex (#1)") || !strings.Contains(plain, "latest first") || strings.Contains(plain, "latest last") {
		t.Fatalf("modal initial position = %q", plain)
	}
	for _, region := range regions {
		if region.action.scrollable {
			if height := region.rect.bottom - region.rect.top; height > 18 {
				t.Fatalf("reference height = %d, want at most 18 including borders", height)
			}
			app.modalReferenceOffset = region.action.scrollMaxOffset
		}
	}
	if got := ansi.Strip(app.renderWithModal(background)); !strings.Contains(got, "latest last") {
		t.Fatal("latest reply's final line is unreachable")
	}
	app.modalReferenceOffset = 0
	if got := ansi.Strip(app.renderWithModal(background)); !strings.Contains(got, "context") {
		t.Fatal("original code context is unreachable")
	}
	app.tabs[0].state.Comments[0].Replies[0].Body = "short final reply"
	app.modalReferenceOffset = -1
	rendered, regions = app.renderWithModalLayout(background)
	rows := strings.Split(ansi.Strip(rendered), "\n")
	for _, region := range regions {
		if region.action.scrollable && !strings.Contains(rows[region.rect.bottom-2], "short final reply") {
			t.Fatalf("blank padding below the latest modal reply: %q", rows[region.rect.top:region.rect.bottom])
		}
	}
}
