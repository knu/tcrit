package tui

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/knu/tcrit/internal/review"
)

func TestFileReferenceSyntax(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, path := range []string{"file", "file.ext"} {
		if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	app := NewApp("file", AppConfig{})
	for _, tt := range []struct {
		body   string
		line   int
		linked bool
	}{
		{"@file", 0, true},
		{"@file.ext L40にある通り、...", 40, true},
		{"@file L40-45", 40, true},
		{"text @file  L999999x", 999999, true},
		{"@file\nL40", 0, true},
		{"@file\tL40", 0, true},
		{"@file L0", 0, false},
		{"@file L9999999999999999999999999999", 0, false},
		{"name@file.ext", 0, false},
		{"@missing L40", 0, false},
		{"@. L40", 0, false},
	} {
		t.Run(tt.body, func(t *testing.T) {
			rendered := app.linkFileReferences(tt.body)
			if ansi.Strip(rendered) != tt.body {
				t.Fatalf("reference rendering changed text: %q", rendered)
			}
			canvas := lipgloss.NewCanvas(100, 5)
			canvas.Compose(lipgloss.NewLayer(rendered))
			var found bool
			for y := 0; y < 5; y++ {
				for x := 0; x < 100; x++ {
					cell := canvas.CellAt(x, y)
					if cell == nil {
						continue
					}
					location, ok := sourceLink(cell.Link.URL)
					if ok {
						found = true
						if location.line != tt.line {
							t.Fatalf("line = %d", location.line)
						}
					}
					if cell.Content == "に" || cell.Content == "-" {
						if cell.Link.URL != "" {
							t.Fatalf("suffix is linked: %q", cell.Content)
						}
					}
				}
			}
			if found != tt.linked {
				t.Fatalf("linked = %v, want %v", found, tt.linked)
			}
		})
	}
}

func TestRenderedReplyReferenceClick(t *testing.T) {
	app := setupAppWithDoc(t, strings.Repeat("line\n", 20))
	app.width, app.height = 100, 35
	app.recalculateLayout()
	app.tab().state.Comments = []review.Comment{{
		ID: "c_test", StartLine: 1, EndLine: 1, Author: "Tester", Body: "original",
		Replies: []review.Reply{{ID: "r_test", Author: "Codex", Body: "日本語 " + strings.Repeat("text ", 5) + "@" + app.tab().path + " L15に追加しました。"}},
	}}
	app.tab().cursorLine, app.tab().cursorOnAnnotation = 1, true
	app.rebuildContent()
	app.updateCommentSidebar()
	// The absolute path is intentionally long enough to wrap.
	screen, _ := app.renderReviewScreen()
	canvas := lipgloss.NewCanvas(app.width, app.height)
	canvas.Compose(lipgloss.NewLayer(screen))
	linkedRows := make(map[int]bool)
	var clickX, clickY int
	for y := 0; y < app.height; y++ {
		for x := 0; x < app.contentViewport.Width(); x++ {
			cell := canvas.CellAt(x, y)
			if cell == nil {
				continue
			}
			if location, ok := sourceLink(cell.Link.URL); ok && location.line == 15 {
				linkedRows[y] = true
				clickX, clickY = x, y
			}
		}
	}
	if len(linkedRows) < 2 {
		t.Fatalf("wrapped link spans %d rows", len(linkedRows))
	}
	app, cmd := clickMouseCmd(app, clickX, clickY)
	if cmd != nil || app.modal != noModal || app.tab().cursorLine != 15 || app.tab().cursorOnAnnotation {
		t.Fatal("click on wrapped reply reference did not navigate")
	}
	app.modal = helpModal
	_, cmd = updateApp(app, tea.MouseClickMsg{X: clickX, Y: clickY, Button: tea.MouseLeft})
	if cmd != nil {
		t.Fatal("background link activated under modal")
	}
}

func TestEscapedFileReferences(t *testing.T) {
	t.Chdir(t.TempDir())
	app := NewApp("file", AppConfig{})
	for _, tt := range []struct{ path, reference string }{
		{"my file.md", `@my\ file.md L40にある通り`},
		{`back\slash.go`, `@back\\slash.go L40-45`},
		{"file(@name).go", `@file\(\@name\).go L40`},
		{"日本.md", `@\日\本.md L40`},
	} {
		t.Run(tt.path, func(t *testing.T) {
			if err := os.WriteFile(tt.path, []byte("x"), 0600); err != nil {
				t.Fatal(err)
			}
			rendered := app.linkFileReferences(tt.reference)
			if ansi.Strip(rendered) != tt.reference {
				t.Fatal("display must preserve escapes as written")
			}
			canvas := lipgloss.NewCanvas(100, 1)
			canvas.Compose(lipgloss.NewLayer(rendered))
			cell := canvas.CellAt(0, 0)
			location, ok := sourceLink(cell.Link.URL)
			if !ok || location.path != tt.path || location.line != 40 {
				t.Fatalf("location = %#v, linked = %v", location, ok)
			}
		})
	}
	if err := os.WriteFile("prefix", []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"@prefix\\", "@prefix\\\nnext"} {
		if strings.Contains(app.linkFileReferences(body), "tcrit://") {
			t.Fatal("incomplete escape linked a shorter filename")
		}
	}
}
