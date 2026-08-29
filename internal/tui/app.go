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
	replyModal
	editModal
	finishModal
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

	contentViewport viewport.Model
	commentViewport viewport.Model
	modalTextarea   textarea.Model

	// Editing state
	editingID      string // ID of the parent comment being edited or replied to
	editingReplyID string // ID of the reply being edited; empty when editing the parent
	modalFocus     int    // 0=textarea, 1=save button, 2=cancel button, 3+=delete buttons
	newFeedback    bool   // true after adding or editing a comment in this round

	err error
}

// tab returns the active FileTab. Panics if no tabs exist.
func (m *AppModel) tab() *FileTab {
	return &m.tabs[m.activeTab]
}

func NewApp(filePath string, cfg AppConfig) AppModel {
	ta := textarea.New()
	ta.Placeholder = "Type your comment..."
	ta.ShowLineNumbers = false
	ta.CharLimit = 2000

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
	ta.CharLimit = 2000

	// Sort files alphabetically by path
	sortedFiles := make([]gitpkg.FileChange, len(files))
	copy(sortedFiles, files)
	sort.Slice(sortedFiles, func(i, j int) bool {
		return sortedFiles[i].Path < sortedFiles[j].Path
	})

	tabs := make([]FileTab, 0, len(sortedFiles))
	for _, f := range sortedFiles {
		var diff *gitpkg.DiffInfo
		if f.Status != gitpkg.StatusDeleted && f.Status != gitpkg.StatusBinary {
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

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		initAdaptiveStyles(msg.IsDark())
		if len(m.tabs) > 0 && m.tab().state != nil {
			m.rebuildContent()
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalculateLayout()
		if len(m.tabs) > 0 && m.tab().state != nil {
			m.rebuildContent()
		}
		return m, nil

	case docRenderedMsg:
		// Load documents and existing review comments for each tab
		for i := range m.tabs {
			t := &m.tabs[i]
			t.state = &fileReview{Comments: m.sessionComments(t.path)}
			if t.isBinary || t.isDeleted {
				continue
			}
			doc, _ := document.Load(t.path)
			t.doc = doc
			t.ensureHighlightCache()
		}

		m.rebuildContent()
		m.updateCommentSidebar()
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil

	case RoundStartMsg:
		m.startNextRound()
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)
	}

	var cmd tea.Cmd
	if m.modal == commentModal || m.modal == replyModal || m.modal == editModal {
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
	if m.modal == commentModal || m.modal == replyModal || m.modal == editModal {
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
		m.persist()
		m.modal = finishModal
		m.modalFocus = 0
		return m, nil

	case key.Matches(msg, keys.Cancel):
		// Esc cancels selection
		if t.selecting {
			t.selecting = false
			m.rebuildContent()
			return m, nil
		}
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
			}
			m.rebuildContent()
			return m, nil
		}
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
				anns := m.annotationsAfterLine(t.cursorLine)
				if t.cursorAnnoIdx < len(anns)-1 {
					t.cursorAnnoIdx++
				} else {
					t.cursorOnAnnotation = false
					t.cursorAnnoIdx = 0
					if t.cursorLine < t.doc.LineCount() {
						t.cursorLine++
					}
				}
			} else {
				anns := m.annotationsAfterLine(t.cursorLine)
				if len(anns) > 0 {
					t.cursorOnAnnotation = true
					t.cursorAnnoIdx = 0
				} else if t.cursorLine < t.doc.LineCount() {
					t.cursorLine++
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
				prevLine := t.cursorLine - 1
				if prevLine >= 1 {
					anns := m.annotationsAfterLine(prevLine)
					if len(anns) > 0 {
						t.cursorLine = prevLine
						t.cursorOnAnnotation = true
						t.cursorAnnoIdx = len(anns) - 1
					} else {
						t.cursorLine = prevLine
					}
				}
			}
			moved = true
		case key.Matches(msg, keys.HalfPageDown):
			t.cursorOnAnnotation = false
			t.cursorAnnoIdx = 0
			jump := m.contentViewport.Height() / 2
			t.cursorLine += jump
			if t.cursorLine > t.doc.LineCount() {
				t.cursorLine = t.doc.LineCount()
			}
			moved = true
		case key.Matches(msg, keys.HalfPageUp):
			t.cursorOnAnnotation = false
			t.cursorAnnoIdx = 0
			jump := m.contentViewport.Height() / 2
			t.cursorLine -= jump
			if t.cursorLine < 1 {
				t.cursorLine = 1
			}
			moved = true
		case key.Matches(msg, keys.Top):
			t.cursorOnAnnotation = false
			t.cursorAnnoIdx = 0
			t.cursorLine = 1
			moved = true
		case key.Matches(msg, keys.Bottom):
			t.cursorOnAnnotation = false
			t.cursorAnnoIdx = 0
			t.cursorLine = t.doc.LineCount()
			moved = true
		case key.Matches(msg, keys.Resolve):
			if t.cursorOnAnnotation {
				anns := m.annotationsAfterLine(t.cursorLine)
				if t.cursorAnnoIdx < len(anns) {
					m.toggleResolve(anns[t.cursorAnnoIdx].id)
				}
				return m, nil
			}
		case key.Matches(msg, keys.Confirm):
			if t.cursorOnAnnotation {
				anns := m.annotationsAfterLine(t.cursorLine)
				if t.cursorAnnoIdx < len(anns) {
					m.openCommentThread(anns[t.cursorAnnoIdx].id)
					return m, nil
				}
			} else if t.state != nil {
				m.modal = commentModal
				m.modalFocus = 0
				m.modalTextarea.Placeholder = "Type your comment..."
				m.modalTextarea.Reset()
				m.modalTextarea.Focus()
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
			t.cursorLine = sel.line
			m.scrollToAnnotation(sel.line, sel.endLine)
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
		m.modalTextarea.Focus()
		return
	}
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
		now := review.Now()
		c := review.Comment{
			ID:        review.RandomCommentID(),
			StartLine: startLine,
			EndLine:   endLine,
			Anchor:    m.anchorText(t, startLine, endLine),
			Body:      body,
			Author:    m.author,
			Scope:     "line",
			CreatedAt: now,
			UpdatedAt: now,
		}
		if m.session != nil {
			c.ReviewRound = m.session.CJ.ReviewRound
		}
		t.state.Comments = append(t.state.Comments, c)
	}
	m.newFeedback = true
	m.editingID = ""
	m.editingReplyID = ""

	m.persist()
	m.modal = noModal
	m.modalTextarea.Blur()
	t.selecting = false
	m.rebuildContent()
	m.updateCommentSidebar()
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
func (m *AppModel) anchorText(t *FileTab, start, end int) string {
	if t.doc == nil {
		return ""
	}
	lines := make([]string, 0, end-start+1)
	for l := start; l <= end && l <= t.doc.LineCount(); l++ {
		lines = append(lines, t.doc.LineAt(l))
	}
	return strings.Join(lines, "\n")
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

	now := review.Now()
	for i := range m.tabs {
		t := &m.tabs[i]
		comments := m.sessionComments(t.path)
		if t.isBinary || t.isDeleted {
			t.state = &fileReview{Comments: comments}
			continue
		}
		doc, _ := document.Load(t.path)
		t.doc = doc
		t.chromaLines = nil
		t.deletedLineCache = nil
		if prev, ok := prevContents[t.path]; ok && t.doc != nil {
			comments = review.CarryForwardFile(comments, prev, t.doc.Content, now)
		}
		t.state = &fileReview{Comments: comments}
		if m.multiFile && m.baseRef != "" {
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

func (m *AppModel) handleTextModal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	focusCount := 3
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
		} else if m.modalFocus >= 3 && m.modal == editModal {
			m.modalDelete(m.modalFocus - 3)
			return m, nil
		}
	case "ctrl+s":
		m.modalSubmit()
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
	m.modal = noModal
	m.editingID = ""
	m.editingReplyID = ""
	m.modalTextarea.Blur()
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
	headerHeight := 1
	if m.detached {
		headerHeight = 2
	}
	tabBarHeight := 0
	if m.multiFile {
		tabBarHeight = 3 // bordered tabs: top border + content + bottom border
	}
	footerHeight := 1
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
	mainHeight := m.height - headerHeight - tabBarHeight - footerHeight - frameBorderHeight - tmuxPadding

	commentWidth := m.width / 4
	if commentWidth < 20 {
		commentWidth = 20
	}
	contentWidth := m.width - commentWidth - frameBorderWidth

	m.contentViewport.SetWidth(contentWidth)
	m.contentViewport.SetHeight(mainHeight)
	m.commentViewport.SetWidth(commentWidth - 3) // -3 for left border + padding + margin
	m.commentViewport.SetHeight(mainHeight - 1)  // -1 for the "Comments (N)" header line

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
func (m *AppModel) annotationsAfterLine(lineNum int) []annotation {
	t := m.tab()
	if t.state == nil {
		return nil
	}
	var anns []annotation
	for _, c := range t.state.Comments {
		if c.EndAt() == lineNum {
			anns = append(anns, newAnnotation(c))
		}
	}
	return anns
}

type commentTarget struct {
	line    int
	annoIdx int
}

func (m *AppModel) commentTargets(tabIndex int) []commentTarget {
	t := &m.tabs[tabIndex]
	if t.state == nil {
		return nil
	}

	indices := make(map[int]int)
	targets := make([]commentTarget, 0, len(t.state.Comments))
	for _, c := range t.state.Comments {
		line := c.EndAt()
		targets = append(targets, commentTarget{line: line, annoIdx: indices[line]})
		indices[line]++
	}
	sort.SliceStable(targets, func(i, j int) bool {
		return targets[i].line < targets[j].line
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
			if target.line == t.cursorLine && target.annoIdx == t.cursorAnnoIdx {
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
			if target.line >= t.cursorLine {
				m.selectComment(m.activeTab, target)
				return true
			}
		}
	} else {
		for i := len(targets) - 1; i >= 0; i-- {
			if targets[i].line <= t.cursorLine {
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
	t.cursorLine = target.line
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
	t.cursorLine = chunk.startLine
	t.cursorOnAnnotation = false
	t.cursorAnnoIdx = 0
	m.rebuildContent()
	m.updateCommentSidebar()
	m.scrollToChunk(chunk)
}

// sidebarItem represents a comment in the sidebar list.
type sidebarItem struct {
	id       string
	line     int
	endLine  int
	body     string
	author   string
	resolved bool
	replies  []review.Reply
}

// annotation represents an inline comment to render.
type annotation struct {
	id       string
	body     string
	line     int
	endLine  int
	author   string
	resolved bool
	replies  []review.Reply
}

func newAnnotation(c review.Comment) annotation {
	return annotation{
		id: c.ID, body: c.Body,
		line: c.StartLine, endLine: c.EndLine,
		author: c.Author, resolved: c.Resolved, replies: c.Replies,
	}
}

// rebuildContent renders the document line-by-line with cursor, selection,
// line numbers, and bordered inline annotations.
func (m *AppModel) rebuildContent() {
	t := m.tab()

	// Handle placeholder tabs
	if t.isBinary {
		m.contentViewport.SetContent("\n  Binary file changed — cannot display content.\n")
		return
	}
	if t.isDeleted {
		m.contentViewport.SetContent("\n  File deleted.\n")
		return
	}

	if t.doc == nil {
		return
	}

	// Collect annotations keyed by the line they appear AFTER
	annosByEndLine := make(map[int][]annotation)
	if t.state != nil {
		for _, c := range t.state.Comments {
			endAt := c.EndAt()
			annosByEndLine[endAt] = append(annosByEndLine[endAt], newAnnotation(c))
		}
	}

	// Count how many comments cover each line (for overlap detection)
	annotatedLines := make(map[int]int)
	if t.state != nil {
		for _, c := range t.state.Comments {
			for l := c.StartLine; l <= c.EndAt(); l++ {
				annotatedLines[l]++
			}
		}
	}

	selStart, selEnd := m.selectionRange()

	// Determine which lines to highlight from selected annotation
	var sidebarHighlightStart, sidebarHighlightEnd int
	if m.focused == commentPane && len(t.sidebarItems) > 0 && t.sidebarCursor < len(t.sidebarItems) {
		sel := t.sidebarItems[t.sidebarCursor]
		sidebarHighlightStart = sel.line
		sidebarHighlightEnd = sel.line
		if sel.endLine > 0 {
			sidebarHighlightEnd = sel.endLine
		}
	} else if m.focused == contentPane && t.cursorOnAnnotation {
		anns := m.annotationsAfterLine(t.cursorLine)
		if t.cursorAnnoIdx < len(anns) {
			ann := anns[t.cursorAnnoIdx]
			sidebarHighlightStart = ann.line
			sidebarHighlightEnd = ann.line
			if ann.endLine > 0 {
				sidebarHighlightEnd = ann.endLine
			}
		}
	}

	contentWidth := m.contentViewport.Width()
	boxWidth := contentWidth - 7
	if boxWidth < 20 {
		boxWidth = 20
	}

	textWidth := contentWidth - 8
	if textWidth < 10 {
		textWidth = 10
	}

	// Use cached syntax highlighting
	isMarkdown := t.isMarkdown
	chromaLines := t.chromaLines

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
	for i, line := range t.doc.Lines {
		lineNum := i + 1

		// Render deleted lines that appear before this line
		if dels, ok := t.deletedAfter[lineNum-1]; ok {
			cachedHL := t.deletedLineCache[lineNum-1]
			for di, del := range dels {
				var cached string
				if cachedHL != nil && di < len(cachedHL) {
					cached = cachedHL[di]
				}
				for wi, delContent := range deletedDisplayLines(del.Content, cached, del.Inline, isMarkdown, textWidth) {
					if wi == 0 {
						delMarker := diffDeletedGutter.Render("-")
						delNum := diffDeletedLineNum.Render(fmt.Sprintf("%d", del.OldLineNum))
						fmt.Fprintf(&b, "%s%s %s\n", delMarker, delNum, delContent)
					} else {
						fmt.Fprintf(&b, " %s %s\n", continuationGutter, delContent)
					}
				}
			}
		}

		isCursor := lineNum == t.cursorLine
		isSelected := t.selecting && lineNum >= selStart && lineNum <= selEnd
		isSidebarHighlight := sidebarHighlightStart > 0 && lineNum >= sidebarHighlightStart && lineNum <= sidebarHighlightEnd
		isChanged := t.changedLines != nil && t.changedLines[lineNum]
		inlineChanges := t.inlineChanges[lineNum]

		// Marker column
		var marker string
		if isCursor && !t.cursorOnAnnotation {
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
					fmt.Fprintf(&b, "%s%s %s\n", marker, numStr, styledLine)
				} else {
					fmt.Fprintf(&b, " %s %s\n", continuationGutter, styledLine)
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

			b.WriteString(fmt.Sprintf("%s%s %s\n", marker, numStr, styledLine))
		} else {
			// Get the display content: Chroma-highlighted or raw
			displayLine := line
			if !isMarkdown && chromaLines != nil && i < len(chromaLines) {
				displayLine = chromaLines[i]
			}

			// For Chroma-highlighted content, we skip wrapping (ANSI codes break lipgloss.Wrap)
			// and apply background overlays directly.
			if !isMarkdown && chromaLines != nil && i < len(chromaLines) {
				styledLine := displayLine
				if isSelected {
					styledLine = inlineBackground(selectedLineBg, styledLine)
				} else if isSidebarHighlight {
					styledLine = inlineBackground(sidebarHighlightBg, styledLine)
				} else if isChanged {
					styledLine = inlineBackground(diffChangedLineBg, styledLine)
				}
				b.WriteString(fmt.Sprintf("%s%s %s\n", marker, numStr, styledLine))
			} else {
				// Markdown or plain text path with wrapping
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

				wrapped := lipgloss.Wrap(line, textWidth, "")
				wrappedLines := strings.Split(wrapped, "\n")
				for wi, wl := range wrappedLines {
					if wi == 0 {
						b.WriteString(fmt.Sprintf("%s%s %s\n", marker, numStr, styleFunc(wl)))
					} else {
						b.WriteString(fmt.Sprintf(" %s %s\n", continuationGutter, styleFunc(wl)))
					}
				}
			}
		}

		// Render inline annotations after this line
		if anns, ok := annosByEndLine[lineNum]; ok {
			for idx, ann := range anns {
				focused := m.focused == contentPane && t.cursorOnAnnotation && t.cursorLine == lineNum && t.cursorAnnoIdx == idx
				b.WriteString(m.renderAnnotationBox(ann, boxWidth, focused))
			}
		}
	}

	m.contentViewport.SetContent(b.String())
}

// annotationExtraLines counts the lines an annotation box renders beyond the
// classic "body + label + borders" baseline, keeping scroll math in sync with
// renderAnnotationBox.
func annotationExtraLines(ann annotation) int {
	return len(ann.replies)
}

// renderAnnotationBox renders a bordered annotation box indented under the gutter.
func (m *AppModel) renderAnnotationBox(ann annotation, maxWidth int, focused bool) string {
	var lineLabel string
	if ann.endLine > ann.line {
		lineLabel = fmt.Sprintf("L%d-%d", ann.line, ann.endLine)
	} else {
		lineLabel = fmt.Sprintf("L%d", ann.line)
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
	boxContent.WriteString(header + "\n")
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
	patched := strings.ReplaceAll(content, "\033[0m", "\033[0m"+bgAnsi)
	return bgAnsi + patched + "\033[0m"
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
	return strings.Split(lipgloss.Wrap(b.String(), width, ""), "\n")
}

func deletedDisplayLines(content, cached string, inline []gitpkg.InlineSegment, isMarkdown bool, width int) []string {
	if len(inline) > 0 {
		return inlineDiffDisplayLines(inline, isMarkdown, width, diffCommonTextBg, diffDeletedTextBg)
	}
	if cached != "" {
		return []string{inlineBackground(diffDeletedLineBg, cached)}
	}
	if !isMarkdown {
		return []string{inlineBackground(diffDeletedLineBg, content)}
	}

	lines := strings.Split(lipgloss.Wrap(content, width, ""), "\n")
	for i := range lines {
		lines[i] = inlineBackground(diffDeletedLineBg, highlightMarkdown(lines[i]))
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

	line = reLink.ReplaceAllStringFunc(line, func(match string) string {
		idx := strings.Index(match, "](")
		if idx < 0 {
			return match
		}
		text := match[1:idx]
		return mdLinkStyle.Render(text)
	})

	return line
}

func (m *AppModel) scrollToCursor() {
	t := m.tab()
	if t.doc == nil {
		return
	}

	renderedLine := 0
	extraCounts := m.extraLinesPerDocLine()
	for i := 1; i < t.cursorLine; i++ {
		renderedLine++
		renderedLine += extraCounts[i]
	}

	cursorBottom := renderedLine + 1 + extraCounts[t.cursorLine]

	vpHeight := m.contentViewport.Height()
	currentTop := m.contentViewport.YOffset()

	if renderedLine < currentTop {
		m.contentViewport.SetYOffset(renderedLine)
	}
	if cursorBottom > currentTop+vpHeight {
		m.contentViewport.SetYOffset(cursorBottom - vpHeight)
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

	extraCounts := m.extraLinesPerDocLine()

	// Compute rendered line for chunk start - padding
	startLine := chunk.startLine - chunkScrollPadding
	if startLine < 1 {
		startLine = 1
	}
	startRendered := 0
	for i := 1; i < startLine; i++ {
		startRendered++
		startRendered += extraCounts[i]
	}

	// Compute rendered line for chunk end + padding
	endLine := chunk.endLine + chunkScrollPadding
	if endLine > t.doc.LineCount() {
		endLine = t.doc.LineCount()
	}
	endRendered := 0
	for i := 1; i <= endLine; i++ {
		endRendered++
		endRendered += extraCounts[i]
	}

	vpHeight := m.contentViewport.Height()

	// If the whole chunk+padding fits, position start at top
	if endRendered-startRendered <= vpHeight {
		m.contentViewport.SetYOffset(startRendered)
	} else {
		// Chunk is taller than viewport — just put cursor near top with padding
		m.contentViewport.SetYOffset(startRendered)
	}
}

func (m *AppModel) scrollToAnnotation(startLine, endLine int) {
	t := m.tab()
	if t.doc == nil {
		return
	}
	if endLine == 0 {
		endLine = startLine
	}

	extraCounts := m.extraLinesPerDocLine()

	startRendered := 0
	for i := 1; i < startLine; i++ {
		startRendered++
		startRendered += extraCounts[i]
	}

	endRendered := 0
	for i := 1; i <= endLine; i++ {
		endRendered++
		endRendered += extraCounts[i]
	}

	vpHeight := m.contentViewport.Height()

	offset := endRendered - vpHeight
	if offset < 0 {
		offset = 0
	}
	if offset > startRendered {
		offset = startRendered
	}

	m.contentViewport.SetYOffset(offset)
}

func (m *AppModel) extraLinesPerDocLine() map[int]int {
	t := m.tab()
	counts := make(map[int]int)
	if t.doc == nil {
		return counts
	}

	contentWidth := m.contentViewport.Width()
	textWidth := contentWidth - 8
	if textWidth < 10 {
		textWidth = 10
	}

	// Mirror rebuildContent: table rows and Chroma-highlighted lines are
	// rendered as a single line each; only the markdown/plain-text path wraps.
	inTable := make(map[int]bool)
	for _, tb := range detectTableBlocks(t.doc.Lines) {
		for l := tb.startLine; l <= tb.endLine; l++ {
			inTable[l] = true
		}
	}
	for i, line := range t.doc.Lines {
		lineNum := i + 1
		if inTable[lineNum] {
			continue
		}
		if !t.isMarkdown && t.chromaLines != nil && i < len(t.chromaLines) {
			continue
		}
		wrapped := lipgloss.Wrap(line, textWidth, "")
		wrapCount := strings.Count(wrapped, "\n")
		if wrapCount > 0 {
			counts[lineNum] += wrapCount
		}
	}

	if t.state != nil {
		for _, c := range t.state.Comments {
			bodyLines := strings.Count(c.Body, "\n") + 1
			counts[c.EndAt()] += bodyLines + 3 + annotationExtraLines(newAnnotation(c))
		}
	}

	// Account for deleted lines rendered before each doc line.
	if t.deletedAfter != nil {
		for afterLine, dels := range t.deletedAfter {
			targetLine := afterLine + 1
			if targetLine < 1 {
				targetLine = 1
			}
			for _, del := range dels {
				counts[targetLine] += len(deletedDisplayLines(del.Content, "", del.Inline, t.isMarkdown, textWidth))
			}
		}
	}

	return counts
}

func (m *AppModel) updateCommentSidebar() {
	t := m.tab()
	if t.state == nil {
		return
	}

	t.sidebarItems = nil
	for _, c := range t.state.Comments {
		t.sidebarItems = append(t.sidebarItems, sidebarItem{
			id: c.ID, line: c.StartLine, endLine: c.EndLine,
			body: c.Body, author: c.Author, resolved: c.Resolved,
			replies: c.Replies,
		})
	}
	sort.Slice(t.sidebarItems, func(i, j int) bool { return t.sidebarItems[i].line < t.sidebarItems[j].line })

	if t.sidebarCursor >= len(t.sidebarItems) {
		t.sidebarCursor = len(t.sidebarItems) - 1
	}
	if t.sidebarCursor < 0 {
		t.sidebarCursor = 0
	}

	var b strings.Builder

	if len(t.sidebarItems) == 0 {
		b.WriteString(commentStyle.Render("No comments yet.\n\nPress enter to comment.\n\nUse 'v' to select\nmultiple lines first."))
		m.commentViewport.SetContent(b.String())
		return
	}

	for idx, it := range t.sidebarItems {
		isSelected := m.focused == commentPane && idx == t.sidebarCursor

		var lineInfo string
		if it.endLine > it.line {
			lineInfo = fmt.Sprintf("L%d-%d", it.line, it.endLine)
		} else {
			lineInfo = fmt.Sprintf("L%d", it.line)
		}
		lineInfo = commentLineStyle.Render(lineInfo)
		if it.author != "" {
			lineInfo += " " + commentLineStyle.Render(it.author)
		}
		if it.resolved {
			lineInfo += " " + resolvedBadge.Render("✓")
		}

		cursorCol := lipgloss.NewStyle().Width(2)
		prefix := cursorCol.Render("")
		if isSelected {
			prefix = cursorCol.Render(cursorMarker.Render(">"))
		}

		b.WriteString(fmt.Sprintf("%s%s\n", prefix, lineInfo))

		clamped := clampLines(it.body, 3)
		bodyLines := strings.Split(clamped, "\n")
		for i, bl := range bodyLines {
			styled := bl
			if isSelected {
				styled = sidebarSelectedText.Render(bl)
			} else {
				styled = commentStyle.Render(bl)
			}
			b.WriteString(" " + styled)
			if i < len(bodyLines)-1 {
				b.WriteString("\n")
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
			b.WriteString("\n " + replyStyle.Render(fmt.Sprintf("↳ %s: %s", who, reply)))
		}
		b.WriteString("\n\n")
	}

	m.commentViewport.SetContent(b.String())
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

	t := m.tab()

	// Header
	commentCount := len(t.state.Comments)
	displayPath := t.path
	if m.filePath != "" {
		displayPath = m.filePath
	}
	var headerContent string
	if t.selecting {
		start, end := m.selectionRange()
		selLabel := visualModeIndicator.Render("VISUAL")
		headerContent = fmt.Sprintf(" Crit: %s  %s L%d-%d", displayPath, selLabel, start, end)
	} else if t.doc != nil {
		headerContent = fmt.Sprintf(" Crit: %s  %d comments  L%d/%d", displayPath, commentCount, t.cursorLine, t.doc.LineCount())
	} else {
		headerContent = fmt.Sprintf(" Crit: %s  %d comments", displayPath, commentCount)
	}
	var header string
	if m.detached {
		pausedBanner := pausedStatusBar.Width(m.width).Render(" AI agent is paused — review the document, then press q to submit")
		header = pausedBanner + "\n" + headerStyle.Width(m.width).Render(headerContent)
	} else {
		header = headerStyle.Width(m.width).Render(headerContent)
	}

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
	sections = append(sections, mainRow, footer)

	full := lipgloss.JoinVertical(lipgloss.Left, sections...)

	if m.modal != noModal {
		full = m.renderWithModal(full)
	}

	v := tea.NewView(full)
	v.AltScreen = true
	return v
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

	// Disambiguate filenames — use basename unless there are collisions
	basenames := make(map[string]int)
	for _, t := range m.tabs {
		base := filepath.Base(t.path)
		basenames[base]++
	}

	type tabLabel struct {
		text     string
		rendered string
		width    int
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
	// Render a single tab with correct border corners for its position.
	// isFirst adjusts the left corner to connect to the outer frame border.
	renderTab := func(i int, isFirst bool) string {
		var style lipgloss.Style
		isActive := i == m.activeTab
		if isActive {
			style = activeTabStyle
		} else {
			style = inactiveTabStyle
		}
		border, _, _, _, _ := style.GetBorder()
		if isFirst && isActive {
			border.BottomLeft = "│"
		} else if isFirst && !isActive {
			border.BottomLeft = "├"
		}
		style = style.Border(border)
		return style.Render(labels[i].text)
	}
	for i := range labels {
		rendered := renderTab(i, i == 0)
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

	// Check if all tabs fit
	totalWidth := 0
	for _, l := range labels {
		totalWidth += l.width
	}

	if totalWidth <= m.width {
		var tabs []string
		for i := range labels {
			tabs = append(tabs, labels[i].rendered)
		}
		row := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
		return addFiller(row)
	}

	// Overflow: show a window of tabs centered on the active tab.
	// Indicators are styled as bordered tabs to align with the tab row.
	renderIndicator := func(text string, isFirst bool) string {
		style := inactiveTabStyle.Foreground(subtle)
		border, _, _, _, _ := style.GetBorder()
		if isFirst {
			border.BottomLeft = "├"
		}
		style = style.Border(border)
		return style.Render(text)
	}

	leftIndicator := ""
	rightIndicator := ""
	leftW := 0
	rightW := 0
	// Pre-render indicators to know their width for available space calc
	if m.activeTab > 0 {
		leftIndicator = renderIndicator(fmt.Sprintf("↤ %d more", m.activeTab), true)
		leftW = lipgloss.Width(leftIndicator)
	}
	if m.activeTab < len(labels)-1 {
		rightIndicator = renderIndicator(fmt.Sprintf("%d more ↦", len(labels)-m.activeTab-1), false)
		rightW = lipgloss.Width(rightIndicator)
	}

	availWidth := m.width - leftW - rightW

	// Find the window of tabs that fits
	start := m.activeTab
	end := m.activeTab + 1
	used := labels[m.activeTab].width

	// Expand window outward from active tab
	for {
		expanded := false
		if start > 0 && used+labels[start-1].width <= availWidth {
			start--
			used += labels[start].width
			expanded = true
		}
		if end < len(labels) && used+labels[end].width <= availWidth {
			used += labels[end].width
			end++
			expanded = true
		}
		if !expanded {
			break
		}
	}

	// Re-render indicators with actual counts now that we know the visible window
	var parts []string
	if start > 0 {
		ind := renderIndicator(fmt.Sprintf("↤ %d more", start), true)
		parts = append(parts, ind)
	}
	for i := start; i < end; i++ {
		parts = append(parts, renderTab(i, i == start && start == 0))
	}
	if end < len(labels) {
		ind := renderIndicator(fmt.Sprintf("%d more ↦", len(labels)-end), false)
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
			k("j/k", "extend"),
			k("enter", "comment selection"),
			k("esc", "cancel"),
			k("v", "toggle select"),
		}
	} else {
		items = []string{
			k("j/k", "move"),
			k("</>", "file top/bottom"),
			k("[/]", "prev/next comment"),
			k("shift+↑↓", "page"),
			k("s", "sidebar"),
			k("v", "select"),
			k("enter", "comment"),
		}
		if len(t.sidebarItems) > 0 {
			items = append(items, k("r", "resolve/unresolve"))
		}
		items = append(items, k("q", "save & quit"))
		if m.multiFile {
			items = append([]string{
				k("tab/S-tab", "next/prev tab"),
				k("n/N", "next/prev change"),
			}, items...)
		}
	}

	return footerStyle.Width(m.width).Render(strings.Join(items, "  "))
}

func (m AppModel) renderModalButton(label, hint string, focused bool) string {
	btn := modalBtnLabel.Render(label)
	h := modalBtnHint.Render(hint)
	content := btn + " " + h
	if focused {
		return modalBtnFocused.Render(content)
	}
	return modalBtnNormal.Render(content)
}

func (m AppModel) renderDeleteButton(label string, focused bool) string {
	if focused {
		return modalDeleteBtnFocused.Render(label)
	}
	return modalBtnNormal.Render(modalDeleteBtnLabel.Render(label))
}

func (m AppModel) renderContextPreview(start, end, maxWidth int) string {
	t := m.tabs[m.activeTab]
	if t.doc == nil {
		return ""
	}
	var lines []string
	maxLineText := maxWidth - 7
	if maxLineText < 10 {
		maxLineText = 10
	}
	for i := start; i <= end && i <= t.doc.LineCount(); i++ {
		lineText := t.doc.LineAt(i)
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
	if len(lines) > 8 {
		lines = append(lines[:7], footerStyle.Render(fmt.Sprintf("  ... +%d more lines", len(lines)-7)))
	}
	return strings.Join(lines, "\n")
}

func (m AppModel) renderWithModal(background string) string {
	var modalContent string
	modalWidth := m.width * 2 / 3
	if modalWidth < 50 {
		modalWidth = 50
	}
	if modalWidth > m.width-4 {
		modalWidth = m.width - 4
	}
	innerWidth := modalWidth - 6

	switch m.modal {
	case commentModal:
		start, end := m.selectionRange()
		var title string
		if start != end {
			title = modalTitleStyle.Render(fmt.Sprintf("Add Comment (lines %d-%d)", start, end))
		} else {
			title = modalTitleStyle.Render(fmt.Sprintf("Add Comment (line %d)", start))
		}
		contextBox := contextBoxStyle.
			Width(innerWidth - 2).
			Render(m.renderContextPreview(start, end, innerWidth-4))

		saveBtn := m.renderModalButton("Save", "ctrl+s", m.modalFocus == 1)
		cancelBtn := m.renderModalButton("Cancel", "esc", m.modalFocus == 2)
		buttons := lipgloss.JoinHorizontal(lipgloss.Center, saveBtn, "  ", cancelBtn)

		modalContent = modalStyle.Width(modalWidth).Render(
			title + "\n" + contextBox + "\n\n" + m.modalTextarea.View() + "\n\n" + buttons)

	case replyModal, editModal:
		titleText := "Edit Comment"
		if m.modal == replyModal {
			titleText = "Add Reply"
		} else if m.editingReplyID != "" {
			titleText = "Edit Reply"
		}
		title := modalTitleStyle.Render(titleText)
		var contextSection, threadSection string
		for _, c := range m.tabs[m.activeTab].state.Comments {
			if c.ID == m.editingID {
				start := c.StartLine
				end := c.EndAt()
				contextSection = contextBoxStyle.
					Width(innerWidth - 2).
					Render(m.renderContextPreview(start, end, innerWidth-4))
				if m.modal == replyModal || m.editingReplyID != "" {
					var thread strings.Builder
					author := c.Author
					if author == "" {
						author = "comment"
					}
					thread.WriteString(commentLineStyle.Render("Comment — "+author) + "\n")
					thread.WriteString(clampLines(c.Body, 3))
					for i, reply := range c.Replies {
						body := reply.Body
						if newline := strings.IndexByte(body, '\n'); newline >= 0 {
							body = body[:newline] + "…"
						}
						replyAuthor := reply.Author
						if replyAuthor == "" {
							replyAuthor = "reply"
						}
						prefix := "  "
						if reply.ID == m.editingReplyID {
							prefix = "> "
						}
						thread.WriteString("\n" + replyStyle.Render(
							fmt.Sprintf("%s%d. %s: %s", prefix, i+1, replyAuthor, body)))
					}
					threadSection = contextBoxStyle.
						Width(innerWidth - 2).
						Render(thread.String())
				}
				break
			}
		}
		saveBtn := m.renderModalButton("Save", "ctrl+s", m.modalFocus == 1)
		cancelBtn := m.renderModalButton("Cancel", "esc", m.modalFocus == 2)
		buttonRows := []string{lipgloss.JoinHorizontal(lipgloss.Center, saveBtn, "  ", cancelBtn)}
		for i, target := range m.modalDeleteTargets() {
			buttonRows = append(buttonRows, m.renderDeleteButton(target.label, m.modalFocus == i+3))
		}

		content := title + "\n"
		if contextSection != "" {
			content += contextSection + "\n\n"
		}
		if threadSection != "" {
			content += threadSection + "\n\n"
		}
		content += m.modalTextarea.View() + "\n\n" + strings.Join(buttonRows, "\n")
		modalContent = modalStyle.Width(modalWidth).Render(content)

	case finishModal:
		unresolved := m.unresolvedTotal()
		var title, info, confirmLabel string
		if unresolved == 0 {
			title = modalTitleStyle.Render("Approve review?")
			info = "No unresolved comments — approving ends the review."
			confirmLabel = "Approve"
		} else if !m.newFeedback {
			title = modalTitleStyle.Render("Resolve all & Approve?")
			info = fmt.Sprintf("%d unresolved comment(s) will be resolved.", unresolved)
			confirmLabel = "Resolve All & Approve"
		} else {
			title = modalTitleStyle.Render("Finish review?")
			info = fmt.Sprintf("%d unresolved comment(s) will be sent to the agent.", unresolved)
			confirmLabel = "Finish Review"
		}

		confirmBtn := m.renderModalButton(confirmLabel, "y", m.modalFocus == 0)
		cancelBtn := m.renderModalButton("Keep reviewing", "n", m.modalFocus == 1)
		buttons := lipgloss.JoinHorizontal(lipgloss.Center, confirmBtn, "  ", cancelBtn)
		hint := footerStyle.Render("esc: back to review · q: quit without finishing")

		modalContent = modalStyle.Width(modalWidth).Render(
			title + "\n" + info + "\n\n" + buttons + "\n" + hint)
	}

	bgW := lipgloss.Width(background)
	bgH := lipgloss.Height(background)

	modalW := lipgloss.Width(modalContent)
	modalH := lipgloss.Height(modalContent)

	mx := (bgW - modalW) / 2
	my := (bgH - modalH) / 2
	if mx < 0 {
		mx = 0
	}
	if my < 0 {
		my = 0
	}

	background = dimRendered(background, bgW, bgH)

	bgLayer := lipgloss.NewLayer(background)
	modalLayer := lipgloss.NewLayer(modalContent).X(mx).Y(my).Z(1)

	comp := lipgloss.NewCompositor(bgLayer, modalLayer)
	return comp.Render()
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
