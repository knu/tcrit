package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/knu/tcrit/internal/document"
	gitpkg "github.com/knu/tcrit/internal/git"
	"github.com/knu/tcrit/internal/review"
)

type pane int

const (
	contentPane pane = iota
	commentPane
)

type modalType int

const (
	noModal modalType = iota
	commentModal
	fileCommentModal
	replyModal
	editModal
	discardChangesModal
	deleteConfirmModal
	finishModal
	helpModal
)

// FinishEvent is emitted on the finish channel when the reviewer finishes a
// round.  The runner turns it into a payload for blocked agent clients.
type FinishEvent struct {
	Approved bool
}

// RoundStartMsg tells the TUI an agent requested the next review round:
// reload comments (including agent replies) and file contents, and advance
// the round counter.
type RoundStartMsg struct{}

// AppConfig carries the cross-cutting dependencies of the TUI.
type AppConfig struct {
	Session *review.Session
	Author  string
	// Serving is true when an agent client may be blocked on this review;
	// an unresolved finish then parks the TUI in a waiting state instead
	// of quitting.
	Serving  bool
	FinishCh chan<- FinishEvent
}

// gutterWidth is the total width of the left gutter: line number (5) + marker (1) + space (1).
const gutterWidth = 7

const displayTabWidth = 4

type AppModel struct {
	width, height int
	focused       pane
	modal         modalType

	// Multi-file tabs (code review mode)
	tabs         []FileTab
	activeTab    int
	multiFile    bool // true when in code review mode
	tabSearching bool
	tabSearch    string
	tabMatches   []int // indices of matching tabs during search

	// Single-file mode (legacy)
	filePath string

	// Review session backing all tabs, and the author stamped on new comments.
	session *review.Session
	author  string

	// Finish-flow state (see AppConfig).
	serving  bool
	finishCh chan<- FinishEvent
	waiting  bool
	baseRef  string

	detached bool

	contentViewport   viewport.Model
	commentViewport   viewport.Model
	modalTextarea     textarea.Model
	mouseSelecting    bool
	hoveredGutterLine int
	hoveredGutterSide string
	contentLayout     renderedContentLayout
	sidebarTargets    []int

	// Editing state
	editingID            string // ID of the parent comment being edited or replied to
	editingReplyID       string // ID of the reply being edited; empty when editing the parent
	modalInitial         string // textarea value when the current text modal opened
	modalReferenceOffset int
	discardReturn        modalType
	deleteReturn         modalType
	pendingDelete        int
	modalFocus           int  // focus index within the active modal
	newFeedback          bool // true after adding or editing a comment in this round

	err error
}

type renderedRange struct {
	start int
	end   int
}

type mouseRect struct {
	left   int
	top    int
	right  int
	bottom int
}

func (r mouseRect) contains(mouse tea.Mouse) bool {
	return mouse.X >= r.left && mouse.X < r.right && mouse.Y >= r.top && mouse.Y < r.bottom
}

type renderedContentLayout struct {
	rows       []contentMouseTarget
	lineRanges map[int]renderedRange
	oldRanges  map[int]renderedRange
}

type renderedScreenLayout struct {
	footerFinish    mouseRect
	hasFooterFinish bool
	modalRegions    []modalMouseRegion
}

func newRenderedContentLayout() renderedContentLayout {
	return renderedContentLayout{
		lineRanges: make(map[int]renderedRange),
		oldRanges:  make(map[int]renderedRange),
	}
}

func (l *renderedContentLayout) appendBlock(b *strings.Builder, block string, target contentMouseTarget) {
	block = strings.TrimSuffix(block, "\n")
	start := len(l.rows)
	for _, row := range strings.Split(block, "\n") {
		b.WriteString(row)
		b.WriteByte('\n')
		l.rows = append(l.rows, target)
	}
	if target.line <= 0 {
		return
	}
	ranges := l.lineRanges
	if target.side == "old" {
		ranges = l.oldRanges
	}
	r, ok := ranges[target.line]
	if !ok {
		r.start = start
	}
	r.end = len(l.rows)
	ranges[target.line] = r
}

// tab returns the active FileTab. Panics if no tabs exist.
func (m *AppModel) tab() *FileTab {
	return &m.tabs[m.activeTab]
}

func NewApp(filePath string, cfg AppConfig) AppModel {
	ta := textarea.New()
	ta.Placeholder = "Type your comment..."
	ta.ShowLineNumbers = false

	tab := FileTab{
		path:       filePath,
		cursorLine: 1,
	}

	return AppModel{
		filePath:        filePath,
		tabs:            []FileTab{tab},
		activeTab:       0,
		session:         cfg.Session,
		author:          cfg.Author,
		serving:         cfg.Serving,
		finishCh:        cfg.FinishCh,
		detached:        os.Getenv("TCRIT_DETACHED") == "1",
		contentViewport: viewport.New(),
		commentViewport: viewport.New(),
		modalTextarea:   ta,
	}
}

// NewCodeReviewApp creates a multi-file code review TUI.
func NewCodeReviewApp(files []gitpkg.FileChange, ref string, cfg AppConfig) AppModel {
	ta := textarea.New()
	ta.Placeholder = "Type your comment..."
	ta.ShowLineNumbers = false

	// Sort files alphabetically by path
	sortedFiles := make([]gitpkg.FileChange, len(files))
	copy(sortedFiles, files)
	sort.Slice(sortedFiles, func(i, j int) bool {
		return sortedFiles[i].Path < sortedFiles[j].Path
	})

	tabs := make([]FileTab, 0, len(sortedFiles))
	for _, f := range sortedFiles {
		var diff *gitpkg.DiffInfo
		if f.Status != gitpkg.StatusBinary {
			diff, _ = gitpkg.DiffFile(f.Path, ref)
		}
		ft := newFileTab(f.Path, diff)
		if f.Status == gitpkg.StatusBinary {
			ft.isBinary = true
		}
		if f.Status == gitpkg.StatusDeleted {
			ft.isDeleted = true
		}
		tabs = append(tabs, ft)
	}

	return AppModel{
		tabs:            tabs,
		activeTab:       0,
		multiFile:       true,
		session:         cfg.Session,
		author:          cfg.Author,
		serving:         cfg.Serving,
		finishCh:        cfg.FinishCh,
		baseRef:         ref,
		detached:        os.Getenv("TCRIT_DETACHED") == "1",
		contentViewport: viewport.New(),
		commentViewport: viewport.New(),
		modalTextarea:   ta,
	}
}

func (m AppModel) Init() tea.Cmd {
	return tea.Batch(m.loadDocuments(), tea.RequestBackgroundColor)
}

func (m AppModel) loadDocuments() tea.Cmd {
	return func() tea.Msg {
		for _, tab := range m.tabs {
			if tab.isBinary || tab.isDeleted {
				continue
			}
			if _, err := document.Load(tab.path); err != nil {
				return errMsg{err}
			}
		}
		return docRenderedMsg{}
	}
}

// selectionRange returns the ordered start/end of the current selection.
// If not selecting, returns cursorLine, cursorLine.
func (m *AppModel) selectionRange() (int, int) {
	t := m.tab()
	if !t.selecting {
		return t.cursorLine, t.cursorLine
	}
	start, end := t.selectAnchor, t.cursorLine
	if start > end {
		start, end = end, start
	}
	return start, end
}

func (m *AppModel) selectionSide() string {
	t := m.tab()
	if t.selecting {
		return t.selectSide
	}
	return t.cursorSide
}

type lineRef struct {
	side string
	line int
}

func (m *AppModel) visualLines(t *FileTab) []lineRef {
	if t.doc == nil {
		return nil
	}
	lines := make([]lineRef, 0, t.doc.LineCount()+len(t.deletedAfter))
	for line := 1; line <= t.doc.LineCount(); line++ {
		for _, del := range t.deletedAfter[line-1] {
			lines = append(lines, lineRef{side: "old", line: del.OldLineNum})
		}
		lines = append(lines, lineRef{line: line})
	}
	for _, del := range t.deletedAfter[t.doc.LineCount()] {
		lines = append(lines, lineRef{side: "old", line: del.OldLineNum})
	}
	return lines
}

func (m *AppModel) adjacentLine(t *FileTab, step int) (lineRef, bool) {
	lines := m.visualLines(t)
	for i, ref := range lines {
		if ref.line != t.cursorLine || ref.side != t.cursorSide {
			continue
		}
		next := i + step
		if next >= 0 && next < len(lines) {
			return lines[next], true
		}
		break
	}
	return lineRef{}, false
}

func (m *AppModel) visualLineIndex(t *FileTab, target lineRef) int {
	for i, ref := range m.visualLines(t) {
		if ref == target {
			return i
		}
	}
	return -1
}

func (m *AppModel) moveCursorBy(t *FileTab, step, count int) {
	for range count {
		next, ok := m.adjacentLine(t, step)
		if !ok || (t.selecting && next.side != t.selectSide) {
			return
		}
		t.cursorLine, t.cursorSide = next.line, next.side
	}
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		initAdaptiveStyles(msg.IsDark())
		if len(m.tabs) > 0 && m.tab().state != nil {
			m.rebuildContent()
			m.updateCommentSidebar()
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalculateLayout()
		if len(m.tabs) > 0 && m.tab().state != nil {
			m.rebuildContent()
			m.updateCommentSidebar()
		}
		return m, nil

	case docRenderedMsg:
		// Load documents and existing review comments for each tab
		for i := range m.tabs {
			t := &m.tabs[i]
			t.state = &fileReview{Comments: m.sessionComments(t.path)}
			if t.isBinary {
				continue
			}
			if t.isDeleted {
				t.doc = &document.Document{Path: t.path}
				t.ensureHighlightCache()
				if lines := m.visualLines(t); len(lines) > 0 {
					t.cursorLine, t.cursorSide = lines[0].line, lines[0].side
				}
				continue
			}
			doc, _ := document.Load(t.path)
			t.doc = doc
			t.ensureHighlightCache()
		}

		m.recalculateLayout()
		m.rebuildContent()
		m.updateCommentSidebar()
		return m, nil

	case editorFinishedMsg:
		m.finishExternalEdit(msg)
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil

	case RoundStartMsg:
		m.startNextRound()
		return m, nil

	case tea.MouseClickMsg:
		return m.handleMouseClick(msg)
	case tea.MouseMotionMsg:
		return m.handleMouseMotion(msg)
	case tea.MouseReleaseMsg:
		return m.handleMouseRelease(msg)
	case tea.MouseWheelMsg:
		return m.handleMouseWheel(msg)

	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)
	}

	var cmd tea.Cmd
	if m.modal == commentModal || m.modal == fileCommentModal || m.modal == replyModal || m.modal == editModal {
		m.modalTextarea, cmd = m.modalTextarea.Update(msg)
		return m, cmd
	}

	switch m.focused {
	case contentPane:
		m.contentViewport, cmd = m.contentViewport.Update(msg)
	case commentPane:
		m.commentViewport, cmd = m.commentViewport.Update(msg)
	}

	return m, cmd
}

func (m *AppModel) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.hoveredGutterLine != 0 {
		m.hoveredGutterLine = 0
		m.hoveredGutterSide = ""
		m.rebuildContent()
	}
	if m.modal == helpModal {
		if key.Matches(msg, keys.Help) || key.Matches(msg, keys.Cancel) {
			m.modal = noModal
		}
		return m, nil
	}
	if m.modal == discardChangesModal {
		return m.handleDiscardChangesModal(msg)
	}
	if m.modal == deleteConfirmModal {
		return m.handleDeleteConfirmModal(msg)
	}
	if m.modal == commentModal || m.modal == fileCommentModal || m.modal == replyModal || m.modal == editModal {
		return m.handleTextModal(msg)
	}
	if m.modal == finishModal {
		return m.handleFinishModal(msg)
	}

	// While waiting for the agent's next round, only quitting is possible.
	if m.waiting {
		if key.Matches(msg, keys.Quit) {
			return m, tea.Quit
		}
		return m, nil
	}

	// Tab search input mode
	if m.tabSearching {
		return m.handleTabSearch(msg)
	}

	t := m.tab()

	switch {
	case key.Matches(msg, keys.Quit):
		// Finishing is an explicit act: q opens the Approve/Finish modal.
		m.openFinishModal()
		return m, nil

	case key.Matches(msg, keys.Cancel):
		// Esc cancels selection
		if t.selecting {
			t.selecting = false
			m.rebuildContent()
			return m, nil
		}
		return m, nil

	case key.Matches(msg, keys.Help):
		m.modal = helpModal
		return m, nil

	case key.Matches(msg, keys.Tab):
		if !t.selecting {
			if m.focused == contentPane {
				m.focused = commentPane
			} else {
				m.focused = contentPane
			}
			m.updateCommentSidebar()
			m.rebuildContent()
		}
		return m, nil

	case key.Matches(msg, keys.VisualMode):
		if m.focused == contentPane && t.doc != nil {
			if t.selecting {
				t.selecting = false
			} else {
				t.selecting = true
				t.selectAnchor = t.cursorLine
				t.selectSide = t.cursorSide
			}
			m.rebuildContent()
			return m, nil
		}

	case key.Matches(msg, keys.FileComment):
		if !t.selecting && t.state != nil {
			m.modal = fileCommentModal
			m.modalFocus = 0
			m.modalTextarea.Placeholder = "Type your file comment..."
			m.modalTextarea.Reset()
			m.modalInitial = ""
			m.modalTextarea.Focus()
		}
		return m, nil

	case key.Matches(msg, keys.Delete):
		if !t.selecting {
			m.openSelectedCommentDelete()
		}
		return m, nil
	}

	// Tab switching (multi-file mode)
	if m.multiFile && m.focused == contentPane && !t.selecting {
		switch {
		case key.Matches(msg, keys.PrevTab):
			if m.activeTab > 0 {
				m.activeTab--
				m.rebuildContent()
				m.updateCommentSidebar()
			}
			return m, nil
		case key.Matches(msg, keys.NextTab):
			if m.activeTab < len(m.tabs)-1 {
				m.activeTab++
				m.rebuildContent()
				m.updateCommentSidebar()
			}
			return m, nil
		case key.Matches(msg, keys.TabSearch):
			m.tabSearching = true
			m.tabSearch = ""
			m.tabMatches = nil
			return m, nil
		}
		// Number keys 1-9 for direct tab access
		if s := msg.String(); len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
			idx := int(s[0]-'0') - 1
			if idx < len(m.tabs) {
				m.activeTab = idx
				m.rebuildContent()
				m.updateCommentSidebar()
			}
			return m, nil
		}
	}

	if m.focused == contentPane && !t.selecting {
		switch {
		case key.Matches(msg, keys.NextComment):
			m.jumpToComment(1)
			return m, nil
		case key.Matches(msg, keys.PrevComment):
			m.jumpToComment(-1)
			return m, nil
		case key.Matches(msg, keys.NextChange):
			m.jumpToChange(1)
			return m, nil
		case key.Matches(msg, keys.PrevChange):
			m.jumpToChange(-1)
			return m, nil
		}
	}

	// Content pane cursor movement (annotation-aware)
	if m.focused == contentPane && t.doc != nil {
		moved := false
		switch {
		case key.Matches(msg, keys.Down):
			if t.cursorOnAnnotation {
				anns := m.annotationsAfterLine(t.cursorLine, t.cursorSide)
				if t.cursorAnnoIdx < len(anns)-1 {
					t.cursorAnnoIdx++
				} else {
					t.cursorOnAnnotation = false
					t.cursorAnnoIdx = 0
					if next, ok := m.adjacentLine(t, 1); ok && (!t.selecting || next.side == t.selectSide) {
						t.cursorLine, t.cursorSide = next.line, next.side
					}
				}
			} else {
				anns := m.annotationsAfterLine(t.cursorLine, t.cursorSide)
				if len(anns) > 0 {
					t.cursorOnAnnotation = true
					t.cursorAnnoIdx = 0
				} else if next, ok := m.adjacentLine(t, 1); ok && (!t.selecting || next.side == t.selectSide) {
					t.cursorLine, t.cursorSide = next.line, next.side
				}
			}
			moved = true
		case key.Matches(msg, keys.Up):
			if t.cursorOnAnnotation {
				if t.cursorAnnoIdx > 0 {
					t.cursorAnnoIdx--
				} else {
					t.cursorOnAnnotation = false
					t.cursorAnnoIdx = 0
				}
			} else {
				if prev, ok := m.adjacentLine(t, -1); ok && (!t.selecting || prev.side == t.selectSide) {
					anns := m.annotationsAfterLine(prev.line, prev.side)
					if len(anns) > 0 {
						t.cursorLine, t.cursorSide = prev.line, prev.side
						t.cursorOnAnnotation = true
						t.cursorAnnoIdx = len(anns) - 1
					} else {
						t.cursorLine, t.cursorSide = prev.line, prev.side
					}
				}
			}
			moved = true
		case key.Matches(msg, keys.HalfPageDown):
			t.cursorOnAnnotation = false
			t.cursorAnnoIdx = 0
			jump := m.contentViewport.Height() / 2
			m.moveCursorBy(t, 1, jump)
			moved = true
		case key.Matches(msg, keys.HalfPageUp):
			t.cursorOnAnnotation = false
			t.cursorAnnoIdx = 0
			jump := m.contentViewport.Height() / 2
			m.moveCursorBy(t, -1, jump)
			moved = true
		case key.Matches(msg, keys.Top):
			t.cursorOnAnnotation = false
			t.cursorAnnoIdx = 0
			if lines := m.visualLines(t); len(lines) > 0 && (!t.selecting || lines[0].side == t.selectSide) {
				t.cursorLine, t.cursorSide = lines[0].line, lines[0].side
			}
			moved = true
		case key.Matches(msg, keys.Bottom):
			t.cursorOnAnnotation = false
			t.cursorAnnoIdx = 0
			if lines := m.visualLines(t); len(lines) > 0 && (!t.selecting || lines[len(lines)-1].side == t.selectSide) {
				last := lines[len(lines)-1]
				t.cursorLine, t.cursorSide = last.line, last.side
			}
			moved = true
		case key.Matches(msg, keys.Resolve):
			if t.cursorOnAnnotation {
				anns := m.annotationsAfterLine(t.cursorLine, t.cursorSide)
				if t.cursorAnnoIdx < len(anns) {
					m.toggleResolve(anns[t.cursorAnnoIdx].id)
				}
				return m, nil
			}
		case key.Matches(msg, keys.Confirm):
			if t.cursorOnAnnotation {
				anns := m.annotationsAfterLine(t.cursorLine, t.cursorSide)
				if t.cursorAnnoIdx < len(anns) {
					m.openCommentThread(anns[t.cursorAnnoIdx].id)
					return m, nil
				}
			} else if t.state != nil {
				m.openLineComment()
				return m, nil
			}
		}

		if moved {
			m.rebuildContent()
			m.scrollToCursor()
			return m, nil
		}
	}

	// Comment pane navigation
	if m.focused == commentPane && len(t.sidebarItems) > 0 {
		sidebarMoved := false
		switch {
		case key.Matches(msg, keys.Down):
			if t.sidebarCursor < len(t.sidebarItems)-1 {
				t.sidebarCursor++
				sidebarMoved = true
			}
		case key.Matches(msg, keys.Up):
			if t.sidebarCursor > 0 {
				t.sidebarCursor--
				sidebarMoved = true
			}
		case key.Matches(msg, keys.Top):
			t.sidebarCursor = 0
			sidebarMoved = true
		case key.Matches(msg, keys.Bottom):
			t.sidebarCursor = len(t.sidebarItems) - 1
			sidebarMoved = true
		}
		if sidebarMoved {
			m.updateCommentSidebar()
			m.rebuildContent()
			sel := t.sidebarItems[t.sidebarCursor]
			if sel.scope != "file" {
				t.cursorLine, t.cursorSide = sel.line, sel.side
				m.scrollToAnnotation(sel.side, sel.line, sel.endLine)
			}
			return m, nil
		}

		// Toggle resolution on the selected annotation
		if key.Matches(msg, keys.Resolve) {
			m.toggleResolve(t.sidebarItems[t.sidebarCursor].id)
			return m, nil
		}

		// Enter to edit selected annotation
		if key.Matches(msg, keys.Confirm) {
			sel := t.sidebarItems[t.sidebarCursor]
			m.openCommentThread(sel.id)
			return m, nil
		}
	}

	return m, nil
}

func (m *AppModel) openCommentThread(id string) {
	t := m.tab()
	if t.state == nil {
		return
	}
	for i := range t.state.Comments {
		c := &t.state.Comments[i]
		if c.ID != id {
			continue
		}
		m.editingID = c.ID
		m.editingReplyID = ""
		m.modalReferenceOffset = -1
		m.modalFocus = 0
		m.modalTextarea.Reset()
		if reply := m.latestOwnReply(c); reply != nil {
			m.editingReplyID = reply.ID
			m.modal = editModal
			m.modalTextarea.SetValue(reply.Body)
			m.modalTextarea.Placeholder = "Edit reply..."
		} else if len(c.Replies) > 0 || !m.authoredThisRound(c.Author, c.ReviewRound) {
			m.modal = replyModal
			m.modalTextarea.Placeholder = "Write a reply..."
		} else {
			m.modal = editModal
			m.modalTextarea.SetValue(c.Body)
			m.modalTextarea.Placeholder = "Edit comment..."
		}
		m.modalInitial = m.modalTextarea.Value()
		m.modalTextarea.Focus()
		return
	}
}

func (m *AppModel) selectedCommentID() string {
	t := m.tab()
	switch m.focused {
	case contentPane:
		if !t.cursorOnAnnotation {
			return ""
		}
		annotations := m.annotationsAfterLine(t.cursorLine, t.cursorSide)
		if t.cursorAnnoIdx < len(annotations) {
			return annotations[t.cursorAnnoIdx].id
		}
	case commentPane:
		if t.sidebarCursor < len(t.sidebarItems) {
			return t.sidebarItems[t.sidebarCursor].id
		}
	}
	return ""
}

func (m *AppModel) openSelectedCommentDelete() {
	id := m.selectedCommentID()
	if id == "" {
		return
	}
	m.editingID = id
	m.editingReplyID = ""
	if len(m.modalDeleteTargets()) == 0 {
		m.editingID = ""
		return
	}
	m.openDeleteConfirmation(0)
}

func (m *AppModel) openLineComment() {
	if m.tab().state == nil {
		return
	}
	m.modal = commentModal
	m.modalFocus = 0
	m.modalTextarea.Placeholder = "Type your comment..."
	m.modalTextarea.Reset()
	m.modalInitial = ""
	m.modalTextarea.Focus()
}

func (m *AppModel) latestOwnReply(c *review.Comment) *review.Reply {
	for i := len(c.Replies) - 1; i >= 0; i-- {
		if m.authoredThisRound(c.Replies[i].Author, c.Replies[i].ReviewRound) {
			return &c.Replies[i]
		}
	}
	return nil
}

func (m *AppModel) modalSubmit() {
	t := m.tab()
	body := strings.TrimSpace(m.modalTextarea.Value())
	if body == "" || t.state == nil {
		return
	}
	var addedLineComment *commentTarget
	var addedFileCommentID string

	switch m.modal {
	case editModal:
		for i := range t.state.Comments {
			c := &t.state.Comments[i]
			if c.ID != m.editingID {
				continue
			}
			if m.editingReplyID == "" {
				c.Body = body
			} else {
				for j := range c.Replies {
					if c.Replies[j].ID == m.editingReplyID {
						c.Replies[j].Body = body
						break
					}
				}
			}
			c.UpdatedAt = review.Now()
			break
		}
	case replyModal:
		now := review.Now()
		for i := range t.state.Comments {
			c := &t.state.Comments[i]
			if c.ID != m.editingID {
				continue
			}
			c.Replies = append(c.Replies, review.Reply{
				ID:          review.RandomReplyID(),
				Body:        body,
				Author:      m.author,
				CreatedAt:   now,
				ReviewRound: m.reviewRound(),
			})
			c.UpdatedAt = now
			c.Resolved = false
			c.ResolvedRound = 0
			break
		}
	case commentModal:
		startLine, endLine := m.selectionRange()
		side := m.selectionSide()
		now := review.Now()
		c := review.Comment{
			ID:        review.RandomCommentID(),
			StartLine: startLine,
			EndLine:   endLine,
			Side:      side,
			Anchor:    m.anchorText(t, side, startLine, endLine),
			Body:      body,
			Author:    m.author,
			Scope:     "line",
			CreatedAt: now,
			UpdatedAt: now,
		}
		if m.session != nil {
			c.ReviewRound = m.session.CJ.ReviewRound
		}
		addedLineComment = &commentTarget{
			line: endLine, side: side, annoIdx: len(m.annotationsAfterLine(endLine, side)),
		}
		t.state.Comments = append(t.state.Comments, c)
	case fileCommentModal:
		now := review.Now()
		c := review.Comment{
			ID:        review.RandomCommentID(),
			Body:      body,
			Author:    m.author,
			Scope:     "file",
			CreatedAt: now,
			UpdatedAt: now,
		}
		if m.session != nil {
			c.ReviewRound = m.session.CJ.ReviewRound
		}
		addedFileCommentID = c.ID
		t.state.Comments = append(t.state.Comments, c)
	}
	m.newFeedback = true
	m.editingID = ""
	m.editingReplyID = ""
	m.modalInitial = ""
	m.discardReturn = noModal
	m.deleteReturn = noModal

	m.persist()
	m.modal = noModal
	m.modalTextarea.Blur()
	t.selecting = false
	if addedLineComment != nil {
		m.focused = contentPane
		m.selectComment(m.activeTab, *addedLineComment)
		return
	}
	m.rebuildContent()
	m.updateCommentSidebar()
	if addedFileCommentID != "" {
		for i, item := range t.sidebarItems {
			if item.id == addedFileCommentID {
				m.focused = commentPane
				t.sidebarCursor = i
				m.updateCommentSidebar()
				break
			}
		}
	}
}

type modalDeleteTarget struct {
	replyID string
	label   string
}

func (m *AppModel) reviewRound() int {
	if m.session == nil {
		return 0
	}
	return m.session.CJ.ReviewRound
}

func (m *AppModel) authoredThisRound(author string, round int) bool {
	return author == m.author && round == m.reviewRound()
}

func (m *AppModel) modalDeleteTargets() []modalDeleteTarget {
	t := m.tab()
	if t.state == nil || m.editingID == "" {
		return nil
	}
	for _, c := range t.state.Comments {
		if c.ID != m.editingID {
			continue
		}
		if m.editingReplyID == "" {
			if len(c.Replies) == 0 && m.authoredThisRound(c.Author, c.ReviewRound) {
				return []modalDeleteTarget{{label: "Delete comment"}}
			}
			return nil
		}
		for i := range c.Replies {
			reply := &c.Replies[i]
			if reply.ID == m.editingReplyID && m.authoredThisRound(reply.Author, reply.ReviewRound) {
				return []modalDeleteTarget{{
					replyID: reply.ID,
					label:   fmt.Sprintf("Delete reply %d", i+1),
				}}
			}
		}
		return nil
	}
	return nil
}

func (m *AppModel) modalDelete(targetIndex int) {
	t := m.tab()
	if t.state == nil || m.editingID == "" {
		return
	}
	targets := m.modalDeleteTargets()
	if targetIndex < 0 || targetIndex >= len(targets) {
		return
	}
	target := targets[targetIndex]
	for i, c := range t.state.Comments {
		if c.ID != m.editingID {
			continue
		}
		if target.replyID == "" {
			t.state.Comments = append(t.state.Comments[:i], t.state.Comments[i+1:]...)
		} else {
			for j, reply := range c.Replies {
				if reply.ID == target.replyID {
					t.state.Comments[i].Replies = append(c.Replies[:j], c.Replies[j+1:]...)
					t.state.Comments[i].UpdatedAt = review.Now()
					break
				}
			}
		}
		break
	}
	m.editingID = ""
	m.editingReplyID = ""
	m.modalInitial = ""
	m.discardReturn = noModal
	m.deleteReturn = noModal
	m.persist()
	m.modal = noModal
	m.modalTextarea.Blur()
	t.cursorOnAnnotation = false
	t.cursorAnnoIdx = 0
	m.rebuildContent()
	m.updateCommentSidebar()
}

// totalComments counts comments across all tabs.
func (m *AppModel) totalComments() int {
	n := 0
	for i := range m.tabs {
		if m.tabs[i].state != nil {
			n += len(m.tabs[i].state.Comments)
		}
	}
	return n
}

// toggleResolve flips a comment's resolution state, stamping ResolvedRound
// with the current round on resolve (mirroring crit's reply semantics).
func (m *AppModel) toggleResolve(id string) {
	t := m.tab()
	if t.state == nil {
		return
	}
	round := 0
	if m.session != nil {
		round = m.session.CJ.ReviewRound
	}
	for i := range t.state.Comments {
		if t.state.Comments[i].ID != id {
			continue
		}
		c := &t.state.Comments[i]
		if c.Resolved {
			c.Resolved = false
			c.ResolvedRound = 0
		} else {
			c.Resolved = true
			c.ResolvedRound = round
		}
		c.UpdatedAt = review.Now()
		break
	}
	m.persist()
	m.rebuildContent()
	m.updateCommentSidebar()
}

// resolveAll marks every comment thread resolved in the current round.
func (m *AppModel) resolveAll() {
	round := 0
	if m.session != nil {
		round = m.session.CJ.ReviewRound
	}
	now := review.Now()
	resolve := func(c *review.Comment) {
		if c.Resolved {
			return
		}
		c.Resolved = true
		c.ResolvedRound = round
		c.UpdatedAt = now
	}

	for i := range m.tabs {
		if m.tabs[i].state == nil {
			continue
		}
		for j := range m.tabs[i].state.Comments {
			resolve(&m.tabs[i].state.Comments[j])
		}
	}
	if m.session != nil {
		for i := range m.session.CJ.ReviewComments {
			resolve(&m.session.CJ.ReviewComments[i])
		}
		for path, file := range m.session.CJ.Files {
			for i := range file.Comments {
				resolve(&file.Comments[i])
			}
			m.session.CJ.Files[path] = file
		}
	}
	m.persist()
}

// sessionComments returns the session's stored comments for a file path.
func (m *AppModel) sessionComments(path string) []review.Comment {
	if m.session == nil {
		return []review.Comment{}
	}
	comments := m.session.FileComments(path)
	if comments == nil {
		comments = []review.Comment{}
	}
	return comments
}

// anchorText joins the full text of lines start..end as the comment's
// drift-correction anchor.
func (m *AppModel) anchorText(t *FileTab, side string, start, end int) string {
	if t.doc == nil {
		return ""
	}
	lines := make([]string, 0, end-start+1)
	if side == "old" {
		byLine := make(map[int]string)
		for _, dels := range t.deletedAfter {
			for _, del := range dels {
				if del.OldLineNum >= start && del.OldLineNum <= end {
					byLine[del.OldLineNum] = del.Content
				}
			}
		}
		for line := start; line <= end; line++ {
			if content, ok := byLine[line]; ok {
				lines = append(lines, content)
			}
		}
		return strings.Join(lines, "\n")
	}
	for l := start; l <= end && l <= t.doc.LineCount(); l++ {
		lines = append(lines, t.doc.LineAt(l))
	}
	return strings.Join(lines, "\n")
}

func (m *AppModel) insertSuggestion() {
	start, end, ok := m.suggestionRange()
	if !ok {
		return
	}
	code := m.anchorText(m.tab(), "", start, end)
	if code == "" {
		return
	}
	body := strings.TrimRight(m.modalTextarea.Value(), "\n")
	if body != "" {
		body += "\n\n"
	}
	m.modalTextarea.SetValue(body + "```suggestion\n" + code + "\n```")
	m.modalTextarea.CursorStart()
	m.modalTextarea.CursorUp()
	m.modalTextarea.CursorEnd()
	// Populate the internal viewport before repositioning it around the cursor.
	_ = m.modalTextarea.View()
	m.modalTextarea.SetHeight(m.modalTextarea.Height())
	m.modalFocus = 0
	m.modalTextarea.Focus()
}

func (m *AppModel) suggestionRange() (int, int, bool) {
	if m.modal == commentModal {
		if m.selectionSide() == "old" {
			return 0, 0, false
		}
		start, end := m.selectionRange()
		return start, end, true
	}
	if m.modal != replyModal && m.modal != editModal {
		return 0, 0, false
	}
	t := m.tab()
	if t.state == nil {
		return 0, 0, false
	}
	for _, c := range t.state.Comments {
		if c.ID == m.editingID && c.Scope != "file" && c.Side != "old" && c.StartLine > 0 {
			return c.StartLine, c.EndAt(), true
		}
	}
	return 0, 0, false
}

func (m *AppModel) canSuggest() bool {
	_, _, ok := m.suggestionRange()
	return ok
}

func (m *AppModel) modalDeleteStartFocus() int {
	if m.canSuggest() {
		return 4
	}
	return 3
}

// persist writes every tab's comments back into the session and saves it.
func (m *AppModel) persist() {
	if m.session == nil {
		return
	}
	for i := range m.tabs {
		if m.tabs[i].state == nil {
			continue
		}
		m.session.SetFileComments(m.tabs[i].path, "", m.tabs[i].state.Comments)
	}
	if err := m.session.Save(); err != nil {
		m.err = err
	}
}

// handleFinishModal processes keys for the finish-review confirmation.
// Confirming emits the finish event; q abandons the session without
// finishing (blocked clients see the connection close).
func (m *AppModel) handleFinishModal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, keys.Cancel) {
		m.modal = noModal
		return m, nil
	}

	switch msg.String() {
	case "y", "Y":
		return m.doFinish()
	case "n", "N":
		m.modal = noModal
		return m, nil
	case "q", "ctrl+c":
		return m, tea.Quit
	case "left", "right", "h", "l", "tab", "shift+tab":
		m.modalFocus = 1 - m.modalFocus
		return m, nil
	case "enter":
		if m.modalFocus == 0 {
			return m.doFinish()
		}
		m.modal = noModal
		return m, nil
	}
	return m, nil
}

func (m *AppModel) openFinishModal() {
	m.persist()
	m.modal = finishModal
	m.modalFocus = 0
}

func (m *AppModel) finishActionLabel() string {
	unresolved := m.unresolvedTotal()
	switch {
	case unresolved == 0:
		return "Approve"
	case !m.newFeedback:
		return "Resolve All & Approve"
	default:
		return "Finish Review"
	}
}

// unresolvedTotal counts unresolved comments across tabs and the session's
// review-level comments.
func (m *AppModel) unresolvedTotal() int {
	n := 0
	for i := range m.tabs {
		if m.tabs[i].state == nil {
			continue
		}
		for _, c := range m.tabs[i].state.Comments {
			if !c.Resolved {
				n++
			}
		}
	}
	if m.session != nil {
		for _, c := range m.session.CJ.ReviewComments {
			if !c.Resolved {
				n++
			}
		}
	}
	return n
}

func (m *AppModel) resolvesAllOnFinish() bool {
	return !m.newFeedback && m.unresolvedTotal() > 0
}

// doFinish persists the review, emits the finish event, and either quits
// (approved, or nothing is waiting on this review) or parks in the waiting
// state until the agent starts the next round.
func (m *AppModel) doFinish() (tea.Model, tea.Cmd) {
	if m.resolvesAllOnFinish() {
		m.resolveAll()
	} else {
		m.persist()
	}
	approved := m.unresolvedTotal() == 0
	if m.finishCh != nil {
		m.finishCh <- FinishEvent{Approved: approved}
	}
	m.modal = noModal
	if approved || !m.serving {
		return m, tea.Quit
	}
	m.waiting = true
	return m, nil
}

// startNextRound reloads the session from disk (picking up agent replies),
// carries comments forward onto the agent's edits with drift correction,
// advances the round counter, and refreshes documents and diffs.
func (m *AppModel) startNextRound() {
	// Capture the content the current comments were authored against
	// before reloading, then reload the session so agent-side replies and
	// resolutions written to review.json are picked up.
	prevContents := make(map[string]string, len(m.tabs))
	for i := range m.tabs {
		if m.tabs[i].doc != nil {
			prevContents[m.tabs[i].path] = m.tabs[i].doc.Content
		}
	}
	if m.session != nil {
		if fresh, err := review.OpenSessionAt(m.session.Key, m.session.Dir); err == nil {
			m.session.CJ = fresh.CJ
		}
	}
	if m.multiFile && m.baseRef != "" {
		if files, err := gitpkg.ChangedFilesFrom(m.baseRef); err == nil {
			m.syncCodeReviewTabs(files)
		}
	}

	now := review.Now()
	for i := range m.tabs {
		t := &m.tabs[i]
		comments := m.sessionComments(t.path)
		if t.isBinary {
			t.state = &fileReview{Comments: comments}
			continue
		}
		if t.isDeleted {
			t.doc = &document.Document{Path: t.path}
		} else {
			doc, _ := document.Load(t.path)
			t.doc = doc
		}
		t.chromaLines = nil
		t.deletedLineCache = nil
		if prev, ok := prevContents[t.path]; ok && t.doc != nil && !t.isDeleted {
			comments = review.CarryForwardFile(comments, prev, t.doc.Content, now)
		}
		t.state = &fileReview{Comments: comments}
		if m.multiFile && m.baseRef != "" {
			t.changedLines = nil
			t.inlineChanges = nil
			t.deletedAfter = nil
			t.changeChunks = nil
			if diff, err := gitpkg.DiffFile(t.path, m.baseRef); err == nil && diff != nil {
				t.changedLines = diff.ChangedLines
				t.inlineChanges = diff.InlineChanges
				t.deletedAfter = diff.DeletedAfter
				t.changeChunks = computeChangeChunks(diff)
			}
		}
		t.ensureHighlightCache()
	}

	// Advance the round only after carry-forward, mirroring crit's ordering.
	if m.session != nil {
		m.session.CJ.ReviewRound++
	}
	m.persist()

	m.waiting = false
	m.newFeedback = false
	m.rebuildContent()
	m.updateCommentSidebar()
}

func (m *AppModel) syncCodeReviewTabs(files []gitpkg.FileChange) {
	activePath := ""
	if m.activeTab >= 0 && m.activeTab < len(m.tabs) {
		activePath = m.tabs[m.activeTab].path
	}

	existing := make(map[string]FileTab, len(m.tabs))
	for _, t := range m.tabs {
		existing[t.path] = t
	}

	changed := make(map[string]bool, len(files))
	tabs := make([]FileTab, 0, len(files))
	for _, f := range files {
		changed[f.Path] = true
		t, ok := existing[f.Path]
		if !ok {
			t = newFileTab(f.Path, nil)
		}
		t.isBinary = f.Status == gitpkg.StatusBinary
		t.isDeleted = f.Status == gitpkg.StatusDeleted
		tabs = append(tabs, t)
	}

	for _, t := range m.tabs {
		hasComments := t.state != nil && len(t.state.Comments) > 0
		if !hasComments {
			hasComments = len(m.sessionComments(t.path)) > 0
		}
		if changed[t.path] || !hasComments {
			continue
		}
		tabs = append(tabs, t)
	}

	sort.Slice(tabs, func(i, j int) bool { return tabs[i].path < tabs[j].path })
	m.tabs = tabs
	if len(tabs) == 0 {
		m.activeTab = 0
		return
	}
	if activePath != "" {
		for i := range tabs {
			if tabs[i].path == activePath {
				m.activeTab = i
				return
			}
		}
	}
	if m.activeTab >= len(tabs) {
		m.activeTab = len(tabs) - 1
	}
}

func (m *AppModel) handleTextModal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	focusCount := 3
	if m.canSuggest() {
		focusCount++
	}
	if m.modal == editModal {
		focusCount += len(m.modalDeleteTargets())
	}

	switch msg.String() {
	case "esc":
		m.closeTextModal()
		return m, nil
	case "tab", "shift+tab":
		if msg.String() == "shift+tab" {
			m.modalFocus = (m.modalFocus + focusCount - 1) % focusCount
		} else {
			m.modalFocus = (m.modalFocus + 1) % focusCount
		}
		if m.modalFocus == 0 {
			m.modalTextarea.Focus()
		} else {
			m.modalTextarea.Blur()
		}
		return m, nil
	case "enter":
		if m.modalFocus == 1 {
			m.modalSubmit()
			return m, nil
		} else if m.modalFocus == 2 {
			m.closeTextModal()
			return m, nil
		} else if m.canSuggest() && m.modalFocus == 3 {
			m.insertSuggestion()
			return m, nil
		} else if m.modal == editModal && m.modalFocus >= m.modalDeleteStartFocus() {
			m.openDeleteConfirmation(m.modalFocus - m.modalDeleteStartFocus())
			return m, nil
		}
	case "ctrl+s":
		m.modalSubmit()
		return m, nil
	case "ctrl+o":
		if m.modalFocus == 0 {
			return m, m.openExternalEditor()
		}
		return m, nil
	case "ctrl+y":
		if m.canSuggest() {
			m.insertSuggestion()
		}
		return m, nil
	}

	if m.modalFocus == 0 {
		var cmd tea.Cmd
		m.modalTextarea, cmd = m.modalTextarea.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *AppModel) closeTextModal() {
	if m.modalTextarea.Value() != m.modalInitial {
		m.discardReturn = m.modal
		m.modal = discardChangesModal
		m.modalFocus = 1
		m.modalTextarea.Blur()
		return
	}
	m.discardTextModal()
}

func (m *AppModel) discardTextModal() {
	m.modal = noModal
	m.discardReturn = noModal
	m.deleteReturn = noModal
	m.editingID = ""
	m.editingReplyID = ""
	m.modalInitial = ""
	m.modalTextarea.Blur()
	m.modalTextarea.Reset()
}

func (m *AppModel) resumeTextModal() {
	m.modal = m.discardReturn
	m.discardReturn = noModal
	m.modalFocus = 0
	m.modalTextarea.Focus()
}

func (m *AppModel) handleDiscardChangesModal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.discardTextModal()
	case "n", "N", "esc":
		m.resumeTextModal()
	case "left", "right", "h", "l", "tab", "shift+tab":
		m.modalFocus = 1 - m.modalFocus
	case "enter":
		if m.modalFocus == 0 {
			m.discardTextModal()
		} else {
			m.resumeTextModal()
		}
	}
	return m, nil
}

func (m *AppModel) openDeleteConfirmation(targetIndex int) {
	if targetIndex < 0 || targetIndex >= len(m.modalDeleteTargets()) {
		return
	}
	m.deleteReturn = m.modal
	m.pendingDelete = targetIndex
	m.modal = deleteConfirmModal
	m.modalFocus = 1
	m.modalTextarea.Blur()
}

func (m *AppModel) cancelDeleteConfirmation() {
	if m.deleteReturn == noModal {
		m.modal = noModal
		m.editingID = ""
		m.editingReplyID = ""
		m.pendingDelete = 0
		m.modalFocus = 0
		return
	}
	m.modal = m.deleteReturn
	m.deleteReturn = noModal
	m.modalFocus = m.modalDeleteStartFocus() + m.pendingDelete
}

func (m *AppModel) confirmDelete() {
	target := m.pendingDelete
	m.deleteReturn = noModal
	m.modalDelete(target)
}

func (m *AppModel) handleDeleteConfirmModal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.confirmDelete()
	case "n", "N", "esc":
		m.cancelDeleteConfirmation()
	case "left", "right", "h", "l", "tab", "shift+tab":
		m.modalFocus = 1 - m.modalFocus
	case "enter":
		if m.modalFocus == 0 {
			m.confirmDelete()
		} else {
			m.cancelDeleteConfirmation()
		}
	}
	return m, nil
}

func (m *AppModel) handleTabSearch(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.tabSearching = false
		m.tabSearch = ""
		m.tabMatches = nil
		return m, nil
	case "enter":
		if len(m.tabMatches) > 0 {
			m.activeTab = m.tabMatches[0]
			m.rebuildContent()
			m.updateCommentSidebar()
		}
		m.tabSearching = false
		m.tabSearch = ""
		m.tabMatches = nil
		return m, nil
	case "backspace":
		if len(m.tabSearch) > 0 {
			m.tabSearch = m.tabSearch[:len(m.tabSearch)-1]
			m.updateTabSearchMatches()
		}
		return m, nil
	case "tab":
		// Cycle to next match
		if len(m.tabMatches) > 1 {
			// Rotate matches
			m.tabMatches = append(m.tabMatches[1:], m.tabMatches[0])
		}
		return m, nil
	default:
		s := msg.String()
		if len(s) == 1 && s[0] >= ' ' && s[0] <= '~' {
			m.tabSearch += s
			m.updateTabSearchMatches()
		}
		return m, nil
	}
}

func (m *AppModel) updateTabSearchMatches() {
	m.tabMatches = nil
	if m.tabSearch == "" {
		return
	}
	query := strings.ToLower(m.tabSearch)
	for i, t := range m.tabs {
		if strings.Contains(strings.ToLower(t.path), query) {
			m.tabMatches = append(m.tabMatches, i)
		}
	}
}

func (m *AppModel) recalculateLayout() {
	headerHeight := m.headerHeight()
	tabBarHeight := m.tabBarHeight()
	footerHeight := 1
	if len(m.tabs) > 0 && m.tab().state != nil {
		footerHeight = lipgloss.Height(m.renderFooter())
	}
	tmuxPadding := 0
	if os.Getenv("TMUX") != "" {
		tmuxPadding = 1
	}
	frameBorderHeight := 0
	frameBorderWidth := 0
	if m.multiFile {
		frameBorderHeight = 1 // bottom border
		frameBorderWidth = 2  // left + right borders
	}
	mainHeight := max(0, m.height-headerHeight-tabBarHeight-footerHeight-frameBorderHeight-tmuxPadding)

	commentWidth := m.width / 4
	if commentWidth < 20 {
		commentWidth = 20
	}
	contentWidth := m.width - commentWidth - frameBorderWidth

	m.contentViewport.SetWidth(contentWidth)
	m.contentViewport.SetHeight(mainHeight)
	m.commentViewport.SetWidth(commentWidth - 3)      // -3 for left border + padding + margin
	m.commentViewport.SetHeight(max(0, mainHeight-1)) // -1 for the "Comments (N)" header line

	modalWidth := m.width * 2 / 3
	if modalWidth < 50 {
		modalWidth = 50
	}
	if modalWidth > m.width-4 {
		modalWidth = m.width - 4
	}
	m.modalTextarea.SetWidth(modalWidth - 10)
	m.modalTextarea.SetHeight(6)
}

// annotationsAfterLine returns annotations that render after the given line
// (keyed by their endLine).
func (m *AppModel) annotationsAfterLine(lineNum int, side string) []annotation {
	t := m.tab()
	if t.state == nil {
		return nil
	}
	var anns []annotation
	for _, c := range t.state.Comments {
		if c.Scope == "file" {
			continue
		}
		if c.EndAt() == lineNum && c.Side == side {
			anns = append(anns, newAnnotation(c))
		}
	}
	return anns
}

type commentTarget struct {
	line    int
	side    string
	annoIdx int
}

func (m *AppModel) commentTargets(tabIndex int) []commentTarget {
	t := &m.tabs[tabIndex]
	if t.state == nil {
		return nil
	}

	indices := make(map[lineRef]int)
	targets := make([]commentTarget, 0, len(t.state.Comments))
	for _, c := range t.state.Comments {
		if c.Scope == "file" {
			continue
		}
		line := c.EndAt()
		ref := lineRef{side: c.Side, line: line}
		targets = append(targets, commentTarget{line: line, side: c.Side, annoIdx: indices[ref]})
		indices[ref]++
	}
	sort.SliceStable(targets, func(i, j int) bool {
		ri := lineRef{side: targets[i].side, line: targets[i].line}
		rj := lineRef{side: targets[j].side, line: targets[j].line}
		return m.visualLineIndex(t, ri) < m.visualLineIndex(t, rj)
	})
	return targets
}

// jumpToComment moves to the adjacent comment in tab, line, and annotation
// order, wrapping across the entire review and skipping tabs without comments.
func (m *AppModel) jumpToComment(step int) bool {
	t := m.tab()
	targets := m.commentTargets(m.activeTab)
	current := -1
	if t.cursorOnAnnotation {
		for i, target := range targets {
			if target.line == t.cursorLine && target.side == t.cursorSide && target.annoIdx == t.cursorAnnoIdx {
				current = i
				break
			}
		}
	}

	if current >= 0 {
		adjacent := current + step
		if adjacent >= 0 && adjacent < len(targets) {
			m.selectComment(m.activeTab, targets[adjacent])
			return true
		}
	} else if step > 0 {
		for _, target := range targets {
			if m.visualLineIndex(t, lineRef{side: target.side, line: target.line}) >= m.visualLineIndex(t, lineRef{side: t.cursorSide, line: t.cursorLine}) {
				m.selectComment(m.activeTab, target)
				return true
			}
		}
	} else {
		for i := len(targets) - 1; i >= 0; i-- {
			if m.visualLineIndex(t, lineRef{side: targets[i].side, line: targets[i].line}) <= m.visualLineIndex(t, lineRef{side: t.cursorSide, line: t.cursorLine}) {
				m.selectComment(m.activeTab, targets[i])
				return true
			}
		}
	}

	for offset := 1; offset <= len(m.tabs); offset++ {
		tabIndex := (m.activeTab + step*offset + len(m.tabs)) % len(m.tabs)
		targets = m.commentTargets(tabIndex)
		if len(targets) == 0 {
			continue
		}
		target := targets[0]
		if step < 0 {
			target = targets[len(targets)-1]
		}
		m.selectComment(tabIndex, target)
		return true
	}
	return false
}

func (m *AppModel) selectComment(tabIndex int, target commentTarget) {
	m.activeTab = tabIndex
	t := m.tab()
	t.cursorLine, t.cursorSide = target.line, target.side
	t.cursorOnAnnotation = true
	t.cursorAnnoIdx = target.annoIdx
	m.rebuildContent()
	m.updateCommentSidebar()
	m.scrollToCursor()
}

// jumpToChange moves to the adjacent change in tab and line order.  Unlike
// comment navigation, it stops at the beginning and end of the review.
func (m *AppModel) jumpToChange(step int) bool {
	t := m.tab()
	if step > 0 {
		for _, chunk := range t.changeChunks {
			if chunk.startLine > t.cursorLine {
				m.selectChange(m.activeTab, chunk)
				return true
			}
		}
	} else {
		for i := len(t.changeChunks) - 1; i >= 0; i-- {
			if t.changeChunks[i].startLine < t.cursorLine {
				m.selectChange(m.activeTab, t.changeChunks[i])
				return true
			}
		}
	}

	for tabIndex := m.activeTab + step; tabIndex >= 0 && tabIndex < len(m.tabs); tabIndex += step {
		chunks := m.tabs[tabIndex].changeChunks
		if len(chunks) == 0 {
			continue
		}
		chunk := chunks[0]
		if step < 0 {
			chunk = chunks[len(chunks)-1]
		}
		m.selectChange(tabIndex, chunk)
		return true
	}
	return false
}

func (m *AppModel) selectChange(tabIndex int, chunk changeChunk) {
	m.activeTab = tabIndex
	t := m.tab()
	t.cursorLine, t.cursorSide = chunk.startLine, ""
	t.cursorOnAnnotation = false
	t.cursorAnnoIdx = 0
	m.rebuildContent()
	m.updateCommentSidebar()
	m.scrollToChunk(chunk)
}

// sidebarItem represents a comment in the sidebar list.
type sidebarItem struct {
	id      string
	scope   string
	line    int
	endLine int
	side    string
	body    string
	author  string
	replies []review.Reply
}

// annotation represents an inline comment to render.
type annotation struct {
	id       string
	body     string
	line     int
	endLine  int
	side     string
	author   string
	resolved bool
	replies  []review.Reply
}

func newAnnotation(c review.Comment) annotation {
	return annotation{
		id: c.ID, body: c.Body,
		line: c.StartLine, endLine: c.EndLine, side: c.Side,
		author: c.Author, resolved: c.Resolved, replies: c.Replies,
	}
}

func expandDisplayTabs(s string) string {
	return strings.ReplaceAll(s, "\t", strings.Repeat(" ", displayTabWidth))
}

func documentDisplayLine(t *FileTab, index int, fallback string) string {
	if !t.isMarkdown && t.chromaLines != nil && index < len(t.chromaLines) {
		return expandDisplayTabs(t.chromaLines[index])
	}
	return expandDisplayTabs(fallback)
}

// rebuildContent renders the document line-by-line with cursor, selection,
// line numbers, and bordered inline annotations.
func (m *AppModel) rebuildContent() {
	t := m.tab()
	m.contentLayout = newRenderedContentLayout()

	// Handle placeholder tabs
	if t.isBinary {
		m.contentViewport.SetContent("\n  Binary file changed — cannot display content.\n")
		return
	}
	if t.doc == nil {
		return
	}

	// Collect annotations keyed by the line they appear AFTER
	annosByEndLine := make(map[int][]annotation)
	oldAnnosByEndLine := make(map[int][]annotation)
	if t.state != nil {
		for _, c := range t.state.Comments {
			if c.Scope == "file" {
				continue
			}
			endAt := c.EndAt()
			if c.Side == "old" {
				oldAnnosByEndLine[endAt] = append(oldAnnosByEndLine[endAt], newAnnotation(c))
			} else {
				annosByEndLine[endAt] = append(annosByEndLine[endAt], newAnnotation(c))
			}
		}
	}

	// Count how many comments cover each line (for overlap detection)
	annotatedLines := make(map[int]int)
	oldAnnotatedLines := make(map[int]int)
	if t.state != nil {
		for _, c := range t.state.Comments {
			if c.Scope == "file" {
				continue
			}
			lines := annotatedLines
			if c.Side == "old" {
				lines = oldAnnotatedLines
			}
			for l := c.StartLine; l <= c.EndAt(); l++ {
				lines[l]++
			}
		}
	}

	selStart, selEnd := m.selectionRange()
	selSide := m.selectionSide()

	// Determine which lines to highlight from the selected annotation.
	sidebarHighlightStart, sidebarHighlightEnd, sidebarHighlightSide := m.highlightedCommentLines()

	contentWidth := m.contentViewport.Width()
	boxWidth := contentWidth - gutterWidth
	if boxWidth < 20 {
		boxWidth = 20
	}

	textWidth := contentWidth - gutterWidth - 1
	if textWidth < 10 {
		textWidth = 10
	}

	// Use cached syntax highlighting
	isMarkdown := t.isMarkdown
	// Detect table blocks so we can align columns across rows
	tableBlocks := detectTableBlocks(t.doc.Lines)
	tableBlockMap := make(map[int]*tableBlock)
	for i := range tableBlocks {
		tb := &tableBlocks[i]
		for l := tb.startLine; l <= tb.endLine; l++ {
			tableBlockMap[l] = tb
		}
	}

	var b strings.Builder
	b.Grow(len(t.doc.Lines) * 200) // pre-allocate to reduce allocations
	layout := newRenderedContentLayout()
	renderDeleted := func(afterLine int) {
		dels := t.deletedAfter[afterLine]
		cachedHL := t.deletedLineCache[afterLine]
		for di, del := range dels {
			target := contentMouseTarget{line: del.OldLineNum, side: "old"}
			isCursor := t.cursorSide == "old" && del.OldLineNum == t.cursorLine
			isSelected := t.selecting && selSide == "old" && del.OldLineNum >= selStart && del.OldLineNum <= selEnd
			isSidebarHighlight := sidebarHighlightSide == "old" && del.OldLineNum >= sidebarHighlightStart && del.OldLineNum <= sidebarHighlightEnd

			marker := diffDeletedGutter.Render("-")
			switch {
			case del.OldLineNum == m.hoveredGutterLine && m.hoveredGutterSide == "old":
				marker = commentGutterMarker.Render(">")
			case isCursor && !t.cursorOnAnnotation:
				marker = cursorMarker.Render(">")
			case isSelected:
				marker = selectedMarker.Render("|")
			case isSidebarHighlight:
				marker = cursorMarker.Render(">")
			case oldAnnotatedLines[del.OldLineNum] > 1:
				marker = gutterOverlap.Render("◆")
			case oldAnnotatedLines[del.OldLineNum] == 1:
				marker = annotationGutter.Render("■")
			}

			numStyle := diffDeletedLineNum
			if isCursor {
				numStyle = cursorLineNumStyle
			} else if isSelected {
				numStyle = selectedLineNumStyle
			}
			num := numStyle.Render(fmt.Sprintf("%d", del.OldLineNum))
			var cached string
			if di < len(cachedHL) {
				cached = cachedHL[di]
			}
			for wi, content := range deletedDisplayLines(del.Content, cached, del.Inline, isMarkdown, textWidth) {
				if isSelected {
					content = inlineBackground(selectedLineBg, ansi.Strip(content))
				} else if isSidebarHighlight {
					content = inlineBackground(sidebarHighlightBg, ansi.Strip(content))
				}
				if wi == 0 {
					layout.appendBlock(&b, fmt.Sprintf("%s%s %s", marker, num, content), target)
				} else {
					layout.appendBlock(&b, fmt.Sprintf(" %s %s", continuationGutter, content), target)
				}
			}
			for idx, ann := range oldAnnosByEndLine[del.OldLineNum] {
				focused := m.focused == contentPane && t.cursorOnAnnotation && isCursor && t.cursorAnnoIdx == idx
				layout.appendBlock(&b, m.renderAnnotationBox(ann, boxWidth, focused), contentMouseTarget{
					line: del.OldLineNum, side: "old", annotation: true, annotationIndex: idx,
				})
			}
		}
	}
	for i, line := range t.doc.Lines {
		lineNum := i + 1
		lineTarget := contentMouseTarget{line: lineNum}

		// Render deleted lines that appear before this line
		renderDeleted(lineNum - 1)

		isCursor := t.cursorSide == "" && lineNum == t.cursorLine
		isSelected := t.selecting && selSide == "" && lineNum >= selStart && lineNum <= selEnd
		isSidebarHighlight := sidebarHighlightSide == "" && sidebarHighlightStart > 0 && lineNum >= sidebarHighlightStart && lineNum <= sidebarHighlightEnd
		isChanged := t.changedLines != nil && t.changedLines[lineNum]
		inlineChanges := t.inlineChanges[lineNum]

		// Marker column
		var marker string
		if lineNum == m.hoveredGutterLine && m.hoveredGutterSide == "" {
			marker = commentGutterMarker.Render(">")
		} else if isCursor && !t.cursorOnAnnotation {
			marker = cursorMarker.Render(">")
		} else if isSelected {
			marker = selectedMarker.Render("|")
		} else if isSidebarHighlight {
			marker = cursorMarker.Render(">")
		} else if count, ok := annotatedLines[lineNum]; ok && count > 0 {
			if count > 1 {
				marker = gutterOverlap.Render("◆")
			} else {
				marker = annotationGutter.Render("■")
			}
		} else if isChanged {
			marker = diffAddedGutter.Render("+")
		} else {
			marker = " "
		}

		// Line number
		var numStr string
		if isCursor {
			numStr = cursorLineNumStyle.Render(fmt.Sprintf("%d", lineNum))
		} else if isSelected {
			numStr = selectedLineNumStyle.Render(fmt.Sprintf("%d", lineNum))
		} else {
			numStr = lineNumStyle.Render(fmt.Sprintf("%d", lineNum))
		}

		// Check if this line is part of a table block
		if len(inlineChanges) > 0 && !isSelected && !isSidebarHighlight {
			for wi, styledLine := range inlineDiffDisplayLines(inlineChanges, isMarkdown, textWidth, diffCommonTextBg, diffAddedTextBg) {
				if wi == 0 {
					layout.appendBlock(&b, fmt.Sprintf("%s%s %s", marker, numStr, styledLine), lineTarget)
				} else {
					layout.appendBlock(&b, fmt.Sprintf(" %s %s", continuationGutter, styledLine), lineTarget)
				}
			}
		} else if tb, inTable := tableBlockMap[lineNum]; inTable {
			var styledLine string
			if reTableSep.MatchString(line) {
				styledLine = formatTableSep(tb.colWidths)
			} else {
				isHeader := lineNum == tb.startLine
				styledLine = formatTableRow(line, tb.colWidths, isHeader)
			}

			if isSelected {
				styledLine = inlineBackground(selectedLineBg, styledLine)
			} else if isSidebarHighlight {
				styledLine = inlineBackground(sidebarHighlightBg, styledLine)
			} else if isChanged {
				styledLine = inlineBackground(diffChangedLineBg, styledLine)
			}

			wrappedLines := strings.Split(lipgloss.Wrap(expandDisplayTabs(styledLine), textWidth, ""), "\n")
			for wi, wrappedLine := range wrappedLines {
				if wi == 0 {
					layout.appendBlock(&b, fmt.Sprintf("%s%s %s", marker, numStr, wrappedLine), lineTarget)
				} else {
					layout.appendBlock(&b, fmt.Sprintf(" %s %s", continuationGutter, wrappedLine), lineTarget)
				}
			}
		} else {
			// Get the display content: Chroma-highlighted or raw
			displayLine := documentDisplayLine(t, i, line)

			styleFunc := func(s string) string { return s }
			if isMarkdown {
				styleFunc = func(s string) string { return highlightMarkdown(s) }
			}
			if isSelected {
				styleFunc = func(s string) string { return inlineBackground(selectedLineBg, s) }
			} else if isSidebarHighlight {
				styleFunc = func(s string) string { return inlineBackground(sidebarHighlightBg, s) }
			} else if isChanged {
				base := styleFunc
				styleFunc = func(s string) string { return inlineBackground(diffChangedLineBg, base(s)) }
			}

			wrapped := lipgloss.Wrap(displayLine, textWidth, "")
			wrappedLines := strings.Split(wrapped, "\n")
			for wi, wl := range wrappedLines {
				if wi == 0 {
					layout.appendBlock(&b, fmt.Sprintf("%s%s %s", marker, numStr, styleFunc(wl)), lineTarget)
				} else {
					layout.appendBlock(&b, fmt.Sprintf(" %s %s", continuationGutter, styleFunc(wl)), lineTarget)
				}
			}
		}

		// Render inline annotations after this line
		if anns, ok := annosByEndLine[lineNum]; ok {
			for idx, ann := range anns {
				focused := m.focused == contentPane && t.cursorOnAnnotation && t.cursorSide == "" && t.cursorLine == lineNum && t.cursorAnnoIdx == idx
				layout.appendBlock(&b, m.renderAnnotationBox(ann, boxWidth, focused), contentMouseTarget{
					line: lineNum, annotation: true, annotationIndex: idx,
				})
			}
		}
	}
	renderDeleted(t.doc.LineCount())

	m.contentLayout = layout
	m.contentViewport.SetContent(b.String())
}

// renderAnnotationBox renders a bordered annotation box indented under the gutter.
func (m *AppModel) renderAnnotationBox(ann annotation, maxWidth int, focused bool) string {
	var lineLabel string
	if ann.endLine > ann.line {
		lineLabel = fmt.Sprintf("L%d-%d", ann.line, ann.endLine)
	} else {
		lineLabel = fmt.Sprintf("L%d", ann.line)
	}
	if ann.side == "old" {
		lineLabel += " (deleted)"
	}

	var boxContent strings.Builder
	label := inlineLabelComment.Render("comment")
	lineRef := commentLineStyle.Render(lineLabel)
	header := fmt.Sprintf("%s %s", label, lineRef)
	if ann.author != "" {
		header += " " + commentLineStyle.Render("— "+ann.author)
	}
	if ann.resolved {
		header += " " + resolvedBadge.Render("✓ resolved")
	}
	boxContent.WriteString(header)
	if !ann.resolved {
		boxContent.WriteString("\n")
		boxContent.WriteString(clampLines(ann.body, 3))
		for _, r := range ann.replies {
			reply := r.Body
			if i := strings.IndexByte(reply, '\n'); i >= 0 {
				reply = reply[:i] + "…"
			}
			who := r.Author
			if who == "" {
				who = "reply"
			}
			boxContent.WriteString("\n" + replyStyle.Render(fmt.Sprintf("↳ %s: %s", who, reply)))
		}
	}
	boxStyle := inlineCommentBox

	if focused {
		boxStyle = boxStyle.BorderForeground(warning)
	}
	box := boxStyle.Width(maxWidth).Render(boxContent.String())

	var prefix string
	if focused {
		cursor := lipgloss.NewStyle().Width(2).Render(cursorMarker.Render(">"))
		prefix = cursor + strings.Repeat(" ", gutterWidth-2)
	} else {
		prefix = strings.Repeat(" ", gutterWidth)
	}

	var b strings.Builder
	for _, line := range strings.Split(box, "\n") {
		b.WriteString(prefix + line + "\n")
	}
	return b.String()
}

var (
	reBold       = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reItalic     = regexp.MustCompile(`(?:^|[^*])\*([^*]+?)\*(?:[^*]|$)`)
	reCode       = regexp.MustCompile("`([^`]+)`")
	reLink       = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	reListItem   = regexp.MustCompile(`^(\s*[-*+]\s)(.*)$`)
	reCheckbox   = regexp.MustCompile(`^(\s*[-*+]\s)\[([ xX])\]\s(.*)$`)
	reNumList    = regexp.MustCompile(`^(\s*\d+\.\s)(.*)$`)
	reBlockquote = regexp.MustCompile(`^(\s*>\s?)(.*)$`)
	reHr         = regexp.MustCompile(`^(\s*)([-*_]{3,})\s*$`)
	reTableRow   = regexp.MustCompile(`^\s*\|.*\|\s*$`)
	reTableSep   = regexp.MustCompile(`^\s*\|[\s:]*[-]+[\s:|-]*\|\s*$`)
)

// highlightMarkdown applies markdown syntax highlighting to a single line.
func highlightMarkdown(line string) string {
	trimmed := strings.TrimSpace(line)

	if reHr.MatchString(line) {
		return mdHrStyle.Render("─────────────────────────────────")
	}

	if strings.HasPrefix(trimmed, "#### ") {
		return mdH4Style.Render(line)
	}
	if strings.HasPrefix(trimmed, "### ") {
		return mdH3Style.Render(line)
	}
	if strings.HasPrefix(trimmed, "## ") {
		return mdH2Style.Render(line)
	}
	if strings.HasPrefix(trimmed, "# ") {
		return mdH1Style.Render(line)
	}

	if reTableSep.MatchString(line) {
		return mdTableSepStyle.Render(line)
	}
	if reTableRow.MatchString(line) {
		cells := strings.Split(line, "|")
		var parts []string
		for i, cell := range cells {
			if i == 0 || i == len(cells)-1 {
				parts = append(parts, cell)
			} else {
				parts = append(parts, highlightInline(cell))
			}
		}
		return strings.Join(parts, mdTablePipe.Render("|"))
	}

	if loc := reBlockquote.FindStringSubmatchIndex(line); loc != nil {
		rest := line[loc[4]:loc[5]]
		return mdBlockquoteBar.Render("▎") + " " + mdBlockquoteStyle.Render(rest)
	}

	if loc := reCheckbox.FindStringSubmatchIndex(line); loc != nil {
		indent := line[loc[2]:loc[3]]
		checked := line[loc[4]:loc[5]]
		rest := line[loc[6]:loc[7]]
		if checked == "x" || checked == "X" {
			return indent + mdCheckboxDone.Render("✓") + " " + mdCheckboxDoneText.Render(rest)
		}
		return indent + mdCheckboxOpen.Render("☐") + " " + highlightInline(rest)
	}

	if loc := reListItem.FindStringSubmatchIndex(line); loc != nil {
		indent := line[loc[2]:loc[3]]
		rest := line[loc[4]:loc[5]]
		return mdListMarkerStyle.Render(indent) + highlightInline(rest)
	}
	if loc := reNumList.FindStringSubmatchIndex(line); loc != nil {
		marker := line[loc[2]:loc[3]]
		rest := line[loc[4]:loc[5]]
		return mdListMarkerStyle.Render(marker) + highlightInline(rest)
	}

	return highlightInline(line)
}

func inlineBackground(style lipgloss.Style, content string) string {
	bgAnsi := bgToAnsi(style.GetBackground())
	if bgAnsi == "" {
		return style.Render(content)
	}
	var patched strings.Builder
	patched.Grow(len(content) + len(bgAnsi))
	reapply := false
	parser := ansi.GetParser()
	defer ansi.PutParser(parser)
	parser.SetHandler(ansi.Handler{HandleCsi: func(cmd ansi.Cmd, params ansi.Params) {
		if cmd != 'm' {
			return
		}
		reapply = len(params) == 0
		params.ForEach(0, func(_ int, param int, _ bool) {
			switch {
			case param == 0 || param == 49:
				reapply = true
			case param == 48, param >= 40 && param <= 47, param >= 100 && param <= 107:
				reapply = false
			}
		})
	}})
	for i := range len(content) {
		parser.Advance(content[i])
		patched.WriteByte(content[i])
		if reapply {
			patched.WriteString(bgAnsi)
			reapply = false
		}
	}
	return bgAnsi + patched.String() + "\033[0m"
}

func inlineDiffDisplayLines(segments []gitpkg.InlineSegment, isMarkdown bool, width int, base, changed lipgloss.Style) []string {
	var b strings.Builder
	for _, segment := range segments {
		content := segment.Content
		if isMarkdown {
			content = highlightMarkdown(content)
		}
		style := base
		if segment.Changed {
			style = changed
		}
		b.WriteString(inlineBackground(style, content))
	}
	return strings.Split(lipgloss.Wrap(expandDisplayTabs(b.String()), width, ""), "\n")
}

func deletedDisplayLines(content, cached string, inline []gitpkg.InlineSegment, isMarkdown bool, width int) []string {
	if len(inline) > 0 {
		return inlineDiffDisplayLines(inline, isMarkdown, width, diffCommonTextBg, diffDeletedTextBg)
	}
	display := content
	if cached != "" {
		display = cached
	}
	lines := strings.Split(lipgloss.Wrap(expandDisplayTabs(display), width, ""), "\n")
	for i := range lines {
		if isMarkdown {
			lines[i] = highlightMarkdown(lines[i])
		}
		lines[i] = inlineBackground(diffDeletedLineBg, lines[i])
	}
	return lines
}

// tableBlock represents a contiguous range of markdown table lines.
type tableBlock struct {
	startLine int
	endLine   int
	colWidths []int
}

func detectTableBlocks(lines []string) []tableBlock {
	var blocks []tableBlock
	inTable := false
	var current tableBlock

	for i, line := range lines {
		isTable := reTableRow.MatchString(line) || reTableSep.MatchString(line)
		if isTable {
			if !inTable {
				inTable = true
				current = tableBlock{startLine: i + 1}
			}
			current.endLine = i + 1

			if !reTableSep.MatchString(line) {
				cells := parseTableCells(line)
				for len(current.colWidths) < len(cells) {
					current.colWidths = append(current.colWidths, 0)
				}
				for ci, cell := range cells {
					if len(cell) > current.colWidths[ci] {
						current.colWidths[ci] = len(cell)
					}
				}
			}
		} else {
			if inTable {
				blocks = append(blocks, current)
				inTable = false
			}
		}
	}
	if inTable {
		blocks = append(blocks, current)
	}
	return blocks
}

func parseTableCells(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
	}
	return cells
}

func formatTableRow(line string, colWidths []int, isHeader bool) string {
	cells := parseTableCells(line)
	pipe := mdTablePipe.Render("│")

	var parts []string
	for ci := 0; ci < len(colWidths); ci++ {
		w := colWidths[ci]
		cell := ""
		if ci < len(cells) {
			cell = cells[ci]
		}
		padded := lipgloss.NewStyle().Width(w).Render(cell)
		if isHeader {
			parts = append(parts, mdTableHeaderStyle.Render(" "+padded+" "))
		} else {
			parts = append(parts, mdTableCellStyle.Render(" "+padded+" "))
		}
	}

	return pipe + strings.Join(parts, pipe) + pipe
}

func formatTableSep(colWidths []int) string {
	pipe := mdTablePipe.Render("│")
	var parts []string
	for _, w := range colWidths {
		parts = append(parts, mdTableSepStyle.Render(strings.Repeat("─", w+2)))
	}
	return pipe + strings.Join(parts, mdTablePipe.Render("┼")) + pipe
}

func highlightInline(line string) string {
	// Render links before injecting any ANSI styles. CSI sequences also start
	// with "[", so running the Markdown link regexp afterward can mistake an
	// escape sequence followed by a link for part of the link label.
	line = reLink.ReplaceAllStringFunc(line, func(match string) string {
		idx := strings.Index(match, "](")
		if idx < 0 {
			return match
		}
		text := match[1:idx]
		return mdLinkStyle.Render(text)
	})

	line = reCode.ReplaceAllStringFunc(line, func(match string) string {
		inner := match[1 : len(match)-1]
		return mdCodeStyle.Render(" " + inner + " ")
	})

	line = reBold.ReplaceAllStringFunc(line, func(match string) string {
		inner := match[2 : len(match)-2]
		return mdBoldStyle.Render(inner)
	})

	line = reItalic.ReplaceAllStringFunc(line, func(match string) string {
		start := 0
		end := len(match)
		if match[0] != '*' {
			start = 1
		}
		if match[end-1] != '*' {
			end--
		}
		inner := match[start+1 : end-1]
		prefix := match[:start]
		suffix := match[end:]
		return prefix + mdItalicStyle.Render(inner) + suffix
	})

	return line
}

func (m *AppModel) scrollToCursor() {
	t := m.tab()
	if t.doc == nil {
		return
	}
	r, ok := m.contentRenderedRange(t.cursorSide, t.cursorLine, t.cursorLine)
	if !ok {
		return
	}

	vpHeight := m.contentViewport.Height()
	currentTop := m.contentViewport.YOffset()

	if r.start < currentTop {
		m.contentViewport.SetYOffset(r.start)
	}
	if r.end > currentTop+vpHeight {
		m.contentViewport.SetYOffset(r.end - vpHeight)
	}
}

const chunkScrollPadding = 4

// scrollToChunk scrolls the viewport to show the entire change chunk
// plus padding lines above and below for context.
func (m *AppModel) scrollToChunk(chunk changeChunk) {
	t := m.tab()
	if t.doc == nil {
		return
	}

	startLine := chunk.startLine - chunkScrollPadding
	if startLine < 1 {
		startLine = 1
	}
	endLine := chunk.endLine + chunkScrollPadding
	if endLine > t.doc.LineCount() {
		endLine = t.doc.LineCount()
	}
	r, ok := m.contentRenderedRange("", startLine, endLine)
	if !ok {
		return
	}

	m.contentViewport.SetYOffset(r.start)
}

func (m *AppModel) scrollToAnnotation(side string, startLine, endLine int) {
	t := m.tab()
	if t.doc == nil {
		return
	}
	if endLine == 0 {
		endLine = startLine
	}

	r, ok := m.contentRenderedRange(side, startLine, endLine)
	if !ok {
		return
	}

	vpHeight := m.contentViewport.Height()

	offset := r.end - vpHeight
	if offset < 0 {
		offset = 0
	}
	if offset > r.start {
		offset = r.start
	}

	m.contentViewport.SetYOffset(offset)
}

func (m *AppModel) contentRenderedRange(side string, startLine, endLine int) (renderedRange, bool) {
	ranges := m.contentLayout.lineRanges
	if side == "old" {
		ranges = m.contentLayout.oldRanges
	}
	start, startOK := ranges[startLine]
	end, endOK := ranges[endLine]
	if !startOK || !endOK {
		return renderedRange{}, false
	}
	return renderedRange{start: start.start, end: end.end}, true
}

func (m *AppModel) updateCommentSidebar() {
	t := m.tab()
	if t.state == nil {
		return
	}
	m.sidebarTargets = nil

	t.sidebarItems = nil
	for _, c := range t.state.Comments {
		if c.Resolved {
			continue
		}
		t.sidebarItems = append(t.sidebarItems, sidebarItem{
			id: c.ID, scope: c.Scope, line: c.StartLine, endLine: c.EndLine,
			side: c.Side, body: c.Body, author: c.Author, replies: c.Replies,
		})
	}
	sort.SliceStable(t.sidebarItems, func(i, j int) bool {
		if t.sidebarItems[i].scope == "file" {
			return t.sidebarItems[j].scope != "file"
		}
		if t.sidebarItems[j].scope == "file" {
			return false
		}
		return t.sidebarItems[i].line < t.sidebarItems[j].line
	})

	if t.sidebarCursor >= len(t.sidebarItems) {
		t.sidebarCursor = len(t.sidebarItems) - 1
	}
	if t.sidebarCursor < 0 {
		t.sidebarCursor = 0
	}

	var b strings.Builder

	if len(t.sidebarItems) == 0 {
		message := "No comments yet.\n\nPress enter for a line comment,\nor 'f' for a file comment."
		if len(t.state.Comments) > 0 {
			message = "All comments resolved."
		}
		b.WriteString(commentStyle.Render(message))
		m.commentViewport.SetContent(b.String())
		return
	}

	for idx, it := range t.sidebarItems {
		isSelected := m.focused == commentPane && idx == t.sidebarCursor
		var item strings.Builder

		var lineInfo string
		if it.scope == "file" {
			lineInfo = "File"
		} else if it.endLine > it.line {
			lineInfo = fmt.Sprintf("L%d-%d", it.line, it.endLine)
		} else {
			lineInfo = fmt.Sprintf("L%d", it.line)
		}
		if it.side == "old" {
			lineInfo += " (deleted)"
		}
		lineInfo = commentLineStyle.Render(lineInfo)
		if it.author != "" {
			lineInfo += " " + commentLineStyle.Render(it.author)
		}
		cursorCol := lipgloss.NewStyle().Width(2)
		prefix := cursorCol.Render("")
		if isSelected {
			prefix = cursorCol.Render(cursorMarker.Render(">"))
		}

		fmt.Fprintf(&item, "%s%s\n", prefix, lineInfo)

		clamped := clampLines(it.body, 3)
		bodyLines := strings.Split(clamped, "\n")
		for i, bl := range bodyLines {
			styled := bl
			if isSelected {
				styled = sidebarSelectedText.Render(bl)
			} else {
				styled = commentStyle.Render(bl)
			}
			item.WriteString(" " + styled)
			if i < len(bodyLines)-1 {
				item.WriteString("\n")
			}
		}
		for _, r := range it.replies {
			reply := r.Body
			if i := strings.IndexByte(reply, '\n'); i >= 0 {
				reply = reply[:i] + "…"
			}
			who := r.Author
			if who == "" {
				who = "reply"
			}
			item.WriteString("\n " + replyStyle.Render(fmt.Sprintf("↳ %s: %s", who, reply)))
		}

		wrapped := lipgloss.Wrap(expandDisplayTabs(item.String()), max(m.commentViewport.Width(), 1), "")
		for _, row := range strings.Split(wrapped, "\n") {
			b.WriteString(row)
			b.WriteByte('\n')
			m.sidebarTargets = append(m.sidebarTargets, idx)
		}
		b.WriteByte('\n')
		m.sidebarTargets = append(m.sidebarTargets, idx)
	}

	m.commentViewport.SetContent(b.String())
}

func unresolvedCommentCount(comments []review.Comment) int {
	count := 0
	for _, c := range comments {
		if !c.Resolved {
			count++
		}
	}
	return count
}

func truncateLeftToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	stringWidth := ansi.StringWidth(s)
	if stringWidth <= width {
		return s
	}
	const prefix = "…"
	prefixWidth := ansi.StringWidth(prefix)
	if width <= prefixWidth {
		return ansi.Truncate(prefix, width, "")
	}
	return prefix + ansi.Cut(s, stringWidth-width+prefixWidth, stringWidth)
}

func (m AppModel) renderHeader() string {
	t := m.tab()
	commentCount := 0
	if t.state != nil {
		commentCount = unresolvedCommentCount(t.state.Comments)
	}
	displayPath := t.path
	if m.filePath != "" {
		displayPath = m.filePath
	}

	prefix := " TCrit: "
	var suffix string
	if t.selecting {
		start, end := m.selectionRange()
		selLabel := visualModeIndicator.Render("VISUAL")
		deleted := ""
		if m.selectionSide() == "old" {
			deleted = " (deleted)"
		}
		suffix = fmt.Sprintf("  %s L%d-%d%s", selLabel, start, end, deleted)
	} else if t.doc != nil {
		if t.cursorSide == "old" {
			suffix = fmt.Sprintf("  %d comments  L%d (deleted)", commentCount, t.cursorLine)
		} else {
			suffix = fmt.Sprintf("  %d comments  L%d/%d", commentCount, t.cursorLine, t.doc.LineCount())
		}
	} else {
		suffix = fmt.Sprintf("  %d comments", commentCount)
	}
	headerWidth := max(0, m.width-headerStyle.GetHorizontalFrameSize())
	if m.width > 0 {
		pathWidth := max(0, headerWidth-ansi.StringWidth(prefix)-ansi.StringWidth(suffix))
		displayPath = truncateLeftToWidth(displayPath, pathWidth)
	}
	headerContent := prefix + displayPath + suffix
	if m.width > 0 {
		headerContent = ansi.Truncate(headerContent, headerWidth, "")
	}
	if !m.detached {
		return headerStyle.Width(m.width).Render(headerContent)
	}
	pausedBanner := pausedStatusBar.Width(m.width).Render(
		" AI agent is paused — review the document, then press q to submit")
	return pausedBanner + "\n" + headerStyle.Width(m.width).Render(headerContent)
}

func (m AppModel) View() tea.View {
	if m.err != nil {
		v := tea.NewView(fmt.Sprintf("Error: %v\n\nPress q to quit.", m.err))
		v.AltScreen = true
		return v
	}

	if m.waiting {
		round := 0
		if m.session != nil {
			round = m.session.CJ.ReviewRound
		}
		msg := fmt.Sprintf(
			"\n  Round %d finished — %d unresolved comment(s) sent to the agent.\n\n"+
				"  Waiting for the agent to address them and start the next round…\n\n"+
				"  q: quit without waiting",
			round, m.unresolvedTotal())
		v := tea.NewView(msg)
		v.AltScreen = true
		return v
	}

	if m.width == 0 || len(m.tabs) == 0 || m.tab().state == nil {
		v := tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}
	full, _ := m.renderReviewScreen()

	v := tea.NewView(full)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	return v
}

func (m AppModel) renderReviewScreen() (string, renderedScreenLayout) {
	t := m.tab()

	commentCount := unresolvedCommentCount(t.state.Comments)
	header := m.renderHeader()

	// Tab bar (multi-file mode)
	var tabBar string
	if m.multiFile {
		tabBar = m.renderTabBar()
	}

	// Content pane
	commentWidth := m.width / 4
	if commentWidth < 20 {
		commentWidth = 20
	}
	contentWidth := m.width - commentWidth

	panelHeight := m.contentViewport.Height()

	contentBox := lipgloss.NewStyle().
		Width(contentWidth).
		Height(panelHeight).
		Render(m.contentViewport.View())

	// Comment sidebar (left border to separate from content)
	sidebarBorderColor := subtle
	if m.focused == commentPane {
		sidebarBorderColor = accent
	}
	sidebarBorder := lipgloss.Border{Left: "│"}
	commentHeader := lipgloss.NewStyle().Bold(true).Foreground(accent).Render(fmt.Sprintf("Comments (%d)", commentCount))
	commentBox := lipgloss.NewStyle().
		Border(sidebarBorder, false, false, false, true).
		BorderForeground(sidebarBorderColor).
		Width(commentWidth - 2).
		Height(panelHeight).
		PaddingLeft(1).
		Render(commentHeader + "\n" + m.commentViewport.View())

	mainRow := lipgloss.JoinHorizontal(lipgloss.Top, contentBox, commentBox)

	// Wrap content in a frame: │ left/right borders, ╰───╯ bottom.
	// The tab bar serves as the top border.
	if m.multiFile {
		borderColor := lipgloss.NewStyle().Foreground(accent)
		lines := strings.Split(mainRow, "\n")
		var framed strings.Builder
		left := borderColor.Render("│")
		right := borderColor.Render("│")
		for _, line := range lines {
			framed.WriteString(left + line + right + "\n")
		}
		bottom := borderColor.Render("╰" + strings.Repeat("─", m.width-2) + "╯")
		framed.WriteString(bottom)
		mainRow = framed.String()
	}

	footer := m.renderFooter()

	var sections []string
	sections = append(sections, header)
	if tabBar != "" {
		sections = append(sections, tabBar)
	}
	sections = append(sections, mainRow)
	footerTop := lipgloss.Height(lipgloss.JoinVertical(lipgloss.Left, sections...))
	sections = append(sections, footer)
	layout := renderedScreenLayout{}
	if !t.selecting {
		button := m.renderModalButton(m.finishActionLabel(), "q", true)
		layout.footerFinish = mouseRect{right: lipgloss.Width(button), top: footerTop, bottom: footerTop + 1}
		layout.hasFooterFinish = true
	}

	full := lipgloss.JoinVertical(lipgloss.Left, sections...)

	if m.modal != noModal {
		underlyingModal := noModal
		switch m.modal {
		case discardChangesModal:
			underlyingModal = m.discardReturn
		case deleteConfirmModal:
			underlyingModal = m.deleteReturn
		}
		if underlyingModal != noModal {
			underlying := m
			underlying.modal = underlyingModal
			underlying.modalFocus = 0
			full = underlying.renderWithModal(full)
		}
		full, layout.modalRegions = m.renderWithModalLayout(full)
	}

	return full, layout
}

type tabLabel struct {
	text     string
	rendered string
	width    int
}

func (m *AppModel) tabLabels() []tabLabel {
	basenames := make(map[string]int)
	for _, t := range m.tabs {
		basenames[filepath.Base(t.path)]++
	}

	labels := make([]tabLabel, len(m.tabs))
	for i, t := range m.tabs {
		label := filepath.Base(t.path)
		if basenames[label] > 1 {
			label = t.path
		}
		if n := len(t.changedLines); n > 0 {
			label += " " + tabChangeCount.Render(fmt.Sprintf("(+%d)", n))
		}
		labels[i] = tabLabel{text: label}
	}
	return labels
}

func (m *AppModel) renderTab(labels []tabLabel, i int, isFirst bool) string {
	style := inactiveTabStyle
	if i == m.activeTab {
		style = activeTabStyle
	}
	border, _, _, _, _ := style.GetBorder()
	if isFirst && i == m.activeTab {
		border.BottomLeft = "│"
	} else if isFirst {
		border.BottomLeft = "├"
	}
	return style.Border(border).Render(labels[i].text)
}

func (m *AppModel) renderTabOverflowIndicator(text string, isFirst bool) string {
	style := inactiveTabStyle.Foreground(subtle)
	border, _, _, _, _ := style.GetBorder()
	if isFirst {
		border.BottomLeft = "├"
	}
	return style.Border(border).Render(text)
}

func (m *AppModel) visibleTabWindow(labels []tabLabel) (int, int) {
	totalWidth := 0
	for _, label := range labels {
		totalWidth += label.width
	}
	if totalWidth <= m.width {
		return 0, len(labels)
	}

	indicatorWidth := func(text string) int {
		return lipgloss.Width(inactiveTabStyle.Render(text))
	}
	leftWidth, rightWidth := 0, 0
	if m.activeTab > 0 {
		leftWidth = indicatorWidth(fmt.Sprintf("↤ %d more", m.activeTab))
	}
	if m.activeTab < len(labels)-1 {
		rightWidth = indicatorWidth(fmt.Sprintf("%d more ↦", len(labels)-m.activeTab-1))
	}

	available := m.width - leftWidth - rightWidth
	start, end := m.activeTab, m.activeTab+1
	used := labels[m.activeTab].width
	for {
		expanded := false
		if start > 0 && used+labels[start-1].width <= available {
			start--
			used += labels[start].width
			expanded = true
		}
		if end < len(labels) && used+labels[end].width <= available {
			used += labels[end].width
			end++
			expanded = true
		}
		if !expanded {
			return start, end
		}
	}
}

func (m *AppModel) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft || m.waiting {
		return m, nil
	}
	m.hoveredGutterLine = 0
	m.hoveredGutterSide = ""
	if m.isTextModal() {
		return m.handleTextModalMouse(mouse)
	}
	if m.modal == discardChangesModal {
		return m.handleDiscardChangesModalMouse(mouse)
	}
	if m.modal == deleteConfirmModal {
		return m.handleDeleteConfirmModalMouse(mouse)
	}
	if m.modal == finishModal {
		return m.handleFinishModalMouse(mouse)
	}
	if m.modal != noModal {
		return m, nil
	}
	if rect, ok := m.footerFinishRect(); ok && rect.contains(mouse) {
		m.openFinishModal()
		return m, nil
	}

	headerHeight := m.headerHeight()
	if m.multiFile && !m.tabSearching && mouse.Y >= headerHeight && mouse.Y < headerHeight+m.tabBarHeight() {
		labels := m.tabLabels()
		for i := range labels {
			labels[i].rendered = m.renderTab(labels, i, i == 0)
			labels[i].width = lipgloss.Width(labels[i].rendered)
		}
		start, end := m.visibleTabWindow(labels)
		x := 0
		if start > 0 {
			indicator := m.renderTabOverflowIndicator(fmt.Sprintf("↤ %d more", start), true)
			width := lipgloss.Width(indicator)
			if mouse.X >= x && mouse.X < x+width {
				m.selectTab(start - 1)
				return m, nil
			}
			x += width
		}
		for i := start; i < end; i++ {
			if mouse.X >= x && mouse.X < x+labels[i].width {
				m.selectTab(i)
				return m, nil
			}
			x += labels[i].width
		}
		if end < len(labels) {
			indicator := m.renderTabOverflowIndicator(fmt.Sprintf("%d more ↦", len(labels)-end), false)
			if mouse.X >= x && mouse.X < x+lipgloss.Width(indicator) {
				m.selectTab(end)
				return m, nil
			}
		}
		return m, nil
	}

	left, top, right, bottom := m.contentBounds()
	if mouse.X >= left && mouse.X < right && mouse.Y >= top && mouse.Y < bottom {
		wasFocused := m.focused == contentPane
		m.focused = contentPane
		if target, ok := m.contentMouseTarget(mouse.Y - top + m.contentViewport.YOffset()); ok {
			t := m.tab()
			if mouse.X == left && !target.annotation && t.selecting {
				start, end := m.selectionRange()
				if start < end && target.side == t.selectSide && target.line == end {
					m.openLineComment()
					m.rebuildContent()
					return m, nil
				}
			}
			openThread := wasFocused && target.annotation && t.cursorOnAnnotation &&
				t.cursorLine == target.line && t.cursorSide == target.side && t.cursorAnnoIdx == target.annotationIndex
			t.cursorLine, t.cursorSide = target.line, target.side
			t.cursorOnAnnotation = target.annotation
			t.cursorAnnoIdx = target.annotationIndex
			if openThread {
				annotations := m.annotationsAfterLine(target.line, target.side)
				if target.annotationIndex < len(annotations) {
					m.openCommentThread(annotations[target.annotationIndex].id)
					return m, nil
				}
			}
			if mouse.X == left && !target.annotation {
				t.selecting = true
				t.selectAnchor = target.line
				t.selectSide = target.side
				m.mouseSelecting = true
			}
		}
		m.updateCommentSidebar()
		m.rebuildContent()
		return m, nil
	}

	left, top, right, bottom = m.commentBounds()
	if mouse.X >= left && mouse.X < right && mouse.Y >= top && mouse.Y < bottom {
		wasFocused := m.focused == commentPane
		m.focused = commentPane
		if mouse.Y > top {
			if i, ok := m.sidebarMouseTarget(mouse.Y - top - 1 + m.commentViewport.YOffset()); ok {
				t := m.tab()
				openThread := wasFocused && t.sidebarCursor == i
				m.selectSidebarItem(i)
				if openThread {
					m.openCommentThread(t.sidebarItems[i].id)
					return m, nil
				}
			}
		}
		m.updateCommentSidebar()
		m.rebuildContent()
	}
	return m, nil
}

func (m *AppModel) selectTab(index int) {
	if index < 0 || index >= len(m.tabs) || m.activeTab == index {
		return
	}
	m.activeTab = index
	m.rebuildContent()
	m.updateCommentSidebar()
}

type modalMouseAction struct {
	focus           int
	deleteIndex     int
	textarea        bool
	scrollable      bool
	scrollOffset    int
	scrollMaxOffset int
}

type modalMouseRegion struct {
	rect   mouseRect
	action modalMouseAction
}

type modalButtonSpec struct {
	rendered string
	action   modalMouseAction
}

func (m *AppModel) isTextModal() bool {
	return m.modal == commentModal || m.modal == fileCommentModal ||
		m.modal == replyModal || m.modal == editModal
}

func (m *AppModel) handleTextModalMouse(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	for _, region := range m.modalMouseRegions() {
		if !region.rect.contains(mouse) {
			continue
		}
		action := region.action
		if action.textarea {
			m.focusTextareaAt(mouse, region.rect)
			return m, nil
		}
		if action.scrollable {
			return m, nil
		}
		m.modalFocus = action.focus
		switch {
		case action.focus == 1:
			m.modalSubmit()
		case action.focus == 2:
			m.closeTextModal()
		case m.canSuggest() && action.focus == 3:
			m.insertSuggestion()
		case m.modal == editModal && action.focus >= m.modalDeleteStartFocus():
			m.openDeleteConfirmation(action.deleteIndex)
		}
		return m, nil
	}
	return m, nil
}

func (m *AppModel) focusTextareaAt(mouse tea.Mouse, rect mouseRect) {
	targetRow := m.modalTextarea.ScrollYOffset() + mouse.Y - rect.top
	m.modalTextarea.MoveToBegin()
	for range targetRow {
		line, column := m.modalTextarea.Line(), m.modalTextarea.Column()
		m.modalTextarea.CursorDown()
		if m.modalTextarea.Line() == line && m.modalTextarea.Column() == column {
			break
		}
	}

	lineInfo := m.modalTextarea.LineInfo()
	textX := max(0, mouse.X-rect.left-lipgloss.Width(m.modalTextarea.Prompt))
	lines := strings.Split(m.modalTextarea.Value(), "\n")
	line := []rune(lines[m.modalTextarea.Line()])
	column := lineInfo.StartColumn
	end := min(len(line), lineInfo.StartColumn+lineInfo.Width)
	for column < end && ansi.StringWidth(string(line[lineInfo.StartColumn:column+1])) <= textX {
		column++
	}
	m.modalTextarea.SetCursorColumn(column)
	m.modalFocus = 0
	m.modalTextarea.Focus()
}

func (m *AppModel) handleFinishModalMouse(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	for _, region := range m.modalMouseRegions() {
		if !region.rect.contains(mouse) {
			continue
		}
		m.modalFocus = region.action.focus
		if region.action.focus == 0 {
			return m.doFinish()
		}
		m.modal = noModal
		return m, nil
	}
	return m, nil
}

func (m *AppModel) handleDiscardChangesModalMouse(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	for _, region := range m.modalMouseRegions() {
		if !region.rect.contains(mouse) {
			continue
		}
		m.modalFocus = region.action.focus
		if region.action.focus == 0 {
			m.discardTextModal()
		} else {
			m.resumeTextModal()
		}
		return m, nil
	}
	return m, nil
}

func (m *AppModel) handleDeleteConfirmModalMouse(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	for _, region := range m.modalMouseRegions() {
		if !region.rect.contains(mouse) {
			continue
		}
		m.modalFocus = region.action.focus
		if region.action.focus == 0 {
			m.confirmDelete()
		} else {
			m.cancelDeleteConfirmation()
		}
		return m, nil
	}
	return m, nil
}

func (m *AppModel) handleMouseMotion(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	if !m.mouseSelecting {
		m.updateGutterHover(msg.Mouse())
		return m, nil
	}
	if msg.Mouse().Button != tea.MouseLeft {
		return m, nil
	}
	m.updateMouseSelection(msg.Mouse(), true)
	return m, nil
}

func (m *AppModel) updateGutterHover(mouse tea.Mouse) {
	line := 0
	side := ""
	if m.modal == noModal && !m.waiting && len(m.tabs) > 0 && m.tab().state != nil {
		left, top, _, bottom := m.contentBounds()
		if mouse.X == left && mouse.Y >= top && mouse.Y < bottom {
			if target, ok := m.contentMouseTarget(mouse.Y - top + m.contentViewport.YOffset()); ok && !target.annotation {
				line = target.line
				side = target.side
			}
		}
	}
	if m.hoveredGutterLine == line && m.hoveredGutterSide == side {
		return
	}
	m.hoveredGutterLine = line
	m.hoveredGutterSide = side
	m.rebuildContent()
}

func (m *AppModel) handleMouseRelease(msg tea.MouseReleaseMsg) (tea.Model, tea.Cmd) {
	if !m.mouseSelecting || msg.Mouse().Button != tea.MouseLeft {
		return m, nil
	}
	m.updateMouseSelection(msg.Mouse(), false)
	m.mouseSelecting = false
	t := m.tab()
	t.selecting = t.cursorLine != t.selectAnchor || t.cursorSide != t.selectSide
	m.openLineComment()
	m.rebuildContent()
	return m, nil
}

func (m *AppModel) updateMouseSelection(mouse tea.Mouse, autoScroll bool) {
	_, top, _, bottom := m.contentBounds()
	viewportY := mouse.Y - top + m.contentViewport.YOffset()
	if autoScroll && mouse.Y <= top {
		m.contentViewport.ScrollUp(1)
		viewportY = m.contentViewport.YOffset()
	} else if autoScroll && mouse.Y >= bottom-1 {
		m.contentViewport.ScrollDown(1)
		viewportY = m.contentViewport.YOffset() + m.contentViewport.Height() - 1
	} else if mouse.Y < top || mouse.Y >= bottom {
		return
	}

	if target, ok := m.contentMouseTarget(viewportY); ok {
		t := m.tab()
		if t.selecting && target.side != t.selectSide {
			return
		}
		t.cursorLine, t.cursorSide = target.line, target.side
		t.cursorOnAnnotation = false
		t.cursorAnnoIdx = 0
		t.selecting = true
		m.rebuildContent()
	}
}

type contentMouseTarget struct {
	line            int
	side            string
	annotation      bool
	annotationIndex int
}

func (m *AppModel) contentMouseTarget(y int) (contentMouseTarget, bool) {
	if y < 0 || y >= len(m.contentLayout.rows) {
		return contentMouseTarget{}, false
	}
	return m.contentLayout.rows[y], true
}

func (m *AppModel) highlightedCommentLines() (int, int, string) {
	t := m.tab()
	if m.focused == commentPane && len(t.sidebarItems) > 0 && t.sidebarCursor < len(t.sidebarItems) {
		item := t.sidebarItems[t.sidebarCursor]
		return item.line, max(item.line, item.endLine), item.side
	}
	if m.focused == contentPane && t.cursorOnAnnotation {
		annotations := m.annotationsAfterLine(t.cursorLine, t.cursorSide)
		if t.cursorAnnoIdx < len(annotations) {
			ann := annotations[t.cursorAnnoIdx]
			return ann.line, max(ann.line, ann.endLine), ann.side
		}
	}
	return 0, 0, ""
}

func (m *AppModel) commentBounds() (left, top, right, bottom int) {
	commentWidth := max(m.width/4, 20)
	left = m.width - commentWidth
	if m.multiFile {
		left++
	}
	top = m.headerHeight() + m.tabBarHeight()
	right = left + commentWidth
	bottom = top + m.contentViewport.Height()
	return left, top, right, bottom
}

func (m *AppModel) sidebarMouseTarget(y int) (int, bool) {
	if y < 0 || y >= len(m.sidebarTargets) {
		return 0, false
	}
	i := m.sidebarTargets[y]
	if i < 0 || i >= len(m.tab().sidebarItems) {
		return 0, false
	}
	return i, true
}

func (m *AppModel) selectSidebarItem(i int) {
	t := m.tab()
	item := t.sidebarItems[i]
	t.sidebarCursor = i
	if item.scope != "file" {
		t.cursorLine, t.cursorSide = item.line, item.side
		t.cursorOnAnnotation = false
		t.cursorAnnoIdx = 0
		m.scrollToAnnotation(item.side, item.line, item.endLine)
	}
}

func (m *AppModel) headerHeight() int {
	if m.width > 0 && len(m.tabs) > 0 {
		return lipgloss.Height(m.renderHeader())
	}
	return 1
}

func (m *AppModel) tabBarHeight() int {
	if !m.multiFile {
		return 0
	}
	return lipgloss.Height(m.renderTabBar())
}

func (m *AppModel) contentBounds() (left, top, right, bottom int) {
	left = 0
	top = m.headerHeight() + m.tabBarHeight()
	if m.multiFile {
		left = 1
	}
	right = left + m.contentViewport.Width()
	bottom = top + m.contentViewport.Height()
	return left, top, right, bottom
}

func (m *AppModel) footerFinishRect() (mouseRect, bool) {
	if len(m.tabs) == 0 || m.tab().state == nil || m.tab().selecting {
		return mouseRect{}, false
	}
	_, layout := m.renderReviewScreen()
	return layout.footerFinish, layout.hasFooterFinish
}

func (m *AppModel) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.isTextModal() {
		return m.handleTextModalWheel(msg.Mouse())
	}
	if m.modal != noModal || m.waiting {
		return m, nil
	}
	mouse := msg.Mouse()
	left, top, right, bottom := m.contentBounds()
	if mouse.X < left || mouse.X >= right || mouse.Y < top || mouse.Y >= bottom {
		return m, nil
	}

	switch mouse.Button {
	case tea.MouseWheelUp:
		m.contentViewport.ScrollUp(m.contentViewport.MouseWheelDelta)
	case tea.MouseWheelDown:
		m.contentViewport.ScrollDown(m.contentViewport.MouseWheelDelta)
	}
	m.updateGutterHover(mouse)
	return m, nil
}

func (m *AppModel) handleTextModalWheel(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	for _, region := range m.modalMouseRegions() {
		if !region.rect.contains(mouse) {
			continue
		}
		if region.action.scrollable {
			switch mouse.Button {
			case tea.MouseWheelUp:
				m.modalReferenceOffset = max(0, region.action.scrollOffset-3)
			case tea.MouseWheelDown:
				m.modalReferenceOffset = min(region.action.scrollMaxOffset, region.action.scrollOffset+3)
			}
			return m, nil
		}
		if !region.action.textarea {
			continue
		}
		m.modalFocus = 0
		m.modalTextarea.Focus()
		for range 3 {
			switch mouse.Button {
			case tea.MouseWheelUp:
				m.modalTextarea.CursorUp()
			case tea.MouseWheelDown:
				m.modalTextarea.CursorDown()
			}
		}
		return m, nil
	}
	return m, nil
}

// renderTabBar renders the tab bar for multi-file mode.
func (m *AppModel) renderTabBar() string {
	if m.tabSearching {
		prompt := tabSearchPromptStyle.Render("/")
		query := m.tabSearch
		matchInfo := ""
		if m.tabSearch != "" {
			matchInfo = fmt.Sprintf(" (%d matches)", len(m.tabMatches))
		}
		return prompt + query + footerStyle.Render(matchInfo)
	}

	labels := m.tabLabels()
	for i := range labels {
		rendered := m.renderTab(labels, i, i == 0)
		labels[i].rendered = rendered
		labels[i].width = lipgloss.Width(rendered)
	}

	// addFiller extends the tab bottom border to the full width,
	// connecting to the outer frame's right border.
	addFiller := func(row string) string {
		rowW := lipgloss.Width(row)
		if rowW >= m.width {
			return row
		}
		// 3 lines matching tab height: empty top, empty middle, ───╮ bottom
		gap := m.width - rowW
		topFill := strings.Repeat(" ", gap)
		midFill := strings.Repeat(" ", gap)
		botFill := strings.Repeat("─", gap-1) + "╮"
		filler := lipgloss.NewStyle().Foreground(accent).Render(
			topFill + "\n" + midFill + "\n" + botFill,
		)
		return lipgloss.JoinHorizontal(lipgloss.Top, row, filler)
	}

	start, end := m.visibleTabWindow(labels)
	if start == 0 && end == len(labels) {
		var tabs []string
		for i := range labels {
			tabs = append(tabs, labels[i].rendered)
		}
		row := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
		return addFiller(row)
	}

	var parts []string
	if start > 0 {
		ind := m.renderTabOverflowIndicator(fmt.Sprintf("↤ %d more", start), true)
		parts = append(parts, ind)
	}
	for i := start; i < end; i++ {
		parts = append(parts, m.renderTab(labels, i, i == start && start == 0))
	}
	if end < len(labels) {
		ind := m.renderTabOverflowIndicator(fmt.Sprintf("%d more ↦", len(labels)-end), false)
		parts = append(parts, ind)
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	return addFiller(row)
}

func (m AppModel) renderFooter() string {
	t := m.tabs[m.activeTab]
	k := func(key, desc string) string {
		return footerKeyStyle.Render(key) + " " + footerStyle.Render(desc)
	}

	var items []string
	if t.selecting {
		items = []string{
			k("enter", "comment selection"),
			k("esc", "cancel"),
			k("v", "toggle select"),
			k("?", "help"),
		}
	} else {
		items = []string{
			k("[/]", "prev/next comment"),
			k("s", "sidebar"),
			k("v", "select lines"),
			k("enter", "comment"),
			k("f", "file comment"),
		}
		if len(t.state.Comments) > 0 {
			items = append(items, k("r", "resolve/unresolve"))
		}
		items = append(items, k("?", "help"))
		if m.multiFile {
			items = append([]string{
				k("tab/S-tab", "next/prev tab"),
				k("n/N", "next/prev change"),
			}, items...)
		}
		items = append([]string{m.renderModalButton(m.finishActionLabel(), "q", true)}, items...)
	}

	return footerStyle.Width(m.width).Render(strings.Join(items, "  "))
}

type helpItem struct {
	keys string
	desc string
}

func renderHelpGroup(title string, items []helpItem, width int) string {
	if width < 6 {
		width = 6
	}
	keyWidth := 0
	for _, item := range items {
		if w := lipgloss.Width(item.keys); w > keyWidth {
			keyWidth = w
		}
	}
	keyWidth++
	if keyWidth > width-5 {
		keyWidth = width - 5
	}

	var b strings.Builder
	b.WriteString(footerKeyStyle.Render(title))
	b.WriteString("\n")
	for i, item := range items {
		key := footerKeyStyle.Width(keyWidth).Render(item.keys)
		b.WriteString(key + footerStyle.Render(item.desc))
		if i < len(items)-1 {
			b.WriteString("\n")
		}
	}
	return lipgloss.NewStyle().Width(width).Render(b.String())
}

func (m AppModel) renderHelp(innerWidth int) string {
	columnWidth := (innerWidth - 4) / 3
	general := renderHelpGroup("General", []helpItem{
		{keys: "enter", desc: "comment/open"},
		{keys: "f", desc: "file comment"},
		{keys: "v", desc: "select"},
		{keys: "s", desc: "sidebar"},
		{keys: "r", desc: "resolve"},
		{keys: "d", desc: "delete comment"},
		{keys: "q/ctrl+c", desc: "finish"},
		{keys: "?", desc: "help"},
	}, columnWidth)

	navigation := renderHelpGroup("Navigation", []helpItem{
		{keys: "↑/↓,j/k", desc: "move"},
		{keys: "PgUp/PgDn", desc: "half page"},
		{keys: "shift+↑/↓,ctrl+u/d", desc: "half page"},
		{keys: "Home/End,g/G,</>", desc: "top/bottom"},
		{keys: "[/]", desc: "comments"},
	}, columnWidth)
	codeReview := renderHelpGroup("Code review / search", []helpItem{
		{keys: "tab/shift+tab", desc: "files"},
		{keys: "1-9", desc: "file tab"},
		{keys: "/", desc: "search"},
		{keys: "n/N", desc: "changes"},
		{keys: "type/Backspace", desc: "filter"},
		{keys: "tab", desc: "next match"},
		{keys: "enter/esc", desc: "open/cancel"},
	}, columnWidth)

	columns := lipgloss.JoinHorizontal(lipgloss.Top, general, "  ", navigation, "  ", codeReview)
	contexts := renderHelpGroup("Selection and dialogs", []helpItem{
		{keys: "↑/↓,j/k · enter/v/esc", desc: "extend · comment/toggle/cancel selection"},
		{keys: "ctrl+s/o/y · tab/S-tab · enter/esc", desc: "save/editor/suggest · focus · activate/close dialog"},
		{keys: "y/n/esc · ←/→,h/l,tab/shift+tab · enter", desc: "confirm/cancel · focus · activate finish dialog"},
		{keys: "q/ctrl+c", desc: "quit from finish dialog"},
	}, innerWidth)

	return columns + "\n" + contexts
}

func (m AppModel) renderModalButton(label, hint string, focused bool) string {
	labelStyle := modalBtnNormalLabel
	keyStyle := modalBtnNormalKey
	if focused {
		labelStyle = modalBtnFocusedLabel
		keyStyle = modalBtnFocusedKey
	}

	keys := strings.Split(hint, " / ")
	var renderedHint strings.Builder
	for i, key := range keys {
		if i > 0 {
			renderedHint.WriteString(labelStyle.Padding(0).Render(" / "))
		}
		renderedHint.WriteString(keyStyle.Render(key))
	}
	return labelStyle.Render(label+" ") + renderedHint.String()
}

func layoutModalButtonRow(specs []modalButtonSpec, width, top int) (string, []modalMouseRegion) {
	var row strings.Builder
	regions := make([]modalMouseRegion, 0, len(specs))
	x, y := 0, top
	for _, spec := range specs {
		buttonWidth := lipgloss.Width(spec.rendered)
		gap := 0
		if x > 0 {
			gap = 2
		}
		if x > 0 && x+gap+buttonWidth > width {
			row.WriteByte('\n')
			x = 0
			y++
			gap = 0
		}
		if gap > 0 {
			row.WriteString("  ")
			x += gap
		}
		row.WriteString(spec.rendered)
		regions = append(regions, modalMouseRegion{
			rect: mouseRect{
				left: x, top: y,
				right: x + buttonWidth, bottom: y + lipgloss.Height(spec.rendered),
			},
			action: spec.action,
		})
		x += buttonWidth
	}
	return row.String(), regions
}

func layoutModalTextarea(before, textareaView string, width int) (string, modalMouseRegion) {
	before = lipgloss.Wrap(before, width, "")
	top := strings.Count(before, "\n")
	return before + textareaView + "\n\n", modalMouseRegion{
		rect: mouseRect{
			left: 0, top: top,
			right: min(width, lipgloss.Width(textareaView)), bottom: top + lipgloss.Height(textareaView),
		},
		action: modalMouseAction{textarea: true},
	}
}

func renderScrollableModalBox(content string, width, maxHeight, offset int) (string, int, int) {
	if content == "" || maxHeight < 3 {
		return "", 0, 0
	}

	contentWidth := max(1, width-2)
	wrapped := lipgloss.Wrap(content, contentWidth, "")
	lines := strings.Split(wrapped, "\n")
	maxContentHeight := maxHeight - 2
	maxOffset := max(0, len(lines)-maxContentHeight)
	if offset < 0 {
		offset = maxOffset
	} else {
		offset = min(offset, maxOffset)
	}
	lines = lines[offset:min(len(lines), offset+maxContentHeight)]
	if offset > 0 {
		lines[0] = footerStyle.Render("↑ ") + ansi.Truncate(lines[0], max(0, contentWidth-2), "")
	}
	if offset < maxOffset {
		last := len(lines) - 1
		lines[last] = ansi.Truncate(lines[last], max(0, contentWidth-2), "") + footerStyle.Render(" ↓")
	}

	borderStyle := lipgloss.NewStyle().Foreground(subtle)
	top := borderStyle.Render("╭" + strings.Repeat("─", contentWidth) + "╮")
	bottom := borderStyle.Render("╰" + strings.Repeat("─", contentWidth) + "╯")
	rows := make([]string, 0, len(lines)+2)
	rows = append(rows, top)
	for _, line := range lines {
		line = ansi.Truncate(line, contentWidth, "")
		rows = append(rows, borderStyle.Render("│")+
			lipgloss.NewStyle().Width(contentWidth).Render(line)+borderStyle.Render("│"))
	}
	rows = append(rows, bottom)

	return strings.Join(rows, "\n"), offset, maxOffset
}

func (m AppModel) modalMouseRegions() []modalMouseRegion {
	_, layout := m.renderReviewScreen()
	return layout.modalRegions
}

func (m AppModel) renderDeleteButton(label, hint string, focused bool) string {
	if hint != "" {
		keyStyle := modalBtnNormalKey
		if focused {
			keyStyle = modalBtnFocusedKey
			return modalDeleteBtnFocused.PaddingRight(0).Render(label+" ") + keyStyle.Render(hint)
		}
		return modalBtnNormal.PaddingRight(0).Render(modalDeleteBtnLabel.Render(label+" ")) + keyStyle.Render(hint)
	}
	if focused {
		return modalDeleteBtnFocused.Render(label)
	}
	return modalBtnNormal.Render(modalDeleteBtnLabel.Render(label))
}

func (m AppModel) renderContextPreview(side string, start, end, maxWidth, maxLines int) string {
	t := m.tabs[m.activeTab]
	if t.doc == nil {
		return ""
	}
	var lines []string
	maxLineText := maxWidth - 7
	if maxLineText < 10 {
		maxLineText = 10
	}
	for i := start; i <= end; i++ {
		lineText := m.anchorText(&t, side, i, i)
		if lineText == "" {
			continue
		}
		wrapped := lipgloss.Wrap(lineText, maxLineText, "")
		num := lineNumStyle.Render(fmt.Sprintf("%d", i))
		wrapLines := strings.Split(wrapped, "\n")
		for wi, wl := range wrapLines {
			if wi == 0 {
				lines = append(lines, num+" "+wl)
			} else {
				lines = append(lines, lipgloss.NewStyle().Width(6).Render("")+wl)
			}
		}
	}
	if maxLines > 0 && len(lines) > maxLines {
		lines = append(lines[:maxLines-1], footerStyle.Render(fmt.Sprintf("  ... +%d more lines", len(lines)-maxLines+1)))
	}
	return strings.Join(lines, "\n")
}

func (m AppModel) renderWithModal(background string) string {
	rendered, _ := m.renderWithModalLayout(background)
	return rendered
}

func (m AppModel) renderWithModalLayout(background string) (string, []modalMouseRegion) {
	var modalContent string
	var regions []modalMouseRegion
	bgW := lipgloss.Width(background)
	bgH := lipgloss.Height(background)
	modalWidth := m.width * 2 / 3
	if m.modal == helpModal {
		modalWidth = m.width - 4
	}
	if modalWidth < 50 {
		modalWidth = 50
	}
	if modalWidth > m.width-4 {
		modalWidth = m.width - 4
	}
	innerWidth := modalWidth - 6

	switch m.modal {
	case helpModal:
		title := modalTitleStyle.MarginBottom(0).Render("Keyboard Help  (? / esc to close)")
		modalContent = modalStyle.Width(modalWidth).Render(
			title + "\n" + m.renderHelp(innerWidth))

	case commentModal:
		start, end := m.selectionRange()
		side := m.selectionSide()
		var title string
		if start != end {
			title = modalTitleStyle.Render(fmt.Sprintf("Add Comment (lines %d-%d)", start, end))
		} else {
			title = modalTitleStyle.Render(fmt.Sprintf("Add Comment (line %d)", start))
		}
		contextBox := contextBoxStyle.
			Width(innerWidth - 2).
			Render(m.renderContextPreview(side, start, end, innerWidth-4, 8))

		prefix, textareaRegion := layoutModalTextarea(
			title+"\n"+contextBox+"\n\n", m.modalTextarea.View(), innerWidth)
		regions = append(regions, textareaRegion)
		buttonSpecs := []modalButtonSpec{
			{rendered: m.renderModalButton("Save", "ctrl+s", m.modalFocus == 1), action: modalMouseAction{focus: 1}},
			{rendered: m.renderModalButton("Close", "esc", m.modalFocus == 2), action: modalMouseAction{focus: 2}},
		}
		if m.canSuggest() {
			buttonSpecs = append(buttonSpecs, modalButtonSpec{
				rendered: m.renderModalButton("Suggest", "ctrl+y", m.modalFocus == 3),
				action:   modalMouseAction{focus: 3},
			})
		}
		buttons, buttonRegions := layoutModalButtonRow(buttonSpecs, innerWidth, strings.Count(prefix, "\n"))
		regions = append(regions, buttonRegions...)
		modalContent = modalStyle.Width(modalWidth).Render(prefix + buttons)

	case fileCommentModal:
		title := modalTitleStyle.Render("Add File Comment")
		path := contextBoxStyle.Width(innerWidth - 2).Render(m.tab().path)
		prefix, textareaRegion := layoutModalTextarea(
			title+"\n"+path+"\n\n", m.modalTextarea.View(), innerWidth)
		regions = append(regions, textareaRegion)
		buttons, buttonRegions := layoutModalButtonRow([]modalButtonSpec{
			{rendered: m.renderModalButton("Save", "ctrl+s", m.modalFocus == 1), action: modalMouseAction{focus: 1}},
			{rendered: m.renderModalButton("Close", "esc", m.modalFocus == 2), action: modalMouseAction{focus: 2}},
		}, innerWidth, strings.Count(prefix, "\n"))
		regions = append(regions, buttonRegions...)
		modalContent = modalStyle.Width(modalWidth).Render(prefix + buttons)

	case replyModal, editModal:
		titleText := "Edit Comment"
		if m.modal == replyModal {
			titleText = "Add Reply"
		} else if m.editingReplyID != "" {
			titleText = "Edit Reply"
		}
		title := modalTitleStyle.Render(titleText)
		var referenceContent string
		for _, c := range m.tabs[m.activeTab].state.Comments {
			if c.ID == m.editingID {
				if c.Scope == "file" {
					referenceContent = m.tab().path
				} else {
					start := c.StartLine
					end := c.EndAt()
					referenceContent = m.renderContextPreview(c.Side, start, end, innerWidth-4, 0)
				}
				if m.modal == replyModal || m.editingReplyID != "" {
					var thread strings.Builder
					author := c.Author
					if author == "" {
						author = "comment"
					}
					thread.WriteString(commentLineStyle.Render("Comment — "+author) + "\n")
					thread.WriteString(c.Body)
					for i, reply := range c.Replies {
						replyAuthor := reply.Author
						if replyAuthor == "" {
							replyAuthor = "reply"
						}
						prefix := "  "
						if reply.ID == m.editingReplyID {
							prefix = "> "
						}
						thread.WriteString("\n" + replyStyle.Render(
							fmt.Sprintf("%s%d. %s: %s", prefix, i+1, replyAuthor, reply.Body)))
					}
					if referenceContent != "" {
						referenceContent += "\n\n"
					}
					referenceContent += thread.String()
				}
				break
			}
		}
		buttonSpecs := []modalButtonSpec{
			{rendered: m.renderModalButton("Save", "ctrl+s", m.modalFocus == 1), action: modalMouseAction{focus: 1}},
			{rendered: m.renderModalButton("Close", "esc", m.modalFocus == 2), action: modalMouseAction{focus: 2}},
		}
		if m.canSuggest() {
			buttonSpecs = append(buttonSpecs, modalButtonSpec{
				rendered: m.renderModalButton("Suggest", "ctrl+y", m.modalFocus == 3),
				action:   modalMouseAction{focus: 3},
			})
		}

		buildContent := func(referenceSection string, scrollOffset, scrollMaxOffset int) (string, []modalMouseRegion) {
			content := title + "\n"
			var contentRegions []modalMouseRegion
			if referenceSection != "" {
				referenceTop := strings.Count(content, "\n")
				content += referenceSection + "\n\n"
				contentRegions = append(contentRegions, modalMouseRegion{
					rect: mouseRect{
						left: 0, top: referenceTop,
						right: lipgloss.Width(referenceSection), bottom: referenceTop + lipgloss.Height(referenceSection),
					},
					action: modalMouseAction{
						scrollable: true, scrollOffset: scrollOffset, scrollMaxOffset: scrollMaxOffset,
					},
				})
			}
			content, textareaRegion := layoutModalTextarea(content, m.modalTextarea.View(), innerWidth)
			contentRegions = append(contentRegions, textareaRegion)
			buttonY := strings.Count(content, "\n")
			buttonRow, buttonRegions := layoutModalButtonRow(buttonSpecs, innerWidth, buttonY)
			content += buttonRow
			contentRegions = append(contentRegions, buttonRegions...)
			buttonY += lipgloss.Height(buttonRow)
			deleteStart := m.modalDeleteStartFocus()
			for i, target := range m.modalDeleteTargets() {
				content += "\n"
				deleteRow, deleteRegions := layoutModalButtonRow([]modalButtonSpec{{
					rendered: m.renderDeleteButton(target.label, "", m.modalFocus == i+deleteStart),
					action:   modalMouseAction{focus: i + deleteStart, deleteIndex: i},
				}}, innerWidth, buttonY)
				content += deleteRow
				contentRegions = append(contentRegions, deleteRegions...)
				buttonY += lipgloss.Height(deleteRow)
			}
			return content, contentRegions
		}

		fixedContent, _ := buildContent("", 0, 0)
		fixedHeight := lipgloss.Height(modalStyle.Width(modalWidth).Render(fixedContent))
		referenceHeight := max(3, bgH-fixedHeight-3)
		referenceSection, scrollOffset, scrollMaxOffset := renderScrollableModalBox(
			referenceContent, innerWidth-2, referenceHeight, m.modalReferenceOffset)
		content, contentRegions := buildContent(referenceSection, scrollOffset, scrollMaxOffset)
		modalContent = modalStyle.Width(modalWidth).Render(content)
		regions = append(regions, contentRegions...)

	case discardChangesModal:
		title := modalTitleStyle.Render("Discard changes?")
		info := "Your unsaved comment changes will be lost."
		prefix := lipgloss.Wrap(title+"\n"+info+"\n\n", innerWidth, "")
		buttons, buttonRegions := layoutModalButtonRow([]modalButtonSpec{
			{rendered: m.renderModalButton("Discard", "y", m.modalFocus == 0), action: modalMouseAction{focus: 0}},
			{rendered: m.renderModalButton("Keep Editing", "n / esc", m.modalFocus == 1), action: modalMouseAction{focus: 1}},
		}, innerWidth, strings.Count(prefix, "\n"))
		regions = append(regions, buttonRegions...)
		modalContent = modalStyle.Width(modalWidth).Render(prefix + buttons)

	case deleteConfirmModal:
		titleText := "Delete comment?"
		if targets := m.modalDeleteTargets(); m.pendingDelete >= 0 && m.pendingDelete < len(targets) && targets[m.pendingDelete].replyID != "" {
			titleText = "Delete reply?"
		}
		title := modalTitleStyle.Render(titleText)
		info := "This cannot be undone."
		prefix := lipgloss.Wrap(title+"\n"+info+"\n\n", innerWidth, "")
		buttons, buttonRegions := layoutModalButtonRow([]modalButtonSpec{
			{rendered: m.renderDeleteButton("Delete", "y", m.modalFocus == 0), action: modalMouseAction{focus: 0}},
			{rendered: m.renderModalButton("Keep", "n / esc", m.modalFocus == 1), action: modalMouseAction{focus: 1}},
		}, innerWidth, strings.Count(prefix, "\n"))
		regions = append(regions, buttonRegions...)
		modalContent = modalStyle.Width(modalWidth).Render(prefix + buttons)

	case finishModal:
		unresolved := m.unresolvedTotal()
		var title, info string
		if unresolved == 0 {
			title = modalTitleStyle.Render("Approve review?")
			info = "No unresolved comments — approving ends the review."
		} else if !m.newFeedback {
			title = modalTitleStyle.Render("Resolve all & Approve?")
			info = fmt.Sprintf("%d unresolved comment(s) will be resolved.", unresolved)
		} else {
			title = modalTitleStyle.Render("Finish review?")
			info = fmt.Sprintf("%d unresolved comment(s) will be sent to the agent.", unresolved)
		}

		prefix := title + "\n" + info + "\n\n"
		prefix = lipgloss.Wrap(prefix, innerWidth, "")
		buttons, buttonRegions := layoutModalButtonRow([]modalButtonSpec{
			{rendered: m.renderModalButton(m.finishActionLabel(), "y", m.modalFocus == 0), action: modalMouseAction{focus: 0}},
			{rendered: m.renderModalButton("Close", "n", m.modalFocus == 1), action: modalMouseAction{focus: 1}},
		}, innerWidth, strings.Count(prefix, "\n"))
		regions = append(regions, buttonRegions...)
		hint := footerStyle.Render("esc: back to review · q: quit without finishing")

		modalContent = modalStyle.Width(modalWidth).Render(prefix + buttons + "\n" + hint)
	}

	modalW := lipgloss.Width(modalContent)
	modalH := lipgloss.Height(modalContent)

	mx := (bgW - modalW) / 2
	my := (bgH - modalH) / 2
	if mx < 0 {
		mx = 0
	}
	if modalH > bgH && (m.modal == replyModal || m.modal == editModal) {
		my = bgH - modalH
	} else if my < 0 {
		my = 0
	}
	contentX := mx + modalStyle.GetBorderLeftSize() + modalStyle.GetPaddingLeft()
	contentY := my + modalStyle.GetBorderTopSize() + modalStyle.GetPaddingTop()
	for i := range regions {
		regions[i].rect.left += contentX
		regions[i].rect.right += contentX
		regions[i].rect.top += contentY
		regions[i].rect.bottom += contentY
	}

	background = dimRendered(background, bgW, bgH)

	bgLayer := lipgloss.NewLayer(background)
	modalLayer := lipgloss.NewLayer(modalContent).X(mx).Y(my).Z(1)

	comp := lipgloss.NewCompositor(bgLayer, modalLayer)
	return comp.Render(), regions
}

func dimRendered(s string, w, h int) string {
	canvas := lipgloss.NewCanvas(w, h)
	canvas.Compose(lipgloss.NewLayer(s))

	dim := lipgloss.Color("#555555")
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cell := canvas.CellAt(x, y)
			if cell != nil {
				cell.Style.Fg = dim
			}
		}
	}
	return canvas.Render()
}

// clampLines truncates text to maxLines and appends "…" if truncated.
func clampLines(text string, maxLines int) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= maxLines {
		return text
	}
	return strings.Join(lines[:maxLines], "\n") + "…"
}
