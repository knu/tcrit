package tui

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Match crit's bare @path syntax, also allowing extensionless files. Only the
// optional Ldigits suffix has no trailing boundary, so L40に and L40-45 work.
var fileReferencePattern = regexp.MustCompile(`(^|\s)@((?:[A-Za-z0-9._/-]|\\[^\r\n])+)(?: +L([0-9]+))?`)

func unescapeReferencePath(path string) string {
	var out strings.Builder
	escaped := false
	for _, r := range path {
		if r == '\\' && !escaped {
			escaped = true
			continue
		}
		out.WriteRune(r)
		escaped = false
	}
	return out.String()
}

func (m *AppModel) linkFileReferences(body string) string {
	var out strings.Builder
	end := 0
	for _, match := range fileReferencePattern.FindAllStringSubmatchIndex(body, -1) {
		if match[1] < len(body) && body[match[1]] == '\\' {
			continue // incomplete escape, not a reference to the shorter prefix
		}
		path := unescapeReferencePath(body[match[4]:match[5]])
		if !regularFile(m.sourcePath(path)) {
			continue
		}
		line := 0
		if match[6] >= 0 {
			var err error
			line, err = strconv.Atoi(body[match[6]:match[7]])
			if err != nil || line < 1 {
				continue
			}
		}
		start := match[4] - 1 // include @, preserve the preceding whitespace
		out.WriteString(body[end:start])
		uri := url.URL{Scheme: "tcrit", Host: "source", RawQuery: url.Values{
			"path": {path}, "line": {strconv.Itoa(line)},
		}.Encode()}
		out.WriteString(ansi.SetHyperlink(uri.String()))
		out.WriteString("\x1b[4m")
		out.WriteString(body[start:match[1]])
		out.WriteString("\x1b[24m")
		out.WriteString(ansi.ResetHyperlink())
		end = match[1]
	}
	out.WriteString(body[end:])
	return out.String()
}

func sourceLink(uri string) (sourceLocation, bool) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "tcrit" || u.Host != "source" {
		return sourceLocation{}, false
	}
	path := u.Query().Get("path")
	line, err := strconv.Atoi(u.Query().Get("line"))
	return sourceLocation{path: path, line: line}, err == nil && line >= 0 && path != ""
}

// Read the composed terminal cell rather than estimating offsets in the raw
// comment. Canvas preserves links through wrapping, clipping and wide glyphs.
func (m *AppModel) sourceReferenceAt(x, y int) (sourceLocation, bool) {
	if m.width <= 0 || m.height <= 0 || m.tab().state == nil {
		return sourceLocation{}, false
	}
	screen, _ := m.renderReviewScreen()
	canvas := lipgloss.NewCanvas(m.width, m.height)
	canvas.Compose(lipgloss.NewLayer(screen))
	cell := canvas.CellAt(x, y)
	if cell == nil {
		return sourceLocation{}, false
	}
	location, ok := sourceLink(cell.Link.URL)
	return location, ok && regularFile(m.sourcePath(location.path))
}
